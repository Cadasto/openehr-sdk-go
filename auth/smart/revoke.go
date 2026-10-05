package smart

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// Revoke signs the source out. It clears the access and refresh tokens and
// asks the authorization server to revoke one of them (RFC 7009): the
// refresh token when the source holds one, otherwise the access token.
// After Revoke, [Source.Token] returns [auth.ErrReauthRequired] until new
// tokens are installed.
//
// Signing out also ends the session. Revoke drops the last token response,
// so [Source.LastTokenResponse] returns the zero value, and it drops the
// identity of the last ID token the source verified, so a later refresh is
// not held to that identity.
//
// Revoke clears all of this before it sends the request, so the source is
// signed out whatever the outcome. A refresh still running then has its
// result discarded, and no refresh can start with the token being revoked.
// The [WithTokenChange] hook sees the zero [TokenChange] once, in its place
// among the changes: the moment the tokens were cleared. Revoke itself
// reports it only after the request has been sent or has failed, so a hook
// that panics or blocks cannot stop the request. If another call is already
// reporting changes, or installs new tokens while the request is out, that
// call reports the zero TokenChange in its turn instead, which may be before
// the request has ended or after Revoke has returned.
//
// The request is a form POST to the server's revocation_endpoint carrying
// the token and its token_type_hint, refresh_token or access_token (RFC 7009
// §2.1). It authenticates the client exactly as the token requests do: a
// public client sends its client_id, a client secret goes in an HTTP Basic
// header or, for client_secret_post, in the form, and a private_key_jwt
// client sends a new client assertion. That assertion is built like the
// token endpoint's, so its aud is the token endpoint URL. RFC 7523 §3 asks
// for a value that identifies the authorization server and allows the token
// endpoint URL as that value.
//
// Revoke returns nil when the server answers 200; under RFC 7009 §2.2 the
// server also answers 200 for a token that is unknown or already invalid.
// Any other answer,
// or a request that gets none, is an [*auth.ExchangeError] matching
// [auth.ErrRevocationFailed], with the status code, the RFC 6749 §5.2 error
// when the body holds one, and the cause. When the server advertises no
// revocation endpoint, Revoke sends nothing: it clears the tokens, calls
// the hook and returns an error matching [auth.ErrInvalidConfig]. A source
// that holds no token (no refresh token and no access-token value) sends no
// request, does not call the hook and returns nil, whether or not a
// revocation endpoint is advertised. It still clears whatever is left of its
// access token and drops its last token response and the identity.
func (s *Source) Revoke(ctx context.Context) error {
	s.mu.Lock()
	s.lastTR = TokenResponse{}
	s.idBinding = nil
	token, hint := s.refresh, "refresh_token"
	if token == "" {
		token, hint = s.cur.Value, "access_token"
	}
	if token == "" {
		// Nothing to revoke, but an access token without a value may still
		// carry other fields: clear it, so Token does not hand it out.
		s.setTokensLocked(auth.Token{}, "")
		s.mu.Unlock()
		return nil
	}
	// Clearing advances the session counter, as SetTokens does, so a
	// refresh that is running now does not install its tokens when it ends.
	s.session++
	s.setTokensLocked(auth.Token{}, "")
	// The exchange still running belongs to the session just ended. Drop it
	// so a later Token does not wait out its HTTP call (REQ-167, REQ-063).
	// That exchange clears inflight only when the stored one is still
	// itself, so a newer exchange is left in place.
	s.inflight = nil
	// The change is queued now, so it keeps its place among the changes
	// installed before and after it, but reported only once the request has
	// been sent or has failed: a hook that panics or blocks cannot stop it.
	s.queueChangeLocked(ctx, TokenChange{})
	s.mu.Unlock()
	defer s.deliverChanges()

	endpoint := s.cfg.Auth.RevocationEndpoint
	if endpoint == nil {
		return fmt.Errorf("%w: the authorization server advertises no revocation_endpoint, so the tokens were cleared but not revoked", auth.ErrInvalidConfig)
	}
	form := url.Values{"token": {token}, "token_type_hint": {hint}}
	req, err := s.clientRequest(ctx, endpoint.String(), form)
	if err != nil {
		return &auth.ExchangeError{Sentinel: auth.ErrRevocationFailed, Inner: err}
	}
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return &auth.ExchangeError{Sentinel: auth.ErrRevocationFailed, Inner: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		// RFC 7009 §2.2: the client ignores the body. Reading it lets the
		// connection be reused; a failure to read it changes nothing.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody))
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return &auth.ExchangeError{Sentinel: auth.ErrRevocationFailed, StatusCode: resp.StatusCode, Inner: err}
	}
	return &auth.ExchangeError{
		Sentinel:   auth.ErrRevocationFailed,
		StatusCode: resp.StatusCode,
		OAuth2:     auth.ParseOAuth2Error(body),
		Inner:      fmt.Errorf("revocation endpoint returned %d", resp.StatusCode),
	}
}
