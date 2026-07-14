// Example: export a compiled operational template as EHRbase
// openEHR_SDK v2.3 WebTemplate JSON (REQ-106) — the lossy, UI-oriented
// projection a form renderer or FLAT-path mapper consumes.
//
// Like cmd/examples/template-explore, this uses PUBLIC packages only
// (openehr/template, openehr/templatecompile, openehr/template/webtemplate)
// — no internal/ import. It prints the form-oriented tree view (node id,
// RM type, occurrences, inputs) that consumers bind FLAT paths to, then
// the deterministic JSON document itself.
//
// Run:
//
//	go run ./cmd/examples/webtemplate-export
//	go run ./cmd/examples/webtemplate-export path/to/template.opt
//	go run ./cmd/examples/webtemplate-export -json path/to/template.opt
//
// With no argument it uses the vendored vital_signs.opt fixture; -json
// dumps the full indented WebTemplate document instead of the summary.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	dumpJSON := flag.Bool("json", false, "print the full indented WebTemplate JSON document")
	flag.Parse()

	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := flag.Args(); len(args) > 0 {
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

	// Build once for the typed tree; encode with the same json.Marshal path
	// webtemplate.Marshal uses (deterministic struct field order).
	wt, err := webtemplate.Build(c)
	if err != nil {
		log.Fatalf("Build: %v", err)
	}
	data, err := json.Marshal(wt)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}

	if *dumpJSON {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, data, "", "  "); err != nil {
			log.Fatalf("indent: %v", err)
		}
		pretty.WriteByte('\n')
		fmt.Print(pretty.String())
		return
	}

	fmt.Printf("template : %s (%s)\n", wt.TemplateID, filepath.Base(optPath))
	fmt.Printf("version  : %s   defaultLanguage: %s\n", wt.Version, wt.DefaultLanguage)
	fmt.Printf("document : %d bytes deterministic JSON (application/openehr.wt+json)\n\n", len(data))

	fmt.Println("form tree (id [rmType] occurrences — inputs):")
	printNode(wt.Tree, 0)

	fmt.Fprintln(os.Stderr, "\nrerun with -json for the full document")
}

// printNode renders one WebTemplate node the way a form renderer reads
// it: the FLAT-path id, the RM type, min..max occurrences, and the input
// widgets (suffix:type) a data-entry client must draw for the leaf.
func printNode(n *webtemplate.Node, depth int) {
	occ := fmt.Sprintf("%d..%d", n.Min, n.Max)
	if n.Max == -1 {
		occ = fmt.Sprintf("%d..*", n.Min)
	}
	line := fmt.Sprintf("%s%s [%s] %s", strings.Repeat("  ", depth), n.ID, n.RMType, occ)
	if sig := inputSig(n.Inputs); sig != "" {
		line += " — " + sig
	}
	fmt.Println(line)
	for _, ch := range n.Children {
		printNode(ch, depth+1)
	}
}

// inputSig summarises a leaf's inputs as "suffix:type" pairs — the same
// signature PROBE-075 pins against the EHRbase reference.
func inputSig(inputs []webtemplate.Input) string {
	parts := make([]string, 0, len(inputs))
	for _, in := range inputs {
		p := in.Type
		if in.Suffix != "" {
			p = in.Suffix + ":" + in.Type
		}
		if len(in.List) > 0 {
			p += fmt.Sprintf("(%d codes)", len(in.List))
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}
