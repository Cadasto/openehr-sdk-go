package simplified

import (
	"errors"
	"fmt"
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

// REQ-053 — a refusal names a duplicate sibling as the body wrote it: respell
// puts each renamed id back, whole ids only, at any index spelling and as a
// lone id, and keeps the Web Template's id where the body spells the node both
// ways or the message names keys as written. The error still wraps its
// sentinel.
func TestREQ053RespellRefusal(t *testing.T) {
	leaf := func(id string) *webtemplate.Node { return &webtemplate.Node{ID: id} }
	node := func(id string, children ...*webtemplate.Node) *webtemplate.Node {
		return &webtemplate.Node{ID: id, Children: children}
	}
	wt := &webtemplate.WebTemplate{Tree: node("t",
		node("x", leaf("a")), node("x_1", leaf("a"), leaf("b"), leaf("y"), leaf("y_1")), node("x_10", leaf("b")),
	)}
	tests := []struct {
		name      string
		body      []string
		msg, want string
	}{
		{
			name: "a key", body: []string{"t/x2:0/a"},
			msg: `decode "t/x_1:0/a": refused`, want: `decode "t/x2:0/a": refused`,
		},
		{
			name: "a key with a suffix, unquoted", body: []string{"t/x2:0/a|code"},
			msg: `t/x_1:0/a|code does not fit`, want: `t/x2:0/a|code does not fit`,
		},
		{
			name: "whole ids only", body: []string{"t/x2:0/a", "t/x_10/b"},
			msg: `"t/x_10/b" and "t/x_1:0/a" and x_10`, want: `"t/x_10/b" and "t/x2:0/a" and x_10`,
		},
		{
			name: "any index spelling", body: []string{"t/x2:0/a"},
			msg: `at "t:0/x_1:0/a:0" and "t/x_1/a"`, want: `at "t:0/x2:0/a:0" and "t/x2/a"`,
		},
		{
			name: "a lone id", body: []string{"t/x2:1/a"},
			msg: `"t/x_1:1/a": FLAT id "x_1" carries :1`, want: `"t/x2:1/a": FLAT id "x2" carries :1`,
		},
		{
			name: "a node spelled both ways", body: []string{"t/x2:0/a", "t/x_1:1/b"},
			msg: `"t/x_1:1/b" and FLAT id "x_1"`, want: `"t/x_1:1/b" and FLAT id "x_1"`,
		},
		{
			// The key that spells y_1 the earlier way is read last.
			name: "a child spelled both ways", body: []string{"t/x_1:0/y_1", "t/x_1:1/y2"},
			msg: `"t/x_1:0/y_1" and FLAT id "y_1"`, want: `"t/x_1:0/y_1" and FLAT id "y_1"`,
		},
		{
			name: "keys as written", body: []string{"t/x_1:0/a", "t/x2:0/b"},
			msg: `"t/x_1:0/a" and "t/x2:0/b"`, want: `"t/x_1:0/a" and "t/x2:0/b"`,
		},
		{
			name: "a path not from the root", body: []string{"t/x2:0/a"},
			msg: `"x_1/a" at /content/x_1 and t|x_1`, want: `"x_1/a" at /content/x_1 and t|x_1`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := make(map[string]any, len(tc.body))
			for _, k := range tc.body {
				body[k] = "v"
			}
			_, fold := foldSiblingSpellings(body, wt, nil)
			err := fold.respell(fmt.Errorf("%w: %s", ErrUnknownPath, tc.msg))
			want := ErrUnknownPath.Error() + ": " + tc.want
			if err.Error() != want {
				t.Errorf("respell(%q) with body %v =\n%s\nwant\n%s", tc.msg, tc.body, err, want)
			}
			if !errors.Is(err, ErrUnknownPath) {
				t.Errorf("respell(%q) = %v, which no longer wraps ErrUnknownPath", tc.msg, err)
			}
		})
	}
	t.Run("nothing renamed", func(t *testing.T) {
		_, fold := foldSiblingSpellings(map[string]any{"t/x_1:0/a": "v"}, wt, nil)
		in := fmt.Errorf("%w: \"t/x_1:0/a\"", ErrUnknownPath)
		if err := fold.respell(in); err != in { //nolint:errorlint // the error itself, wrapped in nothing
			t.Errorf("respell(%v) = %v, want the error itself", in, err)
		}
		if err := fold.respell(nil); err != nil {
			t.Errorf("respell(nil) = %v, want nil", err)
		}
	})
}
