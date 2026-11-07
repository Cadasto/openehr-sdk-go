package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// failingRun pins one failing run that docs/examples.md describes in prose
// rather than in a sample-output block: the program, its arguments, the issue
// lines the run prints, and the sentences that describe them.
//
// The test checks both sides. The run must exit 1 and print exactly the
// issues listed here, and the docs must still say what is listed in claims.
// When either side moves, update the other and this entry with it.
type failingRun struct {
	program string   // directory name under cmd/examples/
	args    []string // the arguments the docs give the run
	// issues is every issue line the run prints, as its path and its code, in
	// any order. The detail text is not compared: the docs do not quote it.
	issues []issueLine
	// lines are other lines of stdout the docs describe, verbatim.
	lines []string
	// claims are the phrases of the docs that describe this run, keyed by the
	// file that carries them. A phrase matches across line breaks: runs of
	// white space compare as one space.
	claims map[string][]string
}

// issueLine is one "  path [code] detail" line of a validation report.
type issueLine struct {
	path, code string
}

// failingRuns lists the documented failing runs. Each program is also on the
// transcripts allowlist, whose build-input tracking covers it.
var failingRuns = []failingRun{
	{
		program: "validate-composition",
		args:    []string{"-invalid"},
		issues: []issueLine{
			{path: "/category", code: "required"},
			{path: "/category/defining_code", code: "code_not_in_value_set"},
		},
		claims: map[string][]string{
			"docs/examples.md": {
				"With `-invalid` the program clears the composition's category before validating, and exits 1.",
				"the result reports one `required` issue at `/category`.",
				"The RM floor also reports `code_not_in_value_set` at `/category/defining_code`",
			},
		},
	},
	{
		program: "validate-from-json",
		args:    []string{"-corpus"},
		issues: []issueLine{
			{path: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value", code: "primitive_out_of_range"},
			{path: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0005]/value", code: "primitive_unit_unknown"},
			{path: "/content[openEHR-EHR-OBSERVATION.body_temperature.v1]/data/events[at0003]/data/item/value", code: "primitive_unit_unknown"},
		},
		lines: []string{
			"result               : not valid",
			"note                 : vital_signs.json is demo CDR data; issues are expected",
		},
		claims: map[string][]string{
			"docs/examples.md": {
				"It is 0 when validation reports no error, and 1 when it does.",
				"With `-corpus` validation reports 3 issues, one `path [code] detail` line each: " +
					"a magnitude out of range (`primitive_out_of_range`) and two units the template does not allow (`primitive_unit_unknown`).",
				"The result line says the composition is not valid, and a note says the issues are expected.",
			},
			"pages/examples.md": {
				"The exit status is 1 when an error is found",
				"`-corpus` validates demo data that reports issues",
			},
		},
	},
}

// TestDocsFailingRuns runs each failing run docs/examples.md and
// pages/examples.md describe, and holds the exit status, the issue codes,
// paths and count, and the other lines the docs name to a real run.
//
// TestDocsExamplesTranscripts runs every program with no arguments and
// expects exit 0, so without this test nothing checks the failing runs the
// docs describe; their description has changed before with nothing to catch
// it.
func TestDocsFailingRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs example programs; skipped under -short")
	}

	root := moduleRoot(t)
	trackBuildInputs(t, root)
	trackRunInputs(t, root)

	docs := map[string]string{}
	for _, name := range []string{"docs/examples.md", "pages/examples.md"} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		docs[name] = collapseSpace(string(raw))
	}

	for _, tc := range failingRuns {
		t.Run(tc.program+" "+strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()

			if !slices.ContainsFunc(transcripts, func(tr transcript) bool { return tr.program == tc.program }) {
				t.Fatalf("cmd/examples/%s is not on the transcripts allowlist, so the build inputs "+
					"of this run are not tracked; allowlist it or track it here", tc.program)
			}
			for file, phrases := range tc.claims {
				for _, phrase := range phrases {
					if !strings.Contains(docs[file], collapseSpace(phrase)) {
						t.Errorf("%s no longer says %q. If the account of `%s %s` changed, "+
							"update its entry in failingRuns to match a real run.",
							file, phrase, tc.program, strings.Join(tc.args, " "))
					}
				}
			}

			stdout, stderr, status := runExampleStatus(t, root, buildExample(t, root, tc.program), tc.args...)
			if status != 1 {
				t.Fatalf("%s %q exited with status %d, want 1\nstdout:\n%s\nstderr:\n%s",
					tc.program, tc.args, status, stdout, stderr)
			}

			got, header := reportedIssues(stdout)
			if !sameIssues(got, tc.issues) {
				t.Errorf("%s %q printed issues %v, want %v (in any order)\nstdout:\n%s",
					tc.program, tc.args, got, tc.issues, stdout)
			}
			if want := issueHeader(len(tc.issues)); header != want {
				t.Errorf("%s %q printed the verdict line %q, want %q", tc.program, tc.args, header, want)
			}
			for _, line := range tc.lines {
				if !slices.Contains(strings.Split(stdout, "\n"), line) {
					t.Errorf("%s %q did not print the line %q\nstdout:\n%s", tc.program, tc.args, line, stdout)
				}
			}
		})
	}
}

// reportedIssues reads a validation report from stdout: the issue lines,
// each "  path [code] detail", and the verdict line that heads them.
func reportedIssues(stdout string) (issues []issueLine, header string) {
	for line := range strings.Lines(stdout) {
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, "validation ") {
			header = line
			continue
		}
		rest, ok := strings.CutPrefix(line, "  ")
		if !ok {
			continue
		}
		path, after, ok := strings.Cut(rest, " [")
		if !ok {
			continue
		}
		code, _, ok := strings.Cut(after, "] ")
		if !ok {
			continue
		}
		issues = append(issues, issueLine{path: path, code: code})
	}
	return issues, header
}

// issueHeader is the verdict line a failed validation of n issues prints.
func issueHeader(n int) string {
	return fmt.Sprintf("%-20s : failed, %d issue(s)", "validation", n)
}

// sameIssues reports whether got and want hold the same issue lines, each as
// often, in any order.
func sameIssues(got, want []issueLine) bool {
	order := func(a, b issueLine) int {
		return strings.Compare(a.path+"\x00"+a.code, b.path+"\x00"+b.code)
	}
	return slices.Equal(slices.SortedFunc(slices.Values(got), order), slices.SortedFunc(slices.Values(want), order))
}

// collapseSpace replaces every run of white space with one space, so a
// phrase matches a hard-wrapped page as well as a one-line paragraph.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
