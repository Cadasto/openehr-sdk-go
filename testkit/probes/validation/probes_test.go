package validationprobes_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// PROBE-025 — positive case. A structurally-complete blood-pressure
// composition against vital_signs.opt produces zero issues.
func TestProbe025CompositionValidate_Positive(t *testing.T) {
	body := loadFixture(t, "vital_signs.opt")
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
	body := loadFixture(t, "vital_signs.opt")
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
	body := loadFixture(t, "vital_signs.opt")
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

// PROBE-026 — empty events. An OBSERVATION whose Data.Events slice
// is empty fires BOTH `required` (existence lower ≥ 1) AND
// `cardinality` (the OPT pins child-count lower ≥ 1) at
// /data/events. Both constraints are independent — multiset
// reflects both for cross-SDK parity.
func TestProbe026MissingNodes_EmptyEvents(t *testing.T) {
	body := loadFixture(t, "vital_signs.opt")
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

// loadFixture reads an OPT file from openehr/template/testdata
// relative to the test file's own location. Reaches across the
// module tree but stays inside the SDK; no transport involvement.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("could not resolve test file path via runtime.Caller")
	}
	path := filepath.Join(filepath.Dir(here), "..", "..", "..", "openehr", "template", "testdata", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
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
