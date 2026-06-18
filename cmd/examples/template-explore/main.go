// Example: introspect a compiled operational template through the
// public REQ-111 surface — walk the [templatecompile.CompiledNode] tree
// to print the template structure (the seed of a form generator) and
// the addressable primitive-leaf paths (the seed of path discovery /
// Builder.Set targets).
//
// Like cmd/examples/compile-build-validate, this uses PUBLIC packages
// only (openehr/template, openehr/templatecompile) — no internal/
// import. It exercises the node-level introspection types
// (CompiledNode / CompiledAttribute) that an external form generator or
// mapping layer would hold and navigate.
//
// Run:
//
//	go run ./cmd/examples/template-explore
//	go run ./cmd/examples/template-explore path/to/template.opt
//
// With no argument it uses the vendored vital_signs.opt fixture.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := os.Args[1:]; len(args) > 0 {
		optPath = args[0]
	}

	opt, err := template.ParseFile(optPath)
	if err != nil {
		log.Fatalf("ParseFile %q: %v", optPath, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("Compile: %v", err)
	}

	fmt.Printf("template : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))
	fmt.Printf("root     : %s\n\n", c.Root().RMTypeName())

	fmt.Println("structure (node → attribute → child node):")
	printNode(c.Root(), 0)

	var leaves []string
	collectLeafPaths(c.Root(), &leaves)
	fmt.Printf("\naddressable primitive-leaf paths (%d) — Builder.Set targets:\n", len(leaves))
	for _, p := range leaves {
		fmt.Printf("  %s\n", p)
	}
}

// printNode walks one CompiledNode: the node line (RM type, pinned id,
// slot/primitive marker, term label), then each CompiledAttribute
// (name, cardinality, required), recursing into its children.
func printNode(n *templatecompile.CompiledNode, depth int) {
	ind := strings.Repeat("  ", depth)
	fmt.Printf("%s%s%s%s\n", ind, n.RMTypeName(), pinnedID(n), nodeMarker(n))
	for _, attr := range n.Attributes() {
		card := "1"
		if attr.Cardinality() == template.Multiple {
			card = "*"
		}
		req := ""
		if attr.Required() {
			req = " required"
		}
		fmt.Printf("%s  .%s [%s]%s\n", ind, attr.Name(), card, req)
		for _, child := range attr.Children() {
			printNode(child, depth+2)
		}
	}
}

// pinnedID renders the OPT-pinned identity of a node: the archetype id
// for archetype roots, else the at-code, else nothing.
func pinnedID(n *templatecompile.CompiledNode) string {
	if a := n.ArchetypeID(); a != "" {
		return " [" + a + "]"
	}
	if id := n.NodeID(); id != "" {
		return " [" + id + "]"
	}
	return ""
}

// nodeMarker annotates slots, primitive leaves, and (where the OPT
// defines a term) the human-readable label.
func nodeMarker(n *templatecompile.CompiledNode) string {
	var b strings.Builder
	switch {
	case n.IsSlot():
		b.WriteString("  (slot)")
	case n.PrimitiveConstraint() != nil:
		b.WriteString("  ·primitive")
	}
	if id := n.NodeID(); id != "" {
		if t, ok := n.Term(id, ""); ok {
			if txt := t.Items["text"]; txt != "" {
				b.WriteString("  \"")
				b.WriteString(txt)
				b.WriteString("\"")
			}
		}
	}
	return b.String()
}

// collectLeafPaths gathers the canonical AQL paths of every
// primitive-constrained node — the leaves a composition builder fills
// via Set / SetText / SetQuantity. Demonstrates path discovery driven
// purely by the public introspection tree.
func collectLeafPaths(n *templatecompile.CompiledNode, out *[]string) {
	if n.PrimitiveConstraint() != nil {
		*out = append(*out, n.AQLPath())
	}
	for _, attr := range n.Attributes() {
		for _, child := range attr.Children() {
			collectLeafPaths(child, out)
		}
	}
}
