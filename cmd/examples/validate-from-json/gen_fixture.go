//go:build ignore

// One-off generator: go run gen_fixture.go
// Writes testdata/minimal_blood_pressure.json, a composition that round-trips
// through canjson and passes both validation passes against vital_signs.opt:
// the RM floor (validation.ValidateRM) and the template constraints
// (validation.ValidateComposition). Passing the template alone is not enough,
// because today the template pass does not run the Reference Model's own
// per-type rules, so the generator refuses to write a fixture that fails
// either pass.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	compiled := compileVitalSigns()
	comp := minimalComposition()
	requireBothPasses("built composition", comp, compiled)
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
	requireBothPasses("round-tripped composition", &back, compiled)
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("testdata/minimal_blood_pressure.json", b, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote testdata/minimal_blood_pressure.json (%d bytes)\n", len(b))
}

// requireBothPasses stops the generator, listing every issue, unless comp
// passes the RM floor and the template constraints.
func requireBothPasses(label string, comp *rm.Composition, compiled *templatecompile.Compiled) {
	passes := []struct {
		name   string
		result validation.Result
	}{
		{"RM floor", validation.ValidateRM(comp)},
		{"template constraints", validation.ValidateComposition(comp, compiled)},
	}
	failed := false
	for _, p := range passes {
		if p.result.OK {
			continue
		}
		failed = true
		fmt.Printf("%s fails the %s pass:\n", label, p.name)
		for _, i := range p.result.Issues {
			fmt.Printf("  %s [%s] %s\n", i.Path, i.Code, i.Detail)
		}
	}
	if failed {
		panic(label + " does not pass both validation passes")
	}
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

// compileVitalSigns parses the vendored vital_signs.opt strictly, so a
// template shape the parser does not support stops the generator instead of
// silently dropping the nodes beneath it, and compiles it.
func compileVitalSigns() *templatecompile.Compiled {
	opt, err := template.ParseFileStrict(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		panic(err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		panic(err)
	}
	return c
}

func minimalComposition() *rm.Composition {
	// Take the category rubric and terminology id from the bundled openEHR
	// terminology instead of typing them here, so each code has one home. A
	// miss means the terminology no longer carries `event`; stop rather than
	// write a fixture with an empty Category.value.
	eventRubric, ok := terminology.CompositionCategory.Rubric("433")
	if !ok {
		panic("code 433 (event) is not a member of the pinned composition_category group")
	}
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		// The Reference Model requires archetype_details on every archetype
		// root (the COMPOSITION and each ENTRY). The template_id records the
		// template the composition was made for.
		ArchetypeDetails: &rm.Archetyped{
			ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.encounter.v1"},
			TemplateID:  &rm.TemplateID{Value: "vital_signs"},
			RMVersion:   rm.Release,
		},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: eventRubric},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: terminology.ID},
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
		Name:            rm.DVText{Value: "Blood pressure"},
		ArchetypeDetails: &rm.Archetyped{
			ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.blood_pressure.v1"},
			RMVersion:   rm.Release,
		},
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
							Value:           &rm.DVQuantity{Magnitude: rm.Real(120), Units: "mm[Hg]"},
						}},
					},
					State: &rm.ItemList{
						ArchetypeNodeID: "at0007",
						Name:            rm.DVText{Value: "state"},
						// An ELEMENT carries a value or a null flavour, never
						// neither: at1001 is the archetype's local code for
						// "Sitting".
						Items: []rm.Element{{
							ArchetypeNodeID: "at0008",
							Name:            rm.DVText{Value: "Position"},
							Value: &rm.DVCodedText{
								Value: "Sitting",
								DefiningCode: rm.CodePhrase{
									TerminologyID: rm.TerminologyID{Value: "local"},
									CodeString:    "at1001",
								},
							},
						}},
					},
				},
			},
		},
		Protocol: &rm.ItemTree{
			ArchetypeNodeID: "at0011",
			Name:            rm.DVText{Value: "protocol"},
			Items: []rm.Item{
				// A CLUSTER holds at least one item. at0001 is the device
				// archetype's "Device name" element.
				&rm.Cluster{
					ArchetypeNodeID: "openEHR-EHR-CLUSTER.device.v1",
					Name:            rm.DVText{Value: "Device"},
					ArchetypeDetails: &rm.Archetyped{
						ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.device.v1"},
						RMVersion:   rm.Release,
					},
					Items: []rm.Item{
						&rm.Element{
							ArchetypeNodeID: "at0001",
							Name:            rm.DVText{Value: "Device name"},
							Value:           &rm.DVText{Value: "Automatic upper-arm cuff"},
						},
					},
				},
			},
		},
	}
}
