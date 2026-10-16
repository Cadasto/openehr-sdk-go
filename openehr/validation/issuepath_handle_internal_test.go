package validation

// issuepath_handle_internal_test.go: REQ-102 § Issue paths — the issue
// paths the walk writes only when something records them. The path rule's
// own cases are in issuepath_internal_test.go; these hold that the handle of
// a node's path writes the path by that rule, and that the ways the walk
// applies the rule, counting a long list's ids into maps and a short one's
// by a scan, give the same predicates.

import (
	"fmt"
	"slices"
	"testing"
)

// byID, byName and byPosition return the predicate of an item written by its
// id, by its id and pinned name, and by its 1-based position.
func byID(id string) predicate {
	return predicate{id: id, kind: predicateID}
}

func byName(id, name string) predicate {
	return predicate{id: id, name: name, kind: predicateIDName}
}

func byPosition(n int) predicate {
	return predicate{kind: predicatePosition, pos: n}
}

// The handle of a path, pushed step by step as the walk pushes them, writes
// the path the string rule writes. Each expected string is the one the
// string functions the walk wrote paths with before it carried handles
// (attributePath, itemPaths, hashEntryPath, joinPath and a coded value's
// path) wrote for the same steps: the root, an attribute of the root, nested
// attributes, each kind of predicate, a name with a backslash and a quote,
// the entries of a Hash, and a sub-path a rule appends.
func TestIssuePath_REQ102_WritesTheStringRule(t *testing.T) {
	const bp = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	tests := []struct {
		name     string
		steps    []pathStep
		segments []string
		want     string
	}{
		{name: "root", want: "/"},
		{name: "attribute of the root", steps: []pathStep{{attr: "content"}}, want: "/content"},
		{name: "nested attributes", steps: []pathStep{{attr: "context"}, {attr: "other_context"}}, want: "/context/other_context"},
		{
			name:  "three nested attributes",
			steps: []pathStep{{attr: "context"}, {attr: "health_care_facility"}, {attr: "external_ref"}},
			want:  "/context/health_care_facility/external_ref",
		},
		{
			name:  "an item by its id",
			steps: []pathStep{{attr: "content"}, {isItem: true, item: byID("openEHR-EHR-OBSERVATION.bmi.v1")}},
			want:  "/content[openEHR-EHR-OBSERVATION.bmi.v1]",
		},
		{
			name:  "an item by its id and a name with a quote and a backslash",
			steps: []pathStep{{attr: "items"}, {isItem: true, item: byName("at0004", `O'Brien \ co`)}},
			want:  `/items[at0004, 'O\'Brien \\ co']`,
		},
		{name: "an item by its id and a plain name", steps: []pathStep{{attr: "items"}, {isItem: true, item: byName("at0004", "x")}}, want: "/items[at0004, 'x']"},
		{name: "an item by its position", steps: []pathStep{{attr: "items"}, {isItem: true, item: byPosition(3)}}, want: "/items[3]"},
		{name: "an item by a three-digit position", steps: []pathStep{{attr: "items"}, {isItem: true, item: byPosition(100)}}, want: "/items[100]"},
		{name: "an item by a five-digit position", steps: []pathStep{{attr: "items"}, {isItem: true, item: byPosition(20000)}}, want: "/items[20000]"},
		// A template may declare an attribute with no name; its step is still
		// an attribute's, written "/", never read as an item's.
		{name: "an attribute with an empty name, of the root", steps: []pathStep{{attr: ""}}, want: "/"},
		{
			name:  "an attribute with an empty name, below an item",
			steps: []pathStep{{attr: "content"}, {isItem: true, item: byID("at0001")}, {attr: ""}},
			want:  "/content[at0001]/",
		},
		{name: "an OPT child with no id of its own", steps: []pathStep{{attr: "items"}, {isItem: true, item: predicate{}}}, want: "/items"},
		{
			name: "items and attributes nested",
			steps: []pathStep{
				{attr: "content"},
				{isItem: true, item: byID(bp)},
				{attr: "data"},
				{attr: "events"},
				{isItem: true, item: byID("at0006")},
				{attr: "data"},
				{attr: "items"},
				{isItem: true, item: byPosition(2)},
				{attr: "value"},
			},
			want: "/content[" + bp + "]/data/events[at0006]/data/items[2]/value",
		},
		{
			name:  "the first entry of a Hash",
			steps: []pathStep{{attr: "description"}, {attr: "other_details"}, {isItem: true, item: entryPredicate(0)}},
			want:  "/description/other_details[1]",
		},
		{
			name:  "the tenth entry of a Hash",
			steps: []pathStep{{attr: "description"}, {attr: "other_details"}, {isItem: true, item: entryPredicate(9)}},
			want:  "/description/other_details[10]",
		},
		{name: "an entry of a Hash of the root", steps: []pathStep{{attr: "other_details"}, {isItem: true, item: entryPredicate(0)}}, want: "/other_details[1]"},
		{name: "a sub-path of the root", segments: []string{"/archetype_details"}, want: "/archetype_details"},
		{
			name:     "a sub-path of an item",
			steps:    []pathStep{{attr: "content"}, {isItem: true, item: byID(bp)}},
			segments: []string{"/archetype_details"},
			want:     "/content[" + bp + "]/archetype_details",
		},
		{
			name:     "a sub-path of an attribute of an item",
			steps:    []pathStep{{attr: "content"}, {isItem: true, item: byID(bp)}, {attr: "language"}},
			segments: []string{"/code_string"},
			want:     "/content[" + bp + "]/language/code_string",
		},
		{name: "a sub-path of an attribute", steps: []pathStep{{attr: "context"}}, segments: []string{"/archetype_details"}, want: "/context/archetype_details"},
		{name: "a coded text's code below the root", segments: []string{"/category", "/defining_code"}, want: "/category/defining_code"},
		{
			name:     "a coded text's code below a node",
			steps:    []pathStep{{attr: "context"}},
			segments: []string{"/setting", "/defining_code"},
			want:     "/context/setting/defining_code",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var s pathStack
			var p issuePath
			for _, step := range tc.steps {
				if step.isItem {
					p = s.item(p, step.item)
				} else {
					p = s.attribute(p, step.attr)
				}
			}
			got := p.String()
			if tc.segments != nil {
				got = p.join(tc.segments...)
			}
			if got != tc.want {
				t.Errorf("path of %+v with %q = %q, want %q", tc.steps, tc.segments, got, tc.want)
			}
		})
	}
}

// A handle writes the steps it spans whatever the walk pushes deeper, and
// whatever the walk replaces deeper once it moves on: the path of a node
// stays its path while the walk visits the node's attributes and items, and
// the next attribute of a node takes the place of the one before it.
func TestIssuePath_REQ102_KeepsItsStepsWhileTheWalkGoesDeeper(t *testing.T) {
	const obsPath = "/content[openEHR-EHR-OBSERVATION.bmi.v1]"
	var s pathStack
	content := s.attribute(issuePath{}, "content")
	obs := s.item(content, byID("openEHR-EHR-OBSERVATION.bmi.v1"))
	for _, name := range []string{"data", "protocol", "subject"} {
		attr := s.attribute(obs, name)
		for i := range 3 {
			s.attribute(s.item(s.attribute(attr, "items"), byPosition(i+1)), "value")
		}
		if got, want := attr.String(), obsPath+"/"+name; got != want {
			t.Errorf("the path of %s after the walk visited below it = %q, want %q", name, got, want)
		}
	}
	if got, want := obs.String(), obsPath; got != want {
		t.Errorf("the observation's path after the walk visited below it = %q, want %q", got, want)
	}
	if got, want := content.String(), "/content"; got != want {
		t.Errorf("the content's path after the walk visited below it = %q, want %q", got, want)
	}
}

// The two ways of counting the ids of a list's items give the same answers,
// on lists of every length up to past the longest one scanned, built from a
// few ids and names so that ids are shared and names repeat.
func TestIDSharing_REQ102_ScanAndMapsAgree(t *testing.T) {
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
func TestItemPaths_REQ102_RuleHoldsOnEachSideOfTheScan(t *testing.T) {
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
