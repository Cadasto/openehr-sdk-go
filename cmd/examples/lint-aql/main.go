// Example: statically lint AQL with openehr/aql/lint and the
// validation.ValidateAQL bridge (REQ-109). Demonstrates the building-block
// path — no transport, no auth, no client: query string → parse against the
// SDK grammar profile → 3-layer lint → Issue list.
//
// Note: templatecompile.Compile is internal; this example lives in the SDK
// module and is the supported v1 call shape for the template-aware Layer 3
// (same constraint as validate-composition — see ADR 0005). Layers 1–2
// (syntax, shape, parameter binding, the REQ-160/161 containment +
// portability semantic group and the REQ-164 path-shape advisories, which
// both run unconditionally against the pinned RM) need no template and are
// usable by any external consumer via openehr/aql/lint directly.
//
// Run:
//
//	go run ./cmd/examples/lint-aql                 # lint a clean + a broken query
//	go run ./cmd/examples/lint-aql [path/to/template.opt]
//
// With no path argument, uses the vendored vital_signs.opt fixture.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := os.Args[1:]; len(args) > 0 {
		optPath = args[0]
	}

	opt, err := template.ParseFile(optPath)
	if err != nil {
		log.Fatalf("ParseFile %q: %v", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("Compile %q: %v", optPath, err)
	}
	fmt.Printf("template : %s (%s)\n\n", opt.TemplateID(), filepath.Base(optPath))

	// Clean: archetype is in the template; path resolves; parameter is bound;
	// and every repeating path step carries a predicate while the SELECT item
	// carries an alias, so the REQ-164 path-shape checks stay quiet too.
	clean := aql.NewQuery(
		"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude AS magnitude " +
			"FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	clean.Parameters = map[string]any{"ehr_id": "7d44b88c-..."}
	report("clean query", clean, compiled)

	// Broken: archetype absent from the template, plus an unbound $param.
	broken := aql.NewQuery(
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1] " +
			"WHERE o/data/events/value/magnitude > $threshold",
	)
	report("broken query", broken, compiled)

	// Semantic: grammatically and shape-valid, but RM-impossible — OBSERVATION
	// never contains COMPOSITION (REQ-160/161). The Layer-2 semantic group
	// (openehr/aql/lint's completed REQ-161 checks) runs unconditionally, so
	// this finding surfaces with no template at all.
	semantic := aql.NewQuery("SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c")
	report("semantic finding (RM-impossible containment)", semantic, compiled)

	// Advisory-only: the hop is real under the pinned RM but crosses a
	// by-reference edge (FOLDER holds OBJECT_REFs, not the COMPOSITIONs), so
	// the group reports Warnings and Result.OK stays true. Most of the REQ-161
	// codes are Warning-severity — an OK result is not an empty one.
	advisory := aql.NewQuery("SELECT c FROM FOLDER f CONTAINS COMPOSITION c")
	report("advisory only (OK, but not issue-free)", advisory, compiled)
}

// report prints what the result HOLDS, not merely what OK summarises.
// [validation.Result.OK] is false only for Error-severity issues (REQ-109),
// so an OK result may still carry Warning advisories — returning early on OK
// would discard them.
func report(label string, q aql.Query, c *templatecompile.Compiled) {
	fmt.Printf("== %s ==\n%s\n", label, q.Q)
	r := validation.ValidateAQL(q, c)

	var errs, warns int
	for _, i := range r.Issues {
		if i.Severity == validation.Error {
			errs++
		} else {
			warns++
		}
	}
	switch {
	case len(r.Issues) == 0:
		fmt.Print("result   : OK — no issues\n\n")
		return
	case r.OK:
		fmt.Printf("result   : OK — no errors, %s\n", plural(warns, "advisory", "advisories"))
	default:
		fmt.Printf("result   : not OK — %s, %s\n",
			plural(errs, "error", "errors"), plural(warns, "advisory", "advisories"))
	}
	for _, i := range r.Issues {
		where := i.Path
		if where == "" {
			where = "-"
		}
		fmt.Printf("  [%s] %s (%s): %s\n", i.Severity, i.Code, where, i.Detail)
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
