package iso8601

import "cmp"

// The fields of a date, a time or a date-time, largest first, as the
// positions of [Fields].
const (
	fieldYear = iota
	fieldMonth
	fieldDay
	fieldHour
	fieldMinute
	fieldSecond
	fieldCount
)

// Fields is a date, a time or a date-time read field by field, from the
// largest down: the year, the month, the day, the hour, the minute, and the
// second with its fraction as one field. A date starts at the year and a
// time at the hour, and a reduced value leaves out the fields after its
// last. The zone is not applied: the fields are those of the clock as
// written. [Fields.Compare] orders two values of one type at the precision
// they share.
type Fields struct {
	v [fieldCount]float64
	// lo and hi bound the fields the value carries: lo is its first, hi
	// one past its last.
	lo, hi int
}

// DateFields returns the fields of the date d: the year, then the month and
// the day where d carries them.
func DateFields(d Date) Fields {
	f := Fields{lo: fieldYear, hi: fieldMonth}
	f.v[fieldYear] = float64(d.Year)
	f.appendDate(d)
	return f
}

// TimeFields returns the fields of the time t: the hour, then the minute and
// the second, its fraction included, where t carries them.
func TimeFields(t Time) Fields {
	f := Fields{lo: fieldHour, hi: fieldHour}
	f.appendTime(t)
	return f
}

// DateTimeFields returns the fields of the date-time of date d and time t:
// those of d, then, when hasTime is true, those of t. [ParseDateTime] gives
// a date-time with no time the zero Time, which reads as midnight, so
// hasTime says whether the text carries a time: it does exactly when it
// holds a "T".
func DateTimeFields(d Date, t Time, hasTime bool) Fields {
	f := DateFields(d)
	if hasTime && f.hi == fieldHour {
		f.appendTime(t)
	}
	return f
}

// appendDate adds the month and the day of d, where d carries them, after
// the year.
func (f *Fields) appendDate(d Date) {
	if !d.MonthKnown {
		return
	}
	f.v[fieldMonth], f.hi = float64(d.Month), fieldDay
	if d.DayKnown {
		f.v[fieldDay], f.hi = float64(d.Day), fieldHour
	}
}

// appendTime adds the hour of t, then its minute and its second where t
// carries them.
func (f *Fields) appendTime(t Time) {
	f.v[fieldHour], f.hi = float64(t.Hour), fieldMinute
	if !t.MinuteKnown {
		return
	}
	f.v[fieldMinute], f.hi = float64(t.Minute), fieldSecond
	if t.SecondKnown {
		f.v[fieldSecond], f.hi = float64(t.Second)+t.Fraction, fieldCount
	}
}

// Compare compares f with g at the precision the two share, field by field
// from the largest down over the fields both carry: it returns -1 or +1 by
// the first field that differs, and 0 when they are equal on every shared
// field. A reduced value so covers the whole period it names: "2021-05" is
// equal to "2021" and to "2021-05-15", and before "2021-06-01". The second
// and its fraction are one field, so "10:30:15" is before "10:30:15.5". f
// and g are values of one type; the zone is not applied, so values written
// in different zones are not compared as instants.
func (f Fields) Compare(g Fields) int {
	for i := max(f.lo, g.lo); i < min(f.hi, g.hi); i++ {
		if c := cmp.Compare(f.v[i], g.v[i]); c != 0 {
			return c
		}
	}
	return 0
}
