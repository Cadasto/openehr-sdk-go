package composition_test

// path_encoding_test.go — REQ-095. Regression proof for a two-parameter path:
// the ehr_id segment carrying a percent-encodable character reaches the wire
// encoded exactly once, never double-encoded.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
)

func TestGetPathSingleEncoded(t *testing.T) {
	var decoded, escaped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoded = r.URL.Path
		escaped = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotImplemented) // assert request shape only
	}))
	defer srv.Close()
	_, _, _ = composition.Get(t.Context(), newClient(t, srv),
		openehrclient.EHRID("ehr id with space"),
		openehrclient.LatestOf(openehrclient.VersionedObjectID("bp.v1")))
	if !strings.Contains(decoded, "/ehr/ehr id with space/composition/") {
		t.Errorf("server-decoded path = %q, want the raw spaced ehr_id segment", decoded)
	}
	if !strings.Contains(escaped, "/ehr/ehr%20id%20with%20space/composition/") {
		t.Errorf("wire path = %q, want a single-encoded ehr_id segment", escaped)
	}
	if strings.Contains(escaped, "%2520") {
		t.Errorf("wire path double-encoded: %q", escaped)
	}
}
