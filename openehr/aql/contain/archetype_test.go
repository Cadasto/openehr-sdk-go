package contain_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
)

// TestArchetypeMatches covers REQ-160 § Archetype/class conformance: the HRID
// type segment must conform to the declared class, with UnknownClass on an
// unparseable HRID or a class/segment the relation does not know (a mismatch is
// only ever asserted between two known classes).
func TestArchetypeMatches(t *testing.T) {
	r := contain.Default()
	cases := []struct {
		name   string
		rmType string
		hrid   string
		want   contain.Verdict
	}{
		{"entity conforms to declared", "ENTRY", "openEHR-EHR-OBSERVATION.blood_pressure.v1", contain.Admissible},
		{"entity equals declared", "OBSERVATION", "openEHR-EHR-OBSERVATION.blood_pressure.v1", contain.Admissible},
		{"genuine mismatch", "EVALUATION", "openEHR-EHR-OBSERVATION.blood_pressure.v1", contain.Never},
		{"unknown type segment", "ENTRY", "openEHR-EHR-FOOTYPE.x.v1", contain.UnknownClass},
		{"unparseable hrid", "ENTRY", "not-a-valid-archetype-id", contain.UnknownClass},
		{"unknown declared class", "FOO_BAR", "openEHR-EHR-OBSERVATION.x.v1", contain.UnknownClass},
		{"case-insensitive declared", "entry", "openEHR-EHR-OBSERVATION.x.v1", contain.Admissible},
	}
	for _, c := range cases {
		if got := r.ArchetypeMatches(c.rmType, c.hrid); got != c.want {
			t.Errorf("%s: ArchetypeMatches(%q, %q) = %v, want %v", c.name, c.rmType, c.hrid, got, c.want)
		}
	}
}
