package iso8601

// The openEHR BASE Time_definitions constants (foundation_types) the
// magnitudes below use. secondsPerDay is spd, Hours_in_day ×
// Minutes_in_hour × Seconds_in_minute.
const (
	averageDaysInYear  = 365.24 // Time_definitions.Average_days_in_year
	averageDaysInMonth = 30.42  // Time_definitions.Average_days_in_month
	daysInWeek         = 7      // Time_definitions.Days_in_week
	secondsPerDay      = 86400.0
)

// ToSeconds returns the length of the duration in seconds, its fraction
// included: BASE Iso8601_duration.to_seconds, which counts a year and a
// month at the Time_definitions averages (365.24 and 30.42 days), a week as
// 7 days and a day as 86400 seconds. A negative duration, openEHR's
// deviation from ISO 8601, has a negative length. Each component is
// converted to float64 before it is scaled, so a large count is not
// multiplied in int, and the length is exact within float64 precision.
func (p Duration) ToSeconds() float64 {
	secs := float64(p.Years)*averageDaysInYear*secondsPerDay +
		float64(p.Months)*averageDaysInMonth*secondsPerDay +
		definiteSeconds(p)
	if p.Negative {
		return -secs
	}
	return secs
}

// definiteSeconds is the length of the week, day and clock components in
// seconds.
func definiteSeconds(p Duration) float64 {
	return float64(p.Weeks)*daysInWeek*secondsPerDay +
		float64(p.Days)*secondsPerDay +
		float64(p.Hours)*3600 +
		float64(p.Minutes)*60 +
		float64(p.Seconds) + p.Fraction
}

// Magnitude returns the number of days from 0001-01-01 to the date. A
// month or a day the date leaves out counts as 1, so a reduced date stands
// for the first day of the period it names ("2004-05" for 2004-05-01).
func (p Date) Magnitude() int {
	m, d := p.Month, p.Day
	if !p.MonthKnown {
		m = 1
	}
	if !p.DayKnown {
		d = 1
	}
	return daysFromCivil(p.Year, m, d) - daysFromCivil(1, 1, 1)
}

// DateOfMagnitude returns the full date whose [Date.Magnitude] is days, the
// inverse of Magnitude on full dates.
func DateOfMagnitude(days int) Date {
	y, m, d := civilFromDays(days + daysFromCivil(1, 1, 1))
	return Date{Year: y, Month: m, Day: d, MonthKnown: true, DayKnown: true}
}

// Magnitude returns the seconds from the start of the day to the time, its
// fraction included. It is clock-local: the zone is not applied, so two
// times naming one instant in different zones differ. A minute or a second
// the time leaves out counts as 0.
func (p Time) Magnitude() float64 {
	return float64(p.Hour*3600+p.Minute*60+p.Second) + p.Fraction
}

// DateTimeMagnitude returns the seconds from 0001-01-01T00:00:00 to the
// date-time of date d and time t, which is zero when the date-time has no
// time. It is clock-local, as [Time.Magnitude] is, and reads a part the
// date or the time leaves out as they do.
func DateTimeMagnitude(d Date, t Time) float64 {
	return float64(d.Magnitude())*secondsPerDay + float64(t.Hour*3600+t.Minute*60+t.Second) + t.Fraction
}

// daysFromCivil returns the number of days since the Unix epoch
// (1970-01-01) for a proleptic-Gregorian date (Howard Hinnant's
// algorithm). It is a stable day index across the multi-millennium span
// where a time.Duration would overflow.
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

// civilFromDays is the inverse of daysFromCivil (Howard Hinnant's
// algorithm): the proleptic-Gregorian date of a day index since the Unix
// epoch.
func civilFromDays(z int) (y, m, d int) {
	z += 719468
	var era int
	if z >= 0 {
		era = z / 146097
	} else {
		era = (z - 146096) / 146097
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d = doy - (153*mp+2)/5 + 1
	m = mp + 3
	if mp >= 10 {
		m = mp - 9
	}
	y = yoe + era*400
	if m <= 2 {
		y++
	}
	return y, m, d
}
