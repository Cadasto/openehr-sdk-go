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
	"strconv"
	"strings"
	"time"
)

// ErrTemporalConversion is returned (wrapped) by ToTime / ToDuration
// when a value cannot map cleanly to a Go time.Time / time.Duration:
// the receiver is nil, the text is malformed or partial, a duration
// carries calendar-nominal year or month components, or a definite
// duration does not fit in a time.Duration.
// Detect with errors.Is(err, rm.ErrTemporalConversion).
var ErrTemporalConversion = errors.New("rm: temporal value not convertible")

// openEHR nominal duration constants (foundation_types TIME_DEFINITIONS):
// average days per year / month, used by DV_DURATION.magnitude.
const (
	nominalDaysInYear  = 365.24
	nominalDaysInMonth = 30.42
	secondsPerDay      = 86400.0
)

// --- parsed component structs -------------------------------------------

type dateParts struct {
	year, month, day     int
	monthKnown, dayKnown bool
}

type timeParts struct {
	hour, minute, second              int
	frac                              float64
	minuteKnown, secondKnown, tzKnown bool
	tz                                string
	// colon marks the extended form (hh:mm[:ss]); basic marks a compact body
	// longer than hh (hhmm[ss]). A bare hh is neither, so it fits both a
	// compact and an extended date-time.
	colon, basic bool
}

type durationParts struct {
	neg                                                 bool
	years, months, weeks, days, hours, minutes, seconds int
	frac                                                float64
}

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
func (d *DVDate) Year() int { p, _ := parseDate(d.isoValue()); return p.year }

// Month returns the month component, or 0 when month-unknown.
func (d *DVDate) Month() int { p, _ := parseDate(d.isoValue()); return p.month }

// Day returns the day component, or 0 when day-unknown.
func (d *DVDate) Day() int { p, _ := parseDate(d.isoValue()); return p.day }

// MonthUnknown reports whether the date omits the month (e.g. "2024"). It is
// false when the value does not parse.
func (d *DVDate) MonthUnknown() bool {
	p, err := parseDate(d.isoValue())
	return err == nil && !p.monthKnown
}

// DayUnknown reports whether the date omits the day (e.g. "2024-03"). It is
// false when the value does not parse.
func (d *DVDate) DayUnknown() bool {
	p, err := parseDate(d.isoValue())
	return err == nil && !p.dayKnown
}

// IsPartial reports whether the date is reduced (day or more missing). It is
// false when the value does not parse.
func (d *DVDate) IsPartial() bool { return d.DayUnknown() }

// Magnitude returns the number of days since the calendar origin
// 0001-01-01 (a legitimately-partial value counts unknown month/day as
// 1). A malformed value returns 0 rather than a fabricated magnitude, so
// Compare does not silently mis-order garbage.
func (d *DVDate) Magnitude() Integer {
	p, err := parseDate(d.isoValue())
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
	p, err := parseDate(d.isoValue())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if !p.dayKnown {
		return time.Time{}, fmt.Errorf("%w: date %q is partial", ErrTemporalConversion, d.isoValue())
	}
	return time.Date(p.year, time.Month(p.month), p.day, 0, 0, 0, 0, time.UTC), nil
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
func (d *DVTime) Hour() int { p, _ := parseTime(d.isoValue()); return p.hour }

// Minute returns the minute component, or 0 when minute-unknown.
func (d *DVTime) Minute() int { p, _ := parseTime(d.isoValue()); return p.minute }

// Second returns the second component, or 0 when second-unknown.
func (d *DVTime) Second() int { p, _ := parseTime(d.isoValue()); return p.second }

// FractionalSecond returns the fractional-second component (0 when
// absent).
func (d *DVTime) FractionalSecond() float64 { p, _ := parseTime(d.isoValue()); return p.frac }

// Timezone returns the timezone designator (e.g. "Z", "+02:00"), or ""
// when none is present.
func (d *DVTime) Timezone() string { p, _ := parseTime(d.isoValue()); return p.tz }

// IsPartial reports whether the time is reduced (second or more missing). It
// is false when the value does not parse.
func (d *DVTime) IsPartial() bool {
	p, err := parseTime(d.isoValue())
	return err == nil && !p.secondKnown
}

// Magnitude returns the number of seconds since the start of day. The
// value is clock-local: the timezone offset is not normalized away (per
// the openEHR DV_TIME.magnitude definition), so two instants equal in
// UTC but stated in different zones do not compare equal. A malformed
// value returns 0.
func (d *DVTime) Magnitude() Real {
	p, err := parseTime(d.isoValue())
	if err != nil {
		return 0
	}
	return Real(float64(p.hour*3600+p.minute*60+p.second) + p.frac)
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
// nil or the value is partial or malformed.
func (d *DVTime) ToTime() (time.Time, error) {
	p, err := parseTime(d.isoValue())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if !p.secondKnown {
		return time.Time{}, fmt.Errorf("%w: time %q is partial", ErrTemporalConversion, d.isoValue())
	}
	loc, err := tzLocation(p)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	return time.Date(0, 1, 1, p.hour, p.minute, p.second, int(p.frac*1e9), loc), nil
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
func (d *DVDateTime) split() (dateParts, timeParts, error) {
	dp, tp, err := splitDateTime(d.isoValue())
	if err != nil {
		return dateParts{}, timeParts{}, err
	}
	return dp, tp, nil
}

// splitDateTime does the work of split. On an error the parts it returns may
// be partly filled.
func splitDateTime(value string) (dateParts, timeParts, error) {
	datePart, timePart, hasT := strings.Cut(value, "T")
	dp, derr := parseDate(datePart)
	if derr != nil {
		return dp, timeParts{}, derr
	}
	if !hasT {
		return dp, timeParts{}, nil
	}
	if !dp.dayKnown {
		return dp, timeParts{}, fmt.Errorf("time part needs a full date in %q", value)
	}
	tp, terr := parseTime(timePart)
	if terr != nil {
		return dp, tp, terr
	}
	// The date and time bodies are both extended or both compact; ISO 8601
	// has no mixed form. The zone suffix is not part of the body, so its
	// style may differ from the body's. The fixtures carry an extended body
	// with a compact zone (10:30:00+0530); the other mix, a compact body with
	// an extended zone (103000+05:30), is accepted alongside it.
	dateExtended := strings.Contains(datePart, "-")
	if (dateExtended && tp.basic) || (!dateExtended && tp.colon) {
		return dp, timeParts{}, fmt.Errorf("mixed extended and compact forms in %q", value)
	}
	return dp, tp, nil
}

// Year returns the year component.
func (d *DVDateTime) Year() int { dp, _, _ := d.split(); return dp.year }

// Month returns the month component, or 0 when unknown.
func (d *DVDateTime) Month() int { dp, _, _ := d.split(); return dp.month }

// Day returns the day component, or 0 when unknown.
func (d *DVDateTime) Day() int { dp, _, _ := d.split(); return dp.day }

// Hour returns the hour component.
func (d *DVDateTime) Hour() int { _, tp, _ := d.split(); return tp.hour }

// Minute returns the minute component.
func (d *DVDateTime) Minute() int { _, tp, _ := d.split(); return tp.minute }

// Second returns the second component.
func (d *DVDateTime) Second() int { _, tp, _ := d.split(); return tp.second }

// FractionalSecond returns the fractional-second component.
func (d *DVDateTime) FractionalSecond() float64 { _, tp, _ := d.split(); return tp.frac }

// Timezone returns the timezone designator, or "" when none.
func (d *DVDateTime) Timezone() string { _, tp, _ := d.split(); return tp.tz }

// MonthUnknown reports whether the date side omits the month. It is false
// when the value does not parse.
func (d *DVDateTime) MonthUnknown() bool {
	dp, _, err := d.split()
	return err == nil && !dp.monthKnown
}

// DayUnknown reports whether the date side omits the day. It is false when
// the value does not parse.
func (d *DVDateTime) DayUnknown() bool {
	dp, _, err := d.split()
	return err == nil && !dp.dayKnown
}

// IsPartial reports whether the date-time is reduced (second or more
// missing, including a missing time part entirely). It is false when the
// value does not parse.
func (d *DVDateTime) IsPartial() bool {
	dp, tp, err := d.split()
	return err == nil && (!dp.dayKnown || !tp.secondKnown)
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
	return float64(dateMagnitudeDays(dp))*secondsPerDay + float64(tp.hour*3600+tp.minute*60+tp.second) + tp.frac
}

// Compare orders this date-time against other by magnitude.
func (d *DVDateTime) Compare(other DVDateTime) int { return cmpFloat(d.Magnitude(), other.Magnitude()) }

// LessThan reports whether this date-time precedes other.
func (d *DVDateTime) LessThan(other DVDateTime) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two date-times.
func (d *DVDateTime) IsStrictlyComparableTo(other DVDateTime) bool { return true }

// ToTime converts a full date-time to a time.Time (UTC when no timezone
// is present), or returns ErrTemporalConversion when the receiver is nil
// or the value is partial or malformed.
func (d *DVDateTime) ToTime() (time.Time, error) {
	dp, tp, err := d.split()
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date-time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if !dp.dayKnown || !tp.secondKnown {
		return time.Time{}, fmt.Errorf("%w: date-time %q is partial", ErrTemporalConversion, d.isoValue())
	}
	loc, err := tzLocation(tp)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date-time %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	return time.Date(dp.year, time.Month(dp.month), dp.day, tp.hour, tp.minute, tp.second, int(tp.frac*1e9), loc), nil
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
func (d *DVDuration) Years() int { p, _ := parseDuration(d.isoValue()); return p.years }

// Months returns the months component.
func (d *DVDuration) Months() int { p, _ := parseDuration(d.isoValue()); return p.months }

// Weeks returns the weeks component.
func (d *DVDuration) Weeks() int { p, _ := parseDuration(d.isoValue()); return p.weeks }

// Days returns the days component.
func (d *DVDuration) Days() int { p, _ := parseDuration(d.isoValue()); return p.days }

// Hours returns the hours component.
func (d *DVDuration) Hours() int { p, _ := parseDuration(d.isoValue()); return p.hours }

// Minutes returns the minutes component.
func (d *DVDuration) Minutes() int { p, _ := parseDuration(d.isoValue()); return p.minutes }

// Seconds returns the whole-seconds component.
func (d *DVDuration) Seconds() int { p, _ := parseDuration(d.isoValue()); return p.seconds }

// FractionalSeconds returns the fractional-second component.
func (d *DVDuration) FractionalSeconds() float64 { p, _ := parseDuration(d.isoValue()); return p.frac }

// IsNegative reports whether the duration carries a leading minus sign
// (openEHR deviation from ISO 8601). It is false when the value does not
// parse.
func (d *DVDuration) IsNegative() bool { p, _ := parseDuration(d.isoValue()); return p.neg }

// Magnitude returns the duration as a number of seconds, using the
// openEHR nominal year (365.24 d) and month (30.42 d) averages for the
// calendar-nominal components. Negative when the duration is negative.
// Components are scaled in floating point, so a long duration keeps its
// length in seconds within float64 precision.
func (d *DVDuration) Magnitude() float64 {
	p, err := parseDuration(d.isoValue())
	if err != nil {
		return 0
	}
	secs := float64(p.years)*nominalDaysInYear*secondsPerDay +
		float64(p.months)*nominalDaysInMonth*secondsPerDay +
		definiteSeconds(p)
	if p.neg {
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
// as definite (7 d, 24 h).
func (d *DVDuration) ToDuration() (time.Duration, error) {
	p, err := parseDuration(d.isoValue())
	if err != nil {
		return 0, fmt.Errorf("%w: duration %q: %w", ErrTemporalConversion, d.isoValue(), err)
	}
	if p.years != 0 || p.months != 0 {
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
func definiteSeconds(p durationParts) float64 {
	return float64(p.weeks)*7*secondsPerDay +
		float64(p.days)*secondsPerDay +
		float64(p.hours)*3600 +
		float64(p.minutes)*60 +
		float64(p.seconds) + p.frac
}

// definiteNanos is the week, day and clock length in nanoseconds, with the
// sign applied. Whole seconds are counted in integers, so a length one
// nanosecond outside the int64 range is not rounded onto either extreme.
// The bool is false when that length does not fit.
func definiteNanos(p durationParts) (int64, bool) {
	mag, ok := unsignedNanos(p)
	if !ok {
		return 0, false
	}
	if p.neg {
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

// unsignedNanos is the absolute length in nanoseconds. The fractional
// second is below one second, so only that part is a float.
func unsignedNanos(p durationParts) (uint64, bool) {
	days, ok := mulU(uint64(p.weeks), 7)
	if !ok {
		return 0, false
	}
	days, ok = addU(days, uint64(p.days))
	if !ok {
		return 0, false
	}
	secs, ok := mulU(days, 86400)
	if !ok {
		return 0, false
	}
	hours, ok := mulU(uint64(p.hours), 3600)
	if !ok {
		return 0, false
	}
	mins, ok := mulU(uint64(p.minutes), 60)
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
	secs, ok = addU(secs, uint64(p.seconds))
	if !ok {
		return 0, false
	}
	ns, ok := mulU(secs, 1_000_000_000)
	if !ok {
		return 0, false
	}
	frac := int64(math.Round(p.frac * 1e9))
	if frac < 0 || frac >= 1_000_000_000 {
		return 0, false
	}
	return addU(ns, uint64(frac))
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

// --- parsing ------------------------------------------------------------

// parseDate parses the BASE valid_iso8601_date forms: YYYY-MM-DD, YYYY-MM,
// YYYY, YYYYMMDD, YYYYMM, with the month and day checked against the
// Gregorian calendar. Every field is zero-filled ASCII digits. On an error
// the parts are zero.
func parseDate(s string) (dateParts, error) {
	p, err := scanDate(s)
	if err != nil {
		return dateParts{}, err
	}
	return p, nil
}

// scanDate does the work of parseDate. On an error the parts it returns may
// be partly filled.
func scanDate(s string) (dateParts, error) {
	var p dateParts
	if s == "" {
		return p, errors.New("empty date")
	}
	var fields []string
	if strings.Contains(s, "-") {
		fields = strings.Split(s, "-")
	} else { // compact form YYYY[MM[DD]]
		switch len(s) {
		case 4:
			fields = []string{s}
		case 6:
			fields = []string{s[:4], s[4:6]}
		case 8:
			fields = []string{s[:4], s[4:6], s[6:8]}
		default:
			return p, fmt.Errorf("bad date %q", s)
		}
	}
	if len(fields) > 3 {
		return p, fmt.Errorf("bad date %q", s)
	}
	widths := [3]int{4, 2, 2}
	var nums [3]int
	for i, f := range fields {
		n, ok := fixedDigits(f, widths[i])
		if !ok {
			return p, fmt.Errorf("bad date %q", s)
		}
		nums[i] = n
	}
	p.year = nums[0]
	if len(fields) >= 2 {
		if nums[1] < 1 || nums[1] > 12 {
			return p, fmt.Errorf("bad month in %q", s)
		}
		p.month, p.monthKnown = nums[1], true
	}
	if len(fields) == 3 {
		if nums[2] < 1 || nums[2] > daysInMonth(p.year, p.month) {
			return p, fmt.Errorf("bad day in %q", s)
		}
		p.day, p.dayKnown = nums[2], true
	}
	return p, nil
}

// parseTime parses the BASE valid_iso8601_time forms: hh:mm:ss, hh:mm
// (extended), hhmmss, hhmm, hh (compact), each with an optional comma or dot
// fraction on the seconds and an optional zone of Z, ±hh, ±hhmm or ±hh:mm.
// On an error the parts are zero.
func parseTime(s string) (timeParts, error) {
	p, err := scanTime(s)
	if err != nil {
		return timeParts{}, err
	}
	return p, nil
}

// scanTime does the work of parseTime. On an error the parts it returns may
// be partly filled.
func scanTime(s string) (timeParts, error) {
	var p timeParts
	body := s
	if i := strings.IndexAny(s, "Z+-"); i >= 0 {
		body, p.tz, p.tzKnown = s[:i], s[i:], true
		if _, err := parseZone(p.tz); err != nil {
			return p, err
		}
	}
	if body == "" {
		return p, errors.New("empty time")
	}
	main, fracDigits, hasFrac := body, "", false
	if i := strings.IndexAny(body, ".,"); i >= 0 {
		main, fracDigits, hasFrac = body[:i], body[i+1:], true
		if !allDigits(fracDigits) {
			return p, fmt.Errorf("bad fractional second in %q", s)
		}
	}
	var fields []string
	if strings.Contains(main, ":") {
		p.colon = true
		fields = strings.Split(main, ":")
	} else { // compact form hh[mm[ss]]
		switch len(main) {
		case 2:
			fields = []string{main}
		case 4:
			fields = []string{main[:2], main[2:4]}
			p.basic = true
		case 6:
			fields = []string{main[:2], main[2:4], main[4:6]}
			p.basic = true
		default:
			return p, fmt.Errorf("bad time %q", s)
		}
	}
	if len(fields) > 3 || (hasFrac && len(fields) != 3) {
		return p, fmt.Errorf("bad time %q", s)
	}
	var nums [3]int
	for i, f := range fields {
		n, ok := fixedDigits(f, 2)
		if !ok {
			return p, fmt.Errorf("bad time %q", s)
		}
		nums[i] = n
	}
	// BASE: hh is 00 to 23; "24:00:00" is not allowed, since it would mean
	// the date was really on the next day.
	if nums[0] > 23 {
		return p, fmt.Errorf("bad hour in %q", s)
	}
	p.hour = nums[0]
	if len(fields) >= 2 {
		if nums[1] > 59 {
			return p, fmt.Errorf("bad minute in %q", s)
		}
		p.minute, p.minuteKnown = nums[1], true
	}
	if len(fields) == 3 {
		if nums[2] > 60 {
			return p, fmt.Errorf("bad second in %q", s)
		}
		p.second, p.secondKnown = nums[2], true
	}
	if hasFrac {
		f, err := strconv.ParseFloat("0."+fracDigits, 64)
		if err != nil {
			return p, fmt.Errorf("bad fractional second in %q", s)
		}
		p.frac = f
	}
	return p, nil
}

// parseZone parses a timezone designator (Z, ±hh, ±hhmm or ±hh:mm) and
// returns its offset in seconds east of UTC. The hour is at most 14 east and
// 12 west (TIME_DEFINITIONS Max_timezone_hour / Min_timezone_hour).
func parseZone(tz string) (int, error) {
	if tz == "Z" {
		return 0, nil
	}
	if tz == "" {
		return 0, errors.New("empty timezone")
	}
	sign, maxHour := 1, 14
	switch tz[0] {
	case '+':
	case '-':
		sign, maxHour = -1, 12
	default:
		return 0, fmt.Errorf("bad timezone %q", tz)
	}
	rest := tz[1:]
	hh, mm := "", "00"
	switch len(rest) {
	case 2:
		hh = rest
	case 4:
		hh, mm = rest[:2], rest[2:]
	case 5:
		if rest[2] != ':' {
			return 0, fmt.Errorf("bad timezone %q", tz)
		}
		hh, mm = rest[:2], rest[3:]
	default:
		return 0, fmt.Errorf("bad timezone %q", tz)
	}
	h, okH := fixedDigits(hh, 2)
	m, okM := fixedDigits(mm, 2)
	if !okH || !okM || h > maxHour || m > 59 {
		return 0, fmt.Errorf("bad timezone %q", tz)
	}
	return sign * (h*3600 + m*60), nil
}

// fixedDigits parses s as exactly width ASCII digits.
func fixedDigits(s string, width int) (int, bool) {
	if len(s) != width || !allDigits(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// allDigits reports whether s is a non-empty run of ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// daysInMonth returns the Gregorian length of month m (1-12) in year y.
func daysInMonth(y, m int) int {
	switch m {
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	default:
		return 31
	}
}

// parseDuration parses the BASE ISO8601_DURATION form
// P[nnY][nnM][nnW][nnD][T[nnH][nnM][nnS]], plus the two documented openEHR
// deviations: a leading negative sign, and the W designator mixed with the
// others (in the order Y M W D). Designators appear at most once, in that
// order; a T section needs at least one component; a fraction (dot or comma)
// is allowed on the seconds only. On an error the parts are zero.
func parseDuration(s string) (durationParts, error) {
	p, err := scanDuration(s)
	if err != nil {
		return durationParts{}, err
	}
	return p, nil
}

// scanDuration does the work of parseDuration. On an error the parts it
// returns may be partly filled.
func scanDuration(s string) (durationParts, error) {
	var p durationParts
	orig := s
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		p.neg = true
		s = rest
	}
	rest, ok := strings.CutPrefix(s, "P")
	if !ok {
		return p, fmt.Errorf("bad duration %q (no 'P')", orig)
	}
	s = rest
	inTime, timeComponents := false, 0
	// rank is the position of the last designator seen in the current
	// section; a designator must rank strictly higher than the one before.
	rank := 0
	start := 0 // start of the number awaiting its designator
	sawComponent := false
	// Byte indices, not runes: designators are ASCII and every byte must
	// be inspected, so `range s` (which skips continuation bytes) is wrong.
	for i := range len(s) {
		c := s[i]
		switch {
		case c == 'T':
			if inTime || start != i {
				return p, fmt.Errorf("misplaced 'T' in duration %q", orig)
			}
			inTime, rank = true, 0
			start = i + 1
			continue
		case (c >= '0' && c <= '9') || c == '.' || c == ',':
			continue
		}
		whole, frac, hasFrac, err := splitNumber(s[start:i])
		if err != nil {
			return p, fmt.Errorf("bad duration component in %q", orig)
		}
		start = i + 1
		// openEHR's ISO8601_DURATION carries a fraction only on the
		// seconds component (fractional_second); a fraction on any other
		// component is malformed — reject it rather than silently
		// truncate to the integer part.
		if hasFrac && c != 'S' {
			return p, fmt.Errorf("fractional %q component not permitted in %q", string(c), orig)
		}
		var r int
		switch {
		case !inTime && c == 'Y':
			r, p.years = 1, whole
		case !inTime && c == 'M':
			r, p.months = 2, whole
		case !inTime && c == 'W':
			r, p.weeks = 3, whole
		case !inTime && c == 'D':
			r, p.days = 4, whole
		case inTime && c == 'H':
			r, p.hours = 1, whole
		case inTime && c == 'M':
			r, p.minutes = 2, whole
		case inTime && c == 'S':
			r, p.seconds, p.frac = 3, whole, frac
		default:
			return p, fmt.Errorf("bad duration designator %q in %q", string(c), orig)
		}
		if r <= rank {
			return p, fmt.Errorf("duration designator %q out of order or repeated in %q", string(c), orig)
		}
		rank = r
		sawComponent = true
		if inTime {
			timeComponents++
		}
	}
	if start != len(s) {
		return p, fmt.Errorf("dangling number in duration %q", orig)
	}
	if inTime && timeComponents == 0 {
		return p, fmt.Errorf("empty time section in duration %q", orig)
	}
	if !sawComponent {
		return p, fmt.Errorf("empty duration %q", orig)
	}
	return p, nil
}

// splitNumber parses "12" → (12, 0, false), "12.5" and "12,5" → (12, 0.5,
// true). Both parts must be plain ASCII digits and a separator needs at least
// one digit after it, so "1.", ".5" and "+1" fail; hasFrac reports that a
// separator was written even when the fraction is zero ("1.0").
func splitNumber(s string) (whole int, frac float64, hasFrac bool, err error) {
	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if !hasFrac {
		intPart, fracPart, hasFrac = strings.Cut(s, ",")
	}
	if !allDigits(intPart) || (hasFrac && !allDigits(fracPart)) {
		return 0, 0, false, errors.New("malformed number")
	}
	whole, err = strconv.Atoi(intPart)
	if err != nil {
		return 0, 0, false, err
	}
	if hasFrac {
		frac, err = strconv.ParseFloat("0."+fracPart, 64)
		if err != nil {
			return 0, 0, false, err
		}
	}
	return whole, frac, hasFrac, nil
}

// dateMagnitudeDays returns days since 0001-01-01, treating an unknown
// month or day as 1.
func dateMagnitudeDays(p dateParts) int {
	m, dd := p.month, p.day
	if !p.monthKnown {
		m = 1
	}
	if !p.dayKnown {
		dd = 1
	}
	return daysFromCivil(p.year, m, dd) - daysFromCivil(1, 1, 1)
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
func tzLocation(p timeParts) (*time.Location, error) {
	if !p.tzKnown || p.tz == "" || p.tz == "Z" {
		return time.UTC, nil
	}
	off, err := parseZone(p.tz)
	if err != nil {
		return nil, err
	}
	return time.FixedZone(p.tz, off), nil
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
