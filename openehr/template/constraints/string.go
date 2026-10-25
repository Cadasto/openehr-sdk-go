package constraints

import (
	"fmt"
	"regexp"
	"slices"
)

// CString constrains an RM String value (C_STRING). Pattern is an
// optional regular expression in Go's [regexp] syntax; List is an
// optional enumeration, closed unless ListOpen is set. When both are
// set, the value must satisfy both.
//
// ListOpen carries the AOM 1.4 C_STRING list_open flag: true when List
// constrains the value but is not exhaustive. Validate then accepts a
// value outside List and still checks Pattern. False, the zero value, is
// a closed List; the OPT parser reads an omitted list_open element as
// false.
//
// A Pattern must match the whole string, as if written ^(?:Pattern)$.
// The AOM does not say whether it is a whole-string or a search match;
// the SDK reads it as whole-string, which is how the specification's own
// examples (/.+/ for a non-empty string, /km\/h|mi\/h/ for a list of
// units) read most naturally and how Archie (String.matches) applies it.
// Flags and anchors in the Pattern itself keep their meaning inside that
// group; a multi-line anchored Pattern such as (?m)^[0-9]+$ no longer
// matches a value like "12\n34", which the search reading accepted.
//
// Default carries the OPT <assumed_value>; empty when omitted.
type CString struct {
	Pattern  string
	List     []string
	ListOpen bool
	Default  string

	re *regexp.Regexp // compiled whole-string form of Pattern; nil until set by NewCString or compiled lazily in Validate
}

// compileWhole compiles pattern so that it must match the whole string.
// The group keeps an alternation (a|b) inside the anchors and keeps any
// flag the pattern sets (such as (?s)) scoped to the pattern. The pattern
// is compiled on its own first: wrapped, an unbalanced one such as a)(b
// would parse, and the error for a malformed one would show the wrapper.
//
// A pattern may end inside an open \Q quote (\Qabc); the quote then runs
// to the end of the pattern, and a plain wrapper would swallow its closing
// group into the quote. Because the pattern compiled on its own, the plain
// wrapper fails only for that reason, so the wrapper is retried with the
// quote closed (\E) before the group.
func compileWhole(pattern string) (*regexp.Regexp, error) {
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, err
	}
	re, err := regexp.Compile(`^(?:` + pattern + `)$`)
	if err != nil {
		if closed, errClosed := regexp.Compile(`^(?:` + pattern + `\E)$`); errClosed == nil {
			return closed, nil
		}
	}
	return re, err
}

// NewCString builds a CString and pre-compiles pattern so repeated
// Validate calls reuse the compiled regexp instead of recompiling. An
// invalid pattern is not reported here. It is left uncompiled and
// surfaces as CodeInvalidValue on Validate, which keeps a value
// violation distinguishable from an unparseable OPT regex. pattern,
// list, and assumed (the C_STRING <assumed_value> default) map to the
// struct fields. The result has a closed list; set ListOpen on it to open
// the list.
func NewCString(pattern string, list []string, assumed string) CString {
	c := CString{Pattern: pattern, List: list, Default: assumed}
	if pattern != "" {
		if re, err := compileWhole(pattern); err == nil {
			c.re = re
		}
	}
	return c
}

func (CString) isPrimitive() {}

// wholePattern returns Pattern compiled for a whole-string match, reusing
// the regexp NewCString compiled when there is one.
func (c CString) wholePattern() (*regexp.Regexp, error) {
	if c.re != nil {
		return c.re, nil
	}
	return compileWhole(c.Pattern)
}

// ExampleValue returns a minimal-valid string example. With a List, open
// or closed, it returns the first entry, or, when Pattern is set, the
// first entry Pattern matches as a whole string, as Validate matches it,
// so a closed enumeration produces a member Validate accepts. When Pattern
// matches no entry, or is malformed, it returns the first entry, which
// Validate then rejects: a closed List admits no value at all, and an open
// one admits only strings outside it. Without a List it returns the
// literal "example". Validate without a pattern accepts any string.
// Pattern-only constraints are not covered by the bounded-constraint
// guarantee, so callers that need a value satisfying the pattern supply
// their own example.
func (c CString) ExampleValue() any {
	if len(c.List) == 0 {
		return "example"
	}
	if c.Pattern != "" {
		if re, err := c.wholePattern(); err == nil {
			for _, s := range c.List {
				if re.MatchString(s) {
					return s
				}
			}
		}
	}
	return c.List[0]
}

// Validate accepts a Go string. Any other type returns CodeWrongType.
// A value outside a closed List is a CodeNotInList; an open List
// (ListOpen) reports none.
// A Pattern must match the whole string, so a value that only contains a
// match is a CodePatternMismatch. A malformed Pattern surfaces as CodeInvalidValue so callers can
// distinguish "value violated the constraint" from "the OPT itself
// ships an unparseable regex".
func (c CString) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string, got %T", value)}}
	}
	var out []Violation
	if len(c.List) > 0 && !c.ListOpen && !slices.Contains(c.List, s) {
		out = append(out, Violation{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("value not in allowed list %v", c.List),
			Value:  Redact(value),
		})
	}
	if c.Pattern != "" {
		re, err := c.wholePattern()
		if err != nil {
			out = append(out, Violation{Code: CodeInvalidValue, Detail: fmt.Sprintf("constraint pattern %q is not a valid regex: %v", c.Pattern, err)})
		} else if !re.MatchString(s) {
			out = append(out, Violation{
				Code:   CodePatternMismatch,
				Detail: fmt.Sprintf("value does not match pattern %q", c.Pattern),
				Value:  Redact(value),
			})
		}
	}
	return out
}
