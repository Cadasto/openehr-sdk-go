package datamap

import (
	"encoding/json"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// validate.go — datamap payload validation (REQ-058). A datamap value is valid
// when it conforms to the JSON Schema that Schema() derives from the OPT, so
// validation is JSON-Schema compilation + check. This is the single home for
// that step: consumers (forms, the write path, diagnostics) all validate
// identically instead of re-deriving the rule.

// Validate reports whether payload conforms to the datamap schema for opt,
// returning a flat list of human-readable violation lines (empty when valid).
func Validate(opt *template.OperationalTemplate, payload any) (bool, []string) {
	return ValidateSchema(Schema(opt), payload)
}

// ValidateSchema validates payload against an already-built datamap schema (as
// returned by Schema). The schema is round-tripped through JSON so every value
// is a canonical JSON type (float64 etc.) the compiler expects.
func ValidateSchema(schema map[string]any, payload any) (bool, []string) {
	rawSchema, err := json.Marshal(schema)
	if err != nil {
		return false, []string{"marshal schema: " + err.Error()}
	}
	var doc any
	if err := json.Unmarshal(rawSchema, &doc); err != nil {
		return false, []string{"decode schema: " + err.Error()}
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("datamap-schema.json", doc); err != nil {
		return false, []string{"add schema: " + err.Error()}
	}
	sch, err := compiler.Compile("datamap-schema.json")
	if err != nil {
		return false, []string{"compile schema: " + err.Error()}
	}
	if err := sch.Validate(payload); err != nil {
		return false, strings.Split(strings.TrimSpace(err.Error()), "\n")
	}
	return true, nil
}
