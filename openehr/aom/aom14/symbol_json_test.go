package aom14_test

// REQ-176, REQ-052: canonical JSON of the ORDINAL and SCALE symbol, a BMM
// `Any` whose canonical XML takes the DV_CODED_TEXT or the CODE_PHRASE shape:
// it carries its `_type` and decodes as the shape that names, and no other.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestREQ176_REQ052_SymbolCanonicalJSON pins that an ORDINAL or SCALE symbol in
// either shape, a *rm.DVCodedText or a *rm.CodePhrase, decodes back from its
// canonical JSON as the value written.
func TestREQ176_REQ052_SymbolCanonicalJSON(t *testing.T) {
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1"}
	for _, symbol := range []any{&rm.DVCodedText{DefiningCode: code}, &code} {
		for _, v := range []any{
			&aom14.Ordinal{Value: 1, Symbol: symbol},
			&aom14.Scale{Value: 1.5, Symbol: symbol},
		} {
			b, err := canjson.Marshal(v)
			if err != nil {
				t.Fatalf("canjson.Marshal(%T holding %T) = %v", v, symbol, err)
			}
			back := reflect.New(reflect.TypeOf(v).Elem()).Interface()
			if err := canjson.Unmarshal(b, back, canjson.WithStrict()); err != nil {
				t.Fatalf("canjson.Unmarshal(%s) = %v", b, err)
			}
			if !reflect.DeepEqual(back, v) {
				t.Errorf("%s decoded back as %#v; want %#v", b, back, v)
			}
		}
	}
}

// TestREQ176_REQ052_SymbolCanonicalJSONRefusals pins what a symbol's canonical
// JSON may not hold: a class that is none of its shapes, refused as a type
// mismatch; no `_type`, refused as a missing one; and a string, which no shape
// reads, refused as a shape failure. A null symbol is nil.
func TestREQ176_REQ052_SymbolCanonicalJSONRefusals(t *testing.T) {
	ordinal := func(symbol string) []byte {
		return []byte(`{"_type":"ORDINAL","value":1,"symbol":` + symbol + `}`)
	}
	for _, tc := range []struct {
		symbol string
		want   error
	}{
		{`{"_type":"DV_TEXT","value":"x"}`, typereg.ErrTypeMismatch},
		{`{"value":"x"}`, typereg.ErrMissingType},
		{`"at1"`, canjson.ErrInvalidShape},
		{`{"_type":"NO_SUCH_TYPE"}`, typereg.ErrUnknownType},
	} {
		if err := canjson.Unmarshal(ordinal(tc.symbol), new(aom14.Ordinal)); !errors.Is(err, tc.want) {
			t.Errorf("canjson.Unmarshal(symbol %s) = %v; want %v", tc.symbol, err, tc.want)
		}
	}
	var null aom14.Ordinal
	if err := canjson.Unmarshal(ordinal(`null`), &null); err != nil || null.Symbol != nil {
		t.Errorf("canjson.Unmarshal(null symbol) = %v, symbol %#v; want nil, nil", err, null.Symbol)
	}
}

// TestREQ176_REQ052_SymbolCanonicalJSONEncodeRefusals pins that canonical JSON
// refuses to write an ORDINAL or SCALE symbol its decoder would not read back,
// as the canonical-XML encoder refuses one: a value that is none of the two
// shapes, such as a string, a *rm.DVText or a number, fails with
// canjson.ErrInvalidValue, and so does an EXPR_LEAF item that is neither a
// class value, a typed string, a string nor nil, such as a number, a boolean,
// a list or a map. Either shape by value, a class value and nil are written.
func TestREQ176_REQ052_SymbolCanonicalJSONEncodeRefusals(t *testing.T) {
	code := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at1"}
	refused := []any{"at0001", &rm.DVText{Value: "x"}, rm.DVText{Value: "x"}, 42, &code.TerminologyID}
	for _, symbol := range refused {
		for _, v := range []any{&aom14.Ordinal{Value: 1, Symbol: symbol}, &aom14.Scale{Value: 1.5, Symbol: symbol}} {
			if b, err := canjson.Marshal(v); !errors.Is(err, canjson.ErrInvalidValue) {
				t.Errorf("canjson.Marshal(%T holding %T) = %s, %v; want canjson.ErrInvalidValue", v, symbol, b, err)
			}
		}
	}
	for _, symbol := range []any{nil, rm.DVCodedText{DefiningCode: code}, code} {
		for _, v := range []any{&aom14.Ordinal{Value: 1, Symbol: symbol}, &aom14.Scale{Value: 1.5, Symbol: symbol}} {
			if _, err := canjson.Marshal(v); err != nil {
				t.Errorf("canjson.Marshal(%T holding %T) = %v; want nil", v, symbol, err)
			}
		}
	}
	for _, item := range []any{42, 2.5, true, []string{"a"}, map[string]any{}, &code.CodeString} {
		leaf := &aom14.ExprLeaf{Type: "String", ReferenceType: "constant", Item: item}
		if b, err := canjson.Marshal(leaf); !errors.Is(err, canjson.ErrInvalidValue) {
			t.Errorf("canjson.Marshal(EXPR_LEAF holding %T) = %s, %v; want canjson.ErrInvalidValue", item, b, err)
		}
	}
	for _, item := range []any{nil, "x", &aom14.CString{}, &rm.DVText{Value: "x"}, aom14.TypedString{Value: "x", Type: "xs:string"}} {
		leaf := &aom14.ExprLeaf{Type: "String", ReferenceType: "constant", Item: item}
		if _, err := canjson.Marshal(leaf); err != nil {
			t.Errorf("canjson.Marshal(EXPR_LEAF holding %T) = %v; want nil", item, err)
		}
	}
}
