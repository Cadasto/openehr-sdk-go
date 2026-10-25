package main

import (
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"uuid"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
)

// runIDs names the EHRs one run creates. Every id begins with the run's own
// identifier, so an EHR the run leaves on a CDR is attributable to it and
// removable by hand (conformance.md § REQ-082, Live mode).
//
// An id is a version 8 UUID (RFC 9562 section 5.8, a layout the application
// defines): its first 48 bits are the per-run identifier, drawn at random
// when the run starts; the version and variant bits follow, with the bits
// around them zero; its last 48 bits number the EHRs the run has asked for,
// from 1. The run 3f2a9c1e-7b40 names its EHRs
// 3f2a9c1e-7b40-8000-8000-000000000001, 3f2a9c1e-7b40-8000-8000-000000000002
// and so on.
type runIDs struct {
	run [6]byte
	n   atomic.Uint64
}

func newRunIDs() *runIDs {
	u := uuid.NewV4() // its first 48 bits are random
	g := &runIDs{}
	copy(g.run[:], u[:6])
	return g
}

// id is the per-run identifier as every EHR id begins with it: 12 hex
// digits, the first 8 and the last 4 joined by a hyphen.
func (g *runIDs) id() string {
	h := hex.EncodeToString(g.run[:])
	return h[:8] + "-" + h[8:]
}

// next returns the id of the run's next EHR. Workers call it concurrently.
func (g *runIDs) next() openehrclient.EHRID {
	var seq [8]byte
	binary.BigEndian.PutUint64(seq[:], g.n.Add(1))
	var u uuid.UUID
	copy(u[:6], g.run[:])
	u[6] = 0x80 // version 8
	u[8] = 0x80 // the RFC 9562 variant
	copy(u[10:], seq[2:])
	return openehrclient.EHRID(u.String())
}
