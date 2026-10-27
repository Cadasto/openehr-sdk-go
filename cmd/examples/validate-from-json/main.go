// Validate a COMPOSITION that arrives as canonical JSON, the way a CI check or
// an inbound gateway would: read the bytes, compile an operational template
// (OPT), and decode and validate the document with one call that lists every
// issue it finds. No HTTP is involved.
//
// validation.ValidateJSON decodes the bytes into an rm.Composition and, in
// that one call, checks:
//
//   - the decode: a document that does not decode into a COMPOSITION, such
//     as malformed JSON or a root that is not a JSON object, is one
//     invalid_shape issue at "/";
//   - the members the document leaves out: a mandatory number, boolean or
//     value struct, such as a DV_QUANTITY's magnitude, decodes to its zero
//     value whether the document gave it or not, so the decoded composition
//     cannot show it absent; the call reads the document and reports each
//     such member as required at its path;
//   - the template constraints: what the OPT declares, node by node:
//     existence, cardinality, RM type, archetype identity and value
//     constraints;
//   - the RM floor: the openEHR Reference Model itself, over every part of
//     the composition the template admits: the attributes the RM makes
//     mandatory on every node, and the RM's own rules for each type, such
//     as an ELEMENT carrying exactly one of a value or a null flavour, and
//     every COMPOSITION and ENTRY carrying archetype_details.
//
// A defect two checks find is reported once. validation.ValidateComposition
// runs the last two checks on a composition already in memory, and
// validation.ValidateRM is the floor alone, for a root no template applies
// to.
//
// By default it validates testdata/minimal_blood_pressure.json, a hand-made
// composition that is valid against the vendored vital_signs.opt. With
// -corpus it validates the vendored testkit/corpus vital_signs.json instead,
// which is demo data and reports issues, so you can see what a failing run
// looks like. Two positional arguments validate your own files.
//
//	go run ./cmd/examples/validate-from-json
//	go run ./cmd/examples/validate-from-json -corpus
//	go run ./cmd/examples/validate-from-json composition.json template.opt
//
// The exit status is 0 when validation reports no error, and 1 when it does.
// A composition that does not decode is one of those outcomes: its issue is
// printed under validation and the status is 1. The status is also 1 when
// the program cannot run: a missing or unreadable file, an OPT that does not
// parse, or the wrong number of file arguments. A bad flag exits with
// status 2.
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

// run validates the chosen composition and reports whether it is valid. An
// error means the program could not do its job (bad path, unreadable OPT); a
// false result means the composition was checked and has issues, a
// composition that does not decode included.
func run() (valid bool, err error) {
	useCorpus, args := parseFlags(os.Args[1:])
	jsonPath, optPath, err := resolvePaths(useCorpus, args)
	if err != nil {
		return false, err
	}
	result, err := validateFiles(jsonPath, optPath)
	if err != nil {
		return false, err
	}
	if report("validation", result) {
		fmt.Println("result               : valid, no error found")
		return true, nil
	}
	fmt.Println("result               : not valid")
	if useCorpus {
		fmt.Println("note                 : vital_signs.json is demo CDR data; issues are expected")
	}
	return false, nil
}

// validateFiles reads the composition at jsonPath, compiles the OPT at
// optPath, and decodes and validates the composition against it, printing
// what it read on the way. An error means a file could not be read or the OPT
// could not be compiled; a composition that does not decode comes back in the
// result, as an invalid_shape issue at "/".
func validateFiles(jsonPath, optPath string) (validation.Result, error) {
	// Step 1: read the wire bytes. They are decoded in step 3, by the same
	// call that validates them.
	body, err := os.ReadFile(jsonPath)
	if err != nil {
		return validation.Result{}, fmt.Errorf("read JSON %q: %w", jsonPath, err)
	}
	fmt.Printf("json                 : %s (%d bytes)\n", filepath.Base(jsonPath), len(body))

	// Step 2: parse and compile the OPT. An operational template is the
	// deployable form of an openEHR template: every archetype it uses,
	// flattened into one XML file with the template's constraints applied.
	// ParseFileStrict rejects an unknown node type that has attributes under
	// it, where the lenient ParseFile would keep it as a leaf and silently
	// drop the constraints beneath it, which a validator must not do.
	// Compile turns the parsed XML into the compiled template that the
	// validator, the composition builder, the instance generator and the
	// AQL lint all read.
	opt, err := template.ParseFileStrict(optPath)
	if err != nil {
		return validation.Result{}, fmt.Errorf("parse OPT %q: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return validation.Result{}, fmt.Errorf("compile OPT %q: %w", optPath, err)
	}
	fmt.Printf("template             : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

	// Step 3: decode and validate in one call. ValidateJSON decodes the bytes
	// as canjson.Unmarshal does: it reads the "_type" discriminators and fills
	// the typed rm structs. It also reads the document for the mandatory
	// members it leaves out. A number, a boolean or a value struct decodes to
	// its zero value whether the document gave it or not, so only the bytes
	// can show it missing. Then one walk visits the composition. Where a node
	// binds to a node of the OPT, the validator checks the template's clauses
	// there: existence, cardinality, RM type and value constraints. On every
	// node the template admits it also applies the Reference Model's own
	// rules. The walk collects every issue instead of stopping at the first,
	// and a member two checks find missing is one issue.
	var composition rm.Composition
	result := validation.ValidateJSON(body, &composition, compiled)
	// composition holds the decoded value now, so the program reads it
	// without decoding the bytes a second time.
	fmt.Printf("composition          : archetype_node_id=%s content_items=%d\n",
		composition.ArchetypeNodeID, len(composition.Content))
	return result, nil
}

// report prints the validation result under name: the verdict, then one line
// per issue. Path points at the offending node, Code is the stable identifier
// to dispatch on, Detail is the explanation for a human. It returns
// result.OK, which is false exactly when an error was found.
func report(name string, result validation.Result) bool {
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
// validation.
func defaultCompositionPath() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the example's source directory")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "minimal_blood_pressure.json"), nil
}
