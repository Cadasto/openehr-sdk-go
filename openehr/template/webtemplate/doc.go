// Package webtemplate exports a compiled openEHR operational template as
// EHRbase openEHR_SDK v2.3 "WebTemplate" JSON, a lossy, UI-oriented
// projection consumed by form renderers and data-entry clients.
//
// The shape and the "id" generation that consumers depend on mirror
// EHRbase v2.3. Parity with the reference is structural, not
// byte-exact; accepted differences are listed in deviations.md beside the
// package tests.
//
// The package takes a *templatecompile.Compiled in and returns bytes out.
// It imports only openehr/templatecompile, openehr/template/constraints,
// internal/templatecompile (the shared name-predicate quoting, so the two
// path builders cannot drift), and the standard library. It never imports
// the transport, auth, client, or serialize layers.
//
// Sibling disambiguation: a node's id comes from the template-level name
// the OPT pins on it, falling back to the archetype concept term, and
// aqlPath carries the matching name predicate ([archetype_id,'Name']).
// Siblings that reuse one archetype are therefore distinct by
// construction. Where nothing separates them (no pinned name and one
// shared term), the second and later claimants take the next free
// ordinal (dv_text, dv_text2), matching the reference. [ErrIDCollision]
// therefore signals a builder bug rather than an unsupported template.
package webtemplate
