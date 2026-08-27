// Package conformance runs the published Provider contract against any
// Provider URL and reports what it found, one line per Capability.
//
// It is the definition of a working Provider. A Provider author runs it against
// their own service before asking anyone to review it; the maintainers run it
// against the in-tree Providers in CI, so a change to the contract that breaks
// an implementation fails the build rather than being discovered by an author.
//
// A Capability a Provider does not implement is reported as such and is not a
// failure — a partial implementation earns a partial pass. Only a genuine
// contract violation fails.
package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
	"github.com/sasiruLK/tinycloud-platform/internal/provider"
)

// Outcome is what running one Capability's checks produced.
type Outcome string

const (
	// Pass means the Capability answered, and answered in contract.
	Pass Outcome = "PASS"
	// NotImplemented means the Provider says it does not serve this
	// Capability, and behaves consistently with saying so. Not a failure.
	NotImplemented Outcome = "NOT IMPLEMENTED"
	// Fail means the Provider violated the contract.
	Fail Outcome = "FAIL"
)

// Result is one Capability's outcome.
type Result struct {
	Capability string  `json:"capability"`
	Outcome    Outcome `json:"outcome"`
	Detail     string  `json:"detail,omitempty"`
}

// Report is the whole run.
type Report struct {
	// Provider is the identifier the Provider gave for itself, if any.
	Provider string `json:"provider,omitempty"`
	// Kind is the Provider kind it declared.
	Kind string `json:"kind,omitempty"`
	// Declared are the Capabilities it says it serves.
	Declared []string `json:"declared"`
	// Results is one entry per Capability of the kind, in contract order.
	Results []Result `json:"results"`
}

// Passed reports whether the Provider is conformant. A Capability that is
// merely absent does not fail the run.
func (r Report) Passed() bool {
	for _, result := range r.Results {
		if result.Outcome == Fail {
			return false
		}
	}
	return true
}

// String renders the report as the suite prints it.
func (r Report) String() string {
	var b strings.Builder
	name := r.Provider
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Fprintf(&b, "provider %s, kind %s\n", name, r.Kind)
	fmt.Fprintf(&b, "declares: %s\n\n", strings.Join(r.Declared, ", "))

	for _, result := range r.Results {
		fmt.Fprintf(&b, "  %-16s %-16s", result.Capability, result.Outcome)
		if result.Detail != "" {
			fmt.Fprintf(&b, " %s", result.Detail)
		}
		b.WriteString("\n")
	}

	if r.Passed() {
		fmt.Fprintf(&b, "\nconformant\n")
	} else {
		fmt.Fprintf(&b, "\nnot conformant\n")
	}
	return b.String()
}

// Suite runs the contract against one Provider.
type Suite struct {
	baseURL string
	token   string
	client  *http.Client
}

// New returns a Suite checking the Provider at baseURL with the bearer token
// given.
func New(baseURL, token string, timeout time.Duration) *Suite {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Suite{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: timeout},
	}
}

// Run exercises every Capability and returns what it found. An error is
// returned only when the Provider could not be asked what it serves at all —
// everything after that is reported per Capability rather than aborting.
func (s *Suite) Run(ctx context.Context) (Report, error) {
	report := Report{Declared: []string{}, Results: []Result{}}

	status, body, err := s.get(ctx, "/"+provider.ContractVersion+"/capabilities")
	if err != nil {
		return report, fmt.Errorf("capability discovery: %w", err)
	}
	if status != http.StatusOK {
		return report, fmt.Errorf("capability discovery: expected 200, got %d: %s", status, snippet(body))
	}

	var discovery struct {
		Kind         string   `json:"kind"`
		Provider     string   `json:"provider"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &discovery); err != nil {
		return report, fmt.Errorf("capability discovery: response is not the documented shape: %w", err)
	}
	if discovery.Kind != provider.KindInfra {
		return report, fmt.Errorf("capability discovery: kind %q is not %q", discovery.Kind, provider.KindInfra)
	}

	report.Kind, report.Provider = discovery.Kind, discovery.Provider
	declared := map[string]bool{}
	for _, capability := range discovery.Capabilities {
		if !provider.ValidCapability(capability) {
			report.Results = append(report.Results, Result{
				Capability: capability,
				Outcome:    Fail,
				Detail:     "declared capability is not one the contract defines",
			})
			continue
		}
		declared[capability] = true
		report.Declared = append(report.Declared, capability)
	}
	sort.Strings(report.Declared)

	// Authentication is part of the contract, not an implementation detail:
	// anything in a cluster that can route to a Provider must not be able to
	// read its Substrate.
	report.Results = append(report.Results, s.checkAuthentication(ctx))

	for _, capability := range provider.InfraCapabilities {
		report.Results = append(report.Results, s.checkCapability(ctx, capability, declared[capability]))
	}
	return report, nil
}

// checkAuthentication asserts that an unauthenticated request is refused.
func (s *Suite) checkAuthentication(ctx context.Context) Result {
	result := Result{Capability: "authentication"}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.baseURL+"/"+provider.ContractVersion+"/capabilities", nil)
	if err != nil {
		return fail(result, err.Error())
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fail(result, err.Error())
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)

	if res.StatusCode != http.StatusUnauthorized {
		return fail(result, fmt.Sprintf("a request with no bearer token got %d, expected 401", res.StatusCode))
	}
	result.Outcome = Pass
	return result
}

// checkCapability runs one Capability's checks. A Capability that was not
// declared must answer 501; one that was must answer in contract.
func (s *Suite) checkCapability(ctx context.Context, capability string, declared bool) Result {
	result := Result{Capability: capability}

	path := "/" + provider.ContractVersion + "/infra/" + capability
	query := ""
	if capability == provider.CapabilityMetrics {
		query = "?metric=" + infra.MetricCPUUtilization + "&window=300"
	}

	status, body, err := s.get(ctx, path+query)
	if err != nil {
		return fail(result, err.Error())
	}

	if !declared {
		if status != http.StatusNotImplemented {
			return fail(result, fmt.Sprintf(
				"not declared in capability discovery, so it must answer 501, but answered %d", status))
		}
		result.Outcome = NotImplemented
		return result
	}

	if status == http.StatusNotImplemented {
		return fail(result, "declared in capability discovery but answers 501")
	}
	if status != http.StatusOK {
		return fail(result, fmt.Sprintf("expected 200, got %d: %s", status, snippet(body)))
	}

	if detail := checkBody(capability, body); detail != "" {
		return fail(result, detail)
	}

	// The metric Capability is one endpoint over several metrics, and every
	// metric the contract names must be answerable.
	if capability == provider.CapabilityMetrics {
		if detail := s.checkEveryMetric(ctx, path); detail != "" {
			return fail(result, detail)
		}
	}

	result.Outcome = Pass
	return result
}

// checkEveryMetric asserts that each metric the contract defines is accepted,
// and that one it does not define is refused rather than guessed at.
func (s *Suite) checkEveryMetric(ctx context.Context, path string) string {
	for _, metric := range infra.Metrics {
		status, body, err := s.get(ctx, path+"?metric="+metric+"&window=300")
		if err != nil {
			return fmt.Sprintf("metric %s: %v", metric, err)
		}
		if status != http.StatusOK {
			return fmt.Sprintf("metric %s: expected 200, got %d: %s", metric, status, snippet(body))
		}
		if detail := checkBody(provider.CapabilityMetrics, body); detail != "" {
			return fmt.Sprintf("metric %s: %s", metric, detail)
		}
	}

	status, _, err := s.get(ctx, path+"?metric=not.a.contract.metric&window=300")
	if err != nil {
		return err.Error()
	}
	if status != http.StatusBadRequest {
		return fmt.Sprintf("a metric the contract does not define got %d, expected 400", status)
	}

	status, _, err = s.get(ctx, path+"?metric="+infra.MetricCPUUtilization)
	if err != nil {
		return err.Error()
	}
	if status != http.StatusBadRequest {
		return fmt.Sprintf("a request with no window got %d, expected 400", status)
	}
	return ""
}

// checkBody asserts the response is the shape the contract documents. It
// decodes into the same types Core does, so anything Core would choke on is
// caught here instead.
func checkBody(capability string, body []byte) string {
	var err error
	switch capability {
	case provider.CapabilityInstances:
		var decoded struct {
			Instances *[]infra.InstanceInfo `json:"instances"`
		}
		if err = json.Unmarshal(body, &decoded); err == nil && decoded.Instances == nil {
			return "response has no `instances` array"
		}
	case provider.CapabilityMetrics:
		var decoded struct {
			Series *[]infra.Series `json:"series"`
		}
		if err = json.Unmarshal(body, &decoded); err == nil && decoded.Series == nil {
			return "response has no `series` array"
		}
	case provider.CapabilityAlarms:
		var decoded struct {
			Alarms *[]infra.AlarmStatus `json:"alarms"`
		}
		if err = json.Unmarshal(body, &decoded); err == nil && decoded.Alarms == nil {
			return "response has no `alarms` array"
		}
	case provider.CapabilityIngress:
		var decoded struct {
			PublicIP *string `json:"publicIp"`
		}
		if err = json.Unmarshal(body, &decoded); err == nil && decoded.PublicIP == nil {
			return "response has no `publicIp` field; an address still pending is the empty string"
		}
	case provider.CapabilityBackups:
		var decoded struct {
			Objects *[]infra.ObjectInfo `json:"objects"`
		}
		if err = json.Unmarshal(body, &decoded); err == nil && decoded.Objects == nil {
			return "response has no `objects` array"
		}
	}
	if err != nil {
		return fmt.Sprintf("response is not the documented shape: %v", err)
	}
	return ""
}

// get performs one authenticated call and returns its status and body.
func (s *Suite) get(ctx context.Context, path string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return res.StatusCode, nil, err
	}
	return res.StatusCode, body, nil
}

func fail(result Result, detail string) Result {
	result.Outcome, result.Detail = Fail, detail
	return result
}

func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200] + "…"
	}
	return text
}
