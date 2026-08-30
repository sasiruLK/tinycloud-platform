package oci_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	ociprovider "github.com/sasiruLK/tinycloud-platform/internal/provider/oci"
)

// These tests drive the Provider at its HTTP boundary — through the same
// server core talks to — rather than calling its methods. What matters is the
// contract's responses; how the Oracle SDK is called to produce them is not
// something a Provider author or core can observe.

const token = "oci-provider-token"

func str(s string) *string { return &s }

// fakes stands in for the Oracle Cloud SDK. Each field is one call's answer.
type fakes struct {
	instances   []core.Instance
	attachments []core.VnicAttachment
	vnics       map[string]string
	datapoints  []monitoring.MetricData
	alarms      []monitoring.AlarmStatusSummary
	nlbIPs      []networkloadbalancer.IpAddress
	objects     []objectstorage.ObjectSummary

	err error
}

func (f *fakes) ListInstances(context.Context, core.ListInstancesRequest) (core.ListInstancesResponse, error) {
	if f.err != nil {
		return core.ListInstancesResponse{}, f.err
	}
	return core.ListInstancesResponse{Items: f.instances}, nil
}

func (f *fakes) ListVnicAttachments(context.Context, core.ListVnicAttachmentsRequest) (core.ListVnicAttachmentsResponse, error) {
	return core.ListVnicAttachmentsResponse{Items: f.attachments}, nil
}

func (f *fakes) GetVnic(_ context.Context, req core.GetVnicRequest) (core.GetVnicResponse, error) {
	ip, ok := f.vnics[*req.VnicId]
	if !ok {
		return core.GetVnicResponse{}, assert.AnError
	}
	return core.GetVnicResponse{Vnic: core.Vnic{PrivateIp: str(ip)}}, nil
}

func (f *fakes) SummarizeMetricsData(context.Context, monitoring.SummarizeMetricsDataRequest) (monitoring.SummarizeMetricsDataResponse, error) {
	if f.err != nil {
		return monitoring.SummarizeMetricsDataResponse{}, f.err
	}
	return monitoring.SummarizeMetricsDataResponse{Items: f.datapoints}, nil
}

func (f *fakes) ListAlarmsStatus(context.Context, monitoring.ListAlarmsStatusRequest) (monitoring.ListAlarmsStatusResponse, error) {
	if f.err != nil {
		return monitoring.ListAlarmsStatusResponse{}, f.err
	}
	return monitoring.ListAlarmsStatusResponse{Items: f.alarms}, nil
}

func (f *fakes) GetNetworkLoadBalancer(context.Context, networkloadbalancer.GetNetworkLoadBalancerRequest) (networkloadbalancer.GetNetworkLoadBalancerResponse, error) {
	if f.err != nil {
		return networkloadbalancer.GetNetworkLoadBalancerResponse{}, f.err
	}
	return networkloadbalancer.GetNetworkLoadBalancerResponse{
		NetworkLoadBalancer: networkloadbalancer.NetworkLoadBalancer{IpAddresses: f.nlbIPs},
	}, nil
}

func (f *fakes) ListObjects(context.Context, objectstorage.ListObjectsRequest) (objectstorage.ListObjectsResponse, error) {
	if f.err != nil {
		return objectstorage.ListObjectsResponse{}, f.err
	}
	return objectstorage.ListObjectsResponse{
		ListObjects: objectstorage.ListObjects{Objects: f.objects},
	}, nil
}

func clients(f *fakes) ociprovider.Clients {
	return ociprovider.Clients{
		Compute: f, Vnic: f, Monitoring: f, LoadBalancer: f, ObjectStorage: f,
	}
}

// serve starts the Provider behind the contract's own server.
func serve(t *testing.T, cfg ociprovider.Config, f *fakes) string {
	t.Helper()
	p, err := ociprovider.New(cfg, clients(f))
	require.NoError(t, err)
	server := httptest.NewServer(provider.NewServer(p, provider.StaticToken(token)))
	t.Cleanup(server.Close)
	return server.URL
}

// get performs one contract call and returns the status and decoded body.
func get(t *testing.T, baseURL, path string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	var body map[string]any
	if res.Header.Get("Content-Type") != "" {
		_ = json.NewDecoder(res.Body).Decode(&body)
	}
	return res.StatusCode, body
}

const compartment = "ocid1.tenancy.oc1..example"

// An identifier an operator did not supply switches off the Capabilities it
// would have answered, so a tenancy that configures one thing does not claim
// to serve five.
func TestCapabilitiesFollowTheIdentifiersSupplied(t *testing.T) {
	tests := []struct {
		name string
		cfg  ociprovider.Config
		want []string
	}{
		{
			name: "compartment only",
			cfg:  ociprovider.Config{CompartmentID: compartment},
			want: []string{"instances", "metrics", "alarms"},
		},
		{
			name: "bucket only",
			cfg:  ociprovider.Config{ObjectStorageNamespace: "ns", Bucket: "tinycloud-backups"},
			want: []string{"backups"},
		},
		{
			name: "a namespace with no bucket serves nothing",
			cfg:  ociprovider.Config{NetworkLoadBalancerID: "ocid1.nlb", ObjectStorageNamespace: "ns"},
			want: []string{"ingress"},
		},
		{
			name: "everything",
			cfg: ociprovider.Config{
				CompartmentID: compartment, NetworkLoadBalancerID: "ocid1.nlb",
				ObjectStorageNamespace: "ns", Bucket: "tinycloud-backups",
			},
			want: []string{"instances", "metrics", "alarms", "ingress", "backups"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.Capabilities())

			status, body := get(t, serve(t, tt.cfg, &fakes{}), "/v0/capabilities")
			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, "oci", body["provider"])
			assert.Equal(t, "infra", body["kind"])
		})
	}
}

// A configuration naming nothing is refused rather than serving an empty
// Provider: an operator who set no identifier has made a mistake, and a
// Provider that starts and declares nothing hides it.
func TestAProviderThatCanAnswerNothingRefusesToStart(t *testing.T) {
	_, err := ociprovider.New(ociprovider.Config{}, clients(&fakes{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OCI_COMPARTMENT_ID")
}

func TestInstancesCarryShapeAndPrivateAddress(t *testing.T) {
	f := &fakes{
		instances: []core.Instance{
			{
				Id: str("ocid1.instance.a"), DisplayName: str("k3s-control"),
				LifecycleState: core.InstanceLifecycleStateRunning, Shape: str("VM.Standard.A1.Flex"),
				FaultDomain: str("FAULT-DOMAIN-1"),
				ShapeConfig: &core.InstanceShapeConfig{
					Ocpus:       common.Float32(2),
					MemoryInGBs: common.Float32(12),
				},
			},
			{
				Id: str("ocid1.instance.gone"), DisplayName: str("build-vm"),
				LifecycleState: core.InstanceLifecycleStateTerminated,
			},
		},
		attachments: []core.VnicAttachment{
			{InstanceId: str("ocid1.instance.a"), VnicId: str("ocid1.vnic.a")},
		},
		vnics: map[string]string{"ocid1.vnic.a": "10.0.0.95"},
	}

	status, body := get(t, serve(t, ociprovider.Config{CompartmentID: compartment}, f), "/v0/infra/instances")
	require.Equal(t, http.StatusOK, status)

	list := body["instances"].([]any)
	require.Len(t, list, 1, "a terminated instance is not a machine the Instance has")

	node := list[0].(map[string]any)
	assert.Equal(t, "k3s-control", node["name"])
	assert.Equal(t, "RUNNING", node["state"])
	assert.Equal(t, "VM.Standard.A1.Flex", node["shape"])
	assert.Equal(t, float64(2), node["ocpus"])
	assert.Equal(t, float64(12), node["memoryGb"])
	assert.Equal(t, "10.0.0.95", node["privateIp"])
}

// A VNIC that cannot be read costs the address, not the machine.
func TestAnUnreadableVnicLeavesTheAddressEmpty(t *testing.T) {
	f := &fakes{
		instances: []core.Instance{{
			Id: str("ocid1.instance.a"), DisplayName: str("k3s-control"),
			LifecycleState: core.InstanceLifecycleStateRunning,
		}},
		attachments: []core.VnicAttachment{
			{InstanceId: str("ocid1.instance.a"), VnicId: str("ocid1.vnic.missing")},
		},
		vnics: map[string]string{},
	}

	status, body := get(t, serve(t, ociprovider.Config{CompartmentID: compartment}, f), "/v0/infra/instances")
	require.Equal(t, http.StatusOK, status)

	node := body["instances"].([]any)[0].(map[string]any)
	assert.Equal(t, "k3s-control", node["name"])
	assert.Equal(t, "", node["privateIp"])
}

// The contract's metric names are translated into Oracle's query language here
// and nowhere else, and the dimensions come back in the contract's vocabulary.
func TestMetricsComeBackInTheContractsVocabulary(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	f := &fakes{
		datapoints: []monitoring.MetricData{{
			Dimensions: map[string]string{"resourceDisplayName": "k3s-control"},
			AggregatedDatapoints: []monitoring.AggregatedDatapoint{
				{Timestamp: &common.SDKTime{Time: now.Add(-10 * time.Minute)}, Value: common.Float64(11)},
				{Timestamp: &common.SDKTime{Time: now}, Value: common.Float64(42)},
			},
		}},
	}

	base := serve(t, ociprovider.Config{CompartmentID: compartment}, f)
	status, body := get(t, base, "/v0/infra/metrics?metric=cpu.utilization&window=1200")
	require.Equal(t, http.StatusOK, status)

	series := body["series"].([]any)
	require.Len(t, series, 1)
	point := series[0].(map[string]any)
	assert.Equal(t, float64(42), point["value"], "the newest datapoint wins")
	assert.Equal(t, map[string]any{"instance": "k3s-control"}, point["dimensions"],
		"Oracle's resourceDisplayName is the contract's instance")
}

// A metric the contract names but this Substrate has no query for is absent,
// not an error. Absent renders as null; an error would raise a warning about
// something nobody promised.
func TestAMetricWithNoOracleEquivalentIsAbsentNotAnError(t *testing.T) {
	base := serve(t, ociprovider.Config{CompartmentID: compartment}, &fakes{})
	status, body := get(t, base, "/v0/infra/metrics?metric=memory.utilization&window=1200")
	require.Equal(t, http.StatusOK, status)
	assert.Empty(t, body["series"])
	assert.NotNil(t, body["series"], "an empty result is an array, never null")
}

func TestAlarmsAreReported(t *testing.T) {
	f := &fakes{alarms: []monitoring.AlarmStatusSummary{
		{DisplayName: str("node-cpu-high"), Severity: monitoring.AlarmStatusSummarySeverityWarning, Status: monitoring.AlarmStatusSummaryStatusFiring},
	}}

	status, body := get(t, serve(t, ociprovider.Config{CompartmentID: compartment}, f), "/v0/infra/alarms")
	require.Equal(t, http.StatusOK, status)

	alarms := body["alarms"].([]any)
	require.Len(t, alarms, 1)
	assert.Equal(t, "node-cpu-high", alarms[0].(map[string]any)["name"])
}

func TestIngressReturnsThePublicAddress(t *testing.T) {
	f := &fakes{nlbIPs: []networkloadbalancer.IpAddress{
		{IpAddress: str("10.0.0.9"), IsPublic: common.Bool(false)},
		{IpAddress: str("129.158.225.37"), IsPublic: common.Bool(true)},
	}}

	status, body := get(t, serve(t, ociprovider.Config{NetworkLoadBalancerID: "ocid1.nlb"}, f), "/v0/infra/ingress")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "129.158.225.37", body["publicIp"], "the private address is not the Instance's address")
}

// A load balancer with no public address yet is pending, which is a state
// rather than a failure — the same case as a Kubernetes Service awaiting one.
func TestAPendingIngressAddressIsEmptyNotAnError(t *testing.T) {
	f := &fakes{nlbIPs: []networkloadbalancer.IpAddress{
		{IpAddress: str("10.0.0.9"), IsPublic: common.Bool(false)},
	}}

	status, body := get(t, serve(t, ociprovider.Config{NetworkLoadBalancerID: "ocid1.nlb"}, f), "/v0/infra/ingress")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "", body["publicIp"])
}

// Core holds no Substrate identifiers, so it cannot name the backup store. The
// Provider that read it says what it is called.
func TestBackupsNameTheStoreTheyCameFrom(t *testing.T) {
	modified := time.Date(2026, 8, 29, 18, 39, 38, 0, time.UTC)
	f := &fakes{objects: []objectstorage.ObjectSummary{
		{Name: str("sqlite/2026-08-29.db"), Size: common.Int64(1000), TimeModified: &common.SDKTime{Time: modified}},
	}}

	cfg := ociprovider.Config{ObjectStorageNamespace: "idzghas4xwzv", Bucket: "tinycloud-backups"}
	status, body := get(t, serve(t, cfg, f), "/v0/infra/backups")
	require.Equal(t, http.StatusOK, status)

	assert.Equal(t, "tinycloud-backups", body["store"])
	objects := body["objects"].([]any)
	require.Len(t, objects, 1)
	assert.Equal(t, "sqlite/2026-08-29.db", objects[0].(map[string]any)["name"])
}

// A Capability this configuration does not switch on is 501, which core turns
// into a named warning rather than a failure.
func TestAnUnconfiguredCapabilityIsNotImplemented(t *testing.T) {
	base := serve(t, ociprovider.Config{CompartmentID: compartment}, &fakes{})

	for _, path := range []string{"/v0/infra/ingress", "/v0/infra/backups"} {
		status, body := get(t, base, path)
		assert.Equal(t, http.StatusNotImplemented, status, path)
		assert.Equal(t, "not_implemented", body["error"], path)
	}
}

// The Substrate being unreadable is 502: the Provider is up, Oracle is not
// answering, and core must tell those apart.
func TestAnUnreadableSubstrateIsAnUpstreamError(t *testing.T) {
	f := &fakes{err: assert.AnError}
	base := serve(t, ociprovider.Config{CompartmentID: compartment}, f)

	status, body := get(t, base, "/v0/infra/instances")
	assert.Equal(t, http.StatusBadGateway, status)
	assert.Equal(t, "upstream_error", body["error"])
}

// A Provider holds its Substrate's credentials. Anything in the cluster that
// can route to it must still be refused.
func TestAnUnauthenticatedRequestIsRefused(t *testing.T) {
	base := serve(t, ociprovider.Config{CompartmentID: compartment}, &fakes{})

	res, err := http.Get(base + "/v0/infra/instances")
	require.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}
