package simplified_test

// REQ-053 — an empty DV_DATE_TIME, DV_DATE or DV_TIME is not set: Go has no
// other spelling of "no value" for the RM-mandatory value string. Encode
// writes no key for one that carries nothing else, so a template-free decode
// of a body with no event time re-encodes without the `…/time: ""` key the
// input did not carry. One that carries a decoration keeps its bare "" key,
// since a decoration without a value does not read back. Decode reads a bare
// "" that no other key of its leaf decorates as absent, as it reads a null,
// so decoding, encoding and decoding again keeps the composition.

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The PROBE-086 corpus template's OBSERVATION event and its temporal leaves.
const (
	emptyTemporalObs   = "conformance-ehrbase.de.v0/conformance_section/conformance_observation"
	emptyTemporalEvent = emptyTemporalObs + "/any_event:0"
)

// emptyTemporalBody is a decodable body on the corpus template, with extra
// merged over it; it carries no event time unless extra does.
func emptyTemporalBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":  "en",
		"ctx/territory": "US",
		"ctx/time":      "2021-12-21T14:19:31.649613+01:00",
		emptyTemporalEvent + "/dv_quantity|magnitude": 65.9,
		emptyTemporalEvent + "/dv_quantity|unit":      "unit",
		emptyTemporalObs + "/language|code":           "en",
		emptyTemporalObs + "/language|terminology":    "ISO_639-1",
	}
	maps.Copy(body, extra)
	return body
}

// reencodeFlat decodes body without the template and encodes the result.
func reencodeFlat(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any) (map[string]any, []byte) {
	t.Helper()
	comp, err := simplified.UnmarshalFlat(mustJSON(t, body), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	out, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	return got, canonicalComposition(t, comp)
}

// TestREQ053EmptyTemporalWritesNoKey — REQ-053. A template-free decode of a
// body with no event time, and of one whose temporal leaf is "", re-encodes
// with no key for the empty value. The event's time is RM-mandatory and the
// decode gives it an empty value again, and a lone "" at an ELEMENT's leaf
// reads as absent, so decoding that output gives the same composition as
// decoding the input.
func TestREQ053EmptyTemporalWritesNoKey(t *testing.T) {
	wt, _ := conformanceTarget(t)
	for _, tc := range []struct {
		name  string
		extra map[string]any
		key   string
	}{
		{name: "no event time", key: emptyTemporalEvent + "/time"},
		{name: "empty event time", extra: map[string]any{emptyTemporalEvent + "/time": ""}, key: emptyTemporalEvent + "/time"},
		{name: "empty DV_DATE_TIME leaf", extra: map[string]any{emptyTemporalEvent + "/dv_date_time": ""}, key: emptyTemporalEvent + "/dv_date_time"},
		{name: "empty DV_DATE leaf", extra: map[string]any{emptyTemporalEvent + "/dv_date": ""}, key: emptyTemporalEvent + "/dv_date"},
		{name: "empty DV_TIME leaf", extra: map[string]any{emptyTemporalEvent + "/dv_time": ""}, key: emptyTemporalEvent + "/dv_time"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, first := reencodeFlat(t, wt, emptyTemporalBody(tc.extra))
			if v, written := got[tc.key]; written {
				t.Errorf("MarshalFlat wrote %q = %#v for an empty value; want no key", tc.key, v)
			}
			_, second := reencodeFlat(t, wt, got)
			if string(first) != string(second) {
				t.Errorf("decoding the re-encoded body gives another composition:\n first: %s\nsecond: %s", first, second)
			}
		})
	}
}

// TestREQ053EmptyTemporalDecoratedKeepsItsKey — REQ-053. An empty temporal
// value that carries a |magnitude_status, a |normal_status or an _accuracy
// keeps its bare "" key beside them, because decode refuses a decoration with
// no value: the body reads back as written, the "" not read as absent.
func TestREQ053EmptyTemporalDecoratedKeepsItsKey(t *testing.T) {
	wt, _ := conformanceTarget(t)
	leaf := emptyTemporalEvent + "/dv_date_time"
	for _, decoration := range []map[string]any{
		{leaf + "|magnitude_status": "<="},
		{leaf + "|normal_status": "N"},
		{leaf + "/_accuracy": "PT1S"},
	} {
		extra := map[string]any{leaf: ""}
		maps.Copy(extra, decoration)
		t.Run(slices.Collect(maps.Keys(decoration))[0], func(t *testing.T) {
			got, _ := reencodeFlat(t, wt, emptyTemporalBody(extra))
			for _, key := range slices.Sorted(maps.Keys(extra)) {
				if have, written := got[key]; !written || have != extra[key] {
					t.Errorf("MarshalFlat wrote %q = %#v (written %v), want %#v", key, have, written, extra[key])
				}
			}
		})
	}
}

// TestREQ053EmptyTemporalBoundStaysRefused — REQ-053, REQ-140. An interval
// bound is no Web Template leaf, so an empty temporal bound is not dropped as
// an unset leaf value is: one carrying a |magnitude_status beside its empty
// value is still refused on encode as an empty RM-mandatory value, naming the
// bound's key.
func TestREQ053EmptyTemporalBoundStaysRefused(t *testing.T) {
	wt, _ := conformanceTarget(t)
	leaf := emptyTemporalEvent + "/dv_date_time"
	comp, err := simplified.UnmarshalFlat(mustJSON(t, emptyTemporalBody(map[string]any{
		emptyTemporalEvent + "/time":  "2021-12-21T16:02:58.0094262+01:00",
		leaf:                          "2021-12-21T16:02:58+01:00",
		leaf + "/_normal_range/lower": "",
		leaf + "/_normal_range/lower|magnitude_status": "=",
		leaf + "/_normal_range/upper":                  "2022-12-21T16:02:58+01:00",
	})), wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	_, err = simplified.MarshalFlat(comp, wt)
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Fatalf("MarshalFlat err = %v, want ErrUnsupportedDatatype for the empty bound", err)
	}
	if want := leaf + "/_normal_range/lower"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to name %q", err, want)
	}
}

// decodeLoneValue decodes body, FLAT, with opts, failing the test on an
// error, and renders the composition as canonical JSON.
func decodeLoneValue(t *testing.T, wt *webtemplate.WebTemplate, body []byte, opts ...simplified.Option) (*rm.Composition, []byte) {
	t.Helper()
	comp, err := simplified.UnmarshalFlat(body, wt, opts...)
	if err != nil {
		t.Fatalf("UnmarshalFlat(%s) = %v", body, err)
	}
	return comp, canonicalComposition(t, comp)
}

// assertReadsAsNull decodes body, which spells key "", in FLAT and in
// STRUCTURED with opts, and holds each composition, and the one decoding its
// FLAT encoding gives, to want, the composition the body gives with key null.
func assertReadsAsNull(t *testing.T, wt *webtemplate.WebTemplate, key string, body map[string]any, want []byte, opts ...simplified.Option) *rm.Composition {
	t.Helper()
	flat := mustJSON(t, body)
	comp, got := decodeLoneValue(t, wt, flat, opts...)
	if string(got) != string(want) {
		t.Errorf("UnmarshalFlat with %q: \"\" =\n%s\nwant what %q: null gives:\n%s", key, got, key, want)
	}
	structured, err := simplified.FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured(%s) = %v", flat, err)
	}
	sc, err := simplified.UnmarshalStructured(structured, wt, opts...)
	if err != nil {
		t.Fatalf("UnmarshalStructured(%s) = %v", structured, err)
	}
	if got := canonicalComposition(t, sc); string(got) != string(want) {
		t.Errorf("UnmarshalStructured(%s) =\n%s\nwant what %q: null gives:\n%s", structured, got, key, want)
	}
	out, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	if _, again := decodeLoneValue(t, wt, out, opts...); string(again) != string(want) {
		t.Errorf("decoding the re-encoded body %s gives\n%s\nwant\n%s", out, again, want)
	}
	return comp
}

// TestREQ053EmptyLoneValueReadsAsNull — REQ-053. A "" at the bare value of a
// DV_DATE_TIME, DV_DURATION or DV_URI leaf of test_all_types that no other
// key of the leaf decorates decodes as a null there does, in FLAT and in
// STRUCTURED, with and without the template: no ELEMENT, and with the
// template the composition validates. Encoding it and decoding the output
// gives that composition again.
func TestREQ053EmptyLoneValueReadsAsNull(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	for _, key := range []string{
		shorthandObs + "/datetime",
		shorthandObs + "/duration_any",
		"test_all_types/test_all_types2:0/uri",
	} {
		for _, mode := range shorthandModes(compiled) {
			t.Run(key[strings.LastIndex(key, "/")+1:]+"/"+mode.name, func(t *testing.T) {
				_, want := decodeLoneValue(t, wt, mustJSON(t, completionBody(map[string]any{key: nil})), mode.opts...)
				comp := assertReadsAsNull(t, wt, key, completionBody(map[string]any{key: ""}), want, mode.opts...)
				if mode.opts == nil {
					return
				}
				if res := validation.ValidateComposition(comp, compiled); !res.OK {
					t.Errorf("ValidateComposition(%q: \"\") is not OK: %+v", key, res.Issues)
				}
			})
		}
	}
}

// TestREQ053EmptyValueBesideNullFlavourReadsAsNull — REQ-053. A null flavour
// is the ELEMENT's, not its value's, so it decorates no "" beside it: the ""
// reads as absent, and the ELEMENT carries the null flavour alone, as with a
// null value. Encode writes no key for that "" either, so the round trip
// keeps the ELEMENT as it was.
func TestREQ053EmptyValueBesideNullFlavourReadsAsNull(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	key := shorthandObs + "/datetime"
	flavour := map[string]any{
		key + "/_null_flavour|code":        "253",
		key + "/_null_flavour|value":       "unknown",
		key + "/_null_flavour|terminology": "openehr",
	}
	for _, mode := range shorthandModes(compiled) {
		t.Run(mode.name, func(t *testing.T) {
			null := maps.Clone(flavour)
			null[key] = nil
			_, want := decodeLoneValue(t, wt, mustJSON(t, completionBody(null)), mode.opts...)
			empty := maps.Clone(flavour)
			empty[key] = ""
			assertReadsAsNull(t, wt, key, completionBody(empty), want, mode.opts...)
		})
	}
}

// TestREQ053EmptyStartTimeBesideCtxTime — REQ-053. A "" at
// <root>/context/start_time reads as absent, as a null there does, so beside
// ctx/time it is no second spelling of the time that disagrees with it:
// ctx/time gives the start time.
func TestREQ053EmptyStartTimeBesideCtxTime(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	const key = "test_all_types/context/start_time"
	for _, mode := range shorthandModes(compiled) {
		t.Run(mode.name, func(t *testing.T) {
			_, want := decodeLoneValue(t, wt, mustJSON(t, completionBody(map[string]any{key: nil})), mode.opts...)
			assertReadsAsNull(t, wt, key, completionBody(map[string]any{key: ""}), want, mode.opts...)
		})
	}
}

// TestREQ053EmptyCtxTimeReadsAsNull — REQ-053. ctx/time spells the start
// time, a DV_DATE_TIME, so a "" there reads as absent, as a null there does
// and as a "" at <root>/context/start_time does: the body decodes as one with
// no ctx/time, in FLAT and in STRUCTURED, with and without the template, and
// encoding it and decoding the output gives that composition again.
func TestREQ053EmptyCtxTimeReadsAsNull(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	const key = "ctx/time"
	for _, mode := range shorthandModes(compiled) {
		t.Run(mode.name, func(t *testing.T) {
			_, want := decodeLoneValue(t, wt, mustJSON(t, completionBody(map[string]any{key: nil})), mode.opts...)
			assertReadsAsNull(t, wt, key, completionBody(map[string]any{key: ""}), want, mode.opts...)
		})
	}
}

// TestREQ053EmptyCtxTimeBesideStartTime — REQ-053. A "" at ctx/time is no
// second spelling of the start time that disagrees with <root>/context/start_time
// beside it: the start time is the real path's, whichever of the two keys
// sorts first. test_all_types sorts its root after ctx/, and the corpus
// template sorts its root before it.
func TestREQ053EmptyCtxTimeBesideStartTime(t *testing.T) {
	allTypesWT, allTypesCompiled := shorthandTarget(t)
	corpusWT, corpusCompiled := conformanceTarget(t)
	for _, tc := range []struct {
		name     string
		wt       *webtemplate.WebTemplate
		compiled *templatecompile.Compiled
		body     func(map[string]any) map[string]any
		path     string
		time     string
	}{
		{
			name: "ctx/time sorts first", wt: allTypesWT, compiled: allTypesCompiled, body: completionBody,
			path: "test_all_types/context/start_time", time: "2019-03-22T22:00:00+01:00",
		},
		{
			name: "start_time sorts first", wt: corpusWT, compiled: corpusCompiled, body: emptyTemporalBody,
			path: "conformance-ehrbase.de.v0/context/start_time", time: "2021-12-21T14:19:31.649613+01:00",
		},
	} {
		for _, mode := range shorthandModes(tc.compiled) {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				_, want := decodeLoneValue(t, tc.wt, mustJSON(t, tc.body(map[string]any{"ctx/time": nil, tc.path: tc.time})), mode.opts...)
				comp := assertReadsAsNull(t, tc.wt, "ctx/time", tc.body(map[string]any{"ctx/time": "", tc.path: tc.time}), want, mode.opts...)
				if comp.Context == nil || comp.Context.StartTime.Value != tc.time {
					t.Errorf("UnmarshalFlat(ctx/time: \"\", %s: %q) gives context %+v, want start time %q", tc.path, tc.time, comp.Context, tc.time)
				}
			})
		}
	}
}

// TestREQ053EmptyTemporalDecoratedUnderAnotherSpelling — REQ-053. An
// _accuracy key that spells the event without its :0 decorates the same
// value as the bare "" spelled with it, so the "" stays a value: the body
// decodes as the one spelling both keys alike does.
func TestREQ053EmptyTemporalDecoratedUnderAnotherSpelling(t *testing.T) {
	wt, _ := conformanceTarget(t)
	leaf := emptyTemporalEvent + "/dv_date_time"
	_, want := decodeLoneValue(t, wt, mustJSON(t, emptyTemporalBody(map[string]any{leaf: "", leaf + "/_accuracy": "PT1S"})))
	_, got := decodeLoneValue(t, wt, mustJSON(t, emptyTemporalBody(map[string]any{
		leaf: "",
		emptyTemporalObs + "/any_event/dv_date_time/_accuracy": "PT1S",
	})))
	if string(got) != string(want) {
		t.Errorf("an _accuracy spelled without the event's :0 decodes to\n%s\nwant what the :0 spelling decodes to\n%s", got, want)
	}
}
