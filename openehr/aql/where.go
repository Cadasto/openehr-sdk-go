package aql

import (
	"errors"
	"fmt"
	"strings"
)

// WhereExpr is a boolean expression in a WHERE clause. The interface is sealed;
// construct expressions with the comparison helpers ([Eq], [Ne], [Gt], [Ge],
// [Lt], [Le]) and combine them with [And] / [Or]. Parsed queries populate the
// same concrete types ([Comparison] / [Junction]) — the read AST and the
// write AST share one vocabulary (REQ-113). Concrete-type
// fields are intended for read access; mutating an expression already
// passed to [FormatWhere] / [Builder.Build] is undefined (both validate and
// then emit as two steps, so a mutation between them emits unvalidated text).
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
// the clause in that case), and so does a typed-nil pointer shape such as
// `(*Comparison)(nil)`: at the TOP level both denote a clause the caller does
// not have. Inside a [NotExpr] or a [Junction] the same absence is refused
// instead — see [DerefWhere] (REQ-119, "absence is positional").
//
// This is the public read-side mirror of the internal expr() method:
// consumers of a parsed [parse.Query] use FormatWhere to round-trip
// the WHERE predicate back to AQL without depending on package-local
// internals.
func FormatWhere(w WhereExpr) (string, error) {
	pred, ok := derefWhere(w)
	if !ok {
		// No predicate at all — the documented "no WHERE clause" case. A
		// typed-nil lands here too: it denotes no predicate just as an untyped
		// nil does, and at the TOP level that is a clause the caller does not
		// have. Inside a composite the same absence is refused instead, because
		// there an operand that vanishes changes what the query asks.
		return "", nil
	}
	if err := pred.validate(); err != nil {
		return "", err
	}
	return pred.expr(), nil
}

// DerefWhere normalises a [WhereExpr] to the predicate it denotes, reporting
// false when it denotes none — an untyped nil, or a nil pointer. It is the
// [DerefValue] of the predicate vocabulary.
//
// The value-receiver problem is the same one: expr and validate have value
// receivers, so `*Comparison` satisfies WhereExpr alongside `Comparison`, and a
// bare type switch that lists only the value shapes silently misses every
// pointer twin. Any code deciding behaviour from a WhereExpr's concrete type —
// including the read-side `w.(aql.Comparison)` idiom [Comparison] describes —
// MUST normalise first, or the same rule will bind one carrier and not the
// other (REQ-119).
//
// Both consequences have bitten this package: calling a value-receiver method
// on a typed-nil crashes a public boundary, and deciding [Junction]
// parenthesisation from the raw interface emitted a nested OR without its
// parentheses — valid AQL asking something else.
func DerefWhere(w WhereExpr) (WhereExpr, bool) { return derefWhere(w) }

// derefWhere is [derefValue] for the WHERE vocabulary: same exhaustive
// value-plus-pointer-twin switch, same fail-closed default, same case-coverage
// hold by the dispatch tripwire.
func derefWhere(w WhereExpr) (WhereExpr, bool) {
	switch x := w.(type) {
	case Comparison, Junction, NotExpr, ExistsExpr, MatchesExpr, LikeExpr:
		return x, true
	case *Comparison:
		if x != nil {
			return *x, true
		}
	case *Junction:
		if x != nil {
			return *x, true
		}
	case *NotExpr:
		if x != nil {
			return *x, true
		}
	case *ExistsExpr:
		if x != nil {
			return *x, true
		}
	case *MatchesExpr:
		if x != nil {
			return *x, true
		}
	case *LikeExpr:
		if x != nil {
			return *x, true
		}
	}
	return nil, false // untyped nil, a typed-nil pointer, or an unlearned shape
}

// FormatValue renders an [aql.Value] to canonical AQL text (the same
// emission the Builder uses internally). Mirrors [FormatWhere] for the value
// side of the vocabulary.
//
// UNLIKE [FormatWhere], it does NOT validate: it has no error to return, so it
// cannot refuse, and a value the grammar has no spelling for renders as its Go
// text (`+Inf`, `NaN`) or as a call the parser rejects. It is therefore the
// deliberate escape hatch for a value the caller has already checked, and is
// excluded from REQ-119's round-trip closure guarantee for that reason. Call
// [ValidateValue] first if the value did not come from a validated source.
//
// It does not PANIC, though: a value with no wire form at all — a nil, or a
// typed-nil pointer shape such as the zero [MatchesExpr.Terminology] — returns
// "". Refusing one is [ValidateValue]'s job, and an unvalidated formatter that
// panics is a worse escape hatch than one that renders nothing.
func FormatValue(v Value) string {
	inner, ok := derefValue(v)
	if !ok {
		return ""
	}
	return inner.token()
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

// known reports whether o is one of the six operators the grammar's
// COMPARISON_OPERATOR admits. Operator is a named string so a parsed query
// keeps its wire spelling, which leaves the type open — [Compare] is the
// first constructor to take one from the caller, so the value is checked at
// validate time rather than emitted verbatim (REQ-117).
func (o Operator) known() bool {
	switch o {
	case OpEq, OpNe, OpGt, OpGe, OpLt, OpLe:
		return true
	default:
		return false
	}
}

// Comparison is a `path <op> value` predicate. It is the concrete type both
// the construction helpers ([Eq] / [Ne] / [Gt] / [Ge] / [Lt] / [Le]) and the
// parser populate; consumers reading a parsed query type-assert
// `w.(aql.Comparison)` and read the fields directly. That bare assert is safe
// for a query [parse.ParseQuery] produced, which only ever populates the value
// shapes; code that also handles hand-assembled predicates MUST route through
// [DerefWhere] first, since `*Comparison` satisfies [WhereExpr] too.
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
	// Both operands render through the total [FormatValue], not token()
	// directly, so expr cannot panic regardless of how it is reached — the
	// same treatment leftToken already had. validate refuses a nil/typed-nil
	// Val on every supported path, so the "" case is unreachable there.
	return c.leftToken() + " " + string(c.Op) + " " + FormatValue(c.Val)
}

// leftToken renders the left operand: the structured [Comparison.Left]
// value when present, the raw path otherwise (REQ-117).
//
// It routes through [FormatValue] rather than calling token directly, so it is
// TOTAL over every shape the field can hold. token has a value receiver, which
// means a typed-nil pointer (`(*FuncCall)(nil)`) satisfies Value with a non-nil
// interface and panics when token is called on it. That matters here and not in
// the other emitters because [Comparison.validate] calls leftToken while
// BUILDING AN ERROR — naming the operand an unknown operator was used on — and
// that error is produced before the operand itself has been checked. A refusal
// path that panics on the way to reporting the refusal is the worst version of
// this bug, so the left operand of a value with no wire form renders as "".
func (c Comparison) leftToken() string {
	if c.Left == nil {
		return c.Path
	}
	return FormatValue(c.Left)
}

func (c Comparison) validate() error {
	if !c.Op.known() {
		return fmt.Errorf("%w: unknown comparison operator %q on %q",
			ErrInvalidQuery, string(c.Op), c.leftToken())
	}
	if c.Left == nil && strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("%w: empty path in %s comparison", ErrInvalidQuery, string(c.Op))
	}
	// Path and Left are the two spellings of ONE left operand, so setting
	// both is a caller error, not a precedence question: leftToken would
	// silently discard Path (REQ-117). The sibling MatchesExpr counts its
	// operand forms the same way.
	if c.Left != nil && strings.TrimSpace(c.Path) != "" {
		return fmt.Errorf("%w: comparison sets both Path %q and a structured Left operand",
			ErrInvalidQuery, c.Path)
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

// known reports whether b is one of the two junction operators the grammar
// admits, mirroring [Operator.known] for the predicate side.
//
// BoolOp is a named string so a parsed query keeps its wire spelling, which
// leaves the type open — and unlike Operator, whose zero value at least fails
// loudly, a [Junction] assembled by hand carries the EMPTY BoolOp by default and
// emitted two predicates joined by nothing (REQ-119). A non-canonical casing
// (`"and"`) is refused rather than re-spelled, for the same reason `==` is not
// silently corrected to `=`.
func (b BoolOp) known() bool {
	switch b {
	case OpAnd, OpOr:
		return true
	default:
		return false
	}
}

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
		// Normalised FIRST, because the parenthesisation rule below decides
		// from the term's concrete shape and a `*Junction` denotes a junction
		// exactly as the value shape does. Reading the raw interface emitted a
		// nested OR without its parentheses — text that parses cleanly and asks
		// something else, the silent-substitution class (REQ-119).
		//
		// A term denoting nothing leaves an empty part rather than being
		// dropped, so the text breaks loudly instead of quietly losing an arm.
		// validate refuses one first, so this is unreachable through either
		// emitter.
		term, ok := derefWhere(t)
		if !ok {
			continue
		}
		// Parenthesise a nested OR inside an AND to preserve precedence;
		// a bare comparison or same-operator junction needs no grouping.
		if inner, isJunction := term.(Junction); isJunction && inner.Op == OpOr && j.Op == OpAnd {
			parts[i] = "(" + term.expr() + ")"
			continue
		}
		parts[i] = term.expr()
	}
	return strings.Join(parts, " "+string(j.Op)+" ")
}

func (j Junction) validate() error {
	// The operator is checked exactly as [Comparison] checks its own: the type
	// is an open named string, so the zero value reaches here from a
	// hand-assembled tree and emitted `a  b` — two predicates joined by nothing.
	if !j.Op.known() {
		return fmt.Errorf("%w: unknown junction operator %q", ErrInvalidQuery, string(j.Op))
	}
	// A junction with no terms has no reading, and emits `()` when it sits
	// inside a NOT or another junction. [And] / [Or] collapse the empty case to
	// nil before a Junction is built; the struct literal has no such guard,
	// which is the same constructor-guards-it asymmetry as the terms below.
	if len(j.Terms) == 0 {
		return fmt.Errorf("%w: %s junction with no terms", ErrInvalidQuery, string(j.Op))
	}
	for i, t := range j.Terms {
		// A term denoting no predicate is refused, not skipped: dropping an
		// arm of an AND/OR widens or narrows the result set silently. [And] /
		// [Or] drop an UNTYPED nil before a Junction is built, but they keep a
		// typed one — `Or((*Comparison)(nil), x)` reaches here — so this binds
		// the constructor path as well as a hand-assembled tree.
		term, ok := derefWhere(t)
		if !ok {
			return fmt.Errorf("%w: %s junction term %d carries no predicate", ErrInvalidQuery, string(j.Op), i)
		}
		if err := term.validate(); err != nil {
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
	// Normalised for the same reason [Junction.expr] is: the parenthesisation
	// below decides from the operand's concrete shape, and `NOT *Junction`
	// rendered without parentheses re-binds the negation to one arm.
	operand, ok := derefWhere(n.Operand)
	if !ok {
		// validate refuses this first; a bare `NOT` is the loud form.
		return "NOT"
	}
	// Parenthesise any junction operand so the precedence reads
	// unambiguously regardless of which junctions surround the NOT.
	if _, isJunction := operand.(Junction); isJunction {
		return "NOT (" + operand.expr() + ")"
	}
	return "NOT " + operand.expr()
}

func (n NotExpr) validate() error {
	// Refused rather than emitted as a bare `NOT`: a negation with nothing to
	// negate has no reading. A typed-nil operand denotes the same absence as an
	// untyped one, so both land here instead of panicking in validate.
	operand, ok := derefWhere(n.Operand)
	if !ok {
		return fmt.Errorf("%w: NOT with nil operand", ErrInvalidQuery)
	}
	return operand.validate()
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
//   - Values — the braced value list (`{'active', 'archived'}`). The grammar
//     is `valueListItem : primitive | PARAMETER | terminologyFunction`, which
//     is NARROWER than the general value position: a [PathValue] and any
//     [FuncCall] other than `TERMINOLOGY` have no spelling here and are
//     refused (REQ-119).
//   - Terminology — a BARE `TERMINOLOGY('op','api','params')` operand,
//     with no braces (REQ-117); construct with [MatchesTerminology]. The
//     grammar's third alternative is `terminologyFunction`, so no other call
//     is admitted, whatever the field's [FuncCall] type would allow.
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
		// terminologyFunction` alternative takes no braces. FormatValue, not
		// token(): every leaf emitter renders operands through the total
		// formatter so no reachable or future path can panic here.
		return m.Path + " MATCHES " + FormatValue(m.Terminology)
	case uri != "":
		// Emitted trimmed, consistent with the [MatchesURI] constructor.
		return m.Path + " MATCHES {" + uri + "}"
	}
	parts := make([]string, len(m.Values))
	for i, v := range m.Values {
		// A member denoting nothing renders as an empty part — loudly broken
		// text, never a silently shorter list; validate refuses it first on
		// every supported path.
		parts[i] = FormatValue(v)
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
	if uri := strings.TrimSpace(m.URI); uri != "" {
		return validateURIOperand(uri, m.Path)
	}
	if m.Terminology != nil {
		if err := validateValue(*m.Terminology); err != nil {
			return err
		}
		// `matchesOperand`'s bare alternative is `terminologyFunction`, not the
		// general `functionCall`, so the field's [FuncCall] type is wider than
		// the position. Any other call emits `MATCHES LENGTH(…)`, which the
		// parser rejects (REQ-119).
		if !isTerminologyCall(*m.Terminology) {
			// asciiUpper, not strings.ToUpper: the name is provably ASCII here
			// (validateValue held the alphabet above), but the Unicode fold is
			// the banned idiom, and the safe site should spell the safe form.
			return fmt.Errorf("%w: MATCHES on %q carries a bare %s() operand; the grammar admits only %s() there",
				ErrInvalidQuery, m.Path, asciiUpper(strings.TrimSpace(m.Terminology.Name)), TerminologyFunc)
		}
		return nil
	}
	for i, v := range m.Values {
		if err := validateValueListItem(v, m.Path, i); err != nil {
			return err
		}
	}
	return nil
}

// isTerminologyCall reports whether f is the grammar's `terminologyFunction`.
//
// The name is trimmed as well as folded, matching [FuncCall.token], which emits
// the trimmed spelling — reading the raw field let ` terminology ` past the
// arity gate and then emit a call the parser rejects.
func isTerminologyCall(f FuncCall) bool {
	return strings.EqualFold(strings.TrimSpace(f.Name), TerminologyFunc)
}

// validateValueListItem refuses a braced MATCHES member the grammar's
// `valueListItem : primitive | PARAMETER | terminologyFunction` cannot carry.
//
// The general value position (`terminal`) is WIDER, so delegating to
// validateValue alone admitted a [PathValue] and an arbitrary [FuncCall] —
// `MATCHES {c/y}` and `MATCHES {LENGTH(c/y)}`, both refused by this SDK's own
// parser. It is the same argument the TERMINOLOGY arity guard makes, one rule
// down: where the grammar narrows a position, the shared validator's breadth
// must not become the operand's (REQ-119).
//
// A [BoolValue] is refused even though `primitive` NAMES the BOOLEAN token:
// BOOLEAN is lexically dead — IDENTIFIER is declared first
// (AqlLexer.g4:168 vs :232) and wins the equal-length tie, so `true` always
// lexes as an identifier. A comparison position survives that because
// `terminal` admits `identifiedPath` and the extractor maps the keyword back
// to a [BoolValue]; the braced list has no path alternative, so `{true}` has
// NO reading and `MATCHES {true}` is a syntax error. The same
// declaration-order shadowing drives [reservedNonFuncWords] and the TERM_CODE
// refusal in [validateURIOperand]; this is its third position.
func validateValueListItem(v Value, path string, i int) error {
	inner, ok := derefValue(v)
	if !ok {
		return fmt.Errorf("%w: nil value at index %d in MATCHES on %q", ErrInvalidQuery, i, path)
	}
	if err := validateValue(inner); err != nil {
		return err
	}
	switch inner.(type) {
	case StringValue, IntValue, RealValue, NullValue, ParamValue:
		return nil
	case BoolValue:
		return fmt.Errorf("%w: MATCHES on %q carries a boolean at index %d; the BOOLEAN token is shadowed by "+
			"IDENTIFIER in the SDK grammar profile, so a braced boolean member never lexes — compare with `= true` instead",
			ErrInvalidQuery, path, i)
	}
	if fc, isCall := inner.(FuncCall); isCall && isTerminologyCall(fc) {
		return nil
	}
	return fmt.Errorf("%w: MATCHES on %q carries %T at index %d; the grammar admits a primitive, a parameter or %s() there",
		ErrInvalidQuery, path, inner, i, TerminologyFunc)
}

// validateLikeOperand refuses a LIKE pattern outside `likeOperand : STRING |
// PARAMETER`. Same narrowing argument as [validateValueListItem]: the field is
// a [Value], the position is two shapes wide.
func validateLikeOperand(v Value, path string) error {
	inner, ok := derefValue(v)
	if !ok {
		return fmt.Errorf("%w: nil pattern in LIKE on %q", ErrInvalidQuery, path)
	}
	if err := validateValue(inner); err != nil {
		return err
	}
	switch inner.(type) {
	case StringValue, ParamValue:
		return nil
	}
	return fmt.Errorf("%w: LIKE on %q carries %T; the grammar admits only a string literal or a parameter",
		ErrInvalidQuery, path, inner)
}

// validateURIOperand refuses a `MATCHES {uri}` operand the grammar's URI token
// cannot carry.
//
// The operand is emitted verbatim between braces because the token is
// unquoted, which makes it the one value position with no escaping to hide
// behind: a `}` closes the operand early, so a URI carrying one turns a single
// predicate into a different, still-VALID query — `MATCHES {uri://a} OR c/y
// MATCHES {uri://b}` — that no round-trip, golden, or parser check can catch.
// Refusing at validation is the only place the substitution is visible.
//
// The check is POSITIONAL, following the token's own decomposition (`URI :
// URI_SCHEME ':' URI_HIER_PART ( '?' URI_QUERY )? ( '#' URI_FRAGMENT )?`,
// resources/aql/grammar/active/AqlLexer.g4), because a flat union of the
// delimiter sets is not what the token admits: `%` leads `URI_PCT_ENCODED` and
// so requires two hex digits, `[`/`]` occur only as an `URI_IP_LITERAL` host,
// and `#` separates the fragment once and appears nowhere inside it. A flat
// alphabet accepted all three and emitted AQL this SDK's own parser rejects
// (REQ-119).
//
// It is still not an RFC-3986 parser: `URI_REG_NAME`, `URI_PORT` and
// `URI_PATH_*` are character classes in this grammar, so a structurally odd but
// spellable URI (an empty host, a dotted-quad out of range) reaches the wire,
// where the backend stays the authority — the same division of labour the SDK
// keeps everywhere else (PROBE-021). The property this guard owes is narrower
// and testable: whatever it accepts MUST come back from ParseQuery as one
// MATCHES predicate on the same URI.
func validateURIOperand(uri, path string) error {
	bad := func(what string, detail any) error {
		return fmt.Errorf("%w: MATCHES on %q carries a URI %s (%v)",
			ErrInvalidQuery, path, what, detail)
	}
	// URI_SCHEME : ALPHA_CHAR ( ALPHA_CHAR | DIGIT | '+' | '-' | '.' )* — an
	// absent scheme is the commonest way a caller passes something that is not
	// a URI at all.
	scheme, rest, ok := strings.Cut(uri, ":")
	if !ok || scheme == "" {
		return bad("operand", "no scheme")
	}
	if !asciiLetter(scheme[0]) {
		return bad("whose scheme does not start with a letter", "scheme")
	}
	for i := range len(scheme) {
		if !schemeChar(scheme[i]) {
			// The offset, never the byte: one character of the operand is
			// still URI text, which the refusal MUST NOT reproduce.
			return bad("with an invalid scheme character", fmt.Sprintf("byte %d", i))
		}
	}

	// The fragment comes off first and the query second, so a '?' inside the
	// fragment stays part of it (URI_FRAGMENT admits '?', URI_QUERY does not
	// admit '#').
	hier, frag, hasFrag := strings.Cut(rest, "#")
	hier, query, _ := strings.Cut(hier, "?")
	if hasFrag {
		if strings.Contains(frag, "#") {
			return bad("with a second", "'#' — URI_FRAGMENT admits none")
		}
		if err := checkURIRun(frag, uriQueryByte); err != nil {
			// Every helper diagnosis below is value-free by construction (an
			// offset or a grammar-rule name, never operand bytes), so passing
			// it through keeps the coordinate — which sub-component, where —
			// at zero echo cost. Discarding it for a bare component name made
			// port vs host vs userinfo indistinguishable.
			return bad("whose fragment is unspellable", err)
		}
	}
	if err := checkURIRun(query, uriQueryByte); err != nil {
		return bad("whose query is unspellable", err)
	}

	// URI_HIER_PART : '//' URI_AUTHORITY URI_PATH_ABEMPTY | URI_PATH_ABSOLUTE
	// | URI_PATH_ROOTLESS | URI_PATH_EMPTY. Only the authority form admits a
	// bracketed host, so brackets are refused everywhere else by URI_PCHAR.
	if authority, ok := strings.CutPrefix(hier, "//"); ok {
		// URI_PATH_ABEMPTY starts at the first '/', so everything before it is
		// the authority and the rest stays with the path check below.
		hier = ""
		if slash := strings.IndexByte(authority, '/'); slash >= 0 {
			authority, hier = authority[:slash], authority[slash:]
		}
		if err := checkURIAuthority(authority); err != nil {
			return bad("whose authority is unspellable", err)
		}
	}
	if err := checkURIRun(hier, uriPathByte); err != nil {
		return bad("whose path is unspellable", err)
	}
	// Spellable as a URI is necessary but not sufficient: the operand is lexed
	// on its own between the braces, and TERM_CODE is declared BEFORE URI
	// (AqlLexer.g4:174 vs :179), so on an equal-length match it wins the tie and
	// `matchesOperand` — which admits URI and not TERM_CODE — rejects the token.
	if termCodeShadows(uri) {
		return bad("that lexes as a TERM_CODE rather than a URI", "the operand is <term>::<code>, "+
			"which MATCHES admits only as an archetype-predicate operand")
	}
	return nil
}

// termCodeShadows reports whether the whole text matches `TERM_CODE :
// TERM_CODE_CHAR+ ( '(' TERM_CODE_CHAR+ ')' )? '::' TERM_CODE_CHAR+ ( '|'
// ~[|[\]]+ '|' )?`, in which case the lexer prefers TERM_CODE over URI.
//
// `TERM_CODE_CHAR : NAME_CHAR | '.'` admits no `:` and no `/`, which is why
// `a::/b` and `a:::b` stay URIs while `SNOMED-CT::73211009` does not.
// The rule deliberately handles only TERM_CODE's BARE form. The grammar's two
// optional groups — the parenthesised qualifier (`SNOMED-CT(2026)::…`) and the
// trailing `|display|` name — cannot reach this function: a `(` before the
// first `:` fails [schemeChar] and a `|` is in no URI component alphabet, so
// both spellings are refused as unspellable URIs before the shadow check runs
// (the parity corpus in emit_parity_test.go pins that ordering). Handling them
// here would be dead code, which is worse than absent code — it reads as a
// reachable branch and invites re-review.
func termCodeShadows(s string) bool {
	head, code, ok := strings.Cut(s, "::")
	return ok && allTermCodeChars(head) && allTermCodeChars(code)
}

func allTermCodeChars(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if c := s[i]; !wordChar(c) && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// checkURIAuthority checks `( URI_USERINFO '@' )? URI_HOST ( ':' URI_PORT )?`.
func checkURIAuthority(authority string) error {
	if userinfo, host, ok := strings.Cut(authority, "@"); ok {
		if err := checkURIRun(userinfo, uriUserinfoByte); err != nil {
			return fmt.Errorf("userinfo: %w", err)
		}
		authority = host
	}
	// URI_IP_LITERAL : '[' URI_IPV6_LITERAL ']' is the only bracketed form, and
	// it is the whole host, so anything after ']' can only be `:` port.
	if rest, ok := strings.CutPrefix(authority, "["); ok {
		lit, tail, closed := strings.Cut(rest, "]")
		if !closed {
			return errors.New("'[' opens an IP literal that is never closed")
		}
		if err := checkURIIPv6(lit); err != nil {
			return err
		}
		if tail == "" {
			return nil
		}
		port, isPort := strings.CutPrefix(tail, ":")
		if !isPort {
			return errors.New(`trailing text after IP literal; only ":" port may`)
		}
		return checkURIPort(port)
	}
	// URI_HOST : URI_IPV4_ADDRESS | URI_REG_NAME — both are drawn from the
	// reg-name alphabet, so one check covers them; a trailing `:digits*` is
	// URI_PORT (which the grammar lets be empty).
	if host, port, ok := strings.Cut(authority, ":"); ok {
		if err := checkURIPort(port); err != nil {
			return err
		}
		authority = host
	}
	if err := checkURIRun(authority, uriRegNameByte); err != nil {
		return fmt.Errorf("host: %w", err)
	}
	return nil
}

// checkURIIPv6 checks URI_IPV6_LITERAL : HEX_QUAD (':' HEX_QUAD)* '::' HEX_QUAD
// (':' HEX_QUAD)* — note this grammar REQUIRES the `::` and a full quad on each
// side, so the common abbreviations (`::1`, `2001:db8::1`) are not spellable.
func checkURIIPv6(lit string) error {
	left, right, ok := strings.Cut(lit, "::")
	if !ok {
		return errors.New(`IPv6 literal carries no "::", which URI_IPV6_LITERAL requires`)
	}
	if strings.Contains(right, "::") {
		return errors.New(`IPv6 literal carries more than one "::"`)
	}
	for _, half := range []string{left, right} {
		if half == "" {
			return errors.New(`IPv6 literal omits a group; URI_IPV6_LITERAL requires a quad either side of "::"`)
		}
		for quad := range strings.SplitSeq(half, ":") {
			if len(quad) != 4 {
				return errors.New("IPv6 literal carries a non-4-digit group; URI_IPV6_LITERAL admits only 4-digit hex quads")
			}
			for i := range len(quad) {
				if !hexDigit(quad[i]) {
					return errors.New("IPv6 literal carries a non-hex digit")
				}
			}
		}
	}
	return nil
}

// checkURIPort checks URI_PORT : DIGIT* (empty is admitted by the grammar).
func checkURIPort(port string) error {
	for i := range len(port) {
		if c := port[i]; c < '0' || c > '9' {
			return errors.New("port is not URI_PORT : DIGIT*")
		}
	}
	return nil
}

// checkURIRun walks one URI component, admitting a byte that passes admits or a
// `%HH` triple (URI_PCT_ENCODED). `%` is handled here rather than in the byte
// predicates because it is the one place the token is not a character class.
func checkURIRun(s string, admits func(byte) bool) error {
	for i := 0; i < len(s); {
		if c := s[i]; c == '%' {
			if i+2 >= len(s) || !hexDigit(s[i+1]) || !hexDigit(s[i+2]) {
				return errors.New("not URI_PCT_ENCODED : '%' HEX_DIGIT HEX_DIGIT")
			}
			i += 3
			continue
		} else if !admits(c) {
			// The offset, never the byte — these messages surface through
			// [validateURIOperand]'s diagnostics, which MUST NOT reproduce
			// URI text.
			return fmt.Errorf("byte %d is outside the alphabet of this position", i)
		}
		i++
	}
	return nil
}

// The byte predicates below spell the grammar's URI fragments. `%` is never
// admitted by one — [checkURIRun] owns URI_PCT_ENCODED.
func schemeChar(c byte) bool {
	return asciiLetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("+-.", c) >= 0
}

func hexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// uriUnreservedByte spells URI_UNRESERVED; uriSubDelimByte spells URI_SUB_DELIMS.
func uriUnreservedByte(c byte) bool {
	return asciiLetter(c) || (c >= '0' && c <= '9') || strings.IndexByte("-._~", c) >= 0
}

func uriSubDelimByte(c byte) bool { return strings.IndexByte("!$&'()*+,;=", c) >= 0 }

// uriRegNameByte spells URI_REG_NAME; uriUserinfoByte adds ':'; uriPcharByte
// adds ':' and '@'; uriPathByte adds '/'; uriQueryByte adds '/' and '?' (which
// is also URI_FRAGMENT's set).
func uriRegNameByte(c byte) bool { return uriUnreservedByte(c) || uriSubDelimByte(c) }

func uriUserinfoByte(c byte) bool { return uriRegNameByte(c) || c == ':' }

func uriPcharByte(c byte) bool { return uriUserinfoByte(c) || c == '@' }

func uriPathByte(c byte) bool { return uriPcharByte(c) || c == '/' }

func uriQueryByte(c byte) bool { return uriPathByte(c) || c == '?' }

// Matches constructs a [MatchesExpr] over a braced value list.
func Matches(path string, values ...Value) WhereExpr {
	return MatchesExpr{Path: path, Values: values}
}

// MatchesTerminology constructs the `<path> MATCHES
// TERMINOLOGY(operation, api, params)` predicate — the bare
// terminology-function operand form (REQ-117).
func MatchesTerminology(path, operation, api, params string) WhereExpr {
	// Constructed directly rather than asserting Terminology()'s result back
	// to its concrete shape — the one raw type assertion left in this package
	// would otherwise need an allowlist entry in the dispatch-site tripwire.
	return MatchesExpr{Path: path, Terminology: &FuncCall{
		Name: TerminologyFunc,
		Args: []Value{StringValue{S: operation}, StringValue{S: api}, StringValue{S: params}},
	}}
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
	// FormatValue is total over nil and typed-nil patterns, rendering "" —
	// loudly broken text; validate refuses both first on every supported path.
	return l.Path + " LIKE " + FormatValue(l.Pattern)
}

func (l LikeExpr) validate() error {
	if strings.TrimSpace(l.Path) == "" {
		return fmt.Errorf("%w: empty path in LIKE", ErrInvalidQuery)
	}
	return validateLikeOperand(l.Pattern, l.Path)
}

// Like constructs a [LikeExpr].
func Like(path string, pattern Value) WhereExpr {
	return LikeExpr{Path: path, Pattern: pattern}
}
