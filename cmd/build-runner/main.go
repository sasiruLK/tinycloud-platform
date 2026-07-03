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
		ImagePrefix:    firstNonEmpty(os.Getenv("IMAGE_PREFIX"), os.Getenv("OCIR_IMAGE_PREFIX")),
		Registry:       firstNonEmpty(os.Getenv("IMAGE_REGISTRY"), os.Getenv("OCIR_REGISTRY")),
		Owner:          firstNonEmpty(os.Getenv("IMAGE_NAMESPACE"), os.Getenv("OCIR_NAMESPACE")),
		BuildPlatform:  os.Getenv("BUILD_PLATFORM"),
		CacheRef:       firstNonEmpty(os.Getenv("BUILD_CACHE_REF"), os.Getenv("OCIR_CACHE_REF")),
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
