package instance_test

import (
	"encoding/binary"
	mrand "math/rand/v2"
	"slices"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// drawUIDs returns the text of the next n uids next issues.
func drawUIDs(next func() *rm.HierObjectID, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = next().Value
	}
	return out
}

// TestREQ107_UIDsFromDrawsVersion4UUIDs is the REQ-107 Reproducible uids
// check that every uid UIDsFrom issues is an RFC 9562 version-4 UUID in
// its lowercase hex-and-dash form: version 4 in the high nibble of byte 6
// and the variant 0b10 in the top two bits of byte 8. Were either left as
// drawn, all 64 uids would still carry it only by a chance of at most
// 2^-128, so the check fails when either is not set.
func TestREQ107_UIDsFromDrawsVersion4UUIDs(t *testing.T) {
	next := instance.UIDsFrom(mrand.NewPCG(7, 7))
	for _, s := range drawUIDs(next, 64) {
		u, err := uuid.Parse(s)
		if err != nil {
			t.Fatalf("uuid.Parse(%q): %v", s, err)
		}
		if u.String() != s {
			t.Errorf("uid %q is not in the lowercase hex-and-dash form %q", s, u.String())
		}
		if version := u[6] >> 4; version != 4 {
			t.Errorf("uid %q has version %d, want 4", s, version)
		}
		if variant := u[8] >> 6; variant != 0b10 {
			t.Errorf("uid %q has variant bits %02b, want 10 (RFC 9562)", s, variant)
		}
	}
}

// TestREQ107_UIDsFromTakesTheOtherBitsFromSource is the REQ-107
// Reproducible uids check that the 122 bits of a uid other than its
// version and variant are taken from the source. A second source in the
// same state gives the expected bits: two Uint64 draws per uid, the first
// for bytes 0 to 7 and the second for bytes 8 to 15, big-endian. The test
// pins that layout, the two draws per uid that UIDsFrom documents,
// because comparing with the source's own draws is what shows the bits
// come from the source.
func TestREQ107_UIDsFromTakesTheOtherBitsFromSource(t *testing.T) {
	next := instance.UIDsFrom(mrand.NewPCG(7, 7))
	reference := mrand.NewPCG(7, 7)
	for i := range 16 {
		var want uuid.UUID
		binary.BigEndian.PutUint64(want[:8], reference.Uint64())
		binary.BigEndian.PutUint64(want[8:], reference.Uint64())
		got, err := uuid.Parse(next().Value)
		if err != nil {
			t.Fatalf("uid %d: uuid.Parse: %v", i, err)
		}
		// Clear the version and variant bits on both sides, so only the
		// bits taken from the source are compared.
		for _, u := range []*uuid.UUID{&want, &got} {
			u[6] &= 0x0f
			u[8] &= 0x3f
		}
		if got != want {
			t.Errorf("uid %d: bits other than version and variant = %s, want %s, the source's two draws", i, got, want)
		}
	}
}

// TestREQ107_UIDsFromIsReproducible is the REQ-107 Reproducible uids
// check that two sources in the same state give the same uids in the same
// order, that a source in another state gives other uids, and that one
// source does not issue a uid twice.
func TestREQ107_UIDsFromIsReproducible(t *testing.T) {
	first := drawUIDs(instance.UIDsFrom(mrand.NewPCG(7, 7)), 16)
	second := drawUIDs(instance.UIDsFrom(mrand.NewPCG(7, 7)), 16)
	if !slices.Equal(first, second) {
		t.Errorf("two UIDsFrom(rand.NewPCG(7, 7)) issued\n  %v\nand\n  %v\nwant the same uids in the same order", first, second)
	}
	for _, s := range drawUIDs(instance.UIDsFrom(mrand.NewPCG(7, 8)), 16) {
		if slices.Contains(first, s) {
			t.Errorf("UIDsFrom(rand.NewPCG(7, 8)) issued %q, which UIDsFrom(rand.NewPCG(7, 7)) issued too", s)
		}
	}
	if distinct := slices.Compact(slices.Sorted(slices.Values(first))); len(distinct) != len(first) {
		t.Errorf("UIDsFrom(rand.NewPCG(7, 7)) issued %d distinct uids in %d, want each once: %v", len(distinct), len(first), first)
	}
}

// TestREQ107_UIDsFromNilSourceIsNil is the REQ-107 Reproducible uids
// check that a nil source gives a nil UIDSource, so the random fallback
// applies.
func TestREQ107_UIDsFromNilSourceIsNil(t *testing.T) {
	if next := instance.UIDsFrom(nil); next != nil {
		t.Errorf("UIDsFrom(nil) returned a non-nil func, want nil so Options.UIDSource stays nil")
	}
}
