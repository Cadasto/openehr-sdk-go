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
// Scope limits (see deviations.md): templates whose sibling nodes sanitise
// to the same id return ErrIDCollision. As of REQ-116 Phase 3 aqlPath does
// carry the reference's name predicate ([archetype_id,'Name']), but id
// derivation still takes the archetype concept term rather than the pinned
// template-level name, so siblings reusing one archetype still collide
// (REQ-116 Phase 4). Such templates do compile: templatecompile admits
// shared-path subtrees.
package webtemplate
