// Example: convert a composition to the FLAT and STRUCTURED simplified
// formats and back. These formats address values by short Web Template ids
// instead of full Reference Model paths, so every conversion needs the
// composition's Web Template. The program builds that from the OPT, encodes a
// vendored composition as FLAT, restructures it as STRUCTURED, decodes the
// FLAT back into a composition, and finally shows the template-aware decode
// whose result validates against the OPT. Nothing here imports an internal/
// package.
//
// Runs offline on the vendored Test_dv_quantity_open_constraint.v0 fixtures:
//
//	go run ./cmd/examples/flat-roundtrip
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"reflect"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// templateID names both vendored fixtures: the OPT and the canonical-JSON
// composition that was recorded against it.
const templateID = "Test_dv_quantity_open_constraint.v0"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: compile the OPT and derive its Web Template. The compiled
	// template drives validation and the template-aware decode; the Web
	// Template supplies the ids the FLAT and STRUCTURED formats are keyed by.
	compiled, wt, err := loadTemplate()
	if err != nil {
		return err
	}

	// Step 2: a canonical composition to convert.
	comp, err := loadComposition()
	if err != nil {
		return err
	}

	// Step 3: composition to FLAT. Every key is a path of Web Template ids,
	// with an optional |suffix naming the part of a value it carries
	// (|magnitude, |unit, |code); composition-level metadata sits under ctx/.
	flat, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		return fmt.Errorf("encode FLAT: %w", err)
	}
	fmt.Printf("FLAT (%s):\n", simplified.MediaTypeFlat)
	if err := printFlat(flat); err != nil {
		return err
	}

	// Step 4: FLAT to STRUCTURED. Same ids and values, nested as JSON objects
	// instead of one flat map, so this step needs no template at all.
	structured, err := simplified.FlatToStructured(flat)
	if err != nil {
		return fmt.Errorf("restructure FLAT as STRUCTURED: %w", err)
	}
	fmt.Printf("\nSTRUCTURED (%s): %d bytes\n", simplified.MediaTypeStructured, len(structured))

	// Step 5: FLAT back to a composition, then to FLAT again. Without a
	// compiled template the decode keeps exactly what the format carries, so
	// encoding the result reproduces the first document key for key.
	decoded, err := simplified.UnmarshalFlat(flat, wt)
	if err != nil {
		return fmt.Errorf("decode FLAT: %w", err)
	}
	flatAgain, err := simplified.MarshalFlat(decoded, wt)
	if err != nil {
		return fmt.Errorf("encode FLAT again: %w", err)
	}
	same, err := sameFlat(flat, flatAgain)
	if err != nil {
		return err
	}
	if !same {
		return errors.New("FLAT -> COMPOSITION -> FLAT changed the document")
	}
	fmt.Println("\nOK: FLAT -> COMPOSITION -> FLAT round-trips for", templateID)

	// Step 6: the template-aware decode. The formats carry no node names and
	// omit attributes the Reference Model requires (HISTORY.origin,
	// EVENT.time, ...). WithTemplate restores the names from the compiled
	// template and fills the other required attributes with synthesised
	// defaults (from ctx values and RM conventions), so treat those as
	// defaults, not recovered data. The result validates against the OPT
	// when the FLAT input carries ctx/time, which this one does.
	conformant, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		return fmt.Errorf("decode FLAT with template: %w", err)
	}
	result := validation.ValidateComposition(conformant, compiled)
	if !result.OK {
		fmt.Printf("decoded composition has %d validation issue(s):\n", len(result.Issues))
		for _, issue := range result.Issues {
			fmt.Printf("  %s [%s] %s\n", issue.Path, issue.Code, issue.Detail)
		}
		return errors.New("template-aware decode does not conform to the OPT")
	}
	fmt.Println("OK: WithTemplate decode validates against the OPT")
	return nil
}

// loadTemplate parses the vendored OPT, compiles it, and builds its Web
// Template. Do this once per template in your own code and reuse both results.
func loadTemplate() (*templatecompile.Compiled, *webtemplate.WebTemplate, error) {
	optPath := fixtures.TemplateOpt(templateID)
	opt, err := template.ParseFile(optPath)
	if err != nil {
		return nil, nil, fmt.Errorf("parse OPT %s: %w", optPath, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return nil, nil, fmt.Errorf("compile template: %w", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		return nil, nil, fmt.Errorf("build web template: %w", err)
	}
	return compiled, wt, nil
}

// loadComposition decodes the vendored canonical-JSON composition for the
// template. canjson is the SDK's codec for the openEHR canonical JSON wire
// format.
func loadComposition() (*rm.Composition, error) {
	path := fixtures.CompositionJSON(templateID)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read composition %s: %w", path, err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		return nil, fmt.Errorf("decode canonical JSON: %w", err)
	}
	return &comp, nil
}

// printFlat lists the FLAT document one key per line. A FLAT document is a
// single JSON object, so decoding it into a plain map is enough to read it;
// the keys are sorted because a Go map has no order.
func printFlat(flat []byte) error {
	var entries map[string]any
	if err := json.Unmarshal(flat, &entries); err != nil {
		return fmt.Errorf("parse FLAT document: %w", err)
	}
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		fmt.Printf("  %s = %v\n", key, entries[key])
	}
	return nil
}

// sameFlat reports whether two FLAT documents hold the same keys and values,
// whatever their key order or whitespace.
func sameFlat(a, b []byte) (bool, error) {
	var entriesA, entriesB map[string]any
	if err := json.Unmarshal(a, &entriesA); err != nil {
		return false, fmt.Errorf("parse first FLAT document: %w", err)
	}
	if err := json.Unmarshal(b, &entriesB); err != nil {
		return false, fmt.Errorf("parse second FLAT document: %w", err)
	}
	return reflect.DeepEqual(entriesA, entriesB), nil
}
