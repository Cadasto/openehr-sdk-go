package demographic_test

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

// TestGetNullBodyIsInvalidShape pins § REQ-151's refusal arm on the
// polymorphic demographic read: a 200 whose body is the JSON `null` literal is
// a wire anomaly exactly like an empty one — it MUST surface as
// transport.ErrInvalidShape rather than as "no version" (which only a 204 may
// mean) or as a *transport.DecodeError. The version read shares the arm, so the
// versioned_party/{uid}/version route is exercised too.
func TestGetNullBodyIsInvalidShape(t *testing.T) { // REQ-151
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, " null ")
	}))
	defer srv.Close()
	c := newClient(t, srv)

	party, _, err := demographic.Get(t.Context(), c, demographic.Person, openehrclient.LatestOf(personVOID))
	if party != nil {
		t.Fatalf("Get(null body) = %+v, want nil", party)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("Get(null body) err = %v, want errors.Is(err, transport.ErrInvalidShape)", err)
	}
	if _, ok := errors.AsType[*transport.DecodeError](err); ok {
		t.Fatalf("Get(null body) err = %v (%T); the no-representation arm is not a DecodeError", err, err)
	}

	pv, _, err := demographic.GetVersion(t.Context(), c, openehrclient.VersionedObjectID(personVOID))
	if pv != nil {
		t.Fatalf("GetVersion(null body) = %+v, want nil", pv)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("GetVersion(null body) err = %v, want errors.Is(err, transport.ErrInvalidShape)", err)
	}
}
