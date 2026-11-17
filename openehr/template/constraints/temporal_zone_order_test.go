package constraints_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestREQ103TemporalRangeOrdersAcrossZone pins REQ-103 § Validate contract
// on a date-time range whose bounds carry no zone, checked against a value
// with one, and the reverse: a value more than 14 hours outside the range
// is refused, as XML Schema 1.0 Part 2 §3.2.7.4 orders a dateTime with a
// zone against one without. The rows are those of GitHub issue 467.
func TestREQ103TemporalRangeOrdersAcrossZone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		c       constraints.PrimitiveConstraint
		detail  string
		refused []string
	}{
		{
			name:    "date-time, zoned value, bounds with no zone",
			c:       constraints.CDateTime{Range: closedSpan("2020-01-01T00:00", "2021-12-31T23:59")},
			detail:  "value outside [2020-01-01T00:00..2021-12-31T23:59]",
			refused: []string{"2022-11-02T19:32:40Z", "2022-11-02T19:32:40+02:00"},
		},
		{
			name:    "date-time, value with no zone, bounds in UTC",
			c:       constraints.CDateTime{Range: closedSpan("2020-01-01T00:00Z", "2021-12-31T23:59Z")},
			detail:  "value outside [2020-01-01T00:00Z..2021-12-31T23:59Z]",
			refused: []string{"2022-11-02T19:32:40"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.refused {
				want := []constraints.Violation{{Code: constraints.CodeOutOfRange, Detail: tc.detail, Value: constraints.Redact(s)}}
				if got := tc.c.Validate(s); !slices.Equal(got, want) {
					t.Errorf("Validate(%q) = %+v, want %+v", s, got, want)
				}
			}
		})
	}
}
