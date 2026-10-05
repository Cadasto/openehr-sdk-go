package validation_test

// rmfloor_party_ref_test.go: REQ-112 — the floor checks a reference
// (OBJECT_REF and its subtypes) with its one invariant evaluator, which
// reports each missing id, namespace or type once. The walk does not also
// read a reference's members, so a missing part is not reported a second
// time as `required`.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ112_FloorReportsAMissingReferencePartOnce is the REQ-112 check that
// ValidateRM reports exactly one issue for each missing part of an empty
// PARTY_REF: as a root, as a member of an ACTOR's roles, and as a member of
// a FOLDER's items.
func TestREQ112_FloorReportsAMissingReferencePartOnce(t *testing.T) {
	cases := []struct {
		name string
		root any
		at   string // the reference's path
	}{
		{name: "PARTY_REF root", root: &rm.PartyRef{}, at: ""},
		{name: "ACTOR.roles member", root: &rm.Person{
			ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1",
			Name:            rm.DVText{Value: "person"},
			Roles:           []rm.PartyRef{{}},
		}, at: "/roles[0]"},
		{name: "FOLDER.items member", root: &rm.Folder{
			ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
			Name:            rm.DVText{Value: "folder"},
			Items:           []rm.ObjectRefLike{rm.PartyRef{}},
		}, at: "/items[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			for _, part := range []string{"id", "namespace", "type"} {
				path := tc.at + "/" + part
				var at []validation.Issue
				for _, issue := range r.Issues {
					if issue.Path == path || strings.HasPrefix(issue.Path, path+"/") {
						at = append(at, issue)
					}
				}
				if len(at) != 1 || at[0].Code != "rm_invariant" {
					t.Errorf("ValidateRM(%T) issues at %s = %+v, want exactly one rm_invariant", tc.root, path, at)
				}
			}
		})
	}
}
