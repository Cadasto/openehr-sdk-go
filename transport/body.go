package transport

import "bytes"

var jsonNull = []byte("null")

// IsNoRepresentationBody reports whether a 2xx response body carries no
// representation: zero bytes, whitespace only, or the JSON `null` literal
// (surrounding whitespace ignored).
//
// The SDK uses it for every 2xx body it classifies, write results
// included. Checking the raw bytes before decoding matters because
// encoding/json unmarshals `null` into a struct as a no-op with a nil
// error, so a decoder that only tests len(body) == 0 would return an
// all-zero value that looks populated. The SDK does not set
// MergeWithLegacySemantics, so under encoding/json/v2 a `null` body that
// slipped past this check would always decode to an all-zero struct with
// a nil error. Hand-written decoders call this function instead of
// testing the length themselves.
func IsNoRepresentationBody(b []byte) bool {
	body := bytes.TrimSpace(b)
	return len(body) == 0 || bytes.Equal(body, jsonNull)
}
