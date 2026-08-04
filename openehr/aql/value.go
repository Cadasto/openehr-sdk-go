package aql

import (
	"fmt"
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
type Value interface {
	// token is the canonical wire form: `$name` for a parameter, an escaped
	// literal otherwise.
	token() string
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

// token quotes the string as an AQL literal, doubling embedded single quotes.
func (v StringValue) token() string { return "'" + strings.ReplaceAll(v.S, "'", "''") + "'" }

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
func (v RealValue) token() string { return strconv.FormatFloat(v.F, 'f', -1, 64) }

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
	case FuncCall:
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("%w: function call with empty name", ErrInvalidQuery)
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
