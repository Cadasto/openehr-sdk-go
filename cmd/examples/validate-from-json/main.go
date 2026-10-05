// Validate a COMPOSITION that arrives as canonical JSON, the way a CI check or
// an inbound gateway would: read the bytes, decode them into RM structs,
// compile an operational template (OPT), and run two validation passes, each
// listing every issue it finds. No HTTP is involved.
//
// The two passes check different things:
//
//   - RM floor (validation.ValidateRM): the openEHR Reference Model alone, with
//     no template. It checks the attributes the RM makes mandatory on every
//     node, and the RM's own rules for each type, such as an ELEMENT carrying
//     exactly one of a value or a null flavour, and an archetype root
//     carrying archetype_details.
//   - Template constraints (validation.ValidateComposition): what the OPT
//     declares, node by node: existence, cardinality, RM type, archetype
//     identity and value constraints.
//
// The two passes compose but do not chain. A composition that satisfies its
// template can still break the Reference Model: today ValidateComposition
// checks the template's constraints and does not run the RM floor's per-type
// rules, so a program that wants both guarantees calls both.
//
// By default it validates testdata/minimal_blood_pressure.json, a hand-made
// composition that passes both against the vendored vital_signs.opt. With
// -corpus it validates the vendored testkit/corpus vital_signs.json instead,
// which is demo data and reports issues, so you can see what a failing run
// looks like. Two positional arguments validate your own files.
//
//	go run ./cmd/examples/validate-from-json
//	go run ./cmd/examples/validate-from-json -corpus
//	go run ./cmd/examples/validate-from-json composition.json template.opt
//
// The exit status is 0 when both passes find no issue. It is 1 when either
// pass reports an issue, or when the program cannot run: a missing or
// unreadable file, a composition or OPT that does not parse, or the wrong
// number of file arguments. A bad flag exits with status 2.
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

// run validates the chosen composition and reports whether it passed both
// passes. An error means the program could not do its job (bad path,
// unreadable OPT); a false result means the composition was checked and has
// issues.
func run() (valid bool, err error) {
	useCorpus, args := parseFlags(os.Args[1:])
	jsonPath, optPath, err := resolvePaths(useCorpus, args)
	if err != nil {
		return false, err
	}

	// Step 1: read the wire bytes and decode them. canjson reads the "_type"
	// discriminators and fills the typed rm structs; a document that is not
	// well-formed canonical JSON fails here, before any validation runs.
	body, err := os.ReadFile(jsonPath)
	if err != nil {
		return false, fmt.Errorf("read JSON %q: %w", jsonPath, err)
	}
	var composition rm.Composition
	if err := canjson.Unmarshal(body, &composition); err != nil {
		return false, fmt.Errorf("decode canonical JSON: %w", err)
	}
	fmt.Printf("json                 : %s (%d bytes)\n", filepath.Base(jsonPath), len(body))
	fmt.Printf("composition          : archetype_node_id=%s content_items=%d\n",
		composition.ArchetypeNodeID, len(composition.Content))

	// Step 2: parse and compile the OPT. An operational template is the
	// deployable form of an openEHR template: every archetype it uses,
	// flattened into one XML file with the template's constraints applied.
	// ParseFileStrict rejects an unknown node type that has attributes under
	// it, where the lenient ParseFile would keep it as a leaf and silently
	// drop the constraints beneath it, which a validator must not do.
	// Compile turns the parsed XML into the driver that the validator (and
	// the composition builder, the instance generator, the AQL lint) walks.
	opt, err := template.ParseFileStrict(optPath)
	if err != nil {
		return false, fmt.Errorf("parse OPT %q: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return false, fmt.Errorf("compile OPT %q: %w", optPath, err)
	}
	fmt.Printf("template             : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

	// Step 3: the RM floor. ValidateRM walks the composition with the
	// Reference Model as its only guide, no template involved.
	rmOK := reportPass("RM floor", validation.ValidateRM(&composition))

	// Step 4: the template constraints. The template drives this walk: for
	// each node the OPT declares, the validator reads the matching part of
	// the composition and checks existence, cardinality, RM type and value
	// constraints. Both passes collect every issue instead of stopping at the
	// first, and the second runs whatever the first found: they check
	// different things, so neither result stands in for the other.
	templateOK := reportPass("template constraints", validation.ValidateComposition(&composition, compiled))

	if rmOK && templateOK {
		fmt.Println("result               : valid, both passes found no error")
		return true, nil
	}
	fmt.Println("result               : not valid")
	if useCorpus {
		fmt.Println("note                 : vital_signs.json is demo CDR data; issues are expected")
	}
	return false, nil
}

// reportPass prints one validation pass under its name: the verdict, then one
// line per issue. Path points at the offending node, Code is the stable
// identifier to dispatch on, Detail is the explanation for a human. It returns
// result.OK, which is false exactly when the pass found an error.
func reportPass(name string, result validation.Result) bool {
	verdict := "OK"
	if !result.OK {
		verdict = "failed"
	}
	if len(result.Issues) == 0 {
		fmt.Printf("%-20s : %s, no issues\n", name, verdict)
		return result.OK
	}
	fmt.Printf("%-20s : %s, %d issue(s)\n", name, verdict, len(result.Issues))
	for _, issue := range result.Issues {
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	return result.OK
}

// parseFlags reads the command line: the -corpus switch and the positional
// file arguments. -cassette is the flag's old name, from before the fixture
// tree was renamed testkit/corpus, and still sets the same switch so scripts
// written against it keep working.
func parseFlags(args []string) (useCorpus bool, rest []string) {
	fs := flag.NewFlagSet("validate-from-json", flag.ExitOnError)
	fs.BoolVar(&useCorpus, "corpus", false, "validate testkit/corpus vital_signs.json, demo data that reports issues")
	fs.BoolVar(&useCorpus, "cassette", false, "deprecated: use -corpus")
	// ExitOnError makes a bad flag print the usage and exit with status 2, as
	// the default command line does, so Parse only ever returns nil here.
	_ = fs.Parse(args)
	return useCorpus, fs.Args()
}

// resolvePaths picks the composition and the OPT to validate: the caller's
// two files, the demo corpus sample, or the clean default fixture next to this
// source file.
func resolvePaths(useCorpus bool, args []string) (jsonPath, optPath string, err error) {
	switch len(args) {
	case 2:
		return args[0], args[1], nil
	case 0:
		// Both vendored inputs are checked against the same vital_signs.opt.
		optPath = fixtures.TemplateOptForName("vital_signs")
		if useCorpus {
			return fixtures.CompositionJSON("vital_signs"), optPath, nil
		}
		jsonPath, err = defaultCompositionPath()
		return jsonPath, optPath, err
	default:
		return "", "", errors.New("usage: validate-from-json [-corpus] [composition.json template.opt]")
	}
}

// defaultCompositionPath locates testdata/minimal_blood_pressure.json
// relative to this source file, so `go run` works from any directory. The
// fixture is written by gen_fixture.go, which refuses to write one that fails
// either validation pass.
func defaultCompositionPath() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the example's source directory")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "minimal_blood_pressure.json"), nil
}
