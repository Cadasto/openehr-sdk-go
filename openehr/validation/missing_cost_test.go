package validation_test

// missing_cost_test.go: REQ-112 § Value-typed mandatory presence, PROBE-110:
// what reading a decode's report of missing members costs. WithMissingPointers
// reads each pointer one reference token at a time and meets a parent the
// members share once, so what it allocates, and the parents it asks for,
// grow with the distinct members the report names, not with each member's
// depth times their number, as writing each pointer's text does. Each test
// reads the report of the same leaf FOLDERs, each missing its name, under a
// FOLDER spine of two depths, and compares the deep call with the shallow
// one.
//
// The walk writes the path of each finding it reports, and that path grows
// with the member's depth however the report came. So each call validates
// the spine without its leaves: the call reads the whole report into the
// trie it threads down the walk, and the walk reads that trie down the
// spine, but it reaches no reported member and writes no path.

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// folderSpine returns a FOLDER nested depth deep whose innermost folder holds
// leaves leaf FOLDERs, each written as leaf.
func folderSpine(depth, leaves int, leaf string) []byte {
	var b strings.Builder
	for range depth {
		b.WriteString(`{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"a","folders":[`)
	}
	for i := range leaves {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(leaf)
	}
	for range depth {
		b.WriteString(`]}`)
	}
	return []byte(b.String())
}

// reportedFolderPointers returns the pointers canjson's WithMissing option
// passes for a FOLDER nested depth deep whose innermost folder holds leaves
// FOLDERs, each missing its name, and that FOLDER's spine decoded without
// them.
func reportedFolderPointers(t *testing.T, depth, leaves int) ([]canjson.Pointer, *rm.Folder) {
	t.Helper()
	var pointers []canjson.Pointer
	keep := canjson.WithMissing(func(p canjson.Pointer) { pointers = append(pointers, p) })
	if err := canjson.Unmarshal(folderSpine(depth, leaves, `{"_type":"FOLDER","archetype_node_id":"a"}`), new(rm.Folder), keep); err != nil {
		t.Fatalf("canjson.Unmarshal(depth %d, WithMissing) = %v, want nil", depth, err)
	}
	if len(pointers) != leaves {
		t.Fatalf("canjson.Unmarshal(depth %d) reported %d members, want %d", depth, len(pointers), leaves)
	}
	spine := new(rm.Folder)
	if err := canjson.Unmarshal(folderSpine(depth, 0, ""), spine); err != nil {
		t.Fatalf("canjson.Unmarshal(the spine of depth %d) = %v, want nil", depth, err)
	}
	return pointers, spine
}

// leastAllocated returns the bytes fn allocates, the least of three runs, so
// that a collection or a goroutine of the runtime does not count. A test
// that calls it does not run in parallel, so no other test's allocations are
// counted.
func leastAllocated(fn func()) uint64 {
	least := allocatedBytes(fn)
	for range 2 {
		least = min(least, allocatedBytes(fn))
	}
	return least
}

func TestWithMissingPointers_REQ112_CostGrowsWithTheMembers(t *testing.T) {
	const (
		leaves  = 1000
		shallow = 2
		deep    = 150
		// factor bounds the deep call's allocation against the shallow
		// one's. The deep report names the members of a longer spine too;
		// writing each pointer's text allocates ten times more.
		factor = 2
	)
	rows := []struct {
		name   string
		option func(pointers []canjson.Pointer) validation.ValidateOption
		// within is whether the row's deep call stays within factor. The
		// text route is the control: it writes each pointer's text, so
		// the measure tells the two routes apart, and a reading of the
		// pointers that writes their text fails the first row.
		within bool
	}{
		{
			name:   "WithMissingPointers",
			option: validation.WithMissingPointers[canjson.Pointer],
			within: true,
		},
		{
			name: "WithMissing of each pointer's text",
			option: func(pointers []canjson.Pointer) validation.ValidateOption {
				texts := make([]string, len(pointers))
				for i, p := range pointers {
					texts[i] = p.String()
				}
				return validation.WithMissing(texts)
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			alloc := func(depth int) uint64 {
				pointers, spine := reportedFolderPointers(t, depth, leaves)
				return leastAllocated(func() {
					if r := validation.ValidateRM(spine, row.option(pointers)); len(r.Issues) != 0 {
						t.Fatalf("ValidateRM(the spine of depth %d, %s) issues = %q, want none: the walk reaches no reported member", depth, row.name, issueKeys(r.Issues))
					}
				})
			}
			low, high := alloc(shallow), alloc(deep)
			if within := high <= factor*low; within != row.within {
				t.Errorf("ValidateRM(%s of %d members) allocates %d bytes at depth %d and %d bytes at depth %d; within %d times as many: %t, want %t",
					row.name, leaves, low, shallow, high, deep, factor, within, row.within)
			}
		})
	}
}

// countedPointer is a canjson.Pointer whose Parent calls are counted in
// calls, which the pointers of one report share.
type countedPointer struct {
	pointer canjson.Pointer
	calls   *int
}

// Parent counts the call, then returns the pointer's parent, counted alike.
func (p countedPointer) Parent() (countedPointer, bool) {
	*p.calls++
	up, ok := p.pointer.Parent()
	return countedPointer{pointer: up, calls: p.calls}, ok
}

// Token returns the pointer's last reference token.
func (p countedPointer) Token() string {
	return p.pointer.Token()
}

// TestWithMissingPointers_REQ112_MeetsASharedParentOnce pins that the call
// meets a parent the members of a report share once: the parents it asks
// the pointers of the deep report for stay within a small factor of those
// of the shallow one, where a climb from each member to the root asks for
// each member's depth times their number.
func TestWithMissingPointers_REQ112_MeetsASharedParentOnce(t *testing.T) {
	t.Parallel()
	const (
		leaves  = 1000
		shallow = 2
		deep    = 150
		factor  = 2
	)
	climbs := func(depth int) int {
		pointers, spine := reportedFolderPointers(t, depth, leaves)
		calls := 0
		counted := make([]countedPointer, len(pointers))
		for i, p := range pointers {
			counted[i] = countedPointer{pointer: p, calls: &calls}
		}
		if r := validation.ValidateRM(spine, validation.WithMissingPointers(counted)); len(r.Issues) != 0 {
			t.Fatalf("ValidateRM(the spine of depth %d, WithMissingPointers) issues = %q, want none: the walk reaches no reported member", depth, issueKeys(r.Issues))
		}
		return calls
	}
	low, high := climbs(shallow), climbs(deep)
	if high > factor*low {
		t.Errorf("WithMissingPointers(%d members) asks for %d parents at depth %d and %d at depth %d, want at most %d times as many",
			leaves, low, shallow, high, deep, factor)
	}
}
