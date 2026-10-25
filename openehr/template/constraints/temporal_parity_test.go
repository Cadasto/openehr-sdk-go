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

// TestREQ103TemporalRangeAgreesWithREQ123Order holds a TemporalRange to the
// order of the RM's Compare (REQ-123), which REQ-103 § Validate contract
// names: for each pair of samples, a range from the second, included,
// admits the first exactly when Compare orders it at or after the second; a
// range from the second, excluded, exactly when after; and a range up to the
// second, included, exactly when at or before. The samples mix full and
// partial values, both layouts, zones and the to_seconds averages of a year
// and a month.
func TestREQ103TemporalRangeAgreesWithREQ123Order(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		validate func(constraints.TemporalRange, string) bool
		compare  func(a, b string) int
		samples  []string
	}{
		{
			name: "CDate",
			validate: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CDate{Range: r}.Validate(s)) == 0
			},
			compare: func(a, b string) int { return (&rm.DVDate{Value: a}).Compare(rm.DVDate{Value: b}) },
			samples: []string{
				"2004", "2004-05", "2004-05-01", "2004-05-20", "20040521", "200406", "2005-01-01",
				"0001-01-01", "2024-02-29",
			},
		},
		{
			name: "CTime",
			validate: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CTime{Range: r}.Validate(s)) == 0
			},
			compare: func(a, b string) int { return (&rm.DVTime{Value: a}).Compare(rm.DVTime{Value: b}) },
			samples: []string{
				"09", "09:30", "09:30:00", "09:30:00.5", "09:30:00+05:00", "09:00:00-05:00", "0930",
				"23:59:60", "00:00:00Z", "14:00Z",
			},
		},
		{
			name: "CDateTime",
			validate: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CDateTime{Range: r}.Validate(s)) == 0
			},
			compare: func(a, b string) int { return (&rm.DVDateTime{Value: a}).Compare(rm.DVDateTime{Value: b}) },
			samples: []string{
				"2004-05-20", "2004-05-20T00", "2004-05-20T00:00:00", "2004-05-20T10:00:00+02:00",
				"2004-05-20T09:00:00Z", "20040520T100000", "2004-05", "2004-05-20T23:59:59.9", "2004",
			},
		},
		{
			name: "CDuration",
			validate: func(r constraints.TemporalRange, s string) bool {
				return len(constraints.CDuration{Range: r}.Validate(s)) == 0
			},
			compare: func(a, b string) int { return (&rm.DVDuration{Value: a}).Compare(rm.DVDuration{Value: b}) },
			samples: []string{
				"P0D", "PT24H", "P1D", "P1Y", "P12M", "P365D", "P366D", "P52W", "-P1D", "PT0.5S",
				"P4Y", "P48M", "P1461D", "-PT0S",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var admitted, refused int
			for _, b := range tc.samples {
				from := constraints.TemporalRange{Lower: b, LowerInclusive: true, UpperUnbounded: true}
				after := constraints.TemporalRange{Lower: b, UpperUnbounded: true}
				upTo := constraints.TemporalRange{LowerUnbounded: true, Upper: b, UpperInclusive: true}
				for _, a := range tc.samples {
					order := tc.compare(a, b)
					for _, check := range []struct {
						r    constraints.TemporalRange
						want bool
					}{{from, order >= 0}, {after, order > 0}, {upTo, order <= 0}} {
						got := tc.validate(check.r, a)
						if got != check.want {
							t.Errorf("%s{Range: %+v}.Validate(%q) admits = %t, but the RM's Compare(%q, %q) = %d", tc.name, check.r, a, got, a, b, order)
						}
						if got {
							admitted++
						} else {
							refused++
						}
					}
				}
			}
			if admitted == 0 || refused == 0 {
				t.Errorf("%s samples: the ranges admit %d and refuse %d, want at least one of each", tc.name, admitted, refused)
			}
		})
	}
}
