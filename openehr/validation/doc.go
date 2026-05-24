// Package validation checks in-memory openEHR Reference Model
// artefacts against a compiled OPT and reports every issue in one
// pass — REQ-102.
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
// # Trust model
//
// Validation is **template-driven**: the compiled OPT is the
// authoritative driver and the composition is the value source.
// For each compiled OPT node the walker reads the corresponding
// RM property via [github.com/cadasto/openehr-sdk-go/openehr/validation/rmread],
// enforces existence / cardinality / alternatives / RM-type match
// / archetype-id identity, and recurses into matched RM children.
// Path strings in [Issue.Path] come from
// [internal/templatecompile.CompiledNode.AQLPath] — composition-
// supplied predicates never form lookup keys, so a composition
// missing an OPT-required node is flagged rather than silently
// bypassed. See
// [docs/plans/2026-05-24-validation-v2-template-driven.md] for
// the migration's phase split.
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
// `serialize/` import is enforced by `TestValidationNoSerializeImport`.
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
