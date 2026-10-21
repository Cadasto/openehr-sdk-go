package instance

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_FillReferenceGivesAnIDItsClassForm is the REQ-107 Identity
// check that fillReference gives an id with no value, held by each class of
// reference, a value in its class's form, and draws a uid from UIDSource only
// where the form takes one: the value of the uid for a HIER_OBJECT_ID, a
// GENERIC_ID, a TEMPLATE_ID and a TERMINOLOGY_ID, that value followed by
// ::example::1 for an OBJECT_VERSION_ID, and openEHR-EHR-CLUSTER.example.v1,
// with no draw, for an ARCHETYPE_ID. A value the id already has stays, and
// takes no draw. It calls fillReference directly on references built by
// hand; through Generate, TestREQ107_EmptyReferenceIDTakesItsClassForm
// checks the same on the template's references.
func TestREQ107_FillReferenceGivesAnIDItsClassForm(t *testing.T) {
	cases := []struct {
		name string
		// ref returns a reference whose id is id, and value returns the
		// value the id holds after the fill.
		ref   func(id rm.ObjectID) any
		id    rm.ObjectID
		value func(ref any) string
		// want is the value after the fill, and draws the number of uids
		// the fill takes from UIDSource.
		want  string
		draws int
	}{
		{name: "OBJECT_REF HIER_OBJECT_ID", ref: objectRef, id: &rm.HierObjectID{}, value: objectRefValue, want: "uid-0001", draws: 1},
		{name: "OBJECT_REF GENERIC_ID", ref: objectRef, id: &rm.GenericID{}, value: objectRefValue, want: "uid-0001", draws: 1},
		{name: "OBJECT_REF TEMPLATE_ID", ref: objectRef, id: &rm.TemplateID{}, value: objectRefValue, want: "uid-0001", draws: 1},
		{name: "OBJECT_REF TERMINOLOGY_ID", ref: objectRef, id: &rm.TerminologyID{}, value: objectRefValue, want: "uid-0001", draws: 1},
		{name: "OBJECT_REF OBJECT_VERSION_ID", ref: objectRef, id: &rm.ObjectVersionID{}, value: objectRefValue, want: "uid-0001::example::1", draws: 1},
		{name: "OBJECT_REF ARCHETYPE_ID", ref: objectRef, id: &rm.ArchetypeID{}, value: objectRefValue, want: "openEHR-EHR-CLUSTER.example.v1"},
		{name: "PARTY_REF OBJECT_VERSION_ID", ref: partyRef, id: &rm.ObjectVersionID{}, value: partyRefValue, want: "uid-0001::example::1", draws: 1},
		{name: "PARTY_REF ARCHETYPE_ID", ref: partyRef, id: &rm.ArchetypeID{}, value: partyRefValue, want: "openEHR-EHR-CLUSTER.example.v1"},
		{name: "LOCATABLE_REF OBJECT_VERSION_ID", ref: locatableRef, id: &rm.ObjectVersionID{}, value: locatableRefValue, want: "uid-0001::example::1", draws: 1},
		{name: "LOCATABLE_REF HIER_OBJECT_ID", ref: locatableRef, id: &rm.HierObjectID{}, value: locatableRefValue, want: "uid-0001", draws: 1},
		{
			name: "OBJECT_REF OBJECT_VERSION_ID the template pins",
			ref:  objectRef, id: &rm.ObjectVersionID{Value: "pinned::system::2"}, value: objectRefValue,
			want: "pinned::system::2",
		},
		{
			name: "OBJECT_REF ARCHETYPE_ID the template pins",
			ref:  objectRef, id: &rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.pinned.v1"}, value: objectRefValue,
			want: "openEHR-EHR-OBSERVATION.pinned.v1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draws := 0
			g := &generator{opts: Options{UIDSource: func() *rm.HierObjectID {
				draws++
				return &rm.HierObjectID{Value: fmt.Sprintf("uid-%04d", draws)}
			}}}
			ref := tc.ref(tc.id)
			g.fillReference(ref, "workflow_id")
			if got := tc.value(ref); got != tc.want {
				t.Errorf("fillReference(%s): id value = %q, want %q", tc.name, got, tc.want)
			}
			if draws != tc.draws {
				t.Errorf("fillReference(%s) drew %d uids from UIDSource, want %d", tc.name, draws, tc.draws)
			}
		})
	}
}

func objectRef(id rm.ObjectID) any { return &rm.ObjectRef{ID: id} }

func partyRef(id rm.ObjectID) any { return &rm.PartyRef{ObjectRef: rm.ObjectRef{ID: id}} }

func locatableRef(id rm.ObjectID) any { return &rm.LocatableRef{ID: id.(rm.UIDBasedID)} }

func objectRefValue(ref any) string { return idValueOf(ref.(*rm.ObjectRef).ID) }

func partyRefValue(ref any) string { return idValueOf(ref.(*rm.PartyRef).ID) }

func locatableRefValue(ref any) string { return idValueOf(ref.(*rm.LocatableRef).ID) }

// idValueOf returns the value of an identifier, or "" for none.
func idValueOf(id any) string {
	if v, ok := stringField(id, "value"); ok {
		return v.get()
	}
	return ""
}
