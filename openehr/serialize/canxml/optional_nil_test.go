package canxml_test

// optional_nil_test.go: REQ-056 (wire.md § REQ-056) for an optional
// single-valued member sent without a value: an element carrying xsi:nil
// set, whatever it holds, or, for a member of a primitive type, an element
// with no text, or whitespace only for a string. Every decode leaves such a
// member absent, a nil pointer, interface or slice, as canonical JSON's null
// does; what a nilled element holds is unmatched content. A mandatory
// member, an enumeration and an empty struct element decode as before.

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// nilCase is one document whose optional member is sent without a value, the
// same document without the member's element, and a check of the decoded
// value that does not lean on the comparison.
type nilCase struct {
	name    string
	into    func() any
	in      string
	without string
	check   func(v any) string // "" when the value is right, else what is wrong
}

func quantityXML(extra string) string {
	return `<value ` + ucNS + ` xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units>kg</units>` + extra + `</value>`
}

func elementXML(extra string) string {
	return `<items ` + ucNS + ` xsi:type="ELEMENT" archetype_node_id="at0004"><name><value>weight</value></name>` + extra + `</items>`
}

func adminEntryXML(extra string) string {
	return `<content ` + ucNS + ` archetype_node_id="openEHR-EHR-ADMIN_ENTRY.a.v1"><name><value>a</value></name>` + xLanguage +
		`<encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding>` +
		`<subject xsi:type="PARTY_SELF"/>` + extra +
		`<data xsi:type="ITEM_TREE" archetype_node_id="at0001"><name><value>tree</value></name></data></content>`
}

func multimediaXML(extra string) string {
	return `<value ` + ucNS + ` xsi:type="DV_MULTIMEDIA">` + extra +
		`<media_type><terminology_id><value>IANA_media-types</value></terminology_id><code_string>text/plain</code_string></media_type><size>1</size></value>`
}

func nilCases() []nilCase {
	precision := func(v any) string {
		if p := v.(*rm.DVQuantity).Precision; p != nil {
			return "Precision is non-nil; want nil"
		}
		return ""
	}
	return []nilCase{
		{name: "an empty Integer", into: func() any { return new(rm.DVQuantity) }, in: quantityXML(`<precision></precision>`), without: quantityXML(""), check: precision},
		{name: "a self-closing Integer", into: func() any { return new(rm.DVQuantity) }, in: quantityXML(`<precision/>`), without: quantityXML(""), check: precision},
		{name: "a nilled Integer", into: func() any { return new(rm.DVQuantity) }, in: quantityXML(`<precision xsi:nil="true"/>`), without: quantityXML(""), check: precision},
		{name: "a nilled Integer, written 1", into: func() any { return new(rm.DVQuantity) }, in: quantityXML(`<precision xsi:nil="1"/>`), without: quantityXML(""), check: precision},
		{
			name: "a string of whitespace", into: func() any { return new(rm.DVQuantity) },
			in: quantityXML("<units_system> \n\t</units_system><units_display_name></units_display_name>"), without: quantityXML(""),
			check: func(v any) string {
				if q := v.(*rm.DVQuantity); q.UnitsSystem != nil || q.UnitsDisplayName != nil {
					return "UnitsSystem or UnitsDisplayName is non-nil; want nil"
				}
				return ""
			},
		},
		{
			name: "a nilled struct", into: func() any { return new(rm.Element) },
			in: elementXML(`<null_flavour xsi:nil="true"/>`), without: elementXML(""),
			check: func(v any) string {
				if v.(*rm.Element).NullFlavour != nil {
					return "NullFlavour is non-nil; want nil"
				}
				return ""
			},
		},
		{
			// A nilled polymorphic member needs no xsi:type.
			name: "a nilled polymorphic member with no xsi:type", into: func() any { return new(rm.AdminEntry) },
			in: adminEntryXML(`<provider xsi:nil="true"/>`), without: adminEntryXML(""),
			check: func(v any) string {
				if v.(*rm.AdminEntry).Provider != nil {
					return "Provider is non-nil; want nil"
				}
				return ""
			},
		},
		{
			name: "a nilled Octet list", into: func() any { return new(rm.DVMultimedia) },
			in: multimediaXML(`<data xsi:nil="true"/>`), without: multimediaXML(""),
			check: func(v any) string {
				if v.(*rm.DVMultimedia).Data != nil {
					return "Data is non-nil; want nil"
				}
				return ""
			},
		},
	}
}

// TestREQ056_XMLOptionalMemberWithoutAValueIsAbsent decodes each case through
// canxml.Unmarshal and a canxml.Decoder, with no option and with each report
// and strict option: every decode succeeds, leaves the member nil, and
// returns the value of the same document without the member's element.
func TestREQ056_XMLOptionalMemberWithoutAValueIsAbsent(t *testing.T) {
	modes := map[string][]canxml.DecoderOption{
		"default":       nil,
		"WithUnmatched": {canxml.WithUnmatched(func(p string) { t.Errorf("WithUnmatched reported %q; want nothing", p) })},
		"WithStrict":    {canxml.WithStrict()},
		"WithMissing":   {canxml.WithMissing(func(string) {})},
		"WithPresence":  {canxml.WithPresence(func(string, canxml.Presence) {})},
	}
	for _, c := range nilCases() {
		t.Run(c.name, func(t *testing.T) {
			want := c.into()
			if err := canxml.Unmarshal([]byte(c.without), want); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", c.without, err)
			}
			for mode, opts := range modes {
				for call, decode := range map[string]func(v any) error{
					"canxml.Unmarshal": func(v any) error { return canxml.Unmarshal([]byte(c.in), v, opts...) },
					"Decoder.Decode":   func(v any) error { return canxml.NewDecoder(strings.NewReader(c.in), opts...).Decode(v) },
				} {
					got := c.into()
					if err := decode(got); err != nil {
						t.Fatalf("%s(%s, %s) = %v; want nil", call, c.in, mode, err)
					}
					if msg := c.check(got); msg != "" {
						t.Errorf("%s %s: %s", call, mode, msg)
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s %s decoded\n%#v\nwant the value without the element\n%#v", call, mode, got, want)
					}
				}
			}
		})
	}
}

// TestREQ056_XMLOptionalMemberWithAValue pins the controls: a member that
// carries a value keeps it. A precision of 0 is a pointer to 0, an empty
// struct element is a value, and `xsi:nil="false"` is no nil.
func TestREQ056_XMLOptionalMemberWithAValue(t *testing.T) {
	var q rm.DVQuantity
	if err := canxml.Unmarshal([]byte(quantityXML(`<precision>0</precision>`)), &q); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil", err)
	}
	if q.Precision == nil || *q.Precision != 0 {
		t.Errorf("Precision = %v; want a pointer to 0", q.Precision)
	}
	q = rm.DVQuantity{}
	if err := canxml.Unmarshal([]byte(quantityXML(`<precision xsi:nil="false">2</precision>`)), &q); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil", err)
	}
	if q.Precision == nil || *q.Precision != 2 {
		t.Errorf("Precision = %v; want a pointer to 2: xsi:nil=\"false\" is no nil", q.Precision)
	}
	var e rm.Element
	if err := canxml.Unmarshal([]byte(elementXML(`<null_flavour/>`)), &e); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil", err)
	}
	if e.NullFlavour == nil {
		t.Errorf("NullFlavour = nil; want the empty struct an empty element is")
	}
}

// TestREQ056_XMLOptionalNumberOfWhitespaceFails pins that a number element of
// whitespace only, unlike an empty one, still fails as a primitive failure.
func TestREQ056_XMLOptionalNumberOfWhitespaceFails(t *testing.T) {
	for _, opts := range [][]canxml.DecoderOption{nil, {canxml.WithStrict()}} {
		err := canxml.Unmarshal([]byte(quantityXML(`<precision> </precision>`)), new(rm.DVQuantity), opts...)
		if !errors.Is(err, canxml.ErrInvalidShape) {
			t.Errorf("canxml.Unmarshal(<precision> </precision>, %d options) = %v; want canxml.ErrInvalidShape", len(opts), err)
		}
	}
}

// TestREQ056_XMLOptionalNilClearsAReusedValue pins that a decode into a value
// that already holds the member leaves it nil, as the document says.
func TestREQ056_XMLOptionalNilClearsAReusedValue(t *testing.T) {
	two := rm.Integer(2)
	for _, in := range []string{quantityXML(`<precision xsi:nil="true"/>`), quantityXML(`<precision></precision>`)} {
		q := rm.DVQuantity{Precision: &two}
		if err := canxml.NewDecoder(strings.NewReader(in)).Decode(&q); err != nil {
			t.Fatalf("Decoder.Decode(%s) = %v; want nil", in, err)
		}
		if q.Precision != nil {
			t.Errorf("Decoder.Decode(%s) into a value with precision 2 left Precision = %d; want nil", in, *q.Precision)
		}
	}
}

// TestREQ056_XMLContentOfANilledElementIsUnmatched pins that the decode does
// not read what a nilled optional element holds: its attributes, child
// elements and text are unmatched content, reported by WithUnmatched and
// refused by WithStrict, while the default leaves the member absent.
func TestREQ056_XMLContentOfANilledElementIsUnmatched(t *testing.T) {
	in := []byte(elementXML(`<null_flavour xsi:nil="true" x-flag="` + xmlPlanted + `"><value>` + xmlPlanted + `</value>` + xmlPlanted + `</null_flavour>`))
	want := []string{"/items[1]/null_flavour[1]/@x-flag", "/items[1]/null_flavour[1]/value[1]", "/items[1]/null_flavour[1]/text()"}
	into := func() any { return new(rm.Element) }
	got, paths, err := unmarshalCollecting(into, in)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil", err)
	}
	if !slices.Equal(paths, want) {
		t.Errorf("WithUnmatched reported %q; want %q", paths, want)
	}
	if got.(*rm.Element).NullFlavour != nil {
		t.Errorf("NullFlavour is non-nil; want nil")
	}
	err = canxml.Unmarshal(in, into(), canxml.WithStrict())
	assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, want[0], false)
}

// TestREQ056_XMLOptionalNilMatchesJSONNull pins that the canonical-XML and
// canonical-JSON forms of one instance decode to one value: a nilled or empty
// optional XML element and a JSON null.
func TestREQ056_XMLOptionalNilMatchesJSONNull(t *testing.T) {
	var fromJSON rm.DVQuantity
	if err := canjson.Unmarshal([]byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg","precision":null,"units_system":null}`), &fromJSON); err != nil {
		t.Fatalf("canjson.Unmarshal = %v; want nil", err)
	}
	for _, in := range []string{
		quantityXML(`<precision xsi:nil="true"/><units_system xsi:nil="true"/>`),
		quantityXML(`<precision></precision><units_system></units_system>`),
	} {
		var fromXML rm.DVQuantity
		if err := canxml.NewDecoder(bytes.NewReader([]byte(in))).Decode(&fromXML); err != nil {
			t.Fatalf("Decoder.Decode(%s) = %v; want nil", in, err)
		}
		if !reflect.DeepEqual(fromXML, fromJSON) {
			t.Errorf("canonical XML %s decoded %+v; want canonical JSON's null to decode the same, %+v", in, fromXML, fromJSON)
		}
	}
}
