package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_CodedDefaultsCarryTheOpenEHRRubric is the REQ-107 check that
// each coded RM default carries the openEHR rubric of its code as its
// text, spelled out here rather than read back from the terminology
// tables: a COMPOSITION's category "event", an EVENT_CONTEXT's setting
// "other care", an ISM_TRANSITION's current state "initial", an
// INTERVAL_EVENT's math function "mean", and an ELEMENT's null flavour
// "no information". The OPT leaves each attribute silent. It holds under
// both policies, both value fills and both compile modes.
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
