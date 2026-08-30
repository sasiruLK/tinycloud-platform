package config

import "os"

type Config struct {
	Port                  string
	CORSOrigins           string
	KubeConfig            string // optional, for local dev
	GitHubToken           string
	GitHubUser            string
	BuildCoordinatorURL   string
	BuildCoordinatorToken string

	// Providers is the JSON list of Providers this Instance reads its
	// infrastructure through, inline or in a file — see internal/provider for
	// the shape. Both empty means no Provider is configured, which renders a
	// dashboard whose sources are named as missing rather than an error page.
	Providers     string
	ProvidersFile string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cors := os.Getenv("CORS_ORIGINS")
	if cors == "" {
		cors = "*"
	}

	return &Config{
		Port:                  port,
		CORSOrigins:           cors,
		KubeConfig:            os.Getenv("KUBECONFIG"),
		GitHubToken:           os.Getenv("GITHUB_TOKEN"),
		GitHubUser:            os.Getenv("GITHUB_USERNAME"),
		BuildCoordinatorURL:   os.Getenv("BUILD_COORDINATOR_URL"),
		BuildCoordinatorToken: os.Getenv("BUILD_COORDINATOR_TOKEN"),

		Providers:     os.Getenv("TINYCLOUD_PROVIDERS"),
		ProvidersFile: os.Getenv("TINYCLOUD_PROVIDERS_FILE"),
	}
}
