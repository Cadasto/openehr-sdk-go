package rmread_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// TestREQ102_ReadObjectRefs is the REQ-102 check that ReadSingle reads the
// parts of an OBJECT_REF, a LOCATABLE_REF and an ACCESS_GROUP_REF, so a
// template that binds a reference in a list such as FOLDER.items finds the
// id, namespace and type the RM makes mandatory. Each part reads as
// present when set and as absent when empty or nil, in the pointer and the
// value form. A LOCATABLE_REF's id is its own UID_BASED_ID field, and its
// optional path reads as present only when set.
func TestREQ102_ReadObjectRefs(t *testing.T) {
	id := &rm.HierObjectID{Value: "6f1d3a52-9c0e-4b7a-8f21-3d5e7a9b1c04"}
	objectRef := rm.ObjectRef{ID: id, Namespace: "local", Type: "COMPOSITION"}
	accessGroupRef := rm.AccessGroupRef{ID: id, Namespace: "local", Type: "ACCESS_GROUP"}
	// The embedded OBJECT_REF id stays nil: a reader that took it instead
	// of the LOCATABLE_REF's own id would read id as absent.
	locatableRef := rm.LocatableRef{ID: id, Namespace: "local", Type: "COMPOSITION", Path: new("/content[at0001]")}

	tests := []struct {
		name  string
		full  []any
		empty []any
		attrs map[string]any
	}{
		{
			name:  "OBJECT_REF",
			full:  []any{objectRef, &objectRef},
			empty: []any{rm.ObjectRef{}, &rm.ObjectRef{}},
			attrs: map[string]any{"id": id, "namespace": "local", "type": "COMPOSITION"},
		},
		{
			name:  "ACCESS_GROUP_REF",
			full:  []any{accessGroupRef, &accessGroupRef},
			empty: []any{rm.AccessGroupRef{}, &rm.AccessGroupRef{}},
			attrs: map[string]any{"id": id, "namespace": "local", "type": "ACCESS_GROUP"},
		},
		{
			name:  "LOCATABLE_REF",
			full:  []any{locatableRef, &locatableRef},
			empty: []any{rm.LocatableRef{}, &rm.LocatableRef{}},
			attrs: map[string]any{"id": id, "namespace": "local", "type": "COMPOSITION", "path": locatableRef.Path},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for attr, want := range tc.attrs {
				for _, v := range tc.full {
					got, ok := rmread.ReadSingle(v, tc.name, attr)
					if !ok || got != want {
						t.Errorf("ReadSingle(%T, %s) = %v, %v, want %v, true", v, attr, got, ok, want)
					}
				}
				for _, v := range tc.empty {
					if got, ok := rmread.ReadSingle(v, tc.name, attr); ok {
						t.Errorf("ReadSingle(empty %T, %s) = %v, true, want ok=false", v, attr, got)
					}
				}
			}
			for _, v := range tc.full {
				if got, ok := rmread.ReadSingle(v, tc.name, "no_such_attr"); ok {
					t.Errorf("ReadSingle(%T, no_such_attr) = %v, true, want ok=false", v, got)
				}
			}
		})
	}
}
