// Package ocilog ships platform events to OCI Logging.
//
// The build coordinator's history lives in one SQLite file on one node's disk.
// That file is the only record of what was built, when, and why a build failed,
// and local-path storage does not survive the node being rebuilt. OCI Logging
// is a second, off-cluster copy: the log group and the `use log-content` policy
// for the tinycloud-nodes dynamic group already existed and nothing had ever
// written to them.
//
// Two rules shape this package:
//
//   - Emitting must never slow down or fail a build. Every call is
//     non-blocking, drops rather than waits when the buffer is full, and
//     survives a nil receiver so an unconfigured coordinator behaves exactly as
//     it did before.
//   - PutLogs is a network call per batch, so entries are batched by count and
//     by time rather than sent one per log line. A verbose build otherwise
//     means thousands of round trips.
package ocilog

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/loggingingestion"
)

const (
	// Bounded so a stuck or slow Logging endpoint costs memory that is capped
	// rather than unbounded. Past this, entries are dropped: losing build log
	// lines from the secondary copy is acceptable, stalling the coordinator
	// that is serving them is not.
	bufferSize = 2048

	// PutLogs accepts far more than this per call; the limit here is about
	// bounding latency between an event happening and it being durable.
	batchSize     = 128
	flushInterval = 5 * time.Second
)

// Event is one thing worth recording. Type separates the stream of build log
// lines from the far rarer lifecycle transitions, so a search can ask for
// either without parsing messages.
type Event struct {
	Type    string // "build.status" or "build.log"
	JobID   string
	App     string
	Status  string
	Stream  string // stdout/stderr, for build.log
	Message string
	Image   string
	Tag     string
	Error   string
}

type Emitter struct {
	client loggingingestion.LoggingClient
	logID  string
	ch     chan entry
	wg     sync.WaitGroup
	once   sync.Once
	done   chan struct{}

	mu      sync.Mutex
	dropped int
}

type entry struct {
	when time.Time
	ev   Event
}

// New builds an Emitter authenticating as the instance it runs on. logID is the
// OCID of the destination log; an empty logID returns (nil, nil) so that
// leaving OCI_LOG_ID unset disables shipping rather than failing startup.
func New(logID string) (*Emitter, error) {
	if logID == "" {
		return nil, nil
	}

	provider, err := auth.InstancePrincipalConfigurationProvider()
	if err != nil {
		return nil, err
	}
	client, err := loggingingestion.NewLoggingClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, err
	}

	e := &Emitter{
		client: client,
		logID:  logID,
		ch:     make(chan entry, bufferSize),
		done:   make(chan struct{}),
	}
	e.wg.Add(1)
	go e.run()
	return e, nil
}

// Emit queues an event. It never blocks and is safe on a nil *Emitter, which is
// what a coordinator running without OCI Logging configured holds.
func (e *Emitter) Emit(ev Event) {
	if e == nil {
		return
	}
	select {
	case e.ch <- entry{when: time.Now().UTC(), ev: ev}:
	default:
		e.mu.Lock()
		e.dropped++
		e.mu.Unlock()
	}
}

// Close flushes what is buffered and stops the worker. Safe on a nil *Emitter
// and safe to call twice.
func (e *Emitter) Close() {
	if e == nil {
		return
	}
	e.once.Do(func() {
		close(e.done)
		e.wg.Wait()
		e.mu.Lock()
		dropped := e.dropped
		e.mu.Unlock()
		if dropped > 0 {
			log.Printf("ocilog: dropped %d events (buffer full)", dropped)
		}
	})
}

func (e *Emitter) run() {
	defer e.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]entry, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		e.put(batch)
		batch = batch[:0]
	}

	for {
		select {
		case ent := <-e.ch:
			batch = append(batch, ent)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-e.done:
			// Drain what is already queued before going away, so the last few
			// lines of a build are not lost to a rolling update.
			for {
				select {
				case ent := <-e.ch:
					batch = append(batch, ent)
					if len(batch) >= batchSize {
						flush()
					}
					continue
				default:
				}
				break
			}
			flush()
			return
		}
	}
}

func (e *Emitter) put(batch []entry) {
	entries := make([]loggingingestion.LogEntry, 0, len(batch))
	for _, b := range batch {
		data, err := b.ev.marshal()
		if err != nil {
			continue
		}
		entries = append(entries, loggingingestion.LogEntry{
			Data: common.String(data),
			Id:   common.String(uuid.NewString()),
			Time: &common.SDKTime{Time: b.when},
		})
	}
	if len(entries) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e.client.PutLogs(ctx, loggingingestion.PutLogsRequest{
		LogId: common.String(e.logID),
		PutLogsDetails: loggingingestion.PutLogsDetails{
			Specversion: common.String("1.0"),
			LogEntryBatches: []loggingingestion.LogEntryBatch{{
				Entries:             entries,
				Source:              common.String("build-coordinator"),
				Type:                common.String("tinycloud.build"),
				Defaultlogentrytime: &common.SDKTime{Time: batch[0].when},
			}},
		},
	})
	if err != nil {
		// Logged and dropped on purpose. This is the secondary copy; the
		// primary record is already in SQLite, and retrying here would grow
		// the buffer during exactly the outage that caused the failure.
		log.Printf("ocilog: put %d entries failed: %v", len(entries), err)
	}
}
