// Package versionedprobes hosts the openEHR conformance probes for
// the openEHR REST versioned-write contract (REQ-054). Each probe
// corresponds to a PROBE-NNN entry in docs/specifications/conformance.md and is
// implemented identically in any openEHR-conformant implementation (REQ-080).
//
// Probes are plain functions returning (Result, error) so they can
// run inside `go test`, inside the conformance harness, or from
// third-party integrations. They take a [transport.Client] backed by
// any compliant openEHR REST deployment — the sandbox today, with
// Cassette and Live the REQ-082 modes still to be wired up for this
// package.
package versionedprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome (REQ-082).
type Result = probe.Result
