package datamap

import (
	"errors"
	"fmt"
	"maps"
	"slices"
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
// encodeComposer builds COMPOSITION.composer (PARTY_IDENTIFIED). The payload
// value is either a plain string (name-only, backwards compatible) or an
// expanded map:
//
//	{"name": "...",
//	 "id": "...", "id_scheme": "...", "id_namespace": "...", "id_type": "...",
//	 "identifiers": [{"id": "...", "type": "...", "issuer": "...", "assigner": "..."}, ...]}
//
// The id* keys become an external_ref (PARTY_REF/GENERIC_ID) so the composer
// is AQL-queryable on identifier (c/composer/external_ref/id/value) instead
// of display name; identifiers become DV_IDENTIFIERs (e.g. AGB, tenant).
func encodeComposer(v any) map[string]any {
	out := map[string]any{"_type": "PARTY_IDENTIFIED", "name": "Cadasto SDK"}
	switch c := v.(type) {
	case string:
		if c != "" {
			out["name"] = c
		}
	case map[string]any:
		if name, _ := c["name"].(string); name != "" {
			out["name"] = name
		}
		if id, _ := c["id"].(string); id != "" {
			out["external_ref"] = map[string]any{
				"_type":     "PARTY_REF",
				"namespace": stringOrDefault(c["id_namespace"], "lab24"),
				"type":      stringOrDefault(c["id_type"], "PERSON"),
				"id": map[string]any{
					"_type":  "GENERIC_ID",
					"value":  id,
					"scheme": stringOrDefault(c["id_scheme"], "id"),
				},
			}
		}
		if ids, _ := c["identifiers"].([]any); len(ids) > 0 {
			list := make([]any, 0, len(ids))
			for _, raw := range ids {
				m, _ := raw.(map[string]any)
				if m == nil {
					continue
				}
				idv, _ := m["id"].(string)
				if idv == "" {
					continue
				}
				dv := map[string]any{"_type": "DV_IDENTIFIER", "id": idv}
				for _, k := range []string{"type", "issuer", "assigner"} {
					if s, _ := m[k].(string); s != "" {
						dv[k] = s
					}
				}
				list = append(list, dv)
			}
			if len(list) > 0 {
				out["identifiers"] = list
			}
		}
	}
	return out
}

func ToComposition(opt *template.OperationalTemplate, payload map[string]any) (map[string]any, error) {
	if IsPartyTemplate(opt) {
		return nil, errors.New("datamap.ToComposition: template roots a demographic PARTY type; use ToParty")
	}
	root, ok := opt.Root().(template.ObjectNode)
	if !ok {
		return nil, errors.New("datamap.ToComposition: OPT root is not an object node")
	}

	language := stringOrDefault(payload["language"], "nl")
	territory := stringOrDefault(payload["territory"], "NL")
	composer := encodeComposer(payload["composer"])

	contextPayload, _ := payload["context"].(map[string]any)
	startTime := stringOrDefault(contextPayload["start_time"], "")
	if startTime == "" {
		return nil, errors.New("datamap.ToComposition: context.start_time is required")
	}

	roots := findContentArchetypeRoots(root)
	if len(roots) == 0 {
		return nil, errors.New("datamap.ToComposition: template has no archetype roots under content")
	}
	contentPayload, _ := payload["content"].(map[string]any)

	// A content-root payload is normally a single map (one entry); a []any of
	// entry-maps (REQ-0029, e.g. a persistent care_plan holding N pathway
	// enrollments of the same archetype root) emits one COMPOSITION content
	// entry per element instead of overwriting down to the last one.
	content := make([]any, 0, len(roots))
	for i := range roots {
		r := roots[i]
		raw := lookupRootValue(contentPayload, r.id, r.label)
		if raw == nil {
			continue
		}
		payloads, err := rootPayloadList(raw)
		if err != nil {
			return nil, fmt.Errorf("content root %s: %w", r.id, err)
		}
		for _, rp := range payloads {
			entry, err := encodeArchetypeRoot(r, rp, startTime, language)
			if err != nil {
				return nil, fmt.Errorf("encode %s: %w", r.id, err)
			}
			content = append(content, entry)
		}
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

	// Category is taken from the OPT (REQ-0029): a care_plan/persistent OPT pins
	// 431|persistent, most others 433|event. A persistent COMPOSITION has no
	// context (RM: COMPOSITION.context is absent iff category is persistent), so
	// omit the EVENT_CONTEXT block entirely in that case. Falls back to event/433
	// when the OPT does not pin the category.
	catCode := optCategoryCode(root)
	if catCode == "" {
		catCode = "433"
	}
	persistent := catCode == "431"

	comp := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": rootArchetypeID(root),
		"name":              dvText(rootName(root)),
		"archetype_details": archetypeDetails(rootArchetypeID(root), opt.TemplateID()),
		"language":          codePhrase("ISO_639-1", language),
		"territory":         codePhrase("ISO_3166-1", territory),
		"category":          dvCodedText(categoryValueForCode(catCode), "openehr", catCode),
		"composer":          composer,
		"content":           content,
	}
	if !persistent {
		comp["context"] = map[string]any{
			"_type":      "EVENT_CONTEXT",
			"start_time": dvDateTime(startTime),
			"setting":    dvCodedText("other care", "openehr", "238"),
		}
	}

	// other_context (ITEM_TREE op EVENT_CONTEXT, bv. een annotations-cluster)
	// — alleen wanneer de datamap er inhoud voor levert; geen leeg skelet.
	// Persistent compositions have no context, so there is nowhere to attach it.
	if !persistent {
		if oc, err := encodeOtherContext(root, payload); err != nil {
			return nil, err
		} else if oc != nil {
			comp["context"].(map[string]any)["other_context"] = oc
		}
	}

	// feeder_audit is een RM-attribuut op COMPOSITION (geen archetyped
	// content) — het draagt de herkomst van de ingevoerde data, incl. de
	// originating-system item-ids (bv. order-/lab-result-nummer). De
	// caller levert 't als platte map; we encoden 't naar de canonical
	// FEEDER_AUDIT-vorm zodat het querybaar in de CDR landt (anders gaat
	// een business-key voor idempotency-find verloren).
	if fa := feederAudit(payload["feeder_audit"]); fa != nil {
		comp["feeder_audit"] = fa
	}

	return comp, nil
}

// feederAudit encodeert de datamap-feeder_audit (platte map) naar de
// canonical FEEDER_AUDIT-vorm. Retourneert nil wanneer er niets bruikbaars
// is (geen map, of geen system_id én geen item-ids) — dan laten we het
// attribuut weg i.p.v. een invalide FEEDER_AUDIT te emitten.
//
// Verwachte input-shape:
//
//	{
//	  "originating_system_audit":   {"system_id": "<sys>"},
//	  "originating_system_item_ids": [{"id": "...", "issuer": "...", "assigner": "...", "type": "..."}]
//	}
func feederAudit(raw any) map[string]any {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	out := map[string]any{"_type": "FEEDER_AUDIT"}

	// originating_system_audit (FEEDER_AUDIT_DETAILS) — system_id is in de
	// RM verplicht; zonder geldige system_id emitten we de details niet.
	if osa, ok := m["originating_system_audit"].(map[string]any); ok {
		if sysID := stringOrDefault(osa["system_id"], ""); sysID != "" {
			out["originating_system_audit"] = map[string]any{
				"_type":     "FEEDER_AUDIT_DETAILS",
				"system_id": sysID,
			}
		}
	}

	// originating_system_item_ids ([]DV_IDENTIFIER) — id is verplicht per
	// identifier; entries zonder id slaan we over.
	if rawIDs, ok := m["originating_system_item_ids"].([]any); ok {
		ids := make([]any, 0, len(rawIDs))
		for _, ri := range rawIDs {
			im, ok := ri.(map[string]any)
			if !ok {
				continue
			}
			id := stringOrDefault(im["id"], "")
			if id == "" {
				continue
			}
			dvID := map[string]any{"_type": "DV_IDENTIFIER", "id": id}
			if v := stringOrDefault(im["issuer"], ""); v != "" {
				dvID["issuer"] = v
			}
			if v := stringOrDefault(im["assigner"], ""); v != "" {
				dvID["assigner"] = v
			}
			if v := stringOrDefault(im["type"], ""); v != "" {
				dvID["type"] = v
			}
			ids = append(ids, dvID)
		}
		if len(ids) > 0 {
			out["originating_system_item_ids"] = ids
		}
	}

	// originating_system_audit is in de RM verplicht (1..1) op FEEDER_AUDIT.
	// Zonder geldige system_id kunnen we geen valide FEEDER_AUDIT bouwen —
	// dan laten we het attribuut weg i.p.v. een door de CDR-geweigerde body.
	if _, hasOSA := out["originating_system_audit"]; !hasOSA {
		return nil
	}
	return out
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
		// protocol (ITEM_TREE, bv. Test request details met order-identifier)
		// — alleen wanneer de datamap er daadwerkelijk inhoud voor levert.
		proto, err := encodeProtocol(r, payload)
		if err != nil {
			return nil, err
		}
		if proto != nil {
			out["protocol"] = proto
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
// encodeProtocol bouwt de OBSERVATION.protocol-ITEM_TREE uit een datamap-
// `protocol`-key, met dezelfde cluster/element-machinerie als de data-tree.
// Retourneert nil (geen protocol-attribuut op de composition) wanneer de
// datamap geen protocol levert, de OPT geen protocol-constraint heeft, of er
// na het matchen niets overblijft — zo blijft een lege protocol-sectie weg.
func encodeProtocol(r contentRoot, payload map[string]any) (map[string]any, error) {
	protoPayload, ok := payload["protocol"].(map[string]any)
	if !ok || len(protoPayload) == 0 {
		return nil, nil
	}
	protoNode, ok := attrFirstObject(findAttr(r.node, "protocol"))
	if !ok {
		return nil, nil
	}
	itemsAttr := structuredItemsAttr(protoNode)
	if itemsAttr == nil {
		return nil, nil
	}
	items, err := encodeItems(itemsAttr, protoPayload, r.terms)
	if err != nil {
		return nil, fmt.Errorf("protocol: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	return encodeStructuredContainer(protoNode, items, "Tree", r.terms), nil
}

// encodeOtherContext bouwt de EVENT_CONTEXT.other_context-ITEM_TREE uit een
// datamap-`other_context`-key (bv. een annotations-cluster), met dezelfde
// cluster/element-machinerie als data/protocol. De terms van eventuele
// slot-archetypes (zoals openEHR-EHR-CLUSTER.annotations.v1) worden uit hun
// eigen ArchetypeRoot gehaald zodat labels kloppen. Retourneert nil wanneer
// de datamap geen other_context levert, de OPT 'm niet constraint, of er na
// matchen niets overblijft — zo blijft een lege other_context-sectie weg.
func encodeOtherContext(root template.ObjectNode, payload map[string]any) (map[string]any, error) {
	ocPayload, ok := payload["other_context"].(map[string]any)
	if !ok || len(ocPayload) == 0 {
		return nil, nil
	}
	ctxNode, ok := attrFirstObject(findAttr(root, "context"))
	if !ok {
		return nil, nil
	}
	ocNode, ok := attrFirstObject(findAttr(ctxNode, "other_context"))
	if !ok {
		return nil, nil
	}
	itemsAttr := structuredItemsAttr(ocNode)
	if itemsAttr == nil {
		return nil, nil
	}
	// Verzamel terms uit slot-archetype-children (annotations.v1 e.d.) zodat
	// hun at-code-labels beschikbaar zijn voor encodeItems.
	terms := map[string]string{}
	for _, c := range itemsAttr.Children() {
		if ar, ok := c.(*template.ArchetypeRoot); ok {
			t, _ := termMaps(ar)
			maps.Copy(terms, t)
		}
	}
	items, err := encodeItems(itemsAttr, ocPayload, terms)
	if err != nil {
		return nil, fmt.Errorf("other_context: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	return encodeStructuredContainer(ocNode, items, "Tree", terms), nil
}

func encodeInstruction(out map[string]any, r contentRoot, payload map[string]any) (map[string]any, error) {
	// narrative: gebruik de datamap-narrative indien aangeleverd, anders de
	// template-term als fallback. (Een order levert hier z'n vrije tekst naar
	// het lab; eerder werd de payload-narrative genegeerd.)
	if nar, ok := payload["narrative"].(string); ok && nar != "" {
		out["narrative"] = dvText(nar)
	} else {
		out["narrative"] = dvText(termOrFallback(r.terms, "narrative", "Instruction"))
	}

	// guideline_id (CARE_ENTRY RM-attribuut, OBJECT_REF) — verwijzing naar de
	// richtlijn/het formulier dat deze entry voortbracht. Passthrough van de
	// caller-payload (map met id/namespace/type); legacy-systemen leggen hier
	// de view-template/formulier-naam vast.
	if g, ok := payload["guideline_id"].(map[string]any); ok && len(g) > 0 {
		out["guideline_id"] = g
	}

	// protocol (ITEM_TREE, bv. order-identifier + status) — dezelfde machinerie
	// als OBSERVATION; alleen gezet wanneer de datamap er inhoud voor levert en
	// de OPT 'm constraint. Eerder ontbrak dit voor INSTRUCTION, waardoor een
	// order z'n protocol-velden (ordernummer/status) verloor bij encoden.
	proto, err := encodeProtocol(r, payload)
	if err != nil {
		return nil, err
	}
	if proto != nil {
		out["protocol"] = proto
	}

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
	// Lifecycle state: default to completed(532) for backward compatibility;
	// a caller (e.g. the protocol-enrollment engine, REQ-0029) may supply an
	// explicit current_state and/or careflow_step as expanded coded-text
	// {code, value, terminology} to record a pathway state (planned/active/
	// abandoned/…) instead.
	currentState := dvCodedText("completed", "openehr", "532")
	if cs, ok := codedTextFromPayload(payload["current_state"]); ok {
		currentState = cs
	}
	ism := map[string]any{
		"_type":         "ISM_TRANSITION",
		"current_state": currentState,
	}
	if step, ok := codedTextFromPayload(payload["careflow_step"]); ok {
		ism["careflow_step"] = step
	}
	out["ism_transition"] = ism
	return out, nil
}

func encodeEvent(eventConstraint template.ObjectNode, payload map[string]any, terms map[string]string, fallbackTime string) (map[string]any, error) {
	t := stringOrDefault(payload["time"], fallbackTime)

	dataNode, ok := attrFirstObject(findAttr(eventConstraint, "data"))
	if !ok {
		return nil, errors.New("event constraint has no data")
	}
	itemsAttr := structuredItemsAttr(dataNode)
	if itemsAttr == nil {
		return nil, errors.New("event ITEM_TREE has no items")
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
		// A slot-filled CLUSTER archetype (e.g. person_identifier.v2 nested in the
		// person_details ITEM_TREE) reports the bare at0000 root node id, shared
		// with its sibling archetypes; it is addressed in the datamap — and must be
		// emitted to Cadasto — by its archetype id, with archetype_details. Its
		// items also belong to the nested archetype's own term dictionary.
		lookupKey := nodeID
		archetypeID := ""
		subTerms := terms
		label := terms[nodeID]
		if ar, ok := obj.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
			archetypeID = ar.ArchetypeID()
			lookupKey = archetypeID
			subTerms = partySectionFromNode(ar).terms
			// The cluster's runtime name comes from the nested archetype's own
			// at0000 term (person_identifier.v2 → "Persoon ID"), not the parent
			// tree's term for the shared bare node id ("Persoon data"). Cadasto
			// rejects a mismatched name against the constrained value.
			label = termOrFallback(subTerms, nodeID, label)
		}
		value, found := lookupChildPayload(payload, lookupKey, label)
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
				subItems, err := encodeItems(subItemsAttr, subPayload, subTerms)
				if err != nil {
					return nil, fmt.Errorf("cluster %s: %w", nodeID, err)
				}
				cluster := map[string]any{
					"_type":             "CLUSTER",
					"archetype_node_id": nodeID,
					"name":              clusterName(subPayload, label),
					"items":             subItems,
				}
				if archetypeID != "" {
					cluster["archetype_node_id"] = archetypeID
					cluster["archetype_details"] = archetypeDetails(archetypeID, "")
				}
				out = append(out, cluster)
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

	// Coded/explicit ELEMENT name. By default an element's name is the template
	// label as DV_TEXT. A payload MAP that carries the Datamap-V2 name meta-keys
	// `_code` (coded name) or `_name` (explicit display) overrides it, and the
	// element value then comes from the payload's `value` field. This supports
	// "named" elements such as openEHR-EHR-CLUSTER.annotations.v1 at0001, whose
	// name MUST be a DV_CODED_TEXT (the annotation type) — Cadasto rejects a
	// DV_TEXT name with 422. The meta-keys are underscore-prefixed so they never
	// collide with bare coded-value short-forms ({code,value,terminology}).
	name := dvText(terms[nodeID])
	if m, ok := valuePayload.(map[string]any); ok {
		_, hasCode := m["_code"]
		display, hasName := m["_name"].(string)
		if hasCode {
			name = clusterName(m, terms[nodeID])
			valuePayload = m["value"]
		} else if hasName {
			name = dvText(display)
			valuePayload = m["value"]
		}
	}

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
				"name":              name,
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
		"name":              name,
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
	case "DV_TEXT":
		// DV_CODED_TEXT IS-A DV_TEXT: een DV_TEXT-constraint staat een coded
		// instance toe. Draagt de payload een code (short-form
		// {code,value,terminology} of een defining_code), emit dan
		// DV_CODED_TEXT zodat de code behouden blijft — anders verliest bv.
		// at0121 "Aangevraagde dienst" (order-OPT) z'n bestelcode bij commit.
		if m, ok := payload.(map[string]any); ok && hasUsableCode(m) {
			if terminology, code, display, pok := parseCodeField(m); pok {
				label := display
				if label == "" {
					label = code
				}
				return dvCodedText(label, terminology, code), nil
			}
			return encodeCodedText(payload, terms)
		}
		return encodeScalarWrap(rmType, payload), nil
	case "DV_DATE_TIME", "DV_DATE", "DV_TIME", "DV_BOOLEAN", "DV_URI", "DV_EHR_URI":
		return encodeScalarWrap(rmType, payload), nil
	case "DV_COUNT":
		return encodeCount(payload)
	case "DV_CODED_TEXT":
		return encodeCodedText(payload, terms)
	case "DV_ORDINAL":
		return encodeOrdinal(constraint, payload, terms)
	case "DV_PROPORTION":
		return encodeProportion(payload)
	case "DV_IDENTIFIER":
		return encodeIdentifier(payload), nil
	case "DV_MULTIMEDIA":
		return encodeMultimedia(payload), nil
	default:
		// DV_INTERVAL<T> (e.g. a validity window <DV_DATE>) is parametric, so it
		// can't be a fixed switch case. The short-form decode strips the outer
		// _type but leaves the lower/upper RM sub-objects intact; rebuild the
		// interval around them under the constraint's parametric type.
		if strings.HasPrefix(rmType, "DV_INTERVAL") {
			return encodeInterval(rmType, payload), nil
		}
		// Unsupported RM value type. The blank datamap skeleton carries empty
		// optional slots of exotic types that we never populate — omit those
		// rather than fail the whole encode. Only a value that actually carries
		// content is a real error.
		if !valueHasContent(payload) {
			return nil, nil
		}
		return nil, fmt.Errorf("RM value type %q not supported", rmType)
	}
}

// encodeInterval rebuilds a DV_INTERVAL<T> from a short-form payload, passing
// the lower/upper RM sub-objects through verbatim (they keep their own _type)
// and copying the inclusion/unbounded flags. Returns nil when the interval has
// no bounds at all (a blank skeleton slot we never populated).
func encodeInterval(rmType string, payload any) map[string]any {
	m, ok := payload.(map[string]any)
	if !ok || !valueHasContent(m) {
		return nil
	}
	out := map[string]any{"_type": rmType}
	for _, k := range []string{"lower", "upper", "lower_included", "upper_included", "lower_unbounded", "upper_unbounded"} {
		if v, ok := m[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

// valueHasContent reports whether payload carries a real value worth encoding.
// Structural/bookkeeping keys (prefixed "_", e.g. _type) and unset defaults
// (empty strings, false, zero) do not count, so an empty DV_INTERVAL or other
// blank skeleton slot reads as contentless.
func valueHasContent(payload any) bool {
	switch v := payload.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case bool:
		return false
	case float64:
		return v != 0
	case int:
		return v != 0
	case map[string]any:
		for k, child := range v {
			if strings.HasPrefix(k, "_") {
				continue
			}
			if valueHasContent(child) {
				return true
			}
		}
		return false
	case []any:
		return slices.ContainsFunc(v, valueHasContent)
	default:
		return true
	}
}

// encodeMultimedia builds a DV_MULTIMEDIA from an object payload, or returns nil
// when there is no actual media (no data / uri) so the element is omitted. The
// blank datamap skeleton carries empty photo slots (media_type with an empty
// code, size 0) that we never populate; emitting them would fail the encode.
func encodeMultimedia(payload any) map[string]any {
	m, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	hasMedia := false
	for _, k := range []string{"data", "uri", "alternate_text"} {
		if s, _ := m[k].(string); s != "" {
			hasMedia = true
		}
	}
	if !hasMedia {
		return nil
	}
	out := map[string]any{"_type": "DV_MULTIMEDIA"}
	for k, v := range m {
		if k != "rmType" {
			out[k] = v
		}
	}
	return out
}

// encodeIdentifier builds a DV_IDENTIFIER from a bare id string or an object
// {id, issuer?, assigner?, type?}. id is mandatory; the optional fields are
// only emitted when non-empty — Cadasto rejects a DV_IDENTIFIER carrying empty
// issuer/assigner/type (see encodeExpandedValue note). Returns nil for an
// empty id so the caller can omit the element entirely.
func encodeIdentifier(payload any) map[string]any {
	switch v := payload.(type) {
	case string:
		if v == "" {
			return nil
		}
		return map[string]any{"_type": "DV_IDENTIFIER", "id": v}
	case map[string]any:
		id := stringOrDefault(v["id"], "")
		if id == "" {
			return nil
		}
		out := map[string]any{"_type": "DV_IDENTIFIER", "id": id}
		for _, k := range []string{"issuer", "assigner", "type"} {
			if s := stringOrDefault(v[k], ""); s != "" {
				out[k] = s
			}
		}
		return out
	default:
		return nil
	}
}

// encodeOrdinal rebuilds a DV_ORDINAL from its short form (the bare symbol code,
// e.g. "at0005" or "local::at0005"). The C_DV_ORDINAL constraint supplies the
// ordinal integer + symbol terminology for the matching code; the term map
// supplies the symbol display text. An object payload is passed through.
func encodeOrdinal(constraint template.ObjectNode, payload any, terms map[string]string) (map[string]any, error) {
	if m, ok := payload.(map[string]any); ok {
		out := map[string]any{"_type": "DV_ORDINAL"}
		maps.Copy(out, m)
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
		maps.Copy(out, m)
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
	// LET OP: GEEN lege issuer/assigner/type aan een DV_IDENTIFIER toevoegen.
	// Cadasto weigert dan de hele composition met 400 "Request data could not
	// be converted to valid object" (bewezen 2026-06-01). De optionele velden
	// horen ofwel een echte waarde te hebben ofwel afwezig te zijn — een
	// `id`-only DV_IDENTIFIER is correct. mConsole's <issuer/> is een
	// XML-render-artefact, geen submitbare canonical JSON.
	return out
}

func encodeQuantity(constraint template.ObjectNode, payload any) (map[string]any, error) {
	units, precision := quantityDefault(constraint)
	if m, ok := payload.(map[string]any); ok {
		out := map[string]any{"_type": "DV_QUANTITY"}
		maps.Copy(out, m)
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
		maps.Copy(out, m)
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
		maps.Copy(out, m)
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
	// An archetype-root container (the person_details ITEM_TREE, a slotted
	// CLUSTER archetype like person_identifier.v2) is addressed by its
	// archetype id — not the bare at0000 root node — and must carry
	// archetype_details. Cadasto rejects the bare node id ("Invalid archetype
	// node ID at0000").
	if ar, ok := container.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
		out["archetype_node_id"] = ar.ArchetypeID()
		out["archetype_details"] = archetypeDetails(ar.ArchetypeID(), "")
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
// As a compact alternative when `_code` is absent, `name` accepts the
// shorthand `"<terminology>::<code>|<display>"` (REQ-058 extension).
// A `name` without `|` is treated as a plain display string (DV_TEXT).
// `_code` always takes precedence over the `name` shorthand when both present.
//
// Returns a DV_CODED_TEXT map when a code is resolved; a plain DV_TEXT with
// the explicit name string or the template label otherwise.
func clusterName(payload map[string]any, label string) map[string]any {
	terminology, codeStr, expandedDisplay, ok := parseCodeField(payload["_code"])
	if !ok {
		// Try compact name shorthand: "term::code|display"
		if nameStr, _ := payload["name"].(string); nameStr != "" {
			terminology, codeStr, expandedDisplay, ok = parseNameShorthand(nameStr)
			if !ok {
				// Plain name string → use as DV_TEXT display value
				return dvText(nameStr)
			}
		}
	}
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

// parseNameShorthand parses the compact `"<terminology>::<code>|<display>"`
// shorthand accepted by the `name` key (REQ-058 extension). Returns ok=false
// when the string contains no `|` separator (treated as plain text by the
// caller) or when the left part yields an empty code.
func parseNameShorthand(s string) (terminology, code, display string, ok bool) {
	idx := indexOf(s, "|")
	if idx < 0 {
		return "", "", "", false
	}
	left := s[:idx]
	right := s[idx+1:]
	t, c := splitTerminology(left)
	if c == "" {
		return "", "", "", false
	}
	return t, c, right, true
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

// categoryValueForCode maps an openEHR "composition category" code to its
// rubric. Covers the two codes the encoder emits; an unknown code falls back to
// "event" (the RM default category). REQ-0029.
func categoryValueForCode(code string) string {
	switch code {
	case "431":
		return "persistent"
	default:
		return "event"
	}
}

// optCategoryCode returns the composition category code the OPT pins on
// /category/defining_code (openEHR terminology group "composition category":
// 431|persistent, 433|event, …), or "" when the OPT leaves it unconstrained.
// A care_plan OPT pins 431|persistent, which the caller uses to omit the
// EVENT_CONTEXT block (a persistent COMPOSITION has no context). REQ-0029.
func optCategoryCode(root template.ObjectNode) string {
	for _, attr := range root.Attributes() {
		if attr.Name() != "category" {
			continue
		}
		for _, child := range attr.Children() {
			obj, ok := child.(template.ObjectNode)
			if !ok {
				continue
			}
			for _, a2 := range obj.Attributes() {
				if a2.Name() != "defining_code" {
					continue
				}
				for _, leaf := range a2.Children() {
					pc, ok := leaf.(interface {
						PrimitiveConstraint() constraints.PrimitiveConstraint
					})
					if !ok {
						continue
					}
					switch cp := pc.PrimitiveConstraint().(type) {
					case constraints.CodePhrase:
						if len(cp.CodeList) > 0 {
							return cp.CodeList[0]
						}
					case *constraints.CodePhrase:
						if len(cp.CodeList) > 0 {
							return cp.CodeList[0]
						}
					}
				}
			}
		}
	}
	return ""
}

// codedTextFromPayload builds a DV_CODED_TEXT from a payload map with string
// keys code/value/terminology. Returns ok=false when the value is absent or
// not a well-formed coded-text map (caller keeps its default). REQ-0029.
func codedTextFromPayload(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	code, _ := m["code"].(string)
	value, _ := m["value"].(string)
	term, _ := m["terminology"].(string)
	if code == "" || term == "" {
		return nil, false
	}
	if value == "" {
		value = code
	}
	return dvCodedText(value, term, code), true
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
// ToComposition computes for the same root. Delegates the key-matching to
// lookupRootValue and coerces the result to a single map — a []any value
// (a multi-entry root, REQ-0029) does not coerce and yields nil, which is
// correct for this function's callers (toparty.go party sections are always
// single-map payloads).
func lookupRootPayload(content map[string]any, id, label string) map[string]any {
	v, _ := lookupRootValue(content, id, label).(map[string]any)
	return v
}

// lookupRootValue returns the raw content-root payload for an archetype id —
// a map[string]any (single entry) or a []any (multiple entries of the same
// root, REQ-0029) — tolerating any "|label" suffix per lookupRootPayload's
// matching rules. Returns nil when no key matches.
func lookupRootValue(content map[string]any, id, label string) any {
	if v, ok := content[id+"|"+label]; ok {
		return v
	}
	if v, ok := content[id]; ok {
		return v
	}
	prefix := id + "|"
	for k, v := range content {
		if k == id || (len(k) >= len(prefix) && k[:len(prefix)] == prefix) {
			return v
		}
	}
	return nil
}

// rootPayloadList normalizes a content-root value to a list of entry-maps: a
// single map → one element (unchanged, backward-compatible behavior); a
// []any of maps → that list, one COMPOSITION content entry per element
// (REQ-0029, multi-pathway care_plan enrollment); anything else → error.
func rootPayloadList(v any) ([]map[string]any, error) {
	switch t := v.(type) {
	case map[string]any:
		return []map[string]any{t}, nil
	case []any:
		out := make([]map[string]any, 0, len(t))
		for i, e := range t {
			m, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("entry[%d] is not an object", i)
			}
			out = append(out, m)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("content-root payload must be an object or array of objects, got %T", v)
	}
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
	// Label-drift fallback: Empty() and the encoder can derive a node's display
	// label from different term scopes — the party-root dictionary vs. a nested
	// ITEM_TREE archetype's own dictionary (e.g. person_details.v2 at0001 is
	// "Demografische gegevens" at the party root but "Geboortegegevens" in its
	// own ontology). That made the labelled key built by Empty miss the encoder's
	// "nodeID|label" lookup, silently dropping the cluster (birth date never
	// reached Cadasto). nodeIDs are unique within one items collection, so a
	// "nodeID|*" prefix match is unambiguous and recovers the payload.
	prefix := nodeID + "|"
	for k, v := range payload {
		if strings.HasPrefix(k, prefix) {
			return v, true
		}
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
