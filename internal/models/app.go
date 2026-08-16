package models

import "time"

// App represents an Argo CD Application as seen by TinyCloud
type App struct {
	Name           string    `json:"name"`
	Namespace      string    `json:"namespace"`
	HealthStatus   string    `json:"health"`
	SyncStatus     string    `json:"syncStatus"`
	Revision       string    `json:"revision"`
	ImageTag       string    `json:"imageTag"`
	TargetRevision string    `json:"targetRevision"`
	LastSyncedAt   time.Time `json:"lastSyncedAt,omitempty"`
	RollbackStatus string    `json:"rollbackStatus"` // normal | rollback
}

// AppDetail extends App with additional runtime details
type AppDetail struct {
	App
	Repo string `json:"repo"`
	Path string `json:"path"`
	// Resources is the flat list Argo CD records. Kept for compatibility.
	Resources []Resource `json:"resources"`
	// Tree is the same set nested by ownerReference, so a Deployment carries its
	// ReplicaSets and their Pods. Argo's status has no parent information, which
	// is why this is reconstructed rather than read.
	Tree []ResourceNode `json:"tree,omitempty"`
}

// ResourceNode mirrors k8s.ResourceNode for the API surface.
type ResourceNode struct {
	Kind      string         `json:"kind"`
	Name      string         `json:"name"`
	Namespace string         `json:"namespace,omitempty"`
	Status    string         `json:"status,omitempty"`
	Health    string         `json:"health,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	Children  []ResourceNode `json:"children,omitempty"`
}

// Resource represents a Kubernetes resource managed by an Argo CD Application
type Resource struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// LogResponse represents pod logs
type LogResponse struct {
	Pod       string   `json:"pod"`
	Container string   `json:"container"`
	Lines     []string `json:"lines"`
}
