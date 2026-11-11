package simplified

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// REQ-053 — the reference writes the k-th of siblings sharing an id `<id>k`,
// k from 2, where the Web Template writes `<id>_<k-1>`. An id is read that way
// only where the parent has the bare id and the sibling it stands for, and a
// number with a leading zero, or below 2, is no such spelling.
func TestREQ053EarlierSpelledSibling(t *testing.T) {
	tests := []struct {
		name     string
		children []string
		id       string
		want     string // "" for no sibling
	}{
		{name: "second claimant", children: []string{"x", "x_1"}, id: "x2", want: "x_1"},
		{name: "third claimant", children: []string{"x", "x_1", "x_2"}, id: "x3", want: "x_2"},
		{name: "no bare id", children: []string{"x_1"}, id: "x2"},
		{name: "no sibling it stands for", children: []string{"x", "x_1"}, id: "x3"},
		{name: "number one", children: []string{"x", "x_0"}, id: "x1"},
		{name: "leading zero", children: []string{"x", "x_1", "x0", "x0_1"}, id: "x02", want: "x0_1"},
		{name: "leading zero alone", children: []string{"x", "x_1"}, id: "x02"},
		{name: "base ending in a digit", children: []string{"x1", "x1_1"}, id: "x12", want: "x1_1"},
		{name: "shortest base first", children: []string{"x", "x_11", "x1", "x1_1"}, id: "x12", want: "x_11"},
		{name: "a number with no base", children: []string{"x", "x_1"}, id: "2"},
		{name: "no digits", children: []string{"x", "x_1"}, id: "x"},
		{name: "number past int", children: []string{"x", "x_1"}, id: "x99999999999999999999"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := &webtemplate.Node{ID: "root"}
			for _, id := range tc.children {
				node.Children = append(node.Children, &webtemplate.Node{ID: id})
			}
			got := ""
			if ch := earlierSpelledSibling(node, tc.id); ch != nil {
				got = ch.ID
			}
			if got != tc.want {
				t.Errorf("earlierSpelledSibling(children %v, %q) = %q, want %q", tc.children, tc.id, got, tc.want)
			}
		})
	}
}

// REQ-053 — currentSpelling renames each segment of a key that spells a
// duplicate sibling the earlier way while the key walks the Web Template, and
// keeps the rest of the key as written: an own child wins, a segment past the
// template's nodes is no sibling, and a key the walk cannot start is kept.
func TestREQ053CurrentSpelling(t *testing.T) {
	leaf := func(id string) *webtemplate.Node { return &webtemplate.Node{ID: id} }
	node := func(id string, children ...*webtemplate.Node) *webtemplate.Node {
		return &webtemplate.Node{ID: id, Children: children}
	}
	root := node("root",
		node("a", node("t"), leaf("t_1")),
		node("a_1", leaf("t"), leaf("t_1")),
		node("b", leaf("v")), node("b_1", leaf("v")), node("b2", leaf("w")),
	)
	tests := []struct {
		key, want string // want "" when the key is kept
	}{
		{key: "root/a2/t2|code", want: "root/a_1/t_1|code"},
		{key: "root/a2:0/t2:1|code", want: "root/a_1:0/t_1:1|code"},
		{key: "root/a:0/t2", want: "root/a:0/t_1"},
		{key: "root/a2/t", want: "root/a_1/t"},
		{key: "root/a_1/t2/_uid", want: "root/a_1/t_1/_uid"},
		{key: "root/b2/w"},
		{key: "root/b2/v"},
		{key: "root/a/t/t2"},
		{key: "root/a/_link:0/t2"},
		{key: "root/a/t"},
		{key: "other/a2/t"},
		{key: "root/a2:01/t"},
		{key: "root/a2/t:01"},
		{key: "root"},
	}
	for _, tc := range tests {
		got, renamed := currentSpelling(root, tc.key)
		if want := tc.want != ""; renamed != want || got != tc.want {
			t.Errorf("currentSpelling(%q) = %q, %v; want %q, %v", tc.key, got, renamed, tc.want, want)
		}
	}
}
