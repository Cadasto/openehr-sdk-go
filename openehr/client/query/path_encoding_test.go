package query_test

// path_encoding_test.go — REQ-095. A stored-query qualified name with an
// encodable character must reach the wire single-encoded: the transport is the
// single canonical path encoder, so the client interpolates the raw name and
// does not pre-escape it (which would double-encode).

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
)

func TestRunStoredPathSingleEncoded(t *testing.T) {
	var decoded, escaped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded = r.URL.Path
		escaped = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotImplemented) // assert request shape only
	}))
	defer srv.Close()
	_, _, _ = query.RunStored(t.Context(), newClient(t, srv), "org.example.q with space", nil)
	if want := "/openehr/v1/query/org.example.q with space"; decoded != want {
		t.Errorf("server-decoded path = %q, want %q (double-encode leaks a literal %%20)", decoded, want)
	}
	if want := "/openehr/v1/query/org.example.q%20with%20space"; escaped != want {
		t.Errorf("wire path = %q, want %q (single-encoded, not %%2520)", escaped, want)
	}
}
