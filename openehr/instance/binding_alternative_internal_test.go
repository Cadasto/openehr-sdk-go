package instance

import (
	"testing"
)

// Alternatives of ELEMENT.value for the bindingAlternative cases.
const (
	altDataValue  = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DATA_VALUE</rm_type_name><node_id></node_id></children>`
	altText       = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>`
	altCount      = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_COUNT</rm_type_name><node_id></node_id></children>`
	altDvDateTime = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_DATE_TIME</rm_type_name><node_id></node_id></children>`
	altDateTime   = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DATE_TIME</rm_type_name><node_id></node_id>` +
		`<item xsi:type="C_DATE_TIME"></item></children>`
)

// TestREQ107_BindingAlternative is the REQ-107 check (What a visit
// writes) that the walk builds a single-valued attribute under the
// alternative REQ-102's alternatives rule binds the built type to: the
// first alternative naming that type exactly, else the first AOM 1.4
// short name that type wraps, else the first alternative. The built type
// is given directly, so the short-name case is pinned whatever
// concreteFor substitutes for an abstract first alternative.
func TestREQ107_BindingAlternative(t *testing.T) {
	cases := []struct {
		name         string
		alternatives string
		built        string
		want         int
	}{
		{name: "a later exact alternative", alternatives: altDataValue + altText, built: "DV_TEXT", want: 1},
		{name: "the first alternative is exact", alternatives: altText + altDataValue, built: "DV_TEXT", want: 0},
		{name: "a later short name the type wraps", alternatives: altDataValue + altDateTime, built: "DV_DATE_TIME", want: 1},
		{name: "exact before the short name", alternatives: altDataValue + altDateTime + altDvDateTime, built: "DV_DATE_TIME", want: 2},
		{name: "no exact or short-name alternative", alternatives: altDataValue + altCount, built: "DV_TEXT", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, attr := visitAttribute(t, visitRootOPT("ELEMENT", visitSingle("value", tc.alternatives)), "value")
			children := attr.Children()
			got := bindingAlternative(children, tc.built)
			if got != children[tc.want] {
				t.Errorf("bindingAlternative(%s, built %s) = %s, want alternative %d (%s)",
					tc.alternatives, tc.built, got.RMTypeName(), tc.want, children[tc.want].RMTypeName())
			}
		})
	}
}
