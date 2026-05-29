package datamap

import (
	"fmt"
	"strings"

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

	// A payload that carried content but matched no root means its content keys
	// do not line up with this template's archetype roots — fail loudly with
	// both key sets rather than silently emitting an empty composition.
	if len(content) == 0 && len(contentPayload) > 0 {
		return nil, fmt.Errorf(
			"datamap.ToComposition: content keys do not match template roots; payload has [%s], template expects [%s]",
			strings.Join(mapKeys(contentPayload), ", "),
			strings.Join(rootKeys(roots), ", "),
		)
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

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func rootKeys(roots []contentRoot) []string {
	keys := make([]string, 0, len(roots))
	for _, r := range roots {
		keys = append(keys, r.id+"|"+r.label)
	}
	return keys
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

	// INSTRUCTION has activities[] (not data) — encode separately.
	if rmType == "INSTRUCTION" {
		return encodeInstruction(out, r, payload)
	}
	// ACTION has a description ITEM_TREE (not data) + time + ism_transition.
	if rmType == "ACTION" {
		return encodeAction(out, r, payload, startTime)
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
	case "EVALUATION", "ADMIN_ENTRY":
		itemsAttr := structuredItemsAttr(dataNode)
		if itemsAttr == nil {
			return nil, fmt.Errorf("%s data has no items", rmType)
		}
		items, err := encodeItems(itemsAttr, payload, r.terms)
		if err != nil {
			return nil, err
		}
		out["data"] = encodeStructuredContainer(dataNode, items, "Tree", r.terms)
	default:
		return nil, fmt.Errorf("RM entry type %q not supported", rmType)
	}
	return out, nil
}

// encodeInstruction builds an INSTRUCTION from a datamap payload: a mandatory
// narrative plus activities[], each an ACTIVITY whose description ITEM_TREE
// carries the encoded items (+ optional timing).
func encodeInstruction(out map[string]any, r contentRoot, payload map[string]any) (map[string]any, error) {
	out["narrative"] = dvText(termOrFallback(r.terms, "narrative", "Instruction"))

	actConstraint, ok := attrFirstObject(findAttr(r.node, "activities"))
	if !ok {
		return nil, fmt.Errorf("INSTRUCTION %s has no activities constraint", r.id)
	}
	var itemsAttr *template.Attribute
	var descConstraint template.ObjectNode
	if dc, ok := attrFirstObject(findAttr(actConstraint, "description")); ok {
		descConstraint = dc
		itemsAttr = structuredItemsAttr(dc)
	}

	actsPayload, _ := payload["activities"].([]any)
	acts := make([]any, 0, len(actsPayload))
	for i, a := range actsPayload {
		am, ok := a.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("activities[%d] is not an object", i)
		}
		items := []any{}
		if itemsAttr != nil {
			enc, err := encodeItems(itemsAttr, am, r.terms)
			if err != nil {
				return nil, fmt.Errorf("activities[%d]: %w", i, err)
			}
			items = enc
		}
		description := map[string]any{
			"_type":             "ITEM_TREE",
			"archetype_node_id": "",
			"name":              dvText("Tree"),
			"items":             items,
		}
		if descConstraint != nil {
			description = encodeStructuredContainer(descConstraint, items, "Tree", r.terms)
		}
		act := map[string]any{
			"_type":               "ACTIVITY",
			"archetype_node_id":   actConstraint.NodeID(),
			"name":                dvText(termOrFallback(r.terms, actConstraint.NodeID(), "Activity")),
			"action_archetype_id": "/.*/",
			"description":         description,
		}
		if t, ok := am["timing"].(string); ok && t != "" {
			act["timing"] = map[string]any{"_type": "DV_PARSABLE", "value": t, "formalism": "timing"}
		}
		acts = append(acts, act)
	}
	out["activities"] = acts
	return out, nil
}

// encodeAction builds an ACTION: its description ITEM_TREE items, a time, and a
// mandatory ism_transition (defaulting current_state to "completed").
func encodeAction(out map[string]any, r contentRoot, payload map[string]any, startTime string) (map[string]any, error) {
	descConstraint, ok := attrFirstObject(findAttr(r.node, "description"))
	if !ok {
		return nil, fmt.Errorf("ACTION %s has no description constraint", r.id)
	}
	items := []any{}
	if itemsAttr := structuredItemsAttr(descConstraint); itemsAttr != nil {
		enc, err := encodeItems(itemsAttr, payload, r.terms)
		if err != nil {
			return nil, err
		}
		items = enc
	}
	out["description"] = encodeStructuredContainer(descConstraint, items, "Tree", r.terms)
	out["time"] = dvDateTime(stringOrDefault(payload["time"], startTime))
	out["ism_transition"] = map[string]any{
		"_type":         "ISM_TRANSITION",
		"current_state": dvCodedText("completed", "openehr", "532"),
	}
	return out, nil
}

func encodeEvent(eventConstraint template.ObjectNode, payload map[string]any, terms map[string]string, fallbackTime string) (map[string]any, error) {
	t := stringOrDefault(payload["time"], fallbackTime)

	dataNode, ok := attrFirstObject(findAttr(eventConstraint, "data"))
	if !ok {
		return nil, fmt.Errorf("event constraint has no data")
	}
	itemsAttr := structuredItemsAttr(dataNode)
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
		"data":              encodeStructuredContainer(dataNode, items, "Tree", terms),
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
		// Unconstrained ELEMENT — sommige OPT-varianten laten de value-attr
		// los om type-keuze aan de caller te geven. In dat geval is een
		// expanded payload ({rmType, …}) de enige manier om te weten welk
		// DV-type we moeten bouwen. Kortere bare-vormen kunnen we niet
		// veilig wrappen zonder gokken op het type.
		if exp := encodeExpandedValue(valuePayload); exp != nil {
			return map[string]any{
				"_type":             "ELEMENT",
				"archetype_node_id": nodeID,
				"name":              dvText(terms[nodeID]),
				"value":             exp,
			}, nil
		}
		return nil, fmt.Errorf("ELEMENT %s has no value constraint; payload must be expanded {rmType:…}", nodeID)
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
	// Expanded notation: a self-describing object carrying an "rmType"
	// discriminator (the inverse of decodeElementValue's expanded branch).
	// Rename rmType→_type verbatim — the remaining fields are already the raw
	// RM attributes, so we don't run the per-type short-form coercions.
	if exp := encodeExpandedValue(payload); exp != nil {
		return exp, nil
	}
	switch rmType := constraint.RMTypeName(); rmType {
	case "DV_QUANTITY":
		return encodeQuantity(constraint, payload)
	case "DV_DATE_TIME", "DV_DATE", "DV_TIME", "DV_TEXT", "DV_BOOLEAN", "DV_URI", "DV_EHR_URI":
		return encodeScalarWrap(rmType, payload), nil
	case "DV_COUNT":
		return encodeCount(payload)
	case "DV_CODED_TEXT":
		return encodeCodedText(payload, terms)
	case "DV_ORDINAL":
		return encodeOrdinal(constraint, payload, terms)
	case "DV_PROPORTION":
		return encodeProportion(payload)
	default:
		return nil, fmt.Errorf("RM value type %q not supported", rmType)
	}
}

// encodeOrdinal rebuilds a DV_ORDINAL from its short form (the bare symbol code,
// e.g. "at0005" or "local::at0005"). The C_DV_ORDINAL constraint supplies the
// ordinal integer + symbol terminology for the matching code; the term map
// supplies the symbol display text. An object payload is passed through.
func encodeOrdinal(constraint template.ObjectNode, payload any, terms map[string]string) (map[string]any, error) {
	if m, ok := payload.(map[string]any); ok {
		out := map[string]any{"_type": "DV_ORDINAL"}
		for k, v := range m {
			out[k] = v
		}
		return out, nil
	}
	code, ok := payload.(string)
	if !ok {
		return nil, fmt.Errorf("DV_ORDINAL: expected string code or object, got %T", payload)
	}
	terminology, codeStr := splitTerminology(code)
	if co, ok := constraint.(*template.ComplexObject); ok {
		if ord, ok := co.PrimitiveConstraint().(constraints.CDvOrdinal); ok {
			for _, sym := range ord.Values {
				if sym.Symbol.CodeString != codeStr {
					continue
				}
				symTerm := sym.Symbol.Terminology
				if symTerm == "" {
					symTerm = terminology
				}
				label := terms[codeStr]
				if label == "" {
					label = codeStr
				}
				return map[string]any{
					"_type":   "DV_ORDINAL",
					"value":   sym.Value,
					"symbol":  dvCodedText(label, symTerm, codeStr),
					"ordinal": sym.Value,
				}, nil
			}
		}
	}
	// Constraint did not resolve the code — emit a best-effort ordinal carrying
	// the symbol so the value is still a valid DV_ORDINAL.
	label := terms[codeStr]
	if label == "" {
		label = codeStr
	}
	return map[string]any{
		"_type":  "DV_ORDINAL",
		"value":  0,
		"symbol": dvCodedText(label, terminology, codeStr),
	}, nil
}

// encodeProportion rebuilds a DV_PROPORTION. An object payload (numerator,
// denominator, type) is passed through verbatim; a bare number is wrapped as a
// ratio (type 0) with denominator 1 so its magnitude equals the short value.
func encodeProportion(payload any) (map[string]any, error) {
	if m, ok := payload.(map[string]any); ok {
		out := map[string]any{"_type": "DV_PROPORTION"}
		for k, v := range m {
			out[k] = v
		}
		return out, nil
	}
	num, ok := toFloat(payload)
	if !ok {
		return nil, fmt.Errorf("DV_PROPORTION: expected number or object, got %T", payload)
	}
	return map[string]any{
		"_type":       "DV_PROPORTION",
		"numerator":   num,
		"denominator": float64(1),
		"type":        0,
	}, nil
}

// encodeExpandedValue converts the expanded value notation
// ({"rmType": "...", ...attrs}) back into an RM value object ({"_type": ...}).
// Returns nil when payload is not an expanded object (so callers fall through
// to the short-form coercions).
func encodeExpandedValue(payload any) map[string]any {
	m, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	rt, ok := m["rmType"].(string)
	if !ok || rt == "" {
		return nil
	}
	out := map[string]any{"_type": rt}
	for k, v := range m {
		if k == "rmType" {
			continue
		}
		out[k] = v
	}
	return out
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

// encodeStructuredContainer builds an ITEM_STRUCTURE RM object, preserving the
// constraint's real subtype and placing the encoded children under the matching
// attribute: ITEM_TREE/ITEM_LIST → "items", ITEM_TABLE → "rows", ITEM_SINGLE →
// a single "item". Defaults to ITEM_TREE when the constraint has no RM type.
func encodeStructuredContainer(container template.ObjectNode, items []any, fallbackName string, terms map[string]string) map[string]any {
	rmType := container.RMTypeName()
	if rmType == "" {
		rmType = "ITEM_TREE"
	}
	out := map[string]any{
		"_type":             rmType,
		"archetype_node_id": container.NodeID(),
		"name":              dvText(termOrFallback(terms, container.NodeID(), fallbackName)),
	}
	switch rmType {
	case "ITEM_SINGLE":
		if len(items) > 0 {
			out["item"] = items[0]
		}
	case "ITEM_TABLE":
		out["rows"] = items
	default:
		out["items"] = items
	}
	return out
}

// clusterName builds a CLUSTER's runtime name from its Datamap-V2 payload.
//
// `_code` accepts two interchangeable shapes per the Datamap V2 spec
// ([docs/specifications/datamap.md § Terminology binding]):
//
//   - Short form: a string `"<terminology>::<code>"`, or `"at*"` (local
//     at-code), or any other bare string (defaults to `local`).
//   - Expanded form: a map `{ "code": "...", "value": "...", "terminology": "..." }`
//     where `terminology` defaults to `local` when absent.
//
// `_name` is the optional display string; falls back to the template label
// or — for the expanded form — to the inner `value`.
//
// Returns a DV_CODED_TEXT map when `_code` carries a non-empty code; a
// plain DV_TEXT with the template label otherwise.
func clusterName(payload map[string]any, label string) map[string]any {
	terminology, codeStr, expandedDisplay, ok := parseCodeField(payload["_code"])
	if !ok {
		return dvText(label)
	}
	display, _ := payload["_name"].(string)
	if display == "" {
		display = expandedDisplay
	}
	if display == "" {
		display = label
	}
	return map[string]any{
		"_type":         "DV_CODED_TEXT",
		"value":         display,
		"defining_code": codePhrase(terminology, codeStr),
	}
}

// parseCodeField extracts (terminology, code, display, ok) from a `_code`
// payload. Accepts both wire-shapes documented in REQ-058. `display`
// carries the expanded form's `value` (string when present, else empty);
// the caller decides how to combine it with sibling `_name`. `ok` is
// false when no usable code could be extracted (nil, empty, malformed).
func parseCodeField(raw any) (terminology, code, display string, ok bool) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return "", "", "", false
		}
		t, c := splitTerminology(v)
		return t, c, "", true
	case map[string]any:
		code, _ = v["code"].(string)
		if code == "" {
			return "", "", "", false
		}
		display, _ = v["value"].(string)
		terminology, _ = v["terminology"].(string)
		if terminology == "" {
			terminology = "local"
		}
		return terminology, code, display, true
	default:
		return "", "", "", false
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
