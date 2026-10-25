// Tests of the CDR cross-check tool (REQ-082), which measures the example
// generator (REQ-107) and the validator (REQ-102) against live CDRs. Every
// CDR here is a fakeCDR on httptest; no test reaches a live deployment.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"uuid"

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

func TestInterruptContext_RestoresTheDefaultOnceTheRunStops_REQ082(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var interrupt context.CancelFunc
		restored := make(chan struct{})
		// notify stands in for signal.NotifyContext, so no signal reaches
		// the test process: interrupt is the first Ctrl-C, and the stop it
		// returns closes restored where the real one restores the default
		// handling of os.Interrupt.
		notify := func(parent context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc) {
			if !slices.Equal(sigs, []os.Signal{os.Interrupt}) {
				t.Errorf("interruptContext asked for the signals %v, want only os.Interrupt", sigs)
			}
			ctx, cancel := context.WithCancel(parent)
			interrupt = cancel
			var once sync.Once
			return ctx, func() { once.Do(func() { close(restored) }); cancel() }
		}
		ctx, stop := interruptContext(t.Context(), notify)
		defer stop()
		synctest.Wait()
		select {
		case <-restored:
			t.Fatal("the default handling of Ctrl-C was restored before any Ctrl-C")
		default:
		}
		interrupt()
		<-ctx.Done()
		synctest.Wait()
		select {
		case <-restored:
		default:
			t.Error("after the first Ctrl-C the default handling is not restored, so a second Ctrl-C is swallowed while the run winds down")
		}
	})
}

func TestRun_SkipsWhenUnset_REQ082_REQ107(t *testing.T) {
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

func TestRun_CDRStagesNeedTheWriteOptIn_REQ082_REQ107(t *testing.T) {
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

func TestRun_PipelineAgainstTwoCDRs_REQ082_REQ107_REQ102(t *testing.T) {
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

func TestRun_FreshEHRPerDocument_REQ082_REQ107(t *testing.T) {
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

func TestNewEHR_SendsNoBodyAndNoContentType_REQ082(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t) // answers 415 to a body or a Content-Type on an EHR create
	c, err := newCDRClient(cdrTarget{name: "fake", base: cdr.url()}, 1, redactor{})
	if err != nil {
		t.Fatal(err)
	}
	id := newRunIDs().next()
	if o := c.newEHR(t.Context(), id); !o.OK {
		t.Fatalf("newEHR(%s) = %+v, want the EHR created: the create sends no body and no Content-Type", id, o)
	}
	st := cdr.state()
	if want := []string{"PUT " + fakeBase + "/ehr/" + string(id)}; !slices.Equal(st.ehrCalls, want) || !slices.Equal(st.ehrIDs, []string{string(id)}) {
		t.Errorf("the CDR saw EHR creates %q making %q, want %q making %s: the run names the EHR itself", st.ehrCalls, st.ehrIDs, want, id)
	}
}

func TestRun_StripsTheByteOrderMarkBeforeUpload_REQ082(t *testing.T) {
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

func TestRun_StoredCopyComparedBySignature_REQ082(t *testing.T) {
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
			var current, superseded []uploadRecord
			for _, u := range readJSONL[uploadRecord](t, out, "upload.jsonl") {
				if u.SupersededBy != "" {
					superseded = append(superseded, u)
				} else {
					current = append(current, u)
				}
			}
			if len(current) != 1 || current[0].TemplateID != tc.wantID || !current[0].Present {
				t.Fatalf("upload.jsonl current records = %+v, want %s present", current, tc.wantID)
			}
			if len(superseded) != tc.wantUpload-1 {
				t.Errorf("upload.jsonl superseded records = %+v, want %d", superseded, tc.wantUpload-1)
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

func TestRun_UnreadableStoredCopyIsPresentOnlyAfterA409_REQ082(t *testing.T) {
	t.Parallel()
	opt, err := os.ReadFile(vendoredOPT())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		// dup is the status of the refusal that says the CDR holds the id.
		dup         int
		wantPresent bool
	}{
		// A 409 says the CDR holds the template: present, with no
		// signature compared.
		{name: "a 409", dup: http.StatusConflict, wantPresent: true},
		// Another refusal that says so is taken at its word only where the
		// copy can be read: unread, the template is absent.
		{name: "a 400 whose text says already", dup: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cdr := newFakeCDR(t)
			cdr.templates[testTemplate] = opt
			cdr.dupStatus = tc.dup
			cdr.storedStatus = http.StatusInternalServerError // the copy cannot be fetched
			cdr.example = vendoredExample(t)
			e := env{
				envTemplates:           templateDir(t, nil, vendoredOPT()),
				"OPENEHR_LIVE_EHRBASE": cdr.url(),
				envAllowMutating:       "1",
			}
			_, _, out, err := runFor(t, e)
			if err != nil {
				t.Fatalf("run = %v", err)
			}
			ups := readJSONL[uploadRecord](t, out, "upload.jsonl")
			if len(ups) != 1 {
				t.Fatalf("upload.jsonl = %+v, want one record: no copy was read, so nothing is renamed", ups)
			}
			// The failed fetch is the record's only trace of the copy.
			if u := ups[0]; u.Status != tc.dup || u.Present != tc.wantPresent || u.StoredSame != nil || u.StoredStatus != http.StatusInternalServerError {
				t.Errorf("upload of %s = status %d, present %v, stored same %v, stored status %d; want %d, present %v, no comparison, stored status 500",
					u.TemplateID, u.Status, u.Present, u.StoredSame, u.StoredStatus, tc.dup, tc.wantPresent)
			}
			st := cdr.state()
			wantPosts, wantAsks := 0, 0
			if tc.wantPresent {
				wantPosts, wantAsks = len(policies)*len(fills)+1, 1 // its documents and its own example
			}
			if len(st.posted) != wantPosts || len(st.exampleAsks) != wantAsks {
				t.Errorf("the CDR got %d compositions and %d example requests, want %d and %d", len(st.posted), len(st.exampleAsks), wantPosts, wantAsks)
			}
		})
	}
}

func TestRun_NeverWritesTheCredential_REQ082(t *testing.T) {
	t.Parallel()
	const user, pass = "operator", "s3cr3t-passphrase"
	cdr := newFakeCDR(t)
	cdr.user, cdr.pass = user, pass
	cdr.refusal = func(auth string) string { // the refusal text quotes the Authorization header
		return fmt.Sprintf(`{"message":"rejected request with header Authorization: %s"}`, auth)
	}
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

func TestLoadConfig_RefusalsNeverQuoteTheValue_REQ082(t *testing.T) {
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

func TestLoadConfig_OutputStaysOutsideTheRepository_REQ082(t *testing.T) {
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

func TestLoadConfig_AnUnreadableGoModRefusesTheOutput_REQ082(t *testing.T) {
	t.Parallel()
	module := []byte("module " + modulePath + "\n")
	cases := []struct {
		name string
		// goMod writes what a checkout at dir holds as its go.mod.
		goMod   func(path string) error
		refused bool
		// noAccess says the case needs a file the test process cannot read.
		noAccess bool
	}{
		{name: "a go.mod of another module", goMod: func(path string) error {
			return os.WriteFile(path, []byte("module example.com/other\n"), 0o644)
		}},
		{name: "a go.mod of this module", refused: true, goMod: func(path string) error {
			return os.WriteFile(path, module, 0o644)
		}},
		{
			// The line is past the scanner's buffer, so the module line
			// after it is never read.
			name: "a go.mod of this module after a line too long to read", refused: true,
			goMod: func(path string) error {
				return os.WriteFile(path, append([]byte("// "+strings.Repeat("x", 1<<17)+"\n"), module...), 0o644)
			},
		},
		{name: "a go.mod that is a directory", refused: true, goMod: func(path string) error { return os.Mkdir(path, 0o755) }},
		{name: "a go.mod that cannot be opened", refused: true, noAccess: true, goMod: func(path string) error {
			if err := os.WriteFile(path, module, 0o644); err != nil {
				return err
			}
			return os.Chmod(path, 0)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.noAccess && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("a file's mode keeps neither Windows nor root from reading it")
			}
			checkout := t.TempDir()
			if err := tc.goMod(filepath.Join(checkout, "go.mod")); err != nil {
				t.Fatal(err)
			}
			e := env{envTemplates: t.TempDir()}
			_, _, err := loadConfig([]string{"-out", filepath.Join(checkout, "out")}, e.get, &bytes.Buffer{})
			if refused := err != nil; refused != tc.refused {
				t.Errorf("loadConfig(-out under %s) = %v, want refused %v: a go.mod that cannot be read may be this module's", tc.name, err, tc.refused)
			}
		})
	}
}

func TestRun_RefusesAnOutputDirectoryInUse_REQ082(t *testing.T) {
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

func TestRun_OnlyChecksTheMatchedTemplates_REQ082(t *testing.T) {
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
	// The summary says the run was filtered, and by what.
	if s.Only != `^minimal_evaluation\.` {
		t.Errorf("summary.json only = %q, want the -only expression", s.Only)
	}
	md, err := os.ReadFile(filepath.Join(out, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "**Only:** template ids matching `^minimal_evaluation\\.`") {
		t.Errorf("summary.md does not say the run was filtered:\n%s", md)
	}
}

// readRunInfo decodes a run's run.json.
func readRunInfo(t *testing.T, out string) runInfo {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var info runInfo
	if err := json.Unmarshal(b, &info); err != nil {
		t.Fatal(err)
	}
	return info
}

// ehrIDsIn returns the EHR ids ehrs.jsonl records.
func ehrIDsIn(t *testing.T, out string) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, r := range readJSONL[ehrRecord](t, out, "ehrs.jsonl") {
		ids[r.EHRID] = true
	}
	return ids
}

func TestRun_EHRsCarryThePerRunIdentifier_REQ082_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	cdr.example = vendoredExample(t)
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
	}
	_, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	runID := readRunInfo(t, out).RunID
	if len(runID) != 13 || runID[8] != '-' {
		t.Fatalf("run.json run_id = %q, want 12 hex digits as xxxxxxxx-xxxx", runID)
	}
	st := cdr.state()
	if want := len(policies)*len(fills) + 1; len(st.ehrIDs) != want {
		t.Fatalf("the CDR created %d EHRs, want %d: one per SDK document and one for its own example", len(st.ehrIDs), want)
	}
	for _, call := range st.ehrCalls {
		if !strings.HasPrefix(call, http.MethodPut+" "+fakeBase+"/ehr/") {
			t.Errorf("EHR create %q, want PUT /ehr/{ehr_id}: the run names every EHR", call)
		}
	}
	recorded := ehrIDsIn(t, out)
	seen := map[string]bool{}
	for _, id := range st.ehrIDs {
		if !strings.HasPrefix(id, runID+"-8000-8000-") {
			t.Errorf("EHR id %s does not begin with the run identifier %s", id, runID)
		}
		u, err := uuid.Parse(id)
		if err != nil || u[6]>>4 != 8 || u[8]>>6 != 0b10 {
			t.Errorf("EHR id %s is not an RFC 9562 version 8 UUID (parse error %v)", id, err)
		}
		if seen[id] {
			t.Errorf("EHR id %s was created twice", id)
		}
		seen[id] = true
		if !recorded[id] {
			t.Errorf("EHR %s is not in ehrs.jsonl", id)
		}
	}
	for _, p := range append(readJSONL[postRecord](t, out, "post.jsonl"), readJSONL[postRecord](t, out, "cross.jsonl")...) {
		if !seen[p.EHRID] {
			t.Errorf("%s %s went to EHR %q, which the run did not create", p.CDR, p.Label, p.EHRID)
		}
	}
}

func TestRun_RefusedEHRCreateIsReported_REQ082_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	cdr.refuseEHRCreate = true
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
	}
	_, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	docs := len(policies) * len(fills)
	posts := readJSONL[postRecord](t, out, "post.jsonl")
	if len(posts) != docs {
		t.Fatalf("post.jsonl has %d lines, want %d: a refused create is recorded for its document", len(posts), docs)
	}
	for _, p := range posts {
		if p.Stage != "ehr" || p.Status != http.StatusMethodNotAllowed || p.Body == "" {
			t.Errorf("%s = stage %q status %d body %q, want the EHR create's 405 and its text", p.Label, p.Stage, p.Status, p.Body)
		}
	}
	st := cdr.state()
	for _, call := range st.ehrCalls {
		if strings.HasPrefix(call, http.MethodPost) {
			t.Errorf("after a refused PUT the run sent %q, want no fallback to an id the CDR picks", call)
		}
	}
	if len(st.posted) != 0 {
		t.Errorf("the CDR got %d compositions, want none: no EHR was created", len(st.posted))
	}
	s := readSummary(t, out)
	if len(s.PerCDR) != 1 {
		t.Fatalf("per CDR = %d entries, want 1", len(s.PerCDR))
	}
	c := s.PerCDR[0]
	if c.EHRFailures != docs || c.EHRsCreated != (ratio{0, docs}) {
		t.Errorf("EHR failures %d, EHRs created %v, want %d and 0 / %d", c.EHRFailures, c.EHRsCreated, docs, docs)
	}
	if len(c.EHRRefusals) != 1 || c.EHRRefusals[0].Status != http.StatusMethodNotAllowed || c.EHRRefusals[0].Documents != docs {
		t.Errorf("EHR refusals = %+v, want one 405 group on %d documents", c.EHRRefusals, docs)
	}
}

func TestRun_InterruptedRunKeepsEveryEHRID_REQ082_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cdr.afterEHRCreate = func(n int) {
		if n == 3 {
			cancel() // as Ctrl-C does, through signal.NotifyContext
		}
	}
	out := filepath.Join(t.TempDir(), "out")
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT(), fixtures.TemplateOptForName("minimal_observation.en.v1")),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
		envOut:                 out,
	}
	if err := run(ctx, []string{"-workers", "2"}, e.get, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run cancelled after the third EHR create = nil error, want the run to report that it stopped")
	}
	cdr.srv.Close() // waits for the requests in flight, so the fake's list is final
	st := cdr.state()
	if len(st.ehrIDs) < 3 || len(st.ehrIDs) >= 2*len(policies)*len(fills) {
		t.Fatalf("the CDR created %d EHRs, want at least 3 and fewer than a whole run", len(st.ehrIDs))
	}
	recorded := ehrIDsIn(t, out)
	for _, id := range st.ehrIDs {
		if !recorded[id] {
			t.Errorf("EHR %s, created before the run stopped, is not in ehrs.jsonl", id)
		}
	}
	gens := readJSONL[genRecord](t, out, "gen.jsonl")
	if len(gens) < 3 {
		t.Errorf("gen.jsonl has %d lines, want the generations made before the stop", len(gens))
	}
	for _, g := range gens {
		if g.Error != "" {
			t.Errorf("gen.jsonl holds %s %s failed with %q, want no generation the stop cut short", g.TemplateID, g.label(), g.Error)
		}
	}
	if s := readSummary(t, out); s.Partial == "" {
		t.Error("summary.json partial is empty, want the summary of a stopped run marked partial")
	}
	md, err := os.ReadFile(filepath.Join(out, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "**Partial:**") {
		t.Errorf("summary.md does not say the run stopped early:\n%s", md)
	}
	if info := readRunInfo(t, out); info.Stopped == "" || info.RunID == "" {
		t.Errorf("run.json = %+v, want the run identifier and why it stopped", info)
	}
}

func TestRun_RedactsBeforeTheCut_REQ082(t *testing.T) {
	t.Parallel()
	const user, pass = "operator", "s3cr3t-passphrase"
	b64 := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	const head, lead = `{"message":"`, "Authorization: Basic "
	// The echoed credential starts 10 bytes before the cut at maxBodyText.
	pad := strings.Repeat("x", maxBodyText-10-len(head)-len(lead))
	cdr := newFakeCDR(t)
	cdr.user, cdr.pass = user, pass
	cdr.refusal = func(auth string) string { return head + pad + "Authorization: " + auth + `"}` }
	e := env{
		envTemplates:                 templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE":       cdr.url(),
		"OPENEHR_LIVE_EHRBASE_BASIC": user + ":" + pass,
		envAllowMutating:             "1",
	}
	stdout, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	prefix := b64[:10] // what a cut before redaction would leave
	if strings.Contains(stdout, prefix) {
		t.Errorf("stdout holds %q, the start of the credential", prefix)
	}
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(prefix)) {
			t.Errorf("%s holds %q, the start of the credential cut at %d bytes", p, prefix, maxBodyText)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The positive control: the record holds the text cut after redaction.
	posts := readJSONL[postRecord](t, out, "post.jsonl")
	if len(posts) == 0 {
		t.Fatal("post.jsonl is empty, want the refusals")
	}
	if body := posts[0].Body; !strings.HasSuffix(body, lead+redacted+"…") {
		t.Errorf("post.jsonl first body ends %q (%d bytes), want it cut after %q", body[max(0, len(body)-40):], len(body), lead+redacted)
	}
}

func TestRun_GroupsARefusalOverTheCutWhole_REQ082_REQ107(t *testing.T) {
	t.Parallel()
	const last = "the last finding, past the cut"
	var findings []string
	for i := range 200 {
		findings = append(findings, fmt.Sprintf("finding %03d at /content[openEHR-EHR-OBSERVATION.x.v1]/data[at0001]/events[at0002]", i))
	}
	body, err := json.Marshal(map[string]any{"message": "invalid", "validationErrors": append(findings, last)})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= maxBodyText {
		t.Fatalf("the refusal is %d bytes, want it over %d", len(body), maxBodyText)
	}
	cdr := newFakeCDR(t)
	cdr.refusal = func(string) string { return string(body) }
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
	}
	_, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	docs := len(policies) * len(fills)
	s := readSummary(t, out)
	if len(s.PerCDR) != 1 {
		t.Fatalf("per CDR = %d entries, want 1", len(s.PerCDR))
	}
	var found bool
	for _, g := range s.PerCDR[0].DocumentRefusals {
		if g.Detail == last && g.Documents == docs {
			found = true
		}
	}
	if !found {
		t.Errorf("document refusals = %d groups, want one for %q on %d documents: the summary reads the whole body", len(s.PerCDR[0].DocumentRefusals), last, docs)
	}
	for _, p := range readJSONL[postRecord](t, out, "post.jsonl") {
		if len(p.Body) > maxBodyText+len("…") || !strings.HasSuffix(p.Body, "…") {
			t.Errorf("%s body is %d bytes, want it cut to %d in the record", p.Label, len(p.Body), maxBodyText)
		}
	}
}

func TestRun_SupersededUploadsKeptAndUncounted_REQ082(t *testing.T) {
	t.Parallel()
	opt, err := os.ReadFile(vendoredOPT())
	if err != nil {
		t.Fatal(err)
	}
	other := bytes.Replace(opt, []byte("<concept>Minimal evaluation</concept>"), []byte("<concept>Another template</concept>"), 1)
	if bytes.Equal(other, opt) {
		t.Fatal("the other content was not built")
	}
	eb, fe := newFakeCDR(t), newFakeCDR(t)
	fe.templates[testTemplate] = other // FerroEHR holds other content; EHRbase takes the original
	e := env{
		envTemplates:            templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE":  eb.url(),
		"OPENEHR_LIVE_FERROEHR": fe.url(),
		envAllowMutating:        "1",
	}
	stdout, _, out, err := runFor(t, e)
	if err != nil {
		t.Fatalf("run = %v", err)
	}
	newID := testTemplate + renameSuffix
	got := map[string]uploadRecord{}
	for _, u := range readJSONL[uploadRecord](t, out, "upload.jsonl") {
		got[u.CDR+" "+u.TemplateID] = u
	}
	if u := got["ehrbase "+testTemplate]; u.SupersededBy != newID || !u.Present {
		t.Errorf("EHRbase's upload of %s = %+v, want it kept, present and superseded by %s", testTemplate, u, newID)
	}
	if u := got["ferroehr "+testTemplate]; u.SupersededBy != newID || u.StoredSame == nil || *u.StoredSame {
		t.Errorf("FerroEHR's upload of %s = %+v, want it kept, other content, superseded by %s", testTemplate, u, newID)
	}
	for _, cdr := range []string{"ehrbase", "ferroehr"} {
		if u := got[cdr+" "+newID]; u.SupersededBy != "" || !u.Present {
			t.Errorf("%s's upload of %s = %+v, want it present", cdr, newID, u)
		}
	}
	s := readSummary(t, out)
	for _, c := range s.PerCDR {
		if c.TemplatesPresent != (ratio{1, 1}) {
			t.Errorf("%s templates present = %v, want 1 / 1: a superseded upload is not counted", c.Name, c.TemplatesPresent)
		}
		// The progress line counts as the summary does.
		if want := "upload " + c.Name + ": 1 of 1 templates present\n"; !strings.Contains(stdout, want) {
			t.Errorf("run printed\n%s\nwant the line %q: a superseded upload is not counted", stdout, want)
		}
		want := []string(nil)
		if c.Name == "ferroehr" {
			want = []string{testTemplate}
		}
		if !slices.Equal(c.StoredDiffers, want) {
			t.Errorf("%s stored differs = %q, want %q", c.Name, c.StoredDiffers, want)
		}
	}
}

func TestRun_OtherContentUnderTheNewIDLeavesThatCDROut_REQ082(t *testing.T) {
	t.Parallel()
	opt, err := os.ReadFile(vendoredOPT())
	if err != nil {
		t.Fatal(err)
	}
	other := bytes.Replace(opt, []byte("<concept>Minimal evaluation</concept>"), []byte("<concept>Another template</concept>"), 1)
	newID := testTemplate + renameSuffix
	otherNew, err := renamedOPT(other, newID)
	if err != nil {
		t.Fatal(err)
	}
	eb, fe := newFakeCDR(t), newFakeCDR(t)
	// EHRbase holds other content under the id and under the new id too;
	// FerroEHR holds neither.
	eb.templates[testTemplate], eb.templates[newID] = other, otherNew
	eb.example, fe.example = vendoredExample(t), vendoredExample(t)
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
	got := map[string]uploadRecord{}
	for _, u := range readJSONL[uploadRecord](t, out, "upload.jsonl") {
		if strings.HasSuffix(u.TemplateID, renameSuffix+renameSuffix) {
			t.Errorf("%s uploaded %s, want a template renamed once at most", u.CDR, u.TemplateID)
		}
		got[u.CDR+" "+u.TemplateID] = u
	}
	if u := got["ehrbase "+newID]; u.Present || u.StoredSame == nil || *u.StoredSame || u.SupersededBy != "" {
		t.Errorf("EHRbase's upload of %s = %+v, want it absent, other content, not superseded", newID, u)
	}
	if u := got["ferroehr "+newID]; !u.Present {
		t.Errorf("FerroEHR's upload of %s = %+v, want it present", newID, u)
	}
	for name, n := range map[string]int{"ehrbase": len(eb.state().uploads), "ferroehr": len(fe.state().uploads)} {
		if n != 2 {
			t.Errorf("%s saw %d uploads, want 2: the id and the new id, no third", name, n)
		}
	}
	for _, g := range readJSONL[genRecord](t, out, "gen.jsonl") {
		if g.TemplateID != newID {
			t.Errorf("generated %s under %s, want %s: the SDK goes on under the new id", g.label(), g.TemplateID, newID)
		}
	}
	ebs, fes := eb.state(), fe.state()
	if len(ebs.posted) != 0 || len(ebs.exampleAsks) != 0 {
		t.Errorf("EHRbase got %d compositions and %d example requests, want none: it holds other content under %s", len(ebs.posted), len(ebs.exampleAsks), newID)
	}
	// FerroEHR gets every SDK document and its own examples, at three levels.
	if want := len(policies)*len(fills) + 3; len(fes.posted) != want || len(fes.exampleAsks) != 3 {
		t.Errorf("FerroEHR got %d compositions and %d example requests, want %d and 3", len(fes.posted), len(fes.exampleAsks), want)
	}
	sdkDocs := 0
	for _, p := range fes.posted {
		if bytes.Contains(p.body, []byte(`"value":"`+newID+`"`)) {
			sdkDocs++
		}
	}
	if want := len(policies) * len(fills); sdkDocs != want {
		t.Errorf("FerroEHR got %d compositions of template %s, want %d", sdkDocs, newID, want)
	}
	for _, c := range readSummary(t, out).PerCDR {
		want := ratio{1, 1}
		if c.Name == "ehrbase" {
			want = ratio{0, 1}
		}
		if c.TemplatesPresent != want {
			t.Errorf("%s templates present = %v, want %v", c.Name, c.TemplatesPresent, want)
		}
	}
}

func TestLoadConfig_OutputThroughASymlinkStaysOutside_REQ082(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd() // cmd/cdrcheck, inside the module
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(wd, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	e := env{envTemplates: t.TempDir()}
	for _, out := range []string{link, filepath.Join(link, "out"), filepath.Join(link, "a", "b")} {
		if _, _, err := loadConfig([]string{"-out", out}, e.get, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "inside the repository") {
			t.Errorf("loadConfig(-out %s), a link into %s, = %v, want a refusal", out, wd, err)
		}
	}
}

func TestNewEHR_ReadsTheCDRsAnswer_REQ082(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(f *fakeCDR, id string)
		wantOK    bool
		wantError string
		status    int
	}{
		{name: "the EHR's representation", setup: func(*fakeCDR, string) {}, wantOK: true},
		{name: "a 201 with no body: the EHR exists", setup: func(f *fakeCDR, _ string) {
			f.ehrAnswer = func(string) string { return "" }
		}, wantOK: true},
		{name: "another ehr_id in the answer", wantError: "ehr_id 99999999-", setup: func(f *fakeCDR, _ string) {
			f.ehrAnswer = func(string) string {
				return `{"_type":"EHR","ehr_id":{"_type":"HIER_OBJECT_ID","value":"99999999-0000-4000-8000-000000000001"},"system_id":{"_type":"HIER_OBJECT_ID","value":"fake"},"time_created":{"_type":"DV_DATE_TIME","value":"2026-01-02T03:04:05Z"}}`
			}
		}},
		{name: "an id the CDR holds already", status: http.StatusConflict, setup: func(f *fakeCDR, id string) { f.ehrs[id] = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cdr := newFakeCDR(t)
			id := newRunIDs().next()
			tc.setup(cdr, string(id))
			c, err := newCDRClient(cdrTarget{name: "fake", base: cdr.url()}, 1, redactor{})
			if err != nil {
				t.Fatal(err)
			}
			o := c.newEHR(t.Context(), id)
			if o.OK != tc.wantOK || !strings.Contains(o.Error, tc.wantError) || (tc.status != 0 && o.Status != tc.status) {
				t.Errorf("newEHR(%s) = %+v, want ok %v, error naming %q, status %d", tc.name, o, tc.wantOK, tc.wantError, tc.status)
			}
		})
	}
}

func TestRun_WritesEachRecordAsItCompletes_REQ082_REQ107(t *testing.T) {
	t.Parallel()
	cdr := newFakeCDR(t)
	// Each post record then holds the refusal text, which has spaces in it.
	cdr.refusal = func(string) string { return `{"message":"refused, as a CDR words it"}` }
	out := filepath.Join(t.TempDir(), "out")
	lines := func(name string) []string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Errorf("read %s mid-run: %v", name, err)
		}
		return slices.Collect(strings.Lines(string(b)))
	}
	var mu sync.Mutex // the hook runs on the fake's goroutine
	var onDisk struct{ ehrs, posts, gens []string }
	cdr.afterEHRCreate = func(n int) {
		if n == 3 { // with one worker, the first two documents are done
			mu.Lock()
			defer mu.Unlock()
			onDisk.ehrs, onDisk.posts, onDisk.gens = lines("ehrs.jsonl"), lines("post.jsonl"), lines("gen.jsonl")
		}
	}
	e := env{
		envTemplates:           templateDir(t, nil, vendoredOPT()),
		"OPENEHR_LIVE_EHRBASE": cdr.url(),
		envAllowMutating:       "1",
		envOut:                 out,
	}
	if _, _, _, err := runFor(t, e, "-workers", "1"); err != nil {
		t.Fatalf("run = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(onDisk.ehrs) != 2 || len(onDisk.posts) != 2 || len(onDisk.gens) != 3 {
		t.Errorf("at the third EHR create the files held %d EHRs, %d posts and %d generations, want 2, 2 and 3: each record is written as it completes",
			len(onDisk.ehrs), len(onDisk.posts), len(onDisk.gens))
	}
	ids := cdr.state().ehrIDs
	for i, line := range onDisk.ehrs {
		if !strings.Contains(line, ids[i]) {
			t.Errorf("ehrs.jsonl line %d = %s, want EHR %s", i+1, line, ids[i])
		}
	}
	for i, line := range onDisk.posts {
		if !strings.Contains(line, "as a CDR words it") {
			t.Errorf("post.jsonl line %d = %s, want the refusal text, spaces and all", i+1, line)
		}
	}
}
