package validationprobes_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// PROBE-025 — positive case. A structurally-complete blood-pressure
// composition against vital_signs.opt produces zero issues.
func TestProbe025CompositionValidate_Positive(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "valid_blood_pressure",
			OPT:         body,
			Composition: validBloodPressureComposition(),
			WantCodes:   nil,
		},
	}
	r, err := probes.Probe025CompositionValidate(cases)
	if err != nil {
		t.Fatalf("Probe025: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe025 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-025" {
		t.Errorf("Probe id = %q, want PROBE-025", r.Probe)
	}
}

// PROBE-025 — primitive-violation case. Out-of-range systolic
// magnitude triggers a single primitive_out_of_range issue. Stable
// across SDKs that implement REQ-103 + REQ-102 v2.
func TestProbe025CompositionValidate_PrimitiveOutOfRange(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(2000), // OPT range is [0,1000)
		Units:     "mm[Hg]",
	}
	cases := []probes.ValidateCase{
		{
			Name:        "systolic_2000_mmHg",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"primitive_out_of_range"},
		},
	}
	r, err := probes.Probe025CompositionValidate(cases)
	if err != nil {
		t.Fatalf("Probe025: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe025 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — missing-required-node case. Removing the systolic
// element from the ITEM_LIST surfaces a `required` issue at the
// /data/items path. Negative-case stability test.
func TestProbe026MissingNodes_MissingSystolic(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items = nil
	cases := []probes.ValidateCase{
		{
			Name:        "missing_systolic_items",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"required"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-026" {
		t.Errorf("Probe id = %q, want PROBE-026", r.Probe)
	}
}

// PROBE-026 — unknown archetype id under /content fires `slot_fill`.
// A Content[i] whose archetype_node_id matches no OPT root and no
// slot include fails the slot-fit fallback.
func TestProbe026MissingNodes_SlotFillUnknownArchetype(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	comp.Content = []rm.ContentItem{
		&rm.Observation{
			ArchetypeNodeID: "openEHR-EHR-OBSERVATION.no_such_archetype.v1",
		},
	}
	cases := []probes.ValidateCase{
		{
			Name:        "unknown_archetype_id_under_content",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"slot_fill"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — DV_QUANTITY units outside the OPT-allowed list fires
// `primitive_unit_unknown` at the element's /value path.
func TestProbe026MissingNodes_PrimitiveUnitUnknown(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(120),
		Units:     "psi", // not in {mm[Hg]}
	}
	cases := []probes.ValidateCase{
		{
			Name:        "systolic_units_psi",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"primitive_unit_unknown"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — category defining_code outside the OPT closed list
// fires `primitive_not_in_list` at /category/defining_code.
func TestProbe026MissingNodes_PrimitiveNotInList(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	comp.Category.DefiningCode = rm.CodePhrase{
		TerminologyID: rm.TerminologyID{Value: "openehr"},
		CodeString:    "999",
	}
	cases := []probes.ValidateCase{
		{
			Name:        "category_code_999",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"primitive_not_in_list"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — empty events. An OBSERVATION whose Data.Events slice
// is empty fires BOTH `required` (existence lower ≥ 1) AND
// `cardinality` (the OPT pins child-count lower ≥ 1) at
// /data/events. Both constraints are independent — multiset
// reflects both for cross-SDK parity.
func TestProbe026MissingNodes_EmptyEvents(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	obs := comp.Content[0].(*rm.Observation)
	obs.Data.Events = nil
	cases := []probes.ValidateCase{
		{
			Name:        "empty_events",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"required", "cardinality"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — per-child occurrences upper bound. vital_signs.opt
// pins the systolic ELEMENT (at0004) under /data/events[at0006]/data/items
// at occurrences 0..1; supplying two copies of the systolic
// element fires `cardinality` at the child path. Mirrors the
// unit-level TestValidateComposition_DuplicateSystolicOccurrences
// so cross-SDK implementations are bound to the same code
// multiset for the occurrences-upper-bound clause.
func TestProbe026MissingNodes_DuplicateSystolicOccurrences(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	comp := validBloodPressureComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	systolic := list.Items[0]
	list.Items = []rm.Element{systolic, systolic}
	cases := []probes.ValidateCase{
		{
			Name:        "duplicate_systolic",
			OPT:         body,
			Composition: comp,
			WantCodes:   []string{"cardinality"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

const probe026AlternativeMismatchOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>alt-probe</value></template_id>
  <concept>alt-probe</concept>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>composer</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_SELF</rm_type_name>
        <node_id />
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_IDENTIFIED</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`

const probe026RMTypeMismatchOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>type-probe</value></template_id>
  <concept>type-probe</concept>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>composer</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_SELF</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`

// PROBE-026 — C_SINGLE_ATTRIBUTE with two alternatives: PartyRelated
// matches neither PARTY_SELF nor PARTY_IDENTIFIED → alternative_mismatch.
func TestProbe026MissingNodes_AlternativeMismatch(t *testing.T) {
	cases := []probes.ValidateCase{
		{
			Name: "composer_party_related",
			OPT:  []byte(probe026AlternativeMismatchOPT),
			Composition: &rm.Composition{
				ArchetypeNodeID: "at0000",
				Name:            rm.DVText{Value: "probe"},
				Category: rm.DVCodedText{
					DVText: rm.DVText{Value: "event"},
					DefiningCode: rm.CodePhrase{
						TerminologyID: rm.TerminologyID{Value: "openehr"},
						CodeString:    "433",
					},
				},
				Language: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
					CodeString:    "en",
				},
				Territory: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
					CodeString:    "NL",
				},
				Composer: &rm.PartyRelated{
					PartyIdentified: rm.PartyIdentified{},
				},
			},
			WantCodes: []string{"alternative_mismatch"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — single-child C_SINGLE_ATTRIBUTE type constraint:
// PartyRelated under PARTY_SELF-only composer → rm_type_mismatch.
func TestProbe026MissingNodes_RMTypeMismatch(t *testing.T) {
	cases := []probes.ValidateCase{
		{
			Name: "composer_party_related_single_alt",
			OPT:  []byte(probe026RMTypeMismatchOPT),
			Composition: &rm.Composition{
				ArchetypeNodeID: "at0000",
				Name:            rm.DVText{Value: "probe"},
				Category: rm.DVCodedText{
					DVText: rm.DVText{Value: "event"},
					DefiningCode: rm.CodePhrase{
						TerminologyID: rm.TerminologyID{Value: "openehr"},
						CodeString:    "433",
					},
				},
				Language: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
					CodeString:    "en",
				},
				Territory: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
					CodeString:    "NL",
				},
				Composer: &rm.PartyRelated{
					PartyIdentified: rm.PartyIdentified{},
				},
			},
			WantCodes: []string{"rm_type_mismatch"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// loadFixture reads a vendored template.opt from testkit/cassettes/templates/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return body
}

func validBloodPressureComposition() *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Content: []rm.ContentItem{
			&rm.Observation{
				ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
				Name:            rm.DVText{Value: "Blood pressure"},
				Language: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
					CodeString:    "en",
				},
				Encoding: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"},
					CodeString:    "UTF-8",
				},
				Subject: rm.PartySelf{},
				Data: rm.History[rm.ItemStructure]{
					ArchetypeNodeID: "at0001",
					Name:            rm.DVText{Value: "history"},
					Origin:          rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
					Events: []rm.Event{
						&rm.PointEvent[rm.ItemStructure]{
							ArchetypeNodeID: "at0006",
							Name:            rm.DVText{Value: "any event"},
							Time:            rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
							Data: &rm.ItemList{
								ArchetypeNodeID: "at0003",
								Name:            rm.DVText{Value: "blood pressure"},
								Items: []rm.Element{{
									ArchetypeNodeID: "at0004",
									Name:            rm.DVText{Value: "Systolic"},
									Value: &rm.DVQuantity{
										Magnitude: rm.Real(120),
										Units:     "mm[Hg]",
									},
								}},
							},
							State: &rm.ItemList{
								ArchetypeNodeID: "at0007",
								Name:            rm.DVText{Value: "state"},
								Items: []rm.Element{{
									ArchetypeNodeID: "at0008",
									Name:            rm.DVText{Value: "Position"},
								}},
							},
						},
					},
				},
				Protocol: &rm.ItemTree{
					ArchetypeNodeID: "at0011",
					Name:            rm.DVText{Value: "protocol"},
					Items: []rm.Item{
						&rm.Cluster{
							ArchetypeNodeID: "openEHR-EHR-CLUSTER.device.v1",
							Name:            rm.DVText{Value: "Device"},
						},
					},
				},
			},
		},
	}
}
