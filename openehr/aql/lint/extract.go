// Package lint statically checks AQL (REQ-109) over the SDK grammar profile.
// It runs three layers — syntax (via [parse]), shape (AST-only), and path /
// template (when a compiled OPT is supplied) — and returns its own
// [Issue] / [Result] model. It is a building block (REQ-013): it imports
// neither transport/ nor auth/ nor any client, and it does not import
// openehr/validation (the dependency arrow is validation → lint).
//
// The CDR remains the execute-time semantic authority (PROBE-021): a
// lint-clean query MAY still be rejected on execution. The SDK grammar
// profile (ADR 0007) carries documented divergences from official QUERY
// 1.1.0 — SDK-AQL-001 spells the string function CONTAINS_STR and REJECTS
// the spec spelling CONTAINS(a,b) (shadowed by the containment operator);
// SDK-AQL-002 admits SELECT *; see resources/aql/grammar/DIVERGENCES.md —
// so "lint-clean" never means "spec-conformant" in either direction.
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
	// SelectAliases are the `AS` aliases declared in the SELECT list, in
	// document order (REQ-117). They are a separate namespace from
	// [Metadata.Aliases]: an ORDER BY key resolves against FROM/CONTAINS
	// first and only then against these, and a SELECT alias never binds a
	// class for the Layer-3 archetype / path checks.
	SelectAliases []string
	// Paths are the identified paths across SELECT / WHERE / ORDER BY.
	Paths []parse.IdentifiedPath
	// Params are the distinct $parameter names referenced, in first-seen
	// order, with the leading `$` stripped.
	Params []string
}

// Extract gathers the lint facts from a parsed document. It performs no
// validation — every check lives in [Lint]. A nil or unparsed document
// yields an empty Metadata rather than a panic (REQ-025).
func Extract(doc *parse.Document) Metadata {
	// REQ-025: parse.Parse yields a nil doc on a syntax error, so the
	// ordinary `doc, err := parse.Parse(q)` sequence reaches here with
	// nothing to read. Return an empty-but-usable Metadata rather than
	// panicking — the same shapes [Lint] refuses.
	if doc == nil || !doc.Parsed() {
		return Metadata{Aliases: map[string]parse.ClassExpr{}}
	}
	md := Metadata{Aliases: make(map[string]parse.ClassExpr, len(doc.Classes))}
	seen := make(map[string]bool)
	for _, ce := range doc.Classes {
		if ce.Alias != "" {
			md.Aliases[ce.Alias] = ce
		}
		if ce.Archetype != "" && !ce.ParamArchetype && !seen[ce.Archetype] {
			seen[ce.Archetype] = true
			md.Archetypes = append(md.Archetypes, ce.Archetype)
		}
	}
	md.SelectAliases = slices.Clone(doc.SelectAliases)
	md.Paths = slices.Clone(doc.Paths)
	md.Params = slices.Clone(doc.Params)
	return md
}
