package constraints_test

// temporal_validate_test.go: REQ-103 and REQ-123. Each temporal validator's
// outcome for a fixed table of ISO 8601 text: nil for a form the RM's
// temporal parse accepts, one CodeInvalidValue violation with the validator's
// Detail text and the input as Value for any other string, and one
// CodeWrongType violation with no Value for a value that is not a string.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

func TestREQ103TemporalValidateOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		validate  func(any) []constraints.Violation
		detail    string // Detail of the CodeInvalidValue violation
		wrongType string // Detail of the CodeWrongType violation for the int 42
		accepted  []string
		refused   []string
	}{
		{
			name:      "CDate",
			validate:  constraints.CDate{}.Validate,
			detail:    "value is not a valid ISO 8601 date",
			wrongType: "expected string date, got int",
			accepted:  []string{"2026-05-24", "2026-05", "2026", "20260524", "202605", "2024-02-29"},
			refused: []string{
				"", "2026-13-01", "2026-02-30", "2023-02-29", "2026-5-24", "26-05-24",
				"2026-05-24T10:00", "not-a-date", "２０２６",
			},
		},
		{
			name:      "CTime",
			validate:  constraints.CTime{}.Validate,
			detail:    "value is not a valid ISO 8601 time",
			wrongType: "expected string time, got int",
			accepted: []string{
				"10:30:00", "10:30", "10", "103000", "1030", "10:30:00.5", "10:30:00,5",
				"10:30:00Z", "10:30:00+01:00", "10:30:00+0100", "10:30:00+01", "10:30:00-12:00",
				"10:30:00.123456789Z", "103000,5+0100", "23:59:60",
			},
			refused: []string{
				"", "25:00", "24:00:00", "T10:00", "10:30:00.+01", "10:30:00,+01", "1:30",
				"10:30:00+15", "10:30.5", "10:30:00.", "noon",
			},
		},
		{
			name:      "CDateTime",
			validate:  constraints.CDateTime{}.Validate,
			detail:    "value is not a valid ISO 8601 date-time",
			wrongType: "expected string date-time, got int",
			accepted: []string{
				"2026-05-24T10:30:00Z", "2026-05-24T10:30:00", "2026-05-24T10:30", "2026-05-24T10",
				"2026-05-24", "2026-05", "2026", "20260524", "202605", "20251024T121033",
				"20260524T103000Z", "2026-05-24T10:30:00+0100", "2026-05-24T10:30:00,5+01",
				"20260524T103000+01:00",
			},
			refused: []string{
				"", "2026-13-01", "2026-13-01T10:00", "2026-05-24T25:00", "T10:00", "2026-05-24T",
				"2026-05T10:30", "2026-05-24T103000", "20260524T10:30:00", "2026-05-24 10:30:00",
				"2026-05-24T10:30:00.+01",
			},
		},
		{
			name:      "CDuration",
			validate:  constraints.CDuration{}.Validate,
			detail:    "value is not a valid ISO 8601 duration",
			wrongType: "expected string duration, got int",
			accepted: []string{
				"P1Y2M3DT4H5M6S", "PT5M", "P0D", "P1W", "P1Y2W3D", "-P1D", "PT1,5S", "PT1.5S",
				"-PT0S", "PT36H",
			},
			refused: []string{
				"", "P", "PT", "P1DT", "P1YT", "P1Y2MT", "P1.5D", "PT1.S", "P1D1Y", "1D",
				"P-1D", "P1M1M", "not-iso", "pt1s",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.accepted {
				if got := tc.validate(s); len(got) != 0 {
					t.Errorf("%s{}.Validate(%q) = %+v, want no violation", tc.name, s, got)
				}
			}
			for _, s := range tc.refused {
				want := []constraints.Violation{{Code: constraints.CodeInvalidValue, Detail: tc.detail, Value: constraints.Redact(s)}}
				if got := tc.validate(s); !slices.Equal(got, want) {
					t.Errorf("%s{}.Validate(%q) = %+v, want %+v", tc.name, s, got, want)
				}
			}
			want := []constraints.Violation{{Code: constraints.CodeWrongType, Detail: tc.wrongType}}
			if got := tc.validate(42); !slices.Equal(got, want) {
				t.Errorf("%s{}.Validate(42) = %+v, want %+v", tc.name, got, want)
			}
		})
	}
}
