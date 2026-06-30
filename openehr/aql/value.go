package aql

import (
	"strconv"
	"strings"
)

// Value is a value position in a query — a bound parameter or a literal. The
// interface is sealed; construct values with [Param], [String], [Int], [Real],
// or [Bool]. Caller-supplied data MUST flow through [Param] (or a literal
// constructor), never by interpolating into a path string — this is the AQL
// injection guard (REQ-055).
//
// Parsed queries populate the same concrete types ([ParamValue] /
// [StringValue] / [IntValue] / [RealValue] / [BoolValue]) — the read AST
// and the write AST share one vocabulary (REQ-113 / SDK-GAP-17). Fields
// on concrete values are READ-ONLY: consumers MUST NOT mutate them; the
// emitter relies on stable inputs.
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
