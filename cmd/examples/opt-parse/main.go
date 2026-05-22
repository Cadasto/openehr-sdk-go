// Example: parse an ADL 1.4 operational template (OPT) and print
// identity, OPT metadata, and path-resolved nodes. Demonstrates the
// building-block path for openehr/template/ (REQ-013, REQ-100 + the
// follow-up Phases 1–3 surface) — no transport, no auth, no client.
// Just bytes → typed tree → path walk.
//
// Surfaces shown:
//   - ParseFile / ParseFileStrict (default and strict parse modes)
//   - OperationalTemplate identity (TemplateID / Concept / UID / Language)
//   - Description() / Annotations() — OPT provenance metadata
//   - ObjectNode supertype for walker dispatch
//   - ParsePath / NodeAt / ValidatePath
//   - WithStrictPaths for ambiguity surfacing (ErrAmbiguousPath)
//
// Run:
//
//	go run ./cmd/examples/opt-parse [path/to/template.opt]
//
// With no argument, the example uses the vital_signs.opt fixture
// vendored under openehr/template/testdata/.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

func main() {
	path := resolveOPTPath()
	// Strict parse: rejects unknown <children> xsi:type with nested
	// <attributes>. Use ParseFile (lenient) when forward-compat is
	// preferred over strictness.
	opt, err := template.ParseFileStrict(path)
	if err != nil {
		log.Fatalf("ParseFileStrict %q: %v", path, err)
	}

	fmt.Printf("template_id : %s\n", opt.TemplateID())
	fmt.Printf("concept     : %s\n", opt.Concept())
	if uid := opt.UID(); uid != "" {
		fmt.Printf("uid         : %s\n", uid)
	}
	if lang := opt.Language(); lang != "" {
		fmt.Printf("language    : %s\n", lang)
	}

	// OPT provenance metadata (Phase 2): <description> + <annotations>
	// blocks. Both are nil/empty when the OPT omits them.
	if d := opt.Description(); d != nil {
		if ls := d.LifecycleState(); ls != "" {
			fmt.Printf("lifecycle   : %s\n", ls)
		}
		if authors := d.OriginalAuthors(); len(authors) > 0 {
			fmt.Printf("authors     : %v\n", authors)
		}
	}
	if ann := opt.Annotations(); len(ann) > 0 {
		fmt.Printf("annotations : %d path(s)\n", len(ann))
	}

	root := opt.Root()
	fmt.Printf("root        : %s [%s]\n", root.RMTypeName(), root.NodeID())

	// ObjectNode (Phase 3) supertypes *ArchetypeRoot + *ComplexObject —
	// walker code that doesn't need to discriminate them lists one
	// interface instead of two concrete types.
	on, ok := root.(template.ObjectNode)
	if !ok {
		log.Fatalf("root is not an ObjectNode: %T", root)
	}
	if ar, ok := root.(*template.ArchetypeRoot); ok {
		fmt.Printf("archetype   : %s\n", ar.ArchetypeID())
	}
	fmt.Println("attributes  :")
	for _, a := range on.Attributes() {
		fmt.Printf("  %s (%s, children=%d)\n", a.Name(), a.Cardinality(), len(a.Children()))
	}

	// Path resolution. /content is the COMPOSITION container in
	// vital_signs.opt; the example fails loud on a different shape so
	// misuse surfaces.
	p, err := opt.ParsePath("/content")
	if err != nil {
		log.Fatalf("ParsePath(/content): %v", err)
	}

	// ValidatePath (Phase 3) is a shorthand for NodeAt that discards
	// the resolved node — useful for code-generator preconditions.
	if err := opt.ValidatePath(p); err != nil {
		log.Fatalf("ValidatePath(/content): %v", err)
	}

	n, err := opt.NodeAt(p)
	if err != nil {
		log.Fatalf("NodeAt(/content): %v", err)
	}
	fmt.Printf("NodeAt(/content): %s [%s]", n.RMTypeName(), n.NodeID())
	if ar, ok := n.(*template.ArchetypeRoot); ok {
		fmt.Printf(" archetype=%s", ar.ArchetypeID())
	}
	fmt.Println()

	// Strict-mode resolution (Phase 3): a predicate-less /content with
	// multiple candidate children would silently first-match in lenient
	// mode but raise ErrAmbiguousPath under WithStrictPaths. Demonstrate
	// the difference if it applies to this template.
	if _, err := opt.NodeAt(p, template.WithStrictPaths()); errors.Is(err, template.ErrAmbiguousPath) {
		fmt.Println("strict       : /content is ambiguous (multiple children) — add an [archetype-id] or [at-code] predicate")
	}
}

func resolveOPTPath() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	// Default: locate the vendored vital_signs.opt fixture relative
	// to this source file, so `go run ./cmd/examples/opt-parse` works
	// from any working directory.
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot locate example source path")
	}
	repoRoot := filepath.Join(filepath.Dir(here), "..", "..", "..")
	return filepath.Join(repoRoot, "openehr", "template", "testdata", "vital_signs.opt")
}
