package simplified

// REQ-140 — the value-decoration underscore families: `_normal_range` and
// `_other_reference_ranges:N` on a DV_ORDERED leaf, `_mapping:N` on a DV_TEXT /
// DV_CODED_TEXT leaf, and the two ELEMENT attributes that stand in for an absent
// value (`_null_flavour`, `_null_reason`).
//
// Every fixture below is copied from the pinned PROBE-086 corpus bodies
// (`ehrbase_conformance_data_types_dv_*.json`,
// `ehrbase_conformance_Element_null_flavor.json`), so the spellings under test
// are the reference implementation's (ADR 0014, design constraint 6).

import (
	"cmp"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// --- _normal_range ------------------------------------------------------

// TestRMAttrNormalRangeQuantity — REQ-140. The DV_QUANTITY anchor, exactly as
// `ehrbase_conformance_data_types_dv_quantity.json` spells it: `/lower` and
// `/upper` carry the anchor datatype's own suffix form, and the boundary
// booleans ride the family instance itself.
func TestRMAttrNormalRangeQuantity(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrElement + "/_normal_range/lower|magnitude": 20.5,
		rmattrElement + "/_normal_range/lower|unit":      "unit",
		rmattrElement + "/_normal_range/upper|magnitude": 66.6,
		rmattrElement + "/_normal_range/upper|unit":      "unit",
		rmattrElement + "/_normal_range|lower_included":  false,
		rmattrElement + "/_normal_range|upper_included":  false,
	})
	q := elementValue[rm.DVQuantity](t, comp)
	if q.NormalRange == nil {
		t.Fatal("normal_range not decoded")
	}
	if got := q.NormalRange.Lower.Magnitude; got != 20.5 {
		t.Errorf("normal_range.lower.magnitude = %v, want 20.5", got)
	}
	if got := q.NormalRange.Upper.Units; got != "unit" {
		t.Errorf("normal_range.upper.units = %q", got)
	}
	if q.NormalRange.LowerIncluded || q.NormalRange.UpperIncluded {
		t.Errorf("explicit |*_included false did not survive: %+v", q.NormalRange.Interval)
	}
	if q.NormalRange.LowerUnbounded || q.NormalRange.UpperUnbounded {
		t.Errorf("absent |*_unbounded materialised as true: %+v", q.NormalRange.Interval)
	}
}

// TestRMAttrNormalRangeAnchors — REQ-140. `/lower` and `/upper` are decoded and
// emitted by the anchor datatype's own captured-key machinery, so every
// DV_ORDERED leaf type the codec maps carries its own bound spelling: a bare
// value for DV_COUNT / the temporal types, `|code`+`|value`+`|ordinal` for
// DV_ORDINAL, `|numerator`+`|denominator`+`|type` for DV_PROPORTION. All four
// shapes are corpus-verified.
func TestRMAttrNormalRangeAnchors(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, keys := range map[string]map[string]any{
		"DV_COUNT bare bound": {
			rmattrCount:                          7,
			rmattrCount + "/_normal_range/lower": 1,
			rmattrCount + "/_normal_range/upper": 8,
		},
		"DV_DATE_TIME bare bound": {
			rmattrDateTime:                          "2022-01-12T13:22:34.000868+01:00",
			rmattrDateTime + "/_normal_range/lower": "2022-01-12T13:22:34.000868+01:00",
			rmattrDateTime + "/_normal_range/upper": "2022-02-12T13:22:34.000868+01:00",
		},
		"DV_ORDINAL suffixed bound": {
			rmattrOrdinal + "|code":                        "at0015",
			rmattrOrdinal + "|value":                       "value1",
			rmattrOrdinal + "|ordinal":                     1,
			rmattrOrdinal + "/_normal_range/lower|code":    "at0015",
			rmattrOrdinal + "/_normal_range/lower|value":   "value1",
			rmattrOrdinal + "/_normal_range/lower|ordinal": 1,
			rmattrOrdinal + "/_normal_range/upper|code":    "at0016",
			rmattrOrdinal + "/_normal_range/upper|value":   "value2",
			rmattrOrdinal + "/_normal_range/upper|ordinal": 2,
		},
		"DV_PROPORTION suffixed bound": {
			rmattrProportion + "|numerator":                       20.5,
			rmattrProportion + "|denominator":                     12.4,
			rmattrProportion + "|type":                            0,
			rmattrProportion + "/_normal_range/lower|numerator":   20.5,
			rmattrProportion + "/_normal_range/lower|denominator": 12.4,
			rmattrProportion + "/_normal_range/lower|type":        0,
			rmattrProportion + "/_normal_range/upper|numerator":   25.5,
			rmattrProportion + "/_normal_range/upper|denominator": 12.4,
			rmattrProportion + "/_normal_range/upper|type":        0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assertRMAttrRoundTrip(t, wt, keys)
		})
	}
}

// TestRMAttrIntervalBoundaryDefaults — REQ-140. The RM declares
// `Interval.lower_included` / `upper_included` optional while the SDK's
// generated `Interval` carries a mandatory Boolean, so the codec fixes the
// mapping in one place: an absent `|*_included` is the closed endpoint (true)
// and is not re-emitted; an absent `|*_unbounded` is false. Both directions are
// pinned here because the corpus writes both shapes — `dv_count`'s
// `_normal_range` omits the flags, `dv_quantity`'s spells them false.
func TestRMAttrIntervalBoundaryDefaults(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrCount:                          7,
		rmattrCount + "/_normal_range/lower": 1,
		rmattrCount + "/_normal_range/upper": 8,
	})
	c := elementValue[rm.DVCount](t, comp)
	if c.NormalRange == nil {
		t.Fatal("normal_range not decoded")
	}
	if !c.NormalRange.LowerIncluded || !c.NormalRange.UpperIncluded {
		t.Errorf("absent |*_included must decode as the closed endpoint, got %+v", c.NormalRange.Interval)
	}
	if c.NormalRange.LowerUnbounded || c.NormalRange.UpperUnbounded {
		t.Errorf("absent |*_unbounded must decode as false, got %+v", c.NormalRange.Interval)
	}
}

// TestRMAttrIntervalUnboundedEnd — REQ-140. An absent bound is the unbounded
// end, flagged by `|upper_unbounded` — the corpus's
// `_other_reference_ranges:0` shape on `dv_quantity` and `dv_ordinal`. Encode
// must write no `/upper` suffix at all for it: there is no bound to spell.
func TestRMAttrIntervalUnboundedEnd(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrElement + "/_other_reference_ranges:0/lower|magnitude":     70.5,
		rmattrElement + "/_other_reference_ranges:0/lower|unit":          "unit",
		rmattrElement + "/_other_reference_ranges:0/meaning|code":        "260360000",
		rmattrElement + "/_other_reference_ranges:0/meaning|terminology": "SNOMED-CT",
		rmattrElement + "/_other_reference_ranges:0/meaning|value":       "very high",
		rmattrElement + "/_other_reference_ranges:0|upper_included":      false,
		rmattrElement + "/_other_reference_ranges:0|upper_unbounded":     true,
	})
	q := elementValue[rm.DVQuantity](t, comp)
	if len(q.OtherReferenceRanges) != 1 {
		t.Fatalf("other_reference_ranges = %d, want 1", len(q.OtherReferenceRanges))
	}
	rr := q.OtherReferenceRanges[0]
	if !rr.Range.UpperUnbounded {
		t.Error("upper_unbounded did not survive")
	}
	if rr.Range.Upper != nil {
		t.Errorf("unbounded end carries a bound: %#v", rr.Range.Upper)
	}
}

// --- _other_reference_ranges:N -----------------------------------------

// TestRMAttrOtherReferenceRangesRoundTrip — REQ-140. REFERENCE_RANGE is the
// interval grammar plus `/meaning`, and the corpus **elides** the RM's
// intervening `range` level: `lower` / `upper` / the boundary booleans sit
// directly under `_other_reference_ranges:N`. Two instances, one with a coded
// meaning and an unbounded upper end, one with an unbounded lower end — the
// `dv_quantity` fixture's exact shape.
func TestRMAttrOtherReferenceRangesRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrElement + "/_other_reference_ranges:0/lower|magnitude":     70.5,
		rmattrElement + "/_other_reference_ranges:0/lower|unit":          "unit",
		rmattrElement + "/_other_reference_ranges:0/meaning|code":        "260360000",
		rmattrElement + "/_other_reference_ranges:0/meaning|terminology": "SNOMED-CT",
		rmattrElement + "/_other_reference_ranges:0/meaning|value":       "very high",
		rmattrElement + "/_other_reference_ranges:0|upper_included":      false,
		rmattrElement + "/_other_reference_ranges:0|upper_unbounded":     true,
		rmattrElement + "/_other_reference_ranges:1/meaning|code":        "260360000",
		rmattrElement + "/_other_reference_ranges:1/meaning|terminology": "SNOMED-CT",
		rmattrElement + "/_other_reference_ranges:1/meaning|value":       "very high",
		rmattrElement + "/_other_reference_ranges:1/upper|magnitude":     77.6,
		rmattrElement + "/_other_reference_ranges:1/upper|unit":          "unit",
		rmattrElement + "/_other_reference_ranges:1|lower_included":      false,
		rmattrElement + "/_other_reference_ranges:1|lower_unbounded":     true,
	})
	q := elementValue[rm.DVQuantity](t, comp)
	if len(q.OtherReferenceRanges) != 2 {
		t.Fatalf("other_reference_ranges = %d, want 2", len(q.OtherReferenceRanges))
	}
	meaning, ok := q.OtherReferenceRanges[0].Meaning.(*rm.DVCodedText)
	if !ok {
		t.Fatalf("meaning = %T, want *rm.DVCodedText (|code present)", q.OtherReferenceRanges[0].Meaning)
	}
	if meaning.DefiningCode.CodeString != "260360000" || meaning.Value != "very high" {
		t.Errorf("meaning = %+v", *meaning)
	}
	if !q.OtherReferenceRanges[1].Range.LowerUnbounded {
		t.Error("second range lost lower_unbounded")
	}
}

// TestRMAttrReferenceRangeBareMeaning — REQ-140. `/meaning` is a DV_TEXT or a
// DV_CODED_TEXT under the Phase A substitution rule — `|code` present selects
// the coded builder, a bare value stays plain. The corpus writes the bare form
// on `dv_count` / `dv_date` / `dv_ordinal` and the coded form on `dv_quantity`.
func TestRMAttrReferenceRangeBareMeaning(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrCount: 7,
		rmattrCount + "/_other_reference_ranges:0/lower":   8,
		rmattrCount + "/_other_reference_ranges:0/upper":   10,
		rmattrCount + "/_other_reference_ranges:0/meaning": "high",
	})
	c := elementValue[rm.DVCount](t, comp)
	if len(c.OtherReferenceRanges) != 1 {
		t.Fatalf("other_reference_ranges = %d, want 1", len(c.OtherReferenceRanges))
	}
	if _, coded := c.OtherReferenceRanges[0].Meaning.(*rm.DVCodedText); coded {
		t.Error("a bare /meaning must stay a plain DV_TEXT")
	}
	if got := c.OtherReferenceRanges[0].Meaning.GetValue(); got != "high" {
		t.Errorf("meaning = %q", got)
	}
}

// TestRMAttrReferenceRangeMissingMeaning — REQ-140. `meaning` is RM-mandatory
// on REFERENCE_RANGE, so a range spelled without one must not decode to a
// coerced empty text.
func TestRMAttrReferenceRangeMissingMeaning(t *testing.T) {
	wt, _ := conformanceWT(t)
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		rmattrElement + "/_other_reference_ranges:0/lower|magnitude": 70.5,
		rmattrElement + "/_other_reference_ranges:0/lower|unit":      "unit",
		rmattrElement + "/_other_reference_ranges:0|upper_unbounded": true,
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), "meaning") {
		t.Errorf("err = %v, want it to name the missing /meaning", err)
	}
}

// --- _mapping:N ---------------------------------------------------------

// TestRMAttrMappingRoundTrip — REQ-140. TERM_MAPPING on a DV_CODED_TEXT leaf,
// exactly as `ehrbase_conformance_data_types_dv_coded_text.json` spells it:
// `|match`, `/target` as a CODE_PHRASE, and an optional `/purpose`
// DV_CODED_TEXT — the second instance carries none.
func TestRMAttrMappingRoundTrip(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrCodedText + "|code":                           "at0022",
		rmattrCodedText + "|value":                          "value3",
		rmattrCodedText + "/_mapping:0|match":               "=",
		rmattrCodedText + "/_mapping:0/target|code":         "21794005",
		rmattrCodedText + "/_mapping:0/target|terminology":  "SNOMED-CT",
		rmattrCodedText + "/_mapping:0/purpose|code":        "671",
		rmattrCodedText + "/_mapping:0/purpose|terminology": "openehr",
		rmattrCodedText + "/_mapping:0/purpose|value":       "research study",
		rmattrCodedText + "/_mapping:1|match":               "=",
		rmattrCodedText + "/_mapping:1/target|code":         "W.11.7",
		rmattrCodedText + "/_mapping:1/target|terminology":  "RTX",
	})
	dv := elementValue[rm.DVCodedText](t, comp)
	if len(dv.Mappings) != 2 {
		t.Fatalf("mappings = %d, want 2", len(dv.Mappings))
	}
	if dv.Mappings[0].Match != "=" {
		t.Errorf("mappings[0].match = %q, want \"=\"", dv.Mappings[0].Match)
	}
	if got := dv.Mappings[0].Target.CodeString; got != "21794005" {
		t.Errorf("mappings[0].target.code_string = %q", got)
	}
	if got := dv.Mappings[0].Target.TerminologyID.Value; got != "SNOMED-CT" {
		t.Errorf("mappings[0].target.terminology_id = %q", got)
	}
	if dv.Mappings[0].Purpose == nil || dv.Mappings[0].Purpose.Value != "research study" {
		t.Errorf("mappings[0].purpose = %+v", dv.Mappings[0].Purpose)
	}
	if dv.Mappings[1].Purpose != nil {
		t.Errorf("mappings[1].purpose = %+v, want nil (no /purpose spelled)", dv.Mappings[1].Purpose)
	}
}

// TestRMAttrMappingComposesWithTextForms — REQ-140. `mappings` is declared on
// DV_TEXT, so the family reaches a leaf whose value is genuinely plain text, a
// genuine DV_CODED_TEXT, **and** the Phase A substituted coded-at-text (a
// DV_CODED_TEXT stored at a DV_TEXT leaf, which the corpus's
// `dv_coded_text_as_dv_text` fixture pairs with these very mapping keys). All
// three must carry the decoration, since it lands on the value the leaf holds
// rather than on the leaf type.
func TestRMAttrMappingComposesWithTextForms(t *testing.T) {
	wt, _ := conformanceWT(t)
	mapping := func(leaf string) map[string]any {
		return map[string]any{
			leaf + "/_mapping:0|match":              "=",
			leaf + "/_mapping:0/target|code":        "21794005",
			leaf + "/_mapping:0/target|terminology": "SNOMED-CT",
		}
	}
	for name, keys := range map[string]map[string]any{
		"plain DV_TEXT": mergeRMAttr(map[string]any{
			rmattrText: "DV_TEXT value",
		}, mapping(rmattrText)),
		"genuine DV_CODED_TEXT": mergeRMAttr(map[string]any{
			rmattrCodedText + "|code":  "at0022",
			rmattrCodedText + "|value": "value3",
		}, mapping(rmattrCodedText)),
		"substituted coded-at-text": mergeRMAttr(map[string]any{
			rmattrText + "|code":        "at0022",
			rmattrText + "|value":       "value3",
			rmattrText + "|terminology": "local",
		}, mapping(rmattrText)),
	} {
		t.Run(name, func(t *testing.T) {
			comp := assertRMAttrRoundTrip(t, wt, keys)
			// Whichever DV_TEXT subtype the leaf holds, the mapping is on it.
			var mappings []rm.TermMapping
			switch dv := elementText(t, comp).(type) {
			case *rm.DVText:
				mappings = dv.Mappings
			case *rm.DVCodedText:
				mappings = dv.Mappings
			default:
				t.Fatalf("dv value is a %T", dv)
			}
			if len(mappings) != 1 {
				t.Fatalf("mappings = %d, want 1", len(mappings))
			}
		})
	}
}

// TestRMAttrMappingMatchValidated — REQ-140. `|match` is the single character
// TERM_MAPPING's `match` admits (`=`, `<`, `>`, `?` — ISO 2788 / 5964). Anything
// else is a typed error rather than a silently truncated or zero rune, in both
// directions.
func TestRMAttrMappingMatchValidated(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, match := range map[string]any{
		"empty":        "",
		"two chars":    "==",
		"outside set":  "~",
		"not a string": 61,
		"multi-byte":   "≈",
	} {
		t.Run(name, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
				rmattrText:                             "DV_TEXT value",
				rmattrText + "/_mapping:0|match":       match,
				rmattrText + "/_mapping:0/target|code": "21794005",
			}))
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
	// …and every legal code round-trips.
	for _, match := range []string{"=", "<", ">", "?"} {
		t.Run("legal "+match, func(t *testing.T) {
			assertRMAttrRoundTrip(t, wt, map[string]any{
				rmattrText:                                    "DV_TEXT value",
				rmattrText + "/_mapping:0|match":              match,
				rmattrText + "/_mapping:0/target|code":        "21794005",
				rmattrText + "/_mapping:0/target|terminology": "SNOMED-CT",
			})
		})
	}
	// Encode refuses a rune outside the set too — the RM type is a bare rune, so
	// nothing but this check stands between a defective value and the wire.
	comp := decodeRMAttr(t, wt, rmattrBody(map[string]any{
		rmattrText:                                    "DV_TEXT value",
		rmattrText + "/_mapping:0|match":              "=",
		rmattrText + "/_mapping:0/target|code":        "21794005",
		rmattrText + "/_mapping:0/target|terminology": "SNOMED-CT",
	}))
	text, ok := elementText(t, comp).(*rm.DVText)
	if !ok {
		t.Fatalf("dv_text leaf decoded as %T", elementText(t, comp))
	}
	text.Mappings[0].Match = "~"
	if _, err := MarshalFlat(comp, wt); !errors.Is(err, ErrUnsupportedDatatype) {
		t.Errorf("MarshalFlat with match %q err = %v, want ErrUnsupportedDatatype", "~", err)
	}
}

// TestRMAttrMappingTargetRequired — REQ-140. `target` is RM-mandatory on
// TERM_MAPPING and a CODE_PHRASE without a code is not a code, so a mapping
// spelled without either is refused rather than decoded to an empty term.
func TestRMAttrMappingTargetRequired(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, keys := range map[string]map[string]any{
		"no /target": {
			rmattrText:                       "DV_TEXT value",
			rmattrText + "/_mapping:0|match": "=",
		},
		"no |match": {
			rmattrText:                             "DV_TEXT value",
			rmattrText + "/_mapping:0/target|code": "21794005",
		},
		"/target without |code": {
			rmattrText:                       "DV_TEXT value",
			rmattrText + "/_mapping:0|match": "=",
			rmattrText + "/_mapping:0/target|terminology": "SNOMED-CT",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(keys)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// elementText digs out the DV_TEXT-family value of the corpus template's
// dv_text / dv_coded_text leaf, whichever subtype it holds.
func elementText(t *testing.T, comp *rm.Composition) rm.DataValue {
	t.Helper()
	obs := firstObservation(t, comp)
	for _, ev := range obs.Data.Events {
		pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
		if !ok {
			continue
		}
		tree, ok := pe.Data.(*rm.ItemTree)
		if !ok {
			continue
		}
		for _, it := range tree.Items {
			el, ok := it.(*rm.Element)
			if !ok {
				continue
			}
			switch el.Value.(type) {
			case *rm.DVText, *rm.DVCodedText:
				return el.Value
			}
		}
	}
	t.Fatal("no DV_TEXT-family ELEMENT found under the observation")
	return nil
}

// --- _null_flavour / _null_reason ---------------------------------------

// TestRMAttrNullFlavourWithoutValue — REQ-140, and the point of the family: an
// ELEMENT whose `value` is absent carries its `null_flavour` instead, so the FLAT
// body has the `_null_flavour` keys and **no bare key at all**. Both directions
// have to cope with that: decode must not trip the missing-required-value arm
// (there is no leaf group to decode), and encode must still walk the ELEMENT's
// attributes although the leaf path resolves to nothing.
//
// The spelling is `ehrbase_conformance_Element_null_flavor.json`'s, which writes
// `|terminology` explicitly beside `|code` and `|value` — so the family rides the
// ordinary DV_CODED_TEXT suffix grammar rather than an `openehr`-implied pair
// (wire.md § REQ-140 corrected in the same commit).
func TestRMAttrNullFlavourWithoutValue(t *testing.T) {
	wt, _ := conformanceWT(t)
	body := rmattrBody(map[string]any{
		rmattrCount + "/_null_flavour|code":        "253",
		rmattrCount + "/_null_flavour|terminology": "openehr",
		rmattrCount + "/_null_flavour|value":       "unknown",
		rmattrCount + "/_null_reason":              "sample reason",
	})
	comp := decodeRMAttr(t, wt, body)
	el := nullFlavouredElement(t, comp)
	if el.Value != nil {
		t.Errorf("ELEMENT.value = %#v, want nil — the body spelled no leaf value", el.Value)
	}
	if el.NullFlavour == nil {
		t.Fatal("null_flavour not decoded")
	}
	if el.NullFlavour.DefiningCode.CodeString != "253" || el.NullFlavour.Value != "unknown" {
		t.Errorf("null_flavour = %+v", *el.NullFlavour)
	}
	if got := el.NullFlavour.DefiningCode.TerminologyID.Value; got != "openehr" {
		t.Errorf("null_flavour terminology = %q, want openehr", got)
	}
	if el.NullReason == nil || el.NullReason.GetValue() != "sample reason" {
		t.Errorf("null_reason = %+v", el.NullReason)
	}

	got := reencodeRMAttr(t, wt, comp)
	for k, want := range body {
		if !strings.Contains(k, "_null") {
			continue
		}
		if have := got[k]; !sameCtxValue(want, have) {
			t.Errorf("re-encode of %q = %#v, want %#v", k, have, want)
		}
	}
	// …and no bare key was invented for the absent value.
	if v, spelled := got[rmattrCount]; spelled {
		t.Errorf("re-encode invented a bare leaf value %#v for a null-flavoured ELEMENT", v)
	}
}

// TestRMAttrNullFlavourBesideValue — REQ-140. The family sits on the ELEMENT, not
// on its value, so it composes with a leaf that *does* carry one (the RM's own
// invariant runs the other way — an absent value requires a null flavour, not the
// reverse — and semantic validation stays with the validation package).
func TestRMAttrNullFlavourBesideValue(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrCount:                          7,
		rmattrCount + "/_null_flavour|code":  "271",
		rmattrCount + "/_null_flavour|value": "no information",
	})
	el := nullFlavouredElement(t, comp)
	if el.Value == nil {
		t.Error("the leaf value was dropped beside a null flavour")
	}
	// No |terminology spelled, so none is invented on the way back either.
	if got := el.NullFlavour.DefiningCode.TerminologyID.Value; got != "" {
		t.Errorf("null_flavour terminology = %q, want empty (none spelled)", got)
	}
}

// TestRMAttrNullFlavourHalfPairRefused — REQ-140. `_null_flavour` is a
// DV_CODED_TEXT: `|code` and `|value` are both required, so half a pair must not
// decode to a coerced empty rubric or an unlabelled code.
func TestRMAttrNullFlavourHalfPairRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, keys := range map[string]map[string]any{
		"code alone":        {rmattrCount + "/_null_flavour|code": "253"},
		"value alone":       {rmattrCount + "/_null_flavour|value": "unknown"},
		"terminology alone": {rmattrCount + "/_null_flavour|terminology": "openehr"},
		"bare value":        {rmattrCount + "/_null_flavour": "unknown"},
		"unknown suffix":    {rmattrCount + "/_null_flavour|typo": "x"},
		"sub-path":          {rmattrCount + "/_null_flavour/target|code": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(keys)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// TestRMAttrNullFamiliesOnlyOnElement — REQ-140. `null_flavour` and `null_reason`
// are declared on ELEMENT, so the families reach a collapsed leaf and nothing
// else — the rminfo rule, not a hand-kept owner list.
func TestRMAttrNullFamiliesOnlyOnElement(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, owner := range []string{rmattrRoot, rmattrSection, rmattrObs} {
		for _, family := range []string{"/_null_flavour|code", "/_null_reason"} {
			t.Run(owner+family, func(t *testing.T) {
				if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
					owner + family: "x",
				})); !errors.Is(err, ErrUnknownPath) {
					t.Errorf("err = %v, want ErrUnknownPath", err)
				}
			})
		}
	}
}

// nullFlavouredElement digs out the ELEMENT carrying a null flavour.
func nullFlavouredElement(t *testing.T, comp *rm.Composition) *rm.Element {
	t.Helper()
	obs := firstObservation(t, comp)
	for _, ev := range obs.Data.Events {
		pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
		if !ok {
			continue
		}
		tree, ok := pe.Data.(*rm.ItemTree)
		if !ok {
			continue
		}
		for _, it := range tree.Items {
			if el, ok := it.(*rm.Element); ok && el.NullFlavour != nil {
				return el
			}
		}
	}
	t.Fatal("no null-flavoured ELEMENT found under the observation")
	return nil
}

// --- the anchor rule ----------------------------------------------------

// TestRMAttrValueFamilyOwnerRule — REQ-140. A value-decoration family is judged
// against the **leaf datatype**, read off the RM (rminfo), not against the
// ELEMENT that holds it: `normal_range` reaches every DV_ORDERED leaf and
// nothing else, `mappings` reaches DV_TEXT / DV_CODED_TEXT and nothing else.
func TestRMAttrValueFamilyOwnerRule(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, key := range []string{
		// DV_TEXT and DV_BOOLEAN are not DV_ORDERED.
		rmattrText + "/_normal_range/lower",
		rmattrBoolean + "/_normal_range/lower",
		rmattrText + "/_other_reference_ranges:0/meaning",
		// DV_QUANTITY carries no mappings.
		rmattrElement + "/_mapping:0|match",
		// …and neither reaches a node that is not a collapsed ELEMENT at all.
		rmattrObs + "/_normal_range/lower",
		rmattrRoot + "/_mapping:0|match",
	} {
		t.Run(key, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{key: "x"})); !errors.Is(err, ErrUnknownPath) {
				t.Errorf("err = %v, want ErrUnknownPath", err)
			}
		})
	}
}

// TestRMAttrIntervalBadTails — REQ-140. A recognised family carrying a tail
// outside its grammar is ErrUnsupportedDatatype naming the offending FLAT key —
// the shape the PROBE-086 census scopes an exclusion by. A tail the *anchor
// datatype* rejects is named the way a clinical leaf's is instead — the bound's
// base key plus the suffix label — because it is the datatype allowlist
// speaking, and the census narrows that refusal to the one suffix.
func TestRMAttrIntervalBadTails(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct{ key, names string }{
		{key: rmattrElement + "/_normal_range|typo"},
		{key: rmattrElement + "/_normal_range/bogus|magnitude"},
		{key: rmattrElement + "/_normal_range/lower|typo", names: rmattrElement + "/_normal_range/lower"},
		{key: rmattrElement + "/_normal_range/lower/deeper|magnitude"},
		{key: rmattrElement + "/_other_reference_ranges:0|typo"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
				rmattrElement + "/_normal_range/lower|magnitude":             20.5,
				rmattrElement + "/_normal_range/lower|unit":                  "unit",
				rmattrElement + "/_normal_range|upper_unbounded":             true,
				rmattrElement + "/_other_reference_ranges:0/lower|magnitude": 70.5,
				rmattrElement + "/_other_reference_ranges:0/lower|unit":      "unit",
				rmattrElement + "/_other_reference_ranges:0|upper_unbounded": true,
				rmattrElement + "/_other_reference_ranges:0/meaning":         "high",
				tc.key: "boom",
			}))
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			want := cmp.Or(tc.names, tc.key)
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %v, want it to name %q", err, want)
			}
			if tc.names != "" && !strings.Contains(err.Error(), "|typo") {
				t.Errorf("err = %v, want it to name the offending suffix too", err)
			}
		})
	}
}

// TestRMAttrIntervalBooleanKind — REQ-140. The boundary suffixes are Booleans;
// a value of another JSON kind is refused where the FLAT key is still in hand
// to name it, rather than surfacing from canjson against the rebuilt tree.
func TestRMAttrIntervalBooleanKind(t *testing.T) {
	wt, _ := conformanceWT(t)
	key := rmattrElement + "/_normal_range|lower_included"
	err := decodeRMAttrErr(t, wt, rmattrBody(map[string]any{
		rmattrElement + "/_normal_range/lower|magnitude": 20.5,
		rmattrElement + "/_normal_range/lower|unit":      "unit",
		key: "yes",
	}))
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("err = %v, want it to name %q", err, key)
	}
}

// TestRMAttrIntervalBoundRidesRaw — REQ-140. A bound is emitted by the anchor
// datatype's own rules, `|raw` fallback included: one the suffix set cannot
// capture — here a `normal_status` coded outside the implied openEHR terminology,
// which would be silently re-terminologised by the bare-code suffix — rides
// `/lower|raw` rather than being narrowed or refused. Decode reads it back
// through the same bypass, so the round-trip is exact. That fallback is what
// makes the decoration emitters total, and therefore what lets a decorated value
// leave the `|raw` path at all.
func TestRMAttrIntervalBoundRidesRaw(t *testing.T) {
	wt, _ := conformanceWT(t)
	assertRMAttrRoundTrip(t, wt, map[string]any{
		rmattrCount:                          7,
		rmattrCount + "/_normal_range/upper": 8,
		rmattrCount + "/_normal_range/lower|raw": map[string]any{
			"_type":     "DV_COUNT",
			"magnitude": 1,
			"normal_status": map[string]any{
				"_type":          "CODE_PHRASE",
				"code_string":    "N",
				"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "SNOMED-CT"},
			},
		},
	})
}

// TestRMAttrSubPathIndexNormalisation — REQ-140. The OPT-free FLAT ↔ STRUCTURED
// interconversion re-spells *every* segment with an explicit `:index`, and the
// value-decoration families are the first with a sub-path for it to reach
// (`/lower:0`). Decode folds `:0` onto the index-less spelling, refuses anything
// higher (no attribute of a DataValue is a list at that position) and refuses two
// spellings competing for one slot.
func TestRMAttrSubPathIndexNormalisation(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(map[string]any{
		rmattrElement + "/_normal_range:0/lower:0|magnitude": 20.5,
		rmattrElement + "/_normal_range:0/lower:0|unit":      "unit",
		rmattrElement + "/_normal_range:0|upper_unbounded":   true,
	}))
	q := elementValue[rm.DVQuantity](t, comp)
	if q.NormalRange == nil || q.NormalRange.Lower.Magnitude != 20.5 {
		t.Fatalf("the `:0`-normalised spelling did not decode: %+v", q.NormalRange)
	}
	for name, extra := range map[string]map[string]any{
		"index above zero": {
			rmattrElement + "/_normal_range/lower:1|magnitude": 20.5,
			rmattrElement + "/_normal_range/lower:1|unit":      "unit",
		},
		"two spellings of one slot": {
			rmattrElement + "/_normal_range/lower|magnitude":   20.5,
			rmattrElement + "/_normal_range/lower:0|magnitude": 20.5,
			rmattrElement + "/_normal_range/lower|unit":        "unit",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := decodeRMAttrErr(t, wt, rmattrBody(extra)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("err = %v, want ErrUnsupportedDatatype", err)
			}
		})
	}
}

// elementValue digs the decoded DataValue out of the collapsed ELEMENT the
// corpus template folds each datatype leaf into.
func elementValue[T any](t *testing.T, comp *rm.Composition) T {
	t.Helper()
	obs := firstObservation(t, comp)
	for _, ev := range obs.Data.Events {
		pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
		if !ok {
			continue
		}
		tree, ok := pe.Data.(*rm.ItemTree)
		if !ok {
			continue
		}
		for _, it := range tree.Items {
			el, ok := it.(*rm.Element)
			if !ok {
				continue
			}
			if v, ok := as[T](el.Value); ok {
				return v
			}
		}
	}
	var zero T
	t.Fatalf("no ELEMENT carrying a %T found under the observation", zero)
	return zero
}

// --- interval bounds ------------------------------------------------------

// A bounded interval end must carry a bound. Encode used to accept an end whose
// `*_unbounded` flag was false while the bound was Void, and the two bound
// representations failed differently: an interface-typed bound
// (`DVInterval[DVOrdered]`, what a canonical decode produces) emitted nothing —
// decode then read the opposite of what the flags said — while a concrete-typed
// bound put its Go zero value on the wire as a *fabricated* bound, a
// `|magnitude` of 0 under an empty, RM-mandatory `|unit`. Both are refused.
func TestIntervalBoundedEndWithoutBoundRefused(t *testing.T) {
	lower := rm.DVQuantity{Magnitude: 1, Units: "mm"}
	for name, encode := range map[string]func(map[string]any) error{
		// Concrete bound type: Upper is the datatype's zero value.
		"concrete": func(out map[string]any) error {
			return intervalToFlat(out, "x/_normal_range", "DV_QUANTITY",
				rm.Interval[rm.DVQuantity]{Lower: lower})
		},
		// Interface bound type: Upper is a nil DV_ORDERED.
		"interface": func(out map[string]any) error {
			return intervalToFlat(out, "x/_normal_range", "DV_QUANTITY",
				rm.Interval[rm.DVOrdered]{Lower: lower})
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := map[string]any{}
			err := encode(out)
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			for key := range out {
				if strings.Contains(key, "/upper") {
					t.Errorf("a refused bound still wrote %q", key)
				}
			}
		})
	}
}

// The refusal must not catch a legitimately zero bound: DV_COUNT's magnitude of
// 0 is a real lower bound, and it carries no empty RM-mandatory suffix.
func TestIntervalZeroCountBoundEncodes(t *testing.T) {
	out := map[string]any{}
	if err := intervalToFlat(out, "x/_normal_range", "DV_COUNT", rm.Interval[rm.DVCount]{
		Lower: rm.DVCount{Magnitude: 0},
		Upper: rm.DVCount{Magnitude: 10},
	}); err != nil {
		t.Fatalf("intervalToFlat on a 0..10 count range: %v", err)
	}
	for key, want := range map[string]any{
		"x/_normal_range/lower": int64(0),
		"x/_normal_range/upper": int64(10),
	} {
		if got := out[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

// An end genuinely marked unbounded still writes only its flag — the shape the
// refusal above must leave alone.
func TestIntervalUnboundedEndEncodes(t *testing.T) {
	out := map[string]any{}
	if err := intervalToFlat(out, "x/_normal_range", "DV_QUANTITY", rm.Interval[rm.DVQuantity]{
		Lower:          rm.DVQuantity{Magnitude: 1, Units: "mm"},
		UpperUnbounded: true,
	}); err != nil {
		t.Fatalf("intervalToFlat on a half-open range: %v", err)
	}
	if got := out["x/_normal_range|upper_unbounded"]; got != true {
		t.Errorf("|upper_unbounded = %v, want true", got)
	}
	for key := range out {
		if strings.Contains(key, "/upper") {
			t.Errorf("an unbounded end wrote a bound key %q", key)
		}
	}
}

// The encode mirror: a TERM_MAPPING whose target carries no code would emit a
// `_mapping:N` with no `/target|code`, which decode then rejects.
func TestRMAttrMappingCodelessTargetRefusedOnEncode(t *testing.T) {
	out := map[string]any{}
	err := mappingsRMAttr(out, "x", []rm.TermMapping{{
		Match:  "=",
		Target: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}},
	}})
	if !errors.Is(err, ErrUnsupportedDatatype) {
		t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
	}
}
