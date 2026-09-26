// Example: export a compiled operational template (OPT) as a Web Template,
// the JSON form that EHRbase-style form renderers and FLAT-format mappers
// consume. The program prints a summary and the form tree (each node's
// FLAT-path id, RM type, occurrences, and the input widgets a data-entry
// client draws), or with -json the whole indented document. Nothing here
// imports an internal/ package.
//
// Runs offline. With no path it uses the vendored vital_signs.opt fixture:
//
//	go run ./cmd/examples/webtemplate-export
//	go run ./cmd/examples/webtemplate-export path/to/template.opt
//	go run ./cmd/examples/webtemplate-export -json path/to/template.opt
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
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dumpJSON := flag.Bool("json", false, "print the full indented WebTemplate JSON document")
	flag.Parse()
	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := flag.Args(); len(args) > 0 {
		optPath = args[0]
	}

	// Step 1: parse and compile the OPT. The Web Template is derived from the
	// compiled form, so the template has to be compiled first.
	opt, err := template.ParseFile(optPath)
	if err != nil {
		return fmt.Errorf("parse OPT %s: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}

	// Step 2: project the compiled template into the Web Template tree. Build
	// returns the typed tree; webtemplate.Marshal would return the JSON bytes
	// in one call. This program needs both, so it builds once and encodes the
	// tree with the same encoding/json call Marshal uses, which keeps the byte
	// count equal to what Marshal would return.
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		return fmt.Errorf("build web template: %w", err)
	}
	document, err := json.Marshal(wt)
	if err != nil {
		return fmt.Errorf("encode web template: %w", err)
	}

	if *dumpJSON {
		return printIndented(document)
	}

	// Step 3: the summary and the form tree.
	fmt.Printf("template : %s (%s)\n", wt.TemplateID, filepath.Base(optPath))
	fmt.Printf("version  : %s   defaultLanguage: %s\n", wt.Version, wt.DefaultLanguage)
	fmt.Printf("document : %d bytes deterministic JSON (application/openehr.wt+json)\n\n", len(document))

	fmt.Println("form tree (id [rmType] occurrences — inputs):")
	printNode(wt.Tree, 0)

	// The hint goes to stderr so stdout stays the summary alone.
	fmt.Fprintln(os.Stderr, "\nrerun with -json for the full document")
	return nil
}

// printIndented re-indents the compact document so a person can read it.
func printIndented(document []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, document, "", "  "); err != nil {
		return fmt.Errorf("indent web template JSON: %w", err)
	}
	pretty.WriteByte('\n')
	fmt.Print(pretty.String())
	return nil
}

// printNode prints one Web Template node the way a form renderer reads it:
// the id (the segment a FLAT path is built from), the RM type, the min..max
// occurrences, and the inputs a data-entry client must draw for a leaf. A
// Max of -1 means unbounded, printed as "*".
func printNode(node *webtemplate.Node, depth int) {
	occurrences := fmt.Sprintf("%d..%d", node.Min, node.Max)
	if node.Max == -1 {
		occurrences = fmt.Sprintf("%d..*", node.Min)
	}
	line := fmt.Sprintf("%s%s [%s] %s", strings.Repeat("  ", depth), node.ID, node.RMType, occurrences)
	if inputs := describeInputs(node.Inputs); inputs != "" {
		line += " — " + inputs
	}
	fmt.Println(line)
	for _, child := range node.Children {
		printNode(child, depth+1)
	}
}

// describeInputs summarises a leaf's inputs as "suffix:type" pairs. The
// suffix is the FLAT-path suffix the input's value is posted under
// (|magnitude, |unit, |code), and a coded input also reports how many codes
// its list offers.
func describeInputs(inputs []webtemplate.Input) string {
	parts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		part := input.Type
		if input.Suffix != "" {
			part = input.Suffix + ":" + input.Type
		}
		if len(input.List) > 0 {
			part += fmt.Sprintf("(%d codes)", len(input.List))
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}
