// Example: read an ADL 1.4 operational template (OPT), compile it, and look
// around inside it: identity, provenance metadata, the root node with its
// attributes, and path lookup. It reads the OPT with
// aom14.ReadOperationalTemplate, which refuses a node type it does not know,
// compiles it with templatecompile.Compile, and looks paths up with
// template.ParsePath, Compiled.ValidatePath and Compiled.Resolve, with and
// without templatecompile.WithStrictPaths.
//
// Runs offline. With no argument it uses the vendored vital_signs.opt fixture:
//
//	go run ./cmd/examples/opt-parse
//	go run ./cmd/examples/opt-parse path/to/template.opt
package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

// run reads, compiles and looks around the OPT that args names first, or the
// vital_signs fixture when args is empty.
func run(args []string) error {
	optPath := fixtures.TemplateOptForName("vital_signs")
	if len(args) > 0 {
		optPath = args[0]
	}

	// Step 1: read the OPT into the operational template model.
	// ReadOperationalTemplate refuses a node type it does not know rather
	// than drop everything beneath it.
	raw, err := os.ReadFile(optPath)
	if err != nil {
		return fmt.Errorf("read OPT %s: %w", optPath, err)
	}
	opt, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("read OPT %s: %w", optPath, err)
	}

	// Step 2: identity and provenance, read from the model.
	printIdentity(opt)
	printProvenance(opt)

	// Step 3: compile it. The compiled template is the tree the SDK walks:
	// every node with its attributes, and the attributes the Reference
	// Model adds where the OPT leaves them out.
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}

	// Step 4: the root node and its attributes.
	printRoot(compiled.Root())

	// Step 5: look a path up in the compiled template: /content, or, where
	// the root has nothing under content, its first attribute with a child.
	path, ok := pathToLookUp(compiled.Root())
	if !ok {
		fmt.Println("lookup      : no attribute of the root has a child to look up")
		return nil
	}
	if path != contentPath {
		fmt.Printf("lookup      : the root has nothing under %s; looking up %s\n", contentPath, path)
	}
	return lookUp(compiled, path)
}

// contentPath is the path of the attribute that holds a COMPOSITION's clinical
// entries.
const contentPath = "/content"

// pathToLookUp returns the path the program looks up: /content when the root
// has a child under it, else the first attribute of the root that has a
// child, such as a demographic template's name. ok is false when no
// attribute of the root has a child.
func pathToLookUp(root *templatecompile.CompiledNode) (path string, ok bool) {
	if attr := root.Attribute("content"); attr != nil && len(attr.Children()) > 0 {
		return contentPath, true
	}
	for _, attr := range root.Attributes() {
		if len(attr.Children()) > 0 {
			return "/" + attr.Name(), true
		}
	}
	return "", false
}

// printIdentity prints the fields that name a template. UID and language are
// optional in the OPT format, so they appear only when present.
func printIdentity(opt *aom14.OperationalTemplate) {
	fmt.Printf("template_id : %s\n", opt.TemplateID.Value)
	fmt.Printf("concept     : %s\n", opt.Concept)
	if opt.UID != nil && opt.UID.Value != "" {
		fmt.Printf("uid         : %s\n", opt.UID.Value)
	}
	if lang := opt.Language.CodeString; lang != "" {
		fmt.Printf("language    : %s\n", lang)
	}
}

// printProvenance prints what the OPT's <description> and <annotations>
// blocks carry: the template's lifecycle state, who authored it, and any
// per-path annotations. Both blocks are optional; Description is nil and
// Annotations has no entries when the template omits them.
func printProvenance(opt *aom14.OperationalTemplate) {
	if desc := opt.Description; desc != nil {
		if desc.LifecycleState != "" {
			fmt.Printf("lifecycle   : %s\n", desc.LifecycleState)
		}
		if len(desc.OriginalAuthor) > 0 {
			fmt.Printf("authors     : %v\n", desc.OriginalAuthor)
		}
	}
	if len(opt.Annotations) > 0 {
		fmt.Printf("annotations : %d path(s)\n", len(opt.Annotations))
	}
}

// printRoot prints the root node's RM type, node id and archetype id, then
// the attributes the template declares on it, with their cardinality and
// child count. The compile also adds each attribute the Reference Model
// requires and the OPT leaves out; those are implicit and hold no child, so
// the program names them on one line.
func printRoot(root *templatecompile.CompiledNode) {
	fmt.Printf("root        : %s [%s]\n", root.RMTypeName(), root.NodeID())
	// A template's root is an archetype root, so it carries an archetype id.
	if id := root.ArchetypeID(); id != "" {
		fmt.Printf("archetype   : %s\n", id)
	}
	fmt.Println("attributes  :")
	var implicit []string
	for _, attr := range root.Attributes() {
		if attr.Implicit() {
			implicit = append(implicit, attr.Name())
			continue
		}
		fmt.Printf("  %s (%s, children=%d)\n", attr.Name(), attr.Cardinality(), len(attr.Children()))
	}
	if len(implicit) > 0 {
		fmt.Printf("implicit    : %s\n", strings.Join(implicit, ", "))
	}
}

// lookUp looks up path, an attribute of the root, then shows what strict
// lookup adds and the predicate that settles it.
func lookUp(compiled *templatecompile.Compiled, path string) error {
	// ParsePath checks the syntax once and needs no template, so a malformed
	// path fails before any lookup happens, and the same Path works on every
	// compiled template.
	parsed, err := template.ParsePath(path)
	if err != nil {
		return fmt.Errorf("parse path %s: %w", path, err)
	}

	// ValidatePath answers "does this path exist?" without returning the
	// node, which is all a precondition check needs.
	if err := compiled.ValidatePath(parsed); err != nil {
		return fmt.Errorf("validate path %s: %w", path, err)
	}

	// Resolve returns the node. The path has no predicate and /content has
	// several children in vital_signs.opt, so by default the lookup takes
	// the first one.
	node, err := compiled.Resolve(parsed)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	fmt.Printf("Resolve(%s): %s [%s]", path, node.RMTypeName(), node.NodeID())
	if id := node.ArchetypeID(); id != "" {
		fmt.Printf(" archetype=%s", id)
	}
	fmt.Println()

	// Strict lookup refuses to guess: the same lookup returns
	// ErrAmbiguousPath. Validators and code generators should prefer it. A
	// template with a single child under the path prints nothing more.
	switch _, err := compiled.Resolve(parsed, templatecompile.WithStrictPaths()); {
	case errors.Is(err, template.ErrAmbiguousPath):
		fmt.Printf("strict      : %s is ambiguous (multiple children)\n", path)
	case err != nil:
		return fmt.Errorf("resolve %s strictly: %w", path, err)
	default:
		return nil
	}

	// A predicate says which child the caller means: the archetype id of an
	// archetype root, otherwise the node's at-code. With it, strict lookup
	// finds the node.
	fixed := path + "[" + cmp.Or(node.ArchetypeID(), node.NodeID()) + "]"
	parsedFixed, err := template.ParsePath(fixed)
	if err != nil {
		return fmt.Errorf("parse path %s: %w", fixed, err)
	}
	strictNode, err := compiled.Resolve(parsedFixed, templatecompile.WithStrictPaths())
	if err != nil {
		return fmt.Errorf("resolve %s strictly: %w", fixed, err)
	}
	fmt.Printf("strict      : Resolve(%s): %s [%s]\n", fixed, strictNode.RMTypeName(), strictNode.NodeID())
	return nil
}
