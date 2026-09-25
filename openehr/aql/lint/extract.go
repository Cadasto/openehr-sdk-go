// Package lint statically checks AQL over the SDK grammar profile.
// It runs three layers, syntax (via [parse]), shape (AST-only), and path /
// template (when a compiled OPT is supplied), and returns its own
// [Issue] / [Result] model. It imports neither transport/ nor auth/ nor any
// client, and it does not import openehr/validation (the dependency arrow is
// validation → lint).
//
// Layer 2 also carries the semantic containment checks and portability
// advisories. The five containment codes (aql_unknown_rm_class,
// aql_contains_not_containable, aql_impossible_containment,
// aql_containment_by_reference, and aql_archetype_class_mismatch) are judged
// against the containment relation ([contain.TypeRelation]) derived
// in-process from the pinned BMM. They flag FROM/CONTAINS shapes that parse
// cleanly and that a CDR will typically accept and answer with zero rows;
// `OBSERVATION CONTAINS COMPOSITION` is the typical example. The remaining
// three (aql_version_no_predicate, aql_versioned_object_unreferenced, and
// aql_fanout_row_grain) are portability advisories for behaviours the QUERY
// specification leaves open; they consult no relation. The whole group
// always runs. Supply [Options.Relation] to lint the containment codes, and
// aql_contains_redundant_step below (the one other code that asks a
// containment-route question), against a relation carrying dialect overlay
// edges. Layer 2 stays AST-only either way: no CDR, no OPT, and no row
// semantics.
//
// Layer 2 carries a third group, path shape:
// aql_path_repeating_unpredicated, aql_paging_no_order_by,
// aql_select_no_alias, aql_fanout_path_grain and
// aql_contains_redundant_step. They flag query shapes whose outcome the
// engine decides instead of the query (which occurrence, which page boundary,
// which column name, how many rows), plus the one containment step that
// provably decides nothing. The group always runs and every code in it is a
// Warning. Two are fed by a single walk of each path's segments against the
// same pinned BMM, which stops silently wherever the BMM cannot type a step;
// two consult no RM fact at all; and aql_contains_redundant_step is the
// group's one consumer of [Options.Relation], since a route round the step is
// exactly what a dialect overlay edge can state.
//
// The CDR remains the execute-time semantic authority: a lint-clean query may
// still be rejected on execution. The SDK grammar profile carries documented
// divergences from the official openEHR QUERY 1.1.0 grammar: SDK-AQL-001
// spells the string function CONTAINS_STR and rejects the spec spelling
// CONTAINS(a,b) (shadowed by the containment operator), and SDK-AQL-002
// admits SELECT * (see resources/aql/grammar/DIVERGENCES.md). "Lint-clean"
// therefore never means "spec-conformant" in either direction.
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
	// document order. They are a separate namespace from [Metadata.Aliases]: an
	// ORDER BY key resolves against FROM/CONTAINS first and only then against
	// these, and a SELECT alias never binds a class for the Layer-3 archetype /
	// path checks.
	SelectAliases []string
	// Paths are the identified paths across SELECT / WHERE / ORDER BY.
	Paths []parse.IdentifiedPath
	// Params are the distinct $parameter names referenced, in first-seen
	// order, with the leading `$` stripped.
	Params []string
}

// Extract gathers the lint facts from a parsed document. It performs no
// validation; every check lives in [Lint]. A nil or unparsed document yields
// an empty Metadata instead of a panic.
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
