package canjson_test

// report_cost_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification*). The cost of a report grows with the size of
// the document, not with its depth times the members reported: a caller that
// only counts the members pays for no pointer text. Each row decodes the same
// number of leaf FOLDERs, each reported once, under a FOLDER spine of two
// depths, and holds the deep decode's allocation within a small factor of the
// shallow one's.

import (
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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

// allocatedBytes returns the bytes fn allocates, the least of three runs, so
// that a collection or a goroutine of the runtime does not count.
func allocatedBytes(fn func()) uint64 {
	least := uint64(math.MaxUint64)
	for range 3 {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		fn()
		runtime.ReadMemStats(&after)
		least = min(least, after.TotalAlloc-before.TotalAlloc)
	}
	return least
}

func TestREQ170_REQ175_ReportCostGrowsWithTheDocument(t *testing.T) {
	const (
		leaves  = 1000
		shallow = 2
		deep    = 150
		// factor bounds the deep decode's allocation against the shallow
		// one's. The deep document is a little longer, by its spine; a
		// report that writes each pointer from the root allocates tens of
		// times more.
		factor = 2
	)
	rows := []struct {
		name string
		leaf string
		opt  func(count *int) canjson.DecoderOption
	}{
		{
			name: "WithMissing",
			leaf: `{"_type":"FOLDER","archetype_node_id":"a"}`,
			opt: func(count *int) canjson.DecoderOption {
				return canjson.WithMissing(func(canjson.Pointer) { *count++ })
			},
		},
		{
			name: "WithUnmatched",
			leaf: `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"x"},"archetype_node_id":"a","x":0}`,
			opt: func(count *int) canjson.DecoderOption {
				return canjson.WithUnmatched(func(canjson.Pointer) { *count++ })
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			alloc := func(depth int) uint64 {
				data := folderSpine(depth, leaves, row.leaf)
				return allocatedBytes(func() {
					count := 0
					var f rm.Folder
					if err := canjson.Unmarshal(data, &f, row.opt(&count)); err != nil {
						t.Fatalf("Unmarshal(depth %d) = %v, want nil", depth, err)
					}
					if count != leaves {
						t.Fatalf("Unmarshal(depth %d) reported %d members, want %d", depth, count, leaves)
					}
				})
			}
			low, high := alloc(shallow), alloc(deep)
			if high > factor*low {
				t.Errorf("%d reported members allocate %d bytes at depth %d and %d bytes at depth %d, want at most %d times as many",
					leaves, low, shallow, high, deep, factor)
			}
		})
	}
}
