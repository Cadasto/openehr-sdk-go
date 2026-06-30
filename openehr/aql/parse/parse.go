// Package parse turns an AQL string into a syntax-checked, generated-type-free
// view (REQ-109). It validates the query against the SDK grammar profile
// (resources/aql/grammar/active, ADR 0007); the CDR remains the execute-time
// semantic authority (PROBE-021), so a query that parses here may still be
// rejected on execution.
//
// This is a building block (REQ-013): it imports neither transport/ nor auth/
// nor any client. The generated ANTLR parser is kept unexported behind this
// package's types.
package parse

import (
	"fmt"
	"sync"

	"github.com/antlr4-go/antlr/v4"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// Position is a 1-based line/column into the source AQL.
type Position struct {
	Line int
	Col  int
}

// SyntaxError is a parse failure at a position. It wraps [aql.ErrSyntax], so
// callers can branch with errors.Is(err, aql.ErrSyntax).
type SyntaxError struct {
	Pos Position
	Msg string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("aql: syntax error at %d:%d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
}

func (e *SyntaxError) Unwrap() error { return aql.ErrSyntax }

// Document is a parsed, syntactically valid AQL query. Clause presence,
// classes, paths, and parameters are extracted during [Parse]. The
// exported slice fields are an owned, read-only view — callers MUST NOT
// mutate them (lint clones what it carries across its own boundary).
type Document struct {
	tree gen.ISelectQueryContext

	// query / queryErr are the structured AST and any catalogue-gap
	// error from extraction, populated lazily on the first call to
	// [Document.Query] / [Document.QueryErr] / [ParseQuery] (SDK-GAP-17
	// Tier 2, REQ-113). Extraction cost is only paid when a consumer
	// asks for the structured shape; lint-only callers stay on the
	// existing flat view. queryOnce guards the lazy build.
	queryOnce sync.Once
	query     *Query
	queryErr  error

	// Distinct is true for SELECT DISTINCT.
	Distinct bool
	// Star is true for a bare SELECT * (SDK-AQL-002 relaxation).
	Star bool
	// NumSelect is the number of SELECT projection items.
	NumSelect int
	// HasWhere / HasOrderBy / HasLimit report optional-clause presence.
	HasWhere   bool
	HasOrderBy bool
	HasLimit   bool

	// Classes are the class expressions bound in the FROM / CONTAINS tree,
	// flattened to document order.
	Classes []ClassExpr
	// Paths are every alias-qualified identified path across the SELECT,
	// WHERE, and ORDER BY clauses, in document order.
	Paths []IdentifiedPath
	// Params are the distinct $parameter names referenced anywhere in the
	// query, in first-seen order, with the leading `$` stripped.
	Params []string
}

// Parse validates q against the SDK grammar profile and returns the parsed
// document. On the first syntax error it returns a *[SyntaxError] (wrapping
// [aql.ErrSyntax]) and a nil document.
func Parse(q string) (*Document, error) {
	lexer := gen.NewAqlLexer(antlr.NewInputStream(q))
	collector := &errorCollector{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(collector)

	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := gen.NewAqlParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(collector)

	tree := p.SelectQuery()
	if len(collector.errors) > 0 {
		return nil, collector.errors[0]
	}

	doc := &Document{tree: tree}
	doc.populate()
	doc.extract()
	return doc, nil
}

// Parsed reports whether d is the result of a successful [Parse] call.
func (d *Document) Parsed() bool { return d != nil && d.tree != nil }

// ParseQuery is the SDK-GAP-17 Tier-2 entry (REQ-113): it validates q
// against the SDK grammar profile (the same grammar [Parse] uses) and
// returns the structured [Query] AST directly — the read-side mirror
// of the [aql.Builder] construction model.
//
// On a syntax error it returns a *[SyntaxError] (wrapping
// [aql.ErrSyntax]) and a nil query, matching [Parse]'s error contract.
//
// When the source parses cleanly but contains a shape outside the
// Tier-2 extraction catalogue, ParseQuery returns the populated
// best-effort [Query] AND an error wrapping [aql.ErrIncompleteAST]
// describing the dropped shapes. Callers can decide whether the
// partial AST is useful (errors.Is(err, aql.ErrIncompleteAST)) or
// fall back to the flat [Document] / [Document.Tree] views.
//
// Internally one parse pass produces both the flat [Document] view
// (returned by [Parse], drives lint per REQ-109) and the structured
// [Query] AST. Callers that need both should use [Parse] and then
// [Document.Query] / [Document.QueryErr] — there is no double-parse
// cost.
func ParseQuery(q string) (*Query, error) {
	doc, err := Parse(q)
	if err != nil {
		return nil, err
	}
	return doc.Query(), doc.QueryErr()
}

// Tree returns the validated ANTLR parse tree backing this document.
//
// Deprecated: Tree exposes the generated parser context
// ([gen.ISelectQueryContext]) and may change shape on any grammar
// regeneration. Use [Document.Query] / [ParseQuery] for the stable
// structured AST; Tree remains as the SDK-GAP-17 Tier-1 escape hatch
// for consumers that need raw-tree access during the transition.
//
// Returns nil for a zero-value document — call only after a successful
// [Parse], or guard with [Document.Parsed].
func (d *Document) Tree() gen.ISelectQueryContext {
	if d == nil {
		return nil
	}
	return d.tree
}

// Query returns the structured AQL AST for this document (SDK-GAP-17
// Tier 2, REQ-113). Lazily extracted on the first call and cached
// behind a [sync.Once] so concurrent callers see a single, stable
// pointer; repeated calls return the same pointer. Returns nil for a
// zero-value document.
//
// The structured AST is the read-side mirror of [aql.Builder] — its
// SELECT items, FROM containment tree, WHERE expression tree, ORDER BY
// terms, and LIMIT / OFFSET values can all be traversed without
// importing the generated parser package or any internal/ package.
// The [aql.WhereExpr] and [aql.Value] vocabularies are SHARED with the
// construction side: emitting a parsed [Query] back to AQL via the
// existing emitter is the round-trip property pinned by Phase 3e.
//
// Inspect [Document.QueryErr] for catalogue-gap diagnostics (an error
// wrapping [aql.ErrIncompleteAST] when the source carried a shape
// outside the v1 extraction catalogue).
func (d *Document) Query() *Query {
	if d == nil || d.tree == nil {
		return nil
	}
	d.queryOnce.Do(func() {
		d.query, d.queryErr = extractQuery(d.tree)
	})
	return d.query
}

// QueryErr reports any catalogue-gap diagnostic captured during
// extraction of the structured [Query] (SDK-GAP-17 Tier 2). Returns
// nil when the source falls fully inside the v1 catalogue, otherwise
// an error wrapping [aql.ErrIncompleteAST] naming the dropped shapes.
//
// Repeated calls return the same error (or nil); QueryErr triggers
// the lazy extraction the same way [Document.Query] does, so calling
// it first is equivalent to calling Query first.
func (d *Document) QueryErr() error {
	if d == nil || d.tree == nil {
		return nil
	}
	d.queryOnce.Do(func() {
		d.query, d.queryErr = extractQuery(d.tree)
	})
	return d.queryErr
}

func (d *Document) populate() {
	if sc := d.tree.SelectClause(); sc != nil {
		d.Distinct = sc.DISTINCT() != nil
		exprs := sc.AllSelectExpr()
		d.NumSelect = len(exprs)
		for _, e := range exprs {
			if e.SYM_ASTERISK() != nil {
				d.Star = true
			}
		}
	}
	d.HasWhere = d.tree.WhereClause() != nil
	d.HasOrderBy = d.tree.OrderByClause() != nil
	d.HasLimit = d.tree.LimitClause() != nil
}

// errorCollector captures syntax errors from the lexer and parser instead of
// printing them to stderr (the ANTLR default).
type errorCollector struct {
	*antlr.DefaultErrorListener
	errors []*SyntaxError
}

func (c *errorCollector) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	// ANTLR columns are 0-based; expose 1-based.
	c.errors = append(c.errors, &SyntaxError{Pos: Position{Line: line, Col: column + 1}, Msg: msg})
}
