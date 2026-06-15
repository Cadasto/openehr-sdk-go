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
type Value interface {
	// token is the canonical wire form: `$name` for a parameter, an escaped
	// literal otherwise.
	token() string
}

type paramValue struct{ name string }

func (p paramValue) token() string { return "$" + p.name }

// Param is a named placeholder. It emits `$name`; supply the bound value via
// [Builder.Bind] (or set [Query.Parameters] directly).
func Param(name string) Value { return paramValue{name: strings.TrimPrefix(name, "$")} }

type stringValue struct{ s string }

// token quotes the string as an AQL literal, doubling embedded single quotes.
func (v stringValue) token() string { return "'" + strings.ReplaceAll(v.s, "'", "''") + "'" }

// String is a string literal. Prefer [Param] for caller-supplied data.
func String(s string) Value { return stringValue{s: s} }

type intValue struct{ n int64 }

func (v intValue) token() string { return strconv.FormatInt(v.n, 10) }

// Int is an integer literal.
func Int(n int64) Value { return intValue{n: n} }

type realValue struct{ f float64 }

// token uses 'f' (decimal) notation, never 'g'/'e' — scientific notation
// (1e+20) is not universally accepted as an AQL REAL literal, and the typed
// builders must not emit anything a backend could reject syntactically.
func (v realValue) token() string { return strconv.FormatFloat(v.f, 'f', -1, 64) }

// Real is a floating-point literal.
func Real(f float64) Value { return realValue{f: f} }

type boolValue struct{ b bool }

func (v boolValue) token() string { return strconv.FormatBool(v.b) }

// Bool is a boolean literal.
func Bool(b bool) Value { return boolValue{b: b} }
