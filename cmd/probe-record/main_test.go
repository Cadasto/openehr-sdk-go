package main

import (
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// TestCaptureScenario_ProducesAValidHARFromTheSandbox exercises the whole
// capture path offline: it drives the ehr-lifecycle scenario through the
// recorder against the in-memory sandbox (no CDR) and asserts the written
// recording passes ValidateHAR. The file it writes is not a corpus recording —
// a sandbox capture witnesses nothing — but it proves the harness produces a
// replayable, redaction-attested HAR. Dropping the ValidateHAR call inside
// captureScenario makes this test unable to catch a malformed capture.
func TestCaptureScenario_ProducesAValidHARFromTheSandbox(t *testing.T) {
	t.Parallel()
	prov := probe.HARProvenance{
		Deployment: "sandbox (test only — not a witness)",
		BaseURL:    "https://sandbox.local/openehr/v1",
		CapturedAt: "2026-09-08T00:00:00Z",
		SDKCommit:  "0000000000000000000000000000000000000000",
	}
	path, err := captureScenario(
		t.Context(),
		scenarios["ehr-lifecycle"],
		"https://sandbox.local/openehr/v1",
		sandbox.New(),
		nil,
		prov,
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("captureScenario against sandbox = %v", err)
	}
	if filepath.Base(path) != "ehr-lifecycle.har" {
		t.Fatalf("recording path = %q, want basename ehr-lifecycle.har", path)
	}
	// captureScenario already validated; assert it independently so this test
	// fails if that internal check is ever removed.
	har, err := probe.ValidateHAR(path)
	if err != nil {
		t.Fatalf("captured recording did not validate: %v", err)
	}
	if len(har.Log.Entries) != 3 {
		t.Fatalf("ehr-lifecycle recorded %d exchanges, want 3 (POST, GET, HEAD)", len(har.Log.Entries))
	}
}
