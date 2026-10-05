// Package examples holds the runnable example programs that show how to use
// the SDK, one directory per program. They are short reference shapes to copy
// from, not production tools. Every one of them runs offline: the REST ones
// talk to a fake server in the same process, so none needs a clinical data
// repository. Some fakes are httptest servers on a loopback address; the
// others are built on the sandbox package and open no listener at all.
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
//   - validate-composition: validate an in-memory composition in two passes,
//     the RM floor and an OPT's constraints
//   - validate-from-json: decode canonical JSON, then validate it in two
//     passes, the RM floor and an OPT's constraints
//   - generate-example: generate an RM instance from an OPT and print it as JSON
//   - aql-build: build AQL with the struct and verb builders, nested CONTAINS
//     with in-text paging, and the opt-in RM containment check
//   - aql-parse-structured: parse AQL into the structured tree and emit it back
//   - lint-aql: lint AQL against the grammar, the RM and a compiled template
//   - compile-build-validate: compile an OPT, build a composition, round-trip
//     it through canonical JSON and validate it against the RM floor and the
//     template, through public packages only
//   - template-explore: walk a compiled OPT: structure tree and leaf paths
//   - webtemplate-export: export a compiled OPT as Web Template JSON
//   - flat-roundtrip: a COMPOSITION to and from the FLAT and STRUCTURED
//     simplified formats, then the template-aware decode validated against
//     the RM floor and the template
//   - ehr_create: create an EHR through the REST client path, against an
//     in-process server
//   - contribution-build: assemble a multi-version CONTRIBUTION and, with
//     -commit, POST it to an in-process fake CDR
//   - definition-lifecycle: upload, list and download a template through the
//     Definition REST client, compile it, and ask for an example composition
//   - composition-crud: save, read and update a COMPOSITION through the REST
//     client, and handle the 412 a stale If-Match meets
//   - query-execute: execute AQL with bound parameters, decode a RESULT_SET
//     cell into a typed value, and classify a refused query
//   - smart-launch: a standalone SMART-on-openEHR PKCE launch, with the
//     AuthorizationRequest stored across the redirect
//   - service-auth: authenticate a backend service with the OAuth 2.0 client
//     credentials grant, reusing the cached token
package examples
