package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sasiruLK/tinycloud-platform/internal/api"
	apimw "github.com/sasiruLK/tinycloud-platform/internal/api/middleware"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
	buildclient "github.com/sasiruLK/tinycloud-platform/internal/build/client"
	"github.com/sasiruLK/tinycloud-platform/internal/config"
	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

func main() {
	cfg := config.Load()

	k8sClient, err := k8s.NewClient()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	app := fiber.New(fiber.Config{
		AppName: "TinyCloud API v1",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			message := "Internal server error"
			errCode := "internal_error"

			if e, ok := err.(*response.HTTPError); ok {
				code = e.Code
				errCode = e.ErrCode
				message = e.Message
			} else if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				if code >= 400 && code < 500 {
					message = e.Message
					errCode = "bad_request"
				}
			}

			if code >= 500 {
				log.Printf("[ERROR] requestId=%s error=%v", response.RequestID(c), err)
			}

			return response.JSONError(c, code, errCode, message)
		},
	})

	app.Use(apimw.RequestID())
	app.Use(recover.New())
	app.Use(apimw.StructuredLogger())
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Prometheus metrics endpoint (unauthenticated)
	metricsHandler := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
	app.Get("/metrics", func(c *fiber.Ctx) error {
		metricsHandler(c.Context())
		return nil
	})

	var builds *buildclient.Client
	if cfg.BuildCoordinatorURL != "" {
		builds = buildclient.New(cfg.BuildCoordinatorURL, cfg.BuildCoordinatorToken)
	}

	// Infrastructure snapshot. Core reads it through the configured Providers
	// and holds no Substrate credentials of its own. A malformed Provider list
	// is a configuration mistake and stops startup; an absent Provider is not,
	// and renders as a named gap on the dashboard.
	providers, err := provider.LoadEntries(cfg.Providers, cfg.ProvidersFile)
	if err != nil {
		log.Fatalf("Invalid provider configuration: %v", err)
	}
	for _, p := range providers {
		log.Printf("Provider %s (%s) at %s", p.Name, p.Kind, p.BaseURL)
	}

	// Sources are resolved lazily on the first refresh, so a Provider that is
	// not up yet never keeps the API from starting. /v1/infra explains itself
	// and every other route is unaffected.
	//
	// There is no fallback behind the Providers any more. Core reads every
	// Substrate the same way, over the contract, and a Capability no configured
	// Provider serves is a named gap on the dashboard rather than a read core
	// performs itself.
	infraCache := infra.NewDefaultCache(infra.DefaultConfig(), func(ctx context.Context) (infra.Sources, error) {
		return provider.InfraSources(ctx, providers), nil
	})
	infraCache.Prime()

	api.SetupRoutes(app, k8sClient, builds, infraCache)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("TinyCloud API starting on port %s", port)
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
