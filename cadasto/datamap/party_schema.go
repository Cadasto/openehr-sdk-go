package datamap

import (
	"maps"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// partySection carries OPT term maps and array-node metadata for one demographic
// subtree (identities entry, details ITEM_TREE, address archetype root, …).
type partySection struct {
	id         string
	label      string
	terms      map[string]string
	descs      map[string]string
	node       template.ObjectNode
	arrayNodes map[string]bool
}

func partySectionFromNode(node template.ObjectNode) partySection {
	terms, descs := map[string]string{}, map[string]string{}
	if ar, ok := node.(*template.ArchetypeRoot); ok {
		terms, descs = termMaps(ar)
	}
	arrayNodes := map[string]bool{}
	collectArrayNodes(node, arrayNodes)
	return partySection{
		id:         sectionID(node),
		label:      sectionLabel(node, terms),
		terms:      terms,
		descs:      descs,
		node:       node,
		arrayNodes: arrayNodes,
	}
}

func sectionID(node template.ObjectNode) string {
	if ar, ok := node.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
		return ar.ArchetypeID()
	}
	if nid := node.NodeID(); nid != "" {
		return nid
	}
	return node.RMTypeName()
}

func sectionLabel(node template.ObjectNode, terms map[string]string) string {
	if ar, ok := node.(*template.ArchetypeRoot); ok {
		if t, ok := ar.Term("at0000"); ok {
			if v := t.Items["text"]; v != "" {
				return v
			}
		}
	}
	if nid := node.NodeID(); nid != "" {
		if lbl := terms[nid]; lbl != "" {
			return lbl
		}
	}
	return node.RMTypeName()
}

func buildPartySchemaObject(opt *template.OperationalTemplate) map[string]any {
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		return map[string]any{"type": "object"}
	}
	sec := partySectionFromNode(root)

	props := map[string]any{
		"template_id": map[string]any{"type": "string"},
		"uid":         map[string]any{"type": "string"},
		"vuid":        map[string]any{"type": "string"},
		"name":        makeField("string", "Party display name"),
	}

	if idSchema := buildPartyIdentitiesSchema(root); idSchema != nil {
		props["identities"] = idSchema
	}
	if detSchema := buildPartyDetailsSchema(root, sec); detSchema != nil {
		props["details"] = detSchema
	}
	if conSchema := buildPartyContactsSchema(root); conSchema != nil {
		props["contacts"] = conSchema
	}
	// relationships: always accept. The OPT often has no relationships
	// constraint, but Cadasto returns PARTY_RELATIONSHIPs (source/target refs)
	// which FromParty decodes — so a lenient array keeps decoded data valid.
	if relSchema := buildPartyRelationshipsSchema(root); relSchema != nil {
		props["relationships"] = relSchema
	} else if findAttr(root, "relationships") != nil {
		props["relationships"] = map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "object", "additionalProperties": true},
		}
	}
	if findAttr(root, "languages") != nil {
		props["languages"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	}
	if findAttr(root, "roles") != nil {
		props["roles"] = buildPartyRefsSchema()
	}
	if findAttr(root, "capabilities") != nil {
		props["capabilities"] = buildPartyCapabilitiesSchema(root)
	}

	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                opt.TemplateID() + " DMv2 party datamap",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
}

func buildPartyIdentitiesSchema(root template.ObjectNode) map[string]any {
	idAttr := findAttr(root, "identities")
	if idAttr == nil {
		return nil
	}
	props := map[string]any{}
	var required []string
	for _, c := range idAttr.Children() {
		ar, ok := c.(*template.ArchetypeRoot)
		if !ok {
			continue
		}
		sec := partySectionFromNode(ar)
		if details, ok := attrFirstObject(findAttr(ar, "details")); ok {
			if itemsAttr := structuredItemsAttr(details); itemsAttr != nil {
				items, req := buildItemsSchemaWithRequired(itemsAttr, contentRootFromParty(sec), "identities")
				schema := map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties":           items,
				}
				if len(req) > 0 {
					schema["required"] = req
				}
				key := sec.id + "|" + sec.label
				props[key] = schema
				occ := fromMultiplicity(ar.Occurrences())
				if occ.lower != nil && *occ.lower > 0 {
					required = append(required, key)
				}
			}
		}
	}
	if len(props) == 0 {
		return nil
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

func buildPartyDetailsSchema(root template.ObjectNode, sec partySection) map[string]any {
	detAttr := findAttr(root, "details")
	if detAttr == nil {
		return nil
	}
	details, ok := attrFirstObject(detAttr)
	if !ok {
		return nil
	}
	if itemsAttr := structuredItemsAttr(details); itemsAttr != nil {
		items, req := buildItemsSchemaWithRequired(itemsAttr, contentRootFromParty(sec), "details")
		out := map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           items,
		}
		if len(req) > 0 {
			out["required"] = req
		}
		return out
	}
	return nil
}

func buildPartyContactsSchema(root template.ObjectNode) map[string]any {
	conAttr := findAttr(root, "contacts")
	if conAttr == nil {
		return nil
	}
	contactNode, ok := attrFirstObject(conAttr)
	if !ok {
		return nil
	}
	sec := partySectionFromNode(contactNode)
	addrProps := map[string]any{}
	addrAttr := findAttr(contactNode, "addresses")
	if addrAttr != nil {
		for _, c := range addrAttr.Children() {
			ar, ok := c.(*template.ArchetypeRoot)
			if !ok {
				continue
			}
			addrSec := partySectionFromNode(ar)
			if details, ok := attrFirstObject(findAttr(ar, "details")); ok {
				if itemsAttr := structuredItemsAttr(details); itemsAttr != nil {
					items, _ := buildItemsSchemaWithRequired(itemsAttr, contentRootFromParty(addrSec), "contacts/addresses")
					key := addrSec.id + "|" + addrSec.label
					addrProps[key] = map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties":           items,
					}
				}
			}
		}
	}
	contactKey := sec.id + "|" + sec.label
	itemSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			contactKey: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           addrProps,
			},
		},
	}
	return map[string]any{"type": "array", "items": itemSchema}
}

func buildPartyRelationshipsSchema(root template.ObjectNode) map[string]any {
	relAttr := findAttr(root, "relationships")
	if relAttr == nil || len(relAttr.Children()) == 0 {
		return nil
	}
	props := map[string]any{
		"source": buildPartyRefsSchema(),
		"target": buildPartyRefsSchema(),
	}
	for _, c := range relAttr.Children() {
		obj, ok := c.(template.ObjectNode)
		if !ok {
			continue
		}
		sec := partySectionFromNode(obj)
		if itemsAttr := structuredItemsAttr(obj); itemsAttr != nil {
			maps.Copy(props, buildItemsSchema(itemsAttr, contentRootFromParty(sec), "relationships"))
		} else if det, ok := attrFirstObject(findAttr(obj, "details")); ok {
			if itemsAttr := structuredItemsAttr(det); itemsAttr != nil {
				maps.Copy(props, buildItemsSchema(itemsAttr, contentRootFromParty(sec), "relationships/details"))
			}
		}
	}
	return map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "object", "additionalProperties": false, "properties": props},
	}
}

func buildPartyCapabilitiesSchema(root template.ObjectNode) map[string]any {
	capAttr := findAttr(root, "capabilities")
	if capAttr == nil {
		return nil
	}
	capNode, ok := attrFirstObject(capAttr)
	if !ok {
		return nil
	}
	sec := partySectionFromNode(capNode)
	if cred, ok := attrFirstObject(findAttr(capNode, "credentials")); ok {
		if itemsAttr := structuredItemsAttr(cred); itemsAttr != nil {
			items, _ := buildItemsSchemaWithRequired(itemsAttr, contentRootFromParty(sec), "capabilities")
			return map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "object", "additionalProperties": false, "properties": items},
			}
		}
	}
	return map[string]any{"type": "array", "items": map[string]any{"type": "object"}}
}

func buildPartyRefsSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":        shortSchema("string", "", nil, ""),
			"namespace": shortSchema("string", "", nil, ""),
			"type":      shortSchema("string", "", nil, ""),
		},
	}
}

func contentRootFromParty(sec partySection) contentRoot {
	return contentRoot{
		id:         sec.id,
		label:      sec.label,
		terms:      sec.terms,
		descs:      sec.descs,
		node:       sec.node,
		arrayNodes: sec.arrayNodes,
	}
}
