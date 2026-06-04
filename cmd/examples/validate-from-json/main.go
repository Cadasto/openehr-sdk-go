// Example: decode canonical-JSON Composition bytes with canjson,
// compile vital_signs.opt, and validate (REQ-052 + REQ-102). Shows the
// wire-bytes → RM → validation path without HTTP.
//
// Default fixture testdata/minimal_blood_pressure.json is a
// single–blood-pressure composition that validates cleanly against
// vital_signs.opt (generated via gen_fixture.go). The vendored
// testkit/cassettes/compositions/vital_signs.json cassette does
// not validate cleanly against that OPT (demo data / constraint
// mismatches) — use -cassette to see that outcome.
//
// Run:
//
//	go run ./cmd/examples/validate-from-json
//	go run ./cmd/examples/validate-from-json -cassette
//	go run ./cmd/examples/validate-from-json composition.json template.opt
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	cassette := flag.Bool("cassette", false, "use testkit vital_signs.json (expected to report validation issues)")
	flag.Parse()
	jsonPath, optPath := resolvePaths(*cassette, flag.Args())

	body, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Fatalf("read JSON %q: %v", jsonPath, err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		log.Fatalf("canjson.Unmarshal: %v", err)
	}
	fmt.Printf("json        : %s (%d bytes)\n", filepath.Base(jsonPath), len(body))
	fmt.Printf("composition : archetype_node_id=%s content_items=%d\n",
		comp.ArchetypeNodeID, len(comp.Content))

	opt, err := template.ParseFile(optPath)
	if err != nil {
		log.Fatalf("parse OPT %q: %v", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("Compile %q: %v", optPath, err)
	}
	fmt.Printf("template    : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

	r := validation.ValidateComposition(&comp, compiled)
	if r.OK {
		fmt.Println("result      : OK — JSON validates against OPT")
		return
	}
	fmt.Printf("result      : %d issue(s)\n", len(r.Issues))
	for _, issue := range r.Issues {
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	if *cassette {
		fmt.Println("note        : vital_signs.json is demo CDR data; issues are expected")
	}
	os.Exit(1)
}

func resolvePaths(useCassette bool, args []string) (jsonPath, optPath string) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot locate example source path")
	}
	exampleDir := filepath.Dir(here)
	defaultJSON := filepath.Join(exampleDir, "testdata", "minimal_blood_pressure.json")
	defaultOPT := fixtures.TemplateOptForName("vital_signs")
	cassetteJSON := fixtures.CompositionJSON("vital_signs")

	switch len(args) {
	case 0:
		if useCassette {
			return cassetteJSON, defaultOPT
		}
		return defaultJSON, defaultOPT
	case 2:
		return args[0], args[1]
	default:
		log.Fatal("usage: validate-from-json [-cassette] [composition.json template.opt]")
	}
	return "", ""
}
