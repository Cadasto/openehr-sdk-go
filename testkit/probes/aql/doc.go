// Package aqlprobes hosts the openEHR conformance probes for openehr/aql:
// the AQL builders and the AQL lint pipeline. Probes are plain functions
// returning (Result, error); all are sandbox-only (no transport), because
// the builders and the lint pipeline are local building blocks.
//
// PROBE-020 asserts wire-output stability: the struct-builder and the
// verb-functions must emit byte-identical AQL for the same logical query, and
// that output must match the checked-in golden in
// openehr/aql/testdata/wire/. A downstream SDK implementing the same builder
// API against the same reference query must reproduce the same canonical
// string.
//
// PROBE-028 asserts lint stability: linting fixed query strings against the
// SDK grammar profile (and, for Layer 3, a compiled OPT) must produce a stable
// issue-code multiset. Any implementation of the lint pipeline with the same
// grammar profile + template must report the same codes. Cassettes live under
// testkit/cassettes/aql/lint/.
//
// PROBE-088 extends the PROBE-020 property to the later builder constructs
// (negated containment, sibling AND / OR junctions with precedence-driven
// parenthesisation, and opt-in in-text LIMIT / OFFSET), each against its own
// golden in openehr/aql/testdata/wire/. It also pins the PROBE-020 golden
// itself ([Probe020CanonicalQuery]): those constructs were a semver-minor
// addition, so a builder program using none of them must still emit the same
// bytes as before. Requesting both paging channels is a build-time error, not
// a silently combined emission.
package aqlprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
