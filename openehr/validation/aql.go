package validation

import (
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// ValidateAQL statically lints an AQL query (REQ-109) and bridges the result
// into the shared REQ-102 [Issue] / [Result] model. It parses q.Q against the
// SDK grammar profile, runs the lint layers, and — when c is non-nil — the
// template-aware archetype / path checks.
//
// This is the validation-package seam onto the openehr/aql/lint building
// block: lint owns its own issue model and never imports validation; this
// function converts lint.Issue → validation.Issue (severity and code carried
// across verbatim) so callers already using ValidateComposition get one
// uniform Result shape and the errors.Is dispatch via [Issue.Err].
//
// The CDR remains the execute-time path authority (PROBE-021): a clean result
// does not guarantee execution success, and the SDK grammar profile admits
// some non-conformant forms by design.
//
// A nil compiled template runs Layers 1–2 only — syntax, shape, parameter
// binding, AND the REQ-160/161 containment + portability semantic group; pass
// a [templatecompile.Compiled] to add the Layer-3 archetype-in-template and
// path checks. The semantic group is NOT gated on c: it judges FROM /
// CONTAINS against the pinned RM containment relation (openehr/aql/contain's
// default), so a nil template can still yield Error-severity issues —
// aql_impossible_containment, aql_contains_not_containable,
// aql_archetype_class_mismatch — that make [Result.OK] false.
//
// A deployment whose CDR admits a containment route the pinned RM does not —
// demographic containment being the named case (REQ-160 § Extensibility) —
// supplies its own relation through [ValidateAQLWithTypeRelation] rather than
// accepting the false Error that would otherwise fall out of the default.
func ValidateAQL(q aql.Query, c *templatecompile.Compiled) Result {
	return ValidateAQLWithTypeRelation(q, c, nil)
}

// ValidateAQLWithTypeRelation is [ValidateAQL] with the REQ-160 containment
// relation supplied by the caller (REQ-161 § Relation supply).
//
// rel answers which ordered pairs of RM TYPE NAMES an AQL CONTAINS may
// connect. A nil rel selects the REQ-160 default relation, so
// ValidateAQLWithTypeRelation(q, c, nil) is exactly [ValidateAQL] — nil does
// not switch the containment group off, and nothing switches it off.
//
// Note that nil and a zero relation are NOT the same thing: nil means the
// default, whereas a non-nil &contain.TypeRelation{} knows no classes at all
// and answers UnknownClass for every one, degrading the whole group to
// Warnings and leaving [Result.OK] true. Reach for [contain.Default] and
// extend it; do not construct a TypeRelation.
//
// Supply a relation extended with
// [contain.TypeRelation.WithOverlay] when the target CDR resolves a route the
// pinned RM does not describe, so a query that is correct in production is not
// reported as a static Error. The EHR Information Model versions no parties,
// for instance, so `EHR CONTAINS VERSIONED_PARTY` is Never by default and a
// deployment running demographics alongside the EHR names that edge itself:
//
//	rel := contain.Default().WithOverlay(contain.Edge{From: "EHR", To: "VERSIONED_PARTY"})
//	res := validation.ValidateAQLWithTypeRelation(q, nil, rel)
//
// A relation widens or narrows only the five containment codes — the scope is
// by code, not by layer, since those codes are themselves Layer 2. The three
// REQ-161 portability advisories consult no relation, and no other check reads
// it, so an overlay cannot retire a syntax, shape, parameter-binding, or
// template finding.
func ValidateAQLWithTypeRelation(q aql.Query, c *templatecompile.Compiled, rel *contain.TypeRelation) Result {
	res := lint.LintString(q.Q, &lint.Options{Compiled: c, Query: &q, Relation: rel})

	issues := make([]Issue, 0, len(res.Issues))
	for _, li := range res.Issues {
		issues = append(issues, Issue{
			Path:     li.Path,
			Code:     li.Code,
			Detail:   li.Detail,
			Severity: bridgeSeverity(li.Severity),
		})
	}
	return resultFromIssues(issues)
}

// bridgeSeverity maps a lint severity to the validation severity. The two
// enums are intentionally separate (lint is a building block); this is the
// single conversion point.
func bridgeSeverity(s lint.Severity) Severity {
	if s == lint.Warning {
		return Warning
	}
	return Error
}
