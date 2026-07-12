package rm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Compile-time pins for the generated LOCATABLE identity surface
// (ADR 0013): the widened Locatable is satisfied by BOTH value and
// pointer forms (value-receiver getters — builders yield values, JSON
// decoding yields pointers), while MutableLocatable is satisfied by
// pointers ONLY (pointer-receiver setters). Generic LOCATABLE types
// participate like any other concrete.
var (
	_ rm.Locatable = rm.Composition{}
	_ rm.Locatable = (*rm.Observation)(nil)
	_ rm.Locatable = rm.History[rm.ItemStructure]{}
	_ rm.Locatable = (*rm.PointEvent[rm.ItemStructure])(nil)

	_ rm.MutableLocatable = (*rm.Composition)(nil)
	_ rm.MutableLocatable = (*rm.Section)(nil)
	_ rm.MutableLocatable = (*rm.IntervalEvent[rm.ItemStructure])(nil)
)

// TestLocatableIdentityAccessors pins the getter contract: each
// accessor returns the field's actual declared type, verbatim —
// including a DV_CODED_TEXT name carried through the DVTextLike
// interface, and nil UID/ArchetypeDetails on a partially-built node.
func TestLocatableIdentityAccessors(t *testing.T) {
	c := rm.Composition{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVCodedText{DVText: rm.DVText{Value: "coded name"}},
	}
	var l rm.Locatable = c
	if got := l.GetArchetypeNodeID(); got != "at0001" {
		t.Errorf("GetArchetypeNodeID = %q, want %q", got, "at0001")
	}
	if got := l.GetName().GetValue(); got != "coded name" {
		t.Errorf("GetName().GetValue() = %q, want %q (DV_CODED_TEXT must survive)", got, "coded name")
	}
	if _, ok := l.GetName().(rm.DVCodedText); !ok {
		t.Errorf("GetName() lost the concrete DV_CODED_TEXT: %T", l.GetName())
	}
	if got := l.GetUID(); got != nil {
		t.Errorf("GetUID on unset field = %v, want nil", got)
	}
	if got := l.GetArchetypeDetails(); got != nil {
		t.Errorf("GetArchetypeDetails on unset field = %v, want nil", got)
	}

	// Pointer form reads identically.
	var lp rm.Locatable = &c
	if got := lp.GetArchetypeNodeID(); got != "at0001" {
		t.Errorf("pointer-form GetArchetypeNodeID = %q, want %q", got, "at0001")
	}
}

// TestMutableLocatableSetters pins the setter contract, including the
// interface-typed SetUID(UIDBasedID) and the GetUID read-back that
// applyLocatableIdentity's set-only-if-unset flow depends on.
func TestMutableLocatableSetters(t *testing.T) {
	var obs rm.Observation
	var m rm.MutableLocatable = &obs

	m.SetArchetypeNodeID("at0002")
	m.SetName(rm.DVText{Value: "renamed"})
	m.SetUID(&rm.HierObjectID{Value: "8fbf9dfc-0000-0000-0000-000000000000"})
	m.SetArchetypeDetails(&rm.Archetyped{})

	if obs.ArchetypeNodeID != "at0002" {
		t.Errorf("SetArchetypeNodeID: field = %q", obs.ArchetypeNodeID)
	}
	if obs.Name.GetValue() != "renamed" {
		t.Errorf("SetName: field value = %q", obs.Name.GetValue())
	}
	if obs.UID == nil || obs.ArchetypeDetails == nil {
		t.Errorf("SetUID/SetArchetypeDetails did not write the fields: %v %v", obs.UID, obs.ArchetypeDetails)
	}
	// Read-back through the getter (set-only-if-unset flows).
	if got := any(obs).(rm.Locatable).GetUID(); got == nil {
		t.Error("GetUID after SetUID = nil")
	}
}

// TestRMTypeName pins the generated reverse registry: Go concrete →
// bare BMM class name (the exact inverse of typereg registration —
// generic instantiations all map to the unparameterised class name),
// with typed-nil pointers reporting ("", false).
func TestRMTypeName(t *testing.T) {
	tests := []struct {
		name   string
		v      any
		want   string
		wantOK bool
	}{
		{"value form", rm.Composition{}, "COMPOSITION", true},
		{"pointer form", &rm.Observation{}, "OBSERVATION", true},
		{"data value", rm.DVQuantity{}, "DV_QUANTITY", true},
		{"generic bound instantiation", rm.History[rm.ItemStructure]{}, "HISTORY", true},
		{"generic pointer", &rm.PointEvent[rm.ItemStructure]{}, "POINT_EVENT", true},
		{"interval default form", rm.DVInterval[rm.DVOrdered]{}, "DV_INTERVAL", true},
		{"interval concrete instantiation", rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL", true},
		{"typed-nil pointer", (*rm.Composition)(nil), "", false},
		{"bare nil", nil, "", false},
		{"non-RM value", struct{}{}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rm.RMTypeName(tc.v)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("RMTypeName(%T) = (%q, %v), want (%q, %v)", tc.v, got, ok, tc.want, tc.wantOK)
			}
		})
	}

	// Registry round-trip parity: for registered names, the constructor's
	// product must reverse-map to the exact registration name.
	for _, name := range []string{
		"COMPOSITION", "SECTION", "OBSERVATION", "EVALUATION", "INSTRUCTION",
		"ACTION", "CLUSTER", "ELEMENT", "HISTORY", "POINT_EVENT",
		"INTERVAL_EVENT", "DV_INTERVAL", "DV_QUANTITY", "DV_CODED_TEXT",
		"FOLDER", "EHR_STATUS", "PERSON", "ORGANISATION",
	} {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Fatalf("registry has no %q", name)
		}
		if got, ok := rm.RMTypeName(ctor()); !ok || got != name {
			t.Errorf("round-trip %s: RMTypeName = (%q, %v)", name, got, ok)
		}
	}
}

// TestIsTypedNil pins the generated nil predicate that the consumers'
// guard-before-read ordering delegates to: true ONLY for an interface
// carrying a typed-nil RM pointer.
func TestIsTypedNil(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want bool
	}{
		{"typed-nil locatable", (*rm.Composition)(nil), true},
		{"typed-nil data value", (*rm.DVQuantity)(nil), true},
		{"typed-nil generic", (*rm.History[rm.ItemStructure])(nil), true},
		{"live pointer", &rm.Composition{}, false},
		{"value form", rm.Composition{}, false},
		{"bare nil interface", nil, false},
		{"non-RM value", struct{}{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rm.IsTypedNil(tc.v); got != tc.want {
				t.Errorf("IsTypedNil(%T) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}
