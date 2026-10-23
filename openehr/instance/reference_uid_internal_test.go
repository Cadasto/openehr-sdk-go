package instance

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_UIDSourceReturningNilLeavesAReferenceWithNoID is the REQ-107
// check that a UIDSource that returns nil, where the generator draws the id
// of a reference that has none, leaves the reference with no id: the id is
// nil, not an interface holding a nil pointer, for every reference class
// fillReference writes. The namespace and type are still given. It calls
// fillReference directly on a reference built by hand with no id; through
// Generate, TestREQ107_UIDSourceReturningNilDoesNotPanic checks the id that
// has no value.
func TestREQ107_UIDSourceReturningNilLeavesAReferenceWithNoID(t *testing.T) {
	cases := []struct {
		name string
		ref  any
		// id returns the id the reference holds after the fill.
		id func(ref any) any
	}{
		{name: "OBJECT_REF", ref: &rm.ObjectRef{}, id: func(ref any) any { return ref.(*rm.ObjectRef).ID }},
		{name: "PARTY_REF", ref: &rm.PartyRef{}, id: func(ref any) any { return ref.(*rm.PartyRef).ID }},
		{name: "ACCESS_GROUP_REF", ref: &rm.AccessGroupRef{}, id: func(ref any) any { return ref.(*rm.AccessGroupRef).ID }},
		{name: "LOCATABLE_REF", ref: &rm.LocatableRef{}, id: func(ref any) any { return ref.(*rm.LocatableRef).ID }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &generator{opts: Options{UIDSource: func() *rm.HierObjectID { return nil }}}
			g.fillReference(tc.ref, "workflow_id")
			if got := tc.id(tc.ref); got != nil {
				t.Errorf("fillReference(%s with no id): id = %#v, want nil", tc.name, got)
			}
			ref, ok := tc.ref.(rm.ObjectRefLike)
			if !ok {
				t.Fatalf("%s is not an ObjectRefLike", tc.name)
			}
			if ref.GetNamespace() != referenceNamespace || ref.GetType() == "" {
				t.Errorf("fillReference(%s with no id): namespace %q, type %q, want %q and a type",
					tc.name, ref.GetNamespace(), ref.GetType(), referenceNamespace)
			}
		})
	}
}
