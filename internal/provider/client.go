package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
)

// capabilityTTL is how long a Provider's answer to Capability discovery is
// trusted. Long enough that discovery costs nothing on the cache's polling
// schedule; short enough that a Provider which gains a Capability, or comes
// back from an outage, is noticed without restarting Core.
const capabilityTTL = 5 * time.Minute

// Client reads one Provider over the `/v0` contract.
//
// It implements every source interface the collector consumes, which is the
// whole trick: the Provider client is one more implementation of the seam that
// already existed, so the collector, the cache, the infra endpoint and the UI
// are untouched by where the data now comes from.
type Client struct {
	name    string
	baseURL string
	token   TokenSource
	http    *http.Client
	nowFunc func() time.Time

	// Capability discovery is cached rather than resolved once at startup, so
	// that a Provider which is down when Core builds its collector is not
	// frozen out until the next restart: each call retries discovery, and the
	// failure is named against the Capability that wanted it.
	mu       sync.Mutex
	declared map[string]bool
	knownAt  time.Time
}

// NewClient returns a client for the Provider at baseURL. Per-call timeouts
// come from the collector, which bounds every Capability call, so the client
// sets none of its own beyond a generous ceiling that stops a Provider holding
// a connection open forever.
func NewClient(name, baseURL string, token TokenSource) *Client {
	return &Client{
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
		nowFunc: time.Now,
	}
}

func (c *Client) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

// Name is the identifier this Provider was configured under. It is what
// appears in a warning, so an operator with three Providers can tell which one
// is missing.
func (c *Client) Name() string { return c.name }

// Capabilities asks the Provider what it serves, and remembers the answer.
// Core reads only the Capabilities listed, so a Provider never has to stub an
// endpoint it has no data for.
func (c *Client) Capabilities(ctx context.Context) ([]string, error) {
	var body capabilitiesBody
	if err := c.get(ctx, pathCapabilities, nil, &body); err != nil {
		return nil, err
	}
	if body.Kind != KindInfra {
		return nil, fmt.Errorf("provider %q declares kind %q, not %q", c.name, body.Kind, KindInfra)
	}

	declared := make([]string, 0, len(body.Capabilities))
	for _, capability := range body.Capabilities {
		// An unknown Capability is a Provider written against a newer contract
		// than this Core. Ignoring it is the compatible reading: everything
		// this Core does understand still works.
		if ValidCapability(capability) {
			declared = append(declared, capability)
		}
	}

	c.mu.Lock()
	c.declared = capabilitySet(declared)
	c.knownAt = c.now()
	c.mu.Unlock()

	return declared, nil
}

// supports reports whether this Provider serves the Capability, rediscovering
// when what it serves is unknown or stale.
//
// A Capability the Provider does not declare fails here, before any request is
// made: Core does not call an endpoint a Provider has said it has not
// implemented. A Provider that cannot be reached to be asked fails with the
// reason, which the collector names against this Capability.
func (c *Client) supports(ctx context.Context, capability string) error {
	c.mu.Lock()
	declared, knownAt := c.declared, c.knownAt
	c.mu.Unlock()

	if declared == nil || c.now().Sub(knownAt) > capabilityTTL {
		if _, err := c.Capabilities(ctx); err != nil {
			return err
		}
		c.mu.Lock()
		declared = c.declared
		c.mu.Unlock()
	}

	if !declared[capability] {
		return notImplemented(capability, c.name)
	}
	return nil
}

// The five source interfaces, in the order the contract lists them.

// ListInstances implements infra.InstanceSource.
func (c *Client) ListInstances(ctx context.Context) ([]infra.InstanceInfo, error) {
	if err := c.supports(ctx, CapabilityInstances); err != nil {
		return nil, err
	}
	var body instancesBody
	if err := c.get(ctx, pathInstances, nil, &body); err != nil {
		return nil, err
	}
	return body.Instances, nil
}

// QueryMetric implements infra.MetricSource.
func (c *Client) QueryMetric(ctx context.Context, metric string, window time.Duration) ([]infra.Series, error) {
	if err := c.supports(ctx, CapabilityMetrics); err != nil {
		return nil, err
	}
	seconds := int(window.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	query := url.Values{"metric": {metric}, "window": {strconv.Itoa(seconds)}}

	var body seriesBody
	if err := c.get(ctx, pathMetrics, query, &body); err != nil {
		return nil, err
	}
	return body.Series, nil
}

// ListAlarmStatuses implements infra.AlarmSource.
func (c *Client) ListAlarmStatuses(ctx context.Context) ([]infra.AlarmStatus, error) {
	if err := c.supports(ctx, CapabilityAlarms); err != nil {
		return nil, err
	}
	var body alarmsBody
	if err := c.get(ctx, pathAlarms, nil, &body); err != nil {
		return nil, err
	}
	return body.Alarms, nil
}

// IngressPublicIP implements infra.IngressSource.
func (c *Client) IngressPublicIP(ctx context.Context) (string, error) {
	if err := c.supports(ctx, CapabilityIngress); err != nil {
		return "", err
	}
	var body ingressBody
	if err := c.get(ctx, pathIngress, nil, &body); err != nil {
		return "", err
	}
	return body.PublicIP, nil
}

// ListObjects implements infra.BackupSource.
func (c *Client) ListObjects(ctx context.Context) ([]infra.ObjectInfo, error) {
	if err := c.supports(ctx, CapabilityBackups); err != nil {
		return nil, err
	}
	var body backupsBody
	if err := c.get(ctx, pathBackups, nil, &body); err != nil {
		return nil, err
	}
	return body.Objects, nil
}

// get performs one contract call and decodes it into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("provider %q: %w", c.name, err)
	}
	req.Header.Set("Accept", "application/json")

	if c.token != nil {
		// Read the token per call so that rotating the Secret behind it takes
		// effect without redeploying Core.
		token, err := c.token()
		if err != nil {
			return fmt.Errorf("provider %q: %w", c.name, err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		// Unreachable, refused, timed out: the Capability is named by the
		// collector, and the last good snapshot stays on screen marked stale.
		return fmt.Errorf("provider %q: %w", c.name, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return c.statusError(res)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("provider %q: decode %s: %w", c.name, path, err)
	}
	return nil
}

// statusError turns a non-200 into the error Core reasons about: 501 is the
// Provider saying it does not serve this Capability, and is reported as "not
// implemented" rather than as a fault.
func (c *Client) statusError(res *http.Response) error {
	var body errorBody
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
	_ = json.Unmarshal(raw, &body)

	detail := body.Message
	if detail == "" {
		detail = strings.TrimSpace(string(raw))
	}

	if res.StatusCode == http.StatusNotImplemented {
		return notImplementedf("provider %q: %s", c.name, detail)
	}
	return fmt.Errorf("provider %q: %s: %s", c.name, res.Status, detail)
}

// unimplemented stands in for a Capability no configured Provider serves. It
// fails every call with ErrNotImplemented naming the Capability, so the
// snapshot's warning says "not supported by this Provider" instead of the
// nil-source wording, which means "you configured nothing at all".
type unimplemented struct {
	capability string
	provider   string
}

func (u unimplemented) err() error { return notImplemented(u.capability, u.provider) }

func (u unimplemented) ListInstances(context.Context) ([]infra.InstanceInfo, error) {
	return nil, u.err()
}
func (u unimplemented) QueryMetric(context.Context, string, time.Duration) ([]infra.Series, error) {
	return nil, u.err()
}
func (u unimplemented) ListAlarmStatuses(context.Context) ([]infra.AlarmStatus, error) {
	return nil, u.err()
}
func (u unimplemented) IngressPublicIP(context.Context) (string, error) { return "", u.err() }
func (u unimplemented) ListObjects(context.Context) ([]infra.ObjectInfo, error) {
	return nil, u.err()
}

// IsNotImplemented reports whether err is a Capability that is absent rather
// than broken.
func IsNotImplemented(err error) bool { return errors.Is(err, ErrNotImplemented) }
