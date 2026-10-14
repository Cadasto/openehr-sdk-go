package validation_test

// rmfloor_interval_class_test.go: REQ-112 Interval flags, REQ-171 Finding
// identity. The flag rules are BASE Interval's, so the floor checks them on
// every class that inherits them, a Point_interval and a Proper_interval root
// included, and the Detail names the class of the interval it reports on.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestValidateRM_REQ112_IntervalFlagsNameTheClass — a lower side marked both
// unbounded and included is reported as lower_included_valid, with the rule
// Interval.Lower_included_valid, and the Detail names the root's own class:
// Point_interval, Proper_interval, or DV_INTERVAL without its generic
// parameter. It names no other interval class.
func TestValidateRM_REQ112_IntervalFlagsNameTheClass(t *testing.T) {
	t.Parallel()
	open := rm.Interval[any]{LowerUnbounded: true, LowerIncluded: true}
	for _, tc := range []struct {
		name  string
		root  any
		class string
	}{
		{name: "Point_interval", root: &rm.PointInterval[any]{LowerUnbounded: true, LowerIncluded: true}, class: "Point_interval"},
		{name: "Proper_interval", root: &rm.ProperInterval[any]{Interval: open}, class: "Proper_interval"},
		{name: "DV_INTERVAL<DV_QUANTITY>", root: &rm.DVInterval[rm.DVQuantity]{Interval: rm.Interval[rm.DVQuantity]{LowerUnbounded: true, LowerIncluded: true}}, class: "DV_INTERVAL"},
	} {
		var flags []validation.Issue
		for _, i := range validation.ValidateRM(tc.root).Issues {
			if i.Code == "lower_included_valid" {
				flags = append(flags, i)
			}
		}
		if len(flags) != 1 {
			t.Errorf("ValidateRM(%s with an included unbounded lower side) reports %d lower_included_valid issues, want 1", tc.name, len(flags))
			continue
		}
		i := flags[0]
		if i.Rule != "Interval.Lower_included_valid" || i.Path != "/" {
			t.Errorf("ValidateRM(%s) lower_included_valid at %q with rule %q, want / and Interval.Lower_included_valid", tc.name, i.Path, i.Rule)
		}
		if !strings.HasPrefix(i.Detail, tc.class+".lower_included ") {
			t.Errorf("ValidateRM(%s) Detail = %q, want it to name %s.lower_included", tc.name, i.Detail, tc.class)
		}
		for _, other := range []string{"Point_interval", "Proper_interval", "DV_INTERVAL", "<"} {
			if other != tc.class && strings.Contains(i.Detail, other) {
				t.Errorf("ValidateRM(%s) Detail = %q, names %s", tc.name, i.Detail, other)
			}
		}
	}
}
