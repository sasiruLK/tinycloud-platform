// Package oci is the Infra Provider for an Oracle Cloud tenancy.
//
// It answers the `/v0` contract from the Oracle Cloud SDK: instances and their
// private addresses from Compute, utilisation and alarms from Monitoring, the
// public address from a Network Load Balancer, and backup objects from Object
// Storage.
//
// This code used to live in core, as the one Substrate-specific read path
// linked into it. Moving it here is what makes core's description of itself
// true: core now speaks to every Substrate through the same contract, and holds
// no cloud credentials on any of them.
//
// Only the SDK sub-packages actually used are imported. The SDK ships a package
// per service and importing the umbrella would drag hundreds of them into the
// binary.
package oci

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

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
)

// Config names the Oracle Cloud resources this Provider reads. Nothing here
// has a default: the identifiers belong to whoever deployed the Instance, and
// a Provider that contacted an account nobody named would be worse than one
// that serves nothing.
//
// Each identifier switches on the Capabilities it can answer, so a tenancy that
// supplies only a bucket gets a Provider that declares `backups` and honestly
// returns 501 for the rest.
type Config struct {
	// CompartmentID enables instances, metrics and alarms.
	CompartmentID string
	// NetworkLoadBalancerID enables ingress.
	NetworkLoadBalancerID string
	// ObjectStorageNamespace and Bucket together enable backups.
	ObjectStorageNamespace string
	Bucket                 string
}

// Capabilities is the subset of the Infra kind this configuration can serve,
// in contract order. It is what the Provider declares at GET /v0/capabilities.
func (c Config) Capabilities() []string {
	var caps []string
	if c.CompartmentID != "" {
		caps = append(caps, provider.CapabilityInstances, provider.CapabilityMetrics, provider.CapabilityAlarms)
	}
	if c.NetworkLoadBalancerID != "" {
		caps = append(caps, provider.CapabilityIngress)
	}
	if c.ObjectStorageNamespace != "" && c.Bucket != "" {
		caps = append(caps, provider.CapabilityBackups)
	}
	return caps
}

// Provider serves the Infra Capabilities an Oracle Cloud tenancy can answer.
type Provider struct {
	cfg     Config
	caps    []string
	compute ComputeAPI
	vnet    VnicAPI
	metrics MonitoringAPI
	nlb     LoadBalancerAPI
	objects ObjectStorageAPI
}

// Clients is the Oracle Cloud surface a Provider reads through. Production
// supplies real SDK clients; a test supplies fakes and needs no account.
type Clients struct {
	Compute       ComputeAPI
	Vnic          VnicAPI
	Monitoring    MonitoringAPI
	LoadBalancer  LoadBalancerAPI
	ObjectStorage ObjectStorageAPI
}

// The SDK surface this Provider uses, exported so that a test — and the
// conformance suite — can substitute it without an Oracle account. Each is the
// subset of one SDK client's methods actually called.
type (
	ComputeAPI interface {
		ListInstances(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error)
		ListVnicAttachments(context.Context, core.ListVnicAttachmentsRequest) (core.ListVnicAttachmentsResponse, error)
	}
	VnicAPI interface {
		GetVnic(context.Context, core.GetVnicRequest) (core.GetVnicResponse, error)
	}
	MonitoringAPI interface {
		SummarizeMetricsData(context.Context, monitoring.SummarizeMetricsDataRequest) (monitoring.SummarizeMetricsDataResponse, error)
		ListAlarmsStatus(context.Context, monitoring.ListAlarmsStatusRequest) (monitoring.ListAlarmsStatusResponse, error)
	}
	LoadBalancerAPI interface {
		GetNetworkLoadBalancer(context.Context, networkloadbalancer.GetNetworkLoadBalancerRequest) (networkloadbalancer.GetNetworkLoadBalancerResponse, error)
	}
	ObjectStorageAPI interface {
		ListObjects(context.Context, objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error)
	}
)

// New returns a Provider reading Oracle Cloud through clients.
func New(cfg Config, clients Clients) (*Provider, error) {
	if len(cfg.Capabilities()) == 0 {
		return nil, fmt.Errorf("no capability is configured: set at least one of OCI_COMPARTMENT_ID, OCI_NLB_ID, or both OCI_OBJECT_STORAGE_NAMESPACE and OCI_BACKUP_BUCKET")
	}
	return &Provider{
		cfg:     cfg,
		caps:    cfg.Capabilities(),
		compute: clients.Compute,
		vnet:    clients.Vnic,
		metrics: clients.Monitoring,
		nlb:     clients.LoadBalancer,
		objects: clients.ObjectStorage,
	}, nil
}

// NewFromInstancePrincipal returns a Provider authenticated as the instance it
// runs on.
//
// The credential comes from the metadata service at 169.254.169.254, so this
// succeeds only on an OCI instance whose dynamic group is covered by an IAM
// policy. On a laptop it fails here with the metadata error attached, which is
// the expected outcome: the Provider refuses to start rather than serving a
// Capability it cannot answer.
func NewFromInstancePrincipal(cfg Config) (*Provider, error) {
	if len(cfg.Capabilities()) == 0 {
		return nil, fmt.Errorf("no capability is configured: set at least one of OCI_COMPARTMENT_ID, OCI_NLB_ID, or both OCI_OBJECT_STORAGE_NAMESPACE and OCI_BACKUP_BUCKET")
	}
	auth, err := auth.InstancePrincipalConfigurationProvider()
	if err != nil {
		return nil, fmt.Errorf("instance principal authentication unavailable (this only works on an OCI instance): %w", err)
	}
	return newWithConfigurationProvider(cfg, auth)
}

func newWithConfigurationProvider(cfg Config, auth common.ConfigurationProvider) (*Provider, error) {
	compute, err := core.NewComputeClientWithConfigurationProvider(auth)
	if err != nil {
		return nil, fmt.Errorf("compute client: %w", err)
	}
	vnet, err := core.NewVirtualNetworkClientWithConfigurationProvider(auth)
	if err != nil {
		return nil, fmt.Errorf("virtual network client: %w", err)
	}
	metrics, err := monitoring.NewMonitoringClientWithConfigurationProvider(auth)
	if err != nil {
		return nil, fmt.Errorf("monitoring client: %w", err)
	}
	nlb, err := networkloadbalancer.NewNetworkLoadBalancerClientWithConfigurationProvider(auth)
	if err != nil {
		return nil, fmt.Errorf("network load balancer client: %w", err)
	}
	objects, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(auth)
	if err != nil {
		return nil, fmt.Errorf("object storage client: %w", err)
	}
	return New(cfg, Clients{
		Compute:       compute,
		Vnic:          vnet,
		Monitoring:    metrics,
		LoadBalancer:  nlb,
		ObjectStorage: objects,
	})
}

// Name identifies this Provider in Capability discovery and in the warnings an
// operator reads on the dashboard.
func (p *Provider) Name() string { return "oci" }

// Capabilities are the Capabilities this Provider serves.
func (p *Provider) Capabilities() []string { return p.caps }

// Instances returns every instance in the compartment that still exists, with
// its private IP resolved through its primary VNIC.
func (p *Provider) Instances(ctx context.Context) ([]infra.InstanceInfo, error) {
	var instances []infra.InstanceInfo
	var page *string
	for {
		res, err := p.compute.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: &p.cfg.CompartmentID,
			Page:          page,
		})
		if err != nil {
			return nil, provider.Upstream(fmt.Errorf("list instances: %w", err))
		}
		for _, item := range res.Items {
			if item.LifecycleState == core.InstanceLifecycleStateTerminated {
				continue
			}
			info := infra.InstanceInfo{
				ID:          deref(item.Id),
				Name:        deref(item.DisplayName),
				State:       string(item.LifecycleState),
				Shape:       deref(item.Shape),
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

	p.attachPrivateIPs(ctx, instances)
	return instances, nil
}

// attachPrivateIPs fills in each instance's private IP. A VNIC lookup that
// fails leaves the address empty rather than failing the whole list: the node's
// shape and state are still worth showing.
func (p *Provider) attachPrivateIPs(ctx context.Context, instances []infra.InstanceInfo) {
	vnicByInstance := map[string]string{}
	var page *string
	for {
		res, err := p.compute.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
			CompartmentId: &p.cfg.CompartmentID,
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
			res, err := p.vnet.GetVnic(ctx, core.GetVnicRequest{VnicId: &id})
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

// Oracle Monitoring namespaces behind the contract's metrics.
const (
	nsCompute = "oci_computeagent"
	nsNLB     = "oci_nlb"
	nsAPM     = "oracle_apm_synthetics"
)

// query is one contract metric expressed in Oracle's terms.
type query struct {
	namespace string
	mql       string
	// dimensions renames Oracle's dimension keys to the contract's.
	dimensions map[string]string
}

// queries is where the contract's vocabulary stops and Oracle's begins.
// Translating a metric name into a Substrate's query language is exactly the
// job ADR-0003 says belongs to a Provider rather than to the wire.
var queries = map[string]query{
	infra.MetricCPUUtilization: {
		namespace:  nsCompute,
		mql:        "CpuUtilization[5m].groupBy(resourceDisplayName).mean()",
		dimensions: map[string]string{"resourceDisplayName": infra.DimInstance},
	},
	infra.MetricMemoryUtilization: {
		namespace:  nsCompute,
		mql:        "MemoryUtilization[5m].groupBy(resourceDisplayName).mean()",
		dimensions: map[string]string{"resourceDisplayName": infra.DimInstance},
	},
	infra.MetricHealthyBackends: {
		namespace:  nsNLB,
		mql:        "HealthyBackends[5m].groupBy(backendSetName).max()",
		dimensions: map[string]string{"backendSetName": infra.DimBackendSet},
	},
	infra.MetricUnhealthyBackends: {
		namespace:  nsNLB,
		mql:        "UnhealthyBackends[5m].groupBy(backendSetName).max()",
		dimensions: map[string]string{"backendSetName": infra.DimBackendSet},
	},
	infra.MetricUptimeAvailability: {
		namespace:  nsAPM,
		mql:        "Availability[1h].groupBy(MonitorName,Target).mean()",
		dimensions: map[string]string{"MonitorName": infra.DimMonitor, "Target": infra.DimTarget},
	},
}

// Metric answers one contract metric out of Oracle Monitoring, returning the
// newest datapoint of each result series. A metric this tenancy has no query
// for yields no series rather than an error: absent is not broken.
func (p *Provider) Metric(ctx context.Context, metric string, window time.Duration) ([]infra.Series, error) {
	q, ok := queries[metric]
	if !ok {
		return nil, nil
	}

	end := time.Now().UTC()
	start := end.Add(-window)

	res, err := p.metrics.SummarizeMetricsData(ctx, monitoring.SummarizeMetricsDataRequest{
		CompartmentId: &p.cfg.CompartmentID,
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: &q.namespace,
			Query:     &q.mql,
			StartTime: &common.SDKTime{Time: start},
			EndTime:   &common.SDKTime{Time: end},
		},
	})
	if err != nil {
		return nil, provider.Upstream(fmt.Errorf("query %s: %w", metric, err))
	}

	series := make([]infra.Series, 0, len(res.Items))
	for _, item := range res.Items {
		latest, ok := newestDatapoint(item.AggregatedDatapoints)
		if !ok {
			continue
		}
		series = append(series, infra.Series{
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

// Alarms returns each alarm's current state.
func (p *Provider) Alarms(ctx context.Context) ([]infra.AlarmStatus, error) {
	var out []infra.AlarmStatus
	var page *string
	for {
		res, err := p.metrics.ListAlarmsStatus(ctx, monitoring.ListAlarmsStatusRequest{
			CompartmentId: &p.cfg.CompartmentID,
			Page:          page,
		})
		if err != nil {
			return nil, provider.Upstream(fmt.Errorf("list alarm status: %w", err))
		}
		for _, item := range res.Items {
			out = append(out, infra.AlarmStatus{
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

// IngressAddress returns the network load balancer's public address.
func (p *Provider) IngressAddress(ctx context.Context) (string, error) {
	res, err := p.nlb.GetNetworkLoadBalancer(ctx, networkloadbalancer.GetNetworkLoadBalancerRequest{
		NetworkLoadBalancerId: &p.cfg.NetworkLoadBalancerID,
	})
	if err != nil {
		return "", provider.Upstream(fmt.Errorf("get network load balancer: %w", err))
	}
	for _, ip := range res.IpAddresses {
		if ip.IsPublic != nil && *ip.IsPublic && ip.IpAddress != nil {
			return *ip.IpAddress, nil
		}
	}
	// Not an error. A load balancer whose public address has not been assigned
	// yet is the same case as a pending Kubernetes Service: absent, not broken.
	return "", nil
}

// BackupObjects walks the whole backup bucket, and names it. Only name, size
// and modification time are requested; the default listing omits size, which
// the capacity gauge needs.
func (p *Provider) BackupObjects(ctx context.Context) (infra.BackupListing, error) {
	fields := "name,size,timeModified"
	limit := 1000

	listing := infra.BackupListing{Store: p.cfg.Bucket}
	var start *string
	for {
		res, err := p.objects.ListObjects(ctx, objectstorage.ListObjectsRequest{
			NamespaceName: &p.cfg.ObjectStorageNamespace,
			BucketName:    &p.cfg.Bucket,
			Fields:        &fields,
			Limit:         &limit,
			Start:         start,
		})
		if err != nil {
			return infra.BackupListing{}, provider.Upstream(fmt.Errorf("list objects in %s: %w", p.cfg.Bucket, err))
		}
		for _, o := range res.Objects {
			info := infra.ObjectInfo{Name: deref(o.Name)}
			if o.Size != nil {
				info.Size = *o.Size
			}
			if o.TimeModified != nil {
				info.Modified = o.TimeModified.Time.UTC()
			}
			listing.Objects = append(listing.Objects, info)
		}
		if res.NextStartWith == nil || *res.NextStartWith == "" {
			break
		}
		start = res.NextStartWith
	}
	return listing, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
