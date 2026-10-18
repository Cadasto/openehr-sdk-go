package iso8601

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// REQ-123: the ISO 8601 date and time scanners. The two equivalence tables
// pin the parts and the exact error text for each input, partly filled parts
// on an error included. They were recorded from the scanners as they stood
// when each call still built a []string of fields, so they hold the rewrite
// to the same results and the same errors, in the same order of checks. The
// allocation pin holds valid input at zero heap allocations.

type dateScanCase struct {
	in      string
	want    Date
	wantErr string // "" when the input parses
}

type timeScanCase struct {
	in      string
	want    Time
	wantErr string // "" when the input parses
}

var dateScanCases = []dateScanCase{
	{in: "", want: Date{}, wantErr: "empty date"},
	{in: "2024", want: Date{Year: 2024}},
	{in: "202401", want: Date{Year: 2024, Month: 1, MonthKnown: true}},
	{in: "20240102", want: Date{Year: 2024, Month: 1, Day: 2, MonthKnown: true, DayKnown: true}},
	{in: "2024-01", want: Date{Year: 2024, Month: 1, MonthKnown: true}},
	{in: "2024-01-02", want: Date{Year: 2024, Month: 1, Day: 2, MonthKnown: true, DayKnown: true}},
	{in: "2024--01", want: Date{}, wantErr: `bad date "2024--01"`},
	{in: "2024-01-02-03", want: Date{}, wantErr: `bad date "2024-01-02-03"`},
	{in: "-2024", want: Date{}, wantErr: `bad date "-2024"`},
	{in: "2024-", want: Date{}, wantErr: `bad date "2024-"`},
	{in: "2024-13", want: Date{Year: 2024}, wantErr: `bad month in "2024-13"`},
	{in: "2024-02-30", want: Date{Year: 2024, Month: 2, MonthKnown: true}, wantErr: `bad day in "2024-02-30"`},
	{in: "-", want: Date{}, wantErr: `bad date "-"`},
	{in: "--", want: Date{}, wantErr: `bad date "--"`},
	{in: "---", want: Date{}, wantErr: `bad date "---"`},
	{in: "2024-01-", want: Date{}, wantErr: `bad date "2024-01-"`},
	{in: "2024-01-02-", want: Date{}, wantErr: `bad date "2024-01-02-"`},
	{in: "2024-13-01-01", want: Date{}, wantErr: `bad date "2024-13-01-01"`},
	{in: "2024-1-02", want: Date{}, wantErr: `bad date "2024-1-02"`},
	{in: "2024-01-2", want: Date{}, wantErr: `bad date "2024-01-2"`},
	{in: "24-01-02", want: Date{}, wantErr: `bad date "24-01-02"`},
	{in: "02024-01", want: Date{}, wantErr: `bad date "02024-01"`},
	{in: "2024-00", want: Date{Year: 2024}, wantErr: `bad month in "2024-00"`},
	{in: "2024-01-00", want: Date{Year: 2024, Month: 1, MonthKnown: true}, wantErr: `bad day in "2024-01-00"`},
	{in: "2024-02-29", want: Date{Year: 2024, Month: 2, Day: 29, MonthKnown: true, DayKnown: true}},
	{in: "2023-02-29", want: Date{Year: 2023, Month: 2, MonthKnown: true}, wantErr: `bad day in "2023-02-29"`},
	{in: "1900-02-29", want: Date{Year: 1900, Month: 2, MonthKnown: true}, wantErr: `bad day in "1900-02-29"`},
	{in: "2000-02-29", want: Date{Year: 2000, Month: 2, Day: 29, MonthKnown: true, DayKnown: true}},
	{in: "2024-04-31", want: Date{Year: 2024, Month: 4, MonthKnown: true}, wantErr: `bad day in "2024-04-31"`},
	{in: "2024-12-31", want: Date{Year: 2024, Month: 12, Day: 31, MonthKnown: true, DayKnown: true}},
	{in: "20241301", want: Date{Year: 2024}, wantErr: `bad month in "20241301"`},
	{in: "20240230", want: Date{Year: 2024, Month: 2, MonthKnown: true}, wantErr: `bad day in "20240230"`},
	{in: "20240100", want: Date{Year: 2024, Month: 1, MonthKnown: true}, wantErr: `bad day in "20240100"`},
	{in: "2024013", want: Date{}, wantErr: `bad date "2024013"`},
	{in: "20240", want: Date{}, wantErr: `bad date "20240"`},
	{in: "202", want: Date{}, wantErr: `bad date "202"`},
	{in: "abcd", want: Date{}, wantErr: `bad date "abcd"`},
	{in: "2024-ab", want: Date{}, wantErr: `bad date "2024-ab"`},
	{in: "2024-01-0x", want: Date{}, wantErr: `bad date "2024-01-0x"`},
	{in: "２０２４", want: Date{}, wantErr: `bad date "２０２４"`},
	{in: "+2024", want: Date{}, wantErr: `bad date "+2024"`},
	{in: "2024-01-02 ", want: Date{}, wantErr: `bad date "2024-01-02 "`},
	{in: " 2024", want: Date{}, wantErr: `bad date " 2024"`},
	{in: "0000", want: Date{}},
	{in: "9999-12-31", want: Date{Year: 9999, Month: 12, Day: 31, MonthKnown: true, DayKnown: true}},
}

var timeScanCases = []timeScanCase{
	{in: "", want: Time{}, wantErr: "empty time"},
	{in: "12", want: Time{Hour: 12}},
	{in: "1230", want: Time{Hour: 12, Minute: 30, MinuteKnown: true, basic: true}},
	{in: "123045", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, basic: true}},
	{in: "12:30", want: Time{Hour: 12, Minute: 30, MinuteKnown: true, colon: true}},
	{in: "12:30:45", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.123456789", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.123456789, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45,5", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.5, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.", want: Time{}, wantErr: `bad fractional second in "12:30:45."`},
	{in: "12:30.5", want: Time{colon: true}, wantErr: `bad time "12:30.5"`},
	{in: "12::30", want: Time{colon: true}, wantErr: `bad time "12::30"`},
	{in: "12:30:45:00", want: Time{colon: true}, wantErr: `bad time "12:30:45:00"`},
	{in: "24:00:00", want: Time{colon: true}, wantErr: `bad hour in "24:00:00"`},
	{in: "12:60", want: Time{Hour: 12, colon: true}, wantErr: `bad minute in "12:60"`},
	{in: "12:30:61", want: Time{Hour: 12, Minute: 30, MinuteKnown: true, colon: true}, wantErr: `bad second in "12:30:61"`},
	{in: "12:30:45Z", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "Z", colon: true}},
	{in: "12:30:45+05:30", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "+05:30", colon: true}},
	{in: "12:30:45-0800", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "-0800", colon: true}},
	{in: "123045+0530", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "+0530", basic: true}},
	{in: "12:30:45.1234567890123456789012345678901234567890", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.12345678901234568, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "Z", want: Time{ZoneKnown: true, Zone: "Z"}, wantErr: "empty time"},
	{in: "+05:30", want: Time{ZoneKnown: true, Zone: "+05:30"}, wantErr: "empty time"},
	{in: "-05:00", want: Time{ZoneKnown: true, Zone: "-05:00"}, wantErr: "empty time"},
	{in: "1", want: Time{}, wantErr: `bad time "1"`},
	{in: "123", want: Time{}, wantErr: `bad time "123"`},
	{in: "12345", want: Time{}, wantErr: `bad time "12345"`},
	{in: "1234567", want: Time{}, wantErr: `bad time "1234567"`},
	{in: ":", want: Time{colon: true}, wantErr: `bad time ":"`},
	{in: "::", want: Time{colon: true}, wantErr: `bad time "::"`},
	{in: ":::", want: Time{colon: true}, wantErr: `bad time ":::"`},
	{in: "12:", want: Time{colon: true}, wantErr: `bad time "12:"`},
	{in: ":30", want: Time{colon: true}, wantErr: `bad time ":30"`},
	{in: "12:30:", want: Time{colon: true}, wantErr: `bad time "12:30:"`},
	{in: "25:61:61:61", want: Time{colon: true}, wantErr: `bad time "25:61:61:61"`},
	{in: "12:30:45.5Z", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.5, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "Z", colon: true}},
	{in: "12:30:45,123+01", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.123, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "+01", colon: true}},
	{in: "12:30:60", want: Time{Hour: 12, Minute: 30, Second: 60, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:60.5", want: Time{Hour: 12, Minute: 30, Second: 60, Fraction: 0.5, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "23:59:59.999999999", want: Time{Hour: 23, Minute: 59, Second: 59, Fraction: 0.999999999, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "00:00:00", want: Time{MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.+01:00", want: Time{ZoneKnown: true, Zone: "+01:00"}, wantErr: `bad fractional second in "12:30:45.+01:00"`},
	{in: "12:30:45.1a", want: Time{}, wantErr: `bad fractional second in "12:30:45.1a"`},
	{in: "12:30:45.5.5", want: Time{}, wantErr: `bad fractional second in "12:30:45.5.5"`},
	{in: "12:30:45,5.5", want: Time{}, wantErr: `bad fractional second in "12:30:45,5.5"`},
	{in: "12.5", want: Time{}, wantErr: `bad time "12.5"`},
	{in: "1230.5", want: Time{basic: true}, wantErr: `bad time "1230.5"`},
	{in: "123045.5", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.5, MinuteKnown: true, SecondKnown: true, basic: true}},
	{in: "123045,123456789", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 0.123456789, MinuteKnown: true, SecondKnown: true, basic: true}},
	{in: "12:3a", want: Time{colon: true}, wantErr: `bad time "12:3a"`},
	{in: "12:30:45é", want: Time{colon: true}, wantErr: `bad time "12:30:45é"`},
	{in: "12:30:45+15", want: Time{ZoneKnown: true, Zone: "+15"}, wantErr: `bad timezone "+15"`},
	{in: "12:30:45-13", want: Time{ZoneKnown: true, Zone: "-13"}, wantErr: `bad timezone "-13"`},
	{in: "12:30:45+14", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "+14", colon: true}},
	{in: "12:30:45-12", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, ZoneKnown: true, Zone: "-12", colon: true}},
	{in: "12:30:45+05:3", want: Time{ZoneKnown: true, Zone: "+05:3"}, wantErr: `bad timezone "+05:3"`},
	{in: "12:30:45ZZ", want: Time{ZoneKnown: true, Zone: "ZZ"}, wantErr: `bad timezone "ZZ"`},
	{in: "12:30:45+05:30:00", want: Time{ZoneKnown: true, Zone: "+05:30:00"}, wantErr: `bad timezone "+05:30:00"`},
	{in: "12:30:45.5-", want: Time{ZoneKnown: true, Zone: "-"}, wantErr: `bad timezone "-"`},
	{in: "1230-05", want: Time{Hour: 12, Minute: 30, MinuteKnown: true, ZoneKnown: true, Zone: "-05", basic: true}},
	{in: "12:30Z", want: Time{Hour: 12, Minute: 30, MinuteKnown: true, ZoneKnown: true, Zone: "Z", colon: true}},
	{in: "12Z", want: Time{Hour: 12, ZoneKnown: true, Zone: "Z"}},
	{in: "12:30:45.000000000", want: Time{Hour: 12, Minute: 30, Second: 45, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.000000000000000000000000000001", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 1e-30, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.999999999999999999999999999999", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 1, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.9999999999999999999999999999999", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 1, MinuteKnown: true, SecondKnown: true, colon: true}},
	{in: "12:30:45.9999999999999999999999", want: Time{Hour: 12, Minute: 30, Second: 45, Fraction: 1, MinuteKnown: true, SecondKnown: true, colon: true}},
}

func TestScanTemporal_REQ123_DateEquivalence(t *testing.T) {
	t.Parallel()
	for _, tc := range dateScanCases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := scanDate(tc.in)
			if got != tc.want {
				t.Errorf("scanDate(%q) parts = %+v, want %+v", tc.in, got, tc.want)
			}
			if gotErr := errText(err); gotErr != tc.wantErr {
				t.Errorf("scanDate(%q) error = %q, want %q", tc.in, gotErr, tc.wantErr)
			}
		})
	}
}

func TestScanTemporal_REQ123_TimeEquivalence(t *testing.T) {
	t.Parallel()
	for _, tc := range timeScanCases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := scanTime(tc.in)
			if got != tc.want {
				t.Errorf("scanTime(%q) parts = %+v, want %+v", tc.in, got, tc.want)
			}
			if math.Float64bits(got.Fraction) != math.Float64bits(tc.want.Fraction) {
				t.Errorf("scanTime(%q) frac bits = %#x, want %#x", tc.in, math.Float64bits(got.Fraction), math.Float64bits(tc.want.Fraction))
			}
			if gotErr := errText(err); gotErr != tc.wantErr {
				t.Errorf("scanTime(%q) error = %q, want %q", tc.in, gotErr, tc.wantErr)
			}
		})
	}
}

// TestScanTemporal_REQ123_FractionMatchesParseFloat holds the fractional
// second bit-identical to strconv.ParseFloat("0."+digits), for both
// separators and both layouts, across fraction lengths that fit a small
// buffer and lengths that do not.
func TestScanTemporal_REQ123_FractionMatchesParseFloat(t *testing.T) {
	t.Parallel()
	digitSets := []string{
		"0", "5", "05", "123", "999", "123456789", "000000001", "999999999",
		"12345678901234567", "1234567890123456789", "12345678901234567890",
		strings.Repeat("9", 22),
		strings.Repeat("1", 29), strings.Repeat("7", 30), strings.Repeat("3", 31),
		strings.Repeat("0", 39) + "1",
		strings.Repeat("1234567890", 4),
	}
	for _, digits := range digitSets {
		want, err := strconv.ParseFloat("0."+digits, 64)
		if err != nil {
			t.Fatalf("strconv.ParseFloat(%q) = %v, want nil", "0."+digits, err)
		}
		for _, in := range []string{
			"12:30:45." + digits, "12:30:45," + digits,
			"123045." + digits, "123045," + digits + "Z",
		} {
			got, err := scanTime(in)
			if err != nil {
				t.Errorf("scanTime(%q) = %v, want nil", in, err)
				continue
			}
			if math.Float64bits(got.Fraction) != math.Float64bits(want) {
				t.Errorf("scanTime(%q) frac = %v (bits %#x), want %v (bits %#x)",
					in, got.Fraction, math.Float64bits(got.Fraction), want, math.Float64bits(want))
			}
		}
	}
}

// Sinks keep the scanners' results live, so the allocation pin measures the
// whole call.
var (
	dateSink Date
	timeSink Time
	errSink  error
)

// TestScanTemporal_REQ123_ValidInputDoesNotAllocate pins the scanners at
// zero heap allocations per call on valid input: the validation floor parses
// every DV_DATE, DV_TIME and DV_DATE_TIME value, and both bounds of every
// date-time interval it orders. Splitting the extended forms into a []string
// allocated once per call.
func TestScanTemporal_REQ123_ValidInputDoesNotAllocate(t *testing.T) {
	dates := []string{"2024", "202403", "20240315", "2024-03", "2024-03-15", "2000-02-29"}
	times := []string{
		"12", "1230", "123045", "12:30", "12:30:45",
		"12Z", "12:30Z", "12:30:45Z", "123045Z", "12:30:45+05:30", "12:30:45-0800", "123045+0530", "12:30:45+01",
		"12:30:45.5", "12:30:45,5", "12:30:45.123", "12:30:45,123456", "12:30:45.123456789", "12:30:45,123456789",
		"123045.5", "123045,123456789",
		"12:30:45.123456789Z", "12:30:45,5+05:30", "123045.123456789-0800",
	}
	dateTimes := []string{
		"2024", "2024-03", "2024-03-15", "20240315",
		"2024-03-15T12", "2024-03-15T12:30", "2024-03-15T12:30:45", "2024-03-15T12:30:45.123456789Z",
		"2024-03-15T12:30:45,5+05:30", "2024-03-15T10:30:00+0530",
		"20240315T123045", "20240315T123045.5Z", "20240315T103000+05:30",
	}
	for _, in := range dates {
		t.Run("date/"+in, func(t *testing.T) {
			if _, err := scanDate(in); err != nil {
				t.Fatalf("scanDate(%q) = %v, want nil", in, err)
			}
			if got := testing.AllocsPerRun(100, func() { dateSink, errSink = scanDate(in) }); got != 0 {
				t.Errorf("scanDate(%q) allocates %v times per call, want 0", in, got)
			}
		})
	}
	for _, in := range times {
		t.Run("time/"+in, func(t *testing.T) {
			if _, err := scanTime(in); err != nil {
				t.Fatalf("scanTime(%q) = %v, want nil", in, err)
			}
			if got := testing.AllocsPerRun(100, func() { timeSink, errSink = scanTime(in) }); got != 0 {
				t.Errorf("scanTime(%q) allocates %v times per call, want 0", in, got)
			}
		})
	}
	for _, in := range dateTimes {
		t.Run("datetime/"+in, func(t *testing.T) {
			if _, _, err := scanDateTime(in); err != nil {
				t.Fatalf("scanDateTime(%q) = %v, want nil", in, err)
			}
			if got := testing.AllocsPerRun(100, func() { dateSink, timeSink, errSink = scanDateTime(in) }); got != 0 {
				t.Errorf("scanDateTime(%q) allocates %v times per call, want 0", in, got)
			}
		})
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
