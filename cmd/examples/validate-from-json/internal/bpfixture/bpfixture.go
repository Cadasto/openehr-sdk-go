// Package bpfixture builds the validate-from-json example's default fixture,
// testdata/minimal_blood_pressure.json: a hand-made blood-pressure composition
// that is valid against the vendored vital_signs.opt.
//
// gen_fixture.go writes what Document returns, and a test in the example's
// directory fails when the committed file differs from it, so the builder and
// the file cannot drift apart.
package bpfixture

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Document returns the bytes of the fixture file: the composition as
// canonical JSON, with the members of every object sorted by name.
//
// It checks the document it returns, not the composition it built.
// validation.ValidateJSON decodes the encoded bytes and checks, in one call,
// the members the document leaves out, the constraints of vital_signs.opt and
// the RM floor. Document returns an error listing every issue when the
// document does not decode or the call reports an error on it.
func Document() ([]byte, error) {
	compiled, err := compileVitalSigns()
	if err != nil {
		return nil, err
	}
	comp, err := composition()
	if err != nil {
		return nil, err
	}
	b, err := canjson.Marshal(comp)
	if err != nil {
		return nil, fmt.Errorf("encode the composition: %w", err)
	}
	if b, err = sortMembers(b); err != nil {
		return nil, err
	}
	if err := check(b, compiled); err != nil {
		return nil, err
	}
	return b, nil
}

// sortMembers re-encodes doc with the members of every object sorted by name.
// canjson writes "_type" first and the rest in model order; the committed
// fixture has always held them sorted, and keeping that layout keeps the file
// stable.
func sortMembers(doc []byte) ([]byte, error) {
	var v any
	if err := jsonv2.Unmarshal(doc, &v); err != nil {
		return nil, fmt.Errorf("re-read the encoded composition: %w", err)
	}
	b, err := jsonv2.Marshal(v, jsonv2.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("sort the members of the encoded composition: %w", err)
	}
	return b, nil
}

// check reports, as one error listing every issue, whether doc fails to
// decode into a COMPOSITION, fails the template constraints or the RM floor,
// or leaves out a member the Reference Model makes mandatory.
func check(doc []byte, compiled *templatecompile.Compiled) error {
	var back rm.Composition
	result := validation.ValidateJSON(doc, &back, compiled)
	if result.OK {
		return nil
	}
	var b strings.Builder
	b.WriteString("the encoded composition is not valid:")
	for _, i := range result.Issues {
		fmt.Fprintf(&b, "\n  %s [%s] %s", i.Path, i.Code, i.Detail)
	}
	return errors.New(b.String())
}

// compileVitalSigns reads the vendored vital_signs.opt into the operational
// template model, by default reading a node type it does not know as a leaf
// without its subtree (canxml.WithStrict and templatecompile.WithStrict
// refuse such readings), and compiles it.
func compileVitalSigns() (*templatecompile.Compiled, error) {
	path := fixtures.TemplateOptForName("vital_signs")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OPT %q: %w", path, err)
	}
	opt, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("read OPT %q: %w", path, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		return nil, fmt.Errorf("compile OPT %q: %w", path, err)
	}
	return c, nil
}

func composition() (*rm.Composition, error) {
	// Take the category rubric and terminology id from the bundled openEHR
	// terminology instead of typing them here, so each code has one home. A
	// miss means the terminology no longer carries `event`; stop rather than
	// write a fixture with an empty Category.value.
	eventRubric, ok := terminology.CompositionCategory.Rubric("433")
	if !ok {
		return nil, errors.New("code 433 (event) is not a member of the pinned composition_category group")
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
		Content: []rm.ContentItem{observation()},
	}, nil
}

func observation() *rm.Observation {
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
						// An ELEMENT carries exactly one of a value or a null
						// flavour: at1001 is the archetype's local code for
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
