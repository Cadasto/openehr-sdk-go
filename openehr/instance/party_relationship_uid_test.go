package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The two ids fillPartyRelationship used to stamp on every relationship,
// independent of Options.UIDSource.
const (
	fixedRelationshipID1 = "00000000-0000-0000-0000-000000000001"
	fixedRelationshipID2 = "00000000-0000-0000-0000-000000000002"
)

// TestREQ107_PartyRelationshipIDsFromUIDSource is the REQ-107 check that a
// PARTY_RELATIONSHIP takes its uid, and an empty source or target id, from
// Options.UIDSource. A counting source fails the test when any of those three
// is 00000000-0000-0000-0000-000000000001 or ...0002.
func TestREQ107_PartyRelationshipIDsFromUIDSource(t *testing.T) {
	c := compileOPTText(t, optTemplate("PARTY_RELATIONSHIP"), true)
	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		t.Run(policy.String(), func(t *testing.T) {
			var issued []string
			n := 0
			out, err := instance.Generate(t.Context(), c, instance.Options{
				Policy: policy,
				Now:    defaultsNow,
				UIDSource: func() *rm.HierObjectID {
					n++
					v := fmt.Sprintf("rel-uid-%04d", n)
					issued = append(issued, v)
					return &rm.HierObjectID{Value: v}
				},
			})
			if err != nil {
				t.Fatalf("Generate(%s): %v", policy, err)
			}
			rel := out.(*rm.PartyRelationship)
			if uid := hierIDText(rel.GetUID()); uid != "" && (fixedRelationshipID(uid) || !slices.Contains(issued, uid)) {
				t.Errorf("PARTY_RELATIONSHIP.uid = %q, want a UIDSource value, issued %v", uid, issued)
			}
			checkPartyRefID(t, "source", rel.Source, issued)
			checkPartyRefID(t, "target", rel.Target, issued)
			for _, iss := range validation.ValidateRM(rel).Issues {
				if iss.Severity == validation.Error {
					t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
				}
			}
		})
	}
}

// TestREQ107_PartyRelationshipNilUIDSourceStillMintsIDs is the REQ-107 check
// that a nil UIDSource still mints a source id and a target id, through the
// same fallback other locatables use, rather than the two fixed literals.
func TestREQ107_PartyRelationshipNilUIDSourceStillMintsIDs(t *testing.T) {
	c := compileOPTText(t, optTemplate("PARTY_RELATIONSHIP"), true)
	first := generatedRelationship(t, c)
	second := generatedRelationship(t, c)
	for _, side := range []struct {
		name string
		ref  rm.PartyRef
	}{
		{name: "source", ref: first.Source},
		{name: "target", ref: first.Target},
	} {
		id := hierIDText(side.ref.ID)
		if id == "" || fixedRelationshipID(id) {
			t.Errorf("PARTY_RELATIONSHIP.%s.id = %q, want a minted id", side.name, id)
		}
		if side.ref.Namespace != "local" || side.ref.Type != "PERSON" {
			t.Errorf("PARTY_RELATIONSHIP.%s = %+v, want namespace local and type PERSON", side.name, side.ref)
		}
	}
	if hierIDText(first.Source.ID) == hierIDText(second.Source.ID) {
		t.Errorf("two nil-UIDSource runs shared source id %q", hierIDText(first.Source.ID))
	}
}

func generatedRelationship(t *testing.T, c *templatecompile.Compiled) *rm.PartyRelationship {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, instance.Options{
		Policy: instance.Minimal,
		Now:    defaultsNow,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	rel, ok := out.(*rm.PartyRelationship)
	if !ok {
		t.Fatalf("Generate = %T, want *rm.PartyRelationship", out)
	}
	return rel
}

func checkPartyRefID(t *testing.T, side string, ref rm.PartyRef, issued []string) {
	t.Helper()
	id := hierIDText(ref.ID)
	if id == "" || fixedRelationshipID(id) || !slices.Contains(issued, id) {
		t.Errorf("PARTY_RELATIONSHIP.%s.id = %q, want a UIDSource value, issued %v", side, id, issued)
	}
	if ref.Namespace != "local" || ref.Type != "PERSON" {
		t.Errorf("PARTY_RELATIONSHIP.%s = %+v, want namespace local and type PERSON", side, ref)
	}
}

func fixedRelationshipID(id string) bool {
	return id == fixedRelationshipID1 || id == fixedRelationshipID2
}

func hierIDText(v any) string {
	switch id := v.(type) {
	case *rm.HierObjectID:
		if id == nil {
			return ""
		}
		return id.Value
	case rm.HierObjectID:
		return id.Value
	default:
		return ""
	}
}
