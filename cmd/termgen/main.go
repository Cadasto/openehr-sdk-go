// termgen generates the openehr/terminology tables from the pinned openEHR
// Terminology XML.
//
// Usage:
//
//	termgen [flags]
//	  -resources string   path to resources/terminology/ directory
//	                      (default "./resources/terminology")
//	  -out string         output module root (default ".")
//	  -verify             do not write; instead compare with the file on
//	                      disk; exit 1 on drift
//
// The generator reads <resources>/openehr_terminology.xml and the `ref:` line
// of <resources>/MANIFEST.txt and emits
// <out>/openehr/terminology/openehr_gen.go — one variable per terminology
// group and code set, plus the release version and the pin's sha256.
// `make termgen` regenerates it; `make termgen-verify` fails the build when
// it drifts from the pin. See docs/specifications/rm-modeling.md § REQ-034
// for the contract and internal/termgen for the implementation.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cadasto/openehr-sdk-go/internal/termgen"
)

func main() {
	var (
		resources = flag.String("resources", "./resources/terminology", "path to resources/terminology/ directory")
		out       = flag.String("out", ".", "output module root")
		verify    = flag.Bool("verify", false, "verify; do not write; exit 1 on drift")
	)
	flag.Parse()

	result, err := termgen.Run(termgen.Options{
		ResourcesDir: *resources,
		OutDir:       *out,
		Verify:       *verify,
		Stderr:       os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "termgen:", err)
		os.Exit(2)
	}
	if result.Drift {
		fmt.Fprintln(os.Stderr, "termgen: drift detected in openehr/terminology/openehr_gen.go — run 'make termgen'")
		os.Exit(1)
	}
}
