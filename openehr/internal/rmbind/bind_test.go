package rmbind_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-102: STRING belongs to the same closed primitive short-name set
// as DATE and DURATION. A Go string matches that leaf; any other Go
// type does not, so the type check does not treat a string as an RM
// type mismatch and does not accept a non-string as STRING.
func TestREQ102_STRINGPrimitiveMatches(t *testing.T) {
	t.Parallel()
	if !rmbind.PrimitiveValueMatchesShortName("STRING", "example") {
		t.Error(`PrimitiveValueMatchesShortName("STRING", "example") = false, want true`)
	}
	if rmbind.PrimitiveValueMatchesShortName("STRING", 1) {
		t.Error(`PrimitiveValueMatchesShortName("STRING", 1) = true, want false`)
	}
}

// REQ-107: an optional String attribute is a *string in the RM. A set
// pointer matches STRING; a nil pointer is absent and matches nothing; a
// pointer never matches a short name other than STRING.
func TestREQ107_STRINGPrimitiveMatchesOptionalPointer(t *testing.T) {
	t.Parallel()
	s := "example"
	if !rmbind.PrimitiveValueMatchesShortName("STRING", &s) {
		t.Error(`PrimitiveValueMatchesShortName("STRING", &"example") = false, want true`)
	}
	var none *string
	if rmbind.PrimitiveValueMatchesShortName("STRING", none) {
		t.Error(`PrimitiveValueMatchesShortName("STRING", (*string)(nil)) = true, want false`)
	}
	if rmbind.PrimitiveValueMatchesShortName("DATE", &s) {
		t.Error(`PrimitiveValueMatchesShortName("DATE", &"example") = true, want false`)
	}
}

// REQ-102: an optional primitive attribute the RM holds by pointer matches
// the short name of the value it points to, as that value does, and no short
// name the value does not match; a nil pointer is an absent attribute and
// matches nothing.
func TestREQ102_PrimitivePointerMatchesAsItsValue(t *testing.T) {
	t.Parallel()
	flag, integer, integer64, number := true, rm.Integer(2), rm.Integer64(2), rm.Real(0.5)
	tests := []struct {
		short, other string
		set, none    any
	}{
		{short: "BOOLEAN", other: "INTEGER", set: &flag, none: (*bool)(nil)},
		{short: "INTEGER", other: "BOOLEAN", set: &integer, none: (*rm.Integer)(nil)},
		{short: "INTEGER", other: "BOOLEAN", set: &integer64, none: (*rm.Integer64)(nil)},
		{short: "REAL", other: "INTEGER", set: &number, none: (*rm.Real)(nil)},
	}
	for _, tc := range tests {
		if !rmbind.PrimitiveValueMatchesShortName(tc.short, tc.set) {
			t.Errorf("PrimitiveValueMatchesShortName(%q, %T set) = false, want true", tc.short, tc.set)
		}
		if rmbind.PrimitiveValueMatchesShortName(tc.short, tc.none) {
			t.Errorf("PrimitiveValueMatchesShortName(%q, %T nil) = true, want false", tc.short, tc.none)
		}
		if rmbind.PrimitiveValueMatchesShortName(tc.other, tc.set) {
			t.Errorf("PrimitiveValueMatchesShortName(%q, %T set) = true, want false", tc.other, tc.set)
		}
	}
}
