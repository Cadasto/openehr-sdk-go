package validation

// missing.go: REQ-112 § Value-typed mandatory presence: the per-call option
// that hands the walk a decode's report of the members a document leaves
// out (wire.md § REQ-175), and the trie the walk reads it from. A Validator
// holds no per-call state (REQ-171 § Surface): the report travels as a
// trailing option of the call, and the engine threads the trie down the
// walk, node by node, so that an attribute the report names is read as
// absent where the walk visits it ([engine.visitAttribute]).

import "strings"

// callConfig is what the options of one call set.
type callConfig struct {
	// missing is the root of the trie built from every [WithMissing] of the
	// call, nil while none named a member.
	missing *missingNode
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
// A nil or empty list adds nothing, and the option given more than once
// reads as the union of its lists. [ValidateJSON] and [ValidateXML] decode a
// document and hand its report on in one call; this option is for a caller
// that decoded with a codec's report option and validates afterwards.
func WithMissing(pointers []string) ValidateOption {
	return func(c *callConfig) {
		for _, p := range pointers {
			c.addMissing(p)
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
	if c.missing == nil {
		c.missing = &missingNode{}
	}
	n := c.missing
	for s := range strings.SplitSeq(rest, "/") {
		s = unescapePointer(s)
		next, ok := n.below[s]
		if !ok {
			if n.below == nil {
				n.below = map[string]*missingNode{}
			}
			next = &missingNode{}
			n.below[s] = next
		}
		n = next
	}
	n.absent = true
}

// missingNode is one node of the trie a call's report is read from: a
// member of the instance, reached from the node above by one pointer
// segment. The root stands for the instance itself.
type missingNode struct {
	// absent is set when a pointer of the report names this member.
	absent bool
	// below holds the nodes under this one, by segment: an attribute by its
	// name, an item of a list by its position written in decimal and counted
	// from 0, and an entry of a Hash by its key. Nil while nothing is named
	// below.
	below map[string]*missingNode
}

// child returns the node under n for the segment key: nil when n is nil, as
// it is on every node of a call with no report, or when nothing is named
// under key.
func (n *missingNode) child(key string) *missingNode {
	if n == nil {
		return nil
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
