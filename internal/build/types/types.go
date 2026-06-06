package types

import "time"

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

type CreateBuildRequest struct {
	Name     string            `json:"name,omitempty"`
	AppName  string            `json:"appName"`
	RepoURL  string            `json:"repoUrl"`
	Ref      string            `json:"ref"`
	Replicas int               `json:"replicas"`
	Port     int               `json:"port"`
	Env      map[string]string `json:"env,omitempty"`
}

type CreateBuildResponse struct {
	AppName string `json:"appName"`
	BuildID string `json:"buildId"`
	Status  string `json:"status"`
}

type BuildJob struct {
	ID                string            `json:"id"`
	AppName           string            `json:"appName"`
	RepoURL           string            `json:"repoUrl"`
	Ref               string            `json:"ref"`
	CommitSHA         string            `json:"commitSha"`
	Framework         string            `json:"framework"`
	Image             string            `json:"image"`
	Tag               string            `json:"tag"`
	Status            string            `json:"status"`
	Attempts          int               `json:"attempts"`
	Replicas          int               `json:"replicas"`
	Port              int               `json:"port"`
	Env               map[string]string `json:"env,omitempty"`
	GitOpsCommitSHA   string            `json:"gitopsCommitSha,omitempty"`
	GitOpsPath        string            `json:"gitopsPath,omitempty"`
	DeployStatus      string            `json:"deployStatus,omitempty"`
	ArgoSyncStatus    string            `json:"argoSyncStatus,omitempty"`
	ArgoHealth        string            `json:"argoHealth,omitempty"`
	AppURL            string            `json:"appUrl,omitempty"`
	VerificationError string            `json:"verificationError,omitempty"`
	Error             string            `json:"error,omitempty"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
	StartedAt         *time.Time        `json:"startedAt,omitempty"`
	FinishedAt        *time.Time        `json:"finishedAt,omitempty"`
}

type BuildLogLine struct {
	Sequence  int64     `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

type BuildLogsResponse struct {
	Lines []BuildLogLine `json:"lines"`
}

type RunnerPollResponse struct {
	Job *BuildJob `json:"job,omitempty"`
}

type RunnerLogRequest struct {
	Stream  string `json:"stream"`
	Message string `json:"message"`
}

type RunnerStatusRequest struct {
	Status            string `json:"status"`
	CommitSHA         string `json:"commitSha,omitempty"`
	Framework         string `json:"framework,omitempty"`
	Image             string `json:"image,omitempty"`
	Tag               string `json:"tag,omitempty"`
	GitOpsCommitSHA   string `json:"gitopsCommitSha,omitempty"`
	GitOpsPath        string `json:"gitopsPath,omitempty"`
	DeployStatus      string `json:"deployStatus,omitempty"`
	AppURL            string `json:"appUrl,omitempty"`
	VerificationError string `json:"verificationError,omitempty"`
	Error             string `json:"error,omitempty"`
}
