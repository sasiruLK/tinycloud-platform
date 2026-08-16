package coordinator

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/sasiruLK/tinycloud-platform/internal/build/dispatch"
	"github.com/sasiruLK/tinycloud-platform/internal/build/types"
	"github.com/sasiruLK/tinycloud-platform/internal/git"
	"github.com/sasiruLK/tinycloud-platform/internal/manifests"
	"github.com/sasiruLK/tinycloud-platform/internal/ocilog"
)

const maxAttempts = 2

type Server struct {
	store      *Store
	token      string
	git        *git.GitOps
	dispatcher *dispatch.Dispatcher
	events     *ocilog.Emitter
}

// NewServer wires the coordinator. dispatcher may be nil, in which case builds
// are accepted and left queued for a polling runner via /v1/runner/poll.
// events may be nil, in which case nothing is shipped to OCI Logging.
func NewServer(store *Store, token string, dispatcher *dispatch.Dispatcher, events *ocilog.Emitter) *Server {
	return &Server{store: store, token: token, git: git.NewGitOps(), dispatcher: dispatcher, events: events}
}

func (s *Server) Register(app *fiber.App) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "healthy"})
	})

	api := app.Group("/v1")
	api.Use(s.auth)
	api.Post("/builds", s.createBuild)
	api.Get("/builds", s.listBuilds)
	api.Get("/builds/:id", s.getBuild)
	api.Get("/builds/:id/logs", s.getLogs)
	api.Post("/runner/poll", s.pollRunner)
	api.Post("/runner/jobs/:id/logs", s.appendRunnerLog)
	api.Post("/runner/jobs/:id/status", s.updateRunnerStatus)
}

func (s *Server) auth(c *fiber.Ctx) error {
	if s.token == "" {
		return c.Next()
	}
	if c.Get("Authorization") != "Bearer "+s.token {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	return c.Next()
}

func (s *Server) createBuild(c *fiber.Ctx) error {
	var req types.CreateBuildRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := normalizeAndValidateBuildRequest(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	ctx := context.Background()
	exists, err := s.git.PathExists(fmt.Sprintf("apps/%s", req.AppName))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to check GitOps repo"})
	}
	if exists {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "app already exists in GitOps repo"})
	}

	existing, err := s.store.GetJobByAppName(ctx, req.AppName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to check existing build"})
	}
	if existing != nil {
		switch existing.Status {
		case types.StatusFailed:
			if err := s.store.DeleteJob(ctx, existing.ID); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to reset failed build"})
			}
		case types.StatusQueued, types.StatusRunning:
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "build already in progress for app"})
		case types.StatusSucceeded:
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "build already exists for app"})
		}
	}

	job := &types.BuildJob{
		ID:       uuid.NewString(),
		AppName:  req.AppName,
		RepoURL:  req.RepoURL,
		Ref:      req.Ref,
		Status:   types.StatusQueued,
		Replicas: req.Replicas,
		Port:     req.Port,
		Env:      req.Env,
	}
	if err := s.store.CreateJob(ctx, job); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "build already exists for app"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create build"})
	}

	// Hand the job to the executor. The job row already exists, so a dispatch
	// failure is recorded against the build rather than lost in a 500 — the user
	// sees why on the build page instead of a build that sits queued forever.
	if s.dispatcher != nil {
		if err := s.dispatcher.Dispatch(context.Background(), job.ID, job.AppName, job.RepoURL, job.Ref, job.Port); err != nil {
			_ = s.store.AppendLog(ctx, job.ID, "stderr", "failed to dispatch build: "+err.Error())
			_ = s.store.UpdateRunnerStatus(ctx, job.ID, types.RunnerStatusRequest{
				Status: types.StatusFailed,
				Error:  "failed to dispatch build to executor: " + err.Error(),
			})
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "failed to dispatch build"})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(types.CreateBuildResponse{
		AppName: job.AppName,
		BuildID: job.ID,
		Status:  job.Status,
	})
}

// listBuilds returns recent build history. The UI has no other way to discover
// a build ID, so without this the entire deploy history is unreachable.
func (s *Server) listBuilds(c *fiber.Ctx) error {
	jobs, err := s.store.ListJobs(context.Background(), c.QueryInt("limit", 50))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list builds"})
	}
	return c.JSON(fiber.Map{"builds": jobs})
}

func (s *Server) getBuild(c *fiber.Ctx) error {
	job, err := s.store.GetJob(context.Background(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "build not found"})
	}
	return c.JSON(job)
}

func (s *Server) getLogs(c *fiber.Ctx) error {
	after := c.QueryInt("after", 0)
	lines, err := s.store.ListLogs(context.Background(), c.Params("id"), int64(after))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list logs"})
	}
	return c.JSON(types.BuildLogsResponse{Lines: lines})
}

func (s *Server) pollRunner(c *fiber.Ctx) error {
	job, err := s.store.ClaimNextQueuedJob(context.Background(), maxAttempts)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to claim job"})
	}
	return c.JSON(types.RunnerPollResponse{Job: job})
}

func (s *Server) appendRunnerLog(c *fiber.Ctx) error {
	var req types.RunnerLogRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Stream == "" {
		req.Stream = "stdout"
	}
	if err := s.store.AppendLog(context.Background(), c.Params("id"), req.Stream, req.Message); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to append log"})
	}
	// After the store, not before: the database is the record of truth and
	// shipping a line that failed to persist would make the two disagree.
	s.events.Emit(ocilog.Event{
		Type:    "build.log",
		JobID:   c.Params("id"),
		Stream:  req.Stream,
		Message: req.Message,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) updateRunnerStatus(c *fiber.Ctx) error {
	var req types.RunnerStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Status == types.StatusSucceeded {
		commitSHA, gitopsPath, appURL, err := s.commitGitOps(c.Params("id"), req)
		if err != nil {
			_ = s.store.UpdateRunnerStatus(context.Background(), c.Params("id"), types.RunnerStatusRequest{
				Status: types.StatusFailed,
				Error:  "image built but GitOps commit failed: " + err.Error(),
			})
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to commit GitOps manifests"})
		}
		req.GitOpsCommitSHA = commitSHA
		req.GitOpsPath = gitopsPath
		req.AppURL = appURL
		req.DeployStatus = "gitops_committed"
	}

	if err := s.store.UpdateRunnerStatus(context.Background(), c.Params("id"), req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update job"})
	}
	s.events.Emit(ocilog.Event{
		Type:   "build.status",
		JobID:  c.Params("id"),
		Status: req.Status,
		Image:  req.Image,
		Tag:    req.Tag,
		Error:  req.Error,
	})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) commitGitOps(jobID string, req types.RunnerStatusRequest) (string, string, string, error) {
	job, err := s.store.GetJob(context.Background(), jobID)
	if err != nil {
		return "", "", "", err
	}
	appReq := manifests.CreateAppRequest{
		Name:     job.AppName,
		Image:    req.Image,
		Tag:      req.Tag,
		Replicas: job.Replicas,
		Port:     job.Port,
		Env:      job.Env,
	}
	files := manifests.GenerateAppFiles(appReq)
	delete(files, fmt.Sprintf("argocd/imageupdater-%s.yaml", job.AppName))
	commitSHA, err := s.git.CommitFiles(fmt.Sprintf("onboard(%s): deploy build %s", job.AppName, short(req.Tag)), files, "tinycloud-build-coordinator")
	if err != nil {
		return "", "", "", err
	}
	gitopsPath := fmt.Sprintf("apps/%s", job.AppName)
	appURL := manifests.AppBaseURL(job.AppName)
	return commitSHA, gitopsPath, appURL, nil
}

func normalizeAndValidateBuildRequest(req *types.CreateBuildRequest) error {
	req.AppName = strings.TrimSpace(req.AppName)
	if req.AppName == "" {
		req.AppName = strings.TrimSpace(req.Name)
	}
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	req.Ref = strings.TrimSpace(req.Ref)
	if req.Ref == "" {
		req.Ref = "main"
	}
	req.Port = 8080
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if req.RepoURL == "" {
		return fmt.Errorf("repoUrl is required")
	}
	if !strings.HasPrefix(req.RepoURL, "https://github.com/") {
		return fmt.Errorf("repoUrl must be an https GitHub repository URL")
	}
	return manifests.ValidateCreateAppRequest(&manifests.CreateAppRequest{
		Name: req.AppName, Image: "ghcr.io/placeholder/app", Tag: manifests.PlaceholderTag, Replicas: req.Replicas, Port: req.Port,
	})
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
