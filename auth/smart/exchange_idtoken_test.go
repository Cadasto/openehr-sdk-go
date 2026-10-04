package smart_test

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

const (
	oidcIssuer = "https://issuer.example"
	oidcKid    = "kid-1"
)

// oidcProvider is a stub authorization server that publishes one RSA
// signing key at /jwks and answers /token with a body the test sets.
type oidcProvider struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	mu    sync.Mutex
	body  string
	forms []url.Values
}

func newOIDCProvider(t *testing.T) *oidcProvider {
	t.Helper()
	p := &oidcProvider{key: newRSAKey(t)}
	set := gojose.JSONWebKeySet{Keys: []gojose.JSONWebKey{{Key: &p.key.PublicKey, KeyID: oidcKid, Algorithm: "RS256", Use: "sig"}}}
	jwksBody, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwks":
			_, _ = w.Write(jwksBody)
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p.mu.Lock()
			p.forms = append(p.forms, r.PostForm)
			body := p.body
			p.mu.Unlock()
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

// setBody sets what /token answers from now on.
func (p *oidcProvider) setBody(body string) {
	p.mu.Lock()
	p.body = body
	p.mu.Unlock()
}

// tokenForms returns the forms /token has received.
func (p *oidcProvider) tokenForms() []url.Values {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]url.Values(nil), p.forms...)
}

// endpoints returns the provider's endpoints, including its JWKS.
func (p *oidcProvider) endpoints() discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://issuer.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL(p.srv.URL + "/token"),
		JWKSURI:               discovery.MustParseURL(p.srv.URL + "/jwks"),
	}
}

// source builds an openid source bound to oidcIssuer on ep.
func (p *oidcProvider) source(t *testing.T, ep discovery.AuthEndpoints, opts ...smart.Option) *smart.Source {
	t.Helper()
	src, err := newSource("client-id", ep, append([]smart.Option{
		smart.WithHTTPClient(p.srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithIssuer(oidcIssuer),
		smart.WithScopes("openid", "launch/patient"),
	}, opts...)...)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	return src
}

// sign signs claims with the provider's key.
func (p *oidcProvider) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	return joseSign(t, gojose.RS256, p.key, oidcKid, claims)
}

// idClaims returns valid ID-token claims for client-id from oidcIssuer
// carrying nonce; an empty nonce leaves the claim out.
func idClaims(nonce string) map[string]any {
	now := time.Now()
	c := map[string]any{
		"iss": oidcIssuer,
		"sub": "user-1",
		"aud": "client-id",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	if nonce != "" {
		c["nonce"] = nonce
	}
	return c
}

// tokenBody is a token-endpoint success body; an empty idToken leaves the
// member out.
func tokenBody(t *testing.T, access, refresh, idToken string) string {
	t.Helper()
	m := map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": 3600, "refresh_token": refresh}
	if idToken != "" {
		m["id_token"] = idToken
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestExchangeVerifiesIDToken pins REQ-064: an ID token in the code
// exchange's response is verified before the call returns, and its claims
// are on TokenResponse.IDTokenClaims, also as LastTokenResponse reports it.
func TestExchangeVerifiesIDToken(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, idClaims(req.Nonce))))

	_, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
	}
	c := tr.IDTokenClaims
	if c == nil {
		t.Fatal("TokenResponse.IDTokenClaims = nil, want the verified claims")
	}
	if c.Subject != "user-1" || c.Issuer != oidcIssuer || c.Nonce != req.Nonce || !slices.Equal(c.Audience, []string{"client-id"}) {
		t.Errorf("IDTokenClaims = %+v, want sub user-1, iss %s, aud [client-id], nonce %q", c, oidcIssuer, req.Nonce)
	}
	if last := src.LastTokenResponse(); last.IDTokenClaims != c {
		t.Errorf("LastTokenResponse().IDTokenClaims = %p, want the returned claims %p", last.IDTokenClaims, c)
	}
	if access, refresh := src.HeldTokens(); access.Value != "at-1" || refresh != "rt-1" {
		t.Errorf("held tokens = %q, %q; want at-1, rt-1", access.Value, refresh)
	}
}

// TestExchangeRefusesBadIDToken pins REQ-064: an ID token that fails
// verification (nonce, issuer, audience, signature, or an algorithm the
// server does not advertise) fails the exchange with an error matching
// auth.ErrJWKSValidationFailed. Nothing unverified is returned, and the
// source keeps the tokens and token response of the session it had.
func TestExchangeRefusesBadIDToken(t *testing.T) { // REQ-064
	tests := []struct {
		name   string
		claims func(nonce string) map[string]any
		signer func(p *oidcProvider) *rsa.PrivateKey // nil signs with the provider's key
		algs   []string                              // the server's id_token_signing_alg_values_supported
	}{
		{name: "nonce differs", claims: func(string) map[string]any { return idClaims("another-launch") }},
		{name: "nonce missing", claims: func(string) map[string]any { return idClaims("") }},
		{name: "issuer differs", claims: func(n string) map[string]any {
			c := idClaims(n)
			c["iss"] = "https://other-issuer.example"
			return c
		}},
		{name: "audience is another client", claims: func(n string) map[string]any {
			c := idClaims(n)
			c["aud"] = "other-client"
			return c
		}},
		{name: "audience lists an untrusted extra", claims: func(n string) map[string]any {
			c := idClaims(n)
			c["aud"] = []string{"client-id", "https://platform.example/openehr"}
			return c
		}},
		{name: "signed by another key", claims: idClaims, signer: func(*oidcProvider) *rsa.PrivateKey { return newRSAKey(t) }},
		{name: "algorithm the server does not advertise", claims: idClaims, algs: []string{"ES384"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			ep := p.endpoints()
			ep.IDTokenSigningAlgValuesSupported = tc.algs
			src := p.source(t, ep)

			// An earlier launch leaves a verified session on the source.
			first, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			goodClaims := idClaims(first.Nonce)
			goodToken := p.sign(t, goodClaims)
			if tc.algs != nil {
				// The allowlist would refuse the RS256 session too; seed it
				// without an ID token instead.
				goodToken = ""
			}
			p.setBody(tokenBody(t, "at-1", "rt-1", goodToken))
			if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", first.State, first); err != nil {
				t.Fatalf("first ExchangeAuthorizationCode() error = %v, want success", err)
			}

			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			key := p.key
			if tc.signer != nil {
				key = tc.signer(p)
			}
			bad := joseSign(t, gojose.RS256, key, oidcKid, tc.claims(req.Nonce))
			p.setBody(tokenBody(t, "at-2", "rt-2", bad))

			tok, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-2", req.State, req)
			if !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("ExchangeAuthorizationCode() error = %v, want auth.ErrJWKSValidationFailed", err)
			}
			if !tok.IsZero() || tr.AccessToken != "" || tr.IDToken != "" || tr.IDTokenClaims != nil {
				t.Errorf("ExchangeAuthorizationCode() returned token %q, access_token %q, an ID token: %t, claims %v on failure; want zero values",
					tok.Value, tr.AccessToken, tr.IDToken != "", tr.IDTokenClaims)
			}
			if access, refresh := src.HeldTokens(); access.Value != "at-1" || refresh != "rt-1" {
				t.Errorf("held tokens after the refused exchange = %q, %q; want the earlier at-1, rt-1", access.Value, refresh)
			}
			if last := src.LastTokenResponse(); last.AccessToken != "at-1" {
				t.Errorf("LastTokenResponse().AccessToken = %q, want the earlier at-1", last.AccessToken)
			}
		})
	}
}

// TestExchangeIDTokenTrustedAudiences pins REQ-064: an extra audience named
// by WithIDTokenTrustedAudiences is accepted at the exchange.
func TestExchangeIDTokenTrustedAudiences(t *testing.T) { // REQ-064
	const platform = "https://platform.example/openehr"
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints(), smart.WithIDTokenTrustedAudiences(platform))
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	c := idClaims(req.Nonce)
	c["aud"] = []string{"client-id", platform}
	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, c)))

	_, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success with %s trusted", err, platform)
	}
	if tr.IDTokenClaims == nil || !slices.Equal(tr.IDTokenClaims.Audience, []string{"client-id", platform}) {
		t.Errorf("IDTokenClaims = %+v, want aud [client-id %s]", tr.IDTokenClaims, platform)
	}
}

// TestWithIDTokenTrustedAudiencesKeepsItsOwnCopy pins that changing the
// caller's slice after building the option does not change what the
// source trusts.
func TestWithIDTokenTrustedAudiencesKeepsItsOwnCopy(t *testing.T) { // REQ-064
	trusted := []string{"https://platform.example/openehr"}
	opt := smart.WithIDTokenTrustedAudiences(trusted...)
	trusted[0] = "https://attacker.example"

	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints(), opt)
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	c := idClaims(req.Nonce)
	c["aud"] = []string{"client-id", "https://attacker.example"}
	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, c)))
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want auth.ErrJWKSValidationFailed for an audience trusted only after the option was built", err)
	}
}

// TestExchangeIDTokenWithoutJWKS pins REQ-064: a source without a JWKS
// fails an exchange whose response carries an ID token with
// auth.ErrInvalidConfig, not as a bad token, and returns and stores
// nothing.
func TestExchangeIDTokenWithoutJWKS(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	ep := p.endpoints()
	ep.JWKSURI = nil
	src := p.source(t, ep)
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, idClaims(req.Nonce))))

	tok, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	if !errors.Is(err, auth.ErrInvalidConfig) || errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want auth.ErrInvalidConfig and not auth.ErrJWKSValidationFailed", err)
	}
	if !tok.IsZero() || tr.IDToken != "" || tr.IDTokenClaims != nil {
		t.Errorf("ExchangeAuthorizationCode() returned token %q, an ID token: %t, claims %v; want zero values", tok.Value, tr.IDToken != "", tr.IDTokenClaims)
	}
	if access, refresh := src.HeldTokens(); !access.IsZero() || refresh != "" {
		t.Errorf("held tokens = %q, %q; want none", access.Value, refresh)
	}
}

// TestCompleteAuthorizationVerifiesIDToken pins REQ-061 and REQ-064:
// CompleteAuthorization exchanges the code as ExchangeAuthorizationCode
// does, so it returns the verified claims and refuses an ID token whose
// nonce is not the launch's.
func TestCompleteAuthorizationVerifiesIDToken(t *testing.T) { // REQ-061 REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	callback := url.Values{"state": {req.State}, "code": {"code-1"}, "iss": {oidcIssuer}}

	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, idClaims("another-launch"))))
	if _, _, err := src.CompleteAuthorization(t.Context(), callback, req); !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("CompleteAuthorization() with a foreign nonce: error = %v, want auth.ErrJWKSValidationFailed", err)
	}

	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, idClaims(req.Nonce))))
	_, tr, err := src.CompleteAuthorization(t.Context(), callback, req)
	if err != nil {
		t.Fatalf("CompleteAuthorization() error = %v, want success", err)
	}
	if tr.IDTokenClaims == nil || tr.IDTokenClaims.Nonce != req.Nonce {
		t.Errorf("IDTokenClaims = %+v, want verified claims with nonce %q", tr.IDTokenClaims, req.Nonce)
	}
	if n := len(p.tokenForms()); n != 2 {
		t.Errorf("token-endpoint calls = %d, want 2", n)
	}
}
