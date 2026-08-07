package aql

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
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
// total, panic-free replacement for `==` on a [Value] (see § Comparability).
//
// Two values are equal when they have the same dynamic type and the same
// canonical wire form. Defining it through the emitted token rather than
// field-by-field keeps one rule for all eight shapes and ties equality to what
// actually reaches the server: values that emit identical AQL are identical to
// the query. Two consequences are worth naming — an [IntValue] never equals
// the numerically equal [RealValue] (they emit `1` and `1.0`, and AQL types
// them differently), and two [PathValue]s agree on their Raw text alone, so a
// parsed path equals the hand-built one that emits the same text even though
// only the parsed one carries decomposed segments.
//
// A nil value equals only another nil value. The type check is reflective
// rather than a type switch so a shape added later (the set is additive) is
// compared correctly the day it lands, instead of silently reporting unequal
// to itself until someone remembers to extend a switch.
func EqualValues(a, b Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	return a.token() == b.token()
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
// produce the same value.
func Param(name string) Value { return ParamValue{Name: strings.TrimPrefix(name, "$")} }

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
func (v StringValue) token() string {
	var sb strings.Builder
	sb.Grow(len(v.S) + 2)
	sb.WriteByte('\'')
	for i := range len(v.S) {
		// Ranges bytes, not runes: every character this switch escapes is
		// ASCII, and a multi-byte rune's continuation bytes are all >= 0x80,
		// so they fall to the default arm and are copied through verbatim.
		switch c := v.S[i]; c {
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
// spellings, which the grammar rejects, so [validateValue] refuses them before
// any supported path can emit one.
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

// token renders `NAME(arg, …)`. It cannot report an error, so it stays
// TOLERANT of a nil argument and skips it; refusing a nil arg is
// [validateValue]'s job, and every public emission path ([Builder.Build],
// [FormatWhere]) validates before emitting — a nil argument never reaches this
// method through a supported entry point.
func (f FuncCall) token() string {
	parts := make([]string, 0, len(f.Args))
	for _, a := range f.Args {
		if a == nil {
			continue
		}
		parts = append(parts, a.token())
	}
	return strings.ToUpper(f.Name) + "(" + strings.Join(parts, ", ") + ")"
}

// Func constructs a [FuncCall] with the given (case-insensitive) name and
// argument list.
func Func(name string, args ...Value) Value {
	return FuncCall{Name: strings.ToUpper(strings.TrimSpace(name)), Args: args}
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

// validateValue reports a structurally unusable [Value] — one whose token
// form would emit syntactically invalid AQL (REQ-117). It complements the
// per-[WhereExpr] validate methods, which own the clause-level rules.
func validateValue(v Value) error {
	switch t := v.(type) {
	case PathValue:
		if strings.TrimSpace(t.Raw) == "" {
			return fmt.Errorf("%w: path value with empty path text", ErrInvalidQuery)
		}
	case RealValue:
		// AQL has no literal for either, and 'f' formatting renders them as
		// the Go spellings (`+Inf`, `NaN`) — text no backend can parse.
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
		if strings.EqualFold(t.Name, TerminologyFunc) {
			if len(t.Args) != 3 {
				return fmt.Errorf("%w: %s() takes exactly 3 string arguments, got %d",
					ErrInvalidQuery, TerminologyFunc, len(t.Args))
			}
			for i, a := range t.Args {
				if _, ok := a.(StringValue); !ok {
					return fmt.Errorf("%w: %s() argument %d is %T; the grammar admits only string literals",
						ErrInvalidQuery, TerminologyFunc, i, a)
				}
			}
		}
		for i, a := range t.Args {
			if a == nil {
				return fmt.Errorf("%w: nil argument at index %d in %s()", ErrInvalidQuery, i, strings.ToUpper(t.Name))
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
// Source: resources/aql/grammar/active/AqlLexer.g4 (§ Keywords, § Operators,
// § aggregate function, BOOLEAN). Held honest against the grammar by
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
	"TRUE": true, "FALSE": true,
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
func validateFuncName(name string) error {
	n := strings.ToUpper(strings.TrimSpace(name))
	if n == "" {
		return fmt.Errorf("%w: function call with empty name", ErrInvalidQuery)
	}
	// IDENTIFIER : ALPHA_CHAR WORD_CHAR* — a letter, then letters/digits/`_`.
	if !asciiLetter(n[0]) {
		return fmt.Errorf("%w: function name %q does not start with a letter", ErrInvalidQuery, name)
	}
	for i := range len(n) {
		if c := n[i]; !wordChar(c) {
			return fmt.Errorf("%w: function name %q carries %q, which no AQL identifier admits", ErrInvalidQuery, name, string(rune(c)))
		}
	}
	if reservedNonFuncWords[n] {
		return fmt.Errorf("%w: %q is a reserved AQL keyword and cannot name a function in a value position", ErrInvalidQuery, n)
	}
	return nil
}
