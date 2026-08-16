package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/build/coordinator"
	"github.com/sasiruLK/tinycloud-platform/internal/build/dispatch"
	"github.com/sasiruLK/tinycloud-platform/internal/ocilog"
)

func main() {
	dbPath := env("BUILD_COORDINATOR_DB", "/var/lib/tinycloud-build-coordinator/builds.db")
	port := env("PORT", "8090")

	store, err := coordinator.OpenStore(dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Builds execute in GitHub Actions; the coordinator keeps the queue,
	// lifecycle, logs and UI. Nil when unconfigured, which leaves jobs queued
	// for a polling runner instead.
	dispatcher := dispatch.New(dispatch.Config{
		Token:          os.Getenv("GITHUB_TOKEN"),
		WorkflowRepo:   env("BUILD_WORKFLOW_REPO", "sasiruLK/tinycloud-platform"),
		CoordinatorURL: env("BUILD_COORDINATOR_PUBLIC_URL", "https://tinycloud.sasiru.lk"),
		ImagePrefix:    env("IMAGE_PREFIX", "ghcr.io/sasirulk"),
	})
	if dispatcher == nil {
		log.Print("build dispatcher disabled: GITHUB_TOKEN or BUILD_WORKFLOW_REPO not set")
	} else {
		log.Print("build executor: GitHub Actions")
	}

	// A second, off-cluster copy of build history. The primary record is the
	// SQLite file on this node's local-path volume, which does not survive the
	// node. Nil when OCI_LOG_ID is unset, and a nil emitter is a no-op.
	emitter, err := ocilog.New(os.Getenv("OCI_LOG_ID"))
	if err != nil {
		// Not fatal. Losing the secondary copy of build logs is not a reason to
		// refuse to run builds at all.
		log.Printf("oci logging disabled: %v", err)
	} else if emitter == nil {
		log.Print("oci logging disabled: OCI_LOG_ID not set")
	} else {
		log.Print("oci logging: shipping build events")
	}
	defer emitter.Close()

	app := fiber.New(fiber.Config{AppName: "TinyCloud Build Coordinator"})
	coordinator.NewServer(store, os.Getenv("BUILD_COORDINATOR_TOKEN"), dispatcher, emitter).Register(app)

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
