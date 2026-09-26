// Package examples hosts worked, runnable example programs for each
// named SDK use case (benchmark, seeder, MCP server, federator).
//
// Production tools (load harnesses, federators, etc.) live in their
// own repositories; the examples here are reference shapes only.
//
// The catalog in docs/examples.md and the walkthrough in
// docs/quick-start.md describe each program in more detail.
//
// Runnable programs (from repo root):
//
//   - cmd/examples/canonical_json: canjson decode
//   - cmd/examples/canxml_roundtrip: canjson to canxml and back
//   - cmd/examples/opt-parse: OPT parse and paths
//   - cmd/examples/validate-composition: validate a composition against an OPT
//   - cmd/examples/validate-from-json: canjson decode, then OPT validation
//   - cmd/examples/primitive-validate: primitive constraints
//   - cmd/examples/ehr_create: REST EHR create (transport and client)
//   - cmd/examples/contribution-build: fluent multi-version Contribution_create assembly
//   - cmd/examples/generate-example: synthesise an RM instance from an OPT
//   - cmd/examples/aql-build: AQL struct and verb builders, containment algebra and in-text paging
//   - cmd/examples/aql-parse-structured: AQL to a parse.Query AST and back through Emit
//   - cmd/examples/lint-aql: AQL static lint and ValidateAQL
//   - cmd/examples/compile-build-validate: compile, build and validate using public imports only
//   - cmd/examples/template-explore: introspect a compiled OPT (structure and leaf paths)
//   - cmd/examples/webtemplate-export: compiled OPT to EHRbase v2.3 WebTemplate JSON
//   - cmd/examples/flat-roundtrip: COMPOSITION to and from the FLAT and STRUCTURED simplified formats
//   - cmd/examples/smart-launch: standalone SMART PKCE launch with state and verifier persistence
package examples
