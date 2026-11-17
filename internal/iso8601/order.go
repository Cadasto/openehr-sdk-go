package iso8601

// maxZoneOffset is the farthest, in seconds, that XML Schema 1.0 Part 2
// §3.2.7.4 reads a value with no zone from UTC when it orders it against a
// value with one: fourteen hours, either way.
const maxZoneOffset = 14 * 3600

// Span is the stretch of time a time or a date-time covers, in its
// clock-local magnitudes ([Time.Magnitude], [DateTimeMagnitude]), the zone
// not applied. Start is the value's magnitude. End is the start of the next
// period for a reduced value, so "2030" spans its whole year and "10:30" its
// whole minute; a value whose seconds are known is one point, End equal to
// Start, since its second and fraction are one field as [Fields.Compare]
// reads them.
type Span struct {
	Start, End float64
}

// TimeSpan returns the span of the time t: from its magnitude to the start
// of the next hour where t leaves out the minutes, to the start of the next
// minute where it leaves out the seconds, and one point otherwise.
func TimeSpan(t Time) Span {
	start := t.Magnitude()
	return Span{Start: start, End: start + reducedLength(t)}
}

// DateTimeSpan returns the span of the date-time of date d and time t,
// which carries t when hasTime is true, as [DateTimeFields] reads it. A
// date-time with a time spans as [TimeSpan] says, on its date. One with no
// time, or with a date that leaves out the day, spans its whole day, month
// or year, up to the start of the next.
func DateTimeSpan(d Date, t Time, hasTime bool) Span {
	if hasTime && d.DayKnown {
		start := DateTimeMagnitude(d, t)
		return Span{Start: start, End: start + reducedLength(t)}
	}
	return Span{Start: float64(d.Magnitude()) * secondsPerDay, End: float64(d.nextMagnitude()) * secondsPerDay}
}

// reducedLength is the seconds from the start of the time t to the start of
// the next period it names: an hour where t leaves out the minutes, a minute
// where it leaves out the seconds, and none otherwise.
func reducedLength(t Time) float64 {
	switch {
	case !t.MinuteKnown:
		return 3600
	case !t.SecondKnown:
		return 60
	}
	return 0
}

// nextMagnitude returns the [Date.Magnitude] of the first day after the
// period the date names: the next day, or the first day of the next month or
// year where the date leaves out the day or the month.
func (p Date) nextMagnitude() int {
	switch {
	case !p.MonthKnown:
		return Date{Year: p.Year + 1}.Magnitude()
	case !p.DayKnown:
		if p.Month == 12 {
			return Date{Year: p.Year + 1}.Magnitude()
		}
		return Date{Year: p.Year, Month: p.Month + 1, MonthKnown: true}.Magnitude()
	}
	return p.Magnitude() + 1
}

// OrderAcrossZone orders a value written with the zone designator zone,
// which covers the span zoned, against a value written with none, which
// covers the span zoneless, as XML Schema 1.0 Part 2 §3.2.7.4 orders a
// dateTime with a zone against one without: the first read in UTC
// ([UTCMagnitude]), the second read at +14:00 and at -14:00, the zones
// farthest from UTC. sign is -1 where zoned ends before zoneless starts at
// +14:00, and +1 where zoned starts after zoneless ends at -14:00.
//
// sign is 0 where zoned lies inside zoneless at both readings: from 14 hours
// after zoneless starts up to 14 hours before it ends. The two are then
// equal, as two values equal on every field they share are, so
// 2030-06-01T10:00Z is equal to 2030 as 2030-06-01T10:00 is. zoneless covers
// its period up to the start of the next, so a point 14 hours before
// zoneless ends is the start of the next period at +14:00 and not inside,
// while a reduced zoned span that ends there is. Only a month or a longer
// period is long enough to hold a zoned value at both readings.
//
// ok is false otherwise, where the two readings disagree: zoned then lies
// within 14 hours of where zoneless starts or ends, 14 hours exactly
// included. ok is false too where [ParseZone] refuses zone, the empty zone
// included.
func OrderAcrossZone(zoned Span, zone string, zoneless Span) (sign int, ok bool) {
	start, ok := UTCMagnitude(zoned.Start, zone)
	if !ok {
		return 0, false
	}
	end, _ := UTCMagnitude(zoned.End, zone)
	// inner is where zoneless ends at +14:00, the earlier of its two ends.
	inner := zoneless.End - maxZoneOffset
	switch {
	case end < zoneless.Start-maxZoneOffset:
		return -1, true
	case start > zoneless.End+maxZoneOffset:
		return 1, true
	case start >= zoneless.Start+maxZoneOffset && (end < inner || (end == inner && start < end)):
		return 0, true
	}
	return 0, false
}
