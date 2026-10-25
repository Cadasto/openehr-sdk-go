package constraints

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
)

// CDate constrains an RM ISO_DATE value (C_DATE). With no Pattern and no
// Range, Validate accepts exactly the strings rm.DVDate.ValidISO8601
// accepts: a full date, a year and month, or a year alone, in the extended
// layout ("2026-05-24", "2026-05", "2026") or the basic one ("20260524",
// "202605"), with the month and day checked against the calendar.
//
// Pattern is the ADL 1.4 date pattern, kept verbatim ("yyyy-mm-dd",
// "yyyy-??-XX"). Validate reads its letters in either case: a field written
// in its letters, or as a literal number ("1995-??-XX"), is mandatory, a
// field written ?? is optional, and one written XX is not allowed. A value
// the pattern does not admit is a CodePatternMismatch. A pattern outside
// that grammar, or one that breaks the ADL rule that only ?? or XX follow a
// ?? field and only XX an XX field, is a CodeInvalidValue on every value.
//
// Range is the AOM 1.4 range, in the order of rm.DVDate.Compare: a reduced
// date stands for the first day of the period it names, so "2004-05" is
// before "2004-05-20". A value outside it is a CodeOutOfRange.
//
// TimezoneValidity is the timezone_validity the ITS-XML C_DATE carries. A
// date has no zone, so ValidityOptional and ValidityDisallowed admit every
// date, and ValidityMandatory, which no date can meet, is a CodeInvalidValue
// on every value; the OPT parser refuses it in strict mode and reads it as
// optional otherwise.
type CDate struct {
	Pattern          string
	Range            TemporalRange
	TimezoneValidity ValidityKind
}

func (CDate) isPrimitive() {}

// ExampleValue returns a date written to Pattern and inside Range: the
// sentinel "2020-01-01", else the range's lower bound, else its upper
// bound, the first that Validate accepts. The sentinel takes the fields
// the pattern allows; a bound is tried as written, then with those fields,
// moved inside the range by one unit of its last field where that drops a
// part of it or its end is excluded. When none passes, it returns
// "2020-01-01", which Validate then refuses. That happens when no date
// meets both the pattern and the range, or the pattern or a bound is
// malformed, and also, though a date meets both, when the pattern writes
// the month or the day as a literal number beside a range: written to the
// pattern, a bound takes the literal, which can move it out of the range.
// "yyyy-06-dd" with the range up to 2004-05-20 gives "2020-01-01", though
// "2003-06-01" is valid. Only an OPT that breaks the ITS-XML pattern
// grammar, which admits no literal, reaches that case.
func (c CDate) ExampleValue() any {
	return clockExample(clockDate, c.Pattern, c.Range, c.TimezoneValidity, "2020-01-01")
}

// Validate accepts a Go string that rm.DVDate.ValidISO8601 accepts as a
// date value, partial dates included, that Pattern admits and that lies in
// Range. It reports any other string as not a valid ISO 8601 date, and
// each clause the date fails with its own violation.
func (c CDate) Validate(value any) []Violation {
	return validateClock(clockDate, c.Pattern, c.Range, c.TimezoneValidity, value)
}

// CTime constrains an RM ISO_TIME value (C_TIME). With no Pattern and no
// Range, Validate accepts exactly the strings rm.DVTime.ValidISO8601
// accepts: the extended layouts hh:mm:ss and hh:mm, the basic layouts
// hhmmss and hhmm, or hh alone, with a comma or dot fraction on the seconds
// and a zone of Z, ±hh, ±hhmm or ±hh:mm. The hour runs from 00 to 23, and
// the second may be 60 for a leap second.
//
// Pattern is the ADL 1.4 time pattern, kept verbatim ("hh:mm:ss",
// "hh:??:XX"), read as [CDate] reads a date pattern. A fraction is allowed
// wherever seconds are. A trailing timezone pattern makes a zone
// mandatory: "Z" admits Z only, and ±hh, ±hh:mm or ±hhmm, signed +, - or ±,
// admits any zone, Z included, whatever shape the suffix writes. Without
// one, a zone is optional.
//
// Range is the AOM 1.4 range. A time is ordered against each bound as the
// RM floor orders two DV_TIME bounds of a DV_INTERVAL: by
// rm.DVTime.Compare, clock-local, when the two carry one zone designator or
// none, a minute or a second the time leaves out counting as 0; as
// instants, each clock read in UTC, when both carry a zone; and not at all
// when one carries a zone and the other does not, so that bound does not
// refuse it. "08:00:00Z" is after "09:30:00+02:00", and "09:00:00" is not
// ordered against it. A value outside the range is a CodeOutOfRange.
//
// TimezoneValidity is the AOM 1.4 timezone_validity: ValidityMandatory
// refuses a time with no zone, and ValidityDisallowed one with a zone, as a
// CodePatternMismatch. Where Pattern writes a zone suffix too, both rules
// apply.
type CTime struct {
	Pattern          string
	Range            TemporalRange
	TimezoneValidity ValidityKind
}

func (CTime) isPrimitive() {}

// ExampleValue returns a time written to Pattern and inside Range, chosen
// as [CDate.ExampleValue] chooses a date, where a literal minute or second
// can move a bound out of the range. Written to the pattern, a time keeps
// its zone, takes Z where the pattern or TimezoneValidity requires a zone it
// lacks, is read in UTC where the pattern requires Z and it carries another
// zone, and drops its zone where TimezoneValidity disallows one. Its
// sentinel is "12:00:00". No candidate carries a fraction of a second or
// changes its zone, so a range only such a time meets gives the sentinel,
// which Validate refuses: the range above 23:59:59 gives "12:00:00",
// though "23:59:59.5" is valid.
func (c CTime) ExampleValue() any {
	return clockExample(clockTime, c.Pattern, c.Range, c.TimezoneValidity, "12:00:00")
}

// Validate accepts a Go string that rm.DVTime.ValidISO8601 accepts as a
// time value, partial times included, that Pattern admits and that lies in
// Range. It reports any other string as not a valid ISO 8601 time, and
// each clause the time fails with its own violation.
func (c CTime) Validate(value any) []Violation {
	return validateClock(clockTime, c.Pattern, c.Range, c.TimezoneValidity, value)
}

// CDateTime constrains an RM ISO_DATE_TIME value (C_DATE_TIME). With no
// Pattern and no Range, Validate accepts exactly the strings
// rm.DVDateTime.ValidISO8601 accepts: a date as [CDate] accepts it,
// optionally followed by "T" and a time as [CTime] accepts it. A time needs
// a full date before it, and the date and the time use the same layout,
// both extended ("2026-05-24T10:30:00Z") or both basic
// ("20260524T103000Z"); the zone may be written in either layout.
//
// Pattern is the ADL 1.4 date/time pattern, kept verbatim
// ("yyyy-mm-ddTHH:MM:SS", "yyyy-mm-ddThh:??:??"), read as [CDate] and
// [CTime] read theirs; "T" or a space separates its date from its time.
//
// Range is the AOM 1.4 range, in the order of rm.DVDateTime.Compare, where
// a reduced value stands for the start of the period it names, and the
// zones read as [CTime] reads them: a date-time is ordered against a bound
// as instants when both carry a zone and the designators differ, and not
// at all when one carries a zone and the other does not. A date with no
// time carries none. A value outside the range is a CodeOutOfRange.
//
// TimezoneValidity is read as [CTime] reads it; a date-time with no time has
// no zone, so ValidityMandatory refuses it.
type CDateTime struct {
	Pattern          string
	Range            TemporalRange
	TimezoneValidity ValidityKind
}

func (CDateTime) isPrimitive() {}

// ExampleValue returns a date-time written to Pattern and inside Range,
// chosen as [CDate.ExampleValue] chooses a date, with the zone written as
// [CTime.ExampleValue] writes it; a literal field after the year can move a
// bound out of the range. Its sentinel is "2020-01-01T12:00:00Z". No
// candidate carries a fraction of a second, so a range only such a
// date-time meets, one whose excluded bounds are a second apart, gives the
// sentinel, which Validate refuses.
func (c CDateTime) ExampleValue() any {
	return clockExample(clockDateTime, c.Pattern, c.Range, c.TimezoneValidity, "2020-01-01T12:00:00Z")
}

// Validate accepts a Go string that rm.DVDateTime.ValidISO8601 accepts as
// a date-time value, a date alone and other partial forms included, that
// Pattern admits and that lies in Range. It reports any other string as
// not a valid ISO 8601 date-time, and each clause the date-time fails with
// its own violation.
func (c CDateTime) Validate(value any) []Violation {
	return validateClock(clockDateTime, c.Pattern, c.Range, c.TimezoneValidity, value)
}

// CDuration constrains an RM ISO_DURATION value (C_DURATION). With no
// Pattern and no Range, Validate accepts exactly the strings
// rm.DVDuration.ValidISO8601 accepts: P[nY][nM][nW][nD][T[nH][nM][nS]],
// with each designator at most once and in that order, at least one
// component in all and at least one after a T, and a comma or dot fraction
// on the seconds only. Two openEHR additions to ISO 8601 are accepted too:
// a leading minus sign ("-P1D") and W mixed with the other designators
// ("P1Y2W").
//
// Pattern is the ADL 1.4 duration pattern, kept verbatim ("PYMWD", "PTM"):
// the designators a value may carry, Y, M, W and D before T and H, M and S
// after it, in either case. A value of either sign is admitted, and a
// fraction wherever S is. A value that carries another designator is a
// CodePatternMismatch. A pattern outside that grammar, or one that names no
// designator, is a CodeInvalidValue on every value. The AOM 1.4
// per-designator flags (years_allowed and the rest) are not read: the
// ITS-XML C_DURATION carries the pattern in their place.
//
// Range is the AOM 1.4 range, in the order of rm.DVDuration.Compare, by
// BASE Iso8601_duration.to_seconds: a year and a month count at the
// Time_definitions averages of 365.24 and 30.42 days, so P1D equals PT24H
// and P1Y is longer than P12M. A value outside it is a CodeOutOfRange.
type CDuration struct {
	Pattern string
	Range   TemporalRange
}

func (CDuration) isPrimitive() {}

// ExampleValue returns a duration written to Pattern and inside Range, the
// first that Validate accepts of: zero in days, or in the pattern's first
// designator when it names no days ("P0D", "P0Y", "PT0M"); the range's
// lower bound as written, then that length counted in each designator the
// pattern names, rounded up and moved past an excluded end; the upper bound
// likewise, rounded down. When none passes, as when no duration meets both
// the pattern and the range, or the pattern or a bound is malformed, it
// returns "P0D", which Validate then refuses. No candidate carries a
// fraction of a second, so it returns "P0D" too where only such a duration
// meets the range: the range between PT0S and PT0.5S, both excluded,
// though "PT0.25S" is valid.
func (c CDuration) ExampleValue() any {
	allowed := allDesignators
	if c.Pattern != "" {
		set, ok := parseDurationPattern(c.Pattern)
		if !ok {
			return "P0D"
		}
		allowed = set
	}
	for _, s := range c.exampleCandidates(allowed) {
		if len(c.Validate(s)) == 0 {
			return s
		}
	}
	return "P0D"
}

// exampleCandidates returns the durations ExampleValue tries, in order,
// for a pattern that admits the designators allowed.
func (c CDuration) exampleCandidates(allowed uint8) []string {
	zero := "P0D"
	if allowed&designatorDays == 0 {
		for _, u := range durationUnits {
			if allowed&u.bit != 0 {
				zero = formatCount(0, u)
				break
			}
		}
	}
	out := []string{zero}
	r := c.Range
	if r.IsBounded() {
		if !r.LowerUnbounded {
			out = append(out, r.Lower)
			if lo, ok := durationSeconds(r.Lower); ok {
				out = appendCounts(out, lo, allowed, true, !r.LowerInclusive)
			}
		}
		if !r.UpperUnbounded {
			out = append(out, r.Upper)
			if hi, ok := durationSeconds(r.Upper); ok {
				out = appendCounts(out, hi, allowed, false, !r.UpperInclusive)
			}
		}
	}
	return out
}

// Validate accepts a Go string that rm.DVDuration.ValidISO8601 accepts as
// a duration value, that carries only the designators Pattern names and
// that lies in Range. It reports any other string, such as "", "P", "PT" or
// "P1DT", as not a valid ISO 8601 duration, and each clause the duration
// fails with its own violation.
func (c CDuration) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string duration, got %T", value)}}
	}
	d, err := iso8601.ParseDuration(s)
	if err != nil {
		return []Violation{{Code: CodeInvalidValue, Detail: "value is not a valid ISO 8601 duration", Value: Redact(value)}}
	}
	var out []Violation
	if c.Pattern != "" {
		if allowed, ok := parseDurationPattern(c.Pattern); !ok {
			out = append(out, malformedPattern(c.Pattern, "duration"))
		} else if designatorsOf(s)&^allowed != 0 {
			out = append(out, patternMismatch(c.Pattern, value))
		}
	}
	secs := d.ToSeconds()
	order := func(bound string) (sign int, ordered, ok bool) {
		b, ok := durationSeconds(bound)
		return cmp.Compare(secs, b), true, ok
	}
	return appendRangeViolation(out, c.Range, order, "duration", value)
}

// ValidityKind is the AOM 1.4 VALIDITY_KIND of the timezone_validity of a
// C_DATE, C_TIME or C_DATE_TIME: whether a value must, may or must not carry
// a time zone. The ITS-XML schema writes it as 1001 (mandatory), 1002
// (optional) or 1003 (disallowed). The zero value is ValidityOptional, so a
// constraint that sets none leaves the zone open.
type ValidityKind uint8

const (
	// ValidityOptional admits a value with or without a zone: 1002, or no
	// timezone_validity.
	ValidityOptional ValidityKind = iota
	// ValidityMandatory requires a zone: 1001.
	ValidityMandatory
	// ValidityDisallowed refuses a zone: 1003.
	ValidityDisallowed
)

// TemporalRange is the AOM 1.4 range of a C_DATE, C_TIME, C_DATE_TIME or
// C_DURATION: an interval whose bounds are ISO 8601 text of the
// constrained type ("2004-05-20", "09:30:00", "PT0S"). Its flags read as
// those of [NumericRange]: LowerInclusive and UpperInclusive close a side,
// and LowerUnbounded and UpperUnbounded open it, its bound then ignored.
//
// The zero value constrains nothing, as if both sides were unbounded:
// [TemporalRange.IsBounded] reports false for it. A side that is not
// unbounded needs a bound the ISO 8601 parse of the constrained type
// accepts; the constraint's Validate reports a bound it refuses as a
// CodeInvalidValue on every value.
type TemporalRange struct {
	// Lower and Upper are the bounds, as ISO 8601 text of the constrained
	// type.
	Lower, Upper string

	// LowerInclusive and UpperInclusive close the bounds. The zero value
	// is exclusive on both sides; the OPT parser reads a flag the OPT
	// omits as true on a bounded side and false on an unbounded one.
	LowerInclusive, UpperInclusive bool

	// LowerUnbounded and UpperUnbounded leave a side open; Lower or Upper
	// is then ignored.
	LowerUnbounded, UpperUnbounded bool
}

// IsBounded reports whether the range constrains anything. It returns
// false for the zero value and for a range with both sides unbounded.
func (r TemporalRange) IsBounded() bool {
	if r == (TemporalRange{}) {
		return false
	}
	return !r.LowerUnbounded || !r.UpperUnbounded
}

// String renders the range in interval notation, as [NumericRange.String]
// does: "[P0Y..P150Y]" for a closed range, "[PT0S..*)" for one open above.
// The zero value renders as "(*..*)".
func (r TemporalRange) String() string {
	unbounded := !r.IsBounded()
	var sb strings.Builder
	if r.LowerInclusive {
		sb.WriteByte('[')
	} else {
		sb.WriteByte('(')
	}
	if r.LowerUnbounded || unbounded {
		sb.WriteByte('*')
	} else {
		sb.WriteString(r.Lower)
	}
	sb.WriteString("..")
	if r.UpperUnbounded || unbounded {
		sb.WriteByte('*')
	} else {
		sb.WriteString(r.Upper)
	}
	if r.UpperInclusive {
		sb.WriteByte(']')
	} else {
		sb.WriteByte(')')
	}
	return sb.String()
}

// boundOrder compares a value with a bound written as ISO 8601 text: sign
// is the sign of the value less the bound, ordered is false for a pair the
// order leaves unordered, and ok is false for a bound that is not ISO 8601
// text of the constrained type.
type boundOrder func(bound string) (sign int, ordered, ok bool)

// contains reports whether a value lies in the range, order comparing it
// with each bound. A bound the value is not ordered against does not
// exclude it. ok is false when a side that is not unbounded has a bound
// order cannot read.
func (r TemporalRange) contains(order boundOrder) (in, ok bool) {
	if !r.IsBounded() {
		return true, true
	}
	in = true
	if !r.LowerUnbounded {
		c, ordered, ok := order(r.Lower)
		if !ok {
			return false, false
		}
		if ordered {
			in = c > 0 || (r.LowerInclusive && c == 0)
		}
	}
	if !r.UpperUnbounded {
		c, ordered, ok := order(r.Upper)
		if !ok {
			return false, false
		}
		if ordered {
			in = in && (c < 0 || (r.UpperInclusive && c == 0))
		}
	}
	return in, true
}

// appendRangeViolation appends to out the violation of the range clause
// for a value that order compares with each bound, if any: a
// CodeInvalidValue for a bound order cannot read, or a CodeOutOfRange for a
// value outside r. kind names the constrained type for the Detail.
func appendRangeViolation(out []Violation, r TemporalRange, order boundOrder, kind string, value any) []Violation {
	in, ok := r.contains(order)
	switch {
	case !ok:
		return append(out, Violation{Code: CodeInvalidValue, Detail: fmt.Sprintf("constraint range %s has a bound that is not a valid ISO 8601 %s", r, kind)})
	case !in:
		return append(out, Violation{Code: CodeOutOfRange, Detail: fmt.Sprintf("value outside %s", r), Value: Redact(value)})
	}
	return out
}

// malformedPattern is the violation for a pattern outside the ADL 1.4
// grammar of kind. It holds no Value: the constraint is at fault, not the
// value.
func malformedPattern(pattern, kind string) Violation {
	return Violation{Code: CodeInvalidValue, Detail: fmt.Sprintf("constraint pattern %q is not a valid ADL 1.4 %s pattern", pattern, kind)}
}

// patternMismatch is the violation for a value pattern does not admit.
func patternMismatch(pattern string, value any) Violation {
	return Violation{Code: CodePatternMismatch, Detail: fmt.Sprintf("value does not match pattern %q", pattern), Value: Redact(value)}
}

// clockKind says which fields the values of a date, time or date-time
// constraint carry.
type clockKind uint8

const (
	clockDate clockKind = iota
	clockTime
	clockDateTime
)

// The fields of a date/time pattern and value, in their written order.
const (
	fieldYear = iota
	fieldMonth
	fieldDay
	fieldHour
	fieldMinute
	fieldSecond
	fieldCount
)

// span returns the fields a value of the kind carries, from lo up to hi.
func (k clockKind) span() (lo, hi int) {
	switch k {
	case clockDate:
		return fieldYear, fieldHour
	case clockTime:
		return fieldHour, fieldCount
	case clockDateTime:
	}
	return fieldYear, fieldCount
}

// String names the kind as a Detail does.
func (k clockKind) String() string {
	switch k {
	case clockDate:
		return "date"
	case clockTime:
		return "time"
	case clockDateTime:
	}
	return "date-time"
}

// fieldLayouts give, for each field, the letter a pattern writes it in,
// its width, and the separator written before it; the first field of a
// kind has none.
var fieldLayouts = [fieldCount]struct {
	letter byte
	width  int
	before byte
}{
	fieldYear:   {'y', 4, 0},
	fieldMonth:  {'m', 2, '-'},
	fieldDay:    {'d', 2, '-'},
	fieldHour:   {'h', 2, 'T'},
	fieldMinute: {'m', 2, ':'},
	fieldSecond: {'s', 2, ':'},
}

// fieldMinimum is the value a field the value leaves out stands for.
var fieldMinimum = [fieldCount]int{fieldMonth: 1, fieldDay: 1}

// validity is what a pattern field asks of the value's field, the AOM 1.4
// VALIDITY_KIND. Its order matters: in a valid pattern it never decreases
// from left to right.
type validity uint8

const (
	fieldMandatory validity = iota
	fieldOptional
	fieldDisallowed
)

// patternField is one field of a date/time pattern.
type patternField struct {
	validity validity
	// literal is the value a field written as a number requires, or -1.
	literal int
}

// zoneRule is what the timezone pattern of a time or date/time pattern
// asks of the value.
type zoneRule uint8

const (
	zoneOptional zoneRule = iota // no timezone pattern
	zoneRequired                 // ±hh, ±hh:mm or ±hhmm: a zone, Z included
	zoneUTC                      // Z: the zone Z
)

// clockPattern is an ADL 1.4 date, time or date/time pattern, read.
type clockPattern struct {
	fields [fieldCount]patternField
	zone   zoneRule
}

// anyClock is the pattern of a constraint that gives none: each field is
// optional, and so is the zone.
func anyClock() clockPattern {
	var p clockPattern
	for i := range p.fields {
		p.fields[i] = patternField{validity: fieldOptional, literal: -1}
	}
	return p
}

// parseClockPattern reads s as an ADL 1.4 pattern of kind k: yyyy-mm-dd,
// hh:mm:ss, or the two joined by T or a space, each field in its letters,
// ?? or XX (the year, and the hour of a time, only in letters) or a
// literal number, in either case, and a time followed by an optional
// timezone pattern. It reports false for other text, for a pattern that
// breaks the ADL rule that only ?? or XX follow a ?? field and only XX an
// XX field, and for a timezone pattern with no time to follow.
func parseClockPattern(k clockKind, s string) (clockPattern, bool) {
	var p clockPattern
	lo, hi := k.span()
	rest := s
	for i := lo; i < hi; i++ {
		l := fieldLayouts[i]
		if i > lo {
			if rest == "" || !separates(rest[0], l.before) {
				return p, false
			}
			rest = rest[1:]
		}
		if len(rest) < l.width {
			return p, false
		}
		f, ok := readPatternField(rest[:l.width], l.letter, i > lo)
		if !ok {
			return p, false
		}
		p.fields[i] = f
		rest = rest[l.width:]
	}
	zone, ok := readZonePattern(rest)
	if !ok || (k == clockDate && zone != zoneOptional) {
		return p, false
	}
	p.zone = zone
	prev := fieldMandatory
	for i := lo; i < hi; i++ {
		if p.fields[i].validity < prev {
			return p, false
		}
		prev = p.fields[i].validity
	}
	if zone != zoneOptional && p.fields[fieldHour].validity == fieldDisallowed {
		return p, false
	}
	return p, true
}

// separates reports whether c is the separator want of a pattern: the
// letter T, in either case, or a space, before the time of a date/time.
func separates(c, want byte) bool {
	if want == 'T' {
		return c == 'T' || c == 't' || c == ' '
	}
	return c == want
}

// readPatternField reads one field of a date/time pattern: the letter
// repeated, ?? or XX where the field may vary, or a literal number.
func readPatternField(s string, letter byte, canVary bool) (patternField, bool) {
	switch {
	case repeats(s, letter):
		return patternField{validity: fieldMandatory, literal: -1}, true
	case canVary && repeats(s, '?'):
		return patternField{validity: fieldOptional, literal: -1}, true
	case canVary && repeats(s, 'x'):
		return patternField{validity: fieldDisallowed, literal: -1}, true
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return patternField{}, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return patternField{}, false
	}
	return patternField{validity: fieldMandatory, literal: n}, true
}

// readZonePattern reads the timezone pattern that may follow a time: none,
// Z, or a sign (+, - or ±) with hh, hh:mm or hhmm, in either case.
func readZonePattern(s string) (zoneRule, bool) {
	switch s {
	case "":
		return zoneOptional, true
	case "Z", "z":
		return zoneUTC, true
	}
	body, ok := strings.CutPrefix(s, "±")
	if !ok {
		if s[0] != '+' && s[0] != '-' {
			return 0, false
		}
		body = s[1:]
	}
	hh, mm, colon := strings.Cut(body, ":")
	switch {
	case colon && len(hh) == 2 && len(mm) == 2:
	case !colon && len(body) == 4:
		hh, mm = body[:2], body[2:]
	case !colon && len(body) == 2:
	default:
		return 0, false
	}
	if !repeats(hh, 'h') || (mm != "" && !repeats(mm, 'm')) {
		return 0, false
	}
	return zoneRequired, true
}

// repeats reports whether s is the lower-case ASCII c repeated, each byte
// in either case.
func repeats(s string, c byte) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		b := s[i]
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if b != c {
			return false
		}
	}
	return true
}

// clockValue is a date, time or date-time value, read by the RM's ISO 8601
// parse.
type clockValue struct {
	date    iso8601.Date
	time    iso8601.Time
	hasTime bool
}

// parseClock reads s as a value of kind k, or reports false when the RM's
// ISO 8601 parse refuses it.
func parseClock(k clockKind, s string) (clockValue, bool) {
	var v clockValue
	var err error
	switch k {
	case clockDate:
		v.date, err = iso8601.ParseDate(s)
	case clockTime:
		v.time, err = iso8601.ParseTime(s)
		v.hasTime = true
	case clockDateTime:
		v.date, v.time, err = iso8601.ParseDateTime(s)
		// A date carries no T, so a date-time that parses has a time
		// exactly when it has one.
		v.hasTime = strings.Contains(s, "T")
	}
	return v, err == nil
}

// field returns the value of field i and whether the value carries it.
func (v clockValue) field(i int) (int, bool) {
	switch i {
	case fieldYear:
		return v.date.Year, true
	case fieldMonth:
		return v.date.Month, v.date.MonthKnown
	case fieldDay:
		return v.date.Day, v.date.DayKnown
	case fieldHour:
		return v.time.Hour, v.hasTime
	case fieldMinute:
		return v.time.Minute, v.time.MinuteKnown
	}
	return v.time.Second, v.time.SecondKnown
}

// magnitude is the value's place in the order of the RM's Compare: days
// for a date, clock-local seconds for a time or a date-time.
func (v clockValue) magnitude(k clockKind) float64 {
	switch k {
	case clockDate:
		return float64(v.date.Magnitude())
	case clockTime:
		return v.time.Magnitude()
	case clockDateTime:
	}
	return iso8601.DateTimeMagnitude(v.date, v.time)
}

// zone returns the zone designator the value carries, as written, or ""
// for a date, a time with no zone, or a date-time with no time.
func (v clockValue) zone() string {
	return v.time.Zone
}

// clockOrder returns the order of the value v of kind k against a bound,
// the order REQ-112 gives two DV_TIME or DV_DATE_TIME bounds of a
// DV_INTERVAL: by the clock-local magnitudes when the two carry one zone
// designator or none, as instants (iso8601.UTCMagnitude) when both carry a
// zone and the designators differ, and not ordered when one carries a zone
// and the other does not. A date carries none, so dates are ordered by
// their magnitudes.
func clockOrder(k clockKind, v clockValue) boundOrder {
	mag, zone := v.magnitude(k), v.zone()
	return func(bound string) (sign int, ordered, ok bool) {
		b, ok := parseClock(k, bound)
		if !ok {
			return 0, false, false
		}
		bmag, bzone := b.magnitude(k), b.zone()
		if zone == bzone {
			return cmp.Compare(mag, bmag), true, true
		}
		utc, zoned := iso8601.UTCMagnitude(mag, zone)
		butc, bzoned := iso8601.UTCMagnitude(bmag, bzone)
		if !zoned || !bzoned {
			return 0, false, true
		}
		return cmp.Compare(utc, butc), true, true
	}
}

// admits reports whether the pattern admits the value v of kind k.
func (p clockPattern) admits(k clockKind, v clockValue) bool {
	lo, hi := k.span()
	for i := lo; i < hi; i++ {
		n, known := v.field(i)
		switch f := p.fields[i]; f.validity {
		case fieldMandatory:
			if !known || (f.literal >= 0 && n != f.literal) {
				return false
			}
		case fieldOptional:
		case fieldDisallowed:
			if known {
				return false
			}
		}
	}
	switch p.zone {
	case zoneRequired:
		return v.time.ZoneKnown
	case zoneUTC:
		return v.time.Zone == "Z"
	case zoneOptional:
	}
	return true
}

// validateClock is the Validate of CDate, CTime and CDateTime, kind k
// naming which.
func validateClock(k clockKind, pattern string, r TemporalRange, tz ValidityKind, value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string %s, got %T", k, value)}}
	}
	v, ok := parseClock(k, s)
	if !ok {
		return []Violation{{Code: CodeInvalidValue, Detail: "value is not a valid ISO 8601 " + k.String(), Value: Redact(value)}}
	}
	var out []Violation
	if pattern != "" {
		if p, ok := parseClockPattern(k, pattern); !ok {
			out = append(out, malformedPattern(pattern, k.String()))
		} else if !p.admits(k, v) {
			out = append(out, patternMismatch(pattern, value))
		}
	}
	out = appendZoneViolation(out, k, tz, v, value)
	return appendRangeViolation(out, r, clockOrder(k, v), k.String(), value)
}

// appendZoneViolation appends to out the violation of the timezone_validity
// clause tz for the value v of kind k, if any: a CodePatternMismatch for a
// value with no zone where tz makes one mandatory, or with one where tz
// disallows it, and a CodeInvalidValue, which holds no Value, for a tz no
// value can meet: mandatory on a date, which carries no zone, or a value
// outside the three kinds.
func appendZoneViolation(out []Violation, k clockKind, tz ValidityKind, v clockValue, value any) []Violation {
	switch tz {
	case ValidityOptional:
	case ValidityMandatory:
		switch {
		case k == clockDate:
			return append(out, Violation{Code: CodeInvalidValue, Detail: "constraint timezone_validity mandatory cannot hold for a date, which carries no time zone"})
		case v.zone() == "":
			return append(out, Violation{Code: CodePatternMismatch, Detail: "value carries no time zone, which timezone_validity makes mandatory", Value: Redact(value)})
		}
	case ValidityDisallowed:
		if v.zone() != "" {
			return append(out, Violation{Code: CodePatternMismatch, Detail: "value carries a time zone, which timezone_validity disallows", Value: Redact(value)})
		}
	default:
		return append(out, Violation{Code: CodeInvalidValue, Detail: fmt.Sprintf("constraint timezone_validity %d is not a VALIDITY_KIND", tz)})
	}
	return out
}

// clockExample is the ExampleValue of CDate, CTime and CDateTime, kind k
// naming which; see [CDate.ExampleValue] and [CTime.ExampleValue].
func clockExample(k clockKind, pattern string, r TemporalRange, tz ValidityKind, sentinel string) string {
	p := anyClock()
	if pattern != "" {
		var ok bool
		if p, ok = parseClockPattern(k, pattern); !ok {
			return sentinel
		}
	}
	var candidates []string
	if v, ok := parseClock(k, sentinel); ok {
		candidates = append(candidates, p.fit(k, v, tz, 0, false))
	}
	if r.IsBounded() {
		if !r.LowerUnbounded {
			candidates = append(candidates, r.Lower)
			if v, ok := parseClock(k, r.Lower); ok {
				candidates = append(candidates, p.fit(k, v, tz, 1, !r.LowerInclusive))
			}
		}
		if !r.UpperUnbounded {
			candidates = append(candidates, r.Upper)
			if v, ok := parseClock(k, r.Upper); ok {
				candidates = append(candidates, p.fit(k, v, tz, -1, !r.UpperInclusive))
			}
		}
	}
	for _, s := range candidates {
		if s != "" && len(validateClock(k, pattern, r, tz, s)) == 0 {
			return s
		}
	}
	return sentinel
}

// fit writes the value v of kind k with the fields the pattern allows,
// fractions left out, and with the zone [clockPattern.zoneOf] gives it
// under the constraint's timezone_validity tz.
// Toward 1 it moves the result up by one unit of its last field when that
// drops a part of v, or when excluded asks for a value past v; toward -1 it
// moves it down by one unit when excluded asks for a value before v. A
// field the pattern writes as a literal takes that literal. It returns ""
// when a move leaves the day of a time.
func (p clockPattern) fit(k clockKind, v clockValue, tz ValidityKind, toward int, excluded bool) string {
	lo, hi := k.span()
	var f [fieldCount]int
	for i := lo; i < hi; i++ {
		n, known := v.field(i)
		if !known {
			n = fieldMinimum[i]
		}
		f[i] = n
	}
	zone, shift := p.zoneOf(v, tz)
	if shift != 0 && !moveField(k, &f, fieldSecond, shift) {
		return ""
	}
	cut := hi
	for i := lo; i < hi; i++ {
		if p.fields[i].validity == fieldDisallowed {
			cut = i
			break
		}
	}
	dropped := v.time.Fraction > 0
	for i := cut; i < hi; i++ {
		dropped = dropped || f[i] != fieldMinimum[i]
		f[i] = fieldMinimum[i]
	}
	var step int
	switch {
	case toward > 0 && (dropped || excluded):
		step = 1
	case toward < 0 && excluded && !dropped:
		step = -1
	}
	if step != 0 && !moveField(k, &f, cut-1, step) {
		return ""
	}
	for i := lo; i < cut; i++ {
		if lit := p.fields[i].literal; lit >= 0 {
			f[i] = lit
		}
	}
	return formatClock(k, f, cut, zone)
}

// zoneOf returns the zone a value written to the pattern carries under the
// timezone_validity tz: none where tz disallows one; else the zone of v, Z
// where the pattern or tz asks for a zone that v lacks, and Z where the
// pattern asks for Z and v carries another zone. In that last case shift is
// the seconds that read the clock of v in UTC, its zone's offset from UTC
// negated; it is 0 otherwise.
func (p clockPattern) zoneOf(v clockValue, tz ValidityKind) (zone string, shift int) {
	if tz == ValidityDisallowed {
		return "", 0
	}
	zone = v.zone()
	if tz == ValidityMandatory && zone == "" {
		zone = "Z"
	}
	switch p.zone {
	case zoneUTC:
		if zone != "" && zone != "Z" {
			// v parsed, so its zone does.
			offset, _ := iso8601.ParseZone(zone)
			shift = -offset
		}
		zone = "Z"
	case zoneRequired:
		if zone == "" {
			zone = "Z"
		}
	case zoneOptional:
	}
	return zone, shift
}

// moveField adds step units of field i to the fields f of a value of kind
// k, carrying into the fields before it. It reports false when a time
// would leave its day.
func moveField(k clockKind, f *[fieldCount]int, i, step int) bool {
	switch i {
	case fieldYear:
		f[fieldYear] += step
	case fieldMonth:
		months := f[fieldYear]*12 + f[fieldMonth] - 1 + step
		f[fieldYear], f[fieldMonth] = months/12, months%12+1
	case fieldDay:
		setDate(f, dateOf(f).Magnitude()+step)
	default:
		units := [fieldCount]int{fieldHour: 3600, fieldMinute: 60, fieldSecond: 1}
		secs := f[fieldHour]*3600 + f[fieldMinute]*60 + f[fieldSecond] + step*units[i]
		const day = 86400
		switch {
		case secs >= day && k == clockDateTime:
			setDate(f, dateOf(f).Magnitude()+1)
			secs -= day
		case secs < 0 && k == clockDateTime:
			setDate(f, dateOf(f).Magnitude()-1)
			secs += day
		case secs < 0 || secs >= day:
			return false
		}
		f[fieldHour], f[fieldMinute], f[fieldSecond] = secs/3600, secs%3600/60, secs%60
	}
	return true
}

// dateOf returns the full date of the fields f.
func dateOf(f *[fieldCount]int) iso8601.Date {
	return iso8601.Date{Year: f[fieldYear], Month: f[fieldMonth], Day: f[fieldDay], MonthKnown: true, DayKnown: true}
}

// setDate sets the date fields of f to the date of magnitude days.
func setDate(f *[fieldCount]int, days int) {
	d := iso8601.DateOfMagnitude(days)
	f[fieldYear], f[fieldMonth], f[fieldDay] = d.Year, d.Month, d.Day
}

// formatClock writes the fields of f before cut in the extended layout of
// kind k, and zone after them when they reach the hour.
func formatClock(k clockKind, f [fieldCount]int, cut int, zone string) string {
	lo, _ := k.span()
	var sb strings.Builder
	for i := lo; i < cut; i++ {
		if i > lo {
			sb.WriteByte(fieldLayouts[i].before)
		}
		if i == fieldYear {
			fmt.Fprintf(&sb, "%04d", f[i])
		} else {
			fmt.Fprintf(&sb, "%02d", f[i])
		}
	}
	if cut > fieldHour {
		sb.WriteString(zone)
	}
	return sb.String()
}

// The designators of a duration, as bits of a set.
const (
	designatorYears uint8 = 1 << iota
	designatorMonths
	designatorWeeks
	designatorDays
	designatorHours
	designatorMinutes
	designatorSeconds

	allDesignators = designatorSeconds<<1 - 1
)

// durationUnit is one designator of a duration: its bit, its letter,
// whether it follows T, and one of it, whose BASE to_seconds is its
// length.
type durationUnit struct {
	bit    uint8
	letter byte
	inTime bool
	one    iso8601.Duration
}

// durationUnits are the designators in their written order.
var durationUnits = [...]durationUnit{
	{designatorYears, 'Y', false, iso8601.Duration{Years: 1}},
	{designatorMonths, 'M', false, iso8601.Duration{Months: 1}},
	{designatorWeeks, 'W', false, iso8601.Duration{Weeks: 1}},
	{designatorDays, 'D', false, iso8601.Duration{Days: 1}},
	{designatorHours, 'H', true, iso8601.Duration{Hours: 1}},
	{designatorMinutes, 'M', true, iso8601.Duration{Minutes: 1}},
	{designatorSeconds, 'S', true, iso8601.Duration{Seconds: 1}},
}

// parseDurationPattern reads s as an ADL 1.4 duration pattern,
// P[Y][M][W][D][T[H][M][S]] in either case (the ITS-XML
// DurationConstraintPattern), into the set of designators it names. It
// reports false for other text and for a pattern that names none.
func parseDurationPattern(s string) (uint8, bool) {
	if s == "" || (s[0] != 'P' && s[0] != 'p') {
		return 0, false
	}
	var set uint8
	next, inTime := 0, false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c == 'T' && !inTime {
			inTime = true
			continue
		}
		j := next
		for j < len(durationUnits) && (durationUnits[j].inTime != inTime || durationUnits[j].letter != c) {
			if durationUnits[j].inTime && !inTime {
				return 0, false
			}
			j++
		}
		if j == len(durationUnits) {
			return 0, false
		}
		set |= durationUnits[j].bit
		next = j + 1
	}
	return set, set != 0
}

// designatorsOf returns the designators a duration the RM's parse accepts
// carries.
func designatorsOf(s string) uint8 {
	var set uint8
	inTime := false
	for i := range len(s) {
		c := s[i]
		if c == 'T' {
			inTime = true
			continue
		}
		for _, u := range durationUnits {
			if u.letter == c && u.inTime == inTime {
				set |= u.bit
				break
			}
		}
	}
	return set
}

// durationSeconds reads s as a duration's BASE to_seconds, or reports false
// when the RM's parse refuses it.
func durationSeconds(s string) (float64, bool) {
	d, err := iso8601.ParseDuration(s)
	return d.ToSeconds(), err == nil
}

// appendCounts appends to out the length secs counted in each designator
// allowed: rounded up when up, else down, and moved one unit further when
// excluded and the count lands on secs.
func appendCounts(out []string, secs float64, allowed uint8, up, excluded bool) []string {
	for _, u := range durationUnits {
		if allowed&u.bit == 0 {
			continue
		}
		unit := u.one.ToSeconds()
		q := secs / unit
		if q > 1e15 || q < -1e15 {
			continue
		}
		n := int64(q)
		switch {
		case up && float64(n) < q:
			n++
		case !up && float64(n) > q:
			n--
		}
		if excluded && float64(n)*unit == secs {
			if up {
				n++
			} else {
				n--
			}
		}
		out = append(out, formatCount(n, u))
	}
	return out
}

// formatCount writes n of the designator u as a duration, with a leading
// minus sign when n is negative.
func formatCount(n int64, u durationUnit) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	t := ""
	if u.inTime {
		t = "T"
	}
	return fmt.Sprintf("%sP%s%d%c", sign, t, n, u.letter)
}
