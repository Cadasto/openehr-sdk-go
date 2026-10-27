package expiresin_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/expiresin"
)

// TestREQ063_SecondsReadsEveryJSONNumberForm pins REQ-063 (auth.md § Token
// refresh) at the reader of expires_in: every number, an integer included,
// is rounded down and bounded by Max either way, so its conversion to int64
// and then to a time.Duration is defined, a number too large for a float64
// counting as the bound on its side, and what is not a number, NaN and Inf
// included, is refused.
func TestREQ063_SecondsReadsEveryJSONNumberForm(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in     string
		want   int64
		wantOK bool
	}{
		{in: "3600", want: 3600, wantOK: true},
		{in: "3600.0", want: 3600, wantOK: true},
		{in: "3.6e3", want: 3600, wantOK: true},
		{in: "3600.5", want: 3600, wantOK: true},
		{in: "3153600000", want: expiresin.Max, wantOK: true},
		{in: "3153600001", want: expiresin.Max, wantOK: true},
		{in: "9223372036854775807", want: expiresin.Max, wantOK: true},
		{in: "9223372036854775808", want: expiresin.Max, wantOK: true},
		{in: "1e300", want: expiresin.Max, wantOK: true},
		{in: "-5", want: -5, wantOK: true},
		{in: "-0.5", want: -1, wantOK: true},
		{in: "-3153600001", want: -expiresin.Max, wantOK: true},
		{in: "-9223372036854775808", want: -expiresin.Max, wantOK: true},
		{in: "-1e300", want: -expiresin.Max, wantOK: true},
		{in: "1e400", want: expiresin.Max, wantOK: true},
		{in: "-1e400", want: -expiresin.Max, wantOK: true},
		{in: "NaN"},
		{in: "Inf"},
		{in: "-Inf"},
		{in: ""},
		{in: "soon"},
	}
	for _, tc := range tests {
		got, ok := expiresin.Seconds(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("Seconds(%q) = (%d, %t), want (%d, %t)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}
