//go:build ignore

// The generator of this example's default fixture,
// testdata/minimal_blood_pressure.json. After changing it, rerun it from this
// directory with go run gen_fixture.go.
//
// The composition and its encoding live in internal/bpfixture, where the
// build compiles them, and a test in this directory fails when the committed
// file is not what this program writes.
//
// The generator checks the document it is about to write, not the
// composition it built. validation.ValidateJSON decodes the encoded bytes and
// checks, in one call, the members the document leaves out, the constraints of
// vital_signs.opt and the RM floor. The generator refuses to write a fixture
// that does not decode or that the call reports an error on.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/cadasto/openehr-sdk-go/cmd/examples/validate-from-json/internal/bpfixture"
)

func main() {
	b, err := bpfixture.Document()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("testdata/minimal_blood_pressure.json", b, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote testdata/minimal_blood_pressure.json (%d bytes)\n", len(b))
}
