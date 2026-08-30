package examples

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// sampleMarker is the exact line docs/examples.md puts above a verbatim
// transcript. The extractor matches it literally: a section that reworded the
// marker must fail loudly rather than quietly drop out of verification.
const sampleMarker = "**Sample output:**"

// markerPrefix is what every sample-output marker in docs/examples.md starts
// with, verbatim or labelled. The marker census keys on it so a *new* spelling
// is caught as an unrecognised marker instead of passing unnoticed for being
// neither the bare form nor a known label.
const markerPrefix = "**Sample output"

// labelledMarkers are the exact spellings docs/examples.md uses when a block is
// deliberately not a full verbatim transcript. Transcribed from the file, not
// guessed: the label sits inside the bold run for the two "(abridged)" forms
// and outside it for contribution-build, so no prefix or contains test would
// classify all four. A section carrying one of these is not expected on the
// allowlist — its block is elided by design, and comparing it would fail.
var labelledMarkers = []string{
	"**Sample output (abridged):**",
	"**Sample output (abridged, keys sorted):**",
	"**Sample output** (body elided):",
}

// transcript pins one example program to the docs/examples.md section whose
// sample-output block has to reproduce that program's stdout.
type transcript struct {
	program string // directory name under cmd/examples/
	section string // the "### …" heading in docs/examples.md
}

// transcripts is the allowlist — an example is verified only when listed here.
// Every other directory under cmd/examples/ belongs in exclusions, and
// TestExampleDirectoryCensus fails if any directory is in neither or in both.
//
// An example that grows a verbatim block, or whose block stops being elided,
// belongs here. One whose output turns nondeterministic moves to exclusions
// instead — do not weaken the comparison to keep it passing.
var transcripts = []transcript{
	{program: "aql-build", section: "aql-build"},
	{program: "aql-parse-structured", section: "aql-parse-structured"},
	{program: "canonical_json", section: "canonical_json"},
	{program: "compile-build-validate", section: "compile-build-validate"},
	{program: "ehr_create", section: "ehr_create"},
	{program: "lint-aql", section: "lint-aql"},
}

// exclusions names every example that is deliberately NOT transcript-verified,
// against the one-line reason it cannot be. This is a structural census, not a
// comment: TestExampleDirectoryCensus reads it, so a new example directory that
// nobody classified fails by name instead of drifting out of coverage in
// silence.
//
// The abridged blocks could still be verified on their verbatim head, up to the
// first elision line — recorded follow-up, not yet built.
var exclusions = map[string]string{
	"canxml_roundtrip":     "no sample-output block in the catalog",
	"opt-parse":            "no sample-output block",
	"primitive-validate":   "no sample-output block",
	"validate-composition": "no sample-output block",
	"validate-from-json":   "no sample-output block",
	"generate-example":     "no sample-output block, and the output is nondeterministic anyway: fresh UUIDs per run and a wall-clock context start_time",
	"template-explore":     `block is labelled "(abridged)": the node tree is deliberately elided down to one branch`,
	"webtemplate-export":   `block is labelled "(abridged)": the form tree is deliberately elided`,
	"flat-roundtrip":       `block is labelled "(abridged, keys sorted)"`,
	"contribution-build":   `block is labelled "(body elided)": the printed Contribution_create JSON body is left out`,
	"smart-launch":         "nondeterministic: a random PKCE state and verifier per run, plus a wall-clock expires_at",
}

// bareMarkerException is the one section that publishes a bare
// "**Sample output:**" marker and still stays off the allowlist: smart-launch
// prints a fresh PKCE state and verifier plus a wall-clock expires_at, so no
// fixed block could ever match, yet the block is a real full transcript rather
// than an elided one and so carries no label. Recorded here and nowhere else —
// TestSampleMarkerCensus consults this constant, and checks it still names an
// excluded example that still carries a bare marker, so a stale exception fails
// rather than quietly excusing a section that no longer needs excusing.
const bareMarkerException = "smart-launch"

// exampleTimeout bounds each build and each run separately. `go run` gave the
// subtests no deadline at all; a program that blocks would have hung the
// package's test binary until the whole-run timeout killed it, with no clue
// which example was at fault. Generous on purpose — a cold build of the SDK
// takes far longer than the run it produces.
const exampleTimeout = 2 * time.Minute

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
		t.Skip("builds and runs example programs; skipped under -short")
	}

	root := moduleRoot(t)
	doc := docLines(t, root)

	for _, tc := range transcripts {
		t.Run(tc.program, func(t *testing.T) {
			t.Parallel()

			want := trimTrailingBlanks(extractSample(t, doc, tc.section))
			got := trimTrailingBlanks(runExample(t, root, buildExample(t, root, tc.program)))
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

// TestExampleDirectoryCensus asserts every example directory is classified
// exactly once — allowlisted for verification, or excluded with a reason. A
// new example that lands unclassified is the failure this test exists to
// catch: without it the program would simply never be checked against its
// documented output, and nothing would say so.
func TestExampleDirectoryCensus(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "cmd", "examples")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	allowed := map[string]bool{}
	for _, tc := range transcripts {
		if allowed[tc.program] {
			t.Errorf("cmd/examples/transcripts_test.go: %q is listed twice in the allowlist", tc.program)
		}
		allowed[tc.program] = true
	}

	var found int
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			// Matches what `go build ./cmd/examples/...` would consider a
			// package directory, so a data directory is not miscounted.
			continue
		}
		found++
		switch {
		case allowed[name] && exclusions[name] != "":
			t.Errorf("cmd/examples/%s is both allowlisted and excluded — it must be exactly one; "+
				"drop whichever entry is wrong in cmd/examples/transcripts_test.go", name)
		case allowed[name] || exclusions[name] != "":
			// Classified.
		default:
			t.Errorf("cmd/examples/%s is in neither the allowlist nor the exclusions of "+
				"cmd/examples/transcripts_test.go, so its documented output is unverified and "+
				"nothing says so. Add it to `transcripts` if docs/examples.md carries a verbatim "+
				"**Sample output:** block for it, or to `exclusions` with the reason it cannot be "+
				"compared.", name)
		}
	}

	for name := range exclusions {
		if !dirExists(filepath.Join(dir, name)) {
			t.Errorf("exclusions names %q, but cmd/examples/%s does not exist — the example was "+
				"renamed or removed and its exclusion outlived it", name, name)
		}
	}
	for _, tc := range transcripts {
		if !dirExists(filepath.Join(dir, tc.program)) {
			t.Errorf("the allowlist names %q, but cmd/examples/%s does not exist", tc.program, tc.program)
		}
	}
	if want := len(allowed) + len(exclusions); found != want {
		t.Errorf("cmd/examples/ holds %d example directories but the census classifies %d "+
			"(%d allowlisted + %d excluded)", found, want, len(allowed), len(exclusions))
	}
}

// TestSampleMarkerCensus asserts the docs side of the same question: every
// docs/examples.md section publishing a bare "**Sample output:**" block is
// verified, unless it is the single recorded exception. A section that grows a
// verbatim block without joining the allowlist is otherwise invisible — the
// per-example test only iterates what it already knows about.
func TestSampleMarkerCensus(t *testing.T) {
	root := moduleRoot(t)
	doc := docLines(t, root)

	allowed := map[string]bool{}
	for _, tc := range transcripts {
		allowed[tc.section] = true
	}

	section := ""
	exceptionSeen := false
	for i, line := range doc {
		if after, ok := strings.CutPrefix(line, "### "); ok {
			section = strings.TrimSpace(after)
			continue
		}
		text := strings.TrimSpace(line)
		if !strings.HasPrefix(text, markerPrefix) {
			continue
		}
		switch {
		case text != sampleMarker && !slices.Contains(labelledMarkers, text):
			t.Errorf("docs/examples.md § %s (line %d): %q is neither the bare %q marker nor one of "+
				"the recorded labelled spellings %q. A reworded marker leaves the block unverified "+
				"— restore a known spelling, or record the new one in labelledMarkers.",
				section, i+1, text, sampleMarker, labelledMarkers)
		case text != sampleMarker:
			// Labelled: the block is elided by design, so it is not expected
			// on the allowlist.
		case section == bareMarkerException:
			exceptionSeen = true
		case !allowed[section]:
			t.Errorf("docs/examples.md § %s (line %d) publishes a bare %q block, but the section is "+
				"not on the allowlist in cmd/examples/transcripts_test.go, so nothing checks it "+
				"against a real run. Add it to `transcripts`, label the block if it is elided, or "+
				"record it as the bare-marker exception if its output is nondeterministic.",
				section, i+1, sampleMarker)
		}
	}

	if !exceptionSeen {
		t.Errorf("bareMarkerException names %q, but docs/examples.md § %s no longer carries a bare "+
			"%q marker — the exception outlived what it excused; drop it",
			bareMarkerException, bareMarkerException, sampleMarker)
	}
	if exclusions[bareMarkerException] == "" {
		t.Errorf("bareMarkerException names %q, which is not in exclusions — the exception and the "+
			"reason it exists must be recorded together", bareMarkerException)
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

// docLines reads docs/examples.md into lines — the input every census and the
// extractor share.
func docLines(t *testing.T, root string) []string {
	t.Helper()

	docPath := filepath.Join(root, "docs", "examples.md")
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	return strings.Split(string(raw), "\n")
}

// dirExists reports whether path is a directory, so a census entry naming a
// removed example fails as a stale entry rather than as a missing program.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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

// buildExample compiles one example program into the test's scratch directory
// and returns the binary. Building first is what makes the run killable:
// `go run` only ever exposes its own process, so cancelling the context left
// the compiled grandchild running.
func buildExample(t *testing.T, root, program string) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), program)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	ctx, cancel := context.WithTimeout(t.Context(), exampleTimeout)
	defer cancel()

	pkg := "./cmd/examples/" + program
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, pkg)
	cmd.Dir = root

	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build -o %s %s (in %s): %v%s\nstderr:\n%s",
			bin, pkg, root, err, deadlineNote(ctx), stderr.String())
	}
	return bin
}

// runExample runs one built example program from the module root and returns
// its stdout. A non-zero exit is a test failure in its own right — a documented
// transcript implies the program still runs offline. The context bounds the run
// and, because bin is the real process rather than a `go run` parent, cancelling
// it actually kills what is hanging.
func runExample(t *testing.T, root, bin string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), exampleTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = root

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s (in %s): %v%s\nstderr:\n%s",
			filepath.Base(bin), root, err, deadlineNote(ctx), stderr.String())
	}
	return stdout.String()
}

// deadlineNote turns a killed-by-timeout into a sentence, so the failure says
// the example ran out of time instead of only reporting "signal: killed".
func deadlineNote(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf(" (killed after the %s per-example timeout)", exampleTimeout)
	}
	return ""
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
