package lint

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// templateIssues runs Layer 3 against a compiled OPT: archetype membership
// (aql_archetype_not_in_template, Error) and archetype-scoped path resolution
// (aql_path_not_in_template, Warning).
func templateIssues(md Metadata, c *templatecompile.Compiled) []Issue {
	var issues []Issue

	// aql_archetype_not_in_template — every literal archetype HRID named in
	// FROM/CONTAINS MUST be present in the template.
	for _, hrid := range md.Archetypes {
		if len(c.AllByArchetypeID(hrid)) == 0 {
			issues = append(issues, Issue{
				Code:     "aql_archetype_not_in_template",
				Path:     hrid,
				Detail:   fmt.Sprintf("archetype %s is not in template %s", hrid, c.TemplateID()),
				Severity: Error,
			})
		}
	}

	// aql_path_not_in_template — best-effort, archetype-scoped. For each
	// identified path whose alias binds to an archetype present in the
	// template, walk the path's structure against that archetype's compiled
	// subtree. See pathDivergence for the (deliberately conservative)
	// false-positive policy.
	for _, p := range md.Paths {
		ce, ok := md.Aliases[p.Alias]
		if !ok || ce.Archetype == "" {
			continue // unbound (Layer 2) or no literal archetype to anchor to
		}
		roots := c.AllByArchetypeID(ce.Archetype)
		if len(roots) == 0 {
			continue // archetype absent — already flagged above
		}
		norm, err := Normalise(p)
		if err != nil || len(norm.Segments) == 0 {
			continue // bare alias root — nothing to resolve
		}
		// The same archetype may fill several slots (several roots). Warn
		// only when the path diverges under EVERY root; report the first
		// divergence. A path valid under any one root is not flagged.
		bad, divergedAll := "", true
		for _, root := range roots {
			seg, diverged := pathDivergence(root, norm.Segments)
			if !diverged {
				divergedAll = false
				break
			}
			if bad == "" {
				bad = seg
			}
		}
		if divergedAll {
			issues = append(issues, Issue{
				Code: "aql_path_not_in_template",
				Path: p.Raw,
				Detail: fmt.Sprintf(
					"segment %q not found under %s (path may still resolve at the CDR)",
					bad, ce.Archetype,
				),
				Severity: Warning,
			})
		}
	}
	return issues
}

// pathDivergence walks segs (alias already stripped) down node's compiled
// subtree. It returns diverged=true only on a high-confidence structural
// miss: a segment names an attribute that does not exist on a node that *has*
// modelled attributes. It deliberately does NOT diverge when:
//
//   - a node is a true leaf (no modelled attributes) — the remaining segments
//     are unmodelled RM attributes (e.g. /value/magnitude), which the OPT
//     does not index; or
//   - an attribute exists but pins no child node (implicit RM-mandatory
//     attribute) — the path descends below the modelled tree.
//
// Consequence (documented false-positive policy, REQ-109): a path through a
// non-mandatory RM attribute the OPT did not constrain may still false-warn.
// The check is a Warning precisely because the CDR is the path authority.
func pathDivergence(node *templatecompile.CompiledNode, segs []parse.PathSegment) (bad string, diverged bool) {
	for _, seg := range segs {
		attr := node.Attribute(seg.Name)
		if attr == nil {
			if len(node.Attributes()) == 0 {
				return "", false // leaf; remaining segments are unmodelled RM attrs
			}
			return seg.Name, true // structural divergence at a node with children
		}
		children := attr.Children()
		if len(children) == 0 {
			return "", false // attribute pins no node; below the modelled tree
		}
		node = pickChild(children, seg.Predicate)
	}
	return "", false
}

// pickChild selects the child to descend into. When the path segment carries
// a predicate matching a child's node id (at-/id-code), that child is taken;
// otherwise the first child is taken deterministically (lenient first-child,
// mirroring template.OperationalTemplate.NodeAt).
func pickChild(children []*templatecompile.CompiledNode, predicate string) *templatecompile.CompiledNode {
	if predicate != "" {
		for _, child := range children {
			if child.NodeID() == predicate {
				return child
			}
		}
	}
	return children[0]
}
