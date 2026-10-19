package fixtures_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// scripts/ingest-robot-fixtures.sh has no default clone either. Without
// ROBOT_ROOT, or with it empty, it refuses before it writes anything, and says
// what ROBOT_ROOT must name. It runs from a scratch tree laid out like this
// repository, so a run that did write would write there.
func TestIngestRobotFixturesRequiresRobotRoot(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), "..", "..", "scripts", "ingest-robot-fixtures.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var base []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ROBOT_") {
			base = append(base, kv)
		}
	}
	cases := []struct {
		name string
		// env is added to the ingest's environment, which holds no
		// ROBOT_ROOT of its own.
		env []string
	}{
		{name: "ROBOT_ROOT unset"},
		{name: "ROBOT_ROOT empty", env: []string{"ROBOT_ROOT="}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sdk := t.TempDir()
			path := filepath.Join(sdk, "scripts", "ingest-robot-fixtures.sh")
			writeText(t, path, string(script))
			writeText(t, filepath.Join(sdk, "testkit", "corpus", "ROBOT_SOURCE.txt"), "commit: before\n")
			before, beforeFiles := entries(t, sdk), tree(t, sdk)

			cmd := exec.CommandContext(t.Context(), bash, path)
			cmd.Env = slices.Concat(base, tc.env)
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			if err == nil {
				t.Fatalf("ingest succeeded; want a refusal naming ROBOT_ROOT\n%s", out)
			}
			for _, want := range robotRootRefusal {
				if !strings.Contains(out, want) {
					t.Errorf("ingest refused with %q; want the refusal to name %q", out, want)
				}
			}
			if after := entries(t, sdk); !slices.Equal(after, before) {
				t.Errorf("the refusal left the scratch tree holding %q; want %q", after, before)
			}
			if changed := changedFiles(beforeFiles, tree(t, sdk)); len(changed) > 0 {
				t.Errorf("the refusal changed %q in the scratch tree; want nothing changed", changed)
			}
		})
	}
}

// entries returns, sorted, every file and directory under dir by its
// slash-separated path relative to dir.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}
