// Command provider-server hosts one in-tree TinyCloud Provider over the
// published `/v0` contract.
//
// It ships in the same image as Core but runs as its own Deployment, with its
// own service account and its own credentials. Core reaches it over HTTP
// exactly as it would reach a Provider written by someone else, which is the
// point: there is one code path for reading infrastructure, and the
// maintainers' own Providers exercise the public contract on every request.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	k8sprovider "github.com/sasiruLK/tinycloud-platform/internal/provider/kubernetes"
)

func main() {
	kind := env("PROVIDER_KIND", provider.KindInfra)
	if kind != provider.KindInfra {
		log.Fatalf("Unsupported provider kind %q: this binary serves %q", kind, provider.KindInfra)
	}

	implementation := env("PROVIDER_IMPLEMENTATION", "kubernetes")
	port := env("PORT", "9090")

	token, err := tokenSource()
	if err != nil {
		log.Fatalf("Provider token: %v", err)
	}

	var served provider.Infra
	switch implementation {
	case "kubernetes":
		served, err = newKubernetesProvider()
	default:
		err = fmt.Errorf("unknown provider implementation %q", implementation)
	}
	if err != nil {
		log.Fatalf("Failed to start %s provider: %v", implementation, err)
	}

	log.Printf("TinyCloud %s provider %q serving %s on port %s",
		kind, served.Name(), strings.Join(served.Capabilities(), ", "), port)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           provider.NewServer(served, token),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Provider server stopped: %v", err)
	}
}

// tokenSource resolves the bearer token Core authenticates with. A file is
// preferred: it is read per request, so rotating the Secret behind it rotates
// the credential without redeploying anything.
func tokenSource() (provider.TokenSource, error) {
	if path := os.Getenv("PROVIDER_TOKEN_FILE"); path != "" {
		source := provider.FileToken(path)
		if _, err := source(); err != nil {
			return nil, err
		}
		return source, nil
	}
	if token := os.Getenv("PROVIDER_TOKEN"); token != "" {
		return provider.StaticToken(token), nil
	}
	// A Provider holds its Substrate's credentials. Serving it to anything in
	// the cluster that can route to it is not a default worth having.
	return nil, fmt.Errorf("one of PROVIDER_TOKEN_FILE or PROVIDER_TOKEN is required")
}

// newKubernetesProvider builds the Provider that needs no cloud account:
// in-cluster credentials, or a kubeconfig when run from a laptop against a
// local cluster.
func newKubernetesProvider() (provider.Infra, error) {
	config, err := clusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}

	return k8sprovider.New(
		clientset,
		k8sprovider.NewRESTNodeMetrics(clientset.CoreV1().RESTClient()),
		k8sprovider.Options{IngressService: os.Getenv("INGRESS_SERVICE")},
	)
}

func clusterConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig %s: %w", kubeconfig, err)
		}
		return config, nil
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster configuration (set KUBECONFIG to run outside a cluster): %w", err)
	}
	return config, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
