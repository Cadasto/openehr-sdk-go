package clientcreds

import (
	"math"
	"testing"
)

// TestREQ063_ExpiresInSecondsReadsEveryJSONNumberForm pins REQ-063 (auth.md §
// Token refresh) at the helper that reads expires_in: an integer is kept as
// sent, any other number is rounded down and bounded by maxExpiresIn either
// way, so its conversion to int64 is defined, and what is not a finite
// number, NaN and ±Inf included, is refused.
func TestREQ063_ExpiresInSecondsReadsEveryJSONNumberForm(t *testing.T) {
	tests := []struct {
		in     string
		want   int64
		wantOK bool
	}{
		{in: "3600", want: 3600, wantOK: true},
		{in: "3600.0", want: 3600, wantOK: true},
		{in: "3.6e3", want: 3600, wantOK: true},
		{in: "3600.5", want: 3600, wantOK: true},
		{in: "9223372036854775807", want: math.MaxInt64, wantOK: true},
		{in: "9223372036854775808", want: maxExpiresIn, wantOK: true},
		{in: "1e300", want: maxExpiresIn, wantOK: true},
		{in: "-5", want: -5, wantOK: true},
		{in: "-0.5", want: -1, wantOK: true},
		{in: "-1e300", want: -maxExpiresIn, wantOK: true},
		{in: "1e400"},
		{in: "NaN"},
		{in: "Inf"},
		{in: "-Inf"},
		{in: ""},
		{in: "soon"},
	}
	for _, tc := range tests {
		got, ok := expiresInSeconds(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("expiresInSeconds(%q) = (%d, %t), want (%d, %t)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}
