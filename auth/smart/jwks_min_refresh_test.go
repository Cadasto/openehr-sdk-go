package smart_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
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

// TestREQ062_NewJWKSSetsTheMinimumRefreshInterval pins REQ-062: NewJWKS sets
// the minimum interval between kid-miss fetches to its 30-second default.
func TestREQ062_NewJWKSSetsTheMinimumRefreshInterval(t *testing.T) {
	jwks, err := smart.NewJWKS(&http.Client{}, "https://idp.test/jwks")
	if err != nil {
		t.Fatal(err)
	}
	if jwks.MinRefreshInterval != defaultMinRefreshInterval {
		t.Errorf("NewJWKS().MinRefreshInterval = %v, want %v", jwks.MinRefreshInterval, defaultMinRefreshInterval)
	}
}

// TestREQ062_NoMinimumRefreshIntervalFetchesOnEveryMiss pins REQ-062: a
// MinRefreshInterval of zero or less sets no minimum, so every kid miss
// fetches the set once before it fails.
func TestREQ062_NoMinimumRefreshIntervalFetchesOnEveryMiss(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				srv := &scriptedServer{keySet: func(r *http.Request, _ int) (*http.Response, error) {
					return scriptedAnswer(r, http.StatusOK, `{"keys":[{"kty":"oct","kid":"k0","use":"sig"}]}`), nil
				}}
				jwks := scriptedJWKS(t, srv)
				jwks.MinRefreshInterval = interval
				for i := range 3 {
					kid := fmt.Sprintf("unknown-%d", i)
					if _, err := jwks.Key(t.Context(), kid); !errors.Is(err, auth.ErrJWKSValidationFailed) {
						t.Fatalf("Key(%q) error = %v, want ErrJWKSValidationFailed", kid, err)
					}
				}
				if _, n := srv.counts(); n != 3 {
					t.Errorf("3 kid misses with MinRefreshInterval %v fetched the key set %d time(s), want 3", interval, n)
				}
			})
		})
	}
}

// TestREQ062_ConcurrentKidMissesShareOneFetch pins REQ-062 with REQ-026:
// concurrent lookups that miss a set older than the minimum interval share
// one fetch, and each fails with auth.ErrJWKSValidationFailed when the fetched
// set does not hold its kid either.
func TestREQ062_ConcurrentKidMissesShareOneFetch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		srv := &scriptedServer{keySet: func(r *http.Request, n int) (*http.Response, error) {
			if n > 1 {
				<-gate
			}
			return scriptedAnswer(r, http.StatusOK, `{"keys":[{"kty":"oct","kid":"k0","use":"sig"}]}`), nil
		}}
		jwks := scriptedJWKS(t, srv)
		if _, err := jwks.Key(t.Context(), "k0"); err != nil {
			t.Fatalf("Key(k0) error = %v, want nil", err)
		}
		time.Sleep(defaultMinRefreshInterval)

		var wg sync.WaitGroup
		for i := range 8 {
			kid := fmt.Sprintf("unknown-%d", i)
			wg.Go(func() {
				if _, err := jwks.Key(t.Context(), kid); !errors.Is(err, auth.ErrJWKSValidationFailed) {
					t.Errorf("Key(%q) error = %v, want ErrJWKSValidationFailed", kid, err)
				}
			})
		}
		synctest.Wait()
		close(gate)
		wg.Wait()
		if _, n := srv.counts(); n != 2 {
			t.Errorf("8 concurrent kid misses fetched the key set %d time(s) in all, want 2: the first lookup and one shared refresh", n)
		}
	})
}

// TestREQ062_StaleSetIsFetchedWhateverTheInterval pins REQ-062: a set older
// than a positive TTL is fetched again on a kid miss even when the minimum
// refresh interval is longer than the TTL.
func TestREQ062_StaleSetIsFetchedWhateverTheInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{keySet: func(r *http.Request, _ int) (*http.Response, error) {
			return scriptedAnswer(r, http.StatusOK, `{"keys":[{"kty":"oct","kid":"k0","use":"sig"}]}`), nil
		}}
		jwks := scriptedJWKS(t, srv)
		jwks.TTL = 10 * time.Second
		if _, err := jwks.Key(t.Context(), "k0"); err != nil {
			t.Fatalf("Key(k0) error = %v, want nil", err)
		}
		time.Sleep(jwks.TTL)
		if _, err := jwks.Key(t.Context(), "unknown"); !errors.Is(err, auth.ErrJWKSValidationFailed) {
			t.Fatalf("Key(unknown) error = %v, want ErrJWKSValidationFailed", err)
		}
		if _, n := srv.counts(); n != 2 {
			t.Errorf("a kid miss on a set the TTL old, %v before the minimum interval ends, fetched the key set %d time(s) in all, want 2", defaultMinRefreshInterval-jwks.TTL, n)
		}
	})
}
