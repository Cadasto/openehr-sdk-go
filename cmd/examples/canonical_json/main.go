// Example: decode a canonical-JSON Composition cassette and print
// a few key fields. Demonstrates the smallest building-block path
// (REQ-013) — no transport, no auth, no discovery: just RM types +
// canjson against bytes.
//
// Run: `go run ./cmd/examples/canonical_json` from any directory.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

func main() {
	body := loadCassette()
	var c rm.Composition
	if err := canjson.Unmarshal(body, &c); err != nil {
		log.Fatalf("canjson decode: %v", err)
	}
	fmt.Printf("composition: archetype_node_id=%s\n", c.ArchetypeNodeID)
	fmt.Printf("  name=%q\n", c.Name.Value)
	fmt.Printf("  language=%s (terminology=%s)\n", c.Language.CodeString, c.Language.TerminologyID.Value)
	fmt.Printf("  territory=%s\n", c.Territory.CodeString)
	fmt.Printf("  category=%s\n", c.Category.Value)
	fmt.Printf("  content items=%d\n", len(c.Content))
	fmt.Println("OK: canonical-JSON Composition decoded from", filepath.Base(cassettePath()))
}

// cassettePath resolves body_weight.json relative to THIS source
// file so `go run ./cmd/examples/canonical_json` works regardless of
// CWD. Mirror of the pattern in canjson/roundtrip_test.go.
func cassettePath() string {
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("runtime.Caller(0) failed — cannot resolve cassette path")
	}
	return filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "canonical_json", "body_weight.json")
}

func loadCassette() []byte {
	b, err := os.ReadFile(cassettePath())
	if err != nil {
		log.Fatalf("read cassette: %v", err)
	}
	return b
}
