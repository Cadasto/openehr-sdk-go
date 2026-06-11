package constraints

import (
	"fmt"
	"regexp"
	"slices"
)

// CString constrains an RM String value (C_STRING). Pattern is an
// optional POSIX-flavoured regex (compiled by [regexp.Compile] at
// validation time); List is an optional closed enumeration. When
// both are set, the value MUST satisfy both.
//
// Default carries the OPT <assumed_value>; empty when omitted.
type CString struct {
	Pattern string
	List    []string
	Default string

	re *regexp.Regexp // compiled Pattern; nil until set by NewCString or compiled lazily in Validate
}

// NewCString builds a CString and pre-compiles pattern so repeated
// Validate calls reuse the compiled regexp instead of recompiling. An
// invalid pattern is not reported here — it is left uncompiled and
// surfaces as CodeInvalidValue on Validate, preserving the
// "value-violation vs unparseable-OPT-regex" distinction. pattern, list,
// and assumed (the C_STRING <assumed_value> default) map to the struct
// fields.
func NewCString(pattern string, list []string, assumed string) CString {
	c := CString{Pattern: pattern, List: list, Default: assumed}
	if pattern != "" {
		if re, err := regexp.Compile(pattern); err == nil {
			c.re = re
		}
	}
	return c
}

func (CString) isPrimitive() {}

// ExampleValue returns a minimal-valid string example. REQ-107.
// First entry of List wins when non-empty so closed enumerations
// produce a member; else the literal "example" — Validate without a
// pattern accepts any string, and pattern-only constraints fall
// outside the bounded-guarantee contract (callers needing pattern
// satisfaction supply their own example).
func (c CString) ExampleValue() any {
	if len(c.List) > 0 {
		return c.List[0]
	}
	return "example"
}

// Validate accepts a Go string. Any other type returns CodeWrongType.
// A malformed Pattern surfaces as CodeInvalidValue so callers can
// distinguish "value violated the constraint" from "the OPT itself
// ships an unparseable regex".
func (c CString) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string, got %T", value)}}
	}
	var out []Violation
	if len(c.List) > 0 && !slices.Contains(c.List, s) {
		out = append(out, Violation{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("%q not in allowed list %v", s, c.List),
		})
	}
	if c.Pattern != "" {
		re := c.re
		var err error
		if re == nil {
			re, err = regexp.Compile(c.Pattern)
		}
		if err != nil {
			out = append(out, Violation{Code: CodeInvalidValue, Detail: fmt.Sprintf("constraint pattern %q is not a valid regex: %v", c.Pattern, err)})
		} else if !re.MatchString(s) {
			out = append(out, Violation{Code: CodePatternMismatch, Detail: fmt.Sprintf("%q does not match pattern %q", s, c.Pattern)})
		}
	}
	return out
}
