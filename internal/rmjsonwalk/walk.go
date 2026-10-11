package rmjsonwalk

import (
	"bytes"
	"encoding/json/jsontext"
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
	// decodes as when it carries no "_type". It is empty where only the
	// value's own "_type" can name its class.
	Type string
	// Bound is the class the target binds the type parameter of each value's
	// class to, such as DV_QUANTITY for an rm.DVInterval[rm.DVQuantity], or
	// empty.
	Bound string
}

// Unmatched walks the first JSON value in data and calls report with the JSON
// Pointer (RFC 6901) of every unmatched member, once each, in the order the
// members appear. A member is unmatched when its name is neither "_type" nor
// an attribute of the RM type its object decodes as. A pointer names the
// member and carries none of its value.
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
// such a value is reported. A value of the wrong JSON kind for its attribute
// is skipped, not reported: the decoder refuses it.
//
// report must not be nil. When it returns an error the walk stops and
// returns that error. Any other error is a malformed document, reported by
// encoding/json/jsontext under its default options; data the canonical-JSON
// decoder accepted walks without one.
func (m *Model) Unmatched(data []byte, root Root, report func(pointer string) error) error {
	types, err := typeIndex(data)
	if err != nil {
		return err
	}
	w := &walk{m: m, types: types, report: report}
	return w.root(jsontext.NewDecoder(bytes.NewReader(data)), root.Nest, root)
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
// members whose meaning depends on it; the index lets the walk know each
// object's type when it opens the object.
func typeIndex(data []byte) (map[int64]string, error) {
	types := make(map[int64]string)
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	return types, indexValue(dec, types)
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

// walk is one pass of [Model.Unmatched] over a document.
type walk struct {
	m      *Model
	types  map[int64]string
	report func(pointer string) error
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
	typ, hasType := w.types[dec.InputOffset()]
	rmType, err := w.m.Resolve(typ, hasType && typ != "", declared)
	for dec.PeekKind() != '}' {
		tok, rerr := dec.ReadToken()
		if rerr != nil {
			return rerr
		}
		if err != nil {
			// The object's type cannot be read, or is no RM class: nothing
			// in it is checked.
			if serr := dec.SkipValue(); serr != nil {
				return serr
			}
			continue
		}
		if merr := w.member(dec, rmType, tok.String(), bound); merr != nil {
			return merr
		}
	}
	_, rerr := dec.ReadToken()
	return rerr
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
		if err := w.report(string(dec.StackPointer())); err != nil {
			return err
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
