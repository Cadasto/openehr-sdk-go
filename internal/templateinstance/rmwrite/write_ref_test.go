package rmwrite

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// fieldWrite is one attribute write a writer test makes: the RM attribute,
// whether it is multi-valued, the child, the Go field of the parent the
// write must land in, and what that field must hold after it.
type fieldWrite struct {
	attr  string
	multi bool
	child any
	field string
	want  any
}

// writerCase is one parent class a writer test writes into: the writes
// that must land, the writes whose child has the wrong type for the
// attribute and must fail with ErrTypeMismatch, and attribute names the
// class does not have, which both writers must refuse with
// ErrUnknownAttribute.
type writerCase struct {
	class      string
	parent     func() any
	writes     []fieldWrite
	mismatched []fieldWrite
	unknown    []string
}

// runWriterCases makes each write of each case on a fresh parent and
// checks the field it lands in, then checks the refusals.
func runWriterCases(t *testing.T, cases []writerCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			t.Parallel()
			for _, w := range tc.writes {
				parent := tc.parent()
				write, call := EnsureSingle, "EnsureSingle"
				if w.multi {
					write, call = AppendMultiple, "AppendMultiple"
				}
				if err := write(parent, w.attr, w.child); err != nil {
					t.Errorf("%s(%s, %q, %T) = %v, want nil", call, tc.class, w.attr, w.child, err)
					continue
				}
				got := reflect.ValueOf(parent).Elem().FieldByName(w.field)
				if !got.IsValid() {
					t.Fatalf("%s has no Go field %s", tc.class, w.field)
				}
				if !reflect.DeepEqual(got.Interface(), w.want) {
					t.Errorf("%s(%s, %q, %T): %s = %#v, want %#v", call, tc.class, w.attr, w.child, w.field, got.Interface(), w.want)
				}
			}
			for _, w := range tc.mismatched {
				write, call := EnsureSingle, "EnsureSingle"
				if w.multi {
					write, call = AppendMultiple, "AppendMultiple"
				}
				if err := write(tc.parent(), w.attr, w.child); !errors.Is(err, ErrTypeMismatch) {
					t.Errorf("%s(%s, %q, %T) = %v, want ErrTypeMismatch", call, tc.class, w.attr, w.child, err)
				}
			}
			for _, attr := range append([]string{"not_an_attribute"}, tc.unknown...) {
				for _, write := range []writeFunc{EnsureSingle, AppendMultiple} {
					if err := write(tc.parent(), attr, &rm.DVText{Value: "x"}); !errors.Is(err, ErrUnknownAttribute) {
						t.Errorf("write(%s, %q) = %v, want ErrUnknownAttribute", tc.class, attr, err)
					}
				}
			}
		})
	}
}

// objectRefWrites are the writes every OBJECT_REF class but LOCATABLE_REF
// takes: an id of any OBJECT_ID class, stored as given, and the namespace
// and type.
func objectRefWrites() []fieldWrite {
	hier := &rm.HierObjectID{Value: "6e4b1c2a-0000-4000-8000-00000000abcd"}
	generic := &rm.GenericID{Value: "NICE-NG136", Scheme: "NICE"}
	return []fieldWrite{
		{attr: "id", child: hier, field: "ID", want: hier},
		{attr: "id", child: generic, field: "ID", want: generic},
		{attr: "namespace", child: "local", field: "Namespace", want: "local"},
		{attr: "type", child: "ANY", field: "Type", want: "ANY"},
	}
}

// objectRefMismatches are children no OBJECT_REF attribute holds.
func objectRefMismatches() []fieldWrite {
	return []fieldWrite{
		{attr: "id", child: (*rm.HierObjectID)(nil)},
		{attr: "id", child: "6e4b1c2a-0000-4000-8000-00000000abcd"},
		{attr: "namespace", child: 1},
		{attr: "type", child: &rm.DVText{Value: "ANY"}},
	}
}

// TestREQ107_ReferenceParentsAddressTheirAttributes is the REQ-107 check
// that rmwrite writes the attributes an OPT can name on an object
// reference and on the identifier it holds, so the generator keeps the id,
// namespace and type an OPT constrains there and can give a reference the
// id the RM requires. An OBJECT_REF, a PARTY_REF and an ACCESS_GROUP_REF
// take an id of any OBJECT_ID class, as given; a LOCATABLE_REF takes a
// UID_BASED_ID one, into the id it declares itself, and its path. Each
// OBJECT_ID class takes its value, and GENERIC_ID its scheme too.
func TestREQ107_ReferenceParentsAddressTheirAttributes(t *testing.T) {
	t.Parallel()
	path := "/activities[at0001]"
	uid := &rm.HierObjectID{Value: "6e4b1c2a-0000-4000-8000-00000000abcd"}
	idValue := func(class string, parent func() any) writerCase {
		return writerCase{
			class:      class,
			parent:     parent,
			writes:     []fieldWrite{{attr: "value", child: "example-id", field: "Value", want: "example-id"}},
			mismatched: []fieldWrite{{attr: "value", child: 1}},
			unknown:    []string{"scheme", "namespace"},
		}
	}
	runWriterCases(t, []writerCase{
		{
			class: "OBJECT_REF", parent: func() any { return &rm.ObjectRef{} },
			writes: objectRefWrites(), mismatched: objectRefMismatches(), unknown: []string{"path"},
		},
		{
			class: "PARTY_REF", parent: func() any { return &rm.PartyRef{} },
			writes: objectRefWrites(), mismatched: objectRefMismatches(), unknown: []string{"path"},
		},
		{
			class: "ACCESS_GROUP_REF", parent: func() any { return &rm.AccessGroupRef{} },
			writes: objectRefWrites(), mismatched: objectRefMismatches(), unknown: []string{"path"},
		},
		{
			class: "LOCATABLE_REF", parent: func() any { return &rm.LocatableRef{} },
			writes: []fieldWrite{
				// The id LOCATABLE_REF declares, not the OBJECT_REF one it embeds.
				{attr: "id", child: uid, field: "ID", want: uid},
				{attr: "path", child: path, field: "Path", want: &path},
				{attr: "namespace", child: "local", field: "Namespace", want: "local"},
				{attr: "type", child: "INSTRUCTION", field: "Type", want: "INSTRUCTION"},
			},
			mismatched: []fieldWrite{
				{attr: "id", child: &rm.GenericID{Value: "x", Scheme: "local"}},
				{attr: "id", child: &rm.TerminologyID{Value: "SNOMED-CT"}},
				{attr: "id", child: (*rm.HierObjectID)(nil)},
				{attr: "path", child: 1},
			},
		},
		idValue("HIER_OBJECT_ID", func() any { return &rm.HierObjectID{} }),
		idValue("OBJECT_VERSION_ID", func() any { return &rm.ObjectVersionID{} }),
		idValue("ARCHETYPE_ID", func() any { return &rm.ArchetypeID{} }),
		idValue("TEMPLATE_ID", func() any { return &rm.TemplateID{} }),
		idValue("TERMINOLOGY_ID", func() any { return &rm.TerminologyID{} }),
		{
			class: "GENERIC_ID", parent: func() any { return &rm.GenericID{} },
			writes: []fieldWrite{
				{attr: "value", child: "NICE-NG136", field: "Value", want: "NICE-NG136"},
				{attr: "scheme", child: "NICE", field: "Scheme", want: "NICE"},
			},
			mismatched: []fieldWrite{{attr: "scheme", child: 1}},
			unknown:    []string{"namespace"},
		},
	})
}

// entryRefWrites are the workflow_id and guideline_id writes of a
// CARE_ENTRY: any OBJECT_REF class, stored as given, so a PARTY_REF stays
// one.
func entryRefWrites() []fieldWrite {
	workflow := &rm.ObjectRef{ID: &rm.HierObjectID{Value: "wf-1"}, Namespace: "local", Type: "ANY"}
	guideline := &rm.PartyRef{ID: &rm.HierObjectID{Value: "g-1"}, Namespace: "local", Type: "PERSON"}
	return []fieldWrite{
		{attr: "workflow_id", child: workflow, field: "WorkflowID", want: workflow},
		{attr: "guideline_id", child: guideline, field: "GuidelineID", want: guideline},
	}
}

// entryRefMismatches are children neither reference of an ENTRY holds.
func entryRefMismatches() []fieldWrite {
	return []fieldWrite{
		{attr: "workflow_id", child: &rm.DVText{Value: "wf"}},
		{attr: "workflow_id", child: (*rm.ObjectRef)(nil)},
		{attr: "guideline_id", child: &rm.HierObjectID{Value: "g-1"}},
	}
}

// TestREQ107_EntryParentsAddressTheirReferences is the REQ-107 check that
// rmwrite writes an ENTRY's workflow_id, a CARE_ENTRY's guideline_id, an
// ACTION's instruction_details and the attributes of the
// INSTRUCTION_DETAILS it holds, so the generator keeps what an OPT
// constrains there. ADMIN_ENTRY is no CARE_ENTRY, so it has no
// guideline_id.
func TestREQ107_EntryParentsAddressTheirReferences(t *testing.T) {
	t.Parallel()
	instructionID := &rm.LocatableRef{
		Namespace: "local",
		Type:      "INSTRUCTION",
		ID:        &rm.HierObjectID{Value: "6e4b1c2a-0000-4000-8000-00000000abcd"},
	}
	details := &rm.InstructionDetails{InstructionID: *instructionID, ActivityID: "activities[at0001]"}
	tree := &rm.ItemTree{ArchetypeNodeID: "at0010"}
	runWriterCases(t, []writerCase{
		{class: "OBSERVATION", parent: func() any { return &rm.Observation{} }, writes: entryRefWrites(), mismatched: entryRefMismatches()},
		{class: "EVALUATION", parent: func() any { return &rm.Evaluation{} }, writes: entryRefWrites(), mismatched: entryRefMismatches()},
		{class: "INSTRUCTION", parent: func() any { return &rm.Instruction{} }, writes: entryRefWrites(), mismatched: entryRefMismatches()},
		{
			class: "ACTION", parent: func() any { return &rm.Action{} },
			writes: append(entryRefWrites(),
				fieldWrite{attr: "instruction_details", child: details, field: "InstructionDetails", want: details}),
			mismatched: append(entryRefMismatches(), fieldWrite{attr: "instruction_details", child: instructionID}),
		},
		{
			class: "ADMIN_ENTRY", parent: func() any { return &rm.AdminEntry{} },
			writes:     entryRefWrites()[:1],
			mismatched: entryRefMismatches()[:2],
			unknown:    []string{"guideline_id"},
		},
		{
			class: "INSTRUCTION_DETAILS", parent: func() any { return &rm.InstructionDetails{} },
			writes: []fieldWrite{
				{attr: "instruction_id", child: instructionID, field: "InstructionID", want: *instructionID},
				{attr: "activity_id", child: "activities[at0001]", field: "ActivityID", want: "activities[at0001]"},
				{attr: "wf_details", child: tree, field: "WfDetails", want: tree},
			},
			mismatched: []fieldWrite{
				{attr: "instruction_id", child: &rm.ObjectRef{Namespace: "local", Type: "ANY"}},
				{attr: "activity_id", child: 1},
				{attr: "wf_details", child: &rm.Cluster{}},
			},
			unknown: []string{"name"},
		},
	})
}

// TestREQ107_EHRContainerParentsAddressTheirAttributes is the REQ-107
// check that rmwrite writes the attributes an OPT can name on an
// EHR_STATUS and a FOLDER, the EHR-wide roots REQ-110 validates, so the
// generator can build either as a template root: an EHR_STATUS's name,
// PARTY_SELF subject, other_details and the two Boolean flags, and a
// FOLDER's name, details, sub-folders and items, which take any class of
// OBJECT_REF as given.
func TestREQ107_EHRContainerParentsAddressTheirAttributes(t *testing.T) {
	t.Parallel()
	tree := &rm.ItemTree{ArchetypeNodeID: "at0001"}
	self := &rm.PartySelf{ExternalRef: &rm.PartyRef{ID: &rm.HierObjectID{Value: "p-1"}, Namespace: "DEMOGRAPHIC", Type: "PERSON"}}
	item := &rm.ObjectRef{ID: &rm.HierObjectID{Value: "c-1"}, Namespace: "local", Type: "VERSIONED_COMPOSITION"}
	group := &rm.AccessGroupRef{ID: &rm.HierObjectID{Value: "g-1"}, Namespace: "local", Type: "ACCESS_GROUP"}
	runWriterCases(t, []writerCase{
		{
			class: "EHR_STATUS", parent: func() any { return &rm.EHRStatus{} },
			writes: []fieldWrite{
				{attr: "name", child: &rm.DVText{Value: "EHR status"}, field: "Name", want: rm.DVText{Value: "EHR status"}},
				{attr: "subject", child: self, field: "Subject", want: *self},
				{attr: "other_details", child: tree, field: "OtherDetails", want: tree},
				{attr: "is_queryable", child: true, field: "IsQueryable", want: true},
				{attr: "is_modifiable", child: true, field: "IsModifiable", want: true},
			},
			mismatched: []fieldWrite{
				{attr: "subject", child: &rm.PartyIdentified{}},
				{attr: "other_details", child: &rm.Cluster{}},
				{attr: "is_queryable", child: "true"},
				{attr: "is_modifiable", child: 1},
			},
			unknown: []string{"settings"},
		},
		{
			class: "FOLDER", parent: func() any { return &rm.Folder{} },
			writes: []fieldWrite{
				{attr: "name", child: &rm.DVText{Value: "episodes"}, field: "Name", want: rm.DVText{Value: "episodes"}},
				{attr: "details", child: tree, field: "Details", want: tree},
				{attr: "folders", multi: true, child: &rm.Folder{ArchetypeNodeID: "at0002"}, field: "Folders", want: []rm.Folder{{ArchetypeNodeID: "at0002"}}},
				{attr: "items", multi: true, child: item, field: "Items", want: []rm.ObjectRefLike{item}},
				{attr: "items", multi: true, child: group, field: "Items", want: []rm.ObjectRefLike{group}},
			},
			mismatched: []fieldWrite{
				{attr: "details", child: &rm.Cluster{}},
				{attr: "folders", multi: true, child: &rm.Section{}},
				{attr: "items", multi: true, child: &rm.HierObjectID{Value: "c-1"}},
				{attr: "items", multi: true, child: (*rm.ObjectRef)(nil)},
			},
			unknown: []string{"settings", "subject"},
		},
	})
}
