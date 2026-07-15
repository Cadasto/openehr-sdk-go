// Example: convert an openEHR COMPOSITION to the FLAT and STRUCTURED
// Simplified Formats and back, driven by the composition's Web Template
// (REQ-053 + REQ-106). Demonstrates the building-block path (REQ-013) — no
// transport, no auth, no discovery: an OPT + a canonical composition in, FLAT /
// STRUCTURED out, and a round-trip back to a composition.
//
// Run: `go run ./cmd/examples/flat-roundtrip` from any directory.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

const templateID = "Test_dv_quantity_open_constraint.v0"

func main() {
	// 1. Build the Web Template + keep the compiled template (both come from the OPT).
	compiled, wt := buildTemplate()

	// 2. Decode a canonical COMPOSITION.
	comp := decodeComposition()

	// 3. COMPOSITION -> FLAT.
	flat, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		log.Fatalf("MarshalFlat: %v", err)
	}
	fmt.Printf("FLAT (%s):\n", simplified.MediaTypeFlat)
	printFlat(flat)

	// 4. FLAT -> STRUCTURED (no OPT needed) and back.
	structured, err := simplified.FlatToStructured(flat)
	if err != nil {
		log.Fatalf("FlatToStructured: %v", err)
	}
	fmt.Printf("\nSTRUCTURED (%s): %d bytes\n", simplified.MediaTypeStructured, len(structured))

	// 5. FLAT -> COMPOSITION -> FLAT: the round-trip reproduces the FLAT.
	comp2, err := simplified.UnmarshalFlat(flat, wt)
	if err != nil {
		log.Fatalf("UnmarshalFlat: %v", err)
	}
	flat2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		log.Fatalf("MarshalFlat (round-trip): %v", err)
	}
	if !sameKeys(flat, flat2) {
		log.Fatal("round-trip mismatch")
	}
	fmt.Println("\nOK: FLAT -> COMPOSITION -> FLAT round-trips for", templateID)

	// 6. Conformant decode: WithTemplate repopulates LOCATABLE.name and completes
	// the RM-mandatory attributes the format does not carry, so the result
	// validates against the OPT.
	conformant, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		log.Fatalf("UnmarshalFlat (WithTemplate): %v", err)
	}
	if r := validation.Validate(conformant, compiled); r.OK {
		fmt.Println("OK: WithTemplate decode validates against the OPT")
	} else {
		fmt.Printf("decoded composition has %d validation issue(s)\n", len(r.Issues))
	}
}

func buildTemplate() (*templatecompile.Compiled, *webtemplate.WebTemplate) {
	optBody, err := os.ReadFile(fixtures.TemplateOpt(templateID))
	if err != nil {
		log.Fatalf("read OPT: %v", err)
	}
	opt, err := fixtures.ParseOPTBytes(optBody)
	if err != nil {
		log.Fatalf("parse OPT: %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatalf("compile: %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		log.Fatalf("build web template: %v", err)
	}
	return compiled, wt
}

func decodeComposition() *rm.Composition {
	body, err := os.ReadFile(fixtures.CompositionJSON(templateID))
	if err != nil {
		log.Fatalf("read composition: %v", err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		log.Fatalf("canjson decode: %v", err)
	}
	return &comp
}

func printFlat(flat []byte) {
	var m map[string]any
	if err := json.Unmarshal(flat, &m); err != nil {
		log.Fatalf("parse flat: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s = %v\n", k, m[k])
	}
}

func sameKeys(a, b []byte) bool {
	var ma, mb map[string]any
	if json.Unmarshal(a, &ma) != nil || json.Unmarshal(b, &mb) != nil {
		return false
	}
	if len(ma) != len(mb) {
		return false
	}
	for k := range ma {
		if _, ok := mb[k]; !ok {
			return false
		}
	}
	return true
}
