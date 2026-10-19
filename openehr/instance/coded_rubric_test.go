package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// TestREQ107_CodedDefaultsCarryTheOpenEHRRubric is the REQ-107 check that
// each coded RM default carries the openEHR rubric of its code as its
// text, spelled out here rather than read back from the terminology
// tables: a COMPOSITION's category "event", an EVENT_CONTEXT's setting
// "other care", an ISM_TRANSITION's current state "initial", an
// INTERVAL_EVENT's math function "mean", and an ELEMENT's null flavour
// "no information". The OPT leaves each attribute silent. It holds under
// every policy, both value fills and both compile modes.
func TestREQ107_CodedDefaultsCarryTheOpenEHRRubric(t *testing.T) {
	cases := []struct {
		name, code, text string
		opt              string
		coded            func(out any) rm.DVCodedText
	}{
		{
			name: "COMPOSITION category", code: "433", text: "event",
			opt:   optTemplate("COMPOSITION"),
			coded: func(out any) rm.DVCodedText { return out.(*rm.Composition).Category },
		},
		{
			name: "EVENT_CONTEXT setting", code: "238", text: "other care",
			opt:   optTemplate("COMPOSITION"),
			coded: func(out any) rm.DVCodedText { return out.(*rm.Composition).Context.Setting },
		},
		{
			name: "ISM_TRANSITION current_state", code: "524", text: "initial",
			opt:   optTemplate("ACTION"),
			coded: func(out any) rm.DVCodedText { return out.(*rm.Action).IsmTransition.CurrentState },
		},
		{
			name: "INTERVAL_EVENT math_function", code: "146", text: "mean",
			opt:   optTemplate("INTERVAL_EVENT", intervalEventAttrs...),
			coded: func(out any) rm.DVCodedText { return out.(*rm.IntervalEvent[rm.ItemStructure]).MathFunction },
		},
		{
			name: "ELEMENT null_flavour", code: "271", text: "no information",
			opt: optTemplate("ELEMENT"),
			coded: func(out any) rm.DVCodedText {
				if nf := out.(*rm.Element).NullFlavour; nf != nil {
					return *nf
				}
				return rm.DVCodedText{}
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				opts.Territory, opts.Composer = "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					got := tc.coded(out)
					want := rm.CodePhrase{CodeString: tc.code, TerminologyID: rm.TerminologyID{Value: "openehr"}}
					if got.DefiningCode != want || got.Value != tc.text {
						t.Errorf("%s = %q %+v, want %q openehr::%s", tc.name, got.Value, got.DefiningCode, tc.text, tc.code)
					}
				})
			}
		}
	}
}

// optCodedTextListing is a DV_CODED_TEXT whose text the OPT limits with a
// C_STRING listing text, and whose defining code it pins to codes of
// terminologyID with a C_CODE_PHRASE.
func optCodedTextListing(text, terminologyID string, codes ...string) string {
	return optNode("DV_CODED_TEXT", "", optStringAttr("value", "<list>"+text+"</list>"),
		optSingle("defining_code", optCodePhrase(terminologyID, codes...)))
}

// TestREQ107_PinnedCodeKeepsTheTextTheOPTLists is the REQ-107 check of
// Precedence 1 on an openEHR code the OPT pins on a COMPOSITION's
// category, an ELEMENT's null_flavour or a PARTY_RELATED's relationship,
// where the OPT also limits the coded text's value to a list that leaves
// out the code's rubric: the generator keeps the listed text the walk
// wrote and the pinned code, and does not write the rubric over the text.
// The output passes the RM floor and the template validator, under every
// policy, both value fills and both compile modes. Where the text is free,
// TestREQ034_REQ107_PinnedCategoryCarriesItsRubric,
// TestREQ034_REQ107_OPTFilledNullFlavourCarriesItsRubric and
// TestREQ107_RelationshipKeepsTheOPTCode check that the rubric is written.
func TestREQ107_PinnedCodeKeepsTheTextTheOPTLists(t *testing.T) {
	cases := []struct {
		name, code, text string
		group            *terminology.Group
		opt              string
		coded            func(t *testing.T, out any) rm.DVCodedText
	}{
		{
			name: "PARTY_RELATED relationship", code: "10", text: "Mum",
			group: terminology.SubjectRelationship,
			opt:   partyRelatedSubject(optSingle("relationship", optCodedTextListing("Mum", terminology.ID, "10"))),
			coded: subjectRelationship,
		},
		{
			name: "COMPOSITION category", code: "433", text: "Event",
			group: terminology.CompositionCategory,
			opt:   optTemplate("COMPOSITION", optSingle("category", optCodedTextListing("Event", terminology.ID, "433"))),
			coded: func(t *testing.T, out any) rm.DVCodedText { return generatedComposition(t, out).Category },
		},
		{
			name: "ELEMENT null_flavour", code: "271", text: "nothing known",
			group: terminology.NullFlavours,
			opt:   optTemplate("ELEMENT", optSingle("null_flavour", optCodedTextListing("nothing known", terminology.ID, "271"))),
			coded: func(t *testing.T, out any) rm.DVCodedText {
				t.Helper()
				nf := out.(*rm.Element).NullFlavour
				if nf == nil {
					t.Fatal("ELEMENT.null_flavour absent, want the pinned code")
				}
				return *nf
			},
		},
	}
	for _, tc := range cases {
		if rubric, ok := tc.group.Rubric(tc.code); !ok || rubric == tc.text {
			t.Fatalf("%s: rubric of %s = %q (%t), want one the listed text %q leaves out", tc.name, tc.code, rubric, ok, tc.text)
		}
		want := rm.CodePhrase{CodeString: tc.code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				opts.Territory, opts.Composer = "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					checkCodedText(t, tc.name, tc.coded(t, out), tc.text, want)
					checkValid(t, out, c)
				})
			}
		}
	}
}
