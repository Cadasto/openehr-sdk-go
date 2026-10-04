package smart_test

import (
	"cmp"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
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

	"github.com/coreos/go-oidc/v3/oidc"
	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
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
func jwksServer(t *testing.T, kid string, pub crypto.PublicKey, alg string) *smart.JWKS {
	t.Helper()
	jwks, _ := countingJWKSServer(t, kid, pub, alg)
	return jwks
}

// countingJWKSServer is jwksServer that also reports how many times the JWKS
// document was requested.
func countingJWKSServer(t *testing.T, kid string, pub crypto.PublicKey, alg string) (*smart.JWKS, *atomic.Int32) {
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
func stubJWKSServer(t *testing.T, status int, body []byte) (*smart.JWKS, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	jwks, err := smart.NewJWKS(srv.Client(), srv.URL)
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

// TestValidateIDTokenRejectsUnlistedAlg confirms the discovery
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
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
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
		jwks func(t *testing.T) (*smart.JWKS, *atomic.Int32)
	}{
		{name: "JWKS serves the key", jwks: func(t *testing.T) (*smart.JWKS, *atomic.Int32) {
			return countingJWKSServer(t, "kid-rs256", &priv.PublicKey, "RS256")
		}},
		{name: "JWKS answers 503", jwks: func(t *testing.T) (*smart.JWKS, *atomic.Int32) {
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
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
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
	octJWKS, err := smart.NewJWKS(srv.Client(), srv.URL)
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
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("token expired 1h ago should be rejected, got %v", err)
	}
}

// TestValidateIDTokenRejectsAlgNone checks that an unsigned token is refused
// in every letter case of "none", including when the caller's allowlist
// names it. go-oidc would refuse such a token as well, so each case also
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
		jwks *smart.JWKS // nil means the JWKS serving kid-rs256
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
	kidless := joseSign(t, gojose.RS256, priv, "", defaultIDClaims(now))

	answering := func(status int) func(t *testing.T) *smart.JWKS {
		return func(t *testing.T) *smart.JWKS {
			jwks, _ := stubJWKSServer(t, status, nil)
			return jwks
		}
	}
	unreachable := func(t *testing.T) *smart.JWKS {
		srv := httptest.NewServer(http.NotFoundHandler())
		client, uri := srv.Client(), srv.URL
		srv.Close()
		jwks, err := smart.NewJWKS(client, uri)
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
		jwks       func(t *testing.T) *smart.JWKS
		algs       []string
		noKid      bool // the token's header carries no kid, so the lookup takes the single-key path
		isFetchErr func(error) bool
	}{
		{name: "JWKS answers 500", jwks: answering(http.StatusInternalServerError), isFetchErr: statusError("500")},
		// The allowlist is checked before the JWKS is fetched; a usable one must not turn the outage into the sentinel.
		{name: "JWKS answers 503 under a supported allowlist", jwks: answering(http.StatusServiceUnavailable), algs: []string{"RS256"}, isFetchErr: statusError("503")},
		{name: "JWKS unreachable", jwks: unreachable, isFetchErr: transportError},
		{name: "JWKS answers 503, token without kid", jwks: answering(http.StatusServiceUnavailable), noKid: true, isFetchErr: statusError("503")},
		{name: "JWKS unreachable, token without kid", jwks: unreachable, noKid: true, isFetchErr: transportError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := tok
			if tc.noKid {
				raw = kidless
			}
			_, err := smart.ValidateIDToken(t.Context(), raw, tc.jwks(t),
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
// token, whether the token is valid or empty.
func TestValidateIDTokenMissingTrustAnchorIsInvalidConfig(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", defaultIDClaims(now))

	cases := []struct {
		name       string
		jwks       *smart.JWKS
		issuer     string
		clientID   string
		emptyToken bool
	}{
		{name: "no JWKS", issuer: "https://issuer.example", clientID: "client-id"},
		{name: "no issuer", jwks: jwks, clientID: "client-id"},
		{name: "no client ID", jwks: jwks, issuer: "https://issuer.example"},
		// The configuration is checked before the token, so an empty token cannot hide it.
		{name: "no JWKS and an empty token", issuer: "https://issuer.example", clientID: "client-id", emptyToken: true},
		{name: "no issuer and an empty token", jwks: jwks, clientID: "client-id", emptyToken: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := tok
			if tc.emptyToken {
				raw = ""
			}
			_, err := smart.ValidateIDToken(t.Context(), raw, tc.jwks, tc.issuer, tc.clientID, "nonce-xyz", now, nil)
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

// TestValidateIDTokenSupportsNoOtherAlg checks that RS256, RS384, ES256 and
// ES384 are the only supported algorithms: every other algorithm go-oidc can
// verify is refused, both with no allowlist (the supported set) and with an
// allowlist that names it. REQ-062 REQ-064
func TestValidateIDTokenSupportsNoOtherAlg(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	rsaKey := newRSAKey(t)
	p521 := newECKey(t, elliptic.P521())
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		alg  gojose.SignatureAlgorithm
		priv any
		pub  crypto.PublicKey
	}{
		{alg: gojose.RS512, priv: rsaKey, pub: &rsaKey.PublicKey},
		{alg: gojose.PS256, priv: rsaKey, pub: &rsaKey.PublicKey},
		{alg: gojose.PS384, priv: rsaKey, pub: &rsaKey.PublicKey},
		{alg: gojose.PS512, priv: rsaKey, pub: &rsaKey.PublicKey},
		{alg: gojose.ES512, priv: p521, pub: &p521.PublicKey},
		{alg: gojose.EdDSA, priv: edPriv, pub: edPub},
	}
	for _, tc := range cases {
		t.Run(string(tc.alg), func(t *testing.T) {
			jwks := jwksServer(t, "kid-other", tc.pub, "")
			tok := joseSign(t, tc.alg, tc.priv, "kid-other", defaultIDClaims(now))

			// Control: go-oidc verifies the token against the served key once
			// the algorithm is allowed, so the refusals below come from the
			// SDK's supported set and not from a broken token or key.
			raw, err := jwks.Key(t.Context(), "kid-other")
			if err != nil {
				t.Fatal(err)
			}
			var served gojose.JSONWebKey
			if err := served.UnmarshalJSON(raw); err != nil {
				t.Fatal(err)
			}
			verifier := oidc.NewVerifier("https://issuer.example",
				&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{served.Key}},
				&oidc.Config{ClientID: "client-id", SupportedSigningAlgs: []string{string(tc.alg)}, Now: func() time.Time { return now }})
			if _, err := verifier.Verify(t.Context(), tok); err != nil {
				t.Fatalf("control: go-oidc Verify(%s token, allowing %s) error = %v, want nil", tc.alg, tc.alg, err)
			}

			_, err = smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", "nonce-xyz", now, nil)
			// REQ-062: no algorithm outside the four is supported.
			if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("ValidateIDToken(%s, no allowlist) error = %v, want ErrJWKSValidationFailed", tc.alg, err)
			}

			allowlist := []string{string(tc.alg)}
			_, err = smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", "nonce-xyz", now, allowlist)
			if err == nil || !strings.Contains(err.Error(), "no supported id_token signing algorithm") {
				t.Fatalf("ValidateIDToken(%s, allowlist %v) error = %v, want the no-supported-algorithm refusal", tc.alg, allowlist, err)
			}
		})
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
		// Another audience is refused unless the caller trusts it; TestValidateIDTokenExtraAudienceAndAzp covers the trusted set.
		{name: "aud lists the client among untrusted others", set: map[string]any{"aud": []string{"other", "client-id"}}, wantErr: true},
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

// TestValidateIDTokenExtraAudienceAndAzp walks the audience rules beyond the
// client ID: every other aud value must be in the caller's trusted set, which
// is empty by default, and an azp claim must name the client. Each refused
// case is otherwise valid, so only these rules refuse it. REQ-062
func TestValidateIDTokenExtraAudienceAndAzp(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")

	cases := []struct {
		name    string
		set     map[string]any
		trusted []string
		wantErr bool
	}{
		{name: "aud is the client alone", set: map[string]any{"aud": "client-id"}},
		{name: "aud lists the client and an untrusted audience", set: map[string]any{"aud": []string{"client-id", "api"}}, wantErr: true},
		{name: "aud lists the client and a trusted audience", set: map[string]any{"aud": []string{"client-id", "api"}}, trusted: []string{"api"}},
		{name: "aud lists a trusted and an untrusted audience", set: map[string]any{"aud": []string{"client-id", "api", "other"}}, trusted: []string{"api"}, wantErr: true},
		{name: "a trusted audience does not stand in for the client", set: map[string]any{"aud": []string{"api"}}, trusted: []string{"api"}, wantErr: true},
		{name: "aud lists the client twice", set: map[string]any{"aud": []string{"client-id", "client-id"}}},
		// go-oidc reads both of these as the client plus an empty audience, so only the SDK's check of every raw entry refuses them.
		{name: "aud lists the client and an empty string", set: map[string]any{"aud": []any{"client-id", ""}}, wantErr: true},
		{name: "aud lists the client and null", set: map[string]any{"aud": []any{"client-id", nil}}, wantErr: true},
		{name: "an empty string is refused even when trusted", set: map[string]any{"aud": []any{"client-id", ""}}, trusted: []string{""}, wantErr: true},
		{name: "azp names the client", set: map[string]any{"azp": "client-id"}},
		{name: "azp names the client beside a trusted audience", set: map[string]any{"aud": []string{"client-id", "api"}, "azp": "client-id"}, trusted: []string{"api"}},
		{name: "azp names another party", set: map[string]any{"azp": "other-client"}, wantErr: true},
		{name: "azp names a trusted audience", set: map[string]any{"aud": []string{"client-id", "api"}, "azp": "api"}, trusted: []string{"api"}, wantErr: true},
		{name: "azp is empty", set: map[string]any{"azp": ""}, wantErr: true},
		{name: "azp is not a string", set: map[string]any{"azp": 42}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := defaultIDClaims(now)
			maps.Copy(claims, tc.set)
			tok := joseSign(t, gojose.RS256, priv, "kid-rs256", claims)

			_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", "nonce-xyz", now, nil,
				smart.WithTrustedAudiences(tc.trusted...))
			// REQ-062: an untrusted extra audience or a foreign azp refuses the token with the JWKS sentinel.
			switch {
			case tc.wantErr && (err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed)):
				t.Fatalf("ValidateIDToken(%s, trusted %q) error = %v, want ErrJWKSValidationFailed", tc.name, tc.trusted, err)
			case !tc.wantErr && err != nil:
				t.Fatalf("ValidateIDToken(%s, trusted %q) error = %v, want nil", tc.name, tc.trusted, err)
			}
		})
	}
}

// TestWithTrustedAudiencesKeepsItsOwnCopy checks that changing the caller's
// slice after building the option does not change the trusted set, and that a
// nil option is ignored. REQ-062
func TestWithTrustedAudiencesKeepsItsOwnCopy(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")
	claims := defaultIDClaims(now)
	claims["aud"] = []string{"client-id", "api"}
	tok := joseSign(t, gojose.RS256, priv, "kid-rs256", claims)

	trusted := []string{"other"}
	opt := smart.WithTrustedAudiences(trusted...)
	trusted[0] = "api"
	_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil, nil, opt)
	// REQ-062: the trusted set is the one given when the option was built.
	if err == nil || !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("ValidateIDToken(aud [client-id api], trusted [other] then edited to [api]) error = %v, want ErrJWKSValidationFailed", err)
	}
}

// TestValidateIDTokenSurfacesOwnNonce checks that IDTokenClaims.Nonce holds the
// token's own nonce claim, whether or not the caller expected one, and is
// empty when the token has none. The claim stays out of Extra. REQ-062
func TestValidateIDTokenSurfacesOwnNonce(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-rs256", &priv.PublicKey, "RS256")

	cases := []struct {
		name     string
		claim    string // the token's nonce claim; empty means the token has none
		expected string // the nonce the caller passes
		want     string
	}{
		{name: "expected and present", claim: "nonce-xyz", expected: "nonce-xyz", want: "nonce-xyz"},
		{name: "present but not expected", claim: "nonce-xyz", want: "nonce-xyz"},
		{name: "neither expected nor present"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := defaultIDClaims(now)
			delete(claims, "nonce")
			if tc.claim != "" {
				claims["nonce"] = tc.claim
			}
			tok := joseSign(t, gojose.RS256, priv, "kid-rs256", claims)

			got, err := smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", tc.expected, now, nil)
			if err != nil {
				t.Fatalf("ValidateIDToken(nonce claim %q, expected %q) error = %v, want nil", tc.claim, tc.expected, err)
			}
			// REQ-062: Nonce is the token's own claim, not the caller's expectation.
			if got.Nonce != tc.want {
				t.Fatalf("ValidateIDToken(nonce claim %q, expected %q) Nonce = %q, want %q", tc.claim, tc.expected, got.Nonce, tc.want)
			}
			if v, ok := got.Extra["nonce"]; ok {
				t.Fatalf("ValidateIDToken(nonce claim %q) Extra[nonce] = %v, want the claim kept out of Extra", tc.claim, v)
			}
		})
	}
}
