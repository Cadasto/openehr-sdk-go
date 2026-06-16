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
