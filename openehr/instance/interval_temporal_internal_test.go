package instance

import "testing"

// TestREQ107_TemporalBoundsCompareAsInstants pins how the REQ-107 ordering
// rule compares DV_DATE_TIME and DV_TIME bounds: as instants when both
// carry a zone, as text only when both share one zoneless layout, and not
// at all otherwise, so such a pair is left alone.
func TestREQ107_TemporalBoundsCompareAsInstants(t *testing.T) {
	cases := []struct {
		name         string
		apply        func(lo, hi string, open bool) (string, string)
		lower, upper string
		want         [2]string
	}{
		{
			// 21:00 UTC then 22:00 UTC: in order, although the text of the
			// lower bound sorts after the upper one.
			name:  "date-time: zoned lower before UTC upper, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T23:00:00+02:00", upper: "2026-10-01T22:00:00Z",
			want: [2]string{"2026-10-01T23:00:00+02:00", "2026-10-01T22:00:00Z"},
		},
		{
			// 23:00 UTC then 22:00 UTC: inverted, although the text is in order.
			name:  "date-time: UTC lower after zoned upper, swapped",
			apply: orderDateTimes,
			lower: "2026-10-01T23:00:00Z", upper: "2026-10-02T00:00:00+02:00",
			want: [2]string{"2026-10-02T00:00:00+02:00", "2026-10-01T23:00:00Z"},
		},
		{
			name:  "date-time: zoned and zoneless, left alone",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00", upper: "2026-10-01T00:00:00Z",
			want: [2]string{"2026-10-02T00:00:00", "2026-10-01T00:00:00Z"},
		},
		{
			// 05:00 UTC then 07:00 UTC, in a layout without seconds that
			// does not parse as RFC 3339: one shape, but the zones differ,
			// so the text is not the order.
			name:  "date-time: zoned without seconds, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T10:00+05:00", upper: "2026-10-01T09:00+02:00",
			want: [2]string{"2026-10-01T10:00+05:00", "2026-10-01T09:00+02:00"},
		},
		{
			name:  "time: zoned without seconds, left alone",
			apply: orderTimes,
			lower: "10:00+05:00", upper: "09:00+02:00",
			want: [2]string{"10:00+05:00", "09:00+02:00"},
		},
		{
			name:  "date-time: two zoneless of one layout, swapped",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00", upper: "2026-10-01T00:00:00",
			want: [2]string{"2026-10-01T00:00:00", "2026-10-02T00:00:00"},
		},
		{
			name:  "date-time: different layouts, left alone",
			apply: orderDateTimes,
			lower: "2026-10-02", upper: "2026-10-01T00:00:00",
			want: [2]string{"2026-10-02", "2026-10-01T00:00:00"},
		},
		{
			// 08:00 UTC then 09:00 UTC.
			name:  "time: zoned lower before UTC upper, left alone",
			apply: orderTimes,
			lower: "10:00:00+02:00", upper: "09:00:00Z",
			want: [2]string{"10:00:00+02:00", "09:00:00Z"},
		},
		{
			name:  "time: zoned and zoneless, left alone",
			apply: orderTimes,
			lower: "10:00:00", upper: "09:00:00Z",
			want: [2]string{"10:00:00", "09:00:00Z"},
		},
		{
			name:  "date: different layouts, left alone",
			apply: orderDates,
			lower: "2026-10", upper: "2026-01-01",
			want: [2]string{"2026-10", "2026-01-01"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLo, gotHi := tc.apply(tc.lower, tc.upper, false)
			if gotLo != tc.want[0] || gotHi != tc.want[1] {
				t.Errorf("orderIntervalBounds(%q, %q) = (%q, %q), want (%q, %q)", tc.lower, tc.upper, gotLo, gotHi, tc.want[0], tc.want[1])
			}
		})
	}
}
