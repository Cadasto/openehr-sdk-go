package instance_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// optSlot is an ARCHETYPE_SLOT of rmType with no include assertion, so the
// generator may stamp the RM-type-prefix fallback id on a fill.
func optSlot(rmType, nodeID string) string {
	return `<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<node_id>` + nodeID + `</node_id></children>`
}

// optMultipleLowerZero is a C_MULTIPLE_ATTRIBUTE called name over children
// whose cardinality has no lower bound, so the attribute is optional.
func optMultipleLowerZero(name string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality></attributes>`
}

// TestREQ107_ElementSlotCarriesExactlyOneOfValueAndNullFlavour pins the two
// places the generator fills an ELEMENT archetype slot. A slot body is not in
// the OPT, so the filled ELEMENT has no value and must carry a null flavour
// (Inv_null_flavour_indicated), under both policies and both fills.
//
//   - required: CLUSTER.items is mandatory (lower 1) and holds only the slot,
//     so the top-up fill of the attribute walks the slot.
//   - optional, CLUSTER: CLUSTER.items is mandatory in the RM, so the
//     attribute is topped up to one item even with a lower bound of 0.
//   - optional, ITEM_TREE: ITEM_TREE.items is optional in the RM, so nothing
//     is appended while the attribute is walked and the RM floor fills the
//     list from the slot afterwards.
func TestREQ107_ElementSlotCarriesExactlyOneOfValueAndNullFlavour(t *testing.T) {
	cases := []struct {
		name string
		opt  string
	}{
		{"required slot", optTemplate("CLUSTER", optMultiple("items", optSlot("ELEMENT", "at9000")))},
		{"optional slot", optTemplate("CLUSTER", optMultipleLowerZero("items", optSlot("ELEMENT", "at9000")))},
		{"optional slot in ITEM_TREE", optTemplate("ITEM_TREE", optMultipleLowerZero("items", optSlot("ELEMENT", "at9000")))},
	}
	policies := []struct {
		name string
		p    instance.Policy
	}{{"Minimal", instance.Minimal}, {"Example", instance.Example}}
	fills := []struct {
		name string
		f    instance.ValueFill
	}{{"ExampleFill", instance.ExampleFill}, {"RandomFill", instance.RandomFill}}

	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, pol := range policies {
			for _, fill := range fills {
				t.Run(tc.name+"/"+pol.name+"/"+fill.name, func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy:    pol.p,
						Language:  "en",
						Territory: "NL",
						Composer:  testComposer(),
						Now:       defaultsNow,
						ValueFill: fill.f,
					})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					var items []rm.Item
					switch v := out.(type) {
					case *rm.Cluster:
						items = v.Items
					case *rm.ItemTree:
						items = v.Items
					}
					if len(items) != 1 {
						t.Fatalf("Generate = %T %+v, want one item, the slot fill", out, out)
					}
					el, ok := items[0].(*rm.Element)
					if !ok {
						t.Fatalf("items[0] is %T, want *rm.Element", items[0])
					}
					if el.Value == nil && el.NullFlavour == nil {
						t.Errorf("slot ELEMENT carries neither value nor null_flavour")
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
