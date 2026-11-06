package fixtures_test

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// testkit/corpus/ehrbase-service/ holds operational templates copied byte for
// byte from the EHRbase server's tests, at the ehrbase/ehrbase commit
// UPSTREAM_PINS.txt names. scripts/ingest-ehrbase-service.sh copies them
// (ingest) and checks them offline (verify). The first test runs verify on
// the vendored tree, so `make test` holds the corpus to its manifest. The
// others run the script in scratch trees laid out like this repository, and
// each shows that a check refuses what it guards against.

const (
	ehrbaseServiceScript = "scripts/ingest-ehrbase-service.sh"
	ehrbaseServiceDir    = "testkit/corpus/ehrbase-service"
	ehrbaseServiceRepo   = "https://github.com/ehrbase/ehrbase"
)

// ehrbaseServiceFile is a `file` record of the corpus manifest.
type ehrbaseServiceFile struct {
	local    string // path relative to ehrbase-service/, such as templates/careplan.v1.opt
	upstream string // path in the upstream repository
	sha256   string
}

// ehrbaseServiceFiles returns the file records of the vendored manifest, in
// file order.
func ehrbaseServiceFiles(t *testing.T) []ehrbaseServiceFile {
	t.Helper()
	text := readText(t, filepath.Join(fixtures.CorpusRoot(), "ehrbase-service", "MANIFEST.txt"))
	var files []ehrbaseServiceFile
	for _, line := range records(text) {
		f := strings.Split(line, "\t")
		if f[0] != "file" {
			continue
		}
		if len(f) != 5 {
			t.Fatalf("manifest file record %q has %d fields, want 5", line, len(f))
		}
		files = append(files, ehrbaseServiceFile{local: f[1], upstream: f[3], sha256: f[4]})
	}
	if len(files) == 0 {
		t.Fatal("the ehrbase-service manifest lists no files")
	}
	return files
}

func TestEHRbaseServiceCorpusVerifies(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script := filepath.Join(fixtures.CorpusRoot(), "..", "..", filepath.FromSlash(ehrbaseServiceScript))
	out, err := exec.CommandContext(t.Context(), bash, script, "verify").CombinedOutput()
	if err != nil {
		t.Fatalf("%s verify on the vendored tree: %v "+
			"(re-run `make ehrbase-service-sync` at the pin; never edit a vendored file)\n%s", ehrbaseServiceScript, err, out)
	}
}

// Every template of the corpus reaches the whole-corpus tests through
// fixtures.ListAllOPTs, named by its stem under the ehrbase-service label.
func TestListAllOPTsNamesTheEHRbaseServiceTemplates(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs() error: %v", err)
	}
	for _, f := range ehrbaseServiceFiles(t) {
		want := fixtures.OPTRef{
			Name: "ehrbase-service/" + strings.TrimSuffix(filepath.Base(f.local), ".opt"),
			Path: filepath.Join(fixtures.CorpusRoot(), "ehrbase-service", filepath.FromSlash(f.local)),
		}
		if !slices.Contains(refs, want) {
			t.Errorf("ListAllOPTs() has no %+v", want)
		}
	}
}

// serviceHarness runs scripts/ingest-ehrbase-service.sh in scratch trees,
// with git configured from nothing but the environment it sets.
type serviceHarness struct {
	bash, gitBin string
	script       string
	env          []string
}

func newServiceHarness(t *testing.T) *serviceHarness {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		if _, err := exec.LookPath("shasum"); err != nil {
			t.Skip("neither sha256sum nor shasum found")
		}
	}
	script := readText(t, filepath.Join(fixtures.CorpusRoot(), "..", "..", filepath.FromSlash(ehrbaseServiceScript)))
	home := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "EHRBASE_SERVICE_") &&
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
	return &serviceHarness{bash: bash, gitBin: gitBin, script: script, env: slices.Clip(env)}
}

// run runs the script in sdk with mode and only env added to its
// environment, and returns its combined output.
func (h *serviceHarness) run(t *testing.T, sdk, mode string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), h.bash, filepath.Join(sdk, filepath.FromSlash(ehrbaseServiceScript)), mode)
	cmd.Env = slices.Concat(h.env, env)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// git runs git in dir and returns its trimmed standard output.
func (h *serviceHarness) git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), h.gitBin, append([]string{"-C", dir}, args...)...)
	cmd.Env = h.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// newSDK returns a scratch tree laid out like this repository, holding the
// script, an UPSTREAM_PINS.txt with pinLine as its one pin line, and files,
// by path relative to the tree's root.
func (h *serviceHarness) newSDK(t *testing.T, pinLine string, files map[string]string) string {
	t.Helper()
	sdk := t.TempDir()
	writeText(t, filepath.Join(sdk, filepath.FromSlash(ehrbaseServiceScript)), h.script)
	writePins(t, sdk, pinLine)
	for rel, body := range files {
		writeText(t, filepath.Join(sdk, filepath.FromSlash(rel)), body)
	}
	return sdk
}

// writePins sets pinLine as the one pin line of the scratch tree sdk.
func writePins(t *testing.T, sdk, pinLine string) {
	t.Helper()
	writeText(t, filepath.Join(sdk, "testkit", "corpus", "UPSTREAM_PINS.txt"), "# scratch pins\n"+pinLine+"\n")
}

// staleStamp is a recorded_utc far from any clock reading, so a manifest that
// carries it shows whether an ingest kept it or wrote a new one.
const staleStamp = "2000-01-01T00:00:00Z"

const recordedUTCPrefix = "# recorded_utc: "

// recordedUTC returns the value of the `# recorded_utc:` line of a manifest,
// or "" when it has none.
func recordedUTC(manifest string) string {
	for line := range strings.Lines(manifest) {
		if v, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), recordedUTCPrefix); ok {
			return v
		}
	}
	return ""
}

// restamp rewrites the `# recorded_utc:` line of the manifest at path to stamp
// and leaves every other byte of it as it was.
func restamp(t *testing.T, path, stamp string) {
	t.Helper()
	text := readText(t, path)
	old := recordedUTC(text)
	if old == "" {
		t.Fatalf("%s has no recorded_utc line to rewrite:\n%s", path, text)
	}
	writeText(t, path, strings.Replace(text, recordedUTCPrefix+old, recordedUTCPrefix+stamp, 1))
}

// scratchOPT is an operational template that carries only its template id.
func scratchOPT(id string) string {
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<template xmlns=\"http://schemas.openehr.org/v1\">\n" +
		"  <template_id>\n    <value>" + id + "</value>\n  </template_id>\n</template>\n"
}

func sha256Text(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// The offline check: every manifest record names a file whose sha256 it
// holds, every file in the subtree is listed, and the one source record
// names an https repository, a full commit and the Apache-2.0 licence.
func TestIngestEHRbaseServiceVerifyRefusesADisagreeingTree(t *testing.T) {
	t.Parallel()
	h := newServiceHarness(t)
	const commit = "0123456789abcdef0123456789abcdef01234567"
	a, b := scratchOPT("a.v0"), scratchOPT("b b.v0")
	source := "source\tehrbase\t" + ehrbaseServiceRepo + "\t" + commit + "\tApache-2.0\n"
	record := func(local, body string) string {
		return "file\t" + local + "\tehrbase\tknowledge/" + local + "\t" + sha256Text(body) + "\n"
	}
	recordA, recordB := record("templates/a.v0.opt", a), record("templates/b b.v0.opt", b)
	manifest := "# generated\n" + source + recordA + recordB
	base := map[string]string{
		ehrbaseServiceDir + "/MANIFEST.txt":         manifest,
		ehrbaseServiceDir + "/templates/a.v0.opt":   a,
		ehrbaseServiceDir + "/templates/b b.v0.opt": b,
	}
	cases := []struct {
		name string
		// change edits the scratch tree before the check; a path set to ""
		// is left out.
		change map[string]string
		// want is what the refusal must name; empty wants the check to pass.
		want string
	}{
		{name: "every file agrees"},
		{
			name:   "altered file",
			change: map[string]string{ehrbaseServiceDir + "/templates/b b.v0.opt": b + " "},
			want:   "FAILED: ehrbase-service/templates/b b.v0.opt",
		},
		{
			name:   "missing file",
			change: map[string]string{ehrbaseServiceDir + "/templates/a.v0.opt": ""},
			want:   "MISSING: ehrbase-service/templates/a.v0.opt",
		},
		{
			name:   "unlisted template",
			change: map[string]string{ehrbaseServiceDir + "/templates/c.v0.opt": scratchOPT("c.v0")},
			want:   "UNTRACKED: ehrbase-service/templates/c.v0.opt",
		},
		{
			name:   "unlisted file beside the manifest",
			change: map[string]string{ehrbaseServiceDir + "/notes.txt": "notes\n"},
			want:   "UNTRACKED: ehrbase-service/notes.txt",
		},
		{
			name: "altered file named by the last, newline-less record",
			change: map[string]string{
				ehrbaseServiceDir + "/MANIFEST.txt":         strings.TrimSuffix(manifest, "\n"),
				ehrbaseServiceDir + "/templates/b b.v0.opt": b + " ",
			},
			want: "FAILED: ehrbase-service/templates/b b.v0.opt",
		},
		{
			name:   "record outside templates/",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": manifest + record("../templates/x.opt", a)},
			want:   "BAD RECORD: ../templates/x.opt",
		},
		{
			name:   "no source record",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": recordA + recordB},
			want:   "want exactly one source record",
		},
		{
			name:   "two source records",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": source + manifest},
			want:   "want exactly one source record",
		},
		{
			name:   "abbreviated commit",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": strings.Replace(manifest, commit, commit[:12], 1)},
			want:   "is not a full commit",
		},
		{
			name:   "another licence",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": strings.Replace(manifest, "Apache-2.0", "GPL-3.0", 1)},
			want:   "licence GPL-3.0, want Apache-2.0",
		},
		{
			name:   "repository not an https URL",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": strings.Replace(manifest, ehrbaseServiceRepo, "ehrbase/ehrbase", 1)},
			want:   "is not an https URL",
		},
		{
			name: "no file records",
			change: map[string]string{
				ehrbaseServiceDir + "/MANIFEST.txt":         source,
				ehrbaseServiceDir + "/templates/a.v0.opt":   "",
				ehrbaseServiceDir + "/templates/b b.v0.opt": "",
			},
			want: "lists no files",
		},
		{
			name:   "no manifest",
			change: map[string]string{ehrbaseServiceDir + "/MANIFEST.txt": ""},
			want:   "no MANIFEST.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := maps.Clone(base)
			maps.Copy(files, tc.change)
			for rel, body := range files {
				if body == "" {
					delete(files, rel)
				}
			}
			sdk := h.newSDK(t, "ehrbase/ehrbase develop "+commit, files)
			out, err := h.run(t, sdk, "verify")
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("verify = %v; want success\n%s", err, out)
			case tc.want == "" && !strings.Contains(out, "Integrity OK (2 file(s))"):
				t.Errorf("verify succeeded with %q; want it to count 2 files", out)
			case tc.want != "" && err == nil:
				t.Fatalf("verify succeeded; want a refusal naming %q\n%s", tc.want, out)
			case tc.want != "" && !strings.Contains(out, tc.want):
				t.Errorf("verify refused with %q; want it to name %q", out, tc.want)
			}
		})
	}
}

// The ingest copies every file of its table from a clone at the pin, byte for
// byte, under its template id, writes the manifest, removes a file the table
// no longer lists, and leaves the tree unchanged when run again at the same
// pin, the manifest's recorded_utc included; a new pin moves it. It refuses
// without a clone, with a clone that is not at the pin, with a file the clone
// changed since the pin, and with a file whose template id is not the one its
// table names. A refusal leaves the vendored tree as it was, even when the
// file it refuses is the last of the table, after every other file was staged.
func TestIngestEHRbaseServiceCopiesOnlyFromThePin(t *testing.T) {
	t.Parallel()
	h := newServiceHarness(t)
	table := ehrbaseServiceFiles(t)

	// The scratch upstream holds a stand-in template at every upstream path
	// the table reads, carrying the template id the table names for it.
	up := t.TempDir()
	h.git(t, up, "init", "-q")
	bodies := map[string]string{}
	for _, f := range table {
		body := scratchOPT(strings.TrimSuffix(filepath.Base(f.local), ".opt"))
		bodies[f.upstream] = body
		writeText(t, filepath.Join(up, filepath.FromSlash(f.upstream)), body)
	}
	h.git(t, up, "add", "-A")
	h.git(t, up, "commit", "-q", "-m", "upstream")
	pin := h.git(t, up, "rev-parse", "HEAD")
	clone := "EHRBASE_SERVICE_CLONE=" + up
	pinLine := "ehrbase/ehrbase develop " + pin
	stale := map[string]string{ehrbaseServiceDir + "/templates/stale.v0.opt": scratchOPT("stale.v0")}
	// Each case that commits upstream runs over its own clone, so a commit one
	// case makes cannot reach another.
	cloneOf := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "clone")
		h.git(t, up, "clone", "-q", up, dir)
		return dir
	}

	t.Run("at the pin", func(t *testing.T) {
		t.Parallel()
		sdk := h.newSDK(t, pinLine, stale)
		if out, err := h.run(t, sdk, "ingest", clone); err != nil {
			t.Fatalf("ingest = %v; want success\n%s", err, out)
		}
		dest := filepath.Join(sdk, filepath.FromSlash(ehrbaseServiceDir))
		got := tree(t, dest)
		want := map[string]string{}
		var manifest []string
		manifest = append(manifest, "source\tehrbase\t"+ehrbaseServiceRepo+"\t"+pin+"\tApache-2.0")
		for _, f := range table {
			want[f.local] = bodies[f.upstream]
			manifest = append(manifest, "file\t"+f.local+"\tehrbase\t"+f.upstream+"\t"+sha256Text(bodies[f.upstream]))
		}
		gotManifest := got["MANIFEST.txt"]
		delete(got, "MANIFEST.txt")
		if changed := changedFiles(want, got); len(changed) > 0 {
			t.Errorf("ingest left these files other than the clone holds them at the pin: %q", changed)
		}
		if recs := records(gotManifest); !slices.Equal(recs, manifest) {
			t.Errorf("ingest wrote the manifest records\n  %s\nwant\n  %s", strings.Join(recs, "\n  "), strings.Join(manifest, "\n  "))
		}
		if out, err := h.run(t, sdk, "verify"); err != nil {
			t.Errorf("verify after ingest = %v; want success\n%s", err, out)
		}
		// Stamp the manifest long ago, so that a second ingest which writes a
		// new stamp shows, in whatever second it runs.
		restamp(t, filepath.Join(dest, "MANIFEST.txt"), staleStamp)
		before := tree(t, dest)
		if out, err := h.run(t, sdk, "ingest", clone); err != nil {
			t.Fatalf("second ingest = %v; want success\n%s", err, out)
		}
		after := tree(t, dest)
		if got := recordedUTC(after["MANIFEST.txt"]); got != staleStamp {
			t.Errorf("a second ingest at the same pin wrote recorded_utc %q; want it to keep %q", got, staleStamp)
		}
		if changed := changedFiles(before, after); len(changed) > 0 {
			t.Errorf("a second ingest at the same pin changed %q", changed)
		}
	})

	// A new pin, with the table's files as they were, is the one change that
	// moves recorded_utc.
	t.Run("re-pin", func(t *testing.T) {
		t.Parallel()
		later := cloneOf(t)
		writeText(t, filepath.Join(later, "README.md"), "later\n")
		h.git(t, later, "add", "-A")
		h.git(t, later, "commit", "-q", "-m", "later")
		newPin := h.git(t, later, "rev-parse", "HEAD")

		sdk := h.newSDK(t, pinLine, nil)
		manifestPath := filepath.Join(sdk, filepath.FromSlash(ehrbaseServiceDir), "MANIFEST.txt")
		if out, err := h.run(t, sdk, "ingest", clone); err != nil {
			t.Fatalf("ingest at the first pin = %v; want success\n%s", err, out)
		}
		restamp(t, manifestPath, staleStamp)
		writePins(t, sdk, "ehrbase/ehrbase develop "+newPin)
		if out, err := h.run(t, sdk, "ingest", "EHRBASE_SERVICE_CLONE="+later); err != nil {
			t.Fatalf("ingest at the new pin = %v; want success\n%s", err, out)
		}
		manifest := readText(t, manifestPath)
		stamp := recordedUTC(manifest)
		if _, err := time.Parse(time.RFC3339, stamp); err != nil || stamp == staleStamp {
			t.Errorf("ingest at a new pin wrote recorded_utc %q; want a new time in RFC 3339 form, not %q", stamp, staleStamp)
		}
		source := "source\tehrbase\t" + ehrbaseServiceRepo + "\t" + newPin + "\tApache-2.0"
		if recs := records(manifest); !slices.Contains(recs, source) {
			t.Errorf("ingest at a new pin wrote the manifest records\n  %s\nwant the source record\n  %s", strings.Join(recs, "\n  "), source)
		}
	})

	// The refusals that break a table file break the last one, so that every
	// other file is staged by the time the run refuses.
	last := table[len(table)-1]
	// What a refused run must leave as it was: a template the table no longer
	// lists and a manifest.
	vendored := maps.Clone(stale)
	vendored[ehrbaseServiceDir+"/MANIFEST.txt"] = "# the previous ingest\n"
	cases := []struct {
		name string
		// setup returns the pin line and the environment of the run.
		setup func(t *testing.T) (pinLine string, env []string)
		want  string
	}{
		{
			name:  "no clone",
			setup: func(*testing.T) (string, []string) { return pinLine, nil },
			want:  "to a local clone of",
		},
		{
			name: "clone is not a directory",
			setup: func(t *testing.T) (string, []string) {
				return pinLine, []string{"EHRBASE_SERVICE_CLONE=" + filepath.Join(t.TempDir(), "absent")}
			},
			want: "clone not found",
		},
		{
			name: "no pin",
			setup: func(*testing.T) (string, []string) {
				return "ehrbase/openEHR_SDK develop " + pin, []string{clone}
			},
			want: "no full commit for ehrbase/ehrbase",
		},
		{
			name: "clone past the pin",
			setup: func(t *testing.T) (string, []string) {
				dir := cloneOf(t)
				writeText(t, filepath.Join(dir, "README.md"), "later\n")
				h.git(t, dir, "add", "-A")
				h.git(t, dir, "commit", "-q", "-m", "later")
				return pinLine, []string{"EHRBASE_SERVICE_CLONE=" + dir}
			},
			want: "refusing to vendor from an unpinned tree",
		},
		{
			name: "file changed in the clone since the pin",
			setup: func(t *testing.T) (string, []string) {
				dir := cloneOf(t)
				writeText(t, filepath.Join(dir, filepath.FromSlash(last.upstream)), bodies[last.upstream]+"<!-- edited -->\n")
				return pinLine, []string{"EHRBASE_SERVICE_CLONE=" + dir}
			},
			want: "differs from the blob at its pin",
		},
		{
			name: "template id not the one the table names",
			setup: func(t *testing.T) (string, []string) {
				dir := cloneOf(t)
				writeText(t, filepath.Join(dir, filepath.FromSlash(last.upstream)), scratchOPT("another.v0"))
				h.git(t, dir, "commit", "-q", "-am", "rename")
				return "ehrbase/ehrbase develop " + h.git(t, dir, "rev-parse", "HEAD"), []string{"EHRBASE_SERVICE_CLONE=" + dir}
			},
			want: "holds template id another.v0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pinLine, env := tc.setup(t)
			sdk := h.newSDK(t, pinLine, vendored)
			before := tree(t, sdk)
			out, err := h.run(t, sdk, "ingest", env...)
			if err == nil {
				t.Fatalf("ingest succeeded; want a refusal naming %q\n%s", tc.want, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("ingest refused with %q; want it to name %q", out, tc.want)
			}
			if changed := changedFiles(before, tree(t, sdk)); len(changed) > 0 {
				t.Errorf("a refused ingest changed the vendored tree: %q", changed)
			}
		})
	}
}
