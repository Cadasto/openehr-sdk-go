// Example: JSON → struct → XML → struct → JSON round-trip. Decodes a
// vendored canonical-JSON Composition cassette through canjson,
// re-encodes it as canonical XML via canxml, decodes the XML back,
// and re-encodes as JSON. Demonstrates the cross-format invariant
// the canxml plan validates (REQ-056).
//
// Run: `go run ./cmd/examples/canxml_roundtrip` from any directory.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	body := loadCassette()
	fmt.Printf("input JSON: %d bytes\n", len(body))

	// JSON → struct A
	var a rm.Composition
	if err := canjson.Unmarshal(body, &a); err != nil {
		log.Fatalf("canjson decode: %v", err)
	}
	// A → XML bytes
	xb, err := canxml.Marshal(&a)
	if err != nil {
		log.Fatalf("canxml encode: %v", err)
	}
	fmt.Printf("canonical XML: %d bytes\n", len(xb))
	preview := string(xb)
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	fmt.Printf("  preview: %s\n", preview)

	// XML → struct C → JSON bytes
	var c rm.Composition
	if err := canxml.Unmarshal(xb, &c); err != nil {
		log.Fatalf("canxml decode: %v", err)
	}
	jd, err := canjson.Marshal(&c)
	if err != nil {
		log.Fatalf("canjson re-encode: %v", err)
	}
	fmt.Printf("re-encoded JSON: %d bytes\n", len(jd))

	// Compare A vs round-tripped JSON structurally (null/absent
	// normalised on both sides).
	ja, err := canjson.Marshal(&a)
	if err != nil {
		log.Fatalf("canjson encode for A: %v", err)
	}
	if !jsonEqualNorm(ja, jd) {
		log.Fatalf("cross-format invariant violated — JSON round-trip diverged from JSON→XML→JSON")
	}
	fmt.Println("OK: JSON ↔ XML cross-format round-trip preserves the Composition structurally")
}

func loadCassette() []byte {
	path := fixtures.CompositionJSON("body_weight")
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read cassette: %v", err)
	}
	return b
}

// jsonEqualNorm parses two canonical-JSON blobs, strips nil-valued
// map entries (the SDK treats null and absent equivalently), and
// compares the resulting trees. Identical to the test-suite helper
// at canxml/crossformat_test.go.
func jsonEqualNorm(a, b []byte) bool {
	av, err := parseAndStrip(a)
	if err != nil {
		return false
	}
	bv, err := parseAndStrip(b)
	if err != nil {
		return false
	}
	return jsonEqual(av, bv)
}

func parseAndStrip(data []byte) (any, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return stripNulls(v), nil
}

func stripNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if val == nil {
				continue
			}
			cleaned := stripNulls(val)
			if cleaned == nil {
				continue
			}
			out[k] = cleaned
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripNulls(e)
		}
		return out
	default:
		return v
	}
}

func jsonEqual(a, b any) bool {
	switch ax := a.(type) {
	case map[string]any:
		bx, ok := b.(map[string]any)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for k, av := range ax {
			bv, ok := bx[k]
			if !ok || !jsonEqual(av, bv) {
				return false
			}
		}
		return true
	case []any:
		bx, ok := b.([]any)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for i := range ax {
			if !jsonEqual(ax[i], bx[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
