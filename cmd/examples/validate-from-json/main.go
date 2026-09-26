// Validate a COMPOSITION that arrives as canonical JSON against an operational
// template (OPT), the way a CI check or an inbound gateway would: read the
// bytes, decode them into RM structs, compile the OPT, and list every
// constraint the document breaks. No HTTP is involved.
//
// By default it validates testdata/minimal_blood_pressure.json, a hand-made
// composition that passes against the vendored vital_signs.opt. With -cassette
// it validates the vendored vital_signs.json cassette instead, which is demo
// data and reports issues, so you can see what a failing run looks like. Two
// positional arguments validate your own files.
//
//	go run ./cmd/examples/validate-from-json
//	go run ./cmd/examples/validate-from-json -cassette
//	go run ./cmd/examples/validate-from-json composition.json template.opt
//
// The exit status is 1 when the composition does not validate.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	valid, err := run()
	if err != nil {
		log.Fatal(err)
	}
	if !valid {
		// Validation issues are a result, not a program failure: they were
		// printed above, and the exit status carries the outcome to scripts.
		os.Exit(1)
	}
}

// run validates the chosen composition and reports whether it passed. An
// error means the program could not do its job (bad path, unreadable OPT); a
// false result means the composition was checked and has issues.
func run() (valid bool, err error) {
	useCassette := flag.Bool("cassette", false, "validate testkit vital_signs.json, demo data that reports issues")
	flag.Parse()
	jsonPath, optPath, err := resolvePaths(*useCassette, flag.Args())
	if err != nil {
		return false, err
	}

	// Step 1: read the wire bytes and decode them. canjson reads the "_type"
	// discriminators and fills the typed rm structs; a document that is not
	// well-formed canonical JSON fails here, before any template is involved.
	body, err := os.ReadFile(jsonPath)
	if err != nil {
		return false, fmt.Errorf("read JSON %q: %w", jsonPath, err)
	}
	var composition rm.Composition
	if err := canjson.Unmarshal(body, &composition); err != nil {
		return false, fmt.Errorf("decode canonical JSON: %w", err)
	}
	fmt.Printf("json        : %s (%d bytes)\n", filepath.Base(jsonPath), len(body))
	fmt.Printf("composition : archetype_node_id=%s content_items=%d\n",
		composition.ArchetypeNodeID, len(composition.Content))

	// Step 2: parse and compile the OPT. An operational template is the
	// deployable form of an openEHR template: every archetype it uses,
	// flattened into one XML file with the template's constraints applied.
	// Compile turns the parsed XML into the driver that the validator (and
	// the composition builder, the instance generator, the AQL lint) walks.
	opt, err := template.ParseFile(optPath)
	if err != nil {
		return false, fmt.Errorf("parse OPT %q: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return false, fmt.Errorf("compile OPT %q: %w", optPath, err)
	}
	fmt.Printf("template    : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

	// Step 3: validate. The template drives the walk: for each node the OPT
	// declares, the validator reads the matching part of the composition and
	// checks existence, cardinality, RM type and primitive constraints. It
	// collects every issue in one pass instead of stopping at the first.
	result := validation.ValidateComposition(&composition, compiled)
	if result.OK {
		fmt.Println("result      : OK — JSON validates against OPT")
		return true, nil
	}
	fmt.Printf("result      : %d issue(s)\n", len(result.Issues))
	for _, issue := range result.Issues {
		// Path points at the offending node, Code is the stable identifier to
		// dispatch on, Detail is the explanation for a human.
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	if *useCassette {
		fmt.Println("note        : vital_signs.json is demo CDR data; issues are expected")
	}
	return false, nil
}

// resolvePaths picks the composition and the OPT to validate: the caller's
// two files, the demo cassette, or the clean default fixture next to this
// source file.
func resolvePaths(useCassette bool, args []string) (jsonPath, optPath string, err error) {
	switch len(args) {
	case 2:
		return args[0], args[1], nil
	case 0:
		// Both vendored inputs are checked against the same vital_signs.opt.
		optPath = fixtures.TemplateOptForName("vital_signs")
		if useCassette {
			return fixtures.CompositionJSON("vital_signs"), optPath, nil
		}
		jsonPath, err = defaultCompositionPath()
		return jsonPath, optPath, err
	default:
		return "", "", errors.New("usage: validate-from-json [-cassette] [composition.json template.opt]")
	}
}

// defaultCompositionPath locates testdata/minimal_blood_pressure.json
// relative to this source file, so `go run` works from any directory. The
// fixture was written once by gen_fixture.go.
func defaultCompositionPath() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the example's source directory")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "minimal_blood_pressure.json"), nil
}
