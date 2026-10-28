package lint

import (
	"fmt"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// templateIssues runs Layer 3 against a compiled OPT: archetype membership
// (aql_archetype_not_in_template, Error) and archetype-scoped path resolution
// (aql_path_not_in_template, Warning).
func templateIssues(md Metadata, c *tcimpl.Compiled) []Issue {
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
		if !ok || ce.Archetype == "" || ce.ParamArchetype {
			// Unbound (Layer 2), no archetype, or $param. A $param carries no
			// literal subtree to walk — the CDR resolves the bound scope at
			// execution (PROBE-021). The ParamArchetype arm is belt-and-braces:
			// Archetype holds the placeholder text ("$arch"), which no compiled
			// index can match, so the len(roots)==0 continue below would absorb
			// the case anyway — but through the wrong reason ("archetype
			// absent"), and only while that lookup stays a miss.
			continue
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
		//
		// NOTE: every OPT fixture in the suite compiles each archetype to a
		// single root, so the multi-root (len(roots) > 1) path is exercised
		// by construction only — no fixture covers it. A synthetic
		// multi-slot OPT would be needed to test it directly.
		var (
			bad         string
			found       bool
			divergedAll = true
		)
		for _, root := range roots {
			seg, diverged := pathDivergence(root, norm.Segments)
			if !diverged {
				divergedAll = false
				break
			}
			if !found {
				bad, found = seg, true
			}
		}
		if divergedAll {
			issues = append(issues, Issue{
				Code: "aql_path_not_in_template",
				Path: displayPath(p),
				Detail: fmt.Sprintf(
					"segment %q not found under %s (path may still resolve at the CDR)",
					bad, ce.Archetype,
				),
				Severity: Warning,
				// The whole path is the finding's extent; the diverging segment
				// is named in Detail, which is value-bearing anyway.
				Span: spanOfText(p.Pos, p.Raw),
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
func pathDivergence(node *tcimpl.CompiledNode, segs []parse.PathSegment) (bad string, diverged bool) {
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
		if node = pickChild(children, seg); node == nil {
			return "", false // the predicate names no child here; nothing to compare below it
		}
	}
	return "", false
}

// pickChild selects the child to descend into. A predicate that names a
// child, by its node id (at-/id-code) or, at an archetype root, by its
// archetype id, with or without a node name, or such a term AND-ed with
// another, selects the first child that id names. When no child carries it,
// pickChild returns nil: the path names a node the template does not have
// here, which is no high-confidence divergence, so the walk stops without a
// warning rather than descend into a sibling the predicate does not select.
// A segment with no predicate, or one that names no child, takes the first
// child deterministically (lenient first-child, mirroring
// template.OperationalTemplate.NodeAt).
//
// The typed predicate ([aql.PathSegment].Parsed) is read where the parser
// structured it. Otherwise the verbatim text is compared with the lexer's
// SKIPPED trivia removed, because REQ-119 made the read side report bracket
// text VERBATIM: `events[ at0001 ]` and `events[at0001\n]` arrive with their
// padding, where `GetText()` once dropped it. Compared raw, the named child
// became unreachable and the lenient fallback silently descended a SIBLING —
// which turned a valid path into an `aql_path_not_in_template` warning
// wherever the siblings differ structurally. REQ-109 § Layer 3 calls this
// descent "predicate-aware" and warns only on high-confidence divergence, so
// the normalisation belongs to the COMPARISON, not to the reported text
// ([Path.Segments] stays verbatim).
func pickChild(children []*tcimpl.CompiledNode, seg aql.PathSegment) *tcimpl.CompiledNode {
	nodeID, archetypeID := predicateIDs(seg)
	if nodeID == "" && archetypeID == "" {
		return children[0]
	}
	for _, child := range children {
		if (nodeID != "" && child.NodeID() == nodeID) || (archetypeID != "" && child.ArchetypeID() == archetypeID) {
			return child
		}
	}
	return nil
}

// predicateIDs returns the node id or the archetype id seg's predicate
// selects a child by, "" for each it does not name.
func predicateIDs(seg aql.PathSegment) (nodeID, archetypeID string) {
	if seg.Parsed == nil {
		return aql.StripPredicateTrivia(seg.Predicate), ""
	}
	var visit func(p aql.SegmentPredicate) bool
	visit = func(p aql.SegmentPredicate) bool {
		switch p := p.(type) {
		case aql.NodeIDPredicate:
			nodeID = p.ID
		case aql.ArchetypePredicate:
			archetypeID = p.HRID
		case aql.JunctionPredicate:
			// Only a conjunction narrows to the id one operand names.
			return p.Op == aql.OpAnd && (visit(p.Left) || visit(p.Right))
		default:
			return false
		}
		return true
	}
	visit(seg.Parsed)
	return nodeID, archetypeID
}
