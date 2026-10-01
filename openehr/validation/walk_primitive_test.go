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
