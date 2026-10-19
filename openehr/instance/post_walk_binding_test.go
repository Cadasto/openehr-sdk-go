package instance_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ107_IntervalOrderReadsBindingAlternative is the REQ-107 check
// (What a visit writes) that a pass after the walk reads the alternative
// the walk built under, not the first one listed. The lower bound's
// alternatives are [INTEGER 0..100, DV_COUNT with magnitude 50]: INTEGER is
// built as a DV_COUNT, the DV_COUNT alternative names that type exactly, so
// the walk builds 50 under it, and REQ-102 binds it there. The upper bound
// is 10, so the pair is inverted, and no ordered pair fits the DV_COUNT
// alternative's 50, so the bounds stay as built. Read from the INTEGER
// alternative, which puts nothing on a magnitude, the lower bound would
// drop to 10, which the validator rejects. The OPT admits no ordered pair,
// so the RM floor, which template validation includes, reports the inverted
// interval: REQ-107 excepts an OPT constraint that admits no value an RM
// rule accepts.
func TestREQ107_IntervalOrderReadsBindingAlternative(t *testing.T) {
	interval := optNode("DV_INTERVAL&lt;DV_COUNT&gt;", "",
		optSingle("lower",
			optPrimitive("INTEGER", "C_INTEGER", "<range>"+optInterval(0, 100)+"</range>"),
			optCount(false, "<list>50</list>")),
		optSingle("upper", optCount(false, "<list>10</list>")))
	c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", interval)), true)
	for _, policy := range everyPolicy {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			t.Run(policy.String()+"/"+fill.String(), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, ValueFill: fill})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				el, ok := out.(*rm.Element)
				if !ok {
					t.Fatalf("Generate root = %T, want *rm.Element", out)
				}
				iv, ok := el.Value.(*rm.DVInterval[rm.DVCount])
				if !ok {
					t.Fatalf("ELEMENT.value is %T, want *rm.DVInterval[rm.DVCount]", el.Value)
				}
				if got := iv.Lower.Magnitude; got != 50 {
					t.Errorf("DV_INTERVAL.lower = %d, want 50, the only value the DV_COUNT alternative admits", got)
				}
				r := validation.Validate(out, c)
				if len(r.Issues) != 1 || r.Issues[0].Code != "rm_invariant" || r.Issues[0].Path != "/value" {
					t.Errorf("Validate(generated ELEMENT) issues = %+v, want the one rm_invariant at /value the inverted pair gives", r.Issues)
				}
			})
		}
	}
}
