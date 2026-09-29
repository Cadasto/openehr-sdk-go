package instance_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// orderSeeds is how many RandomFill seeds each order sweep draws. Before
// the generator ordered its bounds, the DV_COUNT interval OPT inverted in
// 17 of 40 seeds and the DV_QUANTITY one in 21 of 40.
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
// check that restoring the order never moves a bound out of its own side's
// OPT constraint, and that the replacement pairs are tried in their stated
// order. Each case edits the DV_COUNT interval OPT so the two bound
// magnitudes carry different constraints whose ExampleFill values are
// inverted, then checks the pair the generator settles on. Under
// RandomFill every seed must also stay in order and inside both
// constraints.
func TestGenerateIntervalOrderKeepsEachSideInItsConstraint(t *testing.T) {
	raw, err := os.ReadFile(fixtures.TemplateOptForName("Test_dv_interval_dv_count_lower_upper_constraint.v0"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	cases := []struct {
		name                 string
		lowerItem            string // the lower bound's C_INTEGER body
		upperItem            string // the upper bound's C_INTEGER body
		lowerOK, upperOK     func(float64) bool
		wantLower, wantUpper float64
	}{
		{
			// Examples 50 and 10; swapped, 10 and 50 fit both sides.
			name:      "the swapped pair fits",
			lowerItem: intList(50, 10),
			upperItem: intList(10, 50),
			lowerOK:   in(10, 50),
			upperOK:   in(10, 50),
			wantLower: 10, wantUpper: 50,
		},
		{
			// Examples 10 and 0; 0 is outside the lower range, so no swap,
			// but the lower example 10 fits the upper range.
			name:      "the lower example on both sides",
			lowerItem: intRange(10, 20),
			upperItem: intRange(0, 100),
			lowerOK:   between(10, 20),
			upperOK:   between(0, 100),
			wantLower: 10, wantUpper: 10,
		},
		{
			// Examples 20 and 3; 20 is outside the upper list, so neither
			// the swap nor the lower example fits, but 3 fits both lists.
			name:      "the upper example on both sides",
			lowerItem: intList(20, 3),
			upperItem: intList(3, 10),
			lowerOK:   in(20, 3),
			upperOK:   in(3, 10),
			wantLower: 3, wantUpper: 3,
		},
	}
	for _, tc := range cases {
		c := compileSyntheticOPT(t, setCountMagnitudeItems(t, string(raw), tc.lowerItem, tc.upperItem))
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

// intRange and intList build the body of a C_INTEGER item.
func intRange(lo, hi int) string {
	return fmt.Sprintf("<range><lower_included>true</lower_included><upper_included>true</upper_included>"+
		"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>"+
		"<lower>%d</lower><upper>%d</upper></range>", lo, hi)
}

func intList(values ...int) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "<list>%d</list>", v)
	}
	return b.String()
}

// countItemRE matches one C_INTEGER item in the vendored DV_COUNT interval
// OPT; the two under the interval are the lower and upper bound magnitudes.
var countItemRE = regexp.MustCompile(`(?s)<item xsi:type="C_INTEGER">.*?</item>`)

// setCountMagnitudeItems replaces the lower and upper bound magnitude
// constraints of the DV_COUNT interval OPT, in that order.
func setCountMagnitudeItems(t *testing.T, opt, lowerItem, upperItem string) string {
	t.Helper()
	node := intervalNodeRE.FindStringIndex(opt)
	if node == nil {
		t.Fatal("OPT has no DV_INTERVAL<DV_COUNT> node")
	}
	head, tail := opt[:node[1]], opt[node[1]:]
	matches := countItemRE.FindAllStringIndex(tail, -1)
	if len(matches) != 2 {
		t.Fatalf("found %d C_INTEGER items under the interval, want 2", len(matches))
	}
	item := func(body string) string { return `<item xsi:type="C_INTEGER">` + body + `</item>` }
	return head + tail[:matches[0][0]] + item(lowerItem) +
		tail[matches[0][1]:matches[1][0]] + item(upperItem) + tail[matches[1][1]:]
}
