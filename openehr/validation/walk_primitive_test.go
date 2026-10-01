package validation

import "testing"

// REQ-102: STRING belongs to the same closed primitive short-name set
// as DATE and DURATION. A Go string matches that leaf; any other Go
// type does not, so the type check does not treat a string as an RM
// type mismatch and does not accept a non-string as STRING.
func TestREQ102_STRINGPrimitiveMatches(t *testing.T) {
	t.Parallel()
	if !primitiveValueMatchesShortName("STRING", "example") {
		t.Error(`primitiveValueMatchesShortName("STRING", "example") = false, want true`)
	}
	if primitiveValueMatchesShortName("STRING", 1) {
		t.Error(`primitiveValueMatchesShortName("STRING", 1) = true, want false`)
	}
}

// REQ-107: an optional String attribute is a *string in the RM. A set
// pointer matches STRING; a nil pointer is absent and matches nothing; a
// pointer never matches a short name other than STRING.
func TestREQ107_STRINGPrimitiveMatchesOptionalPointer(t *testing.T) {
	t.Parallel()
	s := "example"
	if !primitiveValueMatchesShortName("STRING", &s) {
		t.Error(`primitiveValueMatchesShortName("STRING", &"example") = false, want true`)
	}
	var none *string
	if primitiveValueMatchesShortName("STRING", none) {
		t.Error(`primitiveValueMatchesShortName("STRING", (*string)(nil)) = true, want false`)
	}
	if primitiveValueMatchesShortName("DATE", &s) {
		t.Error(`primitiveValueMatchesShortName("DATE", &"example") = true, want false`)
	}
}

// REQ-107: the primitive validator receives the text an optional String
// attribute holds, not the pointer.
func TestREQ107_PrimitiveInputDereferencesOptionalString(t *testing.T) {
	t.Parallel()
	s := "example"
	if got := primitiveInput(&s); got != "example" {
		t.Errorf("primitiveInput(&%q) = %#v, want the string", s, got)
	}
	var none *string
	if got := primitiveInput(none); got != "" {
		t.Errorf("primitiveInput((*string)(nil)) = %#v, want the empty string", got)
	}
}
