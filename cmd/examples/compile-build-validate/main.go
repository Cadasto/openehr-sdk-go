// Example: the whole clinical pipeline through public SDK packages only. It
// parses an operational template (OPT), compiles it, builds a composition that
// follows it, serialises that composition to canonical JSON and back, and
// validates the result in two passes: the RM floor (validation.ValidateRM),
// against the openEHR Reference Model alone, and the template constraints
// (validation.ValidateComposition), against the same compiled template.
// Nothing here imports an internal/ package, so a program in another Go
// module can do exactly the same.
//
// The two passes compose but do not chain: today ValidateComposition checks
// the template's constraints and does not run the RM floor's per-type rules,
// so a composition can satisfy its template and still break the Reference
// Model.
// A program that wants both guarantees calls both.
//
// Runs offline. With no argument it uses the vendored vital_signs.opt fixture:
//
//	go run ./cmd/examples/compile-build-validate
//	go run ./cmd/examples/compile-build-validate path/to/template.opt
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// systolicPath is the template path of the systolic blood-pressure value in
// vital_signs.opt. A path like this is how the builder addresses one leaf:
// the content item, then the archetype's own nodes down to the DV_QUANTITY
// value. The template-explore example prints every such path of a template.
const systolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"

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

	// Step 1: parse the OPT. An operational template is the ADL 1.4 XML file
	// a clinical modeller exports; it fixes which archetypes, nodes and value
	// constraints a composition may contain. ParseFileStrict rejects an
	// unknown node type that has attributes under it; the lenient ParseFile
	// would keep such a node as a leaf and silently drop the constraints
	// beneath it, which a program that builds and validates data must not do.
	opt, err := template.ParseFileStrict(optPath)
	if err != nil {
		return fmt.Errorf("parse OPT %s: %w", optPath, err)
	}

	// Step 2: compile it. The compiled template is the one artefact the
	// builder and the validator share, so compile once per template and reuse
	// the result for every composition.
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}
	fmt.Printf("template             : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

	// Step 3: build a composition the template allows.
	comp, err := buildComposition(context.Background(), compiled)
	if err != nil {
		return err
	}

	// Step 4: serialise it to canonical JSON, the openEHR REST wire format,
	// and decode it again, the way a server would receive it.
	decoded, size, err := roundTrip(comp)
	if err != nil {
		return err
	}
	fmt.Printf("composition          : %d bytes canonical JSON, round-tripped\n", size)

	// Step 5: validate the decoded copy, the one a server would receive, in
	// both passes. The RM floor walks it with the Reference Model as its
	// only guide; the template pass checks it against the same compiled
	// template the builder used. Both always run, because neither result
	// stands in for the other.
	rmOK := reportPass("RM floor", validation.ValidateRM(decoded))
	templateOK := reportPass("template constraints", validation.ValidateComposition(decoded, compiled))
	if !rmOK || !templateOK {
		return errors.New("round-tripped composition fails at least one validation pass")
	}
	return nil
}

// buildComposition sets one value in a composition shaped by the template.
// The builder starts from a skeleton generated from the compiled template,
// with the mandatory structure already in place, so a program sets only the
// leaves it cares about.
func buildComposition(ctx context.Context, compiled *templatecompile.Compiled) (*rm.Composition, error) {
	// The composer is whoever authored the composition. A PARTY_IDENTIFIED
	// with just a name is the smallest form the Reference Model accepts.
	composer := &rm.PartyIdentified{Name: new("Dr Example")}

	// The builder takes a context like every SDK entry point that may do
	// work on the caller's behalf; there is no deadline to enforce here.
	builder, err := composition.NewBuilder(ctx, compiled,
		composition.WithTerritory("NL"),
		composition.WithComposer(composer),
	)
	if err != nil {
		return nil, fmt.Errorf("new composition builder: %w", err)
	}

	// SetQuantity addresses a DV_QUANTITY leaf by its template path. Set,
	// SetText and SetCodedText are the siblings for other value types.
	if err := builder.SetQuantity(systolicPath, 120, "mm[Hg]"); err != nil {
		return nil, fmt.Errorf("set systolic value: %w", err)
	}

	// Build applies the queued assignments and returns the composition.
	comp, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build composition: %w", err)
	}
	return comp, nil
}

// roundTrip encodes the composition as canonical JSON and decodes it into a
// fresh rm.Composition. canjson is the SDK's codec for that wire format. The
// encoded size is returned so the caller can show the document is real.
func roundTrip(comp *rm.Composition) (*rm.Composition, int, error) {
	encoded, err := canjson.Marshal(comp)
	if err != nil {
		return nil, 0, fmt.Errorf("encode canonical JSON: %w", err)
	}
	var decoded rm.Composition
	if err := canjson.Unmarshal(encoded, &decoded); err != nil {
		return nil, 0, fmt.Errorf("decode canonical JSON: %w", err)
	}
	return &decoded, len(encoded), nil
}

// reportPass prints one validation pass under its name: the verdict, then one
// line per issue. Each pass lists every issue it finds rather than stopping at
// the first, so a caller can show the whole list at once. It returns
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
