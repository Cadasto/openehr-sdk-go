package canxml_test

// report_cost_test.go: REQ-170 and REQ-175 (wire.md § REQ-170 *Report*,
// § REQ-175 *Identification* and *Sent without a value*). The cost of a
// report grows with the size of the document, not with its depth times the
// members reported: a caller that only counts the items pays for no path
// text. Each row decodes a FOLDER nested to two depths, the second twice the
// first, where every level reports one item, and holds the deep decode's
// allocation within a small factor of twice the shallow one's.

import (
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// nestedFolder returns a FOLDER nested depth levels below the root, each
// level holding name and then inner, the content between its
// archetype_node_id and its sub-folder.
func nestedFolder(depth int, name, inner string) []byte {
	var b strings.Builder
	b.WriteString(`<folder xmlns="http://schemas.openehr.org/v1">`)
	b.WriteString(name + `<archetype_node_id>a</archetype_node_id>` + inner)
	for range depth {
		b.WriteString(`<folders>` + name + `<archetype_node_id>a</archetype_node_id>` + inner)
	}
	for range depth {
		b.WriteString(`</folders>`)
	}
	b.WriteString(`</folder>`)
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

func TestREQ170_REQ175_XMLReportCostGrowsWithTheDocument(t *testing.T) {
	const (
		shallow = 400
		deep    = 2 * shallow
		// factor bounds the deep decode's allocation against the shallow
		// one's. A report that grows with the document doubles with the
		// depth; one that writes each path from the root quadruples.
		factor = 3
	)
	rows := []struct {
		name  string
		named string // each level's name member
		inner string // what each level holds after its archetype_node_id
		opt   func(count *int) canxml.DecoderOption
	}{
		{
			name:  "WithPresence",
			named: `<name><value/></name>`,
			opt: func(count *int) canxml.DecoderOption {
				return canxml.WithPresence(func(string, canxml.Presence) { *count++ })
			},
		},
		{
			name: "WithMissing",
			opt: func(count *int) canxml.DecoderOption {
				return canxml.WithMissing(func(string) { *count++ })
			},
		},
		{
			name:  "WithUnmatched",
			named: `<name><value>x</value></name>`,
			inner: `<x/>`,
			opt: func(count *int) canxml.DecoderOption {
				return canxml.WithUnmatched(func(string) { *count++ })
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			alloc := func(depth int) uint64 {
				data := nestedFolder(depth, row.named, row.inner)
				return allocatedBytes(func() {
					count := 0
					var f rm.Folder
					if err := canxml.Unmarshal(data, &f, row.opt(&count)); err != nil {
						t.Fatalf("Unmarshal(depth %d) = %v, want nil", depth, err)
					}
					if count != depth+1 {
						t.Fatalf("Unmarshal(depth %d) reported %d items, want %d", depth, count, depth+1)
					}
				})
			}
			low, high := alloc(shallow), alloc(deep)
			if high > factor*low {
				t.Errorf("one item per level allocates %d bytes at depth %d and %d bytes at depth %d, want at most %d times as many",
					low, shallow, high, deep, factor)
			}
		})
	}
}
