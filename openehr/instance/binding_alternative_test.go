package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ107_SingleAttributeBuildsUnderBindingAlternative is the REQ-107
// check (What a visit writes) that a single-valued attribute takes the RM
// type its first alternative names, or that alternative's concrete
// substitute, built under the constraints of the alternative REQ-102
// binds that type to, so the template validator passes what the
// generator built. Under [DATA_VALUE, DV_TEXT] the DV_TEXT is built under
// the DV_TEXT alternative's C_STRING; built under the open DATA_VALUE it
// would fail that list. Under [INTEGER, DV_COUNT] the INTEGER short name
// is built as its DV_COUNT wrapper, under the DV_COUNT alternative's
// magnitude range, in either order.
func TestREQ107_SingleAttributeBuildsUnderBindingAlternative(t *testing.T) {
	onlyThis := optNode("DV_TEXT", "", optSingle("value", optPrimitive("STRING", "C_STRING", "<list>only-this</list>")))
	integer10To20 := optPrimitive("INTEGER", "C_INTEGER", "<range>"+optInterval(10, 20)+"</range>")
	count0To5 := optNode("DV_COUNT", "", optSingle("magnitude", optPrimitive("INTEGER", "C_INTEGER", "<range>"+optInterval(0, 5)+"</range>")))
	inText := func(v rm.DataValue) bool {
		text, ok := v.(*rm.DVText)
		return ok && text.Value == "only-this"
	}
	inCount := func(v rm.DataValue) bool {
		count, ok := v.(*rm.DVCount)
		return ok && count.Magnitude >= 0 && count.Magnitude <= 5
	}
	cases := []struct {
		name         string
		alternatives []string
		built        func(rm.DataValue) bool
		want         string
	}{
		{name: "DATA_VALUE then DV_TEXT", alternatives: []string{optNode("DATA_VALUE", ""), onlyThis}, built: inText, want: `a DV_TEXT "only-this"`},
		{name: "INTEGER then DV_COUNT", alternatives: []string{integer10To20, count0To5}, built: inCount, want: "a DV_COUNT of 0 to 5"},
		{name: "DV_COUNT then INTEGER", alternatives: []string{count0To5, integer10To20}, built: inCount, want: "a DV_COUNT of 0 to 5"},
	}
	for _, tc := range cases {
		c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.alternatives...)), true)
		for _, policy := range everyPolicy {
			for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
				t.Run(tc.name+"/"+policy.String()+"/"+fill.String(), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy:      policy,
						ValueFill:   fill,
						ValueSource: mrand.NewPCG(1, 2),
					})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					el, ok := out.(*rm.Element)
					if !ok {
						t.Fatalf("Generate root = %T, want *rm.Element", out)
					}
					if !tc.built(el.Value) {
						t.Errorf("Generate built ELEMENT.value %s, want %s", describeBuilt(el.Value), tc.want)
					}
					if r := validation.Validate(out, c); len(r.Issues) != 0 {
						t.Errorf("Validate(generated ELEMENT) issues = %+v, want none", r.Issues)
					}
				})
			}
		}
	}
}

// describeBuilt names a generated ELEMENT value for a failure message.
func describeBuilt(v rm.DataValue) string {
	switch b := v.(type) {
	case nil:
		return "no value"
	case *rm.DVText:
		return fmt.Sprintf("a DV_TEXT %q", b.Value)
	case *rm.DVCount:
		return fmt.Sprintf("a DV_COUNT of %d", b.Magnitude)
	}
	name, _ := rm.RMTypeName(v)
	return "a " + name
}
