package simplified

// REQ-053 — "A value must fit its attribute", for the `|raw` carrier: a
// pre-serialised canonical fragment is placed whole at the RM attribute its
// key addresses, so a fragment whose `_type` is neither that attribute's
// declared type nor an RM subtype of it would reach canonical JSON and fail
// there, naming a canonical path the caller never wrote. Decode refuses it
// first, wrapping ErrUnsupportedDatatype and naming the `|raw` key. The bar is
// the RM attribute, not the datatype the Web Template names, so a fragment
// canonical JSON admits still decodes.

import (
	"errors"
	"maps"
	"strconv"
	"strings"
	"testing"
)

// Canonical fragments of a few datatypes, for the `|raw` cases below.
var (
	rawQuantity = map[string]any{"_type": "DV_QUANTITY", "magnitude": 1, "units": "kg"}
	rawText     = map[string]any{"_type": "DV_TEXT", "value": "x"}
	rawCodePhr  = map[string]any{
		"_type": "CODE_PHRASE", "code_string": "x",
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
	}
)

// Values a misfit `|raw` member holds, which no refusal may echo: a string
// where the attribute takes a number, and a number where it takes a string.
const (
	rawSecretText   = "secret-raw-0451"
	rawSecretNumber = 4510451
)

// Fragments whose `_type` fits the attribute each fills but whose members
// canonical JSON does not decode as that `_type`, for the member-misfit cases
// below.
var (
	misfitText       = map[string]any{"_type": "DV_TEXT", "value": rawSecretNumber}
	misfitCodePhrase = map[string]any{
		"_type": "CODE_PHRASE", "code_string": rawSecretNumber,
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
	}
	misfitCodedText = map[string]any{"_type": "DV_CODED_TEXT", "value": "x", "defining_code": misfitCodePhrase}
	misfitQuantity  = map[string]any{"_type": "DV_QUANTITY", "magnitude": rawSecretText, "units": "kg"}
	misfitCount     = map[string]any{"_type": "DV_COUNT", "magnitude": rawSecretText}
	misfitDateTime  = map[string]any{"_type": "DV_DATE_TIME", "value": rawSecretNumber}
	misfitDuration  = map[string]any{"_type": "DV_DURATION", "value": rawSecretNumber}
	misfitMultimed  = map[string]any{
		"_type": "DV_MULTIMEDIA", "media_type": rawCodePhr, "size": rawSecretText,
		"uri": map[string]any{"_type": "DV_URI", "value": "http://x"},
	}
	misfitParsable   = map[string]any{"_type": "DV_PARSABLE", "value": "x", "formalism": rawSecretNumber}
	misfitIdentifier = map[string]any{"_type": "DV_IDENTIFIER", "id": rawSecretNumber}
)

// TestREQ053RawFragmentMustFitItsAttribute — REQ-053 / REQ-140: a `|raw`
// fragment that canonical JSON would refuse at the RM attribute it fills is
// refused in every mode, wrapping ErrUnsupportedDatatype and naming the `|raw`
// key: one whose `_type` the attribute does not admit, one whose `_type` it
// admits but whose members do not decode as that `_type`, and one naming an
// abstract class no value can be. The refusal echoes neither a member's value
// nor the canonical path of the member inside the fragment. It holds at every
// position a fragment is placed: the underscore families that are one
// datatype (`_name`, `_null_reason`, `_null_flavour`, `_language`, `_charset`,
// `_thumbnail`, `_accuracy`), the sub-objects of the recursive grammar (an
// interval bound held by value and one held as DV_ORDERED, a reference range's
// meaning and bound, a term mapping's target and purpose), the feeder audit's
// content and item id, and the Web Template's own leaves (a collapsed
// ELEMENT's value, an interval leaf's bound, an in-context leaf, a node with
// children). WithTemplate and the STRUCTURED form refuse it the same way.
func TestREQ053RawFragmentMustFitItsAttribute(t *testing.T) {
	wt, c := conformanceWT(t)
	ref := rmattrCount + "/_other_reference_ranges:0"
	feeder := rmattrObs + "/_feeder_audit"
	for name, tc := range map[string]struct {
		extra map[string]any
		key   string
	}{
		"_name, no DV_TEXT": {
			extra: map[string]any{rootNameKey + "|raw": rawQuantity},
			key:   rootNameKey + "|raw",
		},
		"_name, a _type the RM does not define": {
			extra: map[string]any{rootNameKey + "|raw": map[string]any{"_type": "NO_SUCH_TYPE", "value": "x"}},
			key:   rootNameKey + "|raw",
		},
		"_null_reason, no DV_TEXT": {
			extra: map[string]any{
				rmattrCount:                                nil,
				rmattrCount + "/_null_flavour|code":        "253",
				rmattrCount + "/_null_flavour|terminology": "openehr",
				rmattrCount + "/_null_flavour|value":       "unknown",
				rmattrCount + "/_null_reason|raw":          rawQuantity,
			},
			key: rmattrCount + "/_null_reason|raw",
		},
		"_null_flavour, a DV_TEXT where the attribute is a DV_CODED_TEXT": {
			extra: map[string]any{rmattrCount + "/_null_flavour|raw": rawText},
			key:   rmattrCount + "/_null_flavour|raw",
		},
		"_language, no CODE_PHRASE": {
			extra: map[string]any{rmattrText: "t", rmattrText + "/_language|raw": rawQuantity},
			key:   rmattrText + "/_language|raw",
		},
		"_charset, no CODE_PHRASE": {
			extra: map[string]any{rmattrParsable: "x", rmattrParsable + "|formalism": "f", rmattrParsable + "/_charset|raw": rawQuantity},
			key:   rmattrParsable + "/_charset|raw",
		},
		"_thumbnail, no DV_MULTIMEDIA": {
			extra: map[string]any{
				rmattrMultimedia: "http://x", rmattrMultimedia + "|mediatype": "text/plain", rmattrMultimedia + "|size": 1,
				rmattrMultimedia + "/_thumbnail|raw": rawQuantity,
			},
			key: rmattrMultimedia + "/_thumbnail|raw",
		},
		"_accuracy, no DV_DURATION": {
			extra: map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00", rmattrDateTime + "/_accuracy|raw": rawQuantity},
			key:   rmattrDateTime + "/_accuracy|raw",
		},
		"_normal_range bound held by value, another DV_ORDERED": {
			extra: map[string]any{rmattrCount: 7, rmattrCount + "/_normal_range/upper": 8, rmattrCount + "/_normal_range/lower|raw": rawQuantity},
			key:   rmattrCount + "/_normal_range/lower|raw",
		},
		"_normal_range bound held as DV_ORDERED, no DV_ORDERED": {
			extra: map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00", rmattrDateTime + "/_normal_range/lower|raw": rawText},
			key:   rmattrDateTime + "/_normal_range/lower|raw",
		},
		"reference range meaning, no DV_TEXT": {
			extra: map[string]any{rmattrCount: 7, ref + "/lower": 1, ref + "/upper": 2, ref + "/meaning|raw": rawQuantity},
			key:   ref + "/meaning|raw",
		},
		"reference range bound, no DV_ORDERED": {
			extra: map[string]any{rmattrCount: 7, ref + "/lower|raw": rawText, ref + "/upper": 2, ref + "/meaning": "normal"},
			key:   ref + "/lower|raw",
		},
		"term mapping target, no CODE_PHRASE": {
			extra: map[string]any{rmattrText: "t", rmattrText + "/_mapping:0|match": "=", rmattrText + "/_mapping:0/target|raw": rawQuantity},
			key:   rmattrText + "/_mapping:0/target|raw",
		},
		"term mapping purpose, a DV_TEXT where the attribute is a DV_CODED_TEXT": {
			extra: map[string]any{
				rmattrText: "t", rmattrText + "/_mapping:0|match": "=",
				rmattrText + "/_mapping:0/target|code": "x", rmattrText + "/_mapping:0/target|terminology": "y",
				rmattrText + "/_mapping:0/purpose|raw": rawText,
			},
			key: rmattrText + "/_mapping:0/purpose|raw",
		},
		"feeder audit original_content, no DV_ENCAPSULATED": {
			extra: map[string]any{feeder + "/originating_system_audit|system_id": "s", feeder + "/original_content|raw": rawQuantity},
			key:   feeder + "/original_content|raw",
		},
		"feeder audit item id, no DV_IDENTIFIER": {
			extra: map[string]any{feeder + "/originating_system_audit|system_id": "s", feeder + "/originating_system_item_id:0|raw": rawQuantity},
			key:   feeder + "/originating_system_item_id:0|raw",
		},
		"collapsed ELEMENT value, no DATA_VALUE": {
			extra: map[string]any{rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil, rmattrElement + "|raw": rawCodePhr},
			key:   rmattrElement + "|raw",
		},
		"interval leaf bound, no DV_ORDERED": {
			extra: map[string]any{rmattrInterval + "/lower|raw": rawText, rmattrInterval + "/upper|magnitude": 2.0, rmattrInterval + "/upper|unit": "Unit"},
			key:   rmattrInterval + "/lower|raw",
		},
		"in-context EVENT time, no DV_DATE_TIME": {
			extra: map[string]any{rmattrObs + "/any_event:0/time": nil, rmattrObs + "/any_event:0/time|raw": rawQuantity},
			key:   rmattrObs + "/any_event:0/time|raw",
		},
		"in-context ENTRY language, no CODE_PHRASE": {
			extra: map[string]any{rmattrObs + "/language|code": nil, rmattrObs + "/language|terminology": nil, rmattrObs + "/language|raw": rawQuantity},
			key:   rmattrObs + "/language|raw",
		},
		"COMPOSITION category, no DV_CODED_TEXT": {
			extra: map[string]any{rmattrRoot + "/category|raw": rawQuantity},
			key:   rmattrRoot + "/category|raw",
		},
		"node with children, no OBSERVATION": {
			extra: map[string]any{rmattrObs + "|raw": rawQuantity},
			key:   rmattrObs + "|raw",
		},
		// The `_type` fits, but a member does not decode as that `_type`.
		"_name, a DV_TEXT member that does not fit": {
			extra: map[string]any{rootNameKey + "|raw": misfitText},
			key:   rootNameKey + "|raw",
		},
		"_null_reason, a DV_TEXT member that does not fit": {
			extra: map[string]any{
				rmattrCount:                                nil,
				rmattrCount + "/_null_flavour|code":        "253",
				rmattrCount + "/_null_flavour|terminology": "openehr",
				rmattrCount + "/_null_flavour|value":       "unknown",
				rmattrCount + "/_null_reason|raw":          misfitText,
			},
			key: rmattrCount + "/_null_reason|raw",
		},
		"_null_flavour, a DV_CODED_TEXT member that does not fit": {
			extra: map[string]any{rmattrCount: nil, rmattrCount + "/_null_flavour|raw": misfitCodedText},
			key:   rmattrCount + "/_null_flavour|raw",
		},
		"_language, a CODE_PHRASE member that does not fit": {
			extra: map[string]any{rmattrText: "t", rmattrText + "/_language|raw": misfitCodePhrase},
			key:   rmattrText + "/_language|raw",
		},
		"_charset, a CODE_PHRASE member that does not fit": {
			extra: map[string]any{rmattrParsable: "x", rmattrParsable + "|formalism": "f", rmattrParsable + "/_charset|raw": misfitCodePhrase},
			key:   rmattrParsable + "/_charset|raw",
		},
		"_thumbnail, a DV_MULTIMEDIA member that does not fit": {
			extra: map[string]any{
				rmattrMultimedia: "http://x", rmattrMultimedia + "|mediatype": "text/plain", rmattrMultimedia + "|size": 1,
				rmattrMultimedia + "/_thumbnail|raw": misfitMultimed,
			},
			key: rmattrMultimedia + "/_thumbnail|raw",
		},
		"_accuracy, a DV_DURATION member that does not fit": {
			extra: map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00", rmattrDateTime + "/_accuracy|raw": misfitDuration},
			key:   rmattrDateTime + "/_accuracy|raw",
		},
		"_normal_range bound held by value, a member that does not fit": {
			extra: map[string]any{rmattrCount: 7, rmattrCount + "/_normal_range/upper": 8, rmattrCount + "/_normal_range/lower|raw": misfitCount},
			key:   rmattrCount + "/_normal_range/lower|raw",
		},
		"_normal_range bound held as DV_ORDERED, a member that does not fit": {
			extra: map[string]any{rmattrDateTime: "2022-01-12T13:22:34+01:00", rmattrDateTime + "/_normal_range/lower|raw": misfitDateTime},
			key:   rmattrDateTime + "/_normal_range/lower|raw",
		},
		"reference range meaning, a member that does not fit": {
			extra: map[string]any{rmattrCount: 7, ref + "/lower": 1, ref + "/upper": 2, ref + "/meaning|raw": misfitText},
			key:   ref + "/meaning|raw",
		},
		"reference range bound, a member that does not fit": {
			extra: map[string]any{rmattrCount: 7, ref + "/lower|raw": misfitCount, ref + "/upper": 2, ref + "/meaning": "normal"},
			key:   ref + "/lower|raw",
		},
		"term mapping target, a member that does not fit": {
			extra: map[string]any{rmattrText: "t", rmattrText + "/_mapping:0|match": "=", rmattrText + "/_mapping:0/target|raw": misfitCodePhrase},
			key:   rmattrText + "/_mapping:0/target|raw",
		},
		"term mapping purpose, a member that does not fit": {
			extra: map[string]any{
				rmattrText: "t", rmattrText + "/_mapping:0|match": "=",
				rmattrText + "/_mapping:0/target|code": "x", rmattrText + "/_mapping:0/target|terminology": "y",
				rmattrText + "/_mapping:0/purpose|raw": misfitCodedText,
			},
			key: rmattrText + "/_mapping:0/purpose|raw",
		},
		"feeder audit original_content, a member that does not fit": {
			extra: map[string]any{feeder + "/originating_system_audit|system_id": "s", feeder + "/original_content|raw": misfitParsable},
			key:   feeder + "/original_content|raw",
		},
		"feeder audit item id, a member that does not fit": {
			extra: map[string]any{feeder + "/originating_system_audit|system_id": "s", feeder + "/originating_system_item_id:0|raw": misfitIdentifier},
			key:   feeder + "/originating_system_item_id:0|raw",
		},
		"collapsed ELEMENT value, a member that does not fit": {
			extra: map[string]any{rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil, rmattrElement + "|raw": misfitQuantity},
			key:   rmattrElement + "|raw",
		},
		"collapsed ELEMENT value, a class spelled with parameters": {
			extra: map[string]any{
				rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil,
				rmattrElement + "|raw": map[string]any{"_type": "DV_INTERVAL<" + rawSecretText + ">"},
			},
			key: rmattrElement + "|raw",
		},
		"collapsed ELEMENT value, an abstract class": {
			extra: map[string]any{rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil, rmattrElement + "|raw": map[string]any{"_type": "DATA_VALUE"}},
			key:   rmattrElement + "|raw",
		},
		"interval leaf bound, a member that does not fit": {
			extra: map[string]any{rmattrInterval + "/lower|raw": misfitQuantity, rmattrInterval + "/upper|magnitude": 2.0, rmattrInterval + "/upper|unit": "Unit"},
			key:   rmattrInterval + "/lower|raw",
		},
		"interval leaf bound, an abstract class": {
			extra: map[string]any{rmattrInterval + "/lower|raw": map[string]any{"_type": "DV_ORDERED"}, rmattrInterval + "/upper|magnitude": 2.0, rmattrInterval + "/upper|unit": "Unit"},
			key:   rmattrInterval + "/lower|raw",
		},
		"in-context EVENT time, a member that does not fit": {
			extra: map[string]any{rmattrObs + "/any_event:0/time": nil, rmattrObs + "/any_event:0/time|raw": misfitDateTime},
			key:   rmattrObs + "/any_event:0/time|raw",
		},
		"in-context ENTRY language, a member that does not fit": {
			extra: map[string]any{rmattrObs + "/language|code": nil, rmattrObs + "/language|terminology": nil, rmattrObs + "/language|raw": misfitCodePhrase},
			key:   rmattrObs + "/language|raw",
		},
		"COMPOSITION category, a member that does not fit": {
			extra: map[string]any{rmattrRoot + "/category|raw": misfitCodedText},
			key:   rmattrRoot + "/category|raw",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(tc.extra)
			err := refusedInEveryMode(t, wt, body)
			if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), `"`+tc.key+`"`) {
				t.Errorf("FLAT: decode = %v, want ErrUnsupportedDatatype naming %q", err, tc.key)
			}
			for _, unsaid := range []string{rawSecretText, strconv.Itoa(rawSecretNumber), "within", "canjson"} {
				if strings.Contains(err.Error(), unsaid) {
					t.Errorf("FLAT: decode = %v, which says %q: a refusal names the key, not a member's value or its canonical path", err, unsaid)
				}
			}
			if _, err := UnmarshalFlat(mustJSON(t, body), wt, WithTemplate(c)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("FLAT WithTemplate: decode = %v, want ErrUnsupportedDatatype", err)
			}
			if structured := structuredRefusedInEveryMode(t, wt, body); !errors.Is(structured, ErrUnsupportedDatatype) {
				t.Errorf("STRUCTURED: decode = %v, want ErrUnsupportedDatatype", structured)
			}
		})
	}
}

// TestREQ053RawFragmentFitsTheAttributeNotTheDatatype — REQ-053: the `|raw`
// check adds no strictness of its own. Its bar is the RM attribute the
// fragment fills, which can be wider than the datatype the key's position
// names, so each of these decodes: an RM subtype of the declared type, any
// DATA_VALUE at a collapsed ELEMENT (the encoder stamps a substituted value
// with its own type there), another DV_ORDERED at an interval bound the RM
// holds as DV_ORDERED, and the other DV_ENCAPSULATED at the feeder audit's
// content.
func TestREQ053RawFragmentFitsTheAttributeNotTheDatatype(t *testing.T) {
	wt, _ := conformanceWT(t)
	ref := rmattrCount + "/_other_reference_ranges:0"
	coded := map[string]any{
		"_type": "DV_CODED_TEXT", "value": "Routine checkup",
		"defining_code": map[string]any{
			"_type": "CODE_PHRASE", "code_string": "at0001",
			"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
		},
	}
	for name, extra := range map[string]map[string]any{
		"_name, a DV_CODED_TEXT": {rootNameKey + "|raw": coded},
		"collapsed DV_QUANTITY ELEMENT, a DV_TEXT": {
			rmattrElement + "|magnitude": nil, rmattrElement + "|unit": nil, rmattrElement + "|raw": rawText,
		},
		"DV_DATE_TIME _normal_range, a DV_DATE bound": {
			rmattrDateTime: "2022-01-12T13:22:34+01:00",
			rmattrDateTime + "/_normal_range/lower|raw": map[string]any{"_type": "DV_DATE", "value": "2022-01-01"},
		},
		"reference range of a DV_COUNT, a DV_QUANTITY bound": {
			rmattrCount: 7, ref + "/lower|raw": rawQuantity, ref + "/upper": 2, ref + "/meaning": "normal",
		},
		"DV_INTERVAL<DV_QUANTITY> leaf, a DV_COUNT bound": {
			rmattrInterval + "/lower|raw":       map[string]any{"_type": "DV_COUNT", "magnitude": 1},
			rmattrInterval + "/upper|magnitude": 2.0,
			rmattrInterval + "/upper|unit":      "Unit",
		},
		"feeder audit original_content, a DV_MULTIMEDIA": {
			rmattrObs + "/_feeder_audit/originating_system_audit|system_id": "s",
			rmattrObs + "/_feeder_audit/original_content|raw": map[string]any{
				"_type": "DV_MULTIMEDIA", "media_type": rawCodePhr, "size": 1,
				"uri": map[string]any{"_type": "DV_URI", "value": "http://x"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			decodeRMAttr(t, wt, rmattrBody(extra))
		})
	}
}

// TestREQ053CheckRawFitsNamesNoCallerText — REQ-053: a fragment whose `_type` the RM
// does not define is refused without echoing that `_type`, which is text the
// caller wrote; one the RM defines is named by its class.
func TestREQ053CheckRawFitsNamesNoCallerText(t *testing.T) {
	const secret = "secret-value-0451"
	keyOf := func(string) string { return "p/x|raw" }
	err := checkRawFits(secret, "DV_TEXT", keyOf)
	if !errors.Is(err, ErrUnsupportedDatatype) || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), `"p/x|raw"`) {
		t.Errorf("checkRawFits(%q, DV_TEXT) = %v, want ErrUnsupportedDatatype naming the key and not the _type", secret, err)
	}
	err = checkRawFits("DV_QUANTITY", "DV_TEXT", keyOf)
	if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "DV_QUANTITY fragment") {
		t.Errorf("checkRawFits(DV_QUANTITY, DV_TEXT) = %v, want ErrUnsupportedDatatype naming DV_QUANTITY", err)
	}
}

// TestREQ053CheckRawFitsLeavesAnUndefinedAttributeToCanonicalJSON — REQ-053: the
// check adds no strictness of its own, so where the type the attribute is
// declared as is no RM class (a Web Template node type spelled outside the
// BMM), there is nothing to judge a fragment against and it is not refused.
func TestREQ053CheckRawFitsLeavesAnUndefinedAttributeToCanonicalJSON(t *testing.T) {
	if err := checkRawFits("DV_TEXT", "NO_SUCH_CLASS", nil); err != nil {
		t.Errorf("checkRawFits(DV_TEXT, NO_SUCH_CLASS) = %v, want nil", err)
	}
}

// TestREQ053RawAtANodeWithChildrenIsRefused — REQ-053 and REQ-170: `|raw`
// carries a datatype, so a `|raw` key that resolves to a node with children
// (the composition root, the EVENT_CONTEXT, a SECTION, an ENTRY, an EVENT, a
// CLUSTER, an ACTIVITY, an ISM_TRANSITION) is matched content of the wrong
// shape. It is refused in every mode, in FLAT and in STRUCTURED, wrapping
// ErrUnsupportedDatatype and naming the key, whatever the fragment's `_type`
// and whether or not the node has keys of its own. Before, a fragment whose
// `_type` fitted was placed whole at the node and then dropped without a word
// once the node's own keys rebuilt it, or failed in canonical JSON naming a
// path the caller did not write.
func TestREQ053RawAtANodeWithChildrenIsRefused(t *testing.T) {
	wt, c := conformanceWT(t)
	frag := func(typ string) map[string]any {
		return map[string]any{"_type": typ, "archetype_node_id": "at0000", "name": rawText}
	}
	ctxOnly := func(extra map[string]any) map[string]any {
		body := map[string]any{"ctx/language": "en", "ctx/territory": "US", "ctx/time": "2021-12-21T14:19:31.649613+01:00"}
		maps.Copy(body, extra)
		return body
	}
	instruction := rmattrSection + "/conformance_instruction"
	for name, tc := range map[string]struct {
		body map[string]any
		key  string
	}{
		"composition root":                {rmattrBody(map[string]any{rmattrRoot + "|raw": frag("COMPOSITION")}), rmattrRoot + "|raw"},
		"EVENT_CONTEXT":                   {rmattrBody(map[string]any{rmattrContext + "|raw": frag("EVENT_CONTEXT")}), rmattrContext + "|raw"},
		"SECTION":                         {rmattrBody(map[string]any{rmattrSection + "|raw": frag("SECTION")}), rmattrSection + "|raw"},
		"SECTION, a fragment of no fit":   {rmattrBody(map[string]any{rmattrSection + "|raw": rawText}), rmattrSection + "|raw"},
		"SECTION, no canonical object":    {rmattrBody(map[string]any{rmattrSection + "|raw": "x"}), rmattrSection + "|raw"},
		"OBSERVATION beside its own keys": {rmattrBody(map[string]any{rmattrObs + "|raw": frag("OBSERVATION")}), rmattrObs + "|raw"},
		"OBSERVATION alone":               {ctxOnly(map[string]any{rmattrObs + "|raw": frag("OBSERVATION")}), rmattrObs + "|raw"},
		"ADMIN_ENTRY alone":               {ctxOnly(map[string]any{rmattrSection + "/conformance_admin_entry|raw": frag("ADMIN_ENTRY")}), rmattrSection + "/conformance_admin_entry|raw"},
		"EVENT":                           {rmattrBody(map[string]any{rmattrEvent + "|raw": frag("POINT_EVENT")}), rmattrEvent + "|raw"},
		"CLUSTER":                         {rmattrBody(map[string]any{rmattrEvent + "/conformance_cluster|raw": frag("CLUSTER")}), rmattrEvent + "/conformance_cluster|raw"},
		"ACTIVITY":                        {rmattrBody(map[string]any{instruction + "/current_activity|raw": frag("ACTIVITY")}), instruction + "/current_activity|raw"},
		"ISM_TRANSITION":                  {rmattrBody(map[string]any{rmattrAction + "/transition|raw": frag("ISM_TRANSITION")}), rmattrAction + "/transition|raw"},
	} {
		t.Run(name, func(t *testing.T) {
			err := refusedInEveryMode(t, wt, tc.body)
			if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), `"`+tc.key+`"`) {
				t.Errorf("FLAT: decode = %v, want ErrUnsupportedDatatype naming %q", err, tc.key)
			}
			if _, err := UnmarshalFlat(mustJSON(t, tc.body), wt, WithTemplate(c)); !errors.Is(err, ErrUnsupportedDatatype) {
				t.Errorf("FLAT WithTemplate: decode = %v, want ErrUnsupportedDatatype", err)
			}
			structured := structuredRefusedInEveryMode(t, wt, tc.body)
			if !errors.Is(structured, ErrUnsupportedDatatype) || !strings.Contains(structured.Error(), `|raw"`) {
				t.Errorf("STRUCTURED: decode = %v, want ErrUnsupportedDatatype naming the |raw key", structured)
			}
		})
	}
}

// TestREQ053NullRawAtANodeWithChildrenIsAbsent — REQ-053: a null is absent
// wherever its key resolves, so a null `|raw` at a node with children is not
// refused for the datatype it would carry: it decodes, in both forms, as the
// body without it.
func TestREQ053NullRawAtANodeWithChildrenIsAbsent(t *testing.T) {
	wt, _ := conformanceWT(t)
	extra := map[string]any{rmattrObs + "|raw": nil}
	body := rmattrBody(extra)
	assertSameDecodeBothForms(t, wt, body, withoutNullKeys(body, extra))
}
