package canjson

import (
	"encoding/json"
)

// Marshal returns the canonical JSON encoding of v.
//
// The wire profile (REQ-052) is implemented per-RM-type by the
// generator-emitted MarshalJSON methods; this entry point is a thin
// pass-through to encoding/json so the codec can be swapped (sonic,
// easyjson) behind a build tag without touching call sites.
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// MarshalIndent is like [Marshal] but applies prefix and indent to
// each element. Use for human inspection only — byte-stability tests
// compare against compact [Marshal] output.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}
