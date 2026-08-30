package examples

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sampleMarker is the exact line docs/examples.md puts above a verbatim
// transcript. The extractor matches it literally: a section that reworded the
// marker must fail loudly rather than quietly drop out of verification.
const sampleMarker = "**Sample output:**"

// transcript pins one example program to the docs/examples.md section whose
// sample-output block has to reproduce that program's stdout.
type transcript struct {
	program string // directory name under cmd/examples/
	section string // the "### …" heading in docs/examples.md
}

// transcripts is the allowlist — an example is verified only when listed here.
//
// All 17 programs under cmd/examples/ are accounted for; these 6 publish a
// verbatim, reproducible block. The other 11 are excluded, and why:
//
//	canxml_roundtrip     — no sample-output block in the catalog
//	opt-parse            — no sample-output block
//	primitive-validate   — no sample-output block
//	validate-composition — no sample-output block
//	validate-from-json   — no sample-output block
//	generate-example     — no sample-output block, and the output is
//	                       nondeterministic anyway (fresh UUIDs per run and a
//	                       wall-clock context start_time)
//	template-explore     — block is labelled "(abridged)": the node tree is
//	                       deliberately elided down to one branch
//	webtemplate-export   — block is labelled "(abridged)": the form tree is
//	                       deliberately elided
//	flat-roundtrip       — block is labelled "(abridged, keys sorted)"
//	contribution-build   — block is labelled "(body elided)": the printed
//	                       Contribution_create JSON body is left out
//	smart-launch         — nondeterministic: a random PKCE state and verifier
//	                       per run, plus a wall-clock expires_at
//
// The abridged blocks could still be verified on their verbatim head, up to the
// first elision line — recorded follow-up, not yet built.
//
// An example that grows a verbatim block, or whose block stops being elided,
// belongs here. One whose output turns nondeterministic belongs in the list
// above instead — do not weaken the comparison to keep it passing.
var transcripts = []transcript{
	{program: "aql-build", section: "aql-build"},
	{program: "aql-parse-structured", section: "aql-parse-structured"},
	{program: "canonical_json", section: "canonical_json"},
	{program: "compile-build-validate", section: "compile-build-validate"},
	{program: "ehr_create", section: "ehr_create"},
	{program: "lint-aql", section: "lint-aql"},
}

// TestDocsExamplesTranscripts checks every allowlisted sample-output block in
// docs/examples.md against a real run of the program it documents.
//
// The catalog's transcripts are load-bearing developer documentation, and they
// have drifted silently before — REQ-164's new path-shape codes changed the
// lint-aql output without anything failing. AGENTS.md requires cmd/examples/
// docs to move in the same PR as the program; this test is the mechanical half
// of that rule, and needs no CI wiring beyond `go test ./...`.
func TestDocsExamplesTranscripts(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs example programs through `go run`; skipped under -short")
	}

	root := moduleRoot(t)
	docPath := filepath.Join(root, "docs", "examples.md")
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	doc := strings.Split(string(raw), "\n")

	for _, tc := range transcripts {
		t.Run(tc.program, func(t *testing.T) {
			t.Parallel()

			want := trimTrailingBlanks(extractSample(t, doc, tc.section))
			got := trimTrailingBlanks(runExample(t, root, tc.program))
			if want == got {
				return
			}
			t.Errorf("docs/examples.md § %s no longer matches `go run ./cmd/examples/%s`.\n%s"+
				"Regenerate the block from a real run, or exclude the example in the allowlist if "+
				"its output became nondeterministic.",
				tc.section, tc.program, diffTranscript(want, got))
		})
	}
}

// moduleRoot resolves the module root from this file's own location, so the
// test does not depend on the working directory `go test` happened to pick.
func moduleRoot(t *testing.T) string {
	t.Helper()

	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at the resolved module root %s: %v", root, err)
	}
	return root
}

// extractSample returns the fenced block that follows sampleMarker inside the
// named "### …" section. Every step is strict: a missing section, a reworded
// marker, or an unclosed fence fails the test naming what moved, because the
// alternative is a transcript that silently stops being checked.
func extractSample(t *testing.T, doc []string, section string) string {
	t.Helper()

	heading := "### " + section
	start := -1
	for i, line := range doc {
		if line != heading {
			continue
		}
		if start >= 0 {
			t.Fatalf("docs/examples.md: %q appears at both line %d and line %d; the section must be unique",
				heading, start+1, i+1)
		}
		start = i
	}
	if start < 0 {
		t.Fatalf("docs/examples.md: no %q heading — the section was renamed or removed; "+
			"fix the heading or update the allowlist in cmd/examples/transcripts_test.go", heading)
	}

	// A section runs to the next heading of any level.
	end := len(doc)
	for i := start + 1; i < len(doc); i++ {
		if strings.HasPrefix(doc[i], "## ") || strings.HasPrefix(doc[i], "### ") {
			end = i
			break
		}
	}

	marker := -1
	for i := start + 1; i < end; i++ {
		if strings.TrimSpace(doc[i]) == sampleMarker {
			marker = i
			break
		}
	}
	if marker < 0 {
		t.Fatalf("docs/examples.md § %s (line %d): no %q line before the next heading — the marker "+
			"was reworded, which would leave the transcript unverified", section, start+1, sampleMarker)
	}

	openFence := -1
	for i := marker + 1; i < end; i++ {
		if strings.HasPrefix(doc[i], "```") {
			openFence = i
			break
		}
	}
	if openFence < 0 {
		t.Fatalf("docs/examples.md § %s: %q at line %d is not followed by a fenced block before the "+
			"next heading", section, sampleMarker, marker+1)
	}

	closeFence := -1
	for i := openFence + 1; i < end; i++ {
		if strings.TrimSpace(doc[i]) == "```" {
			closeFence = i
			break
		}
	}
	if closeFence < 0 {
		t.Fatalf("docs/examples.md § %s: the fence opened at line %d is never closed before the next "+
			"heading", section, openFence+1)
	}

	return strings.Join(doc[openFence+1:closeFence], "\n")
}

// runExample runs one example program from the module root and returns its
// stdout. A non-zero exit is a test failure in its own right — a documented
// transcript implies the program still runs offline.
func runExample(t *testing.T, root, program string) string {
	t.Helper()

	pkg := "./cmd/examples/" + program
	cmd := exec.CommandContext(t.Context(), "go", "run", pkg)
	cmd.Dir = root

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run %s (in %s): %v\nstderr:\n%s", pkg, root, err, stderr.String())
	}
	return stdout.String()
}

// trimTrailingBlanks drops trailing newlines from both sides of the
// comparison. What a fenced block cannot represent is the final line
// terminator a program's last Println writes — the closing fence stands where
// that byte would be — so no transcript would ever match exactly without this.
// Trimming every trailing newline rather than exactly one is looser than that
// justifies: a fence can hold a blank line before its closing delimiter, so a
// trailing blank line the program prints (aql-build and lint-aql both do) goes
// unguarded. That is the accepted trade-off; leading and interior blank lines
// are still compared exactly.
func trimTrailingBlanks(s string) string {
	return strings.TrimRight(s, "\n")
}

// diffTranscript renders the first line-level difference with two lines of
// context either side. Dumping both transcripts whole would bury the one line
// that actually moved.
func diffTranscript(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	longest := max(len(wantLines), len(gotLines))
	at := -1
	for i := range longest {
		w, wOK := lineAt(wantLines, i)
		g, gOK := lineAt(gotLines, i)
		if w != g || wOK != gOK {
			at = i
			break
		}
	}
	if at < 0 {
		return "  (no line-level difference; the transcripts differ only in trailing whitespace)\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  first difference at line %d — the doc block has %d line(s), the program printed %d\n",
		at+1, len(wantLines), len(gotLines))
	b.WriteString("  --- docs/examples.md\n  +++ go run output\n")
	for i := max(at-2, 0); i < min(at+3, longest); i++ {
		w, wOK := lineAt(wantLines, i)
		g, gOK := lineAt(gotLines, i)
		if wOK && gOK && w == g {
			fmt.Fprintf(&b, "    %s\n", w)
			continue
		}
		fmt.Fprintf(&b, "  - %s\n", absentOr(w, wOK, i))
		fmt.Fprintf(&b, "  + %s\n", absentOr(g, gOK, i))
	}
	return b.String()
}

// lineAt reports the i-th line and whether it exists at all, so a transcript
// that merely ends early is distinguishable from one holding a blank line.
func lineAt(lines []string, i int) (string, bool) {
	if i >= len(lines) {
		return "", false
	}
	return lines[i], true
}

func absentOr(line string, present bool, i int) string {
	if !present {
		return fmt.Sprintf("<no line %d>", i+1)
	}
	return line
}
