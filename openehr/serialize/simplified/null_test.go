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
