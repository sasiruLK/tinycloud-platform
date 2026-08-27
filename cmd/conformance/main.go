// Command conformance runs the published TinyCloud Provider contract against a
// Provider URL and reports pass, fail or not-implemented per Capability.
//
// It is what a Provider author runs to prove their service works before asking
// anyone to review it, and what CI runs against the in-tree Providers so that a
// change to the contract which breaks an implementation fails the build.
//
//	conformance -url http://localhost:9090 -token "$PROVIDER_TOKEN"
//
// The exit status is non-zero only on a genuine contract violation: a Provider
// that implements one Capability and reports the rest as unimplemented is
// conformant, and says so.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/conformance"
)

func main() {
	url := flag.String("url", "", "base URL of the provider to check, e.g. http://localhost:9090")
	token := flag.String("token", os.Getenv("PROVIDER_TOKEN"), "bearer token for the provider (default $PROVIDER_TOKEN)")
	timeout := flag.Duration("timeout", 15*time.Second, "per-request timeout")
	asJSON := flag.Bool("json", false, "print the report as JSON")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "usage: conformance -url <provider url> [-token <token>] [-json]")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	report, err := conformance.New(*url, *token, *timeout).Run(ctx)
	if err != nil {
		// The Provider could not be asked what it serves. That is a contract
		// violation in itself: discovery is how everything else is decided.
		fmt.Fprintf(os.Stderr, "not conformant: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write report: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(report)
	}

	if !report.Passed() {
		os.Exit(1)
	}
}
