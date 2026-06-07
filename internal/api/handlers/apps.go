package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
	buildclient "github.com/sasiruLK/tinycloud-platform/internal/build/client"
	buildtypes "github.com/sasiruLK/tinycloud-platform/internal/build/types"
	"github.com/sasiruLK/tinycloud-platform/internal/git"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
	"github.com/sasiruLK/tinycloud-platform/internal/manifests"
	"github.com/sasiruLK/tinycloud-platform/internal/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Handler holds dependencies
type Handler struct {
	K8s   *k8s.Client
	Git   *git.GitOps
	Build *buildclient.Client
}

// New creates a new Handler
func New(k8sClient *k8s.Client, buildClient *buildclient.Client) *Handler {
	return &Handler{
		K8s:   k8sClient,
		Git:   git.NewGitOps(),
		Build: buildClient,
	}
}

// Health returns API health status
func (h *Handler) Health(c *fiber.Ctx) error {
	return response.JSON(c, fiber.Map{
		"status":  "healthy",
		"version": "1.0.0",
		"gitops":  "self-managed-v4",
		"build":   "standalone-coordinator-runner",
	})
}

// ListApps returns all managed applications (paginated)
func (h *Handler) ListApps(c *fiber.Ctx) error {
	ctx := context.Background()
	appsList, err := h.K8s.ListApplications(ctx)
	if err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to list applications")
	}

	apps := make([]models.App, 0, len(appsList.Items))
	for _, item := range appsList.Items {
		app := convertUnstructuredToApp(&item)
		apps = append(apps, app)
	}

	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	total := len(apps)

	limit, offset, end := response.PaginateSlice(limit, offset, total)
	paginated := apps[offset:end]

	return response.JSONPaginated(c, fiber.Map{"apps": paginated}, limit, offset, total)
}

// CreateApp enqueues a source build. The coordinator commits manifests after a successful image push.
func (h *Handler) CreateApp(c *fiber.Ctx) error {
	if h.Build == nil {
		return response.JSONError(c, fiber.StatusServiceUnavailable, "build_coordinator_unavailable",
			"Build coordinator is not configured")
	}

	var req buildtypes.CreateBuildRequest
	if err := c.BodyParser(&req); err != nil {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request", "Invalid request body")
	}

	req.AppName = strings.TrimSpace(req.AppName)
	if req.AppName == "" {
		req.AppName = strings.TrimSpace(req.Name)
	}
	if req.Ref == "" {
		req.Ref = "main"
	}
	req.Port = 8080
	if req.Replicas == 0 {
		req.Replicas = 1
	}
	if err := manifests.ValidateCreateAppRequest(&manifests.CreateAppRequest{
		Name: req.AppName, Image: "ghcr.io/placeholder/app", Tag: "1.0.0", Replicas: req.Replicas, Port: req.Port,
	}); err != nil {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request", err.Error())
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request", "repoUrl is required")
	}

	ctx := context.Background()

	if _, err := h.K8s.GetApplication(ctx, req.AppName); err == nil {
		return response.JSONError(c, fiber.StatusConflict, "conflict",
			fmt.Sprintf("Application '%s' already exists", req.AppName))
	}

	appDir := fmt.Sprintf("apps/%s", req.AppName)
	exists, err := h.Git.PathExists(appDir)
	if err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to check GitOps repo")
	}
	if exists {
		return response.JSONError(c, fiber.StatusConflict, "conflict",
			fmt.Sprintf("App directory '%s' already exists in GitOps repo", appDir))
	}

	build, err := h.Build.CreateBuild(ctx, req)
	if err != nil {
		return response.JSONError(c, fiber.StatusBadGateway, "build_coordinator_error", err.Error())
	}

	return response.JSONStatus(c, fiber.StatusCreated, build)
}

func (h *Handler) GetBuild(c *fiber.Ctx) error {
	if h.Build == nil {
		return response.JSONError(c, fiber.StatusServiceUnavailable, "build_coordinator_unavailable",
			"Build coordinator is not configured")
	}
	build, err := h.Build.GetBuild(context.Background(), c.Params("id"))
	if err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found", "Build not found")
	}
	h.enrichBuildDeploymentStatus(context.Background(), build)
	return response.JSON(c, build)
}

func (h *Handler) GetBuildLogs(c *fiber.Ctx) error {
	if h.Build == nil {
		return response.JSONError(c, fiber.StatusServiceUnavailable, "build_coordinator_unavailable",
			"Build coordinator is not configured")
	}
	logs, err := h.Build.GetLogs(context.Background(), c.Params("id"), int64(c.QueryInt("after", 0)))
	if err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error", "Failed to retrieve build logs")
	}
	return response.JSON(c, logs)
}

// SuspendApp scales an app to zero replicas via GitOps commit.
func (h *Handler) SuspendApp(c *fiber.Ctx) error {
	name := c.Params("name")
	ctx := context.Background()

	if _, err := h.K8s.GetApplication(ctx, name); err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			fmt.Sprintf("Application '%s' not found", name))
	}

	author, _ := c.Locals("user").(string)
	if err := h.Git.UpdateDeploymentReplicas(name, 0, author); err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to suspend app in GitOps repo")
	}

	return response.JSON(c, fiber.Map{
		"name":    name,
		"status":  "suspended",
		"message": "Deployment scaled to 0 replicas; Argo CD will sync the change",
	})
}

// GetApp returns a single application with full details
func (h *Handler) GetApp(c *fiber.Ctx) error {
	name := c.Params("name")
	ctx := context.Background()

	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			fmt.Sprintf("Application '%s' not found", name))
	}

	resources := getAppResources(app)
	repoURL, _, _ := unstructured.NestedString(app.Object, "spec", "source", "repoURL")
	path, _, _ := unstructured.NestedString(app.Object, "spec", "source", "path")

	detail := models.AppDetail{
		App:       convertUnstructuredToApp(app),
		Repo:      repoURL,
		Path:      path,
		Resources: resources,
	}

	return response.JSON(c, detail)
}

// GetLogs returns pod logs for an app
func (h *Handler) GetLogs(c *fiber.Ctx) error {
	name := c.Params("name")
	container := c.Query("container", "")
	tail := c.QueryInt("tail", 100)

	ctx := context.Background()

	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			fmt.Sprintf("Application '%s' not found", name))
	}

	ns := getAppDestinationNamespace(app)
	if ns == "" {
		ns = "default"
	}

	pods, err := h.K8s.GetDeploymentPods(ctx, ns, name)
	if err != nil || len(pods.Items) == 0 {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			"No pods found for application")
	}

	pod := pods.Items[0]
	podName := pod.Name

	// Auto-detect container if not specified
	if container == "" {
		if len(pod.Spec.Containers) > 0 {
			container = pod.Spec.Containers[0].Name
		} else if len(pod.Spec.InitContainers) > 0 {
			container = pod.Spec.InitContainers[0].Name
		} else {
			return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
				"Pod has no containers")
		}
	}

	logs, err := h.K8s.GetPodLogs(ctx, ns, podName, container, int64(tail))
	if err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to retrieve pod logs")
	}

	lines := strings.Split(strings.TrimSpace(logs), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}

	return response.JSON(c, models.LogResponse{
		Pod:       podName,
		Container: container,
		Lines:     lines,
	})
}

// TriggerSync triggers an Argo CD sync
func (h *Handler) TriggerSync(c *fiber.Ctx) error {
	name := c.Params("name")
	ctx := context.Background()

	if _, err := h.K8s.GetApplication(ctx, name); err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			fmt.Sprintf("Application '%s' not found", name))
	}

	if err := h.K8s.TriggerSync(ctx, name); err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to trigger sync")
	}

	return response.JSON(c, fiber.Map{
		"operationId": fmt.Sprintf("sync-%s-%d", name, time.Now().Unix()),
		"status":      "syncing",
		"message":     "Sync triggered via Argo CD",
	})
}

// RollbackRequest body
type RollbackRequest struct {
	TargetRevision string `json:"targetRevision"`
	Reason         string `json:"reason"`
	InitiatedBy    string `json:"initiatedBy"`
}

// Rollback triggers a rollback to a previous gitops-lab commit
func (h *Handler) Rollback(c *fiber.Ctx) error {
	name := c.Params("name")
	var req RollbackRequest
	if err := c.BodyParser(&req); err != nil {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request",
			"Invalid request body")
	}

	if req.TargetRevision == "" || !isValidSHA(req.TargetRevision) {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request",
			"targetRevision must be a 40-character hex SHA")
	}
	if req.Reason == "" {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request",
			"reason is required")
	}
	if req.InitiatedBy == "" {
		req.InitiatedBy = "api"
	}

	ctx := context.Background()

	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			fmt.Sprintf("Application '%s' not found", name))
	}

	currentTarget, _ := h.K8s.GetAppTargetRevision(ctx, name)
	if strings.HasPrefix(currentTarget, "rollback/") {
		return response.JSONError(c, fiber.StatusConflict, "conflict",
			"Application is already in rollback state")
	}

	valid, err := h.Git.ValidateSHA(req.TargetRevision)
	if err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to validate target revision")
	}
	if !valid {
		return response.JSONError(c, fiber.StatusUnprocessableEntity, "unprocessable_entity",
			"Target revision is not a known-good commit")
	}

	currentRev, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
	currentImage := ""
	images, _, _ := unstructured.NestedSlice(app.Object, "status", "summary", "images")
	if len(images) > 0 {
		if img, ok := images[0].(string); ok {
			currentImage = img
		}
	}

	if err := h.Git.CreateRollbackBranch(name, req.TargetRevision); err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to create rollback branch")
	}

	rollbackBranch := fmt.Sprintf("rollback/%s", name)
	if err := h.K8s.PatchTargetRevision(ctx, name, rollbackBranch); err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to patch Argo CD application")
	}

	rollbackID := fmt.Sprintf("rb-%s-%s", name, time.Now().Format("20060102-150405"))
	entry := &git.RollbackEntry{
		ID:               rollbackID,
		Type:             "rollback",
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		TargetRevision:   req.TargetRevision,
		PreviousRevision: currentRev,
		PreviousImage:    currentImage,
		Reason:           req.Reason,
		RollbackBranch:   rollbackBranch,
		InitiatedBy:      req.InitiatedBy,
	}

	if err := h.Git.RecordRollback(name, entry); err != nil {
		fmt.Printf("Warning: failed to record rollback in git: %v\n", err)
	}

	return response.JSON(c, fiber.Map{
		"rollbackId":       rollbackID,
		"app":              name,
		"rollbackBranch":   rollbackBranch,
		"targetRevision":   req.TargetRevision,
		"previousRevision": currentRev,
		"previousImage":    currentImage,
		"status":           "active",
		"createdAt":        entry.Timestamp,
	})
}

// RestoreRequest body
type RestoreRequest struct {
	Reason      string `json:"reason"`
	InitiatedBy string `json:"initiatedBy"`
}

// Restore returns an app to main branch
func (h *Handler) Restore(c *fiber.Ctx) error {
	name := c.Params("name")
	var req RestoreRequest
	if err := c.BodyParser(&req); err != nil {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request",
			"Invalid request body")
	}

	if req.Reason == "" {
		return response.JSONError(c, fiber.StatusBadRequest, "bad_request",
			"reason is required")
	}
	if req.InitiatedBy == "" {
		req.InitiatedBy = "api"
	}

	ctx := context.Background()

	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return response.JSONError(c, fiber.StatusNotFound, "not_found",
			fmt.Sprintf("Application '%s' not found", name))
	}

	currentTarget, _ := h.K8s.GetAppTargetRevision(ctx, name)
	if !strings.HasPrefix(currentTarget, "rollback/") {
		return response.JSONError(c, fiber.StatusConflict, "conflict",
			"Application is not in rollback state")
	}

	currentRev, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
	currentImage := ""
	images, _, _ := unstructured.NestedSlice(app.Object, "status", "summary", "images")
	if len(images) > 0 {
		if img, ok := images[0].(string); ok {
			currentImage = img
		}
	}

	if err := h.K8s.PatchTargetRevision(ctx, name, "main"); err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to restore Argo CD application")
	}

	restoreID := fmt.Sprintf("rs-%s-%s", name, time.Now().Format("20060102-150405"))
	entry := &git.RollbackEntry{
		ID:                 restoreID,
		Type:               "restore",
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
		RestoredToRevision: currentRev,
		RestoredToImage:    currentImage,
		Reason:             req.Reason,
		InitiatedBy:        req.InitiatedBy,
	}

	if err := h.Git.RecordRestore(name, entry, true); err != nil {
		fmt.Printf("Warning: failed to record restore in git: %v\n", err)
	}

	return response.JSON(c, fiber.Map{
		"restoreId":          restoreID,
		"app":                name,
		"restoredToRevision": currentRev,
		"restoredToImage":    currentImage,
		"status":             "restoring",
		"createdAt":          entry.Timestamp,
	})
}

// ListRollbacks returns rollback history
func (h *Handler) ListRollbacks(c *fiber.Ctx) error {
	rollbacks, err := h.Git.ReadRollbacks()
	if err != nil {
		return response.JSONError(c, fiber.StatusInternalServerError, "internal_error",
			"Failed to read rollback history")
	}

	return response.JSON(c, fiber.Map{
		"version":     rollbacks.Version,
		"generatedAt": rollbacks.GeneratedAt,
		"apps":        rollbacks.Apps,
	})
}

// Helpers

var shaRegex = regexp.MustCompile(`^[a-f0-9]{40}$`)

func isValidSHA(s string) bool {
	return shaRegex.MatchString(s)
}

func convertUnstructuredToApp(u *unstructured.Unstructured) models.App {
	status, _, _ := unstructured.NestedString(u.Object, "status", "sync", "status")
	health, _, _ := unstructured.NestedString(u.Object, "status", "health", "status")
	revision, _, _ := unstructured.NestedString(u.Object, "status", "sync", "revision")
	targetRev, _, _ := unstructured.NestedString(u.Object, "spec", "source", "targetRevision")

	imageTag := ""
	images, _, _ := unstructured.NestedSlice(u.Object, "status", "summary", "images")
	if len(images) > 0 {
		if img, ok := images[0].(string); ok {
			parts := strings.Split(img, ":")
			if len(parts) > 1 {
				imageTag = parts[len(parts)-1]
			}
		}
	}

	rollbackStatus := "normal"
	if strings.HasPrefix(targetRev, "rollback/") {
		rollbackStatus = "rollback"
	}

	return models.App{
		Name:           u.GetName(),
		Namespace:      u.GetNamespace(),
		HealthStatus:   health,
		SyncStatus:     status,
		Revision:       revision,
		ImageTag:       imageTag,
		TargetRevision: targetRev,
		RollbackStatus: rollbackStatus,
	}
}

func getAppDestinationNamespace(u *unstructured.Unstructured) string {
	ns, _, _ := unstructured.NestedString(u.Object, "spec", "destination", "namespace")
	return ns
}

func getAppResources(u *unstructured.Unstructured) []models.Resource {
	resources := []models.Resource{}
	resList, found, _ := unstructured.NestedSlice(u.Object, "status", "resources")
	if !found {
		return resources
	}

	for _, r := range resList {
		if res, ok := r.(map[string]interface{}); ok {
			kind, _, _ := unstructured.NestedString(res, "kind")
			name, _, _ := unstructured.NestedString(res, "name")
			health, _, _ := unstructured.NestedString(res, "health", "status")
			if health == "" {
				health = "Healthy"
			}
			resources = append(resources, models.Resource{
				Kind:   kind,
				Name:   name,
				Status: health,
			})
		}
	}
	return resources
}

func (h *Handler) enrichBuildDeploymentStatus(ctx context.Context, build *buildtypes.BuildJob) {
	if build == nil || build.AppName == "" {
		return
	}
	if build.AppURL == "" {
		build.AppURL = manifests.AppBaseURL(build.AppName)
	}
	if build.Status != buildtypes.StatusSucceeded {
		return
	}

	app, err := h.K8s.GetApplication(ctx, build.AppName)
	if err != nil {
		if build.GitOpsCommitSHA != "" {
			build.DeployStatus = "pending_argocd_application"
			build.VerificationError = "GitOps commit exists, but Argo CD has not created the Application yet"
		}
		return
	}

	build.ArgoSyncStatus, _, _ = unstructured.NestedString(app.Object, "status", "sync", "status")
	build.ArgoHealth, _, _ = unstructured.NestedString(app.Object, "status", "health", "status")

	switch {
	case strings.EqualFold(build.ArgoSyncStatus, "Synced") && strings.EqualFold(build.ArgoHealth, "Healthy"):
		build.DeployStatus = "deployed"
		build.VerificationError = ""
	case strings.EqualFold(build.ArgoSyncStatus, "OutOfSync"):
		build.DeployStatus = "argocd_out_of_sync"
		build.VerificationError = "Argo CD Application exists but is out of sync"
	case strings.EqualFold(build.ArgoHealth, "Degraded"):
		build.DeployStatus = "degraded"
		build.VerificationError = "Argo CD reports the application as degraded"
	default:
		build.DeployStatus = "argocd_progressing"
		build.VerificationError = ""
	}
}
