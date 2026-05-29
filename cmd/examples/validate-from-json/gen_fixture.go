//go:build ignore

// One-off generator: go run gen_fixture.go
// Writes testdata/minimal_blood_pressure.json that round-trips through
// canjson and validates against vital_signs.opt.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

func main() {
	comp := minimalComposition()
	r := validateAgainstVitalSigns(comp)
	if !r.OK {
		for _, i := range r.Issues {
			fmt.Printf("  %s [%s]\n", i.Path, i.Code)
		}
		panic("minimal composition does not validate")
	}
	b, err := canjson.Marshal(comp)
	if err != nil {
		panic(err)
	}
	b, err = patchPartyProxyDiscriminators(b)
	if err != nil {
		panic(err)
	}
	var back rm.Composition
	if err := canjson.Unmarshal(b, &back); err != nil {
		panic(err)
	}
	r2 := validateAgainstVitalSigns(&back)
	if !r2.OK {
		panic(fmt.Sprintf("round-trip composition invalid: %d issues", len(r2.Issues)))
	}
	if err := os.MkdirAll("testdata", 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("testdata/minimal_blood_pressure.json", b, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote testdata/minimal_blood_pressure.json (%d bytes)\n", len(b))
}

// patchPartyProxyDiscriminators fixes empty composer/subject objects emitted
// when PartyProxy interface values are json.Marshal'd without a _type tag.
func patchPartyProxyDiscriminators(b []byte) ([]byte, error) {
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	injectPartySelf(root)
	return json.Marshal(root)
}

func injectPartySelf(v any) {
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if k == "composer" || k == "subject" {
				if m, ok := child.(map[string]any); ok && len(m) == 0 {
					node[k] = map[string]any{"_type": "PARTY_SELF"}
				}
			}
			injectPartySelf(child)
		}
	case []any:
		for _, item := range node {
			injectPartySelf(item)
		}
	}
}

func validateAgainstVitalSigns(comp *rm.Composition) validation.Result {
	opt, err := template.ParseFile("../../../openehr/template/testdata/vital_signs.opt")
	if err != nil {
		panic(err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		panic(err)
	}
	return validation.ValidateComposition(comp, c)
}

func minimalComposition() *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            &rm.DVText{Value: "Encounter"},
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
		Content: []rm.ContentItem{minimalObservation()},
	}
}

func minimalObservation() *rm.Observation {
	return &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		Name:            &rm.DVText{Value: "Blood pressure"},
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
			Name:            &rm.DVText{Value: "history"},
			Origin:          rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
			Events: []rm.Event{
				&rm.PointEvent[rm.ItemStructure]{
					ArchetypeNodeID: "at0006",
					Name:            &rm.DVText{Value: "any event"},
					Time:            rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
					Data: &rm.ItemList{
						ArchetypeNodeID: "at0003",
						Name:            &rm.DVText{Value: "blood pressure"},
						Items: []rm.Element{{
							ArchetypeNodeID: "at0004",
							Name:            &rm.DVText{Value: "Systolic"},
							Value:           &rm.DVQuantity{Magnitude: rm.Real(120), Units: "mm[Hg]"},
						}},
					},
					State: &rm.ItemList{
						ArchetypeNodeID: "at0007",
						Name:            &rm.DVText{Value: "state"},
						Items: []rm.Element{{
							ArchetypeNodeID: "at0008",
							Name:            &rm.DVText{Value: "Position"},
						}},
					},
				},
			},
		},
		Protocol: &rm.ItemTree{
			ArchetypeNodeID: "at0011",
			Name:            &rm.DVText{Value: "protocol"},
			Items: []rm.Item{
				&rm.Cluster{
					ArchetypeNodeID: "openEHR-EHR-CLUSTER.device.v1",
					Name:            &rm.DVText{Value: "Device"},
				},
			},
		},
	}
}
