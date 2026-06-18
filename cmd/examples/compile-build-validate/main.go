// Example: the full clinical pipeline driven entirely through PUBLIC
// SDK packages (REQ-111) — parse an OPT, compile it with the public
// openehr/templatecompile bridge, build a *rm.Composition with the
// REQ-101 builder, serialise it to canonical JSON, round-trip it, and
// validate it against the same compiled template.
//
// The point of this example is the import list: it uses only
// openehr/template, openehr/templatecompile, openehr/composition,
// openehr/serialize/canjson, openehr/validation and openehr/rm — no
// internal/ package. Before REQ-111 the compiled template was only
// constructable inside the SDK module, so this exact program could not
// be written by an external consumer. It now can.
//
// Run:
//
//	go run ./cmd/examples/compile-build-validate [path/to/template.opt]
//
// With no argument it uses the vendored vital_signs.opt fixture.
package main

import (
	"context"
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

// systolicPath addresses the systolic DV_QUANTITY leaf of the
// vital_signs blood_pressure observation.
const systolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"

func main() {
	ctx := context.Background()
	optPath := fixtures.TemplateOptForName("vital_signs")
	if args := os.Args[1:]; len(args) > 0 {
		optPath = args[0]
	}

	// 1. Parse the OPT — public openehr/template.
	opt, err := template.ParseFile(optPath)
	if err != nil {
		log.Fatalf("ParseFile %q: %v", optPath, err)
	}

	// 2. Compile it — public openehr/templatecompile (the REQ-111 bridge).
	c, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("Compile: %v", err)
	}
	fmt.Printf("template : %s (%s)\n", opt.TemplateID(), filepath.Base(optPath))

	// 3. Build a composition — public openehr/composition.
	composer := &rm.PartyIdentified{}
	name := "Dr Example"
	composer.Name = &name
	b, err := composition.NewBuilder(
		ctx, c,
		composition.WithTerritory("NL"),
		composition.WithComposer(composer),
	)
	if err != nil {
		log.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetQuantity(systolicPath, 120, "mm[Hg]"); err != nil {
		log.Fatalf("SetQuantity: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		log.Fatalf("Build: %v", err)
	}

	// 4. Serialise + round-trip — public openehr/serialize/canjson.
	encoded, err := canjson.Marshal(comp)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
	var decoded rm.Composition
	if err := canjson.Unmarshal(encoded, &decoded); err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	fmt.Printf("composition: %d bytes canonical JSON, round-tripped\n", len(encoded))

	// 5. Validate the decoded composition — public openehr/validation.
	if res := validation.ValidateComposition(&decoded, c); res.OK {
		fmt.Println("validation : OK — round-tripped composition conforms to the OPT")
	} else {
		fmt.Printf("validation : %d issue(s)\n", len(res.Issues))
		for _, is := range res.Issues {
			fmt.Printf("  %s [%s] %s\n", is.Path, is.Code, is.Detail)
		}
		os.Exit(1)
	}

	// 6. The other typed validators are reachable too (REQ-110). Against
	//    a COMPOSITION-rooted OPT an EHR_STATUS correctly mismatches —
	//    shown here only to prove the call path is public.
	status := &rm.EHRStatus{Name: rm.DVText{Value: "EHR Status"}, Subject: rm.PartySelf{}}
	fmt.Printf("ehr_status : ValidateEHRStatus callable (OK=%v against a COMPOSITION OPT)\n",
		validation.ValidateEHRStatus(status, c).OK)
}
