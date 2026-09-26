// Decode an openEHR COMPOSITION from canonical JSON into the SDK's typed
// Reference Model (RM) structs and print a few of its fields. This is the
// smallest useful program in the SDK: no HTTP, no auth, no discovery, just
// bytes in and Go structs out.
//
// It runs offline against the vendored body_weight.json cassette:
//
//	go run ./cmd/examples/canonical_json
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// testkit/fixtures resolves the vendored cassettes relative to the module,
	// so the path is right whatever the working directory is.
	path := fixtures.CompositionJSON("body_weight")
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read cassette: %w", err)
	}

	// A COMPOSITION is the top-level clinical document in openEHR. canjson is
	// the canonical JSON codec: it reads the "_type" discriminator on every
	// object and fills the matching rm struct, polymorphic children included.
	var composition rm.Composition
	if err := canjson.Unmarshal(body, &composition); err != nil {
		return fmt.Errorf("decode canonical JSON: %w", err)
	}

	// The archetype node id names the archetype the document is built on.
	// Language and territory are CODE_PHRASE values: a code plus the
	// terminology it comes from. Content holds the entries (observations,
	// evaluations, ...) the document carries.
	fmt.Printf("composition: archetype_node_id=%s\n", composition.ArchetypeNodeID)
	fmt.Printf("  name=%q\n", composition.Name.GetValue())
	fmt.Printf("  language=%s (terminology=%s)\n",
		composition.Language.CodeString, composition.Language.TerminologyID.Value)
	fmt.Printf("  territory=%s\n", composition.Territory.CodeString)
	fmt.Printf("  category=%s\n", composition.Category.Value)
	fmt.Printf("  content items=%d\n", len(composition.Content))
	fmt.Println("OK: canonical-JSON Composition decoded from", filepath.Base(path))
	return nil
}
