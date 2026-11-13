package simplified

// REQ-053 — a duplicate sibling's earlier spelling. The Web Template gives the
// second and later of siblings that share an id the next free `<id>_1`,
// `<id>_2`, … (REQ-106). The reference implementation writes `<id>2`, `<id>3`
// there, and so did this SDK's earlier releases, so decode reads that spelling
// as the sibling it stands for. Encode writes the Web Template's id.
//
// Every stage after the rename names a key in the Web Template's spelling, so
// a refusal is respelled once, on its way out of the decode: each id its
// message names in that spelling goes back to the one the body wrote
// ([siblingSpellingFold.respell]).

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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
	// written is the body before the rename, and root the Web Template root
	// it was renamed against, for [siblingSpellingFold.respell]. Both are nil
	// where nothing was renamed.
	written map[string]any
	root    *webtemplate.Node
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
	fold.written, fold.root = flat, wt.Tree
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

// respell returns err with each id its message names in the Web Template's
// spelling put back the way the body wrote it ([respelling]), so a refusal
// names the keys and paths the caller wrote. The error returned wraps err, so
// [errors.Is] and [errors.As] see what err carries. err is returned as it is
// where it is nil, nothing was renamed, or its message names no renamed id.
//
// A key as written keeps its ids: each lookup of [respelling] its path meets
// holds what that key wrote there, or "", up to an id in the earlier
// spelling, which no lookup is keyed by. So the refusal [settle] makes, which
// names two keys as written, passes through unchanged.
func (f siblingSpellingFold) respell(err error) error {
	if err == nil || f.written == nil {
		return err
	}
	msg := err.Error()
	out := respellingOf(f.written, f.root).apply(msg)
	if out == msg {
		return err
	}
	return &respelledError{msg: out, err: err}
}

// respelledError is a refusal whose message names keys as the body wrote
// them. It wraps the refusal the decode made.
type respelledError struct {
	msg string
	err error
}

// Error returns the respelled message.
func (e *respelledError) Error() string { return e.msg }

// Unwrap returns the refusal the decode made.
func (e *respelledError) Unwrap() error { return e.err }

// respelling maps a path in the Web Template's spelling to the ids the body
// wrote it with. A path a message names is read in this order:
//
//   - the path of a key the body wrote takes that key's ids ([respelling.byKey]);
//   - any other path takes, segment by segment, the id the body writes at
//     that instance ([respelling.byInstance]), or, at an instance no key is
//     below, at that node ([respelling.byPath]);
//   - a lone id takes the one the body writes for it ([respelling.byID]).
//
// A path is keyed with its ids in the Web Template's spelling. An indexed key
// gives every segment its :index, :0 where it spells none, so a key the body
// wrote with no index and the path a stage names with :0 on every segment
// read alike. Each map of ids holds "" for one spelled more than one way, and
// that id stays as the message names it.
type respelling struct {
	// byKey is keyed by the indexed path of each key the body wrote, and by
	// that path and the key's |suffix, and holds the ids the key was written
	// with, one per segment. It holds nil for a path two keys written with
	// other ids have: one key two written keys stand for is the conflict
	// [siblingSpellingFold.settle] refuses, naming both.
	byKey map[string][]string
	// byInstance is keyed by the indexed path of every prefix of a key's path,
	// so each names one instance of a node.
	byInstance map[string]string
	// byPath is keyed by the ids of every prefix of a key's path, joined by
	// "/" and without their :index, so it names a node at every instance.
	byPath map[string]string
	// byID is keyed by a lone id, for a message that names one outside a path
	// (`FLAT id "x_1"`): the body must spell it one way wherever it appears.
	byID map[string]string
}

// respellingOf reads, for each key of body, the id it writes at every segment
// beside the one [currentSpelling] reads it as. The keys are read in sorted
// order, so a rule that let one of two spellings win would pick the same one
// on every run.
func respellingOf(body map[string]any, root *webtemplate.Node) respelling {
	r := respelling{
		byKey:      make(map[string][]string),
		byInstance: make(map[string]string),
		byPath:     make(map[string]string),
		byID:       make(map[string]string),
	}
	for _, key := range slices.Sorted(maps.Keys(body)) {
		current, renamed := currentSpelling(root, key)
		if !renamed {
			current = key
		}
		writtenPath, _ := splitSuffix(key)
		currentPath, suffix := splitSuffix(current)
		// The rename replaces ids alone, so the two paths have the same
		// segments, one for one.
		writtenSegs := strings.Split(writtenPath, "/")
		written := make([]string, 0, len(writtenSegs))
		var ids, indexed strings.Builder
		for i, p := range strings.Split(currentPath, "/") {
			seg, ok := parseFlatSeg(p)
			wseg, wok := parseFlatSeg(writtenSegs[i])
			if !ok || !wok {
				written = nil
				break
			}
			if i > 0 {
				ids.WriteByte('/')
				indexed.WriteByte('/')
			}
			ids.WriteString(seg.id)
			indexed.WriteString(seg.id)
			indexed.WriteByte(':')
			indexed.WriteString(strconv.Itoa(max(seg.idx, 0)))
			record(r.byInstance, indexed.String(), wseg.id)
			record(r.byPath, ids.String(), wseg.id)
			record(r.byID, seg.id, wseg.id)
			written = append(written, wseg.id)
		}
		if written == nil {
			continue
		}
		recordKey(r.byKey, indexed.String(), written)
		if suffix != "" {
			recordKey(r.byKey, indexed.String()+"|"+suffix, written)
		}
	}
	return r
}

// record notes that the body writes written for k, keeping "" where it has
// written something else for k before.
func record(m map[string]string, k, written string) {
	if prev, seen := m[k]; seen && prev != written {
		m[k] = ""
		return
	}
	m[k] = written
}

// recordKey notes that the body writes a key at path k with the ids written,
// keeping nil where it has written one there with other ids before.
func recordKey(m map[string][]string, k string, written []string) {
	if prev, seen := m[k]; seen && !slices.Equal(prev, written) {
		m[k] = nil
		return
	}
	m[k] = written
}

// apply returns msg with every renamed id it names put back. It reads msg as
// text that names paths: a path starts where a run of id characters
// ([idRune]) follows anything but an id character, "/", ":" or "|", and
// takes "/"-separated segments, each an id with an optional ":<digits>"
// index, until something else follows, then a "|<suffix>" where one follows.
// A path of two or more segments is read from the template root, a lone id
// on its own ([respelling.written]). Only whole ids are replaced, and the
// index text msg spells is kept: `x_1` inside `x_10` is the id `x_10`, which
// is not renamed.
func (r respelling) apply(msg string) string {
	var b strings.Builder
	copied := 0
	for pos := 0; pos < len(msg); {
		if !pathStart(msg, pos) {
			_, n := utf8.DecodeRuneInString(msg[pos:])
			pos += n
			continue
		}
		var segs []pathSeg
		p := pos
		for {
			end := idEnd(msg, p)
			next := indexEnd(msg, end)
			segs = append(segs, pathSeg{start: p, end: end, index: msg[end:next]})
			p = next
			if p+1 >= len(msg) || msg[p] != '/' || idEnd(msg, p+1) == p+1 {
				break
			}
			p++
		}
		suffix := ""
		if p < len(msg) && msg[p] == '|' {
			suffix = msg[p+1 : idEnd(msg, p+1)]
		}
		for i, w := range r.written(msg, segs, suffix) {
			if s := segs[i]; w != "" && w != msg[s.start:s.end] {
				b.WriteString(msg[copied:s.start])
				b.WriteString(w)
				copied = s.end
			}
		}
		pos = p
	}
	if copied == 0 {
		return msg
	}
	b.WriteString(msg[copied:])
	return b.String()
}

// pathSeg is one segment of a path a message names: the offsets of its id in
// the message, and its ":<digits>" index as the message spells it, "" for
// none.
type pathSeg struct {
	start, end int
	index      string
}

// written returns, for each segment of a path msg names, the id the body
// writes there, read in the order [respelling] gives, or "" where the
// message's own id stays. segs are the path's segments, and suffix the
// "|<suffix>" that follows it, "" for none.
func (r respelling) written(msg string, segs []pathSeg, suffix string) []string {
	if len(segs) == 1 {
		return []string{r.byID[msg[segs[0].start:segs[0].end]]}
	}
	paths := make([]string, len(segs))
	instances := make([]string, len(segs))
	var ids, indexed strings.Builder
	for i, s := range segs {
		if i > 0 {
			ids.WriteByte('/')
			indexed.WriteByte('/')
		}
		id := msg[s.start:s.end]
		ids.WriteString(id)
		indexed.WriteString(id)
		indexed.WriteString(cmp.Or(s.index, ":0"))
		paths[i], instances[i] = ids.String(), indexed.String()
	}
	whole := instances[len(segs)-1]
	key, ok := r.byKey[whole+"|"+suffix]
	if suffix == "" || !ok {
		key = r.byKey[whole]
	}
	if key != nil {
		return key
	}
	out := make([]string, len(segs))
	for i := range segs {
		w, ok := r.byInstance[instances[i]]
		if !ok {
			w = r.byPath[paths[i]]
		}
		out[i] = w
	}
	return out
}

// idRune reports whether r can be part of a Web Template id: a letter, a
// digit, "_", "." or "-", the runes the Web Template keeps of a node's name.
func idRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '-'
}

// pathStart reports whether a path starts at pos in msg: an id rune there,
// and before it none, nor a "/", ":" or "|" that would make it the inner
// part of a path or a suffix.
func pathStart(msg string, pos int) bool {
	r, _ := utf8.DecodeRuneInString(msg[pos:])
	if !idRune(r) {
		return false
	}
	if pos == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(msg[:pos])
	return !idRune(prev) && prev != '/' && prev != ':' && prev != '|'
}

// idEnd returns the end of the run of id runes that starts at pos in msg.
func idEnd(msg string, pos int) int {
	for pos < len(msg) {
		r, n := utf8.DecodeRuneInString(msg[pos:])
		if !idRune(r) {
			break
		}
		pos += n
	}
	return pos
}

// indexEnd returns the end of the ":<digits>" index at pos in msg, or pos
// where there is none.
func indexEnd(msg string, pos int) int {
	if pos >= len(msg) || msg[pos] != ':' {
		return pos
	}
	end := pos + 1
	for end < len(msg) && '0' <= msg[end] && msg[end] <= '9' {
		end++
	}
	if end == pos+1 {
		return pos
	}
	return end
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
