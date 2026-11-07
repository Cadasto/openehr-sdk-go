package simplified

// REQ-053 / REQ-116 — reused siblings: Web Template nodes that share a node id
// under one parent. Where pinned names tell them apart, encode and decode carry
// the names into resolution and placement; where nothing tells them apart, both
// refuse data there.

import (
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// reusedSiblings is what the codec reads about reused siblings from one Web
// Template.
//
// The Web Template writes a pinned name into a node's aqlPath (REQ-116), and
// the codec resolves a node by its *resolution path*: the aqlPath with only
// the names that tell reused siblings apart. Siblings are the nodes under one
// parent that share an attribute and a node id. Where two or more of them are
// spelled differently and at most one pins no name, the names tell them apart:
//
//   - a sibling that pins a name stands for the instances carrying that name,
//     and its name stays in the resolution path;
//   - the sibling that pins none stands for every other instance there, and
//     is resolved by node id with those names excluded.
//
// Every other name is removed, because resolving by it would drop an instance
// whose stored name differs from the name the template models it with (see
// deviations.md § Conformance). Siblings spelled alike share their resolution
// path, and no name tells them apart.
type reusedSiblings struct {
	// kept holds the aqlPath prefix, as the Web Template spells it, that ends
	// at a segment whose name stays in the resolution path.
	kept map[string]bool
	// excluded maps the resolution path of a sibling that pins no name, among
	// siblings that pin one, to their names: it stands for every instance
	// there that carries none of them.
	excluded map[string][]string
	// paths maps each Web Template node to its resolution path. It is nil
	// where kept is empty, and the resolution path is then the bare path.
	paths map[*webtemplate.Node]string
	// shared holds the resolution paths that more than one node has: reused
	// siblings that no pinned name tells apart.
	shared map[string]bool
	// claimed are the sets of siblings that each pin a name, in the order the
	// Web Template first reaches them.
	claimed []siblingGroup
}

// siblingGroup is one set of reused siblings that each pin a name.
type siblingGroup struct {
	// parent is the aqlPath of the siblings' parent as the Web Template spells
	// it, and segment their shared segment without a name: `/attr[node id]`.
	parent, segment string
	// names are the names the siblings pin, one each.
	names []string
}

// rawSegment is one segment of an aqlPath p as the Web Template spells it:
// p[start:end] is `/attr` or `/attr[node id]` or `/attr[node id,'name']`, and
// p[start:idEnd] is the segment up to the end of its node id. named reports
// whether it carries a name, and bracketed whether it carries a predicate.
type rawSegment struct {
	start, idEnd, end int
	bracketed, named  bool
}

// bare returns seg of p without its name: `/attr` or `/attr[node id]`.
func (seg rawSegment) bare(p string) string {
	if !seg.bracketed {
		return p[seg.start:seg.end]
	}
	return p[seg.start:seg.idEnd] + "]"
}

// name returns the name seg of p carries, its escapes undone.
func (seg rawSegment) name(p string) string {
	// p[idEnd:end] is `,'name']`.
	if !seg.named || seg.end-2 < seg.idEnd+2 {
		return ""
	}
	quoted := p[seg.idEnd+2 : seg.end-2]
	if !strings.Contains(quoted, `\`) {
		return quoted
	}
	var b strings.Builder
	for i := 0; i < len(quoted); i++ {
		if quoted[i] == '\\' && i+1 < len(quoted) {
			i++
		}
		b.WriteByte(quoted[i])
	}
	return b.String()
}

// rawSegments yields the segments of p, an aqlPath as the Web Template spells
// it. A name may hold a slash, a bracket or a comma, and `\'` and `\\` inside
// it are escapes, as [bareAQLPath] reads them.
func rawSegments(p string) iter.Seq[rawSegment] {
	return func(yield func(rawSegment) bool) {
		for i := 0; i < len(p); {
			if p[i] != '/' {
				i++
				continue
			}
			j := i + 1
			for j < len(p) && p[j] != '/' && p[j] != '[' {
				j++
			}
			seg := rawSegment{start: i, idEnd: j, end: j}
			if j < len(p) && p[j] == '[' {
				seg.bracketed = true
				k := j + 1
				for k < len(p) && p[k] != ']' && (p[k] != ',' || k+1 >= len(p) || p[k+1] != '\'') {
					k++
				}
				seg.idEnd = k
				if k < len(p) && p[k] == ',' {
					seg.named = true
					k += 2
					for k < len(p) {
						if p[k] == '\\' && k+1 < len(p) {
							k += 2
							continue
						}
						k++
						if p[k-1] == '\'' {
							break
						}
					}
					for k < len(p) && p[k] != ']' {
						k++
					}
				}
				if k < len(p) {
					k++
				}
				seg.end = k
			}
			if !yield(seg) {
				return
			}
			i = seg.end
		}
	}
}

// reusedSiblingsOf reads the reused siblings of wt.
func reusedSiblingsOf(wt *webtemplate.WebTemplate) *reusedSiblings {
	s := &reusedSiblings{shared: make(map[string]bool)}
	if wt == nil || wt.Tree == nil {
		return s
	}
	var nodes []*webtemplate.Node
	named := false
	var collect func(n *webtemplate.Node)
	collect = func(n *webtemplate.Node) {
		nodes = append(nodes, n)
		named = named || strings.Contains(n.AQLPath, ",'")
		for _, ch := range n.Children {
			collect(ch)
		}
	}
	collect(wt.Tree)
	if named {
		s.readSiblings(nodes)
	}
	if len(s.kept) > 0 {
		s.paths = make(map[*webtemplate.Node]string, len(nodes))
		for _, n := range nodes {
			s.paths[n] = s.resolutionPath(n.AQLPath)
		}
	}
	counts := make(map[string]int, len(nodes))
	for _, n := range nodes {
		counts[s.nodePath(n)]++
	}
	for p, c := range counts {
		if c > 1 {
			s.shared[p] = true
		}
	}
	return s
}

// readSiblings finds the sets of reused siblings among nodes that pinned names
// tell apart: the segments that share the parent's aqlPath, an attribute and a
// node id (p[:idEnd] of each), spelled two or more ways, at most one of them
// without a name.
func (s *reusedSiblings) readSiblings(nodes []*webtemplate.Node) {
	type set struct {
		order           int
		parent, segment string
		spellings       map[string]bool
		named           []string // p[:end] of each spelling that pins a name
		names           []string
		unnamed         string // p[:end] of the spelling that pins none
	}
	sets := make(map[string]*set)
	// A set matters only where one of its spellings carries a name.
	for _, n := range nodes {
		p := n.AQLPath
		for seg := range rawSegments(p) {
			if seg.named && sets[p[:seg.idEnd]] == nil {
				sets[p[:seg.idEnd]] = &set{order: len(sets), parent: p[:seg.start], segment: seg.bare(p), spellings: make(map[string]bool)}
			}
		}
	}
	if len(sets) == 0 {
		return
	}
	for _, n := range nodes {
		p := n.AQLPath
		for seg := range rawSegments(p) {
			g := sets[p[:seg.idEnd]]
			if !seg.bracketed || g == nil || g.spellings[p[:seg.end]] {
				continue
			}
			g.spellings[p[:seg.end]] = true
			if seg.named {
				g.named = append(g.named, p[:seg.end])
				g.names = append(g.names, seg.name(p))
			} else {
				g.unnamed = p[:seg.end]
			}
		}
	}
	ordered := make([]*set, 0, len(sets))
	for _, g := range sets {
		if len(g.spellings) > 1 {
			ordered = append(ordered, g)
		}
	}
	slices.SortFunc(ordered, func(a, b *set) int { return a.order - b.order })
	s.kept = make(map[string]bool)
	for _, g := range ordered {
		for _, m := range g.named {
			s.kept[m] = true
		}
		if g.unnamed == "" {
			s.claimed = append(s.claimed, siblingGroup{parent: g.parent, segment: g.segment, names: g.names})
		}
	}
	for _, g := range ordered {
		if g.unnamed != "" {
			if s.excluded == nil {
				s.excluded = make(map[string][]string)
			}
			s.excluded[s.resolutionPath(g.unnamed)] = g.names
		}
	}
}

// resolutionPath returns p, an aqlPath as the Web Template spells it, with
// only the names [reusedSiblings] keeps.
func (s *reusedSiblings) resolutionPath(p string) string {
	if len(s.kept) == 0 || !strings.Contains(p, ",'") {
		return bareAQLPath(p)
	}
	var b strings.Builder
	b.Grow(len(p))
	last := 0
	for seg := range rawSegments(p) {
		if seg.named && !s.kept[p[:seg.end]] {
			b.WriteString(p[last:seg.idEnd])
			b.WriteByte(']')
			last = seg.end
		}
	}
	b.WriteString(p[last:])
	return b.String()
}

// nodePath returns the resolution path of n.
func (s *reusedSiblings) nodePath(n *webtemplate.Node) string {
	if p, ok := s.paths[n]; ok {
		return p
	}
	return s.resolutionPath(n.AQLPath)
}

// isShared reports whether n shares its resolution path with another node: a
// reused sibling that no pinned name tells apart from the others.
func (s *reusedSiblings) isShared(n *webtemplate.Node) bool {
	return len(s.shared) > 0 && s.shared[s.nodePath(n)]
}

// excludedAt returns the names an instance at the resolution path p must not
// carry: those of the siblings that pin one, where p is the path of the
// sibling that pins none.
func (s *reusedSiblings) excludedAt(p string) []string {
	if s == nil || len(s.excluded) == 0 {
		return nil
	}
	return s.excluded[p]
}

// itemsAt is [rmpath.ItemsAtPath] for the encoder: it resolves rel, a
// resolution path relative to root, whose own resolution path is rootPath.
// Where rel crosses the segment of a sibling that pins no name among siblings
// that pin one, the instances there that carry one of their names are left
// out: they are those siblings'.
func (s *reusedSiblings) itemsAt(root rm.Locatable, rootPath, rel string) ([]any, error) {
	if len(s.excluded) == 0 {
		return rmpath.ItemsAtPath(root, rel)
	}
	abs := rootPath + rel
	items, start := []any{root}, 0
	for seg := range rawSegments(rel) {
		names, ok := s.excluded[abs[:len(rootPath)+seg.end]]
		if !ok {
			continue
		}
		var err error
		if items, err = itemsUnder(items, rel[start:seg.end]); err != nil {
			return nil, err
		}
		items = slices.DeleteFunc(items, func(it any) bool {
			l, ok := it.(rm.Locatable)
			return ok && !rm.IsTypedNil(l) && slices.Contains(names, rm.DVTextValueOf(l.GetName()))
		})
		start = seg.end
	}
	if start == 0 {
		return rmpath.ItemsAtPath(root, rel)
	}
	if start < len(rel) {
		return itemsUnder(items, rel[start:])
	}
	return items, nil
}

// itemsUnder resolves rel from each of roots in turn. A root that is no
// LOCATABLE holds nothing rmpath resolves.
func itemsUnder(roots []any, rel string) ([]any, error) {
	var out []any
	for _, r := range roots {
		l, ok := r.(rm.Locatable)
		if !ok {
			continue
		}
		got, err := rmpath.ItemsAtPath(l, rel)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	return out, nil
}

// itemAt is [itemAtPath] through [reusedSiblings.itemsAt].
func (s *reusedSiblings) itemAt(root rm.Locatable, rootPath, rel string) (any, error) {
	if len(s.excluded) == 0 {
		return itemAtPath(root, rel)
	}
	items, err := s.itemsAt(root, rootPath, rel)
	switch {
	case err != nil:
		return nil, err
	case len(items) == 1:
		return items[0], nil
	case len(items) == 0:
		return nil, rmpath.ErrPathNotFound
	}
	return nil, fmt.Errorf("%w: %q (%d items)", rmpath.ErrPathAmbiguous, rel, len(items))
}

// refuseUnclaimedItems refuses comp where an item stands at the segment of a
// set of reused siblings that each pin a name, but carries none of their
// names. Encode resolves each sibling by its name, so no sibling would claim
// that item, and its data would be dropped without a word. The error wraps
// [rmpath.ErrPathAmbiguous], as the refusal of siblings that no name tells
// apart does, and names no value.
func (s *reusedSiblings) refuseUnclaimedItems(comp *rm.Composition) error {
	for _, g := range s.claimed {
		path := s.resolutionPath(g.parent) + g.segment
		items, err := s.itemsAt(comp, "", path)
		if err != nil {
			return fmt.Errorf("simplified: resolve %q: %w", path, err)
		}
		for _, it := range items {
			l, ok := it.(rm.Locatable)
			if !ok || rm.IsTypedNil(l) {
				continue
			}
			if !slices.Contains(g.names, rm.DVTextValueOf(l.GetName())) {
				return fmt.Errorf("simplified: an item at %q carries a name that none of the reused siblings there pins, so it cannot be told which of them it is (see deviations.md): %w",
					bareAQLPath(path), rmpath.ErrPathAmbiguous)
			}
		}
	}
	return nil
}
