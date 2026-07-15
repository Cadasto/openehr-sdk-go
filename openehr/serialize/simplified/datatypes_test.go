package simplified

// REQ-053 — leaf datatype -> FLAT suffix mapping.
import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func TestLeafToFlat(t *testing.T) {
	tests := []struct {
		name   string
		v      any
		rmType string
		want   map[string]any
	}{
		{
			name:   "DV_TEXT is a bare value",
			v:      rm.DVText{Value: "hello"},
			rmType: "DV_TEXT",
			want:   map[string]any{"p/x": "hello"},
		},
		{
			name:   "DV_TEXT pointer",
			v:      &rm.DVText{Value: "ptr"},
			rmType: "DV_TEXT",
			want:   map[string]any{"p/x": "ptr"},
		},
		{
			name:   "DV_DATE_TIME is a bare value",
			v:      rm.DVDateTime{Value: "2026-01-01T00:00:00"},
			rmType: "DV_DATE_TIME",
			want:   map[string]any{"p/x": "2026-01-01T00:00:00"},
		},
		{
			name:   "DV_QUANTITY splits into magnitude + unit",
			v:      rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
			rmType: "DV_QUANTITY",
			want:   map[string]any{"p/x|magnitude": float64(120), "p/x|unit": "mm[Hg]"},
		},
		{
			name: "DV_CODED_TEXT splits into code, value, terminology",
			v: rm.DVCodedText{
				DVText:       rm.DVText{Value: "event"},
				DefiningCode: rm.CodePhrase{CodeString: "433", TerminologyID: rm.TerminologyID{Value: "openehr"}},
			},
			rmType: "DV_CODED_TEXT",
			want:   map[string]any{"p/x|code": "433", "p/x|value": "event", "p/x|terminology": "openehr"},
		},
		{
			// STABLE RM mappings: DV_COUNT carries magnitude as the bare value.
			name:   "DV_COUNT is a bare value",
			v:      rm.DVCount{Magnitude: 5},
			rmType: "DV_COUNT",
			want:   map[string]any{"p/x": int64(5)},
		},
		{
			// STABLE RM mappings: DV_BOOLEAN carries value as the bare value.
			name:   "DV_BOOLEAN is a bare value",
			v:      rm.DVBoolean{Value: true},
			rmType: "DV_BOOLEAN",
			want:   map[string]any{"p/x": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := map[string]any{}
			if err := leafToFlat(out, "p/x", tc.v, tc.rmType); err != nil {
				t.Fatalf("leafToFlat: %v", err)
			}
			if len(out) != len(tc.want) {
				t.Fatalf("got %d entries %v, want %d %v", len(out), out, len(tc.want), tc.want)
			}
			for k, w := range tc.want {
				if out[k] != w {
					t.Errorf("out[%q] = %#v, want %#v", k, out[k], w)
				}
			}
		})
	}
}

// TestLeafToFlatRawFallback checks that a clinical datatype outside the core
// set is embedded as a |raw canonical fragment rather than dropped (REQ-053) —
// the codec stays lossless.
func TestLeafToFlatRawFallback(t *testing.T) {
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0}, "DV_PROPORTION"); err != nil {
		t.Fatalf("leafToFlat(DV_PROPORTION): %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected p/x|raw canonical fragment, got %#v", out)
	}
	if raw["_type"] != "DV_PROPORTION" {
		t.Errorf("|raw _type = %v, want DV_PROPORTION", raw["_type"])
	}
}
