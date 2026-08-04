package aql

import (
	"fmt"
	"strings"
)

// WhereExpr is a boolean expression in a WHERE clause. The interface is sealed;
// construct expressions with the comparison helpers ([Eq], [Ne], [Gt], [Ge],
// [Lt], [Le]) and combine them with [And] / [Or]. Parsed queries populate the
// same concrete types ([Comparison] / [Junction]) — the read AST and the
// write AST share one vocabulary (REQ-113). Concrete-type
// fields are intended for read access; mutating an expression already
// passed to [FormatWhere] / [Builder.Build] is undefined (the emitter
// caches a `validate()` outcome the mutation would invalidate).
//
// Use [FormatWhere] to render a [WhereExpr] to canonical AQL text (e.g.
// when emitting a parsed [parse.Query] back to a string).
//
// The set grows ADDITIVELY as the structured-AST catalogue closes further
// grammar positions (REQ-117), so a consumer type-switching over it MUST
// treat an unrecognised case as out-of-catalogue — refuse, skip, or
// report — and MUST NOT panic on it.
type WhereExpr interface {
	// expr is the canonical wire form of the predicate.
	expr() string
	// validate reports a malformed predicate (empty path, nil value) so
	// [Builder.Build] can surface it as ErrInvalidQuery instead of panicking
	// or emitting invalid AQL.
	validate() error
}

// FormatWhere renders a [WhereExpr] to canonical AQL text. It validates
// the expression first; a malformed predicate (empty path, nil value,
// …) returns an error wrapping [ErrInvalidQuery]. A nil expression
// returns "" with no error (a vacuously-true WHERE — the builder skips
// the clause in that case).
//
// This is the public read-side mirror of the internal expr() method:
// consumers of a parsed [parse.Query] use FormatWhere to round-trip
// the WHERE predicate back to AQL without depending on package-local
// internals.
func FormatWhere(w WhereExpr) (string, error) {
	if w == nil {
		return "", nil
	}
	if err := w.validate(); err != nil {
		return "", err
	}
	return w.expr(), nil
}

// FormatValue renders an [aql.Value] to canonical AQL text (the same
// emission the Builder uses internally). Returns "" for a nil Value.
// Mirrors [FormatWhere] for the value side of the vocabulary.
func FormatValue(v Value) string {
	if v == nil {
		return ""
	}
	return v.token()
}

// Operator is a comparison operator on a [Comparison]. The wire string is
// the typed string itself; values are not interpolated, so consumers can
// safely match on `c.Op == aql.OpEq` etc.
type Operator string

const (
	// OpEq is `=`.
	OpEq Operator = "="
	// OpNe is `!=`.
	OpNe Operator = "!="
	// OpGt is `>`.
	OpGt Operator = ">"
	// OpGe is `>=`.
	OpGe Operator = ">="
	// OpLt is `<`.
	OpLt Operator = "<"
	// OpLe is `<=`.
	OpLe Operator = "<="
)

// Comparison is a `path <op> value` predicate. It is the concrete type both
// the construction helpers ([Eq] / [Ne] / [Gt] / [Ge] / [Lt] / [Le]) and the
// parser populate; consumers reading a parsed query type-assert
// `w.(aql.Comparison)` and read the fields directly.
//
// Path is the alias-qualified RM path as it appears in the AQL text (e.g.
// `e/ehr_status/subject/external_ref/id/value`); Op is one of the [Operator]
// constants; Val is the [Value] on the right-hand side ([ParamValue] for a
// placeholder, [StringValue] / [IntValue] / [RealValue] / [BoolValue] for a
// literal).
//
// ParsedPath is the structured form of Path (alias + segments), populated
// by the parser on the read side so a consumer reads alias/segments without
// re-splitting the raw string (REQ-113); it is nil on the write side
// (the construction helpers set only Path) and MAY be nil on the read side
// for a path shape the parser does not structure. When non-nil,
// ParsedPath.Raw equals Path (both derive from the same source path).
// Emission uses Path, not ParsedPath, so round-trip is unaffected by its
// presence or absence.
//
// Left carries the left operand when it is NOT a plain path — the
// grammar's `functionCall COMPARISON_OPERATOR terminal` alternative
// (`LENGTH(o/name/value) > 5`, `TERMINOLOGY('a','b','c') = 'x'`), modelled
// as a [FuncCall] (REQ-117). It is nil for the ordinary path form, where
// Path carries the left operand; when Left is non-nil it is authoritative
// for emission and Path is empty (the parser leaves it so). Construct this
// form with [Compare].
type Comparison struct {
	Path       string
	Op         Operator
	Val        Value
	ParsedPath *IdentifiedPath
	Left       Value
}

func (c Comparison) expr() string {
	return c.leftToken() + " " + string(c.Op) + " " + c.Val.token()
}

// leftToken renders the left operand: the structured [Comparison.Left]
// value when present, the raw path otherwise (REQ-117).
func (c Comparison) leftToken() string {
	if c.Left != nil {
		return c.Left.token()
	}
	return c.Path
}

func (c Comparison) validate() error {
	if c.Left == nil && strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("%w: empty path in %s comparison", ErrInvalidQuery, string(c.Op))
	}
	if c.Left != nil {
		if err := validateValue(c.Left); err != nil {
			return err
		}
	}
	if c.Val == nil {
		return fmt.Errorf("%w: nil value in comparison on %q", ErrInvalidQuery, c.leftToken())
	}
	return validateValue(c.Val)
}

// Eq is `path = value`.
func Eq(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpEq, Val: v} }

// Ne is `path != value`.
func Ne(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpNe, Val: v} }

// Gt is `path > value`.
func Gt(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpGt, Val: v} }

// Ge is `path >= value`.
func Ge(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpGe, Val: v} }

// Lt is `path < value`.
func Lt(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpLt, Val: v} }

// Le is `path <= value`.
func Le(path string, v Value) WhereExpr { return Comparison{Path: path, Op: OpLe, Val: v} }

// Compare is `<left> <op> <right>` where the LEFT operand is a structured
// [Value] rather than a path — the write-side mirror of the parser's
// function-call comparison LHS (REQ-117), e.g.
// Compare(Func("LENGTH", Path("o/name/value")), OpGt, Int(5)) emits
// `LENGTH(o/name/value) > 5`. Use [Eq] / [Ne] / [Gt] / [Ge] / [Lt] / [Le]
// for the ordinary path form.
func Compare(left Value, op Operator, right Value) WhereExpr {
	return Comparison{Op: op, Val: right, Left: left}
}

// BoolOp is a boolean junction operator (AND or OR) joining terms in a
// [Junction]. NOT is a single-operand prefix; see [Not] (when introduced
// by the parser-side AST extension).
type BoolOp string

const (
	// OpAnd is the AND junction.
	OpAnd BoolOp = "AND"
	// OpOr is the OR junction.
	OpOr BoolOp = "OR"
)

// Junction is a multi-term boolean junction (`a AND b`, `a OR b OR c`,
// …). Op is one of the [BoolOp] constants; Terms is the ordered list of
// operands. Parsed queries flatten same-operator chains: a literal
// `a OR b OR c` populates a single [Junction] with three Terms; a
// mixed-operator expression `a AND (b OR c)` populates an outer AND
// [Junction] whose second term is itself a [Junction]. The emitter
// re-parenthesises a nested OR inside an AND to preserve precedence
// (and vice-versa is unnecessary because OR has lower precedence).
type Junction struct {
	Op    BoolOp
	Terms []WhereExpr
}

func (j Junction) expr() string {
	parts := make([]string, len(j.Terms))
	for i, t := range j.Terms {
		// Parenthesise a nested OR inside an AND to preserve precedence;
		// a bare comparison or same-operator junction needs no grouping.
		if inner, ok := t.(Junction); ok && inner.Op == OpOr && j.Op == OpAnd {
			parts[i] = "(" + t.expr() + ")"
			continue
		}
		parts[i] = t.expr()
	}
	return strings.Join(parts, " "+string(j.Op)+" ")
}

func (j Junction) validate() error {
	for _, t := range j.Terms {
		if err := t.validate(); err != nil {
			return err
		}
	}
	return nil
}

// And joins predicates with AND. nil terms are dropped; a single surviving term
// is returned unchanged; no terms yields nil (a vacuously-true conjunction —
// the builder emits no WHERE rather than invalid AQL).
func And(terms ...WhereExpr) WhereExpr { return junctionOf(OpAnd, terms) }

// Or joins predicates with OR, with the same nil/empty handling as [And].
func Or(terms ...WhereExpr) WhereExpr { return junctionOf(OpOr, terms) }

func junctionOf(op BoolOp, terms []WhereExpr) WhereExpr {
	kept := make([]WhereExpr, 0, len(terms))
	for _, t := range terms {
		if t != nil {
			kept = append(kept, t)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		return kept[0]
	default:
		return Junction{Op: op, Terms: kept}
	}
}

// NotExpr is a single-operand boolean negation (`NOT <operand>`). Parsed
// queries populate this when the source carries an explicit NOT prefix.
// The Builder composes NOT predicates via the package-level [Not] helper
// passed into [Builder.Where] — mirroring how [And] / [Or] / [Eq] are
// also package-level helpers rather than Builder methods. No dedicated
// Builder.Not method exists by design; predicate composition is intended
// to flow through the helper functions.
type NotExpr struct {
	Operand WhereExpr
}

func (n NotExpr) expr() string {
	if n.Operand == nil {
		return "NOT"
	}
	// Parenthesise any junction operand so the precedence reads
	// unambiguously regardless of which junctions surround the NOT.
	if _, ok := n.Operand.(Junction); ok {
		return "NOT (" + n.Operand.expr() + ")"
	}
	return "NOT " + n.Operand.expr()
}

func (n NotExpr) validate() error {
	if n.Operand == nil {
		return fmt.Errorf("%w: NOT with nil operand", ErrInvalidQuery)
	}
	return n.Operand.validate()
}

// Not constructs a [NotExpr]. A nil operand yields nil (the emitter has
// nothing to express and the builder skips the clause).
func Not(operand WhereExpr) WhereExpr {
	if operand == nil {
		return nil
	}
	return NotExpr{Operand: operand}
}

// ExistsExpr is the `EXISTS <path>` AQL predicate (presence of a node
// at the path). The parser populates this from the EXISTS form.
type ExistsExpr struct {
	Path string
}

func (e ExistsExpr) expr() string { return "EXISTS " + e.Path }

func (e ExistsExpr) validate() error {
	if strings.TrimSpace(e.Path) == "" {
		return fmt.Errorf("%w: empty path in EXISTS", ErrInvalidQuery)
	}
	return nil
}

// Exists constructs an [ExistsExpr]. An empty path is rejected at
// build time, not at construction.
func Exists(path string) WhereExpr { return ExistsExpr{Path: path} }

// MatchesExpr is the `<path> MATCHES <operand>` AQL predicate. The
// grammar admits three operand forms and exactly ONE of the fields below
// carries the operand:
//
//   - Values — the braced value list (`{'active', 'archived'}`); each
//     member is any [Value], including a [FuncCall] for the grammar's
//     `valueListItem : terminologyFunction` alternative.
//   - Terminology — a BARE `TERMINOLOGY('op','api','params')` operand,
//     with no braces (REQ-117); construct with [MatchesTerminology].
//   - URI — a braced URI operand (`{uri://…}`), carried verbatim
//     (REQ-117); construct with [MatchesURI]. A whitespace-only URI counts as
//     ABSENT — for both validation and emission — so it never shadows a
//     populated Values list.
type MatchesExpr struct {
	Path        string
	Values      []Value
	Terminology *FuncCall
	URI         string
}

func (m MatchesExpr) expr() string {
	// The URI-operand test MUST use the same emptiness rule as validate() —
	// TrimSpace, not `!= ""`. A whitespace-only URI is not an operand there, so
	// treating it as one here would emit `MATCHES {   }` and silently drop a
	// validated value list.
	uri := strings.TrimSpace(m.URI)
	switch {
	case m.Terminology != nil:
		// Bare terminology operand — the grammar's `matchesOperand :
		// terminologyFunction` alternative takes no braces.
		return m.Path + " MATCHES " + m.Terminology.token()
	case uri != "":
		// Emitted trimmed, consistent with the [MatchesURI] constructor.
		return m.Path + " MATCHES {" + uri + "}"
	}
	parts := make([]string, len(m.Values))
	for i, v := range m.Values {
		if v == nil {
			parts[i] = ""
			continue
		}
		parts[i] = v.token()
	}
	return m.Path + " MATCHES {" + strings.Join(parts, ", ") + "}"
}

func (m MatchesExpr) validate() error {
	if strings.TrimSpace(m.Path) == "" {
		return fmt.Errorf("%w: empty path in MATCHES", ErrInvalidQuery)
	}
	// The three operand forms are mutually exclusive: emitting more than
	// one would silently drop an operand (REQ-117).
	forms := 0
	if len(m.Values) > 0 {
		forms++
	}
	if m.Terminology != nil {
		forms++
	}
	if strings.TrimSpace(m.URI) != "" {
		forms++
	}
	switch {
	case forms == 0:
		return fmt.Errorf("%w: empty value list in MATCHES on %q", ErrInvalidQuery, m.Path)
	case forms > 1:
		return fmt.Errorf("%w: MATCHES on %q sets more than one operand form", ErrInvalidQuery, m.Path)
	}
	if m.Terminology != nil {
		return validateValue(*m.Terminology)
	}
	for i, v := range m.Values {
		if v == nil {
			return fmt.Errorf("%w: nil value at index %d in MATCHES on %q", ErrInvalidQuery, i, m.Path)
		}
		if err := validateValue(v); err != nil {
			return err
		}
	}
	return nil
}

// Matches constructs a [MatchesExpr] over a braced value list.
func Matches(path string, values ...Value) WhereExpr {
	return MatchesExpr{Path: path, Values: values}
}

// MatchesTerminology constructs the `<path> MATCHES
// TERMINOLOGY(operation, api, params)` predicate — the bare
// terminology-function operand form (REQ-117).
func MatchesTerminology(path, operation, api, params string) WhereExpr {
	fc := Terminology(operation, api, params).(FuncCall)
	return MatchesExpr{Path: path, Terminology: &fc}
}

// MatchesURI constructs the `<path> MATCHES {uri}` predicate — the URI
// operand form (REQ-117). The URI is emitted verbatim inside the braces
// (the grammar's URI token is unquoted).
func MatchesURI(path, uri string) WhereExpr {
	return MatchesExpr{Path: path, URI: strings.TrimSpace(uri)}
}

// LikeExpr is the `<path> LIKE <pattern>` AQL predicate. Pattern is a
// string literal carrying AQL wildcards (`_` single char, `%` any
// sequence). Pattern is a [Value] so the same shape covers both a
// literal pattern and a parameter-bound pattern.
type LikeExpr struct {
	Path    string
	Pattern Value
}

func (l LikeExpr) expr() string {
	if l.Pattern == nil {
		return l.Path + " LIKE "
	}
	return l.Path + " LIKE " + l.Pattern.token()
}

func (l LikeExpr) validate() error {
	if strings.TrimSpace(l.Path) == "" {
		return fmt.Errorf("%w: empty path in LIKE", ErrInvalidQuery)
	}
	if l.Pattern == nil {
		return fmt.Errorf("%w: nil pattern in LIKE on %q", ErrInvalidQuery, l.Path)
	}
	return validateValue(l.Pattern)
}

// Like constructs a [LikeExpr].
func Like(path string, pattern Value) WhereExpr {
	return LikeExpr{Path: path, Pattern: pattern}
}
