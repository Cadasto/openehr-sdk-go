package instance_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_DefaultDateTimesTakeTheClock is the REQ-107 check that a
// DV_DATE_TIME the generator builds as a default, with no primitive
// constraint and no value from the OPT, takes the clock: ACTION.time
// built from its BMM type, as an implicit attribute or as one the
// compiled node does not carry, and, under Example,
// which visits them, an INSTRUCTION's expiry_time and an EVENT_CONTEXT's
// end_time the OPT names without children. An unconstrained DV_DATE_TIME
// the OPT names as a node is TestREQ107_UnconstrainedDateTimeTakesTheClock.
func TestREQ107_DefaultDateTimesTakeTheClock(t *testing.T) {
	clock := defaultsNow.Format(time.RFC3339)
	cases := []struct {
		name     string
		opt      string
		modes    []bool
		policies []instance.Policy
		value    func(t *testing.T, out any) string
	}{
		{
			name:     "ACTION time from the BMM default",
			opt:      optTemplate("ACTION"),
			modes:    []bool{true, false},
			policies: []instance.Policy{instance.Minimal, instance.Example},
			value:    func(_ *testing.T, out any) string { return out.(*rm.Action).Time.Value },
		},
		{
			name:     "INSTRUCTION expiry_time under Example",
			opt:      optTemplate("INSTRUCTION", optOptionalSingle("expiry_time")),
			modes:    []bool{true, false},
			policies: []instance.Policy{instance.Example},
			value: func(t *testing.T, out any) string {
				et := out.(*rm.Instruction).ExpiryTime
				if et == nil {
					t.Fatal("INSTRUCTION.expiry_time absent, want the clock")
				}
				return et.Value
			},
		},
		{
			name:     "EVENT_CONTEXT end_time under Example",
			opt:      optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optOptionalSingle("end_time")))),
			modes:    []bool{true, false},
			policies: []instance.Policy{instance.Example},
			value: func(t *testing.T, out any) string {
				et := generatedComposition(t, out).Context.EndTime
				if et == nil {
					t.Fatal("EVENT_CONTEXT.end_time absent, want the clock")
				}
				return et.Value
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range tc.modes {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range tc.policies {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, policy, fill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy: policy, ValueFill: fill, Now: defaultsNow,
							Language: "en", Territory: "NL", Composer: testComposer(),
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						if got := tc.value(t, out); got != clock {
							t.Errorf("value = %q, want the clock %q", got, clock)
						}
					})
				}
			}
		}
	}
}
