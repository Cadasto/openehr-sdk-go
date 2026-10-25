package instance_test

import (
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"slices"
	"strings"
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
	var list strings.Builder
	for _, text := range texts {
		list.WriteString("<list>" + text + "</list>")
	}
	return optNode("DV_CODED_TEXT", "", optStringAttr("value", list.String()),
		optSingle("defining_code", optCodePhrase(terminologyID, codes...)))
}

// localNameTerms are the term definitions of at0001, at0002 and at0003,
// with the texts First, Second and Third.
const localNameTerms = `<term_definitions code="at0001"><items id="text">First</items></term_definitions>` +
	`<term_definitions code="at0002"><items id="text">Second</items></term_definitions>` +
	`<term_definitions code="at0003"><items id="text">Third</items></term_definitions>`

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
			opt:   optTemplate("ELEMENT", localNameTerms, optSingle("name", optCodedNameListing([]string{"Second"}, "local", "at0001", "at0002"))),
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
			// The C_STRING admits the second and third code's texts: under
			// ExampleFill the first of them in the list wins over the first
			// listed code, while a drawn code whose text it admits keeps
			// its own, which
			// TestREQ107_CodedNameKeepsADrawnCodeWhoseTextThePinAdmits
			// holds over many seeds.
			name: "local codes, two texts pinned",
			opt: optTemplate("ELEMENT", localNameTerms, optSingle("name",
				optCodedNameListing([]string{"Second", "Third"}, "local", "at0001", "at0002", "at0003"))),
			codes: []string{"at0002", "at0003"}, exampleCode: "at0002",
		},
		{
			name: "local codes, no listed code's text pinned",
			opt: optTemplate("ELEMENT", localNameTerms, optSingle("name",
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

// TestREQ107_CodedNameKeepsADrawnCodeWhoseTextThePinAdmits is the REQ-107
// term-text check that a coded name takes another listed code only where
// the C_STRING rejects the term text of the code the walk wrote. The
// C_CODE_PHRASE lists at0001, at0002 and at0003, and the C_STRING admits
// the term texts of at0002 and at0003 only. Under RandomFill the walk
// draws a listed code, so over many seeds the name carries both at0002
// and at0003, each with its term text, and never at0001: a drawn at0003
// keeps its code rather than taking at0002, the first listed code whose
// term text the C_STRING admits. It holds under every policy, and every
// root validates.
func TestREQ107_CodedNameKeepsADrawnCodeWhoseTextThePinAdmits(t *testing.T) {
	const seeds = 64
	opt := optTemplate("ELEMENT", localNameTerms, optSingle("name",
		optCodedNameListing([]string{"Second", "Third"}, "local", "at0001", "at0002", "at0003")))
	admitted := map[string]string{"at0002": "Second", "at0003": "Third"}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, policy := range everyPolicy {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, policy, instance.RandomFill), func(t *testing.T) {
				carried := make(map[string]int)
				for seed := range uint64(seeds) {
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy:      policy,
						ValueFill:   instance.RandomFill,
						ValueSource: mrand.NewPCG(seed, 2),
						Now:         defaultsNow,
					})
					if err != nil {
						t.Fatalf("seed=%d: Generate: %v", seed, err)
					}
					name := elementCodedName(t, out)
					code := name.DefiningCode.CodeString
					carried[code]++
					if want, ok := admitted[code]; !ok || name.Value != want {
						t.Errorf("seed=%d: ELEMENT name = %q coded %s, want at0002 \"Second\" or at0003 \"Third\"", seed, name.Value, code)
					}
					checkValid(t, out, c)
				}
				for _, code := range slices.Sorted(maps.Keys(admitted)) {
					if carried[code] == 0 {
						t.Errorf("over %d seeds the ELEMENT name never carried %s, whose term text %q the C_STRING admits; codes carried: %v", seeds, code, admitted[code], carried)
					}
				}
			})
		}
	}
}

// TestREQ107_CodedValueKeepsTheCodeTheWalkWrote is the REQ-107 term-text
// check on the scope of the coded-name rule above: a coded text that is
// not a name, here an ELEMENT's value, whose C_STRING rejects the term
// text of the code the walk wrote keeps that code and the text the walk
// wrote (Precedence 1), even where another listed code's term text is the
// one the C_STRING admits. Under ExampleFill the walk writes the first
// listed code. It holds under every policy and both value fills, and every
// root validates.
func TestREQ107_CodedValueKeepsTheCodeTheWalkWrote(t *testing.T) {
	opt := optTemplate("ELEMENT",
		`<term_definitions code="at0001"><items id="text">First</items></term_definitions>`+
			`<term_definitions code="at0002"><items id="text">Second</items></term_definitions>`,
		optSingle("value", optCodedNameListing([]string{"Second"}, "local", "at0001", "at0002")))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range defaultsOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				value := elementCodedText(t, out)
				code := value.DefiningCode.CodeString
				if value.Value != "Second" || !slices.Contains([]string{"at0001", "at0002"}, code) {
					t.Errorf("ELEMENT value = %q coded %s, want the text \"Second\" and a listed code", value.Value, code)
				}
				if opts.ValueFill == instance.ExampleFill && code != "at0001" {
					t.Errorf("ELEMENT value under ExampleFill = %q coded %s, want the first listed code at0001", value.Value, code)
				}
				checkValid(t, out, c)
			})
		}
	}
}
