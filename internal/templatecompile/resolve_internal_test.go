package templatecompile

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// TestREQ108_ResolveRefusesAWalkPastTheDepthLimit pins the REQ-108 bound on
// Compiled.Resolve and Compiled.ValidatePath: a walk deeper than 128 levels
// is refused with an error wrapping template.ErrPathNotFound, however deep
// the tree goes. Compile never builds a tree that deep, so the tree here is
// built by hand, one CLUSTER whose items hold itself, and every path down
// it reaches a node until the bound stops the walk.
func TestREQ108_ResolveRefusesAWalkPastTheDepthLimit(t *testing.T) {
	n := &CompiledNode{aqlPath: "/", rmTypeName: "CLUSTER", nodeID: "at0001"}
	n.attributes = []*CompiledAttribute{{name: "items", cardinality: template.Multiple, children: []*CompiledNode{n}}}
	c := &Compiled{root: n}
	parse := func(levels int) template.Path {
		t.Helper()
		p, err := template.ParsePath(strings.Repeat("/items[at0001]", levels))
		if err != nil {
			t.Fatalf("ParsePath: %v", err)
		}
		return p
	}

	if got, err := c.Resolve(parse(maxDepth)); err != nil || got != n {
		t.Fatalf("Resolve of a path %d levels deep = %v, %v; want the CLUSTER", maxDepth, got, err)
	}
	for _, levels := range []int{maxDepth + 1, 4 * maxDepth} {
		got, err := c.Resolve(parse(levels))
		if got != nil || !errors.Is(err, template.ErrPathNotFound) || !errors.Is(err, ErrPathNotFound) {
			t.Errorf("Resolve of a path %d levels deep = %v, %v; want no node and an error matching both not-found sentinels", levels, got, err)
		}
		if err := c.ValidatePath(parse(levels), WithStrictPaths()); !errors.Is(err, template.ErrPathNotFound) {
			t.Errorf("ValidatePath of a path %d levels deep = %v, want an error wrapping template.ErrPathNotFound", levels, err)
		}
	}
}

// TestResolveSegmentsReadThePathText: the segments Resolve walks are those
// ParsePath read, a predicate that holds a slash among them.
func TestResolveSegmentsReadThePathText(t *testing.T) {
	cases := []struct {
		path string
		want []pathSegment
	}{
		{path: "/", want: nil},
		{path: "/content", want: []pathSegment{{name: "content"}}},
		{path: "/content[at0001]/items", want: []pathSegment{{name: "content", predicate: "at0001"}, {name: "items"}}},
		{path: "/a[x/y]/b[z]", want: []pathSegment{{name: "a", predicate: "x/y"}, {name: "b", predicate: "z"}}},
	}
	for _, tc := range cases {
		p, err := template.ParsePath(tc.path)
		if err != nil {
			t.Fatalf("ParsePath(%q): %v", tc.path, err)
		}
		if got := segmentsOf(p); !slices.Equal(got, tc.want) {
			t.Errorf("segmentsOf(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
