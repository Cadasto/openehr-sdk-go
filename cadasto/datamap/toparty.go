package datamap

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// ToParty converts a datamap payload into canonical demographic RM JSON (PERSON,
// ORGANISATION, AGENT, GROUP, ROLE, or nested demographic archetypes such as
// ADDRESS) for the Demographics REST API write path.
func ToParty(opt *template.OperationalTemplate, payload map[string]any) (map[string]any, error) {
	if opt == nil {
		return nil, errors.New("datamap.ToParty: nil template")
	}
	if !IsPartyTemplate(opt) {
		return nil, fmt.Errorf("datamap.ToParty: template root %q is not a demographic PARTY type", opt.Root().RMTypeName())
	}
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		return nil, errors.New("datamap.ToParty: OPT root is not an object node")
	}

	rmType := root.RMTypeName()
	archetypeID := rootArchetypeID(root)
	name := stringOrDefault(payload["name"], rootName(root))

	out := map[string]any{
		"_type":             rmType,
		"archetype_node_id": archetypeID,
		"name":              dvText(name),
		"archetype_details": archetypeDetails(archetypeID, opt.TemplateID()),
	}

	sec := partySectionFromNode(root)

	if ids, err := encodePartyIdentities(root, payload, sec.terms); err != nil {
		return nil, fmt.Errorf("identities: %w", err)
	} else if len(ids) > 0 {
		out["identities"] = ids
	}

	if det, err := encodePartyDetails(root, payload, sec); err != nil {
		return nil, fmt.Errorf("details: %w", err)
	} else if det != nil {
		out["details"] = det
	}

	if cons, err := encodePartyContacts(root, payload, sec.terms); err != nil {
		return nil, fmt.Errorf("contacts: %w", err)
	} else if len(cons) > 0 {
		out["contacts"] = cons
	}

	if rels, err := encodePartyRelationships(root, payload, sec.terms); err != nil {
		return nil, fmt.Errorf("relationships: %w", err)
	} else if len(rels) > 0 {
		out["relationships"] = rels
	}

	if langs, ok := payload["languages"].([]any); ok && len(langs) > 0 {
		encoded := make([]any, 0, len(langs))
		for _, l := range langs {
			if s, ok := l.(string); ok && s != "" {
				encoded = append(encoded, dvText(s))
			}
		}
		if len(encoded) > 0 {
			out["languages"] = encoded
		}
	}

	if roles, ok := payload["roles"].([]any); ok && len(roles) > 0 {
		encoded := make([]any, 0, len(roles))
		for _, r := range roles {
			if ref := encodePartyRef(r); ref != nil {
				encoded = append(encoded, ref)
			}
		}
		if len(encoded) > 0 {
			out["roles"] = encoded
		}
	}

	if caps, err := encodePartyCapabilities(root, payload, sec.terms); err != nil {
		return nil, fmt.Errorf("capabilities: %w", err)
	} else if len(caps) > 0 {
		out["capabilities"] = caps
	}

	// Standalone ADDRESS / PARTY_IDENTITY templates encode their own details tree.
	if rmType == "ADDRESS" || rmType == "PARTY_IDENTITY" {
		if detPayload, ok := payload["details"].(map[string]any); ok {
			if det, err := encodePartyItemTree(root, detPayload, sec); err != nil {
				return nil, err
			} else if det != nil {
				out["details"] = det
			}
		}
	}

	return out, nil
}

func encodePartyIdentities(root template.ObjectNode, payload map[string]any, rootTerms map[string]string) ([]any, error) {
	idAttr := findAttr(root, "identities")
	if idAttr == nil {
		return nil, nil
	}
	idPayload, _ := payload["identities"].(map[string]any)
	if len(idPayload) == 0 {
		return nil, nil
	}

	var out []any
	for _, c := range idAttr.Children() {
		ar, ok := c.(*template.ArchetypeRoot)
		if !ok {
			continue
		}
		sec := partySectionFromNode(ar)
		identityData := lookupRootPayload(idPayload, sec.id, sec.label)
		if identityData == nil {
			continue
		}
		encoded, err := encodePartyIdentity(ar, identityData, sec)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sec.id, err)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func encodePartyIdentity(ar *template.ArchetypeRoot, payload map[string]any, sec partySection) (map[string]any, error) {
	name := identityName(ar, payload, sec)
	out := map[string]any{
		"_type":             "PARTY_IDENTITY",
		"archetype_node_id": ar.ArchetypeID(),
		"name":              name,
		"archetype_details": archetypeDetails(ar.ArchetypeID(), ""),
	}
	detailsNode, ok := attrFirstObject(findAttr(ar, "details"))
	if !ok {
		return out, nil
	}
	itemsAttr := structuredItemsAttr(detailsNode)
	if itemsAttr == nil {
		return out, nil
	}
	items, err := encodeItems(itemsAttr, payload, sec.terms)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return out, nil
	}
	out["details"] = encodeStructuredContainer(detailsNode, items, "Tree", sec.terms)
	return out, nil
}

func identityName(node template.ObjectNode, payload map[string]any, sec partySection) map[string]any {
	if c, ok := payload["_code"]; ok {
		if terminology, code, display, pok := parseCodeField(c); pok {
			label := display
			if label == "" {
				if n, ok := payload["_name"].(string); ok && n != "" {
					label = n
				} else if t := sec.terms[code]; t != "" {
					// The coded name's display must be the term for the code
					// (at0027 → "Officiële naam"), not the archetype label.
					label = t
				} else {
					label = sec.label
				}
			}
			return dvCodedText(label, terminology, code)
		}
	}
	// No explicit code: when the OPT constrains the identity name to a closed
	// coded list (PARTY_IDENTITY.name on person_name.v2 → at0027 "Officiële
	// naam"), default to the first allowed code — Cadasto rejects a plain
	// DV_TEXT name in that slot ("Expected DV_CODED_TEXT, got DV_TEXT").
	if coded, ok := constrainedCodedName(node, sec.terms); ok {
		return coded
	}
	return dvText(sec.label)
}

// constrainedCodedName returns the default DV_CODED_TEXT for a node's `name`
// attribute when the OPT constrains it to a closed CODE_PHRASE list. ok=false
// when the name is unconstrained or not coded (caller falls back to DV_TEXT).
func constrainedCodedName(node template.ObjectNode, terms map[string]string) (map[string]any, bool) {
	nameNode, ok := attrFirstObject(findAttr(node, "name"))
	if !ok || nameNode.RMTypeName() != "DV_CODED_TEXT" {
		return nil, false
	}
	codeNode, ok := attrFirstObject(findAttr(nameNode, "defining_code"))
	if !ok {
		return nil, false
	}
	co, ok := codeNode.(*template.ComplexObject)
	if !ok {
		return nil, false
	}
	cp, ok := co.PrimitiveConstraint().(constraints.CodePhrase)
	if !ok || len(cp.CodeList) == 0 {
		return nil, false
	}
	ref, ok := cp.ExampleValue().(constraints.CodedTermRef)
	if !ok || ref.CodeString == "" {
		return nil, false
	}
	return dvCodedText(termOrFallback(terms, ref.CodeString, ref.CodeString), ref.Terminology, ref.CodeString), true
}

func encodePartyDetails(root template.ObjectNode, payload map[string]any, sec partySection) (map[string]any, error) {
	detPayload, _ := payload["details"].(map[string]any)
	if len(detPayload) == 0 {
		return nil, nil
	}
	detAttr := findAttr(root, "details")
	if detAttr == nil {
		return nil, nil
	}
	detailsNode, ok := attrFirstObject(detAttr)
	if !ok {
		return nil, nil
	}
	// Re-scope to the details ITEM_TREE's own archetype (person_details.v2) so
	// its terms drive the encode: the tree name ("Persoon data") and the value
	// terms for coded items (at0310 → "Man") come from this archetype, not the
	// PARTY root's term dictionary.
	return encodePartyItemTree(detailsNode, detPayload, partySectionFromNode(detailsNode))
}

func encodePartyItemTree(treeNode template.ObjectNode, payload map[string]any, sec partySection) (map[string]any, error) {
	itemsAttr := structuredItemsAttr(treeNode)
	if itemsAttr == nil {
		return nil, nil
	}
	items, err := encodeItems(itemsAttr, payload, sec.terms)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return encodeStructuredContainer(treeNode, items, "Tree", sec.terms), nil
}

func encodePartyContacts(root template.ObjectNode, payload map[string]any, rootTerms map[string]string) ([]any, error) {
	conAttr := findAttr(root, "contacts")
	if conAttr == nil {
		return nil, nil
	}
	contactsPayload, _ := payload["contacts"].([]any)
	if len(contactsPayload) == 0 {
		return nil, nil
	}
	contactNode, ok := attrFirstObject(conAttr)
	if !ok {
		return nil, nil
	}
	sec := partySectionFromNode(contactNode)
	contactKey := sec.id + "|" + sec.label

	var out []any
	for i, raw := range contactsPayload {
		cm, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contacts[%d] is not an object", i)
		}
		contactData := lookupRootPayload(cm, sec.id, sec.label)
		if contactData == nil {
			contactData = cm
		}
		addresses, err := encodePartyAddresses(contactNode, contactData, rootTerms)
		if err != nil {
			return nil, fmt.Errorf("contacts[%d]: %w", i, err)
		}
		if len(addresses) == 0 {
			continue
		}
		contact := map[string]any{
			"_type":             "CONTACT",
			"archetype_node_id": sec.id,
			"name":              dvText(sec.label),
			"addresses":         addresses,
		}
		_ = contactKey // key is only for datamap lookup
		out = append(out, contact)
	}
	return out, nil
}

func encodePartyAddresses(contactNode template.ObjectNode, payload map[string]any, rootTerms map[string]string) ([]any, error) {
	addrAttr := findAttr(contactNode, "addresses")
	if addrAttr == nil {
		return nil, nil
	}
	var out []any
	for _, c := range addrAttr.Children() {
		ar, ok := c.(*template.ArchetypeRoot)
		if !ok {
			continue
		}
		sec := partySectionFromNode(ar)
		value, found := lookupChildPayload(payload, sec.id, sec.label)
		if !found {
			continue
		}
		instances := []any{value}
		if arr, ok := value.([]any); ok {
			instances = arr
		}
		for _, inst := range instances {
			ap, ok := inst.(map[string]any)
			if !ok {
				continue
			}
			encoded, err := encodePartyAddress(ar, ap, sec)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded)
		}
	}
	return out, nil
}

func encodePartyAddress(ar *template.ArchetypeRoot, payload map[string]any, sec partySection) (map[string]any, error) {
	name := addressName(payload, sec)
	out := map[string]any{
		"_type":             "ADDRESS",
		"archetype_node_id": ar.ArchetypeID(),
		"name":              name,
		"archetype_details": archetypeDetails(ar.ArchetypeID(), ""),
	}
	detailsNode, ok := attrFirstObject(findAttr(ar, "details"))
	if !ok {
		return out, nil
	}
	items, err := encodePartyItemTree(detailsNode, payload, sec)
	if err != nil {
		return nil, err
	}
	if items != nil {
		out["details"] = items
	}
	return out, nil
}

func addressName(payload map[string]any, sec partySection) map[string]any {
	if c, ok := payload["_code"]; ok {
		if terminology, code, display, pok := parseCodeField(c); pok {
			label := display
			if label == "" {
				if n, ok := payload["_name"].(string); ok && n != "" {
					label = n
				} else if t := sec.terms[code]; t != "" {
					// The coded name's display must be the term for the code
					// (at0027 → "Officiële naam"), not the archetype label.
					label = t
				} else {
					label = sec.label
				}
			}
			return dvCodedText(label, terminology, code)
		}
	}
	return dvText(sec.label)
}

func encodePartyRelationships(root template.ObjectNode, payload map[string]any, terms map[string]string) ([]any, error) {
	relAttr := findAttr(root, "relationships")
	if relAttr == nil || len(relAttr.Children()) == 0 {
		return nil, nil
	}
	relsPayload, _ := payload["relationships"].([]any)
	if len(relsPayload) == 0 {
		return nil, nil
	}
	var out []any
	for i, raw := range relsPayload {
		rm, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("relationships[%d] is not an object", i)
		}
		rel := map[string]any{
			"_type":             "PARTY_RELATIONSHIP",
			"archetype_node_id": "at0000",
			"name":              dvText("Relationship"),
		}
		if src := encodePartyRef(rm["source"]); src != nil {
			rel["source"] = src
		}
		if tgt := encodePartyRef(rm["target"]); tgt != nil {
			rel["target"] = tgt
		}
		if detNode, ok := attrFirstObject(relAttr); ok {
			if itemsAttr := structuredItemsAttr(detNode); itemsAttr != nil {
				sec := partySectionFromNode(detNode)
				items, err := encodeItems(itemsAttr, rm, sec.terms)
				if err != nil {
					return nil, err
				}
				if len(items) > 0 {
					rel["details"] = encodeStructuredContainer(detNode, items, "Tree", sec.terms)
				}
			}
		}
		out = append(out, rel)
	}
	return out, nil
}

func encodePartyCapabilities(root template.ObjectNode, payload map[string]any, terms map[string]string) ([]any, error) {
	capAttr := findAttr(root, "capabilities")
	if capAttr == nil {
		return nil, nil
	}
	capsPayload, _ := payload["capabilities"].([]any)
	if len(capsPayload) == 0 {
		return nil, nil
	}
	capNode, ok := attrFirstObject(capAttr)
	if !ok {
		return nil, nil
	}
	sec := partySectionFromNode(capNode)
	credNode, ok := attrFirstObject(findAttr(capNode, "credentials"))
	if !ok {
		return nil, nil
	}
	itemsAttr := structuredItemsAttr(credNode)
	if itemsAttr == nil {
		return nil, nil
	}
	var out []any
	for i, raw := range capsPayload {
		cm, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("capabilities[%d] is not an object", i)
		}
		items, err := encodeItems(itemsAttr, cm, sec.terms)
		if err != nil {
			return nil, fmt.Errorf("capabilities[%d]: %w", i, err)
		}
		cap := map[string]any{
			"_type":             "CAPABILITY",
			"archetype_node_id": capNode.NodeID(),
			"name":              dvText(termOrFallback(sec.terms, capNode.NodeID(), "Capability")),
			"credentials":       encodeStructuredContainer(credNode, items, "Tree", sec.terms),
		}
		out = append(out, cap)
	}
	return out, nil
}

func encodePartyRef(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		if s, ok := v.(string); ok && s != "" {
			return map[string]any{
				"_type": "PARTY_REF",
				"id": map[string]any{
					"_type":     "HIER_OBJECT_ID",
					"value":     s,
					"namespace": "local",
				},
			}
		}
		return nil
	}
	id := stringOrDefault(m["id"], "")
	if id == "" {
		return nil
	}
	ref := map[string]any{
		"_type": "PARTY_REF",
		"id": map[string]any{
			"_type":     "HIER_OBJECT_ID",
			"value":     id,
			"namespace": stringOrDefault(m["namespace"], "local"),
		},
	}
	if typ := stringOrDefault(m["type"], ""); typ != "" {
		ref["type"] = typ
	}
	return ref
}
