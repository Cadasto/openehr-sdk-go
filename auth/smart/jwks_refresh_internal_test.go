package smart

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// jwksServing returns a JWKS on a test server that serves before until
// rotate is called and after from then on, and the count of its fetches.
func jwksServing(t *testing.T, before, after string) (j *JWKS, fetches *atomic.Int32, rotate func()) {
	t.Helper()
	var rotated atomic.Bool
	fetches = new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		body := before
		if rotated.Load() {
			body = after
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	j, err := NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	j.TTL = time.Hour
	return j, fetches, func() { rotated.Store(true) }
}

// TestREQ062_RefreshSkipsAFetchWhenANewerSetLanded pins REQ-062's minimum
// refresh interval against lookups that race: a lookup that read the set
// before another lookup's fetch landed does not fetch it again, since the
// set is already newer than the one it found wanting.
func TestREQ062_RefreshSkipsAFetchWhenANewerSetLanded(t *testing.T) {
	j, fetches, _ := jwksServing(t, `{"keys":[{"kty":"oct","kid":"a","use":"sig"}]}`, "")
	j.MinRefreshInterval = 0
	if err := j.refresh(t.Context(), time.Time{}); err != nil || fetches.Load() != 1 {
		t.Fatalf("first refresh = %v with %d fetch(es), want nil and 1", err, fetches.Load())
	}
	// A lookup that read the set before that fetch landed.
	if err := j.refresh(t.Context(), time.Time{}); err != nil {
		t.Fatalf("refresh after a newer set landed = %v, want nil", err)
	}
	if n := fetches.Load(); n != 1 {
		t.Fatalf("refresh after a newer set landed fetched the set again: %d fetch(es), want 1", n)
	}
	j.mu.Lock()
	seen := j.fetchedAt
	j.mu.Unlock()
	if err := j.refresh(t.Context(), seen); err != nil || fetches.Load() != 2 {
		t.Fatalf("refresh of the set as read = %v with %d fetch(es), want nil and 2", err, fetches.Load())
	}
}

// TestREQ062_KidlessRefreshReadsAKeyAnotherLookupFetched pins REQ-062 for a
// token without a kid: when the set is within the minimum interval, the
// lookup does not fetch, but it takes the signing key the set holds now, so a
// key another lookup's fetch rotated in since the token's key was read
// verifies the token.
func TestREQ062_KidlessRefreshReadsAKeyAnotherLookupFetched(t *testing.T) {
	j, fetches, rotate := jwksServing(t,
		`{"keys":[{"kty":"oct","kid":"old","use":"sig"}]}`,
		`{"keys":[{"kty":"oct","kid":"new","use":"sig"}]}`)
	old, err := j.Key(t.Context(), "")
	if err != nil || !strings.Contains(string(old), `"old"`) {
		t.Fatalf("Key(no kid) = (%s, %v), want the old key", old, err)
	}
	j.mu.Lock()
	seen := j.fetchedAt
	j.mu.Unlock()
	// Another lookup's fetch lands the rotated key.
	rotate()
	if err := j.refresh(t.Context(), seen); err != nil {
		t.Fatal(err)
	}
	key, err := j.refreshedSigningKey(t.Context())
	if err != nil || !strings.Contains(string(key), `"new"`) {
		t.Fatalf("refreshedSigningKey within the interval = (%s, %v), want the key the other fetch landed", key, err)
	}
	if n := fetches.Load(); n != 2 {
		t.Fatalf("refreshedSigningKey within the interval: %d fetch(es) in all, want 2", n)
	}
}
