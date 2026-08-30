package conformance_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/sasiruLK/tinycloud-platform/internal/conformance"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	k8sprovider "github.com/sasiruLK/tinycloud-platform/internal/provider/kubernetes"
	ociprovider "github.com/sasiruLK/tinycloud-platform/internal/provider/oci"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const token = "conformance-token"

// The in-tree Providers are checked against the published contract here, so
// that a change to the contract which breaks an implementation fails the build
// rather than being discovered by a Provider author.
func TestKubernetesProviderIsConformant(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "k3s-control"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("12Gi"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress-nginx-controller", Namespace: "ingress-nginx"},
		},
	)

	p, err := k8sprovider.New(clientset, nil, k8sprovider.Options{})
	require.NoError(t, err)

	report := run(t, provider.NewServer(p, provider.StaticToken(token)))

	require.True(t, report.Passed(), "the Kubernetes provider is not conformant:\n%s", report)
	assert.Equal(t, "kubernetes", report.Provider)
	assert.Equal(t, []string{"ingress", "instances", "metrics"}, report.Declared)

	// A partial implementation earns a partial pass: the two Capabilities a
	// cluster has no equivalent of are reported, not failed.
	outcomes := map[string]conformance.Outcome{}
	for _, result := range report.Results {
		outcomes[result.Capability] = result.Outcome
	}
	assert.Equal(t, conformance.Pass, outcomes["instances"])
	assert.Equal(t, conformance.Pass, outcomes["metrics"])
	assert.Equal(t, conformance.Pass, outcomes["ingress"])
	assert.Equal(t, conformance.NotImplemented, outcomes["alarms"])
	assert.Equal(t, conformance.NotImplemented, outcomes["backups"])
	assert.Equal(t, conformance.Pass, outcomes["authentication"])
}

// The Oracle Cloud Provider against the published contract, driven by a fake
// SDK so that this runs in CI with no Oracle account. It is the contract's
// second Substrate: the Kubernetes Provider proved the contract could be served
// without a cloud, and this proves the cloud the project was built on is served
// the same way as any other.
func TestOCIProviderIsConformant(t *testing.T) {
	p, err := ociprovider.New(ociprovider.Config{
		CompartmentID:          "ocid1.tenancy.oc1..example",
		NetworkLoadBalancerID:  "ocid1.networkloadbalancer.oc1..example",
		ObjectStorageNamespace: "example",
		Bucket:                 "tinycloud-backups",
	}, ociprovider.Clients{
		Compute: emptyOracle{}, Vnic: emptyOracle{}, Monitoring: emptyOracle{},
		LoadBalancer: emptyOracle{}, ObjectStorage: emptyOracle{},
	})
	require.NoError(t, err)

	report := run(t, provider.NewServer(p, provider.StaticToken(token)))

	require.True(t, report.Passed(), "the OCI provider is not conformant:\n%s", report)
	assert.Equal(t, "oci", report.Provider)

	// A fully configured tenancy declares every Capability of the Infra kind,
	// which is what makes this the contract's most complete implementation.
	assert.Equal(t, []string{"alarms", "backups", "ingress", "instances", "metrics"}, report.Declared)
	for _, result := range report.Results {
		assert.Equal(t, conformance.Pass, result.Outcome, result.Capability)
	}
}

// emptyOracle is a tenancy that exists and holds nothing. Every call succeeds
// and returns no items, which is what the contract's shape rules are checked
// against: empty collections must still serialise as arrays.
type emptyOracle struct{}

func (emptyOracle) ListInstances(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error) {
	return core.ListInstancesResponse{}, nil
}
func (emptyOracle) ListVnicAttachments(context.Context, core.ListVnicAttachmentsRequest) (core.ListVnicAttachmentsResponse, error) {
	return core.ListVnicAttachmentsResponse{}, nil
}
func (emptyOracle) GetVnic(context.Context, core.GetVnicRequest) (core.GetVnicResponse, error) {
	return core.GetVnicResponse{}, nil
}
func (emptyOracle) SummarizeMetricsData(context.Context, monitoring.SummarizeMetricsDataRequest) (monitoring.SummarizeMetricsDataResponse, error) {
	return monitoring.SummarizeMetricsDataResponse{}, nil
}
func (emptyOracle) ListAlarmsStatus(context.Context, monitoring.ListAlarmsStatusRequest) (monitoring.ListAlarmsStatusResponse, error) {
	return monitoring.ListAlarmsStatusResponse{}, nil
}
func (emptyOracle) GetNetworkLoadBalancer(context.Context, networkloadbalancer.GetNetworkLoadBalancerRequest) (networkloadbalancer.GetNetworkLoadBalancerResponse, error) {
	return networkloadbalancer.GetNetworkLoadBalancerResponse{}, nil
}
func (emptyOracle) ListObjects(context.Context, objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	return objectstorage.ListObjectsResponse{}, nil
}

// The suite has to fail a Provider that violates the contract, or passing it
// means nothing.
func TestSuiteFailsAContractViolation(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		fails   string
	}{
		{
			name: "declares a capability it answers 501 for",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v0/capabilities" {
					fmt.Fprint(w, `{"kind":"infra","provider":"liar","capabilities":["instances"]}`)
					return
				}
				w.WriteHeader(http.StatusNotImplemented)
				fmt.Fprint(w, `{"error":"not_implemented","message":"no"}`)
			},
			fails: "instances",
		},
		{
			name: "answers a capability it did not declare",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v0/capabilities" {
					fmt.Fprint(w, `{"kind":"infra","provider":"eager","capabilities":[]}`)
					return
				}
				fmt.Fprint(w, `{"instances":[],"series":[],"alarms":[],"objects":[],"publicIp":""}`)
			},
			fails: "instances",
		},
		{
			name: "returns null where the contract says array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v0/capabilities" {
					fmt.Fprint(w, `{"kind":"infra","provider":"sloppy","capabilities":["instances"]}`)
					return
				}
				if r.URL.Path == "/v0/infra/instances" {
					fmt.Fprint(w, `{}`)
					return
				}
				w.WriteHeader(http.StatusNotImplemented)
			},
			fails: "instances",
		},
		{
			name: "serves the substrate to anyone who asks",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v0/capabilities" {
					fmt.Fprint(w, `{"kind":"infra","provider":"open","capabilities":[]}`)
					return
				}
				w.WriteHeader(http.StatusNotImplemented)
			},
			fails: "authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := run(t, authenticated(tt.handler, tt.name != "serves the substrate to anyone who asks"))

			assert.False(t, report.Passed(), "expected a failure:\n%s", report)
			for _, result := range report.Results {
				if result.Capability == tt.fails {
					assert.Equal(t, conformance.Fail, result.Outcome, result.Detail)
					assert.NotEmpty(t, result.Detail, "a failure says what was wrong")
				}
			}
		})
	}
}

// authenticated wraps a Provider in the bearer check the contract requires,
// or leaves it open when the test is about a Provider that does not check.
func authenticated(next http.HandlerFunc, enforce bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if enforce && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized","message":"missing or invalid bearer token"}`)
			return
		}
		next(w, r)
	})
}

// run serves handler and returns what the suite makes of it.
func run(t *testing.T, handler http.Handler) conformance.Report {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	report, err := conformance.New(server.URL, token, 5*time.Second).Run(context.Background())
	require.NoError(t, err)
	return report
}
