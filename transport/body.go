package transport

import "bytes"

var jsonNull = []byte("null")

// IsNoRepresentationBody reports whether a 2xx response body carries no
// representation: zero bytes, whitespace only, or the JSON `null` literal
// (surrounding whitespace ignored).
//
// It is the single implementation of the "empty body" that both REQ-094
// (the write-result funnel) and REQ-151 (every other 2xx decode) classify.
// encoding/json unmarshals `null` into a struct as a nil-error no-op, so a
// decoder that only tests len(body) == 0 would hand back a populated-looking
// all-zero value; classifying against the raw bytes before decode is what
// keeps that arm honest. A hand-rolled leaf decode calls this rather than
// testing the length itself.
func IsNoRepresentationBody(b []byte) bool {
	body := bytes.TrimSpace(b)
	return len(body) == 0 || bytes.Equal(body, jsonNull)
}
