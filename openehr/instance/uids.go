package instance

import (
	"encoding/binary"
	mrand "math/rand/v2"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// UIDsFrom returns a source for [Options.UIDSource] that draws each uid
// from src as an RFC 9562 version-4 UUID: two Uint64 draws give its 128
// bits, of which it sets the version and variant bits, so two sources in
// the same state give the same uids in the same order. A nil src gives a
// nil func, so UIDSource stays nil and the random fallback applies.
//
// The returned func draws from src, so, like a math/rand/v2.Source, it is
// not safe for concurrent use: give each concurrent Generate its own. A
// src shared with [Options.ValueSource] stays reproducible, but each uid
// drawn from it shifts the leaf values drawn after it; a separate source
// keeps the leaf values those of a run with no UIDSource.
func UIDsFrom(src mrand.Source) func() *rm.HierObjectID {
	if src == nil {
		return nil
	}
	return func() *rm.HierObjectID {
		var u uuid.UUID
		binary.BigEndian.PutUint64(u[:8], src.Uint64())
		binary.BigEndian.PutUint64(u[8:], src.Uint64())
		u[6] = u[6]&0x0f | 0x40 // version 4
		u[8] = u[8]&0x3f | 0x80 // variant 10, RFC 9562
		return &rm.HierObjectID{Value: u.String()}
	}
}
