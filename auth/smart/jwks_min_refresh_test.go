package smart_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// defaultMinRefreshInterval is the minimum interval NewJWKS sets between a
// fetch of the key set and a fetch that a kid missing from it causes.
const defaultMinRefreshInterval = 30 * time.Second

// TestREQ062_KidMissesFetchTheSetAtMostOncePerInterval pins REQ-062 (auth.md §
// JWKS rotation): a lookup for a kid the cached set does not hold fetches the
// set again only when that set is at least the minimum interval old, 30
// seconds by default. A miss on a set fetched more recently fails at once with
// auth.ErrJWKSValidationFailed, without a fetch, so tokens that name unknown
// kids cannot make the SDK fetch the set for each of them. A miss once the
// interval has passed fetches the set and finds a key the server rotated to.
func TestREQ062_KidMissesFetchTheSetAtMostOncePerInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{keySet: func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				return scriptedAnswer(r, http.StatusOK, `{"keys":[{"kty":"oct","kid":"k0","use":"sig"}]}`), nil
			}
			return scriptedAnswer(r, http.StatusOK, `{"keys":[{"kty":"oct","kid":"k0","use":"sig"},{"kty":"oct","kid":"k1","use":"sig"}]}`), nil
		}}
		jwks := scriptedJWKS(t, srv)
		fetches := func() int {
			_, n := srv.counts()
			return n
		}
		miss := func(kid, when string) {
			t.Helper()
			if _, err := jwks.Key(t.Context(), kid); !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("Key(%q) %s error = %v, want ErrJWKSValidationFailed", kid, when, err)
			}
		}

		for i := range 20 {
			miss(fmt.Sprintf("unknown-%d", i), fmt.Sprintf("as miss %d of 20 in a row", i+1))
		}
		if n := fetches(); n != 1 {
			t.Fatalf("20 kid misses in a row fetched the key set %d time(s), want 1", n)
		}
		if _, err := jwks.Key(t.Context(), "k0"); err != nil || fetches() != 1 {
			t.Fatalf("Key(k0) after the misses = %v with %d fetch(es), want the cached key and 1 fetch", err, fetches())
		}

		time.Sleep(defaultMinRefreshInterval - time.Second)
		miss("k1", "a second before the interval ends")
		if n := fetches(); n != 1 {
			t.Fatalf("Key(k1) a second before the interval ends: key set fetched %d time(s) in all, want 1", n)
		}

		time.Sleep(time.Second)
		key, err := jwks.Key(t.Context(), "k1")
		if err != nil || !strings.Contains(string(key), `"kid":"k1"`) {
			t.Fatalf("Key(k1) once the set is the interval old = (%s, %v), want the key the server rotated to", key, err)
		}
		if n := fetches(); n != 2 {
			t.Fatalf("Key(k1) once the set is the interval old: key set fetched %d time(s) in all, want 2", n)
		}
	})
}
