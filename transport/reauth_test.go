package transport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// fakeReauthSource is an Invalidatable TokenSource that yields a new token
// value after each Invalidate, so a test can prove a 401-driven re-auth
// re-acquired a fresh token.
type fakeReauthSource struct {
	mu          sync.Mutex
	counter     int
	invalidated int
}

func (s *fakeReauthSource) Token(_ context.Context) (auth.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return auth.Token{Value: fmt.Sprintf("tok-%d", s.counter), Type: "Bearer"}, nil
}

func (s *fakeReauthSource) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidated++
	s.counter++
}

// PROBE-073 proves REQ-063 — a wire 401 on an authenticated request triggers
// exactly one re-auth: the transport invalidates the TokenSource, re-acquires
// a fresh token and retries once. Covers the stale-token case a source cannot
// self-detect (e.g. an authorization server that omits expires_in, so the
// cached token has no known expiry and is never proactively refreshed).
func TestDo_Reauth401_InvalidatesAndRetriesOnce(t *testing.T) {
	var (
		mu    sync.Mutex
		auths []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := len(auths)
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		if n == 0 { // first hit: reject as if the cached token were stale
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	src := &fakeReauthSource{}
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()), WithTokenSource(src))

	_, err := c.Do(context.Background(), &Request{Method: "GET", Path: "/ehr/abc"})
	if err != nil {
		t.Fatalf("Do after re-auth: %v (verwacht succes na token-refresh)", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(auths) != 2 {
		t.Fatalf("server zag %d requests, verwacht 2 (401 + retry): %v", len(auths), auths)
	}
	if auths[0] != "Bearer tok-0" {
		t.Errorf("eerste request Authorization = %q, verwacht stale token Bearer tok-0", auths[0])
	}
	if auths[1] != "Bearer tok-1" {
		t.Errorf("retry Authorization = %q, verwacht verse token Bearer tok-1", auths[1])
	}
	if src.invalidated != 1 {
		t.Errorf("Invalidate %d× aangeroepen, verwacht precies 1", src.invalidated)
	}
}

// A non-invalidatable source (StaticTokenSource) cannot refresh, so a 401 is
// surfaced to the caller without a retry loop — exactly one wire request.
func TestDo_Reauth401_NonInvalidatableSurfaces401(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()),
		WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "static", Type: "Bearer"})))

	_, err := c.Do(context.Background(), &Request{Method: "GET", Path: "/ehr/abc"})
	if err == nil {
		t.Fatal("verwacht een 401-fout (static source kan niet verversen)")
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("server zag %d requests, verwacht 1 (geen reauth-retry)", hits)
	}
}
