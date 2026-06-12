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
