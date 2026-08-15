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

	// OCI infrastructure reporting. All optional: empty values fall back to
	// the tenancy defaults compiled into internal/oci.
	OCICompartmentID          string
	OCINetworkLoadBalancerID  string
	OCIObjectStorageNamespace string
	OCIBackupBucket           string
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

		OCICompartmentID:          os.Getenv("OCI_COMPARTMENT_ID"),
		OCINetworkLoadBalancerID:  os.Getenv("OCI_NLB_ID"),
		OCIObjectStorageNamespace: os.Getenv("OCI_OBJECT_STORAGE_NAMESPACE"),
		OCIBackupBucket:           os.Getenv("OCI_BACKUP_BUCKET"),
	}
}
