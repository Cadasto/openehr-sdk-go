package validation_test

// STRAND-14 pin — the template-driven validators do NOT run the RM floor's
// per-type invariant catalogue (§ REQ-112: "the two layers compose but do not
// chain"). This test pins that INTERIM composition by name: a composition that
// is template-valid but carries a floor-only defect stays OK under
// [validation.ValidateComposition] while [validation.ValidateRM] reports the
// defect, so wiring the floor into the template-driven pass makes this test
// fail instead of passing silently. Resolving STRAND-14 flips this test
// deliberately (and amends § REQ-112); it must NOT be "fixed" by chaining the
// floor, because § REQ-112 says the answer MUST NOT be pre-empted in code.
//
// Same shape as the STRAND-13 pin in openehr/rm/rminfo/probe_094_test.go: pin
// current behaviour by name, flip it when the strand resolves.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// floorInvariantCodes are the codes the RM floor's per-type invariant arm
// (§ REQ-112 arm (b)) mints. None of them may appear in a template-driven
// Result while STRAND-14 is open. `rm_invariant` is listed even though neither
// injected defect emits it: it is the catch-all code for the rest of the
// catalogue (DV_INTERVAL bounds, DV_QUANTITY precision, the OBJECT_REF floor),
// so a chaining change that surfaces any of those trips this pin too.
var floorInvariantCodes = []string{"mappings_valid", "term_mapping_match", "rm_invariant"}

// TestStrand14TemplateDrivenDoesNotChainRMFloor injects a floor-only defect on
// a node vital_signs.opt models (the systolic ELEMENT's `name`) and asserts
// both halves: the floor sees the defect, the template-driven pass does not.
func TestStrand14TemplateDrivenDoesNotChainRMFloor(t *testing.T) {
	c := mustCompile(t, "vital_signs")

	// A well-formed target so the injected TERM_MAPPING breaches only the
	// `match` value set — an empty target would add its own `required` issue
	// and blunt the control.
	target := rm.CodePhrase{
		TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
		CodeString:    "271649006",
	}

	cases := []struct {
		name   string
		code   string
		defect string
		inject func(t *testing.T, comp *rm.Composition)
	}{
		{
			name:   "MappingsPresentButEmpty",
			code:   "mappings_valid",
			defect: "systolic ELEMENT name = DV_TEXT with a present-but-empty mappings ([]rm.TermMapping{})",
			inject: func(t *testing.T, comp *rm.Composition) {
				t.Helper()
				el := systolicElement(t, comp)
				el.Name = rm.DVText{Value: "Systolic", Mappings: []rm.TermMapping{}}
			},
		},
		{
			name:   "TermMappingMatchOutOfSet",
			code:   "term_mapping_match",
			defect: `systolic ELEMENT name = DV_TEXT with one TERM_MAPPING whose match is "x" (outside {'>', '=', '<', '?'})`,
			inject: func(t *testing.T, comp *rm.Composition) {
				t.Helper()
				el := systolicElement(t, comp)
				el.Name = rm.DVText{
					Value: "Systolic",
					Mappings: []rm.TermMapping{{
						Match:  rm.Character("x"),
						Target: target,
					}},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := validVitalSignsComposition()

			// Recorded, not asserted: the clean fixture's template-driven
			// result. Quoted in the pin's failure message below so a failure
			// separates "the fixture drifted" from "the floor got chained in".
			// Asserting OK here would pre-empt the pin: a chained floor makes
			// even the clean fixture not-OK, and the reader would see that
			// instead of the STRAND-14 diagnosis.
			clean := validation.ValidateComposition(comp, c)

			// Baseline, so the control below is attributable to the injection.
			if base := validation.ValidateRM(comp); containsCode(base.Issues, tc.code) {
				t.Fatalf("baseline ValidateRM(clean vital_signs fixture) already reports %q, so the control below would not discriminate; issues %+v", tc.code, base.Issues)
			}

			tc.inject(t, comp)

			// Control: the defect is real and the floor sees it.
			floor := validation.ValidateRM(comp)
			if !containsCode(floor.Issues, tc.code) {
				t.Fatalf("ValidateRM(composition with %s) reported no %q issue, want one — the injected defect is not a floor defect any more, so this test no longer pins STRAND-14; issues %+v", tc.defect, tc.code, floor.Issues)
			}

			// Pin: the template-driven pass reports none of the floor's
			// per-type invariant codes, and stays OK overall.
			got := validation.ValidateComposition(comp, c)
			for _, code := range floorInvariantCodes {
				if containsCode(got.Issues, code) {
					t.Errorf("ValidateComposition(composition with %s, vital_signs.opt) reported a %q issue: STRAND-14 (§ REQ-112, compose but do not chain) has been pre-empted in code — resolve the strand and flip this test rather than chaining the floor; issues %+v", tc.defect, code, got.Issues)
				}
			}
			if !got.OK {
				t.Errorf("ValidateComposition(composition with %s, vital_signs.opt).OK = false, want true (the defect is floor-only, so the composition stays template-valid); issues %+v — the same call on the clean fixture reported %+v", tc.defect, got.Issues, clean.Issues)
			}
		})
	}
}

// systolicElement returns the systolic ELEMENT (at0004) inside the vital_signs
// fixture built by validVitalSignsComposition — a node vital_signs.opt models,
// so a chained floor would visit it. Returns a pointer into the composition so
// callers can inject a defect in place.
func systolicElement(t *testing.T, comp *rm.Composition) *rm.Element {
	t.Helper()
	if len(comp.Content) != 1 {
		t.Fatalf("fixture composition has %d content items, want 1 — update this helper alongside validVitalSignsComposition", len(comp.Content))
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("fixture /content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	if len(obs.Data.Events) != 1 {
		t.Fatalf("fixture /content[0]/data has %d events, want 1", len(obs.Data.Events))
	}
	event, ok := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("fixture /content[0]/data/events[0] is %T, want *rm.PointEvent[rm.ItemStructure]", obs.Data.Events[0])
	}
	list, ok := event.Data.(*rm.ItemList)
	if !ok {
		t.Fatalf("fixture /content[0]/data/events[0]/data is %T, want *rm.ItemList", event.Data)
	}
	if len(list.Items) != 1 {
		t.Fatalf("fixture /content[0]/data/events[0]/data has %d items, want 1", len(list.Items))
	}
	if got := list.Items[0].ArchetypeNodeID; got != "at0004" {
		t.Fatalf("fixture /content[0]/data/events[0]/data/items[0] is %q, want at0004 — update this helper alongside validVitalSignsComposition", got)
	}
	return &list.Items[0]
}
