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
// No method panics: a malformed `value` yields zero components and a
// zero magnitude; the fallible Go-bridge conversions return an error
// (also for partial / calendar-nominal values that cannot map cleanly).
// See docs/specifications/rm-functions.md § REQ-123 and ADR 0011.
//
// Temporal arithmetic (add / subtract / diff / multiply / negative /
// add_nominal) is out of scope and remains fail-loud generated stubs.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrTemporalConversion is returned (wrapped) by ToTime / ToDuration
// when a value cannot map cleanly to a Go time.Time / time.Duration —
// it is malformed, partial, or carries calendar-nominal components.
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
}

type durationParts struct {
	neg                                                 bool
	years, months, weeks, days, hours, minutes, seconds int
	frac                                                float64
}

// --- DV_DATE ------------------------------------------------------------

// Year returns the year component (0 when unparseable). REQ-123.
func (d *DVDate) Year() int { p, _ := parseDate(d.Value); return p.year }

// Month returns the month component, or 0 when month-unknown. REQ-123.
func (d *DVDate) Month() int { p, _ := parseDate(d.Value); return p.month }

// Day returns the day component, or 0 when day-unknown. REQ-123.
func (d *DVDate) Day() int { p, _ := parseDate(d.Value); return p.day }

// MonthUnknown reports whether the date omits the month (e.g. "2024").
// REQ-123.
func (d *DVDate) MonthUnknown() bool { p, _ := parseDate(d.Value); return !p.monthKnown }

// DayUnknown reports whether the date omits the day (e.g. "2024-03").
// REQ-123.
func (d *DVDate) DayUnknown() bool { p, _ := parseDate(d.Value); return !p.dayKnown }

// IsPartial reports whether the date is reduced (day or more missing).
// REQ-123.
func (d *DVDate) IsPartial() bool { return d.DayUnknown() }

// Magnitude returns the number of days since the calendar origin
// 0001-01-01 (unknown month/day count as 1). REQ-123.
func (d *DVDate) Magnitude() Integer {
	p, _ := parseDate(d.Value)
	return Integer(dateMagnitudeDays(p))
}

// Compare orders this date against other by magnitude (-1 / 0 / +1).
// REQ-123.
func (d *DVDate) Compare(other DVDate) int { return cmpInt(int(d.Magnitude()), int(other.Magnitude())) }

// LessThan reports whether this date precedes other (by magnitude).
// REQ-123.
func (d *DVDate) LessThan(other DVDate) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two dates. REQ-123.
func (d *DVDate) IsStrictlyComparableTo(other DVDate) bool { return true }

// ToTime converts a full date to a time.Time at midnight UTC, or returns
// ErrTemporalConversion for a partial/malformed value. REQ-123.
func (d *DVDate) ToTime() (time.Time, error) {
	p, err := parseDate(d.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date %q: %w", ErrTemporalConversion, d.Value, err)
	}
	if !p.dayKnown {
		return time.Time{}, fmt.Errorf("%w: date %q is partial", ErrTemporalConversion, d.Value)
	}
	return time.Date(p.year, time.Month(p.month), p.day, 0, 0, 0, 0, time.UTC), nil
}

// --- DV_TIME ------------------------------------------------------------

// Hour returns the hour component (0 when unparseable). REQ-123.
func (d *DVTime) Hour() int { p, _ := parseTime(d.Value); return p.hour }

// Minute returns the minute component, or 0 when minute-unknown. REQ-123.
func (d *DVTime) Minute() int { p, _ := parseTime(d.Value); return p.minute }

// Second returns the second component, or 0 when second-unknown. REQ-123.
func (d *DVTime) Second() int { p, _ := parseTime(d.Value); return p.second }

// FractionalSecond returns the fractional-second component (0 when
// absent). REQ-123.
func (d *DVTime) FractionalSecond() float64 { p, _ := parseTime(d.Value); return p.frac }

// Timezone returns the timezone designator (e.g. "Z", "+02:00"), or ""
// when none is present. REQ-123.
func (d *DVTime) Timezone() string { p, _ := parseTime(d.Value); return p.tz }

// IsPartial reports whether the time is reduced (second or more missing).
// REQ-123.
func (d *DVTime) IsPartial() bool { p, _ := parseTime(d.Value); return !p.secondKnown }

// Magnitude returns the number of seconds since the start of day. REQ-123.
func (d *DVTime) Magnitude() Real {
	p, _ := parseTime(d.Value)
	return Real(float64(p.hour*3600+p.minute*60+p.second) + p.frac)
}

// Compare orders this time against other by magnitude (-1 / 0 / +1).
// REQ-123.
func (d *DVTime) Compare(other DVTime) int {
	return cmpFloat(float64(d.Magnitude()), float64(other.Magnitude()))
}

// LessThan reports whether this time precedes other (by magnitude).
// REQ-123.
func (d *DVTime) LessThan(other DVTime) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two times. REQ-123.
func (d *DVTime) IsStrictlyComparableTo(other DVTime) bool { return true }

// ToTime converts a full time-of-day to a time.Time on the reference
// date 0000-01-01, or returns ErrTemporalConversion for a
// partial/malformed value. REQ-123.
func (d *DVTime) ToTime() (time.Time, error) {
	p, err := parseTime(d.Value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %q: %w", ErrTemporalConversion, d.Value, err)
	}
	if !p.secondKnown {
		return time.Time{}, fmt.Errorf("%w: time %q is partial", ErrTemporalConversion, d.Value)
	}
	loc, err := tzLocation(p)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %q: %w", ErrTemporalConversion, d.Value, err)
	}
	return time.Date(0, 1, 1, p.hour, p.minute, p.second, int(p.frac*1e9), loc), nil
}

// --- DV_DATE_TIME -------------------------------------------------------

func (d *DVDateTime) split() (dateParts, timeParts, error) {
	datePart, timePart, hasT := strings.Cut(d.Value, "T")
	dp, derr := parseDate(datePart)
	if derr != nil {
		return dp, timeParts{}, derr
	}
	if !hasT {
		return dp, timeParts{}, nil
	}
	tp, terr := parseTime(timePart)
	return dp, tp, terr
}

// Year returns the year component. REQ-123.
func (d *DVDateTime) Year() int { dp, _, _ := d.split(); return dp.year }

// Month returns the month component, or 0 when unknown. REQ-123.
func (d *DVDateTime) Month() int { dp, _, _ := d.split(); return dp.month }

// Day returns the day component, or 0 when unknown. REQ-123.
func (d *DVDateTime) Day() int { dp, _, _ := d.split(); return dp.day }

// Hour returns the hour component. REQ-123.
func (d *DVDateTime) Hour() int { _, tp, _ := d.split(); return tp.hour }

// Minute returns the minute component. REQ-123.
func (d *DVDateTime) Minute() int { _, tp, _ := d.split(); return tp.minute }

// Second returns the second component. REQ-123.
func (d *DVDateTime) Second() int { _, tp, _ := d.split(); return tp.second }

// FractionalSecond returns the fractional-second component. REQ-123.
func (d *DVDateTime) FractionalSecond() float64 { _, tp, _ := d.split(); return tp.frac }

// Timezone returns the timezone designator, or "" when none. REQ-123.
func (d *DVDateTime) Timezone() string { _, tp, _ := d.split(); return tp.tz }

// MonthUnknown reports whether the date side omits the month. REQ-123.
func (d *DVDateTime) MonthUnknown() bool { dp, _, _ := d.split(); return !dp.monthKnown }

// DayUnknown reports whether the date side omits the day. REQ-123.
func (d *DVDateTime) DayUnknown() bool { dp, _, _ := d.split(); return !dp.dayKnown }

// IsPartial reports whether the date-time is reduced (second or more
// missing — including a missing time part entirely). REQ-123.
func (d *DVDateTime) IsPartial() bool {
	dp, tp, _ := d.split()
	return !dp.dayKnown || !tp.secondKnown
}

// Magnitude returns the number of seconds since the calendar origin
// 0001-01-01T00:00:00. REQ-123.
func (d *DVDateTime) Magnitude() float64 {
	dp, tp, _ := d.split()
	return float64(dateMagnitudeDays(dp))*secondsPerDay + float64(tp.hour*3600+tp.minute*60+tp.second) + tp.frac
}

// Compare orders this date-time against other by magnitude. REQ-123.
func (d *DVDateTime) Compare(other DVDateTime) int { return cmpFloat(d.Magnitude(), other.Magnitude()) }

// LessThan reports whether this date-time precedes other. REQ-123.
func (d *DVDateTime) LessThan(other DVDateTime) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two date-times. REQ-123.
func (d *DVDateTime) IsStrictlyComparableTo(other DVDateTime) bool { return true }

// ToTime converts a full date-time to a time.Time (UTC when no timezone
// is present), or returns ErrTemporalConversion for a partial/malformed
// value. REQ-123.
func (d *DVDateTime) ToTime() (time.Time, error) {
	dp, tp, err := d.split()
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date-time %q: %w", ErrTemporalConversion, d.Value, err)
	}
	if !dp.dayKnown || !tp.secondKnown {
		return time.Time{}, fmt.Errorf("%w: date-time %q is partial", ErrTemporalConversion, d.Value)
	}
	loc, err := tzLocation(tp)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date-time %q: %w", ErrTemporalConversion, d.Value, err)
	}
	return time.Date(dp.year, time.Month(dp.month), dp.day, tp.hour, tp.minute, tp.second, int(tp.frac*1e9), loc), nil
}

// --- DV_DURATION --------------------------------------------------------

// Years returns the years component. REQ-123.
func (d *DVDuration) Years() int { p, _ := parseDuration(d.Value); return p.years }

// Months returns the months component. REQ-123.
func (d *DVDuration) Months() int { p, _ := parseDuration(d.Value); return p.months }

// Weeks returns the weeks component. REQ-123.
func (d *DVDuration) Weeks() int { p, _ := parseDuration(d.Value); return p.weeks }

// Days returns the days component. REQ-123.
func (d *DVDuration) Days() int { p, _ := parseDuration(d.Value); return p.days }

// Hours returns the hours component. REQ-123.
func (d *DVDuration) Hours() int { p, _ := parseDuration(d.Value); return p.hours }

// Minutes returns the minutes component. REQ-123.
func (d *DVDuration) Minutes() int { p, _ := parseDuration(d.Value); return p.minutes }

// Seconds returns the whole-seconds component. REQ-123.
func (d *DVDuration) Seconds() int { p, _ := parseDuration(d.Value); return p.seconds }

// FractionalSeconds returns the fractional-second component. REQ-123.
func (d *DVDuration) FractionalSeconds() float64 { p, _ := parseDuration(d.Value); return p.frac }

// IsNegative reports whether the duration carries a leading minus sign
// (openEHR deviation from ISO 8601). REQ-123.
func (d *DVDuration) IsNegative() bool { p, _ := parseDuration(d.Value); return p.neg }

// Magnitude returns the duration as a number of seconds, using the
// openEHR nominal year (365.24 d) and month (30.42 d) averages for the
// calendar-nominal components. Negative when the duration is negative.
// REQ-123.
func (d *DVDuration) Magnitude() float64 {
	p, _ := parseDuration(d.Value)
	secs := float64(p.years)*nominalDaysInYear*secondsPerDay +
		float64(p.months)*nominalDaysInMonth*secondsPerDay +
		float64(p.weeks)*7*secondsPerDay +
		float64(p.days)*secondsPerDay +
		float64(p.hours*3600+p.minutes*60+p.seconds) + p.frac
	if p.neg {
		return -secs
	}
	return secs
}

// Compare orders this duration against other by magnitude. REQ-123.
func (d *DVDuration) Compare(other DVDuration) int { return cmpFloat(d.Magnitude(), other.Magnitude()) }

// LessThan reports whether this duration is shorter than other. REQ-123.
func (d *DVDuration) LessThan(other DVDuration) bool { return d.Compare(other) < 0 }

// IsStrictlyComparableTo is true for any two durations. REQ-123.
func (d *DVDuration) IsStrictlyComparableTo(other DVDuration) bool { return true }

// ToDuration converts a definite duration to a time.Duration, or returns
// ErrTemporalConversion when it is malformed or carries calendar-nominal
// years/months (which have no fixed length). Weeks and days are treated
// as definite (7 d, 24 h). REQ-123.
func (d *DVDuration) ToDuration() (time.Duration, error) {
	p, err := parseDuration(d.Value)
	if err != nil {
		return 0, fmt.Errorf("%w: duration %q: %w", ErrTemporalConversion, d.Value, err)
	}
	if p.years != 0 || p.months != 0 {
		return 0, fmt.Errorf("%w: duration %q has calendar-nominal Y/M components", ErrTemporalConversion, d.Value)
	}
	secs := float64(p.weeks)*7*secondsPerDay + float64(p.days)*secondsPerDay +
		float64(p.hours*3600+p.minutes*60+p.seconds) + p.frac
	if p.neg {
		secs = -secs
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// --- parsing ------------------------------------------------------------

func parseDate(s string) (dateParts, error) {
	var p dateParts
	if s == "" {
		return p, errors.New("empty date")
	}
	var fields []string
	if strings.Contains(s, "-") {
		fields = strings.Split(s, "-")
	} else { // basic form YYYY[MM[DD]]
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
	if len(fields) == 0 || len(fields) > 3 {
		return p, fmt.Errorf("bad date %q", s)
	}
	y, err := strconv.Atoi(fields[0])
	if err != nil {
		return p, fmt.Errorf("bad year in %q", s)
	}
	p.year = y
	if len(fields) >= 2 {
		m, err := strconv.Atoi(fields[1])
		if err != nil || m < 1 || m > 12 {
			return p, fmt.Errorf("bad month in %q", s)
		}
		p.month, p.monthKnown = m, true
	}
	if len(fields) == 3 {
		dd, err := strconv.Atoi(fields[2])
		if err != nil || dd < 1 || dd > 31 {
			return p, fmt.Errorf("bad day in %q", s)
		}
		p.day, p.dayKnown = dd, true
	}
	return p, nil
}

func parseTime(s string) (timeParts, error) {
	var p timeParts
	if s == "" {
		return p, errors.New("empty time")
	}
	// Strip timezone suffix.
	switch {
	case strings.HasSuffix(s, "Z"):
		p.tz, p.tzKnown = "Z", true
		s = s[:len(s)-1]
	default:
		if i := strings.IndexByte(s, '+'); i >= 0 {
			p.tz, p.tzKnown = s[i:], true
			s = s[:i]
		} else if i := strings.LastIndexByte(s, '-'); i > 0 {
			p.tz, p.tzKnown = s[i:], true
			s = s[:i]
		}
	}
	fields := strings.Split(s, ":")
	if len(fields) == 0 || len(fields) > 3 {
		return p, fmt.Errorf("bad time %q", s)
	}
	h, err := strconv.Atoi(fields[0])
	if err != nil || h < 0 || h > 23 {
		return p, fmt.Errorf("bad hour in %q", s)
	}
	p.hour = h
	if len(fields) >= 2 {
		m, err := strconv.Atoi(fields[1])
		if err != nil || m < 0 || m > 59 {
			return p, fmt.Errorf("bad minute in %q", s)
		}
		p.minute, p.minuteKnown = m, true
	}
	if len(fields) == 3 {
		secField := fields[2]
		whole, frac, err := splitNumber(secField)
		if err != nil || whole < 0 || whole > 60 {
			return p, fmt.Errorf("bad second in %q", s)
		}
		p.second, p.frac, p.secondKnown = whole, frac, true
	}
	return p, nil
}

func parseDuration(s string) (durationParts, error) {
	var p durationParts
	if strings.HasPrefix(s, "-") {
		p.neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if !strings.HasPrefix(s, "P") {
		return p, fmt.Errorf("bad duration %q (no 'P')", s)
	}
	s = s[1:]
	inTime := false
	num := ""
	any := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 'T':
			inTime = true
			continue
		case (c >= '0' && c <= '9') || c == '.':
			num += string(c)
			continue
		}
		whole, frac, err := splitNumber(num)
		if err != nil {
			return p, fmt.Errorf("bad duration component in %q", s)
		}
		num = ""
		switch c {
		case 'Y':
			p.years = whole
		case 'W':
			p.weeks = whole
		case 'D':
			p.days = whole
		case 'H':
			p.hours = whole
		case 'S':
			p.seconds, p.frac = whole, frac
		case 'M':
			if inTime {
				p.minutes = whole
			} else {
				p.months = whole
			}
		default:
			return p, fmt.Errorf("bad duration designator %q in %q", string(c), s)
		}
		any = true
	}
	if num != "" {
		return p, fmt.Errorf("dangling number in duration %q", s)
	}
	if !any {
		return p, fmt.Errorf("empty duration %q", s)
	}
	return p, nil
}

// splitNumber parses "12" → (12, 0) and "12.5" → (12, 0.5).
func splitNumber(s string) (whole int, frac float64, err error) {
	if s == "" {
		return 0, 0, errors.New("empty number")
	}
	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	whole, err = strconv.Atoi(intPart)
	if err != nil {
		return 0, 0, err
	}
	if hasFrac {
		f, ferr := strconv.ParseFloat("0."+fracPart, 64)
		if ferr != nil {
			return 0, 0, ferr
		}
		frac = f
	}
	return whole, frac, nil
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
	sign := 1
	tz := p.tz
	switch tz[0] {
	case '+':
		tz = tz[1:]
	case '-':
		sign = -1
		tz = tz[1:]
	}
	tz = strings.ReplaceAll(tz, ":", "")
	if len(tz) != 4 {
		return nil, fmt.Errorf("bad timezone %q", p.tz)
	}
	hh, err1 := strconv.Atoi(tz[:2])
	mm, err2 := strconv.Atoi(tz[2:])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("bad timezone %q", p.tz)
	}
	return time.FixedZone(p.tz, sign*(hh*3600+mm*60)), nil
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
