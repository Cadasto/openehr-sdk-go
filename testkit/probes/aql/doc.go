// Package aqlprobes hosts the openEHR conformance probes for openehr/aql —
// the AQL builders (REQ-055) and the AQL lint pipeline (REQ-109). Probes are
// plain functions returning (Result, error); both are sandbox-only (no
// transport) since the builders and the lint pipeline are building blocks per
// REQ-013.
//
// PROBE-020 asserts wire-output stability: the struct-builder and the
// verb-functions MUST emit byte-identical AQL for the same logical query, and
// that output MUST match the checked-in golden in
// openehr/aql/testdata/wire/. A downstream SDK implementing REQ-055 against the
// same reference query MUST reproduce the same canonical string.
//
// PROBE-028 asserts lint stability: linting fixed query strings against the
// SDK grammar profile (and, for Layer 3, a compiled OPT) MUST produce a stable
// issue-code multiset. Any implementation of REQ-109 with the same grammar
// profile + template MUST report the same codes. Cassettes live under
// testkit/cassettes/aql/lint/.
//
// PROBE-088 extends the PROBE-020 property to the builder constructs REQ-117
// adds — negated containment, sibling AND / OR junctions with
// precedence-driven parenthesisation, and opt-in in-text LIMIT / OFFSET — each
// against its own golden in openehr/aql/testdata/wire/. It also pins the
// PROBE-020 golden itself ([Probe020CanonicalQuery]): REQ-117 is semver-minor,
// so a builder program using none of the new API MUST still emit the
// pre-REQ-117 bytes. Requesting both paging channels MUST be a build-time
// error rather than a silently combined emission.
package aqlprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary text for
// failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
