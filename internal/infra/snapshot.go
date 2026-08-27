// Package infra reports the health of the infrastructure an Instance runs on:
// the machines behind its Kubernetes nodes, the alarms watching them, the
// address traffic arrives on, the backup store and the slice of a constrained
// Substrate's allowance already spent.
//
// Nothing here knows which Substrate that is. Every read goes through one of
// the five source interfaces in collect.go, and the implementation Core wires
// up by default is an HTTP client for a Provider — an independent service that
// holds its own Substrate's credentials and answers the published `/v0`
// contract. Core itself holds none.
//
// Everything here is read-only, and a source that is missing, unimplemented or
// broken costs its own fields and nothing else: the value arrives as null and
// the failure is named in the snapshot's warnings.
//
// The whole snapshot is cached (see Cache) because the UI polls this endpoint
// and a Provider's Substrate may meter requests.
package infra

import "time"

// Ceilings for the reference Substrate's free allowance. Neither is
// discoverable through an API — they are published as account limits, not
// resource attributes — so they live here as constants and are stated in the
// payload for the UI's benefit rather than computed.
const (
	// AmpereOcpuTotal is the reference Substrate's entire ARM allowance,
	// fully spent by k3s-control and k3s-worker-1.
	AmpereOcpuTotal = 2.0
	// AmpereMemoryGbTotal is the matching memory allowance.
	AmpereMemoryGbTotal = 12.0
	// ObjectStorageTotalBytes is the 20 GiB free object storage allowance,
	// shared across every bucket in the account.
	ObjectStorageTotalBytes int64 = 20 * 1024 * 1024 * 1024
)

// Node roles, derived from the machine's name.
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

// Node is one machine backing the Instance.
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

// Alarm is one alarm and its current state. Status is one of OK, FIRING or
// SUSPENDED.
type Alarm struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// Ingress describes the address traffic reaches the cluster on.
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

// Uptime is one synthetic monitor's recent availability, 0.0–1.0.
type Uptime struct {
	Monitor      string   `json:"monitor"`
	Target       string   `json:"target"`
	Availability *float64 `json:"availability"`
}

// Capacity is the free-tier budget and how much of it is spent. The totals
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
