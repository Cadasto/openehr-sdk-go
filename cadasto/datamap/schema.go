package datamap

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// schema.go — Datamap V2 JSON Schema builder (REQ-058). Walks an operational
// template (openehr/template) and emits the datamap schema that describes the
// flat-ish JSON a consumer fills in. Ported from the Cadasto dmv2 SchemaBuilder;
// emits map[string]any (key order is not significant — compare structurally).

// Schema builds the datamap JSON Schema for an operational template.
func Schema(opt *template.OperationalTemplate) map[string]any {
	return buildSchemaObject(opt)
}

func buildSchemaObject(opt *template.OperationalTemplate) map[string]any {
	root, _ := opt.Root().(template.ObjectNode)
	roots := findContentArchetypeRoots(root)

	var compTerms, compDescs map[string]string
	if ar, ok := opt.Root().(*template.ArchetypeRoot); ok {
		compTerms, compDescs = termMaps(ar)
	} else {
		compTerms, compDescs = map[string]string{}, map[string]string{}
	}

	props := map[string]any{
		"template_id": map[string]any{"type": "string"},
		"uid":         map[string]any{"type": "string"},
		"vuid":        map[string]any{"type": "string"},
		"composer":    map[string]any{"type": "string"},
		"language":    makeField("string", "ISO 639-1 language code"),
		"territory":   makeField("string", "ISO 3166-1 territory code"),
		"content":     buildContentSchema(roots),
	}
	if ctx := buildContextSchema(root, compTerms, compDescs); ctx != nil {
		props["context"] = ctx
	}

	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                opt.TemplateID() + " DMv2 datamap",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"content"},
		"properties":           props,
	}
}

// contentRoot is a discovered archetype-root under the COMPOSITION's content.
type contentRoot struct {
	id    string
	label string
	terms map[string]string // at-code -> text
	descs map[string]string // at-code -> description
	node  template.ObjectNode
}

func termMaps(ar *template.ArchetypeRoot) (text, desc map[string]string) {
	text = map[string]string{}
	desc = map[string]string{}
	for code, t := range ar.Terms() {
		if v, ok := t.Items["text"]; ok {
			text[code] = strings.TrimSpace(v)
		}
		if v, ok := t.Items["description"]; ok {
			desc[code] = strings.TrimSpace(v)
		}
	}
	return text, desc
}

func findContentArchetypeRoots(root template.ObjectNode) []contentRoot {
	var out []contentRoot
	contentAttr := findAttr(root, "content")
	if contentAttr == nil {
		return out
	}
	for _, c := range contentAttr.Children() {
		ar, ok := c.(*template.ArchetypeRoot)
		if !ok {
			continue
		}
		terms, descs := termMaps(ar)
		r := contentRoot{id: ar.ArchetypeID(), terms: terms, descs: descs, node: ar}
		if lbl, ok := terms["at0000"]; ok && lbl != "" {
			r.label = lbl
		} else if parts := strings.Split(r.id, "."); len(parts) > 0 {
			r.label = parts[len(parts)-1]
		}
		out = append(out, r)
	}
	return out
}

func buildContentSchema(roots []contentRoot) map[string]any {
	props := map[string]any{}
	var required []string
	for _, r := range roots {
		key := r.id + "|" + r.label
		occ := fromMultiplicity(r.node.Occurrences())
		schema := buildArchetypeRootSchema(r)
		withOccurrences(schema, occ)
		props[key] = schema
		if occ.lower != nil && *occ.lower > 0 {
			required = append(required, key)
		}
	}
	out := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func buildArchetypeRootSchema(r contentRoot) map[string]any {
	contentPath := "content[" + r.id + "]"
	dataAttr := findAttr(r.node, "data")
	dataPath := contentPath
	var dataChild template.ObjectNode
	if dataAttr != nil {
		if c, ok := attrFirstObject(dataAttr); ok {
			dataChild = c
			if c.NodeID() != "" {
				dataPath = contentPath + "/data[" + c.NodeID() + "]"
			}
		}
	}

	if dataChild != nil {
		if eventsAttr := findAttr(dataChild, "events"); eventsAttr != nil {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"events"},
				"properties": map[string]any{
					"events": buildEventsSchema(eventsAttr, r, dataPath),
				},
			}
		}
		if itemsAttr := findAttr(dataChild, "items"); itemsAttr != nil {
			items, required := buildItemsSchemaWithRequired(itemsAttr, r, dataPath)
			out := map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           items,
			}
			if len(required) > 0 {
				out["required"] = required
			}
			return out
		}
	}
	return map[string]any{"type": "object", "additionalProperties": false}
}

func buildEventsSchema(eventsAttr *template.Attribute, r contentRoot, parentPath string) map[string]any {
	schema := map[string]any{"type": "array"}
	eventNode, ok := attrFirstObject(eventsAttr)
	if !ok {
		return schema
	}
	occ := fromMultiplicity(eventNode.Occurrences())
	schema["items"] = buildEventItemSchema(eventNode, r, parentPath)
	if occ.lower != nil {
		schema["minItems"] = *occ.lower
	}
	if !occ.upperUnbounded && occ.upper != nil {
		schema["maxItems"] = *occ.upper
	}
	withOccurrences(schema, occ)
	return schema
}

func buildEventItemSchema(eventNode template.ObjectNode, r contentRoot, parentPath string) map[string]any {
	eventPath := parentPath + "/events[" + eventNode.NodeID() + "]"

	props := map[string]any{}

	timeSchema := shortSchema("string", "date-time", nil, "")
	timeSchema["ui"] = buildUi(eventPath+"/time", r.descs[eventNode.NodeID()], nil)
	props["time"] = timeSchema

	props["width"] = makeDescField("string", "ISO 8601 duration (INTERVAL_EVENT only)")

	mfCoded := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"code"},
		"properties": map[string]any{
			"code":        shortSchema("string", "", nil, ""),
			"value":       shortSchema("string", "", nil, ""),
			"terminology": shortSchema("string", "", nil, ""),
			"rmType":      constField("DV_CODED_TEXT"),
		},
	}
	props["math_function"] = map[string]any{
		"oneOf":       []any{shortSchema("string", "", nil, ""), mfCoded},
		"description": "Mathematical function (INTERVAL_EVENT only)",
	}
	props["sample_count"] = makeDescField("integer", "Number of samples (INTERVAL_EVENT only)")

	if dataAttr := findAttr(eventNode, "data"); dataAttr != nil {
		if dataChild, ok := attrFirstObject(dataAttr); ok {
			if itemsAttr := findAttr(dataChild, "items"); itemsAttr != nil {
				itemsPath := eventPath + "/data[" + dataChild.NodeID() + "]"
				for k, v := range buildItemsSchema(itemsAttr, r, itemsPath) {
					props[k] = v
				}
			}
		}
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"time"},
		"properties":           props,
	}
}

func buildItemsSchemaWithRequired(itemsAttr *template.Attribute, r contentRoot, parentPath string) (map[string]any, []string) {
	var required []string
	for _, child := range itemsAttr.Children() {
		obj, ok := child.(template.ObjectNode)
		if !ok || obj.RMTypeName() == "" || obj.NodeID() == "" {
			continue
		}
		occ := fromMultiplicity(obj.Occurrences())
		if occ.lower != nil && *occ.lower > 0 {
			required = append(required, obj.NodeID())
		}
	}
	return buildItemsSchema(itemsAttr, r, parentPath), required
}

func buildItemsSchema(itemsAttr *template.Attribute, r contentRoot, parentPath string) map[string]any {
	props := map[string]any{}
	for _, child := range itemsAttr.Children() {
		obj, ok := child.(template.ObjectNode)
		if !ok {
			continue
		}
		rmType := obj.RMTypeName()
		nodeID := obj.NodeID()
		if rmType == "" || nodeID == "" {
			continue
		}
		nodePath := parentPath + "/items[" + nodeID + "]"
		alias := formatNodeAlias(nodeID, r.terms)
		occ := fromMultiplicity(obj.Occurrences())
		wrapArray := occ.upperUnbounded || (occ.upper != nil && *occ.upper > 1)

		switch rmType {
		case "CLUSTER":
			clusterProps := map[string]any{}
			if subItems := findAttr(obj, "items"); subItems != nil {
				clusterProps = buildItemsSchema(subItems, r, nodePath)
			}
			schema := map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           clusterProps,
			}
			if alias != "" {
				schema["alias"] = alias
			}
			if wrapArray {
				arr := arraySchema(schema, occ)
				arr["ui"] = buildUi(nodePath, r.descs[nodeID], nil)
				if alias != "" {
					arr["alias"] = alias
				}
				props[nodeID] = arr
			} else {
				withOccurrences(schema, occ)
				props[nodeID] = schema
			}

		case "ELEMENT":
			var valueNode template.ObjectNode
			if valueAttr := findAttr(obj, "value"); valueAttr != nil {
				if vn, ok := attrFirstObject(valueAttr); ok {
					valueNode = vn
				}
			}
			rmTypeOfValue := "DV_TEXT"
			var codes []string
			if valueNode != nil {
				rmTypeOfValue = valueNode.RMTypeName()
				codes = collectCodes(valueNode)
			}
			options := buildOptions(codes, r.terms)
			schema := valueSchema(rmTypeOfValue, options)
			ui := buildUi(nodePath+"/value", r.descs[nodeID], options)
			if wrapArray {
				arr := arraySchema(schema, occ)
				arr["ui"] = ui
				if alias != "" {
					arr["alias"] = alias
				}
				props[nodeID] = arr
			} else {
				schema["ui"] = ui
				withOccurrences(schema, occ)
				if alias != "" {
					schema["alias"] = alias
				}
				props[nodeID] = schema
			}
		}
	}
	return props
}

// --- value-node code collection ---

func collectCodes(n template.ObjectNode) []string {
	var out []string
	var walk func(node template.Node)
	walk = func(node template.Node) {
		obj, ok := node.(template.ObjectNode)
		if !ok {
			return
		}
		if co, ok := obj.(*template.ComplexObject); ok {
			switch cp := co.PrimitiveConstraint().(type) {
			case constraints.CodePhrase:
				out = append(out, cp.CodeList...)
			case *constraints.CodePhrase:
				out = append(out, cp.CodeList...)
			}
		}
		for _, a := range obj.Attributes() {
			for _, c := range a.Children() {
				walk(c)
			}
		}
	}
	walk(n)

	seen := map[string]bool{}
	var dedup []string
	for _, c := range out {
		if !seen[c] {
			seen[c] = true
			dedup = append(dedup, c)
		}
	}
	return dedup
}

func buildOptions(codes []string, terms map[string]string) []optChoice {
	if len(codes) == 0 {
		return nil
	}
	out := make([]optChoice, len(codes))
	for i, c := range codes {
		text := terms[c]
		if text == "" {
			text = c
		}
		out[i] = optChoice{code: c, text: text}
	}
	return out
}

// --- small shared helpers ---

func findAttr(n template.ObjectNode, name string) *template.Attribute {
	if n == nil {
		return nil
	}
	for _, a := range n.Attributes() {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

func attrFirstObject(a *template.Attribute) (template.ObjectNode, bool) {
	if a == nil {
		return nil, false
	}
	for _, c := range a.Children() {
		if o, ok := c.(template.ObjectNode); ok {
			return o, true
		}
	}
	return nil, false
}

func formatNodeAlias(nodeID string, terms map[string]string) string {
	if text := terms[nodeID]; text != "" {
		return nodeID + "|" + text
	}
	return ""
}

func buildUi(path, description string, options []optChoice) map[string]any {
	ui := map[string]any{"path": path}
	if description != "" {
		ui["description"] = description
	}
	if len(options) > 0 {
		opts := make([]any, len(options))
		for i, o := range options {
			opts[i] = map[string]any{"code": o.code, "text": o.text}
		}
		ui["options"] = opts
	}
	return ui
}

func makeField(typ, desc string) map[string]any {
	o := map[string]any{"type": typ}
	if desc != "" {
		o["description"] = desc
	}
	return o
}

func makeDescField(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// buildContextSchema mirrors the dmv2 SchemaBuilder context block: a fixed set
// of standard EVENT_CONTEXT fields, optionally extended with the OPT's
// other_context items. Returns nil when the COMPOSITION declares no context.
func buildContextSchema(root template.ObjectNode, compTerms, compDescs map[string]string) map[string]any {
	contextAttr := findAttr(root, "context")
	if contextAttr == nil {
		return nil
	}

	props := map[string]any{
		"start_time": shortSchema("string", "date-time", nil, ""),
		"end_time":   map[string]any{"type": "string", "format": "date-time"},
		"location":   shortSchema("string", "", nil, ""),
	}

	settingExp := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"code"},
		"properties": map[string]any{
			"code":        shortSchema("string", "", nil, ""),
			"value":       shortSchema("string", "", nil, ""),
			"terminology": shortSchema("string", "", nil, ""),
			"rmType":      constField("DV_CODED_TEXT"),
		},
	}
	props["setting"] = map[string]any{"oneOf": []any{shortSchema("string", "", nil, ""), settingExp}}

	idItem := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":       shortSchema("string", "", nil, ""),
			"issuer":   shortSchema("string", "", nil, ""),
			"assigner": shortSchema("string", "", nil, ""),
			"type":     shortSchema("string", "", nil, ""),
		},
	}
	props["health_care_facility"] = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name":        shortSchema("string", "", nil, ""),
			"identifiers": map[string]any{"type": "array", "items": idItem},
		},
	}

	pItem := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"function":  shortSchema("string", "", nil, ""),
			"performer": map[string]any{"type": "object", "properties": map[string]any{"name": shortSchema("string", "", nil, "")}},
			"mode":      shortSchema("string", "", nil, ""),
			"time":      shortSchema("string", "", nil, ""),
		},
	}
	props["participations"] = map[string]any{"type": "array", "items": pItem}

	for _, c := range contextAttr.Children() {
		ctxObj, ok := c.(template.ObjectNode)
		if !ok {
			continue
		}
		ocAttr := findAttr(ctxObj, "other_context")
		if ocAttr == nil {
			continue
		}
		inner, ok := attrFirstObject(ocAttr)
		if !ok {
			break
		}
		path := "context/other_context"
		if inner.NodeID() != "" {
			path = "context/other_context[" + inner.NodeID() + "]"
		}
		if itemsAttr := findAttr(inner, "items"); itemsAttr != nil {
			r := contentRoot{terms: compTerms, descs: compDescs, node: inner}
			props["other_context"] = map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           buildItemsSchema(itemsAttr, r, path),
			}
		}
		break
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
}
