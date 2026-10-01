package smart_test

import (
	"cmp"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

// defaultIDClaims returns a baseline, valid id_token claim set anchored at now.
func defaultIDClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss":      "https://issuer.example",
		"sub":      "user-1",
		"aud":      "client-id",
		"exp":      now.Add(time.Hour).Unix(),
		"iat":      now.Unix(),
		"nonce":    "nonce-xyz",
		"fhirUser": "Practitioner/99",
	}
}

// joseSign signs claims with the given JOSE alg and private key, emitting kid.
func joseSign(t *testing.T, alg gojose.SignatureAlgorithm, key any, kid string, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	opts := (&gojose.SignerOptions{}).WithType("JWT")
	if kid != "" {
		opts = opts.WithHeader("kid", kid)
	}
	signer, err := gojose.NewSigner(gojose.SigningKey{Algorithm: alg, Key: key}, opts)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

// jwksServer serves a JWKS document advertising pub under kid.
func jwksServer(t *testing.T, kid string, pub crypto.PublicKey, alg string) *authsmart.JWKS {
	t.Helper()
	jwks, _ := countingJWKSServer(t, kid, pub, alg)
	return jwks
}

// countingJWKSServer is jwksServer that also reports how many times the JWKS
// document was requested.
func countingJWKSServer(t *testing.T, kid string, pub crypto.PublicKey, alg string) (*authsmart.JWKS, *atomic.Int32) {
	t.Helper()
	jwk := gojose.JSONWebKey{Key: pub, KeyID: kid, Algorithm: alg, Use: "sig"}
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{jwk}}
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return stubJWKSServer(t, http.StatusOK, body)
}

// stubJWKSServer serves body with the given status at the JWKS endpoint and
// reports how many times it was requested.
func stubJWKSServer(t *testing.T, status int, body []byte) (*authsmart.JWKS, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return jwks, &requests
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func newECKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// TestValidateIDTokenRS256 confirms the RS256 baseline still verifies. REQ-062 REQ-064
func TestValidateIDTokenRS256(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("RS256 should verify: %v", err)
	}
	if claims.Subject != "user-1" || claims.FHIRUser != "Practitioner/99" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenRS384 confirms RS384 verifies (alg agility). REQ-062 REQ-064
func TestValidateIDTokenRS384(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs384", &priv.PublicKey, "RS384")
	tok := joseSign(t, gojose.RS384, priv, "kid-rs384", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("RS384 should verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenES384 confirms ES384 (ECDSA P-384) verifies. REQ-062 REQ-064
func TestValidateIDTokenES384(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newECKey(t, elliptic.P384())
	jwks := jwksServer(t, "kid-es384", &priv.PublicKey, "ES384")
	tok := joseSign(t, gojose.ES384, priv, "kid-es384", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("ES384 should verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenES256 confirms ES256 (ECDSA P-256) verifies. REQ-062 REQ-064
func TestValidateIDTokenES256(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newECKey(t, elliptic.P256())
	jwks := jwksServer(t, "kid-es256", &priv.PublicKey, "ES256")
	tok := joseSign(t, gojose.ES256, priv, "kid-es256", defaultIDClaims(now))

	claims, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("ES256 should verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("claims = %#v", claims)
	}
}

// TestValidateIDTokenRespectsDiscoveryAllowlist confirms the discovery
// id_token_signing_alg_values_supported narrows the accepted set: a token
// signed with an alg outside the advertised list is rejected. REQ-062 REQ-064
func TestValidateIDTokenRejectsUnlistedAlg(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newECKey(t, elliptic.P384())
	jwks := jwksServer(t, "kid-es384", &priv.PublicKey, "ES384")
	tok := joseSign(t, gojose.ES384, priv, "kid-es384", defaultIDClaims(now))

	// Allowlist permits only RS256; ES384 must be rejected.
	_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, []string{"RS256"})
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("unlisted alg should be rejected with JWKS failure, got %v", err)
	}
}

// TestValidateIDTokenFailsClosedOnUnsupportedAdvertisedAlgs verifies that when
// the caller passes a non-empty advertised alg set that the SDK supports none
// of (e.g. an AS advertising only HS256), validation FAILS CLOSED rather than
// silently widening back to the default RS/ES set and accepting an RS256 token.
// The refusal comes before the JWKS is fetched, so it holds while the JWKS
// endpoint is down too.
func TestValidateIDTokenFailsClosedOnUnsupportedAdvertisedAlgs(t *testing.T) { // REQ-062
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	endpoints := []struct {
		name string
		jwks func(t *testing.T) (*authsmart.JWKS, *atomic.Int32)
	}{
		{name: "JWKS serves the key", jwks: func(t *testing.T) (*authsmart.JWKS, *atomic.Int32) {
			return countingJWKSServer(t, "kid-rs256", &priv.PublicKey, "RS256")
		}},
		{name: "JWKS answers 503", jwks: func(t *testing.T) (*authsmart.JWKS, *atomic.Int32) {
			return stubJWKSServer(t, http.StatusServiceUnavailable, nil)
		}},
	}
	// The AS advertises only algorithms the SDK does not support. The RS256
	// token would verify against the default set, but the advertised
	// constraint must be honoured.
	allowlists := [][]string{{"HS256", "HS384"}, {"PS256"}}
	for _, ep := range endpoints {
		for _, algs := range allowlists {
			t.Run(ep.name+"/"+strings.Join(algs, ","), func(t *testing.T) {
				jwks, requests := ep.jwks(t)
				_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
					"https://issuer.example", "client-id", "nonce-xyz", now, algs)
				// REQ-062: an empty intersection fails closed with the JWKS sentinel, before any fetch.
				if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
					t.Fatalf("ValidateIDToken(allowlist %v, %s) error = %v, want ErrJWKSValidationFailed", algs, ep.name, err)
				}
				if n := requests.Load(); n != 0 {
					t.Fatalf("ValidateIDToken(allowlist %v, %s) fetched the JWKS %d time(s), want 0: an allowlist that can never verify must be refused before the key lookup", algs, ep.name, n)
				}
			})
		}
	}
}

// TestValidateIDTokenRejectsBadNonce confirms the SDK's nonce check still
// applies after go-oidc signature verification. REQ-064
func TestValidateIDTokenRejectsBadNonce(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "wrong-nonce", now, nil)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("nonce mismatch should be rejected, got %v", err)
	}
}

// TestValidateIDTokenRejectsSymmetricJWK confirms that a JWKS entry whose kty
// is "oct" (symmetric) is rejected with ErrJWKSValidationFailed instead of
// reaching go-oidc and producing a confusing internal error. Fix 1 / REQ-062.
func TestValidateIDTokenRejectsSymmetricJWK(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)

	// Build a JWKS that serves an oct (symmetric) key under the same kid the
	// token header will reference, simulating a compromised/malicious JWKS.
	octKey := []byte("this-is-a-symmetric-hmac-secret!")
	jwk := gojose.JSONWebKey{Key: octKey, KeyID: "kid-oct", Algorithm: "HS256", Use: "sig"}
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{jwk}}
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	octJWKS, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Sign with the real RSA key but reference kid-oct so the JWKS lookup
	// returns the symmetric key entry.
	tok := joseSign(t, gojose.RS256, priv, "kid-oct", defaultIDClaims(now))

	_, gotErr := smart.ValidateIDToken(t.Context(), tok, octJWKS,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if gotErr == nil {
		t.Fatal("expected error for symmetric JWK, got nil")
	}
	if !errors.Is(gotErr, auth.ErrJWKSValidationFailed) {
		t.Fatalf("expected ErrJWKSValidationFailed, got %v", gotErr)
	}
}

// TestValidateIDTokenWithinExpirySkew confirms that a token whose exp is within
// the 30s clockSkew window past expiry is still accepted by claimsFromMap.
// Fix 2 / REQ-062.
func TestValidateIDTokenWithinExpirySkew(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")

	// exp is 10s in the past relative to now — within the 30s skew window.
	claims := defaultIDClaims(now)
	claims["exp"] = now.Add(-10 * time.Second).Unix()
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", claims)

	_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err != nil {
		t.Fatalf("token 10s past exp should be accepted within 30s skew, got: %v", err)
	}
}

// TestValidateIDTokenExpiredBeyondSkew confirms that a token expired well
// beyond the 30s skew window is rejected. Fix 2 / REQ-062.
func TestValidateIDTokenExpiredBeyondSkew(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")

	// exp is 1h in the past — far outside the skew window.
	claims := defaultIDClaims(now)
	claims["exp"] = now.Add(-time.Hour).Unix()
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", claims)

	_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err == nil || !isJWKSFail(err) {
		t.Fatalf("token expired 1h ago should be rejected, got %v", err)
	}
}

// TestValidateIDTokenRejectsAlgNone checks that an unsigned token is refused
// in every letter case of "none", including when the caller's allowlist
// names it. go-oidc would also refuse such a token, so each case also
// requires that the JWKS was never fetched: the SDK's own check refuses the
// token before it looks up a signing key.
func TestValidateIDTokenRejectsAlgNone(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)

	allowlists := []struct {
		name string
		algs []string
	}{
		{name: "no allowlist", algs: nil},
		{name: "allowlist names none", algs: []string{"none", "RS256"}},
	}
	for _, alg := range []string{"none", "NONE", "None"} {
		for _, al := range allowlists {
			t.Run(alg+"/"+al.name, func(t *testing.T) {
				jwks, requests := countingJWKSServer(t, "kid-rs256", &priv.PublicKey, "RS256")
				hdr, err := json.Marshal(map[string]string{"alg": alg, "typ": "JWT", "kid": "kid-rs256"})
				if err != nil {
					t.Fatal(err)
				}
				pl, err := json.Marshal(defaultIDClaims(now))
				if err != nil {
					t.Fatal(err)
				}
				tok := base64.RawURLEncoding.EncodeToString(hdr) + "." +
					base64.RawURLEncoding.EncodeToString(pl) + "."

				_, err = smart.ValidateIDToken(t.Context(), tok, jwks,
					"https://issuer.example", "client-id", "nonce-xyz", now, al.algs)
				// REQ-062: alg none is always rejected, by the SDK itself, with the JWKS sentinel.
				if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
					t.Fatalf("ValidateIDToken(alg %q, allowlist %v) error = %v, want ErrJWKSValidationFailed", alg, al.algs, err)
				}
				if n := requests.Load(); n != 0 {
					t.Fatalf("ValidateIDToken(alg %q, allowlist %v) fetched the JWKS %d time(s), want 0: the SDK must refuse alg none before any key lookup", alg, al.algs, n)
				}
			})
		}
	}
}

// TestValidateIDTokenMalformedTokenMatchesSentinel checks that a token refused
// for its own shape (segments, header, a key id the JWKS does not publish, or
// a key the token selects but that cannot be parsed) reports the JWKS
// validation sentinel, like every other token rejection.
func TestValidateIDTokenMalformedTokenMatchesSentinel(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	unknownKty, _ := stubJWKSServer(t, http.StatusOK, []byte(`{"keys":[{"kid":"k1","kty":"XYZ"}]}`))
	b64 := base64.RawURLEncoding.EncodeToString

	cases := []struct {
		name string
		tok  string
		jwks *authsmart.JWKS // nil means the JWKS serving kid-rs256
	}{
		{name: "empty token", tok: ""},
		{name: "two segments", tok: b64([]byte(`{"alg":"RS256"}`)) + "." + b64([]byte(`{}`))},
		{name: "header not base64url", tok: "!!!." + b64([]byte(`{}`)) + ".sig"},
		{name: "header not JSON", tok: b64([]byte("not json")) + "." + b64([]byte(`{}`)) + ".sig"},
		{name: "kid not in the JWKS", tok: joseSign(t, gojose.RS256, priv, "kid-unknown", defaultIDClaims(now))},
		{name: "kid selects a JWK of unknown kty", tok: joseSign(t, gojose.RS256, priv, "k1", defaultIDClaims(now)), jwks: unknownKty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := cmp.Or(tc.jwks, jwks)
			_, err := smart.ValidateIDToken(t.Context(), tc.tok, set,
				"https://issuer.example", "client-id", "nonce-xyz", now, nil)
			// REQ-062: a rejection decided on the token itself matches the JWKS sentinel.
			if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("ValidateIDToken(%s) error = %v, want ErrJWKSValidationFailed", tc.name, err)
			}
		})
	}
}

// TestValidateIDTokenOutageKeepsItsOwnError checks that a JWKS endpoint that
// fails or cannot be reached reports its own fetch error, so an outage never
// reads as a bad token. The token itself is valid and would verify.
func TestValidateIDTokenOutageKeepsItsOwnError(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	answering := func(status int) func(t *testing.T) *authsmart.JWKS {
		return func(t *testing.T) *authsmart.JWKS {
			jwks, _ := stubJWKSServer(t, status, nil)
			return jwks
		}
	}
	unreachable := func(t *testing.T) *authsmart.JWKS {
		srv := httptest.NewServer(http.NotFoundHandler())
		client, uri := srv.Client(), srv.URL
		srv.Close()
		jwks, err := authsmart.NewJWKS(client, uri)
		if err != nil {
			t.Fatal(err)
		}
		return jwks
	}
	statusError := func(status string) func(error) bool {
		return func(err error) bool { return strings.Contains(err.Error(), "jwks fetch: status "+status) }
	}
	transportError := func(err error) bool {
		ue, ok := errors.AsType[*url.Error](err)
		return ok && ue != nil
	}

	cases := []struct {
		name       string
		jwks       func(t *testing.T) *authsmart.JWKS
		algs       []string
		isFetchErr func(error) bool
	}{
		{name: "JWKS answers 500", jwks: answering(http.StatusInternalServerError), isFetchErr: statusError("500")},
		// The allowlist is checked before the JWKS is fetched; a usable one must not turn the outage into the sentinel.
		{name: "JWKS answers 503 under a supported allowlist", jwks: answering(http.StatusServiceUnavailable), algs: []string{"RS256"}, isFetchErr: statusError("503")},
		{name: "JWKS unreachable", jwks: unreachable, isFetchErr: transportError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := smart.ValidateIDToken(t.Context(), tok, tc.jwks(t),
				"https://issuer.example", "client-id", "nonce-xyz", now, tc.algs)
			// REQ-062: a JWKS fetch failure surfaces as the fetch error and never matches the JWKS sentinel.
			if err == nil || errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("ValidateIDToken(%s) error = %v, want the fetch error, not ErrJWKSValidationFailed", tc.name, err)
			}
			if !tc.isFetchErr(err) {
				t.Fatalf("ValidateIDToken(%s) error = %v, want the fetch error", tc.name, err)
			}
		})
	}
}

// TestValidateIDTokenMissingTrustAnchorIsInvalidConfig checks that a missing
// JWKS, issuer or client ID is reported as a configuration error, not as a bad
// token. The token itself is valid.
func TestValidateIDTokenMissingTrustAnchorIsInvalidConfig(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	cases := []struct {
		name     string
		jwks     *authsmart.JWKS
		issuer   string
		clientID string
	}{
		{name: "no JWKS", issuer: "https://issuer.example", clientID: "client-id"},
		{name: "no issuer", jwks: jwks, clientID: "client-id"},
		{name: "no client ID", jwks: jwks, issuer: "https://issuer.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := smart.ValidateIDToken(t.Context(), tok, tc.jwks, tc.issuer, tc.clientID, "nonce-xyz", now, nil)
			// REQ-062 REQ-064: a missing trust anchor is a configuration error and never matches the JWKS sentinel.
			if err == nil || !errors.Is(err, auth.ErrInvalidConfig) {
				t.Fatalf("ValidateIDToken(%s) error = %v, want ErrInvalidConfig", tc.name, err)
			}
			if errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("ValidateIDToken(%s) error = %v, want it not to match ErrJWKSValidationFailed", tc.name, err)
			}
		})
	}
}

// TestValidateIDTokenAllowlistCannotWiden checks that an allowlist naming an
// algorithm go-jose can verify but the SDK does not support (PS256) does not
// add it to the accepted set, while the supported algorithm it also names
// still verifies.
func TestValidateIDTokenAllowlistCannotWiden(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rsa", &priv.PublicKey, "")
	allowlist := []string{"RS256", "PS256"}

	// Control: the supported member of the allowlist verifies, so the
	// allowlist is not simply rejecting everything.
	rs256 := joseSign(t, gojose.RS256, priv, "kid-rsa", defaultIDClaims(now))
	if _, err := smart.ValidateIDToken(t.Context(), rs256, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, allowlist); err != nil {
		t.Fatalf("ValidateIDToken(RS256, allowlist %v) error = %v, want nil", allowlist, err)
	}

	ps256 := joseSign(t, gojose.PS256, priv, "kid-rsa", defaultIDClaims(now))
	_, err := smart.ValidateIDToken(t.Context(), ps256, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, allowlist)
	// REQ-062: the allowlist narrows the supported set and never widens it.
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("ValidateIDToken(PS256, allowlist %v) error = %v, want ErrJWKSValidationFailed", allowlist, err)
	}
}

// TestValidateIDTokenVerifiesSignatureBeforeClaims sends claims that fail two
// of the SDK's claim checks (expired beyond the skew, wrong nonce). Signed
// with the served key they fail on a claim; signed with another key under the
// same kid they must fail on the signature, with no claim failure reported.
func TestValidateIDTokenVerifiesSignatureBeforeClaims(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	served := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &served.PublicKey, "RS256")

	claims := defaultIDClaims(now)
	claims["exp"] = now.Add(-time.Hour).Unix()
	claims["nonce"] = "other-nonce"
	claimFailure := func(err error) bool {
		msg := err.Error()
		return strings.Contains(msg, "token expired") || strings.Contains(msg, "nonce mismatch")
	}

	// Control: with a valid signature the same claims reach the claim checks
	// and fail there, so the forged case below is not passing vacuously.
	signed := joseSign(t, gojose.RS256, served, "kid-rs256", claims)
	_, err := smart.ValidateIDToken(t.Context(), signed, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	if err == nil || !claimFailure(err) {
		t.Fatalf("ValidateIDToken(validly signed, expired, wrong nonce) error = %v, want a claim failure", err)
	}

	forged := joseSign(t, gojose.RS256, newRSAKey(t), "kid-rs256", claims)
	_, err = smart.ValidateIDToken(t.Context(), forged, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil)
	// REQ-064: the signature is verified before any claim is trusted.
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("ValidateIDToken(badly signed) error = %v, want ErrJWKSValidationFailed", err)
	}
	if claimFailure(err) {
		t.Fatalf("ValidateIDToken(badly signed, expired, wrong nonce) error = %v, want the signature failure: a claim failure means claims were read before the signature was verified", err)
	}
}

// TestValidateIDTokenClaimRules walks the SDK's claim rules at their edges:
// the required exp, the 30-second skew on exp, nbf and iat, the exact iss
// match, the aud membership and the expected nonce.
func TestValidateIDTokenClaimRules(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	at := func(d time.Duration) int64 { return now.Add(d).Unix() }

	cases := []struct {
		name    string
		issuer  string // configured issuer; empty means https://issuer.example
		set     map[string]any
		drop    string
		wantErr bool
	}{
		{name: "exp 29s ago is inside the skew", set: map[string]any{"exp": at(-29 * time.Second)}},
		{name: "exp 30s ago is outside the skew", set: map[string]any{"exp": at(-30 * time.Second)}, wantErr: true},
		// go-oidc skips its own expiry check here, so only the SDK refuses these two.
		{name: "exp absent", drop: "exp", wantErr: true},
		{name: "exp as a numeric string", set: map[string]any{"exp": "1800000000"}, wantErr: true},
		{name: "nbf 30s ahead is inside the skew", set: map[string]any{"nbf": at(30 * time.Second)}},
		{name: "nbf 31s ahead is outside the skew", set: map[string]any{"nbf": at(31 * time.Second)}, wantErr: true},
		{name: "iat 30s ahead is inside the skew", set: map[string]any{"iat": at(30 * time.Second)}},
		{name: "iat 31s ahead is outside the skew", set: map[string]any{"iat": at(31 * time.Second)}, wantErr: true},
		{name: "iss with a trailing slash", set: map[string]any{"iss": "https://issuer.example/"}, wantErr: true},
		{name: "iss in another letter case", set: map[string]any{"iss": "https://ISSUER.example"}, wantErr: true},
		// go-oidc lets this one pair through; only the SDK's exact match refuses it.
		{name: "exact iss accounts.google.com", issuer: "https://accounts.google.com", set: map[string]any{"iss": "https://accounts.google.com"}},
		{name: "scheme-less iss accounts.google.com", issuer: "https://accounts.google.com", set: map[string]any{"iss": "accounts.google.com"}, wantErr: true},
		{name: "aud lists the client among others", set: map[string]any{"aud": []string{"other", "client-id"}}},
		// go-oidc and the SDK both check aud on purpose, so removing either check alone stays green.
		{name: "aud without the client", set: map[string]any{"aud": []string{"other"}}, wantErr: true},
		{name: "nonce absent when one is expected", drop: "nonce", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := defaultIDClaims(now)
			maps.Copy(claims, tc.set)
			if tc.drop != "" {
				delete(claims, tc.drop)
			}
			issuer := tc.issuer
			if issuer == "" {
				issuer = "https://issuer.example"
			}
			tok := joseSign(t, gojose.RS256, priv, "kid-rs256", claims)

			_, err := smart.ValidateIDToken(t.Context(), tok, jwks, issuer, "client-id", "nonce-xyz", now, nil)
			// REQ-064: claim checks with a 30s skew, exact iss, aud membership, expected nonce.
			switch {
			case tc.wantErr && (err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed)):
				t.Fatalf("ValidateIDToken(%s) error = %v, want ErrJWKSValidationFailed", tc.name, err)
			case !tc.wantErr && err != nil:
				t.Fatalf("ValidateIDToken(%s) error = %v, want nil", tc.name, err)
			}
		})
	}
}
