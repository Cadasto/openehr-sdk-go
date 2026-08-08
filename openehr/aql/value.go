package aql

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Value is a value position in a query — a bound parameter, a literal, an
// identified path, or a function call over those. The interface is sealed;
// construct values with [Param], [String], [Int], [Real], [Bool], [Null],
// [Path], [Func], or [Terminology]. Caller-supplied data MUST flow through
// [Param] (or a literal constructor), never by interpolating into a path
// string — this is the AQL injection guard (REQ-055).
//
// Parsed queries populate the same concrete types ([ParamValue] /
// [StringValue] / [IntValue] / [RealValue] / [BoolValue] / [NullValue] /
// [PathValue] / [FuncCall]) — the read AST and the write AST share one
// vocabulary (REQ-113, REQ-117). Concrete-type fields are intended for
// read access; mutating a value already embedded in a [WhereExpr] passed
// to [FormatWhere] / [Builder.Build] is undefined.
//
// The set grows ADDITIVELY as the structured-AST catalogue closes further
// grammar positions (REQ-117), so a consumer type-switching over it MUST
// treat an unrecognised case as out-of-catalogue — refuse, skip, or
// report — and MUST NOT panic on it.
//
// Every shape has a POINTER twin: token has a value receiver, so `*FuncCall`
// and friends satisfy Value too, and [MatchesExpr.Terminology] is itself a
// `*FuncCall`. A consumer type-switching over Value MUST therefore normalise
// first with [DerefValue] — or, failing that, handle `case *aql.FuncCall:`
// alongside `case aql.FuncCall:` or route through [EqualValues] / the
// validating emitters, which normalise for it. [DerefWhere] is the same tool
// for the [WhereExpr] vocabulary.
//
// # Comparability
//
// A Value is NOT safe to compare with `==`, and MUST NOT be used as a map
// key. [PathValue] (through IdentifiedPath.Segments) and [FuncCall] (through
// Args) carry slices, so `==` on one — or on any [WhereExpr] holding one, such
// as a [Comparison] or [LikeExpr] — panics with "comparing uncomparable type",
// and hashing one panics the same way. Both types became uncomparable in
// v0.18.0 when REQ-117 added the slice fields; unlike the [Containment] change
// released alongside it, this one is invisible to the compiler, which is why
// it is called out here. Use [EqualValues] instead.
type Value interface {
	// token is the canonical wire form: `$name` for a parameter, an escaped
	// literal otherwise.
	token() string
}

// EqualValues reports whether two values occupy the same value position — the
// replacement for `==` on a [Value] (see § Comparability), panic-free over every
// value and pointer shape of the catalogue, including a typed-nil pointer.
//
// Two values are equal when they have the same shape and the same canonical wire
// form. Defining it through the emitted token rather than field-by-field keeps
// one rule for all eight shapes and ties equality to what actually reaches the
// server: values that emit identical AQL are identical to the query. Three
// consequences are worth naming — an [IntValue] never equals the numerically
// equal [RealValue] (they emit `1` and `1.0`, and AQL types them differently);
// two [PathValue]s agree on their Raw text alone, so a parsed path equals the
// hand-built one that emits the same text even though only the parsed one
// carries decomposed segments; and a [FuncCall] whose Args carry a nil element
// equals the same call without it, because [FuncCall.token] skips a nil argument
// (the emitters refuse one outright, so this arises only for a value that never
// passed validation).
//
// A pointer is compared as the shape it points at, so `&FuncCall{…}` equals the
// equivalent `FuncCall{…}`; a nil value — untyped, or a typed-nil pointer —
// equals only another nil value. The shape check is [sameShape]: the type
// comparison must be explicit because tokens alone conflate shapes
// (`PathValue{Raw: "true"}` and `BoolValue{B: true}` both spell `true`).
func EqualValues(a, b Value) bool {
	av, aok := derefValue(a)
	bv, bok := derefValue(b)
	if !aok || !bok {
		return aok == bok
	}
	if !sameShape(av, bv) {
		return false
	}
	return av.token() == bv.token()
}

// sameShape reports whether two values carry the same concrete value shape,
// normalising each first so both carriers of a shape count as that shape.
// Exhaustive over the sealed set; the dispatch tripwire's case-coverage sweep
// fails the build when a shape lands without its row here, which is what lets
// this be a plain switch instead of reflection (the idiom spec bans the
// latter).
func sameShape(a, b Value) bool {
	a, aok := derefValue(a)
	b, bok := derefValue(b)
	if !aok || !bok {
		return false
	}
	switch a.(type) {
	case StringValue:
		_, ok := b.(StringValue)
		return ok
	case IntValue:
		_, ok := b.(IntValue)
		return ok
	case RealValue:
		_, ok := b.(RealValue)
		return ok
	case BoolValue:
		_, ok := b.(BoolValue)
		return ok
	case NullValue:
		_, ok := b.(NullValue)
		return ok
	case ParamValue:
		_, ok := b.(ParamValue)
		return ok
	case PathValue:
		_, ok := b.(PathValue)
		return ok
	case FuncCall:
		_, ok := b.(FuncCall)
		return ok
	}
	return false
}

// derefValue normalises a [Value] to its value shape, reporting false when
// there is nothing to compare or emit — an untyped nil, or a nil pointer.
//
// token has a value receiver, so a pointer to any shape also satisfies Value,
// and calling token on a NIL one panics ("value method … called using nil
// pointer"). [MatchesExpr.Terminology] is a `*FuncCall`, so its zero value is
// exactly that pointer; normalising here keeps every caller — validation,
// equality — total over the shapes the API can hand them.
//
// One pointer level suffices: a value-receiver method set promotes to `*T`
// but never to `**T`, so no deeper indirection can satisfy the interface.
// The switch is exhaustive over the sealed set (plus each shape's pointer
// twin) and the dispatch tripwire's case-coverage sweep fails the build when
// a shape lands without its two cases — which is what lets this be a plain
// switch instead of reflection (the idiom spec bans the latter). An unknown
// shape falls through to false: refused, never waved through as a raw
// pointer.
func derefValue(v Value) (Value, bool) {
	switch x := v.(type) {
	case StringValue, IntValue, RealValue, BoolValue, NullValue, ParamValue, PathValue, FuncCall:
		return x, true
	case *StringValue:
		if x != nil {
			return *x, true
		}
	case *IntValue:
		if x != nil {
			return *x, true
		}
	case *RealValue:
		if x != nil {
			return *x, true
		}
	case *BoolValue:
		if x != nil {
			return *x, true
		}
	case *NullValue:
		if x != nil {
			return *x, true
		}
	case *ParamValue:
		if x != nil {
			return *x, true
		}
	case *PathValue:
		if x != nil {
			return *x, true
		}
	case *FuncCall:
		if x != nil {
			return *x, true
		}
	}
	return nil, false // untyped nil, a typed-nil pointer, or an unlearned shape
}

// ParamValue is a named placeholder. Name is the placeholder identifier
// WITHOUT the leading `$` (e.g. `ehr_id`, not `$ehr_id`); the emitter
// re-attaches the dollar on the wire. Bind via [Builder.Bind] or set
// [Query.Parameters] directly.
type ParamValue struct {
	Name string
}

func (p ParamValue) token() string { return "$" + p.Name }

// Param constructs a [ParamValue] for the named placeholder. A leading
// `$` in name is stripped — `Param("$ehr_id")` and `Param("ehr_id")`
// produce the same value. The name must spell the grammar's PARAMETER token
// (see [ValidateValue]); it is checked at validate time, not here, so the
// diagnostic names the query position the placeholder sits in.
func Param(name string) Value { return ParamValue{Name: strings.TrimPrefix(name, "$")} }

// validateParamName refuses a placeholder the grammar's `PARAMETER : '$'
// IDENTIFIER_CHAR` cannot carry, where `IDENTIFIER_CHAR : ALPHA_CHAR
// WORD_CHAR*` — a leading letter followed by letters, digits and underscores.
//
// This position had no guard at all, though [Param] is what § REQ-055 rule 4
// designates as the channel for caller data. Two failure modes followed, and
// the second is the one that matters: `Param("a b")` emits `$a b`, which the
// parser rejects, while `Param("p AND c/secret = 1")` emits text that parses
// CLEANLY as two predicates instead of one — a silent structure change through
// the very channel the injection guard recommends (REQ-119).
func validateParamName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: parameter with empty name", ErrInvalidQuery)
	}
	for i, r := range name {
		alpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if i == 0 {
			// ALPHA_CHAR leads: `$1x` lexes as `$1` followed by an identifier.
			if !alpha {
				return fmt.Errorf("%w: parameter name %q must start with a letter", ErrInvalidQuery, name)
			}
			continue
		}
		// WORD_CHAR : ALPHANUM_CHAR | '_'
		if !alpha && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("%w: parameter name %q carries %q; the grammar admits letters, digits and underscore",
				ErrInvalidQuery, name, r)
		}
	}
	return nil
}

// StringValue is a string literal. Use [Param] for caller-supplied data;
// reaching for a literal directly is only safe for compile-time constants.
type StringValue struct {
	S string
}

// token quotes the string as a single-quoted AQL literal, backslash-escaping
// the two characters the grammar's STRING token cannot carry raw.
//
// The grammar spells a single-quoted string `SYM_SINGLE_QUOTE ( ESCAPE_SEQ |
// UTF8CHAR | OCTAL_ESC | ~(backslash | quote) )* SYM_SINGLE_QUOTE`
// (resources/aql/grammar/active/AqlLexer.g4), so exactly `\` and `'` must be
// escaped and `ESCAPE_SEQ` (`'\\' ['"?abfnrtv\\*]`) is the only escape form —
// there is no SQL-style doubling in AQL. A doubled quote closes the token
// and reopens a new one, which is why the SQL spelling this method used to emit
// produced text the SDK's own parser rejected.
//
// The C0 controls with a defined escape are emitted in escaped form rather than
// raw so canonical AQL stays single-line and printable; every other character
// (including `"`) is admitted raw by the token and rides through unchanged.
//
// A byte that is not part of a valid UTF-8 sequence is emitted as the grammar's
// `OCTAL_ESC` (`\NNN`). A Go string is a byte string, so one lifted from a
// latin-1 source can carry such a byte; emitted raw it survives the token but
// NOT the round trip, because the lexer decodes its input to runes and yields
// U+FFFD instead — the value came back changed, with no error anywhere
// (REQ-119). Octal is the only spelling the grammar has for an arbitrary byte.
func (v StringValue) token() string {
	var sb strings.Builder
	sb.Grow(len(v.S) + 2)
	sb.WriteByte('\'')
	for i := 0; i < len(v.S); {
		c := v.S[i]
		if c < utf8.RuneSelf {
			// Every character the grammar makes us escape is ASCII, so the
			// switch only ever sees a single-byte rune.
			switch c {
			case '\\':
				sb.WriteString(`\\`)
			case '\'':
				sb.WriteString(`\'`)
			case '\a':
				sb.WriteString(`\a`)
			case '\b':
				sb.WriteString(`\b`)
			case '\f':
				sb.WriteString(`\f`)
			case '\n':
				sb.WriteString(`\n`)
			case '\r':
				sb.WriteString(`\r`)
			case '\t':
				sb.WriteString(`\t`)
			case '\v':
				sb.WriteString(`\v`)
			default:
				sb.WriteByte(c)
			}
			i++
			continue
		}
		// DecodeRuneInString returns (RuneError, 1) for exactly the bytes that
		// begin no valid sequence — a correctly encoded U+FFFD returns size 3
		// and so rides through verbatim like any other rune.
		if r, size := utf8.DecodeRuneInString(v.S[i:]); r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&sb, `\%03o`, c)
			i++
		} else {
			sb.WriteString(v.S[i : i+size])
			i += size
		}
	}
	sb.WriteByte('\'')
	return sb.String()
}

// String constructs a [StringValue]. Prefer [Param] for caller-supplied data.
func String(s string) Value { return StringValue{S: s} }

// IntValue is an integer literal.
type IntValue struct {
	N int64
}

func (v IntValue) token() string { return strconv.FormatInt(v.N, 10) }

// Int constructs an [IntValue].
func Int(n int64) Value { return IntValue{N: n} }

// RealValue is a floating-point literal. The emitter uses decimal ('f')
// notation — never scientific ('g'/'e') — since the latter is not
// universally accepted as an AQL REAL literal by all backends.
type RealValue struct {
	F float64
}

// token uses 'f' (decimal) notation, never 'g'/'e' — scientific notation
// (1e+20) is not universally accepted as an AQL REAL literal, and the typed
// builders must not emit anything a backend could reject syntactically.
//
// A fractional part is appended when 'f' produces none, so the text always
// re-lexes as the grammar's `REAL : DIGIT* '.' DIGIT+` rather than as
// `INTEGER : DIGIT+`. Without it a real is not a fixed point of the
// parse/emit round trip (REQ-117): a whole-valued real silently returns as an
// [IntValue], and a real too large for int64 — 1e20 renders as
// `100000000000000000000` — comes back as an out-of-range INTEGER, so the
// emitted text is refused with [ErrIncompleteAST] on re-parse.
//
// Infinities and NaN have no AQL literal at all; they render as the Go
// spellings, and only `+Inf` / `-Inf` are then rejected. `NaN` and a bare `Inf`
// lex as IDENTIFIER and re-parse as a [PathValue] — a silent type substitution
// rather than a loud refusal, which is why [validateValue] refuses all of them
// before any supported path can emit one.
func (v RealValue) token() string {
	s := strconv.FormatFloat(v.F, 'f', -1, 64)
	if strings.Contains(s, ".") || math.IsInf(v.F, 0) || math.IsNaN(v.F) {
		return s
	}
	return s + ".0"
}

// Real constructs a [RealValue].
func Real(f float64) Value { return RealValue{F: f} }

// BoolValue is a boolean literal.
type BoolValue struct {
	B bool
}

func (v BoolValue) token() string { return strconv.FormatBool(v.B) }

// Bool constructs a [BoolValue].
func Bool(b bool) Value { return BoolValue{B: b} }

// NullValue is the AQL `NULL` literal. The token form is the bare keyword
// (no quoting) — distinguishing it from a [StringValue] carrying "NULL".
type NullValue struct{}

func (NullValue) token() string { return "NULL" }

// Null is the AQL NULL literal as a [Value].
func Null() Value { return NullValue{} }

// PathValue is an identified path in a VALUE position (REQ-117): the right
// operand of a path-vs-path comparison (`WHERE a/x = b/y`) or an argument
// of a [FuncCall]. It embeds the shared [IdentifiedPath], so the read side
// exposes alias + segments without re-splitting the text; Raw is the
// emission source.
//
// The write-side constructor [Path] sets only Raw — decomposing a path
// string is the parser's job, mirroring how [Comparison.ParsedPath] is nil
// on the write side.
type PathValue struct {
	IdentifiedPath
}

func (p PathValue) token() string { return p.Raw }

// Path constructs a [PathValue] for an alias-qualified path used as a
// value (a path-vs-path comparison operand or a function argument). The
// path is emitted VERBATIM — it is an openEHR identifier, never
// caller-supplied data; route caller data through [Param].
func Path(raw string) Value {
	return PathValue{IdentifiedPath: IdentifiedPath{Raw: strings.TrimSpace(raw)}}
}

// FuncCall is an AQL function call in a value position (REQ-117) — either
// operand of a comparison (`LENGTH(o/name/value) > 5`, `o/x = LENGTH(o/y)`),
// a nested function argument, or the `TERMINOLOGY(op, api, params)` operand
// of a MATCHES predicate ([MatchesExpr.Terminology]).
//
// Name is the function name; emission upper-cases it so canonical AQL is
// produced regardless of source casing. Args are the operands in source
// order — any [Value], which is exactly the grammar's `terminal :
// primitive | PARAMETER | identifiedPath | functionCall` set (literal,
// [ParamValue], [PathValue], nested [FuncCall]).
//
// The SELECT side models a projected function call as
// [github.com/cadasto/openehr-sdk-go/openehr/aql/parse.FunctionCall]
// (whose args are SELECT operands); this is its value-position sibling.
type FuncCall struct {
	Name string
	Args []Value
}

// token renders `NAME(arg, …)`. The name is trimmed as well as upper-cased, so
// the emitted spelling is the one [validateFuncName] approved rather than one
// carrying stray space the grammar's skipped WS would hide.
//
// It cannot report an error, so it stays TOLERANT of an argument that has no
// wire form — a nil, or a typed-nil pointer shape — and skips it. Refusing one
// is [ValidateValue]'s job, and the validating emission paths ([Builder.Build],
// [FormatWhere], [github.com/cadasto/openehr-sdk-go/openehr/aql/parse.Query.Emit])
// all call it, so such an argument never reaches this method through them. The
// unvalidated [FormatValue] can, which is why skipping beats panicking.
func (f FuncCall) token() string {
	parts := make([]string, 0, len(f.Args))
	for _, a := range f.Args {
		arg, ok := derefValue(a)
		if !ok {
			continue
		}
		parts = append(parts, arg.token())
	}
	return asciiUpper(strings.TrimSpace(f.Name)) + "(" + strings.Join(parts, ", ") + ")"
}

// Func constructs a [FuncCall] with the given (ASCII case-insensitive) name
// and argument list. The name is canonicalised to upper case at intake — the
// constructor twin of [Param] stripping `$` — but only over ASCII letters: a
// name outside the identifier alphabet is stored as written and refused at
// validate time, never respelled into a legal-looking one.
func Func(name string, args ...Value) Value {
	return FuncCall{Name: asciiUpper(strings.TrimSpace(name)), Args: args}
}

// asciiUpper upper-cases the ASCII letters of s and leaves every other byte
// alone. strings.ToUpper is NOT a substitute anywhere a function name is
// canonicalised or compared: its Unicode mapping folds some non-ASCII letters
// INTO the ASCII alphabet (ı → I, ſ → S), turning a name the lexer cannot
// tokenise into a legal-looking spelling instead of leaving it for the
// alphabet check to refuse.
func asciiUpper(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}, s)
}

// TerminologyFunc is the canonical name of the AQL terminology function.
const TerminologyFunc = "TERMINOLOGY"

// Terminology constructs the AQL `TERMINOLOGY(operation, api, params)`
// function call as a [Value] — the grammar's `terminologyFunction`, usable
// as a comparison operand, a MATCHES value-list item, or (via
// [MatchesTerminology]) a bare MATCHES operand. The three arguments are
// string literals in grammar order.
func Terminology(operation, api, params string) Value {
	return FuncCall{
		Name: TerminologyFunc,
		Args: []Value{StringValue{S: operation}, StringValue{S: api}, StringValue{S: params}},
	}
}

// ValidateValue reports a structurally unusable [Value] — one whose token form
// would emit syntactically invalid AQL (REQ-117, REQ-119). It complements the
// per-[WhereExpr] validate methods, which own the clause-level rules.
//
// [FormatWhere] and [Builder.Build] call it for you. It is exported so a write
// path outside this package — notably
// [github.com/cadasto/openehr-sdk-go/openehr/aql/parse.Query.Emit], which
// carries values in SELECT positions this package does not model — can hold the
// same line, and so a consumer assembling a value by hand can check it before
// handing it to the unvalidated [FormatValue].
func ValidateValue(v Value) error { return validateValue(v) }

// DerefValue normalises a [Value] to the value shape it denotes, reporting
// false when it denotes none — an untyped nil, or a nil pointer.
//
// It is the sanctioned way to answer "which shape is this?" about a Value.
// token has a value receiver, so `*FuncCall` satisfies Value alongside
// `FuncCall`, and a bare type switch that lists only the value shapes silently
// misses every pointer twin — which is reachable without anyone writing `&`,
// since [MatchesExpr.Terminology] is itself a `*FuncCall`. Any code deciding
// behaviour from a Value's concrete type MUST normalise first, whether it lives
// in this package or in a write path outside it, or the same rule will bind one
// carrier and not the other (REQ-119).
//
//	if s, ok := aql.DerefValue(v); ok {
//	    if lit, isString := s.(aql.StringValue); isString { … }
//	}
func DerefValue(v Value) (Value, bool) { return derefValue(v) }

func validateValue(v Value) error {
	// A pointer shape satisfies Value (token has a value receiver), and the
	// switch below matches only value shapes — so without this every `*FuncCall`
	// would fall through to `return nil` and emit unchecked, and a nil one would
	// panic in token(). See [derefValue].
	inner, ok := derefValue(v)
	if !ok {
		// An untyped nil is refused exactly like a typed-nil pointer: the two
		// denote the same absence ([EqualValues] says so), and no value
		// POSITION admits an absent operand — unlike a top-level [WhereExpr],
		// a value slot has no legal empty spelling, so [FormatValue]'s ""
		// would be embedded as invalid AQL. Passing one was the last
		// nil/typed-nil asymmetry on the exported check-then-format recipe
		// this doc blesses. Every in-package caller refuses a nil with its
		// own position-named diagnostic before delegating here, so only the
		// exported [ValidateValue] observes this arm.
		if v == nil {
			return fmt.Errorf("%w: nil value", ErrInvalidQuery)
		}
		return fmt.Errorf("%w: nil %T value", ErrInvalidQuery, v)
	}
	switch t := inner.(type) {
	case PathValue:
		if strings.TrimSpace(t.Raw) == "" {
			return fmt.Errorf("%w: path value with empty path text", ErrInvalidQuery)
		}
	case ParamValue:
		if err := validateParamName(t.Name); err != nil {
			return err
		}
	case RealValue:
		// AQL has no literal for either. 'f' formatting renders them as the Go
		// spellings, and only `+Inf` / `-Inf` are then REJECTED: `NaN` and a
		// bare `Inf` lex as IDENTIFIER and come back as a PathValue, so the
		// worse half of this is a silent substitution, not a loud refusal.
		if math.IsInf(t.F, 0) || math.IsNaN(t.F) {
			return fmt.Errorf("%w: real literal %v has no AQL spelling", ErrInvalidQuery, t.F)
		}
	case FuncCall:
		if err := validateFuncName(t.Name); err != nil {
			return err
		}
		// `terminologyFunction : TERMINOLOGY '(' STRING ',' STRING ',' STRING
		// ')'` is its own grammar rule, not the general functionCall — the
		// arity and the argument types are fixed, so any other shape emits
		// text the parser rejects. [Terminology] always builds it correctly;
		// this catches a hand-assembled FuncCall that borrowed the name.
		if isTerminologyCall(t) {
			if len(t.Args) != 3 {
				return fmt.Errorf("%w: %s() takes exactly 3 string arguments, got %d",
					ErrInvalidQuery, TerminologyFunc, len(t.Args))
			}
			for i, a := range t.Args {
				arg, _ := derefValue(a)
				if _, ok := arg.(StringValue); !ok {
					return fmt.Errorf("%w: %s() argument %d is %T; the grammar admits only string literals",
						ErrInvalidQuery, TerminologyFunc, i, a)
				}
			}
		}
		for i, a := range t.Args {
			if a == nil {
				return fmt.Errorf("%w: nil argument at index %d in %s()", ErrInvalidQuery, i, asciiUpper(t.Name))
			}
			if err := validateValue(a); err != nil {
				return err
			}
		}
	}
	return nil
}

// reservedNonFuncWords are the grammar's keyword tokens that a value-position
// function call MUST NOT be named after.
//
// The grammar's `functionCall` takes `STRING_FUNCTION_ID |
// NUMERIC_FUNCTION_ID | DATE_TIME_FUNCTION_ID | IDENTIFIER` (plus the separate
// `terminologyFunction`), and an ANTLR keyword rule declared before IDENTIFIER
// shadows it — so a name spelled like any keyword below never lexes as the
// IDENTIFIER the rule needs. The aggregates are the trap that motivated this
// check: `COUNT`/`MIN`/`MAX`/`SUM`/`AVG` are real AQL functions, but only in
// `aggregateFunctionCall`, which the grammar admits in SELECT alone. Emitting
// `COUNT(o/y) > 5` produced a WHERE clause the SDK's own parser rejects.
//
// Shadowing depends on DECLARATION ORDER, not on being a keyword: ANTLR breaks
// an equal-length tie in favour of the rule declared first, so only a keyword
// declared BEFORE `IDENTIFIER` (AqlLexer.g4:168) shadows it. `true` / `false`
// are the counter-example and MUST NOT be listed here — `BOOLEAN` is declared
// at :232, after IDENTIFIER, so `TRUE(o/x)` lexes as an IDENTIFIER and the
// grammar admits it as a function name. The read side relies on the same
// ordering (see the BOOLEAN notes in parse/extract_query.go), and listing them
// made `Emit` refuse an AST `ParseQuery` had just produced.
//
// Source: resources/aql/grammar/active/AqlLexer.g4 (§ Keywords, § Operators,
// § aggregate function). Held honest against the grammar by
// TestReservedFuncNamesTrackTheGrammar in openehr/aql/parse, which is the only
// side that may import the generated lexer.
var reservedNonFuncWords = map[string]bool{
	"SELECT": true, "AS": true, "FROM": true, "WHERE": true, "ORDER": true,
	"BY": true, "DESC": true, "DESCENDING": true, "ASC": true, "ASCENDING": true,
	"LIMIT": true, "OFFSET": true, "DISTINCT": true, "VERSION": true,
	"LATEST_VERSION": true, "ALL_VERSIONS": true, "NULL": true, "TOP": true,
	"FORWARD": true, "BACKWARD": true, "CONTAINS": true, "AND": true, "OR": true,
	"NOT": true, "EXISTS": true, "LIKE": true, "MATCHES": true,
	"COUNT": true, "MIN": true, "MAX": true, "SUM": true, "AVG": true,
}

// aggregateFuncWords are the subset of [reservedNonFuncWords] that the grammar
// DOES admit as a function name — but in `aggregateFunctionCall`, which
// `columnExpr` reaches and no value position does (AqlParser.g4). A SELECT-side
// name check must therefore admit them; a value-position one must not.
var aggregateFuncWords = map[string]bool{
	"COUNT": true, "MIN": true, "MAX": true, "SUM": true, "AVG": true,
}

// IsAggregateFunc reports whether name (case-insensitively) is one of the
// grammar's `aggregateFunctionCall` names — COUNT, MIN, MAX, SUM, AVG. These
// are admissible in SELECT alone, and their argument SHAPE is fixed by that
// rule rather than by the general `functionCall`; the parse-side emitter uses
// this to hold a projected aggregate to its own rule (REQ-119) without
// duplicating the name set.
func IsAggregateFunc(name string) bool {
	return aggregateFuncWords[asciiUpper(strings.TrimSpace(name))]
}

// asciiLetter and wordChar spell the grammar's ALPHA_CHAR and WORD_CHAR
// fragments (`[a-zA-Z]` and `ALPHA_CHAR | DIGIT | '_'`). They take a byte, not
// a rune, because an identifier is ASCII by definition — a multi-byte rune's
// bytes are all >= 0x80 and correctly fail both.
func asciiLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func wordChar(c byte) bool {
	return asciiLetter(c) || (c >= '0' && c <= '9') || c == '_'
}

// validateFuncName refuses a [FuncCall] name the grammar cannot lex in a value
// position — one that is empty, not shaped like an IDENTIFIER, or spelled like
// a reserved word that shadows IDENTIFIER.
func validateFuncName(name string) error { return checkFuncName(name, false) }

// ValidateSelectFuncName refuses a name the grammar cannot lex as a projected
// function call. It is [validateFuncName]'s SELECT-side sibling and differs in
// exactly one way: the aggregates are admitted, because `aggregateFunctionCall`
// is reachable from `columnExpr` (REQ-119).
//
// Exported for [github.com/cadasto/openehr-sdk-go/openehr/aql/parse], which
// models a projected call as its own type and cannot reach the unexported form.
func ValidateSelectFuncName(name string) error { return checkFuncName(name, true) }

func checkFuncName(name string, allowAggregates bool) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%w: function call with empty name", ErrInvalidQuery)
	}
	// IDENTIFIER : ALPHA_CHAR WORD_CHAR* — a letter, then letters/digits/`_`.
	// The alphabet runs over the ORIGINAL bytes, not the ToUpper'd copy: Go's
	// Unicode case mapping folds non-ASCII letters INTO the ASCII alphabet
	// (ı → I, ſ → S), so checking the upper-cased text accepted names whose
	// as-written spelling the lexer cannot tokenise — and the SELECT side
	// emits the name as written.
	if !asciiLetter(trimmed[0]) {
		return fmt.Errorf("%w: function name %q does not start with a letter", ErrInvalidQuery, name)
	}
	for i := range len(trimmed) {
		if c := trimmed[i]; !wordChar(c) {
			return fmt.Errorf("%w: function name %q carries %q, which no AQL identifier admits",
				ErrInvalidQuery, name, string([]byte{c}))
		}
	}
	// Case-insensitive positions (reserved words, aggregates) compare on the
	// canonical upper-case form — safe now that the alphabet held above.
	n := strings.ToUpper(trimmed)
	if reservedNonFuncWords[n] && (!allowAggregates || !aggregateFuncWords[n]) {
		where := "in a value position"
		if allowAggregates {
			where = "in a SELECT position"
		}
		return fmt.Errorf("%w: %q is a reserved AQL keyword and cannot name a function %s",
			ErrInvalidQuery, n, where)
	}
	return nil
}
