package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Texts the synthetic OPTs below give SNOMED-CT::248152002 under its bare
// code string, the key some OPTs give a term of another terminology.
const (
	bareFemale      = "Female (bare)"
	innerBareFemale = "Female (inner, bare)"
)

// bareTerm is the term definition of code with text, keyed by the code
// string alone.
func bareTerm(code, text string) string {
	return `<term_definitions code="` + code + `"><items id="text">` + text + `</items></term_definitions>`
}

// nestedBareTermOPT is a CLUSTER template whose archetype defines outer,
// holding an inner CLUSTER archetype that defines at0001 and inner. The
// inner archetype holds an ELEMENT at0001 whose value is coded
// SNOMED-CT::248152002.
func nestedBareTermOPT(outer, inner string) string {
	innerTerms := `<term_definitions code="at0001"><items id="text">Inner element</items></term_definitions>` + inner
	element := optNode("ELEMENT", "at0001", optSingle("value", optCodedText("SNOMED-CT", "248152002")))
	root := optArchetypeRoot("CLUSTER", "openEHR-EHR-CLUSTER.inner.v1", innerTerms, optMultiple("items", element))
	return optTemplate("CLUSTER", outer, optMultiple("items", root))
}

// TestREQ107_ExternalCodedTextTakesTheTermUnderItsBareCode is the REQ-107
// term-text check that a DV_CODED_TEXT coded in a terminology other than
// openehr and local takes the text of the term its own archetype, the
// nearest enclosing archetype root, defines under the bare code string,
// where that archetype gives no text under <terminology id>::<code
// string>: as a DV_CODED_TEXT with a code constraint and as one a
// C_CODE_PHRASE constrains as a whole. A term under the qualified key wins
// over one under the bare code, whichever the OPT lists first. A C_STRING
// that rejects the bare term's text keeps the text the walk wrote
// (Precedence 1). A bare term only an enclosing archetype defines names no
// term of the coded text's own archetype, so the walk's text stays, while
// a bare term the own archetype defines wins over a qualified one an
// enclosing archetype defines. A code string that is an at-code, such as
// at0001.1, names a term of the archetype's own and takes no bare term's
// text. It holds under every policy and both value
// fills, in every language asked for, and every root validates.
func TestREQ107_ExternalCodedTextTakesTheTermUnderItsBareCode(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		coded func(t *testing.T, out any) rm.DVCodedText
		want  string
	}{
		{
			name:  "ELEMENT value",
			opt:   optTemplate("ELEMENT", bareTerm("248152002", bareFemale), optSingle("value", optCodedText("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  bareFemale,
		},
		{
			name:  "ELEMENT value a C_CODE_PHRASE constrains as a whole",
			opt:   optTemplate("ELEMENT", bareTerm("248152002", bareFemale), optSingle("value", optCodedTextWhole("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  bareFemale,
		},
		{
			name: "ELEMENT value whose code has a qualified and a bare term",
			opt: optTemplate("ELEMENT", bareTerm("248152002", bareFemale), snomedFemale(femaleTerm),
				optSingle("value", optCodedText("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  femaleTerm,
		},
		{
			name: "ELEMENT value whose qualified term comes first",
			opt: optTemplate("ELEMENT", snomedFemale(femaleTerm), bareTerm("248152002", bareFemale),
				optSingle("value", optCodedText("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  femaleTerm,
		},
		{
			name: "ELEMENT value whose C_STRING rejects the bare term text",
			opt: optTemplate("ELEMENT", bareTerm("248152002", bareFemale), optSingle("value",
				optCodedTextListing("Woman", "SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  "Woman",
		},
		{
			// An at-code under its bare key is the archetype's own term,
			// whatever terminology the coded text names; a specialised
			// at-code carries dot extensions.
			name: "ELEMENT value coded in another terminology with a specialised at-code",
			opt: optTemplate("ELEMENT", bareTerm("at0001.1", "Own term"),
				optSingle("value", optCodedText("ICD10", "at0001.1"))),
			coded: elementCodedText,
			want:  walkText,
		},
		{
			name:  "ELEMENT value whose bare term only an enclosing archetype defines",
			opt:   nestedBareTermOPT(bareTerm("248152002", bareFemale), ""),
			coded: snomedCodedText("248152002"),
			want:  walkText,
		},
		{
			name:  "ELEMENT value whose own archetype defines a bare term and an enclosing one a qualified term",
			opt:   nestedBareTermOPT(snomedFemale(femaleTerm), bareTerm("248152002", innerBareFemale)),
			coded: snomedCodedText("248152002"),
			want:  innerBareFemale,
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, language := range []string{"", "en", "nl"} {
			for _, opts := range defaultsOptions() {
				opts.Language = language
				t.Run(fmt.Sprintf("%s/lang=%q/%v/%v", tc.name, language, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := tc.coded(t, out); got.Value != tc.want {
						t.Errorf("%s = %q %+v, want text %q", tc.name, got.Value, got.DefiningCode, tc.want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
