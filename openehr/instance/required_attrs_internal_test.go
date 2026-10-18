package instance

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestRequiredAttributes_matchesRMInfo checks that the table the generator
// reads the RM-mandatory attributes from answers what
// rminfo.Default.RequiredAttributes answers, in the same order, for every
// class it knows, and nothing for a name it does not know: a generic
// spelling, which rminfo keys by its bare class, or no class at all.
func TestRequiredAttributes_matchesRMInfo(t *testing.T) {
	for _, class := range rminfo.Default.KnownRMTypes() {
		if got, want := requiredAttributes(class), rminfo.Default.RequiredAttributes(class); !slices.Equal(got, want) {
			t.Errorf("requiredAttributes(%q) = %v, want %v", class, got, want)
		}
	}
	for _, name := range []string{"DV_INTERVAL<DV_QUANTITY>", "NOT_A_CLASS", ""} {
		if got := requiredAttributes(name); got != nil {
			t.Errorf("requiredAttributes(%q) = %v, want nil", name, got)
		}
	}
}
