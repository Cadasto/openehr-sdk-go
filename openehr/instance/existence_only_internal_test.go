package instance

import (
	"fmt"
	"testing"
)

// existenceOnlyItems is a C_MULTIPLE_ATTRIBUTE items with the given
// existence lower and upper bound (a lower of -1 leaves the existence out),
// a cardinality of cardLower..*, and children.
func existenceOnlyItems(existLower, existUpper, cardLower int, children string) string {
	existence := ""
	if existLower >= 0 {
		existence = fmt.Sprintf(`<existence><lower_included>true</lower_included><upper_included>true</upper_included>`+
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>`+
			`<lower>%d</lower><upper>%d</upper></existence>`, existLower, existUpper)
	}
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		existence + children +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		fmt.Sprintf(`<upper_unbounded>true</upper_unbounded><lower>%d</lower></interval></cardinality></attributes>`, cardLower)
}

// existenceOnlyChild is an OPT child of xsiType and rmType with the given
// occurrences element ("" leaves the occurrences out).
func existenceOnlyChild(xsiType, rmType, occurrences string) string {
	return `<children xsi:type="` + xsiType + `"><rm_type_name>` + rmType + `</rm_type_name>` + occurrences +
		`<node_id>at0001</node_id></children>`
}

// existenceOnlyOccurrences is an occurrences element of lower..upper.
func existenceOnlyOccurrences(lower, upper int) string {
	return fmt.Sprintf(`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>`+
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>`+
		`<lower>%d</lower><upper>%d</upper></occurrences>`, lower, upper)
}

// TestREQ107_ExistenceOnly is the REQ-107 check (What a visit writes) of
// which multi-valued attribute only its existence asks a member of: its
// existence lower bound is bounded and at least 1, the BMM does not mark
// it mandatory, neither its cardinality lower bound nor the occurrences
// lower bound of a child it allows is bounded and at least 1, and it is no
// list the RM makes non-empty once present, such as a SECTION's items.
// A child the OPT gives no occurrences does not count, and neither does a
// slot's, which the parser does not read (REQ-107 Known gap: slot
// occurrences). The rule is read off the compiled attribute: a required
// child that is not a slot gets a member of its own, so no generated root
// shows the occurrences condition.
func TestREQ107_ExistenceOnly(t *testing.T) {
	element := func(occurrences string) string {
		return existenceOnlyChild("C_COMPLEX_OBJECT", "ELEMENT", occurrences)
	}
	cases := []struct {
		name   string
		rmType string
		items  string
		want   bool
	}{
		{name: "existence 1..1, cardinality 0..*, no child", rmType: "ITEM_TREE", items: existenceOnlyItems(1, 1, 0, ""), want: true},
		{name: "an optional child", rmType: "ITEM_TREE", items: existenceOnlyItems(1, 1, 0, element(existenceOnlyOccurrences(0, 1))), want: true},
		{name: "a child with no occurrences", rmType: "ITEM_TREE", items: existenceOnlyItems(1, 1, 0, element("")), want: true},
		{
			name: "a required child after an optional one", rmType: "ITEM_TREE",
			items: existenceOnlyItems(1, 1, 0, element(existenceOnlyOccurrences(0, 1))+element(existenceOnlyOccurrences(1, 1))),
		},
		{name: "a required child", rmType: "ITEM_TREE", items: existenceOnlyItems(1, 1, 0, element(existenceOnlyOccurrences(1, 1)))},
		{
			// The parser does not read a slot's occurrences, so a slot at
			// 1..1 counts as one with none.
			name: "a slot at occurrences 1..1", rmType: "ITEM_TREE",
			items: existenceOnlyItems(1, 1, 0, existenceOnlyChild("ARCHETYPE_SLOT", "CLUSTER", existenceOnlyOccurrences(1, 1))),
			want:  true,
		},
		{name: "cardinality 1..*", rmType: "ITEM_TREE", items: existenceOnlyItems(1, 1, 1, "")},
		{name: "the BMM marks the items mandatory", rmType: "CLUSTER", items: existenceOnlyItems(1, 1, 0, "")},
		// The RM makes a SECTION's items non-empty once present, so their
		// existence asks for a member as their cardinality would.
		{name: "a list the RM makes non-empty once present", rmType: "SECTION", items: existenceOnlyItems(1, 1, 0, "")},
		{name: "existence 0..1", rmType: "ITEM_TREE", items: existenceOnlyItems(0, 1, 0, "")},
		{name: "no existence", rmType: "ITEM_TREE", items: existenceOnlyItems(-1, 0, 0, "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, attr := visitAttribute(t, visitRootOPT(tc.rmType, tc.items), "items")
			if got := existenceOnly(node, attr); got != tc.want {
				t.Errorf("existenceOnly(%s.items) = %t, want %t", tc.rmType, got, tc.want)
			}
		})
	}
}
