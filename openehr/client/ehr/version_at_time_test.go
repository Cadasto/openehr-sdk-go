package ehr_test

import (
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
)

// REQ-174: a version_at_time value is the instant in UTC with every
// fractional digit the time carries, trailing zeros dropped and no
// fraction for a whole second.
func TestFormatVersionAtTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "whole second has no fraction",
			in:   time.Date(2026, 5, 17, 8, 0, 0, 0, time.UTC),
			want: "2026-05-17T08:00:00Z",
		},
		{
			name: "pin example keeps its milliseconds",
			in:   time.Date(2015, 1, 20, 19, 30, 22, 765_000_000, time.FixedZone("", 3600)),
			want: "2015-01-20T18:30:22.765Z",
		},
		{
			name: "one nanosecond is kept",
			in:   time.Date(2015, 1, 20, 18, 30, 22, 1, time.UTC),
			want: "2015-01-20T18:30:22.000000001Z",
		},
		{
			name: "trailing zeros dropped",
			in:   time.Date(2015, 1, 20, 18, 30, 22, 500_000_000, time.UTC),
			want: "2015-01-20T18:30:22.5Z",
		},
		{
			name: "non-UTC zone converted to Z",
			in:   time.Date(2026, 5, 16, 22, 30, 0, 0, time.FixedZone("", -5*3600)),
			want: "2026-05-17T03:30:00Z",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := openehrclient.FormatVersionAtTime(tc.in); got != tc.want {
				t.Errorf("FormatVersionAtTime(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
