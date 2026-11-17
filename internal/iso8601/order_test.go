package iso8601

import (
	"strings"
	"testing"
)

// REQ-103: the order of a value with a zone against one with none, which the
// template layer's temporal range check uses. Each value covers a span, from
// its magnitude to the start of the next period where it is reduced, and the
// one with no zone is read at +14:00 and at -14:00, as XML Schema 1.0 Part 2
// §3.2.7.4 orders a dateTime.

// hour is the length of an hour in seconds.
const hour = 3600.0

// TestREQ103_TimeSpan holds TimeSpan to the hour or the minute a reduced
// time covers, and to one point for a time whose seconds are known, its
// fraction included; the zone is not applied.
func TestREQ103_TimeSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Span
	}{
		{"10", Span{10 * hour, 11 * hour}},
		{"23", Span{23 * hour, 24 * hour}},
		{"10:30", Span{10*hour + 30*60, 10*hour + 31*60}},
		{"1030+02:00", Span{10*hour + 30*60, 10*hour + 31*60}},
		{"10:30:15", Span{10*hour + 30*60 + 15, 10*hour + 30*60 + 15}},
		{"10:30:15.5Z", Span{10*hour + 30*60 + 15.5, 10*hour + 30*60 + 15.5}},
		{"23:59:60", Span{secondsPerDay, secondsPerDay}},
	}
	for _, tc := range tests {
		p, err := ParseTime(tc.in)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", tc.in, err)
		}
		if got := TimeSpan(p); got != tc.want {
			t.Errorf("TimeSpan(ParseTime(%q)) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

// TestREQ103_DateTimeSpan holds DateTimeSpan to the year, the month or the
// day a date-time with no time covers, across a year end and a leap February,
// and to the hour, the minute or the point a date-time with a time covers.
func TestREQ103_DateTimeSpan(t *testing.T) {
	t.Parallel()
	dayOf := func(s string) float64 {
		t.Helper()
		d, err := ParseDate(s)
		if err != nil {
			t.Fatalf("ParseDate(%q): %v", s, err)
		}
		return float64(d.Magnitude()) * secondsPerDay
	}
	tests := []struct {
		in   string
		want Span
	}{
		{"2030", Span{dayOf("2030-01-01"), dayOf("2031-01-01")}},
		{"2030-12", Span{dayOf("2030-12-01"), dayOf("2031-01-01")}},
		{"2024-02", Span{dayOf("2024-02-01"), dayOf("2024-03-01")}},
		{"2030-06-01", Span{dayOf("2030-06-01"), dayOf("2030-06-02")}},
		{"2030-12-31", Span{dayOf("2030-12-31"), dayOf("2031-01-01")}},
		{"20301231", Span{dayOf("2030-12-31"), dayOf("2031-01-01")}},
		{"2030-06-01T10", Span{dayOf("2030-06-01") + 10*hour, dayOf("2030-06-01") + 11*hour}},
		{"2030-12-31T23Z", Span{dayOf("2030-12-31") + 23*hour, dayOf("2031-01-01")}},
		{"2030-06-01T10:00", Span{dayOf("2030-06-01") + 10*hour, dayOf("2030-06-01") + 10*hour + 60}},
		{"2030-06-01T10:00:00.5+02:00", Span{dayOf("2030-06-01") + 10*hour + 0.5, dayOf("2030-06-01") + 10*hour + 0.5}},
	}
	for _, tc := range tests {
		d, tm, err := ParseDateTime(tc.in)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", tc.in, err)
		}
		hasTime := strings.Contains(tc.in, "T")
		if got := DateTimeSpan(d, tm, hasTime); got != tc.want {
			t.Errorf("DateTimeSpan(ParseDateTime(%q), %t) = %+v, want %+v", tc.in, hasTime, got, tc.want)
		}
	}
	// A time beside a date that leaves out its day, which the parse never
	// gives, is not read: the date-time spans its date, as DateTimeFields
	// reads it.
	reduced := Date{Year: 2030, Month: 6, MonthKnown: true}
	want := Span{dayOf("2030-06-01"), dayOf("2030-07-01")}
	if got := DateTimeSpan(reduced, Time{Hour: 10, MinuteKnown: true}, true); got != want {
		t.Errorf("DateTimeSpan(2030-06, 10:00, true) = %+v, want %+v", got, want)
	}
}

// TestREQ103_OrderAcrossZone pins the order of a value with a zone against
// one with none: before where the first, read in UTC, ends before the second
// read at +14:00 starts, after where it starts after the second read at
// -14:00 ends, and not ordered where the two readings disagree, an edge the
// two share included, or where the zone does not parse.
func TestREQ103_OrderAcrossZone(t *testing.T) {
	t.Parallel()
	point := func(s float64) Span { return Span{s, s} }
	noon := point(12 * hour)
	tests := []struct {
		name     string
		zoned    Span
		zone     string
		zoneless Span
		sign     int
		ok       bool
	}{
		{name: "more than 14 hours before", zoned: point(-2*hour - 1), zone: "Z", zoneless: noon, sign: -1, ok: true},
		{name: "14 hours before, the edge shared", zoned: point(-2 * hour), zone: "Z", zoneless: noon, ok: false},
		{name: "within 14 hours before", zoned: point(0), zone: "Z", zoneless: noon, ok: false},
		{name: "at the same clock", zoned: noon, zone: "Z", zoneless: noon, ok: false},
		{name: "within 14 hours after", zoned: point(25 * hour), zone: "Z", zoneless: noon, ok: false},
		{name: "14 hours after, the edge shared", zoned: point(26 * hour), zone: "Z", zoneless: noon, ok: false},
		{name: "more than 14 hours after", zoned: point(26*hour + 1), zone: "Z", zoneless: noon, sign: 1, ok: true},
		// The zone moves the zoned span: 14:00+02:00 is 12:00 in UTC.
		{name: "read in UTC, before", zoned: point(-1 * hour), zone: "+02:00", zoneless: noon, sign: -1, ok: true},
		{name: "read in UTC, the edge shared", zoned: point(0), zone: "+02:00", zoneless: noon, ok: false},
		{name: "read in UTC, after", zoned: point(23*hour + 1), zone: "-03:00", zoneless: noon, sign: 1, ok: true},
		{name: "read in UTC, within", zoned: point(23 * hour), zone: "-03:00", zoneless: noon, ok: false},
		// A reduced value covers its whole period, on either side.
		{name: "zoned period ends before", zoned: Span{-4 * hour, -2*hour - 1}, zone: "Z", zoneless: noon, sign: -1, ok: true},
		{name: "zoned period reaches within", zoned: Span{-4 * hour, -1 * hour}, zone: "Z", zoneless: noon, ok: false},
		{name: "zoneless period reaches within", zoned: point(30 * hour), zone: "Z", zoneless: Span{12 * hour, 17 * hour}, ok: false},
		{name: "zoneless period ends before", zoned: point(31*hour + 1), zone: "Z", zoneless: Span{12 * hour, 17 * hour}, sign: 1, ok: true},
		{name: "zoneless period starts after", zoned: point(-10 * hour), zone: "Z", zoneless: Span{5 * hour, 6 * hour}, sign: -1, ok: true},
		{name: "zoneless period starts within", zoned: point(-9 * hour), zone: "Z", zoneless: Span{5 * hour, 6 * hour}, ok: false},
		{name: "no zone", zoned: point(-10 * hour), zone: "", zoneless: noon, ok: false},
		{name: "a zone the parse refuses", zoned: point(-10 * hour), zone: "+15:00", zoneless: noon, ok: false},
	}
	for _, tc := range tests {
		sign, ok := OrderAcrossZone(tc.zoned, tc.zone, tc.zoneless)
		if sign != tc.sign || ok != tc.ok {
			t.Errorf("%s: OrderAcrossZone(%+v, %q, %+v) = %d, %t; want %d, %t", tc.name, tc.zoned, tc.zone, tc.zoneless, sign, ok, tc.sign, tc.ok)
		}
	}
}

// TestREQ103_OrderAcrossZoneOfParsedValues holds OrderAcrossZone, on the
// spans of parsed values, to the rows of GitHub issue 467 and to a reduced
// bound with no zone that covers a zoned value: 2030-06-01T10:00Z is not
// ordered against 2030, which it lies inside at every reading.
func TestREQ103_OrderAcrossZoneOfParsedValues(t *testing.T) {
	t.Parallel()
	span := func(s string) (Span, string) {
		t.Helper()
		d, tm, err := ParseDateTime(s)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", s, err)
		}
		return DateTimeSpan(d, tm, strings.Contains(s, "T")), tm.Zone
	}
	tests := []struct {
		zoned, zoneless string
		sign            int
		ok              bool
	}{
		{zoned: "2022-11-02T19:32:40Z", zoneless: "2021-12-31T23:59", sign: 1, ok: true},
		{zoned: "2022-11-02T19:32:40+02:00", zoneless: "2021-12-31T23:59", sign: 1, ok: true},
		{zoned: "2021-12-31T23:59Z", zoneless: "2022-11-02T19:32:40", sign: -1, ok: true},
		{zoned: "2020-01-01T00:00Z", zoneless: "2022-11-02T19:32:40", sign: -1, ok: true},
		{zoned: "2030-06-01T10:00Z", zoneless: "2030", ok: false},
		{zoned: "2030-12-31T23:59:59-12:00", zoneless: "2030", ok: false},
		{zoned: "2031-01-01T14:00:01Z", zoneless: "2030", sign: 1, ok: true},
		{zoned: "2029-12-31T09:59:59Z", zoneless: "2030", sign: -1, ok: true},
		{zoned: "2022-01-01T09:00:00Z", zoneless: "2021-12-31T23:59", ok: false},
	}
	for _, tc := range tests {
		zoned, zone := span(tc.zoned)
		zoneless, _ := span(tc.zoneless)
		sign, ok := OrderAcrossZone(zoned, zone, zoneless)
		if sign != tc.sign || ok != tc.ok {
			t.Errorf("OrderAcrossZone(%s, %s) = %d, %t; want %d, %t", tc.zoned, tc.zoneless, sign, ok, tc.sign, tc.ok)
		}
	}
}
