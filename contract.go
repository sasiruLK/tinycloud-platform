// Package tinycloud is the repository root, and exists to embed the published
// Provider contract so that everything which describes the contract reads the
// same file: the Provider server that answers it, the Conformance suite that
// exercises it, and the API documentation Core serves.
//
// Nothing else lives here. Core is under internal/, its binaries under cmd/.
package tinycloud

import _ "embed"

// ProviderContractV0 is the published OpenAPI document for the `/v0` Provider
// contract. It is the source of truth: the Provider section of the served API
// documentation is this file, not a hand-maintained copy of it, so the two
// cannot drift apart.
//
//go:embed provider-contract-v0.yaml
var ProviderContractV0 []byte
