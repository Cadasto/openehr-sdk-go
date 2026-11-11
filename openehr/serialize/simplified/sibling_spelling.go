package simplified

// REQ-053 — a duplicate sibling's earlier spelling. The Web Template gives the
// second and later of siblings that share an id the next free `<id>_1`,
// `<id>_2`, … (REQ-106). The reference implementation writes `<id>2`, `<id>3`
// there, and so did this SDK's earlier releases, so decode reads that spelling
// as the sibling it stands for. Encode writes the Web Template's id.

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// siblingSpellingFold is what [foldSiblingSpellings] made of a body's keys.
type siblingSpellingFold struct {
	// spellings maps a key of the folded body to every key as written that it
	// stands for, where that is not just itself, the spellings of
	// [emptySuffixFold] included, for the decode's [unmatchedSink].
	spellings map[string][]string
	// conflicts are the pairs of spellings of one key whose values differ,
	// each as written: the second is out of the folded body until
	// [siblingSpellingFold.settle] judges the pair.
	conflicts []spellingPair
}

// spellingPair is two spellings of one key as written: kept is in the folded
// body, held is not.
type spellingPair struct {
	kept, held string
}

// foldSiblingSpellings reads a key that spells a duplicate sibling the earlier
// way ([currentSpelling]) as the key in the Web Template's spelling, and
// returns the body with every such key renamed. spellings are the ones
// [foldEmptySuffix] recorded for flat's keys.
//
// Two keys that become one are two spellings of one field. Beside a key of the
// same value the renamed key is read once. Beside a key of another value it is
// held out, and the pair is judged once the decode knows whether it reads the
// key ([siblingSpellingFold.settle]): two spellings of a field the decode reads
// disagree, and two of one it does not read are two unmatched keys (REQ-170).
// Either way the sink reports every spelling as written. A spelling that holds
// null is absent (REQ-053), so it disagrees with nothing.
//
// The map returned is flat itself when nothing was renamed.
func foldSiblingSpellings(flat map[string]any, wt *webtemplate.WebTemplate, spellings map[string][]string) (map[string]any, siblingSpellingFold) {
	fold := siblingSpellingFold{spellings: spellings}
	// Renamed keys are taken in sorted order, so the same body keeps and holds
	// out the same spelling on every run.
	type rename struct{ key, current string }
	var renames []rename
	for key := range flat {
		if current, renamed := currentSpelling(wt.Tree, key); renamed {
			renames = append(renames, rename{key: key, current: current})
		}
	}
	if len(renames) == 0 {
		return flat, fold
	}
	slices.SortFunc(renames, func(a, b rename) int { return strings.Compare(a.key, b.key) })
	out := maps.Clone(flat)
	fold.spellings = maps.Clone(spellings)
	if fold.spellings == nil {
		fold.spellings = make(map[string][]string)
	}
	for _, r := range renames {
		key, current := r.key, r.current
		written := writtenSpellings(fold.spellings, key)
		delete(out, key)
		delete(fold.spellings, key)
		val := flat[key]
		prev, both := out[current]
		if !both {
			out[current] = val
			fold.spellings[current] = written
			continue
		}
		kept := writtenSpellings(fold.spellings, current)
		fold.spellings[current] = slices.Concat(kept, written)
		switch {
		case val == nil, sameCtxValue(prev, val):
			// A null renamed key is absent beside the other spelling.
		case prev == nil:
			// A null kept key is absent: the renamed one carries the value.
			out[current] = val
		default:
			fold.conflicts = append(fold.conflicts, spellingPair{kept: kept[0], held: written[0]})
		}
	}
	return out, fold
}

// writtenSpellings returns the keys as written that key of a folded body
// stands for: its recorded spellings, or key itself.
func writtenSpellings(spellings map[string][]string, key string) []string {
	if written, ok := spellings[key]; ok {
		return written
	}
	return []string{key}
}

// settle refuses a held-out spelling of a key the decode read: sink reports
// every spelling of a key it did not read, so a held spelling it did not
// report disagrees with a key the decode read. The refusal names both keys and
// neither value.
func (f siblingSpellingFold) settle(sink *unmatchedSink) error {
	for _, c := range f.conflicts {
		if !sink.reported(c.held) {
			return fmt.Errorf("%w: %q and %q spell one field twice and disagree; remove one",
				ErrUnknownPath, c.kept, c.held)
		}
	}
	return nil
}

// currentSpelling returns key with each path segment that spells a duplicate
// sibling the earlier way ([earlierSpelledSibling]) renamed to that sibling's
// id, and whether it renamed any. It walks the Web Template from root while
// each segment names a child of the node before it, by its own id or by the
// earlier spelling; the segments after that are no Web Template path (an
// underscore family, a leaf's sub-path), so none of them is renamed. A key
// that is not rooted at root, or holds a segment whose :index [parseFlatSeg]
// refuses, is left as written for a later stage to judge.
func currentSpelling(root *webtemplate.Node, key string) (string, bool) {
	path, _ := splitSuffix(key)
	first, rest, _ := strings.Cut(path, "/")
	if seg, ok := parseFlatSeg(first); !ok || seg.id != root.ID {
		return "", false
	}
	var b strings.Builder
	node := root
	// start is the offset in key of the segment p, copied the end of what b
	// holds of key.
	start, copied := len(first)+1, 0
	for p := range strings.SplitSeq(rest, "/") {
		seg, ok := parseFlatSeg(p)
		if !ok {
			return "", false
		}
		if node != nil {
			next := childByID(node, seg.id)
			if next == nil {
				if next = earlierSpelledSibling(node, seg.id); next != nil {
					b.WriteString(key[copied:start])
					b.WriteString(next.ID)
					copied = start + len(seg.id)
				}
			}
			node = next
		}
		start += len(p) + 1
	}
	if copied == 0 {
		return "", false
	}
	b.WriteString(key[copied:])
	return b.String(), true
}

// earlierSpelledSibling returns the child of node that id spells the way the
// reference writes a duplicate sibling, or nil. The caller has found no child
// whose own id is id. id spells the child `<base>_<N-1>` when:
//
//   - id is `<base><N>`, N a decimal number of 2 or more with no leading zero;
//   - node has a child `<base>` and a child `<base>_<N-1>`.
//
// A base may end in digits, so id may split more than one way (`x12` is `x`
// and 12, or `x1` and 2): the shortest base whose children are there wins.
func earlierSpelledSibling(node *webtemplate.Node, id string) *webtemplate.Node {
	digitsFrom := len(strings.TrimRightFunc(id, func(r rune) bool { return '0' <= r && r <= '9' }))
	for i := max(digitsFrom, 1); i < len(id); i++ {
		base, number := id[:i], id[i:]
		n, err := strconv.Atoi(number)
		if number[0] == '0' || err != nil || n < 2 || childByID(node, base) == nil {
			continue
		}
		if ch := childByID(node, base+"_"+strconv.Itoa(n-1)); ch != nil {
			return ch
		}
	}
	return nil
}
