package instance

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// nilUIDSource is a UIDSource that returns nil for every uid.
func nilUIDSource() *rm.HierObjectID { return nil }

// TestREQ107_UIDSourceReturningNilLeavesALocatableWithNoUID is the REQ-107
// Identity and Exceptions check that a UIDSource that returns nil, where
// the generator gives a locatable its uid, leaves that locatable with no
// uid: GetUID returns nil, not an interface holding a nil pointer, so a
// reader that asks whether the uid is there sees that it is not. It calls
// the two places the generator stamps a uid: applyLocatableIdentity, for
// an ENTRY, a PARTY and any locatable whose OPT requires its uid, and
// fillPartyRelationship. The value the caller supplies is written as given
// (§ Exceptions and known gaps), so the RM floor reports what it reports,
// such as PARTY Uid_mandatory; this pins only that the uid is absent.
func TestREQ107_UIDSourceReturningNilLeavesALocatableWithNoUID(t *testing.T) {
	cases := []struct {
		name string
		// stamp gives v its uid as the generator does.
		stamp func(v rm.Locatable)
		v     rm.Locatable
	}{
		{
			name:  "OBSERVATION through applyLocatableIdentity",
			stamp: func(v rm.Locatable) { applyLocatableIdentity(v, "at0000", "observation", nil, nilUIDSource) },
			v:     &rm.Observation{},
		},
		{
			name:  "PERSON through applyLocatableIdentity",
			stamp: func(v rm.Locatable) { applyLocatableIdentity(v, "at0000", "person", nil, nilUIDSource) },
			v:     &rm.Person{},
		},
		{
			name: "PARTY_RELATIONSHIP through fillPartyRelationship",
			stamp: func(v rm.Locatable) {
				g := &generator{opts: Options{UIDSource: nilUIDSource}}
				g.fillPartyRelationship(nil, v.(*rm.PartyRelationship))
			},
			v: &rm.PartyRelationship{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.stamp(tc.v)
			if uid := tc.v.GetUID(); uid != nil {
				t.Errorf("%s with a UIDSource that returns nil: uid = %#v, want none", tc.name, uid)
			}
		})
	}
}
