// Package provider is TinyCloud's side of the `/v0` Provider contract: the
// vocabulary of Provider kinds and Capabilities, the HTTP client Core reads
// Providers through, the server that hosts the in-tree Providers, and the
// configuration that names which Providers an Instance has.
//
// The point of the split is that Core has exactly one code path for reading
// infrastructure. An in-tree Provider is not linked into Core; it is served
// over the same contract a third party would implement, so the maintainers'
// own Providers exercise the published contract on every request and cannot
// silently drift from it.
package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
)

// ContractVersion is the path prefix every Capability is served under. It
// moves only when the contract makes a promise it did not make before — per
// ADR-0001, `/v0` promises nothing until a second author has shipped a
// Provider against it.
const ContractVersion = "v0"

// KindInfra is the Provider kind this package implements. Registry and Build
// are named in the glossary and are not implemented yet.
const KindInfra = "infra"

// The Capabilities of the Infra kind. One per endpoint of the contract; a
// Provider declares the subset it serves and returns 501 for the rest.
const (
	CapabilityInstances = "instances"
	CapabilityMetrics   = "metrics"
	CapabilityAlarms    = "alarms"
	CapabilityIngress   = "ingress"
	CapabilityBackups   = "backups"
)

// InfraCapabilities lists every Capability of the Infra kind, in the order the
// Conformance suite reports them.
var InfraCapabilities = []string{
	CapabilityInstances,
	CapabilityMetrics,
	CapabilityAlarms,
	CapabilityIngress,
	CapabilityBackups,
}

// Infra is the interface an in-tree Provider implements. It is deliberately
// not an extension point: a Provider written by anyone else implements the
// HTTP contract instead, and this exists only so that the Providers shipped in
// this repository have something to write Go against.
//
// A method for a Capability the Provider does not declare is never called by
// Server, so an implementation may return ErrNotImplemented from it and stop
// there.
type Infra interface {
	// Name identifies this implementation to humans and logs.
	Name() string
	// Capabilities are the Capabilities this Provider serves. Anything absent
	// is answered with 501.
	Capabilities() []string

	Instances(ctx context.Context) ([]infra.InstanceInfo, error)
	Metric(ctx context.Context, metric string, window time.Duration) ([]infra.Series, error)
	Alarms(ctx context.Context) ([]infra.AlarmStatus, error)
	IngressAddress(ctx context.Context) (string, error)
	BackupObjects(ctx context.Context) (infra.BackupListing, error)
}

// ErrNotImplemented is what a Capability that this Provider does not serve
// returns, on either side of the wire: an in-tree Provider returns it from a
// method it has no data for, and the client returns it when a Provider answers
// 501 or leaves the Capability out of its discovery response. Core turns it
// into a warning naming the Capability, which is how "not supported" stays
// distinguishable from "broken".
//
// Match it with errors.Is. The errors carrying it say which Capability and
// which Provider in their own words, so that the warning an operator reads is
// one sentence rather than a chain of them.
var ErrNotImplemented = errors.New("capability not implemented")

// notImplementedError is an unimplemented Capability, phrased for the person
// reading the dashboard's warnings.
type notImplementedError struct{ message string }

func (e *notImplementedError) Error() string { return e.message }

// Is makes errors.Is(err, ErrNotImplemented) true without the sentinel's
// wording being appended to the message.
func (e *notImplementedError) Is(target error) bool { return target == ErrNotImplemented }

// notImplementedf builds an unimplemented-Capability error.
func notImplementedf(format string, args ...any) error {
	return &notImplementedError{message: fmt.Sprintf(format, args...)}
}

// UpstreamError is a Provider reporting that it is up but its Substrate could
// not be read. It travels as 502, and reaches the snapshot as a warning with
// the Substrate's own complaint attached.
type UpstreamError struct{ Err error }

func (e *UpstreamError) Error() string { return e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

// Upstream wraps err as a Substrate failure.
func Upstream(err error) error { return &UpstreamError{Err: err} }

// ValidMetric reports whether name is a metric the contract defines. A
// Provider rejects anything else with 400 rather than guessing.
func ValidMetric(name string) bool {
	for _, m := range infra.Metrics {
		if m == name {
			return true
		}
	}
	return false
}

// ValidCapability reports whether name is a Capability of the Infra kind.
func ValidCapability(name string) bool {
	for _, c := range InfraCapabilities {
		if c == name {
			return true
		}
	}
	return false
}

// capabilitySet indexes a declared Capability list for lookup.
func capabilitySet(caps []string) map[string]bool {
	set := make(map[string]bool, len(caps))
	for _, c := range caps {
		set[c] = true
	}
	return set
}

// notImplemented returns ErrNotImplemented naming the Capability, so a warning
// says which one is missing rather than only that something was.
func notImplemented(capability, providerName string) error {
	if providerName == "" {
		return notImplementedf("capability %q is served by no configured provider", capability)
	}
	return notImplementedf("provider %q does not implement the %q capability", providerName, capability)
}
