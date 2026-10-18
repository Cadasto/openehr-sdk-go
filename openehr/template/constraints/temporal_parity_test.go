package constraints_test

// temporal_parity_test.go: REQ-103 and REQ-123. The template temporal
// validators and the RM's temporal value parse judge the same ISO 8601 text,
// so a value the template accepts is one the RM floor accepts, and the other
// way round. For each sample this holds CDate, CTime, CDateTime and CDuration
// Validate to accept exactly when the matching rm.DV*.ValidISO8601 is true.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

func TestREQ103TemporalValidateAgreesWithREQ123Parse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		validate func(any) []constraints.Violation
		rmValid  func(string) bool
		samples  []string
	}{
		{
			name:     "CDate",
			validate: constraints.CDate{}.Validate,
			rmValid:  func(s string) bool { return (&rm.DVDate{Value: s}).ValidISO8601() },
			samples: []string{
				"2026-05-24", "2026-05", "2026", "20260524", "202605", "2024-02-29",
				"", "2026-13-01", "2026-02-30", "2023-02-29", "2026-5-24", "26-05-24",
				"2026-05-24T10:00", "not-a-date", "２０２６", "+2026", " 2026",
			},
		},
		{
			name:     "CTime",
			validate: constraints.CTime{}.Validate,
			rmValid:  func(s string) bool { return (&rm.DVTime{Value: s}).ValidISO8601() },
			samples: []string{
				"10:30:00", "10:30", "10", "103000", "1030", "10:30:00.5", "10:30:00,5",
				"10:30:00Z", "10:30:00+01:00", "10:30:00+0100", "10:30:00+01", "10:30:00-12:00",
				"10:30:00.123456789Z", "103000,5+0100", "23:59:60",
				"", "25:00", "24:00:00", "T10:00", "10:30:00.+01", "10:30:00,+01", "1:30",
				"10:30:00+15", "10:30.5", "10:30:00.", "noon",
			},
		},
		{
			name:     "CDateTime",
			validate: constraints.CDateTime{}.Validate,
			rmValid:  func(s string) bool { return (&rm.DVDateTime{Value: s}).ValidISO8601() },
			samples: []string{
				"2026-05-24T10:30:00Z", "2026-05-24T10:30:00", "2026-05-24T10:30", "2026-05-24T10",
				"2026-05-24", "2026-05", "2026", "20260524", "202605", "20251024T121033",
				"20260524T103000Z", "2026-05-24T10:30:00+0100", "2026-05-24T10:30:00.123+01:00",
				"2026-05-24T10:30:00,5+01", "20260524T103000+01:00",
				"", "2026-13-01", "2026-13-01T10:00", "2026-05-24T25:00", "T10:00", "2026-05-24T",
				"2026-05T10:30", "2026-05-24T103000", "20260524T10:30:00", "2026-05-24 10:30:00",
				"2026-05-24T10:30:00.+01",
			},
		},
		{
			name:     "CDuration",
			validate: constraints.CDuration{}.Validate,
			rmValid:  func(s string) bool { return (&rm.DVDuration{Value: s}).ValidISO8601() },
			samples: []string{
				"P1Y2M3DT4H5M6S", "PT5M", "P0D", "P1W", "P1Y2W3D", "-P1D", "PT1,5S", "PT1.5S",
				"-PT0S", "PT36H",
				"", "P", "PT", "P1DT", "P1YT", "P1Y2MT", "P1.5D", "PT1.S", "P1D1Y", "1D",
				"P-1D", "P1M1M", "not-iso", "pt1s",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var accepted, refused int
			for _, s := range tc.samples {
				want := tc.rmValid(s)
				got := len(tc.validate(s)) == 0
				if got != want {
					t.Errorf("%s{}.Validate(%q) accepts = %t, but the RM's ValidISO8601 = %t", tc.name, s, got, want)
				}
				if want {
					accepted++
				} else {
					refused++
				}
			}
			// A table the RM wholly accepts or wholly refuses cannot tell a
			// validator that agrees from one that always says the same.
			if accepted == 0 || refused == 0 {
				t.Errorf("%s samples: the RM accepts %d and refuses %d, want at least one of each", tc.name, accepted, refused)
			}
		})
	}
}
