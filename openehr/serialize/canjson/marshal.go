package canjson

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// ErrInvalidValue is the canjson-local sentinel for a value the
// canonical-JSON encoder refuses. It is the encode-side
// counterpart of [ErrInvalidShape], which stays decode-only: the two,
// together with the transport-level transport.ErrInvalidShape, are
// three distinct sentinels, so `errors.Is` alone tells "this value
// cannot be encoded" from "these bytes cannot be decoded". The codec's
// own error stays reachable through unwrapping (a [json.SemanticError]
// for a value the codec cannot represent, such as a NaN or a channel,
// or a [jsontext.SyntacticError] for invalid UTF-8 reached on output).
var ErrInvalidValue = errors.New("canjson: value cannot be encoded")

// Marshal returns the canonical JSON encoding of v.
//
// The wire profile is implemented per RM type by the
// generator-emitted MarshalJSONTo methods (encoding/json/v2); this
// entry point drives them through [encoding/json/v2.Marshal], threading
// [typereg.EncodeOptions] so a bare map or slice handed in directly, and
// every value the generated methods reach, share the deterministic-map
// and nil-container spellings the generated methods use. `<`, `>` and
// `&` are emitted literally: v2 does not HTML-escape.
//
// A failure wraps [ErrInvalidValue] over the codec's own error with
// `%w: %w`, so both the sentinel and the cause stay reachable.
// Success returns the encoder's bytes unchanged.
func Marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v, typereg.EncodeOptions())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}

// MarshalIndent is like [Marshal] but applies prefix and indent to
// each element, through [jsontext.WithIndentPrefix] and
// [jsontext.WithIndent]. Use it for human inspection only; round-trip
// fidelity is checked semantically over compact [Marshal] output, not
// by comparing indented bytes.
//
// encoding/json/v2 accepts only spaces and tabs in prefix and indent,
// so a prefix or indent carrying any other character is refused with
// [ErrInvalidValue] rather than passed through. The refusal happens
// before any option is built, so no panic crosses the package boundary.
// A failure otherwise wraps [ErrInvalidValue] the same way [Marshal]
// does.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	// The caller's string is not echoed: the sentinel plus a fixed message
	// classify the failure without leaking the offending input.
	if strings.Trim(prefix, " \t") != "" || strings.Trim(indent, " \t") != "" {
		return nil, fmt.Errorf("%w: MarshalIndent: indent and prefix may contain only spaces and tabs", ErrInvalidValue)
	}
	b, err := json.Marshal(v, typereg.EncodeOptions(),
		jsontext.WithIndentPrefix(prefix), jsontext.WithIndent(indent))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidValue, err)
	}
	return b, nil
}
