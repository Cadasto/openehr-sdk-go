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
		if err := EnsureSingle(parent, "ISM_TRANSITION", "current_state", child); err != nil {
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
		if err := EnsureSingle(parent, "PERSON", "details", child); err != nil {
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
		if err := EnsureSingle(parent, "ACTION", "time", &when); err != nil {
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
		if err := AppendMultiple(parent, "PERSON", "relationships", child); err != nil {
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
		if err := EnsureSingle(parent, "ADDRESS", "name", child); err != nil {
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
		if err := EnsureSingle(parent, "", "not_an_attribute", &rm.DVText{Value: "x"}); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("EnsureSingle(%T, not_an_attribute) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
}
