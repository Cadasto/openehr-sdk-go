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
	// `language` rather than `formatting`: the latter became a modelled suffix
	// with the optional-suffix set, so it no longer forces |raw.
	v := rm.DVText{Value: "x", Language: &rm.CodePhrase{CodeString: "de"}}
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

// TestQuantityDecoratedRaw checks a decorated value falls back to |raw rather
// than silently dropping the extra attribute. The decoration is `normal_range`:
// `magnitude_status` used to serve here but is a modelled suffix since the
// optional-suffix set landed, so it no longer forces the fallback.
func TestQuantityDecoratedRaw(t *testing.T) {
	out := map[string]any{}
	q := rm.DVQuantity{Magnitude: 1, Units: "mm", NormalRange: &rm.DVInterval[rm.DVQuantity]{}}
	if err := leafToFlat(out, "p/x", q, "DV_QUANTITY", false); err != nil {
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
	out := map[string]any{}
	// A decorated DV_COUNT rides |raw — decorated by normal_range, which stays
	// outside the modelled suffix set.
	c := rm.DVCount{Magnitude: 9007199254740993, NormalRange: &rm.DVInterval[rm.DVCount]{}}
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

// TestCodePhrasePreferredTermRidesRaw: a preferred_term is outside the
// |code/|terminology pair, so the value must ride |raw rather than be emitted
// with the term silently discarded — the same rule the nested CODE_PHRASE
// inside a DV_CODED_TEXT already follows.
func TestCodePhrasePreferredTermRidesRaw(t *testing.T) {
	pref := "English"
	out := map[string]any{}
	cp := rm.CodePhrase{CodeString: "en", PreferredTerm: &pref, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	if err := leafToFlat(out, "p/x", cp, "CODE_PHRASE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	raw, ok := out["p/x|raw"].(map[string]any)
	if !ok {
		t.Fatalf("expected p/x|raw, got %#v", out)
	}
	if raw["preferred_term"] != "English" {
		t.Errorf("|raw dropped preferred_term: %#v", raw)
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

// TestDateAccuracyRidesRaw: DV_DATE / DV_DATE_TIME / DV_TIME inherit from
// DV_ABSOLUTE_QUANTITY, where `accuracy` is a DV_DURATION object rather than a
// Real, so it has no scalar suffix form and must keep forcing |raw. Guards
// against `accuracy` being added to their capturedKeys by symmetry with
// DV_QUANTITY, which would emit an object where a number belongs.
func TestDateAccuracyRidesRaw(t *testing.T) {
	out := map[string]any{}
	d := rm.DVDate{Value: "2022-01-12", Accuracy: &rm.DVDuration{Value: "P1D"}}
	if err := leafToFlat(out, "p/x", d, "DV_DATE", false); err != nil {
		t.Fatalf("leafToFlat: %v", err)
	}
	if _, ok := out["p/x|raw"]; !ok {
		t.Errorf("DV_DATE with a DV_DURATION accuracy should ride |raw, got %#v", out)
	}
	if _, scalar := out["p/x|accuracy"]; scalar {
		t.Error("emitted a scalar |accuracy for a DV_DURATION-typed accuracy")
	}
}
