package fixtures_test

import (
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// testkit/corpus/reference/ holds the operational templates of
// Cadasto/openehr-reference-content, copied byte for byte at the commit
// UPSTREAM_PINS.txt names. scripts/ingest-reference-content.sh copies them
// (ingest) and checks them offline (verify). The first test runs verify on
// the vendored tree, so `make test` holds the corpus to its manifest. The
// others run the script in scratch trees laid out like this repository, and
// each shows that a check refuses what it guards against.

const (
	referenceScript = "scripts/ingest-reference-content.sh"
	referenceDir    = "testkit/corpus/reference"
	referenceRepo   = "https://github.com/Cadasto/openehr-reference-content"
	referenceLic    = "CC-BY-SA-4.0"
)

func TestReferenceContentCorpusVerifies(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script := filepath.Join(fixtures.CorpusRoot(), "..", "..", filepath.FromSlash(referenceScript))
	out, err := exec.CommandContext(t.Context(), bash, script, "verify").CombinedOutput()
	if err != nil {
		t.Fatalf("%s verify on the vendored tree: %v "+
			"(re-run `make reference-content-sync` at the pin; never edit a vendored file)\n%s", referenceScript, err, out)
	}
}

// Every template of the manifest reaches the whole-corpus tests through
// fixtures.ListAllOPTs, named by its stem under the reference label, and
// ReferenceTemplatesDir is the folder that holds them.
func TestListAllOPTsNamesTheReferenceTemplates(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs() error: %v", err)
	}
	text := readText(t, filepath.Join(fixtures.CorpusRoot(), "reference", "MANIFEST.txt"))
	n := 0
	for _, line := range records(text) {
		f := strings.Split(line, "\t")
		if f[0] != "file" {
			continue
		}
		n++
		want := fixtures.OPTRef{
			Name: "reference/" + strings.TrimSuffix(filepath.Base(f[1]), ".opt"),
			Path: filepath.Join(fixtures.ReferenceTemplatesDir(), filepath.Base(f[1])),
		}
		if !slices.Contains(refs, want) {
			t.Errorf("ListAllOPTs() has no %+v", want)
		}
	}
	if n == 0 {
		t.Fatal("the reference manifest lists no files")
	}
}

// runReference runs scripts/ingest-reference-content.sh in the scratch tree
// sdk with mode and only env added to the harness's environment.
func runReference(t *testing.T, h *serviceHarness, sdk, mode string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), h.bash, filepath.Join(sdk, filepath.FromSlash(referenceScript)), mode)
	cmd.Env = slices.Concat(h.env, env)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// newReferenceSDK returns a scratch tree laid out like this repository,
// holding the reference script, an UPSTREAM_PINS.txt with pinLine as its one
// pin line, and files, by path relative to the tree's root.
func newReferenceSDK(t *testing.T, pinLine string, files map[string]string) string {
	t.Helper()
	sdk := t.TempDir()
	script := readText(t, filepath.Join(fixtures.CorpusRoot(), "..", "..", filepath.FromSlash(referenceScript)))
	writeText(t, filepath.Join(sdk, filepath.FromSlash(referenceScript)), script)
	writePins(t, sdk, pinLine)
	for rel, body := range files {
		writeText(t, filepath.Join(sdk, filepath.FromSlash(rel)), body)
	}
	return sdk
}

// The offline check: every manifest record names a file whose sha256 it
// holds, every file in the subtree is listed, and the one source record
// names an https repository, a full commit and the CC-BY-SA-4.0 licence.
func TestIngestReferenceContentVerifyRefusesADisagreeingTree(t *testing.T) {
	t.Parallel()
	h := newServiceHarness(t)
	const commit = "0123456789abcdef0123456789abcdef01234567"
	a, b := scratchOPT("A.v1"), scratchOPT("B.v1")
	source := "source\treference\t" + referenceRepo + "\t" + commit + "\t" + referenceLic + "\n"
	record := func(id, body string) string {
		return "file\ttemplates/" + id + ".opt\treference\ttech/templates/" + id + "/opt/" + id + ".opt\t" + sha256Text(body) + "\n"
	}
	manifest := "# generated\n" + source + record("A.v1", a) + record("B.v1", b)
	base := map[string]string{
		referenceDir + "/MANIFEST.txt":       manifest,
		referenceDir + "/templates/A.v1.opt": a,
		referenceDir + "/templates/B.v1.opt": b,
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
			change: map[string]string{referenceDir + "/templates/B.v1.opt": b + " "},
			want:   "FAILED: reference/templates/B.v1.opt",
		},
		{
			name:   "missing file",
			change: map[string]string{referenceDir + "/templates/A.v1.opt": ""},
			want:   "MISSING: reference/templates/A.v1.opt",
		},
		{
			name:   "unlisted template",
			change: map[string]string{referenceDir + "/templates/C.v1.opt": scratchOPT("C.v1")},
			want:   "UNTRACKED: reference/templates/C.v1.opt",
		},
		{
			name:   "another licence",
			change: map[string]string{referenceDir + "/MANIFEST.txt": strings.Replace(manifest, referenceLic, "MIT", 1)},
			want:   "licence MIT, want " + referenceLic,
		},
		{
			name:   "abbreviated commit",
			change: map[string]string{referenceDir + "/MANIFEST.txt": strings.Replace(manifest, commit, commit[:12], 1)},
			want:   "is not a full commit",
		},
		{
			name:   "no source record",
			change: map[string]string{referenceDir + "/MANIFEST.txt": record("A.v1", a) + record("B.v1", b)},
			want:   "want exactly one source record",
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
			sdk := newReferenceSDK(t, "Cadasto/openehr-reference-content main "+commit, files)
			out, err := runReference(t, h, sdk, "verify")
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("verify = %v; want success\n%s", err, out)
			case tc.want == "" && !strings.Contains(out, "(2 file(s))"):
				t.Errorf("verify succeeded with %q; want it to count 2 files", out)
			case tc.want != "" && err == nil:
				t.Fatalf("verify succeeded; want a refusal naming %q\n%s", tc.want, out)
			case tc.want != "" && !strings.Contains(out, tc.want):
				t.Errorf("verify refused with %q; want it to name %q", out, tc.want)
			}
		})
	}
}

// The ingest copies every template the upstream holds at the pin, whatever
// its clone has checked out, writes a manifest that records the pin's tag,
// and writes the same bytes when run again. It refuses a pin that no tag
// names, and a template whose id is not its folder's name.
func TestIngestReferenceContentCopiesTheTemplatesAtThePin(t *testing.T) {
	t.Parallel()
	h := newServiceHarness(t)

	upstream := func(t *testing.T, bodies map[string]string, tag string) (dir, pin string) {
		t.Helper()
		dir = t.TempDir()
		h.git(t, dir, "init", "-q")
		for id, body := range bodies {
			writeText(t, filepath.Join(dir, "tech", "templates", id, "opt", id+".opt"), body)
		}
		writeText(t, filepath.Join(dir, "README.md"), "upstream\n")
		h.git(t, dir, "add", "-A")
		h.git(t, dir, "commit", "-q", "-m", "templates")
		pin = h.git(t, dir, "rev-parse", "HEAD")
		if tag != "" {
			h.git(t, dir, "tag", tag)
		}
		return dir, pin
	}
	pinLine := func(pin string) string { return "Cadasto/openehr-reference-content main " + pin }

	t.Run("at the pin", func(t *testing.T) {
		t.Parallel()
		bodies := map[string]string{"A.v1": scratchOPT("A.v1"), "B.v1": scratchOPT("B.v1")}
		up, pin := upstream(t, bodies, "v9.9.9")
		// A later commit and an uncommitted edit the ingest must not read.
		writeText(t, filepath.Join(up, "tech", "templates", "A.v1", "opt", "A.v1.opt"), scratchOPT("A.v1")+"<!--later-->\n")
		h.git(t, up, "commit", "-q", "-am", "later")
		writeText(t, filepath.Join(up, "tech", "templates", "B.v1", "opt", "B.v1.opt"), "edited\n")

		sdk := newReferenceSDK(t, pinLine(pin), map[string]string{referenceDir + "/templates/stale.v1.opt": scratchOPT("stale.v1")})
		clone := "REFERENCE_CONTENT_CLONE=" + up
		if out, err := runReference(t, h, sdk, "ingest", clone); err != nil {
			t.Fatalf("ingest = %v; want success\n%s", err, out)
		}
		dest := filepath.Join(sdk, filepath.FromSlash(referenceDir))
		got := tree(t, dest)
		manifest := got["MANIFEST.txt"]
		delete(got, "MANIFEST.txt")
		want := map[string]string{"templates/A.v1.opt": bodies["A.v1"], "templates/B.v1.opt": bodies["B.v1"]}
		if changed := changedFiles(want, got); len(changed) > 0 {
			t.Errorf("ingest left these files other than the upstream holds them at the pin: %q", changed)
		}
		wantRecords := []string{
			"source\treference\t" + referenceRepo + "\t" + pin + "\t" + referenceLic,
			"file\ttemplates/A.v1.opt\treference\ttech/templates/A.v1/opt/A.v1.opt\t" + sha256Text(bodies["A.v1"]),
			"file\ttemplates/B.v1.opt\treference\ttech/templates/B.v1/opt/B.v1.opt\t" + sha256Text(bodies["B.v1"]),
		}
		if recs := records(manifest); !slices.Equal(recs, wantRecords) {
			t.Errorf("ingest wrote the manifest records\n  %s\nwant\n  %s", strings.Join(recs, "\n  "), strings.Join(wantRecords, "\n  "))
		}
		if !strings.Contains(manifest, "# tag: v9.9.9\n") {
			t.Errorf("ingest wrote a manifest without the pin's tag:\n%s", manifest)
		}
		before := tree(t, dest)
		if out, err := runReference(t, h, sdk, "ingest", clone); err != nil {
			t.Fatalf("second ingest = %v; want success\n%s", err, out)
		}
		if changed := changedFiles(before, tree(t, dest)); len(changed) > 0 {
			t.Errorf("a second ingest at the same pin changed %q; want the same bytes", changed)
		}
	})

	refusals := []struct {
		name   string
		bodies map[string]string
		tag    string
		want   string
	}{
		{name: "untagged pin", bodies: map[string]string{"A.v1": scratchOPT("A.v1")}, want: "is not a released tag's commit"},
		{name: "template id not its folder's", bodies: map[string]string{"A.v1": scratchOPT("Other.v1")}, tag: "v9.9.9", want: "holds template id Other.v1"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up, pin := upstream(t, tc.bodies, tc.tag)
			prior := map[string]string{referenceDir + "/templates/kept.v1.opt": scratchOPT("kept.v1")}
			sdk := newReferenceSDK(t, pinLine(pin), prior)
			out, err := runReference(t, h, sdk, "ingest", "REFERENCE_CONTENT_CLONE="+up)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("ingest = %v, %q; want a refusal naming %q", err, out, tc.want)
			}
			got := tree(t, filepath.Join(sdk, filepath.FromSlash(referenceDir)))
			if changed := changedFiles(map[string]string{"templates/kept.v1.opt": scratchOPT("kept.v1")}, got); len(changed) > 0 {
				t.Errorf("a refused ingest changed the vendored tree: %q", changed)
			}
		})
	}
}
