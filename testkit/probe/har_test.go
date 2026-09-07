package probe_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

func strand11HAR(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	// The live capture that settled ADR 0020 — same bytes the PR reviews.
	path := filepath.Join(filepath.Dir(src), "..", "..", "docs", "plans", "strand-11-evidence", "ehr-create.har")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHARAcceptsStrand11Capture(t *testing.T) {
	t.Parallel()
	rec, err := probe.ValidateHAR(strand11HAR(t))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Log.Entries[0].Response.Status != 201 {
		t.Fatalf("status = %d, want 201", rec.Log.Entries[0].Response.Status)
	}
}

func TestHARRejectsMissingAttestation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "no _req082",
			raw:  `{"log":{"version":"1.2","entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
		},
		{
			name: "redaction did not run",
			raw:  `{"log":{"version":"1.2","_req082":{"provenance":{"deployment":"x","captured_at":"t","sdk_commit":"c"},"redaction":{"ran":false}},"entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
		},
		{
			name: "empty provenance",
			raw:  `{"log":{"version":"1.2","_req082":{"provenance":{},"redaction":{"ran":true}},"entries":[{"request":{"method":"GET","url":"http://x"},"response":{"status":200}}]}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := probe.ValidateHAR([]byte(tc.raw))
			if !errors.Is(err, probe.ErrUnsatisfiableMode) {
				t.Fatalf("ValidateHAR() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
			}
		})
	}
}
