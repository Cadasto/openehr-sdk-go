package datamap

import (
	"reflect"
	"testing"
)

// REQ-058 — value-schema layer: each clinical value renders as
// oneOf[short, expanded] with an rmType discriminator on the expanded form.

func TestValueSchemaQuantity(t *testing.T) {
	got := valueSchema("DV_QUANTITY", nil)
	oneOf, ok := got["oneOf"].([]any)
	if !ok || len(oneOf) != 2 {
		t.Fatalf("expected oneOf with 2 members, got %#v", got["oneOf"])
	}

	short := oneOf[0].(map[string]any)
	if short["type"] != "number" {
		t.Errorf("short form: want type=number, got %v", short["type"])
	}

	exp := oneOf[1].(map[string]any)
	if exp["type"] != "object" || exp["additionalProperties"] != false {
		t.Errorf("expanded form: want object/additionalProperties=false, got %#v", exp)
	}
	props := exp["properties"].(map[string]any)
	if mag := props["magnitude"].(map[string]any); mag["type"] != "number" {
		t.Errorf("magnitude type: want number, got %v", mag["type"])
	}
	if _, ok := props["unit"]; !ok {
		t.Error("expanded DV_QUANTITY missing unit property")
	}
	if rt := props["rmType"].(map[string]any); rt["const"] != "DV_QUANTITY" {
		t.Errorf("rmType const: want DV_QUANTITY, got %v", rt["const"])
	}
	if req := exp["required"].([]string); !reflect.DeepEqual(req, []string{"magnitude"}) {
		t.Errorf("required: want [magnitude], got %v", req)
	}
}

func TestValueSchemaCodedTextEnum(t *testing.T) {
	opts := []optChoice{{code: "at0010", text: "Zittend"}, {code: "at0011", text: "Liggend"}}
	got := valueSchema("DV_CODED_TEXT", opts)
	oneOf := got["oneOf"].([]any)

	short := oneOf[0].(map[string]any)
	if enum, ok := short["enum"].([]string); !ok || !reflect.DeepEqual(enum, []string{"at0010", "at0011"}) {
		t.Errorf("short enum: want [at0010 at0011], got %#v", short["enum"])
	}

	exp := oneOf[1].(map[string]any)
	props := exp["properties"].(map[string]any)
	code := props["code"].(map[string]any)
	if enum, ok := code["enum"].([]string); !ok || len(enum) != 2 {
		t.Errorf("expanded code enum: want 2 entries, got %#v", code["enum"])
	}
	if req := exp["required"].([]string); !reflect.DeepEqual(req, []string{"code"}) {
		t.Errorf("required: want [code], got %v", req)
	}
}

func TestShortSchemaForTypePrimitives(t *testing.T) {
	cases := map[string]string{
		"DV_BOOLEAN":   "boolean",
		"DV_COUNT":     "integer",
		"DV_QUANTITY":  "number",
		"DV_DATE_TIME": "string",
		"DV_TEXT":      "string",
	}
	for rmType, wantType := range cases {
		if got := shortSchemaForType(rmType, nil); got["type"] != wantType {
			t.Errorf("%s short type: want %s, got %v", rmType, wantType, got["type"])
		}
	}
	if got := shortSchemaForType("DV_DATE_TIME", nil); got["format"] != "date-time" {
		t.Errorf("DV_DATE_TIME format: want date-time, got %v", got["format"])
	}
}

func TestExpandedTextHasOptionalFields(t *testing.T) {
	exp := expandedSchemaForType("DV_TEXT", nil)
	props := exp["properties"].(map[string]any)
	for _, k := range []string{"value", "formatting", "hyperlink", "language", "rmType"} {
		if _, ok := props[k]; !ok {
			t.Errorf("DV_TEXT expanded missing property %q", k)
		}
	}
	if req := exp["required"].([]string); !reflect.DeepEqual(req, []string{"value"}) {
		t.Errorf("required: want [value], got %v", req)
	}
}
