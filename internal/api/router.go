package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/api/handlers"
	"github.com/sasiruLK/tinycloud-platform/internal/api/middleware"
	buildclient "github.com/sasiruLK/tinycloud-platform/internal/build/client"
	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
)

// SetupRoutes registers all API routes
func SetupRoutes(app *fiber.App, k8sClient *k8s.Client, buildClient *buildclient.Client, infraCache *infra.Cache) {
	h := handlers.New(k8sClient, buildClient, infraCache)

	// OpenAPI specs (unauthenticated): this API, and the contract its
	// Providers implement.
	app.Get("/openapi.json", OpenAPISpec)
	app.Get("/provider-contract.yaml", ProviderContract)

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
	v1.Post("/apps/:name/rebuild", h.RebuildApp)
	v1.Post("/apps/:name/suspend", h.SuspendApp)
	v1.Post("/apps/:name/rollback", h.Rollback)
	v1.Post("/apps/:name/restore", h.Restore)

	// Builds
	v1.Get("/me", h.Me)
	v1.Get("/apps/:name/resources/:kind/:resource", h.GetResourceManifest)
	v1.Get("/apps/:name/pods/:pod/logs", h.GetPodLogsByName)
	v1.Get("/builds", h.ListBuilds)
	v1.Get("/builds/:id", h.GetBuild)
	v1.Get("/builds/:id/logs", h.GetBuildLogs)

	// GitHub
	v1.Get("/github/repos", h.ListGitHubRepos)

	// Rollbacks
	v1.Get("/rollbacks", h.ListRollbacks)

	// Infrastructure health (cached snapshot, assembled from Providers)
	v1.Get("/infra", h.GetInfra)
}
