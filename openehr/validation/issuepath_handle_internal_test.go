package validation

// issuepath_handle_internal_test.go: the issue paths the walk writes only
// when something records them. The path rule's own cases are in
// issuepath_internal_test.go; these hold that the ways the walk applies it,
// counting a long list's ids into maps and a short one's by a scan, give the
// same predicates.

import (
	"fmt"
	"slices"
	"testing"
)

// The two ways of counting the ids of a list's items give the same answers,
// on lists of every length up to past the longest one scanned, built from a
// few ids and names so that ids are shared and names repeat.
func TestIDSharing_ScanAndMapsAgree(t *testing.T) {
	ids := []string{"", "at0001", "at0002", "at0003"}
	names := []string{"", "Systolic", "Diastolic"}
	for n := 1; n <= 3*sharingScanMax; n++ {
		for seed := range 8 {
			items := make([]predicate, n)
			for i := range items {
				k := (i*7 + seed*13 + i*i*seed) % (len(ids) * len(names))
				items[i].pathItem = pathItem{id: ids[k%len(ids)], name: names[k/len(ids)]}
			}
			scan, maps := idSharing{items: items}, countedIDSharing(items)
			for i, it := range items {
				if got, want := scan.shared(it.id), maps.shared(it.id); got != want {
					t.Fatalf("list %+v: item %d: the scan says shared %t, the maps %t", items, i, got, want)
				}
				if !maps.shared(it.id) {
					continue // tellsApart is asked only of an item whose id is shared
				}
				if got, want := scan.tellsApart(it.pathItem), maps.tellsApart(it.pathItem); got != want {
					t.Fatalf("list %+v: item %d: the scan says its name tells it apart %t, the maps %t", items, i, got, want)
				}
			}
		}
	}
}

// The path rule writes the same paths on each side of the longest list whose
// ids are scanned: the items of a list just past it, whose ids are counted
// into maps, are written as those of a list just short of it.
func TestItemPaths_RuleHoldsOnEachSideOfTheScan(t *testing.T) {
	for _, n := range []int{sharingScanMax, sharingScanMax + 1} {
		t.Run(fmt.Sprintf("%d items", n), func(t *testing.T) {
			tests := []struct {
				name  string
				items func(i int) pathItem
				want  func(i int) string
			}{
				{
					name:  "unique ids",
					items: func(i int) pathItem { return pathItem{id: fmt.Sprintf("at%04d", i+1)} },
					want:  func(i int) string { return fmt.Sprintf("/items[at%04d]", i+1) },
				},
				{
					name:  "shared id, no pinned names",
					items: func(int) pathItem { return pathItem{id: "at0004"} },
					want:  func(i int) string { return fmt.Sprintf("/items[%d]", i+1) },
				},
				{
					name:  "shared id, distinct pinned names",
					items: func(i int) pathItem { return pathItem{id: "at0004", name: fmt.Sprintf("name %d", i)} },
					want:  func(i int) string { return fmt.Sprintf("/items[at0004, 'name %d']", i) },
				},
				{
					name: "shared id, the first two bound to one named node",
					items: func(i int) pathItem {
						return pathItem{id: "at0004", name: fmt.Sprintf("name %d", max(i, 1))}
					},
					want: func(i int) string {
						if i < 2 {
							return fmt.Sprintf("/items[%d]", i+1)
						}
						return fmt.Sprintf("/items[at0004, 'name %d']", i)
					},
				},
			}
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					items := make([]pathItem, n)
					want := make([]string, n)
					for i := range n {
						items[i], want[i] = tc.items(i), tc.want(i)
					}
					if got := itemPaths("/items", asPredicates(items)); !slices.Equal(got, want) {
						t.Errorf("itemPaths(%+v) = %q, want %q", items, got, want)
					}
				})
			}
		})
	}
}

// BenchmarkIDSharing measures the two ways of counting the ids of a list's
// items, each carrying its own id, the case where the scan reads the whole
// list for every item, so that [sharingScanMax] can be set where the maps
// start to cost less.
func BenchmarkIDSharing(b *testing.B) {
	for _, n := range []int{4, 8, 12, 16, 24} {
		items := make([]predicate, n)
		for i := range items {
			items[i].id = fmt.Sprintf("at%04d", i+1)
		}
		count := func(b *testing.B, s func([]predicate) idSharing) {
			b.ReportAllocs()
			for b.Loop() {
				sharing := s(items)
				for i := range items {
					if sharing.shared(items[i].id) {
						_ = sharing.tellsApart(items[i].pathItem)
					}
				}
			}
		}
		b.Run(fmt.Sprintf("%d/scan", n), func(b *testing.B) {
			count(b, func(items []predicate) idSharing { return idSharing{items: items} })
		})
		b.Run(fmt.Sprintf("%d/maps", n), func(b *testing.B) { count(b, countedIDSharing) })
	}
}
