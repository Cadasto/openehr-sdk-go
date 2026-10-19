package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// TestREQ107_EveryPolicyBuildsTheContextFromTheOPT is the REQ-107 check
// that a COMPOSITION's context is built under the OPT's constraints under
// every policy, Required included: the visit rule makes Required visit
// the context as Medium does, although the OPT gives it an existence of
// 0..1, as every vendored COMPOSITION OPT that names it does. So a setting the OPT pins
// to an openEHR setting code other than 238 keeps that code, and a
// required other_context is built. Each case holds in both compile modes,
// and the output passes the RM floor and the template validator.
func TestREQ107_EveryPolicyBuildsTheContextFromTheOPT(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		check func(t *testing.T, ctx *rm.EventContext)
	}{
		{
			name: "optional context, setting pinned to openehr 225",
			opt: optTemplate("COMPOSITION", optOptionalSingleOver("context", optNode("EVENT_CONTEXT", "",
				optSingle("setting", optCodedText(terminology.ID, "225"))))),
			check: func(t *testing.T, ctx *rm.EventContext) {
				t.Helper()
				checkCode(t, "EVENT_CONTEXT.setting.defining_code", ctx.Setting.DefiningCode,
					rm.CodePhrase{CodeString: "225", TerminologyID: rm.TerminologyID{Value: terminology.ID}})
			},
		},
		{
			name: "optional context, other_context required",
			opt: optTemplate("COMPOSITION", optOptionalSingleOver("context", optNode("EVENT_CONTEXT", "",
				optRequiredSingle("other_context", optNode("ITEM_TREE", "at0001"))))),
			check: func(t *testing.T, ctx *rm.EventContext) {
				t.Helper()
				if ctx.OtherContext == nil || rm.IsTypedNil(ctx.OtherContext) {
					t.Error("EVENT_CONTEXT.other_context absent, want the ITEM_TREE the OPT requires")
				}
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy: policy, Territory: "NL", Composer: testComposer(), Now: defaultsNow,
					})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					comp := generatedComposition(t, out)
					if comp.Context == nil {
						t.Fatal("COMPOSITION.context absent, want the EVENT_CONTEXT the OPT names")
					}
					if comp.Context.StartTime.Value == "" {
						t.Error("EVENT_CONTEXT.start_time empty, want the clock")
					}
					tc.check(t, comp.Context)
					checkValid(t, out, c)
				})
			}
		}
	}
}
