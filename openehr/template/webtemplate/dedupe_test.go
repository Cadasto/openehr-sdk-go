package webtemplate

import (
	"slices"
	"testing"
)

// REQ-111 / REQ-106 — siblings that still share an id after naming take the
// Simplified Formats suffix: the first claimant keeps the bare id, and each
// later one takes the next free `<id>_N`, N from 1. "Free" is over the whole
// sibling list, wherever in document order an ordinary id ending `_N` sits,
// since renaming onto it would recreate the collision. The reference writes
// `<id>2`, `<id>3`; that spelling is the recorded deviation, so here it is an
// ordinary id like any other.
func TestDedupeSiblingIDs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "no collision untouched", in: []string{"a", "b"}, want: []string{"a", "b"}},
		// The upstream FLAT conformance template's two ELEMENTs: the
		// reference keys the second dv_text2.
		{name: "upstream two-claimant case", in: []string{"dv_text", "dv_text"}, want: []string{"dv_text", "dv_text_1"}},
		{name: "three claimants", in: []string{"x", "x", "x"}, want: []string{"x", "x_1", "x_2"}},
		{name: "ordinary suffix after the claimants", in: []string{"x", "x", "x_1"}, want: []string{"x", "x_2", "x_1"}},
		{name: "ordinary suffix before the claimants", in: []string{"x_1", "x", "x"}, want: []string{"x_1", "x", "x_2"}},
		{name: "ordinary suffix between the claimants", in: []string{"x", "x_1", "x"}, want: []string{"x", "x_1", "x_2"}},
		{name: "rename dodges a later original", in: []string{"x", "x", "x", "x_2"}, want: []string{"x", "x_1", "x_3", "x_2"}},
		{name: "claimants of a suffixed id", in: []string{"x_1", "x_1"}, want: []string{"x_1", "x_1_1"}},
		{name: "the reference's spelling is an ordinary id", in: []string{"x", "x2", "x"}, want: []string{"x", "x2", "x_1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := &Node{ID: "root"}
			for _, id := range tc.in {
				parent.Children = append(parent.Children, &Node{ID: id})
			}
			if got := dedupedIDs(parent); !slices.Equal(got, tc.want) {
				t.Errorf("dedupeSiblingIDs(%v) = %v, want %v", tc.in, got, tc.want)
			}
			if err := checkIDCollisions(parent); err != nil {
				t.Errorf("checkIDCollisions after dedupe of %v: %v", tc.in, err)
			}
		})
	}
}

// REQ-106 — the format's own id table: "Blood Pressure" is blood_pressure and
// its duplicate blood_pressure_1. A sibling named "Blood Pressure 1"
// sanitises to blood_pressure_1 on its own and keeps it, so the duplicate
// takes blood_pressure_2, whichever of the two comes first.
func TestREQ106DuplicateSiblingIDsFormatTable(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{
			name:  "duplicate",
			names: []string{"Blood Pressure", "Blood Pressure"},
			want:  []string{"blood_pressure", "blood_pressure_1"},
		},
		{
			name:  "named sibling first",
			names: []string{"Blood Pressure 1", "Blood Pressure", "Blood Pressure"},
			want:  []string{"blood_pressure_1", "blood_pressure", "blood_pressure_2"},
		},
		{
			name:  "named sibling last",
			names: []string{"Blood Pressure", "Blood Pressure", "Blood Pressure 1"},
			want:  []string{"blood_pressure", "blood_pressure_2", "blood_pressure_1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := &Node{ID: "root"}
			for _, name := range tc.names {
				parent.Children = append(parent.Children, &Node{ID: sanitizeID(name)})
			}
			if got := dedupedIDs(parent); !slices.Equal(got, tc.want) {
				t.Errorf("ids of siblings named %q = %v, want %v", tc.names, got, tc.want)
			}
		})
	}
}

// dedupedIDs runs dedupeSiblingIDs on parent and returns its children's ids.
func dedupedIDs(parent *Node) []string {
	dedupeSiblingIDs(parent)
	ids := make([]string, len(parent.Children))
	for i, ch := range parent.Children {
		ids[i] = ch.ID
	}
	return ids
}
