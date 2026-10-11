package validation_test

// rmfloor_interval_flags_test.go: REQ-112 Interval flags. BASE Interval's
// Lower_included_valid and Upper_included_valid say that an unbounded side is
// not included. The floor reports a side marked both, on every interval the
// walk reaches, at the interval's own path.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// intervalSide is how one side of a test interval is flagged.
type intervalSide int

const (
	// closedSide carries a bound and includes it.
	closedSide intervalSide = iota
	// openSide is unbounded and not included, the one value BASE allows.
	openSide
	// openIncludedSide is unbounded and included, which BASE forbids.
	openIncludedSide
)

// quantityEnd returns the bound and flags of one side of a concrete-typed
// interval. An open side holds the zero DV_QUANTITY, the only empty bound the
// concrete type can spell.
func quantityEnd(s intervalSide, magnitude rm.Real) (bound rm.DVQuantity, unbounded, included bool) {
	if s == closedSide {
		bound = rm.DVQuantity{Magnitude: magnitude, Units: "mm"}
	}
	return bound, s != closedSide, s != openSide
}

// quantityInterval builds a concrete-typed interval with the given sides.
func quantityInterval(lower, upper intervalSide) rm.Interval[rm.DVQuantity] {
	var iv rm.Interval[rm.DVQuantity]
	iv.Lower, iv.LowerUnbounded, iv.LowerIncluded = quantityEnd(lower, 1)
	iv.Upper, iv.UpperUnbounded, iv.UpperIncluded = quantityEnd(upper, 10)
	return iv
}

// orderedEnd returns the bound and flags of one side of a bare interval, the
// form canonical decode builds. An open side holds no bound.
func orderedEnd(s intervalSide, magnitude rm.Real) (bound rm.DVOrdered, unbounded, included bool) {
	if s == closedSide {
		bound = &rm.DVQuantity{Magnitude: magnitude, Units: "mm"}
	}
	return bound, s != closedSide, s != openSide
}

// orderedInterval builds a bare interval with the given sides.
func orderedInterval(lower, upper intervalSide) rm.Interval[rm.DVOrdered] {
	var iv rm.Interval[rm.DVOrdered]
	iv.Lower, iv.LowerUnbounded, iv.LowerIncluded = orderedEnd(lower, 1)
	iv.Upper, iv.UpperUnbounded, iv.UpperIncluded = orderedEnd(upper, 10)
	return iv
}

// intervalElement returns an ELEMENT whose value is v.
func intervalElement(v rm.DataValue) *rm.Element {
	return &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "range"}, Value: v}
}

// TestValidateRM_REQ112_IntervalFlags — REQ-112 Interval flags. Each carrier
// holds one interval: a DV_INTERVAL value (the bare form under an ELEMENT, and
// a typed one as the root), a typed normal_range and a REFERENCE_RANGE range.
// Each side pattern is planted in it. A side marked both unbounded and
// included is reported at the interval's path, as lower_included_valid or
// upper_included_valid, and both sides wrong give both issues. A correctly
// flagged open side, unbounded and not included, gives none. The issue list
// is exact, so a spurious issue fails the row too.
func TestValidateRM_REQ112_IntervalFlags(t *testing.T) {
	t.Parallel()
	carriers := []struct {
		name string
		root func(lower, upper intervalSide) any
		path string
	}{
		{
			name: "DV_INTERVAL value",
			root: func(lower, upper intervalSide) any {
				return intervalElement(&rm.DVInterval[rm.DVOrdered]{Interval: orderedInterval(lower, upper)})
			},
			path: "/value",
		},
		{
			name: "typed DV_INTERVAL root",
			root: func(lower, upper intervalSide) any {
				return &rm.DVInterval[rm.DVQuantity]{Interval: quantityInterval(lower, upper)}
			},
			path: "/",
		},
		{
			name: "typed normal_range",
			root: func(lower, upper intervalSide) any {
				return &rm.DVQuantity{
					Magnitude: 5, Units: "mm",
					NormalRange: &rm.DVInterval[rm.DVQuantity]{Interval: quantityInterval(lower, upper)},
				}
			},
			path: "/normal_range",
		},
		{
			name: "REFERENCE_RANGE range",
			root: func(lower, upper intervalSide) any {
				return &rm.DVQuantity{
					Magnitude: 5, Units: "mm",
					OtherReferenceRanges: []rm.ReferenceRange[rm.DVQuantity]{{
						Meaning: rm.DVText{Value: "high"},
						Range:   rm.DVInterval[rm.DVOrdered]{Interval: orderedInterval(lower, upper)},
					}},
				}
			},
			path: "/other_reference_ranges[0]/range",
		},
	}
	patterns := []struct {
		name         string
		lower, upper intervalSide
		want         []string // issue codes, reported at the carrier's path
	}{
		{name: "lower open and included", lower: openIncludedSide, upper: closedSide, want: []string{"lower_included_valid"}},
		{name: "upper open and included", lower: closedSide, upper: openIncludedSide, want: []string{"upper_included_valid"}},
		{name: "both open and included", lower: openIncludedSide, upper: openIncludedSide, want: []string{"lower_included_valid", "upper_included_valid"}},
		{name: "lower open, not included", lower: openSide, upper: closedSide},
		{name: "upper open, not included", lower: closedSide, upper: openSide},
		{name: "both open, not included", lower: openSide, upper: openSide},
		{name: "upper open and included, lower open", lower: openSide, upper: openIncludedSide, want: []string{"upper_included_valid"}},
	}
	for _, c := range carriers {
		for _, p := range patterns {
			t.Run(c.name+"/"+p.name, func(t *testing.T) {
				t.Parallel()
				root := c.root(p.lower, p.upper)
				want := make([]string, 0, len(p.want))
				for _, code := range p.want {
					want = append(want, code+" "+c.path)
				}
				r := validation.ValidateRM(root)
				if got := issueKeys(r.Issues); !slices.Equal(got, want) {
					t.Errorf("ValidateRM(%s, %s) issues = %q, want %q\nfull issues: %+v", c.name, p.name, got, want, r.Issues)
				}
				if r.OK != (len(want) == 0) {
					t.Errorf("ValidateRM(%s, %s).OK = %v, want %v", c.name, p.name, r.OK, len(want) == 0)
				}
			})
		}
	}
}

// TestValidateRM_REQ112_IntervalFlagsDetail — REQ-112 Interval flags, REQ-168.
// The issue is an Error, its Detail names the side's BASE invariant, and it
// carries no submitted value: neither the magnitude nor the units of a bound.
// Each bound stands beside its own open flag here, so a Detail that printed
// the side it reports would print that bound.
func TestValidateRM_REQ112_IntervalFlagsDetail(t *testing.T) {
	t.Parallel()
	const (
		magnitude = "987.65"
		units     = "mmol/L"
	)
	root := intervalElement(&rm.DVInterval[rm.DVOrdered]{
		Lower: &rm.DVQuantity{Magnitude: 987.65, Units: units}, LowerUnbounded: true, LowerIncluded: true,
		Upper: &rm.DVQuantity{Magnitude: 1234.5, Units: units}, UpperUnbounded: true, UpperIncluded: true,
	})
	invariants := map[string]string{
		"lower_included_valid": "Lower_included_valid",
		"upper_included_valid": "Upper_included_valid",
	}
	seen := 0
	for _, i := range validation.ValidateRM(root).Issues {
		invariant, ok := invariants[i.Code]
		if !ok {
			continue
		}
		seen++
		if i.Severity != validation.Error {
			t.Errorf("%s severity = %v, want Error", i.Code, i.Severity)
		}
		if !strings.Contains(i.Detail, invariant) {
			t.Errorf("%s Detail = %q, want it to name BASE %s", i.Code, i.Detail, invariant)
		}
		for _, v := range []string{magnitude, "1234.5", units} {
			if strings.Contains(i.Detail, v) {
				t.Errorf("%s Detail = %q carries the submitted value %q", i.Code, i.Detail, v)
			}
		}
	}
	if seen != len(invariants) {
		t.Errorf("ValidateRM reported %d interval-flag issues, want %d", seen, len(invariants))
	}
}
