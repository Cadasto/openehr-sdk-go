package rm

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
	want    dateParts
	wantErr string // "" when the input parses
}

type timeScanCase struct {
	in      string
	want    timeParts
	wantErr string // "" when the input parses
}

var dateScanCases = []dateScanCase{
	{in: "", want: dateParts{}, wantErr: "empty date"},
	{in: "2024", want: dateParts{year: 2024}},
	{in: "202401", want: dateParts{year: 2024, month: 1, monthKnown: true}},
	{in: "20240102", want: dateParts{year: 2024, month: 1, day: 2, monthKnown: true, dayKnown: true}},
	{in: "2024-01", want: dateParts{year: 2024, month: 1, monthKnown: true}},
	{in: "2024-01-02", want: dateParts{year: 2024, month: 1, day: 2, monthKnown: true, dayKnown: true}},
	{in: "2024--01", want: dateParts{}, wantErr: `bad date "2024--01"`},
	{in: "2024-01-02-03", want: dateParts{}, wantErr: `bad date "2024-01-02-03"`},
	{in: "-2024", want: dateParts{}, wantErr: `bad date "-2024"`},
	{in: "2024-", want: dateParts{}, wantErr: `bad date "2024-"`},
	{in: "2024-13", want: dateParts{year: 2024}, wantErr: `bad month in "2024-13"`},
	{in: "2024-02-30", want: dateParts{year: 2024, month: 2, monthKnown: true}, wantErr: `bad day in "2024-02-30"`},
	{in: "-", want: dateParts{}, wantErr: `bad date "-"`},
	{in: "--", want: dateParts{}, wantErr: `bad date "--"`},
	{in: "---", want: dateParts{}, wantErr: `bad date "---"`},
	{in: "2024-01-", want: dateParts{}, wantErr: `bad date "2024-01-"`},
	{in: "2024-01-02-", want: dateParts{}, wantErr: `bad date "2024-01-02-"`},
	{in: "2024-13-01-01", want: dateParts{}, wantErr: `bad date "2024-13-01-01"`},
	{in: "2024-1-02", want: dateParts{}, wantErr: `bad date "2024-1-02"`},
	{in: "2024-01-2", want: dateParts{}, wantErr: `bad date "2024-01-2"`},
	{in: "24-01-02", want: dateParts{}, wantErr: `bad date "24-01-02"`},
	{in: "02024-01", want: dateParts{}, wantErr: `bad date "02024-01"`},
	{in: "2024-00", want: dateParts{year: 2024}, wantErr: `bad month in "2024-00"`},
	{in: "2024-01-00", want: dateParts{year: 2024, month: 1, monthKnown: true}, wantErr: `bad day in "2024-01-00"`},
	{in: "2024-02-29", want: dateParts{year: 2024, month: 2, day: 29, monthKnown: true, dayKnown: true}},
	{in: "2023-02-29", want: dateParts{year: 2023, month: 2, monthKnown: true}, wantErr: `bad day in "2023-02-29"`},
	{in: "1900-02-29", want: dateParts{year: 1900, month: 2, monthKnown: true}, wantErr: `bad day in "1900-02-29"`},
	{in: "2000-02-29", want: dateParts{year: 2000, month: 2, day: 29, monthKnown: true, dayKnown: true}},
	{in: "2024-04-31", want: dateParts{year: 2024, month: 4, monthKnown: true}, wantErr: `bad day in "2024-04-31"`},
	{in: "2024-12-31", want: dateParts{year: 2024, month: 12, day: 31, monthKnown: true, dayKnown: true}},
	{in: "20241301", want: dateParts{year: 2024}, wantErr: `bad month in "20241301"`},
	{in: "20240230", want: dateParts{year: 2024, month: 2, monthKnown: true}, wantErr: `bad day in "20240230"`},
	{in: "20240100", want: dateParts{year: 2024, month: 1, monthKnown: true}, wantErr: `bad day in "20240100"`},
	{in: "2024013", want: dateParts{}, wantErr: `bad date "2024013"`},
	{in: "20240", want: dateParts{}, wantErr: `bad date "20240"`},
	{in: "202", want: dateParts{}, wantErr: `bad date "202"`},
	{in: "abcd", want: dateParts{}, wantErr: `bad date "abcd"`},
	{in: "2024-ab", want: dateParts{}, wantErr: `bad date "2024-ab"`},
	{in: "2024-01-0x", want: dateParts{}, wantErr: `bad date "2024-01-0x"`},
	{in: "２０２４", want: dateParts{}, wantErr: `bad date "２０２４"`},
	{in: "+2024", want: dateParts{}, wantErr: `bad date "+2024"`},
	{in: "2024-01-02 ", want: dateParts{}, wantErr: `bad date "2024-01-02 "`},
	{in: " 2024", want: dateParts{}, wantErr: `bad date " 2024"`},
	{in: "0000", want: dateParts{}},
	{in: "9999-12-31", want: dateParts{year: 9999, month: 12, day: 31, monthKnown: true, dayKnown: true}},
}

var timeScanCases = []timeScanCase{
	{in: "", want: timeParts{}, wantErr: "empty time"},
	{in: "12", want: timeParts{hour: 12}},
	{in: "1230", want: timeParts{hour: 12, minute: 30, minuteKnown: true, basic: true}},
	{in: "123045", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, basic: true}},
	{in: "12:30", want: timeParts{hour: 12, minute: 30, minuteKnown: true, colon: true}},
	{in: "12:30:45", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.123456789", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.123456789, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45,5", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.5, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.", want: timeParts{}, wantErr: `bad fractional second in "12:30:45."`},
	{in: "12:30.5", want: timeParts{colon: true}, wantErr: `bad time "12:30.5"`},
	{in: "12::30", want: timeParts{colon: true}, wantErr: `bad time "12::30"`},
	{in: "12:30:45:00", want: timeParts{colon: true}, wantErr: `bad time "12:30:45:00"`},
	{in: "24:00:00", want: timeParts{colon: true}, wantErr: `bad hour in "24:00:00"`},
	{in: "12:60", want: timeParts{hour: 12, colon: true}, wantErr: `bad minute in "12:60"`},
	{in: "12:30:61", want: timeParts{hour: 12, minute: 30, minuteKnown: true, colon: true}, wantErr: `bad second in "12:30:61"`},
	{in: "12:30:45Z", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "Z", colon: true}},
	{in: "12:30:45+05:30", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "+05:30", colon: true}},
	{in: "12:30:45-0800", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "-0800", colon: true}},
	{in: "123045+0530", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "+0530", basic: true}},
	{in: "12:30:45.1234567890123456789012345678901234567890", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.12345678901234568, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "Z", want: timeParts{tzKnown: true, tz: "Z"}, wantErr: "empty time"},
	{in: "+05:30", want: timeParts{tzKnown: true, tz: "+05:30"}, wantErr: "empty time"},
	{in: "-05:00", want: timeParts{tzKnown: true, tz: "-05:00"}, wantErr: "empty time"},
	{in: "1", want: timeParts{}, wantErr: `bad time "1"`},
	{in: "123", want: timeParts{}, wantErr: `bad time "123"`},
	{in: "12345", want: timeParts{}, wantErr: `bad time "12345"`},
	{in: "1234567", want: timeParts{}, wantErr: `bad time "1234567"`},
	{in: ":", want: timeParts{colon: true}, wantErr: `bad time ":"`},
	{in: "::", want: timeParts{colon: true}, wantErr: `bad time "::"`},
	{in: ":::", want: timeParts{colon: true}, wantErr: `bad time ":::"`},
	{in: "12:", want: timeParts{colon: true}, wantErr: `bad time "12:"`},
	{in: ":30", want: timeParts{colon: true}, wantErr: `bad time ":30"`},
	{in: "12:30:", want: timeParts{colon: true}, wantErr: `bad time "12:30:"`},
	{in: "25:61:61:61", want: timeParts{colon: true}, wantErr: `bad time "25:61:61:61"`},
	{in: "12:30:45.5Z", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.5, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "Z", colon: true}},
	{in: "12:30:45,123+01", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.123, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "+01", colon: true}},
	{in: "12:30:60", want: timeParts{hour: 12, minute: 30, second: 60, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:60.5", want: timeParts{hour: 12, minute: 30, second: 60, frac: 0.5, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "23:59:59.999999999", want: timeParts{hour: 23, minute: 59, second: 59, frac: 0.999999999, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "00:00:00", want: timeParts{minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.+01:00", want: timeParts{tzKnown: true, tz: "+01:00"}, wantErr: `bad fractional second in "12:30:45.+01:00"`},
	{in: "12:30:45.1a", want: timeParts{}, wantErr: `bad fractional second in "12:30:45.1a"`},
	{in: "12:30:45.5.5", want: timeParts{}, wantErr: `bad fractional second in "12:30:45.5.5"`},
	{in: "12:30:45,5.5", want: timeParts{}, wantErr: `bad fractional second in "12:30:45,5.5"`},
	{in: "12.5", want: timeParts{}, wantErr: `bad time "12.5"`},
	{in: "1230.5", want: timeParts{basic: true}, wantErr: `bad time "1230.5"`},
	{in: "123045.5", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.5, minuteKnown: true, secondKnown: true, basic: true}},
	{in: "123045,123456789", want: timeParts{hour: 12, minute: 30, second: 45, frac: 0.123456789, minuteKnown: true, secondKnown: true, basic: true}},
	{in: "12:3a", want: timeParts{colon: true}, wantErr: `bad time "12:3a"`},
	{in: "12:30:45é", want: timeParts{colon: true}, wantErr: `bad time "12:30:45é"`},
	{in: "12:30:45+15", want: timeParts{tzKnown: true, tz: "+15"}, wantErr: `bad timezone "+15"`},
	{in: "12:30:45-13", want: timeParts{tzKnown: true, tz: "-13"}, wantErr: `bad timezone "-13"`},
	{in: "12:30:45+14", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "+14", colon: true}},
	{in: "12:30:45-12", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, tzKnown: true, tz: "-12", colon: true}},
	{in: "12:30:45+05:3", want: timeParts{tzKnown: true, tz: "+05:3"}, wantErr: `bad timezone "+05:3"`},
	{in: "12:30:45ZZ", want: timeParts{tzKnown: true, tz: "ZZ"}, wantErr: `bad timezone "ZZ"`},
	{in: "12:30:45+05:30:00", want: timeParts{tzKnown: true, tz: "+05:30:00"}, wantErr: `bad timezone "+05:30:00"`},
	{in: "12:30:45.5-", want: timeParts{tzKnown: true, tz: "-"}, wantErr: `bad timezone "-"`},
	{in: "1230-05", want: timeParts{hour: 12, minute: 30, minuteKnown: true, tzKnown: true, tz: "-05", basic: true}},
	{in: "12:30Z", want: timeParts{hour: 12, minute: 30, minuteKnown: true, tzKnown: true, tz: "Z", colon: true}},
	{in: "12Z", want: timeParts{hour: 12, tzKnown: true, tz: "Z"}},
	{in: "12:30:45.000000000", want: timeParts{hour: 12, minute: 30, second: 45, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.000000000000000000000000000001", want: timeParts{hour: 12, minute: 30, second: 45, frac: 1e-30, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.999999999999999999999999999999", want: timeParts{hour: 12, minute: 30, second: 45, frac: 1, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.9999999999999999999999999999999", want: timeParts{hour: 12, minute: 30, second: 45, frac: 1, minuteKnown: true, secondKnown: true, colon: true}},
	{in: "12:30:45.9999999999999999999999", want: timeParts{hour: 12, minute: 30, second: 45, frac: 1, minuteKnown: true, secondKnown: true, colon: true}},
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
			if math.Float64bits(got.frac) != math.Float64bits(tc.want.frac) {
				t.Errorf("scanTime(%q) frac bits = %#x, want %#x", tc.in, math.Float64bits(got.frac), math.Float64bits(tc.want.frac))
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
			if math.Float64bits(got.frac) != math.Float64bits(want) {
				t.Errorf("scanTime(%q) frac = %v (bits %#x), want %v (bits %#x)",
					in, got.frac, math.Float64bits(got.frac), want, math.Float64bits(want))
			}
		}
	}
}

// Sinks keep the scanners' results live, so the allocation pin measures the
// whole call.
var (
	dateSink dateParts
	timeSink timeParts
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
			if _, _, err := splitDateTime(in); err != nil {
				t.Fatalf("splitDateTime(%q) = %v, want nil", in, err)
			}
			if got := testing.AllocsPerRun(100, func() { dateSink, timeSink, errSink = splitDateTime(in) }); got != 0 {
				t.Errorf("splitDateTime(%q) allocates %v times per call, want 0", in, got)
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
