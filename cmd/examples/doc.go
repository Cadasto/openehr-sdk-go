// Package examples holds the runnable example programs that show how to use
// the SDK, one directory per program. They are short reference shapes to copy
// from, not production tools. Every one of them runs offline: the REST ones
// talk to an in-process test server, so none needs a clinical data repository.
//
// The catalogue in docs/examples.md describes each program (what it shows,
// how to run it, what the output means, and what to copy into your own
// application); docs/quick-start.md walks through the first few.
//
// Run any of them from the repository root with `go run ./cmd/examples/<name>`:
//
//   - canonical_json: decode a canonical-JSON COMPOSITION into typed RM structs
//   - canxml_roundtrip: one COMPOSITION through canonical JSON and XML and back
//   - opt-parse: parse an operational template (OPT) and resolve paths in it
//   - primitive-validate: check single values against one OPT leaf constraint
//   - validate-composition: validate an in-memory composition against an OPT
//   - validate-from-json: decode canonical JSON, then validate it against an OPT
//   - generate-example: generate an RM instance from an OPT and print it as JSON
//   - aql-build: build AQL with the struct and verb builders, nested CONTAINS
//     with in-text paging, and the opt-in RM containment check
//   - aql-parse-structured: parse AQL into the structured tree and emit it back
//   - lint-aql: lint AQL against the grammar, the RM and a compiled template
//   - compile-build-validate: compile an OPT, build a composition, round-trip
//     it through canonical JSON and validate it, through public packages only
//   - template-explore: walk a compiled OPT: structure tree and leaf paths
//   - webtemplate-export: export a compiled OPT as Web Template JSON
//   - flat-roundtrip: a COMPOSITION to and from the FLAT and STRUCTURED
//     simplified formats
//   - ehr_create: create an EHR through the REST client path, against an
//     in-process server
//   - contribution-build: assemble a multi-version CONTRIBUTION and, with
//     -commit, POST it to an in-process fake CDR
//   - smart-launch: a standalone SMART-on-openEHR PKCE launch, with the
//     AuthorizationRequest stored across the redirect
package examples
