// Take one COMPOSITION through both canonical formats and back: JSON to Go
// structs, structs to canonical XML, XML back to structs, structs to JSON. The
// program then compares a JSON re-encode of the decoded input with the JSON it
// ended with, which shows that the two codecs (canjson and canxml) describe the
// same document.
//
// It runs offline against the vendored body_weight.json cassette:
//
//	go run ./cmd/examples/canxml_roundtrip
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	body, err := os.ReadFile(fixtures.CompositionJSON("body_weight"))
	if err != nil {
		return fmt.Errorf("read cassette: %w", err)
	}
	fmt.Printf("input JSON: %d bytes\n", len(body))

	// Step 1: JSON to structs. canjson fills the typed rm structs from the
	// canonical JSON, using the "_type" discriminators to pick concrete types.
	var fromJSON rm.Composition
	if err := canjson.Unmarshal(body, &fromJSON); err != nil {
		return fmt.Errorf("decode canonical JSON: %w", err)
	}

	// Step 2: structs to canonical XML. Same document, other wire format: the
	// discriminator becomes xsi:type and the element names match the JSON keys.
	xmlBytes, err := canxml.Marshal(&fromJSON)
	if err != nil {
		return fmt.Errorf("encode canonical XML: %w", err)
	}
	fmt.Printf("canonical XML: %d bytes\n", len(xmlBytes))
	fmt.Printf("  preview: %s\n", preview(xmlBytes, 200))

	// Step 3: XML back to structs, then structs back to JSON.
	var fromXML rm.Composition
	if err := canxml.Unmarshal(xmlBytes, &fromXML); err != nil {
		return fmt.Errorf("decode canonical XML: %w", err)
	}
	roundTripped, err := canjson.Marshal(&fromXML)
	if err != nil {
		return fmt.Errorf("re-encode canonical JSON: %w", err)
	}
	fmt.Printf("re-encoded JSON: %d bytes\n", len(roundTripped))

	// Step 4: compare start and end. The starting point is encoded through
	// the same codec as the end point, so the cassette's own formatting
	// (whitespace, member order) does not count; only the decoded content does.
	direct, err := canjson.Marshal(&fromJSON)
	if err != nil {
		return fmt.Errorf("encode canonical JSON: %w", err)
	}
	same, err := sameJSON(direct, roundTripped)
	if err != nil {
		return err
	}
	if !same {
		return errors.New("the JSON to XML to JSON trip changed the composition")
	}
	fmt.Println("OK: JSON ↔ XML cross-format round-trip preserves the Composition structurally")
	return nil
}

// preview returns the first n bytes of a document, with an ellipsis when
// something was cut, so the XML shape is visible without flooding the output.
func preview(doc []byte, n int) string {
	if len(doc) <= n {
		return string(doc)
	}
	return string(doc[:n]) + "..."
}

// sameJSON reports whether two JSON documents describe the same value tree.
// Null members are dropped before comparing because the SDK treats a null
// member and an absent member as the same thing, and the two codecs may pick
// either spelling for an empty optional field.
func sameJSON(a, b []byte) (bool, error) {
	var treeA, treeB any
	if err := json.Unmarshal(a, &treeA); err != nil {
		return false, fmt.Errorf("parse JSON for comparison: %w", err)
	}
	if err := json.Unmarshal(b, &treeB); err != nil {
		return false, fmt.Errorf("parse JSON for comparison: %w", err)
	}
	return reflect.DeepEqual(withoutNulls(treeA), withoutNulls(treeB)), nil
}

// withoutNulls returns a copy of a generic JSON tree with every null object
// member removed, at any depth.
func withoutNulls(v any) any {
	switch node := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for key, val := range node {
			if val != nil {
				out[key] = withoutNulls(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, item := range node {
			out[i] = withoutNulls(item)
		}
		return out
	default:
		return v
	}
}
