package webtemplate

// PROBE-086, PROBE-105 — the reference's spelling of a duplicate sibling id,
// read in this SDK's. REQ-106 spells the second and later of siblings that
// share an id `<id>_1`, `<id>_2`, … where the reference writes `<id>2`,
// `<id>3`, … (the recorded divergence in
// openehr/template/webtemplate/deviations.md § Duplicate sibling ids). An
// upstream FLAT body keys such a sibling the reference's way, and the SDK's
// re-encode keys it this SDK's way, so a comparison across the two spellings
// would report the key as both missing and extra. The harnesses therefore
// read the upstream side in this SDK's spelling before they compare, the way
// [IsCompositionMeta] holds out the metadata respellings.

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	wt "github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// Respelling is one upstream FLAT key that [RespellSiblingIDs] read in this
// SDK's spelling of a duplicate sibling id.
type Respelling struct {
	// Upstream is the key as the reference wrote it.
	Upstream string
	// Ours is the key with each duplicate sibling id spelled the way the Web
	// Template spells it.
	Ours string
}

// String renders the respelling as "upstream -> ours".
func (r Respelling) String() string { return r.Upstream + " -> " + r.Ours }

// RespellSiblingIDs returns a copy of body in which every key that spells a
// duplicate sibling id the reference's way is spelled the way the Web
// Template rooted at root spells it, and the respellings it made, sorted by
// upstream key. Every other key is copied as written, and body itself is not
// changed.
//
// The reference's rule is OPTParser.makeIdUnique: in a group of siblings
// that share an id, the member at index i, counting from 0, gets i+1 written
// straight after the id. Its `<id><N+1>` is therefore this SDK's `<id>_<N>`.
// A path segment is read that way only where the Web Template shows the
// duplicate group, so the mapping cannot reach a key it does not explain:
//
//   - the segment names no child of the node before it by its own id;
//   - it is `<id><M>`, M a decimal number of 2 or more with no leading zero;
//   - that node has the children `<id>` and `<id>_1` to `<id>_<M-1>`.
//
// The segment then becomes `<id>_<M-1>`, its `:index` kept. The walk follows
// the Web Template from root and stops at the first segment that names no
// child either way (an underscore family, a leaf's sub-path), so no segment
// after it is read. A key not rooted at root, a `ctx/` key among them, is
// copied as written.
//
// The Web Template cannot tell a duplicate from a sibling whose own name
// sanitises to `<id>_<N>`. Beside such a sibling the builder skips to the
// next free suffix, and this mapping would read the reference's `<id><N+1>`
// as that sibling. The callers pin the respellings they make, so such a case
// shows up as a changed pin rather than a quiet match.
//
// Two keys that come to one spelling, or a respelled key the body also
// carries as written, are an error naming both: the body then spells one
// field twice, and the comparison would have to drop one of the two.
func RespellSiblingIDs(root *wt.Node, body map[string]any) (map[string]any, []Respelling, error) {
	var respelled []Respelling
	for _, key := range slices.Sorted(maps.Keys(body)) {
		if ours, ok := respellKey(root, key); ok {
			respelled = append(respelled, Respelling{Upstream: key, Ours: ours})
		}
	}
	out := make(map[string]any, len(body))
	maps.Copy(out, body)
	for _, r := range respelled {
		delete(out, r.Upstream)
	}
	// A respelled key names every segment it walks by a child's own id, so a
	// key written that way is never respelled itself: body[r.Ours] is a key
	// as written.
	from := make(map[string]string, len(respelled))
	for _, r := range respelled {
		if _, written := body[r.Ours]; written {
			return nil, nil, fmt.Errorf("upstream keys %q and %q spell one duplicate sibling twice", r.Upstream, r.Ours)
		}
		if prev, twice := from[r.Ours]; twice {
			return nil, nil, fmt.Errorf("upstream keys %q and %q spell one duplicate sibling twice", prev, r.Upstream)
		}
		from[r.Ours] = r.Upstream
		out[r.Ours] = body[r.Upstream]
	}
	return out, respelled, nil
}

// respellKey returns key with each segment that spells a duplicate sibling
// the reference's way ([referenceClaimant]) respelled, and whether it
// respelled any.
func respellKey(root *wt.Node, key string) (string, bool) {
	if root == nil {
		return key, false
	}
	path, suffix, piped := strings.Cut(key, "|")
	segs := strings.Split(path, "/")
	if id, _ := cutIndex(segs[0]); id != root.ID {
		return key, false
	}
	node, respelled := root, false
	for i := 1; i < len(segs) && node != nil; i++ {
		id, index := cutIndex(segs[i])
		next := childNamed(node, id)
		if next == nil {
			if next = referenceClaimant(node, id); next != nil {
				segs[i] = next.ID + index
				respelled = true
			}
		}
		node = next
	}
	if !respelled {
		return key, false
	}
	out := strings.Join(segs, "/")
	if piped {
		out += "|" + suffix
	}
	return out, true
}

// referenceClaimant returns the child of node that id names in the
// reference's spelling of a duplicate sibling, or nil: the rules are listed
// on [RespellSiblingIDs]. A base may end in digits, so id may split more than
// one way (`x12` is `x` and 12, or `x1` and 2), and the shortest base whose
// duplicate group node holds wins.
func referenceClaimant(node *wt.Node, id string) *wt.Node {
	digitsFrom := len(strings.TrimRightFunc(id, func(r rune) bool { return '0' <= r && r <= '9' }))
	for i := max(digitsFrom, 1); i < len(id); i++ {
		base, number := id[:i], id[i:]
		m, err := strconv.Atoi(number)
		if number[0] == '0' || err != nil || m < 2 || childNamed(node, base) == nil {
			continue
		}
		var claimant *wt.Node
		for n := 1; n < m; n++ {
			if claimant = childNamed(node, base+"_"+strconv.Itoa(n)); claimant == nil {
				break
			}
		}
		if claimant != nil {
			return claimant
		}
	}
	return nil
}

// childNamed returns the child of node whose id is id, or nil.
func childNamed(node *wt.Node, id string) *wt.Node {
	for _, ch := range node.Children {
		if ch.ID == id {
			return ch
		}
	}
	return nil
}

// cutIndex splits a FLAT path segment into its id and its `:index`, the colon
// kept, or "" when it has none.
func cutIndex(seg string) (id, index string) {
	if i := strings.IndexByte(seg, ':'); i >= 0 {
		return seg[:i], seg[i:]
	}
	return seg, ""
}
