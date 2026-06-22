package rm_test

import (
	"errors"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-123 — temporal data-value helpers.

func TestDVDateComponentsAndPartial(t *testing.T) {
	full := rm.DVDate{Value: "2024-03-15"}
	if full.Year() != 2024 || full.Month() != 3 || full.Day() != 15 {
		t.Errorf("full = %d-%d-%d", full.Year(), full.Month(), full.Day())
	}
	if full.IsPartial() || full.MonthUnknown() || full.DayUnknown() {
		t.Error("full date reported partial")
	}

	yearMonth := rm.DVDate{Value: "2024-03"}
	if !yearMonth.DayUnknown() || yearMonth.MonthUnknown() || !yearMonth.IsPartial() {
		t.Error("2024-03 partial flags wrong")
	}

	yearOnly := rm.DVDate{Value: "2024"}
	if !yearOnly.MonthUnknown() || !yearOnly.DayUnknown() || !yearOnly.IsPartial() {
		t.Error("2024 partial flags wrong")
	}
}

func TestDVDateOrderingAndConversion(t *testing.T) {
	seq := []rm.DVDate{
		{Value: "2023-12-31"},
		{Value: "2024-01-01"},
		{Value: "2024-03"},
		{Value: "2025"},
	}
	for i := 1; i < len(seq); i++ {
		if !seq[i-1].LessThan(seq[i]) {
			t.Errorf("expected %q < %q", seq[i-1].Value, seq[i].Value)
		}
		if seq[i-1].Compare(seq[i]) != -1 {
			t.Errorf("Compare(%q,%q) != -1", seq[i-1].Value, seq[i].Value)
		}
	}

	tt, err := (&rm.DVDate{Value: "2024-03-15"}).ToTime()
	if err != nil {
		t.Fatalf("ToTime(full) = %v", err)
	}
	if tt.Year() != 2024 || tt.Month() != time.March || tt.Day() != 15 {
		t.Errorf("ToTime = %v", tt)
	}
	if _, err := (&rm.DVDate{Value: "2024-03"}).ToTime(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToTime(partial) err = %v, want ErrTemporalConversion", err)
	}
}

func TestDVTime(t *testing.T) {
	tm := rm.DVTime{Value: "10:30:45.5Z"}
	if tm.Hour() != 10 || tm.Minute() != 30 || tm.Second() != 45 {
		t.Errorf("components = %d:%d:%d", tm.Hour(), tm.Minute(), tm.Second())
	}
	if tm.FractionalSecond() != 0.5 {
		t.Errorf("frac = %v", tm.FractionalSecond())
	}
	if tm.Timezone() != "Z" {
		t.Errorf("tz = %q", tm.Timezone())
	}
	if tm.IsPartial() {
		t.Error("full time reported partial")
	}
	if got := float64(tm.Magnitude()); got != 10*3600+30*60+45+0.5 {
		t.Errorf("Magnitude = %v", got)
	}

	partialTime := rm.DVTime{Value: "10:30"}
	if !partialTime.IsPartial() {
		t.Error("10:30 should be partial")
	}
	if _, err := (&rm.DVTime{Value: "10:30"}).ToTime(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToTime(partial time) err = %v", err)
	}
}

func TestDVDateTime(t *testing.T) {
	dt := rm.DVDateTime{Value: "2024-03-15T10:30:00+02:00"}
	if dt.Year() != 2024 || dt.Hour() != 10 || dt.Minute() != 30 {
		t.Errorf("components wrong: %v", dt)
	}
	if dt.IsPartial() {
		t.Error("full date-time reported partial")
	}
	tt, err := dt.ToTime()
	if err != nil {
		t.Fatalf("ToTime = %v", err)
	}
	if _, off := tt.Zone(); off != 2*3600 {
		t.Errorf("zone offset = %d, want 7200", off)
	}

	// Ordering by magnitude.
	earlier := rm.DVDateTime{Value: "2024-03-15T10:00:00"}
	later := rm.DVDateTime{Value: "2024-03-15T11:00:00"}
	if !earlier.LessThan(later) {
		t.Error("10:00 should be < 11:00")
	}

	partial := rm.DVDateTime{Value: "2024-03"}
	if !partial.IsPartial() {
		t.Error("2024-03 date-time should be partial")
	}
	if _, err := partial.ToTime(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToTime(partial) err = %v", err)
	}
}

func TestDVDuration(t *testing.T) {
	d := rm.DVDuration{Value: "P1Y2M3W4DT5H6M7.5S"}
	if d.Years() != 1 || d.Months() != 2 || d.Weeks() != 3 || d.Days() != 4 {
		t.Errorf("date components = %d/%d/%d/%d", d.Years(), d.Months(), d.Weeks(), d.Days())
	}
	if d.Hours() != 5 || d.Minutes() != 6 || d.Seconds() != 7 || d.FractionalSeconds() != 0.5 {
		t.Errorf("time components = %d/%d/%d/%v", d.Hours(), d.Minutes(), d.Seconds(), d.FractionalSeconds())
	}

	neg := rm.DVDuration{Value: "-P10D"}
	if !neg.IsNegative() || neg.Magnitude() >= 0 {
		t.Errorf("negative duration: isNeg=%v mag=%v", neg.IsNegative(), neg.Magnitude())
	}

	// PT1H == PT60M by magnitude.
	if (&rm.DVDuration{Value: "PT1H"}).Compare(rm.DVDuration{Value: "PT60M"}) != 0 {
		t.Error("PT1H should equal PT60M")
	}

	// Definite duration converts.
	got, err := (&rm.DVDuration{Value: "PT1H30M"}).ToDuration()
	if err != nil {
		t.Fatalf("ToDuration(definite) = %v", err)
	}
	if got != 90*time.Minute {
		t.Errorf("ToDuration = %v, want 1h30m", got)
	}

	// Week/day definite conversion.
	if wd, err := (&rm.DVDuration{Value: "P1W"}).ToDuration(); err != nil || wd != 7*24*time.Hour {
		t.Errorf("ToDuration(P1W) = %v, %v", wd, err)
	}

	// Calendar-nominal Y/M cannot convert.
	if _, err := (&rm.DVDuration{Value: "P1Y"}).ToDuration(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToDuration(P1Y) err = %v, want ErrTemporalConversion", err)
	}
}

func TestDVDurationMonthVsMinute(t *testing.T) {
	months := rm.DVDuration{Value: "P2M"}
	if months.Months() != 2 || months.Minutes() != 0 {
		t.Errorf("P2M = %d months / %d minutes, want 2/0", months.Months(), months.Minutes())
	}
	minutes := rm.DVDuration{Value: "PT2M"}
	if minutes.Minutes() != 2 || minutes.Months() != 0 {
		t.Errorf("PT2M = %d months / %d minutes, want 0/2", minutes.Months(), minutes.Minutes())
	}
}

func TestDVDurationFractionalOnlyOnSeconds(t *testing.T) {
	// A fraction on a non-second component is malformed (openEHR carries
	// a fraction only on seconds) — must not silently truncate.
	if _, err := (&rm.DVDuration{Value: "PT2.5H"}).ToDuration(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToDuration(PT2.5H) err = %v, want ErrTemporalConversion", err)
	}
	if m := (&rm.DVDuration{Value: "PT2.5H"}).Magnitude(); m != 0 {
		t.Errorf("Magnitude(PT2.5H) = %v, want 0 (malformed)", m)
	}
	// Fractional seconds remain valid.
	got, err := (&rm.DVDuration{Value: "PT1.5S"}).ToDuration()
	if err != nil || got != 1500*time.Millisecond {
		t.Errorf("ToDuration(PT1.5S) = %v, %v; want 1.5s", got, err)
	}
}

func TestDVDateTimeNegativeAndBadTimezone(t *testing.T) {
	tt, err := (&rm.DVDateTime{Value: "2024-03-15T10:30:00-05:00"}).ToTime()
	if err != nil {
		t.Fatalf("ToTime(-05:00) = %v", err)
	}
	if _, off := tt.Zone(); off != -5*3600 {
		t.Errorf("zone offset = %d, want -18000", off)
	}
	if _, err := (&rm.DVDateTime{Value: "2024-03-15T10:30:00+9"}).ToTime(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToTime(malformed tz) err = %v, want ErrTemporalConversion", err)
	}
}

func TestDVDateMalformedMagnitudeIsZero(t *testing.T) {
	// Malformed values must not fabricate a comparable magnitude.
	for _, bad := range []string{"2024-13-01", "2024-02-30x", "garbage"} {
		d := rm.DVDate{Value: bad}
		if m := d.Magnitude(); m != 0 {
			t.Errorf("Magnitude(%q) = %d, want 0", bad, m)
		}
	}
	// A legitimately-partial value keeps a real magnitude.
	partial := rm.DVDate{Value: "2024"}
	if partial.Magnitude() == 0 {
		t.Error("Magnitude(2024) should be non-zero")
	}
}

func TestTemporalMalformedNoPanic(t *testing.T) {
	// Best-effort accessors return zero values; no panic.
	bad := rm.DVDate{Value: "garbage"}
	_ = bad.Year()
	_ = bad.Magnitude()
	if _, err := bad.ToTime(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToTime(garbage) err = %v", err)
	}
	if _, err := (&rm.DVDuration{Value: "nonsense"}).ToDuration(); !errors.Is(err, rm.ErrTemporalConversion) {
		t.Errorf("ToDuration(nonsense) err = %v", err)
	}
}

func TestDVDurationWeekMixing(t *testing.T) {
	// openEHR permits the W designator mixed with others (deviation).
	d := rm.DVDuration{Value: "P1Y2W3D"}
	if d.Years() != 1 || d.Weeks() != 2 || d.Days() != 3 {
		t.Errorf("P1Y2W3D = %dY/%dW/%dD, want 1/2/3", d.Years(), d.Weeks(), d.Days())
	}
	if d.Magnitude() <= 0 {
		t.Errorf("Magnitude(P1Y2W3D) = %v, want > 0", d.Magnitude())
	}
}

func TestDVTimeMagnitudeIsClockLocal(t *testing.T) {
	// Magnitude is clock-local — the timezone offset is not normalized,
	// so the same instant in different zones does not compare equal.
	utc := rm.DVTime{Value: "10:00:00Z"}
	plus2 := rm.DVTime{Value: "12:00:00+02:00"} // same instant as 10:00Z
	if !utc.LessThan(plus2) {
		t.Error("expected clock-local 10:00 < 12:00 (tz ignored)")
	}
}
