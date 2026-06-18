package validation

import (
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
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
// some non-conformant forms by design. A nil compiled template runs Layers
// 1–2 only (syntax, shape, parameter binding); pass a [templatecompile.Compiled]
// to enable the archetype-in-template and path checks.
func ValidateAQL(q aql.Query, c *templatecompile.Compiled) Result {
	res := lint.LintString(q.Q, &lint.Options{Compiled: c, Query: &q})

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
