package aom14

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestTypeRegistryWired asserts that the generator-emitted
// typereg_gen.go has populated typereg.Default with concrete AOM 1.4
// types. The AOM target shares the SAME typereg.Default with RM —
// _type strings are disjoint between the two models so collisions
// are not possible. This guards against an accidental regression
// where the AOM typereg file is missing or empty (a no-op init).
func TestTypeRegistryWired(t *testing.T) {
	cases := []string{
		"ARCHETYPE",
		"ARCHETYPE_ONTOLOGY",
		"ARCHETYPE_SLOT",
		"ARCHETYPE_INTERNAL_REF",
		"C_COMPLEX_OBJECT",
		"C_PRIMITIVE_OBJECT",
		"C_STRING",
		"C_QUANTITY",
		"ASSERTION",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			ctor, ok := typereg.Default.Lookup(name)
			if !ok {
				t.Fatalf("typereg has no constructor for %q", name)
			}
			v := ctor()
			if v == nil {
				t.Fatalf("ctor for %q returned nil", name)
			}
		})
	}
}
