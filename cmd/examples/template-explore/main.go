// Example: walk a compiled operational template (OPT) through its public
// introspection tree and print two views of it. The first is the node
// structure a form generator would render: RM type, archetype id or at-code,
// attribute cardinality, whether the Reference Model requires the attribute,
// the human label, and slot and primitive markers. The second is the list of
// primitive-leaf paths a composition builder can assign values to. Nothing
// here imports an internal/ package.
//
// Runs offline. With no argument it uses the vendored vital_signs.opt fixture:
//
//	go run ./cmd/examples/template-explore
//	go run ./cmd/examples/template-explore path/to/template.opt
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
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := os.Args[1:]; len(args) > 0 {
		optPath = args[0]
	}

	// Step 1: parse and compile. The compiled tree is what the composition
	// builder and the validator work from; it is public so your own tooling
	// can walk it too.
	opt, err := template.ParseFile(optPath)
	if err != nil {
		return fmt.Errorf("parse OPT %s: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}
	fmt.Printf("template : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))
	fmt.Printf("root     : %s\n\n", compiled.Root().RMTypeName())

	// Step 2: the structure, one node per line with its attributes indented
	// under it and their child nodes under those.
	fmt.Println("structure (node → attribute → child node):")
	printNode(compiled.Root(), 0)

	// Step 3: the leaves a builder can set. Each path is the argument that
	// composition.Builder.Set (or SetText / SetQuantity / SetCodedText) takes;
	// the compile-build-validate example uses the second one below.
	paths := leafPaths(compiled.Root())
	fmt.Printf("\naddressable primitive-leaf paths (%d) — Builder.Set targets:\n", len(paths))
	for _, path := range paths {
		fmt.Printf("  %s\n", path)
	}
	return nil
}

// printNode prints one node and recurses through its attributes. A
// CompiledNode is an RM object the template constrains (COMPOSITION,
// OBSERVATION, ELEMENT, DV_QUANTITY, ...); each CompiledAttribute under it is
// one RM attribute (content, data, value, ...) together with the child nodes
// allowed there. In the output, [1] marks a single-valued attribute and [*] a
// multi-valued one; "required" means the Reference Model makes the attribute
// mandatory on that type, so a composition must carry a value there.
func printNode(node *templatecompile.CompiledNode, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Printf("%s%s%s%s\n", indent, node.RMTypeName(), pinnedID(node), nodeMarker(node))
	for _, attr := range node.Attributes() {
		cardinality := "1"
		if attr.Cardinality() == template.Multiple {
			cardinality = "*"
		}
		required := ""
		if attr.Required() {
			required = " required"
		}
		fmt.Printf("%s  .%s [%s]%s\n", indent, attr.Name(), cardinality, required)
		for _, child := range attr.Children() {
			printNode(child, depth+2)
		}
	}
}

// pinnedID renders the identity the template pins on a node: the archetype
// id where an archetype is plugged in (an archetype root), otherwise the
// at-code from the archetype's own definition, otherwise nothing (an
// unconstrained data value such as DV_QUANTITY has neither).
func pinnedID(node *templatecompile.CompiledNode) string {
	if archetypeID := node.ArchetypeID(); archetypeID != "" {
		return " [" + archetypeID + "]"
	}
	if nodeID := node.NodeID(); nodeID != "" {
		return " [" + nodeID + "]"
	}
	return ""
}

// nodeMarker annotates a node with what a form generator would key on: a
// slot is an opaque fill point another archetype plugs into, and a primitive
// node carries a value constraint (the editable leaf). When the archetype
// defines a term for the node's at-code, its text is the human label.
func nodeMarker(node *templatecompile.CompiledNode) string {
	var marker strings.Builder
	switch {
	case node.IsSlot():
		marker.WriteString("  (slot)")
	case node.PrimitiveConstraint() != nil:
		marker.WriteString("  ·primitive")
	}
	if nodeID := node.NodeID(); nodeID != "" {
		// Term looks the at-code up in the enclosing archetype's term
		// definitions. The language argument is reserved; an ADL 1.4 OPT
		// carries one language, so the empty string means that one.
		if term, ok := node.Term(nodeID, ""); ok {
			if text := term.Items["text"]; text != "" {
				marker.WriteString("  \"")
				marker.WriteString(text)
				marker.WriteString("\"")
			}
		}
	}
	return marker.String()
}

// leafPaths gathers the canonical path of every node that carries a primitive
// value constraint, in tree order. These are the leaves a composition builder
// fills; everything above them is structure the builder creates by itself.
func leafPaths(node *templatecompile.CompiledNode) []string {
	var paths []string
	if node.PrimitiveConstraint() != nil {
		paths = append(paths, node.AQLPath())
	}
	for _, attr := range node.Attributes() {
		for _, child := range attr.Children() {
			paths = append(paths, leafPaths(child)...)
		}
	}
	return paths
}
