package simplified

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// rebuildArchetypeDetails gives every node of the decoded canonical tree that
// the Web Template identifies by an archetype id an ARCHETYPED, rebuilt from
// the Web Template. FLAT and STRUCTURED carry no key for
// LOCATABLE.archetype_details because the Web Template already records what it
// holds: the archetype id is the node's own id, and the template id is the Web
// Template's. rm_version is the RM release the SDK is generated from
// ([rm.Release]), never a value from the payload. The COMPOSITION root alone
// also gets template_id.
//
// Decode owns the attribute: it writes it on every qualifying node and keeps
// nothing that was there before. Neither value needs the compiled template, so
// both decode modes run this. It has to run after the phantom check: an
// ARCHETYPED added to a gap-filled instance would make it look populated.
func rebuildArchetypeDetails(compJSON map[string]any, wt *webtemplate.WebTemplate) {
	attachArchetypeDetails(compJSON, archetypeRootIDs(wt))
	if wt.TemplateID == "" {
		return
	}
	if ad, ok := compJSON["archetype_details"].(map[string]any); ok {
		ad["template_id"] = map[string]any{"_type": "TEMPLATE_ID", "value": wt.TemplateID}
	}
}

// attachArchetypeDetails walks a canonical subtree and writes an ARCHETYPED,
// without template_id, on every object whose archetype_node_id is in roots.
func attachArchetypeDetails(n any, roots map[string]bool) {
	switch x := n.(type) {
	case map[string]any:
		if id, ok := x["archetype_node_id"].(string); ok && roots[id] {
			x["archetype_details"] = archetypedJSON(id)
		}
		for k, v := range x {
			if k != "archetype_details" {
				attachArchetypeDetails(v, roots)
			}
		}
	case []any:
		for _, v := range x {
			attachArchetypeDetails(v, roots)
		}
	}
}

// archetypedJSON is the canonical-JSON ARCHETYPED for an archetype root with
// the given archetype id.
func archetypedJSON(archetypeID string) map[string]any {
	return map[string]any{
		"_type":        "ARCHETYPED",
		"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": archetypeID},
		"rm_version":   rm.Release,
	}
}

// archetypeRootIDs returns the archetype ids by which the Web Template
// identifies a node. Two signals decide it together:
//
//   - The id must come from the Web Template. A node carries it as its nodeId;
//     a structural wrapper the Web Template folds away (an archetyped ITEM_TREE
//     under an ACTIVITY description, for instance) has no node of its own, and
//     carries it only as the predicate its children's aqlPaths hold for it.
//     Decode rebuilds that wrapper with the predicate as its archetype_node_id,
//     so both places are read.
//   - The id must have the ARCHETYPE_ID lexical form ([rm.ParseArchetypeID]).
//     The Web Template has no archetype-root flag, so the lexical form is what
//     tells an archetype id from an at-code or id-code, which never qualifies.
func archetypeRootIDs(wt *webtemplate.WebTemplate) map[string]bool {
	ids := make(map[string]bool)
	if wt == nil || wt.Tree == nil {
		return ids
	}
	add := func(id string) {
		if id == "" || ids[id] {
			return
		}
		if _, err := rm.ParseArchetypeID(id); err == nil {
			ids[id] = true
		}
	}
	stack := []*webtemplate.Node{wt.Tree}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		add(n.NodeID)
		for _, seg := range parseAQL(n.AQLPath) {
			add(seg.pred)
		}
		stack = append(stack, n.Children...)
	}
	return ids
}
