package query_test

// version_trim_test.go — REQ-057. `qualifiedName` is trimmed before it reaches
// the stored-query path (runStoredAtVersion), but `version` is used raw: a
// version with surrounding whitespace passes the empty-check untouched and
// lands in the URL as-is. Trim once, mirroring qualifiedName.

import (
	"net/http"
	"net/http/httptest"
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

	_, _, _ = query.RunStoredVersion(t.Context(), newClient(t, srv), "org.example.q", " v1 ", nil)
	const want = "/openehr/v1/query/org.example.q/v1"
	if gotPath != want {
		t.Errorf("path = %q, want %q (version reached the wire untrimmed)", gotPath, want)
	}
}
