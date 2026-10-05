// Example: build a composition in memory as plain Reference Model structs,
// compile an operational template (OPT), and validate the composition in two
// passes. This is the smallest validation path: no JSON, no HTTP, just typed
// RM values, a compiled template, and the list of issues each pass returns.
//
// The two passes are the RM floor (validation.ValidateRM), which checks the
// composition against the openEHR Reference Model alone, and the template
// constraints (validation.ValidateComposition), which check it against the
// OPT. Today ValidateComposition checks the template's constraints and does
// not run the RM floor's per-type rules, so a composition can satisfy its
// template and still break the Reference Model. This program runs both passes.
//
// Runs offline. With no argument it uses the vendored vital_signs.opt fixture
// and a hand-built composition that passes both; -invalid clears a required
// attribute first, so both passes have something to report:
//
//	go run ./cmd/examples/validate-composition
//	go run ./cmd/examples/validate-composition path/to/template.opt
//	go run ./cmd/examples/validate-composition -invalid
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	invalid := flag.Bool("invalid", false, "clear the composition's category first, so both passes report a missing required attribute")
	flag.Parse()
	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := flag.Args(); len(args) > 0 {
		optPath = args[0]
	}

	// Step 1: parse the OPT. Strict mode rejects an unknown node type that
	// has attributes under it; the lenient ParseFile keeps such a node as a
	// leaf and silently drops everything beneath it.
	opt, err := template.ParseFileStrict(optPath)
	if err != nil {
		return fmt.Errorf("parse OPT %s: %w", optPath, err)
	}

	// Step 2: compile it. The template pass works from the compiled template.
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}
	fmt.Printf("template             : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))
	fmt.Printf("compiled             : root %s\n", compiled.Root().RMTypeName())

	// Step 3: the composition under test.
	comp, err := vitalSignsComposition()
	if err != nil {
		return err
	}
	if *invalid {
		// Every COMPOSITION must carry a category; the zero value counts as
		// absent, so this is the simplest way to provoke a "required" issue.
		comp.Category = rm.DVCodedText{}
		fmt.Println("mutation             : cleared Category (expect required at /category)")
	}

	// Step 4: the RM floor. ValidateRM walks the composition with the
	// Reference Model as its only guide: RM-mandatory attributes on every
	// node, and the RM's own rules for each type.
	rmOK := reportPass("RM floor", validation.ValidateRM(comp))

	// Step 5: the template constraints. The template drives this walk: for
	// every node it declares, the validator reads the matching RM attribute
	// and checks presence, cardinality, RM type and archetype identity. Both
	// passes collect every issue in one walk rather than stopping at the
	// first, and both always run, because neither result stands in for the
	// other.
	templateOK := reportPass("template constraints", validation.ValidateComposition(comp, compiled))

	if !rmOK || !templateOK {
		return errors.New("composition fails at least one validation pass")
	}
	return nil
}

// reportPass prints one validation pass under its name: the verdict, then one
// line per issue. Path says where, Code is the stable identifier a program
// branches on, and Detail is the explanation for people. It returns result.OK,
// which is false exactly when the pass found an error.
func reportPass(name string, result validation.Result) bool {
	verdict := "OK"
	if !result.OK {
		verdict = "failed"
	}
	if len(result.Issues) == 0 {
		fmt.Printf("%-20s : %s, no issues\n", name, verdict)
		return result.OK
	}
	fmt.Printf("%-20s : %s, %d issue(s)\n", name, verdict, len(result.Issues))
	for _, issue := range result.Issues {
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	return result.OK
}

// vitalSignsComposition builds by hand a composition that passes both the RM
// floor and vital_signs.opt: an encounter holding one blood-pressure
// OBSERVATION with a systolic reading, the patient's position and the device
// used. It matches the default fixture of the validate-from-json example.
// Compare the nesting with the structure tree the template-explore example
// prints for the same template.
func vitalSignsComposition() (*rm.Composition, error) {
	// A composition's category is a coded text from the openEHR terminology.
	// The SDK ships that vocabulary in openehr/terminology, so look the label
	// up there instead of typing "event" next to the code; the two then
	// cannot drift apart.
	eventRubric, ok := terminology.CompositionCategory.Rubric("433")
	if !ok {
		return nil, errors.New(`code 433 ("event") is not in the composition_category vocabulary`)
	}
	return &rm.Composition{
		// On an archetype root the node id is the archetype id itself.
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		// The Reference Model requires archetype_details on every archetype
		// root (the COMPOSITION and each ENTRY); the template pass does not
		// check it, the RM floor does.
		ArchetypeDetails: &rm.Archetyped{
			ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.encounter.v1"},
			TemplateID:  &rm.TemplateID{Value: "vital_signs"},
			RMVersion:   rm.Release,
		},
		Category: rm.DVCodedText{
			Value: eventRubric,
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
		Content: []rm.ContentItem{
			&rm.Observation{
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
				// Below the archetype root, node ids are the archetype's own
				// at-codes; the template-explore output shows which at-code
				// sits where.
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
								// An ELEMENT carries exactly one of a value
								// or a null flavour: at1001 is the
								// archetype's local code for "Sitting".
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
						// A CLUSTER holds at least one item. at0001 is the
						// device archetype's "Device name" element.
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
			},
		},
	}, nil
}
