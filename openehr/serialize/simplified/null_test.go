package simplified

// REQ-053 — "A `null` value reads as absent": a JSON null held by a FLAT key,
// or by a STRUCTURED member or array element, is the absence of what holds it
// wherever the key resolves, never the zero of its type. A leaf whose keys are
// all null is an absent leaf, a null required suffix beside a present key of
// its leaf is a missing required suffix (REQ-170), and a key that does not
// resolve is unmatched content whatever its value. REQ-169 applies the same
// reading to an interval bound.

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// withoutNullKeys is body without the keys of extra: the body a decode of
// body with extra's null keys must equal.
func withoutNullKeys(body, extra map[string]any) map[string]any {
	out := maps.Clone(body)
	for k := range extra {
		delete(out, k)
	}
	return out
}

// assertSameDecodeBothForms decodes body and want, as FLAT and as their
// STRUCTURED forms, under strict, and fails unless each form of body decodes
// to what the same form of want decodes to.
func assertSameDecodeBothForms(t *testing.T, wt *webtemplate.WebTemplate, body, want map[string]any) {
	t.Helper()
	got, ref := decodeBothForms(t, wt, body), decodeBothForms(t, wt, want)
	for form, comp := range got {
		if !bytes.Equal(canonical(t, comp), canonical(t, ref[form])) {
			t.Errorf("%s: the body with null keys decodes to\n%s\nwant the body without them\n%s",
				form, canonical(t, comp), canonical(t, ref[form]))
		}
	}
}

// TestREQ053NullRequiredSuffixIsMissing — REQ-053: "a `null` required suffix
// beside a present key of its leaf is a missing required suffix, refused as
// one". `|magnitude: null` beside a unit is a quantity without a magnitude,
// never one of 0; the same holds for every required suffix, for a bound of an
// interval (REQ-169), and for the required keys of a `_` family. Refused in
// every mode, in FLAT and in STRUCTURED, as a missing required suffix
// (ErrUnsupportedDatatype), never as a canonical-JSON error.
func TestREQ053NullRequiredSuffixIsMissing(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct {
		extra   map[string]any
		missing string
	}{
		"quantity magnitude beside a unit": {
			extra:   map[string]any{rmattrElement + "|magnitude": nil},
			missing: "|magnitude",
		},
		"proportion denominator": {
			extra: map[string]any{
				rmattrProportion + "|numerator": 20.5, rmattrProportion + "|denominator": nil, rmattrProportion + "|type": 0,
			},
			missing: "|denominator",
		},
		"ordinal value": {
			extra: map[string]any{
				rmattrOrdinal + "|code": "at0015", rmattrOrdinal + "|value": nil, rmattrOrdinal + "|ordinal": 1,
			},
			missing: "|value",
		},
		"partly null interval bound": {
			extra: map[string]any{
				rmattrElement + "/_normal_range/lower|magnitude": nil,
				rmattrElement + "/_normal_range/lower|unit":      "unit",
			},
			missing: "|magnitude",
		},
		"link type": {
			extra: map[string]any{
				rmattrElement + "/_link:0|meaning": "problem related note",
				rmattrElement + "/_link:0|type":    nil,
				rmattrElement + "/_link:0|target":  "ehr://ehr.network/347a5490-55ee-4da9-b91a-9bba710f730e",
			},
			missing: "|type",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(tc.extra)
			for form, err := range map[string]error{
				"FLAT":       refusedInEveryMode(t, wt, body),
				"STRUCTURED": structuredRefusedInEveryMode(t, wt, body),
			} {
				if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "missing") ||
					!strings.Contains(err.Error(), tc.missing) {
					t.Errorf("%s: decode = %v, want ErrUnsupportedDatatype for the missing required %s", form, err, tc.missing)
				}
				if strings.Contains(err.Error(), "canjson") {
					t.Errorf("%s: decode = %v, a canonical-JSON error rather than the missing suffix", form, err)
				}
			}
		})
	}
}

// TestREQ053AllNullKeysReadAsAbsent — REQ-053: "a leaf whose keys are all
// `null` is an absent leaf", and the null reading holds wherever the key
// resolves: at a bare value, at the suffixes of a leaf, at a `_` family, at a
// key that resolves but has no carrier, at a metadata witness and at a ctx/
// field. Each body decodes, under strict, in FLAT and in STRUCTURED, to what
// the body without its null keys decodes to: nothing is built from a null.
func TestREQ053AllNullKeysReadAsAbsent(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, extra := range map[string]map[string]any{
		"all-null proportion leaf": {
			rmattrProportion + "|numerator": nil, rmattrProportion + "|denominator": nil, rmattrProportion + "|type": nil,
		},
		"all-null ordinal leaf": {
			rmattrOrdinal + "|code": nil, rmattrOrdinal + "|value": nil, rmattrOrdinal + "|ordinal": nil,
		},
		"null bare text":       {rmattrText: nil},
		"null bare boolean":    {rmattrBoolean: nil},
		"null bare count":      {rmattrCount: nil},
		"null optional suffix": {rmattrElement + "|precision": nil, rmattrElement + "|accuracy_is_percent": nil},
		"null _uid":            {rmattrElement + "/_uid": nil},
		"all-null _link": {
			rmattrElement + "/_link:0|meaning": nil, rmattrElement + "/_link:0|type": nil, rmattrElement + "/_link:0|target": nil,
		},
		"null participation time beside its function": {
			rmattrContext + "/_participation:0|function": "requester",
			rmattrContext + "/_participation:0|time":     nil,
		},
		"null terminology witness": {rmattrRoot + "/language|terminology": nil},
		"null ctx/time":            {"ctx/time": nil},
		"null ctx/composer_self":   {"ctx/composer_self": nil},
		"null bare ctx/setting":    {"ctx/setting": nil},
		"null ctx/setting witness": {"ctx/setting|terminology": nil},
		// The real-path alias of ctx/language beside the ctx/ spelling: a null
		// one is no second, disagreeing spelling.
		"null alias beside its ctx/ spelling": {rmattrRoot + "/language|code": nil},
		// The real-path alias of ctx/time, and a suffix of a leaf the ctx/ short
		// form owns that it cannot carry, which is refused for want of a
		// carrier; a null one carries nothing.
		"null alias of ctx/time":      {rmattrRoot + "/context/start_time": nil},
		"null ctx/-owned leaf suffix": {rmattrRoot + "/language|preferred_term": nil},
		// The bare value of a node with children names no value, and a suffix of
		// the composer's party grammar has no carrier; a null one is absent.
		"null bare value at a node with children": {rmattrObs: nil},
		"null composer external_ref":              {rmattrRoot + "/composer|id": nil},
		"null STRING leaf": {
			rmattrSection + "/conformance_instruction/current_activity/action_archetype_id": nil,
		},
		// `x|` spells the bare value of `x`; a null spelling is no disagreement.
		"null x| beside x": {rmattrText: "text", rmattrText + "|": nil},
		"null x beside x|": {rmattrText: nil, rmattrText + "|": "text"},
		// The reference's in-context spelling of RM data the Web Template does
		// not model is refused, rather than dropped; a null one drops nothing.
		"null in-context INTERVAL_EVENT width": {rmattrEvent + "/width": nil},
		// A family this codec defers has no carrier; an all-null one is absent.
		"null deferred family": {rmattrAction + "/_instruction_details|x": nil},
		"null inlined participation identifier": {
			rmattrContext + "/_participation:0|function":         "requester",
			rmattrContext + "/_participation:0|identifiers_id:0": nil,
		},
		"null interval flag": {
			rmattrElement + "/_normal_range/lower|magnitude": 20.5,
			rmattrElement + "/_normal_range/lower|unit":      "unit",
			rmattrElement + "/_normal_range|lower_unbounded": nil,
		},
		"null feeder audit other_details": {
			rmattrRoot + "/_feeder_audit/originating_system_audit|system_id":       "s",
			rmattrRoot + "/_feeder_audit/originating_system_audit/other_details|x": nil,
		},
		// FEEDER_AUDIT_DETAILS' own String and DV_DATE_TIME keys beside its
		// system_id: a null one is absent, not a value of the wrong kind.
		"null feeder audit version_id": {
			rmattrRoot + "/_feeder_audit/originating_system_audit|system_id":  "s",
			rmattrRoot + "/_feeder_audit/originating_system_audit|version_id": nil,
		},
		"null feeder audit time": {
			rmattrRoot + "/_feeder_audit/originating_system_audit|system_id": "s",
			rmattrRoot + "/_feeder_audit/originating_system_audit|time":      nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := rmattrBody(extra)
			want := withoutNullKeys(body, extra)
			for k, v := range extra {
				if v != nil {
					want[k] = v
				}
			}
			assertSameDecodeBothForms(t, wt, body, want)
		})
	}
}

// TestREQ053NullCtxLanguageIsMissingContext — REQ-053: a null ctx/language is
// an absent one, so the mandatory-context refusal names it as missing rather
// than as a value of the wrong kind.
func TestREQ053NullCtxLanguageIsMissingContext(t *testing.T) {
	wt, _ := conformanceWT(t)
	body := rmattrBody(map[string]any{"ctx/language": nil})
	flat := refusedInEveryMode(t, wt, body)
	structured := structuredRefusedInEveryMode(t, wt, body)
	for form, err := range map[string]error{"FLAT": flat, "STRUCTURED": structured} {
		if !errors.Is(err, ErrMissingContext) {
			t.Errorf("%s: decode = %v, want ErrMissingContext", form, err)
		}
	}
}

// TestREQ053NullAtAnUnresolvedKeyIsUnmatched — REQ-053: "A key that does not
// resolve is unmatched content whatever its value." A null must not hide such
// a key from the report option or from strict (REQ-170).
func TestREQ053NullAtAnUnresolvedKeyIsUnmatched(t *testing.T) {
	wt, _ := conformanceWT(t)
	unmatched := []string{
		rmattrObs + "/no_such_node|magnitude", // a key the Web Template does not resolve
		rmattrObs + "/_x_extension",           // an unknown `_`-segment
		rmattrElement + "|x_note",             // an unknown suffix on a modelled leaf
		rmattrElement + "/_link:0|x_note",     // a suffix outside a family's grammar
		"ctx/x_flag",                          // an unknown ctx/ field
	}
	extra := make(map[string]any, len(unmatched))
	for _, k := range unmatched {
		extra[k] = nil
	}
	assertUnmatched(t, wt, rmattrBody(extra), unmatched...)
}

// TestREQ053NullStructuredMemberAndElementAreAbsent — REQ-053: a null held by
// a STRUCTURED member or array element is the absence of what holds it. A
// leaf member holding null, and a leaf array holding a null element, decode
// under strict to what the document without the member decodes to.
func TestREQ053NullStructuredMemberAndElementAreAbsent(t *testing.T) {
	wt, _ := conformanceWT(t)
	refDoc, _ := structuredDoc(t, rmattrBody(nil))
	ref, err := UnmarshalStructured(refDoc, wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalStructured (reference) = %v", err)
	}
	for name, held := range map[string]any{
		"null member":        nil,
		"null array element": []any{nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, doc := structuredDoc(t, rmattrBody(nil))
			event := structuredObservation(doc)["any_event"].([]any)[0].(map[string]any)
			event["dv_boolean"] = held
			got, err := UnmarshalStructured(mustJSON(t, doc), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalStructured = %v, want the null read as absent", err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
				t.Errorf("decoded\n%s\nwant the document without the member\n%s", canonical(t, got), canonical(t, ref))
			}
		})
	}
}

// TestREQ053InterconversionCarriesNull — REQ-053: OPT-free STRUCTURED → FLAT
// conversion is a mechanical restructuring, so a null array element or a null
// member comes across as a FLAT key holding null, which a decode then reads as
// absent, rather than being refused as a hole or dropped.
func TestREQ053InterconversionCarriesNull(t *testing.T) {
	out, err := StructuredToFlat([]byte(`{"t":{"leaf":[null],"other":null}}`))
	if err != nil {
		t.Fatalf("StructuredToFlat = %v, want the nulls carried", err)
	}
	var flat map[string]any
	if err := json.Unmarshal(out, &flat); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"t/leaf:0": nil, "t/other": nil}
	if !maps.Equal(flat, want) {
		t.Errorf("StructuredToFlat = %s, want %s", out, mustJSON(t, want))
	}
}

// TestREQ053FlatToStructuredCarriesNull — REQ-053: OPT-free FLAT → STRUCTURED
// conversion is a mechanical restructuring, so a key holding null comes across
// holding null in every shape: a lone bare value as a null array element, a
// suffix as a null `|suffix` member, and a bare value beside the leaf's
// suffixes or children as the null `"|"` member, as a bare value that is not
// null is. A slot no key fills, before a higher :index, gains no `"|"`.
func TestREQ053FlatToStructuredCarriesNull(t *testing.T) {
	for name, tc := range map[string]struct{ flat, want string }{
		"lone bare value":         {`{"t/x": null}`, `{"t":{"x":[null]}}`},
		"suffix":                  {`{"t/x|unit": null}`, `{"t":{"x":[{"|unit":null}]}}`},
		"bare beside a suffix":    {`{"t/x": null, "t/x|unit": "mg"}`, `{"t":{"x":[{"|":null,"|unit":"mg"}]}}`},
		"bare beside a child":     {`{"t/x": null, "t/x/y": 1}`, `{"t":{"x":[{"|":null,"y":[1]}]}}`},
		"bare beside a family":    {`{"t/x:0": null, "t/x:0/_uid": "u"}`, `{"t":{"x":[{"|":null,"_uid":["u"]}]}}`},
		"two spellings, one null": {`{"t/x": null, "t/x:0": "a"}`, `{"t":{"x":["a"]}}`},
		// A null is absent beside a value whichever of the two spellings sorts
		// first: the sorted key order decides only which one is placed first.
		"two spellings, the null second": {`{"t/x": "a", "t/x:0": null}`, `{"t":{"x":["a"]}}`},
		"two spellings beside a child, the null first": {
			`{"t/x": null, "t/x/y": 1, "t/x:0": "a"}`, `{"t":{"x":[{"|":"a","y":[1]}]}}`,
		},
		"two spellings beside a child, the null second": {
			`{"t/x": "a", "t/x/y": 1, "t/x:0": null}`, `{"t":{"x":[{"|":"a","y":[1]}]}}`,
		},
		"two spellings of a suffix, the null first":  {`{"t/x:0|unit": null, "t/x|unit": "mg"}`, `{"t":{"x":[{"|unit":"mg"}]}}`},
		"two spellings of a suffix, the null second": {`{"t/x:0|unit": "mg", "t/x|unit": null}`, `{"t":{"x":[{"|unit":"mg"}]}}`},
		"two null spellings of a suffix":             {`{"t/x:0|unit": null, "t/x|unit": null}`, `{"t":{"x":[{"|unit":null}]}}`},
		"two spellings of a nested suffix, one null": {
			`{"t/e/_normal_range/lower|magnitude": 20.5, "t/e/_normal_range/lower:0|magnitude": null}`,
			`{"t":{"e":[{"_normal_range":[{"lower":[{"|magnitude":20.5}]}]}]}}`,
		},
		"slot filled after a higher :index": {
			`{"t/x:10|unit": "a", "t/x:2|unit": "b"}`,
			`{"t":{"x":[null,null,{"|unit":"b"},null,null,null,null,null,null,null,{"|unit":"a"}]}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := FlatToStructured([]byte(tc.flat))
			if err != nil {
				t.Fatalf("FlatToStructured(%s) = %v, want the null carried", tc.flat, err)
			}
			var got, want any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("FlatToStructured(%s) = %s, want %s", tc.flat, out, tc.want)
			}
		})
	}
}

// TestREQ053ConversionCarriesNullCtxFieldBesideSuffixes — REQ-053: FLAT decode
// reads a null ctx/ field beside its suffixes as absent, so OPT-free
// conversion carries it as written rather than refusing it as a direct value
// beside a suffixed leaf. FLAT → STRUCTURED gives it the null `"|"` member of
// the field's object, as it gives a null bare value beside a leaf's suffixes;
// STRUCTURED → FLAT gives the key back as written; and each form decodes as
// the body without the null.
func TestREQ053ConversionCarriesNullCtxFieldBesideSuffixes(t *testing.T) {
	flat := `{"ctx/setting":null,"ctx/setting|code":"238"}`
	out, err := FlatToStructured([]byte(flat))
	if err != nil {
		t.Fatalf("FlatToStructured(%s) = %v, want the null carried", flat, err)
	}
	var got, want any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"ctx":{"setting":{"|":null,"|code":"238"}}}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FlatToStructured(%s) = %s, want %s", flat, out, mustJSON(t, want))
	}
	back, err := StructuredToFlat(out)
	if err != nil {
		t.Fatalf("StructuredToFlat(%s) = %v", out, err)
	}
	if string(back) != flat {
		t.Errorf("StructuredToFlat(%s) = %s, want %s", out, back, flat)
	}

	wt, _ := conformanceWT(t)
	extra := map[string]any{"ctx/setting": nil}
	body := rmattrBody(map[string]any{"ctx/setting": nil, "ctx/setting|code": "238", "ctx/setting|value": "other care"})
	assertSameDecodeBothForms(t, wt, body, withoutNullKeys(body, extra))
}

// TestREQ053DvFromSuffixesReadsNullAsAbsent — REQ-053: the DataValue builder
// itself reads a null suffix as absent, whoever calls it, so a null required
// suffix is a missing one and never reaches the canonical tree as the zero of
// its type (`|magnitude: null` beside a unit decoded to 0 before).
func TestREQ053DvFromSuffixesReadsNullAsAbsent(t *testing.T) {
	dv, err := dvFromSuffixes("DV_QUANTITY", false, map[string]any{"magnitude": nil, "unit": "mg"}, nil)
	if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "missing required |magnitude") {
		t.Errorf("dvFromSuffixes(DV_QUANTITY, magnitude null) = %#v, %v; want the missing |magnitude refused", dv, err)
	}
	dv, err = dvFromSuffixes("DV_QUANTITY", false, map[string]any{"magnitude": 1.5, "unit": "mg", "precision": nil}, nil)
	if err != nil {
		t.Fatalf("dvFromSuffixes(DV_QUANTITY, precision null) = %v", err)
	}
	if _, set := dv["precision"]; set {
		t.Errorf("dvFromSuffixes(DV_QUANTITY, precision null) = %#v, want no precision", dv)
	}
}

// TestREQ053BareValueBesideOnlyNullSuffixesIsAlone — REQ-053 and REQ-170: a
// bare value at a datatype with no bare spelling is unmatched beside a suffix
// the datatype reads, and malformed alone. Null suffixes are absent, so a bare
// value beside nothing else is alone, and refused as a value missing every
// required suffix, rather than set aside while the null suffixes vanish.
func TestREQ053BareValueBesideOnlyNullSuffixesIsAlone(t *testing.T) {
	wt, _ := conformanceWT(t)
	body := rmattrBody(map[string]any{
		rmattrProportion: 1.5, rmattrProportion + "|numerator": nil, rmattrProportion + "|denominator": nil,
	})
	for form, err := range map[string]error{
		"FLAT":       refusedInEveryMode(t, wt, body),
		"STRUCTURED": structuredRefusedInEveryMode(t, wt, body),
	} {
		if !errors.Is(err, ErrUnsupportedDatatype) || !strings.Contains(err.Error(), "bare value") {
			t.Errorf("%s: decode = %v, want the lone bare value refused (ErrUnsupportedDatatype)", form, err)
		}
	}
}

// TestREQ053NullSecondSpellingIsAbsent — REQ-053: two spellings of one sub-path
// (the index-less one and `:0`, which OPT-free conversion writes) must agree,
// but a null spelling is absent, so it disagrees with nothing, and the other
// spelling's value is the one read, whichever of the two sorts first. The
// STRUCTURED form, which has one slot for the pair, carries the value alone
// (REQ-140: OPT-free interconversion preserves it) and decodes the same.
func TestREQ053NullSecondSpellingIsAbsent(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, tc := range map[string]struct{ value, null string }{
		"interval bound": {
			value: rmattrElement + "/_normal_range/lower|magnitude",
			null:  rmattrElement + "/_normal_range/lower:0|magnitude",
		},
		"interval bound, the null index-less": {
			value: rmattrElement + "/_normal_range/lower:0|magnitude",
			null:  rmattrElement + "/_normal_range/lower|magnitude",
		},
		"feeder audit details": {
			value: rmattrRoot + "/_feeder_audit/originating_system_audit|system_id",
			null:  rmattrRoot + "/_feeder_audit/originating_system_audit:0|system_id",
		},
		"feeder audit details, the null index-less": {
			value: rmattrRoot + "/_feeder_audit/originating_system_audit:0|system_id",
			null:  rmattrRoot + "/_feeder_audit/originating_system_audit|system_id",
		},
	} {
		t.Run(name, func(t *testing.T) {
			extra := map[string]any{tc.value: "s"}
			if strings.HasSuffix(tc.value, "|magnitude") {
				extra = map[string]any{tc.value: 20.5, rmattrElement + "/_normal_range/lower|unit": "unit"}
			}
			want := rmattrBody(extra)
			body := rmattrBody(extra)
			body[tc.null] = nil
			got, err := UnmarshalFlat(mustJSON(t, body), wt, WithStrict())
			if err != nil {
				t.Fatalf("UnmarshalFlat = %v, want the null spelling read as absent", err)
			}
			if !bytes.Equal(canonical(t, got), canonical(t, decodeRMAttr(t, wt, want))) {
				t.Errorf("decoded\n%s\nwant the body without the null spelling", canonical(t, got))
			}
			assertSameDecodeBothForms(t, wt, body, want)
		})
	}
}

// TestREQ053NullStructuredTopLevelIsAbsent — REQ-053: a null template root or
// ctx object is absent. No FLAT key can hold either, so the decode reads the
// document without it: a null ctx is missing context, and a null root a
// composition with no content.
func TestREQ053NullStructuredTopLevelIsAbsent(t *testing.T) {
	wt, _ := conformanceWT(t)
	_, doc := structuredDoc(t, rmattrBody(nil))
	doc["ctx"] = nil
	if _, err := UnmarshalStructured(mustJSON(t, doc), wt); !errors.Is(err, ErrMissingContext) {
		t.Errorf("UnmarshalStructured(ctx: null) = %v, want ErrMissingContext", err)
	}
	ctxOnly := map[string]any{"ctx/language": "en", "ctx/territory": "US"}
	ref, err := UnmarshalFlat(mustJSON(t, ctxOnly), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat(ctx only) = %v", err)
	}
	got, err := UnmarshalStructured(mustJSON(t, map[string]any{
		"ctx": map[string]any{"language": "en", "territory": "US"}, rmattrRoot: nil,
	}), wt, WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalStructured(root: null) = %v, want the root read as absent", err)
	}
	if !bytes.Equal(canonical(t, got), canonical(t, ref)) {
		t.Errorf("decoded\n%s\nwant the document with no root\n%s", canonical(t, got), canonical(t, ref))
	}
}

// TestREQ053NullMemberUnmatchedIsReportedByItsMemberPath — REQ-170: a null
// member the conversion carries as a FLAT key is reported, when that key is
// unmatched, by its member path, as every other STRUCTURED item is, never by
// the FLAT key the conversion made of it.
func TestREQ053NullMemberUnmatchedIsReportedByItsMemberPath(t *testing.T) {
	wt, _ := conformanceWT(t)
	_, doc := structuredDoc(t, rmattrBody(nil))
	event := structuredObservation(doc)["any_event"].([]any)[0].(map[string]any)
	event["dv_quantity|x_note"] = nil
	var reported []string
	if _, err := UnmarshalStructured(mustJSON(t, doc), wt, WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("UnmarshalStructured = %v", err)
	}
	want := []string{rmattrRoot + "/conformance_section/0/conformance_observation/0/any_event/0/dv_quantity|x_note"}
	if !slices.Equal(reported, want) {
		t.Errorf("WithUnmatched reported %q, want %q", reported, want)
	}
}
