package parse

// query.go: SDK-GAP-17 Tier 2 — the readable, generated-type-free AST
// (REQ-113). Mirrors the write-side aql.Builder: SELECT items / FROM
// containment tree / WHERE expression tree / ORDER BY terms / LIMIT
// + OFFSET, all readable without importing parse/gen or any internal/
// package. The WHERE and Value vocabularies are SHARED with the
// construction side — Comparison / Junction / NotExpr / ExistsExpr /
// MatchesExpr / LikeExpr / ParamValue / StringValue / etc. all live
// in `openehr/aql`, populated by both Builder and Parse.
//
// Returned by [ParseQuery]; consumers MUST NOT mutate it (the
// document-side IdentifiedPath / ClassExpr slices are owned by the
// document, not the consumer).

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// Query is the structured AQL AST: a parse-time mirror of [aql.Builder]'s
// write-side construction model. Construct via [ParseQuery]; the read-side
// helpers ([Document.Tree] for the raw ANTLR tree, [Document] for the
// flattened lint view) remain available for callers that don't need the
// structured shape.
//
// Field zero values follow AQL semantics: an empty [Select] indicates no
// projection (a malformed query the parser would have rejected); nil
// [Where] means no WHERE clause; nil [Limit] / [Offset] mean the clause
// was absent in the source.
type Query struct {
	// Select is the SELECT projection list (`Items`) plus its flags.
	Select SelectClause

	// From is the FROM clause: a root class plus the optional containment
	// tree below it.
	From FromClause

	// Where is the WHERE predicate as a structured expression tree, or
	// nil when no WHERE clause is present. The concrete shapes
	// (aql.Comparison / aql.Junction / aql.NotExpr / aql.ExistsExpr /
	// aql.MatchesExpr / aql.LikeExpr) carry readable fields a consumer
	// can introspect after a type assertion.
	Where aql.WhereExpr

	// OrderBy is the ORDER BY list in document order; nil when absent.
	OrderBy []OrderTerm

	// Limit is the row-count limit when present; nil when no LIMIT
	// clause appeared in the source.
	Limit *int

	// Offset is the row offset when present; nil when no OFFSET
	// clause appeared in the source.
	Offset *int
}

// SelectClause is the SELECT projection list.
//
// `Distinct` mirrors the `SELECT DISTINCT` keyword; `Star` is true for
// the bare `SELECT *` form (SDK-AQL-002 relaxation), in which case
// `Items` is empty. Otherwise `Items` carries one entry per projected
// expression in source order.
type SelectClause struct {
	Distinct bool
	Star     bool
	Items    []SelectItem
}

// SelectItem is one projected expression in a SELECT list. `Expr` is
// either a [PathExpr] (a bare alias-qualified path) or a [FunctionCall]
// (an aggregate or function wrapper around one or more paths). `Alias`
// is the AS alias when the source used `<expr> AS <name>`; empty
// otherwise.
type SelectItem struct {
	Expr  SelectExpr
	Alias string
}

// SelectExpr is the sealed type of a SELECT operand. The concrete
// shapes are [PathExpr] and [FunctionCall]; consumers dispatch via
// type assertion. Adding a new shape (e.g. arithmetic, literal
// projection) MUST land here and in the extractor at the same time.
type SelectExpr interface {
	isSelectExpr()
}

// PathExpr is a bare alias-qualified RM path projected from a SELECT.
type PathExpr struct {
	IdentifiedPath
}

func (PathExpr) isSelectExpr() {}

// FunctionCall is an aggregate or function wrapping one or more SELECT
// operands — `COUNT(o)`, `MAX(o/data[at0001]/value/magnitude)`,
// `CONCAT(p/given_name, ' ', p/family_name)`, etc. `Name` is the
// upper-cased function name as it appears in the source; `Args` is
// the ordered operand list.
type FunctionCall struct {
	Name string
	Args []SelectExpr
}

func (FunctionCall) isSelectExpr() {}

// FromClause is the FROM clause: a root class plus the optional
// containment tree below it.
//
// `Root` is the leftmost class expression (e.g. `EHR e`, `COMPOSITION c`,
// `EHR e[ehr_id/value=$x]`). `Contains` is the optional CONTAINS
// expression rooted at the FROM root; nil when no CONTAINS appears.
type FromClause struct {
	Root     ClassExpr
	Contains *Containment
}

// Containment is one node in the CONTAINS tree.
//
// A simple `COMPOSITION c CONTAINS OBSERVATION o` populates a [FromClause]
// whose `Contains` is `&Containment{Class: <OBSERVATION o>}`.
//
// A boolean junction (`CONTAINS (OBSERVATION o OR EVALUATION e)`)
// populates a [Containment] whose `Class` is the zero value, `Children`
// is the list of operands, and `ChildJoin` reports the connector
// (AND / OR). A `NOT CONTAINS` populates `Negated = true` on the term
// being negated.
//
// Containment terms can nest: `COMPOSITION c CONTAINS SECTION s CONTAINS
// OBSERVATION o` yields a chain where the outer term's
// `Children[0].Children` carries `OBSERVATION o`. The walker descends
// into both Children and (via the chained CONTAINS keyword) further
// nested containments.
type Containment struct {
	// Class is the class expression at this containment node. Zero
	// value when the node is a pure boolean grouping (Children only).
	Class ClassExpr

	// Children are nested CONTAINS terms below this node. Multiple
	// children imply a boolean junction via ChildJoin.
	Children []Containment

	// ChildJoin is the boolean combinator across Children. Defaults
	// to [ContainsAnd]; only meaningful when len(Children) > 1.
	ChildJoin ContainsJoin

	// Negated is true for `NOT CONTAINS …` / `NOT <term>` forms.
	Negated bool
}

// ContainsJoin is the boolean combinator joining sibling CONTAINS
// terms. AND is the AQL default; OR appears explicitly in the source.
type ContainsJoin int

const (
	// ContainsAnd joins siblings with AND (the default).
	ContainsAnd ContainsJoin = iota
	// ContainsOr joins siblings with OR.
	ContainsOr
)

// String renders the keyword for diagnostics.
func (j ContainsJoin) String() string {
	if j == ContainsOr {
		return "OR"
	}
	return "AND"
}

// OrderTerm is one ORDER BY term: a path and its sort direction.
type OrderTerm struct {
	// Path is the alias-qualified path being ordered.
	Path IdentifiedPath
	// Dir is the sort direction; defaults to [OrderAsc] when the
	// source omitted the keyword (AQL spec default).
	Dir OrderDir
}

// OrderDir is the sort direction of an ORDER BY term.
type OrderDir int

const (
	// OrderAsc is ascending (the AQL spec default).
	OrderAsc OrderDir = iota
	// OrderDesc is descending.
	OrderDesc
)

// String renders the keyword for emission and diagnostics.
func (d OrderDir) String() string {
	if d == OrderDesc {
		return "DESC"
	}
	return "ASC"
}

// Emit renders the structured [Query] back to canonical AQL text — the
// round-trip mirror of [ParseQuery]. The same Comparison / Junction
// emitter the builder uses is consumed via [aql.FormatWhere], so a
// parsed-then-emitted predicate matches a builder-built one byte for
// byte (PROBE-020 / REQ-113).
//
// Idempotence property: ParseQuery(Emit(q)).Emit() == q.Emit() for any
// valid q produced by [ParseQuery] — the v1 catalogue of supported
// shapes is the buildable grammar plus the parser-only shapes
// (NotExpr / ExistsExpr / LikeExpr / MatchesExpr). Shapes outside the
// catalogue (Primitive-in-SELECT, parameter-bound LIMIT) round-trip on
// a best-effort basis.
//
// Returns an error wrapping [aql.ErrInvalidQuery] when the AST carries
// a malformed sub-expression (a nil WHERE comparison value, an empty
// SELECT projection, …).
func (q *Query) Emit() (string, error) {
	if q == nil {
		return "", fmt.Errorf("%w: nil query", aql.ErrInvalidQuery)
	}
	var sb strings.Builder

	// SELECT
	sb.WriteString("SELECT ")
	if q.Select.Distinct {
		sb.WriteString("DISTINCT ")
	}
	switch {
	case q.Select.Star:
		sb.WriteByte('*')
	case len(q.Select.Items) == 0:
		return "", fmt.Errorf("%w: empty SELECT projection", aql.ErrInvalidQuery)
	default:
		for i, item := range q.Select.Items {
			if i > 0 {
				sb.WriteString(", ")
			}
			s, err := emitSelectItem(item)
			if err != nil {
				return "", err
			}
			sb.WriteString(s)
		}
	}

	// FROM
	if q.From.Root.RMType == "" {
		return "", fmt.Errorf("%w: missing FROM root", aql.ErrInvalidQuery)
	}
	sb.WriteString(" FROM ")
	sb.WriteString(emitClassExpr(q.From.Root))
	if q.From.Contains != nil {
		sb.WriteString(" CONTAINS ")
		sb.WriteString(emitContainment(*q.From.Contains))
	}

	// WHERE
	if q.Where != nil {
		pred, err := aql.FormatWhere(q.Where)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(pred) != "" {
			sb.WriteString(" WHERE ")
			sb.WriteString(pred)
		}
	}

	// ORDER BY
	if len(q.OrderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		for i, t := range q.OrderBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(t.Path.Raw)
			sb.WriteByte(' ')
			sb.WriteString(t.Dir.String())
		}
	}

	// LIMIT / OFFSET
	if q.Limit != nil {
		sb.WriteString(" LIMIT ")
		sb.WriteString(strconv.Itoa(*q.Limit))
	}
	if q.Offset != nil {
		sb.WriteString(" OFFSET ")
		sb.WriteString(strconv.Itoa(*q.Offset))
	}

	return sb.String(), nil
}

func emitSelectItem(item SelectItem) (string, error) {
	s, err := emitSelectExpr(item.Expr)
	if err != nil {
		return "", err
	}
	if item.Alias != "" {
		s += " AS " + item.Alias
	}
	return s, nil
}

func emitSelectExpr(e SelectExpr) (string, error) {
	switch v := e.(type) {
	case PathExpr:
		return v.IdentifiedPath.Raw, nil
	case FunctionCall:
		args := make([]string, 0, len(v.Args))
		for _, a := range v.Args {
			s, err := emitSelectExpr(a)
			if err != nil {
				return "", err
			}
			args = append(args, s)
		}
		return v.Name + "(" + strings.Join(args, ", ") + ")", nil
	}
	if e == nil {
		return "", fmt.Errorf("%w: nil SELECT expression", aql.ErrInvalidQuery)
	}
	return "", fmt.Errorf("%w: unsupported SELECT expression %T", aql.ErrInvalidQuery, e)
}

func emitClassExpr(c ClassExpr) string {
	if c.Version {
		// VERSION class — `VERSION <alias>` (predicate emission deferred
		// to the version-aware emitter, not in the v1 catalogue).
		out := "VERSION"
		if c.Alias != "" {
			out += " " + c.Alias
		}
		return out
	}
	out := c.RMType
	if c.Alias != "" {
		out += " " + c.Alias
	}
	if c.Archetype != "" {
		out += "[" + c.Archetype + "]"
	}
	return out
}

func emitContainment(c Containment) string {
	prefix := ""
	if c.Negated {
		prefix = "NOT "
	}
	// Boolean junction: render each child and join with the operator.
	if len(c.Children) > 0 && c.Class.RMType == "" {
		parts := make([]string, len(c.Children))
		for i, ch := range c.Children {
			parts[i] = emitContainment(ch)
		}
		joiner := " " + c.ChildJoin.String() + " "
		return prefix + "(" + strings.Join(parts, joiner) + ")"
	}
	// Class + optional inner chain.
	out := prefix + emitClassExpr(c.Class)
	if len(c.Children) > 0 {
		// CONTAINS chain — emit the chain child(ren).
		for _, ch := range c.Children {
			out += " CONTAINS " + emitContainment(ch)
		}
	}
	return out
}
