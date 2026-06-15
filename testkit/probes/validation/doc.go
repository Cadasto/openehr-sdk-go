// Package validationprobes hosts the openEHR conformance probes
// for openehr/validation/ — composition validation against a
// compiled OPT (REQ-102). Probes are plain functions returning
// (Result, error); they are sandbox-only (no transport) since the
// validator is a building-block per REQ-013.
//
// PROBE-025 covers the canonical fixture path: a complete
// composition validates clean, while structural / primitive
// mutations produce a stable multiset of issue codes that
// downstream SDKs MUST reproduce. PROBE-026 sharpens the negative
// surface with missing-node and cardinality cases.
package validationprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
