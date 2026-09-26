// Example: the whole clinical pipeline through public SDK packages only. It
// parses an operational template (OPT), compiles it, builds a composition that
// follows it, serialises that composition to canonical JSON and back, and
// validates the result against the same compiled template. Nothing here
// imports an internal/ package, so a program in another Go module can do
// exactly the same.
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
	// constraints a composition may contain.
	opt, err := template.ParseFile(optPath)
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
	fmt.Printf("template : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

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
	fmt.Printf("composition: %d bytes canonical JSON, round-tripped\n", size)

	// Step 5: validate the decoded copy against the same compiled template.
	if err := reportValidation(decoded, compiled); err != nil {
		return err
	}

	// Step 6: the validator has typed entry points for other RM roots too.
	return checkEHRStatusValidator(compiled)
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

// reportValidation prints the validator's verdict. The result lists every
// issue found in one pass rather than stopping at the first, so a caller can
// show the whole list at once.
func reportValidation(comp *rm.Composition, compiled *templatecompile.Compiled) error {
	result := validation.ValidateComposition(comp, compiled)
	if result.OK {
		fmt.Println("validation : OK — round-tripped composition conforms to the OPT")
		return nil
	}
	fmt.Printf("validation : %d issue(s)\n", len(result.Issues))
	for _, issue := range result.Issues {
		fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
	}
	return errors.New("round-tripped composition does not conform to the OPT")
}

// checkEHRStatusValidator shows that the validator also has typed entry
// points for other RM roots (ValidateEHRStatus here; ValidateFolder and
// ValidateDemographic are its siblings). An EHR_STATUS can never satisfy a
// template whose root is a COMPOSITION, so the validator must report a root
// type mismatch. An OK here would mean that check has stopped working, and
// the example fails rather than print a misleading line.
func checkEHRStatusValidator(compiled *templatecompile.Compiled) error {
	status := &rm.EHRStatus{Name: rm.DVText{Value: "EHR Status"}, Subject: rm.PartySelf{}}
	result := validation.ValidateEHRStatus(status, compiled)
	if result.OK {
		return errors.New("ValidateEHRStatus unexpectedly OK against a COMPOSITION OPT")
	}
	fmt.Printf("ehr_status : ValidateEHRStatus callable — %d issue(s), root type mismatch as expected\n", len(result.Issues))
	return nil
}
