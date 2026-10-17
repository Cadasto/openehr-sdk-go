package canjson

// The envelope the two decode entry points return (REQ-052): a DecodeError at
// the top of the chain, with the failing value's pointer and the literal the
// decoder refused.

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/refused"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// entryError returns err as [Unmarshal] and [Decoder.Decode] report it: a
// [*DecodeError] at the top of the chain, so errors.AsType[*DecodeError]
// finds it without unwrapping, with err as its Inner. Path is the JSON
// Pointer of the failing value from the root of the document, where the chain
// gives it. Value holds the JSON text of the value the decoder could not
// take, as the document wrote it, where the chain carries one: the number or
// string a *json.SemanticError records for a value its Go type cannot hold,
// or a literal a primitive codec refused and carried with [refused.Literal].
// It is empty otherwise.
//
// An err that is itself a non-nil *DecodeError keeps its Path, Type and
// Inner and gains Value from the chain beneath it; the result is a copy, and
// err is left as it was. A nil err returns nil.
//
// entryError wraps whatever it is given. Leaving out the errors the input
// does not cause, a failing reader's or io.EOF at the end of a stream, is the
// entry point's part.
func entryError(err error) error {
	if err == nil {
		return nil
	}
	path, literal, found := locateFailure(err)
	var value constraints.Redacted
	if found {
		value = constraints.Redact(literal)
	}
	if de, ok := err.(*DecodeError); ok && de != nil { //nolint:errorlint // only an envelope at the top keeps its own path
		out := *de
		if out.Value.Reveal() == nil {
			out.Value = value
		}
		return &out
	}
	return &DecodeError{Path: path, Value: value, Inner: err}
}

// locateFailure walks err's chain and returns the JSON Pointer of the failing
// value from the root of the document and the literal the decoder could not
// take, the deepest one the chain records.
//
// A pointer in the chain is absolute within the decoder that raised it. A
// polymorphic slot is decoded on a decoder of its own (see
// [typereg.DecodePolymorphic]), so beneath a slot's [DecodeError] pointers
// count from the slot, which the slot's Path places; a slot that fell back to
// its declared type for want of a `_type` has no Type, so the slot is told by
// what it is not. The one envelope that is not a slot's is
// [typereg.DecodeInto]'s refusal of a `_type` that names another class, whose
// Path is the `_type` member of the value being decoded, so it extends the
// pointer so far. (A depth refusal or a nil argument has no Path and moves
// nothing.) A hand-written primitive codec decodes its literal on a decoder
// of its own too, but a literal has no members, so the pointers beneath it
// are empty and leave the pointer as it is.
func locateFailure(err error) (pointer, literal string, found bool) {
	var base string // the pointer of the root of the decoder the walk is in
	for err != nil {
		switch e := err.(type) { //nolint:errorlint // each link of the chain is read in turn, by its own type
		case *json.SemanticError:
			if e == nil {
				return pointer, literal, found
			}
			if e.JSONPointer != "" {
				pointer = base + string(e.JSONPointer)
			}
			if len(e.JSONValue) > 0 {
				literal, found = string(e.JSONValue), true
			}
		case *jsontext.SyntacticError:
			if e == nil {
				return pointer, literal, found
			}
			if e.JSONPointer != "" {
				pointer = base + string(e.JSONPointer)
			}
		case *DecodeError:
			if e == nil {
				return pointer, literal, found
			}
			if e.Type == "" && e.Path == typeMemberPath {
				pointer += e.Path
				break
			}
			if e.Path != "" {
				pointer = base + e.Path
			}
			base = pointer
		case *refused.Error:
			if e == nil {
				return pointer, literal, found
			}
			literal, found = e.Text(), true
		}
		err = errors.Unwrap(err)
	}
	return pointer, literal, found
}

// typeMemberPath is the Path of [typereg.DecodeInto]'s refusal of a whole
// value's `_type`: the member of that value, not a slot.
const typeMemberPath = "/_type"
