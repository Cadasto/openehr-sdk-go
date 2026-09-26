// This example checks AQL queries for problems before they reach a server.
// AQL (Archetype Query Language) is the openEHR query language. The linter
// parses the text, checks its shape, judges the FROM / CONTAINS clauses
// against the openEHR Reference Model (RM), and, when it is given a compiled
// template, checks the archetypes and paths against that template too. The
// template is an OPT (operational template): the flattened definition of one
// clinical document type. Everything runs offline; with no argument the
// vendored vital_signs.opt fixture is used.
//
// Run:
//
//	go run ./cmd/examples/lint-aql
//	go run ./cmd/examples/lint-aql path/to/template.opt
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
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

	compiled, err := loadTemplate(optPath)
	if err != nil {
		return err
	}
	fmt.Printf("template : %s (%s)\n\n", compiled.TemplateID(), filepath.Base(optPath))

	// A clean query. The archetype is in the template, the path resolves,
	// the $ehr_id placeholder is bound in Query.Parameters, every repeating
	// step in the path carries a predicate, and the SELECT item has an
	// alias, so even the advisory path-shape checks stay quiet.
	clean := aql.NewQuery(
		"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude AS magnitude " +
			"FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	clean.Parameters = map[string]any{"ehr_id": "7d44b88c-..."}
	report("clean query", clean, compiled)

	// A broken query. The archetype is not in the template and $threshold
	// is never bound; both are errors. The `events` step without a
	// predicate and the missing alias add two advisories on top.
	broken := aql.NewQuery(
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1] " +
			"WHERE o/data/events/value/magnitude > $threshold",
	)
	report("broken query", broken, compiled)

	// A query that is well-formed and can never match: under the RM an
	// OBSERVATION never contains a COMPOSITION. This check judges the query
	// against the RM alone, so it runs whether or not a template is given.
	semantic := aql.NewQuery("SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c")
	report("semantic finding (RM-impossible containment)", semantic, compiled)

	// A query that is OK yet not issue-free. A FOLDER reaches a COMPOSITION
	// only through a reference (a FOLDER holds OBJECT_REFs, not the
	// compositions themselves), so the linter reports a warning rather
	// than an error and Result.OK stays true. Most containment and
	// portability codes are warnings, so an OK result is often not an
	// empty one.
	advisory := aql.NewQuery("SELECT c FROM FOLDER f CONTAINS COMPOSITION c")
	report("advisory only (OK, but not issue-free)", advisory, compiled)
	return nil
}

// loadTemplate parses the OPT file and compiles it. The compiled form is what
// every template-aware SDK entry point takes, ValidateAQL included.
func loadTemplate(optPath string) (*templatecompile.Compiled, error) {
	opt, err := template.ParseFile(optPath)
	if err != nil {
		return nil, fmt.Errorf("parse OPT %q: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return nil, fmt.Errorf("compile OPT %q: %w", optPath, err)
	}
	return compiled, nil
}

// report lints one query and prints every issue it holds. Result.OK is false
// only when an Error-severity issue is present, so an OK result may still
// carry warnings; returning early on OK would hide them.
func report(label string, query aql.Query, compiled *templatecompile.Compiled) {
	fmt.Printf("== %s ==\n%s\n", label, query.Q)
	result := validation.ValidateAQL(query, compiled)

	var errorCount, warningCount int
	for _, issue := range result.Issues {
		if issue.Severity == validation.Error {
			errorCount++
		} else {
			warningCount++
		}
	}
	switch {
	case len(result.Issues) == 0:
		fmt.Print("result   : OK — no issues\n\n")
		return
	case result.OK:
		fmt.Printf("result   : OK — no errors, %s\n", plural(warningCount, "advisory", "advisories"))
	default:
		fmt.Printf("result   : not OK — %s, %s\n",
			plural(errorCount, "error", "errors"), plural(warningCount, "advisory", "advisories"))
	}
	// Each issue carries a stable Code, which is what a program dispatches
	// on, the path it points at when it localises to one, and a free-text
	// Detail.
	for _, issue := range result.Issues {
		where := issue.Path
		if where == "" {
			where = "-"
		}
		fmt.Printf("  [%s] %s (%s): %s\n", issue.Severity, issue.Code, where, issue.Detail)
	}
	fmt.Println()
}

// plural renders a count with the matching noun form.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
