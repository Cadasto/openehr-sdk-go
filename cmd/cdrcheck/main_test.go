// Tests of the CDR cross-check (PROBE-108, for REQ-107 and REQ-102). Every
// CDR here is a fakeCDR on httptest; no test reaches a live deployment.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// readJSONL decodes one JSON Lines file of a run's output.
func readJSONL[T any](t *testing.T, out, name string) []T {
	t.Helper()
	f, err := os.Open(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var recs []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r T
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		recs = append(recs, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return recs
}

func readSummary(t *testing.T, out string) summary {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRun_SkipsWhenUnset_PROBE108_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	cases := []struct {
		name string
		env  env
	}{
		{name: "nothing set", env: env{}},
		{name: "a CDR but no template directory", env: env{
			"OPENEHR_LIVE_EHRBASE": cdr.url(), envAllowMutating: "1",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, out, err := runFor(t, tc.env)
			if err != nil {
				t.Fatalf("run(%v) = %v, want nil: a run with nothing to check skips", tc.env, err)
			}
			if strings.TrimSpace(stdout) != skipNotice {
				t.Errorf("run(%v) printed %q, want the skip notice %q", tc.env, stdout, skipNotice)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Errorf("run(%v) created the output directory %s (stat err %v), want nothing written", tc.env, out, err)
			}
		})
	}
	if n := cdr.count(); n != 0 {
		t.Errorf("the CDR got %d requests, want none from a run that skipped", n)
	}
}

func TestRun_CDRStagesNeedTheWriteOptIn_PROBE108_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
	}
	stdout, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run without %s = %v, want nil", envAllowMutating, err)
	}
	if n := cdr.count(); n != 0 {
		t.Errorf("the CDR got %d requests without %s, want none", n, envAllowMutating)
	}
	if !strings.Contains(stdout, envAllowMutating) {
		t.Errorf("run printed %q, want a line naming %s", stdout, envAllowMutating)
	}
	if _, err := os.Stat(filepath.Join(out, "post.jsonl")); !os.IsNotExist(err) {
		t.Errorf("post.jsonl exists (stat err %v), want no CDR stage output", err)
	}
	gens := readJSONL[genRecord](t, out, "gen.jsonl")
	if got, want := len(gens), len(policies)*len(fills); got != want {
		t.Errorf("gen.jsonl has %d lines, want %d: generation runs offline", got, want)
	}
}

func TestRun_PipelineAgainstTwoCDRs_PROBE108_REQ107_REQ102(t *testing.T) {
	t.Parallel()
	eb, fe := newFakeCDR(t), newFakeCDR(t)
	eb.example, fe.example = vendoredExample(t), vendoredExample(t)
	fe.persistent = true
	e := env{
		envTemplates:            templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE":  eb.url(),
		"OPENEHR_LIVE_FERROEHR": fe.url(),
		envAllowMutating:        "1",
	}
	_, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	s := readSummary(t, out)
	docs := len(policies) * len(fills)
	if s.Generation.Runs != docs || s.Generation.Errors != 0 {
		t.Fatalf("generation = %+v, want %d runs and no error", s.Generation, docs)
	}
	byName := map[string]cdrSummary{}
	for _, c := range s.PerCDR {
		byName[c.Name] = c
	}
	for name, levels := range map[string]int{"ehrbase": 1, "ferroehr": 3} {
		c := byName[name]
		if c.TemplatesPresent != (ratio{1, 1}) {
			t.Errorf("%s templates present = %v, want 1 / 1", name, c.TemplatesPresent)
		}
		if c.DocumentsAccepted != (ratio{docs, docs}) {
			t.Errorf("%s SDK documents accepted = %v, want %d / %d", name, c.DocumentsAccepted, docs, docs)
		}
		if c.ExamplesFetched != (ratio{levels, levels}) {
			t.Errorf("%s examples fetched = %v, want %d / %d", name, c.ExamplesFetched, levels, levels)
		}
		if c.ExamplesSDKValid.Of != levels {
			t.Errorf("%s examples validated = %d, want %d", name, c.ExamplesSDKValid.Of, levels)
		}
		if c.OwnExamplesAccepted != (ratio{levels, levels}) {
			t.Errorf("%s own examples accepted = %v, want %d / %d", name, c.OwnExamplesAccepted, levels, levels)
		}
	}
	ebs, fes := eb.state(), fe.state()
	if got := fes.exampleAsks; len(got) != 3 || !strings.Contains(strings.Join(got, " "), "detail_level=complete") {
		t.Errorf("FerroEHR example requests = %q, want the three detail levels", got)
	}
	if got := ebs.exampleAsks; len(got) != 1 || strings.Contains(got[0], "detail_level") {
		t.Errorf("EHRbase example requests = %q, want one with no detail_level", got)
	}
	for _, p := range append(ebs.posted, fes.posted...) {
		if bytes.Contains(p.body, []byte(exampleUID)) {
			t.Errorf("a CDR example was posted with its uid %s, want it removed", exampleUID)
		}
	}
	for _, name := range []string{"templates.jsonl", "upload.jsonl", "gen.jsonl", "post.jsonl", "examples.jsonl", "validate.jsonl", "cross.jsonl", "run.json", "summary.md"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("output %s: %v", name, err)
		}
	}
}

func TestRun_FreshEHRPerDocument_PROBE108_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	cdr.persistent = true // a second composition in one EHR is refused
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
	}
	_, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	posts := readJSONL[postRecord](t, out, "post.jsonl")
	if len(posts) != len(policies)*len(fills) {
		t.Fatalf("post.jsonl has %d lines, want %d", len(posts), len(policies)*len(fills))
	}
	seen := map[string]bool{}
	for _, p := range posts {
		if !p.OK || p.Status != http.StatusCreated {
			t.Errorf("%s posted %d %q, want accepted with 201: each document goes to an EHR of its own", p.Label, p.Status, p.Body)
		}
		if seen[p.EHRID] {
			t.Errorf("%s went to EHR %s, which another document used", p.Label, p.EHRID)
		}
		seen[p.EHRID] = true
	}
	if st := cdr.state(); st.ehrCreates != len(st.posted) {
		t.Errorf("the CDR saw %d EHR creates for %d compositions, want one each", st.ehrCreates, len(st.posted))
	}
}

func TestNewEHR_SendsNoBodyAndNoContentType_PROBE108(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t) // answers 415 to a body or a Content-Type on POST /ehr
	c, err := newCDRClient(cdrTarget{name: "fake", base: cdr.url()}, 1)
	if err != nil {
		t.Fatal(err)
	}
	id, o := c.newEHR(t.Context())
	if !o.OK || id == "" {
		t.Fatalf("newEHR() = %q, %+v, want an EHR id: the create sends no body and no Content-Type", id, o)
	}
}

func TestRun_StripsTheByteOrderMarkBeforeUpload_PROBE108(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t) // refuses an OPT that starts with a BOM
	e := env{
		envTemplates:           templateDir(t, utf8BOM, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
	}
	_, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	ups := readJSONL[uploadRecord](t, out, "upload.jsonl")
	if len(ups) != 1 || !ups[0].Present {
		t.Fatalf("upload.jsonl = %+v, want the BOM-prefixed OPT present", ups)
	}
	for _, u := range cdr.state().uploads {
		if bytes.HasPrefix(u, utf8BOM) {
			t.Errorf("an upload started with a byte-order mark, want it stripped")
		}
	}
}

func TestRun_StoredCopyComparedBySignature_PROBE108(t *testing.T) {
	t.Parallel()
	opt, err := os.ReadFile(vendoredOPT())
	if err != nil {
		t.Fatal(err)
	}
	// reserialised is the same template as a CDR might serve it back:
	// another namespace prefix, other indentation.
	reserialised := func(b []byte) []byte {
		b = bytes.ReplaceAll(b, []byte("xsi:"), []byte("x:"))
		b = bytes.ReplaceAll(b, []byte("xmlns:xsi="), []byte("xmlns:x="))
		return bytes.ReplaceAll(b, []byte("\n  "), []byte("\n\t"))
	}
	cases := []struct {
		name       string
		stored     []byte
		wantID     string
		wantUpload int
	}{
		{name: "same content, re-serialised", stored: opt, wantID: testTemplate, wantUpload: 1},
		{name: "other content", stored: bytes.Replace(opt, []byte("<concept>Minimal evaluation</concept>"), []byte("<concept>Another template</concept>"), 1), wantID: testTemplate + renameSuffix, wantUpload: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if bytes.Equal(tc.stored, opt) == (tc.wantID != testTemplate) {
				t.Fatal("the stored copy was not built as the case says")
			}
			cdr := newFakeCDR(t)
			cdr.templates[testTemplate] = tc.stored
			cdr.serveStored = reserialised
			e := env{
				envTemplates:           templateDir(t, nil, vendoredOPT()),
				"OPENEHR_LIVE_EHRBASE": cdr.url(),
				envAllowMutating:       "1",
			}
			_, _, out, err := runFor(t, e)
			if err != nil {
				t.Fatalf("run = %v", err)
			}
			st := cdr.state()
			if len(st.uploads) != tc.wantUpload {
				t.Errorf("the CDR saw %d uploads, want %d", len(st.uploads), tc.wantUpload)
			}
			ups := readJSONL[uploadRecord](t, out, "upload.jsonl")
			if len(ups) != 1 || ups[0].TemplateID != tc.wantID || !ups[0].Present {
				t.Fatalf("upload.jsonl = %+v, want %s present", ups, tc.wantID)
			}
			gens := readJSONL[genRecord](t, out, "gen.jsonl")
			for _, g := range gens {
				if g.TemplateID != tc.wantID {
					t.Errorf("generated %s under %s, want %s", g.label(), g.TemplateID, tc.wantID)
				}
			}
			for _, p := range st.posted {
				if !bytes.Contains(p.body, []byte(`"value":"`+tc.wantID+`"`)) {
					t.Errorf("a posted composition does not name template %s", tc.wantID)
				}
			}
		})
	}
}

func TestRun_NeverWritesTheCredential_PROBE108(t *testing.T) {
	t.Parallel()
	const user, pass = "operator", "s3cr3t-passphrase"
	cdr := newFakeCDR(t)
	cdr.user, cdr.pass = user, pass
	cdr.refuseCompositions = true // its refusal text quotes the Authorization header
	cdr.example = vendoredExample(t)
	e := env{
		envTemplates:                 templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE":       cdr.url(),
		"OPENEHR_LIVE_EHRBASE_BASIC": user + ":" + pass,
		envAllowMutating:             "1",
	}
	stdout, stderr, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	secrets := []string{user + ":" + pass, base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))}
	check := func(where, text string) {
		for _, s := range secrets {
			if strings.Contains(text, s) {
				t.Errorf("%s holds the credential form %q", where, s)
			}
		}
	}
	check("stdout", stdout)
	check("stderr", stderr)
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		check(p, string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The positive control: the refusal reached the output, redacted.
	posts := readJSONL[postRecord](t, out, "post.jsonl")
	if len(posts) == 0 || posts[0].OK || !strings.Contains(posts[0].Body, "Authorization: Basic "+redacted) {
		t.Errorf("post.jsonl first line = %+v, want the refusal text with the credential redacted", posts)
	}
}

func TestLoadConfig_RefusalsNeverQuoteTheValue_PROBE108(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cases := []struct {
		name   string
		env    env
		secret string
	}{
		{name: "credential not user:pass", secret: "nocolon-s3cr3t", env: env{
			envTemplates: dir, "OPENEHR_LIVE_FERROEHR": "http://127.0.0.1:1/openehr/v1", "OPENEHR_LIVE_FERROEHR_BASIC": "nocolon-s3cr3t",
		}},
		{name: "credential in the base URL", secret: "s3cr3t-pw", env: env{
			envTemplates: dir, "OPENEHR_LIVE_EHRBASE": "http://operator:s3cr3t-pw@127.0.0.1:1/openehr/v1",
		}},
		{name: "base URL not absolute", secret: "s3cr3t-host", env: env{
			envTemplates: dir, "OPENEHR_LIVE_EHRBASE": "s3cr3t-host/openehr/v1",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var usage bytes.Buffer
			_, _, err := loadConfig([]string{"-out", filepath.Join(t.TempDir(), "out")}, tc.env.get, &usage)
			if err == nil {
				t.Fatalf("loadConfig(%s) = nil error, want a refusal", tc.name)
			}
			if strings.Contains(err.Error(), tc.secret) || strings.Contains(usage.String(), tc.secret) {
				t.Errorf("loadConfig(%s) error %q quotes the value", tc.name, err)
			}
		})
	}
}

func TestLoadConfig_OutputStaysOutsideTheRepository_PROBE108(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd() // cmd/cdrcheck, inside the module
	if err != nil {
		t.Fatal(err)
	}
	e := env{envTemplates: t.TempDir()}
	for _, out := range []string{wd, filepath.Join(wd, "testdata", "out")} {
		if _, _, err := loadConfig([]string{"-out", out}, e.get, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "inside the repository") {
			t.Errorf("loadConfig(-out %s) = %v, want a refusal: the output stays outside the repository", out, err)
		}
	}
	outside := filepath.Join(t.TempDir(), "out")
	if _, _, err := loadConfig([]string{"-out", outside}, e.get, &bytes.Buffer{}); err != nil {
		t.Errorf("loadConfig(-out %s) = %v, want nil", outside, err)
	}
}

func TestRun_RefusesAnOutputDirectoryInUse_PROBE108(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "summary.md"), []byte("an earlier run"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := env{envTemplates: templateDir(t, nil, vendoredOPT()), envOut: out}
	if _, _, _, err := runFor(t, e); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("run into a used output directory = %v, want a refusal", err)
	}
}

func TestRun_OnlyChecksTheMatchedTemplates_PROBE108(t *testing.T) {
	t.Parallel()
	e := env{envTemplates: templateDir(t, nil, vendoredOPT(), fixtures.TemplateOptForName("minimal_observation.en.v1"))}
	_, _, out, err := runFor(t, e, "-only", `^minimal_evaluation\.`)
	if err != nil {
		t.Fatalf("run -only = %v", err)
	}
	for _, g := range readJSONL[genRecord](t, out, "gen.jsonl") {
		if g.TemplateID != testTemplate {
			t.Errorf("generated %s, want only %s", g.TemplateID, testTemplate)
		}
	}
	s := readSummary(t, out)
	if s.Templates.Files != 2 || s.Templates.Kept != 1 || s.Templates.Skipped != 1 {
		t.Errorf("templates = %+v, want 2 files, 1 checked, 1 left out", s.Templates)
	}
}
