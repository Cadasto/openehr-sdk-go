package webtemplate_test

// PROBE-075 — the vendored EHRbase openEHR_SDK v2.3 reference is the
// WebTemplate structural-parity oracle (REQ-106, ADR-0014). The fixture is
// vendored in-repo; a missing file is repo corruption and fails the run.

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// referenceDir is the vendored EHRbase parity fixture directory, relative
// to this package.
const referenceDir = "../../../testkit/corpus/webtemplate"

// referenceStem is the first vendored EHRbase parity fixture's filename
// stem — the historical single oracle, kept as the default other tests
// load. constrain_test pins no template-level node name anywhere, so its
// golden carries zero name predicates across all 104 nodes, which is why
// it reached parity before REQ-111 existed. Since REQ-111 landed the
// matrix is parityFixtures (build_test.go): this stem plus the two
// name-pinning oracles Corona_Anamnese and GECCO_Diagnose.
const referenceStem = "constrain_test"

// loadReference decodes the vendored reference WebTemplate JSON.
func loadReference(t *testing.T) map[string]any {
	t.Helper()
	return loadReferenceStem(t, referenceStem)
}

// loadReferenceStem decodes the vendored reference WebTemplate JSON for one
// fixture stem (see parityFixtures), with the reference's duplicate sibling
// ids mapped to this builder's spelling by mapReferenceSiblingIDs, so every
// parity compare reads one spelling.
func loadReferenceStem(t *testing.T, stem string) map[string]any {
	t.Helper()
	m := readReferenceStem(t, stem)
	if tree, ok := m["tree"].(map[string]any); ok {
		if _, err := mapReferenceSiblingIDs(tree); err != nil {
			t.Fatalf("reference %s: %v", stem, err)
		}
	}
	return m
}

// readReferenceStem decodes the vendored reference WebTemplate JSON for one
// fixture stem as the reference wrote it.
func readReferenceStem(t *testing.T, stem string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(referenceDir, stem+".webtemplate.json"))
	if err != nil {
		t.Fatalf("vendored reference fixture %s unreadable (PROBE-075): %v", stem, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("reference %s is not valid JSON: %v", stem, err)
	}
	return m
}

// mapReferenceSiblingIDs rewrites the duplicate sibling ids of a reference
// tree to the spelling this builder writes, the recorded deviation of
// deviations.md § Duplicate sibling ids. It returns each rename as
// "<id chain> -> <new id>", the chain in the reference's spelling, sorted.
//
// The reference's rule, read at the pinned openEHR_SDK commit
// (OPTParser.makeIdUnique with WebTemplateNode.getId): a node's children are
// grouped by id, and in a group of two or more the member at index i,
// counting from 0, gets the number i+1 written straight after the id, so
// claimants 2, 3, … read `<id>2`, `<id>3`. This builder writes the
// Simplified Formats spelling, `<id>_1`, `<id>_2`, each the next free one.
//
// A child is read as the reference's k-th claimant of a bare id only when its
// siblings hold that id and every lower claimant, as the rule writes them, so
// an ordinary id such as `section_2` or a lone `x2` is kept. A sibling that
// already holds the mapped spelling is an error: the builder would skip it
// for the next free number, which this mapping does not model.
func mapReferenceSiblingIDs(tree map[string]any) ([]string, error) {
	var renamed []string
	var walk func(m map[string]any, chain string) error
	walk = func(m map[string]any, chain string) error {
		chain += "/" + refStr(m, "id")
		children := refChildren(m)
		ids := make(map[string]bool, len(children))
		for _, c := range children {
			ids[refStr(c, "id")] = true
		}
		for _, c := range children {
			// The descendants first, so each chain is the reference's.
			if err := walk(c, chain); err != nil {
				return err
			}
		}
		for _, c := range children {
			id := refStr(c, "id")
			base, k, ok := referenceClaimant(id, ids)
			if !ok {
				continue
			}
			ours := base + "_" + strconv.Itoa(k-1)
			if ids[ours] {
				return fmt.Errorf("%s/%s: a sibling already holds %s, so the builder writes the next free suffix", chain, id, ours)
			}
			c["id"] = ours
			renamed = append(renamed, chain+"/"+id+" -> "+ours)
		}
		return nil
	}
	err := walk(tree, "")
	slices.Sort(renamed)
	return renamed, err
}

// referenceClaimant reports whether id is the reference's k-th claimant
// (k >= 2) of a bare id base, given the ids of id's siblings: id is base
// followed by k, and the siblings hold base and base2 to base(k-1).
func referenceClaimant(id string, siblings map[string]bool) (base string, k int, ok bool) {
	digits := len(id) - len(strings.TrimRightFunc(id, func(r rune) bool { return '0' <= r && r <= '9' }))
	for i := len(id) - digits; i < len(id); i++ {
		base, suffix := id[:i], id[i:]
		n, err := strconv.Atoi(suffix)
		if base == "" || suffix[0] == '0' || err != nil || n < 2 || !siblings[base] {
			continue
		}
		lower := true
		for j := 2; j < n && lower; j++ {
			lower = siblings[base+strconv.Itoa(j)]
		}
		if lower {
			return base, n, true
		}
	}
	return "", 0, false
}

// refChildren returns the object children of a reference node.
func refChildren(m map[string]any) []map[string]any {
	raw, _ := m["children"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		if cm, ok := c.(map[string]any); ok {
			out = append(out, cm)
		}
	}
	return out
}

func TestReferenceFixtureLoads(t *testing.T) {
	ref := loadReference(t)
	if ref["version"] != "2.3" {
		t.Errorf("reference version = %v, want 2.3", ref["version"])
	}
	if _, ok := ref["tree"].(map[string]any); !ok {
		t.Fatalf("reference has no object tree; keys=%v", slices.Sorted(maps.Keys(ref)))
	}
}

// PROBE-075, REQ-106 — mapReferenceSiblingIDs reads the reference's
// duplicate sibling ids, `<id>2`, `<id>3`, as the builder's `<id>_1`,
// `<id>_2`, and leaves every other id as the reference wrote it.
func TestREQ106MapReferenceSiblingIDs(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		want    []string
		wantErr bool
	}{
		{name: "claimants", ids: []string{"x", "x2", "x3"}, want: []string{"x", "x_1", "x_2"}},
		{name: "a lone numbered id", ids: []string{"y", "x2"}, want: []string{"y", "x2"}},
		{name: "a gap in the claimants", ids: []string{"x", "x3"}, want: []string{"x", "x3"}},
		{name: "an ordinary suffix", ids: []string{"section", "section_2", "cluster_1"}, want: []string{"section", "section_2", "cluster_1"}},
		{name: "a base ending in a digit", ids: []string{"x1", "x12"}, want: []string{"x1", "x1_1"}},
		{name: "a leading zero", ids: []string{"x", "x02"}, want: []string{"x", "x02"}},
		{name: "the mapped spelling taken", ids: []string{"x", "x2", "x_1"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			children := make([]any, len(tc.ids))
			for i, id := range tc.ids {
				children[i] = map[string]any{"id": id}
			}
			tree := map[string]any{"id": "root", "children": children}
			_, err := mapReferenceSiblingIDs(tree)
			if tc.wantErr {
				if err == nil {
					t.Errorf("mapReferenceSiblingIDs(%v) err = nil, want an error", tc.ids)
				}
				return
			}
			if err != nil {
				t.Fatalf("mapReferenceSiblingIDs(%v) err = %v", tc.ids, err)
			}
			got := make([]string, 0, len(tc.ids))
			for _, c := range refChildren(tree) {
				got = append(got, refStr(c, "id"))
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("mapReferenceSiblingIDs(%v) ids = %v, want %v", tc.ids, got, tc.want)
			}
		})
	}
}

// PROBE-075, REQ-106 — the renames mapReferenceSiblingIDs makes in each
// vendored golden are pinned whole, so the mapping can neither widen nor
// shift to another node unnoticed. Only test_all_types holds duplicate
// sibling ids; the other goldens separate every sibling by name.
func TestREQ106ReferenceSiblingIDRenames(t *testing.T) {
	want := map[string][]string{
		referenceStem:     nil,
		"Corona_Anamnese": nil,
		"GECCO_Diagnose":  nil,
		actionParityStem: {
			"/test_all_types/test_all_types/time2 -> time_1",
			"/test_all_types/test_all_types2 -> test_all_types_1",
			"/test_all_types/test_all_types3 -> test_all_types_2",
			"/test_all_types/test_all_types3/section_2/section_3/test_all_types2 -> test_all_types_1",
		},
	}
	for _, stem := range slices.Sorted(maps.Keys(want)) {
		t.Run(stem, func(t *testing.T) {
			got, err := mapReferenceSiblingIDs(refTree(t, readReferenceStem(t, stem)))
			if err != nil {
				t.Fatalf("mapReferenceSiblingIDs: %v", err)
			}
			if !slices.Equal(got, want[stem]) {
				t.Errorf("renames = %q, want %q (see deviations.md § Duplicate sibling ids)", got, want[stem])
			}
		})
	}
}
