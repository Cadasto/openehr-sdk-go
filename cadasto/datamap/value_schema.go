package datamap

// value_schema.go — the per-RM-datatype JSON Schema fragments that a datamap
// value can take. Each clinical value is expressed as a oneOf[short, expanded]:
// the "short" form is the ergonomic primitive (a bare number/string/bool), the
// "expanded" form is the explicit object carrying an rmType discriminator.
//
// Ported from the Cadasto dmv2 SchemaBuilder (REQ-058). Unlike the PHP/archive
// origin this emits plain map[string]any — JSON object key order is not
// semantically meaningful, so callers/tests compare schemas structurally rather
// than byte-for-byte.

// optChoice is one allowed coded option (code + human label) for a
// DV_CODED_TEXT / DV_ORDINAL value, resolved from the OPT term definitions.
type optChoice struct {
	code string
	text string
}

func enumFromOptions(opts []optChoice) []string {
	if len(opts) == 0 {
		return nil
	}
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.code
	}
	return out
}

// shortSchema is the compact primitive form: {type, format?, pattern?, enum?}.
func shortSchema(typ, format string, enum []string, pattern string) map[string]any {
	o := map[string]any{"type": typ}
	if format != "" {
		o["format"] = format
	}
	if pattern != "" {
		o["pattern"] = pattern
	}
	if len(enum) > 0 {
		o["enum"] = enum
	}
	return o
}

func constField(v string) map[string]any { return map[string]any{"const": v} }

func enumProperty(enum []string) map[string]any {
	o := map[string]any{"type": "string"}
	if len(enum) > 0 {
		o["enum"] = enum
	}
	return o
}

// expandedSchema wraps a property set into the explicit object form, stamping
// the rmType const discriminator.
func expandedSchema(rmType string, props map[string]any, required []string) map[string]any {
	props["rmType"] = constField(rmType)
	o := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

// shortSchemaForType returns the compact JSON Schema for a value RM type.
func shortSchemaForType(rmType string, options []optChoice) map[string]any {
	switch rmType {
	case "DV_BOOLEAN":
		return shortSchema("boolean", "", nil, "")
	case "DV_COUNT":
		return shortSchema("integer", "", nil, "")
	case "DV_QUANTITY", "DV_PROPORTION":
		return shortSchema("number", "", nil, "")
	case "DV_DATE_TIME":
		return shortSchema("string", "date-time", nil, "")
	case "DV_DATE":
		return shortSchema("string", "date", nil, "")
	case "DV_TIME":
		return shortSchema("string", "", nil, `^[0-2][0-9]:[0-5][0-9]$`)
	case "DV_CODED_TEXT", "DV_ORDINAL":
		return shortSchema("string", "", enumFromOptions(options), "")
	default:
		return shortSchema("string", "", nil, "")
	}
}

// expandedSchemaForType returns the explicit object form (with rmType const).
func expandedSchemaForType(rmType string, options []optChoice) map[string]any {
	enum := enumFromOptions(options)
	switch rmType {
	case "DV_DATE_TIME":
		return expandedSchema(rmType, map[string]any{"value": shortSchema("string", "date-time", nil, "")}, []string{"value"})
	case "DV_DATE":
		return expandedSchema(rmType, map[string]any{"value": shortSchema("string", "date", nil, "")}, []string{"value"})
	case "DV_TIME":
		return expandedSchema(rmType, map[string]any{"value": shortSchema("string", "", nil, `^[0-2][0-9]:[0-5][0-9]$`)}, []string{"value"})
	case "DV_TEXT":
		return expandedSchema(rmType, map[string]any{
			"value":      shortSchema("string", "", nil, ""),
			"formatting": shortSchema("string", "", nil, ""),
			"hyperlink":  shortSchema("string", "uri", nil, ""),
			"language":   shortSchema("string", "", nil, ""),
		}, []string{"value"})
	case "DV_CODED_TEXT":
		return expandedSchema(rmType, map[string]any{
			"code":        enumProperty(enum),
			"value":       shortSchema("string", "", nil, ""),
			"terminology": shortSchema("string", "", nil, ""),
		}, []string{"code"})
	case "DV_ORDINAL":
		return expandedSchema(rmType, map[string]any{
			"code":    enumProperty(enum),
			"ordinal": shortSchema("integer", "", nil, ""),
			"value":   shortSchema("string", "", nil, ""),
		}, []string{"code", "ordinal"})
	case "DV_QUANTITY":
		return expandedSchema(rmType, map[string]any{
			"magnitude": shortSchema("number", "", nil, ""),
			"unit":      shortSchema("string", "", nil, ""),
		}, []string{"magnitude"})
	case "DV_COUNT":
		return expandedSchema(rmType, map[string]any{"magnitude": shortSchema("integer", "", nil, "")}, []string{"magnitude"})
	case "DV_BOOLEAN":
		return expandedSchema(rmType, map[string]any{"value": shortSchema("boolean", "", nil, "")}, []string{"value"})
	case "DV_PROPORTION":
		return expandedSchema(rmType, map[string]any{
			"type":        shortSchema("string", "", nil, ""),
			"numerator":   shortSchema("number", "", nil, ""),
			"denominator": shortSchema("number", "", nil, ""),
		}, []string{"type", "numerator", "denominator"})
	default:
		return expandedSchema(rmType, map[string]any{"value": shortSchema("string", "", nil, "")}, []string{"value"})
	}
}

// valueSchema returns the oneOf[short, expanded] wrapper for a value RM type.
func valueSchema(rmType string, options []optChoice) map[string]any {
	return map[string]any{
		"oneOf": []any{
			shortSchemaForType(rmType, options),
			expandedSchemaForType(rmType, options),
		},
	}
}
