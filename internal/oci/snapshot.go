// Package oci reports the health of the OCI infrastructure TinyCloud runs on:
// the compute instances behind the k3s nodes, the Monitoring alarms watching
// them, the network load balancer fronting ingress, the backup bucket and the
// slice of the Always Free allowance already spent.
//
// Everything here is read-only. The API authenticates with instance principals
// — the node's own identity, resolved through the metadata service — so there
// are no keys to distribute and nothing to rotate. That also means the package
// only works from inside the tenancy: on a laptop the provider fails to
// initialise and every call reports a clear error instead of panicking.
//
// The whole snapshot is cached (see Cache) because the UI polls this endpoint
// and OCI Monitoring meters requests.
package oci

import "time"

// Always Free ceilings for this tenancy. Neither is discoverable through an
// API — Oracle publishes them as account limits, not resource attributes — so
// they live here as constants and are stated in the payload for the UI's
// benefit rather than computed.
const (
	// AmpereOcpuTotal is the tenancy's entire A1.Flex allowance, fully spent
	// by k3s-control and k3s-worker-1.
	AmpereOcpuTotal = 2.0
	// AmpereMemoryGbTotal is the matching memory allowance.
	AmpereMemoryGbTotal = 12.0
	// ObjectStorageTotalBytes is the 20 GiB free Object Storage allowance,
	// shared across every bucket in the tenancy.
	ObjectStorageTotalBytes int64 = 20 * 1024 * 1024 * 1024
)

// Node roles, derived from the instance name and shape.
const (
	RoleControlPlane = "control-plane"
	RoleWorker       = "worker"
	RoleUtility      = "utility"
)

// Snapshot is the /v1/infra payload. Field names are part of the UI contract;
// renaming one breaks the dashboard.
//
// Anything that can genuinely be unknown is a pointer and marshals as null.
// A missing metric must not arrive at the UI as a confident zero — "0% CPU"
// and "we could not reach Monitoring" are very different statements.
type Snapshot struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Stale     bool      `json:"stale"`
	Nodes     []Node    `json:"nodes"`
	Alarms    []Alarm   `json:"alarms"`
	Ingress   *Ingress  `json:"ingress"`
	Backups   *Backups  `json:"backups"`
	Uptime    []Uptime  `json:"uptime"`
	Capacity  Capacity  `json:"capacity"`
	// Warnings names the sources that failed on the last collection, so a
	// partial payload explains its own gaps. Additive to the contract: absent
	// when everything answered.
	Warnings []string `json:"warnings,omitempty"`
}

// Node is one compute instance.
type Node struct {
	Name          string   `json:"name"`
	State         string   `json:"state"`
	Shape         string   `json:"shape"`
	OCPUs         float64  `json:"ocpus"`
	MemoryGB      float64  `json:"memoryGb"`
	FaultDomain   string   `json:"faultDomain"`
	PrivateIP     string   `json:"privateIp"`
	CPUPercent    *float64 `json:"cpuPercent"`
	MemoryPercent *float64 `json:"memoryPercent"`
	Role          string   `json:"role"`
}

// Alarm is one Monitoring alarm and its current state. Status is one of OK,
// FIRING or SUSPENDED.
type Alarm struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// Ingress describes the network load balancer in front of the cluster.
type Ingress struct {
	PublicIP          string `json:"publicIp"`
	HealthyBackends   *int   `json:"healthyBackends"`
	UnhealthyBackends *int   `json:"unhealthyBackends"`
}

// Backups summarises the backup bucket, split by the stream prefixes. The
// object is always present — the dashboard renders the section unconditionally
// — but the totals are null when the bucket could not be listed.
type Backups struct {
	Bucket      string   `json:"bucket"`
	ObjectCount *int     `json:"objectCount"`
	SizeBytes   *int64   `json:"sizeBytes"`
	Streams     []Stream `json:"streams"`
}

// Stream is one backup prefix within the bucket.
type Stream struct {
	Prefix string     `json:"prefix"`
	Count  int        `json:"count"`
	Newest *time.Time `json:"newest"`
}

// Uptime is one APM synthetic monitor's recent availability, 0.0–1.0.
type Uptime struct {
	Monitor      string   `json:"monitor"`
	Target       string   `json:"target"`
	Availability *float64 `json:"availability"`
}

// Capacity is the Always Free budget and how much of it is spent. The totals
// are constants; the used values are null when their source failed, because a
// zeroed usage bar reads as "plenty of room left".
type Capacity struct {
	AmpereOcpuUsed          *float64 `json:"ampereOcpuUsed"`
	AmpereOcpuTotal         float64  `json:"ampereOcpuTotal"`
	AmpereMemoryGbUsed      *float64 `json:"ampereMemoryGbUsed"`
	AmpereMemoryGbTotal     float64  `json:"ampereMemoryGbTotal"`
	ObjectStorageUsedBytes  *int64   `json:"objectStorageUsedBytes"`
	ObjectStorageTotalBytes int64    `json:"objectStorageTotalBytes"`
}

// newSnapshot returns a snapshot with every slice non-nil, so the UI always
// receives [] rather than null for an empty list.
func newSnapshot(now time.Time) *Snapshot {
	return &Snapshot{
		UpdatedAt: now.UTC(),
		Nodes:     []Node{},
		Alarms:    []Alarm{},
		Uptime:    []Uptime{},
		Capacity: Capacity{
			AmpereOcpuTotal:         AmpereOcpuTotal,
			AmpereMemoryGbTotal:     AmpereMemoryGbTotal,
			ObjectStorageTotalBytes: ObjectStorageTotalBytes,
		},
	}
}
