package rminfo_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

func TestIsNonStorableAttr(t *testing.T) {
	cases := []struct {
		parentRM, attr string
		want           bool
	}{
		{"POINT_EVENT", "offset", true},
		{"INTERVAL_EVENT", "offset", true},
		{"DV_QUANTITY", "is_integral", true},
		{"DV_PROPORTION", "is_integral", true},
		{"POINT_EVENT", "time", false},
		{"DV_QUANTITY", "magnitude", false},
		{"OBSERVATION", "data", false},
	}
	for _, tc := range cases {
		t.Run(tc.parentRM+"."+tc.attr, func(t *testing.T) {
			if got := rminfo.IsNonStorableAttr(tc.parentRM, tc.attr); got != tc.want {
				t.Fatalf("IsNonStorableAttr(%q, %q) = %v, want %v", tc.parentRM, tc.attr, got, tc.want)
			}
		})
	}
}
