// Package jsonpoly provides canonical-JSON marshalling helpers for
// polymorphic (interface-typed) RM fields.
//
// The generator-emitted RM MarshalJSON methods have pointer receivers,
// and the `_type` discriminator is emitted inside those methods. When a
// concrete *value* (not a pointer) is stored in an interface-typed
// field (e.g. a DV_CODED_TEXT value placed in a LOCATABLE.name
// DVTextLike slot), that value is not in the pointer method set, so
// encoding/json falls back to default struct encoding and drops the
// mandatory `_type` key — non-conformant ITS-JSON (REQ-052). See
// SDK-GAP-13.
//
// These helpers box such values into a pointer so the pointer-receiver
// MarshalJSON runs regardless of whether the interface holds a value or
// a pointer. The package depends only on reflect + encoding/json (no
// openehr/rm dependency) so the generated marshaller packages
// (openehr/rm, openehr/aom/aom14) can import it without an import cycle.
package jsonpoly

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// Marshal returns the canonical JSON encoding of an interface-typed RM
// value v, guaranteeing the leading `_type` key even when v holds a
// non-pointer concrete whose MarshalJSON has a pointer receiver.
//
// A nil interface returns a nil RawMessage: under an `omitempty` wire
// field the key is omitted; under a mandatory field encoding/json
// re-emits it as JSON null (RawMessage(nil) marshals to "null"). Both
// match encoding/json's treatment of a nil interface field, so the
// `_type` fix is the only observable change.
func Marshal(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer {
		// Box the value so a pointer-receiver MarshalJSON is in scope.
		p := reflect.New(rv.Type())
		p.Elem().Set(rv)
		v = p.Interface()
	}
	return json.Marshal(v)
}

// MarshalSlice returns the canonical JSON array for a slice of
// interface-typed RM values, boxing each element like [Marshal] so
// every element carries its `_type`.
//
// An empty or nil slice returns a nil RawMessage, preserving
// encoding/json's `omitempty` behaviour for the common cases (a nil or
// unset slice is omitted under `omitempty`, or re-emitted as null under
// a mandatory field). A non-empty slice always renders as a JSON array.
// (A mandatory but explicitly-empty interface slice would render as
// null rather than "[]"; no such value occurs in valid RM data, where a
// mandatory collection is never empty.)
func MarshalSlice[T any](s []T) (json.RawMessage, error) {
	if len(s) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i := range s {
		if i > 0 {
			buf.WriteByte(',')
		}
		raw, err := Marshal(any(s[i]))
		if err != nil {
			return nil, err
		}
		if raw == nil {
			raw = json.RawMessage("null")
		}
		buf.Write(raw)
	}
	buf.WriteByte(']')
	return json.RawMessage(buf.Bytes()), nil
}
