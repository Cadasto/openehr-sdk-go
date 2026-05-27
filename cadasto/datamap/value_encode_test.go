package datamap

import (
	"reflect"
	"testing"
)

// REQ-058 — encode-short symmetry: value types that decode to a short form must
// re-encode to a valid RM value (no "not supported" error), and the expanded
// notation must round-trip through encodeExpandedValue.

func TestEncodeProportion(t *testing.T) {
	// Bare number → ratio (type 0) with denominator 1, magnitude == the number.
	got, err := encodeProportion(float64(42))
	if err != nil {
		t.Fatalf("encodeProportion(number): %v", err)
	}
	if got["_type"] != "DV_PROPORTION" || got["numerator"] != float64(42) ||
		got["denominator"] != float64(1) || got["type"] != 0 {
		t.Errorf("number form: got %#v", got)
	}

	// Object payload passes through verbatim (plus _type).
	got, err = encodeProportion(map[string]any{"numerator": float64(1), "denominator": float64(3), "type": 3})
	if err != nil {
		t.Fatalf("encodeProportion(object): %v", err)
	}
	if got["_type"] != "DV_PROPORTION" || got["denominator"] != float64(3) || got["type"] != 3 {
		t.Errorf("object form: got %#v", got)
	}
}

func TestEncodeOrdinalFallback(t *testing.T) {
	// No constraint resolves the code → best-effort DV_ORDINAL carrying the
	// symbol, terminology split from "term::code", display from the term map.
	terms := map[string]string{"at0005": "Moderate"}
	got, err := encodeOrdinal(nil, "local::at0005", terms)
	if err != nil {
		t.Fatalf("encodeOrdinal(string): %v", err)
	}
	if got["_type"] != "DV_ORDINAL" {
		t.Fatalf("want DV_ORDINAL, got %#v", got)
	}
	sym, ok := got["symbol"].(map[string]any)
	if !ok || sym["value"] != "Moderate" {
		t.Errorf("symbol: got %#v", got["symbol"])
	}
	dc, _ := sym["defining_code"].(map[string]any)
	if dc["code_string"] != "at0005" {
		t.Errorf("defining_code.code_string: got %v", dc["code_string"])
	}

	// Object payload passes through.
	got, err = encodeOrdinal(nil, map[string]any{"ordinal": 2, "value": 2}, terms)
	if err != nil {
		t.Fatalf("encodeOrdinal(object): %v", err)
	}
	if got["_type"] != "DV_ORDINAL" || got["ordinal"] != 2 {
		t.Errorf("object form: got %#v", got)
	}
}

func TestEncodeExpandedValue(t *testing.T) {
	// rmType discriminator → _type, remaining fields verbatim.
	out := encodeExpandedValue(map[string]any{
		"rmType":    "DV_QUANTITY",
		"magnitude": float64(120),
		"unit":      "mmHg",
	})
	if out == nil {
		t.Fatal("expected non-nil for rmType-bearing map")
	}
	if out["_type"] != "DV_QUANTITY" || out["magnitude"] != float64(120) || out["unit"] != "mmHg" {
		t.Errorf("got %#v", out)
	}
	if _, leaked := out["rmType"]; leaked {
		t.Error("rmType must not leak into the RM value")
	}

	// Non-expanded payloads (no rmType, or non-map) return nil so callers fall
	// through to the short-form coercions.
	if encodeExpandedValue(map[string]any{"value": "x"}) != nil {
		t.Error("map without rmType should return nil")
	}
	if encodeExpandedValue("plain") != nil {
		t.Error("non-map should return nil")
	}
}

func TestStructuredItemsList(t *testing.T) {
	elem := map[string]any{"_type": "ELEMENT"}

	// ITEM_TREE / ITEM_LIST: items[]
	if got := structuredItemsList(map[string]any{"items": []any{elem}}); !reflect.DeepEqual(got, []any{elem}) {
		t.Errorf("items: got %#v", got)
	}
	// ITEM_TABLE: rows[]
	if got := structuredItemsList(map[string]any{"rows": []any{elem}}); !reflect.DeepEqual(got, []any{elem}) {
		t.Errorf("rows: got %#v", got)
	}
	// ITEM_SINGLE: a single item wrapped to a one-element list.
	if got := structuredItemsList(map[string]any{"item": elem}); !reflect.DeepEqual(got, []any{elem}) {
		t.Errorf("item: got %#v", got)
	}
	// Empty container → nil.
	if got := structuredItemsList(map[string]any{}); got != nil {
		t.Errorf("empty: got %#v", got)
	}
}
