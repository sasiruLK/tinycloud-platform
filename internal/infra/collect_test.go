package infra

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Function-typed fakes: every OCI read is behind an interface precisely so the
// tests never need credentials, a network or SDK types.
type (
	fakeInstances func(context.Context) ([]InstanceInfo, error)
	fakeMetrics   func(context.Context, string, time.Duration) ([]Series, error)
	fakeAlarms    func(context.Context) ([]AlarmStatus, error)
	fakeIngress   func(context.Context) (string, error)
	fakeBackups   func(context.Context) ([]ObjectInfo, error)
)

func (f fakeInstances) ListInstances(ctx context.Context) ([]InstanceInfo, error) { return f(ctx) }
func (f fakeMetrics) QueryMetric(ctx context.Context, metric string, w time.Duration) ([]Series, error) {
	return f(ctx, metric, w)
}
func (f fakeAlarms) ListAlarmStatuses(ctx context.Context) ([]AlarmStatus, error) { return f(ctx) }
func (f fakeIngress) IngressPublicIP(ctx context.Context) (string, error)         { return f(ctx) }
func (f fakeBackups) ListObjects(ctx context.Context) ([]ObjectInfo, error)       { return f(ctx) }

var errBoom = errors.New("NotAuthorizedOrNotFound")

func testTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return ts
}

// healthySources returns sources that answer everything, mirroring the real
// tenancy: two Ampere k3s nodes and two AMD utility boxes, the NLB carrying the
// same two backends in both its :80 and :443 backend sets.
func healthySources(t *testing.T) Sources {
	t.Helper()
	point := testTime(t, "2026-08-15T19:00:00Z")

	return Sources{
		Instances: fakeInstances(func(context.Context) ([]InstanceInfo, error) {
			return []InstanceInfo{
				{ID: "i-2", Name: "k3s-worker-1", State: "RUNNING", Shape: "VM.Standard.A1.Flex",
					OCPUs: 1, MemoryGB: 6, FaultDomain: "FAULT-DOMAIN-3", PrivateIP: "10.0.0.73"},
				{ID: "i-1", Name: "k3s-control", State: "RUNNING", Shape: "VM.Standard.A1.Flex",
					OCPUs: 1, MemoryGB: 6, FaultDomain: "FAULT-DOMAIN-3", PrivateIP: "10.0.0.95"},
				{ID: "i-3", Name: "amd-utility-1", State: "RUNNING", Shape: "VM.Standard.E2.1.Micro",
					OCPUs: 1, MemoryGB: 1, FaultDomain: "FAULT-DOMAIN-2", PrivateIP: "10.0.0.20"},
			}, nil
		}),
		Metrics: fakeMetrics(func(_ context.Context, metric string, _ time.Duration) ([]Series, error) {
			switch metric {
			case MetricCPUUtilization:
				return []Series{
					{Dimensions: map[string]string{DimInstance: "k3s-control"}, Timestamp: point, Value: 16.2},
					{Dimensions: map[string]string{DimInstance: "k3s-worker-1"}, Timestamp: point, Value: 5.8},
				}, nil
			case MetricMemoryUtilization:
				return []Series{
					{Dimensions: map[string]string{DimInstance: "k3s-control"}, Timestamp: point, Value: 43.1},
				}, nil
			case MetricHealthyBackends:
				return []Series{
					{Dimensions: map[string]string{DimBackendSet: "k3s-80"}, Timestamp: point, Value: 2},
					{Dimensions: map[string]string{DimBackendSet: "k3s-443"}, Timestamp: point, Value: 2},
				}, nil
			case MetricUnhealthyBackends:
				return []Series{
					{Dimensions: map[string]string{DimBackendSet: "k3s-80"}, Timestamp: point, Value: 0},
					{Dimensions: map[string]string{DimBackendSet: "k3s-443"}, Timestamp: point, Value: 0},
				}, nil
			case MetricUptimeAvailability:
				return []Series{
					{Dimensions: map[string]string{DimMonitor: "platform-uptime", DimTarget: "https://tinycloud.sasiru.lk/"},
						Timestamp: point, Value: 1.0},
					// An older point for the same monitor must lose to the newer one.
					{Dimensions: map[string]string{DimMonitor: "platform-uptime", DimTarget: "https://tinycloud.sasiru.lk/"},
						Timestamp: point.Add(-time.Hour), Value: 0.0},
				}, nil
			}
			return nil, nil
		}),
		Alarms: fakeAlarms(func(context.Context) ([]AlarmStatus, error) {
			return []AlarmStatus{
				{Name: "site-unreachable", Severity: "CRITICAL", Status: "FIRING"},
				{Name: "node-memory-high", Severity: "CRITICAL", Status: "OK"},
			}, nil
		}),
		Ingress: fakeIngress(func(context.Context) (string, error) { return "129.158.225.37", nil }),
		Backups: fakeBackups(func(context.Context) ([]ObjectInfo, error) {
			return []ObjectInfo{
				{Name: "sqlite/2026-08-15.db", Size: 1000, Modified: testTime(t, "2026-08-15T18:39:38Z")},
				{Name: "sqlite/2026-08-14.db", Size: 900, Modified: testTime(t, "2026-08-14T18:39:38Z")},
				{Name: "gitops/2026-08-15.tar", Size: 100, Modified: testTime(t, "2026-08-15T06:00:00Z")},
				{Name: "stray.txt", Size: 7, Modified: testTime(t, "2026-08-01T00:00:00Z")},
			}, nil
		}),
	}
}

func testCollector(t *testing.T, src Sources) *Collector {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Bucket = "tinycloud-backups"
	cfg.CallTimeout = time.Second
	c := NewCollector(cfg, src)
	c.nowFunc = func() time.Time { return testTime(t, "2026-08-15T19:00:00Z") }
	return c
}

func TestCollectAssemblesSnapshot(t *testing.T) {
	snap := testCollector(t, healthySources(t)).Collect(context.Background())

	require.Empty(t, snap.Warnings, "every source answered")
	assert.False(t, snap.Stale)

	// Nodes come back sorted by name, with roles derived from the name.
	require.Len(t, snap.Nodes, 3)
	assert.Equal(t, []string{"amd-utility-1", "k3s-control", "k3s-worker-1"},
		[]string{snap.Nodes[0].Name, snap.Nodes[1].Name, snap.Nodes[2].Name})
	assert.Equal(t, RoleUtility, snap.Nodes[0].Role)
	assert.Equal(t, RoleControlPlane, snap.Nodes[1].Role)
	assert.Equal(t, RoleWorker, snap.Nodes[2].Role)

	control := snap.Nodes[1]
	assert.Equal(t, "10.0.0.95", control.PrivateIP)
	require.NotNil(t, control.CPUPercent)
	assert.InDelta(t, 16.2, *control.CPUPercent, 0.001)
	require.NotNil(t, control.MemoryPercent)
	assert.InDelta(t, 43.1, *control.MemoryPercent, 0.001)

	// The metric query returned no memory series for the utility box: unknown
	// is null, never a confident zero.
	assert.Nil(t, snap.Nodes[0].CPUPercent)
	assert.Nil(t, snap.Nodes[0].MemoryPercent)

	assert.Equal(t, []Alarm{
		{Name: "node-memory-high", Severity: "CRITICAL", Status: "OK"},
		{Name: "site-unreachable", Severity: "CRITICAL", Status: "FIRING"},
	}, snap.Alarms)

	// The same two nodes sit in both backend sets, so the sets are collapsed by
	// max, not summed.
	require.NotNil(t, snap.Ingress)
	assert.Equal(t, "129.158.225.37", snap.Ingress.PublicIP)
	require.NotNil(t, snap.Ingress.HealthyBackends)
	assert.Equal(t, 2, *snap.Ingress.HealthyBackends)
	require.NotNil(t, snap.Ingress.UnhealthyBackends)
	assert.Equal(t, 0, *snap.Ingress.UnhealthyBackends)

	require.NotNil(t, snap.Backups)
	assert.Equal(t, "tinycloud-backups", snap.Backups.Bucket)
	require.NotNil(t, snap.Backups.ObjectCount)
	assert.Equal(t, 4, *snap.Backups.ObjectCount, "objects outside a stream still count")
	require.NotNil(t, snap.Backups.SizeBytes)
	assert.Equal(t, int64(2007), *snap.Backups.SizeBytes)
	require.Len(t, snap.Backups.Streams, 3)
	assert.Equal(t, "sqlite", snap.Backups.Streams[0].Prefix)
	assert.Equal(t, 2, snap.Backups.Streams[0].Count)
	require.NotNil(t, snap.Backups.Streams[0].Newest)
	assert.Equal(t, testTime(t, "2026-08-15T18:39:38Z"), *snap.Backups.Streams[0].Newest)
	// A stream with no objects reports zero and a null timestamp.
	assert.Equal(t, "coordinator", snap.Backups.Streams[2].Prefix)
	assert.Equal(t, 0, snap.Backups.Streams[2].Count)
	assert.Nil(t, snap.Backups.Streams[2].Newest)

	require.Len(t, snap.Uptime, 1)
	assert.Equal(t, "platform-uptime", snap.Uptime[0].Monitor)
	assert.Equal(t, "https://tinycloud.sasiru.lk/", snap.Uptime[0].Target)
	require.NotNil(t, snap.Uptime[0].Availability)
	assert.Equal(t, 1.0, *snap.Uptime[0].Availability, "newest datapoint wins")

	// Only the A1 shapes draw on the Ampere allowance.
	require.NotNil(t, snap.Capacity.AmpereOcpuUsed)
	assert.Equal(t, 2.0, *snap.Capacity.AmpereOcpuUsed)
	require.NotNil(t, snap.Capacity.AmpereMemoryGbUsed)
	assert.Equal(t, 12.0, *snap.Capacity.AmpereMemoryGbUsed)
	assert.Equal(t, AmpereOcpuTotal, snap.Capacity.AmpereOcpuTotal)
	assert.Equal(t, AmpereMemoryGbTotal, snap.Capacity.AmpereMemoryGbTotal)
	require.NotNil(t, snap.Capacity.ObjectStorageUsedBytes)
	assert.Equal(t, int64(2007), *snap.Capacity.ObjectStorageUsedBytes)
	assert.Equal(t, ObjectStorageTotalBytes, snap.Capacity.ObjectStorageTotalBytes)
}

// A source that fails must cost only its own fields. A dashboard showing most
// of the truth beats a 500, and a permission error has to be visible rather
// than looking like healthy zeros.
func TestCollectSurvivesPartialFailure(t *testing.T) {
	tests := []struct {
		name    string
		break_  func(*Sources)
		warning string
		check   func(*testing.T, *Snapshot)
	}{
		{
			name: "monitoring down, compute answers",
			break_: func(s *Sources) {
				s.Metrics = fakeMetrics(func(context.Context, string, time.Duration) ([]Series, error) { return nil, errBoom })
			},
			warning: "cpu-metrics",
			check: func(t *testing.T, snap *Snapshot) {
				require.Len(t, snap.Nodes, 3, "nodes still reported")
				for _, n := range snap.Nodes {
					assert.Nil(t, n.CPUPercent, n.Name)
					assert.Nil(t, n.MemoryPercent, n.Name)
				}
				// The public IP came from the NLB API, so ingress survives with
				// null backend counts.
				require.NotNil(t, snap.Ingress)
				assert.Equal(t, "129.158.225.37", snap.Ingress.PublicIP)
				assert.Nil(t, snap.Ingress.HealthyBackends)
				assert.Nil(t, snap.Ingress.UnhealthyBackends)
				assert.Empty(t, snap.Uptime)
			},
		},
		{
			name: "compute forbidden",
			break_: func(s *Sources) {
				s.Instances = fakeInstances(func(context.Context) ([]InstanceInfo, error) { return nil, errBoom })
			},
			warning: "instances",
			check: func(t *testing.T, snap *Snapshot) {
				assert.Empty(t, snap.Nodes)
				assert.Nil(t, snap.Capacity.AmpereOcpuUsed, "no instances read means unknown usage, not zero")
				assert.Nil(t, snap.Capacity.AmpereMemoryGbUsed)
				assert.Len(t, snap.Alarms, 2, "alarms unaffected")
			},
		},
		{
			name: "bucket forbidden",
			break_: func(s *Sources) {
				s.Backups = fakeBackups(func(context.Context) ([]ObjectInfo, error) { return nil, errBoom })
			},
			warning: "backups",
			check: func(t *testing.T, snap *Snapshot) {
				// The section survives so the dashboard can render it; the
				// numbers inside it are null, not zero.
				require.NotNil(t, snap.Backups)
				assert.Equal(t, "tinycloud-backups", snap.Backups.Bucket)
				assert.Nil(t, snap.Backups.ObjectCount)
				assert.Nil(t, snap.Backups.SizeBytes)
				assert.Empty(t, snap.Backups.Streams)
				assert.Nil(t, snap.Capacity.ObjectStorageUsedBytes)
				assert.Len(t, snap.Nodes, 3)
			},
		},
		{
			name: "alarms forbidden",
			break_: func(s *Sources) {
				s.Alarms = fakeAlarms(func(context.Context) ([]AlarmStatus, error) { return nil, errBoom })
			},
			warning: "alarms",
			check: func(t *testing.T, snap *Snapshot) {
				assert.Empty(t, snap.Alarms)
				assert.Len(t, snap.Nodes, 3)
			},
		},
		{
			name: "load balancer unreachable",
			break_: func(s *Sources) {
				s.Ingress = fakeIngress(func(context.Context) (string, error) { return "", errBoom })
			},
			warning: "ingress",
			check: func(t *testing.T, snap *Snapshot) {
				require.NotNil(t, snap.Ingress, "backend counts still came from Monitoring")
				assert.Empty(t, snap.Ingress.PublicIP)
				require.NotNil(t, snap.Ingress.HealthyBackends)
				assert.Equal(t, 2, *snap.Ingress.HealthyBackends)
			},
		},
		{
			name:    "source not wired at all",
			break_:  func(s *Sources) { s.Instances = nil },
			warning: "instances",
			check: func(t *testing.T, snap *Snapshot) {
				assert.Empty(t, snap.Nodes)
				assert.Len(t, snap.Alarms, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := healthySources(t)
			tt.break_(&src)

			snap := testCollector(t, src).Collect(context.Background())

			require.NotEmpty(t, snap.Warnings, "a failed source must be named")
			var found bool
			for _, w := range snap.Warnings {
				if len(w) >= len(tt.warning) && w[:len(tt.warning)] == tt.warning {
					found = true
				}
			}
			assert.True(t, found, "warnings %v should mention %q", snap.Warnings, tt.warning)
			tt.check(t, snap)
		})
	}
}

// The UI is built against these exact field names; a rename here is a broken
// dashboard, so the shape is asserted key by key.
func TestSnapshotJSONShape(t *testing.T) {
	snap := testCollector(t, healthySources(t)).Collect(context.Background())

	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	for _, key := range []string{"updatedAt", "stale", "nodes", "alarms", "ingress", "backups", "uptime", "capacity"} {
		assert.Contains(t, got, key)
	}
	assert.Equal(t, "2026-08-15T19:00:00Z", got["updatedAt"])
	assert.Equal(t, false, got["stale"])

	node := got["nodes"].([]any)[1].(map[string]any)
	assert.Equal(t, map[string]any{
		"name": "k3s-control", "state": "RUNNING", "shape": "VM.Standard.A1.Flex",
		"ocpus": 1.0, "memoryGb": 6.0, "faultDomain": "FAULT-DOMAIN-3",
		"privateIp": "10.0.0.95", "cpuPercent": 16.2, "memoryPercent": 43.1,
		"role": "control-plane",
	}, node)

	assert.Equal(t, map[string]any{"name": "node-memory-high", "severity": "CRITICAL", "status": "OK"},
		got["alarms"].([]any)[0])

	assert.Equal(t, map[string]any{"publicIp": "129.158.225.37", "healthyBackends": 2.0, "unhealthyBackends": 0.0},
		got["ingress"])

	backups := got["backups"].(map[string]any)
	assert.Equal(t, "tinycloud-backups", backups["bucket"])
	assert.Equal(t, 4.0, backups["objectCount"])
	assert.Equal(t, 2007.0, backups["sizeBytes"])
	assert.Len(t, backups, 4, "bucket, objectCount, sizeBytes, streams")
	assert.Equal(t, map[string]any{"prefix": "sqlite", "count": 2.0, "newest": "2026-08-15T18:39:38Z"},
		backups["streams"].([]any)[0])

	assert.Equal(t, map[string]any{
		"monitor": "platform-uptime", "target": "https://tinycloud.sasiru.lk/", "availability": 1.0,
	}, got["uptime"].([]any)[0])

	assert.Equal(t, map[string]any{
		"ampereOcpuUsed": 2.0, "ampereOcpuTotal": 2.0,
		"ampereMemoryGbUsed": 12.0, "ampereMemoryGbTotal": 12.0,
		"objectStorageUsedBytes": 2007.0, "objectStorageTotalBytes": 21474836480.0,
	}, got["capacity"])
}

// Unavailable metrics must marshal as null, not as zero.
func TestSnapshotJSONNullsMissingMetrics(t *testing.T) {
	src := healthySources(t)
	src.Metrics = fakeMetrics(func(context.Context, string, time.Duration) ([]Series, error) { return nil, errBoom })
	src.Backups = fakeBackups(func(context.Context) ([]ObjectInfo, error) { return nil, errBoom })

	raw, err := json.Marshal(testCollector(t, src).Collect(context.Background()))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	node := got["nodes"].([]any)[0].(map[string]any)
	require.Contains(t, node, "cpuPercent")
	assert.Nil(t, node["cpuPercent"])
	assert.Nil(t, node["memoryPercent"])

	// The sections stay, so the dashboard still renders; their numbers are null.
	backups := got["backups"].(map[string]any)
	assert.Nil(t, backups["objectCount"])
	assert.Nil(t, backups["sizeBytes"])
	ingress := got["ingress"].(map[string]any)
	assert.Nil(t, ingress["healthyBackends"])
	assert.Equal(t, "129.158.225.37", ingress["publicIp"])
	assert.Nil(t, got["capacity"].(map[string]any)["objectStorageUsedBytes"])
	assert.NotEmpty(t, got["warnings"], "the failures are named in the payload")
}

// Empty lists must marshal as [] so the UI never has to guard against null.
func TestSnapshotJSONEmptyListsAreArrays(t *testing.T) {
	raw, err := json.Marshal(newSnapshot(testTime(t, "2026-08-15T19:00:00Z")))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"nodes":[]`)
	assert.Contains(t, string(raw), `"alarms":[]`)
	assert.Contains(t, string(raw), `"uptime":[]`)
	assert.NotContains(t, string(raw), `"warnings"`, "omitted when nothing failed")
}

func TestRoleFor(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"k3s-control", RoleControlPlane},
		{"k3s-worker-1", RoleWorker},
		{"amd-utility-1", RoleUtility},
		{"AMD-UTILITY-2", RoleUtility},
		{"", RoleUtility},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RoleFor(tt.name))
		})
	}
}

func TestIsAmpere(t *testing.T) {
	assert.True(t, isAmpere("VM.Standard.A1.Flex"))
	assert.False(t, isAmpere("VM.Standard.E2.1.Micro"))
}
