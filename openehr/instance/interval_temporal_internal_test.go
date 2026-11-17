package instance

import (
	"fmt"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_TemporalBoundsCompareAsInstants pins how the REQ-107 ordering
// rule compares DV_DATE, DV_DATE_TIME and DV_TIME bounds, in the order the RM
// floor reads them (REQ-112 DV_INTERVAL): at the precision the two share when
// both are written with one zone designator or both with none
// (TestREQ107_TemporalBoundsCompareAtSharedPrecision pins the partial
// values); as instants when both carry a zone and the designators differ, in
// any layout the ISO 8601 parse accepts, without seconds or with a compact
// zone included; for a zoned value against a zoneless one as the temporal
// range check of constraints.CTime and CDateTime orders them, after XML
// Schema 1.0 Part 2 §3.2.7.4, the zoned one read in UTC and the other at
// +14:00 and at -14:00, a partial value covering its whole period, so only
// where the two lie more than 14 hours apart, or as equal where the zoned one
// lies inside the other's period at both readings; and not at all for a
// value the parse refuses. A pair that does not compare, a mixed pair within
// 14 hours included, is left alone, and so is an equal pair.
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
			// 24 hours apart, so after the zoned upper bound at either
			// reading of the zoneless lower one.
			name:  "date-time: zoneless lower more than 14 hours after zoned upper, swapped",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00", upper: "2026-10-01T00:00:00Z",
			want: [2]string{"2026-10-01T00:00:00Z", "2026-10-02T00:00:00"},
		},
		{
			// 13 hours apart: read at -14:00 the zoneless lower bound is
			// after the upper one, and read at +14:00 before it, so the two
			// do not compare.
			name:  "date-time: zoneless lower within 14 hours after zoned upper, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T13:00:00", upper: "2026-10-01T00:00:00Z",
			want: [2]string{"2026-10-01T13:00:00", "2026-10-01T00:00:00Z"},
		},
		{
			// 2026-10-02T00:00+02:00 is 2026-10-01T22:00Z, 15 hours after
			// 07:00, although the clocks are 17 hours apart.
			name:  "date-time: zoned lower more than 14 hours after zoneless upper, swapped",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00+02:00", upper: "2026-10-01T07:00:00",
			want: [2]string{"2026-10-01T07:00:00", "2026-10-02T00:00:00+02:00"},
		},
		{
			// 2026-10-02T00:00+02:00 is 2026-10-01T22:00Z, 13 hours after
			// 09:00, although the clocks are 15 hours apart: reading the
			// zoned clock, not the instant, would swap the pair.
			name:  "date-time: zoned lower within 14 hours after zoneless upper, left alone",
			apply: orderDateTimes,
			lower: "2026-10-02T00:00:00+02:00", upper: "2026-10-01T09:00:00",
			want: [2]string{"2026-10-02T00:00:00+02:00", "2026-10-01T09:00:00"},
		},
		{
			// More than 14 hours before the zoneless upper bound: in order.
			name:  "date-time: zoned lower and zoneless upper, in order, left alone",
			apply: orderDateTimes,
			lower: "2026-10-01T00:00:00Z", upper: "2026-10-02T00:00:00",
			want: [2]string{"2026-10-01T00:00:00Z", "2026-10-02T00:00:00"},
		},
		{
			// The year 2030 ends at 2031-01-01T00:00, 15 hours before the
			// lower bound, which is a year after the year starts.
			name:  "date-time: zoned lower more than 14 hours after a zoneless year ends, swapped",
			apply: orderDateTimes,
			lower: "2031-01-01T15:00Z", upper: "2030",
			want: [2]string{"2030", "2031-01-01T15:00Z"},
		},
		{
			// 13 hours after the year 2030 ends, although a year after it
			// starts: a partial value covers its whole period.
			name:  "date-time: zoned lower within 14 hours after a zoneless year ends, left alone",
			apply: orderDateTimes,
			lower: "2031-01-01T13:00Z", upper: "2030",
			want: [2]string{"2031-01-01T13:00Z", "2030"},
		},
		{
			// The year 2030 holds 2030-06-01T10:00Z at both readings, so the
			// two are equal and in order either way round.
			name:  "date-time: zoneless year lower holding the zoned upper, equal, left alone",
			apply: orderDateTimes,
			lower: "2030", upper: "2030-06-01T10:00Z",
			want: [2]string{"2030", "2030-06-01T10:00Z"},
		},
		{
			name:  "date-time: zoned lower inside the zoneless year upper, equal, left alone",
			apply: orderDateTimes,
			lower: "2030-06-01T10:00Z", upper: "2030",
			want: [2]string{"2030-06-01T10:00Z", "2030"},
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
			name:  "time: zoneless lower within 14 hours after zoned upper, left alone",
			apply: orderTimes,
			lower: "10:00:00", upper: "09:00:00Z",
			want: [2]string{"10:00:00", "09:00:00Z"},
		},
		{
			name:  "time: zoned lower and zoneless upper, within 14 hours, left alone",
			apply: orderTimes,
			lower: "09:00:00Z", upper: "10:00:00",
			want: [2]string{"09:00:00Z", "10:00:00"},
		},
		{
			// 20:00-05:00 is 25:00Z, 15 hours after 10:00.
			name:  "time: zoned lower more than 14 hours after zoneless upper, swapped",
			apply: orderTimes,
			lower: "20:00:00-05:00", upper: "10:00:00",
			want: [2]string{"10:00:00", "20:00:00-05:00"},
		},
		{
			// 11:00+02:00 is 09:00Z, 14 hours and a half before 23:30.
			name:  "time: zoneless lower more than 14 hours after zoned upper, swapped",
			apply: orderTimes,
			lower: "23:30:00", upper: "11:00:00+02:00",
			want: [2]string{"11:00:00+02:00", "23:30:00"},
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

// TestREQ107_TemporalBoundsTakeTheirRangeEnds pins the REQ-107 Intervals
// rule on DV_DATE, DV_TIME and DV_DATE_TIME bounds whose swap a side's
// C_DATE, C_TIME or C_DATE_TIME refuses: each side takes the end of its
// range nearest the other side, written to its pattern, where that pair is
// in order. An end with fewer fields than the pattern asks for is filled out
// to the start of the period it names on the lower side and to its end on
// the upper side, and an excluded end moves one unit inside the range. A
// pair the two ranges leave no order for is left as generated.
func TestREQ107_TemporalBoundsTakeTheirRangeEnds(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	type kind struct{ rmType, primitive, item string }
	var (
		date     = kind{"DV_DATE", "DATE", "C_DATE"}
		clock    = kind{"DV_TIME", "TIME", "C_TIME"}
		dateTime = kind{"DV_DATE_TIME", "DATE_TIME", "C_DATE_TIME"}
	)
	ranged := func(pattern, lower string, lowerIncluded bool, upper string, upperIncluded bool) string {
		return "<pattern>" + pattern + "</pattern>" + fmt.Sprintf("<range><lower_included>%t</lower_included><upper_included>%t</upper_included>"+
			"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>"+
			"<lower>%s</lower><upper>%s</upper></range>", lowerIncluded, upperIncluded, lower, upper)
	}
	node := func(k kind, lowerBody, upperBody string) *tcimpl.CompiledNode {
		t.Helper()
		bound := func(body string) string { return TemporalBound(k.rmType, k.primitive, k.item, body) }
		return intervalNode(t, RetargetInterval(t, opt, k.rmType, bound(lowerBody), bound(upperBody)))
	}
	order := func(k kind, n *tcimpl.CompiledNode, lower, upper string) [2]string {
		switch k {
		case date:
			iv := &rm.DVInterval[rm.DVDate]{}
			iv.Lower.Value, iv.Upper.Value = lower, upper
			orderIntervalBounds(n, iv)
			return [2]string{iv.Lower.Value, iv.Upper.Value}
		case clock:
			iv := &rm.DVInterval[rm.DVTime]{}
			iv.Lower.Value, iv.Upper.Value = lower, upper
			orderIntervalBounds(n, iv)
			return [2]string{iv.Lower.Value, iv.Upper.Value}
		}
		iv := &rm.DVInterval[rm.DVDateTime]{}
		iv.Lower.Value, iv.Upper.Value = lower, upper
		orderIntervalBounds(n, iv)
		return [2]string{iv.Lower.Value, iv.Upper.Value}
	}
	cases := []struct {
		name         string
		kind         kind
		lowerBody    string
		upperBody    string
		lower, upper string
		want         [2]string
	}{
		{
			// 2020-11-01 lies above the upper side's range, which ends with
			// the month 2020-03; the lower side's range starts with 2020.
			name:      "date: ends with fewer fields than the pattern, filled out",
			kind:      date,
			lowerBody: ranged("yyyy-mm-dd", "2020", true, "2021", true),
			upperBody: ranged("yyyy-mm-dd", "2019", true, "2020-03", true),
			lower:     "2020-11-01", upper: "2020-02-01",
			want: [2]string{"2020-01-01", "2020-03-31"},
		},
		{
			name:      "date: no ordered pair, left as generated",
			kind:      date,
			lowerBody: ranged("yyyy-mm-dd", "2020-06", true, "2021", true),
			upperBody: ranged("yyyy-mm-dd", "2019", true, "2020-03", true),
			lower:     "2020-11-01", upper: "2020-02-01",
			want: [2]string{"2020-11-01", "2020-02-01"},
		},
		{
			// Both sides exclude their end nearest the other.
			name:      "time: excluded ends moved a second inside",
			kind:      clock,
			lowerBody: ranged("hh:mm:ss", "08:00:00", false, "12:00:00", true),
			upperBody: ranged("hh:mm:ss", "06:00:00", true, "09:00:00", false),
			lower:     "11:00:00", upper: "07:00:00",
			want: [2]string{"08:00:01", "08:59:59"},
		},
		{
			// The upper side's end carries no seconds, which its pattern asks
			// for: the last second of that minute, in its zone.
			name:      "date-time: an end without seconds, filled out in its zone",
			kind:      dateTime,
			lowerBody: ranged("yyyy-mm-ddThh:mm:ss", "2020-01-01T00:00:00Z", true, "2020-12-31T23:59:59Z", true),
			upperBody: ranged("yyyy-mm-ddThh:mm:ss", "2019-01-01T00:00:00Z", true, "2020-01-31T12:00Z", true),
			lower:     "2020-06-01T00:00:00Z", upper: "2020-01-15T00:00:00Z",
			want: [2]string{"2020-01-01T00:00:00Z", "2020-01-31T12:00:59Z"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := order(tc.kind, node(tc.kind, tc.lowerBody, tc.upperBody), tc.lower, tc.upper)
			if got != tc.want {
				t.Errorf("orderIntervalBounds(%q, %q) = %q, want %q", tc.lower, tc.upper, got, tc.want)
			}
		})
	}
}
