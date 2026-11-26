package canjson_test

import (
	"encoding/json"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/expectedform"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// emptyMemberCase is one input for the tests below: a document whose
// mandatory members held by value are empty, partly filled or full.
type emptyMemberCase struct {
	name    string
	in      string
	target  func() any
	absent  []string // members the re-encode leaves out
	written []string // members the re-encode writes
}

// relationship is a PARTY_RELATIONSHIP whose source, a PARTY_REF the SDK
// holds by value, is source; its target is set.
func relationship(source string) string {
	return `{"_type":"PARTY_RELATIONSHIP","archetype_node_id":"openEHR-DEMOGRAPHIC-PARTY_RELATIONSHIP.r.v1",` +
		`"name":{"_type":"DV_TEXT","value":"r"},"source":` + source + `,` +
		`"target":{"_type":"PARTY_REF","namespace":"local","type":"PERSON"}}`
}

// emptyEvent is an OBSERVATION whose one POINT_EVENT has time, a DV_DATE_TIME
// the SDK holds by value, set to time.
func emptyEvent(time string) string {
	return `{"_type":"OBSERVATION","archetype_node_id":"openEHR-EHR-OBSERVATION.o.v1",` +
		`"name":{"_type":"DV_TEXT","value":"o"},"language":` + codePhraseNL + `,"encoding":` + codePhraseNL + `,` +
		`"subject":{"_type":"PARTY_SELF"},"data":{"_type":"HISTORY","archetype_node_id":"at0001",` +
		`"name":{"_type":"DV_TEXT","value":"h"},"origin":{"_type":"DV_DATE_TIME","value":"2026-11-26T10:00:00Z"},` +
		`"events":[{"_type":"POINT_EVENT","archetype_node_id":"at0002","name":{"_type":"DV_TEXT","value":"e"},` +
		`"time":` + time + `,"data":{"_type":"ITEM_TREE","archetype_node_id":"at0003","name":{"_type":"DV_TEXT","value":"t"}}}]}}`
}

// emptyMemberCases are the documents of both tests: an empty value at a
// mandatory member held by value, in each spelling an input can give it, and
// a value with one part set, which is no empty value.
var emptyMemberCases = []emptyMemberCase{
	{
		name:   "an empty PARTY_REF in PARTY_RELATIONSHIP.source",
		in:     relationship(`{"_type":"PARTY_REF","namespace":"","type":""}`),
		target: func() any { return new(rm.PartyRelationship) },
		absent: []string{"source"},
	},
	{
		name:   "a PARTY_REF with no member in PARTY_RELATIONSHIP.source",
		in:     relationship(`{"_type":"PARTY_REF"}`),
		target: func() any { return new(rm.PartyRelationship) },
		absent: []string{"source"},
	},
	{
		name:   "an empty CODE_PHRASE, its TERMINOLOGY_ID empty too, in COMPOSITION.language",
		in:     `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.report.v1","name":{"_type":"DV_TEXT","value":"r"},` + `"language":{"_type":"CODE_PHRASE","code_string":"","terminology_id":{"_type":"TERMINOLOGY_ID","value":""}}}`,
		target: func() any { return new(rm.Composition) },
		absent: []string{"language"},
	},
	{
		name:    "a PARTY_REF with only its namespace set in PARTY_RELATIONSHIP.source",
		in:      relationship(`{"_type":"PARTY_REF","namespace":"local","type":""}`),
		target:  func() any { return new(rm.PartyRelationship) },
		written: []string{"source"},
	},
	{
		// A present list, empty or not, is a field the zero test reads as set.
		name:    "a DV_CODED_TEXT whose only member is an empty mappings list in COMPOSITION.category",
		in:      `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.report.v1","name":{"_type":"DV_TEXT","value":"r"},` + `"category":{"_type":"DV_CODED_TEXT","value":"","mappings":[]}}`,
		target:  func() any { return new(rm.Composition) },
		written: []string{"category"},
	},
	{
		name:    "a HISTORY whose only member is an empty events list in OBSERVATION.data",
		in:      `{"_type":"OBSERVATION","archetype_node_id":"openEHR-EHR-OBSERVATION.o.v1","name":{"_type":"DV_TEXT","value":"o"},` + `"data":{"_type":"HISTORY","archetype_node_id":"","events":[]}}`,
		target:  func() any { return new(rm.Observation) },
		written: []string{"data"},
	},
	{
		name:    "a CODE_PHRASE with only its terminology set in COMPOSITION.language",
		in:      `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.report.v1","name":{"_type":"DV_TEXT","value":"r"},` + `"language":{"_type":"CODE_PHRASE","code_string":"","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}}}`,
		target:  func() any { return new(rm.Composition) },
		written: []string{"language"},
	},
}

// TestREQ052_EmptyMandatoryStructMemberIsLeftOut decodes documents that give
// a mandatory member held by value an empty value, re-encodes them, and
// requires the member to be left out; a value with any part set is written
// in full. Each re-encode must be wire-equivalent to the input's expected
// form, which removes an empty value held by value as the encoder does.
//
// REQ-052 § An absent mandatory member: a mandatory member held by value as
// a struct is left out when every field of it is empty.
func TestREQ052_EmptyMandatoryStructMemberIsLeftOut(t *testing.T) {
	for _, tc := range emptyMemberCases {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.target()
			if err := canjson.Unmarshal([]byte(tc.in), v); err != nil {
				t.Fatalf("canjson.Unmarshal(%s, %T): %v", tc.in, v, err)
			}
			got, err := canjson.Marshal(v)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T): %v", v, err)
			}
			members := memberValues(t, got)
			for _, name := range tc.absent {
				if raw, ok := members[name]; ok {
					t.Errorf("canjson.Marshal(decode of %s) = %s\nwrites %q as %s, want it left out", tc.in, got, name, raw)
				}
			}
			for _, name := range tc.written {
				if _, ok := members[name]; !ok {
					t.Errorf("canjson.Marshal(decode of %s) = %s\nleaves out %q, want it written in full", tc.in, got, name)
				}
			}
			want, err := expectedform.BuildFor([]byte(tc.in), v)
			if err != nil {
				t.Fatalf("expectedform.BuildFor(%s, %T): %v", tc.in, v, err)
			}
			if ok, diff := wireequiv.Equivalent(want, got); !ok {
				t.Errorf("canjson.Marshal(decode of %s) = %s\nis not wire-equivalent to the expected form %s (expected form versus output): %s", tc.in, got, want, diff)
			}
		})
	}
}

// TestREQ052_EmptyEventTimeIsLeftOutInsideAComposition holds the rule where
// it meets real documents: an event's time held by value, nested in an
// OBSERVATION's history, given as an empty value or as a value whose only
// member is null, is left out, and a time with a value is written.
func TestREQ052_EmptyEventTimeIsLeftOutInsideAComposition(t *testing.T) {
	for _, tc := range []struct {
		name  string
		time  string
		wrote bool
	}{
		{name: "an empty value", time: `{"_type":"DV_DATE_TIME","value":""}`},
		{name: "a null value", time: `{"value":null}`},
		{name: "a value", time: `{"_type":"DV_DATE_TIME","value":"2026-11-26T10:00:00Z"}`, wrote: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := emptyEvent(tc.time)
			var obs rm.Observation
			if err := canjson.Unmarshal([]byte(in), &obs); err != nil {
				t.Fatalf("canjson.Unmarshal(%s): %v", in, err)
			}
			got, err := canjson.Marshal(&obs)
			if err != nil {
				t.Fatalf("canjson.Marshal: %v", err)
			}
			event := memberValues(t, got)["data"]
			event = memberValues(t, event)["events"]
			var events []map[string]any
			if err := json.Unmarshal(event, &events); err != nil || len(events) != 1 {
				t.Fatalf("events = %s (%v), want one event", event, err)
			}
			if _, ok := events[0]["time"]; ok != tc.wrote {
				t.Errorf("canjson.Marshal(decode of an event whose time is %s) writes time = %v, want %v", tc.time, ok, tc.wrote)
			}
			want, err := expectedform.BuildFor([]byte(in), &obs)
			if err != nil {
				t.Fatalf("expectedform.BuildFor: %v", err)
			}
			if ok, diff := wireequiv.Equivalent(want, got); !ok {
				t.Errorf("re-encode is not wire-equivalent to the expected form (expected form versus output): %s", diff)
			}
		})
	}
}
