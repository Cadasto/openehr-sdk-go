// Package examples hosts worked, runnable example programs for each
// named SDK use case (benchmark, seeder, MCP server, federator).
//
// Production consumers (the openehr-cdr benchmark, the
// soon-to-exist federator, etc.) live in their own repos — examples
// here are reference shapes only.
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
package examples
