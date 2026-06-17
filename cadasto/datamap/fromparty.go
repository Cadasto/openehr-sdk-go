package datamap

import (
	"errors"
	"fmt"
	"maps"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// FromParty converts canonical demographic RM JSON into a datamap payload.
func FromParty(opt *template.OperationalTemplate, party map[string]any, opts ...DecodeOption) (map[string]any, error) {
	return fromParty(opt, party, false, opts...)
}

// FromPartyExpanded is like FromParty but emits expanded {rmType,…} value forms.
func FromPartyExpanded(opt *template.OperationalTemplate, party map[string]any, opts ...DecodeOption) (map[string]any, error) {
	return fromParty(opt, party, true, opts...)
}

func fromParty(opt *template.OperationalTemplate, party map[string]any, expanded bool, opts ...DecodeOption) (map[string]any, error) {
	if party == nil {
		return nil, errors.New("datamap.FromParty: nil party")
	}
	rmType, _ := party["_type"].(string)
	if !partyRMTypes[rmType] {
		return nil, fmt.Errorf("datamap.FromParty: unsupported party type %q", rmType)
	}

	var rootSec partySection
	if opt != nil {
		if root, ok := opt.Root().(template.ObjectNode); ok {
			rootSec = partySectionFromNode(root)
			rootSec.node = root
		}
	}
	cr := contentRootFromParty(rootSec)
	cr.expanded = expanded
	cr.opt = opt

	out := map[string]any{}
	if n := readDVValue(party["name"]); n != nil {
		if s, ok := n.(string); ok {
			out["name"] = s
		}
	}

	if ids, err := decodePartyIdentities(party["identities"], opt, expanded); err != nil {
		return nil, fmt.Errorf("identities: %w", err)
	} else if len(ids) > 0 {
		out["identities"] = ids
	}

	if det, err := decodePartyDetails(party["details"], cr); err != nil {
		return nil, fmt.Errorf("details: %w", err)
	} else if len(det) > 0 {
		out["details"] = det
	}

	if cons, err := decodePartyContacts(party["contacts"], opt, expanded); err != nil {
		return nil, fmt.Errorf("contacts: %w", err)
	} else if len(cons) > 0 {
		out["contacts"] = cons
	}

	if rels, err := decodePartyRelationships(party["relationships"], expanded); err != nil {
		return nil, fmt.Errorf("relationships: %w", err)
	} else if len(rels) > 0 {
		out["relationships"] = rels
	}

	if langs, ok := party["languages"].([]any); ok && len(langs) > 0 {
		decoded := make([]any, 0, len(langs))
		for _, l := range langs {
			if s := readDVValue(l); s != nil {
				if str, ok := s.(string); ok {
					decoded = append(decoded, str)
				}
			}
		}
		if len(decoded) > 0 {
			out["languages"] = decoded
		}
	}

	if roles, ok := party["roles"].([]any); ok && len(roles) > 0 {
		decoded := make([]any, 0, len(roles))
		for _, r := range roles {
			if ref := decodePartyRef(r); ref != nil {
				decoded = append(decoded, ref)
			}
		}
		if len(decoded) > 0 {
			out["roles"] = decoded
		}
	}

	return out, nil
}

func decodePartyIdentities(raw any, opt *template.OperationalTemplate, expanded bool) (map[string]any, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	for i, item := range list {
		identity, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("identity[%d] is not an object", i)
		}
		id, _ := identity["archetype_node_id"].(string)
		label := identityPurposeLabel(identity)
		key := id
		if label != "" {
			key = id + "|" + label
		}
		sec := partySectionForArchetype(opt, id)
		cr := contentRootFromParty(sec)
		cr.expanded = expanded
		details, _ := identity["details"].(map[string]any)
		decoded, err := decodeItems(structuredItemsList(details), cr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if display, code := readCodedName(identity["name"]); code != "" {
			decoded["_code"] = code
			if display != "" {
				decoded["_name"] = display
			}
		}
		if len(decoded) > 0 {
			out[key] = decoded
		}
	}
	return out, nil
}

func identityPurposeLabel(identity map[string]any) string {
	if display, code := readCodedName(identity["name"]); code != "" {
		if display != "" {
			return display
		}
		return code
	}
	if s := readDVValue(identity["name"]); s != nil {
		if str, ok := s.(string); ok {
			return str
		}
	}
	return ""
}

func decodePartyDetails(raw any, cr contentRoot) (map[string]any, error) {
	details, ok := raw.(map[string]any)
	if !ok {
		return nil, nil
	}
	// Re-scope to the details ITEM_TREE's own archetype (person_details.v2) so
	// its at-codes (e.g. at0010 = "Geboortedatum") aren't mislabelled by the
	// PARTY root's merged term dictionary (where at0010 = "Volledige naam").
	cr = rescopeForArchetype(cr, archetypeIDOf(details))
	return decodeItems(structuredItemsList(details), cr)
}

// archetypeIDOf reads archetype_details.archetype_id.value from a canonical RM
// object, or "" when absent.
func archetypeIDOf(m map[string]any) string {
	ad, ok := m["archetype_details"].(map[string]any)
	if !ok {
		return ""
	}
	aid, ok := ad["archetype_id"].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := aid["value"].(string)
	return v
}

// rescopeForArchetype returns cr re-bound to archID's terms/arrayNodes (keeping
// opt + expanded). Returns cr unchanged when opt is nil or archID is empty.
func rescopeForArchetype(cr contentRoot, archID string) contentRoot {
	if cr.opt == nil || archID == "" {
		return cr
	}
	nc := contentRootFromParty(partySectionForArchetype(cr.opt, archID))
	nc.opt = cr.opt
	nc.expanded = cr.expanded
	return nc
}

func decodePartyContacts(raw any, opt *template.OperationalTemplate, expanded bool) ([]any, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, nil
	}
	var contactKey string
	if opt != nil {
		if root, ok := opt.Root().(template.ObjectNode); ok {
			if conAttr := findAttr(root, "contacts"); conAttr != nil {
				if contactNode, ok := attrFirstObject(conAttr); ok {
					sec := partySectionFromNode(contactNode)
					contactKey = sec.id + "|" + sec.label
				}
			}
		}
	}

	var out []any
	for i, item := range list {
		contact, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contact[%d] is not an object", i)
		}
		addresses, err := decodePartyAddresses(contact["addresses"], opt, expanded)
		if err != nil {
			return nil, fmt.Errorf("contact[%d]: %w", i, err)
		}
		if len(addresses) == 0 {
			continue
		}
		wrapper := map[string]any{}
		key := contactKey
		if key == "" {
			if nid, _ := contact["archetype_node_id"].(string); nid != "" {
				key = nid
			}
		}
		if key != "" {
			wrapper[key] = addresses
		} else {
			maps.Copy(wrapper, addresses)
		}
		out = append(out, wrapper)
	}
	return out, nil
}

func decodePartyAddresses(raw any, opt *template.OperationalTemplate, expanded bool) (map[string]any, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, nil
	}
	type bucket struct {
		key  string
		vals []any
	}
	order := []string{}
	byArchetype := map[string]*bucket{}

	for i, item := range list {
		addr, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("address[%d] is not an object", i)
		}
		id, _ := addr["archetype_node_id"].(string)
		label := addressPurposeLabel(addr)
		key := id
		if label != "" {
			key = id + "|" + label
		}
		sec := partySectionForArchetype(opt, id)
		cr := contentRootFromParty(sec)
		cr.expanded = expanded
		details, _ := addr["details"].(map[string]any)
		decoded, err := decodeItems(structuredItemsList(details), cr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if display, code := readCodedName(addr["name"]); code != "" {
			decoded["_code"] = code
			if display != "" {
				decoded["_name"] = display
			}
		}
		b := byArchetype[id]
		if b == nil {
			b = &bucket{key: key}
			byArchetype[id] = b
			order = append(order, id)
		}
		b.vals = append(b.vals, decoded)
	}

	out := map[string]any{}
	for _, id := range order {
		b := byArchetype[id]
		if len(b.vals) > 1 {
			out[b.key] = b.vals
		} else if len(b.vals) == 1 {
			out[b.key] = b.vals[0]
		}
	}
	return out, nil
}

func addressPurposeLabel(addr map[string]any) string {
	if display, code := readCodedName(addr["name"]); code != "" {
		if display != "" {
			return display
		}
		return code
	}
	return ""
}

func decodePartyRelationships(raw any, expanded bool) ([]any, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, nil
	}
	var out []any
	for i, item := range list {
		rel, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("relationship[%d] is not an object", i)
		}
		decoded := map[string]any{}
		if src := decodePartyRef(rel["source"]); src != nil {
			decoded["source"] = src
		}
		if tgt := decodePartyRef(rel["target"]); tgt != nil {
			decoded["target"] = tgt
		}
		if det, ok := rel["details"].(map[string]any); ok {
			cr := contentRoot{arrayNodes: map[string]bool{}, expanded: expanded}
			items, err := decodeItems(structuredItemsList(det), cr)
			if err != nil {
				return nil, err
			}
			maps.Copy(decoded, items)
		}
		if len(decoded) > 0 {
			out = append(out, decoded)
		}
	}
	return out, nil
}

func decodePartyRef(v any) map[string]any {
	ref, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	idObj, _ := ref["id"].(map[string]any)
	if idObj == nil {
		return nil
	}
	id, _ := idObj["value"].(string)
	if id == "" {
		return nil
	}
	out := map[string]any{"id": id}
	if ns, _ := idObj["namespace"].(string); ns != "" {
		out["namespace"] = ns
	}
	if typ, _ := ref["type"].(string); typ != "" {
		out["type"] = typ
	}
	return out
}

func partySectionForArchetype(opt *template.OperationalTemplate, archetypeID string) partySection {
	if opt == nil || archetypeID == "" {
		return partySection{arrayNodes: map[string]bool{}}
	}
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		return partySection{arrayNodes: map[string]bool{}}
	}
	if ar, ok := root.(*template.ArchetypeRoot); ok && ar.ArchetypeID() == archetypeID {
		return partySectionFromNode(ar)
	}
	if found := findArchetypeInTree(root, archetypeID); found != nil {
		return partySectionFromNode(found)
	}
	return partySection{id: archetypeID, arrayNodes: map[string]bool{}}
}

func findArchetypeInTree(node template.ObjectNode, archetypeID string) template.ObjectNode {
	if ar, ok := node.(*template.ArchetypeRoot); ok && ar.ArchetypeID() == archetypeID {
		return ar
	}
	for _, a := range node.Attributes() {
		for _, c := range a.Children() {
			if obj, ok := c.(template.ObjectNode); ok {
				if found := findArchetypeInTree(obj, archetypeID); found != nil {
					return found
				}
			}
		}
	}
	return nil
}
