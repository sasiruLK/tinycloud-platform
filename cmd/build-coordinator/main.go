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
	//
	// None of these have a default. They named the maintainer's workflow repo,
	// host and registry namespace until 2026-08-29, which meant a published
	// image carried one operator's identity into everybody else's cluster —
	// the same defect the Oracle tenancy defaults had, in the build plane
	// rather than the read path. An unconfigured coordinator now names what is
	// missing when a build is requested, and starts either way.
	dispatcher := dispatch.New(dispatch.Config{
		Token:          os.Getenv("GITHUB_TOKEN"),
		WorkflowRepo:   os.Getenv("BUILD_WORKFLOW_REPO"),
		CoordinatorURL: os.Getenv("BUILD_COORDINATOR_PUBLIC_URL"),
		ImagePrefix:    os.Getenv("IMAGE_PREFIX"),
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
	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Print("github webhooks disabled: GITHUB_WEBHOOK_SECRET not set")
	} else {
		log.Print("github webhooks: a push to an app repo will rebuild it")
	}

	coordinator.NewServer(store, os.Getenv("BUILD_COORDINATOR_TOKEN"), dispatcher, emitter, webhookSecret).Register(app)

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
