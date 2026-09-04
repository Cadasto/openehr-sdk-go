package composition_test

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

// TestGetNullBodyIsInvalidShape pins § REQ-151's refusal arm on the hand-rolled
// composition read: a 200 whose body is the JSON `null` literal carries no
// representation and MUST classify as transport.ErrInvalidShape — never as a
// *transport.DecodeError, and never as a populated all-zero *rm.Composition,
// which is what canjson.Unmarshal would silently produce for `null`. Reverting
// the leaf to a len(body) == 0 check fails this test with a nil error.
func TestGetNullBodyIsInvalidShape(t *testing.T) { // REQ-151
	for _, body := range []string{"null", "\n null \n"} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()

			out, meta, err := composition.Get(t.Context(), newClient(t, srv), ehrIDFixture, openehrclient.LatestOf(compositionVOID))
			if out != nil {
				t.Fatalf("Get(null body) = %+v, want nil: a null body must not present as a populated composition", out)
			}
			if !errors.Is(err, transport.ErrInvalidShape) {
				t.Fatalf("Get(null body) err = %v, want errors.Is(err, transport.ErrInvalidShape)", err)
			}
			if _, ok := errors.AsType[*transport.DecodeError](err); ok {
				t.Fatalf("Get(null body) err = %v (%T); the no-representation arm is not a DecodeError", err, err)
			}
			if meta == nil {
				t.Error("Get(null body) returned nil metadata beside the error")
			}
		})
	}
}
