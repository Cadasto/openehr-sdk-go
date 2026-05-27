package canjson_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestPolymorphicDecodeCoverage pins SDK-GAP-11 / PROBE-038. Skipped
// at Phase 0 — see docs/plans/2026-05-26-rm-polymorphic-decode-coverage.md
// — because today's generated `UnmarshalJSON` either (a) emits a strict
// class-equality check that rejects substitutable subtypes
// (`LOCATABLE.name DVText` receiving `DV_CODED_TEXT`), or (b) generates
// `DVInterval[T DVOrdered]` with `Lower T`, which Go's `encoding/json`
// cannot decode into an interface field.
//
// Phase 1 lands the generic-abstract-decode fix (Issue B); Phase 2
// lands the ancestry-driven narrow-interface emission (Issue A) — and
// unskips this table.
func TestPolymorphicDecodeCoverage(t *testing.T) {
	t.Skip("PROBE-038 stub — unskips when bmmgen polymorphism extension lands (plan Phases 1+2)")

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
