package webtemplate

import (
	"testing"
)

// REQ-116 / REQ-106 — the ordinal fallback renames later claimants to the
// next *free* spelling. A plain per-id counter would rename the third of
// [x, x2, x] to the already-taken x2 and trip ErrIDCollision on a template
// the reference can export.
func TestDedupeSiblingIDs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"no collision untouched", []string{"a", "b"}, []string{"a", "b"}},
		{"upstream two-claimant spelling", []string{"dv_text", "dv_text"}, []string{"dv_text", "dv_text2"}},
		{"three claimants", []string{"x", "x", "x"}, []string{"x", "x2", "x3"}},
		{"ordinal already taken later", []string{"x", "x2", "x"}, []string{"x", "x2", "x3"}},
		{"ordinal already taken earlier", []string{"x2", "x", "x"}, []string{"x2", "x", "x3"}},
		{"rename must dodge a later original", []string{"x", "x", "x3"}, []string{"x", "x2", "x3"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := &Node{ID: "root"}
			for _, id := range tc.in {
				parent.Children = append(parent.Children, &Node{ID: id})
			}
			dedupeSiblingIDs(parent)
			got := make([]string, len(parent.Children))
			for i, ch := range parent.Children {
				got[i] = ch.ID
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", got, tc.want)
				}
			}
			if err := checkIDCollisions(parent); err != nil {
				t.Fatalf("checkIDCollisions after dedupe: %v", err)
			}
		})
	}
}
