package constraints_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestREQ103TemporalRangeOrdersAcrossZone pins REQ-103 § Validate contract
// on a time or a date-time range whose bounds carry no zone, checked against
// a value with one, and the reverse. The two are ordered as XML Schema 1.0
// Part 2 §3.2.7.4 orders a dateTime with a zone against one without: the
// one with no zone read at +14:00 and at -14:00, a reduced value covering
// its whole period, and not ordered where the two readings disagree, an edge
// they share included. So a value more than 14 hours outside the range is
// refused, and one within 14 hours of a bound is not refused for it. The
// first rows are those of GitHub issue 467.
func TestREQ103TemporalRangeOrdersAcrossZone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		c        constraints.PrimitiveConstraint
		detail   string // Detail of the CodeOutOfRange violation
		admitted []string
		refused  []string
	}{
		{
			name:   "date-time, zoned value, bounds with no zone",
			c:      constraints.CDateTime{Range: closedSpan("2020-01-01T00:00", "2021-12-31T23:59")},
			detail: "value outside [2020-01-01T00:00..2021-12-31T23:59]",
			admitted: []string{
				"2021-06-01T00:00:00Z", "2022-01-01T09:00:00Z", "2022-01-01T13:00+02:00",
				"2022-01-01T14:00:00Z", "2019-12-31T10:00:00Z", "2019-12-31T09:59Z",
			},
			refused: []string{
				"2022-11-02T19:32:40Z", "2022-11-02T19:32:40+02:00", "2022-01-01T14:00:01Z",
				"2019-12-31T09:59:59Z", "2019-12-31T09:58Z", "2019-06-01T00:00Z",
			},
		},
		{
			name:     "date-time, value with no zone, bounds in UTC",
			c:        constraints.CDateTime{Range: closedSpan("2020-01-01T00:00Z", "2021-12-31T23:59Z")},
			detail:   "value outside [2020-01-01T00:00Z..2021-12-31T23:59Z]",
			admitted: []string{"2021-06-01T00:00:00", "2022-01-01T14:00", "2022-01-01", "2021", "2019-12-31T09:59", "2019-12-31"},
			refused:  []string{"2022-11-02T19:32:40", "2022-01-01T14:00:01", "2022-01-02", "2019-12-31T09:58:59", "2019-06-01T00:00", "2019-12-30"},
		},
		{
			// A reduced bound covers its whole period: the year 2030 holds
			// 2030-06-01T10:00 at every reading, with a zone or without.
			name:     "date-time, up to the year 2030 with no zone",
			c:        constraints.CDateTime{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "2030", UpperInclusive: true}},
			detail:   "value outside (*..2030]",
			admitted: []string{"2030-06-01T10:00Z", "2030-06-01T10:00", "2030-12-31T23:59:59-12:00", "2031-01-01T14:00:00Z"},
			refused:  []string{"2031-01-01T14:00:01Z", "2031-06-01T10:00Z", "2031-01-01T00:00", "2031"},
		},
		{
			// A value with a zone inside the year 2030 at both readings is
			// equal to the bound, as its twin with no zone is at the
			// precision the two share, so the excluded bound refuses both.
			name:    "date-time, below the year 2030 with no zone",
			c:       constraints.CDateTime{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "2030"}},
			detail:  "value outside (*..2030)",
			refused: []string{"2030-06-01T10:00Z", "2030-06-01T10:00"},
		},
		{
			name:     "time, up to 02:00 in UTC",
			c:        constraints.CTime{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "02:00:00Z", UpperInclusive: true}},
			detail:   "value outside (*..02:00:00Z]",
			admitted: []string{"01:00:00", "15:00:00", "16:00:00", "15:59", "16"},
			refused:  []string{"16:00:01", "20:00:00", "17"},
		},
		{
			name:     "time, from 20:00 with no zone",
			c:        constraints.CTime{Range: constraints.TemporalRange{Lower: "20:00:00", LowerInclusive: true, UpperUnbounded: true}},
			detail:   "value outside [20:00:00..*)",
			admitted: []string{"10:00Z", "06:00:00Z", "08:00:00+02:00", "05:59Z", "05Z"},
			refused:  []string{"05:59:59Z", "02:00:00Z", "02:00:00+02:00", "05:58Z", "04Z"},
		},
		{
			// The bound covers the hour 10 to its end: 23:00-02:00, 01:00
			// the next day in UTC, is 14 hours after it.
			name:     "time, up to the hour 10 with no zone",
			c:        constraints.CTime{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "10", UpperInclusive: true}},
			detail:   "value outside (*..10]",
			admitted: []string{"10:59:59Z", "11:30+02:00", "23:00:00-02:00"},
			refused:  []string{"23:00:01-02:00", "23:30-03:00"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.admitted {
				if got := tc.c.Validate(s); len(got) != 0 {
					t.Errorf("Validate(%q) = %+v, want no violation", s, got)
				}
			}
			for _, s := range tc.refused {
				want := []constraints.Violation{{Code: constraints.CodeOutOfRange, Detail: tc.detail, Value: constraints.Redact(s)}}
				if got := tc.c.Validate(s); !slices.Equal(got, want) {
					t.Errorf("Validate(%q) = %+v, want %+v", s, got, want)
				}
			}
		})
	}
}
