package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/handlers"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
)

// SetupRoutes registers all API routes
func SetupRoutes(app *fiber.App, k8sClient *k8s.Client) {
	h := handlers.New(k8sClient)

	v1 := app.Group("/v1")

	// Health
	v1.Get("/health", h.Health)

	// Apps
	v1.Get("/apps", h.ListApps)
	v1.Get("/apps/:name", h.GetApp)
	v1.Get("/apps/:name/logs", h.GetLogs)
	v1.Post("/apps/:name/sync", h.TriggerSync)
	v1.Post("/apps/:name/rollback", h.Rollback)
	v1.Post("/apps/:name/restore", h.Restore)

	// Rollbacks
	v1.Get("/rollbacks", h.ListRollbacks)
}
