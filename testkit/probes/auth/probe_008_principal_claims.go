package authprobes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

// Probe008PrincipalClaimsVerbatim implements PROBE-008: when the token
// carries `principal_uid` and `principal_type` claims (REQ-067), the SDK
// surfaces them on LaunchContext.Principal without coercion; missing
// claims surface as nil, never as guessed defaults.
//
// Pass conditions (all must hold):
//  1. With principal_uid="u-123" / principal_type="AGENT", the resulting
//     LaunchContext.Principal == {UID:"u-123", Type:PrincipalTypeAgent}.
//  2. With no principal_* claims, LaunchContext.Principal is nil.
func Probe008PrincipalClaimsVerbatim(ctx context.Context) (Result, error) { // PROBE-008 (REQ-067)
	r := Result{Probe: "PROBE-008"}

	const (
		issuer   = "https://probe008.example"
		clientID = "probe008-client"
		nonce    = "nonce-008"
		kid      = "kid-008"
	)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return r, fmt.Errorf("PROBE-008: generate key: %w", err)
	}
	jwksBody := mustJWKS(&key.PublicKey, kid)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))
	defer srv.Close()

	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		return r, fmt.Errorf("PROBE-008: build JWKS: %w", err)
	}

	now := time.Now()
	baseClaims := func() map[string]any {
		return map[string]any{
			"iss":   issuer,
			"sub":   "user-008",
			"aud":   clientID,
			"exp":   now.Add(time.Hour).Unix(),
			"iat":   now.Unix(),
			"nonce": nonce,
		}
	}

	validate := func(claims map[string]any) (*smart.LaunchContext, error) {
		tr := authsmart.TokenResponse{
			AccessToken: "at-008",
			TokenType:   "Bearer",
			IDToken:     signRS256(key, kid, claims),
		}
		return smart.LaunchContextFromTokenResponse(
			ctx, tr,
			smart.WithJWKS(jwks),
			smart.WithIssuer(issuer),
			smart.WithClientID(clientID),
			smart.WithExpectedNonce(nonce),
			smart.WithValidationTime(now),
		)
	}

	// Case 1: principal claims present — surfaced verbatim.
	withPrincipal := baseClaims()
	withPrincipal["principal_uid"] = "u-123"
	withPrincipal["principal_type"] = "AGENT"
	lc, err := validate(withPrincipal)
	if err != nil {
		return r, fmt.Errorf("PROBE-008: validate (with principal): %w", err)
	}
	if lc.Principal == nil {
		r.Status = "fail"
		r.Detail = "Principal is nil despite principal_uid / principal_type claims"
		return r, nil
	}
	if lc.Principal.UID != "u-123" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Principal.UID = %q; want u-123 (verbatim)", lc.Principal.UID)
		return r, nil
	}
	if lc.Principal.Type != smart.PrincipalTypeAgent {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Principal.Type = %q; want %q (verbatim, no coercion)", lc.Principal.Type, smart.PrincipalTypeAgent)
		return r, nil
	}

	// Case 2: no principal claims — nil, not a guessed default.
	lcNone, err := validate(baseClaims())
	if err != nil {
		return r, fmt.Errorf("PROBE-008: validate (no principal): %w", err)
	}
	if lcNone.Principal != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Principal = %+v for a token with no principal_* claims; want nil (no guessed default)", lcNone.Principal)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "principal_uid / principal_type surfaced verbatim; absent claims surface as nil Principal"
	return r, nil
}
