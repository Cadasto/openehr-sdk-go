package smart_test

import (
	"cmp"
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
