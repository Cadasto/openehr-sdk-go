package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

// The learning order has its canonical home in docs/examples.md § Suggested
// learning order, one "N. program  ← note" line per step in a text fence.
// The site's examples page repeats it after siteLearningOrderAnchor as a
// numbered list whose items end with the program in backticks.
const (
	learningOrderHeading    = "## Suggested learning order"
	siteLearningOrderAnchor = "If you are new to the SDK, try them in this order:"
)

var (
	canonicalStep = regexp.MustCompile(`^(\d+)\.\s+(\S+)`)
	siteStep      = regexp.MustCompile("^(\\d+)\\. .+ \\(`([^`]+)`\\)$")
)

// TestLearningOrderMirrored holds the site's learning order to the
// catalogue's: the same programs in the same order, each a directory under
// cmd/examples/.
func TestLearningOrderMirrored(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	sitePath := filepath.Join(root, "pages", "examples.md")
	raw, err := os.ReadFile(sitePath)
	if err != nil {
		t.Fatalf("read %s: %v", sitePath, err)
	}
	order, err := checkLearningOrder(docLines(t, root), strings.Split(string(raw), "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, program := range order {
		if !dirExists(filepath.Join(root, "cmd", "examples", program)) {
			t.Errorf("docs/examples.md § Suggested learning order names %q, but cmd/examples/%s does not exist",
				program, program)
		}
	}
}

// TestLearningOrderDriftCaught is the can-fail control of
// TestLearningOrderMirrored: a site list that drops, adds or reorders a
// program, or names none, is reported, and only the matching list passes.
func TestLearningOrderDriftCaught(t *testing.T) {
	t.Parallel()

	doc := []string{learningOrderHeading, "", "```text", "1. canonical_json   ← basics", "2. opt-parse", "```"}
	site := func(items ...string) []string {
		return append([]string{siteLearningOrderAnchor, ""}, items...)
	}
	const (
		decode = "Decode canonical JSON (`canonical_json`)"
		read   = "Read an operational template (`opt-parse`)"
		build  = "Build an AQL query (`aql-build`)"
	)
	tests := []struct {
		name    string
		doc     []string
		site    []string
		wantErr string // a fragment of the error; empty when the lists match
	}{
		{name: "same order", doc: doc, site: site("1. "+decode, "2. "+read, "")},
		{name: "a program dropped", doc: doc, site: site("1. " + decode), wantErr: "copy the order"},
		{name: "a program added", doc: doc, site: site("1. "+decode, "2. "+read, "3. "+build), wantErr: "copy the order"},
		{name: "two programs swapped", doc: doc, site: site("1. "+read, "2. "+decode), wantErr: "copy the order"},
		{name: "an item naming no program", doc: doc, site: site("1. Decode canonical JSON", "2. "+read), wantErr: "is not step 1"},
		{name: "a step number skipped", doc: doc, site: site("1. "+decode, "3. "+read), wantErr: "is not step 2"},
		{name: "no site list", doc: doc, site: []string{"# Examples"}, wantErr: "no line"},
		{name: "no canonical order", doc: []string{"# Examples"}, site: site("1. " + decode), wantErr: "no \"## Suggested"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := checkLearningOrder(tc.doc, tc.site)
			if tc.wantErr == "" {
				if want := []string{"canonical_json", "opt-parse"}; err != nil || !slices.Equal(got, want) {
					t.Errorf("checkLearningOrder = %q, %v; want %q, nil", got, err, want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("checkLearningOrder = %q, %v; want an error containing %q", got, err, tc.wantErr)
			}
		})
	}
}

// checkLearningOrder returns the canonical learning order when the site's
// copy lists the same programs in the same order, and an error naming the
// difference otherwise.
func checkLearningOrder(doc, site []string) ([]string, error) {
	want, err := canonicalLearningOrder(doc)
	if err != nil {
		return nil, fmt.Errorf("docs/examples.md: %w", err)
	}
	got, err := siteLearningOrder(site)
	if err != nil {
		return nil, fmt.Errorf("pages/examples.md: %w", err)
	}
	if !slices.Equal(got, want) {
		return nil, fmt.Errorf("pages/examples.md lists the learning order\n\t%s\n"+
			"but docs/examples.md § Suggested learning order, the canonical one, lists\n\t%s\n"+
			"copy the order from docs/examples.md", strings.Join(got, ", "), strings.Join(want, ", "))
	}
	return want, nil
}

// canonicalLearningOrder reads the programs from the text fence under
// learningOrderHeading. A missing heading or fence, a line that is not the
// next step, or an unclosed fence is an error, so a reworded section cannot
// drop out of the check.
func canonicalLearningOrder(lines []string) ([]string, error) {
	i := slices.Index(lines, learningOrderHeading)
	if i < 0 {
		return nil, fmt.Errorf("no %q heading", learningOrderHeading)
	}
	for i++; i < len(lines) && strings.TrimSpace(lines[i]) == ""; i++ {
	}
	if i == len(lines) || lines[i] != "```text" {
		return nil, fmt.Errorf("no ```text fence right after %q", learningOrderHeading)
	}
	var order []string
	for i++; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "```") {
			if len(order) == 0 {
				return nil, fmt.Errorf("the fence after %q lists no program", learningOrderHeading)
			}
			return order, nil
		}
		m := canonicalStep.FindStringSubmatch(lines[i])
		if m == nil || m[1] != strconv.Itoa(len(order)+1) {
			return nil, fmt.Errorf("line %d, %q, is not step %d of the learning order", i+1, lines[i], len(order)+1)
		}
		order = append(order, m[2])
	}
	return nil, fmt.Errorf("the fence after %q is never closed", learningOrderHeading)
}

// siteLearningOrder reads the programs from the numbered list after
// siteLearningOrderAnchor; the list ends at the first blank line. A missing
// anchor, an empty list, or an item that is not the next step naming its
// program in backticks is an error.
func siteLearningOrder(lines []string) ([]string, error) {
	i := slices.Index(lines, siteLearningOrderAnchor)
	if i < 0 {
		return nil, fmt.Errorf("no line %q", siteLearningOrderAnchor)
	}
	for i++; i < len(lines) && strings.TrimSpace(lines[i]) == ""; i++ {
	}
	var order []string
	for ; i < len(lines) && strings.TrimSpace(lines[i]) != ""; i++ {
		m := siteStep.FindStringSubmatch(lines[i])
		if m == nil || m[1] != strconv.Itoa(len(order)+1) {
			return nil, fmt.Errorf("line %d, %q, is not step %d of the learning order, written as %q",
				i+1, lines[i], len(order)+1, strconv.Itoa(len(order)+1)+". Title (`program`)")
		}
		order = append(order, m[2])
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("no numbered list after %q", siteLearningOrderAnchor)
	}
	return order, nil
}
