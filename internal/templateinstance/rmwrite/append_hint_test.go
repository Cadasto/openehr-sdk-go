package rmwrite

// append_hint_test.go: AppendMultipleHint makes room for the members its
// caller says it will append the first time it appends to a list that holds
// none, and leaves a list it appends nothing to nil, as the RM lists are
// left out of the JSON only while nil.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// listState is the length and capacity of a list, and whether it is nil.
type listState struct {
	length, capacity int
	isNil            bool
}

func stateOf[T any](list []T) listState {
	return listState{length: len(list), capacity: cap(list), isNil: list == nil}
}

// hintedList is a list attribute AppendMultipleHint writes: a new parent,
// the attribute, a new member it takes, and how to read the list.
type hintedList struct {
	name   string
	parent func() any
	attr   string
	member func() any
	state  func(parent any) listState
}

// hintedLists returns one list of each writer AppendMultipleHint dispatches
// to, and each list of the writers that hold more than one.
func hintedLists() []hintedList {
	return []hintedList{
		{"COMPOSITION content", func() any { return &rm.Composition{} }, "content", func() any { return &rm.Observation{} }, func(p any) listState { return stateOf(p.(*rm.Composition).Content) }},
		{"EVENT_CONTEXT participations", func() any { return &rm.EventContext{} }, "participations", func() any { return &rm.Participation{} }, func(p any) listState { return stateOf(p.(*rm.EventContext).Participations) }},
		{"SECTION items", func() any { return &rm.Section{} }, "items", func() any { return &rm.Observation{} }, func(p any) listState { return stateOf(p.(*rm.Section).Items) }},
		{"OBSERVATION other_participations", func() any { return &rm.Observation{} }, "other_participations", func() any { return &rm.Participation{} }, func(p any) listState { return stateOf(p.(*rm.Observation).OtherParticipations) }},
		{"INSTRUCTION activities", func() any { return &rm.Instruction{} }, "activities", func() any { return &rm.Activity{} }, func(p any) listState { return stateOf(p.(*rm.Instruction).Activities) }},
		{"INSTRUCTION other_participations", func() any { return &rm.Instruction{} }, "other_participations", func() any { return &rm.Participation{} }, func(p any) listState { return stateOf(p.(*rm.Instruction).OtherParticipations) }},
		{"ISM_TRANSITION reason", func() any { return &rm.IsmTransition{} }, "reason", func() any { return &rm.DVText{} }, func(p any) listState { return stateOf(p.(*rm.IsmTransition).Reason) }},
		{"HISTORY events", func() any { return &rm.History[rm.ItemStructure]{} }, "events", func() any { return &rm.PointEvent[rm.ItemStructure]{} }, func(p any) listState { return stateOf(p.(*rm.History[rm.ItemStructure]).Events) }},
		{"ITEM_TREE items", func() any { return &rm.ItemTree{} }, "items", func() any { return &rm.Element{} }, func(p any) listState { return stateOf(p.(*rm.ItemTree).Items) }},
		{"ITEM_LIST items", func() any { return &rm.ItemList{} }, "items", func() any { return &rm.Element{} }, func(p any) listState { return stateOf(p.(*rm.ItemList).Items) }},
		{"ITEM_TABLE rows", func() any { return &rm.ItemTable{} }, "rows", func() any { return &rm.Cluster{} }, func(p any) listState { return stateOf(p.(*rm.ItemTable).Rows) }},
		{"CLUSTER items", func() any { return &rm.Cluster{} }, "items", func() any { return &rm.Element{} }, func(p any) listState { return stateOf(p.(*rm.Cluster).Items) }},
		{"PERSON identities", func() any { return &rm.Person{} }, "identities", func() any { return &rm.PartyIdentity{} }, func(p any) listState { return stateOf(p.(*rm.Person).Identities) }},
		{"PERSON contacts", func() any { return &rm.Person{} }, "contacts", func() any { return &rm.Contact{} }, func(p any) listState { return stateOf(p.(*rm.Person).Contacts) }},
		{"PERSON relationships", func() any { return &rm.Person{} }, "relationships", func() any { return &rm.PartyRelationship{} }, func(p any) listState { return stateOf(p.(*rm.Person).Relationships) }},
		{"PERSON languages", func() any { return &rm.Person{} }, "languages", func() any { return &rm.DVText{} }, func(p any) listState { return stateOf(p.(*rm.Person).Languages) }},
		{"PERSON roles", func() any { return &rm.Person{} }, "roles", func() any { return &rm.PartyRef{} }, func(p any) listState { return stateOf(p.(*rm.Person).Roles) }},
		{"ROLE capabilities", func() any { return &rm.Role{} }, "capabilities", func() any { return &rm.Capability{} }, func(p any) listState { return stateOf(p.(*rm.Role).Capabilities) }},
		{"ROLE identities", func() any { return &rm.Role{} }, "identities", func() any { return &rm.PartyIdentity{} }, func(p any) listState { return stateOf(p.(*rm.Role).Identities) }},
		{"CONTACT addresses", func() any { return &rm.Contact{} }, "addresses", func() any { return &rm.Address{} }, func(p any) listState { return stateOf(p.(*rm.Contact).Addresses) }},
		{"PARTY_IDENTIFIED identifiers", func() any { return &rm.PartyIdentified{} }, "identifiers", func() any { return &rm.DVIdentifier{} }, func(p any) listState { return stateOf(p.(*rm.PartyIdentified).Identifiers) }},
		{"DV_TEXT mappings", func() any { return &rm.DVText{} }, "mappings", func() any { return &rm.TermMapping{} }, func(p any) listState { return stateOf(p.(*rm.DVText).Mappings) }},
		{"DV_PARAGRAPH items", func() any { return &rm.DVParagraph{} }, "items", func() any { return &rm.DVText{} }, func(p any) listState { return stateOf(p.(*rm.DVParagraph).Items) }},
		{"FOLDER folders", func() any { return &rm.Folder{} }, "folders", func() any { return &rm.Folder{} }, func(p any) listState { return stateOf(p.(*rm.Folder).Folders) }},
		{"FOLDER items", func() any { return &rm.Folder{} }, "items", func() any { return &rm.ObjectRef{} }, func(p any) listState { return stateOf(p.(*rm.Folder).Items) }},
		{"DV_QUANTITY other_reference_ranges", func() any { return &rm.DVQuantity{} }, "other_reference_ranges", func() any { return &rm.ReferenceRange[rm.DVQuantity]{} }, func(p any) listState { return stateOf(p.(*rm.DVQuantity).OtherReferenceRanges) }},
	}
}

// TestAppendMultipleHintSizesTheListOnce: the first member appended to a
// list with none makes room for hint members, and the list holds them all
// without growing.
func TestAppendMultipleHintSizesTheListOnce(t *testing.T) {
	t.Parallel()
	const hint = 5
	for _, tc := range hintedLists() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parent := tc.parent()
			for i := range hint {
				if err := AppendMultipleHint(parent, tc.attr, tc.member(), hint); err != nil {
					t.Fatalf("AppendMultipleHint(%s, member %d, hint %d) = %v, want nil", tc.name, i+1, hint, err)
				}
				if got, want := tc.state(parent), (listState{length: i + 1, capacity: hint}); got != want {
					t.Fatalf("after member %d of %d, %s is %+v, want %+v", i+1, hint, tc.name, got, want)
				}
			}
		})
	}
}

// TestAppendMultipleHintLeavesARefusedListNil: a member the list does not
// take leaves a list with none nil, whatever the hint.
func TestAppendMultipleHintLeavesARefusedListNil(t *testing.T) {
	t.Parallel()
	for _, tc := range hintedLists() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parent := tc.parent()
			if err := AppendMultipleHint(parent, tc.attr, 42, 5); !errors.Is(err, ErrTypeMismatch) {
				t.Fatalf("AppendMultipleHint(%s, 42, hint 5) = %v, want ErrTypeMismatch", tc.name, err)
			}
			if got := tc.state(parent); !got.isNil {
				t.Errorf("after a refused member, %s is %+v, want nil", tc.name, got)
			}
		})
	}
}

// TestAppendMultipleHintBelowTwo: a hint of 1 or less, AppendMultiple's
// own included, makes room for the one member appended.
func TestAppendMultipleHintBelowTwo(t *testing.T) {
	t.Parallel()
	for _, hint := range []int{-1, 0, 1} {
		c := &rm.Cluster{}
		if err := AppendMultipleHint(c, "items", &rm.Element{}, hint); err != nil {
			t.Fatalf("AppendMultipleHint(CLUSTER items, hint %d) = %v, want nil", hint, err)
		}
		if got, want := stateOf(c.Items), (listState{length: 1, capacity: 1}); got != want {
			t.Errorf("CLUSTER items after AppendMultipleHint with hint %d is %+v, want %+v", hint, got, want)
		}
	}
	c := &rm.Cluster{}
	if err := AppendMultiple(c, "items", &rm.Element{}); err != nil {
		t.Fatalf("AppendMultiple(CLUSTER items) = %v, want nil", err)
	}
	if got, want := stateOf(c.Items), (listState{length: 1, capacity: 1}); got != want {
		t.Errorf("CLUSTER items after AppendMultiple is %+v, want %+v", got, want)
	}
}
