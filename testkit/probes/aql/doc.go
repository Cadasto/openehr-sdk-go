// Package aqlprobes hosts the openEHR conformance probes for openehr/aql —
// the AQL builders (REQ-055). Probes are plain functions returning
// (Result, error); PROBE-020 is sandbox-only (no transport) since the builders
// are a building block per REQ-013.
//
// PROBE-020 asserts wire-output stability: the struct-builder and the
// verb-functions MUST emit byte-identical AQL for the same logical query, and
// that output MUST match the checked-in golden in
// openehr/aql/testdata/wire/. A downstream SDK implementing REQ-055 against the
// same reference query MUST reproduce the same canonical string.
package aqlprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary text for
// failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
