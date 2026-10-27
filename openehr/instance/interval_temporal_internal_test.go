package instance

import "testing"

// TestREQ107_TemporalBoundsCompareAsInstants pins how the REQ-107 ordering
// rule compares DV_DATE, DV_DATE_TIME and DV_TIME bounds, in the order the
// RM floor reads them (REQ-112 DV_INTERVAL): by magnitude when both are
// written with one zone designator or both with none, a partial value
// standing for the start of the period it names; as instants when the
// designators differ, in any layout the ISO 8601 parse accepts, without
// seconds or with a compact zone included; and not at all for a zoned
// value against a zoneless one or a value the parse refuses, so such a pair
// is left alone.
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
			// 01:00 UTC then 00:30 UTC: inverted by less than the lower
			// bound's offset, so reading that offset twice would put the
			// pair in order.
			name:  "date-time: zoned lower after UTC upper, within its offset, swapped",
			apply: orderDateTimes,
			lower: "2026-01-01T06:00:00+05:00", upper: "2026-01-01T00:30:00Z",
			want: [2]string{"2026-01-01T00:30:00Z", "2026-01-01T06:00:00+05:00"},
		},
		{
			// 00:30 UTC then 01:00 UTC: in order, and reading the upper
			// bound's offset twice would invert the pair.
			name:  "date-time: UTC lower before zoned upper, within its offset, left alone",
			apply: orderDateTimes,
			lower: "2026-01-01T00:30:00Z", upper: "2026-01-01T06:00:00+05:00",
			want: [2]string{"2026-01-01T00:30:00Z", "2026-01-01T06:00:00+05:00"},
		},
		{
			name:  "date-time: zoned and zoneless, left alone",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00", upper: "2026-10-01T00:00:00Z",
			want: [2]string{"2026-10-02T00:00:00", "2026-10-01T00:00:00Z"},
		},
		{
			// A zoneless value names no instant, so it is not read as one.
			name:  "date-time: zoned lower and zoneless upper, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T00:00:00Z", upper: "2026-10-02T00:00:00",
			want: [2]string{"2026-10-01T00:00:00Z", "2026-10-02T00:00:00"},
		},
		{
			// 05:00 UTC then 07:00 UTC, in a layout without seconds that
			// does not parse as RFC 3339: one shape, but the zones differ,
			// so the text is not the order. As instants the two are in order.
			name:  "date-time: zoned without seconds, in order as instants, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T10:00+05:00", upper: "2026-10-01T09:00+02:00",
			want: [2]string{"2026-10-01T10:00+05:00", "2026-10-01T09:00+02:00"},
		},
		{
			// 08:00 UTC then 07:00 UTC: inverted, although the text is in order.
			name:  "date-time: zoned without seconds, inverted as instants, swapped",
			apply: orderDateTimes,
			lower: "2026-10-01T10:00+02:00", upper: "2026-10-01T12:00+05:00",
			want: [2]string{"2026-10-01T12:00+05:00", "2026-10-01T10:00+02:00"},
		},
		{
			name:  "date-time: compact zones, inverted as instants, swapped",
			apply: orderDateTimes,
			lower: "2026-10-01T10:00:00+0200", upper: "2026-10-01T12:00:00+0500",
			want: [2]string{"2026-10-01T12:00:00+0500", "2026-10-01T10:00:00+0200"},
		},
		{
			name:  "time: zoned without seconds, in order as instants, left alone",
			apply: orderTimes,
			lower: "10:00+05:00", upper: "09:00+02:00",
			want: [2]string{"10:00+05:00", "09:00+02:00"},
		},
		{
			name:  "time: zoned without seconds, inverted as instants, swapped",
			apply: orderTimes,
			lower: "10:00+02:00", upper: "12:00+05:00",
			want: [2]string{"12:00+05:00", "10:00+02:00"},
		},
		{
			// The fractions have different widths, so the text is not the order.
			name:  "time: zoneless fractions of two widths, swapped",
			apply: orderTimes,
			lower: "10:00:00.5", upper: "10:00:00.25",
			want: [2]string{"10:00:00.25", "10:00:00.5"},
		},
		{
			name:  "date-time: two zoneless of one layout, swapped",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00", upper: "2026-10-01T00:00:00",
			want: [2]string{"2026-10-01T00:00:00", "2026-10-02T00:00:00"},
		},
		{
			// A date-time with no time stands for the start of its day.
			name:  "date-time: a date alone against a full date-time, swapped",
			apply: orderDateTimes,
			lower: "2026-10-02", upper: "2026-10-01T00:00:00",
			want: [2]string{"2026-10-01T00:00:00", "2026-10-02"},
		},
		{
			name:  "date-time: a value the parse refuses, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T00:00:00", upper: "2026-10-02T25:00:00",
			want: [2]string{"2026-10-01T00:00:00", "2026-10-02T25:00:00"},
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
			name:  "time: zoned lower and zoneless upper, left alone",
			apply: orderTimes,
			lower: "09:00:00Z", upper: "10:00:00",
			want: [2]string{"09:00:00Z", "10:00:00"},
		},
		{
			// 2026-10 stands for its first day, which is after 2026-01-01.
			name:  "date: a partial date against a full one, swapped",
			apply: orderDates,
			lower: "2026-10", upper: "2026-01-01",
			want: [2]string{"2026-01-01", "2026-10"},
		},
		{
			name:  "date: a value the parse refuses, left alone",
			apply: orderDates,
			lower: "2026-01-01", upper: "example",
			want: [2]string{"2026-01-01", "example"},
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
