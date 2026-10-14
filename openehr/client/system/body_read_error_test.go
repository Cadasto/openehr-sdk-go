package system_test

// body_read_error_test.go — REQ-172 § Metadata still arrives, held against the
// System leaf: Capabilities against a 200 whose body is cut mid-read still
// hands back the response headers beside the *transport.BodyReadError.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestCapabilitiesCutBodyKeepsMetadata requires system.Capabilities against a
// 200 whose body is cut to return no capabilities, the metadata carrying the
// ETag the server sent, and an error from which the *transport.BodyReadError
// is recovered.
func TestCapabilitiesCutBodyKeepsMetadata(t *testing.T) { // REQ-172
	const etag = "caps-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+etag+`"`)
		// Announce more bytes than are written, then drop the connection.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"solution":"`)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)

	caps, meta, err := system.Capabilities(t.Context(), newClient(t, srv))
	if err == nil {
		t.Fatal("Capabilities against a 200 whose body is cut succeeded; the premise of this test is gone")
	}
	if caps != nil {
		t.Errorf("Capabilities returned %+v beside a body-read failure; a partial body is not the representation", caps)
	}
	if meta == nil {
		t.Fatal("Capabilities returned nil *transport.Metadata; REQ-172 requires the response headers come back beside the error")
	}
	if meta.ETag != etag {
		t.Errorf("Metadata.ETag = %q, want %q", meta.ETag, etag)
	}

	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusOK {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusOK)
	}
}
