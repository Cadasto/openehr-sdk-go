// Package iso8601 parses the ISO 8601 date, time, date-time and duration text
// that openEHR's temporal data values carry, in the forms the openEHR BASE
// predicates valid_iso8601_date, valid_iso8601_time, valid_iso8601_date_time
// and valid_iso8601_duration define.
//
// It is the one parser behind the RM's temporal helpers (openehr/rm) and the
// template layer's temporal validators (openehr/template/constraints), so the
// two accept exactly the same text. The template layer may not import the RM
// package, which is why the parser lives here.
//
// Every function works on the string in memory: none reads a clock, a file or
// the time zone database, and none panics on malformed input.
package iso8601

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Date is a parsed calendar date. A reduced date leaves out the day, or the
// month and the day; the field of a part left out is zero and its Known flag
// false.
type Date struct {
	Year, Month, Day     int
	MonthKnown, DayKnown bool
}

// Time is a parsed time of day. A reduced time leaves out the seconds, or the
// minutes and the seconds; the field of a part left out is zero and its Known
// flag false. Zone holds the zone designator as written ("Z", "+01",
// "+0100", "+01:00"), or "" when there is none.
type Time struct {
	Hour, Minute, Second                int
	Fraction                            float64
	MinuteKnown, SecondKnown, ZoneKnown bool
	Zone                                string
	// colon marks the extended form (hh:mm[:ss]); basic marks a compact body
	// longer than hh (hhmm[ss]). A bare hh is neither, so it fits both a
	// compact and an extended date-time.
	colon, basic bool
}

// Duration is a parsed duration. Fraction is the fractional part of the
// seconds, the only component that may carry one. Negative reports openEHR's
// leading minus sign.
type Duration struct {
	Negative                                            bool
	Years, Months, Weeks, Days, Hours, Minutes, Seconds int
	Fraction                                            float64
}

// ValidDate reports whether s is a date [ParseDate] accepts.
func ValidDate(s string) bool {
	_, err := scanDate(s)
	return err == nil
}

// ValidTime reports whether s is a time [ParseTime] accepts.
func ValidTime(s string) bool {
	_, err := scanTime(s)
	return err == nil
}

// ValidDateTime reports whether s is a date-time [ParseDateTime] accepts.
func ValidDateTime(s string) bool {
	_, _, err := scanDateTime(s)
	return err == nil
}

// ValidDuration reports whether s is a duration [ParseDuration] accepts.
func ValidDuration(s string) bool {
	_, err := scanDuration(s)
	return err == nil
}

// ParseDate parses the BASE valid_iso8601_date forms: YYYY-MM-DD, YYYY-MM,
// YYYY, YYYYMMDD, YYYYMM, with the month and day checked against the
// Gregorian calendar. Every field is zero-filled ASCII digits. On an error
// the parts are zero.
func ParseDate(s string) (Date, error) {
	p, err := scanDate(s)
	if err != nil {
		return Date{}, err
	}
	return p, nil
}

// scanDate does the work of ParseDate. On an error the parts it returns may
// be partly filled.
func scanDate(s string) (Date, error) {
	var p Date
	if s == "" {
		return p, errors.New("empty date")
	}
	var fields [3]string
	var n int
	if strings.Contains(s, "-") {
		var ok bool
		if fields, n, ok = cutFields(s, "-"); !ok {
			return p, fmt.Errorf("bad date %q", s)
		}
	} else { // compact form YYYY[MM[DD]]
		switch len(s) {
		case 4:
			fields[0], n = s, 1
		case 6:
			fields[0], fields[1], n = s[:4], s[4:6], 2
		case 8:
			fields[0], fields[1], fields[2], n = s[:4], s[4:6], s[6:8], 3
		default:
			return p, fmt.Errorf("bad date %q", s)
		}
	}
	widths := [3]int{4, 2, 2}
	var nums [3]int
	for i, f := range fields[:n] {
		v, ok := fixedDigits(f, widths[i])
		if !ok {
			return p, fmt.Errorf("bad date %q", s)
		}
		nums[i] = v
	}
	p.Year = nums[0]
	if n >= 2 {
		if nums[1] < 1 || nums[1] > 12 {
			return p, fmt.Errorf("bad month in %q", s)
		}
		p.Month, p.MonthKnown = nums[1], true
	}
	if n == 3 {
		if nums[2] < 1 || nums[2] > daysInMonth(p.Year, p.Month) {
			return p, fmt.Errorf("bad day in %q", s)
		}
		p.Day, p.DayKnown = nums[2], true
	}
	return p, nil
}

// ParseTime parses the BASE valid_iso8601_time forms: hh:mm:ss, hh:mm
// (extended), hhmmss, hhmm, hh (compact), each with an optional comma or dot
// fraction on the seconds and an optional zone of Z, ±hh, ±hhmm or ±hh:mm.
// The hour runs from 00 to 23 ("24:00:00" is not allowed) and the second up
// to 60, for a leap second. On an error the parts are zero.
func ParseTime(s string) (Time, error) {
	p, err := scanTime(s)
	if err != nil {
		return Time{}, err
	}
	return p, nil
}

// scanTime does the work of ParseTime. On an error the parts it returns may
// be partly filled.
func scanTime(s string) (Time, error) {
	var p Time
	body := s
	if i := strings.IndexAny(s, "Z+-"); i >= 0 {
		body, p.Zone, p.ZoneKnown = s[:i], s[i:], true
		if _, err := ParseZone(p.Zone); err != nil {
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
	var fields [3]string
	var n int
	if strings.Contains(main, ":") {
		p.colon = true
		var ok bool
		if fields, n, ok = cutFields(main, ":"); !ok {
			return p, fmt.Errorf("bad time %q", s)
		}
	} else { // compact form hh[mm[ss]]
		switch len(main) {
		case 2:
			fields[0], n = main, 1
		case 4:
			fields[0], fields[1], n = main[:2], main[2:4], 2
			p.basic = true
		case 6:
			fields[0], fields[1], fields[2], n = main[:2], main[2:4], main[4:6], 3
			p.basic = true
		default:
			return p, fmt.Errorf("bad time %q", s)
		}
	}
	if hasFrac && n != 3 {
		return p, fmt.Errorf("bad time %q", s)
	}
	var nums [3]int
	for i, f := range fields[:n] {
		v, ok := fixedDigits(f, 2)
		if !ok {
			return p, fmt.Errorf("bad time %q", s)
		}
		nums[i] = v
	}
	// BASE: hh is 00 to 23; "24:00:00" is not allowed, since it would mean
	// the date was really on the next day.
	if nums[0] > 23 {
		return p, fmt.Errorf("bad hour in %q", s)
	}
	p.Hour = nums[0]
	if n >= 2 {
		if nums[1] > 59 {
			return p, fmt.Errorf("bad minute in %q", s)
		}
		p.Minute, p.MinuteKnown = nums[1], true
	}
	if n == 3 {
		if nums[2] > 60 {
			return p, fmt.Errorf("bad second in %q", s)
		}
		p.Second, p.SecondKnown = nums[2], true
	}
	if hasFrac {
		// The joined text does not outlive the call, so the compiler builds
		// it in a stack buffer when it fits (up to 30 fraction digits).
		f, err := strconv.ParseFloat("0."+fracDigits, 64)
		if err != nil {
			return p, fmt.Errorf("bad fractional second in %q", s)
		}
		p.Fraction = f
	}
	return p, nil
}

// ParseDateTime parses the BASE valid_iso8601_date_time forms: a date as
// [ParseDate] reads it, optionally followed by "T" and a time as [ParseTime]
// reads it. The time needs a full date, and the date and time bodies are both
// extended or both compact; the zone may use either layout. On an error both
// parts are zero, so no caller reads a part taken before the parse stopped.
func ParseDateTime(s string) (Date, Time, error) {
	dp, tp, err := scanDateTime(s)
	if err != nil {
		return Date{}, Time{}, err
	}
	return dp, tp, nil
}

// scanDateTime does the work of ParseDateTime. On an error the parts it
// returns may be partly filled.
func scanDateTime(value string) (Date, Time, error) {
	datePart, timePart, hasT := strings.Cut(value, "T")
	dp, derr := ParseDate(datePart)
	if derr != nil {
		return dp, Time{}, derr
	}
	if !hasT {
		return dp, Time{}, nil
	}
	if !dp.DayKnown {
		return dp, Time{}, fmt.Errorf("time part needs a full date in %q", value)
	}
	tp, terr := ParseTime(timePart)
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
		return dp, Time{}, fmt.Errorf("mixed extended and compact forms in %q", value)
	}
	return dp, tp, nil
}

// cutFields splits s around each sep, as strings.Split does, but into a
// fixed array, so a scan does not allocate a slice per call. It returns the
// fields and their count, or false when s has more than three fields.
func cutFields(s, sep string) ([3]string, int, bool) {
	var fields [3]string
	n := strings.Count(s, sep) + 1
	if n > len(fields) {
		return fields, 0, false
	}
	for i := range n - 1 {
		fields[i], s, _ = strings.Cut(s, sep)
	}
	fields[n-1] = s
	return fields, n, true
}

// ParseZone parses a timezone designator (Z, ±hh, ±hhmm or ±hh:mm) and
// returns its offset in seconds east of UTC. The hour is at most 14 east and
// 12 west (TIME_DEFINITIONS Max_timezone_hour / Min_timezone_hour).
func ParseZone(tz string) (int, error) {
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

// ParseDuration parses the BASE ISO8601_DURATION form
// P[nnY][nnM][nnW][nnD][T[nnH][nnM][nnS]], plus the two documented openEHR
// deviations: a leading negative sign, and the W designator mixed with the
// others (in the order Y M W D). Designators appear at most once, in that
// order; a T section needs at least one component; a fraction (dot or comma)
// is allowed on the seconds only. On an error the parts are zero.
func ParseDuration(s string) (Duration, error) {
	p, err := scanDuration(s)
	if err != nil {
		return Duration{}, err
	}
	return p, nil
}

// scanDuration does the work of ParseDuration. On an error the parts it
// returns may be partly filled.
func scanDuration(s string) (Duration, error) {
	var p Duration
	orig := s
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		p.Negative = true
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
			r, p.Years = 1, whole
		case !inTime && c == 'M':
			r, p.Months = 2, whole
		case !inTime && c == 'W':
			r, p.Weeks = 3, whole
		case !inTime && c == 'D':
			r, p.Days = 4, whole
		case inTime && c == 'H':
			r, p.Hours = 1, whole
		case inTime && c == 'M':
			r, p.Minutes = 2, whole
		case inTime && c == 'S':
			r, p.Seconds, p.Fraction = 3, whole, frac
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
