package simplified_test

// REQ-053 — an ELEMENT whose value admits several value types, or none, is a
// Web Template node with one child per type, each spelled `<type>_value`. Its
// value is spelled under the child of its type, and its own underscore
// attributes under the ELEMENT. A key at the ELEMENT itself, the spelling of
// earlier releases, reads as its first child.

import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

const (
	// choiceKey is the test_all_types EVALUATION's ELEMENT `choice`, whose value
	// admits a DV_QUANTITY or a DV_COUNT, in that order.
	choiceKey = "test_all_types/test_all_types_1:0/choice"
	// choicePath is the canonical path of that ELEMENT.
	choicePath = "/content[openEHR-EHR-EVALUATION.test_all_types.v1]/data[at0001]/items[at0009]"
	// createdKey is the ehrn_abdm media file's ELEMENT `created`, whose value
	// admits a DV_DATE_TIME or a DV_INTERVAL<DV_DATE_TIME>, in that order.
	createdKey = "opconsultation/document_attachment/media_file/created"
	// createdPath is the canonical path of that ELEMENT.
	createdPath = "/content[openEHR-EHR-ADMIN_ENTRY.document_attachment.v0]/data[at0003]/items[openEHR-EHR-CLUSTER.media_file.v1]/items[at0004]"
	// labresultKey is the conformance cluster's ELEMENT `labresult`, whose value
	// admits a DV_TEXT or a DV_QUANTITY, in that order, and anyKey its ELEMENT
	// `any`, which constrains no value type.
	labresultKey = "conformance-ehrbase.de.v0/conformance_section/conformance_observation/any_event:0/conformance_cluster/labresult"
	anyKey       = "conformance-ehrbase.de.v0/conformance_section/conformance_observation/any_event:0/conformance_cluster/any"
	// labresultPath and anyPath are the canonical paths of those ELEMENTs.
	labresultPath = "/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-OBSERVATION.conformance_observation.v0]/data[at0001]/events[at0002]/data[at0003]/items[openEHR-EHR-CLUSTER.conformance_cluster.v0]/items[at0003]"
	anyPath       = "/content[openEHR-EHR-SECTION.conformance_section.v0]/items[openEHR-EHR-OBSERVATION.conformance_observation.v0]/data[at0001]/events[at0002]/data[at0003]/items[openEHR-EHR-CLUSTER.conformance_cluster.v0]/items[at0005]"
)

// allTypesSet is the Web Template and the compiled template of the
// cross-format test_all_types OPT.
func allTypesSet(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	wt, compiled, _, _ := crossFormatComposerSet(t, "test_all_types")
	return wt, compiled
}

// conformanceBody is a FLAT body against the conformance template holding the
// mandatory context, the observation's event and leaves.
func conformanceBody(leaves map[string]any) map[string]any {
	const obs = "conformance-ehrbase.de.v0/conformance_section/conformance_observation"
	body := map[string]any{
		"ctx/language":                   "en",
		"ctx/territory":                  "US",
		"ctx/time":                       "2021-12-21T14:19:31.649613+01:00",
		obs + "/any_event:0/time":        "2021-12-21T16:02:58.0094262+01:00",
		obs + "/language|code":           "en",
		obs + "/language|terminology":    "ISO_639-1",
		obs + "/any_event:0/dv_count":    3,
		obs + "/encoding|code":           "UTF-8",
		obs + "/encoding|terminology":    "IANA_character-sets",
		"conformance-ehrbase.de.v0/_uid": "bb015b91-68c0-460f-a458-8ef06869b53c::ehrbase.org::1",
	}
	maps.Copy(body, leaves)
	return body
}

// formats are the two decodes of a FLAT body: as FLAT, and as the STRUCTURED
// body FLAT → STRUCTURED conversion gives it.
var formats = []struct {
	name   string
	decode func(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) (*rm.Composition, error)
}{
	{"FLAT", func(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) (*rm.Composition, error) {
		t.Helper()
		return simplified.UnmarshalFlat(mustJSON(t, body), wt, opts...)
	}},
	{"STRUCTURED", func(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) (*rm.Composition, error) {
		t.Helper()
		s, err := simplified.FlatToStructured(mustJSON(t, body))
		if err != nil {
			t.Fatalf("FlatToStructured: %v", err)
		}
		return simplified.UnmarshalStructured(s, wt, opts...)
	}},
}

// canonicalAt follows path, a canonical path whose bracketed segments hold an
// archetype_node_id, from the canonical JSON of comp and returns what is there.
// A bracketed segment selects the list member, or checks the single object,
// that carries the node id.
func canonicalAt(t *testing.T, comp *rm.Composition, path string) (any, bool) {
	t.Helper()
	var cur any = canonicalDoc(t, comp)
	for seg := range strings.SplitSeq(strings.TrimPrefix(path, "/"), "/") {
		attr, id, member := strings.Cut(seg, "[")
		id = strings.TrimSuffix(id, "]")
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = obj[attr]
		if !member {
			continue
		}
		if list, ok := cur.([]any); ok {
			cur = nil
			for _, e := range list {
				if m, ok := e.(map[string]any); ok && m["archetype_node_id"] == id {
					cur = m
					break
				}
			}
		}
		if m, ok := cur.(map[string]any); !ok || m["archetype_node_id"] != id {
			return nil, false
		}
	}
	return cur, cur != nil
}

// jsonOf is the JSON value s spells.
func jsonOf(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", s, err)
	}
	return v
}

// encodedUnder encodes comp as FLAT and returns its keys under prefix: the
// prefix itself, with a suffix, or below it.
func encodedUnder(t *testing.T, comp *rm.Composition, wt *webtemplate.WebTemplate, prefix string) map[string]any {
	t.Helper()
	data, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var flat map[string]any
	if err := json.Unmarshal(data, &flat); err != nil {
		t.Fatalf("unmarshal FLAT: %v", err)
	}
	out := make(map[string]any)
	for k, v := range flat {
		if k == prefix || strings.HasPrefix(k, prefix+"|") || strings.HasPrefix(k, prefix+"/") {
			out[k] = v
		}
	}
	return out
}

// TestREQ053ElementValueAlternativesDecode — REQ-053. The test_all_types
// `choice` ELEMENT admits a DV_QUANTITY or a DV_COUNT. A body spelling its
// value under `quantity_value` decodes to an ELEMENT holding that DV_QUANTITY,
// which validates against the template, and one spelling it under
// `count_value` to a DV_COUNT, in FLAT and STRUCTURED; each encodes back to the
// keys of its own child. The leaf spelling of earlier releases, a key at the
// ELEMENT itself, still decodes, as its first value type, and so does the
// EVALUATION's earlier spelling, test_all_types2.
func TestREQ053ElementValueAlternativesDecode(t *testing.T) {
	wt, compiled := allTypesSet(t)
	quantity := map[string]any{
		choiceKey + "/quantity_value|magnitude": 60.91,
		choiceKey + "/quantity_value|unit":      "mm[H20]",
	}
	for _, tc := range []struct {
		name   string
		leaves map[string]any
		value  string
		keys   map[string]any
	}{
		{name: "quantity_value", leaves: quantity, value: `{"_type":"DV_QUANTITY","magnitude":60.91,"units":"mm[H20]"}`, keys: quantity},
		{
			name: "count_value", leaves: map[string]any{choiceKey + "/count_value": 7},
			value: `{"_type":"DV_COUNT","magnitude":7}`, keys: map[string]any{choiceKey + "/count_value": 7.0},
		},
		{
			name:   "the earlier spelling at the ELEMENT",
			leaves: map[string]any{choiceKey + "|magnitude": 60.91, choiceKey + "|unit": "mm[H20]"},
			value:  `{"_type":"DV_QUANTITY","magnitude":60.91,"units":"mm[H20]"}`, keys: quantity,
		},
		{
			name:   "the earlier spelling at the ELEMENT, indexed",
			leaves: map[string]any{choiceKey + ":0|magnitude": 60.91, choiceKey + ":0|unit": "mm[H20]"},
			value:  `{"_type":"DV_QUANTITY","magnitude":60.91,"units":"mm[H20]"}`, keys: quantity,
		},
		{
			name: "the EVALUATION's earlier spelling",
			leaves: map[string]any{
				"test_all_types/test_all_types2:0/choice/quantity_value|magnitude": 60.91,
				"test_all_types/test_all_types2:0/choice/quantity_value|unit":      "mm[H20]",
			},
			value: `{"_type":"DV_QUANTITY","magnitude":60.91,"units":"mm[H20]"}`, keys: quantity,
		},
	} {
		for _, f := range formats {
			t.Run(tc.name+"/"+f.name, func(t *testing.T) {
				comp, err := f.decode(t, wt, completionBody(tc.leaves), simplified.WithTemplate(compiled), simplified.WithStrict())
				if err != nil {
					t.Fatalf("decode(%v) = %v", tc.leaves, err)
				}
				got, ok := canonicalAt(t, comp, choicePath+"/value")
				if !ok {
					t.Fatalf("decode(%v) holds no value at %s", tc.leaves, choicePath)
				}
				if want := jsonOf(t, tc.value); !reflect.DeepEqual(got, want) {
					t.Errorf("decode(%v): value = %s, want %s", tc.leaves, jsonText(got), tc.value)
				}
				for _, is := range validation.ValidateComposition(comp, compiled).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
				if got := encodedUnder(t, comp, wt, choiceKey); !reflect.DeepEqual(got, tc.keys) {
					t.Errorf("encode writes %v, want %v", got, tc.keys)
				}
			})
		}
	}
}

// TestREQ053ElementValueAlternativesInterval — REQ-053. The ehrn_abdm
// `created` ELEMENT admits a DV_DATE_TIME or a DV_INTERVAL<DV_DATE_TIME>, whose
// child is `interval_of_date_time_value`. Its interval grammar decodes to the
// interval and encodes back to the same keys, as the date-time child does.
func TestREQ053ElementValueAlternativesInterval(t *testing.T) {
	wt, compiled, flat, _ := crossFormatComposerSet(t, "ehrn_abdm")
	var body map[string]any
	if err := json.Unmarshal(flat, &body); err != nil {
		t.Fatalf("unmarshal ehrn_abdm flat.json: %v", err)
	}
	if _, ok := body[createdKey+"/date_time_value"]; !ok {
		t.Fatalf("ehrn_abdm flat.json holds no %s/date_time_value — fixture changed?", createdKey)
	}
	interval := map[string]any{
		createdKey + "/interval_of_date_time_value/lower":          "2022-02-03T04:05:06",
		createdKey + "/interval_of_date_time_value/upper":          "2022-02-04T04:05:06",
		createdKey + "/interval_of_date_time_value|upper_included": false,
	}
	for _, tc := range []struct {
		name   string
		leaves map[string]any
		value  string
	}{
		{name: "date_time_value", leaves: map[string]any{createdKey + "/date_time_value": "2022-02-03T04:05:06"}, value: `{"_type":"DV_DATE_TIME","value":"2022-02-03T04:05:06"}`},
		{
			name: "interval_of_date_time_value", leaves: interval,
			value: `{"_type":"DV_INTERVAL","lower":{"_type":"DV_DATE_TIME","value":"2022-02-03T04:05:06"},"upper":{"_type":"DV_DATE_TIME","value":"2022-02-04T04:05:06"},"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":false}`,
		},
	} {
		for _, f := range formats {
			t.Run(tc.name+"/"+f.name, func(t *testing.T) {
				edited := maps.Clone(body)
				maps.DeleteFunc(edited, func(k string, _ any) bool { return strings.HasPrefix(k, createdKey) })
				maps.Copy(edited, tc.leaves)
				comp, err := f.decode(t, wt, edited, simplified.WithTemplate(compiled))
				if err != nil {
					t.Fatalf("decode = %v", err)
				}
				got, ok := canonicalAt(t, comp, createdPath+"/value")
				if !ok {
					t.Fatalf("decode holds no value at %s", createdPath)
				}
				if want := jsonOf(t, tc.value); !reflect.DeepEqual(got, want) {
					t.Errorf("value = %s, want %s", jsonText(got), tc.value)
				}
				if got := encodedUnder(t, comp, wt, createdKey); !reflect.DeepEqual(got, tc.leaves) {
					t.Errorf("encode writes %v, want %v", got, tc.leaves)
				}
			})
		}
	}
}

// TestREQ053ElementValueAlternativesRefuseTwo — REQ-053. Keys that reach two
// value types of one ELEMENT, or the ELEMENT itself and one of its value types,
// spell two values of its one value. They are refused with ErrUnknownPath
// naming the ELEMENT, never merged into one value, in FLAT and STRUCTURED,
// whether or not each spells a whole value.
func TestREQ053ElementValueAlternativesRefuseTwo(t *testing.T) {
	allTypes, _ := allTypesSet(t)
	ehrn, _, flat, _ := crossFormatComposerSet(t, "ehrn_abdm")
	var ehrnBody map[string]any
	if err := json.Unmarshal(flat, &ehrnBody); err != nil {
		t.Fatalf("unmarshal ehrn_abdm flat.json: %v", err)
	}
	// withCreated is the ehrn_abdm body with leaves in place of its keys for
	// the ELEMENT `created`.
	withCreated := func(leaves map[string]any) map[string]any {
		b := maps.Clone(ehrnBody)
		maps.DeleteFunc(b, func(k string, _ any) bool { return strings.HasPrefix(k, createdKey) })
		maps.Copy(b, leaves)
		return b
	}
	for _, tc := range []struct {
		name    string
		wt      *webtemplate.WebTemplate
		body    map[string]any
		element string
	}{
		{
			name: "two value types", wt: allTypes, element: choiceKey,
			body: completionBody(map[string]any{
				choiceKey + "/quantity_value|magnitude": 60.91, choiceKey + "/quantity_value|unit": "mm[H20]",
				choiceKey + "/count_value": 7,
			}),
		},
		{
			name: "the ELEMENT and its first value type", wt: allTypes, element: choiceKey,
			body: completionBody(map[string]any{
				choiceKey + "|magnitude": 60.91, choiceKey + "|unit": "mm[H20]",
				choiceKey + "/quantity_value|magnitude": 60.91, choiceKey + "/quantity_value|unit": "mm[H20]",
			}),
		},
		{
			name: "half a value at the ELEMENT and half under its first value type", wt: allTypes, element: choiceKey,
			body: completionBody(map[string]any{
				choiceKey + "|magnitude": 60.91, choiceKey + "/quantity_value|unit": "mm[H20]",
			}),
		},
		{
			name: "a value decoration under another value type", wt: allTypes, element: choiceKey,
			body: completionBody(map[string]any{
				choiceKey + "/count_value":                                  7,
				choiceKey + "/quantity_value/_normal_range/lower|magnitude": 1.0,
				choiceKey + "/quantity_value/_normal_range/lower|unit":      "mm[H20]",
			}),
		},
		{
			name: "a value type and an interval", wt: ehrn, element: createdKey,
			body: func() map[string]any {
				b := maps.Clone(ehrnBody)
				b[createdKey+"/interval_of_date_time_value/lower"] = "2022-02-03T04:05:06"
				return b
			}(),
		},
		{
			name: "an interval and a value decoration under another value type", wt: ehrn, element: createdKey,
			body: withCreated(map[string]any{
				createdKey + "/interval_of_date_time_value/lower":   "2022-02-03T04:05:06",
				createdKey + "/date_time_value/_normal_range/lower": "2022-01-01T00:00:00",
				createdKey + "/date_time_value/_normal_range/upper": "2022-12-31T00:00:00",
			}),
		},
		{
			name: "an interval and a value decoration at the ELEMENT", wt: ehrn, element: createdKey,
			body: withCreated(map[string]any{
				createdKey + "/interval_of_date_time_value/lower": "2022-02-03T04:05:06",
				createdKey + "/_normal_range/lower":               "2022-01-01T00:00:00",
				createdKey + "/_normal_range/upper":               "2022-12-31T00:00:00",
			}),
		},
	} {
		for _, f := range formats {
			t.Run(tc.name+"/"+f.name, func(t *testing.T) {
				comp, err := f.decode(t, tc.wt, tc.body)
				if err == nil {
					t.Fatalf("decode = %s, want it refused", canonicalComposition(t, comp))
				}
				if !errors.Is(err, simplified.ErrUnknownPath) {
					t.Fatalf("decode: err = %v, want %v", err, simplified.ErrUnknownPath)
				}
				// The refusal is the ELEMENT's one value claimed twice, not
				// another ErrUnknownPath that happens to quote its key.
				const twoValues = "spell two values of the ELEMENT"
				if !strings.Contains(err.Error(), twoValues) {
					t.Errorf("decode: err = %q, want it to say %q", err, twoValues)
				}
				if element := tc.element[strings.LastIndexByte(tc.element, '/')+1:]; !strings.Contains(err.Error(), element) {
					t.Errorf("decode: err = %q, want it to name %q", err, element)
				}
			})
		}
	}
}

// TestREQ053ElementValueAlternativesAbsentKeys — REQ-053, REQ-170. Only a key the
// decode reads reaches a value type: a value type whose keys all hold null is
// absent, and a key the decode does not read is unmatched content, so neither
// stands beside another value type. A value type id the Web Template does not
// give the ELEMENT is unmatched content too.
func TestREQ053ElementValueAlternativesAbsentKeys(t *testing.T) {
	wt, compiled := allTypesSet(t)
	count := `{"_type":"DV_COUNT","magnitude":7}`
	for _, tc := range []struct {
		name      string
		leaves    map[string]any
		unmatched []string
	}{
		{
			name: "null value type",
			leaves: map[string]any{
				choiceKey + "/quantity_value|magnitude": nil, choiceKey + "/quantity_value|unit": nil,
				choiceKey + "/count_value": 7,
			},
		},
		{
			name:      "unmatched suffix under another value type",
			leaves:    map[string]any{choiceKey + "/quantity_value|x_note": "note", choiceKey + "/count_value": 7},
			unmatched: []string{choiceKey + "/quantity_value|x_note"},
		},
		{
			name:      "unknown value type",
			leaves:    map[string]any{choiceKey + "/boolean_value": true, choiceKey + "/count_value": 7},
			unmatched: []string{choiceKey + "/boolean_value"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reported []string
			comp, err := simplified.UnmarshalFlat(mustJSON(t, completionBody(tc.leaves)), wt,
				simplified.WithTemplate(compiled), simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
			if err != nil {
				t.Fatalf("UnmarshalFlat(%v) = %v", tc.leaves, err)
			}
			if got, _ := canonicalAt(t, comp, choicePath+"/value"); !reflect.DeepEqual(got, jsonOf(t, count)) {
				t.Errorf("UnmarshalFlat(%v): value = %s, want %s", tc.leaves, jsonText(got), count)
			}
			if !slices.Equal(reported, tc.unmatched) {
				t.Errorf("UnmarshalFlat(%v) reports %v, want %v", tc.leaves, reported, tc.unmatched)
			}
		})
	}
}

// TestREQ053ElementValueAlternativesUnderscoreFamilies — REQ-053, REQ-170. An
// ELEMENT with value types carries its own underscore attributes at the
// ELEMENT, so the same family under one of its value types is unmatched
// content there, in FLAT and STRUCTURED; a value decoration under it is read.
// A value decoration at the ELEMENT itself is the earlier leaf spelling and
// reads as its first value type's, as a key at the ELEMENT does.
func TestREQ053ElementValueAlternativesUnderscoreFamilies(t *testing.T) {
	wt, compiled := allTypesSet(t)
	quantity := `{"_type":"DV_QUANTITY","magnitude":60.91,"units":"mm[H20]"}`
	ranged := `{"_type":"DV_QUANTITY","magnitude":60.91,"units":"mm[H20]",` +
		`"normal_range":{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY","magnitude":1,"units":"mm[H20]"},` +
		`"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":true}}`
	for _, tc := range []struct {
		name      string
		leaves    map[string]any
		value     string
		unmatched map[string]string
	}{
		{
			name: "the ELEMENT's uid under a value type",
			leaves: map[string]any{
				choiceKey + "/quantity_value|magnitude": 60.91, choiceKey + "/quantity_value|unit": "mm[H20]",
				choiceKey + "/quantity_value/_uid": "e3b4ab3a-8b2f-4a5d-9b8f-2b7a3e6c1d40",
			},
			value: quantity,
			unmatched: map[string]string{
				"FLAT":       choiceKey + "/quantity_value/_uid",
				"STRUCTURED": "test_all_types/test_all_types_1/0/choice/0/quantity_value/0/_uid",
			},
		},
		{
			name: "a normal range under a value type",
			leaves: map[string]any{
				choiceKey + "/quantity_value|magnitude": 60.91, choiceKey + "/quantity_value|unit": "mm[H20]",
				choiceKey + "/quantity_value/_normal_range/lower|magnitude": 1,
				choiceKey + "/quantity_value/_normal_range/lower|unit":      "mm[H20]",
				choiceKey + "/quantity_value/_normal_range|upper_included":  false,
				choiceKey + "/quantity_value/_normal_range|upper_unbounded": true,
			},
			value: ranged,
		},
		{
			name: "a normal range at the ELEMENT, the earlier spelling",
			leaves: map[string]any{
				choiceKey + "|magnitude": 60.91, choiceKey + "|unit": "mm[H20]",
				choiceKey + "/_normal_range/lower|magnitude": 1,
				choiceKey + "/_normal_range/lower|unit":      "mm[H20]",
				choiceKey + "/_normal_range|upper_included":  false,
				choiceKey + "/_normal_range|upper_unbounded": true,
			},
			value: ranged,
		},
	} {
		for _, f := range formats {
			t.Run(tc.name+"/"+f.name, func(t *testing.T) {
				var reported []string
				comp, err := f.decode(t, wt, completionBody(tc.leaves),
					simplified.WithTemplate(compiled), simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
				if err != nil {
					t.Fatalf("decode(%v) = %v", tc.leaves, err)
				}
				if got, _ := canonicalAt(t, comp, choicePath+"/value"); !reflect.DeepEqual(got, jsonOf(t, tc.value)) {
					t.Errorf("decode(%v): value = %s, want %s", tc.leaves, jsonText(got), tc.value)
				}
				if got, _ := canonicalAt(t, comp, choicePath+"/uid"); got != nil {
					t.Errorf("decode(%v): ELEMENT uid = %s, want none", tc.leaves, jsonText(got))
				}
				var want []string
				if key := tc.unmatched[f.name]; key != "" {
					want = []string{key}
				}
				if !slices.Equal(reported, want) {
					t.Errorf("decode(%v) reports %v, want %v", tc.leaves, reported, want)
				}
			})
		}
	}
}

// TestREQ053ElementValueAlternativesFirstInterval — REQ-053. Where an ELEMENT's
// first value type is an interval, a key at the ELEMENT reads as that interval:
// its bounds and flags spelled at the ELEMENT, the leaf spelling of earlier
// releases, decode in FLAT and STRUCTURED, and encode writes them under the
// interval's child. The ELEMENT's own `_uid` beside them is the ELEMENT's,
// never a key of the interval.
func TestREQ053ElementValueAlternativesFirstInterval(t *testing.T) {
	wt, compiled, _, _ := crossFormatComposerSet(t, "ehrn_abdm")
	created := findNode(t, wt.Tree, createdPath)
	if len(created.Children) != 2 || created.Children[1].ID != "interval_of_date_time_value" {
		t.Fatalf("ehrn_abdm `created` has children %v, want date_time_value then interval_of_date_time_value — fixture changed?", created.Children)
	}
	// The interval first, as a template listing it first gives it.
	created.Children[0], created.Children[1] = created.Children[1], created.Children[0]
	const uid = "e3b4ab3a-8b2f-4a5d-9b8f-2b7a3e6c1d40"
	leaves := map[string]any{
		"ctx/language": "en", "ctx/territory": "IN", "ctx/composer_name": "Max Mustermann", "ctx/time": "2022-02-03T04:05:06",
		createdKey + "/lower":          "2022-02-03T04:05:06",
		createdKey + "/upper":          "2022-02-04T04:05:06",
		createdKey + "|upper_included": false,
		createdKey + "/_uid":           uid,
	}
	want := `{"_type":"DV_INTERVAL","lower":{"_type":"DV_DATE_TIME","value":"2022-02-03T04:05:06"},"upper":{"_type":"DV_DATE_TIME","value":"2022-02-04T04:05:06"},"lower_included":true,"upper_included":false,"lower_unbounded":false,"upper_unbounded":false}`
	wantUID := `{"_type":"HIER_OBJECT_ID","value":"` + uid + `"}`
	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			var reported []string
			comp, err := f.decode(t, wt, leaves, simplified.WithTemplate(compiled), simplified.WithStrict(),
				simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
			if len(reported) > 0 {
				t.Errorf("decode(%v) reports %v unmatched, want none", leaves, reported)
			}
			if err != nil {
				t.Fatalf("decode(%v) = %v", leaves, err)
			}
			if got, _ := canonicalAt(t, comp, createdPath+"/value"); !reflect.DeepEqual(got, jsonOf(t, want)) {
				t.Errorf("decode(%v): value = %s, want %s", leaves, jsonText(got), want)
			}
			if got, _ := canonicalAt(t, comp, createdPath+"/uid"); !reflect.DeepEqual(got, jsonOf(t, wantUID)) {
				t.Errorf("decode(%v): ELEMENT uid = %s, want %s", leaves, jsonText(got), wantUID)
			}
			wantKeys := map[string]any{
				createdKey + "/_uid":                                       uid,
				createdKey + "/interval_of_date_time_value/lower":          "2022-02-03T04:05:06",
				createdKey + "/interval_of_date_time_value/upper":          "2022-02-04T04:05:06",
				createdKey + "/interval_of_date_time_value|upper_included": false,
			}
			if got := encodedUnder(t, comp, wt, createdKey); !reflect.DeepEqual(got, wantKeys) {
				t.Errorf("encode writes %v, want %v", got, wantKeys)
			}
		})
	}
}

// TestREQ053ElementValueAlternativesFirstIntervalRaw — REQ-053. Where an
// ELEMENT's first value type is an interval, a value of no child's type is
// written as |raw under that interval child, and decode reads it back, as it
// reads the |raw at the ELEMENT that earlier releases wrote, in FLAT and
// STRUCTURED, with nothing unmatched. A |raw beside the interval's own keys
// is two spellings of one value, refused naming the |raw key.
func TestREQ053ElementValueAlternativesFirstIntervalRaw(t *testing.T) {
	wt, compiled, _, _ := crossFormatComposerSet(t, "ehrn_abdm")
	created := findNode(t, wt.Tree, createdPath)
	if len(created.Children) != 2 || created.Children[1].ID != "interval_of_date_time_value" {
		t.Fatalf("ehrn_abdm `created` has children %v, want date_time_value then interval_of_date_time_value — fixture changed?", created.Children)
	}
	created.Children[0], created.Children[1] = created.Children[1], created.Children[0]
	ctx := map[string]any{"ctx/language": "en", "ctx/territory": "IN", "ctx/composer_name": "Max Mustermann", "ctx/time": "2022-02-03T04:05:06"}
	text := map[string]any{"_type": "DV_TEXT", "value": "free text"}
	childRaw := createdKey + "/interval_of_date_time_value|raw"
	for _, key := range []string{childRaw, createdKey + "|raw"} {
		leaves := maps.Clone(ctx)
		leaves[key] = text
		for _, f := range formats {
			t.Run(f.name+" "+key, func(t *testing.T) {
				var reported []string
				comp, err := f.decode(t, wt, leaves, simplified.WithTemplate(compiled), simplified.WithStrict(),
					simplified.WithUnmatched(func(key string) { reported = append(reported, key) }))
				if len(reported) > 0 {
					t.Errorf("decode(%v) reports %v unmatched, want none", leaves, reported)
				}
				if err != nil {
					t.Fatalf("decode(%v) = %v", leaves, err)
				}
				if got, _ := canonicalAt(t, comp, createdPath+"/value"); !reflect.DeepEqual(got, text) {
					t.Errorf("decode(%v): value = %s, want %v", leaves, jsonText(got), text)
				}
				if got, want := encodedUnder(t, comp, wt, createdKey), map[string]any{childRaw: text}; !reflect.DeepEqual(got, want) {
					t.Errorf("encode writes %v, want %v", got, want)
				}
			})
		}
	}
	both := maps.Clone(ctx)
	both[childRaw] = text
	both[createdKey+"/interval_of_date_time_value/lower"] = "2022-02-03T04:05:06"
	for _, f := range formats {
		t.Run(f.name+" |raw beside the interval's keys", func(t *testing.T) {
			_, err := f.decode(t, wt, both)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "|raw is mutually exclusive") {
				t.Errorf("decode(%v) = %v, want ErrUnsupportedDatatype refusing the |raw beside the interval's keys", both, err)
			}
		})
	}
}

// findNode is the Web Template node under n whose aqlPath is path.
func findNode(t *testing.T, n *webtemplate.Node, path string) *webtemplate.Node {
	t.Helper()
	var found *webtemplate.Node
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		if n.AQLPath == path && found == nil {
			found = n
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(n)
	if found == nil {
		t.Fatalf("the Web Template has no node at %s", path)
	}
	return found
}

// elementAt is the ELEMENT at path in comp.
func elementAt(t *testing.T, comp *rm.Composition, path string) *rm.Element {
	t.Helper()
	item, err := rmpath.ItemAtPath(comp, path)
	if err != nil {
		t.Fatalf("ItemAtPath(%s): %v", path, err)
	}
	el, ok := item.(*rm.Element)
	if !ok {
		t.Fatalf("ItemAtPath(%s) = %T, want *rm.Element", path, item)
	}
	return el
}

// elementJSON is the canonical JSON of el.
func elementJSON(t *testing.T, el *rm.Element) string {
	t.Helper()
	b, err := canjson.Marshal(el)
	if err != nil {
		t.Fatalf("canjson.Marshal(ELEMENT): %v", err)
	}
	return string(b)
}

// TestREQ053ElementValueAlternativesEncode — REQ-053. Encode writes an
// ELEMENT's value under the child of its type; else under the child it
// substitutes for, a DV_CODED_TEXT at a DV_TEXT child in the coded-text
// suffixes, or an interval whose bounds name no type at the first interval
// child; else as |raw under the first child. The ELEMENT's own underscore
// attributes keep their keys at the ELEMENT, and the value's decorations go
// under the child. Each body decodes back to the value it was written from.
func TestREQ053ElementValueAlternativesEncode(t *testing.T) {
	allTypes, _ := allTypesSet(t)
	conformance, _ := corpusTarget(t)
	ehrn, _, ehrnFlat, _ := crossFormatComposerSet(t, "ehrn_abdm")
	var ehrnBody map[string]any
	if err := json.Unmarshal(ehrnFlat, &ehrnBody); err != nil {
		t.Fatalf("unmarshal ehrn_abdm flat.json: %v", err)
	}
	coded := &rm.DVCodedText{
		Value:        "value2",
		DefiningCode: rm.CodePhrase{CodeString: "at0007", TerminologyID: rm.TerminologyID{Value: "local"}},
	}
	codedKeys := func(at string) map[string]any {
		return map[string]any{at + "|code": "at0007", at + "|value": "value2", at + "|terminology": "local"}
	}
	nullFlavour := &rm.DVCodedText{
		Value:        "unknown",
		DefiningCode: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}},
	}
	for _, tc := range []struct {
		name    string
		wt      *webtemplate.WebTemplate
		body    map[string]any
		at, key string
		edit    func(el *rm.Element)
		want    map[string]any
	}{
		{
			name: "the child of its type", wt: allTypes, body: completionBody(map[string]any{choiceKey + "/count_value": 7}),
			at: choicePath, key: choiceKey,
			edit: func(el *rm.Element) { el.Value = &rm.DVCount{Magnitude: 9} },
			want: map[string]any{choiceKey + "/count_value": 9.0},
		},
		{
			name: "a coded text at a text child", wt: conformance, body: conformanceBody(map[string]any{labresultKey + "/text_value": "labresult 4"}),
			at: labresultPath, key: labresultKey,
			edit: func(el *rm.Element) { el.Value = coded },
			want: codedKeys(labresultKey + "/text_value"),
		},
		{
			name: "a coded text at an ELEMENT that constrains no value type", wt: conformance, body: conformanceBody(map[string]any{anyKey + "/text_value": "any"}),
			at: anyPath, key: anyKey,
			edit: func(el *rm.Element) { el.Value = coded },
			want: codedKeys(anyKey + "/coded_text_value"),
		},
		{
			name: "a type no child has", wt: allTypes, body: completionBody(map[string]any{choiceKey + "/count_value": 7}),
			at: choicePath, key: choiceKey,
			edit: func(el *rm.Element) { el.Value = &rm.DVText{Value: "free text"} },
			want: map[string]any{choiceKey + "/quantity_value|raw": map[string]any{"_type": "DV_TEXT", "value": "free text"}},
		},
		{
			name: "an interval whose bounds name no type", wt: ehrn, body: ehrnBody,
			at: createdPath, key: createdKey,
			edit: func(el *rm.Element) {
				el.Value = &rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}
			},
			want: map[string]any{
				createdKey + "/interval_of_date_time_value|lower_included":  false,
				createdKey + "/interval_of_date_time_value|upper_included":  false,
				createdKey + "/interval_of_date_time_value|lower_unbounded": true,
				createdKey + "/interval_of_date_time_value|upper_unbounded": true,
			},
		},
		{
			name: "a null flavour and no value", wt: allTypes, body: completionBody(map[string]any{choiceKey + "/count_value": 7}),
			at: choicePath, key: choiceKey,
			edit: func(el *rm.Element) { el.Value, el.NullFlavour = nil, nullFlavour },
			want: map[string]any{
				choiceKey + "/_null_flavour|code": "253", choiceKey + "/_null_flavour|value": "unknown",
				choiceKey + "/_null_flavour|terminology": "openehr",
			},
		},
		{
			name: "a uid on the ELEMENT and a normal range on its value", wt: allTypes, body: completionBody(map[string]any{choiceKey + "/count_value": 7}),
			at: choicePath, key: choiceKey,
			edit: func(el *rm.Element) {
				el.UID = &rm.HierObjectID{Value: "e3b4ab3a-8b2f-4a5d-9b8f-2b7a3e6c1d40"}
				el.Value = &rm.DVQuantity{
					Magnitude: 60.91, Units: "mm[H20]",
					NormalRange: &rm.DVInterval[rm.DVQuantity]{
						Lower: rm.DVQuantity{Magnitude: 1, Units: "mm[H20]"}, LowerIncluded: true, UpperUnbounded: true,
					},
				}
			},
			want: map[string]any{
				choiceKey + "/_uid":                                         "e3b4ab3a-8b2f-4a5d-9b8f-2b7a3e6c1d40",
				choiceKey + "/quantity_value|magnitude":                     60.91,
				choiceKey + "/quantity_value|unit":                          "mm[H20]",
				choiceKey + "/quantity_value/_normal_range/lower|magnitude": 1.0,
				choiceKey + "/quantity_value/_normal_range/lower|unit":      "mm[H20]",
				choiceKey + "/quantity_value/_normal_range|upper_included":  false,
				choiceKey + "/quantity_value/_normal_range|upper_unbounded": true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := simplified.UnmarshalFlat(mustJSON(t, tc.body), tc.wt)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			el := elementAt(t, comp, tc.at)
			tc.edit(el)
			want := elementJSON(t, el)
			if got := encodedUnder(t, comp, tc.wt, tc.key); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("encode writes %v, want %v", got, tc.want)
			}
			data, err := simplified.MarshalFlat(comp, tc.wt)
			if err != nil {
				t.Fatalf("MarshalFlat: %v", err)
			}
			back, err := simplified.UnmarshalFlat(data, tc.wt, simplified.WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat(MarshalFlat) = %v", err)
			}
			if got := elementJSON(t, elementAt(t, back, tc.at)); got != want {
				t.Errorf("round trip: ELEMENT = %s, want %s", got, want)
			}
		})
	}
}
