package validation

import "testing"

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
