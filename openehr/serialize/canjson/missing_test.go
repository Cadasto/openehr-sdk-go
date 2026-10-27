package canjson_test

// missing_test.go: REQ-175 (wire.md § REQ-175, Missing members on decode)
// for canonical JSON. A member is missing when the RM makes the attribute
// mandatory on the RM type its object decodes as and the object has no
// member of that name, or holds null there. The default decodes such a
// document as one without; WithMissing reports every missing member, once,
// by its JSON Pointer, as its object closes; strict never refuses one. The
// members are checked once the decode has succeeded, in the pass that serves
// WithUnmatched, so a decode that fails reports nothing and a value of a
// class outside the RM is not checked.

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// rewrite returns doc with the member at each pointer of edits replaced by
// the raw JSON text it maps to, or dropped when that text is empty. Member
// order is kept, so the objects close in the order doc writes them. An edit
// whose pointer names no member of doc fails the test, since it would
// silently test nothing.
func rewrite(t *testing.T, doc string, edits map[string]string) []byte {
	t.Helper()
	dec := jsontext.NewDecoder(strings.NewReader(doc))
	var out bytes.Buffer
	enc := jsontext.NewEncoder(&out)
	applied := map[string]bool{}
	for {
		tok, err := dec.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("rewrite: read %s: %v", doc, err)
		}
		kind, n := dec.StackIndex(dec.StackDepth())
		if kind == '{' && n%2 == 1 { // tok is a member's name
			ptr := string(dec.StackPointer())
			if repl, ok := edits[ptr]; ok {
				applied[ptr] = true
				name := tok.String() // tok is void once the value is skipped
				if err := dec.SkipValue(); err != nil {
					t.Fatalf("rewrite: skip the value of %s: %v", ptr, err)
				}
				if repl == "" {
					continue
				}
				if err := enc.WriteToken(jsontext.String(name)); err != nil {
					t.Fatalf("rewrite: write %s: %v", name, err)
				}
				if err := enc.WriteValue(jsontext.Value(repl)); err != nil {
					t.Fatalf("rewrite: write %s: %v", repl, err)
				}
				continue
			}
		}
		if err := enc.WriteToken(tok); err != nil {
			t.Fatalf("rewrite: write %s: %v", tok, err)
		}
	}
	for p := range edits {
		if !applied[p] {
			t.Fatalf("rewrite: %s names no member of the document; the edit would test nothing", p)
		}
	}
	return out.Bytes()
}

// decodeMissing decodes in into a fresh value from into with WithMissing and
// the extra options, and returns the value, the pointers reported and the
// error.
func decodeMissing(into func() any, in []byte, opts ...canjson.DecoderOption) (any, []string, error) {
	var got []string
	v := into()
	opts = append(opts, canjson.WithMissing(func(p string) { got = append(got, p) }))
	err := canjson.Unmarshal(in, v, opts...)
	return v, got, err
}

// assertReportedAsDefault decodes in three ways and checks every facet
// REQ-175 gives the report option: the default decodes in with no error;
// WithMissing reports exactly want, once each, and returns the default's
// value; WithStrict refuses nothing, with or without WithMissing beside it,
// and the report under strict is the same.
func assertReportedAsDefault(t *testing.T, into func() any, in []byte, want []string) {
	t.Helper()
	def := into()
	if err := canjson.Unmarshal(in, def); err != nil {
		t.Fatalf("canjson.Unmarshal(%s) = %v; want nil: a missing member is no decode error", in, err)
	}
	got, pointers, err := decodeMissing(into, in)
	if err != nil {
		t.Fatalf("canjson.Unmarshal(%s, WithMissing) = %v; want nil: the option reports, never refuses", in, err)
	}
	if !slices.Equal(pointers, want) {
		t.Errorf("WithMissing reported %q; want %q", pointers, want)
	}
	if !reflect.DeepEqual(got, def) {
		t.Errorf("canjson.Unmarshal(WithMissing) decoded\n%#v\nwant the default's value\n%#v", got, def)
	}
	if err := canjson.Unmarshal(in, into(), canjson.WithStrict()); err != nil {
		t.Errorf("canjson.Unmarshal(%s, WithStrict) = %v; want nil: strict refuses unmatched members only", in, err)
	}
	_, strictPointers, err := decodeMissing(into, in, canjson.WithStrict())
	if err != nil {
		t.Errorf("canjson.Unmarshal(%s, WithStrict, WithMissing) = %v; want nil", in, err)
	}
	if !slices.Equal(strictPointers, want) {
		t.Errorf("WithMissing under WithStrict reported %q; want %q", strictPointers, want)
	}
}

// Shared fragments of the composition fixture.
const (
	mcLanguage = `{"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "ISO_639-1"}, "code_string": "en"}`
	mcTime     = `{"_type": "DV_DATE_TIME", "value": "2026-01-01T00:00:00Z"}`
)

// missingComposition is a complete COMPOSITION: every object carries every
// member the RM requires of it, so WithMissing reports nothing for it, and
// the rows of missingRows take members out of it. It also carries what the
// control rows of REQ-175's acceptance need present: a bare PARTY_SELF as
// subject and composer, an empty CLUSTER items list, a DV_INTERVAL with no
// boundary flags and a POINT_EVENT with no offset.
var missingComposition = `{
  "_type": "COMPOSITION",
  "name": {"_type": "DV_TEXT", "value": "encounter"},
  "archetype_node_id": "openEHR-EHR-COMPOSITION.encounter.v1",
  "archetype_details": {"_type": "ARCHETYPED", "archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-COMPOSITION.encounter.v1"}, "rm_version": "1.1.0"},
  "language": ` + mcLanguage + `,
  "territory": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "ISO_3166-1"}, "code_string": "GB"},
  "category": {"_type": "DV_CODED_TEXT", "value": "event", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "openehr"}, "code_string": "433"}},
  "composer": {"_type": "PARTY_SELF"},
  "content": [{
    "_type": "OBSERVATION",
    "name": {"_type": "DV_TEXT", "value": "vitals"},
    "archetype_node_id": "openEHR-EHR-OBSERVATION.vitals.v1",
    "language": ` + mcLanguage + `,
    "encoding": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "IANA_character-sets"}, "code_string": "UTF-8"},
    "subject": {"_type": "PARTY_SELF"},
    "data": {
      "_type": "HISTORY",
      "name": {"_type": "DV_TEXT", "value": "history"},
      "archetype_node_id": "at0001",
      "origin": ` + mcTime + `,
      "events": [{
        "_type": "POINT_EVENT",
        "name": {"_type": "DV_TEXT", "value": "event"},
        "archetype_node_id": "at0002",
        "time": ` + mcTime + `,
        "data": {
          "_type": "ITEM_TREE",
          "name": {"_type": "DV_TEXT", "value": "tree"},
          "archetype_node_id": "at0003",
          "items": [
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "weight"}, "archetype_node_id": "at0004", "value": {"_type": "DV_QUANTITY", "magnitude": 80.5, "units": "kg"}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "count"}, "archetype_node_id": "at0005", "value": {"_type": "DV_COUNT", "magnitude": 2}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "flag"}, "archetype_node_id": "at0006", "value": {"_type": "DV_BOOLEAN", "value": true}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "ratio"}, "archetype_node_id": "at0007", "value": {"_type": "DV_PROPORTION", "numerator": 1, "denominator": 2, "type": 0}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "score"}, "archetype_node_id": "at0008", "value": {"_type": "DV_ORDINAL", "value": 1, "symbol": {"_type": "DV_CODED_TEXT", "value": "mild", "defining_code": {"_type": "CODE_PHRASE", "terminology_id": {"_type": "TERMINOLOGY_ID", "value": "local"}, "code_string": "at0009"}}}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "note"}, "archetype_node_id": "openEHR-EHR-ELEMENT.note.v1", "archetype_details": {"_type": "ARCHETYPED", "archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-ELEMENT.note.v1"}, "rm_version": "1.1.0"}, "value": {"_type": "DV_TEXT", "value": "n"}},
            {"_type": "ELEMENT", "name": {"_type": "DV_TEXT", "value": "range"}, "archetype_node_id": "at0010", "value": {"_type": "DV_INTERVAL", "lower": {"_type": "DV_QUANTITY", "magnitude": 1, "units": "kg"}, "upper": {"_type": "DV_QUANTITY", "magnitude": 2, "units": "kg"}}},
            {"_type": "CLUSTER", "name": {"_type": "DV_TEXT", "value": "empty"}, "archetype_node_id": "at0011", "items": []}
          ]
        }
      }]
    }
  }]
}`

// The pointers of the fixture's objects the rows edit.
const (
	mcEntry   = "/content/0"
	mcHistory = mcEntry + "/data"
	mcItems   = mcHistory + "/events/0/data/items/"
)

// missingRow is one edit of missingComposition and the members WithMissing
// must then report. edits maps a member's pointer to its new raw JSON text,
// or to "" to leave the member out.
type missingRow struct {
	name  string
	edits map[string]string
	want  []string
}

// missingRows puts a missing member everywhere REQ-175's acceptance reaches
// in canonical JSON, one row each (PROBE-110's table): the root, an ENTRY, a
// HISTORY, and a data value of each value-typed kind; a null row; then the
// control rows, whose member is present and not reported; and one row with
// every member left out at once, which pins the order: each object's
// members as the object closes, so the data values' members come before
// their HISTORY's, which comes before the ENTRY's and the root's.
func missingRows() []missingRow {
	omitted := map[string]string{
		"/language":                                "",
		mcEntry + "/subject":                       "",
		mcHistory + "/origin":                      "",
		mcItems + "0/value/magnitude":              "",
		mcItems + "1/value/magnitude":              "",
		mcItems + "2/value/value":                  "",
		mcItems + "3/value/denominator":            "",
		mcItems + "4/value/symbol":                 "",
		mcItems + "5/archetype_details/rm_version": "",
	}
	rows := []missingRow{
		{name: "every member present", want: nil},
		{name: "root language", edits: map[string]string{"/language": ""}, want: []string{"/language"}},
		{name: "ENTRY subject", edits: map[string]string{mcEntry + "/subject": ""}, want: []string{mcEntry + "/subject"}},
		{name: "HISTORY origin", edits: map[string]string{mcHistory + "/origin": ""}, want: []string{mcHistory + "/origin"}},
		{name: "DV_QUANTITY magnitude", edits: map[string]string{mcItems + "0/value/magnitude": ""}, want: []string{mcItems + "0/value/magnitude"}},
		{name: "DV_COUNT magnitude", edits: map[string]string{mcItems + "1/value/magnitude": ""}, want: []string{mcItems + "1/value/magnitude"}},
		{name: "DV_BOOLEAN value", edits: map[string]string{mcItems + "2/value/value": ""}, want: []string{mcItems + "2/value/value"}},
		{name: "DV_PROPORTION denominator", edits: map[string]string{mcItems + "3/value/denominator": ""}, want: []string{mcItems + "3/value/denominator"}},
		{name: "DV_ORDINAL symbol", edits: map[string]string{mcItems + "4/value/symbol": ""}, want: []string{mcItems + "4/value/symbol"}},
		{name: "ARCHETYPED rm_version", edits: map[string]string{mcItems + "5/archetype_details/rm_version": ""}, want: []string{mcItems + "5/archetype_details/rm_version"}},
		{
			name: "null members",
			edits: map[string]string{
				mcEntry + "/subject":          "null",
				mcItems + "0/value/magnitude": "null",
				mcItems + "2/value/value":     "null",
			},
			want: []string{mcItems + "0/value/magnitude", mcItems + "2/value/value", mcEntry + "/subject"},
		},
		{name: "control: magnitude 0", edits: map[string]string{mcItems + "0/value/magnitude": "0"}, want: nil},
		{name: "control: empty units", edits: map[string]string{mcItems + "0/value/units": `""`}, want: nil},
		{name: "control: false value", edits: map[string]string{mcItems + "2/value/value": "false"}, want: nil},
		{name: "control: empty name", edits: map[string]string{mcItems + "0/name/value": `""`}, want: nil},
		{name: "control: empty archetype_node_id", edits: map[string]string{mcItems + "0/archetype_node_id": `""`}, want: nil},
	}
	all := missingRow{name: "every member at once", edits: omitted, want: []string{
		mcItems + "0/value/magnitude",
		mcItems + "1/value/magnitude",
		mcItems + "2/value/value",
		mcItems + "3/value/denominator",
		mcItems + "4/value/symbol",
		mcItems + "5/archetype_details/rm_version",
		mcHistory + "/origin",
		mcEntry + "/subject",
		"/language",
	}}
	return append(rows, all)
}

// TestREQ175_MissingMembersReportedAsDefault drives each row through
// assertReportedAsDefault on canjson.Unmarshal into an rm.Composition.
func TestREQ175_MissingMembersReportedAsDefault(t *testing.T) {
	into := func() any { return new(rm.Composition) }
	for _, row := range missingRows() {
		t.Run(row.name, func(t *testing.T) {
			in := rewrite(t, missingComposition, row.edits)
			assertReportedAsDefault(t, into, in, row.want)
		})
	}
}

// TestREQ175_MissingMembersAtOtherRoots pins the report at roots other than
// a COMPOSITION: an EHR_STATUS without its subject (PROBE-081's root), a
// RESOURCE_DESCRIPTION whose Hash entry lacks a member, named by the entry's
// key, and one without parent_resource, which no serialization carries; a
// slice target, each element by its position; and a value with every
// member missing, in the RM's order.
func TestREQ175_MissingMembersAtOtherRoots(t *testing.T) {
	const descriptionItem = `{"language": ` + mcLanguage + `, "purpose": "p"}`
	cases := []struct {
		name, in string
		into     func() any
		want     []string
	}{
		{
			name: "EHR_STATUS without subject",
			in:   `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"s"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","is_queryable":true,"is_modifiable":true}`,
			into: func() any { return new(rm.EHRStatus) },
			want: []string{"/subject"},
		},
		{
			name: "EHR_STATUS with a bare PARTY_SELF subject",
			in:   `{"_type":"EHR_STATUS","name":{"_type":"DV_TEXT","value":"s"},"archetype_node_id":"openEHR-EHR-EHR_STATUS.generic.v1","subject":{"_type":"PARTY_SELF"},"is_queryable":true,"is_modifiable":true}`,
			into: func() any { return new(rm.EHRStatus) },
		},
		{
			name: "RESOURCE_DESCRIPTION without parent_resource",
			in:   `{"_type":"RESOURCE_DESCRIPTION","original_author":{"name":"a"},"lifecycle_state":"published","details":{"en":` + descriptionItem + `}}`,
			into: func() any { return new(rm.ResourceDescription) },
		},
		{
			name: "a Hash entry by its key",
			in:   `{"_type":"RESOURCE_DESCRIPTION","original_author":{"name":"a"},"lifecycle_state":"published","details":{"en":{"language":` + mcLanguage + `},"de/x":` + descriptionItem + `}}`,
			into: func() any { return new(rm.ResourceDescription) },
			want: []string{"/details/en/purpose"},
		},
		{
			name: "a slice target",
			in:   `[{"units":"kg"},{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},{"_type":"DV_QUANTITY","magnitude":2}]`,
			into: func() any { return new([]rm.DVQuantity) },
			want: []string{"/0/magnitude", "/2/units"},
		},
		{
			name: "every member missing, in the RM's order",
			in:   `{"_type":"DV_ORDINAL"}`,
			into: func() any { return new(rm.DVOrdinal) },
			want: []string{"/symbol", "/value"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReportedAsDefault(t, tc.into, []byte(tc.in), tc.want)
		})
	}
}

// TestREQ175_FailedDecodeReportsNothing pins that the members are checked
// once the decode has succeeded: a document that fails to decode, here for a
// `_type` naming no RM class, reports no missing member, though it leaves
// several out, and the failure comes back as itself.
func TestREQ175_FailedDecodeReportsNothing(t *testing.T) {
	in := rewrite(t, missingComposition, map[string]string{
		"/language":                   "",
		mcItems + "0/value/magnitude": "",
		mcItems + "1/value":           `{"_type":"NO_SUCH_CLASS"}`,
	})
	_, pointers, err := decodeMissing(func() any { return new(rm.Composition) }, in)
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Fatalf("canjson.Unmarshal(WithMissing) err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}
	if len(pointers) != 0 {
		t.Errorf("WithMissing reported %q; want nothing: a failed decode is not checked", pointers)
	}
}

// TestREQ175_ClassesOutsideTheRMAreNotChecked pins the scope: a value of a
// class outside the RM (an AOM 1.4 archetype) and an object inside a generic
// payload the decoder holds as plain JSON (the data of an
// rm.OriginalVersion[any]) are not checked, though they leave members out,
// and nothing inside them is reported. The ORIGINAL_VERSION's own members
// are, which shows the walk ran up to the payload.
func TestREQ175_ClassesOutsideTheRMAreNotChecked(t *testing.T) {
	cases := []struct {
		name, in string
		into     func() any
		want     []string
	}{
		{
			name: "AOM 1.4 archetype",
			in:   `{"_type":"ARCHETYPE","archetype_id":{"_type":"ARCHETYPE_ID"},"definition":{"_type":"C_COMPLEX_OBJECT","rm_type_name":"OBSERVATION"}}`,
			into: func() any { return new(aom14.Archetype) },
		},
		{
			name: "an object inside a generic payload",
			in:   `{"_type":"ORIGINAL_VERSION","data":{"_type":"DV_QUANTITY","units":"kg"}}`,
			into: func() any { return new(rm.OriginalVersion[any]) },
			want: []string{"/contribution", "/commit_audit", "/uid", "/lifecycle_state"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReportedAsDefault(t, tc.into, []byte(tc.in), tc.want)
		})
	}
}

// TestREQ175_WithUnmatchedAndWithMissingShareOnePass pins that the two
// report options are independent, may be combined, and are served by one
// pass: unmatched members come in document order as they are met and
// missing members as their object closes, so the two reports interleave by
// position in the document, where two passes would give one report whole
// before the other. Neither option reports the other's members.
func TestREQ175_WithUnmatchedAndWithMissingShareOnePass(t *testing.T) {
	const in = `{"_type":"DV_CODED_TEXT","x-a":"` + plantedValue + `","defining_code":{"_type":"CODE_PHRASE","x-b":1,"code_string":"c"},"x-c":1}`
	var log []string
	err := canjson.Unmarshal([]byte(in), new(rm.DVCodedText),
		canjson.WithUnmatched(func(p string) { log = append(log, "unmatched "+p) }),
		canjson.WithMissing(func(p string) { log = append(log, "missing "+p) }),
	)
	if err != nil {
		t.Fatalf("canjson.Unmarshal(WithUnmatched, WithMissing) = %v; want nil", err)
	}
	want := []string{
		"unmatched /x-a",
		"unmatched /defining_code/x-b",
		"missing /defining_code/terminology_id",
		"unmatched /x-c",
		"missing /value",
	}
	if !slices.Equal(log, want) {
		t.Errorf("the options reported\n%q\nwant, in one pass,\n%q", log, want)
	}
	for _, p := range log {
		if strings.Contains(p, plantedValue) {
			t.Errorf("reported %q, which carries a value from the input", p)
		}
	}
}

// TestREQ175_NilMissingIsNoOption pins that WithMissing(nil) is the same as
// leaving the option out: the decode is the default's, and a Decoder given
// it still decodes as it streams, where one given a function reads each
// value whole first (see TestREQ170_DecoderWithOptionsReadsTheValueWhole).
func TestREQ175_NilMissingIsNoOption(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","units":"kg"}`)
	var want, got rm.DVQuantity
	if err := canjson.Unmarshal(in, &want); err != nil {
		t.Fatalf("canjson.Unmarshal = %v; want nil", err)
	}
	if err := canjson.Unmarshal(in, &got, canjson.WithMissing(nil)); err != nil {
		t.Fatalf("canjson.Unmarshal(WithMissing(nil)) = %v; want nil", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canjson.Unmarshal(WithMissing(nil)) decoded %+v; want the default's %+v", got, want)
	}

	const shapeFirst = `{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg" x}`
	err := canjson.NewDecoder(strings.NewReader(shapeFirst), canjson.WithMissing(nil)).Decode(new(rm.DVQuantity))
	if !errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("Decoder(WithMissing(nil)).Decode(%s) err = %v; want the shape failure first, as with no options", shapeFirst, err)
	}
	err = canjson.NewDecoder(strings.NewReader(shapeFirst), canjson.WithMissing(func(string) {})).Decode(new(rm.DVQuantity))
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); !ok || se == nil || errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("Decoder(WithMissing).Decode(%s) err = %v; want the malformed bytes reported, with no canjson.ErrInvalidShape: the value is read whole first", shapeFirst, err)
	}
}

// TestREQ175_DecoderReportsPerValue drives the streaming entry point: a
// Decoder takes the option, each value's pointers are relative to that
// value, and strict refuses nothing.
func TestREQ175_DecoderReportsPerValue(t *testing.T) {
	stream := `{"_type":"DV_TEXT"} {"_type":"DV_QUANTITY","units":"kg","magnitude":null}`
	var got []string
	dec := canjson.NewDecoder(strings.NewReader(stream), canjson.WithMissing(func(p string) { got = append(got, p) }), canjson.WithStrict())
	var text rm.DVText
	if err := dec.Decode(&text); err != nil {
		t.Fatalf("Decode(first value) = %v; want nil", err)
	}
	if want := []string{"/value"}; !slices.Equal(got, want) {
		t.Errorf("Decoder reported %q after the first value; want %q", got, want)
	}
	got = nil
	var qty rm.DVQuantity
	if err := dec.Decode(&qty); err != nil {
		t.Fatalf("Decode(second value) = %v; want nil", err)
	}
	if want := []string{"/magnitude"}; !slices.Equal(got, want) {
		t.Errorf("Decoder reported %q after the second value; want %q, relative to that value", got, want)
	}
	if qty.Units != "kg" || qty.Magnitude != 0 {
		t.Errorf("Decoder decoded %+v; want the members it reads filled and the missing one zero", qty)
	}
	if err := dec.Decode(new(rm.DVText)); !errors.Is(err, io.EOF) {
		t.Errorf("Decode(past the end) = %v; want io.EOF", err)
	}
}

// TestREQ175_ConcurrentDecodesKeepTheirOwnReports runs reporting decodes in
// parallel, each with its own function and its own missing member, and
// requires each to see exactly its own.
func TestREQ175_ConcurrentDecodesKeepTheirOwnReports(t *testing.T) {
	inputs := map[string]string{
		"/magnitude": `{"_type":"DV_QUANTITY","units":"kg"}`,
		"/units":     `{"_type":"DV_QUANTITY","magnitude":1}`,
	}
	for want, in := range maps.All(inputs) {
		for i := range 4 {
			t.Run(want+"/"+string(rune('a'+i)), func(t *testing.T) {
				t.Parallel()
				for range 200 {
					var got []string
					err := canjson.Unmarshal([]byte(in), new(rm.DVQuantity), canjson.WithMissing(func(p string) { got = append(got, p) }))
					if err != nil {
						t.Fatalf("canjson.Unmarshal(WithMissing) = %v; want nil", err)
					}
					if !slices.Equal(got, []string{want}) {
						t.Fatalf("WithMissing reported %q; want %q: a concurrent decode's members must not reach this one", got, want)
					}
				}
			})
		}
	}
}
