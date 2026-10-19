package fixtures_test

import (
	"errors"
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

// The AQL suite corpus is the tree's one AQL corpus and the input to
// PROBE-107. It must carry its own provenance pin and generated exclusion list
// beside the mirrored AQL/ tree, and its own ingest, scripts/ingest-robot-aql.sh,
// writes it from one pinned clone. The first two tests check the vendored
// tree; the others run the ingest against a small synthetic upstream clone,
// with the script's pinned commit rewritten to a commit of that clone, so its
// layout, re-run and refusal behaviour, and its independence from the clone's
// working tree, are checked without the real upstream.

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
	pin := pinFields(readText(t, filepath.Join(root, "AQL_SOURCE.txt")))
	for _, key := range []string{"source_repo", "commit", "commit_date"} {
		if pin[key] == "" {
			t.Errorf("suite pin %s is empty; want a value "+
				"(re-run scripts/ingest-robot-aql.sh; never edit a pin)", key)
		}
	}
}

// testkit/corpus/aql/ holds one vendored AQL corpus, suites/. Beside it sits
// lint/, the hand-written PROBE-028 lint inputs, which no ingest vendors. Any
// other entry, such as a second vendored corpus, fails here by name.
func TestAQLCorpusTreeHoldsOneVendoredCorpus_REQ160(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(filepath.Dir(fixtures.AQLSuitesRoot()))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "suites" && e.Name() != "lint" {
			t.Errorf("%s/%s exists; want only suites/, the one vendored AQL corpus, "+
				"and lint/, the hand-written lint inputs", aqlCorpusDir, e.Name())
		}
	}
}

func TestIngestRobotAQLWritesSuiteCorpus_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
	const fork = "example/aql-fork"
	cases := []struct {
		name, remote string
		// override is ROBOT_SOURCE_REPO; empty leaves it unset.
		override, wantRepo string
	}{
		// Each github.com remote form records its owner/repo.
		{name: "https remote", remote: "https://github.com/" + fork + ".git", wantRepo: fork},
		{name: "https remote without .git", remote: "https://github.com/" + fork, wantRepo: fork},
		{name: "scp-like ssh remote", remote: "git@github.com:" + fork + ".git", wantRepo: fork},
		{name: "scp-like ssh remote without .git", remote: "git@github.com:" + fork, wantRepo: fork},
		{name: "ssh URL remote", remote: "ssh://git@github.com/" + fork + ".git", wantRepo: fork},
		{name: "ssh URL remote without .git", remote: "ssh://git@github.com/" + fork, wantRepo: fork},
		// A clone of a local clone has a path for its origin, which names
		// no repository: the override names it, and is what the pins record.
		{
			name: "override with a local path remote", remote: "/srv/mirrors/integration-tests",
			override: "ehrbase/integration-tests", wantRepo: "ehrbase/integration-tests",
		},
		{
			name: "override with a GitHub remote", remote: "https://github.com/" + fork + ".git",
			override: "ehrbase/integration-tests", wantRepo: "ehrbase/integration-tests",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up, head := h.newUpstream(t)
			h.git(t, up, "remote", "set-url", "origin", tc.remote)
			sdk := h.newSDK(t, head)
			var env []string
			if tc.override != "" {
				env = append(env, "ROBOT_SOURCE_REPO="+tc.override)
			}
			if out, err := h.ingest(t, sdk, up, "", env...); err != nil {
				t.Fatalf("ingest at the pin: %v\n%s", err, out)
			}
			got := tree(t, filepath.Join(sdk, suitesDir))
			// The suite corpus is the one AQL corpus: the run writes
			// nothing else under testkit/corpus/aql.
			for _, rel := range slices.Sorted(maps.Keys(tree(t, filepath.Join(sdk, aqlCorpusDir)))) {
				if !strings.HasPrefix(rel, "suites/") {
					t.Errorf("the ingest wrote %s/%s; want it to write only under %s", aqlCorpusDir, rel, suitesDir)
				}
			}

			carried := []string{
				"AQL/__init__.robot",
				"AQL/FROM/AND_OR/from_and_or.robot",
				fieldsAndResults + "/where/combinations/compare.csv",
				fromCSV,
			}
			for _, rel := range carried {
				if want := h.blob(t, up, head, "tests/robot/"+rel); got[rel] != want {
					t.Errorf("suites/%s = %q, want the bytes of the blob at the pin %q", rel, got[rel], want)
				}
			}
			for _, rel := range slices.Sorted(maps.Keys(got)) {
				if !slices.Contains(carried, rel) && rel != "AQL_SOURCE.txt" && rel != "EXCLUDED.txt" {
					t.Errorf("suites/%s is vendored; want only the suites, the combination CSVs and the two generated files", rel)
				}
			}

			wantExcluded := []string{
				"AQL/ORDER_BY/tests_to_add.txt not-a-suite",
				"AQL/SELECT/README.md not-a-suite",
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

// The pinned commit, not the clone's working tree, determines the corpus:
// a run from a clone whose HEAD has moved past the pin, and whose working tree
// then differs from HEAD in each way listed below, writes exactly what a run
// from the pristine clone wrote.
func TestIngestRobotAQLReadsOnlyThePinnedCommit_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
	up, pin := h.newUpstream(t)
	sdk := h.newSDK(t, pin)
	if out, err := h.ingest(t, sdk, up, ""); err != nil {
		t.Fatalf("ingest from the pristine clone: %v\n%s", err, out)
	}
	before := tree(t, filepath.Join(sdk, aqlCorpusDir))

	robot := filepath.Join(up, "tests", "robot")
	far := filepath.Join(robot, filepath.FromSlash(fieldsAndResults))
	// HEAD moves past the pin: a later commit adds a suite, rewrites a FROM
	// input and drops the note under AQL/.
	writeText(t, filepath.Join(robot, "AQL/FROM/later.robot"), "*** Test Cases ***\n")
	writeText(t, filepath.Join(robot, filepath.FromSlash(fromCSV)), "later\n")
	h.git(t, up, "rm", "-q", "tests/robot/AQL/ORDER_BY/tests_to_add.txt")
	if head := h.commitAll(t, up); head == pin {
		t.Fatalf("HEAD = the pin %s after a new commit; want it past the pin", pin)
	}
	// The working tree then differs from HEAD under both roots: an untracked
	// suite, a modified CSV, a deleted suite, ignored files, a configuration
	// that hides untracked files from git status, attributes that would
	// convert line endings or drop suites from an export, and an unreadable
	// directory.
	writeText(t, filepath.Join(robot, "AQL/FROM/extra.robot"), "*** Test Cases ***\n")
	writeText(t, filepath.Join(far, "where/combinations/compare.csv"), "modified\n")
	if err := os.Remove(filepath.Join(robot, "AQL/__init__.robot")); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(up, ".git/info/exclude"), "ignored*\n")
	writeText(t, filepath.Join(robot, "AQL/FROM/ignored.robot"), "*** Test Cases ***\n")
	writeText(t, filepath.Join(far, "where/combinations/ignored.csv"), "ignored\n")
	h.git(t, up, "config", "status.showUntrackedFiles", "no")
	writeText(t, filepath.Join(up, ".git/info/attributes"), "* text eol=crlf\n*.robot export-ignore\n")
	unreadable := filepath.Join(robot, "AQL/FROM/AND_OR")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(unreadable, 0o755); err != nil {
			t.Error(err)
		}
	})

	if out, err := h.ingest(t, sdk, up, ""); err != nil {
		t.Fatalf("ingest from the diverged clone at the same pin: %v\n%s", err, out)
	}
	if changed := changedFiles(before, tree(t, filepath.Join(sdk, aqlCorpusDir))); len(changed) > 0 {
		t.Errorf("ingest from the diverged clone changed %q under testkit/corpus/aql; "+
			"want the outputs of the pristine clone, since both read one commit", changed)
	}
}

func TestIngestRobotAQLRefusesBeforeWriting_PROBE107_REQ160(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
	setOrigin := func(url string) func(t *testing.T, up string) {
		return func(t *testing.T, up string) {
			h.git(t, up, "remote", "set-url", "origin", url)
		}
	}
	cases := []struct {
		name string
		// change alters the upstream clone, committing when the case needs
		// the change to be part of the pinned commit; nil changes nothing.
		change func(t *testing.T, up string)
		// pin is the commit the script is pinned to, given the clone's HEAD
		// after the change; nil pins HEAD.
		pin func(head string) string
		// robotRoot is ROBOT_ROOT relative to the clone; empty is the
		// upstream tests/robot/_resources/test_data_sets.
		robotRoot string
		// env is added to the ingest's environment.
		env []string
		// want is what the refusal must name.
		want string
	}{
		{
			name: "pinned commit not in the clone",
			pin:  func(string) string { return "0123456789abcdef0123456789abcdef01234567" },
			want: "is not a commit in the clone",
		},
		{
			name: "pinned commit by an abbreviated name",
			pin:  func(head string) string { return head[:12] },
			want: "is not a commit in the clone",
		},
		{name: "absolute local path remote", change: setOrigin("/srv/mirrors/integration-tests"), want: "not a github.com repository URL"},
		{name: "relative local path remote", change: setOrigin("../integration-tests"), want: "not a github.com repository URL"},
		{name: "file URL remote", change: setOrigin("file:///srv/mirrors/integration-tests.git"), want: "not a github.com repository URL"},
		{name: "remote on another host", change: setOrigin("https://gitlab.com/example/aql-fork.git"), want: "not a github.com repository URL"},
		{name: "remote on a look-alike host", change: setOrigin("https://github.com.example.org/example/aql-fork.git"), want: "not a github.com repository URL"},
		{name: "GitHub remote naming no repository", change: setOrigin("https://github.com/example"), want: "not a github.com repository URL"},
		{name: "GitHub remote with a parent-directory owner", change: setOrigin("https://github.com/../integration-tests"), want: "not a github.com repository URL"},
		{
			name: "no origin remote",
			change: func(t *testing.T, up string) {
				h.git(t, up, "remote", "remove", "origin")
			},
			want: "not a github.com repository URL",
		},
		{
			name:   "override that is a path",
			change: setOrigin("/srv/mirrors/integration-tests"),
			env:    []string{"ROBOT_SOURCE_REPO=../integration-tests"},
			want:   "ROBOT_SOURCE_REPO=../integration-tests",
		},
		{
			name: "override with three segments",
			env:  []string{"ROBOT_SOURCE_REPO=github.com/ehrbase/integration-tests"},
			want: "ROBOT_SOURCE_REPO=github.com/ehrbase/integration-tests",
		},
		{
			name: "no AQL suites at the pin",
			change: func(t *testing.T, up string) {
				h.git(t, up, "rm", "-r", "-q", "tests/robot/AQL")
				h.commitAll(t, up)
			},
			want: "no Robot AQL suite",
		},
		{
			name: "only notes under AQL at the pin",
			change: func(t *testing.T, up string) {
				h.git(t, up, "rm", "-q", "tests/robot/AQL/__init__.robot", "tests/robot/AQL/FROM/AND_OR/from_and_or.robot")
				h.commitAll(t, up)
			},
			want: "no Robot AQL suite",
		},
		{
			name: "no combination CSVs at the pin",
			change: func(t *testing.T, up string) {
				h.git(t, up, "rm", "-r", "-q", "--", "*.csv")
				h.commitAll(t, up)
			},
			want: "no combination CSV",
		},
		{
			name: "resource file among the suites",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot/AQL/FROM/keywords.resource"), "*** Keywords ***\n")
				h.commitAll(t, up)
			},
			want: "AQL/FROM/keywords.resource",
		},
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
		// One rule for every path under the two roots, whichever list the
		// file would land in: a carried suite and an excluded result.
		{
			name: "whitespace in a carried path",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot/AQL/FROM/two words.robot"), "*** Test Cases ***\n")
				h.commitAll(t, up)
			},
			want: "AQL/FROM/two words.robot",
		},
		{
			name: "whitespace in an excluded path",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot", fieldsAndResults, "where/expected two.json"), "{}\n")
				h.commitAll(t, up)
			},
			want: fieldsAndResults + "/where/expected two.json",
		},
		{
			name: "ROBOT_ROOT elsewhere under tests/robot",
			change: func(t *testing.T, up string) {
				writeText(t, filepath.Join(up, "tests/robot/data/sets/aql/fields_and_results/where/combinations/compare.csv"), "a,b\n")
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

			if tc.change != nil {
				tc.change(t, up)
			}
			pin := h.git(t, up, "rev-parse", "HEAD")
			if tc.pin != nil {
				pin = tc.pin(pin)
			}
			h.pinScript(t, sdk, pin)
			out, err := h.ingest(t, sdk, up, tc.robotRoot, tc.env...)
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

// The ingest has no default clone. Without ROBOT_ROOT, or with it empty, it
// refuses before it writes anything, and says what ROBOT_ROOT must name.
func TestIngestRobotAQLRequiresRobotRoot(t *testing.T) {
	t.Parallel()
	h := newAQLIngest(t)
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
			up, head := h.newUpstream(t)
			sdk := h.newSDK(t, head)
			if out, err := h.ingest(t, sdk, up, ""); err != nil {
				t.Fatalf("ingest with ROBOT_ROOT set: %v\n%s", err, out)
			}
			before := tree(t, sdk)

			out, err := h.run(t, sdk, tc.env...)
			if err == nil {
				t.Fatalf("ingest succeeded; want a refusal naming ROBOT_ROOT\n%s", out)
			}
			for _, want := range robotRootRefusal {
				if !strings.Contains(out, want) {
					t.Errorf("ingest refused with %q; want the refusal to name %q", out, want)
				}
			}
			if changed := changedFiles(before, tree(t, sdk)); len(changed) > 0 {
				t.Errorf("the refusal changed %q in the scratch tree; want nothing changed", changed)
			}
		})
	}
}

// robotRootRefusal is what both Robot ingests print when ROBOT_ROOT is unset
// or empty: the variable, and the upstream directory it must name.
var robotRootRefusal = []string{"ROBOT_ROOT is unset or empty", "tests/robot/_resources/test_data_sets"}

// Paths in the scratch trees, slash-separated.
const (
	fieldsAndResults = "_resources/test_data_sets/aql/fields_and_results"
	aqlCorpusDir     = "testkit/corpus/aql"
	suitesDir        = aqlCorpusDir + "/suites"
	// fromCSV is the FROM-family combination CSV of the synthetic upstream.
	fromCSV = fieldsAndResults + "/from/combinations/from_simple_and_or.csv"
)

var (
	// expectedCommitRE matches the ingest script's pinned commit.
	expectedCommitRE = regexp.MustCompile(`(?m)^EXPECTED_COMMIT=[0-9a-f]+$`)
	// recordedRE matches a pin's recording date line.
	recordedRE = regexp.MustCompile(`(?m)^recorded_utc: .*$`)
)

// aqlIngest runs scripts/ingest-robot-aql.sh in scratch trees.
type aqlIngest struct {
	bash, gitBin string
	script       []byte
	// env isolates git and the ingest from the host: no inherited GIT_* or
	// ROBOT_* variable, no global or system configuration, no global
	// ignore file.
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
	home := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "ROBOT_") &&
			!strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "XDG_CONFIG_HOME=") {
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
	return &aqlIngest{bash: bash, gitBin: gitBin, script: script, env: slices.Clip(env)}
}

// newUpstream creates a git repository shaped like the upstream clone, with
// the upstream's github.com origin, and returns its root and HEAD. Besides
// what the suite corpus carries or excludes, it holds a suite and a data file
// outside the two suite-corpus roots, which the corpus must neither carry nor
// list.
func (h *aqlIngest) newUpstream(t *testing.T) (root, head string) {
	t.Helper()
	root = t.TempDir()
	files := map[string]string{
		"tests/robot/AQL/__init__.robot":                                      "*** Settings ***\n",
		"tests/robot/AQL/FROM/AND_OR/from_and_or.robot":                       "*** Test Cases ***\n",
		"tests/robot/AQL/ORDER_BY/tests_to_add.txt":                           "later\n",
		"tests/robot/AQL/SELECT/README.md":                                    "notes\n",
		"tests/robot/" + fieldsAndResults + "/where/combinations/compare.csv": "a,b\n",
		"tests/robot/" + fieldsAndResults + "/where/expected_compare.json":    "{}\n",
		"tests/robot/" + fromCSV:                                              "from,simple\n",
		"tests/robot/OTHER/outside.robot":                                     "*** Test Cases ***\n",
		"tests/robot/_resources/test_data_sets/aql/data_load/load.json":       "{}\n",
	}
	for rel, body := range files {
		writeText(t, filepath.Join(root, filepath.FromSlash(rel)), body)
	}
	h.git(t, root, "init", "-q")
	h.git(t, root, "remote", "add", "origin", "https://github.com/ehrbase/integration-tests.git")
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

// ingest runs the script in sdk against the clone up, with env added to its
// environment, and returns its combined output.
func (h *aqlIngest) ingest(t *testing.T, sdk, up, robotRoot string, env ...string) (string, error) {
	t.Helper()
	if robotRoot == "" {
		robotRoot = "tests/robot/_resources/test_data_sets"
	}
	return h.run(t, sdk, slices.Concat([]string{"ROBOT_ROOT=" + filepath.Join(up, filepath.FromSlash(robotRoot))}, env)...)
}

// run runs the script in sdk with only env added to its environment, so
// ROBOT_ROOT is unset unless env sets it, and returns its combined output.
func (h *aqlIngest) run(t *testing.T, sdk string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), h.bash, filepath.Join(sdk, "scripts", "ingest-robot-aql.sh"))
	cmd.Env = slices.Concat(h.env, env)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// git runs git in dir and returns its trimmed standard output.
func (h *aqlIngest) git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(h.gitOutput(t, dir, args...))
}

// blob returns the content of path in commit of the repository dir, as git
// stores it: no attribute, filter or line-ending conversion applies.
func (h *aqlIngest) blob(t *testing.T, dir, commit, path string) string {
	t.Helper()
	return h.gitOutput(t, dir, "cat-file", "blob", commit+":"+path)
}

// gitOutput runs git in dir and returns its standard output unchanged. A
// failure stops the test with git's standard error.
func (h *aqlIngest) gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), h.gitBin, append([]string{"-C", dir}, args...)...)
	cmd.Env = h.env
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr != nil {
			stderr = exitErr.Stderr
		}
		t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, stderr)
	}
	return string(out)
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
