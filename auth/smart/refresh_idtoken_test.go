package smart_test

import (
	"crypto/rsa"
	"errors"
	"net/url"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v4"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// staleAccess is an access token inside the default 30-second refresh
// window, so the next Token call refreshes it.
func staleAccess(value string) auth.Token {
	return auth.Token{Value: value, Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(10 * time.Second)}
}

// exchangeSession completes a launch on src whose ID token names sub, so
// the source holds a verified ID token, then marks the access token stale.
func exchangeSession(t *testing.T, p *oidcProvider, src *smart.Source, sub string) smart.TokenResponse {
	t.Helper()
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	c := idClaims(req.Nonce)
	c["sub"] = sub
	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, c)))
	_, tr, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	if err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
	}
	src.SetTokens(staleAccess("at-1"), "rt-1")
	return tr
}

// refreshClaims returns valid ID-token claims for a refresh response; the
// nonce is one no launch sent, since a refresh does not check it.
func refreshClaims(sub string) map[string]any {
	c := idClaims("not-checked-on-refresh")
	c["sub"] = sub
	return c
}

// TestRefreshVerifiesIDToken pins REQ-064: an ID token in a refresh
// response is verified without a nonce check, and its claims reach
// LastTokenResponse.
func TestRefreshVerifiesIDToken(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	exchangeSession(t, p, src, "user-1")
	p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, refreshClaims("user-1"))))

	tok, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
	if tok.Value != "at-2" {
		t.Errorf("Token() = %q, want at-2", tok.Value)
	}
	c := src.LastTokenResponse().IDTokenClaims
	if c == nil || c.Subject != "user-1" {
		t.Errorf("LastTokenResponse().IDTokenClaims = %+v, want verified claims for user-1", c)
	}
	if access, refresh := src.HeldTokens(); access.Value != "at-2" || refresh != "rt-2" {
		t.Errorf("held tokens = %q, %q; want at-2, rt-2", access.Value, refresh)
	}
}

// TestRefreshRefusesBadIDToken pins REQ-064: a refresh whose ID token fails
// verification, or names another subject or audience than the ID token the
// source verified before (OpenID Connect Core 1.0 §12.2), fails with an
// error matching both auth.ErrRefreshFailed and
// auth.ErrJWKSValidationFailed. The failure is not terminal: the source
// keeps its previous access token, refresh token and token response.
func TestRefreshRefusesBadIDToken(t *testing.T) { // REQ-064
	const platform = "https://platform.example/openehr"
	tests := []struct {
		name   string
		claims map[string]any
		signer func() *rsa.PrivateKey // nil signs with the provider's key
	}{
		{name: "signed by another key", claims: refreshClaims("user-1"), signer: func() *rsa.PrivateKey { return newRSAKey(t) }},
		{name: "issuer differs", claims: func() map[string]any {
			c := refreshClaims("user-1")
			c["iss"] = "https://other-issuer.example"
			return c
		}()},
		{name: "another subject than the session's", claims: refreshClaims("user-2")},
		{name: "another audience than the session's", claims: func() map[string]any {
			// Trusted, so only the comparison with the earlier token refuses it.
			c := refreshClaims("user-1")
			c["aud"] = []string{"client-id", platform}
			return c
		}()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			src := p.source(t, p.endpoints(), smart.WithIDTokenTrustedAudiences(platform))
			exchangeSession(t, p, src, "user-1")
			heldAccess, _ := src.HeldTokens()

			key := p.key
			if tc.signer != nil {
				key = tc.signer()
			}
			p.setBody(tokenBody(t, "at-2", "rt-2", joseSign(t, gojose.RS256, key, oidcKid, tc.claims)))

			tok, err := src.Token(t.Context())
			if !errors.Is(err, auth.ErrRefreshFailed) || !errors.Is(err, auth.ErrJWKSValidationFailed) {
				t.Fatalf("Token() error = %v, want one matching auth.ErrRefreshFailed and auth.ErrJWKSValidationFailed", err)
			}
			if errors.Is(err, auth.ErrReauthRequired) {
				t.Errorf("Token() error = %v matches auth.ErrReauthRequired, want a failure that is not terminal", err)
			}
			if !tok.IsZero() {
				t.Errorf("Token() = %q on failure, want no token", tok.Value)
			}
			if access, refresh := src.HeldTokens(); access != heldAccess || refresh != "rt-1" {
				t.Errorf("held tokens after the refused refresh = %q, %q; want the previous %q, rt-1", access.Value, refresh, heldAccess.Value)
			}
			if last := src.LastTokenResponse(); last.AccessToken != "at-1" || last.IDTokenClaims == nil || last.IDTokenClaims.Subject != "user-1" {
				t.Errorf("LastTokenResponse() = access %q, claims %+v; want the exchange's at-1 and user-1", last.AccessToken, last.IDTokenClaims)
			}
		})
	}
}

// TestRefreshIDTokenWithoutJWKS pins REQ-064: a source without a JWKS fails
// a refresh whose response carries an ID token with an error matching
// auth.ErrRefreshFailed and auth.ErrInvalidConfig, not a bad token, and
// keeps its tokens.
func TestRefreshIDTokenWithoutJWKS(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	ep := p.endpoints()
	ep.JWKSURI = nil
	src := p.source(t, ep)
	src.SetTokens(staleAccess("at-1"), "rt-1")
	p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, refreshClaims("user-1"))))

	_, err := src.Token(t.Context())
	if !errors.Is(err, auth.ErrRefreshFailed) || !errors.Is(err, auth.ErrInvalidConfig) || errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("Token() error = %v, want auth.ErrRefreshFailed and auth.ErrInvalidConfig, not auth.ErrJWKSValidationFailed", err)
	}
	if access, refresh := src.HeldTokens(); access.Value != "at-1" || refresh != "rt-1" {
		t.Errorf("held tokens = %q, %q; want at-1, rt-1", access.Value, refresh)
	}
}

// TestRefreshBindsToFirstVerifiedIDToken pins REQ-064: a source that has
// verified no ID token yet accepts any valid one on refresh, and from then
// on later refreshes must repeat that token's subject.
func TestRefreshBindsToFirstVerifiedIDToken(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	src.SetTokens(staleAccess("at-1"), "rt-1")
	p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, refreshClaims("user-7"))))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("first refresh: Token() error = %v, want success", err)
	}

	src.SetTokens(staleAccess("at-2"), "rt-2")
	p.setBody(tokenBody(t, "at-3", "rt-3", p.sign(t, refreshClaims("user-8"))))
	if _, err := src.Token(t.Context()); !errors.Is(err, auth.ErrJWKSValidationFailed) {
		t.Fatalf("second refresh naming another subject: Token() error = %v, want auth.ErrJWKSValidationFailed", err)
	}
}

// TestExchangeWithoutIDTokenEndsEarlierBinding pins REQ-064: a new code
// exchange starts a new session, so when its response has no ID token the
// subject of the earlier session no longer binds the refresh.
func TestExchangeWithoutIDTokenEndsEarlierBinding(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	exchangeSession(t, p, src, "user-1")

	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	p.setBody(tokenBody(t, "at-5", "rt-5", ""))
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-5", req.State, req); err != nil {
		t.Fatalf("second ExchangeAuthorizationCode() error = %v, want success", err)
	}
	src.SetTokens(staleAccess("at-5"), "rt-5")
	p.setBody(tokenBody(t, "at-6", "rt-6", p.sign(t, refreshClaims("user-2"))))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("refresh after a new session: Token() error = %v, want success", err)
	}
}

// TestRefreshBindingIgnoresCallerChanges pins that changing the claims the
// exchange returned does not change what a refresh is compared with.
func TestRefreshBindingIgnoresCallerChanges(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	tr := exchangeSession(t, p, src, "user-1")
	tr.IDTokenClaims.Subject = "changed-by-caller"
	tr.IDTokenClaims.Audience[0] = "changed-by-caller"

	p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, refreshClaims("user-1"))))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v, want the refresh compared with the claims as verified", err)
	}
}

// TestRefreshKeepsTheRedeemedRefreshToken pins REQ-063 and REQ-064: a
// refresh response without a refresh_token leaves the source holding the
// refresh token it just redeemed, which the server may keep valid.
func TestRefreshKeepsTheRedeemedRefreshToken(t *testing.T) { // REQ-063 REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	src.SetTokens(staleAccess("at-1"), "rt-1")
	p.setBody(tokenBody(t, "at-2", "", ""))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
	if access, refresh := src.HeldTokens(); access.Value != "at-2" || refresh != "rt-1" {
		t.Errorf("held tokens = %q, %q; want at-2 and the redeemed rt-1", access.Value, refresh)
	}
}

// exchangeWithAudience completes a launch on src whose ID token for user-1
// carries aud, then marks the access token stale.
func exchangeWithAudience(t *testing.T, p *oidcProvider, src *smart.Source, aud any) {
	t.Helper()
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	c := idClaims(req.Nonce)
	c["aud"] = aud
	p.setBody(tokenBody(t, "at-1", "rt-1", p.sign(t, c)))
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
	}
	src.SetTokens(staleAccess("at-1"), "rt-1")
}

// TestRefreshComparesAudiencesAsASet pins REQ-064: a refreshed ID token must
// list the same audiences as the session's, in any order and however often
// each is repeated; a different set is refused.
func TestRefreshComparesAudiencesAsASet(t *testing.T) { // REQ-064
	const platform = "https://platform.example/openehr"
	tests := []struct {
		name       string
		sessionAud any
		refreshAud any
		wantErr    bool
	}{
		{name: "same audiences in another order", sessionAud: []string{"client-id", platform}, refreshAud: []string{platform, "client-id"}},
		{name: "an audience repeated on refresh", sessionAud: "client-id", refreshAud: []string{"client-id", "client-id"}},
		{name: "an audience repeated in the session", sessionAud: []string{"client-id", platform, platform}, refreshAud: []string{platform, "client-id"}},
		{name: "an audience added", sessionAud: "client-id", refreshAud: []string{"client-id", platform}, wantErr: true},
		{name: "an audience dropped", sessionAud: []string{"client-id", platform}, refreshAud: "client-id", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			src := p.source(t, p.endpoints(), smart.WithIDTokenTrustedAudiences(platform))
			exchangeWithAudience(t, p, src, tc.sessionAud)
			c := refreshClaims("user-1")
			c["aud"] = tc.refreshAud
			p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, c)))

			_, err := src.Token(t.Context())
			if tc.wantErr {
				if !errors.Is(err, auth.ErrRefreshFailed) || !errors.Is(err, auth.ErrJWKSValidationFailed) {
					t.Errorf("Token() error = %v, want auth.ErrRefreshFailed and auth.ErrJWKSValidationFailed for aud %v after %v", err, tc.refreshAud, tc.sessionAud)
				}
				return
			}
			if err != nil {
				t.Errorf("Token() error = %v, want aud %v accepted after %v", err, tc.refreshAud, tc.sessionAud)
			}
		})
	}
}

// TestRefreshIDTokenKeySetOutage pins REQ-064: when the key set cannot be
// fetched, the refresh fails with auth.ErrRefreshFailed and keeps the fetch
// error reachable, never as a bad token or a configuration error, and the
// source keeps its tokens.
func TestRefreshIDTokenKeySetOutage(t *testing.T) { // REQ-064
	tests := []struct {
		name    string
		jwksURI func(p *oidcProvider) string
		wantURL bool // the fetch error is a *url.Error
	}{
		{name: "key set answers 404", jwksURI: func(p *oidcProvider) string { return p.srv.URL + "/missing-jwks" }},
		{name: "key set unreachable", jwksURI: func(*oidcProvider) string { return unreachableURL(t) }, wantURL: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			ep := p.endpoints()
			ep.JWKSURI = discovery.MustParseURL(tc.jwksURI(p))
			src := p.source(t, ep)
			held := staleAccess("at-1")
			src.SetTokens(held, "rt-1")
			p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, refreshClaims("user-1"))))

			_, err := src.Token(t.Context())
			if !errors.Is(err, auth.ErrRefreshFailed) {
				t.Fatalf("Token() error = %v, want auth.ErrRefreshFailed", err)
			}
			if errors.Is(err, auth.ErrJWKSValidationFailed) || errors.Is(err, auth.ErrInvalidConfig) {
				t.Errorf("Token() error = %v reads as a bad token or a configuration error, want an outage", err)
			}
			if _, ok := errors.AsType[*url.Error](err); ok != tc.wantURL {
				t.Errorf("Token() error = %v; carries a *url.Error = %t, want %t", err, ok, tc.wantURL)
			}
			if access, refresh := src.HeldTokens(); access != held || refresh != "rt-1" {
				t.Errorf("held tokens = %q, %q; want the previous at-1, rt-1", access.Value, refresh)
			}
		})
	}
}

// TestRefreshWithoutIDTokenKeepsTheVerifiedClaims pins REQ-064: a refresh
// response without an id_token keeps the identity the session's verified ID
// token established, so LastTokenResponse still carries those claims; a new
// session without an ID token carries none from the earlier one.
func TestRefreshWithoutIDTokenKeepsTheVerifiedClaims(t *testing.T) { // REQ-064
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	tr := exchangeSession(t, p, src, "user-1")

	p.setBody(tokenBody(t, "at-2", "rt-2", ""))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
	last := src.LastTokenResponse()
	if last.AccessToken != "at-2" {
		t.Errorf("LastTokenResponse().AccessToken = %q, want the refresh's at-2", last.AccessToken)
	}
	if last.IDTokenClaims != tr.IDTokenClaims {
		t.Errorf("LastTokenResponse().IDTokenClaims = %+v, want the exchange's verified claims %+v", last.IDTokenClaims, tr.IDTokenClaims)
	}

	// A second refresh without an ID token still keeps them.
	src.SetTokens(staleAccess("at-2"), "rt-2")
	p.setBody(tokenBody(t, "at-3", "rt-3", ""))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("second Token() error = %v, want a refreshed token", err)
	}
	if got := src.LastTokenResponse().IDTokenClaims; got != tr.IDTokenClaims {
		t.Errorf("after a second refresh, LastTokenResponse().IDTokenClaims = %+v, want the exchange's claims", got)
	}

	// A new session without an ID token has no identity to keep.
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	p.setBody(tokenBody(t, "at-5", "rt-5", ""))
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-5", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
	}
	src.SetTokens(staleAccess("at-5"), "rt-5")
	p.setBody(tokenBody(t, "at-6", "rt-6", ""))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() in the new session error = %v, want a refreshed token", err)
	}
	if got := src.LastTokenResponse().IDTokenClaims; got != nil {
		t.Errorf("new session's LastTokenResponse().IDTokenClaims = %+v, want none from the earlier session", got)
	}
}
