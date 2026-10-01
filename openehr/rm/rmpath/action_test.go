package rmpath_test

// REQ-121 — ACTION's `ism_transition` and `instruction_details`, the
// ISM_TRANSITION and INSTRUCTION_DETAILS attributes below them, and ACTIVITY's
// `action_archetype_id`. Before, none of them resolved, so the FLAT encoder,
// which reads "not found" as an absent optional, dropped an ACTION's
// RM-mandatory ism_transition without an error.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
)

// localCode is a DV_CODED_TEXT in the archetype's local terminology.
func localCode(code, value string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        value,
		DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "local"}},
	}
}

// completedAction is an ACTION whose ism_transition is the careflow step
// at0005 of its archetype, completing the instruction activity at0001.
func completedAction() *rm.Action {
	step := localCode("at0005", "Medication administered")
	transition := rm.DVCodedText{
		Value:        "finish",
		DefiningCode: rm.CodePhrase{CodeString: "555", TerminologyID: rm.TerminologyID{Value: "openehr"}},
	}
	return &rm.Action{
		ArchetypeNodeID: "openEHR-EHR-ACTION.medication.v1",
		Name:            rm.DVText{Value: "Medication management"},
		IsmTransition: rm.IsmTransition{
			CurrentState: rm.DVCodedText{
				Value:        "completed",
				DefiningCode: rm.CodePhrase{CodeString: "532", TerminologyID: rm.TerminologyID{Value: "openehr"}},
			},
			Transition:   &transition,
			CareflowStep: &step,
			Reason:       []rm.DVTextLike{&rm.DVText{Value: "as planned"}},
		},
		InstructionDetails: &rm.InstructionDetails{
			InstructionID: rm.LocatableRef{Path: new("/content[openEHR-EHR-INSTRUCTION.medication_order.v3]")},
			ActivityID:    "activities[at0001]",
			WfDetails:     &rm.ItemTree{ArchetypeNodeID: "at0010", Name: rm.DVText{Value: "wf"}},
		},
	}
}

// TestItemAtPathActionTransitionAndInstructionDetails — REQ-121. Every
// attribute below ACTION `ism_transition` and `instruction_details` resolves.
// Struct-valued attributes resolve to a pointer into the instance, DataValue
// and identifier attributes by value, and optional pointers to the pointer —
// the conventions the sibling cases already follow.
func TestItemAtPathActionTransitionAndInstructionDetails(t *testing.T) {
	a := completedAction()
	for _, tc := range []struct {
		path string
		want any
	}{
		{path: "/ism_transition", want: &a.IsmTransition},
		{path: "/ism_transition/current_state", want: a.IsmTransition.CurrentState},
		{path: "/ism_transition/transition", want: a.IsmTransition.Transition},
		{path: "/ism_transition/careflow_step", want: a.IsmTransition.CareflowStep},
		{path: "/ism_transition/reason", want: a.IsmTransition.Reason[0]},
		{path: "/instruction_details", want: a.InstructionDetails},
		{path: "/instruction_details/instruction_id", want: a.InstructionDetails.InstructionID},
		{path: "/instruction_details/activity_id", want: "activities[at0001]"},
		{path: "/instruction_details/wf_details", want: a.InstructionDetails.WfDetails},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got, err := rmpath.ItemAtPath(a, tc.path)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) = %v", tc.path, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ItemAtPath(%s) = %#v, want %#v", tc.path, got, tc.want)
			}
		})
	}
}

// TestItemAtPathActivityActionArchetypeID — REQ-121. ACTIVITY's
// `action_archetype_id` is an RM String and resolves to it, the Web Template's
// one STRING leaf.
func TestItemAtPathActivityActionArchetypeID(t *testing.T) {
	act := &rm.Activity{
		ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "a"},
		ActionArchetypeID: "/openEHR-EHR-ACTION.medication.v1/",
	}
	got, err := rmpath.ItemAtPath(act, "/action_archetype_id")
	if err != nil {
		t.Fatalf("ItemAtPath(/action_archetype_id) = %v", err)
	}
	if got != "/openEHR-EHR-ACTION.medication.v1/" {
		t.Errorf("ItemAtPath(/action_archetype_id) = %#v, want the String", got)
	}
	ins := &rm.Instruction{Name: rm.DVText{Value: "i"}, Activities: []rm.Activity{*act}}
	got, err = rmpath.ItemAtPath(ins, "/activities[at0001]/action_archetype_id")
	if err != nil || got != "/openEHR-EHR-ACTION.medication.v1/" {
		t.Errorf("ItemAtPath through INSTRUCTION = (%#v, %v), want the String", got, err)
	}
}

// TestIsmTransitionPredicateMatchesCareflowStep — REQ-121. ISM_TRANSITION is
// not LOCATABLE, so it carries no archetype_node_id. An archetype path names it
// by the node id of the ACTION archetype's ISM_TRANSITION constraint, and ADL
// 1.4 codes that transition's careflow_step with the same node id, so the
// predicate matches the transition whose careflow step it names, in the `local`
// terminology, and only that one.
func TestIsmTransitionPredicateMatchesCareflowStep(t *testing.T) {
	a := completedAction()
	got, err := rmpath.ItemAtPath(a, "/ism_transition[at0005]/current_state")
	if err != nil {
		t.Fatalf("ItemAtPath(/ism_transition[at0005]/current_state) = %v", err)
	}
	if !reflect.DeepEqual(got, a.IsmTransition.CurrentState) {
		t.Errorf("ItemAtPath(/ism_transition[at0005]/current_state) = %#v, want the current state", got)
	}
	if _, err := rmpath.ItemAtPath(a, "/ism_transition[at0006]"); !errors.Is(err, rmpath.ErrPathNotFound) {
		t.Errorf("ItemAtPath(/ism_transition[at0006]) = %v, want ErrPathNotFound: the careflow step is at0005", err)
	}

	a.IsmTransition.CareflowStep.DefiningCode.TerminologyID.Value = "SNOMED-CT"
	if _, err := rmpath.ItemAtPath(a, "/ism_transition[at0005]"); !errors.Is(err, rmpath.ErrPathNotFound) {
		t.Errorf("ItemAtPath(/ism_transition[at0005]) with a SNOMED-CT::at0005 careflow step = %v, want ErrPathNotFound: an at-code is local", err)
	}

	a.IsmTransition.CareflowStep = nil
	if _, err := rmpath.ItemAtPath(a, "/ism_transition[at0005]"); !errors.Is(err, rmpath.ErrPathNotFound) {
		t.Errorf("ItemAtPath(/ism_transition[at0005]) with no careflow step = %v, want ErrPathNotFound", err)
	}
	if _, err := rmpath.ItemAtPath(a, "/ism_transition"); err != nil {
		t.Errorf("ItemAtPath(/ism_transition) with no careflow step = %v, want the transition", err)
	}
}

// TestItemAtPathActionAbsentInstructionDetails — REQ-121. An ACTION that did
// not come from an instruction has no instruction_details, and the path finds
// nothing rather than a zero value.
func TestItemAtPathActionAbsentInstructionDetails(t *testing.T) {
	a := completedAction()
	a.InstructionDetails = nil
	a.IsmTransition.Transition = nil
	for _, p := range []string{"/instruction_details", "/instruction_details/activity_id", "/ism_transition/transition"} {
		if _, err := rmpath.ItemAtPath(a, p); !errors.Is(err, rmpath.ErrPathNotFound) {
			t.Errorf("ItemAtPath(%s) = %v, want ErrPathNotFound", p, err)
		}
	}
}
