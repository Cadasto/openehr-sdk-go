package simplified

// REQ-053 — "A value must fit its attribute": decode refuses a suffix value,
// required or optional, bare value included, that the canonical-JSON decode of
// its RM attribute would refuse (REQ-052, ADR 0004 tolerance included), with
// an error that wraps ErrUnsupportedDatatype and names the FLAT key, never a
// canonical-JSON error naming a path the caller did not write. The check adds
// no strictness of its own: what canonical JSON admits, it admits.

import (
	"errors"
	"maps"
	"strings"
	"testing"
)

// fitLeaves are the corpus leaves the fit cases below write that have more
// than one required suffix, by FLAT path, each with every required suffix
// holding a value that fits, so a case changes one key alone.
var fitLeaves = map[string]map[string]any{
	rmattrProportion: {rmattrProportion + "|numerator": 20.5, rmattrProportion + "|denominator": 12.4, rmattrProportion + "|type": 0},
	rmattrOrdinal:    {rmattrOrdinal + "|code": "at0015", rmattrOrdinal + "|value": "value1", rmattrOrdinal + "|ordinal": 1},
	rmattrCodedText:  {rmattrCodedText + "|code": "at0001", rmattrCodedText + "|value": "value1", rmattrCodedText + "|terminology": "local"},
}

// fitCase is one FLAT key given one value.
type fitCase struct {
	key   string
	value any
}

// fitBody is the corpus body carrying tc's key and value, beside the other
// required suffixes of its leaf.
func fitBody(tc fitCase) map[string]any {
	extra := map[string]any{}
	leaf, _ := splitSuffix(tc.key)
	maps.Copy(extra, fitLeaves[leaf])
	extra[tc.key] = tc.value
	return rmattrBody(extra)
}

// TestREQ053ValueMustFitItsAttribute — REQ-053: a value the canonical-JSON
// decode of its RM attribute would refuse is refused in every mode, wrapping
// ErrUnsupportedDatatype and naming the FLAT key as written: a string that is
// no decimal where the attribute is a number, a number or a Boolean where it
// is a String, a non-Boolean where it is a Boolean, and a number with a
// non-zero fraction, or outside the int32 range, where it is an Integer
// (DV_PROPORTION |type, |precision, DV_ORDINAL |ordinal). DV_COUNT's bare
// magnitude is an Integer64, which canonical JSON reads as a plain integer.
// The same bodies are refused in STRUCTURED.
func TestREQ053ValueMustFitItsAttribute(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]fitCase{
		"proportion |type with a fraction":       {rmattrProportion + "|type", 1.5},
		"proportion |numerator not a decimal":    {rmattrProportion + "|numerator", "x"},
		"quantity |precision with a fraction":    {rmattrElement + "|precision", 1.5},
		"quantity |precision outside int32":      {rmattrElement + "|precision", 3000000000},
		"quantity |magnitude not a decimal":      {rmattrElement + "|magnitude", "abc"},
		"quantity |unit a number":                {rmattrElement + "|unit", 5},
		"quantity |units_system a Boolean":       {rmattrElement + "|units_system", true},
		"quantity |accuracy_is_percent a string": {rmattrElement + "|accuracy_is_percent", "yes"},
		"ordinal |ordinal with a fraction":       {rmattrOrdinal + "|ordinal", 2.5},
		"coded text |code a number":              {rmattrCodedText + "|code", 5},
		"coded text |terminology a Boolean":      {rmattrCodedText + "|terminology", false},
		"boolean bare value a string":            {rmattrBoolean, "yes"},
		"count bare value with a fraction":       {rmattrCount, 1.5},
		"count bare value quoted":                {rmattrCount, "7"},
		"text bare value a number":               {rmattrText, 5},
		"date time bare value a number":          {rmattrDateTime, 20220112},
	} {
		t.Run(name, func(t *testing.T) {
			body := fitBody(tc)
			flat := refusedInEveryMode(t, wt, body)
			if !errors.Is(flat, ErrUnsupportedDatatype) {
				t.Errorf("FLAT: decode = %v, want ErrUnsupportedDatatype", flat)
			}
			if !strings.Contains(flat.Error(), `"`+tc.key+`"`) {
				t.Errorf("FLAT: decode = %v, want it to name the key %q", flat, tc.key)
			}
			structured := structuredRefusedInEveryMode(t, wt, body)
			if !errors.Is(structured, ErrUnsupportedDatatype) {
				t.Errorf("STRUCTURED: decode = %v, want ErrUnsupportedDatatype", structured)
			}
			for form, err := range map[string]error{"FLAT": flat, "STRUCTURED": structured} {
				if strings.Contains(err.Error(), "canjson") || strings.Contains(err.Error(), "/content") {
					t.Errorf("%s: decode = %v, a canonical-JSON error naming a path the caller did not write", form, err)
				}
			}
		})
	}
}

// TestREQ053FitAdmitsWhatCanonicalJSONAdmits — REQ-053: the check adds no
// strictness beyond naming the key. A whole number written with a fraction
// fits an Integer (REQ-052 § Floating-point precision), a quoted decimal fits
// a Real or an Integer (ADR 0004), and each decodes to the value it spells.
func TestREQ053FitAdmitsWhatCanonicalJSONAdmits(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]fitCase{
		"proportion |type written 1.0":    {rmattrProportion + "|type", 1.0},
		"proportion |type quoted":         {rmattrProportion + "|type", "1"},
		"proportion |numerator quoted":    {rmattrProportion + "|numerator", "20.5"},
		"quantity |precision written 2.0": {rmattrElement + "|precision", 2.0},
		"quantity |precision quoted":      {rmattrElement + "|precision", "2"},
		"quantity |magnitude quoted":      {rmattrElement + "|magnitude", "65.9"},
		"quantity |accuracy quoted":       {rmattrElement + "|accuracy", "0.5"},
		"ordinal |ordinal quoted":         {rmattrOrdinal + "|ordinal", "1"},
		"ordinal |ordinal written 1.0":    {rmattrOrdinal + "|ordinal", 1.0},
		"count bare value":                {rmattrCount, 7},
		"boolean bare value":              {rmattrBoolean, true},
	} {
		t.Run(name, func(t *testing.T) {
			decodeRMAttr(t, wt, fitBody(tc))
		})
	}
}
