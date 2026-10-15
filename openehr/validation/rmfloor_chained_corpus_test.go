package validation_test

// rmfloor_chained_corpus_test.go: REQ-102 § RM floor, REQ-112 — the cases
// issue 238 measured. Each one-defect copy of the BMI skeleton met the
// template and broke an RM rule; template validation now reports the RM
// rule's code at the node's path. Three corpus compositions that were
// template-valid and RM-invalid are now not OK, and the findings both layers
// see in clinical_notes.v0 appear once each.

import (
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// bmiObservationPath is the path of the OBSERVATION in the BMI skeleton.
const bmiObservationPath = "/content[openEHR-EHR-OBSERVATION.body_mass_index.v1]"

// bmiSkeleton returns the composition composition.NewSkeleton generates for
// testkit/corpus/templates/BMI.opt, with its OBSERVATION, its event and the
// event's BMI ELEMENT (at0004) for a test to break in place.
func bmiSkeleton(t *testing.T) (*rm.Composition, *rm.Observation, *rm.PointEvent[rm.ItemStructure], *rm.Element) {
	t.Helper()
	comp, err := composition.NewSkeleton(t.Context(), mustCompile(t, "BMI"),
		composition.WithTerritory("NL"),
		composition.WithComposer(&rm.PartyIdentified{Name: new("Dr Jones")}),
		composition.WithNow(time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatalf("NewSkeleton(BMI): %v", err)
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("BMI skeleton content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	event, ok := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("BMI skeleton event is %T, want *rm.PointEvent[rm.ItemStructure]", obs.Data.Events[0])
	}
	tree, ok := event.Data.(*rm.ItemTree)
	if !ok {
		t.Fatalf("BMI skeleton event data is %T, want *rm.ItemTree", event.Data)
	}
	el, ok := tree.Items[0].(*rm.Element)
	if !ok || el.ArchetypeNodeID != "at0004" {
		t.Fatalf("BMI skeleton event data items[0] is %T, want the BMI ELEMENT at0004", tree.Items[0])
	}
	return comp, obs, event, el
}

// REQ-102, REQ-112 — each one-defect copy of the BMI skeleton that issue 238
// listed is not OK under ValidateComposition and carries the floor's code at
// the node's path, once. The want lists are the whole result.
func TestValidateComposition_REQ102_REQ112_BMIOneDefectCopies(t *testing.T) {
	const element = bmiObservationPath + "/data/events[at0002]/data/items[at0004]"
	precision := rm.Integer(-5)
	tests := []struct {
		name   string
		mutate func(comp *rm.Composition, obs *rm.Observation, event *rm.PointEvent[rm.ItemStructure], el *rm.Element)
		want   []string
	}{
		{
			name: "context start_time not a date",
			mutate: func(comp *rm.Composition, _ *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], _ *rm.Element) {
				comp.Context.StartTime.Value = "not-a-date"
			},
			want: []string{"rm_invariant /context/start_time"},
		},
		{
			name: "event time in month 13",
			mutate: func(_ *rm.Composition, _ *rm.Observation, event *rm.PointEvent[rm.ItemStructure], _ *rm.Element) {
				event.Time.Value = "2026-13-11T09:00:00Z"
			},
			want: []string{"rm_invariant " + bmiObservationPath + "/data/events[at0002]/time"},
		},
		{
			name: "DV_QUANTITY precision -5",
			mutate: func(_ *rm.Composition, _ *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], el *rm.Element) {
				el.Value.(*rm.DVQuantity).Precision = &precision
			},
			want: []string{"rm_invariant " + element + "/value"},
		},
		{
			name: "ELEMENT with both value and null_flavour",
			mutate: func(_ *rm.Composition, _ *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], el *rm.Element) {
				el.NullFlavour = &rm.DVCodedText{
					Value:        "no information",
					DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "271"},
				}
			},
			want: []string{"rm_invariant " + element},
		},
		{
			name: "ELEMENT with neither value nor null_flavour",
			mutate: func(_ *rm.Composition, _ *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], el *rm.Element) {
				el.Value = nil
			},
			want: []string{"rm_invariant " + element},
		},
		{
			name: "OBSERVATION without archetype_details",
			mutate: func(_ *rm.Composition, obs *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], _ *rm.Element) {
				obs.ArchetypeDetails = nil
			},
			want: []string{"is_archetype_root " + bmiObservationPath + "/archetype_details"},
		},
		{
			name: "OBSERVATION rm_version empty",
			mutate: func(_ *rm.Composition, obs *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], _ *rm.Element) {
				obs.ArchetypeDetails.RMVersion = ""
			},
			want: []string{"rm_version_valid " + bmiObservationPath + "/archetype_details/rm_version"},
		},
		{
			name: "COMPOSITION language with an empty code_string",
			mutate: func(comp *rm.Composition, _ *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], _ *rm.Element) {
				comp.Language.CodeString = ""
			},
			want: []string{
				"code_not_in_value_set /language",
				"required /language/code_string",
			},
		},
		{
			name: "present but empty mappings",
			mutate: func(_ *rm.Composition, _ *rm.Observation, _ *rm.PointEvent[rm.ItemStructure], el *rm.Element) {
				el.Name = rm.DVText{Value: "example", Mappings: []rm.TermMapping{}}
			},
			want: []string{"mappings_valid " + element + "/name/mappings"},
		},
	}
	c := mustCompile(t, "BMI")
	if comp, _, _, _ := bmiSkeleton(t); !validation.ValidateComposition(comp, c).OK {
		t.Fatalf("precondition: the BMI skeleton is not valid: %q", issueKeys(validation.ValidateComposition(comp, c).Issues))
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp, obs, event, el := bmiSkeleton(t)
			tc.mutate(comp, obs, event, el)
			r := validation.ValidateComposition(comp, c)
			if r.OK {
				t.Errorf("ValidateComposition.OK = true, want false")
			}
			if !equalKeys(r.Issues, tc.want) {
				t.Errorf("ValidateComposition issues = %q, want %q", issueKeys(r.Issues), tc.want)
			}
		})
	}
}

// decodedCorpus decodes the corpus composition of templateID and compiles
// its OPT.
func decodedCorpus(t *testing.T, templateID string) validation.Result {
	t.Helper()
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON(templateID), &comp)
	return validation.ValidateComposition(&comp, mustCompile(t, templateID))
}

// REQ-102, REQ-112 — the three corpus compositions issue 238 found
// template-valid and RM-invalid are not OK: each DV_INTERVAL has its lower
// bound above its upper one, and Demonstration.v1 also has an ELEMENT with
// both value and null_flavour. The want lists are the whole result.
func TestValidateComposition_REQ102_REQ112_CorpusFloorBreaches(t *testing.T) {
	// Both events of the interval tests carry at0002, so the event is
	// written by its position.
	const intervalTest = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[1]/data/items[at0046]/value"
	tests := []struct {
		template string
		want     []string
	}{
		{template: "Test_dv_interval_dv_count_open_constraint.v0", want: []string{"rm_invariant " + intervalTest}},
		{template: "Test_dv_interval_dv_quantity_open_constraint.v0", want: []string{"rm_invariant " + intervalTest}},
		{template: "Demonstration.v1", want: demonstrationFloorIssues},
	}
	for _, tc := range tests {
		t.Run(tc.template, func(t *testing.T) {
			r := decodedCorpus(t, tc.template)
			if r.OK {
				t.Errorf("ValidateComposition(%s).OK = true, want false", tc.template)
			}
			if !equalKeys(r.Issues, tc.want) {
				t.Errorf("ValidateComposition(%s) issues = %q, want %q", tc.template, issueKeys(r.Issues), tc.want)
			}
		})
	}
}

// demonstrationFloorIssues is the whole result of ValidateComposition on the
// Demonstration.v1 corpus composition, sorted as issueKeys sorts it: in each
// event, the ELEMENT at0016 carries both value and null_flavour, and the
// DV_INTERVALs at0022 (DV_COUNT), at0023 (DV_QUANTITY) and at0024
// (DV_DATE_TIME) have their lower bound above their upper one. All are the
// RM rules'; the template reports nothing.
var demonstrationFloorIssues = func() []string {
	const obs = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events"
	var out []string
	for _, cluster := range []struct {
		path      string
		intervals []string
	}{
		// at0024: inverted DV_INTERVAL<DV_DATE_TIME> in the vendored fixture (DV_INTERVAL.Limits_consistent).
		{path: "[at0002]/data/items[at0004]", intervals: []string{"at0022", "at0023", "at0024"}},
		// at0024: inverted DV_INTERVAL<DV_DATE_TIME> in the vendored fixture (DV_INTERVAL.Limits_consistent).
		{path: "[at0033]/data/items[at0004]", intervals: []string{"at0022", "at0023", "at0024"}},
		{path: "[at0034]/data/items[at0004]", intervals: []string{"at0022"}},
		{path: "[at0035]/data/items[2]", intervals: []string{"at0023"}},
		{path: "[at0035]/data/items[3]", intervals: []string{"at0023"}},
	} {
		out = append(out, "rm_invariant "+obs+cluster.path+"/items[at0016]")
		for _, iv := range cluster.intervals {
			out = append(out, "rm_invariant "+obs+cluster.path+"/items["+iv+"]/value")
		}
	}
	return out
}()

// REQ-102, REQ-112, REQ-171 — clinical_notes.v0 is where the template and the
// RM rules both see defects; each is one issue at one path, so no code and
// path appears twice in the result.
func TestValidateComposition_REQ102_REQ112_ClinicalNotesOnce(t *testing.T) {
	r := decodedCorpus(t, "clinical_notes.v0")
	if !equalKeys(r.Issues, clinicalNotesIssues) {
		t.Errorf("ValidateComposition(clinical_notes.v0) issues = %q, want %q", issueKeys(r.Issues), clinicalNotesIssues)
	}
	keys := issueKeys(r.Issues)
	for i := 1; i < len(keys); i++ {
		if keys[i] == keys[i-1] {
			t.Errorf("ValidateComposition(clinical_notes.v0) carries %q twice", keys[i])
		}
	}
}

// clinicalNotesIssues is the whole result of ValidateComposition on the
// clinical_notes.v0 corpus composition, sorted as issueKeys sorts it. The
// seven `required` findings marked below are absences both the template and
// the RM rules see, each reported once. The other two are the RM rules'
// alone: the timing's attributes. Three of the seven are a temporal value
// whose `value` is JSON null, which REQ-112 reports once, at its
// `value/value`, and not again by the value's Value_valid. Issue 238 counted
// the same seven as duplicates, when the template and the RM rules ran in
// two walks.
var clinicalNotesIssues = func() []string {
	const order = "/content[openEHR-EHR-INSTRUCTION.medication_order.v3]"
	const direction = order + "/activities[at0001]/description/items[openEHR-EHR-CLUSTER.therapeutic_direction.v1]"
	const story = "/content[openEHR-EHR-OBSERVATION.story.v1]"
	return []string{
		"required " + order + "/activities[at0001]/action_archetype_id", // both
		"required " + direction + "/items[at0066]/value/value",          // both
		"required " + direction + "/items[at0178]/value/value",          // both
		"required " + direction + "/items[at0179]/value/value",          // both
		"required " + order + "/activities[at0001]/timing/formalism",
		"required " + order + "/activities[at0001]/timing/value",
		"required " + order + "/narrative",                // both
		"required " + story + "/data/events[at0002]/time", // both
		"required " + story + "/data/origin",              // both
	}
}()

// externalCodeChild is an ELEMENT value child DV_CODED_TEXT whose
// defining_code is a C_CODE_PHRASE of an external terminology with no code
// list, so the template checks no code against it.
const externalCodeChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>SNOMED-CT</value></terminology_id>
          </children>
        </attributes>
      </children>`

// REQ-102, REQ-112 — a blank code_string under an external C_CODE_PHRASE
// with no code list meets the template, which has no code to check it
// against; the RM rules, which template validation applies, report the
// CODE_PHRASE's RM-mandatory code_string once, as required.
func TestValidate_REQ102_REQ112_ExternalCodePhraseBlankCode(t *testing.T) {
	c := mustCompileInline(t, elementValueOPT(externalCodeChild))
	coded := rm.DVCodedText{
		Value:        "coded",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "271649006"},
	}
	if r := validation.Validate(valueElement(coded), c); !r.OK {
		t.Fatalf("precondition: Validate(coded SNOMED-CT value) issues = %q, want none", issueKeys(r.Issues))
	}
	coded.DefiningCode.CodeString = ""
	r := validation.Validate(valueElement(coded), c)
	want := []string{"required /value/defining_code/code_string"}
	if r.OK || !equalKeys(r.Issues, want) {
		t.Errorf("Validate(blank SNOMED-CT code) OK = %t, issues = %q; want not OK with %q", r.OK, issueKeys(r.Issues), want)
	}
}
