package ehr_test

// path_encoding_test.go — REQ-095. Regression proof for the previously-latent
// EHR client: a path parameter carrying a percent-encodable character reaches
// the wire encoded exactly once (the client interpolates the raw id; the
// transport is the single canonical encoder). Real EHR ids are UUIDs with no
// encodable character, so the space here is a deliberate stressor for the
// encoding path — it must not double-encode (`%20` → `%2520` → 404).

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
)

func TestGetPathSingleEncoded(t *testing.T) {
	var decoded, escaped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded = r.URL.Path
		escaped = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotImplemented) // assert request shape only
	}))
	defer srv.Close()
	_, _, _ = ehr.Get(t.Context(), newClient(t, srv), ehr.EHRID("ehr id with space"))
	if want := "/openehr/v1/ehr/ehr id with space"; decoded != want {
		t.Errorf("server-decoded path = %q, want %q (double-encode leaks a literal %%20)", decoded, want)
	}
	if want := "/openehr/v1/ehr/ehr%20id%20with%20space"; escaped != want {
		t.Errorf("wire path = %q, want %q (single-encoded, not %%2520)", escaped, want)
	}
}
