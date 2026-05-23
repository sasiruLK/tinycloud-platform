package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
	"github.com/sasiruLK/tinycloud-platform/internal/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Handler holds dependencies
type Handler struct {
	K8s *k8s.Client
}

// New creates a new Handler
func New(k8sClient *k8s.Client) *Handler {
	return &Handler{K8s: k8sClient}
}

// Health returns API health status
func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "healthy",
		"version": "1.0.0",
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

	// Get managed resources
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

	// Get app to find its namespace
	app, err := h.K8s.GetApplication(ctx, name)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}

	ns := getAppDestinationNamespace(app)
	if ns == "" {
		ns = "default"
	}

	// Find pods for the deployment
	pods, err := h.K8s.GetDeploymentPods(ctx, ns, name)
	if err != nil || len(pods.Items) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "no pods found for app")
	}

	// Get logs from first pod
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

// Rollback triggers a rollback (Phase 2)
func (h *Handler) Rollback(c *fiber.Ctx) error {
	// For MVP Phase 1, return placeholder
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"message": "Rollback endpoint - implement in Phase 2 with Git integration",
	})
}

// Restore triggers a restore (Phase 2)
func (h *Handler) Restore(c *fiber.Ctx) error {
	// For MVP Phase 1, return placeholder
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"message": "Restore endpoint - implement in Phase 2 with Git integration",
	})
}

// ListRollbacks returns rollback history (Phase 2)
func (h *Handler) ListRollbacks(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"rollbacks": []interface{}{},
		"message":   "Rollback history - implement in Phase 2",
	})
}

// Helpers

func convertUnstructuredToApp(u *unstructured.Unstructured) models.App {
	status, _, _ := unstructured.NestedString(u.Object, "status", "sync", "status")
	health, _, _ := unstructured.NestedString(u.Object, "status", "health", "status")
	revision, _, _ := unstructured.NestedString(u.Object, "status", "sync", "revision")
	targetRev, _, _ := unstructured.NestedString(u.Object, "spec", "source", "targetRevision")

	// Try to extract image tag from status summary or resources
	imageTag := ""
	resources, found, _ := unstructured.NestedSlice(u.Object, "status", "resources")
	if found {
		for _, r := range resources {
			if res, ok := r.(map[string]interface{}); ok {
				if kind, _, _ := unstructured.NestedString(res, "kind"); kind == "Deployment" {
					// Try to get image from status if available
					break
				}
			}
		}
	}

	// Fallback: extract from sidecar file or spec (simplified)
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
	if targetRev != "main" && targetRev != "HEAD" && !strings.HasPrefix(targetRev, "rollback/") {
		rollbackStatus = "rollback"
	}
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
