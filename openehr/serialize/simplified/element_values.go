package simplified

// REQ-053 — an ELEMENT whose value admits several value types, or none. The Web
// Template gives such an ELEMENT a node of its own, with one child per value
// type (`quantity_value`, `count_value`, `interval_of_date_time_value`), each at
// the ELEMENT's `/value` path. The value is spelled under the child of its
// type, and the ELEMENT's own underscore attributes under the ELEMENT. A key at
// the ELEMENT itself, the leaf spelling earlier releases gave it, reads as its
// first child. Keys that reach two of its value types spell two values of its
// one value, and are refused rather than merged.

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// hasValueTypes reports whether n is an ELEMENT node with a child per value
// type its template admits. The Web Template gives an ELEMENT children only
// then: one whose value admits a single type, or a coded text and a free text,
// collapses into a value leaf instead.
func hasValueTypes(n *webtemplate.Node) bool {
	return len(n.Children) > 0 && nodeRMType(n) == "ELEMENT"
}

// valueLeaf returns the leaf a key resolving to n reads its value with: n's
// first value type where n has value types ([hasValueTypes]), and n itself
// otherwise.
func valueLeaf(n *webtemplate.Node) *webtemplate.Node {
	if hasValueTypes(n) {
		return n.Children[0]
	}
	return n
}

// valueReach is the value type of one ELEMENT instance that a key reaches.
type valueReach struct {
	// element is the FLAT path of the ELEMENT instance, with an index on every
	// segment ([indexedSegs]), so its spellings with and without `:0` are one.
	element string
	// valueType is the id of the child the key is spelled under, or "" for a
	// key at the ELEMENT itself, which reads as its first child.
	valueType string
}

// valueTypeReached returns the value type that a key whose segments segs
// resolve to node reaches, where node is an ELEMENT with value types
// ([hasValueTypes]) or one of its children, and false otherwise.
func valueTypeReached(segs []flatSeg, node *webtemplate.Node, sib *reusedSiblings) (valueReach, bool) {
	switch {
	case len(segs) == 0:
	case hasValueTypes(node):
		return valueReach{element: indexedSegs(segs)}, true
	case sib.elementOf(node) != nil && len(segs) > 1:
		return valueReach{element: indexedSegs(segs[:len(segs)-1]), valueType: node.ID}, true
	}
	return valueReach{}, false
}

// valueClaims records, for each ELEMENT instance with value types, the value
// type the body's keys reach and the first key that reached it.
type valueClaims map[string]valueClaim

// valueClaim is the value type the keys of one ELEMENT instance reach.
type valueClaim struct {
	valueType, key string
}

// claim records that key reaches r. It refuses key with ErrUnknownPath where an
// earlier key reached another value type of the same ELEMENT instance, or the
// ELEMENT itself where key reaches a child, or the reverse: ELEMENT.value is
// single-valued, so the two would spell two values of it (REQ-053). key names a
// key, or the path its family or leaf shares, and never a value.
func (c valueClaims) claim(r valueReach, key string) error {
	held, seen := c[r.element]
	if !seen {
		c[r.element] = valueClaim{valueType: r.valueType, key: key}
		return nil
	}
	if held.valueType == r.valueType {
		return nil
	}
	return fmt.Errorf("simplified: %q and %q: %w: they spell two values of the ELEMENT at %q, which holds one, under %s and %s — refused rather than merged",
		held.key, key, ErrUnknownPath, r.element, valueTypeLabel(held.valueType), valueTypeLabel(r.valueType))
}

// valueTypeLabel names a value type of [valueClaim] for a message.
func valueTypeLabel(valueType string) string {
	if valueType == "" {
		return "the ELEMENT itself"
	}
	return "its value type " + valueType
}

// indexedSegs spells segs as a FLAT path with an index on every segment, :0
// where the segment carries none, which addresses the same node
// ([resolveLeaf]).
func indexedSegs(segs []flatSeg) string {
	var b strings.Builder
	for i, seg := range segs {
		if i > 0 {
			b.WriteByte('/')
		}
		writeFlatSeg(&b, flatSeg{id: seg.id, idx: max(seg.idx, 0)})
	}
	return b.String()
}

// emitElementValue writes an ELEMENT el, the instance v resolved to at node, an
// ELEMENT node with value types ([hasValueTypes]), under flatPath: its value
// under the child [valueChildFor] picks, with the value's own underscore
// decorations below that child, and the ELEMENT's own underscore attributes
// (`_uid`, `_null_flavour`, …) at flatPath. An ELEMENT with no value writes
// its attributes alone.
func emitElementValue(out map[string]any, node *webtemplate.Node, flatPath string, v any, el rm.Element) error {
	if el.Value != nil && !rm.IsTypedNil(el.Value) {
		child, raw := valueChildFor(node, el.Value)
		path := flatPath + "/" + child.ID
		if raw {
			name, _ := rm.RMTypeName(el.Value)
			if err := emitRaw(out, path, el.Value, cmp.Or(dvTypeName(el.Value), name)); err != nil {
				return err
			}
		} else if err := leafToFlat(out, path, el.Value, nodeRMType(child), leafListOpen(child)); err != nil {
			return err
		}
	}
	return rmattrEncode(v, flatPath, out)
}

// valueChildFor returns the child of node, an ELEMENT node with value types,
// that spells value v: the first child whose type is v's ([valueTypeName]);
// else, for a DV_CODED_TEXT, the first DV_TEXT child, where it takes the coded
// text's suffixes, the substitution a DV_TEXT leaf admits (REQ-053); else the
// first child, with raw true, under which v rides |raw. An interval whose
// bounds name no type takes the first interval child.
func valueChildFor(node *webtemplate.Node, v any) (child *webtemplate.Node, raw bool) {
	typ := valueTypeName(v)
	for _, ch := range node.Children {
		if nodeRMType(ch) == typ {
			return ch, false
		}
	}
	for _, ch := range node.Children {
		switch {
		case typ == "DV_CODED_TEXT" && nodeRMType(ch) == "DV_TEXT":
			return ch, false
		case typ == "DV_INTERVAL" && isIntervalLeafType(nodeRMType(ch)):
			return ch, false
		}
	}
	return node.Children[0], true
}

// valueTypeName is the RM type of the DataValue v as a Web Template spells a
// node's type: an interval names its bound type in angle brackets, read from
// its instantiation or, for the abstract one a canonical decode gives, from
// its bounds; one whose bounds name none is DV_INTERVAL alone.
func valueTypeName(v any) string {
	if name := dvTypeName(v); name != "" {
		return name
	}
	name, _ := rm.RMTypeName(v)
	if name == "DV_INTERVAL" {
		if bound := intervalBoundType(v); bound != "" {
			return "DV_INTERVAL<" + bound + ">"
		}
	}
	return name
}

// intervalBoundType returns the RM type of the bounds of v, a DV_INTERVAL, or
// "" where neither its instantiation nor its bounds name one. The generated
// DVInterval[T] instantiations are enumerated, as [intervalLeafToFlat]
// enumerates them, since the SDK uses no reflection.
func intervalBoundType(v any) string {
	if iv, ok := as[rm.DVInterval[rm.DVOrdered]](v); ok {
		for _, b := range []rm.DVOrdered{iv.Lower, iv.Upper} {
			if b == nil || rm.IsTypedNil(b) {
				continue
			}
			if name, ok := rm.RMTypeName(b); ok {
				return name
			}
		}
		return ""
	}
	switch {
	case isTyped[rm.DVInterval[rm.DVQuantity]](v):
		return "DV_QUANTITY"
	case isTyped[rm.DVInterval[rm.DVCount]](v):
		return "DV_COUNT"
	case isTyped[rm.DVInterval[rm.DVDateTime]](v):
		return "DV_DATE_TIME"
	case isTyped[rm.DVInterval[rm.DVDate]](v):
		return "DV_DATE"
	case isTyped[rm.DVInterval[rm.DVTime]](v):
		return "DV_TIME"
	case isTyped[rm.DVInterval[rm.DVDuration]](v):
		return "DV_DURATION"
	case isTyped[rm.DVInterval[rm.DVOrdinal]](v):
		return "DV_ORDINAL"
	case isTyped[rm.DVInterval[rm.DVProportion]](v):
		return "DV_PROPORTION"
	case isTyped[rm.DVInterval[rm.DVScale]](v):
		return "DV_SCALE"
	}
	return ""
}
