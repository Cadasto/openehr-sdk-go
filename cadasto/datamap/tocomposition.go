package datamap

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// tocomposition.go — the write path (REQ-058): a datamap payload + its OPT →
// a canonical openEHR RM COMPOSITION (map[string]any). Ported from the dmv2
// encode step onto openehr/template.
//
// Scope: COMPOSITION with OBSERVATION (data.events.data.items) and
// EVALUATION/ADMIN_ENTRY/ACTION/INSTRUCTION (data.items) entries; CLUSTER
// containers; DV_QUANTITY/DV_DATE_TIME/DV_DATE/DV_TIME/DV_TEXT/DV_BOOLEAN/
// DV_COUNT/DV_CODED_TEXT leaves. Demographic party encoding is out of scope.

// ToComposition builds a canonical RM COMPOSITION from a datamap payload,
// walking the OPT for structure, node ids and DV_QUANTITY units. The payload
// shape mirrors what FromComposition produces (content keyed by
// "<archetype-id>|<label>", items by "<at-code>|<label>"); bare keys without
// the "|label" suffix are also accepted. context.start_time is required.
func ToComposition(opt *template.OperationalTemplate, payload map[string]any) (map[string]any, error) {
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		return nil, fmt.Errorf("datamap.ToComposition: OPT root is not an object node")
	}

	language := stringOrDefault(payload["language"], "nl")
	territory := stringOrDefault(payload["territory"], "NL")
	composer := stringOrDefault(payload["composer"], "Cadasto SDK")

	contextPayload, _ := payload["context"].(map[string]any)
	startTime := stringOrDefault(contextPayload["start_time"], "")
	if startTime == "" {
		return nil, fmt.Errorf("datamap.ToComposition: context.start_time is required")
	}

	roots := findContentArchetypeRoots(root)
	if len(roots) == 0 {
		return nil, fmt.Errorf("datamap.ToComposition: template has no archetype roots under content")
	}
	contentPayload, _ := payload["content"].(map[string]any)

	content := make([]any, 0, len(roots))
	for i := range roots {
		r := roots[i]
		rootPayload := lookupRootPayload(contentPayload, r.id, r.label)
		if rootPayload == nil {
			continue
		}
		entry, err := encodeArchetypeRoot(r, rootPayload, startTime, language)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", r.id, err)
		}
		content = append(content, entry)
	}

	return map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": rootArchetypeID(root),
		"name":              dvText(rootName(root)),
		"archetype_details": archetypeDetails(rootArchetypeID(root), opt.TemplateID()),
		"language":          codePhrase("ISO_639-1", language),
		"territory":         codePhrase("ISO_3166-1", territory),
		"category":          dvCodedText("event", "openehr", "433"),
		"composer":          map[string]any{"_type": "PARTY_IDENTIFIED", "name": composer},
		"context": map[string]any{
			"_type":      "EVENT_CONTEXT",
			"start_time": dvDateTime(startTime),
			"setting":    dvCodedText("other care", "openehr", "238"),
		},
		"content": content,
	}, nil
}

func encodeArchetypeRoot(r contentRoot, payload map[string]any, startTime, language string) (map[string]any, error) {
	rmType := r.node.RMTypeName()
	out := map[string]any{
		"_type":             rmType,
		"archetype_node_id": r.id,
		"name":              dvText(r.label),
		"archetype_details": archetypeDetails(r.id, ""),
		"language":          codePhrase("ISO_639-1", language),
		"encoding":          codePhrase("IANA_character-sets", "UTF-8"),
		"subject":           map[string]any{"_type": "PARTY_SELF"},
	}

	dataNode, ok := attrFirstObject(findAttr(r.node, "data"))
	if !ok {
		return nil, fmt.Errorf("%s has no data", rmType)
	}

	switch rmType {
	case "OBSERVATION":
		eventConstraint, ok := attrFirstObject(findAttr(dataNode, "events"))
		if !ok {
			return nil, fmt.Errorf("OBSERVATION %s has no events constraint", r.id)
		}
		eventsPayload, _ := payload["events"].([]any)
		events := make([]any, 0, len(eventsPayload))
		for i, ev := range eventsPayload {
			evMap, ok := ev.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("events[%d] is not an object", i)
			}
			encoded, err := encodeEvent(eventConstraint, evMap, r.terms, startTime)
			if err != nil {
				return nil, fmt.Errorf("events[%d]: %w", i, err)
			}
			events = append(events, encoded)
		}
		out["data"] = map[string]any{
			"_type":             "HISTORY",
			"archetype_node_id": dataNode.NodeID(),
			"name":              dvText(termOrFallback(r.terms, dataNode.NodeID(), "Event Series")),
			"origin":            dvDateTime(startTime),
			"events":            events,
		}
	case "EVALUATION", "ADMIN_ENTRY", "ACTION", "INSTRUCTION":
		itemsAttr := findAttr(dataNode, "items")
		if itemsAttr == nil {
			return nil, fmt.Errorf("%s data has no items", rmType)
		}
		items, err := encodeItems(itemsAttr, payload, r.terms)
		if err != nil {
			return nil, err
		}
		out["data"] = map[string]any{
			"_type":             "ITEM_TREE",
			"archetype_node_id": dataNode.NodeID(),
			"name":              dvText(termOrFallback(r.terms, dataNode.NodeID(), "Tree")),
			"items":             items,
		}
	default:
		return nil, fmt.Errorf("RM entry type %q not supported", rmType)
	}
	return out, nil
}

func encodeEvent(eventConstraint template.ObjectNode, payload map[string]any, terms map[string]string, fallbackTime string) (map[string]any, error) {
	t := stringOrDefault(payload["time"], fallbackTime)

	dataNode, ok := attrFirstObject(findAttr(eventConstraint, "data"))
	if !ok {
		return nil, fmt.Errorf("event constraint has no data")
	}
	itemsAttr := findAttr(dataNode, "items")
	if itemsAttr == nil {
		return nil, fmt.Errorf("event ITEM_TREE has no items")
	}
	items, err := encodeItems(itemsAttr, payload, terms)
	if err != nil {
		return nil, err
	}

	eventType := eventConstraint.RMTypeName()
	if eventType == "" || eventType == "EVENT" {
		if _, hasWidth := payload["width"]; hasWidth {
			eventType = "INTERVAL_EVENT"
		} else {
			eventType = "POINT_EVENT"
		}
	}

	return map[string]any{
		"_type":             eventType,
		"archetype_node_id": eventConstraint.NodeID(),
		"name":              dvText(termOrFallback(terms, eventConstraint.NodeID(), "Any event")),
		"time":              dvDateTime(t),
		"data": map[string]any{
			"_type":             "ITEM_TREE",
			"archetype_node_id": dataNode.NodeID(),
			"name":              dvText(termOrFallback(terms, dataNode.NodeID(), "Tree")),
			"items":             items,
		},
	}, nil
}

func encodeItems(itemsAttr *template.Attribute, payload map[string]any, terms map[string]string) ([]any, error) {
	out := []any{}
	for _, child := range itemsAttr.Children() {
		obj, ok := child.(template.ObjectNode)
		if !ok {
			continue
		}
		nodeID := obj.NodeID()
		if nodeID == "" || obj.RMTypeName() == "" {
			continue
		}
		label := terms[nodeID]
		value, found := lookupChildPayload(payload, nodeID, label)
		if !found {
			continue
		}

		// A slice means multiple instances of this repeating node (e.g. several
		// "Result group" clusters); a scalar/object is a single instance.
		instances := []any{value}
		if arr, ok := value.([]any); ok {
			instances = arr
		}

		for _, inst := range instances {
			switch obj.RMTypeName() {
			case "CLUSTER":
				subPayload, ok := inst.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("CLUSTER %s payload must be an object, got %T", nodeID, inst)
				}
				subItemsAttr := findAttr(obj, "items")
				if subItemsAttr == nil {
					continue
				}
				subItems, err := encodeItems(subItemsAttr, subPayload, terms)
				if err != nil {
					return nil, fmt.Errorf("cluster %s: %w", nodeID, err)
				}
				out = append(out, map[string]any{
					"_type":             "CLUSTER",
					"archetype_node_id": nodeID,
					"name":              clusterName(subPayload, label),
					"items":             subItems,
				})
			case "ELEMENT":
				el, err := encodeElement(obj, inst, terms)
				if err != nil {
					return nil, fmt.Errorf("element %s: %w", nodeID, err)
				}
				out = append(out, el)
			}
		}
	}
	return out, nil
}

func encodeElement(elem template.ObjectNode, valuePayload any, terms map[string]string) (map[string]any, error) {
	nodeID := elem.NodeID()
	valueConstraint, ok := attrFirstObject(findAttr(elem, "value"))
	if !ok {
		return nil, fmt.Errorf("ELEMENT %s has no value constraint", nodeID)
	}
	rmValue, err := encodeValue(valueConstraint, valuePayload, terms)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"_type":             "ELEMENT",
		"archetype_node_id": nodeID,
		"name":              dvText(terms[nodeID]),
		"value":             rmValue,
	}, nil
}

func encodeValue(constraint template.ObjectNode, payload any, terms map[string]string) (map[string]any, error) {
	switch rmType := constraint.RMTypeName(); rmType {
	case "DV_QUANTITY":
		return encodeQuantity(constraint, payload)
	case "DV_DATE_TIME", "DV_DATE", "DV_TIME", "DV_TEXT", "DV_BOOLEAN":
		return encodeScalarWrap(rmType, payload), nil
	case "DV_COUNT":
		return encodeCount(payload)
	case "DV_CODED_TEXT":
		return encodeCodedText(payload, terms)
	default:
		return nil, fmt.Errorf("RM value type %q not supported", rmType)
	}
}

func encodeQuantity(constraint template.ObjectNode, payload any) (map[string]any, error) {
	units, precision := quantityDefault(constraint)
	if m, ok := payload.(map[string]any); ok {
		out := map[string]any{"_type": "DV_QUANTITY"}
		for k, v := range m {
			out[k] = v
		}
		if _, has := out["units"]; !has {
			out["units"] = units
		}
		if _, has := out["precision"]; !has {
			out["precision"] = precision
		}
		return out, nil
	}
	mag, ok := toFloat(payload)
	if !ok {
		return nil, fmt.Errorf("DV_QUANTITY: expected number or object, got %T", payload)
	}
	return map[string]any{"_type": "DV_QUANTITY", "magnitude": mag, "units": units, "precision": precision}, nil
}

func encodeCount(payload any) (map[string]any, error) {
	if m, ok := payload.(map[string]any); ok {
		out := map[string]any{"_type": "DV_COUNT"}
		for k, v := range m {
			out[k] = v
		}
		return out, nil
	}
	mag, ok := toInt(payload)
	if !ok {
		return nil, fmt.Errorf("DV_COUNT: expected integer or object, got %T", payload)
	}
	return map[string]any{"_type": "DV_COUNT", "magnitude": mag}, nil
}

func encodeCodedText(payload any, terms map[string]string) (map[string]any, error) {
	if m, ok := payload.(map[string]any); ok {
		// A DV_CODED_TEXT with a null/empty code is invalid RM and the CDR
		// rejects it; such a value is really free text, so emit DV_TEXT.
		if !hasUsableCode(m) {
			if v, ok := m["value"].(string); ok {
				return map[string]any{"_type": "DV_TEXT", "value": v}, nil
			}
		}
		out := map[string]any{"_type": "DV_CODED_TEXT"}
		for k, v := range m {
			out[k] = v
		}
		return out, nil
	}
	code, ok := payload.(string)
	if !ok {
		return nil, fmt.Errorf("DV_CODED_TEXT: expected string code or object, got %T", payload)
	}
	terminology, codeStr := splitTerminology(code)
	label := terms[codeStr]
	if label == "" {
		label = codeStr
	}
	return map[string]any{
		"_type":         "DV_CODED_TEXT",
		"value":         label,
		"defining_code": codePhrase(terminology, codeStr),
	}, nil
}

// hasUsableCode reports whether a coded-value payload carries a non-empty code,
// either as a top-level "code" or inside a defining_code.code_string.
func hasUsableCode(m map[string]any) bool {
	if c, ok := m["code"].(string); ok && c != "" {
		return true
	}
	if dc, ok := m["defining_code"].(map[string]any); ok {
		if cs, ok := dc["code_string"].(string); ok && cs != "" {
			return true
		}
	}
	return false
}

func encodeScalarWrap(rmType string, payload any) map[string]any {
	if m, ok := payload.(map[string]any); ok {
		// Only carry value-type-relevant fields; drop foreign keys (e.g. a
		// leftover defining_code from a coded-text source) that would make the
		// scalar DV invalid. DV_TEXT also allows formatting/hyperlink/language.
		out := map[string]any{"_type": rmType, "value": m["value"]}
		if rmType == "DV_TEXT" {
			for _, k := range []string{"formatting", "hyperlink", "language"} {
				if v, ok := m[k]; ok {
					out[k] = v
				}
			}
		}
		return out
	}
	return map[string]any{"_type": rmType, "value": payload}
}

// quantityDefault reads the first allowed units + precision from a DV_QUANTITY
// value node's C_DV_QUANTITY constraint. Zero values when unconstrained.
func quantityDefault(n template.ObjectNode) (units string, precision int) {
	co, ok := n.(*template.ComplexObject)
	if !ok {
		return "", 0
	}
	dq, ok := co.PrimitiveConstraint().(constraints.DvQuantity)
	if !ok || len(dq.Units) == 0 {
		return "", 0
	}
	u := dq.Units[0]
	if u.Precision.IsBounded() && !u.Precision.LowerUnbounded {
		precision = int(u.Precision.Lower)
	}
	return u.Units, precision
}

// ---- RM JSON builders ----

func dvText(value string) map[string]any {
	return map[string]any{"_type": "DV_TEXT", "value": value}
}

// clusterName builds a CLUSTER's runtime name. When the payload carries a coded
// runtime name (_code "<terminology>::<code>", optional _name display), it
// emits a DV_CODED_TEXT; otherwise a plain DV_TEXT with the template label.
func clusterName(payload map[string]any, label string) map[string]any {
	code, _ := payload["_code"].(string)
	if code == "" {
		return dvText(label)
	}
	display, _ := payload["_name"].(string)
	if display == "" {
		display = label
	}
	terminology, codeStr := splitTerminology(code)
	return map[string]any{
		"_type":         "DV_CODED_TEXT",
		"value":         display,
		"defining_code": codePhrase(terminology, codeStr),
	}
}

func dvDateTime(value string) map[string]any {
	return map[string]any{"_type": "DV_DATE_TIME", "value": value}
}

func dvCodedText(value, terminology, code string) map[string]any {
	return map[string]any{"_type": "DV_CODED_TEXT", "value": value, "defining_code": codePhrase(terminology, code)}
}

func codePhrase(terminology, code string) map[string]any {
	return map[string]any{
		"_type":          "CODE_PHRASE",
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": terminology},
		"code_string":    code,
	}
}

func archetypeDetails(archetypeID, templateID string) map[string]any {
	out := map[string]any{
		"_type":        "ARCHETYPED",
		"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": archetypeID},
		"rm_version":   "1.0.2",
	}
	if templateID != "" {
		out["template_id"] = map[string]any{"_type": "TEMPLATE_ID", "value": templateID}
	}
	return out
}

func rootArchetypeID(root template.ObjectNode) string {
	if ar, ok := root.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
		return ar.ArchetypeID()
	}
	return root.RMTypeName()
}

func rootName(root template.ObjectNode) string {
	if ar, ok := root.(*template.ArchetypeRoot); ok {
		if t, ok := ar.Term("at0000"); ok {
			if v := t.Items["text"]; v != "" {
				return v
			}
		}
	}
	return "Encounter"
}

func termOrFallback(terms map[string]string, code, fallback string) string {
	if t := terms[code]; t != "" {
		return t
	}
	return fallback
}

// lookupRootPayload finds the content-root payload for an archetype id,
// tolerating any "|label" suffix: FromComposition keys roots by the
// composition's stored name, which may differ from the OPT term label that
// ToComposition computes for the same root.
func lookupRootPayload(content map[string]any, id, label string) map[string]any {
	if v, ok := content[id+"|"+label].(map[string]any); ok {
		return v
	}
	if v, ok := content[id].(map[string]any); ok {
		return v
	}
	prefix := id + "|"
	for k, v := range content {
		if k == id || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) {
			if m, ok := v.(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

func lookupChildPayload(payload map[string]any, nodeID, label string) (any, bool) {
	if label != "" {
		if v, ok := payload[nodeID+"|"+label]; ok {
			return v, true
		}
	}
	if v, ok := payload[nodeID]; ok {
		return v, true
	}
	return nil, false
}

func stringOrDefault(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func splitTerminology(code string) (terminology, value string) {
	if i := indexOf(code, "::"); i > 0 {
		return code[:i], code[i+2:]
	}
	return "local", code
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
