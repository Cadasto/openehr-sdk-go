package rm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-123 — the temporal parse accepts every form the BASE predicates
// valid_iso8601_date, valid_iso8601_time and valid_iso8601_date_time
// accept (extended and compact, dot or comma fraction, Z / ±hh / ±hhmm /
// ±hh:mm zone, partial forms) and nothing else. The values below that
// the previous parse rejected came from real fixtures.

type isoCase struct {
	value string
	want  bool
}

func TestREQ123_ValidISO8601DateForms(t *testing.T) {
	cases := []isoCase{
		{"2024-03-15", true},
		{"2024-03", true},
		{"2024", true},
		{"20240315", true},
		{"202403", true},
		{"0000", true},
		{"2024-02-29", true},  // leap year
		{"2000-02-29", true},  // divisible by 400
		{"2023-02-29", false}, // not a leap year
		{"1900-02-29", false}, // divisible by 100, not 400
		{"2024-04-31", false}, // April has 30 days
		{"20240230", false},
		{"2024-00-10", false},
		{"2024-13-01", false},
		{"2024-03-00", false},
		{"2024-3-5", false},
		{"24", false},
		{"202", false},
		{"2024031", false},
		{"2024-0315", false},
		{"202403-15", false},
		{"2024-03-15-", false},
		{"2024-03-15-01", false},
		{"+2024", false},
		{"-2024-03", false},
		{"2024-03-15 ", false},
		{"example", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := (&rm.DVDate{Value: tc.value}).ValidISO8601(); got != tc.want {
				t.Errorf("DVDate(%q).ValidISO8601() = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestREQ123_ValidISO8601TimeForms(t *testing.T) {
	cases := []isoCase{
		// extended
		{"10:30:45", true},
		{"10:30:45.5", true},
		{"10:30:45,5", true},
		{"10:30:45,000+0100", true},
		{"10:30:45Z", true},
		{"10:30:45+01", true},
		{"10:30:45+0100", true},
		{"10:30:45+01:00", true},
		{"10:30:45-08:00", true},
		{"10:30:45-0800", true},
		{"10:30:45.123456789Z", true},
		{"10:30", true},
		{"10:30Z", true},
		{"10:30+01:00", true},
		// compact
		{"103045", true},
		{"103045.5", true},
		{"103045,5Z", true},
		{"103045+0100", true},
		{"103045-08", true},
		{"1030", true},
		{"1030Z", true},
		{"10", true},
		{"10Z", true},
		// bounds
		{"00:00:00", true},
		{"23:59:59", true},
		{"23:59:60", true}, // leap second
		{"24:00:00", true},
		{"24:00", true},
		{"24", true},
		{"240000", true},
		{"24:00:01", false},
		{"24:30", false},
		{"2430", false},
		{"25:00:00", false},
		{"10:60:00", false},
		{"10:00:61", false},
		{"10:00:00+14:00", true},
		{"10:00:00+15:00", false},
		{"10:00:00-12:00", true},
		{"10:00:00-13:00", false},
		{"10:00:00+10:60", false},
		{"10:00:00+00:00", true},
		// malformed
		{"10:00:00+zz", false},
		{"10:00:00+", false},
		{"10:00:00-", false},
		{"10:00:00+1", false},
		{"10:00:00+100", false},
		{"10:00:00+01:0", false},
		{"10:00:00+01:000", false},
		{"10:00:00+0100:", false},
		{"10:00:00Z+01", false},
		{"10:00:00Zjunk", false},
		{"10:00:00z", false},
		{"10:00:00+01:00:00", false},
		{"1:00", false},
		{"10:0", false},
		{"10:00:0", false},
		{"100", false},
		{"10000", false},
		{"1000000", false},
		{"10:00:00:00", false},
		{"10:0000", false},
		{"1000:00", false},
		{"10:00:00.", false},
		{"10:00:00,", false},
		{"10:00.5", false}, // fraction needs seconds
		{"1030.5", false},
		{"10.5", false},
		{"ab:cd", false},
		{"Z", false},
		{"+01:00", false},
		{" 10:00:00", false},
		{"10:00:00 ", false},
		{"example", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := (&rm.DVTime{Value: tc.value}).ValidISO8601(); got != tc.want {
				t.Errorf("DVTime(%q).ValidISO8601() = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestREQ123_ValidISO8601DateTimeForms(t *testing.T) {
	cases := []isoCase{
		// fixture origins
		{"20251024T121033", true},
		{"20260210T111148,000+0100", true},
		{"2019-01-28T21:22:19,979+0000", true},
		// extended
		{"2025-10-24T12:10:33", true},
		{"2025-10-24T12:10:33Z", true},
		{"2025-10-24T12:10:33.5+01", true},
		{"2025-10-24T12:10:33-05:30", true},
		{"2025-10-24T12:10:33-0530", true},
		{"2025-10-24T12:10", true},
		{"2025-10-24T12", true},
		{"2025-10-24T12Z", true},
		// compact
		{"20251024T121033Z", true},
		{"20251024T121033.25-0800", true},
		{"20251024T1210", true},
		{"20251024T12", true},
		// date-only partial forms stay valid (REQ-123 / REQ-112)
		{"2024", true},
		{"2024-03", true},
		{"2024-03-05", true},
		{"20240305", true},
		{"202403", true},
		// 24:00 end of day
		{"2025-10-24T24:00:00", true},
		{"20251024T240000", true},
		{"2025-10-24T24:00:01", false},
		// out of range
		{"2025-13-01T10:00:00", false},
		{"2025-02-30T10:00:00", false},
		{"2025-10-24T25:00:00", false},
		{"2025-10-24T10:60:00", false},
		{"2025-10-24T10:00:61", false},
		// zone body
		{"2025-10-24T10:00:00+zz", false},
		{"2025-10-24T10:00:00+", false},
		{"2025-10-24T10:00:00+1", false},
		{"2025-10-24T10:00:00+15:00", false},
		{"2025-10-24T10:00:00-13:00", false},
		{"2025-10-24T10:00:00Z+01", false},
		// structure
		{"2025-10-24T", false},
		{"2025-10T10", false}, // time part needs a full date
		{"2025T10", false},
		{"2025-10-24T10:0", false},
		{"2025-10-24T10:00:00.", false},
		{"2025-10-24T10:00.5", false},
		{"2025-10-24T10:00:00 ", false},
		{"2025-10-24t10:00:00", false},
		{"2025-10-24 10:00:00", false},
		{"2025-10-24T10:00:00T10", false},
		// mixing extended and compact in the date/time body is not a form
		{"20251024T10:00:00", false},
		{"2025-10-24T100000", false},
		{"2025-10-24T1000", false},
		{"2025-1-24T10:00:00", false},
		{"example", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := (&rm.DVDateTime{Value: tc.value}).ValidISO8601(); got != tc.want {
				t.Errorf("DVDateTime(%q).ValidISO8601() = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// The newly accepted forms must decompose into the right components (the
// old parse returned zero hours for them), and convert.
func TestREQ123_WidenedFormsDecompose(t *testing.T) {
	t.Run("compact date-time without zone", func(t *testing.T) {
		dt := rm.DVDateTime{Value: "20251024T121033"}
		if dt.Year() != 2025 || dt.Month() != 10 || dt.Day() != 24 ||
			dt.Hour() != 12 || dt.Minute() != 10 || dt.Second() != 33 {
			t.Errorf("components = %d-%d-%dT%d:%d:%d", dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second())
		}
		if dt.IsPartial() || dt.Timezone() != "" || dt.FractionalSecond() != 0 {
			t.Errorf("partial=%v tz=%q frac=%v", dt.IsPartial(), dt.Timezone(), dt.FractionalSecond())
		}
		tt, err := dt.ToTime()
		if err != nil {
			t.Fatalf("ToTime = %v", err)
		}
		if tt.Hour() != 12 || tt.Minute() != 10 || tt.Second() != 33 {
			t.Errorf("ToTime = %v", tt)
		}
	})

	t.Run("comma fraction with compact zone", func(t *testing.T) {
		dt := rm.DVDateTime{Value: "2019-01-28T21:22:19,979+0000"}
		if dt.Hour() != 21 || dt.Minute() != 22 || dt.Second() != 19 {
			t.Errorf("components = %d:%d:%d", dt.Hour(), dt.Minute(), dt.Second())
		}
		if dt.FractionalSecond() != 0.979 {
			t.Errorf("frac = %v, want 0.979", dt.FractionalSecond())
		}
		if dt.Timezone() != "+0000" {
			t.Errorf("tz = %q", dt.Timezone())
		}
		tt, err := dt.ToTime()
		if err != nil {
			t.Fatalf("ToTime = %v", err)
		}
		if _, off := tt.Zone(); off != 0 {
			t.Errorf("offset = %d, want 0", off)
		}
	})

	t.Run("compact zone offset", func(t *testing.T) {
		dt := rm.DVDateTime{Value: "20260210T111148,000+0100"}
		if dt.Hour() != 11 || dt.Minute() != 11 || dt.Second() != 48 {
			t.Errorf("components = %d:%d:%d", dt.Hour(), dt.Minute(), dt.Second())
		}
		tt, err := dt.ToTime()
		if err != nil {
			t.Fatalf("ToTime = %v", err)
		}
		if _, off := tt.Zone(); off != 3600 {
			t.Errorf("offset = %d, want 3600", off)
		}
	})

	t.Run("hour-only zone", func(t *testing.T) {
		dt := rm.DVDateTime{Value: "2025-10-24T12:10:33-05"}
		tt, err := dt.ToTime()
		if err != nil {
			t.Fatalf("ToTime = %v", err)
		}
		if _, off := tt.Zone(); off != -5*3600 {
			t.Errorf("offset = %d, want -18000", off)
		}
	})

	t.Run("compact time hhmm is partial", func(t *testing.T) {
		tm := rm.DVTime{Value: "1015"}
		if tm.Hour() != 10 || tm.Minute() != 15 || tm.Second() != 0 || !tm.IsPartial() {
			t.Errorf("1015 = %d:%d:%d partial=%v", tm.Hour(), tm.Minute(), tm.Second(), tm.IsPartial())
		}
		if _, err := tm.ToTime(); err == nil {
			t.Error("ToTime(partial compact time) = nil error")
		}
	})

	t.Run("compact time hh is partial", func(t *testing.T) {
		tm := rm.DVTime{Value: "10"}
		if tm.Hour() != 10 || tm.Minute() != 0 || !tm.IsPartial() {
			t.Errorf("10 = %d:%d partial=%v", tm.Hour(), tm.Minute(), tm.IsPartial())
		}
	})

	t.Run("compact time with comma fraction", func(t *testing.T) {
		tm := rm.DVTime{Value: "103045,5-0830"}
		if tm.Hour() != 10 || tm.Minute() != 30 || tm.Second() != 45 || tm.FractionalSecond() != 0.5 || tm.Timezone() != "-0830" {
			t.Errorf("components = %d:%d:%d frac=%v tz=%q", tm.Hour(), tm.Minute(), tm.Second(), tm.FractionalSecond(), tm.Timezone())
		}
		if got := float64(tm.Magnitude()); got != 10*3600+30*60+45+0.5 {
			t.Errorf("Magnitude = %v", got)
		}
	})

	t.Run("compact date", func(t *testing.T) {
		d := rm.DVDate{Value: "20240315"}
		if d.Year() != 2024 || d.Month() != 3 || d.Day() != 15 || d.IsPartial() {
			t.Errorf("20240315 = %d-%d-%d partial=%v", d.Year(), d.Month(), d.Day(), d.IsPartial())
		}
		if (&rm.DVDate{Value: "202403"}).DayUnknown() != true {
			t.Error("202403 should be day-unknown")
		}
	})

	t.Run("compact and extended order identically", func(t *testing.T) {
		a := rm.DVDateTime{Value: "20251024T121033"}
		b := rm.DVDateTime{Value: "2025-10-24T12:10:33"}
		if a.Compare(b) != 0 {
			t.Errorf("compact vs extended magnitude: %v vs %v", a.Magnitude(), b.Magnitude())
		}
	})

	t.Run("malformed zone does not convert", func(t *testing.T) {
		if _, err := (&rm.DVDateTime{Value: "2025-10-24T10:00:00+zz"}).ToTime(); err == nil {
			t.Error("ToTime(+zz) = nil error")
		}
		if _, err := (&rm.DVTime{Value: "10:00:00+zz"}).ToTime(); err == nil {
			t.Error("DVTime.ToTime(+zz) = nil error")
		}
	})
}
