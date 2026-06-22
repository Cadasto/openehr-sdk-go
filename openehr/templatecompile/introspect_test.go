package templatecompile_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// fieldSpec holds the introspection types the way a downstream form
// generator or mapper would. Its existence proves CompiledNode /
// CompiledAttribute are nameable in external struct fields — not merely
// reachable inline through Compiled's methods (which was already
// possible before they were exported).
type fieldSpec struct {
	node *templatecompile.CompiledNode
	attr *templatecompile.CompiledAttribute
}

// collectPaths proves the node types are nameable in a function
// signature: a downstream package can take *templatecompile.CompiledNode
// and recurse through *templatecompile.CompiledAttribute.
func collectPaths(n *templatecompile.CompiledNode, out *[]string) {
	if n == nil {
		return
	}
	*out = append(*out, n.AQLPath())
	for _, attr := range n.Attributes() {
		for _, child := range attr.Children() {
			collectPaths(child, out)
		}
	}
}

// countNodes proves the index methods' []*CompiledNode return is nameable
// as a function parameter.
func countNodes(nodes []*templatecompile.CompiledNode) int { return len(nodes) }

func TestCompiledNode_ExternallyNavigable(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	root := c.Root()
	if root.RMTypeName() != "COMPOSITION" {
		t.Fatalf("root RM type = %q, want COMPOSITION", root.RMTypeName())
	}
	if at, err := c.NodeAt("/"); err != nil || at != root {
		t.Fatalf("NodeAt(\"/\") = (%v, %v), want the root node", at, err)
	}

	// Walk the tree through a function typed on the public node types.
	var paths []string
	collectPaths(root, &paths)
	if len(paths) < 5 {
		t.Fatalf("expected to navigate several nodes, walked %d", len(paths))
	}

	// Index lookups return a nameable typed slice.
	if countNodes(c.AllByRMType("OBSERVATION")) == 0 {
		t.Fatal("AllByRMType(\"OBSERVATION\") returned none; vital_signs has observation archetypes")
	}

	// Attribute introspection through nameable struct fields: COMPOSITION
	// .content is a Multiple attribute.
	attr := root.Attribute("content")
	if attr == nil {
		t.Fatal("COMPOSITION root has no \"content\" attribute")
	}
	spec := fieldSpec{node: root, attr: attr}
	if spec.node.RMTypeName() != "COMPOSITION" {
		t.Errorf("spec.node RM type = %q, want COMPOSITION", spec.node.RMTypeName())
	}
	if spec.attr.Cardinality() != template.Multiple {
		t.Errorf("COMPOSITION.content cardinality = %v, want Multiple", spec.attr.Cardinality())
	}
}
