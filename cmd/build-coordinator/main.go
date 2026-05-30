package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/build/coordinator"
)

func main() {
	dbPath := env("BUILD_COORDINATOR_DB", "/var/lib/tinycloud-build-coordinator/builds.db")
	port := env("PORT", "8090")

	store, err := coordinator.OpenStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	app := fiber.New(fiber.Config{AppName: "TinyCloud Build Coordinator"})
	coordinator.NewServer(store, os.Getenv("BUILD_COORDINATOR_TOKEN")).Register(app)

	log.Printf("TinyCloud build coordinator starting on port %s", port)
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("failed to start coordinator: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
