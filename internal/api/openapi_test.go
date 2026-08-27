package api

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	tinycloud "github.com/sasiruLK/tinycloud-platform"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Provider section of this API's documentation is generated from the
// contract, so a Capability added to the contract appears in the served
// documentation without anyone remembering to write it down twice.
func TestProviderDocumentationIsGeneratedFromTheContract(t *testing.T) {
	app := fiber.New()
	app.Get("/openapi.json", OpenAPISpec)

	res, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/openapi.json", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, fiber.StatusOK, res.StatusCode)

	var spec struct {
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tags"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&spec))

	var providers string
	for _, tag := range spec.Tags {
		if tag.Name == "providers" {
			providers = tag.Description
		}
	}
	require.NotEmpty(t, providers, "the served documentation has a providers section")

	for _, capability := range provider.InfraCapabilities {
		assert.Contains(t, providers, "GET /v0/infra/"+capability,
			"every capability of the contract is documented")
	}
	assert.Contains(t, providers, "GET /v0/capabilities")
}

// The Provider contract is served as the published document itself, not as a
// description of it maintained alongside it. A Provider author generating a
// client from what a running instance serves must get the same file the
// repository publishes, or the two have already drifted.
func TestProviderContractIsServedVerbatim(t *testing.T) {
	app := fiber.New()
	app.Get("/provider-contract.yaml", ProviderContract)

	res, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/provider-contract.yaml", nil))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, fiber.StatusOK, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, string(tinycloud.ProviderContractV0), string(body))
	assert.Contains(t, res.Header.Get(fiber.HeaderContentType), "application/yaml")
}
