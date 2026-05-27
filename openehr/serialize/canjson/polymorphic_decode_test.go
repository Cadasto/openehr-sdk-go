package canjson_test

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestPolymorphicDecodeCoverage pins SDK-GAP-11 / PROBE-038. All three
// fixtures decode + re-marshal cleanly after Phase 2 lands the
// ancestry-driven narrow polymorphic interfaces (DVTextLike etc.) on
// top of Phase 1's generic-abstract-bound dispatch (DVInterval[T]).
//
// The assertion mirrors PROBE-038's discriminator-multiset check:
// every `_type` value present in the input must reappear in the
// re-marshalled output (counted). A silent narrowing — DV_CODED_TEXT
// decoded into a parent DVText struct that drops defining_code and
// re-emits as DV_TEXT — would show up here as a missing discriminator.
func TestPolymorphicDecodeCoverage(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		{
			name: "LOCATABLE.name receives DV_CODED_TEXT (Issue A — substitutable subtype)",
			file: fixtures.RMJSON("polymorphic/name_dv_coded_text"),
		},
		{
			name: "ELEMENT.value DV_INTERVAL<DV_QUANTITY> (Issue B — generic over abstract bound)",
			file: fixtures.RMJSON("polymorphic/dv_interval_quantity"),
		},
		{
			name: "representative composition (both issues + DV_ORDINAL)",
			file: fixtures.RMJSON("polymorphic/representative_full"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("read fixture %s: %v", tc.file, err)
			}
			var comp rm.Composition
			if err := canjson.Unmarshal(data, &comp); err != nil {
				t.Fatalf("canjson.Unmarshal: %v", err)
			}
			out, err := canjson.Marshal(&comp)
			if err != nil {
				t.Fatalf("canjson.Marshal (re-marshal): %v", err)
			}
			want := collectDiscriminators(t, data)
			got := collectDiscriminators(t, out)
			for k, n := range want {
				if got[k] < n {
					t.Errorf("re-marshalled body lost %s discriminator(s): want %d, got %d — subtype narrowed on decode", k, n, got[k])
				}
			}
		})
	}
}

// collectDiscriminators is the unit-test mirror of PROBE-038's
// discriminator-multiset walker — kept inline so the test stays
// self-contained and the probe package's helpers stay unexported.
func collectDiscriminators(t *testing.T, b []byte) map[string]int {
	t.Helper()
	out := map[string]int{}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var top any
	if err := dec.Decode(&top); err != nil {
		t.Fatalf("collectDiscriminators: %v", err)
	}
	var walk func(any)
	walk = func(v any) {
		switch tt := v.(type) {
		case map[string]any:
			if tn, ok := tt["_type"].(string); ok {
				out[tn]++
			}
			keys := make([]string, 0, len(tt))
			for k := range tt {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(tt[k])
			}
		case []any:
			for _, e := range tt {
				walk(e)
			}
		}
	}
	walk(top)
	return out
}
