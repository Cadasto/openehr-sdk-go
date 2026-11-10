package simplified_test

// REQ-053 — an empty DV_DATE_TIME, DV_DATE or DV_TIME is not set: Go has no
// other spelling of "no value" for the RM-mandatory value string. Encode
// writes no key for one that carries nothing else, so a template-free decode
// of a body with no event time re-encodes without the `…/time: ""` key the
// input did not carry. One that carries a decoration keeps its bare "" key,
// since a decoration without a value does not read back.

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
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
// decode gives it an empty value again, so decoding that output gives the
// same composition as decoding the input; an empty leaf value is not set, and
// the leaf is absent from that output.
func TestREQ053EmptyTemporalWritesNoKey(t *testing.T) {
	wt, _ := conformanceTarget(t)
	for _, tc := range []struct {
		name       string
		extra      map[string]any
		key        string
		sameDecode bool
	}{
		{name: "no event time", key: emptyTemporalEvent + "/time", sameDecode: true},
		{name: "empty event time", extra: map[string]any{emptyTemporalEvent + "/time": ""}, key: emptyTemporalEvent + "/time", sameDecode: true},
		{name: "empty DV_DATE_TIME leaf", extra: map[string]any{emptyTemporalEvent + "/dv_date_time": ""}, key: emptyTemporalEvent + "/dv_date_time"},
		{name: "empty DV_DATE leaf", extra: map[string]any{emptyTemporalEvent + "/dv_date": ""}, key: emptyTemporalEvent + "/dv_date"},
		{name: "empty DV_TIME leaf", extra: map[string]any{emptyTemporalEvent + "/dv_time": ""}, key: emptyTemporalEvent + "/dv_time"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, first := reencodeFlat(t, wt, emptyTemporalBody(tc.extra))
			if v, written := got[tc.key]; written {
				t.Errorf("MarshalFlat wrote %q = %#v for an empty value; want no key", tc.key, v)
			}
			if !tc.sameDecode {
				return
			}
			_, second := reencodeFlat(t, wt, got)
			if string(first) != string(second) {
				t.Errorf("decoding the re-encoded body gives another composition:\n first: %s\nsecond: %s", first, second)
			}
		})
	}
}

// TestREQ053EmptyTemporalDecoratedKeepsItsKey — REQ-053. An empty temporal
// value that carries a |magnitude_status or a |normal_status keeps its bare
// "" key beside them, because decode refuses a decoration with no value: the
// body reads back as written.
func TestREQ053EmptyTemporalDecoratedKeepsItsKey(t *testing.T) {
	wt, _ := conformanceTarget(t)
	leaf := emptyTemporalEvent + "/dv_date_time"
	for _, decoration := range []map[string]any{
		{leaf + "|magnitude_status": "<="},
		{leaf + "|normal_status": "N"},
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
