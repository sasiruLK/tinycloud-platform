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
	}
}
