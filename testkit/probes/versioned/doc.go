// Package versionedprobes hosts the openEHR conformance probes for
// the openEHR REST versioned-write contract. Each probe implements one
// numbered conformance probe (PROBE-NNN) and behaves identically in any
// openEHR-conformant implementation.
//
// Probes are plain functions returning (Result, error) so they can
// run inside `go test`, inside the conformance harness, or from
// third-party integrations. They take a [transport.Client] backed by
// any compliant openEHR REST deployment. Today that is the sandbox;
// Cassette and Live modes are not yet wired up for this package.
package versionedprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
