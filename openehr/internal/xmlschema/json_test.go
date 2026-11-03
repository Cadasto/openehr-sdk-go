package xmlschema_test

// REQ-176, REQ-052: the canonical-JSON decoders of a BMM `Any` member, and of
// one whose canonical XML takes a choice of shapes, which the generated
// decoders set in the place of the member: a mandatory one held as a Go any
// and an optional one held as a *any.

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlschema"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestREQ176_REQ052_AnyJSONDecoders pins what the two decoders store for each
// JSON value an `Any` member holds: a class value for an object with a
// `_type`, a *TypedString for an object without one, a Go string for a string,
// and nothing for null, the optional member's pointer left nil.
func TestREQ176_REQ052_AnyJSONDecoders(t *testing.T) {
	for _, tc := range []struct {
		json string
		want any
	}{
		{`{"_type":"DV_BOOLEAN","value":true}`, &rm.DVBoolean{Value: true}},
		{`{"value":"a","xsi_type":"xs:string"}`, &xmlschema.TypedString{Value: "a", Type: "xs:string"}},
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
