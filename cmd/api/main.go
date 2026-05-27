package main

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"github.com/sasiruLK/tinycloud-platform/internal/api"
	apimw "github.com/sasiruLK/tinycloud-platform/internal/api/middleware"
	"github.com/sasiruLK/tinycloud-platform/internal/api/response"
	"github.com/sasiruLK/tinycloud-platform/internal/config"
	"github.com/sasiruLK/tinycloud-platform/internal/k8s"
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

	api.SetupRoutes(app, k8sClient)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("TinyCloud API starting on port %s", port)
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
