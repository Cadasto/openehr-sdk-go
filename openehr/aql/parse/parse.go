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

	// query is the structured AST populated lazily on the first call to
	// [Document.Query] or [ParseQuery] (SDK-GAP-17 Tier 2, REQ-113).
	// Extraction cost is only paid when a consumer asks for the
	// structured shape; lint-only callers stay on the existing flat view.
	query *Query

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
// Internally one parse pass produces both the flat [Document] view
// (returned by [Parse], drives lint per REQ-109) and the structured
// [Query] AST. Callers that need both should use [Parse] and then
// [Document.Query] — there is no double-parse cost.
func ParseQuery(q string) (*Query, error) {
	doc, err := Parse(q)
	if err != nil {
		return nil, err
	}
	return doc.Query(), nil
}

// Tree returns the validated ANTLR parse tree backing this document.
//
// Unstable consumer contract — the return type comes from the generated
// parser package ([gen.ISelectQueryContext]) and may change shape on
// any grammar regeneration. The accessor is the SDK-GAP-17 Tier-1
// interim (REQ-113); prefer the structured [Query] AST ([Document.Query]
// or [ParseQuery]) for new consumers that need to read SELECT /
// CONTAINS / WHERE / ORDER BY / LIMIT structure without coupling to
// the generated typed-context tree.
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
// Tier 2, REQ-113). Lazily extracted on the first call and cached;
// repeated calls return the same pointer. Returns nil for a
// zero-value document.
//
// The structured AST is the read-side mirror of [aql.Builder] — its
// SELECT items, FROM containment tree, WHERE expression tree, ORDER BY
// terms, and LIMIT / OFFSET values can all be traversed without
// importing the generated parser package or any internal/ package.
// The [aql.WhereExpr] and [aql.Value] vocabularies are SHARED with the
// construction side: emitting a parsed [Query] back to AQL via the
// existing emitter is the round-trip property pinned by Phase 3e.
func (d *Document) Query() *Query {
	if d == nil || d.tree == nil {
		return nil
	}
	if d.query == nil {
		d.query = extractQuery(d.tree)
	}
	return d.query
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
