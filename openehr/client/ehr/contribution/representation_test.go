package contribution_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func representationBatch() *contribution.Submission {
	return &contribution.Submission{
		Audit:    newAudit(),
		Versions: []contribution.CommitVersion{newOriginalVersion()},
	}
}

// REQ-094: PreferRepresentation with an empty 2xx body is a
// NoRepresentationError carrying the commit metadata, never a silent
// metadata-only success.
func TestCommitRepresentationEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated) // representation asked, empty body
	}))
	defer srv.Close()

	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))

	nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
	if !ok {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("empty representation body must wrap ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Contribution on empty body, got %+v", out)
	}
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive")
	}
	const wantUID openehrclient.VersionUID = "cont-1"
	if meta.VersionUID != wantUID || nre.Meta.VersionUID != wantUID {
		t.Errorf("VersionUID meta=%q nre=%q, want %q", meta.VersionUID, nre.Meta.VersionUID, wantUID)
	}
}

// REQ-094: a JSON null 2xx body classifies as empty — it would decode
// into rm.Contribution as a nil-error no-op, not as the persisted
// resource.
func TestCommitRepresentationNullBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))

	nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
	if !ok {
		t.Fatalf("null body: err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a null body classifies as empty and must wrap ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Contribution on a null body, got %+v", out)
	}
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive a null body")
	}
}

// REQ-094: a whitespace-only 2xx body classifies as empty
// (ErrInvalidShape), not as a decode failure.
func TestCommitRepresentationWhitespaceBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(" \n\t"))
	}))
	defer srv.Close()

	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))

	nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
	if !ok {
		t.Fatalf("whitespace body: err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a whitespace-only body classifies as empty and must wrap ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Contribution on a whitespace-only body, got %+v", out)
	}
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive a whitespace-only body")
	}
}

// REQ-094: PreferRepresentation with an undecodable 2xx body is a
// NoRepresentationError wrapping the decode error.
func TestCommitRepresentationUndecodable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("}{ not json"))
	}))
	defer srv.Close()

	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))

	nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
	if !ok {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if _, ok := errors.AsType[*json.SyntaxError](err); !ok {
		t.Fatalf("decode cause not in chain: %v", err)
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a decode failure is not an empty-body ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Contribution on undecodable body, got %+v", out)
	}
	const wantUID openehrclient.VersionUID = "cont-1"
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive a decode failure")
	}
	if meta.VersionUID != wantUID || nre.Meta.VersionUID != wantUID {
		t.Errorf("VersionUID meta=%q nre=%q, want %q", meta.VersionUID, nre.Meta.VersionUID, wantUID)
	}
}

// REQ-094: identifier mode is a nil-error success that returns no
// resource. Commit does not populate the identifier slot from the body
// (deferred); metadata comes from the response headers.
func TestCommitIdentifierSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uid":"cont-1"}`))
	}))
	defer srv.Close()

	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferIdentifier))
	if err != nil {
		t.Fatalf("identifier success err = %v", err)
	}
	if openehrclient.HasResource(out) {
		t.Errorf("identifier mode returns no Contribution, got %+v", out)
	}
	if meta == nil {
		t.Fatal("metadata must be populated in identifier mode")
	}
}

// REQ-094: a wire failure stays a *transport.WireError and is never
// reclassified as a NoRepresentationError, even when representation
// was requested.
func TestCommitRepresentationWireErrorNotNoRepresentation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"batch conflict","code":"VERSION_CONFLICT"}`))
	}))
	defer srv.Close()

	_, _, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))

	if _, ok := errors.AsType[*transport.WireError](err); !ok {
		t.Fatalf("409 err = %v, want *transport.WireError", err)
	}
	if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
		t.Fatal("a wire failure must not be a NoRepresentationError")
	}
}
