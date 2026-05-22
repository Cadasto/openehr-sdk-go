// Example: parse an ADL 1.4 operational template (OPT) and print
// identity + a path-resolved node. Demonstrates the building-block
// path for openehr/template/ (REQ-013, REQ-100) — no transport, no
// auth, no client. Just bytes → typed tree → path walk.
//
// Run:
//
//	go run ./cmd/examples/opt-parse [path/to/template.opt]
//
// With no argument, the example uses the vital_signs.opt fixture
// vendored under openehr/template/testdata/.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

func main() {
	path := resolveOPTPath()
	opt, err := template.ParseFile(path)
	if err != nil {
		log.Fatalf("ParseFile %q: %v", path, err)
	}

	fmt.Printf("template_id : %s\n", opt.TemplateID())
	fmt.Printf("concept     : %s\n", opt.Concept())
	if uid := opt.UID(); uid != "" {
		fmt.Printf("uid         : %s\n", uid)
	}
	if lang := opt.Language(); lang != "" {
		fmt.Printf("language    : %s\n", lang)
	}

	root := opt.Root()
	fmt.Printf("root        : %s [%s]\n", root.RMTypeName(), root.NodeID())

	var attrs []*template.Attribute
	switch r := root.(type) {
	case *template.ArchetypeRoot:
		fmt.Printf("archetype   : %s\n", r.ArchetypeID())
		attrs = r.Attributes()
	case *template.ComplexObject:
		attrs = r.Attributes()
	default:
		log.Fatalf("unsupported root node type: %T", root)
	}
	fmt.Println("attributes  :")
	for _, a := range attrs {
		card := "single"
		if a.Cardinality() == template.Multiple {
			card = "multiple"
		}
		fmt.Printf("  %s (%s, children=%d)\n", a.Name(), card, len(a.Children()))
	}

	// Demonstrate path resolution. /content is COMPOSITION-shaped;
	// the example assumes a composition-rooted OPT and fails loud
	// on a different shape so misuse surfaces.
	p, err := opt.ParsePath("/content")
	if err != nil {
		log.Fatalf("ParsePath(/content): %v", err)
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
