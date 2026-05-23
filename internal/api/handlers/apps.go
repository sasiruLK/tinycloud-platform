package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/git"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
	"github.com/sasiruLK/tinycloud-platform/internal/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Handler holds dependencies
type Handler struct {
	K8s  *k8s.Client
	Git  *git.GitOps
}

// New creates a new Handler
func New(k8sClient *k8s.Client) *Handler {
	return &Handler{
		K8s: k8sClient,
		Git: git.NewGitOps(),
	}
}

// Health returns API health status
func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "healthy",
		"version": "1.0.0",
		"gitops":  "self-managed-v4",
		"build":   "native-arm64-cross-compile",
	})
}

// ListApps returns all managed applications
func (h *Handler) ListApps(c *fiber.Ctx) error {
	ctx := context.Background()
	appsList, err := h.K8s.ListApplications(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to list apps: %v", err))
	}

	apps := make([]models.App, 0, len(appsList.Items))
	for _, item := range appsList.Items {
		app := convertUnstructuredToApp(&item)
		apps = append(apps, app)
	}

	return c.JSON(fiber.Map{"apps": apps})
}

// GetApp returns a single application
func (h *Handler) GetApp(c *fiber.Ctx) error {
	name := c.Params("name")
	ctx := context.Background()

	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("app not found: %v", err))
	}

	resources := getAppResources(app)

	detail := models.AppDetail{
		App:       convertUnstructuredToApp(app),
		Resources: resources,
	}

	return c.JSON(detail)
}

// GetLogs returns pod logs for an app
func (h *Handler) GetLogs(c *fiber.Ctx) error {
	name := c.Params("name")
	container := c.Query("container", "app")
	tail := c.QueryInt("tail", 100)

	ctx := context.Background()

	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}

	ns := getAppDestinationNamespace(app)
	if ns == "" {
		ns = "default"
	}

	pods, err := h.K8s.GetDeploymentPods(ctx, ns, name)
	if err != nil || len(pods.Items) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "no pods found for app")
	}

	podName := pods.Items[0].Name
	logs, err := h.K8s.GetPodLogs(ctx, ns, podName, container, int64(tail))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to get logs: %v", err))
	}

	lines := strings.Split(strings.TrimSpace(logs), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}

	return c.JSON(models.LogResponse{
		Pod:       podName,
		Container: container,
		Lines:     lines,
	})
}

// TriggerSync triggers an Argo CD sync
func (h *Handler) TriggerSync(c *fiber.Ctx) error {
	name := c.Params("name")
	ctx := context.Background()

	if err := h.K8s.TriggerSync(ctx, name); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to trigger sync: %v", err))
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
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
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	// Validate inputs
	if req.TargetRevision == "" || !isValidSHA(req.TargetRevision) {
		return fiber.NewError(fiber.StatusBadRequest, "targetRevision must be a 40-char hex SHA")
	}
	if req.Reason == "" {
		return fiber.NewError(fiber.StatusBadRequest, "reason is required")
	}
	if req.InitiatedBy == "" {
		req.InitiatedBy = "api"
	}

	ctx := context.Background()

	// Check app exists
	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}

	// Check not already in rollback
	currentTarget, _ := h.K8s.GetAppTargetRevision(ctx, name)
	if strings.HasPrefix(currentTarget, "rollback/") {
		return fiber.NewError(fiber.StatusConflict, "app is already in rollback state")
	}

	// Validate target SHA exists in gitops-lab
	valid, err := h.Git.ValidateSHA(req.TargetRevision)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to validate SHA: %v", err))
	}
	if !valid {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "target revision is not a known-good commit")
	}

	// Get current image info for history
	currentRev, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
	currentImage := ""
	images, _, _ := unstructured.NestedSlice(app.Object, "status", "summary", "images")
	if len(images) > 0 {
		if img, ok := images[0].(string); ok {
			currentImage = img
		}
	}

	// Get target image from commit (simplified: we'll extract from the commit's sidecar or just record the SHA)
	// For MVP, we just record the targetRevision; the actual image is in the gitops-lab commit
	targetImage := ""
	// TODO: could read .argocd-source-*.yaml from the target commit

	// Step 1: Create rollback branch
	if err := h.Git.CreateRollbackBranch(name, req.TargetRevision); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to create rollback branch: %v", err))
	}

	// Step 2: Patch Argo CD Application targetRevision
	rollbackBranch := fmt.Sprintf("rollback/%s", name)
	if err := h.K8s.PatchTargetRevision(ctx, name, rollbackBranch); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to patch Argo CD app: %v", err))
	}

	// Step 3: Record in rollbacks.yaml
	rollbackID := fmt.Sprintf("rb-%s-%s", name, time.Now().Format("20060102-150405"))
	entry := &git.RollbackEntry{
		ID:               rollbackID,
		Type:             "rollback",
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		TargetRevision:   req.TargetRevision,
		TargetImage:      targetImage,
		PreviousRevision: currentRev,
		PreviousImage:    currentImage,
		Reason:           req.Reason,
		RollbackBranch:   rollbackBranch,
		InitiatedBy:      req.InitiatedBy,
	}

	if err := h.Git.RecordRollback(name, entry); err != nil {
		// Log but don't fail — Argo CD is already tracking rollback branch
		fmt.Printf("Warning: failed to record rollback in git: %v\n", err)
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"rollbackId":     rollbackID,
		"app":            name,
		"rollbackBranch": rollbackBranch,
		"targetRevision": req.TargetRevision,
		"targetImage":    targetImage,
		"previousRevision": currentRev,
		"previousImage":    currentImage,
		"status":         "active",
		"createdAt":      entry.Timestamp,
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
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if req.Reason == "" {
		return fiber.NewError(fiber.StatusBadRequest, "reason is required")
	}
	if req.InitiatedBy == "" {
		req.InitiatedBy = "api"
	}

	ctx := context.Background()

	// Check app exists
	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}

	// Check app is in rollback state
	currentTarget, _ := h.K8s.GetAppTargetRevision(ctx, name)
	if !strings.HasPrefix(currentTarget, "rollback/") {
		return fiber.NewError(fiber.StatusConflict, "app is not in rollback state")
	}

	// Get current revision/image for history
	currentRev, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
	currentImage := ""
	images, _, _ := unstructured.NestedSlice(app.Object, "status", "summary", "images")
	if len(images) > 0 {
		if img, ok := images[0].(string); ok {
			currentImage = img
		}
	}

	// Step 1: Patch Argo CD back to main
	if err := h.K8s.PatchTargetRevision(ctx, name, "main"); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to restore Argo CD app: %v", err))
	}

	// Step 2: Record restore in rollbacks.yaml and fast-forward branch
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

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"restoreId":        restoreID,
		"app":              name,
		"restoredToRevision": currentRev,
		"restoredToImage":  currentImage,
		"status":           "restoring",
		"createdAt":        entry.Timestamp,
	})
}

// ListRollbacks returns rollback history
func (h *Handler) ListRollbacks(c *fiber.Ctx) error {
	rollbacks, err := h.Git.ReadRollbacks()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to read rollbacks: %v", err))
	}

	return c.JSON(fiber.Map{
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
