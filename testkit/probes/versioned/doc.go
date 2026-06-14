// Package versionedprobes hosts the openEHR conformance probes for
// the openEHR REST versioned-write contract (REQ-054). Each probe
// corresponds to a PROBE-NNN entry in docs/specifications/conformance.md and is
// implemented identically in any openEHR-conformant implementation (REQ-080).
//
// Probes are plain functions returning (Result, error) so they can
// run inside `go test`, inside the conformance harness, or from
// third-party integrations. They take a [transport.Client] backed by
// any compliant openEHR REST deployment (sandbox, cassette, live).
package versionedprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
