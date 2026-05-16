package rm

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestTypeRegistryWired asserts that the generator-emitted
// typereg_gen.go has populated typereg.Default with concrete RM
// types. This protects against accidental regression where the
// typereg file is empty (a no-op init) — which would silently break
// every polymorphic decode site.
func TestTypeRegistryWired(t *testing.T) {
	cases := []string{
		"DV_QUANTITY",
		"DV_TEXT",
		"DV_CODED_TEXT",
		"COMPOSITION",
		"OBSERVATION",
		"EVALUATION",
		"EHR_STATUS",
		"OBJECT_VERSION_ID",
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
