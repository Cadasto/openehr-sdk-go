package canjson

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidValue is the canjson-local sentinel for a value the
// canonical-JSON encoder refuses (REQ-052). It is the encode-side
// counterpart of [ErrInvalidShape], which stays decode-only: the two,
// together with the transport-level transport.ErrInvalidShape, are
// three distinct sentinels, so `errors.Is` alone tells "this value
// cannot be encoded" from "these bytes cannot be decoded". The
// underlying encoder error (for example [json.UnsupportedTypeError])
// stays reachable through unwrapping.
var ErrInvalidValue = errors.New("canjson: value cannot be encoded")

// Marshal returns the canonical JSON encoding of v.
//
// The wire profile (REQ-052) is implemented per-RM-type by the
// generator-emitted MarshalJSON methods; this entry point is a thin
// pass-through to encoding/json so the codec can be swapped (sonic,
// easyjson) behind a build tag without touching call sites.
//
// A failure wraps [ErrInvalidValue] over the encoder's own error
// (REQ-052). Success returns the encoder's bytes unchanged.
func Marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}

// MarshalIndent is like [Marshal] but applies prefix and indent to
// each element. Use for human inspection only — byte-stability tests
// compare against compact [Marshal] output.
//
// A failure wraps [ErrInvalidValue] the same way [Marshal] does
// (REQ-052).
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	b, err := json.MarshalIndent(v, prefix, indent)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}
