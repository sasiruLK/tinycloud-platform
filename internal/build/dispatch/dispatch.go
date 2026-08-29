// Package dispatch triggers the GitHub Actions workflow that executes builds.
//
// TinyCloud owns the build plane — the queue, lifecycle, logs and UI live in the
// coordinator. Only the container build itself runs elsewhere, because both ARM
// nodes are fully committed to k3s (2 OCPU / 12 GB is this tenancy's entire
// Ampere allowance) and an in-cluster build competing for the single 6 GB worker
// could evict the API that serves the build page.
//
// The workflow reports back through the same /v1/runner endpoints the old
// on-VM runner used, so the coordinator does not care which executor ran.
package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EventType is the repository_dispatch event the build workflow listens for.
const EventType = "build-app"

// Config describes where to send dispatches.
type Config struct {
	// Token is a GitHub PAT with `repo` scope on WorkflowRepo.
	Token string
	// WorkflowRepo is "owner/name" of the repo holding build-app.yaml.
	WorkflowRepo string
	// CoordinatorURL is the base URL the workflow reports back to.
	CoordinatorURL string
	// ImagePrefix is the registry path images are pushed under.
	ImagePrefix string

	HTTPClient *http.Client

	// baseURL overrides the GitHub API root in tests.
	baseURL string
}

// Dispatcher sends build jobs to GitHub Actions.
type Dispatcher struct {
	cfg  Config
	http *http.Client
}

// New returns a Dispatcher, or nil if it is not configured. A nil Dispatcher is
// safe to call and reports a clear error, so the coordinator can start without
// GitHub credentials and fail only when a build is actually requested.
func New(cfg Config) *Dispatcher {
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.WorkflowRepo) == "" {
		return nil
	}
	c := cfg.HTTPClient
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	return &Dispatcher{cfg: cfg, http: c}
}

// Payload is the client_payload delivered to the workflow. Field names match the
// `github.event.client_payload.*` references in .github/workflows/build-app.yaml;
// changing one without the other silently produces empty values in the run.
type Payload struct {
	JobID          string `json:"job_id"`
	AppName        string `json:"app_name"`
	RepoURL        string `json:"repo_url"`
	Ref            string `json:"ref"`
	Port           int    `json:"port"`
	CoordinatorURL string `json:"coordinator_url"`
	ImagePrefix    string `json:"image_prefix"`
}

type dispatchBody struct {
	EventType     string  `json:"event_type"`
	ClientPayload Payload `json:"client_payload"`
}

// Dispatch asks GitHub Actions to build the job. It returns once GitHub has
// accepted the request; the run itself reports progress back over the runner API.
func (d *Dispatcher) Dispatch(ctx context.Context, jobID, appName, repoURL, ref string, port int) error {
	if d == nil {
		return fmt.Errorf("build dispatcher not configured: set GITHUB_TOKEN and BUILD_WORKFLOW_REPO")
	}
	// Checked here rather than in New so that an operator who has not finished
	// configuring the build plane still gets a coordinator that starts, a
	// dashboard that renders, and a named failure at the point they ask for a
	// build. Neither value has a default: one would push this instance's images
	// into somebody else's registry namespace, and the other would report this
	// instance's build status to somebody else's host.
	if strings.TrimSpace(d.cfg.ImagePrefix) == "" {
		return fmt.Errorf("build dispatcher not configured: set IMAGE_PREFIX to the registry path images are pushed under, e.g. ghcr.io/your-account")
	}
	if strings.TrimSpace(d.cfg.CoordinatorURL) == "" {
		return fmt.Errorf("build dispatcher not configured: set BUILD_COORDINATOR_PUBLIC_URL to the address the build reports back to")
	}

	body, err := json.Marshal(dispatchBody{
		EventType: EventType,
		ClientPayload: Payload{
			JobID:          jobID,
			AppName:        appName,
			RepoURL:        repoURL,
			Ref:            ref,
			Port:           port,
			CoordinatorURL: strings.TrimRight(d.cfg.CoordinatorURL, "/"),
			ImagePrefix:    d.cfg.ImagePrefix,
		},
	})
	if err != nil {
		return fmt.Errorf("encode dispatch payload: %w", err)
	}

	base := d.cfg.baseURL
	if base == "" {
		base = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/dispatches", base, d.cfg.WorkflowRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build dispatch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	res, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch to GitHub Actions: %w", err)
	}
	defer res.Body.Close()

	// 204 No Content is success for repository_dispatch.
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	return fmt.Errorf("dispatch to GitHub Actions returned %s: %s", res.Status, strings.TrimSpace(string(detail)))
}
