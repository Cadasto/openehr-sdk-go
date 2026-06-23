package validation

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestIntervalRMTypeMatches_boundsBackedCollapse covers SDK-GAP-13
// sub-gap B: a round-tripped DV_INTERVAL<T> re-decodes as the bare
// DVInterval[DVOrdered] (typereg has a single "DV_INTERVAL"
// registration), so describeRMType reports "DV_INTERVAL". Conformance
// to the OPT's parameterised interval is decided from the bounds'
// runtime types, which survive the round-trip via their own `_type`.
func TestIntervalRMTypeMatches_boundsBackedCollapse(t *testing.T) {
	collapsed := rm.DVInterval[rm.DVOrdered]{}
	collapsed.Lower = &rm.DVQuantity{Magnitude: 30, Units: "cm"}
	collapsed.Upper = &rm.DVQuantity{Magnitude: 90, Units: "cm"}

	cases := []struct {
		name string
		got  string
		want string
		val  any
		ok   bool
	}{
		{"quantity bounds satisfy DV_INTERVAL<DV_QUANTITY>", "DV_INTERVAL", "DV_INTERVAL<DV_QUANTITY>", collapsed, true},
		{"quantity bounds via pointer", "DV_INTERVAL", "DV_INTERVAL<DV_QUANTITY>", &collapsed, true},
		{"quantity bounds reject DV_INTERVAL<DV_COUNT>", "DV_INTERVAL", "DV_INTERVAL<DV_COUNT>", collapsed, false},
		{"exact name match needs no bounds", "DV_INTERVAL<DV_QUANTITY>", "DV_INTERVAL<DV_QUANTITY>", nil, true},
		{"concrete interval satisfies bare DV_INTERVAL", "DV_INTERVAL<DV_QUANTITY>", "DV_INTERVAL", nil, true},
		{"non-interval type does not match", "DV_QUANTITY", "DV_INTERVAL<DV_QUANTITY>", nil, false},
	}
	for _, tc := range cases {
		if got := intervalRMTypeMatches(tc.got, tc.want, tc.val); got != tc.ok {
			t.Errorf("%s: intervalRMTypeMatches(%q,%q)=%v, want %v", tc.name, tc.got, tc.want, got, tc.ok)
		}
	}

	// A fully unbounded interval carries no bound to key off and
	// trivially satisfies any element type.
	unbounded := rm.DVInterval[rm.DVOrdered]{}
	unbounded.LowerUnbounded = true
	unbounded.UpperUnbounded = true
	if !intervalRMTypeMatches("DV_INTERVAL", "DV_INTERVAL<DV_DATE>", unbounded) {
		t.Error("fully unbounded interval should satisfy any DV_INTERVAL<T>")
	}
}
