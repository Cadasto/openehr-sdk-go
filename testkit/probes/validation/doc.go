// Package validationprobes hosts the openEHR conformance probes
// for openehr/validation/ — template-driven RM validation against a
// compiled OPT (REQ-102, REQ-110). Probes are plain functions returning
// (Result, error); they are sandbox-only (no transport) since the
// validator is a building-block per REQ-013.
//
// PROBE-025 covers the canonical fixture path: a complete
// composition validates clean, while structural / primitive
// mutations produce a stable multiset of issue codes that
// downstream SDKs MUST reproduce. PROBE-026 sharpens the negative
// surface with missing-node and cardinality cases.
//
// PROBE-074 extends the conformance surface beyond COMPOSITION
// (REQ-110): the same compiled-OPT walker validates the demographic
// PARTY hierarchy (PERSON / ORGANISATION / GROUP / AGENT / ROLE +
// sub-components) and the EHR-IM roots FOLDER / EHR_STATUS, asserting
// the issue-code multiset per (OPT, root) shape via [validation.Validate].
package validationprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
