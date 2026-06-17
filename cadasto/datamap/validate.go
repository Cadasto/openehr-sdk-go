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
//
// PARTY (demographic) payloads are validated against a LOOSENED copy of the
// schema: FromParty decodes demographics dynamically — global per-nodeID array
// detection, empty-value skipping, instance-coded identity/address labels, and
// nested archetype CLUSTERs (person_identifier.v2 variants, …) — so the strict
// OPT-derived schema false-rejects valid patients. The strict schema is still
// what Schema()/Empty() build (the new-form skeleton needs concrete
// properties); only validation tolerates the decoder's shape. The composition
// path stays strict.
func Validate(opt *template.OperationalTemplate, payload any) (bool, []string) {
	schema := Schema(opt)
	if IsPartyTemplate(opt) {
		if loosened, ok := loosenPartySchema(schema).(map[string]any); ok {
			schema = loosened
		}
	}
	return ValidateSchema(schema, payload)
}

// loosenPartySchema relaxes a party schema so FromParty's dynamic output
// validates: drops `required` (empty fields are skipped, at0000 root may be
// absent), opens `additionalProperties` (coded labels + nested CLUSTER
// variants), rewrites `oneOf`→`anyOf` (decoded values may match >1 variant),
// and lets any object also appear as an array of itself (single-occurrence
// nodes array when their at-code repeats elsewhere). Structure-only — leaf
// value types stay intact.
func loosenPartySchema(node any) any {
	switch n := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(n))
		for k, v := range n {
			switch k {
			case "required", "additionalProperties":
				// dropped/forced below
			case "oneOf":
				out["anyOf"] = loosenPartySchema(v)
			default:
				out[k] = loosenPartySchema(v)
			}
		}
		if out["type"] == "object" || out["properties"] != nil || out["patternProperties"] != nil {
			out["additionalProperties"] = true
			arrayOf := map[string]any{"type": "array", "items": out}
			return map[string]any{"anyOf": []any{out, arrayOf}}
		}
		return out
	case []any:
		arr := make([]any, len(n))
		for i, v := range n {
			arr[i] = loosenPartySchema(v)
		}
		return arr
	default:
		return node
	}
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
