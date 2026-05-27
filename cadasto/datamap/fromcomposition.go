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
	// Per-archetype-root term maps from the OPT, so decoded keys carry the same
	// "atNNNN|Label" labels the Schema emits (SPEC §4.3) and therefore validate.
	rootsByID := map[string]contentRoot{}
	if opt != nil {
		if root, ok := opt.Root().(template.ObjectNode); ok {
			for _, r := range findContentArchetypeRoots(root) {
				rootsByID[r.id] = r
			}
		}
	}
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

	// The composition version uid lives in composition.uid.value
	// ("<object>::<system>::<version>"); surface it (and the bare
	// versioned-object id as vuid) so the datamap shows which version it is.
	if uid := readNameValue(composition["uid"]); uid != "" {
		out["uid"] = uid
		if i := indexOf(uid, "::"); i >= 0 {
			out["vuid"] = uid[:i]
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
			archetypeID, _ := entry["archetype_node_id"].(string)
			key, value, err := decodeArchetypeRoot(entry, rootsByID[archetypeID])
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
// "<archetype-id>|<label>" key and inner payload. The matching OPT root (r)
// supplies the term labels so keys align with Schema().
func decodeArchetypeRoot(node map[string]any, r contentRoot) (string, map[string]any, error) {
	archetypeID, _ := node["archetype_node_id"].(string)
	key := archetypeID
	if r.label != "" {
		key = archetypeID + "|" + r.label
	} else if label := readNameValue(node["name"]); label != "" {
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
			decoded, err := decodeEvent(ev, r)
			if err != nil {
				return "", nil, fmt.Errorf("events[%d]: %w", i, err)
			}
			events = append(events, decoded)
		}
		payload["events"] = events
	case "EVALUATION", "ADMIN_ENTRY", "ACTION", "INSTRUCTION":
		items, err := decodeItems(asList(data["items"]), r)
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

func decodeEvent(event map[string]any, r contentRoot) (map[string]any, error) {
	out := map[string]any{}
	if t := readDVValue(event["time"]); t != nil {
		out["time"] = t
	}
	if w := readDVValue(event["width"]); w != nil {
		out["width"] = w
	}
	data, _ := event["data"].(map[string]any)
	items, err := decodeItems(asList(data["items"]), r)
	if err != nil {
		return nil, err
	}
	for k, v := range items {
		out[k] = v
	}
	return out, nil
}

// decodeItems keys each item "atNNNN|Label" using the OPT term map (matching
// Schema). Array-valued nodes (occurrences allow >1) are ALWAYS emitted as an
// array — even with a single instance — to match the Schema's arraySchema
// wrapping; single-occurrence nodes stay a scalar/object.
func decodeItems(items []any, r contentRoot) (map[string]any, error) {
	type bucket struct {
		key  string
		vals []any
	}
	order := []string{}
	byNode := map[string]*bucket{}

	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item[%d] is not an object", i)
		}
		nodeID, _ := item["archetype_node_id"].(string)
		key := nodeID
		if lbl := r.terms[nodeID]; lbl != "" {
			key = nodeID + "|" + lbl
		}

		var decoded any
		switch item["_type"] {
		case "CLUSTER":
			sub, err := decodeItems(asList(item["items"]), r)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", nodeID, err)
			}
			// A coded runtime name identifies which instance this is (e.g. the
			// lab determination "MDRD"/"eGFR/1.73m2" on a repeating result
			// group). Plain-text names are the template label and are skipped.
			if display, code := readCodedName(item["name"]); code != "" {
				sub["_code"] = code
				if display != "" {
					sub["_name"] = display
				}
			}
			decoded = sub
		case "ELEMENT":
			decoded = decodeElementValue(item["value"])
		default:
			continue
		}

		b := byNode[nodeID]
		if b == nil {
			b = &bucket{key: key}
			byNode[nodeID] = b
			order = append(order, nodeID)
		}
		b.vals = append(b.vals, decoded)
	}

	out := map[string]any{}
	for _, nodeID := range order {
		b := byNode[nodeID]
		if r.arrayNodes[nodeID] || len(b.vals) > 1 {
			out[b.key] = b.vals
		} else {
			out[b.key] = b.vals[0]
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
	case "DV_COUNT", "DV_QUANTITY", "DV_PROPORTION":
		// Datamap short form is the bare magnitude number; units/precision are
		// template-derived and refilled by ToComposition.
		if m, ok := value["magnitude"]; ok {
			return m
		}
	case "DV_CODED_TEXT", "DV_ORDINAL":
		// Datamap short form is the bare code string (from defining_code).
		if code := readCodePhraseCode(value["defining_code"]); code != "" {
			return code
		}
		// A coded text with no usable code is effectively free text — collapse
		// to its value string so it re-encodes as a valid DV_TEXT.
		if s, ok := value["value"].(string); ok {
			return s
		}
	}
	// Defensive: any value still carrying a defining_code is a coded value whose
	// _type wasn't matched above — collapse it to its code so it stays
	// schema-valid (datamap short form for coded values is the bare code).
	if code := readCodePhraseCode(value["defining_code"]); code != "" {
		return code
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

// readCodedName returns the display text and "<terminology>::<code>" of a
// runtime DV_CODED_TEXT name. Returns empty code for a plain DV_TEXT name (the
// template label), which the caller treats as "no meaningful runtime name".
func readCodedName(v any) (display, code string) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", ""
	}
	if t, _ := m["_type"].(string); t != "DV_CODED_TEXT" {
		return "", ""
	}
	display, _ = m["value"].(string)
	dc, ok := m["defining_code"].(map[string]any)
	if !ok {
		return display, ""
	}
	cs, _ := dc["code_string"].(string)
	if cs == "" {
		return display, ""
	}
	if ti, ok := dc["terminology_id"].(map[string]any); ok {
		if term, _ := ti["value"].(string); term != "" {
			return display, term + "::" + cs
		}
	}
	return display, cs
}
