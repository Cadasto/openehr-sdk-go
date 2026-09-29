package instance_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

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
	templates := []string{
		"Test_dv_interval_dv_count_lower_upper_constraint.v0",
		"Test_dv_interval_dv_count_open_constraint.v0",
		"Test_dv_interval_dv_quantity_lower_upper_constraint.v0",
		"Test_dv_interval_dv_quantity_open_constraint.v0",
	}
	for _, name := range templates {
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(name+"/"+policy.String(), func(t *testing.T) {
				c := compileFixture(t, name)
				out, err := instance.Generate(t.Context(), c, instance.Options{
					Policy:    policy,
					Territory: "NL",
					Composer:  testComposer(),
					Now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				})
				if err != nil {
					t.Fatalf("Generate(%s, %v): %v", name, policy, err)
				}
				data, err := canjson.Marshal(out)
				if err != nil {
					t.Fatalf("canjson.Marshal: %v", err)
				}
				var doc any
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatalf("json.Unmarshal of the canonical JSON: %v", err)
				}

				bounds := 0
				walkIntervals(doc, "", func(path string, iv map[string]any) {
					for _, side := range []string{"lower", "upper"} {
						bound, ok := iv[side]
						if !ok {
							continue
						}
						bounds++
						if open, _ := iv[side+"_unbounded"].(bool); open {
							t.Errorf("%s: %s bound %v stands beside %s_unbounded: true", path, side, bound, side)
						}
					}
				})
				if bounds == 0 {
					t.Fatalf("no DV_INTERVAL bound in the composition generated from %s, so nothing was checked", name)
				}
			})
		}
	}
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
