// Example: synthesise an RM instance from a compiled OPT and emit
// the canonical JSON to stdout. Demonstrates REQ-107 end-to-end —
// no transport, no auth, no client; just bytes → typed tree →
// instance graph → canonical JSON.
//
// Surfaces shown:
//   - openehr/template.ParseFile (lenient parse)
//   - internal/templatecompile.Compile (compiled walker-friendly tree)
//   - openehr/instance.Generate with Minimal / Example policy
//   - serialize/canjson.Marshal (example imports serialize; library does not)
//
// Run:
//
//	go run ./cmd/examples/generate-example \
//	    --opt testkit/cassettes/templates/vital_signs.opt \
//	    --territory NL \
//	    --composer-name "Test Composer" \
//	    --policy example
//
// With no --opt, the example defaults to the vendored vital_signs
// fixture so the demo works from any working directory.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	optFlag := flag.String("opt", "", "path to an ADL 1.4 operational template (defaults to vendored vital_signs.opt)")
	policyFlag := flag.String("policy", "example", "generation policy: 'minimal' or 'example'")
	territoryFlag := flag.String("territory", "NL", "ISO 3166-1 territory code (required for COMPOSITION roots)")
	composerFlag := flag.String("composer-name", "Example Composer", "PartyIdentified name for the COMPOSITION composer")
	flag.Parse()

	optPath := *optFlag
	if optPath == "" {
		optPath = defaultOPTPath()
	}

	policy, err := parsePolicy(*policyFlag)
	if err != nil {
		log.Fatal(err)
	}

	opt, err := template.ParseFile(optPath)
	if err != nil {
		log.Fatalf("ParseFile %q: %v", optPath, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("Compile %q: %v", optPath, err)
	}

	composer := &rm.PartyIdentified{Name: composerFlag}

	out, err := instance.Generate(context.Background(), c, instance.Options{
		Policy:    policy,
		Territory: *territoryFlag,
		Composer:  composer,
	})
	if err != nil {
		log.Fatalf("Generate: %v", err)
	}

	buf, err := canjson.Marshal(out)
	if err != nil {
		log.Fatalf("canjson.Marshal: %v", err)
	}
	if _, err := os.Stdout.Write(buf); err != nil {
		log.Fatalf("write stdout: %v", err)
	}
	fmt.Println()
}

// parsePolicy maps the CLI string to the instance.Policy constant.
// Returns an error for unknown values rather than silently
// defaulting — surfaces typos at the command line.
func parsePolicy(s string) (instance.Policy, error) {
	switch s {
	case "minimal":
		return instance.Minimal, nil
	case "example":
		return instance.Example, nil
	}
	return 0, fmt.Errorf("unknown --policy %q (want 'minimal' or 'example')", s)
}

// defaultOPTPath resolves the vendored vital_signs template fixture.
func defaultOPTPath() string {
	return fixtures.TemplateOptForName("vital_signs")
}
