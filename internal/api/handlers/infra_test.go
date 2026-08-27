package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive /v1/infra end to end against a Provider: an in-process HTTP
// server answering the `/v0` contract, wired up exactly as a Provider in a
// cluster would be. Nothing here knows how Core reaches it — the assertions are
// on the served payload, which is the whole contract the UI depends on.

const testToken = "test-provider-token"

// fakeProvider is a Provider written by hand rather than with this repository's
// own server, so that what these tests exercise is the published contract and
// not an in-process shortcut. Each field is one Capability's behaviour.
type fakeProvider struct {
	capabilities []string
	instances    func() (int, string)
	metrics      func(metric string) (int, string)
	ingress      func() (int, string)
	alarms       func() (int, string)
	backups      func() (int, string)
}

func (f fakeProvider) start(t *testing.T) string {
	t.Helper()

	answer := func(w http.ResponseWriter, fn func() (int, string)) {
		if fn == nil {
			w.WriteHeader(http.StatusNotImplemented)
			fmt.Fprint(w, `{"error":"not_implemented","message":"not implemented by this provider"}`)
			return
		}
		status, body := fn()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v0/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"unauthorized","message":"missing or invalid bearer token"}`)
			return
		}

		switch r.URL.Path {
		case "/v0/capabilities":
			declared, err := json.Marshal(f.capabilities)
			require.NoError(t, err)
			fmt.Fprintf(w, `{"kind":"infra","provider":"fake","capabilities":%s}`, declared)
		case "/v0/infra/instances":
			answer(w, f.instances)
		case "/v0/infra/metrics":
			metric := r.URL.Query().Get("metric")
			if f.metrics == nil {
				answer(w, nil)
				return
			}
			answer(w, func() (int, string) { return f.metrics(metric) })
		case "/v0/infra/alarms":
			answer(w, f.alarms)
		case "/v0/infra/ingress":
			answer(w, f.ingress)
		case "/v0/infra/backups":
			answer(w, f.backups)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL
}

// providerEntry is how an operator lists a Provider in configuration.
func providerEntry(baseURL string) []provider.Entry {
	return []provider.Entry{{
		Kind:    provider.KindInfra,
		Name:    "fake",
		BaseURL: baseURL,
		Token:   testToken,
	}}
}

// infraCache builds the cache the endpoint serves from, reading through the
// Provider at baseURL.
func infraCache(t *testing.T, baseURL string, opts infra.CacheOptions) *infra.Cache {
	t.Helper()
	cfg := infra.DefaultConfig()
	cfg.Bucket = "tinycloud-backups"
	cfg.CallTimeout = 200 * time.Millisecond

	return infra.NewCache(func(ctx context.Context) (*infra.Collector, error) {
		src := provider.InfraSources(ctx, providerEntry(baseURL), infra.Sources{})
		return infra.NewCollector(cfg, src), nil
	}, opts)
}

func infraApp(t *testing.T, h *Handler) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Get("/v1/infra", h.GetInfra)
	return app
}

// readyCache returns a cache that has already collected once.
func readyCache(t *testing.T, cache *infra.Cache) *infra.Cache {
	t.Helper()
	cache.Prime()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := cache.Get(); err == nil {
			return cache
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cache never became ready")
	return nil
}

// getInfra drives the route and returns the decoded envelope's data.
func getInfra(t *testing.T, cache *infra.Cache) map[string]any {
	t.Helper()

	res, err := infraApp(t, &Handler{Infra: cache}).Test(httptest.NewRequest(fiber.MethodGet, "/v1/infra", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, fiber.StatusOK, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	// The route follows the response envelope every other v1 route uses.
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.NotNil(t, envelope.Data)
	return envelope.Data
}

// warningsOf returns the snapshot's warnings as strings.
func warningsOf(t *testing.T, data map[string]any) []string {
	t.Helper()
	raw, ok := data["warnings"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, w := range raw {
		out = append(out, w.(string))
	}
	return out
}

// assertWarns asserts that some warning mentions each of the fragments.
func assertWarns(t *testing.T, warnings []string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		found := false
		for _, w := range warnings {
			if strings.Contains(w, fragment) {
				found = true
				break
			}
		}
		assert.True(t, found, "expected a warning mentioning %q, got %v", fragment, warnings)
	}
}

// A Provider serving three of the five Capabilities produces a working
// dashboard: its data is rendered, what it cannot answer is null, and every
// gap is named.
func TestGetInfraServesSnapshotFromProvider(t *testing.T) {
	url := fakeProvider{
		capabilities: []string{"instances", "metrics", "ingress"},
		instances: func() (int, string) {
			return http.StatusOK, `{"instances":[
				{"id":"n1","name":"k3s-control","state":"RUNNING","shape":"VM.Standard.A1.Flex",
				 "ocpus":1,"memoryGb":6,"faultDomain":"FAULT-DOMAIN-3","privateIp":"10.0.0.95"},
				{"id":"n2","name":"k3s-worker-1","state":"RUNNING","shape":"VM.Standard.A1.Flex",
				 "ocpus":1,"memoryGb":6,"faultDomain":"FAULT-DOMAIN-3","privateIp":"10.0.0.73"}]}`
		},
		metrics: func(metric string) (int, string) {
			if metric == "cpu.utilization" {
				return http.StatusOK, `{"series":[{"dimensions":{"instance":"k3s-control"},
					"timestamp":"2026-08-15T19:00:00Z","value":16.2}]}`
			}
			// Everything else is a metric this Substrate has no equivalent of.
			return http.StatusOK, `{"series":[]}`
		},
		ingress: func() (int, string) { return http.StatusOK, `{"publicIp":"129.158.225.37"}` },
	}.start(t)

	data := getInfra(t, readyCache(t, infraCache(t, url, infra.CacheOptions{})))

	for _, key := range []string{"updatedAt", "stale", "nodes", "alarms", "ingress", "backups", "capacity"} {
		assert.Contains(t, data, key)
	}

	nodes := data["nodes"].([]any)
	require.Len(t, nodes, 2)

	control := nodes[0].(map[string]any)
	assert.Equal(t, "k3s-control", control["name"])
	assert.Equal(t, "control-plane", control["role"])
	assert.Equal(t, "10.0.0.95", control["privateIp"])
	assert.InDelta(t, 16.2, control["cpuPercent"], 0.001)

	worker := nodes[1].(map[string]any)
	assert.Equal(t, "worker", worker["role"])
	// The Provider reported no CPU series for this node, and no memory series
	// at all. A missing metric is null, never a confident zero.
	assert.Nil(t, worker["cpuPercent"])
	assert.Nil(t, control["memoryPercent"])

	assert.Equal(t, "129.158.225.37", data["ingress"].(map[string]any)["publicIp"])
	// An empty collection is an array, so the UI can render it without a guard.
	assert.Equal(t, []any{}, data["alarms"])

	assertWarns(t, warningsOf(t, data), "alarms", "backups", "no configured provider")
}

// A Capability the Provider does not serve must be distinguishable from one
// that is broken: both are named, and only the failure reads as a fault.
func TestGetInfraNamesUnimplementedAndFailedCapabilities(t *testing.T) {
	url := fakeProvider{
		// The Provider declares backups but answers 501 anyway — the contract
		// allows Core to trust either, and both must render.
		capabilities: []string{"instances", "alarms", "backups"},
		instances:    func() (int, string) { return http.StatusOK, `{"instances":[]}` },
		alarms: func() (int, string) {
			return http.StatusBadGateway, `{"error":"upstream_error","message":"alarms: monitoring API refused the request"}`
		},
		backups: func() (int, string) {
			return http.StatusNotImplemented, `{"error":"not_implemented","message":"capability backups is not implemented by provider fake"}`
		},
	}.start(t)

	data := getInfra(t, readyCache(t, infraCache(t, url, infra.CacheOptions{})))
	warnings := warningsOf(t, data)

	assert.Equal(t, []any{}, data["nodes"], "the Capability that answered is still rendered")
	assertWarns(t, warnings, "alarms: ", "monitoring API refused the request")
	assertWarns(t, warnings, "backups: ", "not implemented by provider fake")
	// The metrics and ingress Capabilities were never declared, so Core did not
	// call them, and says so rather than reporting them as failures.
	assertWarns(t, warnings, "ingress: ", "cpu-metrics: ")

	// Ingress and backups are rendered unconditionally, with unknown values null.
	assert.Equal(t, "", data["ingress"].(map[string]any)["publicIp"])
	assert.Nil(t, data["backups"].(map[string]any)["objectCount"])
}

// A Provider that is slow, broken or gone must never blank the dashboard or
// take the endpoint down with it.
func TestGetInfraSurvivesProviderDegradation(t *testing.T) {
	tests := []struct {
		name     string
		provider func(t *testing.T) string
		warning  string
	}{
		{
			name: "provider fails its Substrate read",
			provider: func(t *testing.T) string {
				return fakeProvider{
					capabilities: []string{"instances"},
					instances: func() (int, string) {
						return http.StatusBadGateway, `{"error":"upstream_error","message":"instances: substrate unavailable"}`
					},
				}.start(t)
			},
			warning: "substrate unavailable",
		},
		{
			name: "provider is slower than the call timeout",
			provider: func(t *testing.T) string {
				return fakeProvider{
					capabilities: []string{"instances"},
					instances: func() (int, string) {
						time.Sleep(2 * time.Second)
						return http.StatusOK, `{"instances":[]}`
					},
				}.start(t)
			},
			warning: "instances: ",
		},
		{
			name: "provider is unreachable",
			provider: func(t *testing.T) string {
				// A Provider that was never up: the address is real, nothing
				// is listening on it.
				server := httptest.NewServer(http.NotFoundHandler())
				url := server.URL
				server.Close()
				return url
			},
			warning: "instances: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := getInfra(t, readyCache(t, infraCache(t, tt.provider(t), infra.CacheOptions{})))

			assert.Equal(t, []any{}, data["nodes"], "an empty dashboard, not a missing one")
			assert.NotNil(t, data["ingress"])
			assert.NotNil(t, data["backups"])
			assertWarns(t, warningsOf(t, data), tt.warning)
		})
	}
}

// A Provider going down must leave the last good snapshot on screen, flagged
// stale, rather than blanking the dashboard.
func TestGetInfraServesLastGoodSnapshotWhenProviderGoesDown(t *testing.T) {
	up := true
	mux := http.NewServeMux()
	mux.HandleFunc("/v0/", func(w http.ResponseWriter, r *http.Request) {
		if !up {
			// The Provider is being restarted: the connection is refused in a
			// cluster, and answers 503 here, which reaches Core the same way.
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"upstream_error","message":"provider restarting"}`)
			return
		}
		if r.URL.Path == "/v0/capabilities" {
			fmt.Fprint(w, `{"kind":"infra","provider":"fake","capabilities":["instances"]}`)
			return
		}
		fmt.Fprint(w, `{"instances":[{"id":"n1","name":"k3s-control","state":"RUNNING","shape":"VM.Standard.A1.Flex",
			"ocpus":1,"memoryGb":6,"faultDomain":"FAULT-DOMAIN-3","privateIp":"10.0.0.95"}]}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Short lives, so the snapshot ages within the test rather than over five
	// minutes of real time.
	cache := readyCache(t, infraCache(t, server.URL, infra.CacheOptions{
		TTL:        50 * time.Millisecond,
		StaleAfter: 100 * time.Millisecond,
		Timeout:    2 * time.Second,
	}))

	fresh := getInfra(t, cache)
	require.Len(t, fresh["nodes"].([]any), 1)
	require.Equal(t, false, fresh["stale"])

	up = false
	time.Sleep(300 * time.Millisecond)

	after := getInfra(t, cache)
	assert.Equal(t, true, after["stale"], "the outage is visible")
	nodes := after["nodes"].([]any)
	require.Len(t, nodes, 1, "the last good snapshot is still on screen")
	assert.Equal(t, "k3s-control", nodes[0].(map[string]any)["name"])
}

// With no Provider configured at all, every source is absent — and the
// dashboard still renders, naming what it does not have.
func TestGetInfraRendersWithNoProviderConfigured(t *testing.T) {
	cache := readyCache(t, infra.NewCache(func(ctx context.Context) (*infra.Collector, error) {
		return infra.NewCollector(infra.DefaultConfig(), provider.InfraSources(ctx, nil, infra.Sources{})), nil
	}, infra.CacheOptions{}))

	data := getInfra(t, cache)
	assert.Equal(t, []any{}, data["nodes"])
	assert.NotNil(t, data["capacity"])
	assertWarns(t, warningsOf(t, data), "instances: ", "not configured")
}

// The endpoint must explain itself instead of panicking or serving an empty
// dashboard when it has nothing at all to show.
func TestGetInfraReportsUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		handler  *Handler
		contains string
	}{
		{
			name:     "not configured",
			handler:  &Handler{},
			contains: "not configured",
		},
		{
			name: "sources cannot be built",
			handler: &Handler{Infra: infra.NewCache(func(context.Context) (*infra.Collector, error) {
				return nil, errors.New("instance principal authentication unavailable")
			}, infra.CacheOptions{})},
			contains: "instance principal authentication unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.handler.Infra != nil {
				tt.handler.Infra.Prime()
				time.Sleep(50 * time.Millisecond)
			}

			res, err := infraApp(t, tt.handler).Test(httptest.NewRequest(fiber.MethodGet, "/v1/infra", nil))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, fiber.StatusServiceUnavailable, res.StatusCode)

			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			var errResp struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(body, &errResp))
			assert.Equal(t, "infra_unavailable", errResp.Error)
			assert.Contains(t, errResp.Message, tt.contains)
		})
	}
}
