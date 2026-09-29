package simplified

import "testing"

// REQ-140 — a Web Template leaf takes the DV_INTERVAL grammar only when its
// type names DV_INTERVAL with exactly one bound type, and that bound type is
// the anchor its bounds are spelled with. A bare DV_INTERVAL, a malformed
// spelling, or two parameters is no interval leaf, so its value rides |raw
// whole instead of pairing the four boundary flags with a bound type that
// has no suffix form. White space around the parts does not matter.
func TestIntervalLeafAnchor(t *testing.T) {
	cases := []struct {
		rmType string
		anchor string
		ok     bool
	}{
		{rmType: "DV_INTERVAL<DV_QUANTITY>", anchor: "DV_QUANTITY", ok: true},
		{rmType: "DV_INTERVAL<DV_DATE_TIME>", anchor: "DV_DATE_TIME", ok: true},
		{rmType: "DV_INTERVAL<DV_INTERVAL<DV_COUNT>>", anchor: "DV_INTERVAL<DV_COUNT>", ok: true},
		{rmType: "DV_INTERVAL< DV_QUANTITY >", anchor: "DV_QUANTITY", ok: true},
		{rmType: " DV_INTERVAL<DV_QUANTITY>", anchor: "DV_QUANTITY", ok: true},
		{rmType: "DV_INTERVAL"},
		{rmType: "DV_QUANTITY"},
		{rmType: "DV_INTERVAL<>"},
		{rmType: "DV_INTERVAL<DV_QUANTITY"},
		{rmType: "DV_INTERVAL<DV_QUANTITY>>"},
		{rmType: "DV_INTERVAL<DV_QUANTITY,DV_COUNT>"},
		{rmType: "REFERENCE_RANGE<DV_QUANTITY>"},
	}
	for _, tc := range cases {
		anchor, ok := intervalLeafAnchor(tc.rmType)
		if anchor != tc.anchor || ok != tc.ok {
			t.Errorf("intervalLeafAnchor(%q) = (%q, %v), want (%q, %v)", tc.rmType, anchor, ok, tc.anchor, tc.ok)
		}
	}
}
