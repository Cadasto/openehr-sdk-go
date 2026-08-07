package parse

// query.go: REQ-113 Tier 2 — the readable, generated-type-free AST.
// Mirrors the write-side aql.Builder: SELECT items / FROM
// containment tree / WHERE expression tree / ORDER BY terms / LIMIT
// + OFFSET, all readable without importing parse/gen or any internal/
// package. The WHERE and Value vocabularies are SHARED with the
// construction side — Comparison / Junction / NotExpr / ExistsExpr /
// MatchesExpr / LikeExpr / ParamValue / StringValue / etc. all live
// in `openehr/aql`, populated by both Builder and Parse.
//
// Returned by [ParseQuery]; mutation of fields after the Query has
// been emitted via [Query.Emit] (or shared across goroutines that may
// emit it) is undefined — the document-side IdentifiedPath /
// ClassExpr slices are intended to read identical equality with
// [Document.Paths] / [Document.Classes].

import (
	"fmt"
	"slices"
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
	// clause appeared in the source. Concrete shapes are [IntLimit] for
	// integer literals (`LIMIT 50`) and [ParamLimit] for parameter-bound
	// limits (`LIMIT $n`).
	Limit LimitExpr

	// Offset is the row offset when present; nil when no OFFSET
	// clause appeared in the source. Same concrete shapes as [Limit].
	Offset LimitExpr

	// incomplete records the catalogue-gap error from the extractor
	// (an error wrapping [aql.ErrIncompleteAST] when present). Emit
	// refuses to render an incomplete AST so a caller who ignored
	// [ParseQuery]'s error cannot accidentally produce semantically
	// wrong AQL. Direct-construction Querys leave this nil (the
	// caller owns the AST shape).
	incomplete error
}

// LimitExpr is the sealed type of a LIMIT / OFFSET value. Concrete shapes
// are [IntLimit] (integer literal) and [ParamLimit] (parameter-bound limit
// — the AQL `LIMIT $n` form). Consumers dispatch via type assertion.
//
// The set grows ADDITIVELY (REQ-117), so a consumer type-switching over it
// MUST treat an unrecognised case as out-of-catalogue — refuse, skip, or
// report — and MUST NOT panic on it.
type LimitExpr interface {
	isLimitExpr()
	// token is the canonical wire form: an integer literal for [IntLimit],
	// `$name` for [ParamLimit].
	token() string
}

// IntLimit is an integer-literal LIMIT / OFFSET value.
type IntLimit struct {
	N int
}

func (IntLimit) isLimitExpr()    {}
func (l IntLimit) token() string { return strconv.Itoa(l.N) }

// ParamLimit is a parameter-bound LIMIT / OFFSET value (`LIMIT $n`).
// Name carries the placeholder identifier WITHOUT the leading `$`.
type ParamLimit struct {
	Name string
}

func (ParamLimit) isLimitExpr()    {}
func (l ParamLimit) token() string { return "$" + l.Name }

// SelectClause is the SELECT projection list.
//
// `Distinct` mirrors the `SELECT DISTINCT` keyword; `Star` is true when
// the projection carries a `*` (SDK-AQL-002 relaxation). For the BARE
// `SELECT *` form `Items` is empty — the flag alone carries the
// projection. Otherwise `Items` carries one entry per projected
// expression in source order, including a [StarExpr] at the star's
// position when a star is mixed with column projections
// (`SELECT *, c/uid/value` — REQ-117).
type SelectClause struct {
	Distinct bool
	Star     bool
	Items    []SelectItem

	// Top is the row limit from a `SELECT TOP n [FORWARD|BACKWARD]` clause,
	// nil when the source declared none — so `TOP 0` (a real bound) never
	// collapses into "unbounded" (REQ-118).
	//
	// The clause is DEPRECATED upstream (openEHR QUERY Release-1.1.0
	// § 4.4.3, in favour of `LIMIT` with `ORDER BY`) and is modelled so a
	// consumer can read and re-emit a query it did not author; see
	// [aql.TopClause].
	//
	// Top and [Query.Limit] are reported INDEPENDENTLY, exactly as the
	// source wrote them: § 4.4.3 forbids the combination, so the parser
	// neither normalises one into the other nor picks a winner — the lint
	// gate diagnoses it (`aql_top_with_limit`) and [aql.Builder] refuses to
	// construct it.
	Top *aql.TopClause
}

// TopClause is the deprecated `SELECT TOP` row limit — re-exported from
// [aql.TopClause], the shared SELECT-clause vocabulary (REQ-118).
type TopClause = aql.TopClause

// TopDir is a [TopClause] direction — re-exported from [aql.TopDir]
// (REQ-118). Use [aql.TopForward] / [aql.TopBackward].
type TopDir = aql.TopDir

// SelectItem is one projected expression in a SELECT list. `Expr` is one
// of the [SelectExpr] shapes — a [PathExpr] (a bare alias-qualified
// path), a [FunctionCall] (an aggregate or function wrapper), a
// [LiteralExpr] (a primitive or parameter literal), or a [StarExpr].
// `Alias` is the AS alias when the source used `<expr> AS <name>`; empty
// otherwise.
type SelectItem struct {
	Expr  SelectExpr
	Alias string
}

// SelectExpr is the sealed type of a SELECT operand. The concrete
// shapes are [PathExpr], [FunctionCall], [LiteralExpr], and [StarExpr];
// consumers dispatch via type assertion.
//
// The set grows ADDITIVELY as the catalogue closes further grammar
// positions (REQ-117), so a consumer type-switching over it MUST treat
// an unrecognised case as out-of-catalogue — refuse, skip, or report —
// and MUST NOT panic on it. Adding a new shape lands here, in the
// extractor, and in the emitter at the same time.
type SelectExpr interface {
	isSelectExpr()
}

// PathExpr is a bare alias-qualified RM path projected from a SELECT.
type PathExpr struct {
	IdentifiedPath
}

func (PathExpr) isSelectExpr() {}

// LiteralExpr is a primitive or parameter literal projected from a
// SELECT — `SELECT 1, e/ehr_id/value FROM …` — or supplied as a
// function-call argument (`CONCAT('a', $p, …)`). Value carries the
// shared [aql.Value] vocabulary the WHERE side uses, so a consumer
// reads a projected literal and a compared literal through one model
// (REQ-117).
//
// Raw is the literal's SOURCE TEXT as written, which the openEHR result
// schema needs as the column name when a projection carries neither an
// `AS` alias nor a path to fall back on (REQ-118). It is read-side
// fidelity only:
//
//   - it is populated by [ParseQuery], and is EMPTY on a LiteralExpr a
//     caller constructed, so it MUST NOT be treated as required;
//   - emission renders Value in canonical form, never Raw — the canonical
//     write form is normative (REQ-055).
//
// The two therefore differ whenever the source was not already canonical:
// `1.50` yields Value `aql.RealValue{1.5}` with Raw `1.50`, and a
// double-quoted `"x"` yields `aql.StringValue{x}` with Raw `"x"`. Use
// [aql.FormatValue] to render a Value that has no Raw.
type LiteralExpr struct {
	Value aql.Value
	Raw   string
}

func (LiteralExpr) isSelectExpr() {}

// StarExpr is an explicit `*` projection item. It appears in
// [SelectClause.Items] only when the star is mixed with column
// projections (`SELECT *, c/uid/value`), so the item list stays
// order-preserving; the bare `SELECT *` form leaves Items empty and is
// carried by [SelectClause.Star] alone (REQ-117). Star is true in both
// cases.
type StarExpr struct{}

func (StarExpr) isSelectExpr() {}

// FunctionCall is an aggregate or function wrapping one or more SELECT
// operands — `COUNT(o)`, `MAX(o/data[at0001]/value/magnitude)`,
// `CONCAT(p/given_name, ' ', p/family_name)`, etc. `Name` is the
// upper-cased function name as it appears in the source; `Args` is
// the ordered operand list.
//
// `Star` is true for the `COUNT(*)` aggregate form (Args is empty in
// that case); `Distinct` is true when the aggregate carried the
// `DISTINCT` keyword (`COUNT(DISTINCT path)`).
type FunctionCall struct {
	Name     string
	Args     []SelectExpr
	Distinct bool
	Star     bool
}

func (FunctionCall) isSelectExpr() {}

// FromClause is the FROM clause: either a root class plus the optional
// containment tree below it, or a boolean junction of containment
// operands.
//
// `Root` is the leftmost class expression (e.g. `EHR e`, `COMPOSITION c`,
// `EHR e[ehr_id/value=$x]`). `Contains` is the optional CONTAINS
// expression rooted at the FROM root; nil when no CONTAINS appears.
//
// `Junction` carries a boolean junction AT the FROM root
// (`FROM COMPOSITION c1 OR COMPOSITION c2`, incl. AND and grouping) —
// the same [Containment] tree the nested side already uses (REQ-117).
// It is nil for the ordinary single-root FROM, so `Root` keeps working
// unchanged there. When Junction is non-nil the clause has NO single
// root class, so `Root` and `Contains` are left ZERO: a consumer that
// only reads `Root` sees an empty FROM (and its own validation refuses)
// rather than a silently truncated one.
type FromClause struct {
	Root     ClassExpr
	Contains *Containment
	Junction *Containment
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
// round-trip mirror of [ParseQuery]. The WHERE predicate is rendered
// via [aql.FormatWhere], the same renderer the construction-side
// [aql.Builder] consumes — so a parsed-then-emitted predicate matches
// a builder-built one byte-for-byte. SELECT / FROM / CONTAINS /
// ORDER BY / LIMIT clauses are emitted by this package's helpers;
// the canonical form across both entry points is pinned by PROBE-020
// (Builder) and the round-trip suites here (parse).
//
// Idempotence property: ParseQuery(Emit(q)).Emit() == q.Emit() for any
// q produced by [ParseQuery] — since REQ-117 (and REQ-118, which added the
// deprecated `SELECT TOP` carrier) the catalogue is the whole SDK grammar
// profile (see [aql.ErrIncompleteAST] for the residual numeric-literal
// refusal). A source shape the extractor cannot model
// produces a PARTIAL Query — clauses that extracted cleanly are
// populated, dropped clauses are left zero-value — plus an
// [aql.ErrIncompleteAST] error from [ParseQuery]. Emit on a partial
// AST refuses with the same error so a caller who ignored the parse
// return cannot accidentally emit semantically wrong AQL.
//
// Canonical form for the constructs REQ-117 added: function names
// upper-cased, arguments joined by `, `, `TERMINOLOGY(a, b, c)` as a bare
// MATCHES operand (no braces) and `{uri}` for the URI form, and
// containment junctions parenthesised only where the grouping is
// load-bearing (see [emitContainmentOperands]).
//
// Returns an error wrapping [aql.ErrInvalidQuery] when the AST carries
// a malformed sub-expression (a nil WHERE comparison value, an empty
// SELECT projection, an OFFSET without LIMIT, a duplicate alias …), or
// [aql.ErrIncompleteAST] when the AST came from an extractor-
// incomplete parse.
func (q *Query) Emit() (string, error) {
	if q == nil {
		return "", fmt.Errorf("%w: nil query", aql.ErrInvalidQuery)
	}
	// Refuse to render an extractor-incomplete AST so a caller who
	// ignored [ParseQuery]'s error cannot accidentally emit
	// semantically wrong AQL (the extractor recorded which clauses
	// were dropped). The error wraps [aql.ErrIncompleteAST].
	if q.incomplete != nil {
		return "", q.incomplete
	}
	var sb strings.Builder

	// SELECT
	sb.WriteString("SELECT ")
	if q.Select.Distinct {
		sb.WriteString("DISTINCT ")
	}
	// REQ-118: the deprecated `TOP n [FORWARD|BACKWARD]` row limit sits
	// between DISTINCT and the projection list (grammar: `SELECT DISTINCT?
	// top? selectExpr …`). A negative count could only emit text the parser
	// rejects — the `top` production admits no sign.
	if t := q.Select.Top; t != nil {
		if t.N < 0 {
			return "", fmt.Errorf("%w: negative SELECT TOP count %d", aql.ErrInvalidQuery, t.N)
		}
		// A direction outside the vocabulary renders as nothing at all, which
		// would silently emit an undirected bound — refuse instead, mirroring
		// [aql.Builder]'s build-time guard.
		switch t.Dir {
		case aql.TopDirUnspecified, aql.TopForward, aql.TopBackward:
		default:
			return "", fmt.Errorf("%w: unknown SELECT TOP direction %d", aql.ErrInvalidQuery, int(t.Dir))
		}
		sb.WriteString(aql.FormatTop(t))
		sb.WriteByte(' ')
	}
	switch {
	// Items lead: a mixed `SELECT *, col` carries the star as a
	// [StarExpr] item, so the list is authoritative whenever it is
	// populated (REQ-117). The bare `SELECT *` form has no items.
	case len(q.Select.Items) == 0 && q.Select.Star:
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
	if q.From.Junction == nil && q.From.Root.RMType == "" {
		return "", fmt.Errorf("%w: missing FROM root", aql.ErrInvalidQuery)
	}
	// A junction root and a single root are mutually exclusive: the
	// grammar has no `(A OR B) CONTAINS C` form, so emitting both would
	// produce text the parser rejects (REQ-117).
	if q.From.Junction != nil && (q.From.Root.RMType != "" || q.From.Contains != nil) {
		return "", fmt.Errorf("%w: FROM sets both a root class and a root junction", aql.ErrInvalidQuery)
	}
	if dup := duplicateAlias(q.From); dup != "" {
		return "", fmt.Errorf("%w: duplicate alias %q", aql.ErrInvalidQuery, dup)
	}
	if err := validateContainmentTree(q.From); err != nil {
		return "", err
	}
	sb.WriteString(" FROM ")
	if q.From.Junction != nil {
		// REQ-117: a junction AT the root needs no grouping parentheses —
		// nothing encloses it. Nested junctions keep the parentheses
		// emitContainment writes for them.
		sb.WriteString(emitContainmentOperands(*q.From.Junction))
	} else {
		sb.WriteString(emitClassExpr(q.From.Root))
	}
	if q.From.Contains != nil {
		// Containment.Negated belongs to the connector: the parent of a
		// negated subtree writes `NOT CONTAINS` instead of `CONTAINS`.
		if q.From.Contains.Negated {
			sb.WriteString(" NOT CONTAINS ")
		} else {
			sb.WriteString(" CONTAINS ")
		}
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

	// LIMIT / OFFSET — grammar requires LIMIT before OFFSET, so emitting
	// OFFSET without LIMIT would produce text the parser rejects.
	if q.Offset != nil && q.Limit == nil {
		return "", fmt.Errorf("%w: OFFSET without LIMIT", aql.ErrInvalidQuery)
	}
	if q.Limit != nil {
		sb.WriteString(" LIMIT ")
		sb.WriteString(q.Limit.token())
	}
	if q.Offset != nil {
		sb.WriteString(" OFFSET ")
		sb.WriteString(q.Offset.token())
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
		return v.Raw, nil
	case StarExpr:
		// REQ-117: an explicit star item inside a mixed projection list.
		return "*", nil
	case LiteralExpr:
		// REQ-117: a primitive / parameter literal projection, rendered
		// through the shared value emitter so escaping matches WHERE.
		if v.Value == nil {
			return "", fmt.Errorf("%w: SELECT literal with nil value", aql.ErrInvalidQuery)
		}
		// REQ-119: [aql.FormatValue] cannot refuse — it has no error to return —
		// so the value-position guards ([aql.ValidateValue]: the non-finite real,
		// the reserved function name, TERMINOLOGY's arity) must be applied HERE
		// or the closure property holds for WHERE and not for SELECT. Without
		// this, `SELECT +Inf` and `SELECT TERMINOLOGY('a')` emitted with
		// err == nil, and `SELECT NaN` came back a path.
		if err := aql.ValidateValue(v.Value); err != nil {
			return "", fmt.Errorf("SELECT literal: %w", err)
		}
		return aql.FormatValue(v.Value), nil
	case FunctionCall:
		// REQ-119: a projected call's name must lex as the grammar's IDENTIFIER
		// or one of its *_FUNCTION_ID tokens. Unlike a value position, SELECT
		// reaches `aggregateFunctionCall`, so the aggregates are admissible here
		// — which is why this is [aql.ValidateSelectFuncName] and not the
		// value-position check.
		if err := aql.ValidateSelectFuncName(v.Name); err != nil {
			return "", fmt.Errorf("SELECT function call: %w", err)
		}
		var body string
		switch {
		case v.Star:
			body = "*"
		case v.Distinct:
			args := make([]string, 0, len(v.Args))
			for _, a := range v.Args {
				s, err := emitSelectExpr(a)
				if err != nil {
					return "", err
				}
				args = append(args, s)
			}
			body = "DISTINCT " + strings.Join(args, ", ")
		default:
			args := make([]string, 0, len(v.Args))
			for _, a := range v.Args {
				s, err := emitSelectExpr(a)
				if err != nil {
					return "", err
				}
				args = append(args, s)
			}
			body = strings.Join(args, ", ")
		}
		// Trimmed, so the emitted spelling is the one just validated; the casing
		// is left as parsed (REQ-055 canonicalises keywords, not function names).
		return strings.TrimSpace(v.Name) + "(" + body + ")", nil
	}
	if e == nil {
		return "", fmt.Errorf("%w: nil SELECT expression", aql.ErrInvalidQuery)
	}
	return "", fmt.Errorf("%w: unsupported SELECT expression %T", aql.ErrInvalidQuery, e)
}

func emitClassExpr(c ClassExpr) string {
	if c.Version {
		out := "VERSION"
		if c.Alias != "" {
			out += " " + c.Alias
		}
		if c.Predicate != "" {
			out += "[" + c.Predicate + "]"
		}
		return out
	}
	out := c.RMType
	if c.Alias != "" {
		out += " " + c.Alias
	}
	switch {
	case c.Archetype != "":
		// Archetype carries either a literal HRID or, when
		// ParamArchetype is true, the source `$name` placeholder
		// verbatim — both forms wrap in brackets unchanged.
		out += "[" + c.Archetype + "]"
	case c.Predicate != "":
		out += "[" + c.Predicate + "]"
	}
	return out
}

// duplicateAlias walks the FROM tree and returns the first non-empty
// alias seen more than once, or "" when all aliases are unique. Mirrors
// the alias-uniqueness guard in [aql.Builder.Build] so emission errors
// surface symmetrically on the read side.
func duplicateAlias(from FromClause) string {
	seen := make(map[string]struct{})
	check := func(a string) string {
		if a == "" {
			return ""
		}
		if _, ok := seen[a]; ok {
			return a
		}
		seen[a] = struct{}{}
		return ""
	}
	if dup := check(from.Root.Alias); dup != "" {
		return dup
	}
	var walk func(c *Containment) string
	walk = func(c *Containment) string {
		if c == nil {
			return ""
		}
		if dup := check(c.Class.Alias); dup != "" {
			return dup
		}
		for i := range c.Children {
			if dup := walk(&c.Children[i]); dup != "" {
				return dup
			}
		}
		return ""
	}
	// REQ-117: a FROM-root junction binds its aliases too.
	if dup := walk(from.Junction); dup != "" {
		return dup
	}
	return walk(from.Contains)
}

// validateContainmentTree reports the first structural defect in the FROM tree
// that [emitContainment] would otherwise render as text the SDK's own parser
// rejects, or nil when the tree is emittable.
//
// It is the read-side mirror of [aql.Containment.validateTree] (REQ-117,
// REQ-119) and refuses the same two things:
//
//   - An INCOMPLETE class node. A node that is neither a junction (operands, no
//     class of its own) nor a usable class emits nothing at all, so the chain
//     renders a dangling `… CONTAINS ` — or, with ChildJoin set on a class node,
//     silently DROPS the join and renders a plain CONTAINS chain instead.
//   - A MISPLACED junction. The grammar takes a parenthesised group as a whole
//     `containsExpr` alternative, so no CONTAINS keyword may follow one, and a
//     junction in a non-final position renders `… CONTAINS (A OR B) CONTAINS C`.
//
// Completeness is checked FIRST, because [isContainmentJunction] infers the node
// kind from an absent RMType: an incomplete class node otherwise looks like a
// junction and gets diagnosed as a misplaced one, naming a junction the caller
// never wrote.
//
// The extractor never builds such a tree (it only ever mirrors AQL that
// already parsed), so this guards the direct-construction path the [Query]
// doc blesses: a consumer that assembles or rewrites an AST by hand. Without
// it, the write side refuses the shape at Build() while the read side emitted
// it with err == nil — the asymmetry [Containment.emit] claims does not exist.
func validateContainmentTree(from FromClause) error {
	fail := func() error {
		return fmt.Errorf("%w: containment junction followed by a further CONTAINS term — a junction may "+
			"only end a containment chain; write the deeper nesting inside its operands", aql.ErrInvalidQuery)
	}
	// A junction carries operands and no class; a class node carries a class and
	// no join of its own. Anything else has no emittable spelling.
	var checkComplete func(c Containment) error
	checkComplete = func(c Containment) error {
		if !isContainmentJunction(c) {
			if c.Class.RMType == "" && !c.Class.Version {
				return fmt.Errorf("%w: CONTAINS requires an RM type (or VERSION) and an alias", aql.ErrInvalidQuery)
			}
			if c.ChildJoin != 0 {
				return fmt.Errorf("%w: class containment %q carries a ChildJoin; a join belongs to a junction "+
					"node (no class of its own), and emission would silently drop it",
					aql.ErrInvalidQuery, c.Class.RMType)
			}
		}
		for _, ch := range c.Children {
			if err := checkComplete(ch); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range []*Containment{from.Junction, from.Contains} {
		if root == nil {
			continue
		}
		if err := checkComplete(*root); err != nil {
			return err
		}
	}
	// chainEndsInJunction asks whether the FLATTENED chain rooted at c ends in
	// a junction — an element whose own tail is a junction is still followed
	// by the next element's CONTAINS keyword.
	var chainEndsInJunction func(c Containment) bool
	chainEndsInJunction = func(c Containment) bool {
		if isContainmentJunction(c) {
			return true
		}
		return len(c.Children) > 0 && chainEndsInJunction(c.Children[len(c.Children)-1])
	}
	var walk func(c *Containment) error
	walk = func(c *Containment) error {
		if c == nil {
			return nil
		}
		// A junction node's children are unordered operands with no CONTAINS
		// between them, so the placement rule applies to a class node's chain
		// only — the same carve-out the builder side makes.
		if !isContainmentJunction(*c) &&
			slices.ContainsFunc(c.Children[:max(len(c.Children)-1, 0)], chainEndsInJunction) {
			return fail()
		}
		for i := range c.Children {
			if err := walk(&c.Children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	// The FROM root is itself the head of a chain when it carries Contains.
	if err := walk(from.Junction); err != nil {
		return err
	}
	return walk(from.Contains)
}

// isContainmentJunction reports whether a node is a pure boolean
// grouping — operands only, no class of its own (REQ-117).
func isContainmentJunction(c Containment) bool {
	return c.Class.RMType == "" && len(c.Children) > 0
}

// emitContainmentOperands renders a junction node's operands joined by its
// keyword, WITHOUT enclosing parentheses (REQ-117). The FROM root uses it
// directly; [emitContainment] wraps the result for a nested junction.
//
// An operand is parenthesised only where the grouping is load-bearing:
//   - an OR junction inside an AND junction (precedence: AND binds tighter);
//   - a CONTAINS chain, because the grammar's `CONTAINS containsExpr` right
//     operand is greedy — without the parentheses the following AND / OR
//     operand would re-parse INTO the chain.
//
// A same- or tighter-binding junction operand needs no grouping, mirroring
// [aql.Junction]'s WHERE-side rule.
func emitContainmentOperands(c Containment) string {
	parts := make([]string, len(c.Children))
	for i, ch := range c.Children {
		switch {
		case isContainmentJunction(ch):
			inner := emitContainmentOperands(ch)
			if c.ChildJoin == ContainsAnd && ch.ChildJoin == ContainsOr {
				inner = "(" + inner + ")"
			}
			parts[i] = inner
		case len(ch.Children) > 0:
			parts[i] = "(" + emitContainment(ch) + ")"
		default:
			parts[i] = emitContainment(ch)
		}
	}
	return strings.Join(parts, " "+c.ChildJoin.String()+" ")
}

// emitContainment renders a Containment node. The Negated flag is
// consumed by the PARENT (which writes `NOT CONTAINS` instead of
// `CONTAINS`) — emitContainment itself ignores it and just renders
// the class + chained children.
func emitContainment(c Containment) string {
	// Boolean junction: render the operands and group them. A junction
	// nested under a CONTAINS keyword or inside another junction keeps
	// its parentheses so the grouping survives a re-parse; only the FROM
	// root drops them (see [Query.Emit]).
	if isContainmentJunction(c) {
		return "(" + emitContainmentOperands(c) + ")"
	}
	// Class + optional inner chain. A child's Negated flag selects
	// `NOT CONTAINS` over `CONTAINS` for the connector to that child.
	var sb strings.Builder
	sb.WriteString(emitClassExpr(c.Class))
	for _, ch := range c.Children {
		if ch.Negated {
			sb.WriteString(" NOT CONTAINS ")
		} else {
			sb.WriteString(" CONTAINS ")
		}
		sb.WriteString(emitContainment(ch))
	}
	return sb.String()
}
