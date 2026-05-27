package datamap

import "github.com/cadasto/openehr-sdk-go/openehr/template"

// empty.go — blank datamap skeleton generation (REQ-058). A "new" form needs an
// empty payload of the right shape to bind to; this walks the datamap schema
// and emits empty values for every field.

// Empty returns a blank datamap skeleton for opt: every object property is
// recursed, arrays are seeded with one empty element, scalars get their zero
// value, and a oneOf takes its first branch. The caller is responsible for
// seeding write-time defaults (e.g. context.start_time), which are deliberately
// left blank here.
func Empty(opt *template.OperationalTemplate) map[string]any {
	if m, ok := emptyFromSchema(Schema(opt)).(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// emptyFromSchema produces a blank instance for a JSON Schema node: objects get
// every property recursed, arrays get one empty element, scalars get their zero
// value. oneOf picks the first branch.
func emptyFromSchema(node any) any {
	schema, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	if variants, ok := schema["oneOf"].([]any); ok && len(variants) > 0 {
		return emptyFromSchema(variants[0])
	}
	switch schema["type"] {
	case "object":
		out := map[string]any{}
		if props, ok := schema["properties"].(map[string]any); ok {
			for k, v := range props {
				out[k] = emptyFromSchema(v)
			}
		}
		return out
	case "array":
		if items, ok := schema["items"]; ok {
			return []any{emptyFromSchema(items)}
		}
		return []any{}
	case "string":
		return ""
	case "integer", "number":
		return 0
	case "boolean":
		return false
	default:
		return nil
	}
}
