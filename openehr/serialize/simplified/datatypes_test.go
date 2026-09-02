package simplified

// REQ-053 — leaf datatype -> FLAT suffix mapping.
import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
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
		{
			// A standalone CODE_PHRASE leaf — ENTRY language / encoding, which the
			// reference emits in its own right rather than nested in a
			// DV_CODED_TEXT (PROBE-086).
			rmType: "CODE_PHRASE",
			v:      rm.CodePhrase{CodeString: "en", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}},
			check: func(t *testing.T, dv map[string]any) {
				tid, _ := dv["terminology_id"].(map[string]any)
				if dv["code_string"] != "en" || tid["value"] != "ISO_639-1" {
					t.Errorf("code phrase = %#v", dv)
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

// TestCodedTextAtTextLeafRidesSuffixes — REQ-053: the one substitution carried
// in suffix form. A fully-captured DV_CODED_TEXT stored at a DV_TEXT-typed
// leaf (legal RM substitution) emits the DV_CODED_TEXT suffix set — |code,
// |value, |terminology, |formatting — with no bare key and no |raw, matching
// the reference implementation's dv_coded_text_as_dv_text corpus shape.
// Decode re-selects the coded builder from |code at the DV_TEXT leaf, so the
// pair is inverse.
func TestCodedTextAtTextLeafRidesSuffixes(t *testing.T) {
	v := rm.DVCodedText{
		DVText:       rm.DVText{Value: "Radial styloid tenosynovitis", Formatting: new("plain")},
		DefiningCode: rm.CodePhrase{CodeString: "21794005", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
	}
	for _, form := range []struct {
		name string
		v    any
	}{{"value", v}, {"pointer", &v}} {
		t.Run(form.name, func(t *testing.T) {
			out := map[string]any{}
			if err := leafToFlat(out, "p/x", form.v, "DV_TEXT", false); err != nil {
				t.Fatalf("leafToFlat: %v", err)
			}
			want := map[string]any{
				"p/x|code":        "21794005",
				"p/x|value":       "Radial styloid tenosynovitis",
				"p/x|terminology": "SNOMED-CT",
				"p/x|formatting":  "plain",
			}
			if len(out) != len(want) {
				t.Fatalf("got %d entries %#v, want %d %#v", len(out), out, len(want), want)
			}
			for k, w := range want {
				if out[k] != w {
					t.Errorf("out[%q] = %#v, want %#v", k, out[k], w)
				}
			}
			// And back: |code at the DV_TEXT leaf selects the DV_CODED_TEXT builder.
			dv, err := dvFromSuffixes("DV_TEXT", false, suffixesOf(out, "p/x"))
			if err != nil {
				t.Fatalf("dvFromSuffixes: %v", err)
			}
			if dv["_type"] != "DV_CODED_TEXT" || dv["value"] != "Radial styloid tenosynovitis" || dv["formatting"] != "plain" {
				t.Errorf("decoded %#v, want a DV_CODED_TEXT with value + formatting", dv)
			}
			dc, _ := dv["defining_code"].(map[string]any)
			if dc["code_string"] != "21794005" {
				t.Errorf("decoded defining_code = %#v, want code_string 21794005", dv["defining_code"])
			}
			tid, _ := dc["terminology_id"].(map[string]any)
			if tid["value"] != "SNOMED-CT" {
				t.Errorf("decoded terminology = %#v, want SNOMED-CT", dc["terminology_id"])
			}
		})
	}
}

// TestDecoratedCodedTextAtTextLeaf: the carve-out admits only a value the wire
// can carry whole, and REQ-140 moved that line. `mappings` is now expressible as
// `_mapping:N` keys beside the coded suffixes — the shape the corpus's
// `dv_coded_text_as_dv_text` fixture actually carries — while an extra still
// outside the grammar (`hyperlink`, which the reference's suffix set has no
// channel for and no underscore family spells) keeps the substituted value on
// `|raw`, stamped with its dynamic type (REQ-053: never drop silently).
func TestDecoratedCodedTextAtTextLeaf(t *testing.T) {
	v := rm.DVCodedText{
		DVText: rm.DVText{
			Value: "Radial styloid tenosynovitis",
			Mappings: []rm.TermMapping{{
				Match:  "=",
				Target: rm.CodePhrase{CodeString: "21794005", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
			}},
		},
		DefiningCode: rm.CodePhrase{CodeString: "21794005", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
	}
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", v, "DV_TEXT", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, rode := out["p/x|raw"]; rode {
		t.Errorf("a mapping is now expressible and must not ride |raw: %#v", out)
	}
	for key, want := range map[string]any{
		"p/x|code":                          "21794005",
		"p/x|value":                         "Radial styloid tenosynovitis",
		"p/x/_mapping:0|match":              "=",
		"p/x/_mapping:0/target|code":        "21794005",
		"p/x/_mapping:0/target|terminology": "SNOMED-CT",
	} {
		if got := out[key]; got != want {
			t.Errorf("%s = %#v, want %#v (all keys: %#v)", key, got, want, out)
		}
	}

	raw := map[string]any{}
	v.Hyperlink = &rm.DVURI{Value: "http://example.test/x"}
	if err := leafToFlat(raw, "p/x", v, "DV_TEXT", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	frag, ok := raw["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected |raw for a hyperlink-decorated coded text, got %#v", raw)
	}
	if frag["_type"] != "DV_CODED_TEXT" {
		t.Errorf("|raw _type = %v, want DV_CODED_TEXT (dynamic type)", frag["_type"])
	}
	if _, has := frag["mappings"]; !has {
		t.Errorf("|raw fragment lost the mappings: %#v", frag)
	}
	if len(raw) != 1 {
		t.Errorf("|raw must ride alone — no `_` keys beside it, got %#v", raw)
	}
}

// TestCodedSuffixesAtTextLeafErrors — the fail-loud boundaries of the
// carve-out (REQ-053): |code selects the coded builder, which then follows
// DV_CODED_TEXT's own rules — |value is required, and the coded form has no
// bare spelling. Without |code the leaf stays a plain DV_TEXT, so a stray
// |value or |terminology is refused by the DV_TEXT allowlist as before.
func TestCodedSuffixesAtTextLeafErrors(t *testing.T) {
	for name, sfx := range map[string]map[string]any{
		"code without value":          {"code": "21794005"},
		"bare value beside code":      {"code": "21794005", "value": "v", "": "v"},
		"value without code":          {"value": "v"},
		"terminology without code":    {"": "v", "terminology": "SNOMED-CT"},
		"code with unmodelled suffix": {"code": "21794005", "value": "v", "mapping": "x"},
		"other is not a DV_TEXT form": {"other": "v"},
		"code beside other refused":   {"code": "21794005", "value": "v", "other": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := dvFromSuffixes("DV_TEXT", false, sfx); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("dvFromSuffixes(DV_TEXT, %v) err = %v, want ErrUnsupportedDatatype", sfx, err)
			}
		})
	}
	// A plain DV_TEXT leaf (bare value, optional |formatting) is untouched by
	// the discriminator.
	dv, err := dvFromSuffixes("DV_TEXT", false, map[string]any{"": "plain text"})
	if err != nil {
		t.Fatalf("plain DV_TEXT rejected: %v", err)
	}
	if dv["_type"] != "DV_TEXT" || dv["value"] != "plain text" {
		t.Errorf("plain DV_TEXT decode = %#v", dv)
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
	// `hyperlink` rather than `formatting` or `language`: the first became a
	// modelled suffix with the optional-suffix set and the second the REQ-140
	// `_language` member, so neither forces |raw any more.
	v := rm.DVText{Value: "x", Hyperlink: &rm.DVURI{Value: "http://example.test/x"}}
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

// TestNormalRangeNarrowsRawBoundary — REQ-140. The `|raw` boundary narrowed
// deliberately when the underscore families landed, and both sides of it are
// pinned here because the failure modes are opposite and both silent:
//
//   - a value whose only extras the underscore grammar now carries takes the
//     suffix form *plus* `_` keys, and must not fall back to `|raw` (which would
//     be a needless respelling of every decorated value on the wire);
//   - a value with an extra still outside the grammar — a `normal_status` coded
//     outside the implied openEHR terminology, the unchanged rule — rides one
//     `|raw` fragment and must emit **no** `_` keys, because the fragment already
//     carries the normal_range and spelling it twice would let the two disagree.
func TestNormalRangeNarrowsRawBoundary(t *testing.T) {
	normal := &rm.DVInterval[rm.DVQuantity]{Interval: rm.Interval[rm.DVQuantity]{
		Lower:         rm.DVQuantity{Magnitude: 1, Units: "mm"},
		Upper:         rm.DVQuantity{Magnitude: 9, Units: "mm"},
		LowerIncluded: true, UpperIncluded: true,
	}}

	suffixed := map[string]any{}
	q := rm.DVQuantity{Magnitude: 1, Units: "mm", NormalRange: normal}
	if err := leafToFlat(suffixed, "p/x", q, "DV_QUANTITY", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, rode := suffixed["p/x|raw"]; rode {
		t.Errorf("a normal_range is now expressible and must not ride |raw: %#v", suffixed)
	}
	for key, want := range map[string]any{
		"p/x|magnitude":                     1.0,
		"p/x|unit":                          "mm",
		"p/x/_normal_range/lower|magnitude": 1.0,
		"p/x/_normal_range/upper|magnitude": 9.0,
	} {
		if got := suffixed[key]; got != want {
			t.Errorf("%s = %#v, want %#v (all keys: %#v)", key, got, want, suffixed)
		}
	}
	// Closed endpoints are the decode default, so they are not spelled.
	for _, key := range []string{"p/x/_normal_range|lower_included", "p/x/_normal_range|upper_included"} {
		if _, spelled := suffixed[key]; spelled {
			t.Errorf("%s spelled although it carries the default", key)
		}
	}

	raw := map[string]any{}
	q.NormalStatus = &rm.CodePhrase{CodeString: "N", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}
	if err := leafToFlat(raw, "p/x", q, "DV_QUANTITY", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := raw["p/x|raw"]; !ok {
		t.Errorf("an extra outside the grammar must still ride |raw, got %#v", raw)
	}
	if len(raw) != 1 {
		t.Errorf("a |raw value must be the only entry, got %#v", raw)
	}
}

// TestRawFragmentPreservesLargeInteger checks a decorated DV_COUNT above 2^53
// keeps its magnitude exactly through the |raw path (json.Number, not float64).
func TestRawFragmentPreservesLargeInteger(t *testing.T) {
	out := map[string]any{}
	// A decorated DV_COUNT rides |raw — decorated by a normal_status coded outside
	// the openEHR terminology the bare `|normal_status` code implies, which is the
	// decoration that stays outside the modelled sets (REQ-140 narrowed the rest).
	c := rm.DVCount{
		Magnitude:    9007199254740993,
		NormalStatus: &rm.CodePhrase{CodeString: "N", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
	}
	if err := leafToFlat(out, "p/x", c, "DV_COUNT", false); err != nil {
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

// TestCodePhraseEmptyCodeSkipped: ENTRY language / encoding are non-pointer
// CODE_PHRASE fields, so an unset one arrives as a zero value with no pointer
// for the nil check to catch. Writing it anyway would put empty |code leaves on
// every composition whose metadata came in through the ctx/ forms.
func TestCodePhraseEmptyCodeSkipped(t *testing.T) {
	out := map[string]any{}
	if err := leafToFlat(out, "p/language", rm.CodePhrase{}, "CODE_PHRASE", false); err != nil {
		t.Fatalf("leafToFlat(zero CODE_PHRASE): %v", err)
	}
	if len(out) != 0 {
		t.Errorf("zero CODE_PHRASE wrote %v, want no entries", out)
	}
}

// TestCodePhraseTerminologyOptional: the encoder omits |terminology for an
// empty TERMINOLOGY_ID, so the decoder must accept a |code-only leaf rather
// than demand a suffix its own encoder never wrote.
func TestCodePhraseTerminologyOptional(t *testing.T) {
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", rm.CodePhrase{CodeString: "UTF-8"}, "CODE_PHRASE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|terminology"]; ok {
		t.Errorf("empty TERMINOLOGY_ID wrote a |terminology suffix: %v", out)
	}
	dv, err := dvFromSuffixes("CODE_PHRASE", false, suffixesOf(out, "p/x"))
	if err != nil {
		t.Fatalf("dvFromSuffixes: %v", err)
	}
	if dv["code_string"] != "UTF-8" {
		t.Errorf("code_string = %#v", dv["code_string"])
	}
	if _, ok := dv["terminology_id"]; ok {
		t.Errorf("absent |terminology rebuilt a terminology_id: %#v", dv)
	}
}

// TestCodePhraseMissingCodeErrors: |code is what makes a CODE_PHRASE a code, so
// its absence must fail loudly rather than rebuild an empty-coded value.
func TestCodePhraseMissingCodeErrors(t *testing.T) {
	_, err := dvFromSuffixes("CODE_PHRASE", false, map[string]any{"terminology": "ISO_639-1"})
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
	}
}

// TestCodePhrasePreferredTermRidesSuffix — REQ-140. A **standalone** CODE_PHRASE
// leaf has a `|preferred_term` suffix (the corpus writes it at
// `dv_text/_language|preferred_term`), so the attribute no longer forces |raw
// there. The **nested** spelling — a DV_CODED_TEXT's defining_code, whose triple
// is |code+|value+|terminology — still has no channel for it and still rides |raw,
// which is the asymmetry this test pins in both directions.
func TestCodePhrasePreferredTermRidesSuffix(t *testing.T) {
	pref := "English"
	cp := rm.CodePhrase{CodeString: "en", PreferredTerm: &pref, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}

	out := map[string]any{}
	if err := leafToFlat(out, "p/x", cp, "CODE_PHRASE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, rode := out["p/x|raw"]; rode {
		t.Errorf("a standalone CODE_PHRASE's preferred_term is a suffix and must not ride |raw: %#v", out)
	}
	if out["p/x|preferred_term"] != "English" {
		t.Errorf("|preferred_term = %#v (all keys: %#v)", out["p/x|preferred_term"], out)
	}

	nested := map[string]any{}
	coded := rm.DVCodedText{DVText: rm.DVText{Value: "English"}, DefiningCode: cp}
	if err := leafToFlat(nested, "p/y", coded, "DV_CODED_TEXT", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := nested["p/y|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected p/y|raw for a defining_code carrying a preferred_term, got %#v", nested)
	}
	dc, _ := raw["defining_code"].(map[string]any)
	if dc["preferred_term"] != "English" {
		t.Errorf("|raw dropped the nested preferred_term: %#v", raw)
	}
}

// TestCodePhraseCodelessRidesRaw: an empty code_string makes
// [codePhraseToFlat] write nothing at all, so a value that still carries a
// terminology or a preferred_term must ride |raw — encoding it to nothing would
// lose the rest of the value.
func TestCodePhraseCodelessRidesRaw(t *testing.T) {
	pref := "English"
	for name, cp := range map[string]rm.CodePhrase{
		"terminology only":    {TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}},
		"preferred term only": {PreferredTerm: &pref},
	} {
		t.Run(name, func(t *testing.T) {
			out := map[string]any{}
			if err := leafToFlat(out, "p/x", cp, "CODE_PHRASE", false); err != nil {
				t.Fatalf("leafToFlat: %v", err)
			}
			if _, ok := out["p/x|raw"]; !ok {
				t.Errorf("expected p/x|raw, got %#v", out)
			}
		})
	}
}

// TestOptionalOrderedSuffixesRoundTrip: the optional attributes a value leaf
// inherits from DV_ORDERED / DV_QUANTIFIED / DV_AMOUNT survive encode and decode
// as suffixes. Before they were modelled, a value carrying any of them rode
// |raw as a whole — the PROBE-086 corpus emits them beside ordinary values.
func TestOptionalOrderedSuffixesRoundTrip(t *testing.T) {
	status, pct, acc, prec := "~", true, rm.Real(50.5), rm.Integer(1)
	sys, disp := "units_system", "units_display_name"
	normal := &rm.CodePhrase{CodeString: "N", TerminologyID: rm.TerminologyID{Value: "openehr"}}

	tests := []struct {
		rmType string
		v      any
		want   map[string]any
	}{
		{
			rmType: "DV_QUANTITY",
			v: rm.DVQuantity{
				Magnitude: 65.9, Units: "unit",
				MagnitudeStatus: &status, NormalStatus: normal,
				Accuracy: &acc, AccuracyIsPercent: &pct, Precision: &prec,
				UnitsSystem: &sys, UnitsDisplayName: &disp,
			},
			want: map[string]any{
				"p/x|magnitude": 65.9, "p/x|unit": "unit",
				"p/x|magnitude_status": "~", "p/x|normal_status": "N",
				"p/x|accuracy": 50.5, "p/x|accuracy_is_percent": true,
				"p/x|precision":    int64(1),
				"p/x|units_system": "units_system", "p/x|units_display_name": "units_display_name",
			},
		},
		{
			rmType: "DV_COUNT",
			v: rm.DVCount{
				Magnitude: 7, MagnitudeStatus: &status, NormalStatus: normal,
				Accuracy: &acc, AccuracyIsPercent: &pct,
			},
			want: map[string]any{
				"p/x": int64(7), "p/x|magnitude_status": "~", "p/x|normal_status": "N",
				"p/x|accuracy": 50.5, "p/x|accuracy_is_percent": true,
			},
		},
		{
			rmType: "DV_PROPORTION",
			v: rm.DVProportion{
				Numerator: 20.5, Denominator: 12.4, Type: 0,
				MagnitudeStatus: &status, NormalStatus: normal,
				Accuracy: &acc, AccuracyIsPercent: &pct, Precision: &prec,
			},
			want: map[string]any{
				"p/x|numerator": 20.5, "p/x|denominator": 12.4, "p/x|type": int64(0),
				"p/x|magnitude_status": "~", "p/x|normal_status": "N",
				"p/x|accuracy": 50.5, "p/x|accuracy_is_percent": true, "p/x|precision": int64(1),
			},
		},
		{
			rmType: "DV_DATE",
			v:      rm.DVDate{Value: "2022-01-12", MagnitudeStatus: &status, NormalStatus: normal},
			want: map[string]any{
				"p/x": "2022-01-12", "p/x|magnitude_status": "~", "p/x|normal_status": "N",
			},
		},
		{
			rmType: "DV_TEXT",
			v:      rm.DVText{Value: "DV_TEXT value", Formatting: new("plain")},
			want:   map[string]any{"p/x": "DV_TEXT value", "p/x|formatting": "plain"},
		},
		{
			// DV_DURATION inherits accuracy from DV_AMOUNT as a Real, so unlike the
			// DV_TEMPORAL types (see TestDateAccuracyRidesRaw) it has a scalar suffix
			// form and must not fall back to |raw.
			rmType: "DV_DURATION",
			v: rm.DVDuration{
				Value: "PT1H30M", MagnitudeStatus: &status, NormalStatus: normal,
				Accuracy: &acc, AccuracyIsPercent: &pct,
			},
			want: map[string]any{
				"p/x": "PT1H30M", "p/x|magnitude_status": "~", "p/x|normal_status": "N",
				"p/x|accuracy": 50.5, "p/x|accuracy_is_percent": true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.rmType, func(t *testing.T) {
			out := map[string]any{}
			if err := leafToFlat(out, "p/x", tc.v, tc.rmType, false); err != nil {
				t.Fatalf("leafToFlat: %v", err)
			}
			if _, rode := out["p/x|raw"]; rode {
				t.Fatalf("value rode |raw; the suffixes should now capture it: %#v", out)
			}
			if len(out) != len(tc.want) {
				t.Fatalf("got %d entries %#v, want %d %#v", len(out), out, len(tc.want), tc.want)
			}
			for k, w := range tc.want {
				if out[k] != w {
					t.Errorf("out[%q] = %#v, want %#v", k, out[k], w)
				}
			}
			// And back: every emitted suffix must rebuild its RM attribute. Asserted
			// per suffix rather than on magnitude_status alone — a rebuild that
			// dropped, say, units_system would otherwise pass while losing data.
			dv, err := dvFromSuffixes(tc.rmType, false, suffixesOf(out, "p/x"))
			if err != nil {
				t.Fatalf("dvFromSuffixes: %v", err)
			}
			// suffix -> (canonical attribute, value the rebuild must carry)
			for suffix, exp := range map[string]struct {
				attr string
				want any
			}{
				"magnitude_status":    {"magnitude_status", "~"},
				"accuracy":            {"accuracy", 50.5},
				"accuracy_is_percent": {"accuracy_is_percent", true},
				"precision":           {"precision", int64(1)},
				"units_system":        {"units_system", "units_system"},
				"units_display_name":  {"units_display_name", "units_display_name"},
				"formatting":          {"formatting", "plain"},
			} {
				if _, emitted := tc.want["p/x|"+suffix]; !emitted {
					continue
				}
				if dv[exp.attr] != exp.want {
					t.Errorf("decoded %s = %#v, want %#v", exp.attr, dv[exp.attr], exp.want)
				}
			}
			if _, expected := tc.want["p/x|normal_status"]; expected {
				ns, _ := dv["normal_status"].(map[string]any)
				if ns["code_string"] != "N" {
					t.Errorf("decoded normal_status = %#v, want code_string N", dv["normal_status"])
				}
				tid, _ := ns["terminology_id"].(map[string]any)
				if tid["value"] != "openehr" {
					t.Errorf("decoded normal_status terminology = %#v, want openehr", ns["terminology_id"])
				}
			}
		})
	}
}

// TestUndecoratedValueUnchangedBySuffixSet: the optional suffixes are written
// only when present, so a plain value's FLAT form is byte-identical to what the
// codec emitted before they were modelled. This is what makes the change
// additive for the common case.
func TestUndecoratedValueUnchangedBySuffixSet(t *testing.T) {
	out := map[string]any{}
	if err := leafToFlat(out, "p/x", rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}, "DV_QUANTITY", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	want := map[string]any{"p/x|magnitude": float64(120), "p/x|unit": "mm[Hg]"}
	if len(out) != len(want) {
		t.Fatalf("undecorated quantity emitted %#v, want exactly %#v", out, want)
	}
	for k, w := range want {
		if out[k] != w {
			t.Errorf("out[%q] = %#v, want %#v", k, out[k], w)
		}
	}
}

// TestNonOpenehrNormalStatusRidesRaw: |normal_status carries a bare code and
// decode rebuilds it in the implied openEHR terminology, so a status coded
// elsewhere would be silently re-terminologised. It rides |raw instead — the
// same rule the non-local DV_ORDINAL symbol follows.
func TestNonOpenehrNormalStatusRidesRaw(t *testing.T) {
	out := map[string]any{}
	q := rm.DVQuantity{
		Magnitude: 1, Units: "mm",
		NormalStatus: &rm.CodePhrase{CodeString: "N", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
	}
	if err := leafToFlat(out, "p/x", q, "DV_QUANTITY", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|raw"]; !ok {
		t.Errorf("non-openehr normal_status should ride |raw, got %#v", out)
	}
}

// TestOptionalSuffixAbsentStaysAbsent: an omitted optional suffix must not
// become a zero value in the canonical RM — "unset" and "zero" are different
// clinical claims (a magnitude_status of "" is not the same as none).
func TestOptionalSuffixAbsentStaysAbsent(t *testing.T) {
	dv, err := dvFromSuffixes("DV_QUANTITY", false, map[string]any{"magnitude": 1.0, "unit": "mm"})
	if err != nil {
		t.Fatalf("dvFromSuffixes: %v", err)
	}
	for _, attr := range []string{"magnitude_status", "normal_status", "accuracy", "accuracy_is_percent", "precision"} {
		if _, present := dv[attr]; present {
			t.Errorf("absent |%s materialised as %#v", attr, dv[attr])
		}
	}
}

// TestFormattedTextAtOpenCodedLeafRidesRaw — regression, PR #86 review.
// `formatting` joined capturedKeys["DV_CODED_TEXT"] with the optional-suffix
// set, which made capturedFully accept a *formatted* DV_TEXT at an open coded
// leaf; emitText then wrote |other alone and discarded the formatting. |other
// carries the value by itself, so the value has to ride |raw.
func TestFormattedTextAtOpenCodedLeafRidesRaw(t *testing.T) {
	out := map[string]any{}
	v := rm.DVText{Value: "free text", Formatting: new("markdown")}
	if err := leafToFlat(out, "p/x", v, "DV_CODED_TEXT", true); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("formatting silently dropped — expected |raw, got %#v", out)
	}
	if raw["formatting"] != "markdown" {
		t.Errorf("|raw lost the formatting: %#v", raw)
	}
	if _, leaked := out["p/x|other"]; leaked {
		t.Errorf("emitted |other beside |raw: %#v", out)
	}
	// An *un*formatted DV_TEXT at the same leaf must still take the |other form.
	plain := map[string]any{}
	if err := leafToFlat(plain, "p/x", rm.DVText{Value: "free text"}, "DV_CODED_TEXT", true); err != nil {
		t.Fatalf("leafToFlat(plain): %v", err)
	}
	if plain["p/x|other"] != "free text" || len(plain) != 1 {
		t.Errorf("undecorated DV_TEXT at open coded leaf = %#v, want just |other", plain)
	}
}

// TestMappingBesideOtherFreeText — REQ-140. The fourth DV_TEXT shape the codec
// carries at a leaf: a DV_TEXT at an *open* DV_CODED_TEXT leaf rides `|other`,
// which carries the value alone — but `_mapping:N` is a key of its own, not a
// companion suffix, so the two compose. Unit-level because the PROBE-086 corpus
// template constrains no open value-set leaf.
func TestMappingBesideOtherFreeText(t *testing.T) {
	out := map[string]any{}
	v := rm.DVText{
		Value: "free text",
		Mappings: []rm.TermMapping{{
			Match:  "=",
			Target: rm.CodePhrase{CodeString: "21794005", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
		}},
	}
	if err := leafToFlat(out, "p/x", v, "DV_CODED_TEXT", true); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if out["p/x|other"] != "free text" {
		t.Errorf("|other = %#v, want the free text", out["p/x|other"])
	}
	if out["p/x/_mapping:0|match"] != "=" {
		t.Errorf("the mapping was dropped beside |other: %#v", out)
	}
	if _, rode := out["p/x|raw"]; rode {
		t.Errorf("|other + _mapping is expressible and must not ride |raw: %#v", out)
	}
	// Decode is the inverse at both positions: |other alone rebuilds the DV_TEXT…
	dv, err := dvFromSuffixes("DV_CODED_TEXT", true, map[string]any{"other": "free text"})
	if err != nil {
		t.Fatalf("dvFromSuffixes: %v", err)
	}
	if dv["_type"] != "DV_TEXT" {
		t.Errorf("|other rebuilt a %v, want DV_TEXT", dv["_type"])
	}
	// …and the family lands on it, judged against the leaf type, which inherits
	// `mappings` from DV_TEXT.
	if _, declared := rminfo.Default.AttributeRMType("DV_CODED_TEXT", "mappings"); !declared {
		t.Error("DV_CODED_TEXT no longer declares mappings — the router would refuse the family")
	}
}

// TestOtherRejectsCompanionSuffixes — regression, PR #86 review. The |other
// rebuild returns a bare DV_TEXT, so any companion suffix the allowlist admits
// would be accepted and then dropped. |other is mutually exclusive with every
// other suffix, not just |code.
func TestOtherRejectsCompanionSuffixes(t *testing.T) {
	for _, sfx := range []map[string]any{
		{"other": "x", "formatting": "markdown"},
		{"other": "x", "code": "c", "value": "v"},
		{"other": "x", "terminology": "local"},
	} {
		_, err := dvFromSuffixes("DV_CODED_TEXT", true, sfx)
		if !errors.Is(err, ErrUnsupportedDatatype) {
			t.Errorf("dvFromSuffixes(%v) err = %v, want ErrUnsupportedDatatype", sfx, err)
		}
	}
	// |other alone still decodes to a DV_TEXT.
	dv, err := dvFromSuffixes("DV_CODED_TEXT", true, map[string]any{"other": "x"})
	if err != nil {
		t.Fatalf("bare |other rejected: %v", err)
	}
	if dv["_type"] != "DV_TEXT" || dv["value"] != "x" {
		t.Errorf("|other alone = %#v, want DV_TEXT", dv)
	}
}

// TestTerminologyOnlyCodePhraseRidesRaw — regression, PR #86 review round 3. A
// CODE_PHRASE with a terminology but no code encoded to *nothing*: the empty-code
// skip in codePhraseToFlat fired, and capturedFully still called the value
// captured, so the |raw backstop never engaged. The all-zero value must keep
// skipping (TestCodePhraseEmptyCodeSkipped — it is what a ctx/-decoded
// composition's ENTRY language / encoding hold), but a partly-populated one has to
// ride |raw losslessly, as a preferred_term already does.
func TestTerminologyOnlyCodePhraseRidesRaw(t *testing.T) {
	out := map[string]any{}
	cp := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	if err := leafToFlat(out, "p/x", cp, "CODE_PHRASE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("terminology-only CODE_PHRASE encoded to %#v, want a |raw fragment", out)
	}
	if raw["_type"] != "CODE_PHRASE" {
		t.Errorf("|raw _type = %v, want CODE_PHRASE", raw["_type"])
	}
	tid, _ := raw["terminology_id"].(map[string]any)
	if tid["value"] != "ISO_639-1" {
		t.Errorf("|raw lost the terminology: %#v", raw)
	}
	// And back: |raw decodes to the same canonical fragment, so the value survives
	// the round-trip rather than being reconstructed from suffixes it never had.
	dv, err := dvFromSuffixes("CODE_PHRASE", false, suffixesOf(out, "p/x"))
	if err != nil {
		t.Fatalf("dvFromSuffixes: %v", err)
	}
	if dv["_type"] != "CODE_PHRASE" {
		t.Errorf("_type = %v, want CODE_PHRASE", dv["_type"])
	}
	if cs, _ := dv["code_string"].(string); cs != "" {
		t.Errorf("code_string = %q, want empty (none was encoded)", cs)
	}
	tid, _ = dv["terminology_id"].(map[string]any)
	if tid["value"] != "ISO_639-1" {
		t.Errorf("decoded terminology_id = %#v, want ISO_639-1", dv["terminology_id"])
	}
}

// TestCodedTextWithFormattingRoundTrips covers the third corner of the
// |other/formatting family: |other alone and a formatted DV_TEXT at an open coded
// leaf are pinned above, but a *genuine* DV_CODED_TEXT carrying `formatting`
// alongside its defining_code must keep the suffix form — both the code and the
// decoration survive, and nothing falls back to |raw.
func TestCodedTextWithFormattingRoundTrips(t *testing.T) {
	v := rm.DVCodedText{
		DVText:       rm.DVText{Value: "event", Formatting: new("markdown")},
		DefiningCode: rm.CodePhrase{CodeString: "433", TerminologyID: rm.TerminologyID{Value: "openehr"}},
	}
	for _, listOpen := range []bool{false, true} {
		out := map[string]any{}
		if err := leafToFlat(out, "p/x", v, "DV_CODED_TEXT", listOpen); err != nil {
			t.Fatalf("leafToFlat(listOpen=%v): %v", listOpen, err)
		}
		want := map[string]any{
			"p/x|code": "433", "p/x|value": "event",
			"p/x|terminology": "openehr", "p/x|formatting": "markdown",
		}
		if len(out) != len(want) {
			t.Fatalf("listOpen=%v: got %#v, want exactly %#v", listOpen, out, want)
		}
		for k, w := range want {
			if out[k] != w {
				t.Errorf("listOpen=%v: out[%q] = %#v, want %#v", listOpen, k, out[k], w)
			}
		}
		dv, err := dvFromSuffixes("DV_CODED_TEXT", listOpen, suffixesOf(out, "p/x"))
		if err != nil {
			t.Fatalf("dvFromSuffixes(listOpen=%v): %v", listOpen, err)
		}
		if dv["_type"] != "DV_CODED_TEXT" || dv["formatting"] != "markdown" {
			t.Errorf("listOpen=%v: decoded %#v, want DV_CODED_TEXT with formatting", listOpen, dv)
		}
		dc, _ := dv["defining_code"].(map[string]any)
		if dc["code_string"] != "433" {
			t.Errorf("listOpen=%v: decoded defining_code = %#v", listOpen, dv["defining_code"])
		}
	}
}

// TestOrderedSuffixWrongKindRejected — PR #86 review round 3. The pass-through
// suffixes hand their value to canjson untouched, so a malformed one used to
// surface as a bare canjson error naming the canonical path and no FLAT key at
// all. The kind is checked while the key is still in hand (see
// TestMalformedOrderedSuffixNamesFlatKey for the surfaced message). No gap
// sentinel: a malformed value is a payload defect, not a modelled-gap refusal.
func TestOrderedSuffixWrongKindRejected(t *testing.T) {
	base := map[string]any{"magnitude": 1.0, "unit": "mm"}
	for _, tc := range []struct {
		suffix string
		bad    any
		want   string
	}{
		{"magnitude_status", 1.0, "string"},
		{"accuracy", "abc", "number"},
		{"accuracy_is_percent", "yes", "boolean"},
		{"precision", true, "number"},
		{"units_system", 1.0, "string"},
		{"units_display_name", false, "string"},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			sfx := maps.Clone(base)
			sfx[tc.suffix] = tc.bad
			_, err := dvFromSuffixes("DV_QUANTITY", false, sfx)
			if err == nil {
				t.Fatalf("|%s = %#v accepted", tc.suffix, tc.bad)
			}
			for _, want := range []string{"|" + tc.suffix, tc.want} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if errors.Is(err, ErrUnknownPath) || errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v carries a modelled-gap sentinel; a malformed value is a payload defect", err)
			}
			// The well-formed value of the same kind still passes through.
			sfx[tc.suffix] = map[string]any{
				"string": any("~"), "number": any(1.5), "boolean": any(true),
			}[tc.want]
			if _, err := dvFromSuffixes("DV_QUANTITY", false, sfx); err != nil {
				t.Errorf("well-formed |%s rejected: %v", tc.suffix, err)
			}
		})
	}
	// |formatting rides the same table on DV_TEXT.
	if _, err := dvFromSuffixes("DV_TEXT", false, map[string]any{"": "x", "formatting": 1.0}); err == nil {
		t.Error("|formatting = 1.0 accepted, want a kind error")
	}
	// json.Number is the kind a decoded body actually carries.
	if _, err := dvFromSuffixes("DV_QUANTITY", false, map[string]any{
		"magnitude": json.Number("1"), "unit": "mm", "accuracy": json.Number("0.5"),
	}); err != nil {
		t.Errorf("json.Number |accuracy rejected: %v", err)
	}
	// The check adds no strictness of its own: canjson parses a *quoted* number
	// into Real / Integer, so a producer that quotes every scalar must keep
	// decoding. Only a string that is no number at all is refused.
	for _, sfx := range []map[string]any{
		{"magnitude": 1.0, "unit": "mm", "accuracy": "0.5"},
		{"magnitude": 1.0, "unit": "mm", "precision": "2"},
	} {
		if _, err := dvFromSuffixes("DV_QUANTITY", false, sfx); err != nil {
			t.Errorf("quoted number rejected (canjson accepts it): %v in %v", err, sfx)
		}
	}
}

// TestDateAccuracyRidesUnderscoreFamily — REQ-140. DV_DATE / DV_DATE_TIME /
// DV_TIME inherit from DV_TEMPORAL, which redefines `accuracy` as a DV_DURATION
// object (its parent DV_ABSOLUTE_QUANTITY declares a DV_AMOUNT) rather than the
// Real that DV_AMOUNT — and so DV_QUANTITY, DV_COUNT, DV_DURATION,
// DV_PROPORTION — carries. It therefore has no scalar suffix and rides the
// `_accuracy` family (the reference's spelling), which forced the whole value onto
// |raw until REQ-140 Phase C3. The scalar `|accuracy` must still not appear, which
// is what a `capturedKeys` entry added by symmetry with DV_QUANTITY would produce.
func TestDateAccuracyRidesUnderscoreFamily(t *testing.T) {
	out := map[string]any{}
	d := rm.DVDate{Value: "2022-01-12", Accuracy: &rm.DVDuration{Value: "P1D"}}
	if err := leafToFlat(out, "p/x", d, "DV_DATE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|raw"]; ok {
		t.Errorf("a DV_DURATION accuracy is now expressible and must not ride |raw: %#v", out)
	}
	if out["p/x/_accuracy"] != "P1D" {
		t.Errorf("p/x/_accuracy = %#v (all keys: %#v)", out["p/x/_accuracy"], out)
	}
	if _, scalar := out["p/x|accuracy"]; scalar {
		t.Error("emitted a scalar |accuracy for a DV_DURATION-typed accuracy")
	}
}

// TestEmptyCodeDoesNotPromoteTextLeaf — REQ-053. The DV_CODED_TEXT-at-DV_TEXT
// discriminator is a non-empty string |code, not the key's mere presence.
// `|code: ""` / `|code: null` is what a form emits for "free text, no code
// selected"; promoting those would mint a DV_CODED_TEXT whose
// CODE_PHRASE.code_string is empty — RM-invalid, stable enough on re-encode that
// nothing downstream flags it, and matched by any AQL predicate testing
// defining_code/code_string against ”. They stay a plain DV_TEXT, so the stray
// |code is refused by the allowlist exactly as it was before the carve-out
// (PR #88 review).
func TestEmptyCodeDoesNotPromoteTextLeaf(t *testing.T) {
	for name, sfx := range map[string]map[string]any{
		"empty string code":              {"code": "", "value": "free text"},
		"null code":                      {"code": nil, "value": "free text"},
		"empty code beside a bare value": {"code": "", "": "free text"},
	} {
		t.Run(name, func(t *testing.T) {
			dv, err := dvFromSuffixes("DV_TEXT", false, sfx)
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("dvFromSuffixes(DV_TEXT, %v) = %#v, err = %v; want ErrUnsupportedDatatype (must not promote to DV_CODED_TEXT)", sfx, dv, err)
			}
		})
	}
}
