package composition_test

// body_read_error_test.go — REQ-151 § Metadata still arrives, at the
// Composition write leaf: a 201 whose body is cut mid-read still hands back
// the version metadata that proves the commit, beside the
// *transport.BodyReadError.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestSaveCutBodyKeepsVersionMetadata is the reported case at the leaf:
// composition.Save against a 201 whose body is cut returns no composition,
// the version metadata the ETag names, and an error from which the
// *transport.BodyReadError is recovered. A caller can then tell the write
// committed and which version it made, instead of retrying it blind.
func TestSaveCutBodyKeepsVersionMetadata(t *testing.T) { // REQ-151
	const newVUID openehrclient.VersionUID = "5ea3b1b0-7b8f-4f0c-9d3e-0c1f2a3b4c5d::repro.example::1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+string(newVUID)+`"`)
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/composition/"+string(newVUID))
		// Announce more bytes than are written, then drop the connection.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"_type":"COMPOSITION","name":{"value":"`)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer srv.Close()

	comp, meta, err := composition.Save(t.Context(), newClient(t, srv), ehrIDFixture, readComposition(t),
		composition.WithPrefer(transport.PreferRepresentation))
	if err == nil {
		t.Fatal("Save against a 201 whose body is cut succeeded; the premise of this test is gone")
	}
	if comp != nil {
		t.Errorf("Save returned %+v beside a body-read failure; a partial body is not the representation", comp)
	}
	if meta == nil {
		t.Fatal("Save returned nil version metadata; REQ-151 requires the commit's metadata come back beside the error")
	}
	if meta.VersionUID != newVUID {
		t.Errorf("VersionMetadata.VersionUID = %q, want %q, the version the ETag names", meta.VersionUID, newVUID)
	}

	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusCreated {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusCreated)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("errors.Is(err, io.ErrUnexpectedEOF) = false for %v", err)
	}
	// A body that was never read is not REQ-094's committed-but-unusable
	// representation either.
	if nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
		t.Errorf("errors.AsType[*ehr.NoRepresentationError] matched a body-read failure (%v)", nre)
	}
}

// TestGetCutBodyKeepsVersionMetadata is the read side of the same rule:
// composition.Get against a 200 whose body is cut returns no composition,
// the version metadata the ETag names, and the *transport.BodyReadError.
// The ETag is the only part of the response that arrived intact, so it is
// what the caller must still receive.
func TestGetCutBodyKeepsVersionMetadata(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"`+string(compositionVUID)+`"`)
		// Announce more bytes than are written, then drop the connection.
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"_type":"COMPOSITION","name":{"value":"`)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer srv.Close()

	comp, meta, err := composition.Get(t.Context(), newClient(t, srv), ehrIDFixture,
		openehrclient.LatestOf(compositionVOID))
	if err == nil {
		t.Fatal("Get against a 200 whose body is cut succeeded; the premise of this test is gone")
	}
	if comp != nil {
		t.Errorf("Get returned %+v beside a body-read failure; a partial body is not the representation", comp)
	}
	if meta == nil {
		t.Fatal("Get returned nil version metadata; REQ-151 requires the response metadata come back beside the error")
	}
	if meta.VersionUID != compositionVUID {
		t.Errorf("VersionMetadata.VersionUID = %q, want %q, the version the ETag names", meta.VersionUID, compositionVUID)
	}

	bre, ok := errors.AsType[*transport.BodyReadError](err)
	if !ok {
		t.Fatalf("errors.AsType[*transport.BodyReadError] did not match %T (%v)", err, err)
	}
	if bre.StatusCode != http.StatusOK {
		t.Errorf("BodyReadError.StatusCode = %d, want %d", bre.StatusCode, http.StatusOK)
	}
}
