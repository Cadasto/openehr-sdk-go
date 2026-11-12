package validation

// missing.go: REQ-112 § Value-typed mandatory presence: the per-call
// options that hand the walk a decode's report of the members a document
// leaves out (wire.md § REQ-175), as pointer text or as the codecs' pointer
// values, and the trie the walk reads it from. A Validator holds no per-call
// state (REQ-171 § Surface): the report travels as a trailing option of the
// call, and the engine threads the trie down the walk, node by node, so that
// an attribute the report names is read as absent where the walk visits it
// ([engine.visitAttribute]).

import (
	"slices"
	"strings"
)

// callConfig is what the options of one call set.
type callConfig struct {
	// missing is the root of the trie built from every [WithMissing] and
	// [WithMissingPointers] of the call, nil while none named a member.
	missing *missingNode
}

// root returns the root of the trie of c, made first when no option of the
// call has named a member yet.
func (c *callConfig) root() *missingNode {
	if c.missing == nil {
		c.missing = &missingNode{}
	}
	return c.missing
}

// newCallConfig returns what opts set; a nil option is ignored.
func newCallConfig(opts []ValidateOption) callConfig {
	var c callConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// ValidateOption configures one call of [Validator.Validate], [Validate] or
// [ValidateRM]. The typed wrappers take none. A nil option is ignored.
type ValidateOption func(*callConfig)

// WithMissing hands the call the members a decode reported missing, each by
// its JSON Pointer: the pointer, in the canonical-JSON form of the instance,
// of a member the Reference Model makes mandatory that the document left out
// or sent as null, as the WithMissing options of the canonical JSON and XML
// codecs report them. The walk reads each such member as absent where the
// pointer names it, whatever the Go value holds there, and reports it as it
// reports any absent attribute: the template's required where the template
// requires it, and the Reference Model's required otherwise. That is the one
// finding at the member's path, and the walk reports nothing below it. A
// pointer into a Hash names an entry by its key, and one into a list an item
// by its position, counted from 0.
//
// A pointer that names no attribute the walk visits draws no finding: one
// into content the template refuses, past a limit, or into a value the walk
// does not enter. Neither does the empty pointer, which names the root, nor
// a text that is no JSON Pointer.
//
// A nil or empty list adds nothing, and the option given more than once, or
// beside [WithMissingPointers], reads as the union of their lists.
// [ValidateJSON] and [ValidateXML] decode a document and hand its report on
// in one call; this option is for a caller that decoded with a codec's
// report option and validates afterwards. A caller that kept the pointer
// values the codec passed, rather than their text, hands them to
// [WithMissingPointers] instead, which writes none of their text.
func WithMissing(pointers []string) ValidateOption {
	return func(c *callConfig) {
		for _, p := range pointers {
			c.addMissing(p)
		}
	}
}

// WithMissingPointers hands the call the members a decode reported missing,
// as [WithMissing] does, but by the pointer values the report passed rather
// than by their text: the canjson.Pointer values the canonical-JSON codec's
// WithMissing option passes, or the canxml.Pointer values the canonical-XML
// codec's passes. A caller keeps the pointers as the option passes them and
// hands them on here, and the call gives what WithMissing of their text
// gives.
//
// The call reads each pointer's parent and last reference token, through
// its Parent and Token methods, and writes none of its text. Parents that
// compare equal with == are read once, so the cost of a report grows with
// the distinct members on its paths, not with the depth of each member
// times their number. A pointer type of the caller's own serves too: Parent
// returns the pointer of the object, list or Hash that holds the member, and
// false for the root; Token returns the last reference token unescaped, a
// list item's position in decimal.
//
// A pointer that names no attribute the walk visits draws no finding, as
// for [WithMissing], and the root pointer, the zero value of either codec's
// Pointer, names no member. A nil or empty list adds nothing, and the option
// given more than once, or beside [WithMissing], reads as the union of
// their lists.
func WithMissingPointers[P interface {
	comparable
	Parent() (P, bool)
	Token() string
}](pointers []P) ValidateOption {
	return func(c *callConfig) {
		// met holds the node of each pointer already met as the parent of
		// another, so the climb from a pointer stops at the first parent
		// built; chain holds the pointers of one climb, the member first,
		// then turned to be built from the top down.
		var (
			met   map[P]*missingNode
			chain []P
		)
		for _, p := range pointers {
			chain = chain[:0]
			var n *missingNode
			for q := p; ; {
				if m, ok := met[q]; ok {
					n = m
					break
				}
				up, ok := q.Parent()
				if !ok {
					break
				}
				chain = append(chain, q)
				q = up
			}
			if n == nil {
				if len(chain) == 0 {
					continue // the root names no member
				}
				n = c.root()
			}
			slices.Reverse(chain)
			for i, q := range chain {
				n = n.add(q.Token())
				if i < len(chain)-1 {
					if met == nil {
						met = map[P]*missingNode{}
					}
					met[q] = n
				}
			}
			n.absent = true
		}
	}
}

// addMissing marks the member pointer names as absent in the trie of c,
// building the root and the nodes that lead to the member. The pointer is
// read as RFC 6901 writes it: "/" before each segment, and in each segment
// "~1" read as "/" and "~0" as "~", in that order. The empty pointer, which
// names the root, marks nothing, and so does a text that is no JSON Pointer:
// one that does not start with "/", or one with a "~" that opens neither
// escape, which builds no node either.
func (c *callConfig) addMissing(pointer string) {
	rest, ok := strings.CutPrefix(pointer, "/")
	if !ok || !wellEscaped(rest) {
		return
	}
	n := c.root()
	for s := range strings.SplitSeq(rest, "/") {
		n = n.add(unescapePointer(s))
	}
	n.absent = true
}

// missingNode is one node of the trie a call's report is read from: a
// member of the instance, reached from the node above by one pointer
// segment. The root stands for the instance itself.
//
// The nodes under a node are keyed by segment: an attribute by its name, an
// item of a list by its position written in decimal and counted from 0, and
// an entry of a Hash by its key. Most nodes of a report have one node under
// them, the attribute or item on the way to a member, so the first is held
// in the node itself and a map is made only for a second.
type missingNode struct {
	// absent is set when a pointer of the report names this member.
	absent bool
	// key is the segment of first; first is the first node named under
	// this one, nil while nothing is named below.
	key   string
	first *missingNode
	// below holds the nodes under this one after the first, by segment;
	// nil while fewer than two are named below.
	below map[string]*missingNode
}

// add returns the node under n for the segment key, made first when nothing
// is named under key yet.
func (n *missingNode) add(key string) *missingNode {
	if next := n.child(key); next != nil {
		return next
	}
	next := &missingNode{}
	if n.first == nil {
		n.key, n.first = key, next
		return next
	}
	if n.below == nil {
		n.below = map[string]*missingNode{}
	}
	n.below[key] = next
	return next
}

// child returns the node under n for the segment key: nil when n is nil, as
// it is on every node of a call with no report, or when nothing is named
// under key.
func (n *missingNode) child(key string) *missingNode {
	if n == nil {
		return nil
	}
	if n.first != nil && n.key == key {
		return n.first
	}
	return n.below[key]
}

// marked reports whether a pointer of the report names n; a nil n is named
// by none.
func (n *missingNode) marked() bool {
	return n != nil && n.absent
}

// wellEscaped reports whether every "~" in s opens one of the two escapes
// RFC 6901 defines, "~0" or "~1".
func wellEscaped(s string) bool {
	for {
		_, after, found := strings.Cut(s, "~")
		if !found {
			return true
		}
		if after == "" || (after[0] != '0' && after[0] != '1') {
			return false
		}
		s = after[1:]
	}
}

// unescapePointer returns the segment s names: "~1" is read as "/" first and
// "~0" as "~" after it, so that "~01" is "~1" and not "/". A segment with no
// "~" is returned as it is.
func unescapePointer(s string) string {
	if !strings.Contains(s, "~") {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
}
