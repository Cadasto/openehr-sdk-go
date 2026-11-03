package aom14_test

// REQ-176, REQ-052: canonical JSON of the EXPR_LEAF item, a BMM `Any`: a class
// value carries its `_type` and decodes as the class it names, a String the
// XML gave an XML Schema built-in type is an object holding its text and that
// `xsi:type`, and a plain String is a JSON string; each decodes back to the
// value written.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ176_REQ052_ExprLeafItemCanonicalJSON pins the canonical JSON of each
// value an EXPR_LEAF item holds and its decode back to the same value: a
// *aom14.CString as an object with `_type` C_STRING, a *aom14.TypedString as
// an object with its `value` and `xsi_type` and no `_type`, and a Go string as
// a JSON string.
func TestREQ176_REQ052_ExprLeafItemCanonicalJSON(t *testing.T) {
	pattern := ".*"
	for _, tc := range []struct {
		name string
		item any
		want string // the item's canonical JSON
	}{
		{"C_STRING", &aom14.CString{Pattern: &pattern}, `{"_type":"C_STRING","pattern":".*"}`},
		{"xs:string", &aom14.TypedString{Value: "archetype_id/value", Type: "xs:string"}, `{"value":"archetype_id/value","xsi_type":"xs:string"}`},
		{"xsd:string", &aom14.TypedString{Value: "archetype_id/value", Type: "xsd:string"}, `{"value":"archetype_id/value","xsi_type":"xsd:string"}`},
		{"no xsi:type", "archetype_id/value", `"archetype_id/value"`},
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
		if !reflect.DeepEqual(&back, leaf) {
			t.Errorf("%s: decoded back as %#v; want %#v", tc.name, back.Item, tc.item)
		}
	}
}

// TestREQ176_REQ052_ExprLeafItemCanonicalJSONRefusals pins what an EXPR_LEAF
// item's canonical JSON may not hold: a number, a boolean or an array, which
// no item reads as; an object with no `_type` whose `xsi_type` is no prefixed
// XML Schema built-in datatype; and a `_type` the registry does not hold. Each
// is a shape failure, the last an unknown type. A null item is nil.
func TestREQ176_REQ052_ExprLeafItemCanonicalJSONRefusals(t *testing.T) {
	leaf := func(item string) []byte {
		return []byte(`{"_type":"EXPR_LEAF","type":"String","item":` + item + `,"reference_type":"attribute"}`)
	}
	for _, item := range []string{`1`, `true`, `["a"]`, `{"value":"a","xsi_type":"string"}`, `{"value":"a","xsi_type":"xs:nothing"}`, `{"value":"a"}`} {
		err := canjson.Unmarshal(leaf(item), new(aom14.ExprLeaf))
		if !errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("canjson.Unmarshal(item %s) = %v; want canjson.ErrInvalidShape", item, err)
		}
	}
	if err := canjson.Unmarshal(leaf(`{"_type":"NO_SUCH_TYPE"}`), new(aom14.ExprLeaf)); !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("canjson.Unmarshal(item of an unknown _type) = %v; want typereg.ErrUnknownType", err)
	}
	var null aom14.ExprLeaf
	if err := canjson.Unmarshal(leaf(`null`), &null); err != nil || null.Item != nil {
		t.Errorf("canjson.Unmarshal(null item) = %v, item %#v; want nil, nil", err, null.Item)
	}
}
