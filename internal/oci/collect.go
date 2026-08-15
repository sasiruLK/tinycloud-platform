package oci

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metric namespaces and queries. The MQL windows are deliberately wider than
// the metric's own resolution: compute agent posts every minute, the APM
// synthetics monitors run every 15 minutes, so a narrow window regularly
// returns nothing at all.
const (
	nsCompute = "oci_computeagent"
	nsNLB     = "oci_nlb"
	nsAPM     = "oracle_apm_synthetics"

	queryCPU          = "CpuUtilization[5m].groupBy(resourceDisplayName).mean()"
	queryMemory       = "MemoryUtilization[5m].groupBy(resourceDisplayName).mean()"
	queryHealthy      = "HealthyBackends[5m].groupBy(backendSetName).max()"
	queryUnhealthy    = "UnhealthyBackends[5m].groupBy(backendSetName).max()"
	queryAvailability = "Availability[1h].groupBy(MonitorName,Target).mean()"

	// Lookback windows handed to Monitoring alongside each query.
	computeWindow = 20 * time.Minute
	nlbWindow     = 20 * time.Minute
	apmWindow     = 3 * time.Hour

	dimInstance    = "resourceDisplayName"
	dimMonitorName = "MonitorName"
	dimTarget      = "Target"
)

// Config names the OCI resources the snapshot is assembled from. The defaults
// describe this tenancy; every field can be overridden from the environment so
// the package is not welded to one account.
type Config struct {
	CompartmentID          string
	NetworkLoadBalancerID  string
	ObjectStorageNamespace string
	Bucket                 string
	// BackupPrefixes are the streams reported separately. Objects outside
	// them still count towards the bucket totals.
	BackupPrefixes []string
	// CallTimeout bounds every individual OCI call.
	CallTimeout time.Duration
}

// DefaultConfig returns the verified configuration for the TinyCloud tenancy.
func DefaultConfig() Config {
	return Config{
		CompartmentID:          "ocid1.tenancy.oc1..aaaaaaaa7xgc5ijlnvzktzftj6ho6jpzymmiira5vhug65pcvtcdy26m3ebq",
		NetworkLoadBalancerID:  "ocid1.networkloadbalancer.oc1.iad.amaaaaaaul44qqiaaemwwblgkpws7pf5b2p3wetsqlyn3lhkzmls425odupq",
		ObjectStorageNamespace: "idzghas4xwzv",
		Bucket:                 "tinycloud-backups",
		BackupPrefixes:         []string{"sqlite", "gitops", "coordinator"},
		CallTimeout:            5 * time.Second,
	}
}

// ConfigWithOverrides returns DefaultConfig with any non-empty argument
// substituted, so the tenancy details can be moved to the environment without
// the endpoint needing configuration to work today.
func ConfigWithOverrides(compartmentID, nlbID, namespace, bucket string) Config {
	cfg := DefaultConfig()
	if compartmentID != "" {
		cfg.CompartmentID = compartmentID
	}
	if nlbID != "" {
		cfg.NetworkLoadBalancerID = nlbID
	}
	if namespace != "" {
		cfg.ObjectStorageNamespace = namespace
	}
	if bucket != "" {
		cfg.Bucket = bucket
	}
	return cfg
}

// InstanceInfo is one compute instance, flattened out of the SDK's types.
type InstanceInfo struct {
	ID          string
	Name        string
	State       string
	Shape       string
	OCPUs       float64
	MemoryGB    float64
	FaultDomain string
	PrivateIP   string
}

// Series is the latest datapoint of one Monitoring result series.
type Series struct {
	Dimensions map[string]string
	Timestamp  time.Time
	Value      float64
}

// AlarmStatus is one alarm's current state.
type AlarmStatus struct {
	Name     string
	Severity string
	Status   string
}

// ObjectInfo is one Object Storage object.
type ObjectInfo struct {
	Name     string
	Size     int64
	Modified time.Time
}

// The collector talks to OCI only through these interfaces, so tests can fake
// every service without SDK types or live credentials.
type (
	// InstanceSource lists the compute instances in the compartment.
	InstanceSource interface {
		ListInstances(ctx context.Context) ([]InstanceInfo, error)
	}
	// MetricSource runs one MQL query and returns the latest point per series.
	MetricSource interface {
		QueryMetric(ctx context.Context, namespace, query string, window time.Duration) ([]Series, error)
	}
	// AlarmSource lists alarm statuses.
	AlarmSource interface {
		ListAlarmStatuses(ctx context.Context) ([]AlarmStatus, error)
	}
	// IngressSource returns the load balancer's public IP.
	IngressSource interface {
		IngressPublicIP(ctx context.Context) (string, error)
	}
	// BackupSource lists the objects in the backup bucket.
	BackupSource interface {
		ListObjects(ctx context.Context) ([]ObjectInfo, error)
	}
)

// Sources bundles the five read paths a snapshot needs. A nil member is
// skipped and reported as a warning rather than panicking.
type Sources struct {
	Instances InstanceSource
	Metrics   MetricSource
	Alarms    AlarmSource
	Ingress   IngressSource
	Backups   BackupSource
}

// Collector assembles one snapshot from the sources.
type Collector struct {
	cfg     Config
	src     Sources
	nowFunc func() time.Time
}

// NewCollector returns a Collector reading through src.
func NewCollector(cfg Config, src Sources) *Collector {
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = 5 * time.Second
	}
	return &Collector{cfg: cfg, src: src, nowFunc: time.Now}
}

func (c *Collector) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

// collectResult is the raw output of the fan-out, before assembly.
type collectResult struct {
	mu sync.Mutex

	instances []InstanceInfo
	alarms    []AlarmStatus
	publicIP  string
	objects   []ObjectInfo

	cpu       []Series
	memory    []Series
	healthy   []Series
	unhealthy []Series
	uptime    []Series

	// ok records which sources answered. A source that failed contributes
	// nulls, never zeros.
	ok       map[string]bool
	warnings []string
}

func (r *collectResult) succeed(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ok[source] = true
}

func (r *collectResult) fail(source string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings = append(r.warnings, fmt.Sprintf("%s: %v", source, err))
}

// Collect fans every OCI read out concurrently and assembles whatever came
// back. It never returns an error: a dashboard showing most of the truth beats
// a 500, so a failed source leaves its fields null and adds a warning.
func (c *Collector) Collect(ctx context.Context) *Snapshot {
	res := &collectResult{ok: map[string]bool{}}
	var wg sync.WaitGroup

	// run executes fn under its own timeout so one slow service cannot hold
	// the whole fan-out open.
	run := func(source string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			callCtx, cancel := context.WithTimeout(ctx, c.cfg.CallTimeout)
			defer cancel()
			if err := fn(callCtx); err != nil {
				res.fail(source, err)
				return
			}
			res.succeed(source)
		}()
	}

	metric := func(source, namespace, query string, window time.Duration, dst *[]Series) {
		if c.src.Metrics == nil {
			res.fail(source, errNoSource)
			return
		}
		run(source, func(ctx context.Context) error {
			series, err := c.src.Metrics.QueryMetric(ctx, namespace, query, window)
			if err != nil {
				return err
			}
			res.mu.Lock()
			*dst = series
			res.mu.Unlock()
			return nil
		})
	}

	if c.src.Instances != nil {
		run(srcInstances, func(ctx context.Context) error {
			list, err := c.src.Instances.ListInstances(ctx)
			if err != nil {
				return err
			}
			res.mu.Lock()
			res.instances = list
			res.mu.Unlock()
			return nil
		})
	} else {
		res.fail(srcInstances, errNoSource)
	}

	if c.src.Alarms != nil {
		run(srcAlarms, func(ctx context.Context) error {
			list, err := c.src.Alarms.ListAlarmStatuses(ctx)
			if err != nil {
				return err
			}
			res.mu.Lock()
			res.alarms = list
			res.mu.Unlock()
			return nil
		})
	} else {
		res.fail(srcAlarms, errNoSource)
	}

	if c.src.Ingress != nil {
		run(srcIngress, func(ctx context.Context) error {
			ip, err := c.src.Ingress.IngressPublicIP(ctx)
			if err != nil {
				return err
			}
			res.mu.Lock()
			res.publicIP = ip
			res.mu.Unlock()
			return nil
		})
	} else {
		res.fail(srcIngress, errNoSource)
	}

	if c.src.Backups != nil {
		run(srcBackups, func(ctx context.Context) error {
			objs, err := c.src.Backups.ListObjects(ctx)
			if err != nil {
				return err
			}
			res.mu.Lock()
			res.objects = objs
			res.mu.Unlock()
			return nil
		})
	} else {
		res.fail(srcBackups, errNoSource)
	}

	metric(srcCPU, nsCompute, queryCPU, computeWindow, &res.cpu)
	metric(srcMemory, nsCompute, queryMemory, computeWindow, &res.memory)
	metric(srcHealthy, nsNLB, queryHealthy, nlbWindow, &res.healthy)
	metric(srcUnhealthy, nsNLB, queryUnhealthy, nlbWindow, &res.unhealthy)
	metric(srcUptime, nsAPM, queryAvailability, apmWindow, &res.uptime)

	wg.Wait()

	return c.assemble(res)
}

// Source names, used both as warning prefixes and as the keys recording which
// reads succeeded.
const (
	srcInstances = "instances"
	srcAlarms    = "alarms"
	srcIngress   = "ingress"
	srcBackups   = "backups"
	srcCPU       = "cpu-metrics"
	srcMemory    = "memory-metrics"
	srcHealthy   = "healthy-backends"
	srcUnhealthy = "unhealthy-backends"
	srcUptime    = "uptime-metrics"
)

var errNoSource = fmt.Errorf("source not configured")

func (c *Collector) assemble(res *collectResult) *Snapshot {
	snap := newSnapshot(c.now())

	cpuByNode := latestByDimension(res.cpu, dimInstance)
	memByNode := latestByDimension(res.memory, dimInstance)

	var ampereOCPU, ampereMemory float64
	for _, inst := range res.instances {
		node := Node{
			Name:        inst.Name,
			State:       inst.State,
			Shape:       inst.Shape,
			OCPUs:       inst.OCPUs,
			MemoryGB:    inst.MemoryGB,
			FaultDomain: inst.FaultDomain,
			PrivateIP:   inst.PrivateIP,
			Role:        RoleFor(inst.Name),
		}
		if res.ok[srcCPU] {
			node.CPUPercent = cpuByNode[inst.Name]
		}
		if res.ok[srcMemory] {
			node.MemoryPercent = memByNode[inst.Name]
		}
		snap.Nodes = append(snap.Nodes, node)

		if isAmpere(inst.Shape) {
			ampereOCPU += inst.OCPUs
			ampereMemory += inst.MemoryGB
		}
	}
	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].Name < snap.Nodes[j].Name })

	if res.ok[srcInstances] {
		snap.Capacity.AmpereOcpuUsed = &ampereOCPU
		snap.Capacity.AmpereMemoryGbUsed = &ampereMemory
	}

	for _, a := range res.alarms {
		snap.Alarms = append(snap.Alarms, Alarm{Name: a.Name, Severity: a.Severity, Status: a.Status})
	}
	sort.Slice(snap.Alarms, func(i, j int) bool { return snap.Alarms[i].Name < snap.Alarms[j].Name })

	// Ingress and backups are always present, even when every read behind them
	// failed: the dashboard renders both sections unconditionally, and a null
	// section would break it. The unknown values inside them are null.
	snap.Ingress = &Ingress{PublicIP: res.publicIP}
	if res.ok[srcHealthy] {
		snap.Ingress.HealthyBackends = maxAcrossSeries(res.healthy)
	}
	if res.ok[srcUnhealthy] {
		snap.Ingress.UnhealthyBackends = maxAcrossSeries(res.unhealthy)
	}

	snap.Backups = &Backups{Bucket: c.cfg.Bucket, Streams: []Stream{}}
	if res.ok[srcBackups] {
		snap.Backups = summariseBackups(c.cfg.Bucket, c.cfg.BackupPrefixes, res.objects)
		size := *snap.Backups.SizeBytes
		snap.Capacity.ObjectStorageUsedBytes = &size
	}

	if res.ok[srcUptime] {
		snap.Uptime = summariseUptime(res.uptime)
	}

	sort.Strings(res.warnings)
	snap.Warnings = res.warnings
	return snap
}

// RoleFor maps an instance name to its role in the platform. The k3s nodes are
// named for their role; everything else is a utility box.
func RoleFor(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "control"):
		return RoleControlPlane
	case strings.Contains(n, "worker"):
		return RoleWorker
	default:
		return RoleUtility
	}
}

// isAmpere reports whether the shape draws on the A1 (Ampere) allowance.
func isAmpere(shape string) bool {
	return strings.Contains(strings.ToUpper(shape), ".A1.")
}

// latestByDimension keys the newest datapoint of each series by one dimension.
func latestByDimension(series []Series, dimension string) map[string]*float64 {
	out := map[string]*float64{}
	newest := map[string]time.Time{}
	for _, s := range series {
		key := s.Dimensions[dimension]
		if key == "" {
			continue
		}
		if prev, seen := newest[key]; seen && !s.Timestamp.After(prev) {
			continue
		}
		v := s.Value
		out[key] = &v
		newest[key] = s.Timestamp
	}
	return out
}

// maxAcrossSeries collapses the per-backend-set counts into one number. The
// NLB carries the same two k3s nodes in both the :80 and :443 backend sets, so
// summing would double-count them; the highest set is the node count.
func maxAcrossSeries(series []Series) *int {
	if len(series) == 0 {
		return nil
	}
	best := series[0].Value
	for _, s := range series[1:] {
		if s.Value > best {
			best = s.Value
		}
	}
	n := int(best + 0.5)
	return &n
}

// summariseBackups counts the bucket as a whole and each configured stream
// prefix separately.
func summariseBackups(bucket string, prefixes []string, objects []ObjectInfo) *Backups {
	var total int
	var size int64
	b := &Backups{Bucket: bucket, Streams: []Stream{}, ObjectCount: &total, SizeBytes: &size}
	counts := map[string]int{}
	newest := map[string]time.Time{}

	for _, o := range objects {
		total++
		size += o.Size
		for _, p := range prefixes {
			if !strings.HasPrefix(o.Name, strings.TrimSuffix(p, "/")+"/") {
				continue
			}
			counts[p]++
			if o.Modified.After(newest[p]) {
				newest[p] = o.Modified
			}
			break
		}
	}

	for _, p := range prefixes {
		s := Stream{Prefix: p, Count: counts[p]}
		if t, ok := newest[p]; ok && !t.IsZero() {
			utc := t.UTC()
			s.Newest = &utc
		}
		b.Streams = append(b.Streams, s)
	}
	return b
}

// summariseUptime turns the APM availability series into one entry per
// monitor, newest datapoint wins.
func summariseUptime(series []Series) []Uptime {
	type key struct{ monitor, target string }
	newest := map[key]Series{}
	for _, s := range series {
		k := key{monitor: s.Dimensions[dimMonitorName], target: s.Dimensions[dimTarget]}
		if k.monitor == "" {
			continue
		}
		if prev, ok := newest[k]; ok && !s.Timestamp.After(prev.Timestamp) {
			continue
		}
		newest[k] = s
	}

	out := make([]Uptime, 0, len(newest))
	for k, s := range newest {
		v := s.Value
		out = append(out, Uptime{Monitor: k.monitor, Target: k.target, Availability: &v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Monitor < out[j].Monitor })
	return out
}
