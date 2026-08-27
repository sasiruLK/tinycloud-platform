package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
)

// Entry is one configured Provider: which kind it implements, what to call it,
// where it lives, and how to authenticate to it.
//
// Providers are listed in configuration rather than discovered by label or by
// custom resource, so that an operator can see at a glance where the
// dashboard's data comes from. Per ADR-0001 moving to a `Provider` custom
// resource later is additive — the same fields from a different source.
type Entry struct {
	// Kind is the Provider kind, currently only "infra".
	Kind string `json:"kind"`
	// Name identifies this Provider in warnings and logs.
	Name string `json:"name"`
	// BaseURL is the root the `/v0` paths hang off, without a trailing slash.
	BaseURL string `json:"baseUrl"`
	// TokenFile is the path the Provider's bearer token is mounted at,
	// normally a projected Kubernetes Secret. Read per call, so rotating the
	// Secret rotates the credential without redeploying Core.
	TokenFile string `json:"tokenFile,omitempty"`
	// Token is the bearer token inline. For local development: a token in
	// configuration is a token in `kubectl get`, so in a cluster use TokenFile.
	Token string `json:"token,omitempty"`
}

// Validate reports whether the entry can be used. A malformed entry is a
// configuration mistake and is worth refusing loudly at startup — unlike a
// Provider that is merely absent, which is not an error at all.
func (e Entry) Validate() error {
	if e.Kind != KindInfra {
		return fmt.Errorf("provider %q: unknown kind %q (this build serves %q)", e.Name, e.Kind, KindInfra)
	}
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("provider with base URL %q: name is required", e.BaseURL)
	}
	parsed, err := url.Parse(e.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("provider %q: baseUrl %q is not an absolute http(s) URL", e.Name, e.BaseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("provider %q: baseUrl scheme %q is not http or https", e.Name, parsed.Scheme)
	}
	if e.TokenFile == "" && e.Token == "" {
		// Core reaches Providers only over an authenticated connection: an
		// unauthenticated Provider would answer anything else in the cluster
		// that can route to it.
		return fmt.Errorf("provider %q: one of tokenFile or token is required", e.Name)
	}
	return nil
}

// tokenSource returns how this entry's bearer token is obtained.
func (e Entry) tokenSource() TokenSource {
	if e.TokenFile != "" {
		return FileToken(e.TokenFile)
	}
	return StaticToken(e.Token)
}

// LoadEntries reads the configured Providers from an inline JSON list or from
// a file containing the same JSON — a mounted ConfigMap, typically. Both empty
// means no Providers are configured, which is a valid Instance: the dashboard
// renders with its sources named as missing.
func LoadEntries(inline, path string) ([]Entry, error) {
	raw := strings.TrimSpace(inline)
	if raw == "" && path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read provider configuration from %s: %w", path, err)
		}
		raw = strings.TrimSpace(string(content))
	}
	if raw == "" {
		return nil, nil
	}

	var entries []Entry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("provider configuration is not a JSON list of providers: %w", err)
	}
	for _, e := range entries {
		if err := e.Validate(); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// anyInfraSource is implemented by everything that can stand in for a
// Capability: the HTTP client, and the stub that reports one as unimplemented.
type anyInfraSource interface {
	infra.InstanceSource
	infra.MetricSource
	infra.AlarmSource
	infra.IngressSource
	infra.BackupSource
}

// infraSlots pairs each Capability with the source it fills in, so that
// composing the five is one loop rather than five near-identical branches.
var infraSlots = []struct {
	capability string
	filled     func(infra.Sources) bool
	fill       func(*infra.Sources, anyInfraSource)
}{
	{CapabilityInstances, func(s infra.Sources) bool { return s.Instances != nil },
		func(s *infra.Sources, v anyInfraSource) { s.Instances = v }},
	{CapabilityMetrics, func(s infra.Sources) bool { return s.Metrics != nil },
		func(s *infra.Sources, v anyInfraSource) { s.Metrics = v }},
	{CapabilityAlarms, func(s infra.Sources) bool { return s.Alarms != nil },
		func(s *infra.Sources, v anyInfraSource) { s.Alarms = v }},
	{CapabilityIngress, func(s infra.Sources) bool { return s.Ingress != nil },
		func(s *infra.Sources, v anyInfraSource) { s.Ingress = v }},
	{CapabilityBackups, func(s infra.Sources) bool { return s.Backups != nil },
		func(s *infra.Sources, v anyInfraSource) { s.Backups = v }},
}

// InfraSources composes the collector's five sources out of the configured
// Infra Providers.
//
// A Capability goes to the first Provider that declares it. What no Provider
// declares falls back to fallback — the in-process Oracle Cloud reads, when an
// operator has configured them — and what neither supplies is answered with a
// "not implemented" error naming the Capability, so the dashboard can say "not
// supported" rather than "broken". With nothing configured at all every source
// is nil, which the collector already reports as a warning per source.
//
// A Provider that cannot be reached to be asked what it serves is wired up for
// whatever is left over after that, rather than being dropped: it is down, not
// absent, so each call names it in a warning and retries discovery, and it
// starts answering by itself when it comes back. It ranks below the fallback
// precisely because nobody knows what it serves — a Provider that is merely
// unreachable must not take alarms away from Oracle reads that work.
func InfraSources(ctx context.Context, entries []Entry, fallback infra.Sources) infra.Sources {
	var (
		configured   bool
		byCapability = map[string]*Client{}
		undiscovered []*Client
	)

	for _, e := range entries {
		if e.Kind != KindInfra {
			continue
		}
		configured = true

		client := NewClient(e.Name, e.BaseURL, e.tokenSource())
		declared, err := client.Capabilities(ctx)
		if err != nil {
			log.Printf("[WARN] provider %q could not be asked what it serves: %v", e.Name, err)
			undiscovered = append(undiscovered, client)
			continue
		}
		for _, capability := range declared {
			if _, taken := byCapability[capability]; !taken {
				byCapability[capability] = client
			}
		}
	}

	src := fallback
	for _, slot := range infraSlots {
		switch {
		case byCapability[slot.capability] != nil:
			slot.fill(&src, byCapability[slot.capability])
		case slot.filled(src):
			// The fallback supplies this one.
		case len(undiscovered) > 0:
			slot.fill(&src, undiscovered[0])
		case configured:
			slot.fill(&src, unimplemented{capability: slot.capability})
		}
	}
	return src
}
