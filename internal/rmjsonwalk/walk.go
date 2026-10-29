package rmjsonwalk

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"slices"
	"unicode/utf8"
)

// Root is the RM shape a decode target gives a document's root: the JSON
// containers the target wraps around its RM values, and the RM type each of
// those values is declared as. [Model.RootOf] reads it from a decode target.
type Root struct {
	// Nest lists the containers, outermost first: '[' for a Go slice or
	// array, whose JSON array elements each hold one value, and '{' for a Go
	// map, whose JSON object members each hold one. It is empty for a target
	// that is itself an RM value.
	Nest string
	// Type is the RM type each value is declared as: the class a value
	// decodes as when it carries no "_type". For a polymorphic RM interface
	// it is the class the interface stands for, an abstract class where only
	// the value's own "_type" can name its concrete class. It is empty where
	// nothing declares a type.
	Type string
	// Bound is the class the target binds the type parameter of each value's
	// class to, such as DV_QUANTITY for an rm.DVInterval[rm.DVQuantity] or
	// the abstract DATA_VALUE for an rm.OriginalVersion[rm.DataValue], or
	// empty.
	Bound string
}

// Hooks are the functions a [Model.Walk] calls for what it finds. Each takes
// the JSON Pointer (RFC 6901) of a member, which names the member and
// carries none of its value: its segments are member names, positions in a
// JSON array counted from 0, and the keys of a Hash. A nil hook asks for
// nothing, and the walk does no work for it. When a hook returns an error
// the walk stops and returns that error.
type Hooks struct {
	// Unmatched is called for every unmatched member, once each, in the
	// order the members appear. A member is unmatched when its name is
	// neither "_type" nor an attribute of the RM type its object decodes
	// as.
	Unmatched func(pointer string) error
	// Missing is called for every missing member, once each: an attribute
	// the RM makes mandatory on the RM type an object decodes as, which the
	// object has no member for or holds null at, as the attribute's name
	// under the object's pointer. A member that carries a value other than
	// null is never missing, whatever the value, and neither is an
	// attribute no serialization
	// carries, an interval boundary flag or "_type" (see [Model.required]).
	// The members of one object are reported as the object closes, in the
	// order the RM declares the attributes, an ancestor's before the
	// class's own; the objects inside an object close first.
	Missing func(pointer string) error
}

// Walk walks the first JSON value in data and calls the hooks in h for what
// it finds, in one pass over the document.
//
// root says how the document's root holds its RM values. A value where root
// expects a container of another JSON kind is skipped. A Hash attribute whose
// values are RM objects is read member by member, each pointer naming the
// member's key.
//
// The walk looks inside an object only when it can read the object's RM type
// (see [Model.Resolve]); an empty "_type" reads as an elided one, as the
// decoder reads it. It does not look inside an object of a class outside the
// model, such as an AOM 1.4 class, nor inside an object whose type it cannot
// read, a primitive value or a generic payload ([Attribute]), and nothing in
// such a value is reported, missing members included. A value of the wrong
// JSON kind for its attribute is skipped, not reported: the decoder refuses
// it, and the member is present.
//
// The walk reads an object's type as it opens the object, from a "_type"
// written as the object's first member. JSON member order is free, so an
// object may carry its "_type" later, after members whose meaning depends on
// it, or spell the member's name or its value with an escape. When the walk
// meets such an object among those it looks inside, it reads the document
// again: once for the "_type" of every object, wherever it sits, and once
// more to walk it. The report is the same either way. No hook is called
// before the walk has read the whole value once, so a malformed document
// calls none, and the hooks never see a pass the walk gave up.
//
// The walk holds the calls back while it reads the document the first time,
// but only up to a budget: the larger of 64 KiB and the document's length,
// each call counted as its pointer's length plus 24 bytes. Past it the walk
// reads the document again as above, calling each hook as it goes, so a hook
// that returns an error at the first of a great many unmatched or missing
// members still stops the walk there.
//
// An error other than a hook's own is a malformed document, reported by
// encoding/json/jsontext under its default options; data the canonical-JSON
// decoder accepted walks without one.
func (m *Model) Walk(data []byte, root Root, h Hooks) error {
	if calls, ok := m.walkOnce(data, root, h); ok {
		return replay(calls, h)
	}
	return m.walkIndexed(data, root, h)
}

// walkOnce is the one pass of [Model.Walk]. It reads each object's type from
// a leading "_type" as it opens the object ([leadingType]) and holds the hook
// calls back, returning them in order. ok is false when the pass cannot stand
// for [Model.walkIndexed]: an object it looks inside carries a "_type" string
// it did not read as the object opened, so it may have walked the object as
// the wrong type, the calls it holds pass their budget, or data is malformed.
// The two passes then decide, the error included.
//
// The budget is the larger of [holdFloor] and the length of data, each call
// counted as its pointer's length plus [callBytes]. A hook may stop the walk
// at its first call, as a strict decode does at the first unmatched member,
// and the two passes stop there too; the one pass, holding the calls back,
// would read on and hold a pointer for every member of a hostile document
// that reports them all. Past the budget it gives up instead: the walk then
// holds at most the budget beyond what the two passes hold, and reads the
// document at most once more than they do.
func (m *Model) walkOnce(data []byte, root Root, h Hooks) (calls []call, ok bool) {
	w := &walk{m: m, hooks: h, once: true, budget: max(holdFloor, len(data))}
	if err := w.root(newDecoder(data), root.Nest, root); err != nil {
		return nil, false
	}
	return w.calls, true
}

const (
	// holdFloor is the least budget, in bytes, of the calls the one pass
	// holds ([Model.walkOnce]). Real documents stay far below it: the
	// largest report of a corpus composition holds under 1 KiB, and under
	// 46 KiB with a member no RM class has added to every object.
	holdFloor = 64 << 10
	// callBytes is what a held call counts for beside its pointer's bytes:
	// a call's own size on a 64-bit platform.
	callBytes = 24
)

// walkIndexed is [Model.Walk] in two passes: [typeIndex] reads the "_type" of
// every object first, wherever it sits among the object's members, and the
// walk then calls the hooks as it goes.
func (m *Model) walkIndexed(data []byte, root Root, h Hooks) error {
	types, err := typeIndex(data)
	if err != nil {
		return err
	}
	w := &walk{m: m, types: types, hooks: h}
	return w.root(newDecoder(data), root.Nest, root)
}

// call is a hook call the one pass holds back: the hook, Missing or else
// Unmatched, and the pointer it is called with.
type call struct {
	missing bool
	pointer string
}

// replay makes calls in order, and stops at the first hook that returns an
// error, which it returns.
func replay(calls []call, h Hooks) error {
	for _, c := range calls {
		hook := h.Unmatched
		if c.missing {
			hook = h.Missing
		}
		if err := hook(c.pointer); err != nil {
			return err
		}
	}
	return nil
}

// errLateType gives the one pass up at an object whose "_type" it did not
// read as the object opened. [Model.Walk] never returns it.
var errLateType = errors.New("rmjsonwalk: _type is not the leading member of its object")

// errOverBudget gives the one pass up when the calls it holds pass their
// budget ([Model.walkOnce]). [Model.Walk] never returns it.
var errOverBudget = errors.New("rmjsonwalk: the hook calls held exceed their budget")

// leadingType reads the "_type" of an object from unread, the bytes that
// follow the object's opening brace, without reading a token. ok is true
// when the object's first member is "_type", its name written without an
// escape, and the member's value is a string of ASCII text with no escape,
// so its bytes are the string the decoder reads; JSON whitespace may surround
// the name, the colon and the value. Any other first member, or none, gives
// false. Nothing past the value's closing quote is checked: the walk reads
// the object's members next, and the decoder refuses a malformed one.
func leadingType(unread []byte) (typ string, ok bool) {
	const name = `"_type"`
	i := skipSpace(unread, 0)
	if !bytes.HasPrefix(unread[i:], []byte(name)) {
		return "", false
	}
	i = skipSpace(unread, i+len(name))
	if i >= len(unread) || unread[i] != ':' {
		return "", false
	}
	i = skipSpace(unread, i+1)
	if i >= len(unread) || unread[i] != '"' {
		return "", false
	}
	start := i + 1
	for j := start; j < len(unread); j++ {
		switch c := unread[j]; {
		case c == '"':
			return string(unread[start:j]), true
		case c == '\\' || c < ' ' || c >= utf8.RuneSelf:
			return "", false
		}
	}
	return "", false
}

// skipSpace returns the index of the first byte of b at or after i that is
// not JSON whitespace (space, tab, line feed or carriage return), or len(b).
func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// newDecoder returns a decoder that reads data in place. Given a
// *bytes.Buffer, jsontext parses the buffer's own bytes, where from any other
// reader it copies the whole document into a buffer of its own. The one
// condition it sets is that nothing writes to the buffer while the decoder
// reads it: nothing writes to data during a walk, and jsontext leaves a
// buffer it reads in place as it is.
func newDecoder(data []byte) *jsontext.Decoder {
	return jsontext.NewDecoder(bytes.NewBuffer(data))
}

// Unmatched is [Model.Walk] with report as the Unmatched hook alone: it calls
// report with the pointer of every unmatched member, once each, in the order
// the members appear. report must not be nil.
func (m *Model) Unmatched(data []byte, root Root, report func(pointer string) error) error {
	return m.Walk(data, root, Hooks{Unmatched: report})
}

// root reads the next value from dec as the containers nest still lists
// around root's RM values.
func (w *walk) root(dec *jsontext.Decoder, nest string, root Root) error {
	if nest == "" {
		return w.value(dec, root.Type, root.Bound)
	}
	return each(dec, jsontext.Kind(nest[0]), func() error {
		return w.root(dec, nest[1:], root)
	})
}

// each reads the next value from dec as a JSON array (open '[') or object
// (open '{') and calls fn to read each element or member value in turn. A
// value of any other kind is skipped.
func each(dec *jsontext.Decoder, open jsontext.Kind, fn func() error) error {
	if dec.PeekKind() != open {
		return dec.SkipValue()
	}
	if _, err := dec.ReadToken(); err != nil {
		return err
	}
	end := jsontext.Kind(']')
	if open == '{' {
		end = '}'
	}
	for dec.PeekKind() != end {
		if open == '{' {
			if _, err := dec.ReadToken(); err != nil { // the member's name
				return err
			}
		}
		if err := fn(); err != nil {
			return err
		}
	}
	_, err := dec.ReadToken()
	return err
}

// typeIndex reads data once and records the "_type" of every object that
// carries a string there, keyed by the input offset just past the object's
// opening brace. JSON member order is free, so an object's "_type" can follow
// members whose meaning depends on it; the index lets [Model.walkIndexed]
// know each object's type when it opens the object, where the one pass knows
// only a leading "_type".
func typeIndex(data []byte) (map[int64]string, error) {
	types := make(map[int64]string)
	return types, indexValue(newDecoder(data), types)
}

// indexValue reads the next value from dec into the index.
func indexValue(dec *jsontext.Decoder, types map[int64]string) error {
	kind := dec.PeekKind()
	if kind != '{' && kind != '[' {
		return dec.SkipValue()
	}
	if _, err := dec.ReadToken(); err != nil {
		return err
	}
	if kind == '[' {
		for dec.PeekKind() != ']' {
			if err := indexValue(dec, types); err != nil {
				return err
			}
		}
		_, err := dec.ReadToken()
		return err
	}
	at := dec.InputOffset()
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return err
		}
		if name.String() == "_type" && dec.PeekKind() == '"' {
			typ, err := dec.ReadToken()
			if err != nil {
				return err
			}
			types[at] = typ.String()
			continue
		}
		if err := indexValue(dec, types); err != nil {
			return err
		}
	}
	_, err := dec.ReadToken()
	return err
}

// walk is one pass of [Model.Walk] over a document.
type walk struct {
	m     *Model
	hooks Hooks
	// once marks the one pass ([Model.walkOnce]), which reads each object's
	// type from a leading "_type" and holds the hook calls back in calls, as
	// long as budget, the bytes it may still hold, does not run out.
	// Otherwise types holds the "_type" of every object, keyed as
	// [typeIndex] keys it, and the hooks are called as the walk goes.
	once   bool
	calls  []call
	budget int
	types  map[int64]string
}

// typeOf returns the "_type" of the object dec has just opened, and whether
// it carries one: in the one pass a leading one, read from dec's unread
// bytes, and otherwise the one the index holds.
func (w *walk) typeOf(dec *jsontext.Decoder) (string, bool) {
	if w.once {
		return leadingType(dec.UnreadBuffer())
	}
	typ, ok := w.types[dec.InputOffset()]
	return typ, ok
}

// report calls the Missing hook with pointer, or the Unmatched hook when
// missing is false. The one pass holds the call back instead, counting it
// against its budget, and gives the pass up with [errOverBudget] once the
// calls it holds would pass it.
func (w *walk) report(missing bool, pointer string) error {
	if w.once {
		w.budget -= len(pointer) + callBytes
		if w.budget < 0 {
			return errOverBudget
		}
		w.calls = append(w.calls, call{missing: missing, pointer: pointer})
		return nil
	}
	if missing {
		return w.hooks.Missing(pointer)
	}
	return w.hooks.Unmatched(pointer)
}

// value reads the next value from dec as one value of declared type declared,
// in a slot that binds the value's type parameter to bound. An object is
// walked; anything else is skipped.
func (w *walk) value(dec *jsontext.Decoder, declared, bound string) error {
	if dec.PeekKind() != '{' {
		return dec.SkipValue()
	}
	if _, err := dec.ReadToken(); err != nil {
		return err
	}
	typ, hasType := w.typeOf(dec)
	rmType, err := w.m.Resolve(typ, hasType && typ != "", declared)
	// With a Missing hook, required lists the presence-checked attributes
	// of rmType and seen records which of them the object holds a value for.
	// Without one both stay nil, and the loop below does nothing for them.
	var (
		required []string
		seen     []bool
	)
	if err == nil && w.hooks.Missing != nil {
		required = w.m.required(rmType)
		seen = make([]bool, len(required))
	}
	for dec.PeekKind() != '}' {
		tok, rerr := dec.ReadToken()
		if rerr != nil {
			return rerr
		}
		// The one pass reads an object that did not open with its "_type"
		// as its declared type; a "_type" string among its members may name
		// another, and the pass is given up. tok is read before dec peeks
		// at the value: a decoder that reads a copy of the document
		// invalidates the token it last read when it peeks.
		late := w.once && !hasType && tok.String() == "_type"
		if err != nil {
			if late && dec.PeekKind() == '"' {
				return errLateType
			}
			// The object's type cannot be read, or is no RM class: nothing
			// in it is checked.
			if serr := dec.SkipValue(); serr != nil {
				return serr
			}
			continue
		}
		name := tok.String()
		if late && dec.PeekKind() == '"' {
			return errLateType
		}
		if i := slices.Index(required, name); i >= 0 && dec.PeekKind() != 'n' {
			seen[i] = true
		}
		if merr := w.member(dec, rmType, name, bound); merr != nil {
			return merr
		}
	}
	if _, rerr := dec.ReadToken(); rerr != nil {
		return rerr
	}
	// The object has closed: dec's pointer names it, and each required
	// attribute it holds no value for is missing.
	for i, attr := range required {
		if seen[i] {
			continue
		}
		if herr := w.report(true, string(dec.StackPointer())+"/"+attr); herr != nil {
			return herr
		}
	}
	return nil
}

// member reads the value of the member name of an object of type rmType,
// whose slot binds its type parameter to bound, and reports the member when
// rmType has no attribute of that name.
func (w *walk) member(dec *jsontext.Decoder, rmType, name, bound string) error {
	if name == "_type" {
		return dec.SkipValue()
	}
	attr, ok := w.m.Attribute(rmType, name, bound)
	switch {
	case !ok:
		if w.hooks.Unmatched != nil {
			if err := w.report(false, string(dec.StackPointer())); err != nil {
				return err
			}
		}
		return dec.SkipValue()
	case attr.Payload, attr.Kind == KindPrimitive:
		return dec.SkipValue()
	case attr.Hash, attr.Container:
		open := jsontext.Kind('[')
		if attr.Hash {
			open = '{'
		}
		return each(dec, open, func() error { return w.value(dec, attr.Type, attr.Bound) })
	default:
		return w.value(dec, attr.Type, attr.Bound)
	}
}
