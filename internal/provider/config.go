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

// InfraSources composes the collector's five sources out of the configured
// Infra Providers.
//
// Each Capability comes from the first Provider that declares it. What no
// Provider declares falls back to fallback — the in-process Oracle Cloud reads,
// when an operator has configured them — and what neither supplies is answered
// with a "not implemented" error naming the Capability, so the dashboard can
// say "not supported" rather than "broken". With nothing configured at all
// every source is nil, which the collector already reports as a warning per
// source.
//
// A Provider that cannot be reached to be asked what it serves is wired up
// anyway, claiming whatever no other Provider has claimed. It is down, not
// absent: each Capability call then names it in a warning and retries
// discovery, so it starts answering by itself when it comes back rather than
// waiting for Core to restart.
func InfraSources(ctx context.Context, entries []Entry, fallback infra.Sources) infra.Sources {
	var (
		configured   bool
		byCapability = map[string]*Client{}
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
			declared = InfraCapabilities
		}
		for _, capability := range declared {
			if _, taken := byCapability[capability]; !taken {
				byCapability[capability] = client
			}
		}
	}

	src := fallback
	if client, ok := byCapability[CapabilityInstances]; ok {
		src.Instances = client
	} else if src.Instances == nil && configured {
		src.Instances = unimplemented{capability: CapabilityInstances}
	}
	if client, ok := byCapability[CapabilityMetrics]; ok {
		src.Metrics = client
	} else if src.Metrics == nil && configured {
		src.Metrics = unimplemented{capability: CapabilityMetrics}
	}
	if client, ok := byCapability[CapabilityAlarms]; ok {
		src.Alarms = client
	} else if src.Alarms == nil && configured {
		src.Alarms = unimplemented{capability: CapabilityAlarms}
	}
	if client, ok := byCapability[CapabilityIngress]; ok {
		src.Ingress = client
	} else if src.Ingress == nil && configured {
		src.Ingress = unimplemented{capability: CapabilityIngress}
	}
	if client, ok := byCapability[CapabilityBackups]; ok {
		src.Backups = client
	} else if src.Backups == nil && configured {
		src.Backups = unimplemented{capability: CapabilityBackups}
	}
	return src
}
