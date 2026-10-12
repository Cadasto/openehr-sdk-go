package canxml_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// deepClusterTree returns an ITEM_TREE holding depth CLUSTER items, each
// inside the one before, with an item of an unknown `xsi:type` innermost.
func deepClusterTree(depth int) []byte {
	var b strings.Builder
	b.WriteString(`<item_tree ` + ucNS + ` archetype_node_id="at0">`)
	for range depth {
		b.WriteString(`<items xsi:type="CLUSTER" archetype_node_id="at1">`)
	}
	b.WriteString(`<items xsi:type="NO_SUCH_CLASS" archetype_node_id="at2"/>`)
	for range depth {
		b.WriteString(`</items>`)
	}
	b.WriteString(`</item_tree>`)
	return []byte(b.String())
}

// decodeDeep decodes in into an ITEM_TREE, renders the error, and returns the
// error's message, the bytes allocated meanwhile (the least of three runs, so
// a stray allocation elsewhere in the process does not count) and the error.
func decodeDeep(t *testing.T, in []byte) (string, uint64, error) {
	t.Helper()
	var (
		err  error
		msg  string
		best uint64
	)
	for i := range 3 {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		err = canxml.Unmarshal(in, new(rm.ItemTree))
		if err != nil {
			msg = err.Error()
		}
		runtime.ReadMemStats(&after)
		if n := after.TotalAlloc - before.TotalAlloc; i == 0 || n < best {
			best = n
		}
	}
	return msg, best, err
}

// TestREQ056_XMLDeepDispatchFailureCostsLinearly pins the cost of a decode
// that fails deep inside nested polymorphic slots: the error, built and
// rendered, costs memory in proportion to the depth, not to its square. Each
// slot on the way out adds its own envelope around the failure below; when
// every envelope formatted the whole message below it, a failure 10,000 slots
// deep took seconds.
//
// Doubling the depth must at most about double the bytes allocated (the
// bound is three times, against four for a cost that grows with the square).
// The error keeps its classification: it wraps typereg.ErrUnknownType, and
// the outermost *canxml.DecodeError names the outermost slot.
//
// Can-fail control: wrap the failure below in a formatted error at every
// slot, or render a chain of envelopes by formatting each inner message in
// turn, and the bytes allocated grow with the square of the depth.
func TestREQ056_XMLDeepDispatchFailureCostsLinearly(t *testing.T) {
	const depth = 1500
	msgN, allocN, errN := decodeDeep(t, deepClusterTree(depth))
	msg2N, alloc2N, err2N := decodeDeep(t, deepClusterTree(2*depth))

	for _, tc := range []struct {
		depth int
		err   error
	}{{depth, errN}, {2 * depth, err2N}} {
		if !errors.Is(tc.err, typereg.ErrUnknownType) {
			t.Fatalf("canxml.Unmarshal(depth %d) err = %.200v; want errors.Is(_, typereg.ErrUnknownType)", tc.depth, tc.err)
		}
		de, ok := errors.AsType[*canxml.DecodeError](tc.err)
		if !ok || de == nil {
			t.Fatalf("canxml.Unmarshal(depth %d) err = %.200v; want a *canxml.DecodeError", tc.depth, tc.err)
		}
		if de.Path != "/items" || de.Type != "CLUSTER" {
			t.Errorf("canxml.Unmarshal(depth %d): outermost DecodeError Path=%q Type=%q; want /items and CLUSTER", tc.depth, de.Path, de.Type)
		}
	}
	if !strings.HasSuffix(msg2N, `canxml: "NO_SUCH_CLASS": typereg: _type not in registry`) {
		t.Errorf("canxml.Unmarshal(depth %d) message ends %q; want it to end with the innermost failure", 2*depth, msg2N[max(0, len(msg2N)-120):])
	}
	if got := strings.Count(msg2N, "decode /items"); got != 2*depth+1 {
		t.Errorf("canxml.Unmarshal(depth %d) message names %d slots; want %d, one per slot on the way out", 2*depth, got, 2*depth+1)
	}
	if len(msg2N) > 3*len(msgN) {
		t.Errorf("message length: depth %d gives %d bytes, depth %d gives %d; want it to grow with the depth", depth, len(msgN), 2*depth, len(msg2N))
	}
	if alloc2N > 3*allocN {
		t.Errorf("bytes allocated: depth %d costs %d, depth %d costs %d (%.1fx); want at most 3x: the cost grows with the square of the depth",
			depth, allocN, 2*depth, alloc2N, float64(alloc2N)/float64(allocN))
	}
	t.Logf("depth %d: %d bytes allocated, message %d bytes; depth %d: %d bytes allocated, message %d bytes",
		depth, allocN, len(msgN), 2*depth, alloc2N, len(msg2N))
}

// TestREQ056_XMLNestedDispatchFailureMessage pins the message of a failure two
// slots deep: each slot names itself, from the outside in, and the failure
// itself comes last, with the canxml prefix it has at one level.
func TestREQ056_XMLNestedDispatchFailureMessage(t *testing.T) {
	err := canxml.Unmarshal(deepClusterTree(1), new(rm.ItemTree))
	const want = `decode /items (_type="CLUSTER"): decode /items (_type="NO_SUCH_CLASS"): canxml: "NO_SUCH_CLASS": typereg: _type not in registry`
	if err == nil || err.Error() != want {
		t.Errorf("canxml.Unmarshal err = %v\nwant %s", err, want)
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("canxml.Unmarshal err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}

	one := canxml.Unmarshal(deepClusterTree(0), new(rm.ItemTree))
	const wantOne = `decode /items (_type="NO_SUCH_CLASS"): canxml: "NO_SUCH_CLASS": typereg: _type not in registry`
	if one == nil || one.Error() != wantOne {
		t.Errorf("canxml.Unmarshal (one level) err = %v\nwant %s", one, wantOne)
	}
}
