package definition_test

// version_trim_test.go — REQ-057. DeleteStoredQuery trims `version` only for
// the empty-check (stored_query.go:392) but builds Path from the raw value —
// an asymmetry with `name`, which is trimmed once and reused for both. Trim
// once, mirroring name.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
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
