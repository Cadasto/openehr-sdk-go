package templatecompile_test

// REQ-116 Phase 3 — the compiled-path regression guard.
//
// Emitting name predicates changes compiled AQL paths, and those paths
// are consumed by REQ-102 validation, REQ-107 instance generation and
// REQ-053 FLAT/STRUCTURED. The guard partitions the vendored corpus by
// the only thing that may cause a path to move — whether the OPT pins a
// template-level node name anywhere — and snapshots both halves:
//
//   - no-name.txt   templates pinning no name. These paths MUST stay
//     byte-identical across the whole of REQ-116. A diff here is a
//     regression, never an expected delta.
//   - pins-name.txt templates pinning at least one name. These paths
//     are expected to gain predicates in Phase 3; the diff on this file
//     IS the deliberately recorded delta, reviewed rather than assumed.
//
// The partition is structural (computed from NodeName(), not a hand-kept
// list), so a newly vendored OPT lands in the right half automatically.
// Regenerate both with:
//
//	go test ./internal/templatecompile/ -run TestCompiledPathSnapshot -update

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templatecompile/walk"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

var updateSnapshot = flag.Bool("update", false, "update the compiled-path snapshots")

// optPaths compiles one vendored OPT and returns its compiled AQL paths
// in document order together with whether any node pins a name. Paths
// are emitted per *node*, not per distinct path: sibling nodes sharing a
// path each contribute a line, which is what makes a predicate landing
// on one of them visible in the diff.
func optPaths(path string) (paths []string, pinsName bool, err error) {
	parsed, err := template.ParseFile(path)
	if err != nil {
		return nil, false, err
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		return nil, false, err
	}
	err = walk.Walk(c, walk.VisitorFunc{Pre: func(ctx *walk.Context) error {
		paths = append(paths, ctx.Node().AQLPath())
		if ctx.Node().NodeName() != "" {
			pinsName = true
		}
		return nil
	}})
	if err != nil {
		return nil, false, err
	}
	return paths, pinsName, nil
}

func TestCompiledPathSnapshot(t *testing.T) {
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("no vendored OPTs found — fixture layout changed?")
	}

	var withName, withoutName bytes.Buffer
	var nWith, nWithout, nUnparsed int
	for _, ref := range refs {
		paths, pinsName, err := optPaths(ref.Path)
		out := &withoutName
		if pinsName {
			out = &withName
		}
		if err != nil {
			// Recorded, not skipped: an OPT that starts or stops
			// parsing is a corpus change worth surfacing here.
			nUnparsed++
			fmt.Fprintf(&withoutName, "## %s\n!! does not compile: %v\n\n", ref.Name, err)
			continue
		}
		if pinsName {
			nWith++
		} else {
			nWithout++
		}
		fmt.Fprintf(out, "## %s  nodes=%d\n", ref.Name, len(paths))
		for _, p := range paths {
			fmt.Fprintf(out, "%s\n", p)
		}
		out.WriteByte('\n')
	}

	census := fmt.Sprintf("# corpus: %d OPTs — %d pin a name, %d pin none, %d do not compile\n",
		len(refs), nWith, nWithout, nUnparsed)

	for _, tc := range []struct {
		file   string
		header string
		body   *bytes.Buffer
	}{
		{
			file: "no-name.txt",
			header: "# REQ-116 compiled-path snapshot — templates pinning NO node name.\n" +
				"# These paths must stay byte-identical for the whole of REQ-116:\n" +
				"# a diff here is a regression, not an expected delta.\n",
			body: &withoutName,
		},
		{
			file: "pins-name.txt",
			header: "# REQ-116 compiled-path snapshot — templates pinning at least one node name.\n" +
				"# Phase 3 adds name predicates to these paths; the diff on this file\n" +
				"# is the deliberately recorded delta.\n",
			body: &withName,
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			var got bytes.Buffer
			got.WriteString(tc.header)
			got.WriteString(census)
			got.WriteByte('\n')
			got.Write(tc.body.Bytes())

			golden := filepath.Join("testdata", "pathsnapshot", tc.file)
			if *updateSnapshot {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read snapshot (run with -update): %v", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("compiled paths differ from %s.\n"+
					"If this is no-name.txt, a template that pins no name changed shape — that is a REQ-116 regression.\n"+
					"Regenerate with: go test ./internal/templatecompile/ -run TestCompiledPathSnapshot -update",
					golden)
			}
		})
	}
}
