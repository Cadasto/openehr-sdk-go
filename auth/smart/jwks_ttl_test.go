package smart_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestREQ062_ShortJWKSTTLFindsAKidInTheSetJustFetched pins REQ-062 (auth.md §
// JWKS rotation): a lookup that fetched the key set looks its kid up in the
// set it fetched, without asking again whether the set is stale. So a TTL
// shorter than one fetch still finds a kid the set publishes, with one fetch,
// instead of reporting it as not found after the refresh.
//
// The test runs on the real clock: under a synctest fake clock no time
// passes between the fetch and the lookup, so the set would not go stale in
// between. Each kid gets a new, empty JWKS, so every lookup has to fetch. A
// shared JWKS would fetch again for the second kid only if the clock had
// moved since the first fetch, which a coarse clock may not do.
func TestREQ062_ShortJWKSTTLFindsAKidInTheSetJustFetched(t *testing.T) {
	const keySet = `{"keys":[{"kty":"oct","kid":"k0","use":"sig"},{"kty":"oct","kid":"k1","use":"sig"}]}`
	srv := &scriptedServer{keySet: func(r *http.Request, _ int) (*http.Response, error) {
		return scriptedAnswer(r, http.StatusOK, keySet), nil
	}}

	for _, want := range []string{"k1", "k0"} {
		jwks := scriptedJWKS(t, srv)
		jwks.TTL = time.Nanosecond
		_, before := srv.counts()
		key, err := jwks.Key(t.Context(), want)
		if err != nil || !strings.Contains(string(key), `"kid":"`+want+`"`) {
			t.Errorf("Key(%q) with a TTL of 1ns = (%s, %v), want the published key", want, key, err)
		}
		if _, after := srv.counts(); after-before != 1 {
			t.Errorf("Key(%q) fetched the key set %d times, want once", want, after-before)
		}
	}
}
