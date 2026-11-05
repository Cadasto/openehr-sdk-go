package canxml_test

// missing_test.go: REQ-175 (wire.md § REQ-175, Missing members on decode)
// for canonical XML, on the REQ-056 codec; PROBE-110's rows. A member is
// missing when the RM makes the attribute mandatory on the class its element
// decodes as and the element carries no value for it: no child element of
// that name and no XML attribute of it where the schema carries one, a child
// with xsi:nil set, or a primitive child whose text is empty or whitespace.
// The default decodes such a document as one without; WithMissing reports
// every missing member, once, by its JSON Pointer in the canonical-JSON form
// of the instance, as its element closes; strict never refuses one. The
// composition fixture mirrors canjson's missing_test.go member for member,
// and the rows assert the pointer list that test asserts, so the two codecs
// are held to one answer.

import (
	"encoding/xml"
	"errors"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xmlLists names the multi-valued members the fixture carries, whose child
// elements a pointer counts from 0 as the canonical-JSON form does.
var xmlLists = map[string]bool{"content": true, "events": true, "items": true}

// rewriteXML returns doc with the element at each pointer of edits replaced
// by the XML text it maps to, or dropped when that text is empty. A pointer
// is the element's in the canonical-JSON form of the instance, as WithMissing
// names a member: the root is "", and each element below it adds its local
// name and, under a member of xmlLists, its position among its same-named
// siblings. The rest of doc is kept byte for byte, so the elements close in
// the order doc writes them. An edit whose pointer names no element of doc
// fails the test, since it would silently test nothing.
func rewriteXML(t *testing.T, doc string, edits map[string]string) []byte {
	t.Helper()
	type span struct {
		from, to int
		repl     string
	}
	type frame struct {
		pointer string
		counts  map[string]int
	}
	var spans []span
	stack := []frame{{counts: map[string]int{}}} // the document, whose one child is the root
	applied := map[string]bool{}
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		from := int(dec.InputOffset())
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("rewriteXML: read %s: %v", doc, err)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			parent := &stack[len(stack)-1]
			ptr := parent.pointer + "/" + tk.Name.Local
			if xmlLists[tk.Name.Local] {
				ptr += "/" + strconv.Itoa(parent.counts[tk.Name.Local])
				parent.counts[tk.Name.Local]++
			}
			if len(stack) == 1 {
				ptr = "" // the root element is the value itself
			}
			if repl, ok := edits[ptr]; ok {
				applied[ptr] = true
				if err := dec.Skip(); err != nil {
					t.Fatalf("rewriteXML: skip %s: %v", ptr, err)
				}
				spans = append(spans, span{from: from, to: int(dec.InputOffset()), repl: repl})
				continue
			}
			stack = append(stack, frame{pointer: ptr, counts: map[string]int{}})
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	for p := range edits {
		if !applied[p] {
			t.Fatalf("rewriteXML: %s names no element of the document; the edit would test nothing", p)
		}
	}
	out := []byte(doc)
	for _, s := range slices.Backward(spans) {
		out = slices.Concat(out[:s.from], []byte(s.repl), out[s.to:])
	}
	return out
}

// decodeXMLMissing decodes in into a fresh value from into with WithMissing
// and the extra options, and returns the value, the pointers reported and
// the error.
func decodeXMLMissing(into func() any, in []byte, opts ...canxml.DecoderOption) (any, []string, error) {
	var got []string
	v := into()
	opts = append(opts, canxml.WithMissing(func(p string) { got = append(got, p) }))
	err := canxml.Unmarshal(in, v, opts...)
	return v, got, err
}

// assertXMLReportedAsDefault decodes in three ways and checks every facet
// REQ-175 gives the report option: the default decodes in with no error;
// WithMissing reports exactly want, once each, and returns the default's
// value; WithStrict refuses nothing, with or without WithMissing beside it,
// and the report under strict is the same.
func assertXMLReportedAsDefault(t *testing.T, into func() any, in []byte, want []string) {
	t.Helper()
	assertXMLReported(t, into, in, want, false)
}

// assertXMLReported is [assertXMLReportedAsDefault] for in, which also holds
// unmatched content when unmatched is set: WithStrict then refuses it, with
// or without WithMissing beside it, through canxml.ErrUnknownElement, never
// the missing members.
func assertXMLReported(t *testing.T, into func() any, in []byte, want []string, unmatched bool) {
	t.Helper()
	def := into()
	if err := canxml.Unmarshal(in, def); err != nil {
		t.Fatalf("canxml.Unmarshal(%s) = %v; want nil: a missing member is no decode error", in, err)
	}
	got, pointers, err := decodeXMLMissing(into, in)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(%s, WithMissing) = %v; want nil: the option reports, never refuses", in, err)
	}
	if !slices.Equal(pointers, want) {
		t.Errorf("WithMissing reported %q; want %q", pointers, want)
	}
	if !reflect.DeepEqual(got, def) {
		t.Errorf("canxml.Unmarshal(WithMissing) decoded\n%#v\nwant the default's value\n%#v", got, def)
	}
	if unmatched {
		for _, opts := range [][]canxml.DecoderOption{{canxml.WithStrict()}, {canxml.WithStrict(), canxml.WithMissing(func(string) {})}} {
			if err := canxml.Unmarshal(in, into(), opts...); !errors.Is(err, canxml.ErrUnknownElement) {
				t.Errorf("canxml.Unmarshal(%s, WithStrict) = %v; want canxml.ErrUnknownElement: strict refuses the unmatched content", in, err)
			}
		}
		return
	}
	if err := canxml.Unmarshal(in, into(), canxml.WithStrict()); err != nil {
		t.Errorf("canxml.Unmarshal(%s, WithStrict) = %v; want nil: strict refuses unmatched content only", in, err)
	}
	_, strictPointers, err := decodeXMLMissing(into, in, canxml.WithStrict())
	if err != nil {
		t.Errorf("canxml.Unmarshal(%s, WithStrict, WithMissing) = %v; want nil", in, err)
	}
	if !slices.Equal(strictPointers, want) {
		t.Errorf("WithMissing under WithStrict reported %q; want %q", strictPointers, want)
	}
}

// missingCompositionXML is canjson's missingComposition in canonical XML,
// member for member: a complete COMPOSITION, every element carrying every
// member the RM requires of it, so WithMissing reports nothing for it, and
// the rows of missingXMLRows take members out of it. It also carries what
// the control rows of REQ-175's acceptance need present: a bare PARTY_SELF
// as subject and composer, a DV_INTERVAL with no boundary flags, a
// POINT_EVENT with no offset, and archetype_node_id as an XML attribute. Its
// CLUSTER holds one item, since canonical XML has no element for an empty
// list (see the row "a CLUSTER with no items").
var missingCompositionXML = `<composition ` + ucNS + ` archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1">
  <name><value>encounter</value></name>
  <archetype_details><archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id><rm_version>1.1.0</rm_version></archetype_details>
  ` + xLanguage + `
  <territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory>
  <category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category>
  <composer xsi:type="PARTY_SELF"/>
  <content xsi:type="OBSERVATION" archetype_node_id="openEHR-EHR-OBSERVATION.vitals.v1">
    <name><value>vitals</value></name>
    ` + xLanguage + `
    <encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding>
    <subject xsi:type="PARTY_SELF"/>
    <data xsi:type="HISTORY" archetype_node_id="at0001">
      <name><value>history</value></name>
      <origin>` + xTime + `</origin>
      <events xsi:type="POINT_EVENT" archetype_node_id="at0002">
        <name><value>event</value></name>
        <time>` + xTime + `</time>
        <data xsi:type="ITEM_TREE" archetype_node_id="at0003">
          <name><value>tree</value></name>
          <items xsi:type="ELEMENT" archetype_node_id="at0004"><name><value>weight</value></name><value xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units>kg</units></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0005"><name><value>count</value></name><value xsi:type="DV_COUNT"><magnitude>2</magnitude></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0006"><name><value>flag</value></name><value xsi:type="DV_BOOLEAN"><value>true</value></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0007"><name><value>ratio</value></name><value xsi:type="DV_PROPORTION"><numerator>1</numerator><denominator>2</denominator><type>0</type></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0008"><name><value>score</value></name><value xsi:type="DV_ORDINAL"><value>1</value><symbol><value>mild</value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at0009</code_string></defining_code></symbol></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="openEHR-EHR-ELEMENT.note.v1"><name><value>note</value></name><archetype_details><archetype_id><value>openEHR-EHR-ELEMENT.note.v1</value></archetype_id><rm_version>1.1.0</rm_version></archetype_details><value xsi:type="DV_TEXT"><value>n</value></value></items>
          <items xsi:type="ELEMENT" archetype_node_id="at0010"><name><value>range</value></name><value xsi:type="DV_INTERVAL"><lower xsi:type="DV_QUANTITY"><magnitude>1</magnitude><units>kg</units></lower><upper xsi:type="DV_QUANTITY"><magnitude>2</magnitude><units>kg</units></upper></value></items>
          <items xsi:type="CLUSTER" archetype_node_id="at0011"><name><value>cluster</value></name><items xsi:type="ELEMENT" archetype_node_id="at0012"><name><value>inner</value></name></items></items>
        </data>
      </events>
    </data>
  </content>
</composition>`

// The pointers of the fixture's elements the rows edit, as canjson's.
const (
	mxEntry   = "/content/0"
	mxHistory = mxEntry + "/data"
	mxItems   = mxHistory + "/events/0/data/items/"
)

// missingXMLRow is one edit of missingCompositionXML and the members
// WithMissing must then report. edits maps an element's pointer to its new
// XML text, or to "" to leave the element out.
type missingXMLRow struct {
	name  string
	edits map[string]string
	want  []string
	// unmatched marks a document that also holds unmatched content, which
	// strict refuses: what a nilled element holds (REQ-056).
	unmatched bool
}

// missingXMLRows puts a missing member everywhere REQ-175's acceptance
// reaches in canonical XML, one row each (PROBE-110's table), the rows of
// canjson's missingRows in XML: the root, an ENTRY, a HISTORY, and a data
// value of each value-typed kind; the xsi:nil and empty-element forms; the
// control rows, whose member is present and not reported; the one place the
// two formats differ, a mandatory list with no items; and one row with
// every member left out at once, which pins the order: each element's
// members as the element closes, so the data values' members come before
// their HISTORY's, which comes before the ENTRY's and the root's, in the
// list canjson's test asserts.
func missingXMLRows() []missingXMLRow {
	omitted := map[string]string{
		"/language":                                "",
		mxEntry + "/subject":                       "",
		mxHistory + "/origin":                      "",
		mxItems + "0/value/magnitude":              "",
		mxItems + "1/value/magnitude":              "",
		mxItems + "2/value/value":                  "",
		mxItems + "3/value/denominator":            "",
		mxItems + "4/value/symbol":                 "",
		mxItems + "5/archetype_details/rm_version": "",
	}
	rows := []missingXMLRow{
		{name: "every member present", want: nil},
		{name: "root language", edits: map[string]string{"/language": ""}, want: []string{"/language"}},
		{name: "ENTRY subject", edits: map[string]string{mxEntry + "/subject": ""}, want: []string{mxEntry + "/subject"}},
		{name: "HISTORY origin", edits: map[string]string{mxHistory + "/origin": ""}, want: []string{mxHistory + "/origin"}},
		{name: "DV_QUANTITY magnitude", edits: map[string]string{mxItems + "0/value/magnitude": ""}, want: []string{mxItems + "0/value/magnitude"}},
		{name: "DV_COUNT magnitude", edits: map[string]string{mxItems + "1/value/magnitude": ""}, want: []string{mxItems + "1/value/magnitude"}},
		{name: "DV_BOOLEAN value", edits: map[string]string{mxItems + "2/value/value": ""}, want: []string{mxItems + "2/value/value"}},
		{name: "DV_PROPORTION denominator", edits: map[string]string{mxItems + "3/value/denominator": ""}, want: []string{mxItems + "3/value/denominator"}},
		{name: "DV_ORDINAL symbol", edits: map[string]string{mxItems + "4/value/symbol": ""}, want: []string{mxItems + "4/value/symbol"}},
		{name: "ARCHETYPED rm_version", edits: map[string]string{mxItems + "5/archetype_details/rm_version": ""}, want: []string{mxItems + "5/archetype_details/rm_version"}},
		{
			// Both XML Schema spellings of xsi:nil, on a polymorphic slot, a
			// number and a boolean.
			name: "xsi:nil members",
			edits: map[string]string{
				mxEntry + "/subject":          `<subject xsi:type="PARTY_SELF" xsi:nil="true"/>`,
				mxItems + "0/value/magnitude": `<magnitude xsi:nil="1"/>`,
				mxItems + "2/value/value":     `<value xsi:nil="true"/>`,
			},
			want: []string{mxItems + "0/value/magnitude", mxItems + "2/value/value", mxEntry + "/subject"},
		},
		{
			// A nilled element is reported once, by its parent: the DV_DATE_TIME's
			// own value, which the nilled element leaves out too, is not.
			name:  "nothing inside a nilled element",
			edits: map[string]string{mxHistory + "/origin": `<origin xsi:nil="true"/>`},
			want:  []string{mxHistory + "/origin"},
		},
		{
			// A nilled element is reported once, by its parent, whatever it
			// holds, and nothing below it is: a DV_DATE_TIME holding an empty
			// `value`, and a PARTY_IDENTIFIED holding a PARTY_REF that lacks
			// its `namespace` and `type`, whose id lacks its `value` one level
			// further down. What a nilled element holds is unmatched content,
			// which strict refuses.
			name: "nothing inside a nilled element that holds content",
			edits: map[string]string{
				mxHistory + "/origin": `<origin xsi:nil="true"><value></value></origin>`,
				mxEntry + "/subject":  `<subject xsi:type="PARTY_IDENTIFIED" xsi:nil="true"><external_ref><id xsi:type="HIER_OBJECT_ID"/></external_ref></subject>`,
			},
			want:      []string{mxHistory + "/origin", mxEntry + "/subject"},
			unmatched: true,
		},
		{
			// A primitive element with no text, or XML whitespace only, carries
			// no value: a number, a string beside the fixture's `<units>kg</units>`,
			// a boolean written self-closing, and a string holding a line break.
			name: "empty primitive elements",
			edits: map[string]string{
				mxItems + "0/value/magnitude":              `<magnitude></magnitude>`,
				mxItems + "0/value/units":                  `<units></units>`,
				mxItems + "2/value/value":                  `<value/>`,
				mxItems + "5/archetype_details/rm_version": "<rm_version>\n\t</rm_version>",
			},
			want: []string{
				mxItems + "0/value/magnitude",
				mxItems + "0/value/units",
				mxItems + "2/value/value",
				mxItems + "5/archetype_details/rm_version",
			},
		},
		{name: "control: magnitude 0", edits: map[string]string{mxItems + "0/value/magnitude": `<magnitude>0</magnitude>`}, want: nil},
		{name: "control: false value", edits: map[string]string{mxItems + "2/value/value": `<value>false</value>`}, want: nil},
		{name: "control: nil written false", edits: map[string]string{mxItems + "1/value/magnitude": `<magnitude xsi:nil="false">2</magnitude>`}, want: nil},
		{
			// Canonical XML has no element for an empty list, so a CLUSTER whose
			// items are all left out leaves its mandatory items out, where
			// canonical JSON's `"items": []` is present.
			name:  "a CLUSTER with no items",
			edits: map[string]string{mxItems + "7/items/0": ""},
			want:  []string{mxItems + "7/items"},
		},
	}
	all := missingXMLRow{name: "every member at once", edits: omitted, want: []string{
		mxItems + "0/value/magnitude",
		mxItems + "1/value/magnitude",
		mxItems + "2/value/value",
		mxItems + "3/value/denominator",
		mxItems + "4/value/symbol",
		mxItems + "5/archetype_details/rm_version",
		mxHistory + "/origin",
		mxEntry + "/subject",
		"/language",
	}}
	return append(rows, all)
}

// TestREQ175_XMLMissingMembersReportedAsDefault drives each row through
// assertXMLReportedAsDefault on canxml.Unmarshal into an rm.Composition.
func TestREQ175_XMLMissingMembersReportedAsDefault(t *testing.T) {
	into := func() any { return new(rm.Composition) }
	for _, row := range missingXMLRows() {
		t.Run(row.name, func(t *testing.T) {
			in := rewriteXML(t, missingCompositionXML, row.edits)
			assertXMLReported(t, into, in, row.want, row.unmatched)
		})
	}
}

// TestREQ175_XMLMissingMembersAtOtherRoots pins the report at roots other
// than a COMPOSITION: an EHR_STATUS without its subject (PROBE-081's root)
// and with a bare one, a by-value struct element with no content;
// archetype_node_id written as an XML attribute, as a child element and left
// out; and a value with every member missing, in the RM's order.
func TestREQ175_XMLMissingMembersAtOtherRoots(t *testing.T) {
	const nodeID = "openEHR-EHR-EHR_STATUS.generic.v1"
	status := func(attrs, children string) string {
		return `<ehr_status ` + ucNS + attrs + `><name><value>s</value></name>` + children +
			`<is_queryable>true</is_queryable><is_modifiable>true</is_modifiable></ehr_status>`
	}
	intoStatus := func() any { return new(rm.EHRStatus) }
	cases := []struct {
		name, in string
		into     func() any
		want     []string
	}{
		{
			name: "EHR_STATUS without subject",
			in:   status(` archetype_node_id="`+nodeID+`"`, ""),
			into: intoStatus,
			want: []string{"/subject"},
		},
		{
			name: "EHR_STATUS with a bare subject",
			in:   status(` archetype_node_id="`+nodeID+`"`, `<subject/>`),
			into: intoStatus,
		},
		{
			name: "archetype_node_id as a child element",
			in:   status("", `<archetype_node_id>`+nodeID+`</archetype_node_id><subject/>`),
			into: intoStatus,
		},
		{
			name: "archetype_node_id left out",
			in:   status("", `<subject/>`),
			into: intoStatus,
			want: []string{"/archetype_node_id"},
		},
		{
			name: "every member missing, in the RM's order",
			in:   `<dv_ordinal ` + ucNS + `/>`,
			into: func() any { return new(rm.DVOrdinal) },
			want: []string{"/symbol", "/value"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertXMLReportedAsDefault(t, tc.into, []byte(tc.in), tc.want)
		})
	}
}

// TestREQ175_XMLAOM14ClassesAreChecked pins the scope canonical XML has and
// canonical JSON has not: every class the generated decoders read is
// checked, the AOM 1.4 classes included. A C_COMPLEX_OBJECT without its
// occurrences, holding a C_SINGLE_ATTRIBUTE without its existence, reports
// both, the inner one first.
func TestREQ175_XMLAOM14ClassesAreChecked(t *testing.T) {
	in := []byte(`<definition ` + ucNS + `><rm_type_name>OBSERVATION</rm_type_name><node_id>at0000</node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name></attributes></definition>`)
	into := func() any { return new(aom14.CComplexObject) }
	assertXMLReportedAsDefault(t, into, in, []string{"/attributes/0/existence", "/occurrences"})
}

// TestREQ175_XMLWithUnmatchedAndWithMissingShareOneDecode pins that the two
// report options are independent, may be combined, and are served by one
// pass over the input, each with its own grammar: unmatched items come in
// document order as they are met, by their element path, and missing
// members as their element closes, by their pointer, so the two reports
// interleave by position in the document. Neither reports the other's
// items, and no pointer carries a value from the input.
func TestREQ175_XMLWithUnmatchedAndWithMissingShareOneDecode(t *testing.T) {
	in := `<dv_coded_text ` + ucNS + ` x-a="` + xmlPlanted + `"><x-b>` + xmlPlanted + `</x-b>` +
		`<defining_code><x-c/><code_string>c</code_string></defining_code><x-d/></dv_coded_text>`
	var log []string
	err := canxml.Unmarshal([]byte(in), new(rm.DVCodedText),
		canxml.WithUnmatched(func(p string) { log = append(log, "unmatched "+p) }),
		canxml.WithMissing(func(p string) { log = append(log, "missing "+p) }),
	)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithUnmatched, WithMissing) = %v; want nil", err)
	}
	want := []string{
		"unmatched /dv_coded_text[1]/@x-a",
		"unmatched /dv_coded_text[1]/x-b[1]",
		"unmatched /dv_coded_text[1]/defining_code[1]/x-c[1]",
		"missing /defining_code/terminology_id",
		"unmatched /dv_coded_text[1]/x-d[1]",
		"missing /value",
	}
	if !slices.Equal(log, want) {
		t.Errorf("the options reported\n%q\nwant, in one pass,\n%q", log, want)
	}
	assertNoPlantedValue(t, log)
}

// TestREQ175_XMLDecoderReportsPerValue drives the streaming entry point: a
// Decoder takes the option, each value's pointers are relative to that
// value, and strict refuses nothing.
func TestREQ175_XMLDecoderReportsPerValue(t *testing.T) {
	stream := `<dv_text ` + ucNS + `/> <dv_quantity ` + ucNS + `><units>kg</units><magnitude xsi:nil="true"/></dv_quantity>`
	var got []string
	dec := canxml.NewDecoder(strings.NewReader(stream), canxml.WithMissing(func(p string) { got = append(got, p) }), canxml.WithStrict())
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

// TestREQ175_XMLNilMissingIsNoOption pins that WithMissing(nil) is the same
// as leaving the option out: the decode is the default's, and a Decoder
// given it is untracked, so a primitive failure names the element alone, as
// with no options, where a Decoder given a function names it by its path
// from the root of the value.
func TestREQ175_XMLNilMissingIsNoOption(t *testing.T) {
	in := []byte(`<dv_quantity ` + ucNS + `><units>kg</units></dv_quantity>`)
	var want, got rm.DVQuantity
	if err := canxml.Unmarshal(in, &want); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil", err)
	}
	if err := canxml.Unmarshal(in, &got, canxml.WithMissing(nil)); err != nil {
		t.Fatalf("canxml.Unmarshal(WithMissing(nil)) = %v; want nil", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canxml.Unmarshal(WithMissing(nil)) decoded %+v; want the default's %+v", got, want)
	}

	bad := `<dv_quantity ` + ucNS + `><magnitude>` + xmlPlanted + `</magnitude><units>kg</units></dv_quantity>`
	err := canxml.NewDecoder(strings.NewReader(bad), canxml.WithMissing(nil)).Decode(new(rm.DVQuantity))
	if de, ok := errors.AsType[*canxml.DecodeError](err); !ok || de == nil || de.Path != "/magnitude" {
		t.Errorf("Decoder(WithMissing(nil)).Decode err = %v; want a DecodeError on \"/magnitude\", the element alone, as with no options", err)
	}
	err = canxml.NewDecoder(strings.NewReader(bad), canxml.WithMissing(func(string) {})).Decode(new(rm.DVQuantity))
	if de, ok := errors.AsType[*canxml.DecodeError](err); !ok || de == nil || de.Path != "/dv_quantity[1]/magnitude[1]" {
		t.Errorf("Decoder(WithMissing).Decode err = %v; want a DecodeError on \"/dv_quantity[1]/magnitude[1]\": the decode is tracked", err)
	}
}

// TestREQ175_XMLUnmodelledMemberIsPresent pins that the element of a member
// the codec does not model, RESOURCE_ANNOTATIONS `documentation`, a Hash of
// Hashes, gives the member a value though the decode skips it as unmatched
// content: WithMissing reports the member only when the document leaves it
// out, with WithUnmatched beside it or not.
func TestREQ175_XMLUnmodelledMemberIsPresent(t *testing.T) {
	into := func() any { return new(rm.ResourceAnnotations) }
	for _, c := range []struct {
		name string
		in   string
		want []string
	}{
		{name: "carried", in: `<annotations ` + ucNS + `><documentation id="ui"><item id="a"><item id="b">c</item></item></documentation></annotations>`},
		{name: "left out", in: `<annotations ` + ucNS + `/>`, want: []string{"/documentation"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, opts := range [][]canxml.DecoderOption{nil, {canxml.WithUnmatched(func(string) {})}} {
				_, got, err := decodeXMLMissing(into, []byte(c.in), opts...)
				if err != nil {
					t.Fatalf("canxml.Unmarshal(WithMissing) = %v; want nil", err)
				}
				if !slices.Equal(got, c.want) {
					t.Errorf("WithMissing (%d other options) reported %q; want %q", len(opts), got, c.want)
				}
			}
		})
	}
}

// TestREQ175_XMLMandatoryWithoutAValueDecodesAsAbsent pins that a mandatory
// member sent without a value, a string element whose text is XML whitespace
// only or an element carrying xsi:nil whatever it holds, is missing and
// decodes as the default for a missing member asks: in every mode, through
// canxml.Unmarshal and a canxml.Decoder, the document decodes to the value of
// the same document without the element, its zero value. A nilled
// polymorphic member needs no xsi:type. What a nilled element holds is
// unmatched content, which strict refuses. TERMINOLOGY_CODE, whose decoder is
// written by hand, does the same, `terminology_id` in both spellings. An item
// of a list, which is no member, and text with whitespace around it keep
// their text.
func TestREQ175_XMLMandatoryWithoutAValueDecodesAsAbsent(t *testing.T) {
	const ws = " \t\n"
	quantity := func(units string) string {
		return `<q ` + ucNS + `><magnitude>1</magnitude>` + units + `</q>`
	}
	code := func(id, codeString string) string {
		return `<c ` + ucNS + `>` + id + codeString + `</c>`
	}
	observation := func(subject string) string {
		return `<o ` + ucNS + ` archetype_node_id="openEHR-EHR-OBSERVATION.x.v1"><name><value>o</value></name>` + subject + `</o>`
	}
	item := func(keywords string) string {
		return `<item ` + ucNS + `><language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language><purpose>p</purpose>` + keywords + `</item>`
	}
	for _, c := range []struct {
		name        string
		into        func() any
		in, without string
		unmatched   bool             // the nilled element holds content, which strict refuses
		keep        func(v any) bool // a control: whether the value keeps its text
		kept        string
	}{
		{name: "DV_QUANTITY units", into: func() any { return new(rm.DVQuantity) }, in: quantity(`<units>` + ws + `</units>`), without: quantity("")},
		{name: "DV_TEXT value", into: func() any { return new(rm.DVText) }, in: `<t ` + ucNS + `><value> </value></t>`, without: `<t ` + ucNS + `/>`},
		{
			name: "TERMINOLOGY_CODE code_string", into: func() any { return new(rm.TerminologyCode) },
			in: code(`<terminology_id>snomed_ct</terminology_id>`, `<code_string>`+ws+`</code_string>`), without: code(`<terminology_id>snomed_ct</terminology_id>`, ""),
		},
		{
			name: "TERMINOLOGY_CODE terminology_id as text", into: func() any { return new(rm.TerminologyCode) },
			in: code(`<terminology_id>`+ws+`</terminology_id>`, `<code_string>1</code_string>`), without: code("", `<code_string>1</code_string>`),
		},
		{
			name: "TERMINOLOGY_CODE terminology_id in a value child", into: func() any { return new(rm.TerminologyCode) },
			in: code(`<terminology_id><value>`+ws+`</value></terminology_id>`, `<code_string>1</code_string>`), without: code("", `<code_string>1</code_string>`),
		},
		{
			name: "AOM 1.4 C_COMPLEX_OBJECT rm_type_name", into: func() any { return new(aom14.CComplexObject) },
			in: `<c ` + ucNS + `><rm_type_name>` + ws + `</rm_type_name><node_id>at0000</node_id></c>`, without: `<c ` + ucNS + `><node_id>at0000</node_id></c>`,
		},
		{name: "nilled DV_QUANTITY units", into: func() any { return new(rm.DVQuantity) }, in: quantity(`<units xsi:nil="true"/>`), without: quantity("")},
		{name: "nilled DV_QUANTITY units holding text", into: func() any { return new(rm.DVQuantity) }, in: quantity(`<units xsi:nil="true">kg</units>`), without: quantity(""), unmatched: true},
		{
			name: "nilled CODE_PHRASE terminology_id holding content", into: func() any { return new(rm.CodePhrase) },
			in:      `<c ` + ucNS + `><terminology_id xsi:nil="1"><value>local</value></terminology_id><code_string>1</code_string></c>`,
			without: `<c ` + ucNS + `><code_string>1</code_string></c>`, unmatched: true,
		},
		{
			name: "nilled polymorphic OBSERVATION subject, no xsi:type", into: func() any { return new(rm.Observation) },
			in: observation(`<subject xsi:nil="true"/>`), without: observation(""),
		},
		{
			name: "nilled TERMINOLOGY_CODE members holding text", into: func() any { return new(rm.TerminologyCode) },
			in: code(`<terminology_id xsi:nil="true">snomed_ct</terminology_id>`, `<code_string xsi:nil="true">1</code_string>`), without: code("", ""), unmatched: true,
		},
		{
			name: "control: a list item", into: func() any { return new(rm.ResourceDescriptionItem) },
			in: item(`<keywords> </keywords>`), kept: "a keyword of one space",
			keep: func(v any) bool { return slices.Equal(v.(*rm.ResourceDescriptionItem).Keywords, []string{" "}) },
		},
		{
			name: "control: text with whitespace around it", into: func() any { return new(rm.DVText) },
			in: `<t ` + ucNS + `><value> a </value></t>`, kept: `the value " a "`,
			keep: func(v any) bool { return v.(*rm.DVText).Value == " a " },
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, m := range []struct {
				name string
				opts []canxml.DecoderOption
			}{
				{name: "default"},
				{name: "WithStrict", opts: []canxml.DecoderOption{canxml.WithStrict()}},
				{name: "WithUnmatched", opts: []canxml.DecoderOption{canxml.WithUnmatched(func(string) {})}},
				{name: "WithMissing", opts: []canxml.DecoderOption{canxml.WithMissing(func(string) {})}},
				{name: "WithPresence", opts: []canxml.DecoderOption{canxml.WithPresence(func(string, canxml.Presence) {})}},
				{name: "WithMaxDepth", opts: []canxml.DecoderOption{canxml.WithMaxDepth(64)}},
			} {
				decode := map[string]func(string) (any, error){
					"canxml.Unmarshal": func(doc string) (any, error) {
						v := c.into()
						return v, canxml.Unmarshal([]byte(doc), v, m.opts...)
					},
					"Decoder.Decode": func(doc string) (any, error) {
						v := c.into()
						return v, canxml.NewDecoder(strings.NewReader(doc), m.opts...).Decode(v)
					},
				}
				for call, run := range decode {
					got, err := run(c.in)
					if c.unmatched && m.name == "WithStrict" {
						if !errors.Is(err, canxml.ErrUnknownElement) {
							t.Errorf("%s %s(%s) = %v; want canxml.ErrUnknownElement: what a nilled element holds is unmatched content", call, m.name, c.in, err)
						}
						continue
					}
					if err != nil {
						t.Fatalf("%s %s(%s) = %v; want nil", call, m.name, c.in, err)
					}
					if c.keep != nil {
						if !c.keep(got) {
							t.Errorf("%s %s(%s) = %#v; want %s kept", call, m.name, c.in, got, c.kept)
						}
						continue
					}
					want, err := run(c.without)
					if err != nil {
						t.Fatalf("%s %s(%s) = %v; want nil", call, m.name, c.without, err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s %s(%s) = %#v; want %#v, the value of the document without the element", call, m.name, c.in, got, want)
					}
				}
			}
		})
	}
}

// TestREQ175_XMLMandatoryWithoutAValueClearsAReusedValue pins that a decode
// into a value that already holds a mandatory member gives it its zero value
// when the document sends it without one, nilled or a string of whitespace
// only, as an optional one sent so is left nil: the document says the member
// has no value, where one that leaves the element out says nothing of it.
func TestREQ175_XMLMandatoryWithoutAValueClearsAReusedValue(t *testing.T) {
	quantity := func(units string) string {
		return `<q ` + ucNS + `><magnitude>1</magnitude>` + units + `</q>`
	}
	for _, in := range []string{quantity(`<units xsi:nil="true"/>`), quantity(`<units> </units>`)} {
		q := rm.DVQuantity{Units: "kg"}
		if err := canxml.NewDecoder(strings.NewReader(in)).Decode(&q); err != nil {
			t.Fatalf("Decoder.Decode(%s) = %v; want nil", in, err)
		}
		if q.Units != "" {
			t.Errorf("Decoder.Decode(%s) into a value with units kg left Units = %q; want \"\"", in, q.Units)
		}
	}
	code := rm.TerminologyCode{TerminologyID: "snomed_ct", CodeString: "1"}
	in := `<c ` + ucNS + `><terminology_id xsi:nil="true"/><code_string xsi:nil="true"/></c>`
	if err := canxml.NewDecoder(strings.NewReader(in)).Decode(&code); err != nil {
		t.Fatalf("Decoder.Decode(%s) = %v; want nil", in, err)
	}
	if code.TerminologyID != "" || code.CodeString != "" {
		t.Errorf("Decoder.Decode(%s) into a filled TERMINOLOGY_CODE left %+v; want both members empty", in, code)
	}
}
