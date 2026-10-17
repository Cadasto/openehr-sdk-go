package fixtures_test

import (
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The AQL suite corpus is the input to PROBE-107. It must carry its own
// provenance pin and generated exclusion list beside the mirrored AQL/ tree,
// and one ingest, scripts/ingest-robot-aql.sh, writes it and the AQL
// conformance corpus from one pinned clone, so the two pins name one commit.
// The first test checks the vendored tree; the others run the ingest against a
// small synthetic upstream clone, with the script's pinned commit rewritten to
// that clone's HEAD, so its layout, re-run and refusal behaviour are checked
// without the real upstream.

func TestAQLSuitesRootHoldsPinnedCorpus_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	root := fixtures.AQLSuitesRoot()
	for _, name := range []string{"AQL_SOURCE.txt", "EXCLUDED.txt"} {
		if fi, err := os.Stat(filepath.Join(root, name)); err != nil || !fi.Mode().IsRegular() {
			t.Errorf("%s under AQLSuitesRoot() %s: stat error %v; want a regular file", name, root, err)
		}
	}
	if fi, err := os.Stat(filepath.Join(root, "AQL")); err != nil || !fi.IsDir() {
		t.Errorf("AQL under AQLSuitesRoot() %s: stat error %v; want a directory", root, err)
	}
	suites := pinFields(readText(t, filepath.Join(root, "AQL_SOURCE.txt")))
	conformance := pinFields(readText(t, filepath.Join(fixtures.AQLConformanceRoot(), "AQL_SOURCE.txt")))
	for _, key := range []string{"source_repo", "commit", "commit_date"} {
		if suites[key] == "" || suites[key] != conformance[key] {
			t.Errorf("suite pin %s = %q, conformance pin %s = %q; want one non-empty value "+
				"(re-run scripts/ingest-robot-aql.sh; never edit a pin)", key, suites[key], key, conformance[key])
		}
	}
}

func TestIngestRobotAQLWritesSuiteCorpus_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
	cases := []struct {
		name, remote, wantRepo string
	}{
		// A clone of a local clone has a path for its origin, which names
		// no github.com tree: the pins name the canonical upstream.
		{name: "local path remote", remote: "/srv/mirrors/integration-tests", wantRepo: "ehrbase/integration-tests"},
		{name: "GitHub remote", remote: "https://github.com/example/aql-fork.git", wantRepo: "example/aql-fork"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up, head := h.newUpstream(t)
			h.git(t, up, "remote", "add", "origin", tc.remote)
			sdk := h.newSDK(t, head)
			if out, err := h.ingest(t, sdk, up, ""); err != nil {
				t.Fatalf("ingest at the pin: %v\n%s", err, out)
			}
			got := tree(t, filepath.Join(sdk, suitesDir))

			carried := []string{
				"AQL/__init__.robot",
				"AQL/FROM/AND_OR/from_and_or.robot",
				fieldsAndResults + "/where/combinations/compare.csv",
			}
			for _, name := range h.inputs {
				carried = append(carried, fieldsAndResults+"/from/combinations/"+name)
			}
			for _, rel := range carried {
				if want := readText(t, filepath.Join(up, "tests", "robot", rel)); got[rel] != want {
					t.Errorf("suites/%s = %q, want the upstream bytes %q", rel, got[rel], want)
				}
			}
			for _, rel := range slices.Sorted(maps.Keys(got)) {
				if !slices.Contains(carried, rel) && rel != "AQL_SOURCE.txt" && rel != "EXCLUDED.txt" {
					t.Errorf("suites/%s is vendored; want only the suites, the combination CSVs and the two generated files", rel)
				}
			}

			wantExcluded := []string{
				"AQL/ORDER_BY/tests_to_add.txt not-a-suite",
				fieldsAndResults + "/where/expected_compare.json execution-semantics",
			}
			if gotExcluded := records(got["EXCLUDED.txt"]); !slices.Equal(gotExcluded, wantExcluded) {
				t.Errorf("EXCLUDED.txt records = %q, want %q", gotExcluded, wantExcluded)
			}

			pin := pinFields(got["AQL_SOURCE.txt"])
			wantPin := map[string]string{
				"source_repo": tc.wantRepo,
				"source_path": "tests/robot",
				"commit":      head,
				"source_tree": "https://github.com/" + tc.wantRepo + "/tree/" + head + "/tests/robot",
				"license":     "Apache-2.0 (see ../../THIRD_PARTY_LICENSES.md)",
			}
			for _, key := range slices.Sorted(maps.Keys(wantPin)) {
				if pin[key] != wantPin[key] {
					t.Errorf("AQL_SOURCE.txt %s = %q, want %q", key, pin[key], wantPin[key])
				}
			}
			conformance := pinFields(readText(t, filepath.Join(sdk, conformanceDir, "AQL_SOURCE.txt")))
			for _, key := range []string{"source_repo", "commit", "commit_date"} {
				if conformance[key] != pin[key] {
					t.Errorf("conformance pin %s = %q, suite pin %s = %q; want one value from one run", key, conformance[key], key, pin[key])
				}
			}
		})
	}
}

func TestIngestRobotAQLSuiteCorpusReRunAndRePin_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
	up, head := h.newUpstream(t)
	sdk := h.newSDK(t, head)
	if out, err := h.ingest(t, sdk, up, ""); err != nil {
		t.Fatalf("first ingest: %v\n%s", err, out)
	}

	// A run on an earlier day stamped an earlier date: a re-run at the same
	// pin keeps it, so it rewrites nothing.
	const earlier = "recorded_utc: 2000-01-01"
	pinPath := filepath.Join(sdk, suitesDir, "AQL_SOURCE.txt")
	writeText(t, pinPath, recordedRE.ReplaceAllString(readText(t, pinPath), earlier))
	before := tree(t, filepath.Join(sdk, aqlCorpusDir))
	if out, err := h.ingest(t, sdk, up, ""); err != nil {
		t.Fatalf("re-run at the same pin: %v\n%s", err, out)
	}
	if changed := changedFiles(before, tree(t, filepath.Join(sdk, aqlCorpusDir))); len(changed) > 0 {
		t.Errorf("re-run at the same pin changed %q under testkit/corpus/aql; want nothing changed", changed)
	}

	// A re-pin to a commit that drops a suite and a CSV upstream must not
	// leave their old copies behind.
	dropped := []string{"AQL/FROM/AND_OR/from_and_or.robot", fieldsAndResults + "/where/combinations/compare.csv"}
	for _, rel := range dropped {
		h.git(t, up, "rm", "-q", "tests/robot/"+rel)
	}
	repinned := h.commitAll(t, up)
	h.pinScript(t, sdk, repinned)
	if out, err := h.ingest(t, sdk, up, ""); err != nil {
		t.Fatalf("ingest after the re-pin: %v\n%s", err, out)
	}
	got := tree(t, filepath.Join(sdk, suitesDir))
	for _, rel := range dropped {
		if _, ok := got[rel]; ok {
			t.Errorf("suites/%s is still vendored after the re-pin dropped it upstream", rel)
		}
	}
	if _, ok := got["AQL/__init__.robot"]; !ok {
		t.Error("suites/AQL/__init__.robot is gone after the re-pin; want the suites upstream still has")
	}
	pin := pinFields(got["AQL_SOURCE.txt"])
	if pin["commit"] != repinned || "recorded_utc: "+pin["recorded_utc"] == earlier {
		t.Errorf("AQL_SOURCE.txt after the re-pin: commit %q, recorded_utc %q; want commit %q and a new recording date",
			pin["commit"], pin["recorded_utc"], repinned)
	}
}

func TestIngestRobotAQLRefusesBeforeWriting_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
	cases := []struct {
		name string
		// change alters the upstream clone, committing when the case needs
		// the change to be part of the pinned commit.
		change func(t *testing.T, up string)
		// robotRoot is ROBOT_ROOT relative to the clone; empty is the
		// upstream tests/robot/_resources/test_data_sets.
		robotRoot string
		// want is what the refusal must name.
		want string
	}{
		{
			name: "CSV outside a combinations directory",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot", fieldsAndResults, "where/stray.csv"), "a\n")
				h.commitAll(t, up)
			},
			want: fieldsAndResults + "/where/stray.csv",
		},
		{
			name: "CSV below a combinations subdirectory",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot", fieldsAndResults, "where/combinations/old/compare.csv"), "a\n")
				h.commitAll(t, up)
			},
			want: fieldsAndResults + "/where/combinations/old/compare.csv",
		},
		{
			name: "file no rule classifies",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot", fieldsAndResults, "where/notes.md"), "notes\n")
				h.commitAll(t, up)
			},
			want: fieldsAndResults + "/where/notes.md",
		},
		{
			name: "symbolic link among the suites",
			change: func(t *testing.T, up string) {
				if err := os.Symlink("../__init__.robot", filepath.Join(up, "tests/robot/AQL/FROM/link.robot")); err != nil {
					t.Skipf("cannot create a symbolic link: %v", err)
				}
				h.commitAll(t, up)
			},
			want: "AQL/FROM/link.robot",
		},
		{
			name: "whitespace in a suite path",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot/AQL/FROM/two words.robot"), "*** Test Cases ***\n")
				h.commitAll(t, up)
			},
			want: "AQL/FROM/two words.robot",
		},
		{
			name: "untracked suite",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot/AQL/FROM/extra.robot"), "*** Test Cases ***\n")
			},
			want: "tests/robot/AQL has local modifications",
		},
		{
			name: "ignored file among the suites",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, ".gitignore"), "*.log\n")
				h.commitAll(t, up)
				writeText(t, filepath.Join(up, "tests/robot/AQL/run.log"), "a run\n")
			},
			want: "tests/robot/AQL has local modifications",
		},
		{
			name: "ROBOT_ROOT elsewhere under tests/robot",
			change: func(t *testing.T, up string) {
				for _, name := range h.inputs {
					writeText(t, filepath.Join(up, "tests/robot/data/sets/aql/fields_and_results/from/combinations", name), "from\n")
				}
				h.commitAll(t, up)
			},
			robotRoot: "tests/robot/data/sets",
			want:      "_resources/test_data_sets",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up, head := h.newUpstream(t)
			sdk := h.newSDK(t, head)
			if out, err := h.ingest(t, sdk, up, ""); err != nil {
				t.Fatalf("ingest at the base pin: %v\n%s", err, out)
			}
			before := tree(t, filepath.Join(sdk, aqlCorpusDir))

			tc.change(t, up)
			h.pinScript(t, sdk, h.git(t, up, "rev-parse", "HEAD"))
			out, err := h.ingest(t, sdk, up, tc.robotRoot)
			if err == nil {
				t.Fatalf("ingest succeeded; want a refusal naming %q\n%s", tc.want, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("ingest refused with %q; want the refusal to name %q", out, tc.want)
			}
			if changed := changedFiles(before, tree(t, filepath.Join(sdk, aqlCorpusDir))); len(changed) > 0 {
				t.Errorf("the refusal changed %q under testkit/corpus/aql; want nothing changed", changed)
			}
		})
	}
}

// Paths in the scratch trees, slash-separated.
const (
	fieldsAndResults = "_resources/test_data_sets/aql/fields_and_results"
	aqlCorpusDir     = "testkit/corpus/aql"
	suitesDir        = aqlCorpusDir + "/suites"
	conformanceDir   = aqlCorpusDir + "/conformance"
)

var (
	// corpusEntryRE matches one entry of the ingest script's CORPUS list,
	// `"FAMILY name.csv"`, and captures the CSV name.
	corpusEntryRE = regexp.MustCompile(`(?m)^\s+"[A-Z_]+ ([^"\s/]+\.csv)"$`)
	// expectedCommitRE matches the ingest script's pinned commit.
	expectedCommitRE = regexp.MustCompile(`(?m)^EXPECTED_COMMIT=[0-9a-f]+$`)
	// recordedRE matches a pin's recording date line.
	recordedRE = regexp.MustCompile(`(?m)^recorded_utc: .*$`)
)

// aqlIngest runs scripts/ingest-robot-aql.sh in scratch trees.
type aqlIngest struct {
	bash, gitBin string
	script       []byte
	// inputs are the FROM-family CSV names the script's CORPUS list
	// requires every upstream clone to hold.
	inputs []string
	// env isolates git from the host: no inherited GIT_* variable, no
	// global or system configuration, no global ignore file.
	env []string
}

func newAQLIngest(t *testing.T) *aqlIngest {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found")
	}
	script, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), "..", "..", "scripts", "ingest-robot-aql.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var inputs []string
	for _, m := range corpusEntryRE.FindAllSubmatch(script, -1) {
		if name := string(m[1]); !slices.Contains(inputs, name) {
			inputs = append(inputs, name)
		}
	}
	if len(inputs) == 0 {
		t.Fatal("no CORPUS entry found in scripts/ingest-robot-aql.sh")
	}
	home := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "XDG_CONFIG_HOME=") {
			env = append(env, kv)
		}
	}
	env = append(env,
		"HOME="+home, "XDG_CONFIG_HOME="+home,
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.org",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.org",
		"GIT_AUTHOR_DATE=2026-01-02T03:04:05Z", "GIT_COMMITTER_DATE=2026-01-02T03:04:05Z",
	)
	return &aqlIngest{bash: bash, gitBin: gitBin, script: script, inputs: inputs, env: slices.Clip(env)}
}

// newUpstream creates a git repository shaped like the upstream clone and
// returns its root and HEAD. Besides what the suite corpus carries or
// excludes, it holds a suite and a data file outside the two suite-corpus
// roots, which the corpus must neither carry nor list.
func (h *aqlIngest) newUpstream(t *testing.T) (root, head string) {
	t.Helper()
	root = t.TempDir()
	files := map[string]string{
		"tests/robot/AQL/__init__.robot":                                      "*** Settings ***\n",
		"tests/robot/AQL/FROM/AND_OR/from_and_or.robot":                       "*** Test Cases ***\n",
		"tests/robot/AQL/ORDER_BY/tests_to_add.txt":                           "later\n",
		"tests/robot/" + fieldsAndResults + "/where/combinations/compare.csv": "a,b\n",
		"tests/robot/" + fieldsAndResults + "/where/expected_compare.json":    "{}\n",
		"tests/robot/OTHER/outside.robot":                                     "*** Test Cases ***\n",
		"tests/robot/_resources/test_data_sets/aql/data_load/load.json":       "{}\n",
	}
	for _, name := range h.inputs {
		files["tests/robot/"+fieldsAndResults+"/from/combinations/"+name] = "from," + name + "\n"
	}
	for rel, body := range files {
		writeText(t, filepath.Join(root, filepath.FromSlash(rel)), body)
	}
	h.git(t, root, "init", "-q")
	return root, h.commitAll(t, root)
}

// newSDK returns a scratch tree laid out like this repository, holding only
// the ingest script pinned to commit.
func (h *aqlIngest) newSDK(t *testing.T, commit string) string {
	t.Helper()
	sdk := t.TempDir()
	h.pinScript(t, sdk, commit)
	return sdk
}

// pinScript writes the ingest script into sdk with EXPECTED_COMMIT set to
// commit.
func (h *aqlIngest) pinScript(t *testing.T, sdk, commit string) {
	t.Helper()
	if n := len(expectedCommitRE.FindAll(h.script, -1)); n != 1 {
		t.Fatalf("scripts/ingest-robot-aql.sh has %d EXPECTED_COMMIT lines, want 1", n)
	}
	pinned := expectedCommitRE.ReplaceAllLiteral(h.script, []byte("EXPECTED_COMMIT="+commit))
	writeText(t, filepath.Join(sdk, "scripts", "ingest-robot-aql.sh"), string(pinned))
}

// ingest runs the script in sdk against the clone up and returns its
// combined output.
func (h *aqlIngest) ingest(t *testing.T, sdk, up, robotRoot string) (string, error) {
	t.Helper()
	if robotRoot == "" {
		robotRoot = "tests/robot/_resources/test_data_sets"
	}
	cmd := exec.CommandContext(t.Context(), h.bash, filepath.Join(sdk, "scripts", "ingest-robot-aql.sh"))
	cmd.Env = slices.Concat(h.env, []string{"ROBOT_ROOT=" + filepath.Join(up, filepath.FromSlash(robotRoot))})
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// git runs git in dir and returns its trimmed standard output.
func (h *aqlIngest) git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), h.gitBin, append([]string{"-C", dir}, args...)...)
	cmd.Env = h.env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

// commitAll commits every change in dir and returns the new HEAD.
func (h *aqlIngest) commitAll(t *testing.T, dir string) string {
	t.Helper()
	h.git(t, dir, "add", "-A")
	h.git(t, dir, "commit", "-q", "-m", "upstream")
	return h.git(t, dir, "rev-parse", "HEAD")
}

// tree returns every file under dir by its slash-separated path relative to
// dir, with its content.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// changedFiles returns, sorted, every path whose content differs between two
// [tree] results, or that only one of them holds.
func changedFiles(before, after map[string]string) []string {
	var changed []string
	for rel, body := range before {
		if got, ok := after[rel]; !ok || got != body {
			changed = append(changed, rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			changed = append(changed, rel)
		}
	}
	slices.Sort(changed)
	return changed
}

// records returns the lines of a generated list that are not comments.
func records(text string) []string {
	var out []string
	for line := range strings.Lines(text) {
		if line = strings.TrimSuffix(line, "\n"); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// pinFields returns the `key: value` fields of an AQL_SOURCE.txt pin.
func pinFields(text string) map[string]string {
	fields := map[string]string{}
	for _, line := range records(text) {
		if key, value, ok := strings.Cut(line, ": "); ok {
			fields[key] = value
		}
	}
	return fields
}

func readText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
