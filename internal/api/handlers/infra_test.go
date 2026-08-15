package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sasiruLK/tinycloud-platform/internal/oci"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubInstances is the only OCI source these tests wire up: the rest stay nil,
// which also exercises the partial-failure path end to end.
type stubInstances struct {
	instances []oci.InstanceInfo
	err       error
}

func (s stubInstances) ListInstances(context.Context) ([]oci.InstanceInfo, error) {
	return s.instances, s.err
}

func infraApp(t *testing.T, h *Handler) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Get("/v1/infra", h.GetInfra)
	return app
}

// readyCache returns a cache that has already collected once.
func readyCache(t *testing.T, src oci.Sources) *oci.Cache {
	t.Helper()
	cache := oci.NewCache(func(context.Context) (*oci.Collector, error) {
		return oci.NewCollector(oci.DefaultConfig(), src), nil
	}, oci.CacheOptions{})
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

func TestGetInfraServesSnapshot(t *testing.T) {
	cache := readyCache(t, oci.Sources{
		Instances: stubInstances{instances: []oci.InstanceInfo{{
			Name: "k3s-control", State: "RUNNING", Shape: "VM.Standard.A1.Flex",
			OCPUs: 1, MemoryGB: 6, FaultDomain: "FAULT-DOMAIN-3", PrivateIP: "10.0.0.95",
		}}},
	})

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

	for _, key := range []string{"updatedAt", "stale", "nodes", "alarms", "capacity"} {
		assert.Contains(t, envelope.Data, key)
	}

	node := envelope.Data["nodes"].([]any)[0].(map[string]any)
	assert.Equal(t, "k3s-control", node["name"])
	assert.Equal(t, "control-plane", node["role"])
	assert.Equal(t, "10.0.0.95", node["privateIp"])
	// Monitoring was never wired here, so the percentages are null, not zero.
	assert.Nil(t, node["cpuPercent"])
	assert.Nil(t, node["memoryPercent"])
	assert.NotEmpty(t, envelope.Data["warnings"], "the sources that failed are named")
}

// The endpoint must explain itself instead of panicking or serving an empty
// dashboard when it cannot authenticate.
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
			name: "instance principals unavailable",
			handler: &Handler{Infra: oci.NewCache(func(context.Context) (*oci.Collector, error) {
				return nil, errors.New("instance principal authentication unavailable")
			}, oci.CacheOptions{})},
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
