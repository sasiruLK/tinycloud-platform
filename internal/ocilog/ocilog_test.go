package ocilog

import (
	"encoding/json"
	"testing"
	"time"
)

// A coordinator running without OCI_LOG_ID holds a nil *Emitter and calls these
// on every build. They must be no-ops rather than panics.
func TestNilEmitterIsSafe(t *testing.T) {
	var e *Emitter
	e.Emit(Event{Type: "build.log", Message: "hello"})
	e.Close()
	e.Close() // idempotent
}

func TestNewWithoutLogIDIsDisabled(t *testing.T) {
	e, err := New("")
	if err != nil {
		t.Fatalf("empty log id should not error, got %v", err)
	}
	if e != nil {
		t.Fatalf("empty log id should disable shipping, got %#v", e)
	}
}

// Emit sits on the path that serves runner log callbacks. If it ever blocks,
// a slow Logging endpoint stalls builds — so a full buffer must drop.
func TestEmitDoesNotBlockWhenBufferIsFull(t *testing.T) {
	e := &Emitter{
		ch:   make(chan entry, 2),
		done: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < bufferSize+100; i++ {
			e.Emit(Event{Type: "build.log", Message: "line"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked when the buffer was full")
	}

	e.mu.Lock()
	dropped := e.dropped
	e.mu.Unlock()
	if dropped == 0 {
		t.Fatal("expected overflow to be counted as dropped")
	}
}

func TestEventMarshalOmitsEmptyFields(t *testing.T) {
	got, err := Event{Type: "build.log", JobID: "j1", Stream: "stdout", Message: "compiling"}.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, k := range []string{"type", "jobId", "stream", "message"} {
		if _, ok := m[k]; !ok {
			t.Errorf("expected key %q in %s", k, got)
		}
	}
	// A log line carries no build status; those keys should not be present at
	// all rather than present and empty.
	for _, k := range []string{"status", "image", "tag", "error", "app"} {
		if _, ok := m[k]; ok {
			t.Errorf("unexpected empty key %q in %s", k, got)
		}
	}
}
