package instance_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// elementCodedName is the name of a generated ELEMENT root, which must be a
// DV_CODED_TEXT.
func elementCodedName(t *testing.T, out any) rm.DVCodedText {
	t.Helper()
	el, ok := out.(*rm.Element)
	if !ok {
		t.Fatalf("root is %T, want *rm.Element", out)
	}
	switch name := el.Name.(type) {
	case rm.DVCodedText:
		return name
	case *rm.DVCodedText:
		return *name
	default:
		t.Fatalf("ELEMENT name is %T, want a DV_CODED_TEXT", el.Name)
		return rm.DVCodedText{}
	}
}

// optCodedNameListing is a DV_CODED_TEXT whose value a C_STRING limits to
// texts and whose defining code a C_CODE_PHRASE limits to codes of
// terminologyID.
func optCodedNameListing(texts []string, terminologyID string, codes ...string) string {
	var list string
	for _, text := range texts {
		list += "<list>" + text + "</list>"
	}
	return optNode("DV_CODED_TEXT", "", optStringAttr("value", list),
		optSingle("defining_code", optCodePhrase(terminologyID, codes...)))
}

// TestREQ107_CodedNameTakesTheCodeOfItsPinnedText is the REQ-107 term-text
// check on a locatable's name coded in a terminology other than openehr,
// whose value a C_STRING limits and whose defining code a C_CODE_PHRASE
// limits to a list of codes. Where the term text of the code the walk wrote
// is one the C_STRING rejects, the name takes the first listed code whose
// term text the C_STRING admits, with that text: local codes, codes of
// another terminology defined under <terminology id>::<code string>, and
// codes defined under the bare code string. So the name's code and text
// name one term, as a CDR that checks the text against the term reads it.
// Where the C_STRING admits the term text of no listed code, the name
// keeps the text the walk wrote and a listed code. It holds under every
// policy and both value fills, and every root validates.
func TestREQ107_CodedNameTakesTheCodeOfItsPinnedText(t *testing.T) {
	localTerms := `<term_definitions code="at0001"><items id="text">First</items></term_definitions>` +
		`<term_definitions code="at0002"><items id="text">Second</items></term_definitions>` +
		`<term_definitions code="at0003"><items id="text">Third</items></term_definitions>`
	qualifiedTerms := `<term_definitions code="SNOMED-CT::111"><items id="text">One</items></term_definitions>` +
		`<term_definitions code="SNOMED-CT::222"><items id="text">Two</items></term_definitions>`
	bareTerms := bareTerm("111", "One") + bareTerm("222", "Two")
	terms := map[string]string{
		"at0001": "First", "at0002": "Second", "at0003": "Third",
		"111": "One", "222": "Two",
	}
	cases := []struct {
		name  string
		opt   string
		codes []string // the codes the name may carry
		// exampleCode is the code it carries under ExampleFill.
		exampleCode string
		// text is the text it carries where the C_STRING admits no listed
		// code's term text, and "" where it carries its code's term text.
		text string
	}{
		{
			name:  "local codes, the second code's text pinned",
			opt:   optTemplate("ELEMENT", localTerms, optSingle("name", optCodedNameListing([]string{"Second"}, "local", "at0001", "at0002"))),
			codes: []string{"at0002"}, exampleCode: "at0002",
		},
		{
			name:  "codes of another terminology under the qualified key",
			opt:   optTemplate("ELEMENT", qualifiedTerms, optSingle("name", optCodedNameListing([]string{"Two"}, "SNOMED-CT", "111", "222"))),
			codes: []string{"222"}, exampleCode: "222",
		},
		{
			name:  "codes of another terminology under the bare code",
			opt:   optTemplate("ELEMENT", bareTerms, optSingle("name", optCodedNameListing([]string{"Two"}, "SNOMED-CT", "111", "222"))),
			codes: []string{"222"}, exampleCode: "222",
		},
		{
			// The C_STRING admits the second and third code's texts: the
			// first of them in the list wins over the first listed code,
			// while a drawn code whose text it admits keeps its own.
			name: "local codes, two texts pinned",
			opt: optTemplate("ELEMENT", localTerms, optSingle("name",
				optCodedNameListing([]string{"Second", "Third"}, "local", "at0001", "at0002", "at0003"))),
			codes: []string{"at0002", "at0003"}, exampleCode: "at0002",
		},
		{
			name: "local codes, no listed code's text pinned",
			opt: optTemplate("ELEMENT", localTerms, optSingle("name",
				optCodedNameListing([]string{"Neither"}, "local", "at0001", "at0002"))),
			codes: []string{"at0001", "at0002"}, exampleCode: "at0001", text: "Neither",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					name := elementCodedName(t, out)
					code := name.DefiningCode.CodeString
					if !slices.Contains(tc.codes, code) {
						t.Errorf("ELEMENT name = %q coded %s, want a code of %q", name.Value, code, tc.codes)
					}
					if opts.ValueFill == instance.ExampleFill && code != tc.exampleCode {
						t.Errorf("ELEMENT name under ExampleFill = %q coded %s, want the code %s", name.Value, code, tc.exampleCode)
					}
					want := tc.text
					if want == "" {
						want = terms[code]
					}
					if name.Value != want {
						t.Errorf("ELEMENT name = %q coded %s, want the text %q", name.Value, code, want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
