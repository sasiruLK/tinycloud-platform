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
	Resources []Resource `json:"resources"`
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
