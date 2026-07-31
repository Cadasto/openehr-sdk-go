// Package webtemplate exports a compiled openEHR operational template as
// EHRbase openEHR_SDK v2.3 "WebTemplate" JSON — a lossy, UI-oriented
// projection consumed by form renderers and data-entry clients.
//
// The shape and the consumer-critical "id" generation mirror EHRbase
// v2.3 (REQ-106, ADR-0014). Parity with the reference is structural, not
// byte-exact; accepted differences are listed in deviations.md beside the
// package tests.
//
// The package is a building block (REQ-013): it takes a
// *templatecompile.Compiled in and returns bytes out, importing only
// openehr/templatecompile, openehr/template/constraints, and the standard
// library — never the transport, auth, client, or serialize layers.
//
// Sibling disambiguation (REQ-116): a node's id comes from the
// template-level name the OPT pins on it, falling back to the archetype
// concept term, and aqlPath carries the matching name predicate
// ([archetype_id,'Name']). Siblings that reuse one archetype are therefore
// distinct by construction. Where nothing separates them — no pinned name
// and one shared term — the second and later claimants take an ordinal
// (dv_text, dv_text2), matching the reference; ErrIDCollision now signals a
// builder bug rather than an unsupported template.
package webtemplate
