// Decode an openEHR COMPOSITION from canonical JSON into the SDK's typed
// Reference Model (RM) structs and print a few of its fields. This is the
// smallest useful program in the SDK: no HTTP, no auth, no discovery, just
// bytes in and Go structs out.
//
// It runs offline against the vendored body_weight.json fixture, or against a
// canonical-JSON COMPOSITION file you name:
//
//	go run ./cmd/examples/canonical_json
//	go run ./cmd/examples/canonical_json composition.json
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

// run decodes the COMPOSITION in the file args names, or the vendored fixture
// when args is empty, and prints a few of its fields.
func run(args []string) error {
	path, err := compositionPath(args)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read composition: %w", err)
	}

	// A COMPOSITION is the top-level clinical document in openEHR. canjson is
	// the canonical JSON codec: it reads the "_type" discriminator on every
	// object and fills the matching rm struct, polymorphic children included.
	var composition rm.Composition
	if err := canjson.Unmarshal(body, &composition); err != nil {
		return fmt.Errorf("decode canonical JSON: %w", err)
	}

	// The archetype node id names the archetype the document is built on.
	// The name is a DV_TEXT or a DV_CODED_TEXT, held as an interface that is
	// nil when the document leaves it out. Language and territory are
	// CODE_PHRASE values: a code plus the terminology it comes from. Content
	// holds the entries (observations, evaluations, ...) the document carries.
	name := ""
	if composition.Name != nil {
		name = composition.Name.GetValue()
	}
	fmt.Printf("composition: archetype_node_id=%s\n", composition.ArchetypeNodeID)
	fmt.Printf("  name=%q\n", name)
	fmt.Printf("  language=%s (terminology=%s)\n",
		composition.Language.CodeString, composition.Language.TerminologyID.Value)
	fmt.Printf("  territory=%s\n", composition.Territory.CodeString)
	fmt.Printf("  category=%s\n", composition.Category.Value)
	fmt.Printf("  content items=%d\n", len(composition.Content))
	fmt.Println("OK: canonical-JSON Composition decoded from", filepath.Base(path))
	return nil
}

// compositionPath picks the file to decode: the one the caller named, or the
// vendored body_weight.json fixture when there is none.
func compositionPath(args []string) (string, error) {
	switch len(args) {
	case 0:
		// testkit/fixtures resolves the vendored fixtures relative to the
		// module, so the path is right whatever the working directory is.
		return fixtures.CompositionJSON("body_weight"), nil
	case 1:
		return args[0], nil
	default:
		return "", errors.New("usage: canonical_json [composition.json]")
	}
}
