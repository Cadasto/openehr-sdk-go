package contribution_test

// decode_error_test.go — REQ-151 § Keyed exclusions, held at contribution.Commit.
//
// Commit is named in the write-result funnel bullet: its
// committed-but-unusable arm is REQ-094's *ehr.NoRepresentationError and MUST
// NOT be re-typed by REQ-151. The distinction is not cosmetic — a Commit that
// reaches this arm has already been persisted by the server, so the caller's
// recovery is to re-read the contribution, not to re-parse the body a read
// failed on. Handing back a *transport.DecodeError would say the opposite.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestCommitUndecodableIsNotDecodeError is the negative pin. It restates the
// arm's existing contract — *ehr.NoRepresentationError carrying the commit
// metadata — and then asserts the exclusion itself: errors.AsType for
// *transport.DecodeError must NOT match. Deleting the exclusion from the code
// would fail this test.
func TestCommitUndecodableIsNotDecodeError(t *testing.T) { // REQ-151
	const undecodable = `{"_type":"NOT_A_CONTRIBUTION","uid":{"_type":"HIER_OBJECT_ID","value":"x"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/ehr/"+string(ehrIDFixture)+"/contribution/cont-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(undecodable))
	}))
	defer srv.Close()

	out, meta, err := contribution.Commit(t.Context(), newClient(t, srv), ehrIDFixture, representationBatch(),
		contribution.WithPrefer(transport.PreferRepresentation))
	if err == nil {
		t.Fatalf("Commit decoded %s as an rm.Contribution without error; the premise of this test is gone", undecodable)
	}

	nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
	if !ok {
		t.Fatalf("err = %T (%v), want *ehr.NoRepresentationError — REQ-094 owns contribution.Commit's committed-but-unusable arm", err, err)
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a decode failure is not an empty-body ErrInvalidShape")
	}
	if out != nil {
		t.Errorf("no Contribution on an undecodable representation body, got %+v", out)
	}
	const wantUID openehrclient.VersionUID = "cont-1"
	if meta == nil || nre.Meta == nil || nre.Meta.VersionUID != wantUID {
		t.Errorf("commit metadata must survive: meta=%v nre.Meta=%v", meta, nre.Meta)
	}

	// The keyed exclusion itself.
	if de, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Errorf("contribution.Commit's decode failure surfaced as *transport.DecodeError (%v); REQ-151 § Keyed exclusions leaves it to REQ-094's *ehr.NoRepresentationError", de)
	}
}
