package datamap

import (
	"maps"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// schema.go — Datamap V2 JSON Schema builder (REQ-058). Walks an operational
// template (openehr/template) and emits the datamap schema that describes the
// flat-ish JSON a consumer fills in. Ported from the Cadasto dmv2 SchemaBuilder;
// emits map[string]any (key order is not significant — compare structurally).

// Schema builds the datamap JSON Schema for an operational template.
// Demographic PARTY templates (PERSON, ORGANISATION, AGENT, GROUP, ROLE, …)
// emit the party profile schema; clinical COMPOSITION templates emit the
// composition profile (REQ-058 Option B).
func Schema(opt *template.OperationalTemplate) map[string]any {
	if IsPartyTemplate(opt) {
		return buildPartySchemaObject(opt)
	}
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
	id         string
	label      string
	terms      map[string]string // at-code -> text
	descs      map[string]string // at-code -> description
	node       template.ObjectNode
	arrayNodes map[string]bool // at-code -> true when occurrences allow >1 (array-valued)
	expanded   bool            // FromComposition: emit expanded ({rmType,…}) instead of short scalars
	// opt enables per-archetype term re-scoping during decode: an at-code such
	// as at0010 means different things in different archetypes (person_name →
	// "Volledige naam", person_details → "Geboortedatum"), so when the decoder
	// enters an archetype subtree it must use THAT archetype's terms, not an
	// ancestor's merged dictionary. nil = no re-scoping (labels stay as-is).
	opt *template.OperationalTemplate
}

// collectArrayNodes records every descendant at-code whose occurrences allow
// more than one instance — these are array-valued in the datamap (matching the
// Schema's arraySchema wrapping), even when only a single instance is present.
func collectArrayNodes(n template.ObjectNode, out map[string]bool) {
	if n == nil {
		return
	}
	for _, a := range n.Attributes() {
		for _, c := range a.Children() {
			obj, ok := c.(template.ObjectNode)
			if !ok {
				continue
			}
			occ := fromMultiplicity(obj.Occurrences())
			if nid := obj.NodeID(); nid != "" && (occ.upperUnbounded || (occ.upper != nil && *occ.upper > 1)) {
				out[nid] = true
			}
			collectArrayNodes(obj, out)
		}
	}
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
		arrayNodes := map[string]bool{}
		collectArrayNodes(ar, arrayNodes)
		r := contentRoot{id: ar.ArchetypeID(), terms: terms, descs: descs, node: ar, arrayNodes: arrayNodes}
		// Name the runtime content node from the ArchetypeRoot's actual
		// node_id, not a hardcoded "at0000": a template may specialise the
		// root (e.g. at0000.1) and rename it (the OPT's term for the
		// specialised code is the value the CDR validates the COMPOSITION's
		// /content[...]/name against). Hardcoding at0000 picked the parent
		// archetype term (e.g. "*Healthcare service request(en)") and made
		// Cadasto reject the composition with a 422 name-mismatch.
		rootCode := ar.NodeID()
		if rootCode == "" {
			rootCode = "at0000"
		}
		if lbl, ok := terms[rootCode]; ok && lbl != "" {
			r.label = lbl
		} else if lbl, ok := terms["at0000"]; ok && lbl != "" {
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

	// INSTRUCTION: activities[] (each ACTIVITY carries a description ITEM_TREE).
	if activitiesAttr := findAttr(r.node, "activities"); activitiesAttr != nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"activities": buildActivitiesSchema(activitiesAttr, r, contentPath),
			},
		}
	}

	// ACTION: a description ITEM_TREE (not `data`) plus a time.
	if descAttr := findAttr(r.node, "description"); descAttr != nil {
		props := map[string]any{"time": shortSchema("string", "date-time", nil, "")}
		if descChild, ok := attrFirstObject(descAttr); ok {
			if itemsAttr := structuredItemsAttr(descChild); itemsAttr != nil {
				itemsPath := contentPath + "/description[" + descChild.NodeID() + "]"
				maps.Copy(props, buildItemsSchema(itemsAttr, r, itemsPath))
			}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	}

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
		if itemsAttr := structuredItemsAttr(dataChild); itemsAttr != nil {
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

// buildActivitiesSchema models an INSTRUCTION's activities[] — each item is the
// activity's description ITEM_TREE items plus an optional timing string.
func buildActivitiesSchema(activitiesAttr *template.Attribute, r contentRoot, parentPath string) map[string]any {
	schema := map[string]any{"type": "array"}
	act, ok := attrFirstObject(activitiesAttr)
	if !ok {
		return schema
	}
	occ := fromMultiplicity(act.Occurrences())
	schema["items"] = buildActivityItemSchema(act, r, parentPath)
	if occ.lower != nil {
		schema["minItems"] = *occ.lower
	}
	if !occ.upperUnbounded && occ.upper != nil {
		schema["maxItems"] = *occ.upper
	}
	return schema
}

func buildActivityItemSchema(act template.ObjectNode, r contentRoot, parentPath string) map[string]any {
	actPath := parentPath + "/activities[" + act.NodeID() + "]"
	props := map[string]any{
		"timing": makeDescField("string", "ISO 8601 timing expression (optional)"),
	}
	if descAttr := findAttr(act, "description"); descAttr != nil {
		if descChild, ok := attrFirstObject(descAttr); ok {
			if itemsAttr := structuredItemsAttr(descChild); itemsAttr != nil {
				itemsPath := actPath + "/description[" + descChild.NodeID() + "]"
				maps.Copy(props, buildItemsSchema(itemsAttr, r, itemsPath))
			}
		}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
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
			if itemsAttr := structuredItemsAttr(dataChild); itemsAttr != nil {
				itemsPath := eventPath + "/data[" + dataChild.NodeID() + "]"
				maps.Copy(props, buildItemsSchema(itemsAttr, r, itemsPath))
			}
		}
	}

	// time is NOT required: not every stored event carries a decodable time,
	// and ToComposition fills a fallback (the composition start_time) when absent.
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
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
			key := obj.NodeID()
			if alias := formatNodeAlias(obj.NodeID(), r.terms); alias != "" {
				key = alias
			}
			required = append(required, key)
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
		// Property key is the labelled "atNNNN|Label" form (SPEC §4.3 datamap
		// examples; matches the content-root convention). FromComposition emits
		// the same labelled key so decoded data validates.
		key := nodeID
		if alias != "" {
			key = alias
		}
		occ := fromMultiplicity(obj.Occurrences())
		wrapArray := occ.upperUnbounded || (occ.upper != nil && *occ.upper > 1)

		switch rmType {
		case "CLUSTER":
			clusterProps := map[string]any{}
			if subItems := findAttr(obj, "items"); subItems != nil {
				clusterProps = buildItemsSchema(subItems, r, nodePath)
			}
			// Optional runtime-name slots for instance-named clusters (e.g. a
			// repeating lab "result group" named per determination). Decode
			// emits these only when the composition carries a coded name.
			//
			// `_code` accepts both wire-shapes documented in REQ-058:
			// short-form string ("SNOMED-CT::386725007") or expanded object
			// ({code,value,terminology}). The encoder normalises both into
			// the same canonical DV_CODED_TEXT.
			clusterProps["_code"] = map[string]any{
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"code":        map[string]any{"type": "string"},
							"value":       map[string]any{"type": "string"},
							"terminology": map[string]any{"type": "string"},
						},
						"required": []any{"code"},
					},
				},
			}
			clusterProps["_name"] = shortSchema("string", "", nil, "")
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
				props[key] = arr
			} else {
				withOccurrences(schema, occ)
				props[key] = schema
			}

		case "ELEMENT":
			// An ELEMENT value may constrain MULTIPLE RM types (a choice, e.g.
			// a lab "Result value" allowing DV_QUANTITY or DV_TEXT). Emit a
			// oneOf over every allowed type so any of them validates.
			var valueNodes []template.ObjectNode
			if valueAttr := findAttr(obj, "value"); valueAttr != nil {
				for _, c := range valueAttr.Children() {
					// Skip the abstract DATA_VALUE base — it means the value is
					// unconstrained (any data type), handled permissively below.
					if vn, ok := c.(template.ObjectNode); ok {
						if t := vn.RMTypeName(); t != "" && t != "DATA_VALUE" {
							valueNodes = append(valueNodes, vn)
						}
					}
				}
			}
			var options []optChoice
			var schema map[string]any
			if len(valueNodes) == 0 {
				// Unconstrained value (any DATA_VALUE) — accept scalar or object.
				schema = map[string]any{"description": "unconstrained value (any data type)"}
			} else if len(valueNodes) == 1 {
				rmTypeOfValue := "DV_TEXT"
				if len(valueNodes) == 1 {
					rmTypeOfValue = valueNodes[0].RMTypeName()
					options = buildOptions(collectCodes(valueNodes[0]), r.terms)
				}
				schema = valueSchema(rmTypeOfValue, options)
			} else {
				// Multi-type value: a short branch per allowed RM type, plus a
				// single permissive object that accepts any expanded form.
				branches := make([]any, 0, len(valueNodes)+1)
				for _, vn := range valueNodes {
					opts := buildOptions(collectCodes(vn), r.terms)
					options = append(options, opts...)
					branches = append(branches, shortSchemaForType(vn.RMTypeName(), opts))
				}
				branches = append(branches, map[string]any{"type": "object", "additionalProperties": true})
				schema = map[string]any{"oneOf": branches}
			}
			ui := buildUi(nodePath+"/value", r.descs[nodeID], options)
			if wrapArray {
				arr := arraySchema(schema, occ)
				arr["ui"] = ui
				if alias != "" {
					arr["alias"] = alias
				}
				props[key] = arr
			} else {
				schema["ui"] = ui
				withOccurrences(schema, occ)
				if alias != "" {
					schema["alias"] = alias
				}
				props[key] = schema
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

// structuredItemsAttr returns the child-holding attribute of an ITEM_STRUCTURE
// container, normalising across the subtypes: ITEM_TREE/ITEM_LIST expose their
// ELEMENT/CLUSTER children under "items", ITEM_TABLE under "rows" (each a
// CLUSTER row), ITEM_SINGLE under "item" (a single ELEMENT). Returns nil when
// none is present. The downstream items-walkers iterate Children() uniformly,
// so a single-child "item" attribute works without special-casing.
func structuredItemsAttr(container template.ObjectNode) *template.Attribute {
	for _, name := range []string{"items", "rows", "item"} {
		if a := findAttr(container, name); a != nil {
			return a
		}
	}
	return nil
}

func contextChildren(a *template.Attribute) []template.Node {
	if a == nil {
		return nil
	}
	return a.Children()
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
	// The RM COMPOSITION always carries an EVENT_CONTEXT and FromComposition
	// always emits a "context" key, so the schema must always model it — even
	// when the OPT adds no context constraints. The OPT's context attr (when
	// present) only extends the base block with other_context items.
	contextAttr := findAttr(root, "context")

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

	for _, c := range contextChildren(contextAttr) {
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
