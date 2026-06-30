// Package examples hosts worked, runnable example programs for each
// named SDK use case (benchmark, seeder, MCP server, federator).
//
// Production consumers (the reference CDR load harness, the
// soon-to-exist federator, etc.) live in their own repos — examples
// here are reference shapes only.
//
// Developer docs: docs/examples.md (catalog) and docs/quick-start.md
// (onboarding). When adding or materially changing an example, update
// those files and this bullet list in the same PR — see AGENTS.md
// § Runnable examples and docs/ai-workflow.md § Developer examples.
//
// Runnable programs (from repo root):
//
//   - cmd/examples/canonical_json       — canjson decode (REQ-052)
//   - cmd/examples/canxml_roundtrip     — canjson ↔ canxml (REQ-056)
//   - cmd/examples/opt-parse            — OPT parse + paths (REQ-100)
//   - cmd/examples/validate-composition — OPT vs composition (REQ-102)
//   - cmd/examples/validate-from-json     — canjson + OPT validate (REQ-052, REQ-102)
//   - cmd/examples/primitive-validate     — primitive constraints (REQ-103)
//   - cmd/examples/ehr_create           — REST EHR create (transport + client)
//   - cmd/examples/generate-example     — OPT → RM instance synthesis (REQ-107)
//   - cmd/examples/aql-build            — AQL struct + verb builders (REQ-055)
//   - cmd/examples/aql-parse-structured — AQL → parse.Query AST + Emit round-trip (REQ-113)
//   - cmd/examples/lint-aql             — AQL static lint + ValidateAQL (REQ-109)
//   - cmd/examples/compile-build-validate — public compile→build→validate, public-only imports (REQ-111)
//   - cmd/examples/template-explore     — introspect a compiled OPT: structure + leaf paths (REQ-111)
//   - cmd/examples/smart-launch         — standalone SMART PKCE launch, state+verifier persistence (REQ-061)
package examples
