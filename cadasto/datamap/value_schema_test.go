package datamap

import (
	"reflect"
	"testing"
)

// REQ-058 — value-schema layer: each clinical value renders as
// oneOf[short, expanded]. The short branch is the ergonomic scalar (with enum
// for coded values); the expanded branch is a permissive object that accepts
// any RM value object (the "{rmType, …}" form), so round-tripped CDR data with
// foreign/extra fields validates. Typed-expanded modelling lives in
// expandedSchemaForType (see TestExpandedTextHasOptionalFields).

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
	if exp["type"] != "object" || exp["additionalProperties"] != true {
		t.Errorf("expanded form: want permissive object (additionalProperties=true), got %#v", exp)
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
	if exp["type"] != "object" || exp["additionalProperties"] != true {
		t.Errorf("expanded form: want permissive object (additionalProperties=true), got %#v", exp)
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
