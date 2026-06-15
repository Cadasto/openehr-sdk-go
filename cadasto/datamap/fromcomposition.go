package datamap

import (
	"errors"
	"fmt"
	"maps"

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
func FromComposition(opt *template.OperationalTemplate, composition map[string]any, opts ...DecodeOption) (map[string]any, error) {
	return fromComposition(opt, composition, false, opts...)
}

// FromCompositionExpanded is like FromComposition but emits the expanded value
// form ({rmType, …RM fields…}) instead of collapsing to short scalars, so
// types + units are preserved (and the result re-encodes losslessly).
func FromCompositionExpanded(opt *template.OperationalTemplate, composition map[string]any, opts ...DecodeOption) (map[string]any, error) {
	return fromComposition(opt, composition, true, opts...)
}

// DecodeOption tweaks the decode path. Default decode mirrors the short
// datamap (archetyped content only); options opt-in to additional RM
// attributes that aren't part of the round-trippable content payload.
type DecodeOption func(*decodeConfig)

type decodeConfig struct {
	feederAudit bool
}

// WithFeederAudit includes the composition's FEEDER_AUDIT (origin/system
// item-ids such as an order- or lab-result-number) in the decoded datamap
// under the "feeder_audit" key. Off by default because feeder_audit is RM
// provenance, not archetyped content — callers (e.g. the diagnostics
// playground) enable it to inspect what ToComposition wrote.
func WithFeederAudit() DecodeOption {
	return func(c *decodeConfig) { c.feederAudit = true }
}

func fromComposition(opt *template.OperationalTemplate, composition map[string]any, expanded bool, opts ...DecodeOption) (map[string]any, error) {
	var cfg decodeConfig
	for _, o := range opts {
		o(&cfg)
	}
	// Per-archetype-root term maps from the OPT, so decoded keys carry the same
	// "atNNNN|Label" labels the Schema emits (SPEC §4.3) and therefore validate.
	rootsByID := map[string]contentRoot{}
	if opt != nil {
		if root, ok := opt.Root().(template.ObjectNode); ok {
			for _, r := range findContentArchetypeRoots(root) {
				r.expanded = expanded
				rootsByID[r.id] = r
			}
		}
	}
	if composition == nil {
		return nil, errors.New("datamap.FromComposition: nil composition")
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

	if cfg.feederAudit {
		if fa := decodeFeederAudit(composition["feeder_audit"]); fa != nil {
			out["feeder_audit"] = fa
		}
	}

	return out, nil
}

// decodeFeederAudit is de inverse van de ToComposition-encoder: het zet de
// canonical FEEDER_AUDIT terug naar de platte datamap-vorm. Retourneert nil
// wanneer er geen feeder_audit op de composition staat.
func decodeFeederAudit(raw any) map[string]any {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	if osa, ok := m["originating_system_audit"].(map[string]any); ok {
		if sysID, _ := osa["system_id"].(string); sysID != "" {
			out["originating_system_audit"] = map[string]any{"system_id": sysID}
		}
	}
	if rawIDs, ok := m["originating_system_item_ids"].([]any); ok {
		ids := make([]any, 0, len(rawIDs))
		for _, ri := range rawIDs {
			im, ok := ri.(map[string]any)
			if !ok {
				continue
			}
			dvID := map[string]any{}
			for _, k := range []string{"id", "issuer", "assigner", "type"} {
				if v, _ := im[k].(string); v != "" {
					dvID[k] = v
				}
			}
			if len(dvID) > 0 {
				ids = append(ids, dvID)
			}
		}
		if len(ids) > 0 {
			out["originating_system_item_ids"] = ids
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
		// protocol (bv. Test request details) — alleen terugzetten wanneer de
		// composition er daadwerkelijk een gevulde protocol-ITEM_TREE voor
		// heeft, zodat we geen lege "protocol"-key emitten.
		if proto, ok := node["protocol"].(map[string]any); ok {
			items, err := decodeItems(structuredItemsList(proto), r)
			if err != nil {
				return "", nil, fmt.Errorf("protocol: %w", err)
			}
			if len(items) > 0 {
				payload["protocol"] = items
			}
		}
	case "INSTRUCTION":
		actsRaw, _ := node["activities"].([]any)
		acts := make([]any, 0, len(actsRaw))
		for i, aRaw := range actsRaw {
			a, ok := aRaw.(map[string]any)
			if !ok {
				return "", nil, fmt.Errorf("activities[%d] is not an object", i)
			}
			decoded, err := decodeActivity(a, r)
			if err != nil {
				return "", nil, fmt.Errorf("activities[%d]: %w", i, err)
			}
			acts = append(acts, decoded)
		}
		payload["activities"] = acts
		// protocol (bv. order-/aanvrager-details) — spiegelt de OBSERVATION-tak:
		// alleen terugzetten bij een gevulde protocol-ITEM_TREE.
		if proto, ok := node["protocol"].(map[string]any); ok {
			items, err := decodeItems(structuredItemsList(proto), r)
			if err != nil {
				return "", nil, fmt.Errorf("protocol: %w", err)
			}
			if len(items) > 0 {
				payload["protocol"] = items
			}
		}
		// narrative (INSTRUCTION RM-attribuut, DV_TEXT) — emit de bare value
		// wanneer aanwezig.
		if nv := readDVValue(node["narrative"]); nv != nil && nv != "" {
			payload["narrative"] = nv
		}
		// guideline_id (OBJECT_REF) — passthrough terug de datamap in, zodat
		// de formulier-referentie niet verloren gaat bij decoderen.
		if g, ok := node["guideline_id"].(map[string]any); ok && len(g) > 0 {
			payload["guideline_id"] = g
		}
	case "ACTION":
		if t := readDVValue(node["time"]); t != nil && t != "" {
			payload["time"] = t
		}
		desc, _ := node["description"].(map[string]any)
		items, err := decodeItems(structuredItemsList(desc), r)
		if err != nil {
			return "", nil, err
		}
		maps.Copy(payload, items)
	case "EVALUATION", "ADMIN_ENTRY":
		items, err := decodeItems(structuredItemsList(data), r)
		if err != nil {
			return "", nil, err
		}
		maps.Copy(payload, items)
	default:
		return "", nil, fmt.Errorf("RM entry type %q not supported", rmType)
	}

	return key, payload, nil
}

// decodeActivity decodes one ACTIVITY: its description ITEM_TREE items plus an
// optional timing string (DV_PARSABLE/DV_TEXT value).
func decodeActivity(activity map[string]any, r contentRoot) (map[string]any, error) {
	out := map[string]any{}
	if t := readDVValue(activity["timing"]); t != nil && t != "" {
		out["timing"] = t
	}
	desc, _ := activity["description"].(map[string]any)
	items, err := decodeItems(structuredItemsList(desc), r)
	if err != nil {
		return nil, err
	}
	maps.Copy(out, items)
	return out, nil
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
	items, err := decodeItems(structuredItemsList(data), r)
	if err != nil {
		return nil, err
	}
	maps.Copy(out, items)
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
			decoded = decodeElementValue(item["value"], r.expanded)
		default:
			continue
		}
		// Skip empty/null element values — they aren't in the datamap and the
		// strict schema would reject a bare null.
		if decoded == nil {
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
func decodeElementValue(v any, expanded bool) any {
	value, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if expanded {
		// Expanded form: the RM value with rmType discriminator (drop _type and
		// nulls). Re-encodes losslessly via encodeValue's rmType path.
		out := map[string]any{}
		for k, val := range value {
			if k != "_type" && val != nil {
				out[k] = val
			}
		}
		if len(out) == 0 {
			return nil
		}
		if rmType, _ := value["_type"].(string); rmType != "" {
			out["rmType"] = rmType
		}
		return out
	}
	switch rmType, _ := value["_type"].(string); rmType {
	case "DV_TEXT", "DV_DATE_TIME", "DV_DATE", "DV_TIME", "DV_BOOLEAN", "DV_URI":
		return value["value"] // nil when empty → caller skips it
	case "DV_COUNT", "DV_QUANTITY", "DV_PROPORTION":
		// Datamap short form is the bare magnitude number; units/precision are
		// template-derived and refilled by ToComposition.
		return value["magnitude"]
	case "DV_CODED_TEXT":
		// Short form is the bare code; a coded value with no code is free text.
		if code := readCodePhraseCode(value["defining_code"]); code != "" {
			return code
		}
		if s, ok := value["value"].(string); ok && s != "" {
			return s
		}
		return nil
	case "DV_ORDINAL":
		// The code lives under symbol.defining_code.
		if sym, ok := value["symbol"].(map[string]any); ok {
			if code := readCodePhraseCode(sym["defining_code"]); code != "" {
				return code
			}
		}
		return nil
	}
	// Defensive: any value still carrying a defining_code is a coded value whose
	// _type wasn't matched above — collapse it to its code.
	if code := readCodePhraseCode(value["defining_code"]); code != "" {
		return code
	}
	// Fallback: strip RM bookkeeping. An object that collapses to nothing
	// meaningful (all-null) yields nil so the caller omits the empty field.
	out := map[string]any{}
	for k, val := range value {
		if k != "_type" && val != nil {
			out[k] = val
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// CompositionTemplateID extracts the template_id from a canonical composition's
// archetype_details, the OPT id needed to decode it via FromComposition.
// Returns "" when absent.
func CompositionTemplateID(comp map[string]any) string {
	ad, ok := comp["archetype_details"].(map[string]any)
	if !ok {
		return ""
	}
	tid, ok := ad["template_id"].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := tid["value"].(string)
	return v
}

// structuredItemsList extracts the ELEMENT/CLUSTER list from a decoded
// ITEM_STRUCTURE RM object, across subtypes: ITEM_TREE/ITEM_LIST use "items",
// ITEM_TABLE uses "rows", ITEM_SINGLE uses a single "item" (wrapped to a list
// so decodeItems can treat all containers uniformly).
func structuredItemsList(container map[string]any) []any {
	if v, ok := container["items"]; ok {
		return asList(v)
	}
	if v, ok := container["rows"]; ok {
		return asList(v)
	}
	if v, ok := container["item"]; ok && v != nil {
		return []any{v}
	}
	return nil
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
