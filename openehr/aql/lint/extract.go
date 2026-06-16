// Package lint statically checks AQL (REQ-109) over the SDK grammar profile.
// It runs three layers — syntax (via [parse]), shape (AST-only), and path /
// template (when a compiled OPT is supplied) — and returns its own
// [Issue] / [Result] model. It is a building block (REQ-013): it imports
// neither transport/ nor auth/ nor any client, and it does not import
// openehr/validation (the dependency arrow is validation → lint).
//
// The CDR remains the execute-time semantic authority (PROBE-021): a
// lint-clean query MAY still be rejected on execution, and the SDK grammar
// profile deliberately admits some non-conformant forms (e.g. SELECT *), so
// "lint-clean" never means "spec-conformant".
package lint

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// Metadata is the set of facts extracted from a parsed query that the lint
// rules (and future tooling) reason over. It carries no diagnostics of its
// own; [Lint] turns these facts into [Issue]s.
type Metadata struct {
	// Archetypes are the distinct literal archetype HRIDs bound in the
	// FROM / CONTAINS tree, in document order. $param archetype predicates
	// are not listed here (see [parse.ClassExpr.ParamArchetype]).
	Archetypes []string
	// Aliases maps each bound alias to its class expression. Anonymous
	// class expressions (no alias) are absent.
	Aliases map[string]parse.ClassExpr
	// Paths are the identified paths across SELECT / WHERE / ORDER BY.
	Paths []parse.IdentifiedPath
	// Params are the distinct $parameter names referenced, in first-seen
	// order, with the leading `$` stripped.
	Params []string
}

// Extract gathers the lint facts from a parsed document. It performs no
// validation — every check lives in [Lint].
func Extract(doc *parse.Document) Metadata {
	md := Metadata{Aliases: make(map[string]parse.ClassExpr, len(doc.Classes))}
	seen := make(map[string]bool)
	for _, ce := range doc.Classes {
		if ce.Alias != "" {
			md.Aliases[ce.Alias] = ce
		}
		if ce.Archetype != "" && !seen[ce.Archetype] {
			seen[ce.Archetype] = true
			md.Archetypes = append(md.Archetypes, ce.Archetype)
		}
	}
	md.Paths = slices.Clone(doc.Paths)
	md.Params = slices.Clone(doc.Params)
	return md
}
