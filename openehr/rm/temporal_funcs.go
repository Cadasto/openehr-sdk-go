package rm

// REQ-123 — temporal data-value helpers.
//
// Read, inspection, comparison, and conversion helpers for the ISO
// 8601-backed temporal data values DV_DATE, DV_TIME, DV_DATE_TIME and
// DV_DURATION. Each type's `value` string is parsed on demand; the
// suppressed Magnitude / LessThan / IsStrictlyComparableTo stubs are
// implemented here (manual_impl.go), plus component accessors,
// partial-form inspection, an idiomatic Compare, and Go-bridge
// conversions (ToTime / ToDuration).
//
// The text itself is parsed by internal/iso8601, which the template
// layer's temporal validators share: openehr/template may not import this
// package, and one parser keeps the two from disagreeing on what a valid
// value is.
//
// No method panics: a nil receiver and a malformed `value` yield zero
// components, false inspection flags and a zero magnitude, never the
// parts read before the parse stopped; the fallible Go-bridge conversions
// return an error (also for partial / calendar-nominal values that cannot
// map cleanly, and for a definite duration that does not fit time.Duration).
// See docs/specifications/rm-functions.md § REQ-123 and ADR 0011.
//
// Temporal arithmetic (add / subtract / diff / multiply / negative /
// add_nominal) is out of scope and remains fail-loud generated stubs.

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
)

// ErrTemporalConversion is returned (wrapped) by ToTime / ToDuration
// when a value cannot map cleanly to a Go time.Time / time.Duration:
// the receiver is nil, the text is malformed or partial, a time names a
// leap second, a duration carries calendar-nominal year or month
// components, or a definite duration does not fit in a time.Duration.
// Detect with errors.Is(err, rm.ErrTemporalConversion).
var ErrTemporalConversion = errors.New("rm: temporal value not convertible")

// openEHR nominal duration constants (foundation_types TIME_DEFINITIONS):
// average days per year / month, used by DV_DURATION.magnitude.
const (
	nominalDaysInYear  = 365.24
	nominalDaysInMonth = 30.42
	secondsPerDay      = 86400.0
)

// leapSecond is the seconds value of a positive leap second (hh:mm:60). ISO
// 8601 allows it, but time.Date would carry it into the next minute.
const leapSecond = 60

// --- DV_DATE ------------------------------------------------------------

// isoValue is the date text, or "" when the receiver is nil. Empty text
// does not parse, so nil takes the same path as unparseable input.
func (d *DVDate) isoValue() string {
	if d == nil {
		return ""
	}
	return d.Value
}

// Year returns the year component (0 when unparseable).
func (d *DVDate) Year() int { p, _ := iso8601.ParseDate(d.isoValue()); return p.Year }

// Month returns the month component, or 0 when month-unknown.
func (d *DVDate) Month() int { p, _ := iso8601.ParseDate(d.isoValue()); return p.Month }

// Day returns the day component, or 0 when day-unknown.
func (d *DVDate) Day() int { p, _ := iso8601.ParseDate(d.isoValue()); return p.Day }

// MonthUnknown reports whether the date omits the month (e.g. "2024"). It is
// false when the value does not parse.
func (d *DVDate) MonthUnknown() bool {
	p, err := iso8601.ParseDate(d.isoValue())
	return err == nil && !p.MonthKnown
}

// DayUnknown reports whether the date omits the day (e.g. "2024-03"). It is
// false when the value does not parse.
func (d *DVDate) DayUnknown() bool {
	p, err := iso8601.ParseDate(d.isoValue())
	return err == nil && !p.DayKnown
}

// IsPartial reports whether the date is reduced (day or more missing). It is
// false when the value does not parse.
func (d *DVDate) IsPartial() bool { return d.DayUnknown() }

// Magnitude returns the number of days since the calendar origin
// 0001-01-01 (a legitimately-partial value counts unknown month/day as
// 1). A malformed value returns 0 rather than a fabricated magnitude, so
// Compare does not silently mis-order garbage.
func (d *DVDate) Magnitude() Integer {
	p, err := iso8601.ParseDate(d.isoValue())
	if err != nil {
		return 0
	}
	return Integer(dateMagnitudeDays(p))
}

// Compare orders this date against other by magnitude (-1 / 0 / +1).
func (d *DVDate) Compare(other DVDate) int { return cmpInt(int(d.Magnitude()), int(other.Magnitude())) }

// LessThan reports whether this date precedes other (by magnitude).
func (d *DVDate) LessThan(other DVDate) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two dates.
func (d *DVDate) IsStrictlyComparableTo(other DVDate) bool { return true }

// ToTime converts a full date to a time.Time at midnight UTC, or returns
// ErrTemporalConversion when the receiver is nil or the value is partial
// or malformed.
func (d *DVDate) ToTime() (time.Time, error) {
	p, err := iso8601.ParseDate(d.isoValue())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if !p.DayKnown {
		return time.Time{}, fmt.Errorf("%w: date %q is partial", ErrTemporalConversion, d.isoValue())
	}
	return time.Date(p.Year, time.Month(p.Month), p.Day, 0, 0, 0, 0, time.UTC), nil
}

// --- DV_TIME ------------------------------------------------------------

// isoValue is the time text, or "" when the receiver is nil.
func (d *DVTime) isoValue() string {
	if d == nil {
		return ""
	}
	return d.Value
}

// Hour returns the hour component (0 when unparseable).
func (d *DVTime) Hour() int { p, _ := iso8601.ParseTime(d.isoValue()); return p.Hour }

// Minute returns the minute component, or 0 when minute-unknown.
func (d *DVTime) Minute() int { p, _ := iso8601.ParseTime(d.isoValue()); return p.Minute }

// Second returns the second component, or 0 when second-unknown.
func (d *DVTime) Second() int { p, _ := iso8601.ParseTime(d.isoValue()); return p.Second }

// FractionalSecond returns the fractional-second component (0 when
// absent).
func (d *DVTime) FractionalSecond() float64 {
	p, _ := iso8601.ParseTime(d.isoValue())
	return p.Fraction
}

// Timezone returns the timezone designator (e.g. "Z", "+02:00"), or ""
// when none is present.
func (d *DVTime) Timezone() string { p, _ := iso8601.ParseTime(d.isoValue()); return p.Zone }

// IsPartial reports whether the time is reduced (second or more missing). It
// is false when the value does not parse.
func (d *DVTime) IsPartial() bool {
	p, err := iso8601.ParseTime(d.isoValue())
	return err == nil && !p.SecondKnown
}

// Magnitude returns the number of seconds since the start of day. The
// value is clock-local: the timezone offset is not normalized away (per
// the openEHR DV_TIME.magnitude definition), so two instants equal in
// UTC but stated in different zones do not compare equal. A malformed
// value returns 0.
func (d *DVTime) Magnitude() Real {
	p, err := iso8601.ParseTime(d.isoValue())
	if err != nil {
		return 0
	}
	return Real(float64(p.Hour*3600+p.Minute*60+p.Second) + p.Fraction)
}

// Compare orders this time against other by magnitude (-1 / 0 / +1).
func (d *DVTime) Compare(other DVTime) int {
	return cmpFloat(float64(d.Magnitude()), float64(other.Magnitude()))
}

// LessThan reports whether this time precedes other (by magnitude).
func (d *DVTime) LessThan(other DVTime) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two times.
func (d *DVTime) IsStrictlyComparableTo(other DVTime) bool { return true }

// ToTime converts a full time-of-day to a time.Time on the reference
// date 0000-01-01, or returns ErrTemporalConversion when the receiver is
// nil, the value is partial or malformed, or it names a leap second
// (second 60), which a time.Time cannot hold.
func (d *DVTime) ToTime() (time.Time, error) {
	p, err := iso8601.ParseTime(d.isoValue())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if !p.SecondKnown {
		return time.Time{}, fmt.Errorf("%w: time %q is partial", ErrTemporalConversion, d.isoValue())
	}
	if p.Second == leapSecond {
		return time.Time{}, fmt.Errorf("%w: time names a leap second", ErrTemporalConversion)
	}
	loc, err := tzLocation(p)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	return time.Date(0, 1, 1, p.Hour, p.Minute, p.Second, int(p.Fraction*1e9), loc), nil
}

// --- DV_DATE_TIME -------------------------------------------------------

// isoValue is the date-time text, or "" when the receiver is nil.
func (d *DVDateTime) isoValue() string {
	if d == nil {
		return ""
	}
	return d.Value
}

// split parses the date-time's value into its date and time parts. On an
// error both parts are zero, so no accessor reports a part read before the
// parse stopped.
func (d *DVDateTime) split() (iso8601.Date, iso8601.Time, error) {
	return iso8601.ParseDateTime(d.isoValue())
}

// Year returns the year component.
func (d *DVDateTime) Year() int { dp, _, _ := d.split(); return dp.Year }

// Month returns the month component, or 0 when unknown.
func (d *DVDateTime) Month() int { dp, _, _ := d.split(); return dp.Month }

// Day returns the day component, or 0 when unknown.
func (d *DVDateTime) Day() int { dp, _, _ := d.split(); return dp.Day }

// Hour returns the hour component.
func (d *DVDateTime) Hour() int { _, tp, _ := d.split(); return tp.Hour }

// Minute returns the minute component.
func (d *DVDateTime) Minute() int { _, tp, _ := d.split(); return tp.Minute }

// Second returns the second component.
func (d *DVDateTime) Second() int { _, tp, _ := d.split(); return tp.Second }

// FractionalSecond returns the fractional-second component.
func (d *DVDateTime) FractionalSecond() float64 { _, tp, _ := d.split(); return tp.Fraction }

// Timezone returns the timezone designator, or "" when none.
func (d *DVDateTime) Timezone() string { _, tp, _ := d.split(); return tp.Zone }

// MonthUnknown reports whether the date side omits the month. It is false
// when the value does not parse.
func (d *DVDateTime) MonthUnknown() bool {
	dp, _, err := d.split()
	return err == nil && !dp.MonthKnown
}

// DayUnknown reports whether the date side omits the day. It is false when
// the value does not parse.
func (d *DVDateTime) DayUnknown() bool {
	dp, _, err := d.split()
	return err == nil && !dp.DayKnown
}

// IsPartial reports whether the date-time is reduced (second or more
// missing, including a missing time part entirely). It is false when the
// value does not parse.
func (d *DVDateTime) IsPartial() bool {
	dp, tp, err := d.split()
	return err == nil && (!dp.DayKnown || !tp.SecondKnown)
}

// Magnitude returns the number of seconds since the calendar origin
// 0001-01-01T00:00:00. The value is clock-local (the timezone offset is
// not normalized away, per the openEHR definition). A malformed value
// returns 0.
func (d *DVDateTime) Magnitude() float64 {
	dp, tp, err := d.split()
	if err != nil {
		return 0
	}
	return float64(dateMagnitudeDays(dp))*secondsPerDay + float64(tp.Hour*3600+tp.Minute*60+tp.Second) + tp.Fraction
}

// Compare orders this date-time against other by magnitude.
func (d *DVDateTime) Compare(other DVDateTime) int { return cmpFloat(d.Magnitude(), other.Magnitude()) }

// LessThan reports whether this date-time precedes other.
func (d *DVDateTime) LessThan(other DVDateTime) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two date-times.
func (d *DVDateTime) IsStrictlyComparableTo(other DVDateTime) bool { return true }

// ToTime converts a full date-time to a time.Time (UTC when no timezone
// is present), or returns ErrTemporalConversion when the receiver is nil,
// the value is partial or malformed, or it names a leap second (second
// 60), which a time.Time cannot hold.
func (d *DVDateTime) ToTime() (time.Time, error) {
	dp, tp, err := d.split()
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date-time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if !dp.DayKnown || !tp.SecondKnown {
		return time.Time{}, fmt.Errorf("%w: date-time %q is partial", ErrTemporalConversion, d.isoValue())
	}
	if tp.Second == leapSecond {
		return time.Time{}, fmt.Errorf("%w: date-time names a leap second", ErrTemporalConversion)
	}
	loc, err := tzLocation(tp)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date-time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	return time.Date(dp.Year, time.Month(dp.Month), dp.Day, tp.Hour, tp.Minute, tp.Second, int(tp.Fraction*1e9), loc), nil
}

// --- DV_DURATION --------------------------------------------------------

// isoValue is the duration text, or "" when the receiver is nil.
func (d *DVDuration) isoValue() string {
	if d == nil {
		return ""
	}
	return d.Value
}

// Years returns the years component.
func (d *DVDuration) Years() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Years }

// Months returns the months component.
func (d *DVDuration) Months() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Months }

// Weeks returns the weeks component.
func (d *DVDuration) Weeks() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Weeks }

// Days returns the days component.
func (d *DVDuration) Days() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Days }

// Hours returns the hours component.
func (d *DVDuration) Hours() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Hours }

// Minutes returns the minutes component.
func (d *DVDuration) Minutes() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Minutes }

// Seconds returns the whole-seconds component.
func (d *DVDuration) Seconds() int { p, _ := iso8601.ParseDuration(d.isoValue()); return p.Seconds }

// FractionalSeconds returns the fractional-second component.
func (d *DVDuration) FractionalSeconds() float64 {
	p, _ := iso8601.ParseDuration(d.isoValue())
	return p.Fraction
}

// IsNegative reports whether the duration carries a leading minus sign
// (openEHR deviation from ISO 8601). It is false when the value does not
// parse.
func (d *DVDuration) IsNegative() bool {
	p, _ := iso8601.ParseDuration(d.isoValue())
	return p.Negative
}

// Magnitude returns the duration as a number of seconds, using the
// openEHR nominal year (365.24 d) and month (30.42 d) averages for the
// calendar-nominal components. Negative when the duration is negative.
// Components are scaled in floating point, so a long duration keeps its
// length in seconds within float64 precision.
func (d *DVDuration) Magnitude() float64 {
	p, err := iso8601.ParseDuration(d.isoValue())
	if err != nil {
		return 0
	}
	secs := float64(p.Years)*nominalDaysInYear*secondsPerDay +
		float64(p.Months)*nominalDaysInMonth*secondsPerDay +
		definiteSeconds(p)
	if p.Negative {
		return -secs
	}
	return secs
}

// Compare orders this duration against other by magnitude.
func (d *DVDuration) Compare(other DVDuration) int { return cmpFloat(d.Magnitude(), other.Magnitude()) }

// LessThan reports whether this duration is shorter than other.
func (d *DVDuration) LessThan(other DVDuration) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two durations.
func (d *DVDuration) IsStrictlyComparableTo(other DVDuration) bool { return true }

// ToDuration converts a definite duration to a time.Duration, or returns
// ErrTemporalConversion when the receiver is nil, the value is malformed,
// it carries calendar-nominal years or months (which have no fixed length),
// or the length does not fit in a time.Duration. Weeks and days are treated
// as definite (7 d, 24 h), and the fractional second is rounded to the
// nearest nanosecond.
func (d *DVDuration) ToDuration() (time.Duration, error) {
	p, err := iso8601.ParseDuration(d.isoValue())
	if err != nil {
		return 0, fmt.Errorf("%w: duration %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if p.Years != 0 || p.Months != 0 {
		return 0, fmt.Errorf("%w: duration %q has calendar-nominal Y/M components", ErrTemporalConversion, d.isoValue())
	}
	ns, ok := definiteNanos(p)
	if !ok {
		return 0, fmt.Errorf("%w: duration %q does not fit in a time.Duration", ErrTemporalConversion, d.isoValue())
	}
	return time.Duration(ns), nil
}

// definiteSeconds is the length of the week, day and clock components in
// seconds. Each component is converted to float64 before it is scaled, so
// a large hour count is not multiplied in int.
func definiteSeconds(p iso8601.Duration) float64 {
	return float64(p.Weeks)*7*secondsPerDay +
		float64(p.Days)*secondsPerDay +
		float64(p.Hours)*3600 +
		float64(p.Minutes)*60 +
		float64(p.Seconds) + p.Fraction
}

// definiteNanos is the week, day and clock length in nanoseconds, with the
// sign applied. Whole seconds are counted in integers, so a length one
// nanosecond outside the int64 range is not rounded onto either extreme.
// The bool is false when that length does not fit.
func definiteNanos(p iso8601.Duration) (int64, bool) {
	mag, ok := unsignedNanos(p)
	if !ok {
		return 0, false
	}
	if p.Negative {
		// 2^63 nanoseconds is math.MinInt64; one more does not fit.
		const minMag = uint64(math.MaxInt64) + 1
		if mag > minMag {
			return 0, false
		}
		if mag == minMag {
			return math.MinInt64, true
		}
		return -int64(mag), true
	}
	if mag > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(mag), true
}

// unsignedNanos is the absolute length in nanoseconds. Only the fractional
// second is a float, rounded to the nearest nanosecond.
func unsignedNanos(p iso8601.Duration) (uint64, bool) {
	days, ok := mulU(uint64(p.Weeks), 7)
	if !ok {
		return 0, false
	}
	days, ok = addU(days, uint64(p.Days))
	if !ok {
		return 0, false
	}
	secs, ok := mulU(days, 86400)
	if !ok {
		return 0, false
	}
	hours, ok := mulU(uint64(p.Hours), 3600)
	if !ok {
		return 0, false
	}
	mins, ok := mulU(uint64(p.Minutes), 60)
	if !ok {
		return 0, false
	}
	secs, ok = addU(secs, hours)
	if !ok {
		return 0, false
	}
	secs, ok = addU(secs, mins)
	if !ok {
		return 0, false
	}
	secs, ok = addU(secs, uint64(p.Seconds))
	if !ok {
		return 0, false
	}
	ns, ok := mulU(secs, 1_000_000_000)
	if !ok {
		return 0, false
	}
	// A fraction just below one second can round up to 1e9 nanoseconds, a
	// whole second; adding it carries that second, and addU catches overflow.
	return addU(ns, uint64(math.Round(p.Fraction*1e9)))
}

func mulU(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	if c/a != b {
		return 0, false
	}
	return c, true
}

func addU(a, b uint64) (uint64, bool) {
	c := a + b
	if c < a {
		return 0, false
	}
	return c, true
}

// dateMagnitudeDays returns days since 0001-01-01, treating an unknown
// month or day as 1.
func dateMagnitudeDays(p iso8601.Date) int {
	m, dd := p.Month, p.Day
	if !p.MonthKnown {
		m = 1
	}
	if !p.DayKnown {
		dd = 1
	}
	return daysFromCivil(p.Year, m, dd) - daysFromCivil(1, 1, 1)
}

// daysFromCivil returns the number of days since the Unix epoch
// (1970-01-01) for a proleptic-Gregorian date (Howard Hinnant's
// algorithm). Used as a stable day index for date magnitude across the
// multi-millennium span where time.Duration would overflow.
func daysFromCivil(y, m, d int) int {
	if m <= 2 {
		y--
	}
	var era int
	if y >= 0 {
		era = y / 400
	} else {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// tzLocation builds a *time.Location from parsed timezone parts,
// defaulting to UTC when absent.
func tzLocation(p iso8601.Time) (*time.Location, error) {
	if !p.ZoneKnown || p.Zone == "" || p.Zone == "Z" {
		return time.UTC, nil
	}
	off, err := iso8601.ParseZone(p.Zone)
	if err != nil {
		return nil, err
	}
	return time.FixedZone(p.Zone, off), nil
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
