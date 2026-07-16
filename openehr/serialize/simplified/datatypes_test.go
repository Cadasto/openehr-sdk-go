package simplified

// REQ-053 — leaf datatype -> FLAT suffix mapping.
import (
	"encoding/json"
	"errors"
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

// TestSubstitutedSubtypeRidesRaw: a legal RM subtype substitution (DV_EHR_URI
// stored at a DV_URI leaf) must not take the suffix form — decode would rebuild
// it as the leaf type, silently demoting DV_EHR_URI to DV_URI. It rides |raw
// stamped with its dynamic type.
func TestSubstitutedSubtypeRidesRaw(t *testing.T) {
	out := map[string]any{}
	v := rm.DVEHRURI{DVURI: rm.DVURI{Value: "ehr://ehr/1"}}
	if err := leafToFlat(out, "p/x", v, "DV_URI", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected |raw for substituted subtype, got %#v", out)
	}
	if raw["_type"] != "DV_EHR_URI" {
		t.Errorf("|raw _type = %v, want DV_EHR_URI (dynamic type, not the leaf type)", raw["_type"])
	}
}

// TestNonLocalOrdinalRidesRaw: the ordinal suffix set has no |terminology
// channel and decode rebuilds the symbol as archetype-local, so a symbol coded
// in an external terminology must ride |raw rather than being rewritten.
func TestNonLocalOrdinalRidesRaw(t *testing.T) {
	mk := func(term string) rm.DVOrdinal {
		return rm.DVOrdinal{
			Symbol: rm.DVCodedText{
				DVText:       rm.DVText{Value: "mild"},
				DefiningCode: rm.CodePhrase{CodeString: "c1", TerminologyID: rm.TerminologyID{Value: term}},
			},
			Value: 1,
		}
	}
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", mk("SNOMED-CT"), "DV_ORDINAL", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|raw"]; !ok {
		t.Errorf("SNOMED-coded ordinal should ride |raw, got %#v", out)
	}
	out = map[string]any{}
	if err := leafToFlat(out, "p/x", mk("local"), "DV_ORDINAL", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|ordinal"]; !ok {
		t.Errorf("local-coded ordinal should keep the suffix form, got %#v", out)
	}
}

// TestPreferredTermRidesRaw: CODE_PHRASE.preferred_term has no suffix channel;
// a coded text carrying it must ride |raw, not silently drop it.
func TestPreferredTermRidesRaw(t *testing.T) {
	pt := "Preferred rubric"
	v := rm.DVCodedText{
		DVText:       rm.DVText{Value: "v"},
		DefiningCode: rm.CodePhrase{CodeString: "c", TerminologyID: rm.TerminologyID{Value: "openehr"}, PreferredTerm: &pt},
	}
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", v, "DV_CODED_TEXT", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected |raw for preferred_term-decorated coded text, got %#v", out)
	}
	dc, _ := raw["defining_code"].(map[string]any)
	if dc["preferred_term"] != pt {
		t.Errorf("|raw fragment lost preferred_term: %#v", dc)
	}
}

// TestClosedCodedTextEncodeErrors: a DV_TEXT at a closed DV_CODED_TEXT leaf has
// no decodable FLAT form (|other needs an open list; a bare value is rejected by
// the decode allowlist) — encode must fail loudly, not emit undecodable output.
func TestClosedCodedTextEncodeErrors(t *testing.T) {
	out := map[string]any{}
	err := leafToFlat(out, "p/x", rm.DVText{Value: "free"}, "DV_CODED_TEXT", false)
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("leafToFlat(DVText at closed coded leaf) err = %v, want ErrUnsupportedDatatype", err)
	}
	if len(out) != 0 {
		t.Errorf("errored encode wrote %d entries, want 0", len(out))
	}
}

// TestDecoratedTextAtCodedLeafStampsDynamicType: a decorated DV_TEXT at an open
// DV_CODED_TEXT leaf rides |raw stamped DV_TEXT (its dynamic type) — stamping
// the leaf type would make decode reconstruct a DV_CODED_TEXT with the text's
// fields silently dropped.
func TestDecoratedTextAtCodedLeafStampsDynamicType(t *testing.T) {
	fm := "markdown"
	v := rm.DVText{Value: "x", Formatting: &fm}
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", v, "DV_CODED_TEXT", true); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected |raw for decorated text, got %#v", out)
	}
	if raw["_type"] != "DV_TEXT" {
		t.Errorf("|raw _type = %v, want DV_TEXT (dynamic type)", raw["_type"])
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
