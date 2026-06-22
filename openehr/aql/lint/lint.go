package lint

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Severity classifies a lint [Issue]. Error means the query is (statically)
// wrong; Warning is advisory — the query may still execute, and the SDK
// grammar profile / CDR may admit it.
type Severity int

const (
	// Error means a static defect a strict consumer SHOULD reject.
	Error Severity = iota
	// Warning is advisory; it does not make a [Result] not-OK.
	Warning
)

// String renders "error" / "warning"; out-of-range values render numerically.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return fmt.Sprintf("severity(%d)", int(s))
}

// Issue is one finding from a lint pass. Lint is collect-all (every issue,
// not fail-fast). The zero value is not meaningful.
type Issue struct {
	// Code is a stable programmatic identifier (e.g. "aql_syntax",
	// "aql_archetype_not_in_template"). Consumers SHOULD dispatch on Code.
	Code string
	// Path is the AQL path or class the issue concerns; "" when not
	// localised.
	Path string
	// Detail is a human-readable message (carries ANTLR line/col for
	// syntax errors).
	Detail string
	// Severity classifies the issue.
	Severity Severity
}

// Result aggregates every [Issue] from one [Lint] / [LintString] call.
type Result struct {
	// Issues is the full list of findings in a stable, deterministic
	// order: by layer, then document order within a layer (aql_unused_param
	// is sorted by parameter key, since unreferenced params have no
	// document position). Never nil after a lint call (zero-length when
	// clean).
	Issues []Issue
}

// OK reports whether the result carries no Error-severity issue. Warnings do
// not make a result not-OK.
func (r Result) OK() bool {
	for _, i := range r.Issues {
		if i.Severity == Error {
			return false
		}
	}
	return true
}

// Options tunes a lint pass. The zero value (or nil) runs the AST-shape
// checks only: Layer 3 needs Compiled, and the Layer-2 parameter-binding
// checks need Query.
type Options struct {
	// Compiled, when non-nil, enables Layer 3 (archetype / path checks
	// against a compiled OPT).
	Compiled *templatecompile.Compiled
	// Query, when non-nil, enables parameter-binding checks
	// (aql_unbound_param / aql_unused_param) against its Parameters map.
	Query *aql.Query
}

// LintString parses q against the SDK grammar profile and lints the result.
// It is the entry point for raw AQL (the [validation.ValidateAQL] bridge uses
// it). An empty/whitespace-only query yields aql_empty; a parse failure
// yields aql_syntax; otherwise it runs [Lint].
func LintString(q string, opts *Options) Result {
	if strings.TrimSpace(q) == "" {
		return Result{Issues: []Issue{{
			Code:     "aql_empty",
			Detail:   "query is empty",
			Severity: Error,
		}}}
	}
	doc, err := parse.Parse(q)
	if err != nil {
		return Result{Issues: []Issue{{
			Code:     "aql_syntax",
			Detail:   syntaxDetail(err),
			Severity: Error,
		}}}
	}
	return Lint(doc, opts)
}

// syntaxDetail formats a parse failure for lint consumers. REQ-109 requires
// line/column in Detail for aql_syntax; [parse.SyntaxError] carries position.
func syntaxDetail(err error) string {
	var se *parse.SyntaxError
	if errors.As(err, &se) {
		return fmt.Sprintf("%d:%d: %s", se.Pos.Line, se.Pos.Col, se.Msg)
	}
	return err.Error()
}

// Lint runs Layers 2–3 on a document obtained from a successful
// [parse.Parse] (Layer 1). opts may be nil. As a guard, a nil or unparsed
// (zero-value) document yields a single aql_syntax issue rather than an
// empty result. Lint is collect-all: it returns every issue across every
// enabled layer.
func Lint(doc *parse.Document, opts *Options) Result {
	if doc == nil || !doc.Parsed() {
		return Result{Issues: []Issue{{
			Code:     "aql_syntax",
			Detail:   "not a parsed document",
			Severity: Error,
		}}}
	}
	if opts == nil {
		opts = &Options{}
	}
	md := Extract(doc)
	issues := []Issue{}

	issues = append(issues, shapeIssues(doc, md)...)
	if opts.Query != nil {
		issues = append(issues, paramIssues(md, opts.Query)...)
	}
	if opts.Compiled != nil {
		issues = append(issues, templateIssues(md, opts.Compiled)...)
	}
	return Result{Issues: issues}
}

// shapeIssues runs the Layer-2 (AST-only) checks: alias binding and
// identifiable-scope. SELECT/FROM presence is guaranteed by a successful
// parse (the grammar requires both), so no aql_select / aql_from issue can
// arise here.
func shapeIssues(doc *parse.Document, md Metadata) []Issue {
	var issues []Issue

	// aql_unknown_alias — every identified path's root alias MUST bind to a
	// class in FROM / CONTAINS.
	for _, p := range doc.Paths {
		if _, ok := md.Aliases[p.Alias]; !ok {
			issues = append(issues, Issue{
				Code:     "aql_unknown_alias",
				Path:     p.Raw,
				Detail:   fmt.Sprintf("path alias %q is not bound in FROM/CONTAINS", p.Alias),
				Severity: Error,
			})
		}
	}

	// aql_from_archetype — the query SHOULD identify what it selects: at
	// least one archetype HRID, a $param archetype predicate, a VERSION
	// operand, or an EHR root. A query with none scans broadly; advisory.
	if !hasIdentifiableScope(doc) {
		issues = append(issues, Issue{
			Code:     "aql_from_archetype",
			Detail:   "FROM/CONTAINS names no archetype, $param, VERSION, or EHR scope",
			Severity: Warning,
		})
	}
	return issues
}

func hasIdentifiableScope(doc *parse.Document) bool {
	for _, ce := range doc.Classes {
		if ce.Archetype != "" || ce.ParamArchetype || ce.Version || ce.RMType == "EHR" {
			return true
		}
	}
	return false
}

// paramIssues runs the Layer-2 parameter-binding checks against a Query's
// Parameters map.
func paramIssues(md Metadata, q *aql.Query) []Issue {
	var issues []Issue

	// aql_unbound_param — every $name referenced MUST have a Parameters key.
	for _, name := range md.Params {
		if _, ok := q.Parameters[name]; !ok {
			issues = append(issues, Issue{
				Code:     "aql_unbound_param",
				Detail:   fmt.Sprintf("$%s is referenced but not bound in Query.Parameters", name),
				Severity: Error,
			})
		}
	}

	// aql_unused_param — a bound parameter not referenced is advisory.
	referenced := make(map[string]bool, len(md.Params))
	for _, name := range md.Params {
		referenced[name] = true
	}
	// Sort keys: map iteration order is random, but Result.Issues is
	// documented to be in deterministic discovery order.
	for _, key := range slices.Sorted(maps.Keys(q.Parameters)) {
		if !referenced[key] {
			issues = append(issues, Issue{
				Code:     "aql_unused_param",
				Detail:   fmt.Sprintf("Query.Parameters[%q] is bound but never referenced", key),
				Severity: Warning,
			})
		}
	}
	return issues
}
