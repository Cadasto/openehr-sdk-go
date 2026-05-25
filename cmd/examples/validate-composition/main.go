// Example: parse an OPT, compile it, and validate an in-memory
// *rm.Composition with openehr/validation (REQ-102). Demonstrates
// the clinical building-block path — no transport, no auth, no
// serialize on the validation path: template → compiled OPT → RM
// instance → Issue list.
//
// Note: templatecompile.Compile is internal; this example lives in
// the SDK module and is the supported v1 call shape. External repos
// cannot call ValidateComposition until template.Compile is
// re-exported (see docs/adr/0005-compiled-template-foundation.md).
//
// Run:
//
//	go run ./cmd/examples/validate-composition [path/to/template.opt]
//	go run ./cmd/examples/validate-composition -invalid
//
// With no arguments, uses the vendored vital_signs.opt fixture and
// validates a hand-built composition that matches that template.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

func main() {
	invalid := flag.Bool("invalid", false, "wipe Category to demonstrate a required-issue failure")
	flag.Parse()
	optPath := resolveOPTPath(flag.Args())

	opt, err := template.ParseFileStrict(optPath)
	if err != nil {
		log.Fatalf("ParseFileStrict %q: %v", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("Compile %q: %v", optPath, err)
	}
	fmt.Printf("template    : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))
	fmt.Printf("compiled    : root %s\n", compiled.Root().RMTypeName())

	comp := exampleVitalSignsComposition()
	if *invalid {
		comp.Category = rm.DVCodedText{}
		fmt.Println("mutation    : cleared Category (expect required at /category)")
	}

	r := validation.ValidateComposition(comp, compiled)
	if r.OK {
		fmt.Println("result      : OK — no issues")
		return
	}
	fmt.Printf("result      : %d issue(s)\n", len(r.Issues))
	for _, issue := range r.Issues {
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	os.Exit(1)
}

func resolveOPTPath(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot locate example source path")
	}
	repoRoot := filepath.Join(filepath.Dir(here), "..", "..", "..")
	return filepath.Join(repoRoot, "openehr", "template", "testdata", "vital_signs.opt")
}

// exampleVitalSignsComposition is a minimal structurally-complete
// composition for vital_signs.opt — mirrors the positive fixture in
// openehr/validation/composition_test.go.
func exampleVitalSignsComposition() *rm.Composition {
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
