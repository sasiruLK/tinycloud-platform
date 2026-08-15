package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReturnsNilWhenUnconfigured(t *testing.T) {
	assert.Nil(t, New(Config{}), "no token or repo")
	assert.Nil(t, New(Config{Token: "t"}), "no repo")
	assert.Nil(t, New(Config{WorkflowRepo: "o/r"}), "no token")
	assert.NotNil(t, New(Config{Token: "t", WorkflowRepo: "o/r"}))
}

// A nil Dispatcher must be callable: the coordinator starts without GitHub
// credentials and should surface a clear error at build time, not panic.
func TestNilDispatcherReturnsError(t *testing.T) {
	var d *Dispatcher
	err := d.Dispatch(context.Background(), "j1", "app", "https://github.com/o/r", "main", 8080)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestDispatchSendsExpectedPayload(t *testing.T) {
	var gotPath, gotAuth string
	var body dispatchBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	d := New(Config{
		Token: "tok", WorkflowRepo: "sasiruLK/tinycloud-platform",
		CoordinatorURL: "https://tinycloud.sasiru.lk/", ImagePrefix: "ghcr.io/sasirulk",
		HTTPClient: srv.Client(),
	})
	require.NotNil(t, d)
	d.cfg.baseURL = srv.URL

	require.NoError(t, d.Dispatch(context.Background(), "job-1", "my-app", "https://github.com/o/r", "main", 8080))

	assert.Equal(t, "/repos/sasiruLK/tinycloud-platform/dispatches", gotPath)
	assert.Equal(t, "Bearer tok", gotAuth)
	assert.Equal(t, EventType, body.EventType)
	assert.Equal(t, "job-1", body.ClientPayload.JobID)
	assert.Equal(t, "my-app", body.ClientPayload.AppName)
	assert.Equal(t, 8080, body.ClientPayload.Port)
	// Trailing slash must be trimmed or the workflow builds "…lk//v1/runner/…".
	assert.Equal(t, "https://tinycloud.sasiru.lk", body.ClientPayload.CoordinatorURL)
}

func TestDispatchSurfacesGitHubError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	d := New(Config{Token: "tok", WorkflowRepo: "o/r", HTTPClient: srv.Client()})
	d.cfg.baseURL = srv.URL

	err := d.Dispatch(context.Background(), "j", "a", "https://github.com/o/r", "main", 8080)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "Not Found")
}
