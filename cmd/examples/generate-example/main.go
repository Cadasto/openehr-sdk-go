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
//	    --opt testkit/corpus/templates/vital_signs.opt \
//	    --territory NL \
//	    --composer-name "Test Composer" \
//	    --policy large
//
// The output is one line of JSON; pipe it to a file or into validate-from-json.
// Each run gets fresh uids and a wall-clock context start time.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
	policyFlag := flag.String("policy", defaultPolicy, "how much to build; case does not matter: 'required' or its alias 'minimal' (the mandatory data and a composition's context), 'medium' (also every attribute the template constrains, at most one optional archetype under each), 'large' or its alias 'complete' (a member of every optional archetype)")
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
	// ReadOperationalTemplate reads it into the operational template model,
	// and by default reads a node type it does not know as a leaf without
	// its subtree, so the instance lacks that subtree. A generator that must
	// not see a node reduced passes canxml.WithStrict() here and
	// templatecompile.WithStrict() to Compile, which refuse such readings.
	// Compile turns the model into the driver the generator walks; the same
	// compiled value feeds the validator and the composition builder.
	raw, err := os.ReadFile(optPath)
	if err != nil {
		return fmt.Errorf("read OPT %q: %w", optPath, err)
	}
	opt, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("read OPT %q: %w", optPath, err)
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

// defaultPolicy is the --policy default, the instance package's default
// level.
const defaultPolicy = "medium"

// parsePolicy reads the --policy text with instance.ParsePolicy, which
// takes the names required, minimal, medium, large and complete in any
// case. Required, and minimal with it, builds only the data points the
// template makes mandatory and a composition's context, the ITS-REST
// template example's detail_level=required. Medium also builds every
// attribute the template constrains, with at most one optional archetype
// under each, its detail_level=medium. Large, and complete with it for
// now, builds a member of every optional archetype the template names,
// short of its detail_level=complete. Under each of them every primitive
// leaf holds its example value. Any other text is an error that names the
// flag, so a typo on the command line does not silently pick a default.
func parsePolicy(name string) (instance.Policy, error) {
	p, err := instance.ParsePolicy(name)
	if err != nil {
		return 0, fmt.Errorf("--policy (want required, minimal, medium, large or complete): %w", err)
	}
	return p, nil
}
