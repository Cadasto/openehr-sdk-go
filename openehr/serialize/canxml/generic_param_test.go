package canxml_test

// generic_param_test.go: REQ-056 for a member typed by an open generic
// parameter, such as the `data` of ORIGINAL_VERSION and the bounds of BASE
// Interval. The encoder writes a class value there with its `xsi:type` as
// the first attribute, whatever the Go instantiation, so the decoder reads
// back what the encoder wrote; a primitive value keeps its plain text.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xsiNamespace is the XML Schema instance namespace `xsi:type` belongs to.
const xsiNamespace = "http://www.w3.org/2001/XMLSchema-instance"

// firstElement returns the start tag of the first element named local in b,
// at any depth below the root, and whether there is one.
func firstElement(t *testing.T, b []byte, local string) (xml.StartElement, bool) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return xml.StartElement{}, false
		}
		if err != nil {
			t.Fatalf("read XML token: %v: %s", err, b)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > 1 && el.Name.Local == local {
				return el, true
			}
		case xml.EndElement:
			depth--
		}
	}
}

// xsiTypeOf returns the value of start's `xsi:type` attribute, and whether
// it is start's first attribute; an element with none returns "".
func xsiTypeOf(start xml.StartElement) (string, bool) {
	for i, a := range start.Attr {
		if a.Name.Space == xsiNamespace && a.Name.Local == "type" {
			return a.Value, i == 0
		}
	}
	return "", false
}

// originalVersionOf returns an ORIGINAL_VERSION holding data, with the
// members the codec needs.
func originalVersionOf[T any](data T) *rm.OriginalVersion[T] {
	return &rm.OriginalVersion[T]{
		UID:            rm.ObjectVersionID{Value: "8849182c-82ad-4088-a07f-48ead4180515::cdr.example.com::1"},
		Data:           &data,
		LifecycleState: codedText("complete", "openehr", "532"),
	}
}

// genericParamPerson returns the PERSON of the corpus ORIGINAL_VERSION twin.
func genericParamPerson() *rm.Person {
	return &rm.Person{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1",
		Name:            &rm.DVText{Value: "Jane Doe"},
	}
}

// genericParamComposition returns a COMPOSITION with the members the codec
// needs.
func genericParamComposition() *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            &rm.DVText{Value: "Encounter"},
		Language:        rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: "en"},
		Territory:       rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}, CodeString: "GB"},
		Category:        codedText("event", "openehr", "433"),
		Composer:        &rm.PartySelf{},
	}
}

// TestREQ056_XMLGenericParamClassValueRoundTrips pins a member typed by an
// open generic parameter. A class value there is written with its `xsi:type`
// as the element's first attribute, whether the Go instantiation is an
// interface (ORIGINAL_VERSION<PARTY>, ORIGINAL_VERSION<any>, Proper_interval
// of DV_ORDERED) or the class itself (ORIGINAL_VERSION<COMPOSITION>,
// Point_interval of DV_QUANTITY), and the value decodes back, strictly, to
// the same canonical JSON. A primitive value is written as its text, with no
// `xsi:type`.
//
// Can-fail control: before the encoder wrote `xsi:type` there, the
// interface instantiations failed to decode with typereg.ErrMissingType and
// the concrete ones were written with no `xsi:type`.
func TestREQ056_XMLGenericParamClassValueRoundTrips(t *testing.T) {
	cases := []struct {
		name  string
		value any
		fresh func() any
		elem  string
		want  string // the xsi:type of elem, or "" for none
	}{
		{
			name:  "ORIGINAL_VERSION<PARTY> holding a PERSON",
			value: originalVersionOf[rm.Party](genericParamPerson()),
			fresh: func() any { return new(rm.OriginalVersion[rm.Party]) },
			elem:  "data",
			want:  "PERSON",
		},
		{
			name:  "ORIGINAL_VERSION<any> holding a PERSON",
			value: originalVersionOf[any](genericParamPerson()),
			fresh: func() any { return new(rm.OriginalVersion[any]) },
			elem:  "data",
			want:  "PERSON",
		},
		{
			name:  "ORIGINAL_VERSION<COMPOSITION>",
			value: originalVersionOf(*genericParamComposition()),
			fresh: func() any { return new(rm.OriginalVersion[rm.Composition]) },
			elem:  "data",
			want:  "COMPOSITION",
		},
		{
			name:  "ORIGINAL_VERSION<*COMPOSITION>",
			value: originalVersionOf(genericParamComposition()),
			fresh: func() any { return new(rm.OriginalVersion[*rm.Composition]) },
			elem:  "data",
			want:  "COMPOSITION",
		},
		{
			name: "IMPORTED_VERSION whose item holds a PERSON",
			value: &rm.ImportedVersion[rm.Party]{
				Item: *originalVersionOf[any](genericParamPerson()),
			},
			fresh: func() any { return new(rm.ImportedVersion[rm.Party]) },
			elem:  "data",
			want:  "PERSON",
		},
		{
			name: "Proper_interval of DV_ORDERED",
			value: &rm.ProperInterval[rm.DVOrdered]{
				Lower: &rm.DVQuantity{Magnitude: 1.5, Units: "mm"}, LowerIncluded: true,
				Upper: &rm.DVQuantity{Magnitude: 3, Units: "mm"}, UpperIncluded: true,
			},
			fresh: func() any { return new(rm.ProperInterval[rm.DVOrdered]) },
			elem:  "lower",
			want:  "DV_QUANTITY",
		},
		{
			name: "Point_interval of DV_QUANTITY",
			value: &rm.PointInterval[rm.DVQuantity]{
				Lower: rm.DVQuantity{Magnitude: 2, Units: "mm"}, LowerIncluded: true,
				Upper: rm.DVQuantity{Magnitude: 2, Units: "mm"}, UpperIncluded: true,
			},
			fresh: func() any { return new(rm.PointInterval[rm.DVQuantity]) },
			elem:  "upper",
			want:  "DV_QUANTITY",
		},
		{
			name:  "Proper_interval of Integer keeps a primitive's text",
			value: &rm.ProperInterval[rm.Integer]{Lower: 1, LowerIncluded: true, Upper: 5, UpperIncluded: true},
			fresh: func() any { return new(rm.ProperInterval[rm.Integer]) },
			elem:  "lower",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := canxml.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canxml.Marshal(%T) = %v; want nil", tc.value, err)
			}
			start, ok := firstElement(t, b, tc.elem)
			if !ok {
				t.Fatalf("canxml.Marshal(%T) wrote no %s element:\n%s", tc.value, tc.elem, b)
			}
			got, first := xsiTypeOf(start)
			if got != tc.want || (tc.want != "" && !first) {
				t.Errorf("canxml.Marshal(%T): %s has xsi:type %q (first attribute: %t); want %q as its first attribute, or none for \"\":\n%s",
					tc.value, tc.elem, got, first, tc.want, b)
			}

			back := tc.fresh()
			if err := canxml.Unmarshal(b, back, canxml.WithStrict()); err != nil {
				t.Fatalf("canxml.Unmarshal(WithStrict) of what canxml.Marshal(%T) wrote = %v; want nil:\n%s", tc.value, err, b)
			}
			wantJSON, err := canjson.Marshal(tc.value)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T) = %v", tc.value, err)
			}
			gotJSON, err := canjson.Marshal(back)
			if err != nil {
				t.Fatalf("canjson.Marshal(decoded %T) = %v", back, err)
			}
			if !bytes.Equal(gotJSON, wantJSON) {
				t.Errorf("XML round trip of %T:\n got %s\nwant %s\nwire: %s", tc.value, gotJSON, wantJSON, b)
			}
		})
	}
}
