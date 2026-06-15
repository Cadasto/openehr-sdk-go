// Package instanceprobes hosts the openEHR conformance probes for
// openehr/instance/ — template-driven RM instance synthesis
// (REQ-107). Probes are plain functions returning (Result, error);
// they are sandbox-only (no transport) since the generator is a
// building block per REQ-013.
//
// PROBE-027 closes the loop: Generate followed by
// ValidateComposition on the same compiled OPT MUST surface
// Result.OK = true for both Minimal and Example policies. Downstream
// SDKs implementing REQ-107 + REQ-102 v2 against the same fixtures
// MUST reproduce the same OK outcome.
package instanceprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
