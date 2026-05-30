package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/handlers"
	"github.com/sasiruLK/tinycloud-platform/internal/api/middleware"
	buildclient "github.com/sasiruLK/tinycloud-platform/internal/build/client"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
)

// SetupRoutes registers all API routes
func SetupRoutes(app *fiber.App, k8sClient *k8s.Client, buildClient *buildclient.Client) {
	h := handlers.New(k8sClient, buildClient)

	// OpenAPI spec (unauthenticated)
	app.Get("/openapi.json", OpenAPISpec)

	v1 := app.Group("/v1")

	// Auth middleware for all v1 routes (except /health which is handled internally)
	v1.Use(middleware.AuthMiddleware())

	// Health
	v1.Get("/health", h.Health)

	// Apps
	v1.Get("/apps", h.ListApps)
	v1.Post("/apps", h.CreateApp)
	v1.Get("/apps/:name", h.GetApp)
	v1.Get("/apps/:name/logs", h.GetLogs)
	v1.Post("/apps/:name/sync", h.TriggerSync)
	v1.Post("/apps/:name/suspend", h.SuspendApp)
	v1.Post("/apps/:name/rollback", h.Rollback)
	v1.Post("/apps/:name/restore", h.Restore)

	// Builds
	v1.Get("/builds/:id", h.GetBuild)
	v1.Get("/builds/:id/logs", h.GetBuildLogs)

	// GitHub
	v1.Get("/github/repos", h.ListGitHubRepos)

	// Rollbacks
	v1.Get("/rollbacks", h.ListRollbacks)
}
