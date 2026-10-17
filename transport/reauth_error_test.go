package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TestREQ063_ReauthFailureKeepsThe401 pins what Do returns when the 401
// safety net calls Reauth and Reauth fails: the reauth error, wrapped, and
// beside it the 401 that triggered it. errors.Is still matches the reauth
// error and ErrUnauthorized, errors.AsType still recovers the *WireError with
// its status, the request is not sent again, and the text added stays
// value-free (REQ-063, REQ-093).
func TestREQ063_ReauthFailureKeepsThe401(t *testing.T) {
	t.Parallel()
	const ehrID = "7d44b88c-4199-4bad-97dc-d78268e01398"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	errReauth := errors.New("reauth boom")
	c, err := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(&stubTokenSource{tokens: []string{"tok"}}),
		WithReauthOn401(auth.ReautherFunc(func(context.Context) error { return errReauth })),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.Do(t.Context(), &Request{Method: http.MethodGet, Path: "/ehr/" + ehrID})

	if !errors.Is(err, errReauth) {
		t.Errorf("Do error = %v, want it to wrap the reauth error", err)
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Do error = %v, want errors.Is(err, ErrUnauthorized)", err)
	}
	we, ok := errors.AsType[*WireError](err)
	if !ok || we == nil {
		t.Fatalf("Do error = %v, want a *WireError in its chain", err)
	}
	if we.StatusCode != http.StatusUnauthorized {
		t.Errorf("WireError.StatusCode = %d, want %d", we.StatusCode, http.StatusUnauthorized)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "transport: reauth: reauth boom: ") {
		t.Errorf("Do error = %q, want it to start with the reauth error", msg)
	}
	if !strings.Contains(msg, "status=401") {
		t.Errorf("Do error = %q, want it to name the status 401", msg)
	}
	if strings.Contains(msg, ehrID) {
		t.Errorf("Do error = %q, want no caller identifier %q", msg, ehrID)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("upstream calls = %d, want exactly 1 (reauth failed, no retry)", n)
	}
}
