package definition_test

// version_trim_test.go — REQ-057 § Path-parameter normalisation, Definition
// side. DeleteStoredQuery and GetStoredQuery trim `name` and `version` once
// each and reuse the trimmed values for both the empty-check and the wire
// path, so a version with surrounding whitespace reaches the server trimmed
// and one that is empty after trimming is refused before any request.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestDeleteStoredQueryTrimsVersion(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if _, err := definition.DeleteStoredQuery(t.Context(), newClient(t, srv), "org.example.q", " v1 "); err != nil {
		t.Fatalf("DeleteStoredQuery: %v", err)
	}
	const want = "/openehr/v1/definition/query/org.example.q/v1"
	if gotPath != want {
		t.Errorf("path = %q, want %q (version reached the wire untrimmed)", gotPath, want)
	}
}

// TestDeleteStoredQueryRefusesEmptyVersion is the Definition-side half of the
// refusal § REQ-057 makes normative: the delete route has no unversioned form,
// so an empty-after-trim version has no wire meaning at all and is a caller
// error rather than a request the server should rule on.
func TestDeleteStoredQueryRefusesEmptyVersion(t *testing.T) {
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
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			_, err := definition.DeleteStoredQuery(t.Context(), newClient(t, srv), "org.example.q", tt.version)
			if err == nil {
				t.Fatalf("DeleteStoredQuery(version=%q) err = nil, want a refusal", tt.version)
			}
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("DeleteStoredQuery(version=%q) err = %v, want errors.Is(err, transport.ErrInvalidConfig)", tt.version, err)
			}
			// Naming the operation is what separates this refusal from REQ-150's
			// path-segment validator, which also rejects an empty segment with the
			// same sentinel and no request: without this line the test stays green
			// when the operation's own guard is removed.
			if !strings.Contains(err.Error(), "definition.DeleteStoredQuery") {
				t.Errorf("DeleteStoredQuery(version=%q) err = %v, want the operation's own refusal naming definition.DeleteStoredQuery", tt.version, err)
			}
			if hits != 0 {
				t.Errorf("server hits = %d, want 0 — the refusal MUST precede any request", hits)
			}
		})
	}
}

// TestGetStoredQueryRefusesEmptyVersion is the Get half of the same
// Definition-side refusal (REQ-057 § Path-parameter normalisation).
func TestGetStoredQueryRefusesEmptyVersion(t *testing.T) {
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
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			_, _, err := definition.GetStoredQuery(t.Context(), newClient(t, srv), "org.example.q", tt.version)
			if err == nil {
				t.Fatalf("GetStoredQuery(version=%q) err = nil, want a refusal", tt.version)
			}
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("GetStoredQuery(version=%q) err = %v, want errors.Is(err, transport.ErrInvalidConfig)", tt.version, err)
			}
			// Naming the operation is what separates this refusal from REQ-150's
			// path-segment validator, which also rejects an empty segment with the
			// same sentinel and no request: without this line the test stays green
			// when the operation's own guard is removed.
			if !strings.Contains(err.Error(), "definition.GetStoredQuery") {
				t.Errorf("GetStoredQuery(version=%q) err = %v, want the operation's own refusal naming definition.GetStoredQuery", tt.version, err)
			}
			if hits != 0 {
				t.Errorf("server hits = %d, want 0 — the refusal MUST precede any request", hits)
			}
		})
	}
}
