package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/build/runner"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pollInterval, err := time.ParseDuration(env("RUNNER_POLL_INTERVAL", "5s"))
	if err != nil {
		log.Fatalf("invalid RUNNER_POLL_INTERVAL: %v", err)
	}

	r := runner.New(runner.Config{
		CoordinatorURL: env("BUILD_COORDINATOR_URL", "http://127.0.0.1:8090"),
		Token:          os.Getenv("BUILD_COORDINATOR_TOKEN"),
		WorkDir:        env("RUNNER_WORK_DIR", "/tmp/tinycloud-builds"),
		Registry:       env("GHCR_REGISTRY", "ghcr.io"),
		Owner:          env("GHCR_OWNER", "sasirulk"),
		SourceToken:    os.Getenv("SOURCE_GITHUB_TOKEN"),
		PollInterval:   pollInterval,
	})

	log.Print("TinyCloud build runner starting")
	if err := r.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("runner stopped: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
