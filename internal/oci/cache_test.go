package oci

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// countingCache wires a cache to fake sources and counts how many collections
// actually reached "OCI".
func countingCache(t *testing.T, clock *fakeClock) (*Cache, *int64) {
	t.Helper()
	var collects int64
	src := healthySources(t)
	inner := src.Instances
	src.Instances = fakeInstances(func(ctx context.Context) ([]InstanceInfo, error) {
		atomic.AddInt64(&collects, 1)
		return inner.ListInstances(ctx)
	})

	c := NewCache(func(context.Context) (*Collector, error) {
		return testCollector(t, src), nil
	}, CacheOptions{TTL: 60 * time.Second, StaleAfter: 5 * time.Minute, Timeout: 5 * time.Second})
	c.nowFunc = clock.now
	return c, &collects
}

// The UI polls this endpoint and OCI Monitoring meters requests, so repeated
// reads inside the TTL must not reach OCI at all.
func TestCacheServesFromMemoryWithinTTL(t *testing.T) {
	clock := &fakeClock{t: testTime(t, "2026-08-15T19:00:00Z")}
	c, collects := countingCache(t, clock)

	c.Prime()
	require.True(t, c.waitForRefresh(2*time.Second))

	for i := 0; i < 5; i++ {
		clock.advance(10 * time.Second)
		snap, err := c.Get()
		require.NoError(t, err)
		require.NotNil(t, snap)
		assert.False(t, snap.Stale)
	}
	assert.Equal(t, int64(1), atomic.LoadInt64(collects), "five polls, one collection")
}

// Past the TTL the caller still gets the cached snapshot immediately; the
// refresh happens behind the response.
func TestCacheRefreshesInBackgroundPastTTL(t *testing.T) {
	clock := &fakeClock{t: testTime(t, "2026-08-15T19:00:00Z")}
	c, collects := countingCache(t, clock)

	c.Prime()
	require.True(t, c.waitForRefresh(2*time.Second))
	require.Equal(t, int64(1), atomic.LoadInt64(collects))

	clock.advance(61 * time.Second)
	snap, err := c.Get()
	require.NoError(t, err, "the stale-but-usable snapshot is served, not an error")
	require.NotNil(t, snap)

	require.True(t, c.waitForRefresh(2*time.Second))
	assert.Equal(t, int64(2), atomic.LoadInt64(collects), "refresh ran after the request was answered")
}

// A cache older than five minutes is still served, flagged so the UI can say so.
func TestCacheFlagsStaleSnapshots(t *testing.T) {
	tests := []struct {
		name      string
		age       time.Duration
		wantStale bool
	}{
		{"fresh", 0, false},
		{"past ttl but not stale", 90 * time.Second, false},
		{"exactly five minutes", 5 * time.Minute, false},
		{"older than five minutes", 5*time.Minute + time.Second, true},
		{"hours old", 3 * time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := testTime(t, "2026-08-15T19:00:00Z")
			clock := &fakeClock{t: base}

			// No collector factory: refreshes triggered by age fail, which is
			// exactly the situation where a stale snapshot must survive.
			c := NewCache(nil, CacheOptions{TTL: 60 * time.Second, StaleAfter: 5 * time.Minute})
			c.nowFunc = clock.now
			c.snap = newSnapshot(base)
			c.fetchedAt = base

			clock.advance(tt.age)
			snap, err := c.Get()
			require.NoError(t, err)
			assert.Equal(t, tt.wantStale, snap.Stale)

			require.True(t, c.waitForRefresh(2*time.Second))
			again, err := c.Get()
			require.NoError(t, err, "a failed refresh must not discard the snapshot")
			assert.Equal(t, tt.wantStale, again.Stale)
		})
	}
}

// On a laptop the instance principal provider fails. The endpoint has to say
// why rather than panic or return an empty dashboard.
func TestCacheReportsCollectorConstructionFailure(t *testing.T) {
	clock := &fakeClock{t: testTime(t, "2026-08-15T19:00:00Z")}
	c := NewCache(func(context.Context) (*Collector, error) {
		return nil, errors.New("instance principal authentication unavailable")
	}, CacheOptions{TTL: 60 * time.Second})
	c.nowFunc = clock.now

	c.Prime()
	require.True(t, c.waitForRefresh(2*time.Second))

	snap, err := c.Get()
	assert.Nil(t, snap)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotReady)
	assert.Contains(t, err.Error(), "instance principal authentication unavailable")
}

// A broken provider or a missing IAM policy must not turn every poll into a
// retry against the metadata service.
func TestCacheThrottlesFailedRefreshes(t *testing.T) {
	clock := &fakeClock{t: testTime(t, "2026-08-15T19:00:00Z")}
	var attempts int64
	c := NewCache(func(context.Context) (*Collector, error) {
		atomic.AddInt64(&attempts, 1)
		return nil, errors.New("NotAuthorizedOrNotFound")
	}, CacheOptions{TTL: 60 * time.Second})
	c.nowFunc = clock.now

	for i := 0; i < 4; i++ {
		_, err := c.Get()
		require.Error(t, err)
		require.True(t, c.waitForRefresh(2*time.Second))
		clock.advance(10 * time.Second)
	}
	assert.Equal(t, int64(1), atomic.LoadInt64(&attempts), "throttled to one attempt per TTL")

	clock.advance(61 * time.Second)
	_, err := c.Get()
	require.Error(t, err)
	require.True(t, c.waitForRefresh(2*time.Second))
	assert.Equal(t, int64(2), atomic.LoadInt64(&attempts), "retried once the TTL elapsed")
}

// A cold cache must answer instantly rather than waiting on a multi-call
// fan-out that may take seconds.
func TestCacheNeverBlocksOnColdCollection(t *testing.T) {
	release := make(chan struct{})
	src := healthySources(t)
	src.Instances = fakeInstances(func(ctx context.Context) ([]InstanceInfo, error) {
		<-release
		return nil, nil
	})

	c := NewCache(func(context.Context) (*Collector, error) {
		return testCollector(t, src), nil
	}, CacheOptions{TTL: 60 * time.Second, Timeout: 5 * time.Second})

	start := time.Now()
	_, err := c.Get()
	elapsed := time.Since(start)
	close(release)

	require.ErrorIs(t, err, ErrNotReady)
	assert.Less(t, elapsed, 100*time.Millisecond, "request returned without waiting for OCI")
	require.True(t, c.waitForRefresh(2*time.Second))
}
