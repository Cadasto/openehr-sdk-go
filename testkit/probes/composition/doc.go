// Package compositionprobes hosts the openEHR conformance probes
// for openehr/composition/ — the OPT-driven composition builder
// (REQ-101). Probes are plain functions returning (Result, error);
// they are sandbox-only (no transport) since the builder is a
// building block per REQ-013.
//
// PROBE-023 closes the authoring round-trip: NewBuilder + Set →
// Build → canjson.Marshal → containment checks on key paths.
// Downstream SDKs implementing REQ-101 against the same fixtures
// MUST reproduce the same pass outcome.
package compositionprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
