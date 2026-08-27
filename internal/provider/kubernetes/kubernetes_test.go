package kubernetes_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	k8sprovider "github.com/sasiruLK/tinycloud-platform/internal/provider/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the Kubernetes Provider where Core meets it: over HTTP, on
// the published contract. What is asserted is the response an operator's Core —
// or anyone else's client — would receive, not how the cluster was read.

const token = "kubernetes-provider-token"

// node builds a cluster node as a cloud-provisioned cluster labels them.
func node(name string, ready bool) *corev1.Node {
	state := corev1.ConditionFalse
	if ready {
		state = corev1.ConditionTrue
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID("uid-" + name),
			Labels: map[string]string{
				"node.kubernetes.io/instance-type": "VM.Standard.A1.Flex",
				"topology.kubernetes.io/zone":      "FAULT-DOMAIN-3",
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("12Gi"),
			},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.95"}},
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: state}},
		},
	}
}

// ingressService is the controller Service the Provider reads the public
// address from. An empty address stands for one still being assigned.
func ingressService(address string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress-nginx-controller", Namespace: "ingress-nginx"},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
	}
	if address != "" {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: address}}
	}
	return service
}

// stubMetrics stands in for metrics-server.
type stubMetrics struct {
	metrics []k8sprovider.NodeMetric
	err     error
}

func (s stubMetrics) NodeMetrics(context.Context) ([]k8sprovider.NodeMetric, error) {
	return s.metrics, s.err
}

// serve starts the Provider over the contract and returns its base URL.
func serve(t *testing.T, metrics k8sprovider.NodeMetricsSource, objects ...runtime.Object) string {
	t.Helper()

	clientset := fake.NewSimpleClientset(objects...)
	p, err := k8sprovider.New(clientset, metrics, k8sprovider.Options{})
	require.NoError(t, err)

	server := httptest.NewServer(provider.NewServer(p, provider.StaticToken(token)))
	t.Cleanup(server.Close)
	return server.URL
}

// get performs one authenticated contract call and returns its status and body.
func get(t *testing.T, baseURL, path string) (int, map[string]any) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	return res.StatusCode, body
}

// A cluster with no cloud account behind it still answers the Capabilities it
// can, and says which those are.
func TestKubernetesProviderDeclaresWhatItServes(t *testing.T) {
	status, body := get(t, serve(t, nil), "/v0/capabilities")

	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "infra", body["kind"])
	assert.Equal(t, "kubernetes", body["provider"])
	assert.Equal(t, []any{"instances", "metrics", "ingress"}, body["capabilities"])
}

// The nodes of the cluster are the machines of the Instance.
func TestKubernetesProviderServesNodesAsInstances(t *testing.T) {
	url := serve(t, nil, node("k3s-control", true), node("k3s-worker-1", false))

	status, body := get(t, url, "/v0/infra/instances")
	require.Equal(t, http.StatusOK, status)

	instances := body["instances"].([]any)
	require.Len(t, instances, 2)

	byName := map[string]map[string]any{}
	for _, raw := range instances {
		instance := raw.(map[string]any)
		byName[instance["name"].(string)] = instance
	}

	control := byName["k3s-control"]
	assert.Equal(t, "RUNNING", control["state"], "a Ready node is up, in the word the dashboard understands")
	assert.Equal(t, "VM.Standard.A1.Flex", control["shape"])
	assert.Equal(t, "FAULT-DOMAIN-3", control["faultDomain"])
	assert.Equal(t, "10.0.0.95", control["privateIp"])
	assert.InDelta(t, 2, control["ocpus"], 0.001)
	assert.InDelta(t, 12, control["memoryGb"], 0.001)

	assert.Equal(t, "NOT_READY", byName["k3s-worker-1"]["state"])
}

// Utilisation is a percentage of the node's own capacity, keyed by the node
// name so that Core can match it to the instance it came with.
func TestKubernetesProviderServesUtilisationFromMetricsServer(t *testing.T) {
	metrics := stubMetrics{metrics: []k8sprovider.NodeMetric{{
		Name:        "k3s-control",
		CPUCores:    0.5,                    // of two cores
		MemoryBytes: 3 * 1024 * 1024 * 1024, // of twelve GiB
		Timestamp:   time.Date(2026, 8, 15, 19, 0, 0, 0, time.UTC),
	}}}
	url := serve(t, metrics, node("k3s-control", true))

	status, body := get(t, url, "/v0/infra/metrics?metric=cpu.utilization&window=1200")
	require.Equal(t, http.StatusOK, status)

	series := body["series"].([]any)
	require.Len(t, series, 1)
	point := series[0].(map[string]any)
	assert.Equal(t, map[string]any{"instance": "k3s-control"}, point["dimensions"])
	assert.InDelta(t, 25.0, point["value"], 0.001)

	_, memory := get(t, url, "/v0/infra/metrics?metric=memory.utilization&window=1200")
	assert.InDelta(t, 25.0, memory["series"].([]any)[0].(map[string]any)["value"], 0.001)

	// A metric a cluster has no equivalent of is empty rather than an error, so
	// it reaches the dashboard as null instead of as a fault.
	_, uptime := get(t, url, "/v0/infra/metrics?metric=uptime.availability&window=10800")
	assert.Equal(t, []any{}, uptime["series"])
}

// metrics-server is not installed on every cluster, and a cluster without it is
// not a broken one.
func TestKubernetesProviderDegradesWithoutMetricsServer(t *testing.T) {
	absent := stubMetrics{err: fmt.Errorf("the server could not find the requested resource: %w",
		k8sprovider.ErrMetricsUnavailable)}

	for name, metrics := range map[string]k8sprovider.NodeMetricsSource{
		"metrics-server absent":    absent,
		"no metrics source at all": nil,
	} {
		t.Run(name, func(t *testing.T) {
			url := serve(t, metrics, node("k3s-control", true))

			status, body := get(t, url, "/v0/infra/metrics?metric=cpu.utilization&window=1200")
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, []any{}, body["series"], "absent values, not a failure")
		})
	}
}

// A metrics API that is installed but broken is a Substrate failure, and says
// so rather than pretending the cluster is idle.
func TestKubernetesProviderReportsBrokenMetricsAsUpstreamFailure(t *testing.T) {
	url := serve(t, stubMetrics{err: errors.New("connection refused")}, node("k3s-control", true))

	status, body := get(t, url, "/v0/infra/metrics?metric=cpu.utilization&window=1200")
	assert.Equal(t, http.StatusBadGateway, status)
	assert.Equal(t, "upstream_error", body["error"])
}

// The ingress address comes from the controller's Service, and pending is a
// state rather than an error.
func TestKubernetesProviderServesIngressAddress(t *testing.T) {
	assigned := serve(t, nil, ingressService("129.158.225.37"))
	status, body := get(t, assigned, "/v0/infra/ingress")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "129.158.225.37", body["publicIp"])

	pending := serve(t, nil, ingressService(""))
	status, body = get(t, pending, "/v0/infra/ingress")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "", body["publicIp"], "an address still being assigned is empty, not an error")
}

// A cluster with no ingress controller installed is a normal cluster, not a
// broken substrate — a fresh install must look like a working product.
func TestKubernetesProviderTreatsAMissingIngressControllerAsNoAddress(t *testing.T) {
	url := serve(t, nil, node("k3s-control", true))

	status, body := get(t, url, "/v0/infra/ingress")
	require.Equal(t, http.StatusOK, status, "no ingress controller is absent, not broken")
	assert.Equal(t, "", body["publicIp"])
}

// Alarms and backups have no cluster-native equivalent. Reporting that honestly
// is the point of Capability discovery.
func TestKubernetesProviderReportsUnimplementedCapabilities(t *testing.T) {
	url := serve(t, nil)

	for _, path := range []string{"/v0/infra/alarms", "/v0/infra/backups"} {
		status, body := get(t, url, path)
		assert.Equal(t, http.StatusNotImplemented, status, path)
		assert.Equal(t, "not_implemented", body["error"], path)
		assert.Contains(t, body["message"], "kubernetes")
	}
}

// Anything in the cluster that can route to a Provider can ask it about the
// Substrate, so the token is not optional.
func TestKubernetesProviderRequiresBearerToken(t *testing.T) {
	url := serve(t, nil, node("k3s-control", true))

	res, err := http.Get(url + "/v0/infra/instances")
	require.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	// The scheme is part of the credential: a bare token is not a token.
	bare, err := http.NewRequest(http.MethodGet, url+"/v0/infra/instances", nil)
	require.NoError(t, err)
	bare.Header.Set("Authorization", token)
	res, err = http.DefaultClient.Do(bare)
	require.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	// Liveness is the one unauthenticated path: a kubelet probe carries no token.
	probe, err := http.Get(url + "/healthz")
	require.NoError(t, err)
	defer probe.Body.Close()
	assert.Equal(t, http.StatusOK, probe.StatusCode)
}

// An unknown metric is the caller's mistake, and is refused rather than guessed.
func TestKubernetesProviderRejectsAnUnknownMetric(t *testing.T) {
	url := serve(t, nil, node("k3s-control", true))

	status, body := get(t, url, "/v0/infra/metrics?metric=disk.utilization&window=1200")
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "bad_request", body["error"])

	status, _ = get(t, url, "/v0/infra/metrics?metric=cpu.utilization")
	assert.Equal(t, http.StatusBadRequest, status, "the window is required")
}
