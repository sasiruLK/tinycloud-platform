package infra

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

// sdkSources implements every Sources interface against the real OCI SDK.
//
// This is the one substrate-specific read path still linked into Core, kept so
// that an Instance already running on Oracle Cloud loses no capability the day
// Providers arrive. It is wired only when its configuration is supplied, and
// it is scheduled to move out to an Infra Provider of its own, at which point
// Core holds no cloud credentials at all.
//
// Only the sub-packages actually used are imported: the SDK ships a package per
// service and importing the umbrella would drag hundreds of them into the
// binary.
type sdkSources struct {
	cfg     Config
	compute core.ComputeClient
	vnet    core.VirtualNetworkClient
	metrics monitoring.MonitoringClient
	nlb     networkloadbalancer.NetworkLoadBalancerClient
	objects objectstorage.ObjectStorageClient
}

// Configured reports whether any Oracle Cloud read is configured at all. With
// nothing configured — the state of the published image — no credential is
// resolved and no account is contacted.
func (c Config) Configured() bool {
	return c.CompartmentID != "" || c.NetworkLoadBalancerID != "" ||
		(c.ObjectStorageNamespace != "" && c.Bucket != "")
}

// NewSources builds SDK-backed sources authenticated as the instance itself,
// for the Capabilities cfg names. A Capability whose identifiers are absent
// yields a nil source, which the collector reports as a warning: an Instance
// that configures only a bucket gets backups and nothing else.
//
// The credential comes from the metadata service at 169.254.169.254, so this
// only succeeds on an OCI instance whose dynamic group is covered by an IAM
// policy. On a developer laptop it fails here, with the metadata error
// attached — that is the expected outcome, and the caller degrades to an
// explanatory error on /v1/infra rather than failing to start.
func NewSources(cfg Config) (Sources, error) {
	if !cfg.Configured() {
		return Sources{}, nil
	}
	provider, err := auth.InstancePrincipalConfigurationProvider()
	if err != nil {
		return Sources{}, fmt.Errorf("instance principal authentication unavailable (this only works on an OCI instance): %w", err)
	}
	return newSourcesWithProvider(cfg, provider)
}

func newSourcesWithProvider(cfg Config, provider common.ConfigurationProvider) (Sources, error) {
	compute, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return Sources{}, fmt.Errorf("compute client: %w", err)
	}
	vnet, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return Sources{}, fmt.Errorf("virtual network client: %w", err)
	}
	metrics, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		return Sources{}, fmt.Errorf("monitoring client: %w", err)
	}
	nlb, err := networkloadbalancer.NewNetworkLoadBalancerClientWithConfigurationProvider(provider)
	if err != nil {
		return Sources{}, fmt.Errorf("network load balancer client: %w", err)
	}
	objects, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		return Sources{}, fmt.Errorf("object storage client: %w", err)
	}

	s := &sdkSources{cfg: cfg, compute: compute, vnet: vnet, metrics: metrics, nlb: nlb, objects: objects}

	// Only the reads whose identifiers were supplied are wired up. The rest
	// stay nil and are reported as unconfigured rather than attempted.
	src := Sources{}
	if cfg.CompartmentID != "" {
		src.Instances, src.Metrics, src.Alarms = s, s, s
	}
	if cfg.NetworkLoadBalancerID != "" {
		src.Ingress = s
	}
	if cfg.ObjectStorageNamespace != "" && cfg.Bucket != "" {
		src.Backups = s
	}
	return src, nil
}

// ListInstances returns every instance in the compartment that still exists,
// with its private IP resolved through its primary VNIC.
func (s *sdkSources) ListInstances(ctx context.Context) ([]InstanceInfo, error) {
	var instances []InstanceInfo
	var page *string
	for {
		res, err := s.compute.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: &s.cfg.CompartmentID,
			Page:          page,
		})
		if err != nil {
			return nil, fmt.Errorf("list instances: %w", err)
		}
		for _, item := range res.Items {
			if item.LifecycleState == core.InstanceLifecycleStateTerminated {
				continue
			}
			info := InstanceInfo{
				ID:    deref(item.Id),
				Name:  deref(item.DisplayName),
				State: string(item.LifecycleState),
				Shape: deref(item.Shape),

				FaultDomain: deref(item.FaultDomain),
			}
			if item.ShapeConfig != nil {
				if item.ShapeConfig.Ocpus != nil {
					info.OCPUs = float64(*item.ShapeConfig.Ocpus)
				}
				if item.ShapeConfig.MemoryInGBs != nil {
					info.MemoryGB = float64(*item.ShapeConfig.MemoryInGBs)
				}
			}
			instances = append(instances, info)
		}
		if res.OpcNextPage == nil {
			break
		}
		page = res.OpcNextPage
	}

	s.attachPrivateIPs(ctx, instances)
	return instances, nil
}

// attachPrivateIPs fills in each instance's private IP. A VNIC lookup that
// fails leaves the address empty rather than failing the whole list: the node's
// shape and state are still worth showing.
func (s *sdkSources) attachPrivateIPs(ctx context.Context, instances []InstanceInfo) {
	vnicByInstance := map[string]string{}
	var page *string
	for {
		res, err := s.compute.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
			CompartmentId: &s.cfg.CompartmentID,
			Page:          page,
		})
		if err != nil {
			return
		}
		for _, att := range res.Items {
			if att.VnicId == nil || att.InstanceId == nil {
				continue
			}
			if _, seen := vnicByInstance[*att.InstanceId]; !seen {
				vnicByInstance[*att.InstanceId] = *att.VnicId
			}
		}
		if res.OpcNextPage == nil {
			break
		}
		page = res.OpcNextPage
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := range instances {
		vnicID, ok := vnicByInstance[instances[i].ID]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			res, err := s.vnet.GetVnic(ctx, core.GetVnicRequest{VnicId: &id})
			if err != nil || res.PrivateIp == nil {
				return
			}
			mu.Lock()
			instances[idx].PrivateIP = *res.PrivateIp
			mu.Unlock()
		}(i, vnicID)
	}
	wg.Wait()
}

// Oracle Monitoring namespaces and the MQL behind each contract metric. The
// query language is Oracle's; the metric names are the contract's, so this
// mapping is where one stops and the other begins.
const (
	nsCompute = "oci_computeagent"
	nsNLB     = "oci_nlb"
	nsAPM     = "oracle_apm_synthetics"
)

// ociQuery is one contract metric expressed in Oracle's terms.
type ociQuery struct {
	namespace string
	query     string
	// dimensions renames Oracle's dimension keys to the contract's.
	dimensions map[string]string
}

var ociQueries = map[string]ociQuery{
	MetricCPUUtilization: {
		namespace:  nsCompute,
		query:      "CpuUtilization[5m].groupBy(resourceDisplayName).mean()",
		dimensions: map[string]string{"resourceDisplayName": DimInstance},
	},
	MetricMemoryUtilization: {
		namespace:  nsCompute,
		query:      "MemoryUtilization[5m].groupBy(resourceDisplayName).mean()",
		dimensions: map[string]string{"resourceDisplayName": DimInstance},
	},
	MetricHealthyBackends: {
		namespace:  nsNLB,
		query:      "HealthyBackends[5m].groupBy(backendSetName).max()",
		dimensions: map[string]string{"backendSetName": DimBackendSet},
	},
	MetricUnhealthyBackends: {
		namespace:  nsNLB,
		query:      "UnhealthyBackends[5m].groupBy(backendSetName).max()",
		dimensions: map[string]string{"backendSetName": DimBackendSet},
	},
	MetricUptimeAvailability: {
		namespace:  nsAPM,
		query:      "Availability[1h].groupBy(MonitorName,Target).mean()",
		dimensions: map[string]string{"MonitorName": DimMonitor, "Target": DimTarget},
	},
}

// QueryMetric answers one contract metric out of Oracle Monitoring, returning
// the newest datapoint of each result series. A metric Oracle has no query for
// yields no series rather than an error: absent is not broken.
func (s *sdkSources) QueryMetric(ctx context.Context, metric string, window time.Duration) ([]Series, error) {
	q, ok := ociQueries[metric]
	if !ok {
		return nil, nil
	}

	end := time.Now().UTC()
	start := end.Add(-window)

	res, err := s.metrics.SummarizeMetricsData(ctx, monitoring.SummarizeMetricsDataRequest{
		CompartmentId: &s.cfg.CompartmentID,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: &q.namespace,
			Query:     &q.query,
			StartTime: &common.SDKTime{Time: start},
			EndTime:   &common.SDKTime{Time: end},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", metric, err)
	}

	series := make([]Series, 0, len(res.Items))
	for _, item := range res.Items {
		latest, ok := newestDatapoint(item.AggregatedDatapoints)
		if !ok {
			continue
		}
		series = append(series, Series{
			Dimensions: renameDimensions(item.Dimensions, q.dimensions),
			Timestamp:  latest.Timestamp.Time.UTC(),
			Value:      *latest.Value,
		})
	}
	return series, nil
}

// renameDimensions maps a vendor's dimension keys onto the contract's, dropping
// the ones the contract does not name.
func renameDimensions(dims map[string]string, rename map[string]string) map[string]string {
	out := make(map[string]string, len(rename))
	for from, to := range rename {
		if v, ok := dims[from]; ok {
			out[to] = v
		}
	}
	return out
}

func newestDatapoint(points []monitoring.AggregatedDatapoint) (monitoring.AggregatedDatapoint, bool) {
	var best monitoring.AggregatedDatapoint
	found := false
	for _, p := range points {
		if p.Value == nil || p.Timestamp == nil {
			continue
		}
		if found && !p.Timestamp.Time.After(best.Timestamp.Time) {
			continue
		}
		best, found = p, true
	}
	return best, found
}

// ListAlarmStatuses returns each alarm's current state.
func (s *sdkSources) ListAlarmStatuses(ctx context.Context) ([]AlarmStatus, error) {
	var out []AlarmStatus
	var page *string
	for {
		res, err := s.metrics.ListAlarmsStatus(ctx, monitoring.ListAlarmsStatusRequest{
			CompartmentId: &s.cfg.CompartmentID,
			Page:          page,
		})
		if err != nil {
			return nil, fmt.Errorf("list alarm status: %w", err)
		}
		for _, item := range res.Items {
			out = append(out, AlarmStatus{
				Name:     deref(item.DisplayName),
				Severity: string(item.Severity),
				Status:   string(item.Status),
			})
		}
		if res.OpcNextPage == nil {
			break
		}
		page = res.OpcNextPage
	}
	return out, nil
}

// IngressPublicIP returns the network load balancer's public address.
func (s *sdkSources) IngressPublicIP(ctx context.Context) (string, error) {
	res, err := s.nlb.GetNetworkLoadBalancer(ctx, networkloadbalancer.GetNetworkLoadBalancerRequest{
		NetworkLoadBalancerId: &s.cfg.NetworkLoadBalancerID,
	})
	if err != nil {
		return "", fmt.Errorf("get network load balancer: %w", err)
	}
	for _, ip := range res.IpAddresses {
		if ip.IsPublic != nil && *ip.IsPublic && ip.IpAddress != nil {
			return *ip.IpAddress, nil
		}
	}
	return "", fmt.Errorf("network load balancer %s has no public IP", s.cfg.NetworkLoadBalancerID)
}

// ListObjects walks the whole backup bucket. Only name, size and modification
// time are requested; the default listing omits size, which the capacity gauge
// needs.
func (s *sdkSources) ListObjects(ctx context.Context) ([]ObjectInfo, error) {
	fields := "name,size,timeModified"
	limit := 1000

	var out []ObjectInfo
	var start *string
	for {
		res, err := s.objects.ListObjects(ctx, objectstorage.ListObjectsRequest{
			NamespaceName: &s.cfg.ObjectStorageNamespace,
			BucketName:    &s.cfg.Bucket,
			Fields:        &fields,
			Limit:         &limit,
			Start:         start,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects in %s: %w", s.cfg.Bucket, err)
		}
		for _, o := range res.Objects {
			info := ObjectInfo{Name: deref(o.Name)}
			if o.Size != nil {
				info.Size = *o.Size
			}
			if o.TimeModified != nil {
				info.Modified = o.TimeModified.Time.UTC()
			}
			out = append(out, info)
		}
		if res.NextStartWith == nil || *res.NextStartWith == "" {
			break
		}
		start = res.NextStartWith
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
