// Example: build a composition in memory as plain Reference Model structs,
// compile an operational template (OPT), and validate the composition against
// it. This is the smallest validation path: no JSON, no HTTP, just typed RM
// values, a compiled template, and the list of issues the validator returns.
//
// Runs offline. With no argument it uses the vendored vital_signs.opt fixture
// and a hand-built composition that matches it; -invalid clears a required
// attribute first, so the validator has something to report:
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
	invalid := flag.Bool("invalid", false, "clear the composition's category first, so the validator reports a missing required attribute")
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

	// Step 2: compile it. The validator works from the compiled template.
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}
	fmt.Printf("template    : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))
	fmt.Printf("compiled    : root %s\n", compiled.Root().RMTypeName())

	// Step 3: the composition under test.
	comp, err := vitalSignsComposition()
	if err != nil {
		return err
	}
	if *invalid {
		// Every COMPOSITION must carry a category; the zero value counts as
		// absent, so this is the simplest way to provoke a "required" issue.
		comp.Category = rm.DVCodedText{}
		fmt.Println("mutation    : cleared Category (expect required at /category)")
	}

	// Step 4: validate. The template drives the walk: for every node it
	// declares, the validator reads the matching RM attribute and checks
	// presence, cardinality, RM type and archetype identity. It collects every
	// issue in one pass rather than stopping at the first.
	result := validation.ValidateComposition(comp, compiled)
	if result.OK {
		fmt.Println("result      : OK — no issues")
		return nil
	}
	fmt.Printf("result      : %d issue(s)\n", len(result.Issues))
	for _, issue := range result.Issues {
		// Path says where, Code is the stable identifier a program branches
		// on, and Detail is the explanation for people.
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	return errors.New("composition does not conform to the OPT")
}

// vitalSignsComposition builds by hand a composition that satisfies
// vital_signs.opt: an encounter holding one blood-pressure OBSERVATION with a
// systolic reading. It mirrors the passing fixture in the validation package's
// own tests. Compare the nesting with the structure tree the template-explore
// example prints for the same template.
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
	}, nil
}
