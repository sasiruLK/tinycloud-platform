package config

import "os"

type Config struct {
	Port        string
	CORSOrigins string
	KubeConfig  string // optional, for local dev
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
		Port:        port,
		CORSOrigins: cors,
		KubeConfig:  os.Getenv("KUBECONFIG"),
	}
}
