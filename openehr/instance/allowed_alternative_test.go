package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// optCount is a DV_COUNT node, prohibited when prohibited is set, whose
// magnitude the OPT constrains with a C_INTEGER carrying body.
func optCount(prohibited bool, body string) string {
	upper := -1
	if prohibited {
		upper = 0
	}
	return optOccurring("C_COMPLEX_OBJECT", "DV_COUNT", "", 0, upper,
		optSingle("magnitude", optPrimitive("INTEGER", "C_INTEGER", body)))
}

// optCodedTextOccurring is a DV_CODED_TEXT node with occurrences
// lower..upper whose defining_code the OPT constrains to terminologyID,
// listing codes.
func optCodedTextOccurring(lower, upper int, terminologyID string, codes ...string) string {
	return optOccurring("C_COMPLEX_OBJECT", "DV_CODED_TEXT", "", lower, upper,
		optSingle("defining_code", optCodePhrase(terminologyID, codes...)))
}

// TestREQ107_ProhibitedFirstAlternativeSuppliesNothing is the REQ-107 check
// that where the first OPT alternative of a single attribute is prohibited
// (occurrences 0..0), the generator reads the next allowed one, the
// alternative the walk builds, wherever it consults the OPT after the
// walk: an interval bound's constraint when it orders the bounds, the
// EVENT_CONTEXT node whose setting constraint the setting default yields
// to, a coded default's own constraint, and the code an ISM_TRANSITION's
// current state takes from the OPT. A prohibited alternative never
// supplies a constraint or an example.
func TestREQ107_ProhibitedFirstAlternativeSuppliesNothing(t *testing.T) {
	rangeTo100 := "<range><lower_included>true</lower_included><upper_included>true</upper_included>" +
		"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>" +
		"<lower>0</lower><upper>100</upper></range>"
	cases := []struct {
		name  string
		opt   string
		check func(t *testing.T, out any)
	}{
		{
			// The walk's lower bound, 50, lies above the upper bound, 10,
			// and the allowed lower alternative admits nothing below 50,
			// so no ordered pair fits: the bounds stay as built. Read from
			// the prohibited alternative, 0..100 would let the lower bound
			// drop to 0, a value the allowed alternative rejects.
			name: "interval lower bound",
			opt: optTemplate("ELEMENT", optSingle("value", optNode("DV_INTERVAL&lt;DV_COUNT&gt;", "",
				optSingle("lower", optCount(true, rangeTo100), optCount(false, "<list>50</list>")),
				optSingle("upper", optCount(false, "<list>10</list>"))))),
			check: func(t *testing.T, out any) {
				iv, ok := out.(*rm.Element).Value.(*rm.DVInterval[rm.DVCount])
				if !ok {
					t.Fatalf("ELEMENT.value is %T, want *rm.DVInterval[rm.DVCount]", out.(*rm.Element).Value)
				}
				if got := iv.Lower.Magnitude; got != 50 {
					t.Errorf("DV_INTERVAL.lower = %d, want 50, the only value the allowed alternative admits", got)
				}
			},
		},
		{
			// The allowed EVENT_CONTEXT admits openehr::238; the prohibited
			// one, read first, would reject it.
			name: "EVENT_CONTEXT setting",
			opt: optTemplate("COMPOSITION", optSingle("context",
				optOccurring("C_COMPLEX_OBJECT", "EVENT_CONTEXT", "", 0, 0, optSingle("setting", optCodedText("local"))),
				optOccurring("C_COMPLEX_OBJECT", "EVENT_CONTEXT", "", 0, 1, optSingle("setting", optCodedText(terminology.ID))))),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "EVENT_CONTEXT.setting", generatedComposition(t, out).Context.Setting, terminology.Setting, "238")
			},
		},
		{
			// The allowed alternative rejects openehr::146; the
			// prohibited one, read first, would admit it.
			name: "INTERVAL_EVENT math_function",
			opt: optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function",
				optCodedTextOccurring(0, 0, terminology.ID), optCodedTextOccurring(0, 1, "local"))},
				intervalEventAttrs...)...),
			check: func(t *testing.T, out any) {
				checkCode(t, "INTERVAL_EVENT.math_function", out.(*rm.IntervalEvent[rm.ItemStructure]).MathFunction.DefiningCode, localAt0000)
			},
		},
		{
			// The allowed alternative names no code, so the RM default
			// 524 applies; the prohibited one would supply 526.
			name: "ISM_TRANSITION current_state",
			opt: yieldAction("", "", strings.Join([]string{
				optCodedTextOccurring(0, 0, terminology.ID, "526"), optCodedTextOccurring(0, 1, terminology.ID),
			}, "")),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "ISM_TRANSITION.current_state", out.(*rm.Action).IsmTransition.CurrentState, terminology.InstructionStates, "524")
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				opts.Language, opts.Territory, opts.Composer = "en", "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
				})
			}
		}
	}
}
