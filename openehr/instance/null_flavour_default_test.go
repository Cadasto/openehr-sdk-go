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
