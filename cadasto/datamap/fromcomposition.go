package datamap

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// fromcomposition.go — the read path (REQ-058): a canonical openEHR RM
// COMPOSITION (as returned by the CDR, decoded to map[string]any) → a datamap
// payload in the same shape ToComposition accepts. Ported from the dmv2 decode
// step.

// FromComposition converts a canonical RM COMPOSITION (map[string]any) into a
// datamap payload. DV_* leaves are emitted in their datamap form (value-only
// scalars for simple types; the remaining fields for structured types), so the
// result re-encodes cleanly via ToComposition (modulo server-assigned uid).
//
// The opt parameter is accepted for API symmetry with ToComposition and to
// allow future OPT-driven label/coded-value resolution; it is currently unused
// — decode walks the composition's own name / archetype_node_id fields.
func FromComposition(opt *template.OperationalTemplate, composition map[string]any) (map[string]any, error) {
	_ = opt
	if composition == nil {
		return nil, fmt.Errorf("datamap.FromComposition: nil composition")
	}
	if rmType, _ := composition["_type"].(string); rmType != "COMPOSITION" {
		return nil, fmt.Errorf("datamap.FromComposition: expected COMPOSITION, got %q", rmType)
	}

	out := map[string]any{}

	if lang := readCodePhraseCode(composition["language"]); lang != "" {
		out["language"] = lang
	}
	if terr := readCodePhraseCode(composition["territory"]); terr != "" {
		out["territory"] = terr
	}
	if composer, ok := composition["composer"].(map[string]any); ok {
		if name, _ := composer["name"].(string); name != "" {
			out["composer"] = name
		}
	}

	if ctx, ok := composition["context"].(map[string]any); ok {
		decoded := map[string]any{}
		if st := readDVValue(ctx["start_time"]); st != nil {
			decoded["start_time"] = st
		}
		if setCode := readDVCodedTextCode(ctx["setting"]); setCode != "" {
			decoded["setting"] = setCode
		}
		if len(decoded) > 0 {
			out["context"] = decoded
		}
	}

	if contentList, _ := composition["content"].([]any); len(contentList) > 0 {
		content := map[string]any{}
		for i, raw := range contentList {
			entry, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("content[%d] is not an object", i)
			}
			key, value, err := decodeArchetypeRoot(entry)
			if err != nil {
				return nil, fmt.Errorf("content[%d]: %w", i, err)
			}
			content[key] = value
		}
		out["content"] = content
	}

	return out, nil
}

// decodeArchetypeRoot decodes one entry under content into its
// "<archetype-id>|<label>" key and inner payload.
func decodeArchetypeRoot(node map[string]any) (string, map[string]any, error) {
	archetypeID, _ := node["archetype_node_id"].(string)
	key := archetypeID
	if label := readNameValue(node["name"]); label != "" {
		key = archetypeID + "|" + label
	}

	rmType, _ := node["_type"].(string)
	data, _ := node["data"].(map[string]any)
	payload := map[string]any{}

	switch rmType {
	case "OBSERVATION":
		eventsRaw, _ := data["events"].([]any)
		events := make([]any, 0, len(eventsRaw))
		for i, evRaw := range eventsRaw {
			ev, ok := evRaw.(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("events[%d] is not an object", i)
			}
			decoded, err := decodeEvent(ev)
			if err != nil {
				return "", nil, fmt.Errorf("events[%d]: %w", i, err)
			}
			events = append(events, decoded)
		}
		payload["events"] = events
	case "EVALUATION", "ADMIN_ENTRY", "ACTION", "INSTRUCTION":
		items, err := decodeItems(asList(data["items"]))
		if err != nil {
			return "", nil, err
		}
		for k, v := range items {
			payload[k] = v
		}
	default:
		return "", nil, fmt.Errorf("RM entry type %q not supported", rmType)
	}

	return key, payload, nil
}

func decodeEvent(event map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if t := readDVValue(event["time"]); t != nil {
		out["time"] = t
	}
	if w := readDVValue(event["width"]); w != nil {
		out["width"] = w
	}
	data, _ := event["data"].(map[string]any)
	items, err := decodeItems(asList(data["items"]))
	if err != nil {
		return nil, err
	}
	for k, v := range items {
		out[k] = v
	}
	return out, nil
}

func decodeItems(items []any) (map[string]any, error) {
	out := map[string]any{}
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item[%d] is not an object", i)
		}
		nodeID, _ := item["archetype_node_id"].(string)
		key := nodeID
		if label := readNameValue(item["name"]); label != "" {
			key = nodeID + "|" + label
		}

		switch item["_type"] {
		case "CLUSTER":
			sub, err := decodeItems(asList(item["items"]))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", nodeID, err)
			}
			out[key] = sub
		case "ELEMENT":
			out[key] = decodeElementValue(item["value"])
		default:
			// Skip unknown structural nodes rather than erroring.
		}
	}
	return out, nil
}

// decodeElementValue strips RM bookkeeping from a DV_* value. Value-only DV
// types collapse to their bare scalar; structured types keep their fields.
func decodeElementValue(v any) any {
	value, ok := v.(map[string]any)
	if !ok {
		return v
	}
	switch rmType, _ := value["_type"].(string); rmType {
	case "DV_TEXT", "DV_DATE_TIME", "DV_DATE", "DV_TIME", "DV_BOOLEAN", "DV_URI":
		if s, ok := value["value"]; ok {
			return s
		}
	case "DV_COUNT":
		if m, ok := value["magnitude"]; ok {
			return m
		}
	}
	out := map[string]any{}
	for k, val := range value {
		if k != "_type" {
			out[k] = val
		}
	}
	return out
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func readCodePhraseCode(v any) string {
	cp, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	cs, _ := cp["code_string"].(string)
	return cs
}

func readDVValue(v any) any {
	if v == nil {
		return nil
	}
	if obj, ok := v.(map[string]any); ok {
		if val, ok := obj["value"]; ok {
			return val
		}
		return nil
	}
	return v
}

func readDVCodedTextCode(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	dc, ok := obj["defining_code"].(map[string]any)
	if !ok {
		return ""
	}
	cs, _ := dc["code_string"].(string)
	return cs
}

func readNameValue(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := obj["value"].(string)
	return s
}
