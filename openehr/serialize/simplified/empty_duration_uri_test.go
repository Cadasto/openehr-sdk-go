package simplified_test

// REQ-053 — an empty DV_DURATION, DV_URI or DV_EHR_URI is not set, as an
// empty temporal value is not (empty_temporal_test.go): Go has no other
// spelling of "no value" for the RM-mandatory value string. A Web Template
// leaf writes no key for one that carries nothing else, so a template-free
// decode of a body whose INTERVAL_EVENT has no width re-encodes without the
// `…/width: ""` key the input did not carry. One that carries a decoration
// keeps its bare "" key, since a decoration without a value does not read
// back.

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// coronaWidth is the width leaf of the corona template's INTERVAL_EVENT.
const coronaWidth = "bericht/risikogebiet/reisefall:0/beliebiges_intervallereignis:0/width"

// TestREQ053EmptyDurationAndURIWriteNoKey — REQ-053. A template-free decode
// of corona's body without its interval event's width gives the event an
// empty width, the RM-mandatory DV_DURATION, and re-encodes with no key for
// it; decoding that output gives the same composition. An ELEMENT value
// spelled "" at a DV_DURATION, DV_URI or DV_EHR_URI leaf is not set either,
// and the leaf is absent from the output.
func TestREQ053EmptyDurationAndURIWriteNoKey(t *testing.T) {
	t.Run("interval event with no width", func(t *testing.T) {
		opt, err := readOPTFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
		if err != nil {
			t.Fatalf("readOPTFile(Corona_Anamnese): %v", err)
		}
		wt, _ := compileTemplate(t, "Corona_Anamnese", opt)
		raw, err := os.ReadFile(filepath.Join(fixtures.CrossFormatRoot(), "corona", "flat.json"))
		if err != nil {
			t.Fatalf("read corona flat.json: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("parse corona flat.json: %v", err)
		}
		if _, ok := body[coronaWidth]; !ok {
			t.Fatalf("corona flat.json has no %q — fixture changed?", coronaWidth)
		}
		delete(body, coronaWidth)
		got, first := reencodeFlat(t, wt, body)
		if v, written := got[coronaWidth]; written {
			t.Errorf("MarshalFlat wrote %q = %#v for an empty width; want no key", coronaWidth, v)
		}
		if _, second := reencodeFlat(t, wt, got); string(first) != string(second) {
			t.Errorf("decoding the re-encoded body gives another composition:\n first: %s\nsecond: %s", first, second)
		}
	})
	wt, _ := conformanceTarget(t)
	for _, leaf := range []string{"dv_duration", "dv_uri", "dv_ehr_uri"} {
		key := emptyTemporalEvent + "/" + leaf
		t.Run("empty "+leaf+" leaf", func(t *testing.T) {
			got, _ := reencodeFlat(t, wt, emptyTemporalBody(map[string]any{key: ""}))
			if v, written := got[key]; written {
				t.Errorf("MarshalFlat wrote %q = %#v for an empty value; want no key", key, v)
			}
		})
	}
}

// TestREQ053EmptyDurationDecoratedKeepsItsKey — REQ-053. An empty duration
// that carries a |magnitude_status or a |normal_status keeps its bare ""
// key beside them, because decode refuses a decoration with no value: the
// body reads back as written.
func TestREQ053EmptyDurationDecoratedKeepsItsKey(t *testing.T) {
	wt, _ := conformanceTarget(t)
	leaf := emptyTemporalEvent + "/dv_duration"
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
