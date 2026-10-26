package ehr_test

import (
	"errors"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// REQ-174: a version_at_time value is the instant in UTC with every
// fractional digit the time carries, trailing zeros dropped and no
// fraction for a whole second. RFC 3339 writes the years 0000 to 9999
// only, so a time whose UTC year falls outside them is refused with
// ErrInvalidConfig and no value.
func TestFormatVersionAtTime(t *testing.T) {
	tests := []struct {
		name    string
		in      time.Time
		want    string
		wantErr error
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
		{
			name: "first instant of year 0000 accepted",
			in:   time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC),
			want: "0000-01-01T00:00:00Z",
		},
		{
			name: "last instant of year 9999 accepted",
			in:   time.Date(9999, 12, 31, 23, 59, 59, 999_999_999, time.UTC),
			want: "9999-12-31T23:59:59.999999999Z",
		},
		{
			name: "local year 10000 that is 9999 in UTC accepted",
			in:   time.Date(10000, 1, 1, 3, 0, 0, 0, time.FixedZone("", 5*3600)),
			want: "9999-12-31T22:00:00Z",
		},
		{
			name:    "local year 9999 that is 10000 in UTC refused",
			in:      time.Date(9999, 12, 31, 23, 59, 59, 0, time.FixedZone("", -5*3600)),
			wantErr: transport.ErrInvalidConfig,
		},
		{
			name:    "year -1 refused",
			in:      time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC),
			wantErr: transport.ErrInvalidConfig,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := openehrclient.FormatVersionAtTime(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("FormatVersionAtTime(%v) err = %v, want %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("FormatVersionAtTime(%v) = %q, want %q", tc.in, got, tc.want)
			}
			if tc.wantErr != nil {
				return
			}
			back, err := time.Parse(time.RFC3339Nano, got)
			if err != nil {
				t.Fatalf("FormatVersionAtTime(%v) = %q, which RFC 3339 cannot read: %v", tc.in, got, err)
			}
			if !back.Equal(tc.in) {
				t.Errorf("FormatVersionAtTime(%v) = %q, which reads back as %v, another instant", tc.in, got, back)
			}
		})
	}
}
