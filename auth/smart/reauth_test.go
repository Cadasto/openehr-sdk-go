package smart_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// freshAccess is an access token well outside the refresh window, so only
// a forced refresh replaces it.
func freshAccess(value string) auth.Token {
	return auth.Token{Value: value, Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(time.Hour).Truncate(time.Second)}
}

// TestReauthKeepsTokensWhenTheForcedRefreshFails pins REQ-063 and REQ-064:
// when the refresh Reauth forces fails without being terminal (an ID token
// refused on its content, a key set that cannot be fetched, a server
// error), the source keeps its previous access and refresh tokens.
func TestReauthKeepsTokensWhenTheForcedRefreshFails(t *testing.T) { // REQ-063 REQ-064
	tests := []struct {
		name    string
		setup   func(t *testing.T, p *oidcProvider) discovery.AuthEndpoints
		respond func(t *testing.T, p *oidcProvider)
		want    error
		notWant error
	}{
		{
			name:  "ID token refused on its content",
			setup: func(_ *testing.T, p *oidcProvider) discovery.AuthEndpoints { return p.endpoints() },
			respond: func(t *testing.T, p *oidcProvider) {
				c := refreshClaims("user-1")
				c["aud"] = "other-client"
				p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, c)))
			},
			want: auth.ErrJWKSValidationFailed,
		},
		{
			name: "key set cannot be fetched",
			setup: func(_ *testing.T, p *oidcProvider) discovery.AuthEndpoints {
				ep := p.endpoints()
				ep.JWKSURI = discovery.MustParseURL(p.srv.URL + "/missing-jwks")
				return ep
			},
			respond: func(t *testing.T, p *oidcProvider) {
				p.setBody(tokenBody(t, "at-2", "rt-2", p.sign(t, refreshClaims("user-1"))))
			},
			notWant: auth.ErrJWKSValidationFailed,
		},
		{
			name:  "token endpoint answers 503",
			setup: func(_ *testing.T, p *oidcProvider) discovery.AuthEndpoints { return p.endpoints() },
			respond: func(_ *testing.T, p *oidcProvider) {
				p.setResponse(http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`)
			},
			notWant: auth.ErrJWKSValidationFailed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			src := p.source(t, tc.setup(t, p))
			held := freshAccess("at-1")
			src.SetTokens(held, "rt-1")
			tc.respond(t, p)

			err := src.Reauth(t.Context())
			if !errors.Is(err, auth.ErrRefreshFailed) {
				t.Fatalf("Reauth() error = %v, want auth.ErrRefreshFailed", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("Reauth() error = %v, want it to match %v too", err, tc.want)
			}
			if tc.notWant != nil && errors.Is(err, tc.notWant) {
				t.Errorf("Reauth() error = %v, want it not to match %v", err, tc.notWant)
			}
			if errors.Is(err, auth.ErrReauthRequired) {
				t.Errorf("Reauth() error = %v matches auth.ErrReauthRequired, want a failure that is not terminal", err)
			}
			if n := len(p.tokenForms()); n != 1 {
				t.Errorf("token-endpoint calls = %d, want the 1 refresh Reauth forces", n)
			}
			if access, refresh := src.HeldTokens(); access != held || refresh != "rt-1" {
				t.Errorf("held tokens after the failed forced refresh = %q (expires %v), %q; want the previous %q (expires %v), rt-1",
					access.Value, access.ExpiresAt, refresh, held.Value, held.ExpiresAt)
			}
		})
	}
}

// TestReauthRefreshStaysDueAfterAFailure pins REQ-063: after a forced
// refresh fails without being terminal, the held access token still counts
// as stale, so the next Token call tries the refresh again rather than
// handing out the token the server rejected.
func TestReauthRefreshStaysDueAfterAFailure(t *testing.T) { // REQ-063
	p := newOIDCProvider(t)
	src := p.source(t, p.endpoints())
	src.SetTokens(freshAccess("at-1"), "rt-1")
	p.setResponse(http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`)
	if err := src.Reauth(t.Context()); !errors.Is(err, auth.ErrRefreshFailed) {
		t.Fatalf("Reauth() error = %v, want auth.ErrRefreshFailed", err)
	}

	p.setBody(tokenBody(t, "at-2", "rt-2", ""))
	tok, err := src.Token(t.Context())
	if err != nil || tok.Value != "at-2" {
		t.Fatalf("Token() after the failed forced refresh = %q, %v; want the retried refresh's at-2", tok.Value, err)
	}
	if n := len(p.tokenForms()); n != 2 {
		t.Errorf("token-endpoint calls = %d, want 2", n)
	}
}

// TestNewTokensEndAForcedRefresh pins REQ-063: once a forced refresh has
// failed, tokens from SetTokens or from a new code exchange replace the
// rejected one, and Token hands them out without refreshing.
func TestNewTokensEndAForcedRefresh(t *testing.T) { // REQ-063
	tests := []struct {
		name    string
		replace func(t *testing.T, p *oidcProvider, src *smart.Source)
		calls   int // token-endpoint calls in all, the failed forced refresh included
	}{
		{
			name: "SetTokens",
			replace: func(_ *testing.T, _ *oidcProvider, src *smart.Source) {
				src.SetTokens(freshAccess("at-new"), "rt-new")
			},
			calls: 1,
		},
		{
			name: "code exchange",
			replace: func(t *testing.T, p *oidcProvider, src *smart.Source) {
				req, err := src.BeginAuthorization("")
				if err != nil {
					t.Fatalf("BeginAuthorization: %v", err)
				}
				p.setBody(tokenBody(t, "at-new", "rt-new", ""))
				if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code", req.State, req); err != nil {
					t.Fatalf("ExchangeAuthorizationCode() error = %v, want success", err)
				}
			},
			calls: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			src := p.source(t, p.endpoints())
			src.SetTokens(freshAccess("at-1"), "rt-1")
			p.setResponse(http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`)
			if err := src.Reauth(t.Context()); !errors.Is(err, auth.ErrRefreshFailed) {
				t.Fatalf("Reauth() error = %v, want auth.ErrRefreshFailed", err)
			}
			tc.replace(t, p, src)
			tok, err := src.Token(t.Context())
			if err != nil || tok.Value != "at-new" {
				t.Errorf("Token() = %q, %v; want the new at-new", tok.Value, err)
			}
			if n := len(p.tokenForms()); n != tc.calls {
				t.Errorf("token-endpoint calls = %d, want %d: no refresh of the new tokens", n, tc.calls)
			}
		})
	}
}
