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
// gives it. Type is the `_type` a polymorphic slot read, when the slot is
// itself the failing value, as it is for a `_type` the registry does not
// know; a failure inside the slot's value leaves Type empty, and the slot's
// own envelope beneath names it. Value holds the JSON text of the value the decoder could not
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
	f := locateFailure(err)
	var value constraints.Redacted
	if f.found {
		value = constraints.Redact(f.literal)
	}
	if de, ok := err.(*DecodeError); ok && de != nil { //nolint:errorlint // only an envelope at the top keeps its own path
		out := *de
		if out.Value.Reveal() == nil {
			out.Value = value
		}
		return &out
	}
	return &DecodeError{Path: f.pointer, Type: f.typ, Value: value, Inner: err}
}

// failure is what locateFailure reads from an error chain.
type failure struct {
	pointer string // the failing value's JSON Pointer from the root of the document
	typ     string // the `_type` the slot at pointer read, if the failing value is a slot
	literal string // the literal the decoder could not take, when found
	found   bool
}

// locateFailure walks err's chain and returns the JSON Pointer of the failing
// value from the root of the document, the `_type` of the slot at that
// pointer, and the literal the decoder could not take, the deepest one the
// chain records.
//
// A pointer in the chain is absolute within the decoder that raised it. A
// polymorphic slot is decoded on a decoder of its own, or, nested in a slot read
// whole, has its positions moved to count from it (see
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
func locateFailure(err error) failure {
	var f failure
	var base string     // the pointer of the root of the decoder the walk is in
	var slotPath string // the pointer of the deepest slot the walk has passed
	for err != nil {
		switch e := err.(type) { //nolint:errorlint // each link of the chain is read in turn, by its own type
		case *json.SemanticError:
			if e == nil {
				return f.result(slotPath)
			}
			if e.JSONPointer != "" {
				f.pointer = base + string(e.JSONPointer)
			}
			if len(e.JSONValue) > 0 {
				f.literal, f.found = string(e.JSONValue), true
			}
		case *jsontext.SyntacticError:
			if e == nil {
				return f.result(slotPath)
			}
			if e.JSONPointer != "" {
				f.pointer = base + string(e.JSONPointer)
			}
		case *DecodeError:
			if e == nil {
				return f.result(slotPath)
			}
			if e.Type == "" && e.Path == typeMemberPath {
				f.pointer += e.Path
				break
			}
			if e.Path != "" {
				f.pointer = base + e.Path
			}
			base = f.pointer
			slotPath, f.typ = f.pointer, e.Type
		case *refused.Error:
			if e == nil {
				return f.result(slotPath)
			}
			f.literal, f.found = e.Text(), true
		}
		err = errors.Unwrap(err)
	}
	return f.result(slotPath)
}

// result returns f with its type kept only when the deepest slot the walk
// passed, at slotPath, is the failing value itself.
func (f failure) result(slotPath string) failure {
	if f.pointer != slotPath {
		f.typ = ""
	}
	return f
}

// typeMemberPath is the Path of [typereg.DecodeInto]'s refusal of a whole
// value's `_type`: the member of that value, not a slot.
const typeMemberPath = "/_type"
