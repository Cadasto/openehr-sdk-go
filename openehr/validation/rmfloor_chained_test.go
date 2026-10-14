package validation_test

// rmfloor_chained_test.go: REQ-102 § RM floor, REQ-110, REQ-112 — every
// template-driven entry point also runs the RM floor over the whole root and
// returns its findings in the same Result, on the nodes the OPT models and on
// those it does not (ADR 0025). These tests inject a defect only the floor
// reports and fail if the floor is not part of the template-driven result.
// They replace the STRAND-14 pin, which held the opposite while the strand
// was open.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// systolicPath is the path of the systolic ELEMENT in
// validVitalSignsComposition, a node vital_signs.opt models.
const systolicPath = bpEventItems + "[at0004]"

// devicePath is the path of the ELEMENT inside the device CLUSTER of
// validVitalSignsComposition. The CLUSTER fills a slot, so vital_signs.opt
// does not model the ELEMENT and the template walk does not visit it.
const devicePath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol/items[openEHR-EHR-CLUSTER.device.v1]/items[at0001]"

// REQ-102, REQ-112 — a defect only the floor reports, on a node the OPT
// models or on one it does not, makes ValidateComposition not OK and appears
// once in its result, at the path ValidateRM gives it.
func TestValidateComposition_REQ102_REQ112_IncludesRMFloor(t *testing.T) {
	c := mustCompile(t, "vital_signs")

	// A well-formed target so the injected TERM_MAPPING breaches only the
	// `match` value set: an empty target would add its own issues.
	target := rm.CodePhrase{
		TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"},
		CodeString:    "271649006",
	}
	precision := rm.Integer(-2)

	cases := []struct {
		name   string
		code   string
		path   string
		inject func(t *testing.T, comp *rm.Composition)
	}{
		{
			name: "present but empty mappings on a modelled name",
			code: "mappings_valid",
			path: systolicPath + "/name/mappings",
			inject: func(t *testing.T, comp *rm.Composition) {
				t.Helper()
				systolicElement(t, comp).Name = rm.DVText{Value: "Systolic", Mappings: []rm.TermMapping{}}
			},
		},
		{
			name: "DV_QUANTITY precision below -1 on a modelled value",
			code: "rm_invariant",
			path: systolicPath + "/value",
			inject: func(t *testing.T, comp *rm.Composition) {
				t.Helper()
				el := systolicElement(t, comp)
				q, ok := el.Value.(*rm.DVQuantity)
				if !ok {
					t.Fatalf("fixture systolic ELEMENT value is %T, want *rm.DVQuantity", el.Value)
				}
				q.Precision = &precision
			},
		},
		{
			name: "TERM_MAPPING match outside its set on a modelled name",
			code: "term_mapping_match",
			path: systolicPath + "/name/mappings[1]/match",
			inject: func(t *testing.T, comp *rm.Composition) {
				t.Helper()
				systolicElement(t, comp).Name = rm.DVText{
					Value:    "Systolic",
					Mappings: []rm.TermMapping{{Match: rm.Character("x"), Target: target}},
				}
			},
		},
		{
			name: "present but empty mappings on a node the OPT does not model",
			code: "mappings_valid",
			path: devicePath + "/value/mappings",
			inject: func(t *testing.T, comp *rm.Composition) {
				t.Helper()
				deviceElement(t, comp).Value = &rm.DVText{Value: "Cuff", Mappings: []rm.TermMapping{}}
			},
		},
	}

	if r := validation.ValidateComposition(validVitalSignsComposition(), c); !r.OK {
		t.Fatalf("precondition: ValidateComposition(clean vital_signs fixture) issues = %q, want none", issueKeys(r.Issues))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := validVitalSignsComposition()
			tc.inject(t, comp)

			// Control: the floor alone reports the defect at this path.
			if floor := validation.ValidateRM(comp); !containsIssue(floor.Issues, tc.path, tc.code) {
				t.Fatalf("ValidateRM: no %s at %s, so the defect is not a floor defect; got %q", tc.code, tc.path, issueKeys(floor.Issues))
			}

			got := validation.ValidateComposition(comp, c)
			if got.OK {
				t.Errorf("ValidateComposition.OK = true, want false: the floor's %s at %s is an error", tc.code, tc.path)
			}
			if n := countIssue(got.Issues, tc.path, tc.code); n != 1 {
				t.Errorf("ValidateComposition carries %s at %s %d times, want once; issues %q", tc.code, tc.path, n, issueKeys(got.Issues))
			}
		})
	}
}

// REQ-110, REQ-112 — every template-driven entry point includes the floor,
// not only ValidateComposition: each reports a defect only the floor finds
// on the root it validates.
func TestValidate_REQ110_REQ112_EveryEntryIncludesRMFloor(t *testing.T) {
	vital := mustCompile(t, "vital_signs")
	folderOPT := mustCompileInline(t, rootOnlyOPT("FOLDER", "openEHR-EHR-FOLDER.generic.v1"))
	statusOPT := mustCompileInline(t, rootOnlyOPT("EHR_STATUS", "openEHR-EHR-EHR_STATUS.generic.v1"))
	orgOPT := mustCompileInline(t, rootOnlyOPT("ORGANISATION", "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1"))

	withoutDetails := validVitalSignsComposition()
	withoutDetails.ArchetypeDetails = nil
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            rm.DVText{Value: "root", Mappings: []rm.TermMapping{}},
	}
	status := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            rm.DVText{Value: "EHR Status"},
		Subject:         rm.PartySelf{},
		IsModifiable:    true,
		IsQueryable:     true,
	}
	org := &rm.Organisation{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-ORGANISATION.organisation.v1",
		Name:            rm.DVText{Value: "Org"},
	}

	tests := []struct {
		name       string
		result     validation.Result
		code, path string
	}{
		{name: "Validate", result: validation.Validate(withoutDetails, vital), code: "is_archetype_root", path: "/archetype_details"},
		{name: "ValidateComposition", result: validation.ValidateComposition(withoutDetails, vital), code: "is_archetype_root", path: "/archetype_details"},
		{name: "ValidateFolder", result: validation.ValidateFolder(folder, folderOPT), code: "mappings_valid", path: "/name/mappings"},
		{name: "ValidateEHRStatus", result: validation.ValidateEHRStatus(status, statusOPT), code: "is_archetype_root", path: "/archetype_details"},
		{name: "ValidateDemographic", result: validation.ValidateDemographic(org, orgOPT), code: "is_archetype_root", path: "/archetype_details"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.result.OK || countIssue(tc.result.Issues, tc.path, tc.code) != 1 {
				t.Errorf("%s: OK = %t, issues %q; want not OK with one %s at %s", tc.name, tc.result.OK, issueKeys(tc.result.Issues), tc.code, tc.path)
			}
		})
	}
}

// REQ-102, REQ-112, REQ-116 — inside template validation the floor takes
// the names the template walk bound, so a floor finding inside an item bound
// by its pinned name carries that name in its predicate. The floor alone
// binds no OPT node and writes the item's position. The defect sits below
// the reused OBSERVATION, on nodes the reusing OPT does not model.
func TestValidateComposition_REQ102_REQ112_FloorTakesBoundNames(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	comp := reusedComposition("Erste", obrienName)
	obs, ok := comp.Content[1].(*rm.Observation)
	if !ok {
		t.Fatalf("content[1] is %T, want *rm.Observation", comp.Content[1])
	}
	precision := rm.Integer(-5)
	obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure]).Data.(*rm.ItemList).Items[0].Value.(*rm.DVQuantity).Precision = &precision

	const below = "/data/events[at0006]/data/items[at0004]/value"
	byTemplate := validation.ValidateComposition(comp, c)
	if want := []string{"rm_invariant " + obrienPath + below}; !equalKeys(byTemplate.Issues, want) {
		t.Errorf("ValidateComposition issues = %q, want %q", issueKeys(byTemplate.Issues), want)
	}
	byFloor := validation.ValidateRM(comp)
	if want := []string{"rm_invariant /content[2]" + below}; !equalKeys(byFloor.Issues, want) {
		t.Errorf("ValidateRM issues = %q, want %q", issueKeys(byFloor.Issues), want)
	}
}

// REQ-102 — the template walk may report one code twice at one path for two
// clauses, and both stay: a participations that is present and empty breaks
// the attribute's own cardinality, and the PARTICIPATION child it requires
// once, which declares no id and is written at the attribute's path, occurs
// too seldom.
func TestValidateComposition_REQ102_TemplateRepeatsStay(t *testing.T) {
	c := mustCompileSyntheticOPT(t, strings.Replace(
		strings.Replace(participationsOPT, "<lower>0</lower>", "<lower>1</lower>", 1),
		"<rm_attribute_name>participations</rm_attribute_name>",
		"<rm_attribute_name>participations</rm_attribute_name>"+
			"<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique>"+
			"<interval><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>",
		1))
	r := validation.ValidateComposition(participationsComposition([]rm.Participation{}...), c)
	want := []string{"cardinality /context/participations", "cardinality /context/participations"}
	if !equalKeys(r.Issues, want) {
		t.Errorf("ValidateComposition issues = %q, want %q", issueKeys(r.Issues), want)
	}
}

// REQ-102, REQ-112 — a root outside the closed RM set is one the floor
// cannot descend. Inside template validation the floor reports that as it
// does alone, beside the template walk's type mismatch at the root.
func TestValidate_REQ102_REQ112_UnknownRootReportsFloorRefusal(t *testing.T) {
	type notRM struct{ Name string }
	c := mustCompileInline(t, rootOnlyOPT("FOLDER", "openEHR-EHR-FOLDER.generic.v1"))
	r := validation.Validate(&notRM{Name: "root"}, c)
	if r.OK || countIssue(r.Issues, "/", "rm_type_mismatch") != 1 || countIssue(r.Issues, "/", "rm_type_unknown") != 1 {
		t.Errorf("Validate(non-RM root) OK = %t, issues %q; want not OK with one rm_type_mismatch and one rm_type_unknown at /", r.OK, issueKeys(r.Issues))
	}
}

// rootOnlyOPT returns an OPT whose root, of RM type rmType and archetype
// archetypeID, constrains nothing below it.
func rootOnlyOPT(rmType, archetypeID string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>root-only</value></template_id>
  <concept>root-only</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>` + rmType + `</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>` + archetypeID + `</value></archetype_id>
  </definition>
</template>`
}

// countIssue counts the issues that carry code at path.
func countIssue(issues []validation.Issue, path, code string) int {
	n := 0
	for _, i := range issues {
		if i.Path == path && i.Code == code {
			n++
		}
	}
	return n
}

// equalKeys reports whether issues, written as issueKeys writes them, are
// want.
func equalKeys(issues []validation.Issue, want []string) bool {
	return strings.Join(issueKeys(issues), "\n") == strings.Join(want, "\n")
}

// systolicElement returns the systolic ELEMENT (at0004) inside the vital_signs
// fixture built by validVitalSignsComposition, a node vital_signs.opt models.
// It returns a pointer into the composition so callers can inject a defect in
// place.
func systolicElement(t *testing.T, comp *rm.Composition) *rm.Element {
	t.Helper()
	list, ok := bpEvent(t, comp).Data.(*rm.ItemList)
	if !ok {
		t.Fatalf("fixture event data is %T, want *rm.ItemList", bpEvent(t, comp).Data)
	}
	if len(list.Items) != 1 || list.Items[0].ArchetypeNodeID != "at0004" {
		t.Fatalf("fixture event data items = %+v, want the one systolic at0004", list.Items)
	}
	return &list.Items[0]
}

// deviceElement returns the ELEMENT inside the device CLUSTER of the
// vital_signs fixture, a node vital_signs.opt does not model.
func deviceElement(t *testing.T, comp *rm.Composition) *rm.Element {
	t.Helper()
	obs := bpObservation(t, comp)
	tree, ok := obs.Protocol.(*rm.ItemTree)
	if !ok || len(tree.Items) != 1 {
		t.Fatalf("fixture protocol is %T with the wrong items, want an ITEM_TREE holding the device CLUSTER", obs.Protocol)
	}
	cluster, ok := tree.Items[0].(*rm.Cluster)
	if !ok || len(cluster.Items) != 1 {
		t.Fatalf("fixture protocol item is %T, want the device CLUSTER holding one item", tree.Items[0])
	}
	el, ok := cluster.Items[0].(*rm.Element)
	if !ok {
		t.Fatalf("fixture device item is %T, want *rm.Element", cluster.Items[0])
	}
	return el
}

// bpObservation returns the one OBSERVATION of the vital_signs fixture.
func bpObservation(t *testing.T, comp *rm.Composition) *rm.Observation {
	t.Helper()
	if len(comp.Content) != 1 {
		t.Fatalf("fixture composition has %d content items, want 1", len(comp.Content))
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("fixture /content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	return obs
}

// bpEvent returns the one event of the vital_signs fixture's OBSERVATION.
func bpEvent(t *testing.T, comp *rm.Composition) *rm.PointEvent[rm.ItemStructure] {
	t.Helper()
	obs := bpObservation(t, comp)
	if len(obs.Data.Events) != 1 {
		t.Fatalf("fixture data has %d events, want 1", len(obs.Data.Events))
	}
	event, ok := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("fixture event is %T, want *rm.PointEvent[rm.ItemStructure]", obs.Data.Events[0])
	}
	return event
}
