// Package validation checks in-memory openEHR Reference Model
// artefacts against a compiled OPT and reports every issue in one
// pass — REQ-102 (COMPOSITION) and REQ-110 (any archetypeable root).
//
// Public entry:
//
//	r := validation.ValidateComposition(comp, compiled)
//	if !r.OK {
//	    for _, issue := range r.Issues {
//	        log.Printf("%s: %s — %s", issue.Path, issue.Code, issue.Detail)
//	    }
//	}
//
// The walker is value-source-generic: [Validate] runs it over any RM
// root the closed RM set recognises, and the typed wrappers
// [ValidateComposition], [ValidateDemographic] (PERSON / ORGANISATION /
// GROUP / AGENT / ROLE), [ValidateFolder], and [ValidateEHRStatus]
// delegate to it (REQ-110). PARTY sub-components (ADDRESS, CONTACT,
// PARTY_IDENTITY, PARTY_RELATIONSHIP, CAPABILITY) validate in place
// during a PARTY walk or as roots via [Validate].
//
// # Trust model
//
// Validation is **template-driven**: the compiled OPT is the
// authoritative driver and the composition is the value source.
// For each compiled OPT node the walker reads the corresponding
// RM property via [github.com/cadasto/openehr-sdk-go/openehr/validation/rmread],
// enforces existence / cardinality / alternatives / RM-type match
// / archetype-id identity, and recurses into matched RM children.
//
// Path strings in [Issue.Path] are built by appending OPT-side
// attribute names and matched-child predicates to the parent OPT
// node's path as the walker descends. The OPT contributes the
// attribute names; the RM-side archetype_node_id of each matched
// child contributes the bracket predicate. Composition-supplied
// predicates therefore appear in the path only on RM nodes the
// walker has bound to an OPT child — a composition missing an
// OPT-required node is flagged at the parent attribute's path
// (no descent), rather than silently bypassed.
//
// See [docs/plans/2026-05-24-validation-v2-template-driven.md]
// for the migration's phase split.
//
// # Collect-all
//
// Every failing clause emits one [Issue]; the walk never
// short-circuits. UIs and CI runners need the full list.
//
// # REQ-013 building-block independence
//
// The validator MUST be importable without `transport/`, `auth/`,
// `openehr/client/*`, or `openehr/serialize/`. The forbidden
// forbidden-import set is enforced by `TestValidationForbiddenImports`.
//
// # v1: module-local callability
//
// The `c` argument is typed against the SDK's internal compiled-
// template package (internal/templatecompile.Compiled). Per Go's
// internal/ visibility rule, external modules cannot construct
// the argument and therefore cannot call [ValidateComposition]
// in v1. The validator is callable from any package within
// github.com/cadasto/openehr-sdk-go (composition builder, codegen,
// MCP servers, CI tools vendoring the SDK). External callability
// lands alongside REQ-101 when template.Compile is re-exported.
// See docs/adr/0005-compiled-template-foundation.md §C2.
package validation
