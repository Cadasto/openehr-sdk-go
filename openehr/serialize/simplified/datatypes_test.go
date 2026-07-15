package simplified

// REQ-053 — leaf datatype -> FLAT suffix mapping.
import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// suffixesOf reconstructs the decode-side suffix map from encoded FLAT entries
// at base (bare value under "").
func suffixesOf(out map[string]any, base string) map[string]any {
	sfx := map[string]any{}
	for k, v := range out {
		switch {
		case k == base:
			sfx[""] = v
		case strings.HasPrefix(k, base+"|"):
			sfx[strings.TrimPrefix(k, base+"|")] = v
		}
	}
	return sfx
}

// TestNewDatatypesEncodeDecode checks the first-class exotic datatypes are
// inverse: leafToFlat then dvFromSuffixes reproduces the datatype + fields.
func TestNewDatatypesEncodeDecode(t *testing.T) {
	issuer := "Issuer"
	tests := []struct {
		rmType string
		v      any
		check  func(t *testing.T, dv map[string]any)
	}{
		{
			rmType: "DV_DURATION",
			v:      rm.DVDuration{Value: "P2DT11H33M"},
			check: func(t *testing.T, dv map[string]any) {
				if dv["value"] != "P2DT11H33M" {
					t.Errorf("duration value = %#v", dv["value"])
				}
			},
		},
		{
			rmType: "DV_ORDINAL",
			v: rm.DVOrdinal{
				Symbol: rm.DVCodedText{DVText: rm.DVText{Value: "mild"}, DefiningCode: rm.CodePhrase{CodeString: "at0015"}},
				Value:  1,
			},
			check: func(t *testing.T, dv map[string]any) {
				sym, _ := dv["symbol"].(map[string]any)
				dc, _ := sym["defining_code"].(map[string]any)
				if dc["code_string"] != "at0015" || sym["value"] != "mild" {
					t.Errorf("ordinal symbol = %#v", sym)
				}
			},
		},
		{
			rmType: "DV_PROPORTION",
			v:      rm.DVProportion{Numerator: 20.5, Denominator: 12.4, Type: 0},
			check: func(t *testing.T, dv map[string]any) {
				if dv["numerator"] != 20.5 || dv["denominator"] != 12.4 {
					t.Errorf("proportion = %#v", dv)
				}
			},
		},
		{
			rmType: "DV_IDENTIFIER",
			v:      rm.DVIdentifier{ID: "A123", Issuer: &issuer},
			check: func(t *testing.T, dv map[string]any) {
				if dv["id"] != "A123" || dv["issuer"] != "Issuer" {
					t.Errorf("identifier = %#v", dv)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.rmType, func(t *testing.T) {
			out := map[string]any{}
			if err := leafToFlat(out, "p/x", tc.v, tc.rmType, false); err != nil {
				t.Fatalf("leafToFlat: %v", err)
			}
			dv, err := dvFromSuffixes(tc.rmType, false, suffixesOf(out, "p/x"))
			if err != nil {
				t.Fatalf("dvFromSuffixes: %v", err)
			}
			if dv["_type"] != tc.rmType {
				t.Errorf("_type = %v, want %s", dv["_type"], tc.rmType)
			}
			tc.check(t, dv)
		})
	}
}

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
			if err := leafToFlat(out, "p/x", tc.v, tc.rmType, false); err != nil {
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

// TestOtherOpenValueSet covers the |other open-value-set fallback: a DV_TEXT at
// a DV_CODED_TEXT leaf encodes to |other and decodes back to a DV_TEXT.
func TestOtherOpenValueSet(t *testing.T) {
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", rm.DVText{Value: "free text"}, "DV_CODED_TEXT", true); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if out["p/x|other"] != "free text" {
		t.Fatalf("expected p/x|other, got %#v", out)
	}
	dv, err := dvFromSuffixes("DV_CODED_TEXT", true, map[string]any{"other": "free text"})
	if err != nil {
		t.Fatalf("dvFromSuffixes(|other): %v", err)
	}
	if dv["_type"] != "DV_TEXT" || dv["value"] != "free text" {
		t.Errorf("|other decode = %#v, want DV_TEXT", dv)
	}
	if _, err := dvFromSuffixes("DV_CODED_TEXT", true, map[string]any{"other": "x", "code": "c", "value": "v"}); err == nil {
		t.Error("|other + |code = nil error, want rejection")
	}
}

// TestQuantityDecoratedRaw checks a decorated value (a DV_QUANTITY carrying
// magnitude_status) falls back to |raw rather than silently dropping the extra
// attribute.
func TestQuantityDecoratedRaw(t *testing.T) {
	status := "~"
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", rm.DVQuantity{Magnitude: 1, Units: "mm", MagnitudeStatus: &status}, "DV_QUANTITY", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|raw"]; !ok {
		t.Errorf("decorated quantity should emit |raw, got %#v", out)
	}
	if _, ok := out["p/x|magnitude"]; ok {
		t.Error("decorated quantity should not emit bare |magnitude suffixes")
	}
}

// TestRawFragmentPreservesLargeInteger checks a decorated DV_COUNT above 2^53
// keeps its magnitude exactly through the |raw path (json.Number, not float64).
func TestRawFragmentPreservesLargeInteger(t *testing.T) {
	status := "~"
	out := map[string]any{}
	// A decorated DV_COUNT (magnitude_status) rides |raw.
	if err := leafToFlat(out, "p/x", rm.DVCount{Magnitude: 9007199254740993, MagnitudeStatus: &status}, "DV_COUNT", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected |raw, got %#v", out)
	}
	num, ok := raw["magnitude"].(json.Number)
	if !ok || num.String() != "9007199254740993" {
		t.Errorf("|raw magnitude = %#v, want json.Number 9007199254740993", raw["magnitude"])
	}
}

// TestLeafToFlatTypedNil checks a typed-nil RM pointer is skipped, not
// dereferenced (which would panic).
func TestLeafToFlatTypedNil(t *testing.T) {
	out := map[string]any{}
	var p *rm.DVText
	if err := leafToFlat(out, "p/x", p, "DV_TEXT", false); err != nil {
		t.Fatalf("leafToFlat(typed-nil): %v", err)
	}
	if len(out) != 0 {
		t.Errorf("typed-nil wrote %d entries, want 0", len(out))
	}
}

// TestLeafToFlatRawFallback checks that a clinical datatype outside the core
// set is embedded as a |raw canonical fragment rather than dropped (REQ-053) —
// the codec stays lossless.
func TestLeafToFlatRawFallback(t *testing.T) {
	out := map[string]any{}
	// DV_PARAGRAPH is outside the first-class set, so it must fall back to |raw.
	if err := leafToFlat(out, "p/x", rm.DVParagraph{}, "DV_PARAGRAPH", false); err != nil {
		t.Fatalf("leafToFlat(DV_PARAGRAPH): %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected p/x|raw canonical fragment, got %#v", out)
	}
	if raw["_type"] != "DV_PARAGRAPH" {
		t.Errorf("|raw _type = %v, want DV_PARAGRAPH", raw["_type"])
	}
}
