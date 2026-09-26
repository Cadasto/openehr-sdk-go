package examples

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mirror is a copy of one example's output published outside
// docs/examples.md. The README and the site page show the canonical_json
// output to a first-time reader, so a drift there is as visible as one in the
// catalogue, and TestDocsExamplesTranscripts reads only the catalogue.
type mirror struct {
	file    string // path relative to the module root
	program string // directory name under cmd/examples/
	// anchor is the exact line that introduces the output. When the output
	// sits in its own fence after the command's fence, fenced is true and the
	// block is the next fence after the anchor; otherwise the output starts on
	// the line after the anchor, inside the same fence.
	anchor string
	fenced bool
}

var mirrors = []mirror{
	{file: "README.md", program: "canonical_json", anchor: "$ go run ./cmd/examples/canonical_json"},
	{file: "pages/examples.md", program: "canonical_json", anchor: "go run ./cmd/examples/canonical_json", fenced: true},
}

// TestMirroredSamples holds every mirrored copy of an example's output to a
// real run of the program, the same bar the catalogue's transcripts meet.
func TestMirroredSamples(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs example programs; skipped under -short")
	}

	root := moduleRoot(t)
	for _, m := range mirrors {
		t.Run(m.file+"/"+m.program, func(t *testing.T) {
			t.Parallel()

			want := trimTrailingBlanks(extractMirror(t, root, m))
			got := trimTrailingBlanks(runExample(t, root, buildExample(t, root, m.program)))
			if want == got {
				return
			}
			t.Errorf("%s no longer shows what `go run ./cmd/examples/%s` prints.\n%s"+
				"Copy the block from a real run.", m.file, m.program, diffTranscript(want, got))
		})
	}
}

// extractMirror returns the output block m points at. A missing anchor, a
// missing output fence or an unclosed fence fails the test, so a reworded
// page cannot silently drop out of the check.
func extractMirror(t *testing.T, root string, m mirror) string {
	t.Helper()

	path := filepath.Join(root, m.file)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(raw), "\n")

	start := -1
	for i, line := range lines {
		if line == m.anchor {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s: no line %q; the sample was moved or reworded, update mirrors", m.file, m.anchor)
	}
	if m.fenced {
		// Only the command fence's closing line and blank lines may sit
		// between the anchor and the output fence; anything else means the
		// output block is not the one right after the command.
		for start < len(lines) && !strings.HasPrefix(lines[start], "```text") {
			if line := strings.TrimSpace(lines[start]); line != "" && line != "```" {
				t.Fatalf("%s: expected a ```text fence right after %q, found %q", m.file, m.anchor, line)
			}
			start++
		}
		if start == len(lines) {
			t.Fatalf("%s: no ```text fence after %q", m.file, m.anchor)
		}
		start++
	}
	for end := start; end < len(lines); end++ {
		if strings.HasPrefix(lines[end], "```") {
			return strings.Join(lines[start:end], "\n") + "\n"
		}
	}
	t.Fatalf("%s: the fence after %q is never closed", m.file, m.anchor)
	return ""
}
