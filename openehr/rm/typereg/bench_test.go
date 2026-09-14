package typereg_test

// bench_test.go: registry dispatch baselines for the move of the
// canonical-JSON path to encoding/json/v2
// (docs/plans/2026-09-14-json-v2-migration.md, phase 3.5).
//
// [Registry.Decode] is where the `_type` peek and the concrete decode meet, and
// it is the site the migration changes most: today each nested polymorphic slot
// re-reads its own bytes to find the discriminator, and ADR 0022 replaces that
// with one json.UnmarshalFromFunc per interface, threading options down the
// decoder. These two benchmarks isolate that cost from the surrounding
// composition walk, one on a polymorphic node and one on a leaf.
//
// The package under test is imported for its own exported surface; the rm
// import is what populates typereg.Default, since the generated registration
// lives in the rm package's init.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// benchElement is an ELEMENT with both of its polymorphic slots filled: `name`
// dispatches to DV_TEXT and `value` to DV_QUANTITY, so one Decode call covers
// the outer peek and two nested ones.
var benchElement = []byte(`{"_type":"ELEMENT","archetype_node_id":"at0004","name":{"_type":"DV_TEXT","value":"systolic"},"value":{"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}}`)

// benchDVQuantity is the leaf half: one peek and one concrete decode, no nested
// dispatch, so the difference between the two benchmarks is the dispatch cost.
var benchDVQuantity = []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)

// BenchmarkRegistryDecodeElement measures the `_type` peek plus the concrete
// decode on a node whose own fields dispatch again.
func BenchmarkRegistryDecodeElement(b *testing.B) {
	benchRegistryDecode(b, benchElement, func(v any) bool {
		_, ok := v.(*rm.Element)
		return ok
	}, "*rm.Element")
}

// BenchmarkRegistryDecodeDVQuantity measures the same path on a leaf type.
func BenchmarkRegistryDecodeDVQuantity(b *testing.B) {
	benchRegistryDecode(b, benchDVQuantity, func(v any) bool {
		_, ok := v.(*rm.DVQuantity)
		return ok
	}, "*rm.DVQuantity")
}

// benchRegistryDecode runs one Decode benchmark. The setup decode is the
// control: a body the registry stopped resolving, or resolved to the wrong
// concrete type, fails here instead of being measured as a cheap error path.
func benchRegistryDecode(b *testing.B, body []byte, isWanted func(any) bool, wantType string) {
	b.Helper()
	setup, err := typereg.Default.Decode(body)
	if err != nil {
		b.Fatalf("setup decode: %v", err)
	}
	if !isWanted(setup) {
		b.Fatalf("setup decode produced %T, want %s", setup, wantType)
	}
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := typereg.Default.Decode(body); err != nil {
			b.Fatalf("Decode: %v", err)
		}
	}
}
