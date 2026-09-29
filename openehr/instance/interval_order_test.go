package instance_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// orderSeeds is how many RandomFill seeds each order sweep draws.
const orderSeeds = 40

// boundedMagnitudes returns the magnitudes of a canonical-JSON interval's
// two bounds when both sides are bounded and the bounds compare the way
// the RM floor compares them: two DV_COUNT, or two DV_QUANTITY in the
// same units.
func boundedMagnitudes(iv map[string]any) (lower, upper float64, ok bool) {
	if open, _ := iv["lower_unbounded"].(bool); open {
		return 0, 0, false
	}
	if open, _ := iv["upper_unbounded"].(bool); open {
		return 0, 0, false
	}
	lo, _ := iv["lower"].(map[string]any)
	hi, _ := iv["upper"].(map[string]any)
	if lo == nil || hi == nil || lo["_type"] != hi["_type"] {
		return 0, 0, false
	}
	switch lo["_type"] {
	case "DV_COUNT":
	case "DV_QUANTITY":
		if lo["units"] != hi["units"] {
			return 0, 0, false
		}
	default:
		return 0, 0, false
	}
	lower, okL := lo["magnitude"].(float64)
	upper, okU := hi["magnitude"].(float64)
	return lower, upper, okL && okU
}

// TestGenerateIntervalBoundsInOrder is the REQ-107 check that a generated
// DV_INTERVAL with both sides bounded never has its lower bound above its
// upper one, which BASE Interval's Limits_consistent forbids. RandomFill
// samples each bound from its own OPT constraint, so without ordering the
// two draws land either way round. The sweep covers the four vendored
// interval OPTs over a fixed set of seeds.
func TestGenerateIntervalBoundsInOrder(t *testing.T) {
	for _, name := range intervalTemplates {
		t.Run(name, func(t *testing.T) {
			c := compileFixture(t, name)
			compared := 0
			for seed := range uint64(orderSeeds) {
				opts := intervalOptions(instance.Example)
				opts.ValueFill = instance.RandomFill
				opts.ValueSource = rand.NewPCG(seed, 1)
				walkIntervals(generateCanonical(t, c, opts), "", func(path string, iv map[string]any) {
					lower, upper, ok := boundedMagnitudes(iv)
					if !ok {
						return
					}
					compared++
					if lower > upper {
						t.Errorf("seed %d, %s: lower %v > upper %v", seed, path, lower, upper)
					}
				})
			}
			if compared == 0 {
				t.Fatalf("no comparable bounded DV_INTERVAL generated from %s, so nothing was checked", name)
			}
		})
	}
}

// TestGenerateIntervalOrderKeepsEachSideInItsConstraint is the REQ-107
// end-to-end check of the ordering rule through Generate. Each case edits
// the vendored DV_COUNT interval OPT so the two bound magnitudes carry
// different constraints whose ExampleFill values are inverted, then checks
// the pair the generator settles on: the swapped pair when it fits, and
// otherwise the lowest value the lower side accepts with the highest value
// the upper side accepts. Under RandomFill every seed must also stay in
// order and inside both constraints. The in-package tests pin each step of
// the rule on its own.
func TestGenerateIntervalOrderKeepsEachSideInItsConstraint(t *testing.T) {
	opt := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	closed := instance.Closed
	cases := []struct {
		name                 string
		lowerItem, upperItem string // the two bounds' C_INTEGER bodies
		lowerOK, upperOK     func(float64) bool
		wantLower, wantUpper float64
	}{
		{
			// Examples 50 and 10; swapped, 10 and 50 fit both sides.
			name:      "the swapped pair fits",
			lowerItem: instance.IntList(50, 10),
			upperItem: instance.IntList(10, 50),
			lowerOK:   in(10, 50),
			upperOK:   in(10, 50),
			wantLower: 10, wantUpper: 50,
		},
		{
			// Examples 10 and 0; 0 is outside the lower range, so no swap.
			name:      "a lower range above the upper example",
			lowerItem: instance.IntRange(closed(10), closed(20)),
			upperItem: instance.IntRange(closed(0), closed(100)),
			lowerOK:   between(10, 20),
			upperOK:   between(0, 100),
			wantLower: 10, wantUpper: 100,
		},
		{
			// Examples 70 and 50; 50 is not in the lower list, so no swap,
			// though 55 fits the upper range.
			name:      "a lower list above the upper example",
			lowerItem: instance.IntList(70, 55),
			upperItem: instance.IntRange(closed(50), closed(60)),
			lowerOK:   in(70, 55),
			upperOK:   between(50, 60),
			wantLower: 55, wantUpper: 60,
		},
	}
	for _, tc := range cases {
		c := compileSyntheticOPT(t, instance.EditCountBounds(t, opt, tc.lowerItem, tc.upperItem))
		onlyInterval := func(t *testing.T, opts instance.Options) (lower, upper float64) {
			t.Helper()
			var found []map[string]any
			walkIntervals(generateCanonical(t, c, opts), "", func(_ string, iv map[string]any) {
				found = append(found, iv)
			})
			if len(found) != 1 {
				t.Fatalf("generated %d DV_INTERVAL values, want 1", len(found))
			}
			lower, upper, ok := boundedMagnitudes(found[0])
			if !ok {
				t.Fatalf("interval %v has no comparable bounded pair", found[0])
			}
			return lower, upper
		}
		for _, policy := range intervalPolicies {
			t.Run(tc.name+"/ExampleFill/"+policy.String(), func(t *testing.T) {
				lower, upper := onlyInterval(t, intervalOptions(policy))
				if lower != tc.wantLower || upper != tc.wantUpper {
					t.Errorf("bounds = [%v, %v], want [%v, %v]", lower, upper, tc.wantLower, tc.wantUpper)
				}
			})
		}
		t.Run(tc.name+"/RandomFill", func(t *testing.T) {
			for seed := range uint64(orderSeeds) {
				opts := intervalOptions(instance.Example)
				opts.ValueFill = instance.RandomFill
				opts.ValueSource = rand.NewPCG(seed, 1)
				lower, upper := onlyInterval(t, opts)
				if lower > upper || !tc.lowerOK(lower) || !tc.upperOK(upper) {
					t.Errorf("seed %d: bounds = [%v, %v], want lower <= upper, each inside its own side's constraint", seed, lower, upper)
				}
			}
		})
	}
}

func between(lo, hi float64) func(float64) bool {
	return func(v float64) bool { return v >= lo && v <= hi }
}

func in(allowed ...float64) func(float64) bool {
	return func(v float64) bool { return slices.Contains(allowed, v) }
}
