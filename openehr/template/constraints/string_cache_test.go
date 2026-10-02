package constraints

import "testing"

// TestNewCString_PreCompilesValidPattern asserts that NewCString pre-compiles
// a valid pattern into the unexported re field.
func TestNewCString_PreCompilesValidPattern(t *testing.T) {
	c := NewCString("a.*b", nil, "")
	if c.re == nil {
		t.Error("NewCString with valid pattern: re == nil, want non-nil (pre-compiled)")
	}
}

// TestNewCString_InvalidPatternLeftNil asserts that NewCString leaves re nil
// for an invalid pattern so the error surfaces at Validate time, not at
// parse time — preserving the "value-violation vs unparseable-OPT-regex"
// distinction.
func TestNewCString_InvalidPatternLeftNil(t *testing.T) {
	c := NewCString("[", nil, "")
	if c.re != nil {
		t.Error("NewCString with invalid pattern: re != nil, want nil (deferred to Validate)")
	}
}

// TestNewCString_FieldsSet asserts that exported fields are set as expected.
func TestNewCString_FieldsSet(t *testing.T) {
	c := NewCString("a.*b", []string{"axb", "ab"}, "axb")
	if c.Pattern != "a.*b" {
		t.Errorf("Pattern = %q, want a.*b", c.Pattern)
	}
	if len(c.List) != 2 || c.List[0] != "axb" || c.List[1] != "ab" {
		t.Errorf("List = %v, want [axb ab]", c.List)
	}
	if c.Default != "axb" {
		t.Errorf("Default = %q, want axb", c.Default)
	}
}

// TestNewCString_Validate_NoViolationOnMatch asserts that a value matching
// the pattern produces no violations.
func TestNewCString_Validate_NoViolationOnMatch(t *testing.T) {
	c := NewCString("a.*b", nil, "")
	if v := c.Validate("axb"); len(v) != 0 {
		t.Errorf("Validate(axb) = %v, want no violations", v)
	}
}

// TestNewCString_Validate_PatternMismatch asserts that a value not matching
// the pattern produces a CodePatternMismatch violation.
func TestNewCString_Validate_PatternMismatch(t *testing.T) {
	c := NewCString("a.*b", nil, "")
	v := c.Validate("zzz")
	if len(v) != 1 || v[0].Code != CodePatternMismatch {
		t.Errorf("Validate(zzz) = %v, want one CodePatternMismatch", v)
	}
}

// TestNewCString_Validate_BadPatternIsCodeInvalidValue asserts that an
// invalid regex stored in Pattern surfaces as CodeInvalidValue at Validate
// time, not as a panic or silent pass.
func TestNewCString_Validate_BadPatternIsCodeInvalidValue(t *testing.T) {
	c := NewCString("[", nil, "")
	v := c.Validate("anything")
	if len(v) != 1 || v[0].Code != CodeInvalidValue {
		t.Errorf("Validate with bad pattern = %v, want one CodeInvalidValue", v)
	}
}

// TestCString_ZeroValue_LazyFallback asserts that a zero-value CString
// struct literal with a valid Pattern still works via the lazy fallback
// path in Validate (i.e. re is nil but the pattern is compiled locally).
func TestCString_ZeroValue_LazyFallback(t *testing.T) {
	c := CString{Pattern: "a.*b"}
	if v := c.Validate("axb"); len(v) != 0 {
		t.Errorf("zero-value CString{Pattern} Validate(axb) = %v, want no violations", v)
	}
}

// TestREQ103_CString_PatternMatchesWholeString (REQ-103) pins the whole-string
// reading of a C_STRING pattern: a value that contains a match but is not
// matched end to end is refused, on both the pre-compiled path
// (NewCString) and the lazy path (a literal CString).
func TestREQ103_CString_PatternMatchesWholeString(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		value   string
		want    bool // true: Validate returns no violation
	}{
		{name: "match inside the string", pattern: "[0-9]+", value: "abc123def", want: false},
		{name: "match at the start only", pattern: "[0-9]+", value: "123abc", want: false},
		{name: "match at the end only", pattern: "[0-9]+", value: "abc123", want: false},
		{name: "whole string matches", pattern: "[0-9]+", value: "123", want: true},
		{name: "alternation matches whole values", pattern: `km\/h|mi\/h`, value: "km/h", want: true},
		{name: "alternation does not match a substring", pattern: `km\/h|mi\/h`, value: "km/h extra", want: false},
		{name: "alternation is grouped before anchoring", pattern: "a|bc", value: "abc", want: false},
		{name: "pattern already anchored", pattern: "^[0-9]+$", value: "123", want: true},
		{name: "pattern already anchored, substring", pattern: "^[0-9]+$", value: "abc123", want: false},
		{name: "any-string pattern", pattern: ".*", value: "anything at all", want: true},
		{name: "dot does not cross a newline", pattern: ".*", value: "two\nlines", want: false},
		{name: "own dot-all flag crosses a newline", pattern: "(?s).*", value: "two\nlines", want: true},
		{name: "own multi-line flag does not move the anchors", pattern: "(?m)^[0-9]+$", value: "12\n34", want: false},
		{name: "non-empty pattern refuses empty", pattern: ".+", value: "", want: false},
		{name: "unterminated quote matches the whole literal", pattern: `\Qabc`, value: "abc", want: true},
		{name: "unterminated quote does not match a longer string", pattern: `\Qabc`, value: "abcd", want: false},
		{name: "unterminated quote keeps its metacharacters literal", pattern: `\Qa|b.`, value: "a|b.", want: true},
		{name: "unterminated quote does not match an alternative", pattern: `\Qa|b.`, value: "a", want: false},
		{name: "closed quote still matches the whole string", pattern: `\Qa.b\E`, value: "a.b", want: true},
		{name: "own case-fold flag stays scoped to the pattern", pattern: "(?i)abc", value: "ABC", want: true},
	}
	for _, tc := range cases {
		for _, path := range []struct {
			name string
			c    CString
		}{
			{"compiled", NewCString(tc.pattern, nil, "")},
			{"lazy", CString{Pattern: tc.pattern}},
		} {
			t.Run(tc.name+"/"+path.name, func(t *testing.T) {
				got := path.c.Validate(tc.value)
				if tc.want {
					if len(got) != 0 {
						t.Errorf("Validate(%q) with pattern %q = %v, want no violation", tc.value, tc.pattern, got)
					}
					return
				}
				if len(got) != 1 || got[0].Code != CodePatternMismatch {
					t.Errorf("Validate(%q) with pattern %q = %v, want one CodePatternMismatch", tc.value, tc.pattern, got)
				}
			})
		}
	}
}

// TestREQ103_CString_UnbalancedPatternStaysInvalid (REQ-103) asserts that wrapping a
// pattern for whole-string matching does not repair a malformed one: a
// pattern that only parses once wrapped is still CodeInvalidValue, and its
// detail names the pattern as the OPT wrote it.
func TestREQ103_CString_UnbalancedPatternStaysInvalid(t *testing.T) {
	for _, c := range []CString{NewCString("a)(b", nil, ""), {Pattern: "a)(b"}} {
		v := c.Validate("ab")
		if len(v) != 1 || v[0].Code != CodeInvalidValue {
			t.Errorf("Validate(ab) with pattern %q = %v, want one CodeInvalidValue", c.Pattern, v)
		}
		if c.re != nil {
			t.Errorf("pattern %q: re = %v, want nil for a malformed pattern", c.Pattern, c.re)
		}
	}
}
