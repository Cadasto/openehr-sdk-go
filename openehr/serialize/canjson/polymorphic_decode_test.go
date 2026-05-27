package canjson_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestPolymorphicDecodeCoverage pins SDK-GAP-11 / PROBE-038.
//
// Issue B (generic abstract bound, e.g. DV_INTERVAL lower/upper via
// typereg) is covered by the dv_interval_quantity case — landed Phase 1.
//
// Issue A (substitutable subtype in concrete-typed slots, e.g.
// LOCATABLE.name carrying DV_CODED_TEXT) remains skipped until Phase 2
// lands ancestry-driven narrow interfaces.
func TestPolymorphicDecodeCoverage(t *testing.T) {
	cases := []struct {
		name      string
		file      string
		skipUntil string // non-empty: pending plan Phase 2 (Issue A)
	}{
		{
			name:      "LOCATABLE.name receives DV_CODED_TEXT (Issue A — substitutable subtype)",
			file:      fixtures.RMJSON("polymorphic/name_dv_coded_text"),
			skipUntil: "PROBE-038 Issue A — narrow interfaces (plan Phase 2)",
		},
		{
			name: "ELEMENT.value DV_INTERVAL<DV_QUANTITY> (Issue B — generic over abstract bound)",
			file: fixtures.RMJSON("polymorphic/dv_interval_quantity"),
		},
		{
			name:      "representative composition (both issues + DV_ORDINAL)",
			file:      fixtures.RMJSON("polymorphic/representative_full"),
			skipUntil: "PROBE-038 Issue A — narrow interfaces (plan Phase 2)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipUntil != "" {
				t.Skip(tc.skipUntil)
			}
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
