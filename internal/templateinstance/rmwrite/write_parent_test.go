package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-107: EnsureSingle addresses the three RM attributes the
// synthesiser attaches and the previous closed switch refused.
// The child values are the concrete types newRMForOPTType returns
// for the vendored OPTs (pointer DV_CODED_TEXT, pointer ITEM_TREE).
func TestREQ107_EnsureSingleRefusedParents(t *testing.T) {
	t.Parallel()

	t.Run("IsmTransition current_state", func(t *testing.T) {
		t.Parallel()
		parent := &rm.IsmTransition{}
		child := &rm.DVCodedText{
			Value: "planned",
			DefiningCode: rm.CodePhrase{
				CodeString:    "524",
				TerminologyID: rm.TerminologyID{Value: "openehr"},
			},
		}
		if err := EnsureSingle(parent, "current_state", child); err != nil {
			t.Fatalf("EnsureSingle(current_state) = %v", err)
		}
		if parent.CurrentState.Value != "planned" || parent.CurrentState.DefiningCode.CodeString != "524" {
			t.Errorf("CurrentState = %+v, want planned / 524", parent.CurrentState)
		}
	})

	t.Run("Person details", func(t *testing.T) {
		t.Parallel()
		parent := &rm.Person{}
		child := &rm.ItemTree{ArchetypeNodeID: "at0001"}
		if err := EnsureSingle(parent, "details", child); err != nil {
			t.Fatalf("EnsureSingle(details) = %v", err)
		}
		tree, ok := parent.Details.(*rm.ItemTree)
		if !ok || tree.ArchetypeNodeID != "at0001" {
			t.Errorf("Details = %T %+v, want *rm.ItemTree at0001", parent.Details, parent.Details)
		}
	})

	t.Run("Action time", func(t *testing.T) {
		t.Parallel()
		parent := &rm.Action{}
		when := rm.DVDateTime{Value: "2020-01-02T03:04:05Z"}
		if err := EnsureSingle(parent, "time", &when); err != nil {
			t.Fatalf("EnsureSingle(time) = %v", err)
		}
		if parent.Time.Value != when.Value {
			t.Errorf("Time = %q, want %q", parent.Time.Value, when.Value)
		}
	})

	t.Run("Person relationships", func(t *testing.T) {
		t.Parallel()
		parent := &rm.Person{}
		child := &rm.PartyRelationship{ArchetypeNodeID: "at0001"}
		if err := AppendMultiple(parent, "relationships", child); err != nil {
			t.Fatalf("AppendMultiple(relationships) = %v", err)
		}
		if len(parent.Relationships) != 1 || parent.Relationships[0].ArchetypeNodeID != "at0001" {
			t.Errorf("Relationships = %+v, want one at0001", parent.Relationships)
		}
	})

	t.Run("Address name", func(t *testing.T) {
		t.Parallel()
		parent := &rm.Address{}
		child := &rm.DVCodedText{
			Value: "home",
			DefiningCode: rm.CodePhrase{
				CodeString:    "at0461",
				TerminologyID: rm.TerminologyID{Value: "local"},
			},
		}
		if err := EnsureSingle(parent, "name", child); err != nil {
			t.Fatalf("EnsureSingle(name) = %v", err)
		}
		if parent.Name == nil || parent.Name.GetValue() != "home" {
			t.Errorf("Name = %v, want home", parent.Name)
		}
		code, ok := parent.Name.GetDefiningCode()
		if !ok || code.CodeString != "at0461" {
			t.Errorf("Name defining code = (%+v, %v), want at0461", code, ok)
		}
	})
}

// REQ-107: an attribute the parent does not address stays
// ErrUnknownAttribute after the new arms exist.
func TestREQ107_EnsureSingleRefusedParentsUnknownAttribute(t *testing.T) {
	t.Parallel()
	parents := []any{&rm.IsmTransition{}, &rm.Person{}, &rm.Address{}}
	for _, parent := range parents {
		if err := EnsureSingle(parent, "not_an_attribute", &rm.DVText{Value: "x"}); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("EnsureSingle(%T, not_an_attribute) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
}

// REQ-101: an attribute the RM types as DV_TEXT or PARTY_IDENTIFIED takes a
// value of a class that descends from it, DV_CODED_TEXT or PARTY_RELATED,
// wherever the parent's Go field can hold it, so the composition builder
// can write every value its type check accepts. The attribute's own class
// still writes as before, and a value of a class that does not descend from
// the attribute's type stays ErrTypeMismatch.
func TestREQ101_EnsureSingleWritesConformingSubtype(t *testing.T) {
	t.Parallel()
	coded := &rm.DVCodedText{
		Value:        "Lorem",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "12345"},
	}
	text := &rm.DVText{Value: "Lorem"}
	clinic := "Clinic"
	related := &rm.PartyRelated{Name: &clinic}
	identified := &rm.PartyIdentified{Name: &clinic}

	type writeCase struct {
		name   string
		parent func() any
		attr   string
		child  any
		// got reads the attribute back from the parent EnsureSingle wrote.
		got func(parent any) any
		// wantType is the RM type the attribute reads back with.
		wantType string
	}
	nullReason := func(p any) any { return p.(*rm.Element).NullReason }
	narrative := func(p any) any { return p.(*rm.Instruction).Narrative }
	facility := func(p any) any { return p.(*rm.EventContext).HealthCareFacility }
	newElement := func() any { return &rm.Element{} }
	newInstruction := func() any { return &rm.Instruction{} }
	newContext := func() any { return &rm.EventContext{} }

	accepted := []writeCase{
		{"ELEMENT null_reason DV_CODED_TEXT", newElement, "null_reason", coded, nullReason, "DV_CODED_TEXT"},
		{"ELEMENT null_reason DV_TEXT", newElement, "null_reason", text, nullReason, "DV_TEXT"},
		{"INSTRUCTION narrative DV_CODED_TEXT", newInstruction, "narrative", coded, narrative, "DV_CODED_TEXT"},
		{"INSTRUCTION narrative DV_TEXT", newInstruction, "narrative", text, narrative, "DV_TEXT"},
		{"EVENT_CONTEXT health_care_facility PARTY_RELATED", newContext, "health_care_facility", related, facility, "PARTY_RELATED"},
		{"EVENT_CONTEXT health_care_facility PARTY_IDENTIFIED", newContext, "health_care_facility", identified, facility, "PARTY_IDENTIFIED"},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parent := tc.parent()
			if err := EnsureSingle(parent, tc.attr, tc.child); err != nil {
				t.Fatalf("EnsureSingle(%T, %q, %T) = %v, want nil", parent, tc.attr, tc.child, err)
			}
			got := tc.got(parent)
			if name, _ := rm.RMTypeName(got); name != tc.wantType {
				t.Errorf("%T.%s = %T (RM %q), want %s", parent, tc.attr, got, name, tc.wantType)
			}
			if like, ok := got.(rm.DVTextLike); ok {
				code, isCoded := like.GetDefiningCode()
				if wantCoded := tc.wantType == "DV_CODED_TEXT"; isCoded != wantCoded || (isCoded && code.CodeString != "12345") {
					t.Errorf("%T.%s defining code = (%+v, %v), want coded %v", parent, tc.attr, code, isCoded, wantCoded)
				}
			}
		})
	}

	refused := []writeCase{
		{name: "ELEMENT null_reason DV_QUANTITY", parent: newElement, attr: "null_reason", child: &rm.DVQuantity{Units: "mm"}},
		{name: "INSTRUCTION narrative DV_QUANTITY", parent: newInstruction, attr: "narrative", child: &rm.DVQuantity{Units: "mm"}},
		{name: "EVENT_CONTEXT health_care_facility PARTY_SELF", parent: newContext, attr: "health_care_facility", child: &rm.PartySelf{}},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parent := tc.parent()
			if err := EnsureSingle(parent, tc.attr, tc.child); !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("EnsureSingle(%T, %q, %T) = %v, want ErrTypeMismatch", parent, tc.attr, tc.child, err)
			}
		})
	}
}

// TestREQ107_EnsureSingleWritesAnEventContextLocation checks that
// EVENT_CONTEXT location takes a string as a value or as a pointer, like
// every other optional String attribute, and refuses a nil pointer and a
// child of another type with ErrTypeMismatch.
func TestREQ107_EnsureSingleWritesAnEventContextLocation(t *testing.T) {
	t.Parallel()
	const ward = "ward A3"
	for name, child := range map[string]any{"value": ward, "pointer": new(ward)} {
		c := &rm.EventContext{}
		if err := EnsureSingle(c, "location", child); err != nil {
			t.Errorf("EnsureSingle(EVENT_CONTEXT, location, %s) = %v, want nil", name, err)
			continue
		}
		if c.Location == nil || *c.Location != ward {
			t.Errorf("EVENT_CONTEXT.location after writing a %s = %v, want %q", name, c.Location, ward)
		}
	}
	for _, child := range []any{(*string)(nil), 42, []byte(ward)} {
		if err := EnsureSingle(&rm.EventContext{}, "location", child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(EVENT_CONTEXT, location, %#v) = %v, want ErrTypeMismatch", child, err)
		}
	}
}
