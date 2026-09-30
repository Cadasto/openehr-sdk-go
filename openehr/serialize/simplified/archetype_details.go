package simplified

import (
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// rebuildArchetypeDetails gives every node of the decoded canonical tree that
// sits at an archetype root an ARCHETYPED, rebuilt from the Web Template. FLAT
// and STRUCTURED carry no key for LOCATABLE.archetype_details because the Web
// Template already records what it holds: the archetype id is the node's own
// id, and the template id is the Web Template's. rm_version is the RM release
// the SDK is generated from ([rm.Release]), never a value from the payload.
// The COMPOSITION root alone also gets template_id.
//
// Which node ids are archetype ids is decided by two signals together (see
// [archetypeRootIDs]): the id must be a node id of the Web Template, and it
// must have the ARCHETYPE_ID lexical form. The Web Template has no
// archetype-root flag of its own, so the lexical form is what tells an
// archetype id from an at-code or id-code, and membership in the Web
// Template's node set ties each rebuilt archetype_id to a node the template
// declares.
//
// An archetype_details already present is left as it is. Neither value needs
// the compiled template, so both decode modes run this. It has to run after the
// phantom check: an ARCHETYPED added to a gap-filled instance would make it
// look populated.
func rebuildArchetypeDetails(compJSON map[string]any, wt *webtemplate.WebTemplate) {
	_, rootHad := compJSON["archetype_details"]
	attachArchetypeDetails(compJSON, archetypeRootIDs(wt))
	if rootHad || wt.TemplateID == "" {
		return
	}
	if ad, ok := compJSON["archetype_details"].(map[string]any); ok {
		ad["template_id"] = map[string]any{"_type": "TEMPLATE_ID", "value": wt.TemplateID}
	}
}

// attachArchetypeDetails walks a canonical subtree and adds an ARCHETYPED,
// without template_id, to every object whose archetype_node_id is in roots and
// that carries no archetype_details yet.
func attachArchetypeDetails(n any, roots map[string]bool) {
	switch x := n.(type) {
	case map[string]any:
		if id, ok := x["archetype_node_id"].(string); ok && roots[id] {
			if _, has := x["archetype_details"]; !has {
				x["archetype_details"] = archetypedJSON(id)
			}
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

// archetypeRootIDs returns the Web Template node ids that are archetype ids:
// every nodeId in the tree that parses as an ARCHETYPE_ID ([rm.ParseArchetypeID]).
// An at-code or id-code fails that parse, so it never qualifies.
func archetypeRootIDs(wt *webtemplate.WebTemplate) map[string]bool {
	ids := make(map[string]bool)
	if wt == nil || wt.Tree == nil {
		return ids
	}
	stack := []*webtemplate.Node{wt.Tree}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		if n.NodeID != "" && !ids[n.NodeID] {
			if _, err := rm.ParseArchetypeID(n.NodeID); err == nil {
				ids[n.NodeID] = true
			}
		}
		stack = append(stack, n.Children...)
	}
	return ids
}
