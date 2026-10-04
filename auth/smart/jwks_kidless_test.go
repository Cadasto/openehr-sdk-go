package smart_test

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
