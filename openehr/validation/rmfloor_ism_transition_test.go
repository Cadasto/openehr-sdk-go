package validation_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// ismAction returns an otherwise complete ACTION whose ISM_TRANSITION
// carries a coded current_state and the given careflow_step. The coded
// current_state makes the transition read as present, so the floor walks
// into it.
func ismAction(careflowStep *rm.DVCodedText) *rm.Action {
	return &rm.Action{
		ArchetypeNodeID:  "openEHR-EHR-ACTION.medication.v1",
		Name:             rm.DVText{Value: "Medication management"},
		ArchetypeDetails: &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-ACTION.medication.v1"}, RMVersion: "1.1.0"},
		Language:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Encoding:         rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}, CodeString: "UTF-8"},
		Subject:          rm.PartySelf{},
		Time:             rm.DVDateTime{Value: "2026-10-01T10:00:00Z"},
		Description:      &rm.ItemTree{ArchetypeNodeID: "at0017", Name: rm.DVText{Value: "Tree"}},
		IsmTransition: rm.IsmTransition{
			CurrentState: rm.DVCodedText{
				Value:        "completed",
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "532"},
			},
			CareflowStep: careflowStep,
		},
	}
}

// ismTransitionIssues keeps the issues the floor reports under
// /ism_transition.
func ismTransitionIssues(issues []validation.Issue) []validation.Issue {
	var out []validation.Issue
	for _, i := range issues {
		if strings.HasPrefix(i.Path, "/ism_transition") {
			out = append(out, i)
		}
	}
	return out
}

// TestValidateRMAction_IsmTransitionCareflowStepMissingDefiningCode pins
// that the floor descends into an ACTION's ISM_TRANSITION (REQ-112).
// careflow_step is optional on ISM_TRANSITION, but once present it is a
// DV_CODED_TEXT whose defining_code is RM-mandatory. A careflow_step with a
// value and no defining_code must surface `required` at that path. The
// floor reaches it only because rmread models ISM_TRANSITION.
func TestValidateRMAction_IsmTransitionCareflowStepMissingDefiningCode(t *testing.T) {
	act := ismAction(&rm.DVCodedText{Value: "Medication administered"})
	r := validation.ValidateRM(act)
	if r.OK {
		t.Fatalf("ValidateRM(ACTION with careflow_step lacking defining_code) should not be OK; issues=%+v", r.Issues)
	}
	if !containsIssue(r.Issues, "/ism_transition/careflow_step/defining_code", "required") {
		t.Errorf("expected required at /ism_transition/careflow_step/defining_code, got %+v", r.Issues)
	}
}

// TestValidateRMAction_IsmTransitionCompleteCareflowStep is the positive
// twin (REQ-112): the same ACTION with a fully coded careflow_step reports
// nothing under /ism_transition, and nothing at all.
func TestValidateRMAction_IsmTransitionCompleteCareflowStep(t *testing.T) {
	act := ismAction(&rm.DVCodedText{
		Value:        "Medication administered",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0006"},
	})
	r := validation.ValidateRM(act)
	if got := ismTransitionIssues(r.Issues); len(got) != 0 {
		t.Errorf("ValidateRM(ACTION with complete careflow_step) reported issues under /ism_transition: %+v", got)
	}
	if !r.OK || len(r.Issues) != 0 {
		t.Errorf("ValidateRM(ACTION with complete careflow_step) want OK with no issues; got %+v", r.Issues)
	}
}
