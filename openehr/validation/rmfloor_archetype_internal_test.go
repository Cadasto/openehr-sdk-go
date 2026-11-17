package validation

// rmfloor_archetype_internal_test.go: REQ-112 Archetype roots — how the floor
// tells an archetype id from an at-code in archetype_node_id.

import "testing"

// archetypeIDSink keeps the compiler from dropping the call AllocsPerRun
// measures.
var archetypeIDSink bool

// REQ-112 Archetype roots — isArchetypeID reads an archetype_node_id as an
// archetype id when it parses as an ARCHETYPE_ID, and an at-code, an
// id-code, or an id that only looks like an archetype id, as none. An
// at-code allocates nothing: the floor asks on every LOCATABLE the walk
// visits, and the parse builds an error for every id it refuses. The test is
// not parallel, since AllocsPerRun refuses to run in a parallel test.
func TestIsArchetypeID_REQ112(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want bool
	}{
		{id: "openEHR-EHR-OBSERVATION.blood_pressure.v1", want: true},
		{id: "openEHR-EHR-OBSERVATION.lab_test-blood_glucose.v2", want: true},
		{id: "openEHR-DEMOGRAPHIC-PERSON.person.v1", want: true},
		{id: "at0001"},
		{id: "at0000.1"},
		{id: "id1.1.2"},
		{id: ""},
		{id: "openEHR-EHR-FOLDER.episode"},
		{id: "EHR-FOLDER.episode.v1"},
		{id: "openEHR-EHR-FOLDER.episode.1"},
	} {
		if got := isArchetypeID(tc.id); got != tc.want {
			t.Errorf("isArchetypeID(%q) = %t, want %t", tc.id, got, tc.want)
		}
	}
	for _, id := range []string{"at0001", "at0000.1", "id1.1.2"} {
		if got := testing.AllocsPerRun(100, func() { archetypeIDSink = isArchetypeID(id) }); got != 0 {
			t.Errorf("isArchetypeID(%q) allocates %.0f objects per call, want none: an at-code must not reach the parse", id, got)
		}
	}
}
