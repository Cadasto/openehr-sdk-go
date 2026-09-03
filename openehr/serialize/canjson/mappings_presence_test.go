package canjson_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-052: records what decode preserves and what re-encode collapses for
// DV_TEXT.mappings — the two are NOT the same, and the REQ-112 RM-floor
// invariant `mappings_valid` depends on the difference.
//
//   - Decode preserves presence: an absent `mappings` key and an explicit
//     JSON `null` both decode to a nil slice, while a literal `[]` decodes
//     to a non-nil empty slice. So "supplied but empty" is distinguishable
//     from "not supplied", which is exactly the signal the floor reads
//     (openehr/validation's checkTermMappings) to flag a present-but-empty
//     mappings and leave absent/null alone.
//   - Re-encode collapses all three: the field carries `omitempty`, so a
//     nil and a non-nil empty slice both write no key at all. The
//     round-trip is therefore lossy on that one distinction — a decoded
//     literal `[]` does not survive re-encoding.
//
// Locks both halves so a future change to either is a conscious one, not
// an accident.
func TestDVTextMappingsDecodePresenceAndEncodeCollapse(t *testing.T) {
	const wantEncoded = `{"_type":"DV_TEXT","value":"x"}`

	cases := []struct {
		name string
		in   string
		// wantNil is the decoded nilness: true for "not supplied"
		// (absent key or JSON null), false for a supplied literal `[]`,
		// which decodes non-nil and empty.
		wantNil bool
	}{
		{name: "absent mappings", in: `{"_type":"DV_TEXT","value":"x"}`, wantNil: true},
		{name: "null mappings", in: `{"_type":"DV_TEXT","value":"x","mappings":null}`, wantNil: true},
		{name: "empty mappings array", in: `{"_type":"DV_TEXT","value":"x","mappings":[]}`, wantNil: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d rm.DVText
			if err := canjson.Unmarshal([]byte(tc.in), &d); err != nil {
				t.Fatalf("decode %s: input %s: %v", tc.name, tc.in, err)
			}

			// Decode side: presence is preserved as slice nilness.
			if gotNil := d.Mappings == nil; gotNil != tc.wantNil {
				t.Errorf("decoded %s -> Mappings == nil is %v, want %v (decode preserves absent/null as nil and a literal [] as non-nil empty; REQ-112's mappings_valid reads this)",
					tc.in, gotNil, tc.wantNil)
			}
			if len(d.Mappings) != 0 {
				t.Errorf("decoded %s -> len(Mappings) = %d, want 0", tc.in, len(d.Mappings))
			}

			// Encode side: `omitempty` collapses nil and non-nil empty alike.
			out, err := canjson.Marshal(&d)
			if err != nil {
				t.Fatalf("encode %s: %v", tc.name, err)
			}
			if got := string(out); got != wantEncoded {
				t.Fatalf("%s: re-encoded %s -> %s, want %s (collapse documented in wire.md REQ-052)", tc.name, tc.in, got, wantEncoded)
			}
		})
	}
}
