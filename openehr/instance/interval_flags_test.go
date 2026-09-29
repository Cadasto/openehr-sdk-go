package instance_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// intervalTemplates are the vendored OPTs whose one ELEMENT value is a
// DV_INTERVAL with both bounds constrained. None of them constrains
// lower_included or upper_included.
var intervalTemplates = []string{
	"Test_dv_interval_dv_count_lower_upper_constraint.v0",
	"Test_dv_interval_dv_count_open_constraint.v0",
	"Test_dv_interval_dv_quantity_lower_upper_constraint.v0",
	"Test_dv_interval_dv_quantity_open_constraint.v0",
}

var intervalPolicies = []instance.Policy{instance.Minimal, instance.Example}

// intervalOptions returns the generator options the interval tests share.
func intervalOptions(policy instance.Policy) instance.Options {
	return instance.Options{
		Policy:    policy,
		Territory: "NL",
		Composer:  testComposer(),
		Now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// generateCanonical generates a composition from c and returns it as
// decoded canonical JSON.
func generateCanonical(t *testing.T, c *templatecompile.Compiled, opts instance.Options) any {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate(%v, %v): %v", opts.Policy, opts.ValueFill, err)
	}
	data, err := canjson.Marshal(out)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("json.Unmarshal of the canonical JSON: %v", err)
	}
	return doc
}

// forEachGeneratedInterval runs one subtest per interval template and
// policy, and calls check for every DV_INTERVAL in the composition
// generated there. A subtest in which no interval carries a bound fails,
// so a check cannot pass with nothing to check.
func forEachGeneratedInterval(t *testing.T, check func(t *testing.T, path string, iv map[string]any)) {
	t.Helper()
	for _, name := range intervalTemplates {
		for _, policy := range intervalPolicies {
			t.Run(name+"/"+policy.String(), func(t *testing.T) {
				doc := generateCanonical(t, compileFixture(t, name), intervalOptions(policy))
				bounds := 0
				walkIntervals(doc, "", func(path string, iv map[string]any) {
					for _, side := range []string{"lower", "upper"} {
						if _, ok := iv[side]; ok {
							bounds++
						}
					}
					check(t, path, iv)
				})
				if bounds == 0 {
					t.Fatalf("no DV_INTERVAL bound in the composition generated from %s, so nothing was checked", name)
				}
			})
		}
	}
}

// TestGenerateIntervalBoundNotBesideOpenFlag is the REQ-107 end-to-end
// check that a generated DV_INTERVAL never carries a bound beside its own
// `*_unbounded: true`. BASE Interval says an open side carries no bound,
// and the FLAT codec refuses the pair (wire.md § REQ-140, "Interval
// boundary flags"), so an instance carrying it could not be FLAT-encoded.
// The generator seeds both flags true and then writes the bounds the OPT
// constrains, so every side it fills must come out closed.
//
// The check reads the canonical JSON rather than FLAT: FLAT reports a
// concrete bound equal to its type's zero (a DV_COUNT of 0, which these
// templates generate) as the unbounded end instead of refusing it, so it
// would miss half of the cases. In canonical JSON every interval position
// looks the same.
func TestGenerateIntervalBoundNotBesideOpenFlag(t *testing.T) {
	forEachGeneratedInterval(t, func(t *testing.T, path string, iv map[string]any) {
		for _, side := range []string{"lower", "upper"} {
			bound, ok := iv[side]
			if !ok {
				continue
			}
			if open, _ := iv[side+"_unbounded"].(bool); open {
				t.Errorf("%s: %s bound %v stands beside %s_unbounded: true", path, side, bound, side)
			}
		}
	})
}

// TestGenerateIntervalSideIncludesItsBound is the REQ-107 check that a
// generated DV_INTERVAL is a sensible example rather than an empty range
// such as (0, 0). A bounded side includes its bound unless the OPT
// constrains `*_included`, and these templates do not; that is the closed
// endpoint the template parser assumes when an OPT range omits the flag.
// An open side never includes its bound: BASE Interval's
// Lower_included_valid and Upper_included_valid say
// `lower_unbounded implies not lower_included`, and likewise for upper.
func TestGenerateIntervalSideIncludesItsBound(t *testing.T) {
	forEachGeneratedInterval(t, func(t *testing.T, path string, iv map[string]any) {
		for _, side := range []string{"lower", "upper"} {
			open, _ := iv[side+"_unbounded"].(bool)
			included, _ := iv[side+"_included"].(bool)
			switch {
			case open && included:
				t.Errorf("%s: open %s side has %s_included: true", path, side, side)
			case !open && !included:
				t.Errorf("%s: bounded %s side has %s_included: false, which the OPT does not ask for", path, side, side)
			}
		}
	})
}

// TestGenerateIntervalIncludedFollowsOPT is the REQ-107 check that the
// generator honours an OPT C_BOOLEAN on an interval's `*_included`, and
// leaves a side the OPT gives no bound open and excluded. Each case edits
// the vendored DV_COUNT interval OPT, whose interval constrains both
// bounds and neither `*_included`.
func TestGenerateIntervalIncludedFollowsOPT(t *testing.T) {
	type sideWant struct{ open, included bool }
	cases := []struct {
		name         string
		edit         func(t *testing.T, opt string) string
		lower, upper sideWant
	}{
		{
			name:  "upper_included constrained to false",
			edit:  injectIncludedConstraint("upper_included", false, true),
			lower: sideWant{open: false, included: true},
			upper: sideWant{open: false, included: false},
		},
		{
			name:  "lower_included constrained to false",
			edit:  injectIncludedConstraint("lower_included", false, true),
			lower: sideWant{open: false, included: false},
			upper: sideWant{open: false, included: true},
		},
		{
			// Admitting both values decides nothing, so the closed default holds.
			name:  "lower_included admitting true and false",
			edit:  injectIncludedConstraint("lower_included", true, true),
			lower: sideWant{open: false, included: true},
			upper: sideWant{open: false, included: true},
		},
		{
			name:  "no lower bound constrained",
			edit:  removeLowerBound,
			lower: sideWant{open: true, included: false},
			upper: sideWant{open: false, included: true},
		},
	}
	raw, err := os.ReadFile(fixtures.TemplateOptForName("Test_dv_interval_dv_count_lower_upper_constraint.v0"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, tc := range cases {
		for _, policy := range intervalPolicies {
			t.Run(tc.name+"/"+policy.String(), func(t *testing.T) {
				c := compileSyntheticOPT(t, tc.edit(t, string(raw)))
				var found []map[string]any
				walkIntervals(generateCanonical(t, c, intervalOptions(policy)), "", func(_ string, iv map[string]any) {
					found = append(found, iv)
				})
				if len(found) != 1 {
					t.Fatalf("generated %d DV_INTERVAL values, want 1", len(found))
				}
				iv := found[0]
				for side, want := range map[string]sideWant{"lower": tc.lower, "upper": tc.upper} {
					open, _ := iv[side+"_unbounded"].(bool)
					included, _ := iv[side+"_included"].(bool)
					if open != want.open || included != want.included {
						t.Errorf("%s side: (%s_unbounded, %s_included) = (%v, %v), want (%v, %v)",
							side, side, side, open, included, want.open, want.included)
					}
				}
			})
		}
	}
}

// These find the DV_INTERVAL node in the vendored DV_COUNT interval OPT,
// and the starts of its lower-bound and upper-bound attributes.
var (
	intervalNodeRE = regexp.MustCompile(`<rm_type_name>DV_INTERVAL&lt;DV_COUNT&gt;</rm_type_name>`)
	lowerAttrRE    = regexp.MustCompile(`<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>lower</rm_attribute_name>`)
	upperAttrRE    = regexp.MustCompile(`<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>upper</rm_attribute_name>`)
)

// lowerAttrSpan returns where the interval's lower-bound attribute starts
// and where the upper-bound attribute that follows it starts.
func lowerAttrSpan(t *testing.T, opt string) (start, end int) {
	t.Helper()
	node := intervalNodeRE.FindStringIndex(opt)
	if node == nil {
		t.Fatal("OPT has no DV_INTERVAL<DV_COUNT> node")
	}
	lower := lowerAttrRE.FindStringIndex(opt[node[1]:])
	upper := upperAttrRE.FindStringIndex(opt[node[1]:])
	if lower == nil || upper == nil || upper[0] < lower[0] {
		t.Fatal("OPT interval node has no lower attribute followed by an upper attribute")
	}
	return node[1] + lower[0], node[1] + upper[0]
}

// injectIncludedConstraint returns an edit that adds a C_BOOLEAN on the
// interval's attr (lower_included or upper_included) with the given
// true_valid and false_valid.
func injectIncludedConstraint(attr string, trueValid, falseValid bool) func(t *testing.T, opt string) string {
	return func(t *testing.T, opt string) string {
		t.Helper()
		at, _ := lowerAttrSpan(t, opt)
		block := fmt.Sprintf(`<attributes xsi:type="C_SINGLE_ATTRIBUTE">
<rm_attribute_name>%s</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>
<match_negated>false</match_negated>
<children xsi:type="C_PRIMITIVE_OBJECT">
<rm_type_name>BOOLEAN</rm_type_name>
<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>
<node_id></node_id>
<item xsi:type="C_BOOLEAN"><true_valid>%t</true_valid><false_valid>%t</false_valid></item>
</children>
</attributes>
`, attr, trueValid, falseValid)
		return opt[:at] + block + opt[at:]
	}
}

// removeLowerBound drops the interval's lower-bound attribute, so the OPT
// constrains only the upper bound.
func removeLowerBound(t *testing.T, opt string) string {
	t.Helper()
	start, end := lowerAttrSpan(t, opt)
	return opt[:start] + opt[end:]
}

// walkIntervals calls visit for every DV_INTERVAL object in a decoded
// canonical-JSON document, passing a slash-separated path to it. Object
// keys are walked in sorted order so failures report in a stable order.
func walkIntervals(v any, path string, visit func(path string, iv map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		if typ, _ := x["_type"].(string); strings.HasPrefix(typ, "DV_INTERVAL") {
			visit(path, x)
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			walkIntervals(x[k], path+"/"+k, visit)
		}
	case []any:
		for i, e := range x {
			walkIntervals(e, fmt.Sprintf("%s/%d", path, i), visit)
		}
	}
}
