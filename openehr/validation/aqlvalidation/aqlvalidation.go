// Package aqlvalidation statically lints AQL query text and reports the
// result in the [validation.Issue] / [validation.Result] model, so a caller
// already using [validation.ValidateComposition] gets one uniform result
// shape for compositions and queries.
//
// It is the bridge from openehr/aql/lint, which has its own issue model and
// never imports openehr/validation, to that shared model. It lives apart from
// openehr/validation so that a program that only validates instances does
// not link the AQL parser: importing openehr/validation pulls in no AQL
// package and no parser runtime.
//
// [Validate] and [ValidateWithTypeRelation] carry each lint issue across with
// its path, code, detail and severity. Every issue they return has
// [validation.LayerNone], an empty Rule and an empty Value, since a lint issue
// judges the query and not an instance, and [validation.Issue.Err] maps the
// codes aql_syntax and aql_empty to [validation.ErrAQLSyntax]. The Coverage
// of every result is the zero value, which is complete.
package aqlvalidation

import (
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Validate statically lints an AQL query and converts the result into the
// shared [validation.Issue] / [validation.Result] model. It parses q.Q
// against the SDK grammar profile, runs the lint layers, and, when c is
// non-nil, the template-aware archetype / path checks.
//
// It connects the validation model to openehr/aql/lint. The lint package has
// its own issue model and never imports validation; this function converts
// lint.Issue to validation.Issue (severity and code carried across verbatim)
// so callers already using [validation.ValidateComposition] get one uniform
// Result shape and errors.Is dispatch via [validation.Issue.Err].
//
// The CDR remains the authority on paths at execution time: a clean
// result does not guarantee execution success, and the SDK grammar
// profile admits some non-conformant forms by design.
//
// A nil compiled template runs lint layers 1 and 2 only: syntax, shape,
// parameter binding, and the containment and portability semantic group.
// Pass a [templatecompile.Compiled] to add lint layer 3, the
// archetype-in-template and path checks. These are the layers of
// openehr/aql/lint, not [validation.Issue.Layer], which is
// [validation.LayerNone] on every issue this function returns. The semantic
// group does not depend on c: it judges FROM / CONTAINS against the built-in
// RM containment relation (openehr/aql/contain's default), so a nil template
// can still yield Error-severity issues (aql_impossible_containment,
// aql_contains_not_containable, aql_archetype_class_mismatch) that make
// [validation.Result.OK] false.
//
// A deployment whose CDR admits a containment route the built-in RM
// relation does not (demographic containment is the common case)
// supplies its own relation through [ValidateWithTypeRelation]
// instead of accepting the false Error the default would produce.
func Validate(q aql.Query, c *templatecompile.Compiled) validation.Result {
	return ValidateWithTypeRelation(q, c, nil)
}

// ValidateWithTypeRelation is [Validate] with the RM containment relation
// supplied by the caller.
//
// rel answers which ordered pairs of RM type names an AQL CONTAINS may
// connect. A nil rel selects the default relation, so
// ValidateWithTypeRelation(q, c, nil) is exactly [Validate]. nil does not
// switch the containment group off, and nothing switches it off.
//
// nil and a zero relation are not the same thing: nil means the
// default, whereas a non-nil &contain.TypeRelation{} knows no classes at
// all and answers UnknownClass for every one, degrading the whole group
// to Warnings and leaving [validation.Result.OK] true. Start from
// [contain.Default] and extend it; do not construct a TypeRelation.
//
// Supply a relation extended with
// [contain.TypeRelation.WithOverlay] when the target CDR resolves a route
// the built-in RM relation does not describe, so a query that is correct
// in production is not reported as a static Error. The EHR Information
// Model versions no parties, for instance, so `EHR CONTAINS
// VERSIONED_PARTY` is Never by default and a deployment running
// demographics alongside the EHR names that edge itself:
//
//	rel := contain.Default().WithOverlay(contain.Edge{From: "EHR", To: "VERSIONED_PARTY"})
//	res := aqlvalidation.ValidateWithTypeRelation(q, nil, rel)
//
// A relation widens or narrows only the five containment codes. The
// scope is by code, not by lint layer, although the five codes all belong
// to lint layer 2. The three portability advisories consult no relation, and no
// other check reads it, so an overlay cannot retire a syntax, shape,
// parameter-binding, or template finding.
func ValidateWithTypeRelation(q aql.Query, c *templatecompile.Compiled, rel *contain.TypeRelation) validation.Result {
	res := lint.LintString(q.Q, &lint.Options{Compiled: c, Query: &q, Relation: rel})

	// The Result is built here rather than by the validation package, whose
	// constructor is unexported: OK is false exactly when an Error-severity
	// issue is present, Issues is never nil, and Coverage is the zero value.
	out := validation.Result{OK: true, Issues: make([]validation.Issue, 0, len(res.Issues))}
	for _, li := range res.Issues {
		sev := bridgeSeverity(li.Severity)
		if sev == validation.Error {
			out.OK = false
		}
		out.Issues = append(out.Issues, validation.Issue{
			Path:     li.Path,
			Code:     li.Code,
			Detail:   li.Detail,
			Severity: sev,
		})
	}
	return out
}

// bridgeSeverity maps a lint severity to the validation severity. The two
// enums are intentionally separate (lint is a building block); this is the
// single conversion point.
func bridgeSeverity(s lint.Severity) validation.Severity {
	if s == lint.Warning {
		return validation.Warning
	}
	return validation.Error
}
