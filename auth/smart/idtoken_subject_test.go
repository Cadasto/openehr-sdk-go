package smart_test

import (
	"errors"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// TestValidateIDTokenRequiresSubject pins REQ-062: OpenID Connect Core 1.0
// §2 requires sub, so a signed token whose sub is missing, empty or not a
// string is refused with auth.ErrJWKSValidationFailed, and a non-empty
// string sub is accepted.
func TestValidateIDTokenRequiresSubject(t *testing.T) { // REQ-062
	now := time.Unix(1_700_000_000, 0)
	priv := newRSAKey(t)
	jwks := jwksServer(t, "kid-sub", &priv.PublicKey, "RS256")
	tests := []struct {
		name    string
		sub     any  // the sub claim; ignored when absent is set
		absent  bool // leave sub out
		wantErr bool
	}{
		{name: "non-empty string", sub: "user-1"},
		{name: "missing", absent: true, wantErr: true},
		{name: "empty string", sub: "", wantErr: true},
		{name: "null", sub: nil, wantErr: true},
		{name: "number", sub: 42, wantErr: true},
		{name: "array", sub: []string{"user-1"}, wantErr: true},
		{name: "object", sub: map[string]any{"id": "user-1"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := defaultIDClaims(now)
			if tc.absent {
				delete(claims, "sub")
			} else {
				claims["sub"] = tc.sub
			}
			raw := joseSign(t, gojose.RS256, priv, "kid-sub", claims)
			got, err := smart.ValidateIDToken(t.Context(), raw, jwks, "https://issuer.example", "client-id", "nonce-xyz", now, nil)
			if !tc.wantErr {
				if err != nil || got == nil || got.Subject != tc.sub {
					t.Fatalf("ValidateIDToken(sub %v) = %+v, %v; want claims with that subject", tc.sub, got, err)
				}
				return
			}
			if !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Errorf("ValidateIDToken(sub %#v, absent %t) error = %v, want auth.ErrJWKSValidationFailed", tc.sub, tc.absent, err)
			}
			if got != nil {
				t.Errorf("ValidateIDToken(sub %#v, absent %t) returned claims %+v, want none", tc.sub, tc.absent, got)
			}
		})
	}
}
