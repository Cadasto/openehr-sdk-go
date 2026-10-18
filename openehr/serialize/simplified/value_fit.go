package simplified

// REQ-053 — "A value must fit its attribute". A FLAT suffix value lands in
// the rebuilt canonical-JSON tree, which canjson then decodes. A value the
// canonical-JSON decode of its RM attribute refuses would fail there, naming
// a canonical path the caller never wrote and no FLAT key. So decode checks
// each value against its attribute while the key is still in hand, and
// refuses it wrapping [ErrUnsupportedDatatype] and naming the key.
//
// The check is canonical JSON's own: the attribute's type comes from the RM
// (rminfo), and the value is decoded with the decoder canjson uses for that
// type ([rm.Real], [rm.Integer], [rm.Integer64], or encoding/json/v2 for the
// rest). So it admits everything canonical JSON admits, quoted decimals and
// whole numbers written with a fraction included (ADR 0004, REQ-052
// § Floating-point precision), and adds no strictness beyond naming the key.

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// orderedSuffixAttrPaths maps the optional DV_ORDERED / DV_QUANTIFIED /
// DV_AMOUNT suffixes, and DV_TEXT's `|formatting`, onto the canonical
// attribute path each value fills, for every datatype whose
// [allowedSuffixes] admit them.
var orderedSuffixAttrPaths = map[string]string{
	"magnitude_status":    "magnitude_status",
	"normal_status":       "normal_status/code_string",
	"accuracy":            "accuracy",
	"accuracy_is_percent": "accuracy_is_percent",
	"precision":           "precision",
	"units_system":        "units_system",
	"units_display_name":  "units_display_name",
	"formatting":          "formatting",
}

// suffixAttrPaths maps each datatype's own suffixes ("" for the bare value)
// onto the canonical attribute path, from the datatype, that [dvFromSuffixes]
// writes the value at. Together with [orderedSuffixAttrPaths] it covers every
// suffix in [allowedSuffixes], plus DV_CODED_TEXT's `|other`, which rebuilds a
// DV_TEXT `value`.
var suffixAttrPaths = map[string]map[string]string{
	"DV_TEXT": {"": "value"},
	"DV_CODED_TEXT": {
		"code": "defining_code/code_string", "value": "value",
		"terminology": "defining_code/terminology_id/value", "other": "value",
	},
	"DV_DATE_TIME":  {"": "value"},
	"DV_DATE":       {"": "value"},
	"DV_TIME":       {"": "value"},
	"DV_DURATION":   {"": "value"},
	"DV_URI":        {"": "value"},
	"DV_EHR_URI":    {"": "value"},
	"DV_QUANTITY":   {"magnitude": "magnitude", "unit": "units"},
	"DV_COUNT":      {"": "magnitude"},
	"DV_BOOLEAN":    {"": "value"},
	"DV_ORDINAL":    {"code": "symbol/defining_code/code_string", "value": "symbol/value", "ordinal": "value"},
	"DV_SCALE":      {"code": "symbol/defining_code/code_string", "value": "symbol/value", "ordinal": "value"},
	"DV_PROPORTION": {"numerator": "numerator", "denominator": "denominator", "type": "type"},
	"DV_IDENTIFIER": {"id": "id", "issuer": "issuer", "assigner": "assigner", "type": "type"},
	"CODE_PHRASE":   {"code": "code_string", "terminology": "terminology_id/value", "preferred_term": "preferred_term"},
	"DV_PARSABLE":   {"": "value", "formalism": "formalism"},
	"DV_MULTIMEDIA": {
		"": "uri/value", "mediatype": "media_type/code_string", "size": "size",
		"data": "data", "integrity_check": "integrity_check", "alternatetext": "alternate_text",
		"integrity_check_algorithm": "integrity_check_algorithm/code_string",
		"compression_algorithm":     "compression_algorithm/code_string",
	},
}

// valueFit is the RM attribute a suffix value fills: attr names it
// (`DV_PROPORTION.type`) and bmmType is its RM primitive type, the one the
// value is decoded as.
type valueFit struct {
	attr    string
	bmmType string
}

// suffixFits holds the [valueFit] of every suffix of every datatype in
// [allowedSuffixes], plus DV_CODED_TEXT's `|other`, read off the RM once. An
// attribute path rminfo does not resolve leaves bmmType empty, which
// [valueFit.admits] refuses; TestSuffixFitsResolveEverySuffix keeps that from
// happening.
var suffixFits = func() map[string]map[string]valueFit {
	out := make(map[string]map[string]valueFit, len(allowedSuffixes))
	for rmType, allowed := range allowedSuffixes {
		suffixes := maps.Clone(allowed)
		if rmType == "DV_CODED_TEXT" {
			suffixes["other"] = true
		}
		fits := make(map[string]valueFit, len(suffixes))
		for suffix := range suffixes {
			path, own := suffixAttrPaths[rmType][suffix]
			if !own {
				path = orderedSuffixAttrPaths[suffix]
			}
			bmm, _ := attrBMMType(rmType, path)
			fits[suffix] = valueFit{attr: rmType + "." + strings.ReplaceAll(path, "/", "."), bmmType: bmm}
		}
		out[rmType] = fits
	}
	return out
}()

// stringLeafFit is the fit of a STRING leaf's bare value: an RM String.
var stringLeafFit = valueFit{attr: stringLeafType, bmmType: "String"}

// attrBMMType walks a "/"-separated attribute path down from rmType through
// rminfo and returns the RM type of its last attribute.
func attrBMMType(rmType, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	t := rmType
	for attr := range strings.SplitSeq(path, "/") {
		next, ok := rminfo.Default.AttributeRMType(bmmtype.Class(t), attr)
		if !ok {
			return "", false
		}
		t = next
	}
	return t, true
}

// decodeTarget returns a new Go value of the type canonical JSON decodes an
// RM primitive of type bmmType into, or nil for a type this check does not
// know.
func decodeTarget(bmmType string) any {
	switch bmmType {
	case "String":
		return new(string)
	case "Boolean":
		return new(bool)
	case "Integer":
		return new(rm.Integer)
	case "Integer64":
		return new(rm.Integer64)
	case "Real":
		return new(rm.Real)
	case "Octet":
		return new([]byte)
	}
	return nil
}

// admits reports whether canonical JSON decodes v as this attribute. v is
// written as JSON exactly as the rebuilt tree is ([decodeFlatMap] marshals it
// with encoding/json), then decoded into the attribute's Go type.
func (f valueFit) admits(v any) bool {
	// A JSON string always decodes as a String and a JSON Boolean as a
	// Boolean, so the commonest values skip the round trip.
	switch v.(type) {
	case string:
		if f.bmmType == "String" {
			return true
		}
	case bool:
		if f.bmmType == "Boolean" {
			return true
		}
	}
	target := decodeTarget(f.bmmType)
	if target == nil {
		return false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return jsonv2.Unmarshal(b, target) == nil
}

// takes says, in words, what the attribute admits, for a refusal message.
func (f valueFit) takes() string {
	switch f.bmmType {
	case "String":
		return "a string"
	case "Boolean":
		return "a boolean"
	case "Integer":
		return "a whole number in the Integer range, bare or quoted"
	case "Integer64":
		return "a whole number in the Integer64 range, bare or quoted"
	case "Real":
		return "a number, bare or quoted"
	case "Octet":
		return "a base64 string"
	}
	return "nothing this codec can check"
}

// check returns nil when v, the value of suffix, fits the attribute, and
// otherwise its [valueFit.refusal], naming the FLAT key keyOf gives for suffix.
// The name is built only for a refusal.
func (f valueFit) check(keyOf func(string) string, suffix string, v any) error {
	if f.admits(v) {
		return nil
	}
	return f.refusal(suffixLabel(keyOf, suffix), v)
}

// refusal is the error for a value v that does not fit the attribute:
// [ErrUnsupportedDatatype], naming label (the FLAT key) and the attribute, and
// the JSON kind of v but never v itself.
func (f valueFit) refusal(label string, v any) error {
	typ := f.bmmType
	if typ == "" {
		typ = "an attribute rminfo does not resolve"
	}
	return fmt.Errorf("%w: %s does not fit %s (%s), which takes %s; got a JSON %s",
		ErrUnsupportedDatatype, label, f.attr, typ, f.takes(), jsonKind(v))
}

// jsonKind names the JSON kind of a decoded value.
func jsonKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	return fmt.Sprintf("%T", v)
}

// suffixLabel names the FLAT key a suffix arrived under for a refusal, quoted.
// With no keyOf it names the suffix alone.
func suffixLabel(keyOf func(string) string, suffix string) string {
	if keyOf != nil {
		return strconv.Quote(keyOf(suffix))
	}
	if suffix == "" {
		return "the bare value"
	}
	return "|" + suffix
}

// checkSuffixFits checks every value in sfx, a suffix map of rmType that has
// passed [checkSuffixAllowlist], against the RM attribute it fills. Of the
// values that do not fit it refuses the first in sorted suffix order, so a
// leaf with two names the same one on every run. keyOf names the FLAT key each
// suffix arrived under, and is called only for the refusal.
func checkSuffixFits(rmType string, sfx map[string]any, keyOf func(string) string) error {
	fits, known := suffixFits[rmType]
	if !known {
		return nil
	}
	first, refused := "", false
	for suffix, v := range sfx {
		if fit, ok := fits[suffix]; ok && fit.admits(v) {
			continue
		}
		if !refused || suffix < first {
			first, refused = suffix, true
		}
	}
	if !refused {
		return nil
	}
	fit, ok := fits[first]
	if !ok {
		// checkSuffixAllowlist admitted a suffix suffixFits does not hold: the
		// two tables have drifted apart.
		return fmt.Errorf("%w: %s has no attribute recorded for %s", ErrUnsupportedDatatype, rmType, suffixLabel(keyOf, first))
	}
	return fit.refusal(suffixLabel(keyOf, first), sfx[first])
}

// checkRawFits returns nil when a `|raw` fragment of type typ fits the RM
// attribute it fills, declared as holds: typ is holds or an RM subtype of it,
// the test canonical JSON applies when it decodes that attribute. Otherwise it
// refuses the fragment, wrapping [ErrUnsupportedDatatype] and naming the
// `|raw` key keyOf gives, so the misfit never surfaces as a canonical-JSON
// error naming a path the caller did not write (REQ-053). The refusal names
// the fragment's class only when the RM defines it, so text the caller wrote
// as a `_type` never reaches the message. A holds the RM does not define (a
// Web Template node type spelled outside the BMM) gives nothing to judge
// against, and the fragment is left to canonical JSON.
func checkRawFits(typ, holds string, keyOf func(string) string) error {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		// rminfo.Default implements Hierarchy, so this cannot happen; with no
		// hierarchy to ask, canonical JSON judges the fragment.
		return nil
	}
	class, holdsClass := bmmtype.Class(typ), bmmtype.Class(holds)
	if conforms, _ := h.ConformsTo(class, holdsClass); conforms {
		return nil
	}
	if _, defined := h.Ancestors(class); !defined {
		return fmt.Errorf("%w: %s holds a fragment whose _type the RM does not define, where the attribute it fills takes a %s",
			ErrUnsupportedDatatype, suffixLabel(keyOf, "raw"), holdsClass)
	}
	if _, defined := h.Ancestors(holdsClass); !defined {
		return nil
	}
	return fmt.Errorf("%w: %s holds a %s fragment, which does not fit the attribute it fills, a %s",
		ErrUnsupportedDatatype, suffixLabel(keyOf, "raw"), class, holdsClass)
}

// checkRawDecodes returns nil when canonical JSON decodes frag, a `|raw`
// fragment whose _type typ has passed [checkRawFits], as that _type. That is
// the decode canonical JSON gives the fragment where it is placed: it
// dispatches on _type through [typereg], and the `_type` check has already
// made typ fit the attribute. Otherwise it refuses the fragment, wrapping
// [ErrUnsupportedDatatype] and naming the `|raw` key keyOf gives, so a member
// that does not fit, or a class no value can be, never surfaces as a
// canonical-JSON error naming a path the caller did not write (REQ-053). The
// refusal names the class, which [checkRawFits] has found in the RM, and
// nothing the decode said, since that names the member's path inside the
// fragment and can quote its value. A _type canonical JSON does not dispatch
// on (an abstract class, or a class spelled with parameters) is named by its
// class alone, so no parameter text the caller wrote reaches the message.
func checkRawDecodes(frag map[string]any, typ string, keyOf func(string) string) error {
	if _, concrete := typereg.Default.Lookup(typ); !concrete {
		return fmt.Errorf("%w: %s holds a %s fragment, but canonical JSON decodes a value only as a concrete class spelled without parameters",
			ErrUnsupportedDatatype, suffixLabel(keyOf, "raw"), bmmtype.Class(typ))
	}
	b, err := json.Marshal(frag)
	if err == nil {
		_, err = typereg.Default.Decode(b)
	}
	if err != nil {
		return fmt.Errorf("%w: %s holds a %s fragment whose members canonical JSON does not decode as a %s",
			ErrUnsupportedDatatype, suffixLabel(keyOf, "raw"), typ, typ)
	}
	return nil
}
