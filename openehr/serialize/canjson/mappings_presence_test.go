package canjson_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// REQ-052: records the known model-level collapse — []/null/absent mappings all
// decode to an empty slice and re-encode to an absent key. Locks the behaviour
// so a future change is a conscious one, not an accident.
func TestDVTextMappingsCollapse(t *testing.T) {
	const want = `{"_type":"DV_TEXT","value":"x"}`

	cases := []struct {
		name string
		in   string
	}{
		{name: "absent mappings", in: `{"_type":"DV_TEXT","value":"x"}`},
		{name: "empty mappings array", in: `{"_type":"DV_TEXT","value":"x","mappings":[]}`},
		{name: "null mappings", in: `{"_type":"DV_TEXT","value":"x","mappings":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var d rm.DVText
			if err := canjson.Unmarshal([]byte(tc.in), &d); err != nil {
				t.Fatalf("decode %s: input %s: %v", tc.name, tc.in, err)
			}
			out, err := canjson.Marshal(&d)
			if err != nil {
				t.Fatalf("encode %s: %v", tc.name, err)
			}
			if got := string(out); got != want {
				t.Fatalf("%s: re-encoded %s -> %s, want %s (collapse documented in wire.md REQ-052)", tc.name, tc.in, got, want)
			}
		})
	}
}
