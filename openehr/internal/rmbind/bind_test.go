package rmbind_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
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
