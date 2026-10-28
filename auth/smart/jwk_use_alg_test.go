package smart_test

import (
	"bytes"
	"cmp"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// TestREQ062_KeyNotForSigningVerifiesNoToken pins REQ-062 (auth.md § JWKS
// rotation): a key whose use is present and is not sig verifies no ID token,
// also when the token's kid names it, and the token is refused with
// auth.ErrJWKSValidationFailed. Each token is signed by the key its kid
// names, so it would verify if the SDK used that key.
func TestREQ062_KeyNotForSigningVerifiesNoToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	key := newRSAKey(t)
	cases := []struct {
		name    string
		use     string
		wantErr bool
	}{
		{name: "use sig", use: "sig"},
		{name: "no use", use: ""},
		{name: "use enc", use: "enc", wantErr: true},
		{name: "another use", use: "tls", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jwks, _ := stubJWKSServer(t, http.StatusOK, keySetBody(t, publishedKey{key: key, kid: "kid-1", use: tc.use}))
			tok := joseSign(t, gojose.RS256, key, "kid-1", defaultIDClaims(now))

			_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", "nonce-xyz", now, nil)
			// REQ-062: only a key whose use, if present, is sig verifies an ID token.
			switch {
			case tc.wantErr && !errors.Is(err, auth.ErrJWKSValidationFailed):
				t.Errorf("ValidateIDToken(kid naming a key with use %q) error = %v, want ErrJWKSValidationFailed", tc.use, err)
			case !tc.wantErr && err != nil:
				t.Errorf("ValidateIDToken(kid naming a key with use %q) error = %v, want nil", tc.use, err)
			}
			_, err = jwks.Key(t.Context(), "kid-1")
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("JWKS.Key(kid naming a key with use %q) error = %v, want an error: %t", tc.use, err, tc.wantErr)
			}
		})
	}
}

// TestREQ062_KeyAlgMustBeTheTokenAlg pins REQ-062 (auth.md § JWKS rotation):
// a key that publishes an alg verifies only a token whose header names that
// alg (RFC 8725 §3.1), with or without a kid, and refuses any other with
// auth.ErrJWKSValidationFailed. A key without an alg verifies any supported
// alg its type fits. The RSA key signs every token, so each refused token
// would verify if the SDK ignored the key's alg.
func TestREQ062_KeyAlgMustBeTheTokenAlg(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	key := newRSAKey(t)
	cases := []struct {
		name     string
		keyAlg   string
		tokenAlg gojose.SignatureAlgorithm
		wantErr  bool
	}{
		{name: "same alg", keyAlg: "RS256", tokenAlg: gojose.RS256},
		{name: "no alg", keyAlg: "", tokenAlg: gojose.RS384},
		{name: "another alg", keyAlg: "RS384", tokenAlg: gojose.RS256, wantErr: true},
		{name: "an alg the SDK does not support", keyAlg: "PS256", tokenAlg: gojose.RS256, wantErr: true},
	}
	for _, kid := range []string{"kid-1", ""} {
		for _, tc := range cases {
			t.Run(tc.name+", kid "+cmp.Or(kid, "none"), func(t *testing.T) {
				jwks, _ := stubJWKSServer(t, http.StatusOK, keySetBody(t, publishedKey{key: key, kid: kid, use: "sig", alg: tc.keyAlg}))
				tok := joseSign(t, tc.tokenAlg, key, kid, defaultIDClaims(now))

				_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
					"https://issuer.example", "client-id", "nonce-xyz", now, nil)
				// REQ-062: a key's alg, when present, must be the token's alg.
				switch {
				case tc.wantErr && !errors.Is(err, auth.ErrJWKSValidationFailed):
					t.Errorf("ValidateIDToken(%s token, key alg %q) error = %v, want ErrJWKSValidationFailed", tc.tokenAlg, tc.keyAlg, err)
				case !tc.wantErr && err != nil:
					t.Errorf("ValidateIDToken(%s token, key alg %q) error = %v, want nil", tc.tokenAlg, tc.keyAlg, err)
				}
			})
		}
	}
}

// TestREQ062_MemberNamesAreCaseSensitive pins REQ-062 (auth.md § JWKS
// rotation) against a member whose name differs from alg, kid or use only in
// letter case: JOSE member names are case-sensitive (RFC 7515 §4, RFC 7517
// §4), so such a member is not the one the rule reads. Each extra member
// follows the real one, where a reader that folds case would take it. Each
// token is signed by a key the set holds, so each refused token would verify
// if the SDK read the extra member as the real one.
func TestREQ062_MemberNamesAreCaseSensitive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	key, other := newRSAKey(t), newRSAKey(t)
	cases := []struct {
		name   string
		set    []byte
		header string
	}{
		{
			// The key's alg is RS384 and the token's alg is RS256.
			name:   "an ALG member naming the key's alg",
			set:    keySetBody(t, publishedKey{key: key, kid: "kid-1", use: "sig", alg: "RS384"}),
			header: `{"alg":"RS256","kid":"kid-1","typ":"JWT","ALG":"RS384"}`,
		},
		{
			// A token without a kid needs a set of exactly one signing key.
			name:   "a KID member and no kid, two signing keys",
			set:    keySetBody(t, publishedKey{key: other, kid: "kid-1", use: "sig"}, publishedKey{key: key, kid: "kid-2", use: "sig"}),
			header: `{"alg":"RS256","typ":"JWT","KID":"kid-2"}`,
		},
		{
			name:   "a key with use enc and a USE member of sig",
			set:    withKeyMember(t, keySetBody(t, publishedKey{key: key, kid: "kid-1", use: "enc"}), `"use":"enc"`, `"USE":"sig"`),
			header: `{"alg":"RS256","kid":"kid-1","typ":"JWT"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jwks, _ := stubJWKSServer(t, http.StatusOK, tc.set)
			tok := signRS256(t, key, tc.header, defaultIDClaims(now))

			_, err := smart.ValidateIDToken(t.Context(), tok, jwks,
				"https://issuer.example", "client-id", "nonce-xyz", now, nil)
			if !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Errorf("ValidateIDToken(header %s, set %s) error = %v, want ErrJWKSValidationFailed", tc.header, tc.set, err)
			}
		})
	}
}

// signRS256 signs claims with key as a compact JWS whose protected header is
// header, written as given, so a test controls each member and its order.
func signRS256(t *testing.T, key *rsa.PrivateKey, header string, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// TestREQ062_AKeyWithARepeatedMemberLeavesTheOthers pins that a key whose
// member names repeat is skipped, as RFC 7517 §4 allows, while the set's
// other keys still verify: the first key repeats its use, and a token whose
// kid names the second key verifies.
func TestREQ062_AKeyWithARepeatedMemberLeavesTheOthers(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	key, other := newRSAKey(t), newRSAKey(t)
	set := withKeyMember(t, keySetBody(t,
		publishedKey{key: other, kid: "kid-1", use: "sig"},
		publishedKey{key: key, kid: "kid-2", use: "sig"},
	), `"use":"sig"`, `"use":"sig"`)
	jwks, _ := stubJWKSServer(t, http.StatusOK, set)
	tok := signRS256(t, key, `{"alg":"RS256","kid":"kid-2","typ":"JWT"}`, defaultIDClaims(now))

	if _, err := smart.ValidateIDToken(t.Context(), tok, jwks,
		"https://issuer.example", "client-id", "nonce-xyz", now, nil); err != nil {
		t.Errorf("ValidateIDToken(kid-2, beside a kid-1 key that repeats use) error = %v, want nil", err)
	}
	if _, err := jwks.Key(t.Context(), "kid-1"); err == nil {
		t.Errorf("JWKS.Key(kid-1, a key that repeats use) error = nil, want an error")
	}
}

// withKeyMember adds member to the set body after the first occurrence of
// after, a member of its first key that holds it.
func withKeyMember(t *testing.T, set []byte, after, member string) []byte {
	t.Helper()
	if !bytes.Contains(set, []byte(after)) {
		t.Fatalf("set %s holds no %s to add %s after", set, after, member)
	}
	return bytes.Replace(set, []byte(after), []byte(after+","+member), 1)
}
