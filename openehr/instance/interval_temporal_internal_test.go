package instance

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_TemporalBoundsCompareAsInstants pins how the REQ-107 ordering
// rule compares DV_DATE, DV_DATE_TIME and DV_TIME bounds, in the order the
// RM floor reads them (REQ-112 DV_INTERVAL): at the precision the two share
// when both are written with one zone designator or both with none
// (TestREQ107_TemporalBoundsCompareAtSharedPrecision pins the partial
// values); as instants when the designators differ, in any layout the ISO
// 8601 parse accepts, without seconds or with a compact zone included; and
// not at all for a zoned value against a zoneless one or a value the parse
// refuses, so such a pair is left alone.
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
			// A date-time with no time shares its date with the other: the
			// day 2026-10-02 is after 2026-10-01.
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
			// On the month the two share, 2026-10 is after 2026-01-01.
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

// TestREQ107_TemporalBoundsCompareAtSharedPrecision pins the REQ-107
// ordering rule on partial DV_DATE, DV_TIME and DV_DATE_TIME bounds written
// with one zone designator or both with none: they compare at the precision
// the two share, field by field over the fields both carry, as the RM floor
// orders them (REQ-112 DV_INTERVAL). So a pair equal on every shared field,
// 2021-05 and 2021 or 10:30 and 10, is in order either way round and left
// alone, where reading a partial value as the start of its period would swap
// it; and a pair that differs on a shared field is swapped when the first
// field that differs puts the lower bound above. Bounds whose designators
// differ still compare as instants, a partial value standing for the start
// of its period: 10:30+00:00 is above 10Z although the two are equal on the
// hour they share.
func TestREQ107_TemporalBoundsCompareAtSharedPrecision(t *testing.T) {
	cases := []struct {
		name         string
		apply        func(lo, hi string, open bool) (string, string)
		lower, upper string
		want         [2]string
	}{
		{
			name:  "date: a month above its year, left alone",
			apply: orderDates,
			lower: "2021-05", upper: "2021",
			want: [2]string{"2021-05", "2021"},
		},
		{
			name:  "date: a year below a month in it, left alone",
			apply: orderDates,
			lower: "2021", upper: "2021-05",
			want: [2]string{"2021", "2021-05"},
		},
		{
			name:  "date: a day above its month, compact, left alone",
			apply: orderDates,
			lower: "20210520", upper: "2021-05",
			want: [2]string{"20210520", "2021-05"},
		},
		{
			// The month 06 is after 05, whatever the day of 2021-05-20.
			name:  "date: a month above a day in the month before, swapped",
			apply: orderDates,
			lower: "2021-06", upper: "2021-05-20",
			want: [2]string{"2021-05-20", "2021-06"},
		},
		{
			name:  "time: a minute above its hour, left alone",
			apply: orderTimes,
			lower: "10:30", upper: "10",
			want: [2]string{"10:30", "10"},
		},
		{
			name:  "time: an hour above a minute in the hour before, swapped",
			apply: orderTimes,
			lower: "11", upper: "10:30",
			want: [2]string{"10:30", "11"},
		},
		{
			name:  "time: in one zone, a minute above its hour, left alone",
			apply: orderTimes,
			lower: "10:30+02:00", upper: "10+02:00",
			want: [2]string{"10:30+02:00", "10+02:00"},
		},
		{
			// 10:30Z is after 10:00Z, although the two are equal on the hour.
			name:  "time: in +00:00 and Z, equal on the hour, swapped as instants",
			apply: orderTimes,
			lower: "10:30+00:00", upper: "10Z",
			want: [2]string{"10Z", "10:30+00:00"},
		},
		{
			name:  "date-time: a second above its minute, left alone",
			apply: orderDateTimes,
			lower: "2021-12-31T23:59:30", upper: "2021-12-31T23:59",
			want: [2]string{"2021-12-31T23:59:30", "2021-12-31T23:59"},
		},
		{
			name:  "date-time: a time above its date, left alone",
			apply: orderDateTimes,
			lower: "2021-12-31T23:59:30", upper: "2021-12-31",
			want: [2]string{"2021-12-31T23:59:30", "2021-12-31"},
		},
		{
			name:  "date-time: a day above a time on the day before, swapped",
			apply: orderDateTimes,
			lower: "2021-05-21", upper: "2021-05-20T23:59:59",
			want: [2]string{"2021-05-20T23:59:59", "2021-05-21"},
		},
		{
			name:  "date-time: in one zone, a minute above its hour, left alone",
			apply: orderDateTimes,
			lower: "2021-05-20T10:30Z", upper: "2021-05-20T10Z",
			want: [2]string{"2021-05-20T10:30Z", "2021-05-20T10Z"},
		},
		{
			name:  "date-time: in +00:00 and Z, equal on the hour, swapped as instants",
			apply: orderDateTimes,
			lower: "2021-05-20T10:30+00:00", upper: "2021-05-20T10Z",
			want: [2]string{"2021-05-20T10Z", "2021-05-20T10:30+00:00"},
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

	t.Run("DV_ORDERED dates: a month above its year, left alone", func(t *testing.T) {
		iv := &rm.DVInterval[rm.DVOrdered]{}
		iv.Lower, iv.Upper = rm.DVDate{Value: "2021-05"}, &rm.DVDate{Value: "2021"}
		orderIntervalBounds(nil, iv)
		if got := [2]string{orderedString(iv.Lower), orderedString(iv.Upper)}; got != [2]string{"2021-05", "2021"} {
			t.Errorf("orderIntervalBounds = %q, want the pair left alone", got)
		}
	})
}
