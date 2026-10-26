package iso8601

import "testing"

// REQ-123 and REQ-103: the magnitudes the RM's Compare and the template
// layer's temporal range check share. ToSeconds is BASE
// Iso8601_duration.to_seconds, with the Time_definitions averages for a
// year and a month; a date's magnitude counts days from 0001-01-01, a
// reduced date standing for its first day; a time's and a date-time's are
// clock-local seconds.

func TestREQ123_DurationToSeconds(t *testing.T) {
	t.Parallel()
	const day = 86400.0
	tests := []struct {
		in   string
		want float64
	}{
		{"P0D", 0},
		{"P1Y", 365.24 * day},
		{"P1M", 30.42 * day},
		{"P1W", 7 * day},
		{"P1D", day},
		{"PT24H", day},
		{"PT1M", 60},
		{"PT1.5S", 1.5},
		{"-P1D", -day},
		{"-PT0S", 0},
		{"P1Y2M3W4DT5H6M7,25S", 365.24*day + 2*30.42*day + (3*7*day + 4*day + 5*3600 + 6*60 + 7 + 0.25)},
	}
	for _, tc := range tests {
		p, err := ParseDuration(tc.in)
		if err != nil {
			t.Fatalf("ParseDuration(%q): %v", tc.in, err)
		}
		if got := p.ToSeconds(); got != tc.want {
			t.Errorf("ParseDuration(%q).ToSeconds() = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestREQ123_DateMagnitude(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want int
	}{
		{"0001-01-01", 0},
		{"0001-01-02", 1},
		{"0002-01-01", 365},
		{"1970-01-01", 719162},
		{"2004-05-01", 731701},
		{"2004-05", 731701},
		{"2004", 731580},
		{"20040501", 731701},
	}
	for _, tc := range tests {
		p, err := ParseDate(tc.in)
		if err != nil {
			t.Fatalf("ParseDate(%q): %v", tc.in, err)
		}
		if got := p.Magnitude(); got != tc.want {
			t.Errorf("ParseDate(%q).Magnitude() = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestREQ103_DateOfMagnitude holds DateOfMagnitude to the inverse of
// Date.Magnitude on full dates, across leap days, century years and the
// years before the Unix epoch.
func TestREQ103_DateOfMagnitude(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"0001-01-01", "0004-02-29", "1600-02-29", "1700-03-01", "1899-12-31", "1969-12-31",
		"1970-01-01", "2000-02-29", "2004-05-20", "2100-02-28", "2100-03-01", "9999-12-31",
	} {
		p, err := ParseDate(s)
		if err != nil {
			t.Fatalf("ParseDate(%q): %v", s, err)
		}
		if got := DateOfMagnitude(p.Magnitude()); got != p {
			t.Errorf("DateOfMagnitude(ParseDate(%q).Magnitude()) = %+v, want %+v", s, got, p)
		}
	}
	// Every day of a span of four centuries maps back to itself.
	start := Date{Year: 1899, Month: 12, Day: 31, MonthKnown: true, DayKnown: true}.Magnitude()
	for days := start; days < start+146097+366; days++ {
		if got := DateOfMagnitude(days).Magnitude(); got != days {
			t.Fatalf("DateOfMagnitude(%d).Magnitude() = %d", days, got)
		}
	}
}

func TestREQ123_TimeAndDateTimeMagnitude(t *testing.T) {
	t.Parallel()
	times := []struct {
		in   string
		want float64
	}{
		{"00:00:00", 0},
		{"09", 9 * 3600},
		{"09:30", 9*3600 + 30*60},
		{"09:30:00.5", 9*3600 + 30*60 + 0.5},
		{"09:30:00+05:00", 9*3600 + 30*60},
		{"09:30:00-05:00", 9*3600 + 30*60},
		{"23:59:60", 86400},
	}
	for _, tc := range times {
		p, err := ParseTime(tc.in)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", tc.in, err)
		}
		if got := p.Magnitude(); got != tc.want {
			t.Errorf("ParseTime(%q).Magnitude() = %v, want %v", tc.in, got, tc.want)
		}
	}
	const may1 = 731701 * 86400.0
	stamps := []struct {
		in   string
		want float64
	}{
		{"2004-05-01", may1},
		{"2004-05", may1},
		{"2004-05-01T00", may1},
		{"2004-05-01T10:15:30.25Z", may1 + 10*3600 + 15*60 + 30.25},
		{"2004-05-01T10:15:30.25+02:00", may1 + 10*3600 + 15*60 + 30.25},
		{"20040501T101530", may1 + 10*3600 + 15*60 + 30},
	}
	for _, tc := range stamps {
		d, tm, err := ParseDateTime(tc.in)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", tc.in, err)
		}
		if got := DateTimeMagnitude(d, tm); got != tc.want {
			t.Errorf("DateTimeMagnitude(ParseDateTime(%q)) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestREQ103_UTCMagnitude pins the instant order REQ-103's temporal range
// shares with REQ-112's DV_INTERVAL bound order: a magnitude less its zone's
// offset from UTC, east positive. A value with no zone names no instant, and
// neither does a designator the zone parse refuses.
func TestREQ103_UTCMagnitude(t *testing.T) {
	t.Parallel()
	const mag = 10 * 3600.0
	tests := []struct {
		zone string
		want float64
		ok   bool
	}{
		{zone: "Z", want: mag, ok: true},
		{zone: "+00:00", want: mag, ok: true},
		{zone: "-00", want: mag, ok: true},
		{zone: "+02:00", want: mag - 7200, ok: true},
		{zone: "+0530", want: mag - 19800, ok: true},
		{zone: "-05", want: mag + 18000, ok: true},
		{zone: "+14:00", want: mag - 50400, ok: true},
		{zone: "", ok: false},
		{zone: "+15", ok: false},
		{zone: "+02:0", ok: false},
		{zone: "02:00", ok: false},
	}
	for _, tc := range tests {
		got, ok := UTCMagnitude(mag, tc.zone)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("UTCMagnitude(%v, %q) = %v, %t; want %v, %t", mag, tc.zone, got, ok, tc.want, tc.ok)
		}
	}
	// The magnitudes of two zoned values compare as their instants do:
	// 10:00+02:00 is 08:00Z, before 09:00Z, and 2004-05-20T00:30+01:00
	// is the day before's 23:30Z. Each parse and each UTCMagnitude must
	// succeed first: two failed ones give two zero magnitudes, which the
	// equality below would accept.
	parseTime := func(s string) Time {
		t.Helper()
		v, err := ParseTime(s)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", s, err)
		}
		return v
	}
	parseDateTime := func(s string) (Date, Time) {
		t.Helper()
		d, tm, err := ParseDateTime(s)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", s, err)
		}
		return d, tm
	}
	utc := func(m float64, zone string) float64 {
		t.Helper()
		got, ok := UTCMagnitude(m, zone)
		if !ok {
			t.Fatalf("UTCMagnitude(%v, %q) = %v, false; want an instant", m, zone, got)
		}
		return got
	}
	early := parseTime("10:00:00+02:00")
	late := parseTime("09:00:00Z")
	e := utc(early.Magnitude(), early.Zone)
	l := utc(late.Magnitude(), late.Zone)
	if e >= l {
		t.Errorf("UTCMagnitude of 10:00:00+02:00 = %v, of 09:00:00Z = %v; want the first below the second", e, l)
	}
	d, tm := parseDateTime("2004-05-20T00:30:00+01:00")
	prev, prevTime := parseDateTime("2004-05-19T23:30:00Z")
	a := utc(DateTimeMagnitude(d, tm), tm.Zone)
	b := utc(DateTimeMagnitude(prev, prevTime), prevTime.Zone)
	if a != b {
		t.Errorf("UTCMagnitude of 2004-05-20T00:30:00+01:00 = %v, of 2004-05-19T23:30:00Z = %v; want them equal", a, b)
	}
}
