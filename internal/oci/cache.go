package oci

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Cache serves the infrastructure snapshot from memory and refreshes it in the
// background.
//
// The UI polls /v1/infra and one refresh is a dozen OCI calls, several of them
// metered by Monitoring. So a request never triggers a synchronous fan-out: it
// gets whatever is in memory and, if that is older than the TTL, leaves a
// refresh running behind it. Requests are cheap and predictable; OCI sees at
// most one refresh per TTL.
type Cache struct {
	// newCollector builds the collector on first use. Instance principals are
	// resolved through the metadata service, which is I/O that must not run in
	// main() — a laptop with no metadata service would then block or kill
	// startup. Deferring it here keeps the rest of the API starting normally
	// and turns the failure into an error on this endpoint alone.
	newCollector func(context.Context) (*Collector, error)

	ttl        time.Duration // age at which a snapshot is refreshed
	staleAfter time.Duration // age at which a snapshot is flagged stale
	timeout    time.Duration // budget for one whole refresh
	nowFunc    func() time.Time

	mu         sync.Mutex
	collector  *Collector
	snap       *Snapshot
	fetchedAt  time.Time
	attempted  time.Time // last refresh start, successful or not
	refreshing bool
	lastErr    error

	// done is closed after each refresh; tests wait on it.
	done chan struct{}
}

// ErrNotReady is returned while no snapshot has been collected yet.
var ErrNotReady = errors.New("infrastructure snapshot not collected yet")

// CacheOptions tunes the cache. Zero values fall back to the defaults.
type CacheOptions struct {
	TTL        time.Duration
	StaleAfter time.Duration
	Timeout    time.Duration
}

// NewCache returns a cache that collects through newCollector. Nothing is
// fetched until Prime or Get is called.
func NewCache(newCollector func(context.Context) (*Collector, error), opts CacheOptions) *Cache {
	if opts.TTL <= 0 {
		opts.TTL = 60 * time.Second
	}
	if opts.StaleAfter <= 0 {
		opts.StaleAfter = 5 * time.Minute
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	return &Cache{
		newCollector: newCollector,
		ttl:          opts.TTL,
		staleAfter:   opts.StaleAfter,
		timeout:      opts.Timeout,
		nowFunc:      time.Now,
		done:         make(chan struct{}),
	}
}

// NewDefaultCache returns the cache the API serves /v1/infra from: instance
// principal authentication, 60 second TTL, stale after five minutes.
//
// Authentication is resolved lazily on the first refresh, so constructing this
// never blocks startup and never fails — off an OCI instance it simply reports
// why on the endpoint itself.
func NewDefaultCache(cfg Config) *Cache {
	return NewCache(func(context.Context) (*Collector, error) {
		src, err := NewSources(cfg)
		if err != nil {
			return nil, err
		}
		return NewCollector(cfg, src), nil
	}, CacheOptions{})
}

func (c *Cache) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

// Prime starts the first collection in the background, so the first UI request
// after a rollout has a warm cache to read.
func (c *Cache) Prime() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startRefreshLocked()
}

// Get returns the cached snapshot, marked stale when it is older than the
// stale threshold, and kicks off a background refresh when it is older than
// the TTL. It never blocks on OCI. Until the first collection completes it
// returns ErrNotReady, wrapped with whatever the last attempt failed on.
func (c *Cache) Get() (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	age := c.now().Sub(c.fetchedAt)
	if c.snap == nil || age >= c.ttl {
		c.startRefreshLocked()
	}

	if c.snap == nil {
		if c.lastErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotReady, c.lastErr)
		}
		return nil, ErrNotReady
	}

	out := *c.snap
	out.Stale = age > c.staleAfter
	return &out, nil
}

// startRefreshLocked launches a refresh unless one is already running. A failed
// attempt is throttled to one per TTL, so a broken instance principal or a
// missing IAM policy does not turn every poll into a retry storm.
func (c *Cache) startRefreshLocked() {
	if c.refreshing {
		return
	}
	if !c.attempted.IsZero() && c.now().Sub(c.attempted) < c.ttl {
		return
	}
	c.refreshing = true
	c.attempted = c.now()
	go c.refresh()
}

// refresh collects a snapshot on its own context. It deliberately does not
// inherit a request context: the request is already answered by the time this
// finishes, and cancelling it would leave the cache cold forever.
func (c *Cache) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	snap, err := c.collect(ctx)

	c.mu.Lock()
	c.refreshing = false
	c.lastErr = err
	if err == nil {
		c.snap = snap
		c.fetchedAt = c.now()
	}
	close(c.done)
	c.done = make(chan struct{})
	c.mu.Unlock()

	if err != nil {
		log.Printf("[WARN] infra snapshot refresh failed: %v", err)
	}
}

// collect builds the collector if needed, then runs one collection. Only a
// failure to build the collector — no instance principal, no OCI reachable at
// all — is an error; a partial collection is a valid snapshot.
func (c *Cache) collect(ctx context.Context) (*Snapshot, error) {
	c.mu.Lock()
	collector := c.collector
	c.mu.Unlock()

	if collector == nil {
		if c.newCollector == nil {
			return nil, errors.New("no OCI collector configured")
		}
		built, err := c.newCollector(ctx)
		if err != nil {
			return nil, err
		}
		collector = built
		c.mu.Lock()
		c.collector = collector
		c.mu.Unlock()
	}

	return collector.Collect(ctx), nil
}

// waitForRefresh blocks until the in-flight refresh finishes. Test helper.
func (c *Cache) waitForRefresh(timeout time.Duration) bool {
	c.mu.Lock()
	if !c.refreshing {
		c.mu.Unlock()
		return true
	}
	done := c.done
	c.mu.Unlock()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
