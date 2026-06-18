package authprobes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart"
)

// Probe006JWKSRotationTransparent implements PROBE-006: a signing-key
// rotation on the authorization server triggers exactly one JWKS refresh
// in the SDK, after which the token validates and the caller proceeds —
// no double-refresh, no surfaced validation failure (REQ-062).
//
// Scenario:
//   - An id_token is signed with key "kid-rotated".
//   - The JWKS endpoint first serves an OLD key set (a stale "kid-old"
//     only), then — after one rotation — serves the set containing
//     "kid-rotated". This mirrors silent server-side rotation: the SDK's
//     cached JWKS does not contain the token's kid.
//   - The SDK's JWKS.Key refreshes once on the cache miss; ValidateIDToken
//     then verifies the signature and the claims succeed.
//
// Pass conditions (all must hold):
//  1. ValidateIDToken (via LaunchContextFromTokenResponse) succeeds.
//  2. The JWKS endpoint was fetched exactly twice total: once to seed the
//     cache (stale set), once on the miss-driven refresh (rotated set) —
//     i.e. exactly one refresh beyond the initial fetch, no double-refresh.
func Probe006JWKSRotationTransparent(ctx context.Context) (Result, error) { // PROBE-006 (REQ-062)
	r := Result{Probe: "PROBE-006"}

	const (
		issuer   = "https://probe006.example"
		clientID = "probe006-client"
		nonce    = "nonce-006"
		oldKid   = "kid-old"
		newKid   = "kid-rotated"
	)

	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return r, fmt.Errorf("PROBE-006: generate old key: %w", err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return r, fmt.Errorf("PROBE-006: generate rotated key: %w", err)
	}

	oldSet := mustJWKS(&oldKey.PublicKey, oldKid)
	rotatedSet := mustJWKS(&newKey.PublicKey, newKid)

	var (
		fetches  atomic.Int32
		rotated  atomic.Bool
		jwksBody = func() []byte {
			if rotated.Load() {
				return rotatedSet
			}
			return oldSet
		}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody())
	}))
	defer srv.Close()

	jwks, err := authsmart.NewJWKS(srv.Client(), srv.URL)
	if err != nil {
		return r, fmt.Errorf("PROBE-006: build JWKS: %w", err)
	}
	// TTL long enough that staleness never triggers a refresh — only the
	// kid-miss path can, isolating the rotation behaviour under test.
	jwks.TTL = time.Hour

	// Seed the cache with the OLD (pre-rotation) key set: one fetch.
	if _, err := jwks.Key(ctx, oldKid); err != nil {
		return r, fmt.Errorf("PROBE-006: seed cache: %w", err)
	}

	// The server rotates its signing key; the id_token is signed with the
	// rotated key whose kid is absent from the SDK's cached set.
	rotated.Store(true)
	now := time.Now()
	idToken := signRS256(newKey, newKid, map[string]any{
		"iss":   issuer,
		"sub":   "user-006",
		"aud":   clientID,
		"exp":   now.Add(time.Hour).Unix(),
		"iat":   now.Unix(),
		"nonce": nonce,
	})

	tr := authsmart.TokenResponse{
		AccessToken: "at-006",
		TokenType:   "Bearer",
		IDToken:     idToken,
	}
	lc, err := smart.LaunchContextFromTokenResponse(
		ctx, tr,
		smart.WithJWKS(jwks),
		smart.WithIssuer(issuer),
		smart.WithClientID(clientID),
		smart.WithExpectedNonce(nonce),
		smart.WithValidationTime(now),
	)
	if err != nil {
		if errors.Is(err, auth.ErrJWKSValidationFailed) {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("validation failed across rotation (refresh-on-miss did not recover): %v", err)
			return r, nil
		}
		return r, fmt.Errorf("PROBE-006: unexpected validation error: %w", err)
	}
	if lc == nil || lc.IDToken == nil {
		r.Status = "fail"
		r.Detail = "LaunchContext or IDToken claims nil after rotation"
		return r, nil
	}

	// Exactly two fetches: the initial seed + one refresh on the kid miss.
	if got := fetches.Load(); got != 2 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("JWKS fetched %d times; want exactly 2 (seed + one refresh, no double-refresh)", got)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "kid rotation triggered exactly one JWKS refresh; id_token validated transparently"
	return r, nil
}

// mustJWKS serves a single-key JWKS document advertising pub under kid.
func mustJWKS(pub any, kid string) []byte {
	jwk := gojose.JSONWebKey{Key: pub, KeyID: kid, Algorithm: "RS256", Use: "sig"}
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{jwk}}
	b, err := json.Marshal(set)
	if err != nil {
		panic(fmt.Sprintf("authprobes: marshal JWKS: %v", err))
	}
	return b
}

// signRS256 signs claims with key as an RS256 JWT carrying kid.
func signRS256(key *rsa.PrivateKey, kid string, claims map[string]any) string {
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(fmt.Sprintf("authprobes: marshal claims: %v", err))
	}
	opts := (&gojose.SignerOptions{}).WithType("JWT")
	if kid != "" {
		opts = opts.WithHeader("kid", kid)
	}
	signer, err := gojose.NewSigner(gojose.SigningKey{Algorithm: gojose.RS256, Key: key}, opts)
	if err != nil {
		panic(fmt.Sprintf("authprobes: new signer: %v", err))
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		panic(fmt.Sprintf("authprobes: sign: %v", err))
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		panic(fmt.Sprintf("authprobes: serialize: %v", err))
	}
	return compact
}
