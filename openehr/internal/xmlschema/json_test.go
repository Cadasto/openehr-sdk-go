package xmlschema_test

// REQ-176, REQ-052: the canonical-JSON decoders of a BMM `Any` member, and of
// one whose canonical XML takes a choice of shapes, which the generated
// decoders set in the place of the member: a mandatory one held as a Go any
// and an optional one held as a *any.

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ176_REQ052_AnyJSONDecoders pins what the two decoders store for each
// JSON value an `Any` member holds: a class value for an object whose `_type`
// names a class, a *TypedString of the `xsd:` built-in canonical for the
// primitive an object's `_type` names, whether `_type` comes first or not, a
// Go string for a string, and nothing for null, the optional member's pointer
// left nil.
func TestREQ176_REQ052_AnyJSONDecoders(t *testing.T) {
	for _, tc := range []struct {
		json string
		want any
	}{
		{`{"_type":"DV_BOOLEAN","value":true}`, &rm.DVBoolean{Value: true}},
		{`{"value":true,"_type":"DV_BOOLEAN"}`, &rm.DVBoolean{Value: true}},
		{`{"_type":"String","value":"a"}`, &xmlschema.TypedString{Value: "a", Type: "xsd:string"}},
		{`{"value":"a","_type":"String"}`, &xmlschema.TypedString{Value: "a", Type: "xsd:string"}},
		{` { "_type" : "Boolean" , "value" : false } `, &xmlschema.TypedString{Value: "false", Type: "xsd:boolean"}},
		{`"a"`, "a"},
		{`null`, nil},
	} {
		var mandatory any = "held before"
		w := struct {
			X xmlschema.AnyJSON `json:"x"`
		}{X: xmlschema.AnyJSON{To: &mandatory}}
		if err := json.Unmarshal([]byte(`{"x":`+tc.json+`}`), &w, typereg.Unmarshalers()); err != nil {
			t.Fatalf("AnyJSON(%s) = %v", tc.json, err)
		}
		if !reflect.DeepEqual(mandatory, tc.want) {
			t.Errorf("AnyJSON(%s) stored %#v; want %#v", tc.json, mandatory, tc.want)
		}

		held := any("held before")
		optional := &held
		o := struct {
			X xmlschema.OptionalAnyJSON `json:"x"`
		}{X: xmlschema.OptionalAnyJSON{To: &optional}}
		if err := json.Unmarshal([]byte(`{"x":`+tc.json+`}`), &o, typereg.Unmarshalers()); err != nil {
			t.Fatalf("OptionalAnyJSON(%s) = %v", tc.json, err)
		}
		switch {
		case tc.want == nil && optional != nil:
			t.Errorf("OptionalAnyJSON(null) left %#v; want a nil pointer", *optional)
		case tc.want != nil && (optional == nil || !reflect.DeepEqual(*optional, tc.want)):
			t.Errorf("OptionalAnyJSON(%s) stored %v; want a pointer to %#v", tc.json, optional, tc.want)
		}
	}
}

// TestREQ176_REQ052_ChoiceJSONDecoders pins what the two decoders of a member
// whose canonical XML takes a choice of shapes store: the class an object's
// `_type` names when it is one of the shapes, and nothing for null, the
// optional member's pointer left nil; a class that is none of the shapes is
// refused as a type mismatch.
func TestREQ176_REQ052_ChoiceJSONDecoders(t *testing.T) {
	types := []string{"DV_BOOLEAN"}
	for _, tc := range []struct {
		json string
		want any
	}{
		{`{"_type":"DV_BOOLEAN","value":true}`, &rm.DVBoolean{Value: true}},
		{`null`, nil},
	} {
		var mandatory any = "held before"
		w := struct {
			X xmlschema.ChoiceJSON `json:"x"`
		}{X: xmlschema.ChoiceJSON{To: &mandatory, Types: types}}
		if err := json.Unmarshal([]byte(`{"x":`+tc.json+`}`), &w, typereg.Unmarshalers()); err != nil {
			t.Fatalf("ChoiceJSON(%s) = %v", tc.json, err)
		}
		if !reflect.DeepEqual(mandatory, tc.want) {
			t.Errorf("ChoiceJSON(%s) stored %#v; want %#v", tc.json, mandatory, tc.want)
		}

		held := any("held before")
		optional := &held
		o := struct {
			X xmlschema.OptionalChoiceJSON `json:"x"`
		}{X: xmlschema.OptionalChoiceJSON{To: &optional, Types: types}}
		if err := json.Unmarshal([]byte(`{"x":`+tc.json+`}`), &o, typereg.Unmarshalers()); err != nil {
			t.Fatalf("OptionalChoiceJSON(%s) = %v", tc.json, err)
		}
		switch {
		case tc.want == nil && optional != nil:
			t.Errorf("OptionalChoiceJSON(null) left %#v; want a nil pointer", *optional)
		case tc.want != nil && (optional == nil || !reflect.DeepEqual(*optional, tc.want)):
			t.Errorf("OptionalChoiceJSON(%s) stored %v; want a pointer to %#v", tc.json, optional, tc.want)
		}
	}
	var v any
	w := struct {
		X xmlschema.ChoiceJSON `json:"x"`
	}{X: xmlschema.ChoiceJSON{To: &v, Types: types}}
	err := json.Unmarshal([]byte(`{"x":{"_type":"DV_TEXT","value":"a"}}`), &w, typereg.Unmarshalers())
	if !errors.Is(err, typereg.ErrTypeMismatch) {
		t.Errorf("ChoiceJSON(a class none of the shapes) = %v; want typereg.ErrTypeMismatch", err)
	}
}

// TestREQ176_REQ052_JSONRefusalsKeepTheirText pins the value-free text of the
// decode refusals the canonical-JSON decoders raise themselves, each of which
// has a fixed text that holds no value: the text of the DecodeError
// canjson.Unmarshal returns keeps that text rather than naming the refusal by
// its Go type alone, and holds none of the input's values.
//
// Can-fail control: drop a refusal's text from the forms the value-free
// renderer keeps, and its case reads "cause withheld (*errors.errorString)".
func TestREQ176_REQ052_JSONRefusalsKeepTheirText(t *testing.T) {
	type anyMember struct {
		X xmlschema.AnyJSON `json:"x"`
	}
	type choiceMember struct {
		X xmlschema.ChoiceJSON `json:"x"`
	}
	for _, tc := range []struct {
		name   string
		json   string
		target func() any
		keep   string
		absent string
	}{
		{
			name: "a typed item with no value", json: `{"_type":"String"}`,
			target: func() any { return new(xmlschema.TypedString) },
			keep:   "a typed item holds no value",
		},
		{
			name: "a typed item with a null value", json: `{"_type":"String","value":null}`,
			target: func() any { return new(xmlschema.TypedString) },
			keep:   "a typed item holds no value",
		},
		{
			name: "a typed item whose _type names no primitive", json: `{"_type":"Jane","value":"Doe"}`,
			target: func() any { return new(xmlschema.TypedString) },
			keep:   "the _type of a typed item names no primitive an XML Schema built-in datatype reads as",
			absent: "Doe",
		},
		{
			name: "an Any member holding a number", json: `{"x":98765.4321}`,
			target: func() any {
				var v any
				return &anyMember{X: xmlschema.AnyJSON{To: &v}}
			},
			keep:   "an Any member holds a JSON object, a string or null",
			absent: "98765",
		},
		{
			name: "a member of a choice of shapes holding a string", json: `{"x":"Jane Doe"}`,
			target: func() any {
				var v any
				return &choiceMember{X: xmlschema.ChoiceJSON{To: &v, Types: []string{"DV_BOOLEAN"}}}
			},
			keep:   "a member of a choice of shapes holds a JSON object or null",
			absent: "Jane",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := canjson.Unmarshal([]byte(tc.json), tc.target())
			if err == nil {
				t.Fatalf("canjson.Unmarshal(%s) = nil; want a refusal", tc.json)
			}
			got := err.Error()
			if !strings.Contains(got, tc.keep) || strings.Contains(got, "cause withheld") {
				t.Errorf("canjson.Unmarshal(%s) = %q; want it to keep %q", tc.json, got, tc.keep)
			}
			if tc.absent != "" && strings.Contains(got, tc.absent) {
				t.Errorf("canjson.Unmarshal(%s) = %q; it carries %q", tc.json, got, tc.absent)
			}
		})
	}
}

// TestREQ176_REQ052_DecimalItemHasNoExponent pins that a typed item's JSON
// number follows the XML Schema built-in its Type names, not only the BMM
// primitive the built-in reads as: an `xs:decimal` item, a Real, is a decimal
// with no exponent, so `1e3` is refused with strconv.ErrSyntax, while an
// `xs:float` item, a Real too, and an `xs:double` item keep theirs.
func TestREQ176_REQ052_DecimalItemHasNoExponent(t *testing.T) {
	for _, tc := range []struct {
		typ, value string
		want       string // the JSON written; "" for a refusal
	}{
		{"xs:decimal", "2.50", `{"_type":"Real","value":2.50}`},
		{"xs:decimal", "-.5", `{"_type":"Real","value":-0.5}`},
		{"xs:decimal", "1e3", ""},
		{"xs:decimal", "1E3", ""},
		{"xs:decimal", " 2.5e-1\n", ""},
		{"xsd:decimal", "1e3", ""},
		{"xs:float", "1e3", `{"_type":"Real","value":1e3}`},
		{"xs:double", "1e3", `{"_type":"Double","value":1e3}`},
		{"xs:double", "1.E3", `{"_type":"Double","value":1E3}`},
	} {
		item := xmlschema.TypedString{Type: tc.typ, Value: tc.value}
		got, err := json.Marshal(item)
		if tc.want == "" {
			if !errors.Is(err, strconv.ErrSyntax) {
				t.Errorf("json.Marshal(%#v) = %s, %v; want a refusal wrapping strconv.ErrSyntax", item, got, err)
			}
			continue
		}
		if err != nil || string(got) != tc.want {
			t.Errorf("json.Marshal(%#v) = %s, %v; want %s, nil", item, got, err, tc.want)
		}
	}
}

// TestREQ176_REQ052_IntegerItemInItsBuiltinsRange pins that a typed item's
// JSON integer follows the range of the XML Schema built-in its Type names,
// not only that of the BMM primitive the built-in reads as: each integer
// built-in writes its least and its greatest value and refuses the integer
// below the one and above the other with strconv.ErrRange, a side XML Schema
// leaves unbounded held to the 64 bits of an Integer64, as the greatest
// `unsignedLong` is.
func TestREQ176_REQ052_IntegerItemInItsBuiltinsRange(t *testing.T) {
	const (
		min64      = "-9223372036854775808"
		max64      = "9223372036854775807"
		belowMin64 = "-9223372036854775809"
		aboveMax64 = "9223372036854775808"
	)
	for _, tc := range []struct {
		builtin, primitive string
		least, greatest    string // written
		below, above       string // refused
	}{
		{"byte", "Integer", "-128", "127", "-129", "128"},
		{"short", "Integer", "-32768", "32767", "-32769", "32768"},
		{"int", "Integer", "-2147483648", "2147483647", "-2147483649", "2147483648"},
		{"unsignedByte", "Integer", "0", "255", "-1", "256"},
		{"unsignedShort", "Integer", "0", "65535", "-1", "65536"},
		{"unsignedInt", "Integer64", "0", "4294967295", "-1", "4294967296"},
		{"unsignedLong", "Integer64", "0", max64, "-1", aboveMax64},
		{"long", "Integer64", min64, max64, belowMin64, aboveMax64},
		{"integer", "Integer64", min64, max64, belowMin64, aboveMax64},
		{"nonNegativeInteger", "Integer64", "0", max64, "-1", aboveMax64},
		{"positiveInteger", "Integer64", "1", max64, "0", aboveMax64},
		{"nonPositiveInteger", "Integer64", min64, "0", belowMin64, "1"},
		{"negativeInteger", "Integer64", min64, "-1", belowMin64, "0"},
	} {
		for _, text := range []string{tc.least, tc.greatest} {
			item := xmlschema.TypedString{Type: "xs:" + tc.builtin, Value: text}
			want := `{"_type":"` + tc.primitive + `","value":` + text + `}`
			if got, err := json.Marshal(item); err != nil || string(got) != want {
				t.Errorf("json.Marshal(%#v) = %s, %v; want %s, nil", item, got, err, want)
			}
		}
		for _, text := range []string{tc.below, tc.above} {
			item := xmlschema.TypedString{Type: "xs:" + tc.builtin, Value: text}
			if got, err := json.Marshal(item); !errors.Is(err, strconv.ErrRange) {
				t.Errorf("json.Marshal(%#v) = %s, %v; want a refusal wrapping strconv.ErrRange", item, got, err)
			}
		}
	}
}

// TestREQ176_REQ052_OtherBuiltinItemsAsString pins the canonical JSON of a
// typed item whose XML Schema built-in no other primitive reads, such as
// `xs:gYear`: an object whose `_type` is String and whose `value` is the text
// as written, which decodes back as an `xsd:string` item, since canonical JSON
// keeps the primitive and not the built-in.
func TestREQ176_REQ052_OtherBuiltinItemsAsString(t *testing.T) {
	for _, tc := range []struct{ builtin, text string }{
		{"gYearMonth", "2020-01"},
		{"gYear", "2020"},
		{"gMonthDay", "--01-31"},
		{"gDay", "---31"},
		{"gMonth", "--01"},
		{"hexBinary", "0F"},
		{"base64Binary", "AQ=="},
		{"QName", "xs:foo"},
		{"NOTATION", "xs:png"},
		{"NMTOKENS", "a b"},
		{"IDREFS", "a b"},
		{"ENTITY", "e"},
		{"ENTITIES", "e f"},
		{"anySimpleType", "x"},
		{"anyType", "x"},
	} {
		item := xmlschema.TypedString{Type: "xs:" + tc.builtin, Value: tc.text}
		got, err := json.Marshal(item)
		if want := `{"_type":"String","value":"` + tc.text + `"}`; err != nil || string(got) != want {
			t.Errorf("json.Marshal(%#v) = %s, %v; want %s, nil", item, got, err, want)
			continue
		}
		var v any
		w := struct {
			X xmlschema.AnyJSON `json:"x"`
		}{X: xmlschema.AnyJSON{To: &v}}
		if err := json.Unmarshal([]byte(`{"x":`+string(got)+`}`), &w, typereg.Unmarshalers()); err != nil {
			t.Errorf("AnyJSON(%s) = %v", got, err)
			continue
		}
		if want := (&xmlschema.TypedString{Value: tc.text, Type: "xsd:string"}); !reflect.DeepEqual(v, want) {
			t.Errorf("AnyJSON(%s) stored %#v; want %#v", got, v, want)
		}
	}
}

// registerUriClass registers a class under the name of the BMM primitive Uri,
// once per test binary, so that a test can tell whether a decoder asks the
// type registry before it reads a primitive's `_type` as a typed item.
var registerUriClass = sync.OnceFunc(func() {
	typereg.Default.Register("Uri", func() any { return new(rm.DVURI) })
})

// TestREQ176_REQ052_AnyJSONReadsAPrimitiveBeforeTheRegistry pins the order of
// the `Any` member's decode: an object whose `_type` names a BMM primitive an
// XML Schema built-in reads as is a typed item even when the type registry
// holds a class of that name, whether `_type` comes first or not.
func TestREQ176_REQ052_AnyJSONReadsAPrimitiveBeforeTheRegistry(t *testing.T) {
	registerUriClass()
	for _, in := range []string{`{"_type":"Uri","value":"http://example.org/a"}`, `{"value":"http://example.org/a","_type":"Uri"}`} {
		var v any
		w := struct {
			X xmlschema.AnyJSON `json:"x"`
		}{X: xmlschema.AnyJSON{To: &v}}
		if err := json.Unmarshal([]byte(`{"x":`+in+`}`), &w, typereg.Unmarshalers()); err != nil {
			t.Fatalf("AnyJSON(%s) = %v", in, err)
		}
		if want := (&xmlschema.TypedString{Value: "http://example.org/a", Type: "xsd:anyURI"}); !reflect.DeepEqual(v, want) {
			t.Errorf("AnyJSON(%s) stored %#v; want %#v", in, v, want)
		}
	}
}
