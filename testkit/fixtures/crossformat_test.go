package fixtures

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The cross-format corpus is the input to PROBE-105 (REQ-080). Its
// MANIFEST.txt must record, per file, the source repository, commit,
// upstream path and sha256, and the tests below check its integrity offline
// under `make ci`. Each check is a function over a corpus root so that the
// scratch-corpus tests further down can show it turns red when the property
// it guards is broken.

// crossFormatIntegrityFindings returns one message per file or opt record in
// m whose file under corpusRoot is missing or does not hash to the sha256 the
// record holds.
func crossFormatIntegrityFindings(corpusRoot string, m crossFormatManifestData) []string {
	var out []string
	check := func(rel, want string) {
		raw, err := os.ReadFile(filepath.Join(corpusRoot, filepath.FromSlash(rel)))
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", rel, err))
			return
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != want {
			out = append(out, fmt.Sprintf("%s: sha256 %s, manifest records %s", rel, got, want))
		}
	}
	for _, f := range m.files {
		check("crossformat/"+f.set+"/"+f.local, f.sha256)
	}
	for _, p := range m.opts {
		check(p.target, p.sha256)
	}
	return out
}

// crossFormatClosureFindings returns one message per entry under
// corpusRoot/crossformat/, other than the top-level MANIFEST.txt, that is not
// a regular file listed by a file record of m.
func crossFormatClosureFindings(corpusRoot string, m crossFormatManifestData) ([]string, error) {
	root := filepath.Join(corpusRoot, "crossformat")
	listed := map[string]bool{crossFormatManifest: true}
	for _, f := range m.files {
		listed[f.set+"/"+f.local] = true
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case !d.Type().IsRegular():
			out = append(out, rel+": not a regular file")
		case !listed[rel]:
			out = append(out, rel+": not listed by a file record")
		}
		return nil
	})
	return out, err
}

// crossFormatShapeFindings returns one message per set of m that does not
// have exactly one OPT (a template.opt file or an opt pointer, never both) or
// that has fewer than two of the four formats.
func crossFormatShapeFindings(m crossFormatManifestData) []string {
	type shape struct{ opts, formats int }
	sets := map[string]*shape{}
	get := func(name string) *shape {
		if sets[name] == nil {
			sets[name] = &shape{}
		}
		return sets[name]
	}
	for _, f := range m.files {
		if f.local == crossFormatTemplate {
			get(f.set).opts++
		} else {
			get(f.set).formats++
		}
	}
	for _, p := range m.opts {
		get(p.set).opts++
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(sets)) {
		s := sets[name]
		if s.opts != 1 {
			out = append(out, fmt.Sprintf("%s: %d OPTs (template.opt and opt pointers), want exactly 1", name, s.opts))
		}
		if s.formats < 2 {
			out = append(out, fmt.Sprintf("%s: %d formats, want at least 2", name, s.formats))
		}
	}
	return out
}

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// crossFormatProvenanceFindings returns one message per source record of m
// that does not name an https repository, a full commit id, and the
// Apache-2.0 licence the corpus is restricted to.
func crossFormatProvenanceFindings(m crossFormatManifestData) []string {
	var out []string
	for _, s := range m.sources {
		if !strings.HasPrefix(s.repo, "https://") {
			out = append(out, fmt.Sprintf("source %s: repository %q is not an https URL", s.name, s.repo))
		}
		if !fullCommit.MatchString(s.commit) {
			out = append(out, fmt.Sprintf("source %s: commit %q is not a full commit id", s.name, s.commit))
		}
		if s.licence != "Apache-2.0" {
			out = append(out, fmt.Sprintf("source %s: licence %q, want Apache-2.0", s.name, s.licence))
		}
	}
	return out
}

func loadCrossFormatManifest(t *testing.T) crossFormatManifestData {
	t.Helper()
	m, err := readCrossFormatManifest(filepath.Join(CrossFormatRoot(), crossFormatManifest))
	if err != nil {
		t.Fatalf("read the cross-format manifest: %v", err)
	}
	return m
}

func TestCrossFormatManifestIntegrity_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	m := loadCrossFormatManifest(t)
	for _, msg := range crossFormatIntegrityFindings(CorpusRoot(), m) {
		t.Errorf("integrity: %s (re-run scripts/ingest-crossformat.sh ingest at the pins; never edit a vendored file)", msg)
	}
	if len(m.opts) == 0 {
		t.Errorf("manifest has no opt records; the sets that reuse an OPT vendored elsewhere are not checked")
	}
}

func TestCrossFormatManifestProvenance_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	m := loadCrossFormatManifest(t)
	if len(m.sources) == 0 {
		t.Fatal("manifest declares no source")
	}
	for _, msg := range crossFormatProvenanceFindings(m) {
		t.Error(msg)
	}
}

func TestCrossFormatClosure_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	m := loadCrossFormatManifest(t)
	found, err := crossFormatClosureFindings(CorpusRoot(), m)
	if err != nil {
		t.Fatalf("walk %s: %v", CrossFormatRoot(), err)
	}
	for _, msg := range found {
		t.Errorf("closure: %s (vendor it through scripts/ingest-crossformat.sh)", msg)
	}
}

func TestCrossFormatShape_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	for _, msg := range crossFormatShapeFindings(loadCrossFormatManifest(t)) {
		t.Errorf("shape: %s", msg)
	}
}

func TestListCrossFormatSets_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	got, err := ListCrossFormatSets()
	if err != nil {
		t.Fatalf("ListCrossFormatSets() error: %v", err)
	}
	cf := func(set, name string) string { return filepath.Join(CrossFormatRoot(), set, name) }
	vendored := func(rel string) string { return filepath.Join(CorpusRoot(), filepath.FromSlash(rel)) }
	want := []CrossFormatSet{
		{Name: "alternative_events", OPT: cf("alternative_events", "template.opt"), CanonicalJSON: cf("alternative_events", "canonical.json"), FLAT: cf("alternative_events", "flat.json")},
		{Name: "consult_record", OPT: cf("consult_record", "template.opt"), CanonicalJSON: cf("consult_record", "canonical.json"), CanonicalXML: cf("consult_record", "canonical.xml"), FLAT: cf("consult_record", "flat.json"), STRUCTURED: cf("consult_record", "structured.json")},
		{Name: "corona", OPT: vendored("webtemplate/Corona_Anamnese.opt"), CanonicalJSON: cf("corona", "canonical.json"), FLAT: cf("corona", "flat.json"), STRUCTURED: cf("corona", "structured.json")},
		{Name: "ehrn_abdm", OPT: cf("ehrn_abdm", "template.opt"), CanonicalJSON: cf("ehrn_abdm", "canonical.json"), FLAT: cf("ehrn_abdm", "flat.json")},
		{Name: "family_history", OPT: cf("family_history", "template.opt"), CanonicalJSON: cf("family_history", "canonical.json"), CanonicalXML: cf("family_history", "canonical.xml")},
		{Name: "multi_list", OPT: cf("multi_list", "template.opt"), FLAT: cf("multi_list", "flat.json"), STRUCTURED: cf("multi_list", "structured.json")},
		{Name: "multi_occurrence", OPT: cf("multi_occurrence", "template.opt"), CanonicalJSON: cf("multi_occurrence", "canonical.json"), FLAT: cf("multi_occurrence", "flat.json")},
		{Name: "nested", OPT: vendored("templates/nested.en.v1.opt"), CanonicalXML: cf("nested", "canonical.xml"), FLAT: cf("nested", "flat.json")},
		{Name: "persistent_minimal", OPT: vendored("templates/persistent_minimal.en.v1.opt"), CanonicalXML: cf("persistent_minimal", "canonical.xml"), FLAT: cf("persistent_minimal", "flat.json")},
		{Name: "test_all_types", OPT: cf("test_all_types", "template.opt"), CanonicalJSON: cf("test_all_types", "canonical.json"), FLAT: cf("test_all_types", "flat.json")},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ListCrossFormatSets() returned %d sets, want %d:\n%s", len(got), len(want), diffSets(got, want))
	}
	for _, s := range got {
		for _, p := range []string{s.OPT, s.CanonicalJSON, s.CanonicalXML, s.FLAT, s.STRUCTURED} {
			if p == "" {
				continue
			}
			if _, err := os.Stat(p); err != nil {
				t.Errorf("set %s: %v", s.Name, err)
			}
		}
	}
}

// diffSets prints every position where got and want differ.
func diffSets(got, want []CrossFormatSet) string {
	var b strings.Builder
	for i := range max(len(got), len(want)) {
		var g, w CrossFormatSet
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			fmt.Fprintf(&b, "  [%d]\n    got  %+v\n    want %+v\n", i, g, w)
		}
	}
	return b.String()
}

// The tests below are the can-fail controls: each builds a consistent
// scratch corpus, checks the guard passes on it, breaks the property the
// guard protects, and checks the guard names the break.

const scratchSource = "source\tsdk\thttps://example.invalid/sdk\t0123456789abcdef0123456789abcdef01234567\tApache-2.0\n"

// scratchCrossFormat writes a consistent two-set corpus under a temporary
// root and returns the root. Set a reuses templates/shared.opt through an opt
// pointer; set b carries its own template.opt.
func scratchCrossFormat(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"templates/shared.opt":          "<template>shared</template>\n",
		"crossformat/a/canonical.json":  `{"_type":"COMPOSITION"}`,
		"crossformat/a/flat.json":       `{"a/_uid":"1"}`,
		"crossformat/b/template.opt":    "<template>b</template>\n",
		"crossformat/b/canonical.xml":   "<composition/>\n",
		"crossformat/b/structured.json": `{"b":{}}`,
	}
	var manifest strings.Builder
	manifest.WriteString("# scratch\n" + scratchSource)
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		writeScratch(t, root, rel, files[rel])
		if local, ok := strings.CutPrefix(rel, "crossformat/"); ok {
			fmt.Fprintf(&manifest, "file\t%s\tsdk\tup/%s\t%s\n", local, local, sha256Hex(files[rel]))
		}
	}
	fmt.Fprintf(&manifest, "opt\ta\ttemplates/shared.opt\tsdk\tup/shared.opt\t%s\n", sha256Hex(files["templates/shared.opt"]))
	writeScratch(t, root, "crossformat/"+crossFormatManifest, manifest.String())
	return root
}

func writeScratch(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendScratch(t *testing.T, root, rel, extra string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(extra); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func readScratchManifest(t *testing.T, root string) crossFormatManifestData {
	t.Helper()
	m, err := readCrossFormatManifest(filepath.Join(root, "crossformat", crossFormatManifest))
	if err != nil {
		t.Fatalf("read scratch manifest: %v", err)
	}
	return m
}

// wantFindings fails unless found holds exactly one finding per prefix, in
// any order. Path findings start with "path:", set findings with "set: ".
func wantFindings(t *testing.T, check string, found []string, prefixes ...string) {
	t.Helper()
	if len(found) != len(prefixes) {
		t.Fatalf("%s found %d problems, want %d starting %q:\n  %s", check, len(found), len(prefixes), prefixes, strings.Join(found, "\n  "))
	}
	for _, p := range prefixes {
		if !slices.ContainsFunc(found, func(msg string) bool { return strings.HasPrefix(msg, p) }) {
			t.Errorf("%s has no problem starting %q; found:\n  %s", check, p, strings.Join(found, "\n  "))
		}
	}
}

func TestCrossFormatIntegrityFindsAlteredAndMissingFiles_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	root := scratchCrossFormat(t)
	m := readScratchManifest(t, root)
	wantFindings(t, "integrity on the consistent corpus", crossFormatIntegrityFindings(root, m))

	appendScratch(t, root, "crossformat/a/flat.json", " ")
	appendScratch(t, root, "templates/shared.opt", " ")
	if err := os.Remove(filepath.Join(root, "crossformat", "b", "structured.json")); err != nil {
		t.Fatal(err)
	}
	wantFindings(t, "integrity on the altered corpus", crossFormatIntegrityFindings(root, m),
		"crossformat/a/flat.json: sha256", "templates/shared.opt: sha256", "crossformat/b/structured.json:")
}

func TestCrossFormatClosureFindsUnlistedFiles_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	root := scratchCrossFormat(t)
	m := readScratchManifest(t, root)
	found, err := crossFormatClosureFindings(root, m)
	if err != nil {
		t.Fatal(err)
	}
	wantFindings(t, "closure on the consistent corpus", found)

	writeScratch(t, root, "crossformat/b/flat.json", `{"b/_uid":"2"}`)
	writeScratch(t, root, "crossformat/c/canonical.json", `{}`)
	writeScratch(t, root, "crossformat/a/MANIFEST.txt", "")
	found, err = crossFormatClosureFindings(root, m)
	if err != nil {
		t.Fatal(err)
	}
	wantFindings(t, "closure with unlisted files", found, "b/flat.json:", "c/canonical.json:", "a/MANIFEST.txt:")
}

func TestCrossFormatShapeFindsBadSets_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	const sha = "0000000000000000000000000000000000000000000000000000000000000000"
	file := func(rel string) string { return "file\t" + rel + "\tsdk\tup/" + rel + "\t" + sha + "\n" }
	pointer := func(set string) string { return "opt\t" + set + "\ttemplates/x.opt\tsdk\tup/x.opt\t" + sha + "\n" }
	ok := file("s/template.opt") + file("s/canonical.json") + file("s/flat.json")
	cases := []struct {
		name     string
		manifest string
		want     []string // the prefix of each finding
	}{
		{name: "own template and two formats", manifest: ok},
		{name: "pointer and two formats", manifest: file("s/canonical.xml") + file("s/structured.json") + pointer("s")},
		{name: "template and pointer", manifest: ok + pointer("s"), want: []string{"s: 2 OPTs"}},
		{name: "no OPT", manifest: file("s/canonical.json") + file("s/flat.json"), want: []string{"s: 0 OPTs"}},
		{name: "one format", manifest: file("s/template.opt") + file("s/canonical.json"), want: []string{"s: 1 formats"}},
		{name: "OPT only", manifest: file("s/template.opt"), want: []string{"s: 0 formats"}},
		{name: "pointer to a set without files", manifest: ok + pointer("t"), want: []string{"t: 0 formats"}},
		{name: "no OPT and one format", manifest: file("s/flat.json"), want: []string{"s: 0 OPTs", "s: 1 formats"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseCrossFormatManifest(strings.NewReader(scratchSource + tc.manifest))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			wantFindings(t, "shape", crossFormatShapeFindings(m), tc.want...)
		})
	}
}

func TestCrossFormatProvenanceFindsBadSources_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	const commit = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name, source string
		want         []string // the prefix of each finding
	}{
		{name: "pinned Apache-2.0 https source", source: scratchSource},
		{name: "other licence", source: "source\tsdk\thttps://example.invalid/sdk\t" + commit + "\tGPL-3.0\n", want: []string{"source sdk: licence"}},
		{name: "abbreviated commit", source: "source\tsdk\thttps://example.invalid/sdk\t0123456\tApache-2.0\n", want: []string{"source sdk: commit"}},
		{name: "branch instead of commit", source: "source\tsdk\thttps://example.invalid/sdk\tdevelop\tApache-2.0\n", want: []string{"source sdk: commit"}},
		{name: "not a URL", source: "source\tsdk\tehrbase/openEHR_SDK\t" + commit + "\tApache-2.0\n", want: []string{"source sdk: repository"}},
	}
	const files = "file\ts/canonical.json\tsdk\tup/c.json\tx\nfile\ts/flat.json\tsdk\tup/f.json\tx\n"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseCrossFormatManifest(strings.NewReader(tc.source + files))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			wantFindings(t, "provenance", crossFormatProvenanceFindings(m), tc.want...)
		})
	}
}

func TestParseCrossFormatManifestRefusesMalformedRecords_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	const okFile = "file\ts/canonical.json\tsdk\tup/c.json\tx\n"
	cases := []struct{ name, body, want string }{
		{name: "unknown kind", body: okFile + "files\ts/flat.json\tsdk\tup\tx\n", want: `unknown record kind "files"`},
		{name: "missing field", body: okFile + "file\ts/flat.json\tsdk\tx\n", want: "file record has 4 non-empty fields, want 5"},
		{name: "empty field", body: okFile + "file\ts/flat.json\tsdk\t\tx\n", want: "file record has 4 non-empty fields, want 5"},
		{name: "unknown file name", body: okFile + "file\ts/flat.xml\tsdk\tup\tx\n", want: `unknown file name "flat.xml"`},
		{name: "file outside a set", body: okFile + "file\tflat.json\tsdk\tup\tx\n", want: "is not {set}/{file name}"},
		{name: "set is a parent reference", body: okFile + "file\t../flat.json\tsdk\tup\tx\n", want: "is not {set}/{file name}"},
		{name: "nested file", body: okFile + "file\ts/x/flat.json\tsdk\tup\tx\n", want: `unknown file name "x/flat.json"`},
		{name: "opt target escapes the corpus", body: okFile + "opt\ts\t../x.opt\tsdk\tup\tx\n", want: "is not a path inside testkit/corpus/"},
		{name: "opt target is absolute", body: okFile + "opt\ts\t/etc/x.opt\tsdk\tup\tx\n", want: "is not a path inside testkit/corpus/"},
		{name: "undeclared source", body: okFile + "file\ts/flat.json\trobot\tup\tx\n", want: `names undeclared source "robot"`},
		{name: "undeclared opt source", body: okFile + "opt\ts\ttemplates/x.opt\trobot\tup\tx\n", want: `names undeclared source "robot"`},
		{name: "file listed twice", body: okFile + okFile, want: "file s/canonical.json listed twice"},
		{name: "two opt pointers", body: okFile + "opt\ts\tt/x.opt\tsdk\tup\tx\nopt\ts\tt/y.opt\tsdk\tup\tx\n", want: `set "s" has two opt pointers`},
		{name: "source declared twice", body: scratchSource + okFile, want: `source "sdk" declared twice`},
		{name: "no files", body: "# only a comment\n", want: "manifest lists no files"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCrossFormatManifest(strings.NewReader(scratchSource + tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parseCrossFormatManifest() error = %v, want one containing %q", err, tc.want)
			}
		})
	}
	if _, err := parseCrossFormatManifest(strings.NewReader(scratchSource + okFile)); err != nil {
		t.Errorf("parseCrossFormatManifest() on a well-formed manifest: %v", err)
	}
}

func TestListCrossFormatSetsOverScratchCorpus_PROBE105_REQ080(t *testing.T) {
	t.Parallel()
	root := scratchCrossFormat(t)
	got, err := listCrossFormatSets(root)
	if err != nil {
		t.Fatalf("listCrossFormatSets() error: %v", err)
	}
	cf := func(rel string) string { return filepath.Join(root, "crossformat", filepath.FromSlash(rel)) }
	want := []CrossFormatSet{
		{Name: "a", OPT: filepath.Join(root, "templates", "shared.opt"), CanonicalJSON: cf("a/canonical.json"), FLAT: cf("a/flat.json")},
		{Name: "b", OPT: cf("b/template.opt"), CanonicalXML: cf("b/canonical.xml"), STRUCTURED: cf("b/structured.json")},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("listCrossFormatSets() differs:\n%s", diffSets(got, want))
	}

	cases := []struct{ name, extra, want string }{
		{name: "template and pointer", extra: "opt\tb\ttemplates/shared.opt\tsdk\tup\tx\n", want: `set "b" has both a template.opt and an opt pointer`},
		{name: "pointer without files", extra: "opt\tz\ttemplates/shared.opt\tsdk\tup\tx\n", want: `opt pointer for set "z", which lists no files`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := t.TempDir()
			body, err := os.ReadFile(cf(crossFormatManifest))
			if err != nil {
				t.Fatal(err)
			}
			writeScratch(t, bad, "crossformat/"+crossFormatManifest, string(body)+tc.extra)
			_, err = listCrossFormatSets(bad)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("listCrossFormatSets() error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestIngestCrossFormatVerifyReadsTheLastRecord_PROBE105_REQ080 pins the
// offline integrity check of scripts/ingest-crossformat.sh verify, which
// `make crossformat-verify` runs: a MANIFEST.txt whose last record has no
// final newline must still have that record checked. The last record here is
// an opt pointer, which the subtree closure check cannot catch, so a verify
// that drops the line passes over the altered OPT.
func TestIngestCrossFormatVerifyReadsTheLastRecord_PROBE105_REQ080(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		if _, err := exec.LookPath("shasum"); err != nil {
			t.Skip("neither sha256sum nor shasum found")
		}
	}
	script, err := os.ReadFile(filepath.Join(CorpusRoot(), "..", "..", "scripts", "ingest-crossformat.sh"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	digest := func(data []byte) string {
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	flat, opt := []byte(`{"ctx/language":"en"}`), []byte("<template/>\n")
	write("scripts/ingest-crossformat.sh", script)
	write("testkit/corpus/crossformat/s/flat.json", flat)
	write("testkit/corpus/templates/s.opt", opt)
	manifest := "# generated\n" +
		"source\tsdk\thttps://example.org/sdk\t0123456789abcdef0123456789abcdef01234567\tApache-2.0\n" +
		"file\ts/flat.json\tsdk\tflat/s.json\t" + digest(flat) + "\n" +
		"opt\ts\ttemplates/s.opt\tsdk\topt/s.opt\t" + digest(opt) // no final newline
	write("testkit/corpus/crossformat/MANIFEST.txt", []byte(manifest))

	verify := func() (string, error) {
		cmd := exec.Command(bash, filepath.Join(root, "scripts", "ingest-crossformat.sh"), "verify")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := verify(); err != nil || !strings.Contains(out, "1 OPT pointer(s)") {
		t.Fatalf("verify over an intact tree = %v, %q; want success that counts the last record's OPT pointer", err, out)
	}
	write("testkit/corpus/templates/s.opt", []byte("<template/> \n"))
	if out, err := verify(); err == nil {
		t.Errorf("verify over an altered OPT named by the last, newline-less record succeeded: %q", out)
	}
}
