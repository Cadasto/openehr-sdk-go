package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
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
// writes is not a corpus recording — a sandbox capture witnesses nothing — but
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

// TestCaptureScenario_RefusedCaptureLeavesTheCorpusUntouched is the can-fail
// control for the validate-then-publish order. Incomplete provenance cannot
// pass HAR.Validate, so the capture must fail and the recording already in the
// output directory must survive byte-for-byte — a re-capture that goes wrong
// must not cost the corpus the witness it had. It also asserts no temp file is
// left behind.
//
// Publishing before validating (the order this replaces) fails here: the
// sentinel is overwritten.
func TestCaptureScenario_RefusedCaptureLeavesTheCorpusUntouched(t *testing.T) {
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
