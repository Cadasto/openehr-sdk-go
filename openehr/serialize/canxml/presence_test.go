package canxml_test

// presence_test.go: REQ-175 (wire.md § REQ-175, *Sent without a value*) for
// canonical XML; PROBE-110's sent-without-a-value rows. A member is sent
// without a value when its element carries xsi:nil set, or is a primitive
// element with no text or whitespace only. WithPresence reports each such
// member, mandatory or optional, once, by its JSON Pointer in the
// canonical-JSON form of the instance and the form it took, as its element
// closes, while the decode returns the default's value; WithMissing is
// unchanged beside it, and strict refuses none of it. The rows edit
// missingCompositionXML, the fixture of missing_test.go.

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// sent is one member WithPresence reports: its pointer and the form it took.
type sent struct {
	pointer  string
	presence canxml.Presence
}

func sentNil(pointer string) sent   { return sent{pointer, canxml.PresenceNil} }
func sentEmpty(pointer string) sent { return sent{pointer, canxml.PresenceEmpty} }

// decodeXMLPresence decodes in into a fresh value from into with
// WithPresence and the extra options, and returns the value, the members
// reported and the error.
func decodeXMLPresence(into func() any, in []byte, opts ...canxml.DecoderOption) (any, []sent, error) {
	var got []sent
	v := into()
	opts = append(opts, canxml.WithPresence(func(p string, s canxml.Presence) { got = append(got, sent{p, s}) }))
	err := canxml.Unmarshal(in, v, opts...)
	return v, got, err
}

// presenceRow is one edit of missingCompositionXML, the members WithPresence
// must then report, and the members WithMissing must report on the same
// document.
type presenceRow struct {
	name    string
	edits   map[string]string
	want    []sent
	missing []string
}

// The fixture's DV_QUANTITY, ELEMENT inside the CLUSTER, and their parts the
// rows send without a value.
const (
	pxQuantity = mxItems + "0/value"
	pxInner    = mxItems + "7/items/0"
	pxInnerXML = `<items xsi:type="ELEMENT" archetype_node_id="at0012"><name><value>inner</value></name>`
)

func presenceRows() []presenceRow {
	return []presenceRow{
		{name: "every member carries a value"},
		{
			// An optional member sent empty decodes as an absent one does: no
			// missing member, and the value cannot say it was sent.
			name:  "optional primitive sent empty",
			edits: map[string]string{pxQuantity + "/units": `<units>kg</units><precision></precision>`},
			want:  []sent{sentEmpty(pxQuantity + "/precision")},
		},
		{
			name:  "optional primitive sent nilled",
			edits: map[string]string{pxQuantity + "/units": `<units>kg</units><precision xsi:nil="true"/>`},
			want:  []sent{sentNil(pxQuantity + "/precision")},
		},
		{
			// A struct member, with xsi:nil's other XML Schema spelling.
			name:  "optional struct sent nilled",
			edits: map[string]string{pxInner: pxInnerXML + `<null_flavour xsi:nil="1"/></items>`},
			want:  []sent{sentNil(pxInner + "/null_flavour")},
		},
		{
			// A mandatory member sent without a value is a missing member too:
			// both options receive it, by one pointer.
			name: "mandatory members sent empty and nilled",
			edits: map[string]string{
				pxQuantity + "/units": `<units></units>`,
				mxEntry + "/subject":  `<subject xsi:type="PARTY_SELF" xsi:nil="true"/>`,
			},
			want:    []sent{sentNil(mxEntry + "/subject"), sentEmpty(pxQuantity + "/units")},
			missing: []string{pxQuantity + "/units", mxEntry + "/subject"},
		},
		{
			// A string element holding whitespace only carries no value.
			name:    "a string element of whitespace",
			edits:   map[string]string{mxItems + "5/archetype_details/rm_version": "<rm_version>\n\t</rm_version>"},
			want:    []sent{sentEmpty(mxItems + "5/archetype_details/rm_version")},
			missing: []string{mxItems + "5/archetype_details/rm_version"},
		},
		{
			// The nilled element is reported, and nothing inside it: not its
			// empty `value`.
			name:    "nothing inside a nilled element",
			edits:   map[string]string{mxHistory + "/origin": `<origin xsi:nil="true"><value></value></origin>`},
			want:    []sent{sentNil(mxHistory + "/origin")},
			missing: []string{mxHistory + "/origin"},
		},
		{
			// An item of a multi-valued member is named by its position. The
			// CLUSTER's mandatory items then carry no value, so they are
			// missing.
			name:    "a list item sent nilled",
			edits:   map[string]string{pxInner: `<items xsi:type="ELEMENT" xsi:nil="true"/>`},
			want:    []sent{sentNil(pxInner)},
			missing: []string{mxItems + "7/items"},
		},
		{
			// The members are reported as their elements close, in document
			// order.
			name: "document order",
			edits: map[string]string{
				mxEntry + "/subject":  `<subject xsi:type="PARTY_SELF"/><guideline_id xsi:nil="true"/>`,
				pxQuantity + "/units": `<units>kg</units><precision></precision>`,
				pxInner:               pxInnerXML + `<null_flavour xsi:nil="true"/></items>`,
			},
			want: []sent{
				sentNil(mxEntry + "/guideline_id"),
				sentEmpty(pxQuantity + "/precision"),
				sentNil(pxInner + "/null_flavour"),
			},
		},
		{
			// A repeat counts for nothing: the first `language`, nilled, is the
			// member's value, so it is reported, and missing; the repeat, which
			// carries a value, is unmatched content.
			name:    "a nilled member, then a repeat with a value",
			edits:   map[string]string{"/language": `<language xsi:nil="true"/>` + xLanguage},
			want:    []sent{sentNil("/language")},
			missing: []string{"/language"},
		},
		{
			name:  "a member with a value, then a nilled repeat",
			edits: map[string]string{"/language": xLanguage + `<language xsi:nil="true"/>`},
		},
		{
			// An element in a foreign namespace is unmatched content whatever
			// its local name: it gives no member a value, and is not a member
			// sent without one.
			name:    "a foreign element named like a mandatory member",
			edits:   map[string]string{"/language": `<x:language xmlns:x="urn:example:ext" xsi:nil="true"><x:code_string>en</x:code_string></x:language>`},
			missing: []string{"/language"},
		},
		{
			// An unknown element is unmatched content, nothing inside it is a
			// member.
			name:  "an unknown empty element",
			edits: map[string]string{pxQuantity + "/units": `<units>kg</units><x-ext></x-ext><x-ext xsi:nil="true"/>`},
		},
		{name: "control: magnitude 0", edits: map[string]string{pxQuantity + "/magnitude": `<magnitude>0</magnitude>`}},
		{name: "control: nil written false", edits: map[string]string{pxQuantity + "/magnitude": `<magnitude xsi:nil="false">2</magnitude>`}},
		{
			// An empty struct element is a value.
			name:  "control: an empty struct element",
			edits: map[string]string{mxEntry + "/subject": `<subject xsi:type="PARTY_SELF"></subject>`},
		},
	}
}

// TestREQ175_XMLPresenceReportsMembersSentWithoutAValue drives each row
// through canxml.Unmarshal into an rm.Composition: the default decodes it
// with no error; WithPresence receives exactly the row's members, once each,
// in order, and the default's value; WithMissing on the same document
// receives the row's missing members, alone and beside WithPresence; strict
// refuses none of it.
func TestREQ175_XMLPresenceReportsMembersSentWithoutAValue(t *testing.T) {
	into := func() any { return new(rm.Composition) }
	for _, row := range presenceRows() {
		t.Run(row.name, func(t *testing.T) {
			in := rewriteXML(t, missingCompositionXML, row.edits)
			def := into()
			if err := canxml.Unmarshal(in, def); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", in, err)
			}
			got, reported, err := decodeXMLPresence(into, in)
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithPresence) = %v; want nil: the option reports, never refuses", err)
			}
			if !slices.Equal(reported, row.want) {
				t.Errorf("WithPresence reported %v; want %v", reported, row.want)
			}
			if !reflect.DeepEqual(got, def) {
				t.Errorf("canxml.Unmarshal(WithPresence) decoded\n%#v\nwant the default's value\n%#v", got, def)
			}

			_, missing, err := decodeXMLMissing(into, in)
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithMissing) = %v; want nil", err)
			}
			if !slices.Equal(missing, row.missing) {
				t.Errorf("WithMissing reported %q; want %q", missing, row.missing)
			}

			var both []string
			_, beside, err := decodeXMLPresence(into, in, canxml.WithMissing(func(p string) { both = append(both, p) }), canxml.WithStrict())
			if err != nil && !errors.Is(err, canxml.ErrUnknownElement) {
				t.Fatalf("canxml.Unmarshal(WithPresence, WithMissing, WithStrict) = %v; want nil, or the refusal of unmatched content", err)
			}
			if err == nil {
				if !slices.Equal(beside, row.want) {
					t.Errorf("WithPresence beside WithMissing and WithStrict reported %v; want %v", beside, row.want)
				}
				if !slices.Equal(both, row.missing) {
					t.Errorf("WithMissing beside WithPresence and WithStrict reported %q; want %q", both, row.missing)
				}
			}
		})
	}
}

// TestREQ175_XMLPresenceIsNeverAStrictRefusal pins that strict refuses no
// member sent without a value, mandatory or optional, nilled or empty.
func TestREQ175_XMLPresenceIsNeverAStrictRefusal(t *testing.T) {
	in := rewriteXML(t, missingCompositionXML, map[string]string{
		pxQuantity + "/units":     `<units></units><precision xsi:nil="true"/>`,
		pxQuantity + "/magnitude": `<magnitude></magnitude>`,
		mxEntry + "/subject":      `<subject xsi:type="PARTY_SELF" xsi:nil="true"/>`,
	})
	_, reported, err := decodeXMLPresence(func() any { return new(rm.Composition) }, in, canxml.WithStrict())
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithPresence, WithStrict) = %v; want nil: strict refuses unmatched content only", err)
	}
	want := []sent{sentNil(mxEntry + "/subject"), sentEmpty(pxQuantity + "/magnitude"), sentEmpty(pxQuantity + "/units"), sentNil(pxQuantity + "/precision")}
	if !slices.Equal(reported, want) {
		t.Errorf("WithPresence under WithStrict reported %v; want %v", reported, want)
	}
}

// TestREQ175_XMLPresenceHashEntries pins that an entry of a Hash member is a
// value whatever its text, as for WithMissing; that a member inside the value
// of an entry is named under the entry's key, which lies inside the value,
// even when the member comes before the element that carries the key; and
// that the content of an entry repeating an earlier entry's key counts for
// nothing, as it does for WithMissing: its empty `copyright` is not reported.
func TestREQ175_XMLPresenceHashEntries(t *testing.T) {
	language := func(code string) string {
		return `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>` + code + `</code_string></language>`
	}
	in := []byte(`<description ` + ucNS + `>` +
		`<original_author id="name"></original_author>` +
		`<lifecycle_state>published</lifecycle_state>` +
		`<other_details id="k"></other_details>` +
		`<details xsi:type="RESOURCE_DESCRIPTION_ITEM">` + language("en") + `<purpose>p</purpose><use></use></details>` +
		`<details xsi:type="RESOURCE_DESCRIPTION_ITEM">` + language("en") + `<purpose>p</purpose><copyright></copyright></details>` +
		`<details xsi:type="RESOURCE_DESCRIPTION_ITEM"><use></use><misuse xsi:nil="true"/>` + language("fr") + `<purpose>p</purpose></details>` +
		`</description>`)
	_, reported, err := decodeXMLPresence(func() any { return new(rm.ResourceDescription) }, in)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithPresence) = %v; want nil", err)
	}
	want := []sent{sentEmpty("/details/en/use"), sentEmpty("/details/fr/use"), sentNil("/details/fr/misuse")}
	if !slices.Equal(reported, want) {
		t.Errorf("WithPresence reported %v; want %v: no Hash entry, and the member of an entry's value under its key", reported, want)
	}
}

// TestREQ175_XMLPresenceOctetList pins the pointer of a DV_MULTIMEDIA `data`
// or `integrity_check` sent without a value: an Octet list is one element
// whose text is base64, and one string in canonical JSON, so its pointer
// takes no index.
func TestREQ175_XMLPresenceOctetList(t *testing.T) {
	multimedia := func(inner string) []byte {
		return []byte(`<dv_multimedia ` + ucNS + `>` + inner +
			`<media_type><terminology_id><value>IANA_media-types</value></terminology_id><code_string>image/png</code_string></media_type>` +
			`<size>0</size></dv_multimedia>`)
	}
	for _, c := range []struct {
		name string
		in   []byte
		want []sent
	}{
		{name: "empty data and integrity_check", in: multimedia(`<data></data><integrity_check></integrity_check>`), want: []sent{sentEmpty("/data"), sentEmpty("/integrity_check")}},
		{name: "nilled data", in: multimedia(`<data xsi:nil="true"/>`), want: []sent{sentNil("/data")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, reported, err := decodeXMLPresence(func() any { return new(rm.DVMultimedia) }, c.in)
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithPresence) = %v; want nil", err)
			}
			if !slices.Equal(reported, c.want) {
				t.Errorf("WithPresence reported %v; want %v", reported, c.want)
			}
		})
	}
}

// TestREQ175_XMLPresenceAOM14 pins the report on an AOM 1.4 class, which the
// generated decoders read too, and that an element the schema accepts and no
// member holds, dropped when its text is "false", is no member: a
// C_MULTIPLE_ATTRIBUTE's `match_negated`.
func TestREQ175_XMLPresenceAOM14(t *testing.T) {
	in := []byte(`<attribute ` + ucNS + `><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>` +
		`<match_negated xsi:nil="true">false</match_negated>` +
		`<cardinality><is_ordered></is_ordered><is_unique>false</is_unique>` +
		`<interval><lower_included>true</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>` +
		`</attribute>`)
	_, reported, err := decodeXMLPresence(func() any { return new(aom14.CMultipleAttribute) }, in)
	if err != nil {
		t.Fatalf("canxml.Unmarshal(WithPresence) = %v; want nil", err)
	}
	if want := []sent{sentEmpty("/cardinality/is_ordered")}; !slices.Equal(reported, want) {
		t.Errorf("WithPresence reported %v; want %v", reported, want)
	}
}

// TestREQ175_XMLPresenceDecoderReportsPerValue pins the option on a
// canxml.Decoder: each value's pointers are relative to that value.
func TestREQ175_XMLPresenceDecoderReportsPerValue(t *testing.T) {
	stream := `<dv_text ` + ucNS + `><value></value></dv_text> <dv_quantity ` + ucNS + `><magnitude>1</magnitude><units>kg</units><precision xsi:nil="true"/></dv_quantity>`
	var got []sent
	dec := canxml.NewDecoder(strings.NewReader(stream), canxml.WithPresence(func(p string, s canxml.Presence) { got = append(got, sent{p, s}) }))
	if err := dec.Decode(new(rm.DVText)); err != nil {
		t.Fatalf("Decode(first value) = %v; want nil", err)
	}
	if want := []sent{sentEmpty("/value")}; !slices.Equal(got, want) {
		t.Errorf("Decoder reported %v after the first value; want %v", got, want)
	}
	got = nil
	if err := dec.Decode(new(rm.DVQuantity)); err != nil {
		t.Fatalf("Decode(second value) = %v; want nil", err)
	}
	if want := []sent{sentNil("/precision")}; !slices.Equal(got, want) {
		t.Errorf("Decoder reported %v after the second value; want %v, relative to that value", got, want)
	}
	if err := dec.Decode(new(rm.DVText)); !errors.Is(err, io.EOF) {
		t.Errorf("Decode(past the end) = %v; want io.EOF", err)
	}
}

// TestREQ175_XMLNilPresenceIsNoOption pins that WithPresence(nil) is the same
// as leaving the option out: a Decoder given it is untracked, so a primitive
// failure names the element alone, as with no option, where a Decoder given a
// function names it by its path from the root of the value.
func TestREQ175_XMLNilPresenceIsNoOption(t *testing.T) {
	bad := []byte(`<dv_quantity ` + ucNS + `><magnitude>` + xmlPlanted + `</magnitude><units>kg</units></dv_quantity>`)
	decode := func(opts ...canxml.DecoderOption) string {
		t.Helper()
		err := canxml.NewDecoder(bytes.NewReader(bad), opts...).Decode(new(rm.DVQuantity))
		if err == nil {
			t.Fatalf("Decoder.Decode(%s) = nil; want a primitive failure", bad)
		}
		return err.Error()
	}
	plain := decode()
	if tracked := decode(canxml.WithPresence(func(string, canxml.Presence) {})); tracked == plain {
		t.Fatalf("Decoder(WithPresence(fn)).Decode = %q, the plain decoder's; want the tracked decode's path", tracked)
	}
	if got := decode(canxml.WithPresence(nil)); got != plain {
		t.Errorf("Decoder(WithPresence(nil)).Decode = %q; want the plain decoder's %q", got, plain)
	}
}

// TestREQ175_XMLPresenceString pins the names the two forms print as.
func TestREQ175_XMLPresenceString(t *testing.T) {
	for p, want := range map[canxml.Presence]string{canxml.PresenceNil: "nil", canxml.PresenceEmpty: "empty", canxml.Presence(0): "Presence(0)"} {
		if got := p.String(); got != want {
			t.Errorf("Presence(%d).String() = %q; want %q", int(p), got, want)
		}
	}
}

// TestREQ175_XMLPresenceTerminologyCode pins the report on the hand-written
// TERMINOLOGY_CODE decoder: an empty `terminology_id`, in its text shape and
// in its TERMINOLOGY_ID shape with an empty `value`, is reported empty once;
// a nilled one is reported nil once; one with an identifier is not. Its
// optional `terminology_version` and `uri` sent without a value are reported
// and decode as absent (REQ-056).
func TestREQ175_XMLPresenceTerminologyCode(t *testing.T) {
	code := func(id, rest string) string {
		return `<terminology_code ` + ucNS + `>` + id + `<code_string>1</code_string>` + rest + `</terminology_code>`
	}
	for _, c := range []struct {
		name string
		in   string
		want []sent
	}{
		{name: "an identifier as text", in: code(`<terminology_id>snomed_ct</terminology_id>`, "")},
		{name: "an identifier in a value child", in: code(`<terminology_id><value>snomed_ct</value></terminology_id>`, "")},
		{name: "empty text", in: code(`<terminology_id></terminology_id>`, ""), want: []sent{sentEmpty("/terminology_id")}},
		{name: "whitespace", in: code("<terminology_id> \n</terminology_id>", ""), want: []sent{sentEmpty("/terminology_id")}},
		{name: "an empty value child", in: code(`<terminology_id><value></value></terminology_id>`, ""), want: []sent{sentEmpty("/terminology_id")}},
		{name: "nilled", in: code(`<terminology_id xsi:nil="true"/>`, ""), want: []sent{sentNil("/terminology_id")}},
		{
			name: "optional members without a value",
			in:   code(`<terminology_id>snomed_ct</terminology_id>`, `<terminology_version></terminology_version><uri xsi:nil="true"/>`),
			want: []sent{sentEmpty("/terminology_version"), sentNil("/uri")},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, reported, err := decodeXMLPresence(func() any { return new(rm.TerminologyCode) }, []byte(c.in))
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithPresence) = %v; want nil", err)
			}
			if !slices.Equal(reported, c.want) {
				t.Errorf("WithPresence reported %v; want %v", reported, c.want)
			}
			tc := got.(*rm.TerminologyCode)
			if tc.TerminologyVersion != nil || tc.URI != nil {
				t.Errorf("TerminologyVersion = %v, URI = %v; want both nil", tc.TerminologyVersion, tc.URI)
			}
			var def rm.TerminologyCode
			if err := canxml.Unmarshal([]byte(c.in), &def); err != nil {
				t.Fatalf("canxml.Unmarshal = %v; want nil", err)
			}
			if !reflect.DeepEqual(*tc, def) {
				t.Errorf("canxml.Unmarshal(WithPresence) decoded %+v; want the default's %+v", *tc, def)
			}
		})
	}
}
