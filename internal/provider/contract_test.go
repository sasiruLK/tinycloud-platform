package provider

import (
	"testing"

	tinycloud "github.com/sasiruLK/tinycloud-platform"
	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// The published contract is the source of truth for what a Provider must
// implement, so it has to describe the same thing this code does. These tests
// fail when the document and the implementation drift apart — which, on a
// contract other people write against, is the drift that matters.

func contract(t *testing.T) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(tinycloud.ProviderContractV0, &document))
	return document
}

// paths returns the paths the document defines.
func paths(t *testing.T) map[string]bool {
	t.Helper()
	raw, ok := contract(t)["paths"].(map[any]any)
	require.True(t, ok, "the contract defines paths")

	out := map[string]bool{}
	for path := range raw {
		out[path.(string)] = true
	}
	return out
}

// enumOf walks the document to a schema and returns its enum values.
func enumOf(t *testing.T, schema string) []string {
	t.Helper()
	components := contract(t)["components"].(map[any]any)
	schemas := components["schemas"].(map[any]any)
	definition, ok := schemas[schema].(map[any]any)
	require.True(t, ok, "the contract defines schema %s", schema)

	values, ok := definition["enum"].([]any)
	require.True(t, ok, "schema %s is an enum", schema)

	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.(string))
	}
	return out
}

func TestContractDocumentsEveryCapabilityServed(t *testing.T) {
	documented := paths(t)

	for _, capability := range InfraCapabilities {
		assert.True(t, documented[pathFor(capability)],
			"capability %q is served at %s but the contract does not document it", capability, pathFor(capability))
	}
	assert.True(t, documented[pathCapabilities], "capability discovery is documented")
	assert.True(t, documented[pathHealth], "the liveness path is documented")

	// And nothing is documented that is not served, which would send a
	// Provider author implementing an endpoint Core never calls.
	served := map[string]bool{pathCapabilities: true, pathHealth: true}
	for _, capability := range InfraCapabilities {
		served[pathFor(capability)] = true
	}
	for path := range documented {
		assert.True(t, served[path], "the contract documents %s, which nothing serves", path)
	}
}

func TestContractDocumentsEveryMetricAndCapabilityName(t *testing.T) {
	assert.ElementsMatch(t, infra.Metrics, enumOf(t, "MetricName"),
		"the metric names the collector asks for are the ones the contract defines")

	components := contract(t)["components"].(map[any]any)
	schemas := components["schemas"].(map[any]any)
	capabilities := schemas["Capabilities"].(map[any]any)
	properties := capabilities["properties"].(map[any]any)
	items := properties["capabilities"].(map[any]any)["items"].(map[any]any)

	declared := make([]string, 0, len(items["enum"].([]any)))
	for _, v := range items["enum"].([]any) {
		declared = append(declared, v.(string))
	}
	assert.ElementsMatch(t, InfraCapabilities, declared)
}

func TestContractIsVersionedInTheURL(t *testing.T) {
	for path := range paths(t) {
		if path == pathHealth {
			// Liveness is not a Capability and carries no version.
			continue
		}
		assert.Contains(t, path, "/"+ContractVersion+"/",
			"a Provider author must be able to tell from the URL when a change is expected to break them")
	}
}
