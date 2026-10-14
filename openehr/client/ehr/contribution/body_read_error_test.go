package contribution_test

// body_read_error_test.go — REQ-172 § Metadata still arrives, held against the
// Contribution write leaf: a 201 whose body is cut mid-read still hands back
// the metadata that proves the commit, beside the *transport.BodyReadError.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestCommitCutBodyKeepsMetadata requires contribution.Commit against a 201
// whose body is cut to return no contribution, the metadata carrying the ETag
// and Location the server sent, and an error from which the
// *transport.BodyReadError is recovered. The caller can then tell the commit
// happened and which contribution it made, instead of sending it again.
func TestCommitCutBodyKeepsMetadata(t *testing.T) { // REQ-172
	const contributionUID = "0826851c-c4c2-4d61-92b9-410fb8275ff0"
	location := "/openehr/v1/ehr/" + string(ehrIDFixture) + "/contribution/" + contributionUID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"`+contributionUID+`"`)
		w.Header().Set("Location", location)
		// Announce more bytes than are written, then drop the connection.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"_type":"CONTRIBUTION","uid":{"value":"`)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)

	batch := &contribution.Submission{
		Audit:    newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, batch,
		contribution.WithPrefer(transport.PreferRepresentation))
	if err == nil {
		t.Fatal("Commit against a 201 whose body is cut succeeded; the premise of this test is gone")
	}
	if out != nil {
		t.Errorf("Commit returned %+v beside a body-read failure; a partial body is not the representation", out)
	}
	if meta == nil || meta.Metadata == nil {
		t.Fatal("Commit returned nil metadata; REQ-172 requires the commit's metadata come back beside the error")
	}
	if meta.ETag != contributionUID {
		t.Errorf("Metadata.ETag = %q, want %q", meta.ETag, contributionUID)
	}
	if meta.Location != location {
		t.Errorf("Metadata.Location = %q, want %q", meta.Location, location)
	}

	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusCreated {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusCreated)
	}
	// A body that was never read is not REQ-094's committed-but-unusable
	// representation either.
	if nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
		t.Errorf("errors.AsType[*ehr.NoRepresentationError] matched a body-read failure (%v)", nre)
	}
}
