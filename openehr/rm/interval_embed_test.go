package rm_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestEmbeddedIntervalDropsOuterField records how encoding treats a struct
// that embeds Interval. MarshalJSONTo has a value receiver, so the outer
// struct promotes that method and the encoder writes only the interval.
// The outer struct's own fields are left out of the JSON, and a later decode
// of that JSON cannot bring them back. The test fails if an outer field
// starts to appear, so this loss cannot change without an intentional edit.
func TestEmbeddedIntervalDropsOuterField(t *testing.T) {
	type holder struct {
		rm.Interval[int]
		Note string `json:"note"`
	}
	h := holder{
		Interval: rm.Interval[int]{
			Lower:         1,
			Upper:         9,
			LowerIncluded: true,
			UpperIncluded: true,
		},
		Note: "outer-note",
	}
	cases := []struct {
		name string
		val  any
	}{
		{name: "value", val: h},
		{name: "pointer", val: &h},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := canjson.Marshal(tc.val)
			if err != nil {
				t.Fatalf("Marshal() err = %v", err)
			}
			text := string(out)
			if strings.Contains(text, `"note"`) || strings.Contains(text, "outer-note") {
				t.Errorf("outer field was encoded: %s", text)
			}
			if !strings.Contains(text, `"lower":1`) || !strings.Contains(text, `"upper":9`) {
				t.Errorf("interval bounds missing: %s", text)
			}
		})
	}
}
