package canjson

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// ErrInvalidValue is the canjson-local sentinel for a value the
// canonical-JSON encoder refuses (REQ-052). It is the encode-side
// counterpart of [ErrInvalidShape], which stays decode-only: the two,
// together with the transport-level transport.ErrInvalidShape, are
// three distinct sentinels, so `errors.Is` alone tells "this value
// cannot be encoded" from "these bytes cannot be decoded". The
// underlying encoder error — a [json.SemanticError] for a value the
// codec cannot represent (a NaN, a channel), a [jsontext.SyntacticError]
// for invalid UTF-8 reached on output — stays reachable through
// unwrapping.
var ErrInvalidValue = errors.New("canjson: value cannot be encoded")

// Marshal returns the canonical JSON encoding of v.
//
// The wire profile (REQ-052) is implemented per-RM-type by the
// generator-emitted MarshalJSONTo methods (encoding/json/v2); this
// entry point drives them through [encoding/json/v2.Marshal], threading
// [typereg.EncodeOptions] so a bare map or slice handed in directly, and
// every value the generated methods reach, share the deterministic-map
// and nil-container spellings the generated methods use. `<`, `>` and
// `&` are emitted literally: v2 does not HTML-escape (REQ-052).
//
// A failure wraps [ErrInvalidValue] over the encoder's own error with
// `%w: %w`, so both the sentinel and the cause stay reachable
// (REQ-052). Success returns the encoder's bytes unchanged.
func Marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v, typereg.EncodeOptions())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}

// MarshalIndent is like [Marshal] but applies prefix and indent to
// each element, through [jsontext.WithIndentPrefix] and
// [jsontext.WithIndent]. Use for human inspection only — round-trip
// fidelity is asserted semantically over compact [Marshal] output, not
// by comparing indented bytes (REQ-052).
//
// A failure wraps [ErrInvalidValue] the same way [Marshal] does
// (REQ-052).
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	b, err := json.Marshal(v, typereg.EncodeOptions(),
		jsontext.WithIndentPrefix(prefix), jsontext.WithIndent(indent))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}
