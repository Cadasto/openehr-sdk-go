package instance_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// optBooleanAttr is a C_SINGLE_ATTRIBUTE called name whose child is a
// C_BOOLEAN that admits true only.
func optBooleanAttr(name string) string {
	return optSingle(name, optPrimitive("BOOLEAN", "C_BOOLEAN",
		"<true_valid>true</true_valid><false_valid>false</false_valid>"))
}

// nonStorableOPTs are templates that name an attribute the RM computes
// rather than stores, with a child the generator would try to write there.
// Each also names a storable attribute, so the walk has work to do beside
// the skipped one.
var nonStorableOPTs = []struct {
	name string
	opt  string
}{
	{
		name: "DV_QUANTITY is_integral",
		opt: optTemplate("ELEMENT", optSingle("value", optNode("DV_QUANTITY", "",
			optBooleanAttr("is_integral")))),
	},
	{
		name: "DV_PROPORTION is_integral",
		opt: optTemplate("ELEMENT", optSingle("value", optNode("DV_PROPORTION", "",
			optSingle("numerator", optPrimitive("REAL", "C_REAL", "<list>3</list>")),
			optBooleanAttr("is_integral")))),
	},
	{
		name: "POINT_EVENT offset",
		opt: optTemplate("POINT_EVENT",
			optSingle("offset", optNode("DV_DURATION", "")),
			optSingle("data", emptyTree)),
	},
	{
		name: "INTERVAL_EVENT offset",
		opt: optTemplate("INTERVAL_EVENT",
			optSingle("offset", optNode("DV_DURATION", "")),
			optSingle("data", emptyTree)),
	},
}

// TestREQ107_NonStorableAttributesAreNotVisited is the REQ-107 check that
// Generate does not visit offset on POINT_EVENT or INTERVAL_EVENT, or
// is_integral on DV_QUANTITY or DV_PROPORTION, under either policy and
// either value fill. A visit of offset fails, because the RM writer has no
// offset field, and a visit of is_integral on DV_QUANTITY fails, because a
// boolean is not a quantity; the DV_PROPORTION visit is silent, which the
// internal TestREQ107_VisitsSkipsNonStorableAttributes covers. Each output
// also passes the RM floor.
func TestREQ107_NonStorableAttributesAreNotVisited(t *testing.T) {
	for _, tc := range nonStorableOPTs {
		c := compileOPTText(t, tc.opt, true)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
				t.Run(tc.name+"/"+policy.String()+"/"+fill.String(), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy:    policy,
						ValueFill: fill,
						Now:       defaultsNow,
					})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					for _, iss := range validation.ValidateRM(out).Issues {
						if iss.Severity == validation.Error {
							t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					}
				})
			}
		}
	}
}
