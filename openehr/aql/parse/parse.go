// Package parse turns an AQL string into a syntax-checked, generated-type-free
// view. It validates the query against the SDK grammar profile
// (resources/aql/grammar/active); the CDR remains the execute-time semantic
// authority, so a query that parses here may still be rejected on execution.
//
// It imports neither transport/ nor auth/ nor any client. The generated ANTLR
// parser is kept unexported behind this package's types.
package parse

import (
	"errors"
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

// Span is a half-open source range [Start, End) in the same line/column
// vocabulary as [Position]. It deliberately does not use byte offsets, so
// that a span, a [SyntaxError] position and an AST node's Pos are directly
// comparable. End is the position just past the last character of the
// construct.
//
// The zero Span means "not attributable".
type Span struct {
	Start Position
	End   Position
}

// IsZero reports whether the span carries no position.
func (s Span) IsZero() bool { return s == Span{} }

// SyntaxError is a parse failure at a position. It wraps [aql.ErrSyntax], so
// callers can branch with errors.Is(err, aql.ErrSyntax). One that [Parse]
// returns for parentheses and brackets nested past its limit matches
// [aql.ErrLimitExceeded] as well.
type SyntaxError struct {
	Pos Position
	Msg string

	// limit marks the refusal of parentheses and brackets nested past
	// maxNesting, which [SyntaxError.Is] matches to aql.ErrLimitExceeded.
	limit bool
}

// Error names the position and the message. A nil receiver answers with
// the zero SyntaxError's text instead of reading Pos: a failed errors.As /
// errors.AsType leaves a typed nil that a caller may box into a non-nil
// error interface and print. A zero Pos (the nil receiver's own shape, and
// any caller-built SyntaxError that never set one) omits the position
// segment instead of printing a made-up "at 0:0:"; no real construction
// site reports that position (the ANTLR listener's Line/Col start at 1).
func (e *SyntaxError) Error() string {
	if e == nil {
		return (&SyntaxError{}).Error()
	}
	if e.Pos == (Position{}) {
		return "aql: syntax error: " + e.Msg
	}
	return fmt.Sprintf("aql: syntax error at %d:%d: %s", e.Pos.Line, e.Pos.Col, e.Msg)
}

// Unwrap exposes [aql.ErrSyntax]. It reads no field, so a nil receiver
// classifies as a syntax error exactly as a real one does.
func (e *SyntaxError) Unwrap() error { return aql.ErrSyntax }

// Is reports whether e is the refusal of parentheses and brackets nested past
// [Parse]'s limit and target is [aql.ErrLimitExceeded]. A nil receiver matches no
// limit, as the zero SyntaxError does not.
func (e *SyntaxError) Is(target error) bool {
	return e != nil && e.limit && target == aql.ErrLimitExceeded //nolint:errorlint // an Is method compares its target as it is, by contract
}

// Document is a parsed, syntactically valid AQL query. Clause presence,
// classes, paths, and parameters are extracted during [Parse]. The
// exported slice fields are an owned, read-only view; callers must not
// mutate them (lint clones what it carries across its own boundary).
//
// A Document keeps the parse tree it was read from, and through it the
// parser's tokens and state, for as long as the Document is reachable:
// several times the memory of the [Query] it yields. To keep parsed queries
// for long, in a cache say, keep the [Query] from [ParseQuery] or
// [Document.Query], which refers to no part of the tree, and let the
// Document go.
type Document struct {
	tree gen.ISelectQueryContext
	// src is the text tree was parsed from. The structured extraction reads
	// it to report each clause's byte offsets: the ANTLR input stream holds
	// runes, and re-encoding them would not give back the caller's bytes when
	// the text carries invalid UTF-8.
	src string

	// query / queryErr are the structured AST and any catalogue-gap
	// error from extraction, populated lazily on the first call to
	// [Document.Query] / [Document.QueryErr] / [ParseQuery] (REQ-113
	// Tier 2). Extraction cost is only paid when a consumer
	// asks for the structured shape; lint-only callers stay on the
	// existing flat view. queryOnce guards the lazy build.
	queryOnce sync.Once
	query     *Query
	queryErr  error
	// dropped is the value-free record per construct extraction could not
	// represent (REQ-113 § Value-free structured drop records), populated by
	// the same lazy pass. Exposed as a copy by [Document.Dropped].
	dropped []DroppedConstruct

	// Distinct is true for SELECT DISTINCT.
	Distinct bool
	// Star is true when the projection carries a `*` item: both bare
	// `SELECT *` and mixed forms such as `SELECT *, e/ehr_id/value` (the
	// SDK-AQL-002 relaxation the linter's aql_select_star warns on). It is not
	// set for `COUNT(*)`: that star belongs to the aggregate call
	// ([FunctionCall.Star]), not the projection.
	Star bool
	// NumSelect is the number of SELECT projection items.
	NumSelect int
	// HasWhere / HasOrderBy / HasLimit report optional-clause presence.
	HasWhere   bool
	HasOrderBy bool
	HasLimit   bool

	// HasTop reports that the source carried a `SELECT TOP` clause, read
	// from the parse tree, so it is true even when the clause's count is
	// unrepresentable and [Document.Top] is therefore nil. Presence and
	// representability are separate questions: a check about whether the
	// deprecated construct was used at all (as the linter's is) must key on
	// this flag, or an out-of-range count would silence it.
	HasTop bool

	// Top is the decoded `SELECT TOP n [FORWARD|BACKWARD]` row limit, the
	// flat-view mirror of [SelectClause.Top], so the linter reads the clause
	// without paying for the structured extraction.
	//
	// It is nil both when no clause was written and when the clause's count
	// is outside Go `int`: nothing is ever truncated into a bound. Use
	// [Document.HasTop] to tell those two cases apart. The structured extractor
	// refuses the unrepresentable source, since that layer owns
	// representability.
	Top *aql.TopClause

	// SelectAliases are the `AS` aliases declared on SELECT projection
	// items, in document order; a projection without an AS clause
	// contributes nothing. They are a namespace of their own (an ORDER BY
	// key may name one instead of a FROM/CONTAINS alias), which is why they
	// are not merged into [Document.Classes].
	SelectAliases []string
	// Classes are the class expressions bound in the FROM / CONTAINS tree,
	// flattened to document order.
	//
	// This is the flat lint view, so every [ClassExpr] read here has a nil
	// [ClassExpr.PredicateComparison]; a standing predicate is carried as
	// verbatim text ([ClassExpr.Predicate]) and as [ClassExpr.ParsedPredicate],
	// which both views fill. Read [Document.Query] for the parsed comparison;
	// see [ClassExpr.PredicateComparison] for the rule.
	Classes []ClassExpr
	// Paths are every alias-qualified identified path across the SELECT,
	// WHERE, and ORDER BY clauses, in document order. A bare `true` /
	// `false` keyword in a value position, a comparison operand
	// (`WHERE s/x = true`) or a SELECT projection item (`SELECT true`), is a
	// literal, not a path, and is absent even though the lexer yields it as an
	// IDENTIFIER. A keyword carrying a path predicate or a path tail
	// (`true/nested`) is a real path and is recorded, as is an ORDER BY key
	// naming a keyword (the ORDER BY position admits no literal).
	Paths []IdentifiedPath
	// Params are the distinct $parameter names referenced anywhere in the
	// query, in first-seen order, with the leading `$` stripped.
	Params []string
}

// Parse validates q against the SDK grammar profile and returns the parsed
// document. On the first syntax error it returns a *[SyntaxError] (wrapping
// [aql.ErrSyntax]) and a nil document.
//
// Parse refuses a query whose parentheses and path-predicate brackets,
// counted together, nest more than 128 levels deep, before it parses the
// query, with a *[SyntaxError] at the parenthesis or bracket that opens level
// 129 that matches [aql.ErrLimitExceeded] as well. The parser keeps what it
// learns about each new nesting depth for the life of the process, so the
// limit bounds what a deeply nested query can make it keep. A parenthesis or
// bracket inside a string literal does not count.
func Parse(q string) (*Document, error) {
	stream, lexErrors := lex(q)
	if err := checkNesting(stream.GetAllTokens()); err != nil {
		return nil, err
	}
	if tree, ok := parseSLL(stream, len(lexErrors)); ok {
		doc := &Document{tree: tree, src: q}
		doc.populate()
		doc.extract()
		return doc, nil
	}
	// The default parse reads q again, so that the lexer's errors and the
	// parser's arrive in the order the parse meets them and the error
	// returned is the first in the input.
	lexer := gen.NewAqlLexer(antlr.NewInputStream(q))
	collector := &errorCollector{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(collector)

	p := gen.NewAqlParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	p.RemoveErrorListeners()
	p.AddErrorListener(collector)

	tree := p.SelectQuery()
	if len(collector.errors) > 0 {
		return nil, collector.errors[0]
	}

	doc := &Document{tree: tree, src: q}
	doc.populate()
	doc.extract()
	return doc, nil
}

// maxNesting is the deepest nesting of parentheses and brackets, counted
// together, that [Parse] admits (REQ-109 § Nesting limit). The parser's
// prediction state is shared by every parse for the life of the process and
// grows with each new depth a query reaches, through either delimiter, a
// refused query's included, so the depth is counted on the tokens before any
// parse runs.
const maxNesting = 128

// lex reads q into a token stream, filled to its end, and returns it with the
// errors the lexer reported while reading it.
func lex(q string) (*antlr.CommonTokenStream, []*SyntaxError) {
	lexer := gen.NewAqlLexer(antlr.NewInputStream(q))
	errs := &errorCollector{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	stream.Fill()
	return stream, errs.errors
}

// checkNesting refuses tokens whose parentheses and brackets, counted
// together, nest more than maxNesting levels deep, at the one that opens the
// first level past it. A closing parenthesis or bracket with none open leaves
// the depth at zero, so it cannot lend depth to the ones after it.
func checkNesting(tokens []antlr.Token) error {
	depth := 0
	for _, t := range tokens {
		switch t.GetTokenType() {
		case gen.AqlLexerSYM_LEFT_PAREN, gen.AqlLexerSYM_LEFT_BRACKET:
			depth++
			if depth > maxNesting {
				return &SyntaxError{
					Pos:   Position{Line: t.GetLine(), Col: t.GetColumn() + 1}, // ANTLR columns are 0-based
					Msg:   fmt.Sprintf("parentheses and brackets nested more than %d levels deep", maxNesting),
					limit: true,
				}
			}
		case gen.AqlLexerSYM_RIGHT_PAREN, gen.AqlLexerSYM_RIGHT_BRACKET:
			depth = max(depth-1, 0)
		}
	}
	return nil
}

// parseSLL parses the tokens of stream in ANTLR's SLL prediction mode,
// stopping at the first error, and reports whether that parse succeeded.
// SLL prediction skips the full-context analysis the default LL mode runs
// where the grammar needs more than one token of lookahead to choose, as it
// does inside path predicates. ANTLR guarantees that an SLL parse that
// succeeds builds the tree LL builds, while one that fails may only lack LL's
// power, so [Parse] reads a failure as no answer and parses the query again in
// the default mode. The second parse, and it alone, reports the errors, so a
// query Parse refuses gets the error it always got. lexErrors is how many
// errors the lexer reported while filling stream: with any, the SLL parse
// declines without parsing, since a character the lexer dropped can leave
// tokens the parser accepts.
func parseSLL(stream *antlr.CommonTokenStream, lexErrors int) (tree gen.ISelectQueryContext, ok bool) {
	if lexErrors > 0 {
		return nil, false
	}
	p := gen.NewAqlParser(stream)
	p.RemoveErrorListeners()
	p.SetErrorHandler(&bailOnError{DefaultErrorStrategy: antlr.NewDefaultErrorStrategy()})
	p.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)

	defer func() {
		if r := recover(); r != nil {
			if r != errBailedOut { //nolint:errorlint // the sentinel is panicked as it is, never wrapped
				panic(r)
			}
			tree, ok = nil, false
		}
	}()
	return p.SelectQuery(), true
}

// errBailedOut is what [bailOnError] panics with to stop the SLL parse at its
// first error; [parseSLL] recovers it, and nothing else.
var errBailedOut = errors.New("aql: SLL parse stopped at its first error")

// bailOnError stops a parse at its first error, by panicking with
// errBailedOut, rather than recovering and parsing on. antlr4-go's own
// BailErrorStrategy cannot be used here: the generated rules hand the
// cancellation it records to ReportError, whose default strategy panics on it,
// and then clear it, so the parse would carry on.
type bailOnError struct {
	*antlr.DefaultErrorStrategy
}

func (*bailOnError) ReportError(antlr.Parser, antlr.RecognitionException) {}

func (*bailOnError) Recover(antlr.Parser, antlr.RecognitionException) { panic(errBailedOut) }

func (*bailOnError) RecoverInline(antlr.Parser) antlr.Token { panic(errBailedOut) }

func (*bailOnError) Sync(antlr.Parser) {}

// Parsed reports whether d is the result of a successful [Parse] call.
func (d *Document) Parsed() bool { return d != nil && d.tree != nil }

// ParseQuery is the structured entry point: it validates q against the SDK
// grammar profile (the same grammar [Parse] uses) and returns the structured
// [Query] AST directly, the read-side mirror of the [aql.Builder]
// construction model.
//
// On a syntax error it returns a *[SyntaxError] (wrapping
// [aql.ErrSyntax]) and a nil query, matching [Parse]'s error contract.
//
// When the source parses cleanly but contains a shape outside the
// structured-AST catalogue, ParseQuery returns the populated
// best-effort [Query] and an error wrapping [aql.ErrIncompleteAST]
// describing the dropped shapes. Callers can decide whether the
// partial AST is useful (errors.Is(err, aql.ErrIncompleteAST)) or
// fall back to the flat [Document] / [Document.Tree] views.
//
// Internally one parse pass produces both the flat [Document] view
// (returned by [Parse], which drives lint) and the structured [Query] AST.
// Callers that need both should use [Parse] and then [Document.Query] /
// [Document.QueryErr]; there is no double-parse cost.
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
// structured AST, and [Query.Spans] for where each clause sits in the
// source; Tree remains as a raw-tree escape hatch for consumers that
// still need it.
//
// Returns nil for a zero-value document; call only after a successful
// [Parse], or guard with [Document.Parsed].
func (d *Document) Tree() gen.ISelectQueryContext {
	if d == nil {
		return nil
	}
	return d.tree
}

// Query returns the structured AQL AST for this document. It is extracted
// lazily on the first call and cached behind a [sync.Once], so concurrent
// callers see a single, stable pointer; repeated calls return the same
// pointer. Returns nil for a zero-value document.
//
// The structured AST is the read-side mirror of [aql.Builder]: its SELECT
// items, FROM containment tree, WHERE expression tree, ORDER BY terms, and
// LIMIT / OFFSET values can all be traversed without importing the
// generated parser package or any internal/ package. The [aql.WhereExpr]
// and [aql.Value] vocabularies are shared with the construction side, so a
// parsed [Query] can be emitted back to AQL through the same emitter.
//
// Inspect [Document.QueryErr] for catalogue-gap diagnostics (an error
// wrapping [aql.ErrIncompleteAST] when the source carried a shape
// outside the extraction catalogue).
func (d *Document) Query() *Query {
	if d == nil || d.tree == nil {
		return nil
	}
	d.queryOnce.Do(func() {
		d.query, d.dropped, d.queryErr = extractQuery(d.tree, d.src)
	})
	return d.query
}

// QueryErr reports any catalogue-gap diagnostic captured during
// extraction of the structured [Query]. Returns nil when the source falls
// fully inside the catalogue, otherwise an error wrapping
// [aql.ErrIncompleteAST] naming the dropped shapes.
//
// Repeated calls return the same error (or nil); QueryErr triggers
// the lazy extraction the same way [Document.Query] does, so calling
// it first is equivalent to calling Query first.
func (d *Document) QueryErr() error {
	if d == nil || d.tree == nil {
		return nil
	}
	d.queryOnce.Do(func() {
		d.query, d.dropped, d.queryErr = extractQuery(d.tree, d.src)
	})
	return d.queryErr
}

func (d *Document) populate() {
	if sc := d.tree.SelectClause(); sc != nil {
		d.Distinct = sc.DISTINCT() != nil
		// REQ-109: the deprecated TOP clause, mirrored into the flat view for
		// the lint gate. HasTop records PRESENCE from the tree; Top records
		// the DECODED bound and stays nil when the count is unrepresentable,
		// in lockstep with the structured extractor's topClause — nothing is
		// truncated into a bound. Keeping the two separate is what lets a
		// presence check survive an out-of-range count.
		if top := sc.Top(); top != nil {
			d.HasTop = true
			var ex astExtractor
			d.Top = ex.topClause(top)
		}
		exprs := sc.AllSelectExpr()
		d.NumSelect = len(exprs)
		for _, e := range exprs {
			if e.SYM_ASTERISK() != nil {
				d.Star = true
			}
			// REQ-109: `columnExpr AS alias` — the only IDENTIFIER that is a
			// direct child of selectExpr is the AS alias (a columnExpr's own
			// identifiers sit deeper). Kept in lockstep with the structured
			// extractor's extractSelectItem.
			if id := e.IDENTIFIER(); id != nil {
				d.SelectAliases = append(d.SelectAliases, id.GetText())
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
