package query_test

// version_trim_test.go — REQ-057 § Path-parameter normalisation, execution
// side. `qualifiedName` and `version` are each trimmed once inside
// runStoredAtVersion and the trimmed value is reused for both the empty-check
// and the wire path. An EXPLICIT version that is empty after trimming is
// refused before any request is issued: only RunStored addresses the latest
// version, and it does so by omitting the /{version} segment, so a silent
// fallback would execute different query logic than the caller asked for.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
)

func TestRunStoredVersionTrimsVersion(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNotImplemented) // assert request shape only
	}))
	defer srv.Close()

	// The stub answers 501, so a non-nil error is the expected outcome; the
	// assertion is on the path the request carried, not on the verdict.
	_, _, err := query.RunStoredVersion(t.Context(), newClient(t, srv), "org.example.q", " v1 ", nil)
	if err == nil {
		t.Fatal("RunStoredVersion err = nil, want the stub's 501 — the request never reached the server")
	}
	const want = "/openehr/v1/query/org.example.q/v1"
	if gotPath != want {
		t.Errorf("path = %q, want %q (version reached the wire untrimmed)", gotPath, want)
	}
}

// TestRunStoredVersionRefusesEmptyVersion pins the refusal half: an explicit
// version that is empty after trimming is a caller error, not a request for the
// latest version. Before the trim landed, whitespace stayed on the versioned
// route and the server ruled on it; trimming alone would have turned the same
// call into the unversioned route, silently executing the latest version.
func TestRunStoredVersionRefusesEmptyVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"whitespace only", "   "},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits++
				w.WriteHeader(http.StatusNotImplemented)
			}))
			defer srv.Close()

			_, _, err := query.RunStoredVersion(t.Context(), newClient(t, srv), "org.example.q", tt.version, nil)
			if err == nil {
				t.Fatalf("RunStoredVersion(version=%q) err = nil, want a refusal", tt.version)
			}
			if !errors.Is(err, query.ErrInvalidConfig) {
				t.Errorf("RunStoredVersion(version=%q) err = %v, want errors.Is(err, query.ErrInvalidConfig)", tt.version, err)
			}
			const prefix = "query.RunStoredVersion: "
			if !strings.HasPrefix(err.Error(), prefix) {
				t.Errorf("RunStoredVersion(version=%q) err = %q, want the %q prefix naming the invoked operation", tt.version, err, prefix)
			}
			if hits != 0 {
				t.Errorf("server hits = %d, want 0 — the refusal MUST precede any request", hits)
			}
		})
	}
}
