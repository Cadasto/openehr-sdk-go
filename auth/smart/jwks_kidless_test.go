package smart_test

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// publishedKey is one entry of a JWKS document built by keySetBody.
type publishedKey struct {
	key *rsa.PrivateKey
	kid string // empty publishes the key without a kid
	use string // empty publishes the key without a use
}

// keySetBody marshals the public halves of keys as a JWKS document.
func keySetBody(t *testing.T, keys ...publishedKey) []byte {
	t.Helper()
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{}}
	for _, k := range keys {
		set.Keys = append(set.Keys, gojose.JSONWebKey{Key: &k.key.PublicKey, KeyID: k.kid, Use: k.use})
	}
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// TestValidateIDTokenWithoutKid walks the rule for an ID token whose header
// carries no kid: it verifies with the set's only signing key, a key whose
// use, if present, is sig, and is refused when the set holds no such key or
// more than one. Each refused token is signed by a key in the set, so it
// would verify if the SDK picked that key. REQ-062
func TestValidateIDTokenWithoutKid(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	first, second := newRSAKey(t), newRSAKey(t)

	cases := []struct {
		name    string
		set     []publishedKey
		signer  *rsa.PrivateKey
		wantErr bool
	}{
		{name: "one key without kid or use", set: []publishedKey{{key: first}}, signer: first},
		{name: "one key without kid, use sig", set: []publishedKey{{key: first, use: "sig"}}, signer: first},
		{name: "one key that has a kid", set: []publishedKey{{key: first, kid: "kid-1", use: "sig"}}, signer: first},
		{name: "one signing key beside an encryption key", set: []publishedKey{{key: second, use: "enc"}, {key: first, use: "sig"}}, signer: first},
		{name: "two signing keys, token signed by the first", set: []publishedKey{{key: first}, {key: second, use: "sig"}}, signer: first, wantErr: true},
		{name: "two signing keys, token signed by the second", set: []publishedKey{{key: first}, {key: second, use: "sig"}}, signer: second, wantErr: true},
		{name: "two signing keys with kids", set: []publishedKey{{key: first, kid: "kid-1"}, {key: second, kid: "kid-2"}}, signer: first, wantErr: true},
		{name: "only an encryption key", set: []publishedKey{{key: first, use: "enc"}}, signer: first, wantErr: true},
		{name: "no keys", signer: first, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jwks, _ := stubJWKSServer(t, http.StatusOK, keySetBody(t, tc.set...))
			tok := joseSign(t, gojose.RS256, tc.signer, "", defaultIDClaims(now))

			claims, err := smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", "nonce-xyz", now, nil)
			// REQ-062: a token without kid verifies only against a set holding exactly one signing key.
			switch {
			case tc.wantErr && (err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed)):
				t.Fatalf("ValidateIDToken(no kid, %s) error = %v, want ErrJWKSValidationFailed", tc.name, err)
			case !tc.wantErr && err != nil:
				t.Fatalf("ValidateIDToken(no kid, %s) error = %v, want nil", tc.name, err)
			case !tc.wantErr && claims.Subject != "user-1":
				t.Fatalf("ValidateIDToken(no kid, %s) Subject = %q, want user-1", tc.name, claims.Subject)
			}
		})
	}
}

// TestValidateIDTokenWithoutKidSkipsNonKeyEntries checks that entries of the
// keys array that are not JSON objects (null, a string) do not count as
// signing keys, so the one real key still verifies a token without a kid.
// REQ-062
func TestValidateIDTokenWithoutKidSkipsNonKeyEntries(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwk, err := json.Marshal(gojose.JSONWebKey{Key: &priv.PublicKey, Use: "sig"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"keys":[null,"not a key",` + string(jwk) + `]}`)
	jwks, _ := stubJWKSServer(t, http.StatusOK, body)
	tok := joseSign(t, gojose.RS256, priv, "", defaultIDClaims(now))

	_, err = smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	// REQ-062: only a key counts towards the one signing key a token without kid needs.
	if err != nil {
		t.Fatalf("ValidateIDToken(no kid, keys [null, string, one key]) error = %v, want nil", err)
	}
}

// TestJWKSKeyWithoutKid checks that JWKS.Key with an empty kid returns the
// set's only signing key, kept although it was published without a kid.
// REQ-062
func TestJWKSKeyWithoutKid(t *testing.T) {
	jwks, fetches := stubJWKSServer(t, http.StatusOK, keySetBody(t, publishedKey{key: newRSAKey(t), use: "sig"}))

	raw, err := jwks.Key(t.Context(), "")
	if err != nil {
		t.Fatalf("Key(no kid) error = %v, want the only signing key", err)
	}
	var got gojose.JSONWebKey
	if err := got.UnmarshalJSON(raw); err != nil {
		t.Fatalf("Key(no kid) returned %s, which does not parse as a JWK: %v", raw, err)
	}
	if got.KeyID != "" || got.Use != "sig" {
		t.Fatalf("Key(no kid) = kid %q use %q, want the key published without a kid", got.KeyID, got.Use)
	}
	if n := fetches.Load(); n != 1 {
		t.Fatalf("Key(no kid) fetched the JWKS %d time(s), want 1", n)
	}
}

// TestValidateIDTokenKidMissesKidlessKey checks that a token that names a kid
// is still looked up by that kid: a set whose only key has no kid does not
// match it, and the miss refreshes the set once before the token is refused.
// REQ-062
func TestValidateIDTokenKidMissesKidlessKey(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks, fetches := stubJWKSServer(t, http.StatusOK, keySetBody(t, publishedKey{key: priv}))
	jwks.TTL = time.Hour

	// Seed the cache with a kid-less lookup, which succeeds: one fetch.
	if _, err := jwks.Key(t.Context(), ""); err != nil {
		t.Fatalf("Key(no kid) error = %v, want nil", err)
	}

	tok := joseSign(t, gojose.RS256, priv, "kid-1", defaultIDClaims(now))
	_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	// REQ-062: a kid is looked up by kid only, with one refresh on a miss.
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("ValidateIDToken(kid-1 against a set whose only key has no kid) error = %v, want ErrJWKSValidationFailed", err)
	}
	if n := fetches.Load(); n != 2 {
		t.Fatalf("JWKS fetched %d time(s), want 2: the seed and one refresh on the kid miss", n)
	}
}

// TestJWKSConcurrentLookupsCoalesce checks that concurrent lookups on a cold
// cache, with and without a kid, share one JWKS fetch and all succeed.
// REQ-062
func TestJWKSConcurrentLookupsCoalesce(t *testing.T) {
	body := keySetBody(t, publishedKey{key: newRSAKey(t), kid: "kid-1", use: "sig"})
	synctest.Test(t, func(t *testing.T) {
		var fetches atomic.Int32
		gate := make(chan struct{})
		srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fetches.Add(1)
			<-gate
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		// Client() starts the in-memory server and sets srv.URL.
		cli := srv.Client()
		jwks, err := smart.NewJWKS(cli, srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		for i := range 8 {
			kid := ""
			if i%2 == 1 {
				kid = "kid-1"
			}
			wg.Go(func() {
				if _, err := jwks.Key(t.Context(), kid); err != nil {
					t.Errorf("Key(%q) error = %v, want nil", kid, err)
				}
			})
		}
		synctest.Wait()
		close(gate)
		wg.Wait()
		if n := fetches.Load(); n != 1 {
			t.Errorf("concurrent lookups fetched the JWKS %d time(s), want 1", n)
		}
	})
}

// TestJWKSKeyWithoutKidReusesCachedSet checks that a second lookup without a
// kid inside the TTL reads the cached set instead of fetching it again.
// REQ-062
func TestJWKSKeyWithoutKidReusesCachedSet(t *testing.T) {
	jwks, fetches := stubJWKSServer(t, http.StatusOK, keySetBody(t, publishedKey{key: newRSAKey(t)}))

	for i := range 2 {
		if _, err := jwks.Key(t.Context(), ""); err != nil {
			t.Fatalf("Key(no kid) call %d error = %v, want nil", i+1, err)
		}
	}
	// REQ-062: the set is fetched on first use and cached.
	if n := fetches.Load(); n != 1 {
		t.Fatalf("two lookups without a kid fetched the JWKS %d time(s), want 1", n)
	}
}

// TestJWKSKeyWithoutKidRefetchesAfterTTL checks that a lookup without a kid
// fetches the set again once it is TTL old, and not a second earlier. REQ-062
func TestJWKSKeyWithoutKidRefetchesAfterTTL(t *testing.T) {
	body := keySetBody(t, publishedKey{key: newRSAKey(t)})
	synctest.Test(t, func(t *testing.T) {
		var fetches atomic.Int32
		srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fetches.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		// Client() starts the in-memory server and sets srv.URL.
		cli := srv.Client()
		jwks, err := smart.NewJWKS(cli, srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		jwks.TTL = time.Minute

		lookup := func(after string, want int32) {
			t.Helper()
			if _, err := jwks.Key(t.Context(), ""); err != nil {
				t.Fatalf("Key(no kid) %s error = %v, want nil", after, err)
			}
			// REQ-062: the cache honours the TTL.
			if n := fetches.Load(); n != want {
				t.Fatalf("Key(no kid) %s: JWKS fetched %d time(s), want %d", after, n, want)
			}
		}
		lookup("on first use", 1)
		time.Sleep(time.Minute - time.Second)
		lookup("a second before the TTL ends", 1)
		time.Sleep(time.Second)
		lookup("once the set is TTL old", 2)
	})
}

// rotatingJWKS serves before until rotate is called and after from then on,
// and reports how many times the set was fetched. Its TTL is an hour, so only
// a forced refresh fetches the set again.
func rotatingJWKS(t *testing.T, before, after []byte) (jwks *smart.JWKS, fetches *atomic.Int32, rotate func()) {
	t.Helper()
	var rotated atomic.Bool
	fetches = new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if rotated.Load() {
			_, _ = w.Write(after)
			return
		}
		_, _ = w.Write(before)
	}))
	t.Cleanup(srv.Close)
	jwks, err := smart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	jwks.TTL = time.Hour
	return jwks, fetches, func() { rotated.Store(true) }
}

// TestValidateIDTokenWithoutKidFollowsRotation checks that a token without a
// kid, signed by a key the server rotated to after the set was cached, is
// accepted after exactly one refetch, while a token the cached key verifies
// causes none. REQ-062
func TestValidateIDTokenWithoutKidFollowsRotation(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	keyA, keyB := newRSAKey(t), newRSAKey(t)
	jwks, fetches, rotate := rotatingJWKS(t, keySetBody(t, publishedKey{key: keyA}), keySetBody(t, publishedKey{key: keyB}))
	validate := func(signer *rsa.PrivateKey) error {
		_, err := smart.ValidateIDToken(t.Context(), joseSign(t, gojose.RS256, signer, "", defaultIDClaims(now)), jwks,
			"https://issuer.example", "client-id", "nonce-xyz", now, nil)
		return err
	}

	if err := validate(keyA); err != nil {
		t.Fatalf("ValidateIDToken(no kid, signed by the cached key) error = %v, want nil", err)
	}
	if n := fetches.Load(); n != 1 {
		t.Fatalf("a token the cached key verifies: JWKS fetched %d time(s), want 1", n)
	}

	rotate()
	// REQ-062: a token without kid that the cached key does not verify refreshes the set once.
	if err := validate(keyB); err != nil {
		t.Fatalf("ValidateIDToken(no kid, signed by the rotated key) error = %v, want nil", err)
	}
	if n := fetches.Load(); n != 2 {
		t.Fatalf("a token signed by the rotated key: JWKS fetched %d time(s), want 2: the first fetch and one refresh", n)
	}
}

// TestValidateIDTokenWithoutKidBadSignatureRefreshesOnce checks that a token
// without a kid whose signature no published key verifies is refused after one
// refetch, not a loop, and that neither a claim failure on a good signature nor
// a bad signature under a kid the set holds refetches the set. REQ-062
func TestValidateIDTokenWithoutKidBadSignatureRefreshesOnce(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	served, other := newRSAKey(t), newRSAKey(t)
	set := keySetBody(t, publishedKey{key: served, kid: "kid-a"})
	jwks, fetches, _ := rotatingJWKS(t, set, set)

	steps := []struct {
		name    string
		signer  *rsa.PrivateKey
		kid     string
		claims  map[string]any
		wantErr bool
		fetches int32
	}{
		{name: "no kid, signed by the served key", signer: served, fetches: 1},
		{name: "no kid, signed by an unpublished key", signer: other, wantErr: true, fetches: 2},
		{name: "no kid, good signature, wrong issuer", signer: served, claims: map[string]any{"iss": "https://other.example"}, wantErr: true, fetches: 2},
		{name: "kid the set holds, signed by an unpublished key", signer: other, kid: "kid-a", wantErr: true, fetches: 2},
	}
	for _, st := range steps {
		claims := defaultIDClaims(now)
		maps.Copy(claims, st.claims)
		_, err := smart.ValidateIDToken(t.Context(), joseSign(t, gojose.RS256, st.signer, st.kid, claims), jwks,
			"https://issuer.example", "client-id", "nonce-xyz", now, nil)
		// REQ-062: only a kid-less signature the cached key does not verify refreshes the set, and only once.
		switch {
		case st.wantErr && (err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed)):
			t.Fatalf("ValidateIDToken(%s) error = %v, want ErrJWKSValidationFailed", st.name, err)
		case !st.wantErr && err != nil:
			t.Fatalf("ValidateIDToken(%s) error = %v, want nil", st.name, err)
		}
		if n := fetches.Load(); n != st.fetches {
			t.Fatalf("after ValidateIDToken(%s): JWKS fetched %d time(s) in all, want %d", st.name, n, st.fetches)
		}
	}
}

// TestValidateIDTokenWithoutKidRefetchOutageKeepsItsOwnError checks that when
// the refetch a kid-less signature miss causes fails, the fetch error is
// returned and does not match the JWKS sentinel, so the outage never reads as
// a bad token. REQ-062
func TestValidateIDTokenWithoutKidRefetchOutageKeepsItsOwnError(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cached, rotated := newRSAKey(t), newRSAKey(t)
	set := keySetBody(t, publishedKey{key: cached})
	var down atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(set)
	}))
	t.Cleanup(srv.Close)
	jwks, err := smart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	jwks.TTL = time.Hour
	if _, err := jwks.Key(t.Context(), ""); err != nil {
		t.Fatalf("Key(no kid) error = %v, want nil", err)
	}

	down.Store(true)
	_, err = smart.ValidateIDToken(t.Context(), joseSign(t, gojose.RS256, rotated, "", defaultIDClaims(now)), jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	// REQ-062: a failed refetch surfaces as the fetch error, never as the JWKS sentinel.
	if err == nil || errors.Is(err, auth.ErrJWKSValidationFailed) || !strings.Contains(err.Error(), "jwks fetch: status 503") {
		t.Fatalf("ValidateIDToken(no kid, rotated key, refetch answers 503) error = %v, want the fetch error, not ErrJWKSValidationFailed", err)
	}
}
