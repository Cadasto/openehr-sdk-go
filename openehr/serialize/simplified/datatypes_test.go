package simplified

// REQ-053 — leaf datatype -> FLAT suffix mapping.
import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func TestLeafToFlat(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want map[string]any
	}{
		{
			name: "DV_TEXT is a bare value",
			v:    rm.DVText{Value: "hello"},
			want: map[string]any{"p/x": "hello"},
		},
		{
			name: "DV_TEXT pointer",
			v:    &rm.DVText{Value: "ptr"},
			want: map[string]any{"p/x": "ptr"},
		},
		{
			name: "DV_DATE_TIME is a bare value",
			v:    rm.DVDateTime{Value: "2026-01-01T00:00:00"},
			want: map[string]any{"p/x": "2026-01-01T00:00:00"},
		},
		{
			name: "DV_QUANTITY splits into magnitude + unit",
			v:    rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
			want: map[string]any{"p/x|magnitude": float64(120), "p/x|unit": "mm[Hg]"},
		},
		{
			name: "DV_CODED_TEXT splits into code, value, terminology",
			v: rm.DVCodedText{
				DVText:       rm.DVText{Value: "event"},
				DefiningCode: rm.CodePhrase{CodeString: "433", TerminologyID: rm.TerminologyID{Value: "openehr"}},
			},
			want: map[string]any{"p/x|code": "433", "p/x|value": "event", "p/x|terminology": "openehr"},
		},
		{
			name: "DV_COUNT is a magnitude suffix",
			v:    rm.DVCount{Magnitude: 5},
			want: map[string]any{"p/x|magnitude": int64(5)},
		},
		{
			name: "DV_BOOLEAN is a value suffix",
			v:    rm.DVBoolean{Value: true},
			want: map[string]any{"p/x|value": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := map[string]any{}
			leafToFlat(out, "p/x", tc.v)
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
