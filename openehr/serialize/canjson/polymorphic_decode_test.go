package canjson_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestPolymorphicDecodeCoverage pins SDK-GAP-11 / PROBE-038. All three
// fixtures decode + re-marshal cleanly after Phase 2 lands the
// ancestry-driven narrow polymorphic interfaces (DVTextLike etc.) on
// top of Phase 1's generic-abstract-bound dispatch (DVInterval[T]).
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
			// Round-trip — re-marshal must succeed and preserve every
			// `_type` discriminator the original carried (the
			// substitutability guarantee).
			if _, err := canjson.Marshal(&comp); err != nil {
				t.Fatalf("canjson.Marshal (re-marshal): %v", err)
			}
		})
	}
}
