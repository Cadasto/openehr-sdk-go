package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// validProvenance is provenance a capture passes on: complete, and with a base
// URL carrying no credential. Tests that exercise a refusal spoil one field of
// it, so what is under test is the field they changed.
func validProvenance() probe.HARProvenance {
	return probe.HARProvenance{
		Deployment: "sandbox (test only — not a witness)",
		BaseURL:    "https://sandbox.local/openehr/v1",
		CapturedAt: "2026-09-08T00:00:00Z",
		SDKCommit:  "0000000000000000000000000000000000000000",
	}
}

// TestCaptureScenario_ProducesAValidHARFromTheSandbox exercises the whole
// capture path offline: it drives the ehr-lifecycle scenario through the
// recorder against the in-memory sandbox (no CDR) and asserts the published
// recording is the three-exchange document the scenario drives. The file it
// writes is not a checked-in recording — a sandbox capture witnesses nothing — but
// it proves the harness produces a replayable, redaction-attested HAR.
//
// This is the positive control only. It cannot fail when captureScenario stops
// validating, because a sandbox capture is valid either way; the refusal tests
// below are what hold that gate.
func TestCaptureScenario_ProducesAValidHARFromTheSandbox(t *testing.T) {
	t.Parallel()
	path, err := captureScenario(
		t.Context(),
		scenarios["ehr-lifecycle"],
		"https://sandbox.local/openehr/v1",
		sandbox.New(),
		nil,
		validProvenance(),
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("captureScenario against sandbox = %v", err)
	}
	if filepath.Base(path) != "ehr-lifecycle.har" {
		t.Fatalf("recording path = %q, want basename ehr-lifecycle.har", path)
	}
	har, err := probe.ValidateHAR(path)
	if err != nil {
		t.Fatalf("published recording did not validate: %v", err)
	}
	if len(har.Log.Entries) != 3 {
		t.Fatalf("ehr-lifecycle recorded %d exchanges, want 3 (POST, GET, HEAD)", len(har.Log.Entries))
	}
}

// TestCaptureScenario_RefusedCaptureLeavesTheRecordingUntouched is the can-fail
// control for the validate-then-publish order. Incomplete provenance cannot
// pass HAR.Validate, so the capture must fail and the recording already in the
// output directory must survive byte-for-byte — a re-capture that goes wrong
// must not cost the recordings the witness they had. It also asserts no temp file is
// left behind.
//
// Publishing before validating (the order this replaces) fails here: the
// sentinel is overwritten.
func TestCaptureScenario_RefusedCaptureLeavesTheRecordingUntouched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := filepath.Join(dir, "ehr-lifecycle.har")
	sentinel := []byte(`{"log":{"version":"1.2","note":"the recording that was already here"}}`)
	if err := os.WriteFile(existing, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}

	prov := validProvenance()
	prov.Deployment = "" // HAR.Validate: provenance is incomplete

	_, err := captureScenario(t.Context(), scenarios["ehr-lifecycle"], "https://sandbox.local/openehr/v1", sandbox.New(), nil, prov, dir)
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("captureScenario with incomplete provenance = %v, want %v", err, probe.ErrUnsatisfiableMode)
	}
	got, readErr := os.ReadFile(existing)
	if readErr != nil {
		t.Fatalf("the existing recording is gone: %v", readErr)
	}
	if !bytes.Equal(got, sentinel) {
		t.Fatalf("a refused capture replaced the existing recording:\n got %s\nwant %s", got, sentinel)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("output directory holds %v, want only the untouched recording", names)
	}
}

// TestCaptureScenario_RefusesACredentialInProvenanceWithoutWriting is the
// can-fail control for REQ-082's "credentials MUST NOT reach disk". The
// recorder redacts entry URLs, so provenance is the one place a credential
// survives capture; a base URL carrying userinfo must be caught while the
// document is still in memory, leaving the output directory empty rather than
// holding a file that is deleted after the fact.
//
// The refusal must also name the channel and not the credential (REQ-093).
func TestCaptureScenario_RefusesACredentialInProvenanceWithoutWriting(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t-passphrase"
	dir := t.TempDir()

	prov := validProvenance()
	prov.BaseURL = "https://operator:" + secret + "@cdr.example/openehr/v1"

	_, err := captureScenario(t.Context(), scenarios["ehr-lifecycle"], prov.BaseURL, sandbox.New(), nil, prov, dir)
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("captureScenario with a credential in provenance = %v, want %v", err, probe.ErrUnsatisfiableMode)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("refusal echoed the credential: %v", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("a credential-bearing capture wrote %v; nothing may reach disk", names)
	}
}

// TestReplayCheck_RefusesATruncatedRecording is the unit control for
// replayCheck itself. A capture missing its last exchange is structurally
// valid — every entry it does carry is well-formed and redacted — so
// HAR.Validate passes it. Only replaying the scenario shows that the recording
// cannot answer the requests a probe makes.
//
// This calls replayCheck directly, so it says nothing about captureScenario
// calling it; that wiring is held by
// TestCaptureScenario_RefusesACaptureThatCannotReplay.
func TestReplayCheck_RefusesATruncatedRecording(t *testing.T) {
	t.Parallel()
	sc := scenarios["ehr-lifecycle"]
	path, err := captureScenario(t.Context(), sc, "https://sandbox.local/openehr/v1", sandbox.New(), nil, validProvenance(), t.TempDir())
	if err != nil {
		t.Fatalf("captureScenario against sandbox = %v", err)
	}
	har, err := probe.ValidateHAR(path)
	if err != nil {
		t.Fatal(err)
	}

	full := har.Log.Entries
	if len(full) != 3 {
		t.Fatalf("fixture has %d entries, want the 3-exchange lifecycle capture", len(full))
	}
	// The positive control: the whole capture replays. Without it, a
	// replayCheck that refused everything would pass the arm below.
	if err := replayCheck(t.Context(), sc, har); err != nil {
		t.Fatalf("the complete capture did not replay: %v", err)
	}

	har.Log.Entries = full[:len(full)-1] // drop the HEAD
	if err := har.Validate(); err != nil {
		t.Fatalf("the truncated capture must still validate, else this proves nothing: %v", err)
	}
	if err := replayCheck(t.Context(), sc, har); !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("replayCheck on a truncated capture = %v, want %v", err, probe.ErrUnmatchedRecording)
	}
}

// TestCaptureScenario_RefusesACaptureThatCannotReplay is the can-fail control
// for replayCheck being wired into the capture gate — removing that call from
// captureScenario fails here, which the direct replayCheck test above cannot
// detect.
//
// The fixture is the shape the review named: a deployment whose REST base the
// replayer does not normalise. The sandbox reduces a path through its first
// "/openehr/v1" segment wherever it sits, so it serves "/api/openehr/v1/ehr"
// and the capture is valid; probe.Replayer instead strips a closed list of
// known bases, which does not include "/api/openehr/v1", so the recorded paths
// only ever match themselves. Validation cannot see that — replay is what
// catches it, and it must catch it here rather than inside a probe.
//
// If stripRESTPrefix is ever widened to a segment scan like the sandbox's,
// this fixture stops being unreplayable and the test fails; pick another base
// the replayer does not normalise, or retire it with the failure mode.
func TestCaptureScenario_RefusesACaptureThatCannotReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const unnormalisedBase = "https://sandbox.local/api/openehr/v1"

	prov := validProvenance()
	prov.BaseURL = unnormalisedBase

	_, err := captureScenario(t.Context(), scenarios["ehr-lifecycle"], unnormalisedBase, sandbox.New(), nil, prov, dir)
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("captureScenario under an unnormalised REST base = %v, want %v", err, probe.ErrUnmatchedRecording)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("an unreplayable capture published %v, want nothing", names)
	}
}

// optTemplateID finds the first template_id value in an ADL 1.4 OPT, which
// is the template's own id.
var optTemplateID = regexp.MustCompile(`(?s)<template_id>\s*<value>([^<]+)</value>`)

// compositionServer answers the composition-minimal scenario the way a CDR
// does, on top of the sandbox's EHR surface. Its template store holds each
// id once: an upload naming an id it already holds gets 409, as the
// ITS-REST Definition API answers a repeated template_id
// (409_template_already_exists). A save gets 422 unless its
// openehr-template-id header and its archetype_details.template_id both
// name a template the store holds.
type compositionServer struct {
	mu        sync.Mutex
	templates []string          // template ids, in upload order
	saved     map[string][]byte // composition bodies, by version uid
}

func newCompositionServer() *compositionServer {
	return &compositionServer{saved: make(map[string][]byte)}
}

// backend returns a sandbox serving s's routes. Every call shares s's store,
// so two captures against two backends from one s meet the same templates.
func (s *compositionServer) backend() *sandbox.Backend {
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, "/definition/template/adl1.4", s.uploadTemplate)
	b.HandleFunc(http.MethodPost, "/ehr/", s.saveComposition)
	b.HandleFunc(http.MethodGet, "/ehr/", s.getComposition)
	return b
}

// uploaded returns the template ids the store holds, in upload order.
func (s *compositionServer) uploaded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.templates)
}

func (s *compositionServer) uploadTemplate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	m := optTemplateID.FindSubmatch(body)
	if m == nil {
		http.Error(w, "the OPT names no template_id", http.StatusBadRequest)
		return
	}
	id := string(m[1])
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.templates, id) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"template already exists"}`)
		return
	}
	s.templates = append(s.templates, id)
	w.Header().Set("Location", "https://sandbox.local/openehr/v1/definition/template/adl1.4/"+id)
	w.WriteHeader(http.StatusCreated)
}

func (s *compositionServer) saveComposition(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/composition") {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var comp struct {
		ArchetypeDetails struct {
			TemplateID struct {
				Value string `json:"value"`
			} `json:"template_id"`
		} `json:"archetype_details"`
	}
	if err := json.Unmarshal(body, &comp); err != nil {
		http.Error(w, "the body is not a composition", http.StatusBadRequest)
		return
	}
	header := r.Header.Get("openehr-template-id")
	s.mu.Lock()
	defer s.mu.Unlock()
	if header == "" || header != comp.ArchetypeDetails.TemplateID.Value || !slices.Contains(s.templates, header) {
		http.Error(w, "the composition names no uploaded template", http.StatusUnprocessableEntity)
		return
	}
	uid := fmt.Sprintf("composition-%d", len(s.saved)+1)
	s.saved[uid] = body
	w.Header().Set("Location", "https://sandbox.local"+r.URL.Path+"/"+uid)
	w.Header().Set("ETag", `"`+uid+`::sandbox.local::1"`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *compositionServer) getComposition(w http.ResponseWriter, r *http.Request) {
	// A read names the versioned object or one of its versions, as on a
	// real server; the store keys bodies by the versioned object.
	dir, ref := path.Split(r.URL.Path)
	uid, _, _ := strings.Cut(ref, "::")
	s.mu.Lock()
	body, ok := s.saved[uid]
	s.mu.Unlock()
	if !strings.HasSuffix(dir, "/composition/") || !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// TestCaptureScenario_CompositionMinimalCapturesTwiceAgainstOneServer_REQ082
// pins the per-capture template id (REQ-082 Live: the per-run identifier
// appears in every resource a mutating capture creates). A CDR holds a
// template id once, so a capture that always uploaded under one fixed id
// got 409 the second time against the same server and could never refresh
// the recording. Two captures against one server must both succeed, each
// under a template id of its own; the server refuses a save that does not
// name the template its capture uploaded.
func TestCaptureScenario_CompositionMinimalCapturesTwiceAgainstOneServer_REQ082(t *testing.T) {
	t.Parallel()
	srv := newCompositionServer()
	for i := range 2 {
		_, err := captureScenario(t.Context(), scenarios["composition-minimal"], "https://sandbox.local/openehr/v1",
			srv.backend(), nil, validProvenance(), t.TempDir())
		if err != nil {
			t.Fatalf("capture %d of composition-minimal against one server = %v, want success", i+1, err)
		}
	}
	got := srv.uploaded()
	if len(got) != 2 || got[0] == got[1] {
		t.Fatalf("two captures uploaded template ids %q, want two different ids", got)
	}
}

// TestRun_HelpNeverEchoesTheCredentialEnvVar pins that the -basic credential
// is read from the environment after parsing, never installed as the flag's
// default. flag.PrintDefaults prints a non-empty default, so a default sourced
// from $OPENEHR_LIVE_EHRBASE_BASIC puts user:pass on stderr for `-h` and for
// every parse error.
//
// Restoring os.Getenv as the flag default fails here.
func TestRun_HelpNeverEchoesTheCredentialEnvVar(t *testing.T) {
	// No t.Parallel: t.Setenv.
	const secret = "operator:s3cr3t-passphrase"
	t.Setenv("OPENEHR_LIVE_EHRBASE_BASIC", secret)
	t.Setenv("OPENEHR_LIVE_EHRBASE", "https://operator:s3cr3t-passphrase@cdr.example/openehr/v1")

	var out bytes.Buffer
	if err := run([]string{"-h"}, &out); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("run(-h) = %v, want %v", err, flag.ErrHelp)
	}
	if strings.Contains(out.String(), "s3cr3t-passphrase") {
		t.Fatalf("usage echoed a credential from the environment:\n%s", out.String())
	}
}

// TestProvenanceBaseURL_RefusesEveryCredentialChannel holds the up-front -base
// check to the same channels ValidateHAR scans, so a credential-bearing base
// is refused before a mutating capture runs rather than after.
func TestProvenanceBaseURL_RefusesEveryCredentialChannel(t *testing.T) {
	t.Parallel()
	for name, base := range map[string]string{
		"userinfo":           "https://operator:s3cr3t@cdr.example/openehr/v1",
		"credential query":   "https://cdr.example/openehr/v1?access_token=s3cr3t",
		"user-only userinfo": "https://operator@cdr.example/openehr/v1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := provenanceBaseURL(base); err == nil {
				t.Fatalf("provenanceBaseURL(%s) accepted a credential-bearing base", name)
			} else if strings.Contains(err.Error(), "s3cr3t") {
				t.Fatalf("refusal echoed the credential: %v", err)
			}
		})
	}
	clean := "https://cdr.example/openehr/v1"
	got, err := provenanceBaseURL("  " + clean + "  ")
	if err != nil {
		t.Fatalf("provenanceBaseURL on a clean base = %v", err)
	}
	if got != clean {
		t.Fatalf("provenanceBaseURL trimmed to %q, want %q", got, clean)
	}
}
