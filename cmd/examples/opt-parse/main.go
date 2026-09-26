// Example: parse an ADL 1.4 operational template (OPT) and look around inside
// it: identity, provenance metadata, the root node with its attributes, and
// path resolution. It shows both parse modes (ParseFileStrict here, with the
// lenient ParseFile as the alternative), the ObjectNode interface a tree
// walker dispatches on, and ParsePath, NodeAt, ValidatePath and
// WithStrictPaths. Nothing here needs a network or a compiled template.
//
// Runs offline. With no argument it uses the vendored vital_signs.opt fixture:
//
//	go run ./cmd/examples/opt-parse
//	go run ./cmd/examples/opt-parse path/to/template.opt
package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
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

	// Step 1: parse. Strict mode rejects an unknown node type that has
	// attributes under it; the lenient ParseFile keeps such a node as a leaf
	// and silently drops everything beneath it.
	opt, err := template.ParseFileStrict(optPath)
	if err != nil {
		return fmt.Errorf("parse OPT %s: %w", optPath, err)
	}

	// Step 2: identity and provenance.
	printIdentity(opt)
	printProvenance(opt)

	// Step 3: the root node and its attributes.
	if err := printRoot(opt); err != nil {
		return err
	}

	// Step 4: resolve a path into the tree.
	return resolveContent(opt)
}

// printIdentity prints the fields that name a template. UID and language are
// optional in the OPT format, so they appear only when present.
func printIdentity(opt *template.OperationalTemplate) {
	fmt.Printf("template_id : %s\n", opt.TemplateID())
	fmt.Printf("concept     : %s\n", opt.Concept())
	if uid := opt.UID(); uid != "" {
		fmt.Printf("uid         : %s\n", uid)
	}
	if lang := opt.Language(); lang != "" {
		fmt.Printf("language    : %s\n", lang)
	}
}

// printProvenance prints what the OPT's <description> and <annotations>
// blocks carry: the template's lifecycle state, who authored it, and any
// per-path annotations. Both blocks are optional; Description returns nil and
// Annotations has no entries when the template omits them.
func printProvenance(opt *template.OperationalTemplate) {
	if desc := opt.Description(); desc != nil {
		if state := desc.LifecycleState(); state != "" {
			fmt.Printf("lifecycle   : %s\n", state)
		}
		if authors := desc.OriginalAuthors(); len(authors) > 0 {
			fmt.Printf("authors     : %v\n", authors)
		}
	}
	if annotations := opt.Annotations(); len(annotations) > 0 {
		fmt.Printf("annotations : %d path(s)\n", len(annotations))
	}
}

// printRoot prints the root node's RM type, node id and archetype id, then
// its declared attributes with their cardinality and child count.
func printRoot(opt *template.OperationalTemplate) error {
	root := opt.Root()
	fmt.Printf("root        : %s [%s]\n", root.RMTypeName(), root.NodeID())

	// Root returns the tree's Node interface. ObjectNode is the supertype of
	// the two node kinds a walker can descend into (*ArchetypeRoot and
	// *ComplexObject), so asserting it gives access to the attributes without
	// caring which of the two the root is.
	object, ok := root.(template.ObjectNode)
	if !ok {
		return fmt.Errorf("root node is %T, want a template.ObjectNode", root)
	}

	// Only an archetype root carries an archetype id, and a template's root
	// always is one.
	if archetypeRoot, ok := root.(*template.ArchetypeRoot); ok {
		fmt.Printf("archetype   : %s\n", archetypeRoot.ArchetypeID())
	}
	fmt.Println("attributes  :")
	for _, attr := range object.Attributes() {
		fmt.Printf("  %s (%s, children=%d)\n", attr.Name(), attr.Cardinality(), len(attr.Children()))
	}
	return nil
}

// resolveContent resolves the path /content, the attribute that holds a
// COMPOSITION's clinical entries, and then shows what strict resolution adds.
func resolveContent(opt *template.OperationalTemplate) error {
	const path = "/content"

	// ParsePath checks the syntax once, so a malformed path fails before any
	// lookup happens.
	parsed, err := opt.ParsePath(path)
	if err != nil {
		return fmt.Errorf("parse path %s: %w", path, err)
	}

	// ValidatePath answers "does this path exist?" without returning the
	// node, which is all a precondition check needs.
	if err := opt.ValidatePath(parsed); err != nil {
		return fmt.Errorf("validate path %s: %w", path, err)
	}

	// NodeAt returns the node. The path has no predicate and /content has
	// several children in vital_signs.opt, so the default (lenient) mode
	// picks the first one.
	node, err := opt.NodeAt(parsed)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	fmt.Printf("NodeAt(%s): %s [%s]", path, node.RMTypeName(), node.NodeID())
	if archetypeRoot, ok := node.(*template.ArchetypeRoot); ok {
		fmt.Printf(" archetype=%s", archetypeRoot.ArchetypeID())
	}
	fmt.Println()

	// Strict mode refuses to guess: the same lookup returns ErrAmbiguousPath,
	// and the caller adds a predicate such as
	// /content[openEHR-EHR-OBSERVATION.blood_pressure.v1] to say which child
	// it means. Validators and code generators should prefer this mode. A
	// template with a single child under /content prints nothing here.
	switch _, err := opt.NodeAt(parsed, template.WithStrictPaths()); {
	case errors.Is(err, template.ErrAmbiguousPath):
		fmt.Println("strict       : /content is ambiguous (multiple children) — add an [archetype-id] or [at-code] predicate")
	case err != nil:
		return fmt.Errorf("resolve %s strictly: %w", path, err)
	}
	return nil
}
