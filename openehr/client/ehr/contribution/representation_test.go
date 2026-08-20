package contribution_test

import (
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

	var nre *openehrclient.NoRepresentationError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("empty representation body must wrap ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Contribution on empty body, got %+v", out)
	}
	if meta == nil || nre.Meta == nil {
		t.Error("commit metadata must survive")
	}
}

// REQ-094: PreferRepresentation with an undecodable 2xx body is a
// NoRepresentationError wrapping the decode error.
func TestCommitRepresentationUndecodable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("}{ not json"))
	}))
	defer srv.Close()

	_, _, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))

	if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); !ok {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
}
