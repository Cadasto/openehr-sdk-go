package simplified

// REQ-053 — "A value must fit its attribute": decode refuses a suffix value,
// required or optional, bare value included, that the canonical-JSON decode of
// its RM attribute would refuse (REQ-052, ADR 0004 tolerance included), with
// an error that wraps ErrUnsupportedDatatype and names the FLAT key, never a
// canonical-JSON error naming a path the caller did not write. The check adds
// no strictness of its own: what canonical JSON admits, it admits.

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
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

// TestREQ053CodeThatDoesNotFitAtATextLeafNamesItsKey — REQ-053: a `|code` at
// a DV_TEXT leaf selects the DV_CODED_TEXT it carries whatever its JSON kind,
// so one that is not a string is refused as a value that does not fit the
// coded text's code_string, naming the `|code` key, never the `|value` beside
// it nor the leaf alone. `|code: ""` stays a plain DV_TEXT
// (TestEmptyCodeDoesNotPromoteTextLeaf).
func TestREQ053CodeThatDoesNotFitAtATextLeafNamesItsKey(t *testing.T) {
	wt, _ := conformanceWT(t)
	key := rmattrText + "|code"
	for name, code := range map[string]any{"a number": 5, "a Boolean": true} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(map[string]any{key: code, rmattrText + "|value": "x"})
			want := `|code" does not fit DV_CODED_TEXT.defining_code.code_string`
			flat := refusedInEveryMode(t, wt, body)
			if !errors.Is(flat, ErrUnsupportedDatatype) || !strings.Contains(flat.Error(), `"`+rmattrText+want) {
				t.Errorf("FLAT: decode = %v, want ErrUnsupportedDatatype naming %q", flat, key)
			}
			structured := structuredRefusedInEveryMode(t, wt, body)
			if !errors.Is(structured, ErrUnsupportedDatatype) || !strings.Contains(structured.Error(), want) {
				t.Errorf("STRUCTURED: decode = %v, want ErrUnsupportedDatatype naming the |code key", structured)
			}
		})
	}
}

// TestCheckSuffixFitsNamesTheFirstMisfit — REQ-053: a leaf carrying more than
// one value that does not fit is refused naming the first of them in sorted
// suffix order, the same one on every run, never one chosen by Go's map order.
func TestCheckSuffixFitsNamesTheFirstMisfit(t *testing.T) {
	sfx := map[string]any{"unit": 5, "magnitude": "abc", "precision": 1.5, "units_system": "s"}
	for range 64 {
		err := checkSuffixFits("DV_QUANTITY", sfx, nil)
		if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "|magnitude does not fit") {
			t.Fatalf("checkSuffixFits(DV_QUANTITY, %v) = %v, want |magnitude, the first value that does not fit, named", sfx, err)
		}
	}
}

// TestREQ053FitNamesTheBareKeyAnEmptySuffixSpells — REQ-053: `x|` spells the
// bare value of `x`, so a bare value written `x|` that does not fit is refused
// naming `x`, the key it spells.
func TestREQ053FitNamesTheBareKeyAnEmptySuffixSpells(t *testing.T) {
	wt, _ := conformanceWT(t)
	err := refusedInEveryMode(t, wt, rmattrBody(map[string]any{rmattrCount + "|": 1.5}))
	if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), `"`+rmattrCount+`" does not fit DV_COUNT.magnitude`) {
		t.Errorf("decode = %v, want ErrUnsupportedDatatype naming %q", err, rmattrCount)
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

// TestSuffixFitsResolveEverySuffix — REQ-053. Every suffix a datatype reads
// ([allowedSuffixes], plus DV_CODED_TEXT's `|other`) has an attribute path
// that rminfo resolves to an RM primitive the fit check can decode, and no
// path is recorded for a suffix no datatype reads. A suffix without a
// resolvable attribute would refuse every value, adding strictness canonical
// JSON does not have.
func TestSuffixFitsResolveEverySuffix(t *testing.T) {
	for rmType, allowed := range allowedSuffixes {
		suffixes := maps.Clone(allowed)
		if rmType == "DV_CODED_TEXT" {
			suffixes["other"] = true
		}
		for suffix := range suffixes {
			fit, ok := suffixFits[rmType][suffix]
			if !ok {
				t.Errorf("suffixFits[%s][%q] is missing", rmType, suffix)
				continue
			}
			if decodeTarget(fit.bmmType) == nil {
				t.Errorf("suffixFits[%s][%q] = %+v: the attribute resolves to no RM primitive the check decodes", rmType, suffix, fit)
			}
		}
	}
	for rmType, paths := range suffixAttrPaths {
		for suffix := range paths {
			if !allowedSuffixes[rmType][suffix] && (rmType != "DV_CODED_TEXT" || suffix != "other") {
				t.Errorf("suffixAttrPaths[%s][%q] is recorded for a suffix the datatype does not read", rmType, suffix)
			}
		}
	}
	for suffix := range orderedSuffixAttrPaths {
		read := false
		for _, allowed := range allowedSuffixes {
			read = read || allowed[suffix]
		}
		if !read {
			t.Errorf("orderedSuffixAttrPaths[%q] is recorded for a suffix no datatype reads", suffix)
		}
	}
}

// canonicalAttrValue builds the canonical JSON of an rmType value whose
// attribute path holds v, every object on the way carrying the `_type` rminfo
// names for it, the shape canonical JSON decodes at that attribute.
func canonicalAttrValue(t *testing.T, rmType, path string, v any) []byte {
	t.Helper()
	root := map[string]any{"_type": rmType}
	obj, typ := root, rmType
	attrs := strings.Split(path, "/")
	for _, attr := range attrs[:len(attrs)-1] {
		next, ok := attrBMMType(typ, attr)
		if !ok {
			t.Fatalf("rminfo does not resolve %s.%s", typ, attr)
		}
		child := map[string]any{"_type": next}
		obj[attr], obj, typ = child, child, next
	}
	obj[attrs[len(attrs)-1]] = v
	b, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestSuffixFitMatchesCanonicalJSON — REQ-053: the fit check refuses a value
// exactly when the canonical-JSON decode of its attribute does. For every
// suffix of every datatype and a spread of values (strings that are and are
// not decimals, whole and fractional numbers, numbers out of range, Booleans,
// composites), the check's verdict is the verdict of decoding the value at the
// attribute through the type registry canjson decodes with.
func TestSuffixFitMatchesCanonicalJSON(t *testing.T) {
	values := []any{
		"abc", "", "1", "1.5", "1.0", "aGk=", "2021-12-21",
		json.Number("1"), json.Number("1.0"), json.Number("1.5"), json.Number("-0"), json.Number("1e2"),
		json.Number("3000000000"), json.Number("9223372036854775808"), json.Number("1e400"),
		1.5, 2.0, int64(7), true, false,
		map[string]any{"a": "b"},
		[]any{"a"},
	}
	for rmType, fits := range suffixFits {
		for suffix, fit := range fits {
			path, own := suffixAttrPaths[rmType][suffix]
			if !own {
				path = orderedSuffixAttrPaths[suffix]
			}
			for _, v := range values {
				_, decodeErr := typereg.Default.Decode(canonicalAttrValue(t, rmType, path, v))
				if got, want := fit.admits(v), decodeErr == nil; got != want {
					t.Errorf("suffixFits[%s][%q].admits(%#v) = %v, but canonical JSON at %s decodes it with error %v",
						rmType, suffix, v, got, path, decodeErr)
				}
			}
		}
	}
}

// TestREQ053FitNamesTheKeyAtEveryPosition — REQ-053: a value that does not fit
// its attribute is refused naming the FLAT key as written wherever the datatype
// is decoded: an interval bound, a reference range's meaning, a term mapping's
// target, an ELEMENT's null flavour, a DV_TEXT's language, a participation's
// inlined identifier and a feeder audit's item id.
func TestREQ053FitNamesTheKeyAtEveryPosition(t *testing.T) {
	wt, _ := conformanceWT(t)
	ref := rmattrElement + "/_other_reference_ranges:0"
	for name, tc := range map[string]struct {
		extra map[string]any
		key   string
	}{
		"interval bound": {
			extra: map[string]any{rmattrElement + "/_normal_range/lower|magnitude": "abc", rmattrElement + "/_normal_range/lower|unit": "unit"},
			key:   rmattrElement + "/_normal_range/lower|magnitude",
		},
		"reference range meaning": {
			extra: map[string]any{ref + "/lower|magnitude": 1.5, ref + "/lower|unit": "unit", ref + "/meaning": 5},
			key:   ref + "/meaning",
		},
		"term mapping target": {
			extra: map[string]any{
				rmattrText: "text", rmattrText + "/_mapping:0|match": "=",
				rmattrText + "/_mapping:0/target|code": 5, rmattrText + "/_mapping:0/target|terminology": "SNOMED-CT",
			},
			key: rmattrText + "/_mapping:0/target|code",
		},
		"null flavour": {
			extra: map[string]any{rmattrBoolean + "/_null_flavour|code": "271", rmattrBoolean + "/_null_flavour|value": true},
			key:   rmattrBoolean + "/_null_flavour|value",
		},
		"text language": {
			extra: map[string]any{rmattrText: "text", rmattrText + "/_language|code": false, rmattrText + "/_language|terminology": "ISO_639-1"},
			key:   rmattrText + "/_language|code",
		},
		"inlined participation identifier": {
			extra: map[string]any{
				rmattrContext + "/_participation:0|function":         "requester",
				rmattrContext + "/_participation:0|identifiers_id:0": 5,
			},
			key: rmattrContext + "/_participation:0|identifiers_id:0",
		},
		"feeder audit item id": {
			extra: map[string]any{
				rmattrRoot + "/_feeder_audit/originating_system_audit|system_id": "s",
				rmattrRoot + "/_feeder_audit/originating_system_item_id:0|id":    5,
			},
			key: rmattrRoot + "/_feeder_audit/originating_system_item_id:0|id",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := refusedInEveryMode(t, wt, rmattrBody(tc.extra))
			if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), `"`+tc.key+`" does not fit`) {
				t.Errorf("decode = %v, want ErrUnsupportedDatatype naming %q", err, tc.key)
			}
		})
	}
}

// fullSuffixSets holds, per datatype, a value for every suffix it reads, each
// distinct from its neighbours, so a suffix whose recorded attribute path is
// wrong finds another suffix's value there.
var fullSuffixSets = map[string]map[string]any{
	"DV_TEXT":       {"": "t", "formatting": "f"},
	"DV_CODED_TEXT": {"code": "c", "value": "v", "terminology": "t", "formatting": "f"},
	"DV_DATE_TIME":  {"": "2021-01-01T00:00:00Z", "magnitude_status": "=", "normal_status": "N"},
	"DV_DATE":       {"": "2021-01-01", "magnitude_status": "=", "normal_status": "N"},
	"DV_TIME":       {"": "10:00:00", "magnitude_status": "=", "normal_status": "N"},
	"DV_DURATION":   {"": "P1D", "magnitude_status": "=", "normal_status": "N", "accuracy": 1.5, "accuracy_is_percent": true},
	"DV_URI":        {"": "http://x"},
	"DV_EHR_URI":    {"": "ehr://x"},
	"DV_QUANTITY": {
		"magnitude": 2.5, "unit": "mg", "magnitude_status": "=", "normal_status": "N", "accuracy": 1.5,
		"accuracy_is_percent": true, "precision": 3, "units_system": "s", "units_display_name": "d",
	},
	"DV_COUNT":   {"": 7, "magnitude_status": "=", "normal_status": "N", "accuracy": 1.5, "accuracy_is_percent": true},
	"DV_BOOLEAN": {"": true},
	"DV_ORDINAL": {"code": "c", "value": "v", "ordinal": 4},
	"DV_SCALE":   {"code": "c", "value": "v", "ordinal": 4.5},
	"DV_PROPORTION": {
		"numerator": 2.5, "denominator": 3.5, "type": 1, "magnitude_status": "=", "normal_status": "N",
		"accuracy": 1.5, "accuracy_is_percent": true, "precision": 3,
	},
	"DV_IDENTIFIER": {"id": "i", "issuer": "is", "assigner": "as", "type": "ty"},
	"CODE_PHRASE":   {"code": "c", "terminology": "t", "preferred_term": "p"},
	"DV_PARSABLE":   {"": "v", "formalism": "f"},
	"DV_MULTIMEDIA": {
		"": "http://u", "mediatype": "text/plain", "size": 5, "data": "aGk=", "alternatetext": "a",
		"integrity_check": "aWM=", "integrity_check_algorithm": "SHA-1", "compression_algorithm": "zip",
	},
}

// TestSuffixAttrPathsAreWhereTheValueLands — REQ-053. The attribute path the
// fit check reads a suffix's RM type from is the one [dvFromSuffixes] writes
// the suffix's value at, for every suffix of every datatype, `|other`
// included: a wrong path would check a value against another attribute's
// type.
func TestSuffixAttrPathsAreWhereTheValueLands(t *testing.T) {
	at := func(dv map[string]any, path string) any {
		var cur any = dv
		for attr := range strings.SplitSeq(path, "/") {
			obj, _ := cur.(map[string]any)
			cur = obj[attr]
		}
		return cur
	}
	check := func(rmType string, listOpen bool, sfx map[string]any) {
		t.Helper()
		dv, err := dvFromSuffixes(rmType, listOpen, sfx, nil)
		if err != nil {
			t.Fatalf("dvFromSuffixes(%s, %v) = %v", rmType, sfx, err)
		}
		for suffix, want := range sfx {
			path, own := suffixAttrPaths[rmType][suffix]
			if !own {
				path = orderedSuffixAttrPaths[suffix]
			}
			if got := at(dv, path); !sameCtxValue(got, want) {
				t.Errorf("%s |%s is recorded at %q, which holds %#v; the value %#v landed elsewhere in %#v",
					rmType, suffix, path, got, want, dv)
			}
		}
	}
	for rmType, allowed := range allowedSuffixes {
		sfx := fullSuffixSets[rmType]
		for suffix := range allowed {
			if _, ok := sfx[suffix]; !ok {
				t.Errorf("fullSuffixSets[%s] has no value for |%s", rmType, suffix)
			}
		}
		check(rmType, false, sfx)
	}
	check("DV_CODED_TEXT", true, map[string]any{"other": "o"})
}
