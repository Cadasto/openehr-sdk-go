package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// TestREQ107_NamedNullFlavourWithoutCodeGetsNoInformation is the REQ-107
// check that an ELEMENT null_flavour the OPT names without a code, as an
// optional attribute with no child, as a DV_CODED_TEXT with no constraint,
// or as a C_CODE_PHRASE under openehr with an empty code list, takes the
// RM default openehr 271|no information|, not the walk's at0000
// placeholder. It holds under both policies, both value fills and both
// compile modes, and the output passes the RM floor and the template
// validator.
func TestREQ107_NamedNullFlavourWithoutCodeGetsNoInformation(t *testing.T) {
	cases := []struct {
		name string
		attr string
	}{
		{name: "optional, no child", attr: optOptionalSingle("null_flavour")},
		{name: "bare DV_CODED_TEXT", attr: optSingle("null_flavour", optNode("DV_CODED_TEXT", ""))},
		{name: "empty-list C_CODE_PHRASE openehr", attr: optSingle("null_flavour", optCodedText(terminology.ID))},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", tc.attr), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					el := out.(*rm.Element)
					if el.NullFlavour == nil {
						t.Fatalf("ELEMENT.null_flavour absent, want openehr::271")
					}
					checkOpenEHRCode(t, "ELEMENT.null_flavour", *el.NullFlavour, terminology.NullFlavours, "271")
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}

// TestREQ107_ProhibitedNullFlavourTakesAValue is the REQ-107 check that an
// ELEMENT whose OPT prohibits null_flavour, and whose walk wrote no value,
// takes a value built as for an ELEMENT.value the OPT leaves silent: the
// RM rule that an ELEMENT carry exactly one of value and null_flavour wins
// over Minimal. A null_reason the walk wrote then goes, because an ELEMENT
// with a value carries none. Where the OPT prohibits value as well, the RM
// rule wins over that prohibition too, with the null flavour (the
// both-prohibited row of TestREQ107_RMDefaultsYieldToTheOPT).
func TestREQ107_ProhibitedNullFlavourTakesAValue(t *testing.T) {
	cases := []struct {
		name  string
		attrs []string
	}{
		{name: "null_flavour prohibited", attrs: []string{optProhibitedSingle("null_flavour")}},
		{
			name:  "null_flavour prohibited, null_reason named",
			attrs: []string{optProhibitedSingle("null_flavour"), optOptionalSingle("null_reason")},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", tc.attrs...), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					el := out.(*rm.Element)
					if el.NullFlavour != nil {
						t.Errorf("ELEMENT.null_flavour = %+v, want none: the OPT prohibits it", el.NullFlavour)
					}
					if el.Value == nil || rm.IsTypedNil(el.Value) {
						t.Fatal("ELEMENT.value absent, want the value built in place of the null flavour")
					}
					if el.NullReason != nil {
						t.Errorf("ELEMENT.null_reason = %+v, want none beside a value", el.NullReason)
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}

// TestREQ107_BothProhibitedNullFlavourIsWalked is the REQ-107 check that an
// ELEMENT whose OPT prohibits both value and null_flavour, which the RM
// rule Inv_null_flavour_indicated then needs, has its null_flavour written
// as if the OPT allowed it: the walk takes the prohibited attribute's own
// child, so a pinned code stays, and the RM default 271 applies only where
// that leaves no code and the child admits 271. It holds under both
// policies, both value fills and both compile modes.
func TestREQ107_BothProhibitedNullFlavourIsWalked(t *testing.T) {
	cases := []struct {
		name string
		nf   string
		want rm.CodePhrase
	}{
		{
			name: "no child", nf: optProhibitedSingle("null_flavour"),
			want: rm.CodePhrase{CodeString: "271", TerminologyID: rm.TerminologyID{Value: terminology.ID}},
		},
		{
			name: "a C_CODE_PHRASE pinning 253", nf: optProhibitedSingle("null_flavour", optCodedText(terminology.ID, "253")),
			want: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: terminology.ID}},
		},
		{
			name: "an empty-list C_CODE_PHRASE under openehr", nf: optProhibitedSingle("null_flavour", optCodedText(terminology.ID)),
			want: rm.CodePhrase{CodeString: "271", TerminologyID: rm.TerminologyID{Value: terminology.ID}},
		},
		{
			name: "an empty-list C_CODE_PHRASE under local", nf: optProhibitedSingle("null_flavour", optCodedText("local")),
			want: localAt0000,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optProhibitedSingle("value"), tc.nf), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					el := out.(*rm.Element)
					if el.Value != nil {
						t.Errorf("ELEMENT.value = %#v, want none: the OPT prohibits it", el.Value)
					}
					if el.NullFlavour == nil {
						t.Fatalf("ELEMENT.null_flavour absent, want %+v", tc.want)
					}
					checkCode(t, "ELEMENT.null_flavour", el.NullFlavour.DefiningCode, tc.want)
				})
			}
		}
	}
}

// TestREQ107_ValueGivesWayToARequiredNullAttribute is the REQ-107 check of
// the ELEMENT precedence where the OPT allows both a value and a
// null_flavour: the value gives way when the OPT requires the null_flavour
// or the null_reason but not the value, so the ELEMENT has no value and
// keeps or takes its null_flavour (271 when the walk wrote none), and the
// value wins otherwise, the null attributes then dropped. Both directions,
// and an OPT that requires both, under both policies, both value fills and
// both compile modes. Where the OPT does not contradict itself, the
// template validator finds no error.
func TestREQ107_ValueGivesWayToARequiredNullAttribute(t *testing.T) {
	optionalText := optOptionalSingleOver("value", optNode("DV_TEXT", ""))
	cases := []struct {
		name      string
		attrs     []string
		wantValue bool
		wantNF    string // the null flavour code, "" for none
		wantNR    bool   // a null_reason
		contra    bool   // the OPT contradicts itself
	}{
		{
			name: "null_flavour required, pinned 253", attrs: []string{optionalText, optSingle("null_flavour", optCodedText(terminology.ID, "253"))},
			wantNF: "253",
		},
		{
			name: "null_flavour required, no child", attrs: []string{optionalText, optSingle("null_flavour")},
			wantNF: "271",
		},
		{
			name: "null_reason required", attrs: []string{optionalText, optSingle("null_reason", optNode("DV_TEXT", ""))},
			wantNF: "271", wantNR: true,
		},
		{
			name: "neither required", attrs: []string{optionalText, optOptionalSingleOver("null_flavour", optCodedText(terminology.ID, "253"))},
			wantValue: true,
		},
		{
			name: "value and null_flavour required", attrs: []string{optSingle("value", optNode("DV_TEXT", "")), optSingle("null_flavour", optCodedText(terminology.ID, "253"))},
			wantValue: true, contra: true,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", tc.attrs...), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					el := out.(*rm.Element)
					if has := el.Value != nil && !rm.IsTypedNil(el.Value); has != tc.wantValue {
						t.Errorf("ELEMENT.value present = %t, want %t", has, tc.wantValue)
					}
					gotNF := ""
					if el.NullFlavour != nil {
						gotNF = el.NullFlavour.DefiningCode.CodeString
					}
					if gotNF != tc.wantNF {
						t.Errorf("ELEMENT.null_flavour code = %q, want %q", gotNF, tc.wantNF)
					}
					if has := el.NullReason != nil; has != tc.wantNR {
						t.Errorf("ELEMENT.null_reason present = %t, want %t", has, tc.wantNR)
					}
					noFloorErrors(t, out)
					if tc.contra {
						return
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}
