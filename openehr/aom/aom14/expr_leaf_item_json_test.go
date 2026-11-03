package aom14_test

// REQ-176, REQ-052: canonical JSON of the EXPR_LEAF item, a BMM `Any`: a class
// value carries its `_type` and decodes as the class it names; an item the XML
// gave an XML Schema built-in type is an object whose `_type` is the BMM
// primitive the built-in reads as and whose `value` is its text as that
// primitive's JSON value, which decodes back under `xsd:` and the one built-in
// canonical for the primitive; and a plain String is a JSON string.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ176_REQ052_ExprLeafItemCanonicalJSON pins the canonical JSON of each
// value an EXPR_LEAF item holds and its decode back: a *aom14.CString as an
// object with `_type` C_STRING, and a Go string as a JSON string, each back as
// written; a *aom14.TypedString as an object whose `_type` is its primitive
// and whose `value` is a JSON string for a String, a Uri and the ISO 8601
// primitives, a JSON boolean for a Boolean, written `1` or with white space
// around it as XML Schema reads one, and a JSON number for the integer and
// real primitives, back with its text as that JSON value and its Type as
// `xsd:` and the built-in canonical for the primitive, so an `xs:string` item
// comes back as `xsd:string` and an `xs:decimal` one as `xsd:float`. A Real
// or a Double keeps its text's own digits, none of them passed through a
// float64, so a literal whose digits a float64 does not keep, such as
// `0.1000000000000000055511151231257827`, which a float64 writes as 0.1, or
// `1e-400`, which it reads as zero, is written as it is. The text is rewritten
// only where JSON's number grammar differs from XML Schema's: a leading `+`
// dropped, `0` before a leading point, a trailing point dropped and leading
// zeros dropped.
func TestREQ176_REQ052_ExprLeafItemCanonicalJSON(t *testing.T) {
	pattern := ".*"
	typed := func(typ, value string) *aom14.TypedString { return &aom14.TypedString{Value: value, Type: typ} }
	for _, tc := range []struct {
		name string
		item any
		want string // the item's canonical JSON
		back any    // the item decoded from that JSON
	}{
		{"C_STRING", &aom14.CString{Pattern: &pattern}, `{"_type":"C_STRING","pattern":".*"}`, &aom14.CString{Pattern: &pattern}},
		{"xs:string", typed("xs:string", "archetype_id/value"), `{"_type":"String","value":"archetype_id/value"}`, typed("xsd:string", "archetype_id/value")},
		{"xsd:string", typed("xsd:string", "archetype_id/value"), `{"_type":"String","value":"archetype_id/value"}`, typed("xsd:string", "archetype_id/value")},
		{"xs:token", typed("xs:token", "at0001"), `{"_type":"String","value":"at0001"}`, typed("xsd:string", "at0001")},
		{"xs:anyURI", typed("xs:anyURI", "http://example.org/a"), `{"_type":"Uri","value":"http://example.org/a"}`, typed("xsd:anyURI", "http://example.org/a")},
		{"xs:boolean true", typed("xs:boolean", "true"), `{"_type":"Boolean","value":true}`, typed("xsd:boolean", "true")},
		{"xs:boolean 1", typed("xs:boolean", " 1\n"), `{"_type":"Boolean","value":true}`, typed("xsd:boolean", "true")},
		{"xs:boolean 0", typed("xs:boolean", "0"), `{"_type":"Boolean","value":false}`, typed("xsd:boolean", "false")},
		{"xs:int", typed("xs:int", "-42"), `{"_type":"Integer","value":-42}`, typed("xsd:int", "-42")},
		{"xs:integer", typed("xs:integer", " +7 "), `{"_type":"Integer","value":7}`, typed("xsd:int", "7")},
		{"xs:long", typed("xs:long", "9223372036854775807"), `{"_type":"Integer64","value":9223372036854775807}`, typed("xsd:long", "9223372036854775807")},
		{"xs:decimal", typed("xs:decimal", "2.50"), `{"_type":"Real","value":2.50}`, typed("xsd:float", "2.50")},
		{"xs:double", typed("xs:double", "1e3"), `{"_type":"Double","value":1e3}`, typed("xsd:double", "1e3")},
		{"xs:double exact", typed("xs:double", "0.1000000000000000055511151231257827"), `{"_type":"Double","value":0.1000000000000000055511151231257827}`, typed("xsd:double", "0.1000000000000000055511151231257827")},
		{"xs:float underflow", typed("xs:float", "1e-400"), `{"_type":"Real","value":1e-400}`, typed("xsd:float", "1e-400")},
		{"xs:float plus", typed("xs:float", "+1.5"), `{"_type":"Real","value":1.5}`, typed("xsd:float", "1.5")},
		{"xs:decimal leading point", typed("xs:decimal", ".5"), `{"_type":"Real","value":0.5}`, typed("xsd:float", "0.5")},
		{"xs:decimal signed leading point", typed("xs:decimal", "-.5"), `{"_type":"Real","value":-0.5}`, typed("xsd:float", "-0.5")},
		{"xs:decimal trailing point", typed("xs:decimal", "1."), `{"_type":"Real","value":1}`, typed("xsd:float", "1")},
		{"xs:double trailing point", typed("xs:double", "1.E3"), `{"_type":"Double","value":1E3}`, typed("xsd:double", "1E3")},
		{"xs:decimal leading zeros", typed("xs:decimal", "-007.50"), `{"_type":"Real","value":-7.50}`, typed("xsd:float", "-7.50")},
		{"xs:double zeros", typed("xs:double", "\t+00.0e+01 "), `{"_type":"Double","value":0.0e+01}`, typed("xsd:double", "0.0e+01")},
		{"xs:date", typed("xs:date", "2026-11-03"), `{"_type":"Iso8601_date","value":"2026-11-03"}`, typed("xsd:date", "2026-11-03")},
		{"xs:time", typed("xs:time", "10:30:00"), `{"_type":"Iso8601_time","value":"10:30:00"}`, typed("xsd:time", "10:30:00")},
		{"xs:dateTime", typed("xs:dateTime", "2026-11-03T10:30:00Z"), `{"_type":"Iso8601_date_time","value":"2026-11-03T10:30:00Z"}`, typed("xsd:dateTime", "2026-11-03T10:30:00Z")},
		{"xs:duration", typed("xs:duration", "P1D"), `{"_type":"Iso8601_duration","value":"P1D"}`, typed("xsd:duration", "P1D")},
		{"no xsi:type", "archetype_id/value", `"archetype_id/value"`, "archetype_id/value"},
	} {
		leaf := &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: tc.item}
		b, err := canjson.Marshal(leaf)
		if err != nil {
			t.Fatalf("%s: canjson.Marshal = %v", tc.name, err)
		}
		var doc struct {
			Item jsontext.Value `json:"item"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: reading the canonical JSON: %v", tc.name, err)
		}
		if got := string(doc.Item); got != tc.want {
			t.Errorf("%s: item written as %s; want %s", tc.name, got, tc.want)
		}
		var back aom14.ExprLeaf
		if err := canjson.Unmarshal(b, &back, canjson.WithStrict()); err != nil {
			t.Fatalf("%s: canjson.Unmarshal = %v", tc.name, err)
		}
		if want := (&aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: tc.back}); !reflect.DeepEqual(&back, want) {
			t.Errorf("%s: decoded back as %#v; want %#v", tc.name, back.Item, tc.back)
		}
	}
}

// TestREQ176_REQ052_ExprLeafItemCanonicalJSONRefusals pins what an EXPR_LEAF
// item's canonical JSON may not hold. A number, a boolean or an array, which
// no item reads as, is a shape failure. So is an object whose `_type` names a
// primitive and whose `value` is missing, null, of another JSON kind than the
// primitive's, or a number the primitive cannot hold, wherever `_type` stands.
// An object with no `_type`, the `xsi_type` form among them, is a missing
// type, and a `_type` the registry does not hold, a built-in's own name among
// them, an unknown type. A null item is nil.
func TestREQ176_REQ052_ExprLeafItemCanonicalJSONRefusals(t *testing.T) {
	leaf := func(item string) []byte {
		return []byte(`{"_type":"EXPR_LEAF","type":"String","item":` + item + `,"reference_type":"attribute"}`)
	}
	for _, item := range []string{
		`1`, `true`, `["a"]`,
		`{"_type":"String"}`,
		`{"_type":"String","value":null}`,
		`{"_type":"String","value":1}`,
		`{"value":1,"_type":"String"}`,
		`{"_type":"Uri","value":true}`,
		`{"_type":"Iso8601_date","value":["2026-11-03"]}`,
		`{"_type":"Boolean","value":"true"}`,
		`{"_type":"Boolean","value":1}`,
		`{"_type":"Integer","value":"1"}`,
		`{"_type":"Integer","value":2.5}`,
		`{"_type":"Integer","value":2147483648}`,
		`{"_type":"Integer64","value":9223372036854775808}`,
		`{"_type":"Real","value":"2.5"}`,
		`{"_type":"Double","value":1e400}`,
	} {
		err := canjson.Unmarshal(leaf(item), new(aom14.ExprLeaf))
		if !errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("canjson.Unmarshal(item %s) = %v; want canjson.ErrInvalidShape", item, err)
		}
	}
	for _, item := range []string{`{"value":"a","xsi_type":"xs:string"}`, `{"value":"a","xsi_type":"string"}`, `{"value":"a"}`} {
		if err := canjson.Unmarshal(leaf(item), new(aom14.ExprLeaf)); !errors.Is(err, typereg.ErrMissingType) {
			t.Errorf("canjson.Unmarshal(item %s) = %v; want typereg.ErrMissingType", item, err)
		}
	}
	for _, item := range []string{`{"_type":"NO_SUCH_TYPE"}`, `{"_type":"string","value":"a"}`} {
		if err := canjson.Unmarshal(leaf(item), new(aom14.ExprLeaf)); !errors.Is(err, typereg.ErrUnknownType) {
			t.Errorf("canjson.Unmarshal(item %s) = %v; want typereg.ErrUnknownType", item, err)
		}
	}
	var null aom14.ExprLeaf
	if err := canjson.Unmarshal(leaf(`null`), &null); err != nil || null.Item != nil {
		t.Errorf("canjson.Unmarshal(null item) = %v, item %#v; want nil, nil", err, null.Item)
	}
}

// TestREQ176_REQ052_ExprLeafItemCanonicalJSONEncodeRefusals pins what canonical
// JSON refuses to write for a *aom14.TypedString item, with
// canjson.ErrInvalidValue and an error that does not quote the text: a Type
// that is no prefixed XML Schema built-in datatype, and text the built-in's
// primitive does not read: a Boolean other than true, false, 1 or 0; an
// Integer or Integer64 that is no whole number in its range; and a Real or
// Double that is no decimal number in the float64 range, an infinity and a
// not-a-number among them. Where the text is refused for its spelling or its
// range, the refusal wraps strconv.ErrSyntax or strconv.ErrRange, so a Real
// or a Double that no XML Schema literal spells, such as a hexadecimal float
// or digits with an underscore, is refused before it is written.
func TestREQ176_REQ052_ExprLeafItemCanonicalJSONEncodeRefusals(t *testing.T) {
	for _, tc := range []struct {
		item  aom14.TypedString
		cause error // the strconv sentinel the refusal wraps, or nil
	}{
		{item: aom14.TypedString{Type: "string", Value: "unprefixed"}},
		{item: aom14.TypedString{Type: "xs:nothing", Value: "unknown"}},
		{item: aom14.TypedString{Type: "xs:boolean", Value: "yes"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:boolean", Value: ""}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:int", Value: "abc"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:int", Value: "1.0"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:int", Value: "2147483648"}, cause: strconv.ErrRange},
		{item: aom14.TypedString{Type: "xs:long", Value: "9223372036854775808"}, cause: strconv.ErrRange},
		{item: aom14.TypedString{Type: "xs:decimal", Value: "2.5.1"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:float", Value: "0x1p-2"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "1_000"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "1e400"}, cause: strconv.ErrRange},
		{item: aom14.TypedString{Type: "xs:double", Value: "."}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "-"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "1e"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "e3"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "1e+"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "+-1"}, cause: strconv.ErrSyntax},
		{item: aom14.TypedString{Type: "xs:double", Value: "INF"}},
		{item: aom14.TypedString{Type: "xs:double", Value: "-INF"}},
		{item: aom14.TypedString{Type: "xs:decimal", Value: "NaN"}},
	} {
		leaf := &aom14.ExprLeaf{Type: "String", ReferenceType: "attribute", Item: &tc.item}
		_, err := canjson.Marshal(leaf)
		if !errors.Is(err, canjson.ErrInvalidValue) {
			t.Errorf("canjson.Marshal(item %#v) = %v; want canjson.ErrInvalidValue", tc.item, err)
			continue
		}
		if tc.cause != nil && !errors.Is(err, tc.cause) {
			t.Errorf("canjson.Marshal(item %#v) = %v; want it to wrap %v", tc.item, err, tc.cause)
		}
		if len(tc.item.Value) > 2 && strings.Contains(err.Error(), tc.item.Value) {
			t.Errorf("canjson.Marshal(item %#v) = %q; want an error that does not quote the text", tc.item, err)
		}
	}
}
