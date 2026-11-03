package constraints_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// boundedBy returns the TemporalRange from lower to upper, an empty bound
// leaving its side unbounded and lowerIn and upperIn including a bounded
// side.
func boundedBy(lower string, lowerIn bool, upper string, upperIn bool) constraints.TemporalRange {
	return constraints.TemporalRange{
		Lower: lower, LowerInclusive: lower != "" && lowerIn, LowerUnbounded: lower == "",
		Upper: upper, UpperInclusive: upper != "" && upperIn, UpperUnbounded: upper == "",
	}
}

// checkRangeOutcome reports whether c judges value as want: no violation when
// want is true, else one CodeOutOfRange.
func checkRangeOutcome(t *testing.T, c constraints.PrimitiveConstraint, value string, want bool) {
	t.Helper()
	got := c.Validate(value)
	switch {
	case want && len(got) != 0:
		t.Errorf("%#v.Validate(%q) = %+v, want no violation", c, value, got)
	case !want && (len(got) != 1 || got[0].Code != constraints.CodeOutOfRange):
		t.Errorf("%#v.Validate(%q) = %+v, want one CodeOutOfRange", c, value, got)
	}
}

// TestREQ103TemporalRangeSharedPrecision pins REQ-103 § Validate contract
// on a date, time or date-time range: a value and a bound written with one
// zone designator, or both with none, are compared at the precision they
// share, field by field from the largest down, the second with its fraction
// as one field. Equal on every shared field is equal, so a bound covers the
// whole period it names: 2021-05 lies within 1900..2021 and outside
// 1900..<2021, and 2021 within 2021-06.. and outside ..<2021-06. Where both
// carry a zone and the designators differ, the two are still ordered as
// instants, a partial value standing for the start of its period. The rows
// are those of GitHub issue 370, on each type that can carry them.
func TestREQ103TemporalRangeSharedPrecision(t *testing.T) {
	t.Parallel()
	dates := func(r constraints.TemporalRange) []constraints.PrimitiveConstraint {
		return []constraints.PrimitiveConstraint{constraints.CDate{Range: r}, constraints.CDateTime{Range: r}}
	}
	tests := []struct {
		name     string
		cs       []constraints.PrimitiveConstraint
		admitted []string
		refused  []string
	}{
		{
			name:     "1900..2021",
			cs:       dates(boundedBy("1900", true, "2021", true)),
			admitted: []string{"2021-05", "2021-12-31", "2021", "1900", "1900-01-01"},
			refused:  []string{"2022-01", "2022", "1899-12-31"},
		},
		{
			name:     "1900..<2021",
			cs:       dates(boundedBy("1900", true, "2021", false)),
			admitted: []string{"2020-12-31", "2020"},
			refused:  []string{"2021-05", "2021", "2021-01-01"},
		},
		{
			name:     "2021-05-15..",
			cs:       dates(boundedBy("2021-05-15", true, "", false)),
			admitted: []string{"2021-05", "2021", "2021-05-15", "2021-06"},
			refused:  []string{"2021-04", "2021-05-14", "2020"},
		},
		{
			name:     "2021-06..",
			cs:       dates(boundedBy("2021-06", true, "", false)),
			admitted: []string{"2021", "2021-06-30"},
			refused:  []string{"2021-05", "2020"},
		},
		{
			name:     "..<2021-06",
			cs:       dates(boundedBy("", false, "2021-06", false)),
			admitted: []string{"2021-05-31", "2020"},
			refused:  []string{"2021", "2021-06", "2021-06-01"},
		},
		{
			name:     "..2021-12-31T23:59",
			cs:       []constraints.PrimitiveConstraint{constraints.CDateTime{Range: boundedBy("", false, "2021-12-31T23:59", true)}},
			admitted: []string{"2021-12-31T23:59:30", "2021-12-31T23:59:59.999", "2021-12-31", "2021", "2021-12-31T23"},
			refused:  []string{"2022-01-01T00:00", "2022"},
		},
		{
			name:     "date-time, ..<2021-12-31T23:59:30.5",
			cs:       []constraints.PrimitiveConstraint{constraints.CDateTime{Range: boundedBy("", false, "2021-12-31T23:59:30.5", false)}},
			admitted: []string{"2021-12-31T23:59:30", "2021-12-31T23:59:30.25"},
			refused:  []string{"2021-12-31T23:59", "2021-12-31T23:59:30.5", "2021-12-31T23:59:31"},
		},
		{
			name:     "time, ..10",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("", false, "10", true)}},
			admitted: []string{"10:30", "10:59:59.999", "10", "09:59"},
			refused:  []string{"11", "11:00:00"},
		},
		{
			name:     "time, 08..10",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("08", true, "10", true)}},
			admitted: []string{"10:30", "08", "08:00:00.0"},
			refused:  []string{"11:00", "07:59:59"},
		},
		{
			name:     "time, 08..<10",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("08", true, "10", false)}},
			admitted: []string{"09:59:59.999"},
			refused:  []string{"10:30", "10"},
		},
		{
			name:     "time, 10:15..",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("10:15", true, "", false)}},
			admitted: []string{"10", "10:15:00.0", "10:15:59"},
			refused:  []string{"10:14:59", "09"},
		},
		{
			name:     "time, 10:30..",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("10:30", true, "", false)}},
			admitted: []string{"10"},
			refused:  []string{"09", "10:29"},
		},
		{
			name:     "time, ..<10:30",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("", false, "10:30", false)}},
			admitted: []string{"10:29:59.999", "09"},
			refused:  []string{"10", "10:30:15"},
		},
		{
			// One designator: the shared precision. Another: instants, the
			// bound 10+02:00 standing for 08:00 in UTC.
			name:     "time, ..10+02:00",
			cs:       []constraints.PrimitiveConstraint{constraints.CTime{Range: boundedBy("", false, "10+02:00", true)}},
			admitted: []string{"10:30+02:00", "10:59:59+02:00", "07:59Z", "08:00Z", "11:30"},
			refused:  []string{"11+02:00", "08:30Z", "12:00:01+04:00"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, c := range tc.cs {
				for _, s := range tc.admitted {
					checkRangeOutcome(t, c, s, true)
				}
				for _, s := range tc.refused {
					checkRangeOutcome(t, c, s, false)
				}
			}
		})
	}
}

// TestREQ103TemporalRangeVeredictumCases pins REQ-103 § Validate contract
// on the 46 rows of four range cases of the Veredictum 0.4.0 conformance
// catalogue, each judged as the catalogue expects: every row of an accepted
// case lies within its range, and every row of a c_*_range case outside it.
// The catalogue's recipe includes each bounded side and leaves a side with
// no bound unbounded (an empty bound here). The literals are copied from its
// rows files.
func TestREQ103TemporalRangeVeredictumCases(t *testing.T) {
	t.Parallel()
	const (
		dateAccepted     = "dv_date/dv_date-validate_range-accepted"
		dateRefused      = "dv_date/dv_date-validate_range-c_date_range"
		dateTimeAccepted = "dv_date_time/dv_date_time-validate_range-accepted"
		dateTimeRefused  = "dv_date_time/dv_date_time-validate_range-c_date_time_range"
	)
	rows := []struct {
		workflow            string
		value, lower, upper string
	}{
		{dateAccepted, "2021", "1900", "2030"},
		{dateAccepted, "2021-10", "1900-03", "2030-07"},
		{dateAccepted, "2021-10-24", "1900-03-13", "2030-07-09"},

		{dateRefused, "2021", "2022", "2030"},
		{dateRefused, "2021", "1900", "2020"},
		{dateRefused, "2021-10", "2022-03", "2030-07"},
		{dateRefused, "2021-10", "1900-03", "2020-07"},
		{dateRefused, "2021-10-24", "2022-03-13", "2030-07-09"},
		{dateRefused, "2021-10-24", "1900-03-13", "2020-07-09"},

		{dateTimeAccepted, "2021", "1900", "2030"},
		{dateTimeAccepted, "2021-10", "1900-03", "2030-07"},
		{dateTimeAccepted, "2021-10-24", "1900-03-13", "2030-07-09"},
		{dateTimeAccepted, "2021-05", "1900", "2030"},
		{dateTimeAccepted, "2021-05", "1900", "2021"},
		{dateTimeAccepted, "2021", "2020-07", "2022-03"},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00", "1900-03-13T23"},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00:00", "1900-03-13T23:59"},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00:00:00", "1900-03-13T23:59:59"},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00:00:00.0", "1900-03-13T23:59:59.999"},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00", ""},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00:00", ""},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00:00:00", ""},
		{dateTimeAccepted, "1900-03-13T10", "1900-03-13T00:00:00.0", ""},

		{dateTimeRefused, "2021", "2022", "2030"},
		{dateTimeRefused, "2021", "1900", "2020"},
		{dateTimeRefused, "2021-10", "2022-03", "2030-07"},
		{dateTimeRefused, "2021-10", "1900-03", "2020-07"},
		{dateTimeRefused, "2021-10-24", "2022-03-13", "2030-07-09"},
		{dateTimeRefused, "2021-10-24", "1900-03-13", "2020-07-09"},
		{dateTimeRefused, "2021-05", "2022", "2030"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11", "1900-03-13T23"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11:00", "1900-03-13T23:59"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11:00:00", "1900-03-13T23:59:59"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11:00:00.0", "1900-03-13T23:59:59.999"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T00", "1900-03-13T09"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T00:00", "1900-03-13T09:59"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T00:00:00", "1900-03-13T09:59:59"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T00:00:00.0", "1900-03-13T09:59:59.999"},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11", ""},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11:00", ""},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11:00:00", ""},
		{dateTimeRefused, "1900-03-13T10", "1900-03-13T11:00:00.0", ""},
		{dateTimeRefused, "1900-03-13T10", "", "1900-03-13T09"},
		{dateTimeRefused, "1900-03-13T10", "", "1900-03-13T09:59"},
		{dateTimeRefused, "1900-03-13T10", "", "1900-03-13T09:59:59"},
		{dateTimeRefused, "1900-03-13T10", "", "1900-03-13T09:59:59.999"},
	}
	if len(rows) != 46 {
		t.Fatalf("the table holds %d rows, want the 46 of the four cases", len(rows))
	}
	for _, row := range rows {
		r := boundedBy(row.lower, true, row.upper, true)
		var c constraints.PrimitiveConstraint = constraints.CDateTime{Range: r}
		if row.workflow == dateAccepted || row.workflow == dateRefused {
			c = constraints.CDate{Range: r}
		}
		inside := row.workflow == dateAccepted || row.workflow == dateTimeAccepted
		t.Run(row.workflow+" "+row.value+" in "+r.String(), func(t *testing.T) {
			t.Parallel()
			checkRangeOutcome(t, c, row.value, inside)
		})
	}
}
