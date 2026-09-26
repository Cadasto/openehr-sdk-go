// Generate an RM instance from an operational template (OPT) and print it as
// canonical JSON. The template alone decides the shape of the document; the
// SDK creates every node the template requires and fills the leaves with
// placeholder values. Seeders and fixture generators use this shape. No HTTP
// is involved.
//
// With no flags it uses the vendored vital_signs.opt, so it runs offline from
// any directory. Every flag has a default:
//
//	go run ./cmd/examples/generate-example
//	go run ./cmd/examples/generate-example \
//	    --opt testkit/cassettes/templates/vital_signs.opt \
//	    --territory NL \
//	    --composer-name "Test Composer" \
//	    --policy example
//
// The output is one line of JSON; pipe it to a file or into validate-from-json.
// Each run gets fresh uids and a wall-clock context start time.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	optFlag := flag.String("opt", "", "path to an ADL 1.4 operational template (default: the vendored vital_signs.opt)")
	policyFlag := flag.String("policy", "example", "how much to fill in: 'minimal' or 'example'")
	territoryFlag := flag.String("territory", "NL", "ISO 3166-1 territory code; a COMPOSITION root requires one")
	composerFlag := flag.String("composer-name", "Example Composer", "name recorded as the composition's composer")
	flag.Parse()

	optPath := *optFlag
	if optPath == "" {
		optPath = fixtures.TemplateOptForName("vital_signs")
	}
	policy, err := parsePolicy(*policyFlag)
	if err != nil {
		return err
	}

	// Step 1: parse and compile the OPT. An operational template is the
	// deployable form of an openEHR template: every archetype it uses,
	// flattened into one XML file with the template's constraints applied.
	// Compile turns it into the driver the generator walks; the same compiled
	// value feeds the validator and the composition builder.
	opt, err := template.ParseFile(optPath)
	if err != nil {
		return fmt.Errorf("parse OPT %q: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile OPT %q: %w", optPath, err)
	}

	// Step 2: generate. A COMPOSITION root needs two facts the template does
	// not carry: the territory the document belongs to and who composed it.
	// Optional RM strings are pointers, hence new(...) for the composer name.
	generated, err := instance.Generate(context.Background(), compiled, instance.Options{
		Policy:    policy,
		Territory: *territoryFlag,
		Composer:  &rm.PartyIdentified{Name: new(*composerFlag)},
	})
	if err != nil {
		return fmt.Errorf("generate instance: %w", err)
	}

	// Step 3: print. Generate returns the root as `any` because a template can
	// be rooted on any archetypeable type; canjson encodes it without knowing
	// the concrete type. Code that needs the typed value casts it with
	// instance.AsComposition and friends.
	encoded, err := canjson.Marshal(generated)
	if err != nil {
		return fmt.Errorf("encode canonical JSON: %w", err)
	}
	if _, err := os.Stdout.Write(encoded); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	fmt.Println()
	return nil
}

// parsePolicy maps the flag text onto the two generation policies. Minimal
// creates only the nodes the template requires, the smallest valid tree;
// Example also populates every primitive leaf with its example value. An
// unknown value is an error, so a typo on the command line does not silently
// pick a default.
func parsePolicy(name string) (instance.Policy, error) {
	switch name {
	case "minimal":
		return instance.Minimal, nil
	case "example":
		return instance.Example, nil
	default:
		return 0, fmt.Errorf("unknown --policy %q (want 'minimal' or 'example')", name)
	}
}
