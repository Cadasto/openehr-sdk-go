// Package wireequiv is a JSON wire-equivalence oracle for the canonical-JSON
// round-trip tests and PROBE-030 (REQ-052).
//
// Two documents are wire-equivalent when they parse into the same generic JSON
// value. An object becomes a map keyed by member name, so member order is
// ignored; an array becomes a slice, so element order is compared; a number is
// kept as its literal text, so a value past 2^53 is compared exactly rather
// than through a lossy float64; strings, booleans and null compare by value,
// with string-escaping differences erased by the parse ("<" equals "<").
// This is the wire-equivalence model the amended REQ-052 allows only as a
// secondary check: JSON member order carries no meaning (RFC 8259 section 4)
// and the encoder makes no byte-level promise.
//
// The oracle decodes with the v1 encoding/json package on purpose, and stays on
// v1 whatever codec the code under test uses. It is the independent reference
// the canonical-JSON codec is measured against, so it must not share that
// codec's implementation: were both sides to run on encoding/json/v2, a shared
// defect could pass unnoticed. v1's decode into a generic value with
// Decoder.UseNumber is a fixed, second reader of the same wire bytes.
//
// Two model limits follow from that v1 decode, and neither is in scope for this
// oracle: a duplicate object member collapses to the last value (v1 keeps the
// last), so it cannot witness REQ-052's rule that the codec refuse duplicate
// names, and only the first JSON value in each document is read, so trailing
// data after it is ignored. Both are the codec's own tests to make.
package wireequiv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Equivalent reports whether documents a and b are wire-equivalent, as defined
// in the package comment. When they are not equivalent, the second result names
// the first differing location as an RFC 6901 JSON Pointer with the two values;
// when they are equivalent it is the empty string. A document that does not
// parse as JSON is reported through the second result, never as a panic.
func Equivalent(a, b []byte) (bool, string) {
	va, err := decode(a)
	if err != nil {
		return false, "left document is not valid JSON: " + err.Error()
	}
	vb, err := decode(b)
	if err != nil {
		return false, "right document is not valid JSON: " + err.Error()
	}
	if reflect.DeepEqual(va, vb) {
		return true, ""
	}
	return false, firstDiff(va, vb, "")
}

// decode parses one document into a generic JSON value, keeping every number as
// its literal text (json.Number) so an integer past 2^53 is not rounded to the
// nearest float64 on the way in, which would mask exactly the precision the
// codec promises.
func decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// firstDiff returns the RFC 6901 JSON Pointer of the first place a and b differ,
// with a short description of the two values there. Members are walked in sorted
// order so the reported location is deterministic. firstDiff is only ever called
// on values that reflect.DeepEqual has already judged unequal, so it always
// finds and reports a difference.
func firstDiff(a, b any, ptr string) string {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return describe(ptr, a, b)
		}
		for _, k := range sortedUnionKeys(av, bv) {
			ae, aok := av[k]
			be, bok := bv[k]
			child := ptr + "/" + escapeToken(k)
			if !aok || !bok {
				return child + ": present on one side only"
			}
			if !reflect.DeepEqual(ae, be) {
				return firstDiff(ae, be, child)
			}
		}
		return describe(ptr, a, b)
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return describe(ptr, a, b)
		}
		if len(av) != len(bv) {
			return fmt.Sprintf("%s: array lengths differ (%d versus %d)", pointerOrRoot(ptr), len(av), len(bv))
		}
		for i := range av {
			child := ptr + "/" + strconv.Itoa(i)
			if !reflect.DeepEqual(av[i], bv[i]) {
				return firstDiff(av[i], bv[i], child)
			}
		}
		return describe(ptr, a, b)
	default:
		return describe(ptr, a, b)
	}
}

// describe formats a leaf or type mismatch at ptr with both rendered values.
func describe(ptr string, a, b any) string {
	return fmt.Sprintf("%s: %s versus %s", pointerOrRoot(ptr), render(a), render(b))
}

// pointerOrRoot renders the empty pointer (the whole document) readably.
func pointerOrRoot(ptr string) string {
	if ptr == "" {
		return "(document root)"
	}
	return ptr
}

// render gives a compact, typed rendering of one decoded JSON value.
func render(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return "null"
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// sortedUnionKeys returns the union of a's and b's keys in sorted order.
func sortedUnionKeys(a, b map[string]any) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}

// escapeToken escapes one member name as an RFC 6901 reference token: "~"
// becomes "~0" and "/" becomes "~1", done in a single pass so the inserted
// digits are not re-escaped.
func escapeToken(name string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}
