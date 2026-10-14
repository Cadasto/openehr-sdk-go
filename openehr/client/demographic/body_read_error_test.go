package demographic_test

// body_read_error_test.go — REQ-172 § Metadata still arrives, held against the
// PARTY read leaf (getParty, behind Get): a 200 whose body is cut mid-read
// still hands back the version metadata the ETag names, beside the
// *transport.BodyReadError.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestGetCutBodyKeepsVersionMetadata requires demographic.Get against a 200
// whose body is cut to return no PARTY, the version metadata the ETag names,
// and an error from which the *transport.BodyReadError is recovered.
func TestGetCutBodyKeepsVersionMetadata(t *testing.T) { // REQ-172
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+personVersion+`"`)
		// Announce more bytes than are written, then drop the connection.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"_type":"PERSON","name":{"value":"`)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)

	party, meta, err := demographic.Get(t.Context(), newClient(t, srv), demographic.Person,
		openehrclient.LatestOf(personVOID))
	if err == nil {
		t.Fatal("Get against a 200 whose body is cut succeeded; the premise of this test is gone")
	}
	if party != nil {
		t.Errorf("Get returned %+v beside a body-read failure; a partial body is not the representation", party)
	}
	if meta == nil {
		t.Fatal("Get returned nil version metadata; REQ-172 requires the response metadata come back beside the error")
	}
	if meta.VersionUID != personVersion {
		t.Errorf("VersionMetadata.VersionUID = %q, want %q, the version the ETag names", meta.VersionUID, personVersion)
	}

	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusOK {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusOK)
	}
}
