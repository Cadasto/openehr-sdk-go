package fixtures_test

import (
	"maps"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// testkit/corpus/UPSTREAM_PINS.txt names one commit per upstream repository,
// and every corpus vendored from that repository must record it.
// scripts/corpus-pins.sh verify, which `make corpus-pins-check` runs, holds
// them to it. The first test runs the check on the vendored tree; the second
// runs it on scratch trees that each break one record, and the check must
// name that record.

func TestCorpusPinsVendoredTreeAgrees(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script := filepath.Join(fixtures.CorpusRoot(), "..", "..", "scripts", "corpus-pins.sh")
	out, err := exec.CommandContext(t.Context(), bash, script, "verify").CombinedOutput()
	if err != nil {
		t.Fatalf("corpus-pins.sh verify on the vendored tree: %v "+
			"(re-run the vendoring scripts UPSTREAM_PINS.txt lists; never edit a pin)\n%s", err, out)
	}
}

func TestCorpusPinsVerifyNamesTheDisagreeingRecord(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script := readText(t, filepath.Join(fixtures.CorpusRoot(), "..", "..", "scripts", "corpus-pins.sh"))

	const (
		sdk   = "1111111111111111111111111111111111111111"
		robot = "2222222222222222222222222222222222222222"
		other = "3333333333333333333333333333333333333333"
	)
	pins := "# scratch pins\n" +
		"ehrbase/openEHR_SDK          develop  " + sdk + "\n" +
		"ehrbase/integration-tests    main     " + robot + "\n"
	pinned := func(repo, commit string) string {
		return "# provenance\nsource_repo: " + repo + "\ncommit: " + commit + "\n"
	}
	crossformat := func(sdkCommit, robotCommit string) string {
		return "# generated\n" +
			"source\tsdk\thttps://github.com/ehrbase/openEHR_SDK\t" + sdkCommit + "\tApache-2.0\n" +
			"source\trobot\thttps://github.com/ehrbase/integration-tests\t" + robotCommit + "\tApache-2.0\n"
	}
	// base is the scratch tree, by path relative to its root; every record
	// agrees with its pin.
	base := map[string]string{
		"scripts/corpus-pins.sh":                       script,
		"testkit/corpus/UPSTREAM_PINS.txt":             pins,
		"testkit/corpus/flat-conformance/MANIFEST.txt": pinned("ehrbase/openEHR_SDK", sdk),
		"testkit/corpus/aql/suites/AQL_SOURCE.txt":     pinned("ehrbase/integration-tests", robot),
		"testkit/corpus/ROBOT_SOURCE.txt":              pinned("ehrbase/integration-tests", robot),
		"testkit/corpus/crossformat/MANIFEST.txt":      crossformat(sdk, robot),
	}

	cases := []struct {
		name string
		// change edits the scratch tree before the check; a path set to ""
		// is left out.
		change map[string]string
		// want is what the refusal must name; empty wants the check to pass.
		want string
	}{
		{name: "every record agrees"},
		{
			name:   "FLAT corpus on another commit",
			change: map[string]string{"testkit/corpus/flat-conformance/MANIFEST.txt": pinned("ehrbase/openEHR_SDK", other)},
			want:   "STALE: flat-conformance/MANIFEST.txt",
		},
		{
			name:   "AQL suite corpus on another commit",
			change: map[string]string{"testkit/corpus/aql/suites/AQL_SOURCE.txt": pinned("ehrbase/integration-tests", other)},
			want:   "STALE: aql/suites/AQL_SOURCE.txt",
		},
		{
			name:   "Robot fixtures on another commit",
			change: map[string]string{"testkit/corpus/ROBOT_SOURCE.txt": pinned("ehrbase/integration-tests", other)},
			want:   "STALE: ROBOT_SOURCE.txt",
		},
		{
			name:   "one cross-format source on another commit",
			change: map[string]string{"testkit/corpus/crossformat/MANIFEST.txt": crossformat(sdk, other)},
			want:   "STALE: crossformat/MANIFEST.txt records ehrbase/integration-tests",
		},
		{
			name:   "corpus from a repository with no pin",
			change: map[string]string{"testkit/corpus/aql/suites/AQL_SOURCE.txt": pinned("example/aql-fork", robot)},
			want:   "UNPINNED: aql/suites/AQL_SOURCE.txt",
		},
		{
			name:   "provenance file missing",
			change: map[string]string{"testkit/corpus/ROBOT_SOURCE.txt": ""},
			want:   "MISSING: ROBOT_SOURCE.txt",
		},
		{
			name:   "two pins for one repository",
			change: map[string]string{"testkit/corpus/UPSTREAM_PINS.txt": pins + "ehrbase/openEHR_SDK develop " + sdk + "\n"},
			want:   "DUPLICATE pin: ehrbase/openEHR_SDK",
		},
		{
			name:   "abbreviated pin",
			change: map[string]string{"testkit/corpus/UPSTREAM_PINS.txt": strings.Replace(pins, sdk, sdk[:12], 1)},
			want:   "MALFORMED pin line: ehrbase/openEHR_SDK",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			files := maps.Clone(base)
			maps.Copy(files, tc.change)
			for rel, text := range files {
				if text != "" {
					writeText(t, filepath.Join(root, filepath.FromSlash(rel)), text)
				}
			}
			cmd := exec.CommandContext(t.Context(), bash, filepath.Join(root, "scripts", "corpus-pins.sh"), "verify")
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("corpus-pins.sh verify = %v; want success\n%s", err, out)
			case tc.want != "" && err == nil:
				t.Fatalf("corpus-pins.sh verify succeeded; want a refusal naming %q\n%s", tc.want, out)
			case tc.want != "" && !strings.Contains(out, tc.want):
				t.Errorf("corpus-pins.sh verify refused with %q; want it to name %q", out, tc.want)
			}
		})
	}
}

// upstreamPin returns the commit the UPSTREAM_PINS.txt text pins for repo,
// or "" when no line names it.
func upstreamPin(text, repo string) string {
	for _, line := range records(text) {
		if f := strings.Fields(line); len(f) == 3 && f[0] == repo {
			return f[2]
		}
	}
	return ""
}
