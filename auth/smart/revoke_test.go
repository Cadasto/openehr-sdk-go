package smart_test

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// revokeFixture is a source on a stub server whose token-change hook
// records each change with the tokens the source held when it ran, and
// which records what the source holds while each revocation request is on
// its way.
type revokeFixture struct {
	as  *stubServer
	src *smart.Source

	mu      sync.Mutex
	changes []smart.TokenChange
	held    [][2]string // access and refresh token held when the hook ran
	atPost  []heldState // what the source held while a revocation request was sent
}

// heldState is what a source holds at one moment.
type heldState struct {
	access, refresh string
	last            smart.TokenResponse
}

// observingTransport calls observe for each request to the revocation
// endpoint, in the goroutine sending it, before passing it on to next.
type observingTransport struct {
	next    http.RoundTripper
	observe func()
}

func (o observingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/revoke" {
		o.observe()
	}
	return o.next.RoundTrip(r)
}

// newRevokeFixture builds the fixture; edit, when not nil, changes the
// server's endpoints first.
func newRevokeFixture(t *testing.T, edit func(*discovery.AuthEndpoints), opts ...smart.Option) *revokeFixture {
	t.Helper()
	f := &revokeFixture{as: newStubServer(t)}
	ep := f.as.endpoints()
	if edit != nil {
		edit(&ep)
	}
	client := &http.Client{Transport: observingTransport{next: f.as.srv.Client().Transport, observe: f.observe}}
	f.src = f.as.source(t, ep, append([]smart.Option{smart.WithTokenChange(f.record), smart.WithHTTPClient(client)}, opts...)...)
	return f
}

func (f *revokeFixture) record(_ context.Context, c smart.TokenChange) {
	access, refresh := f.src.HeldTokens()
	f.mu.Lock()
	f.changes = append(f.changes, c)
	f.held = append(f.held, [2]string{access.Value, refresh})
	f.mu.Unlock()
}

func (f *revokeFixture) observe() {
	access, refresh := f.src.HeldTokens()
	last := f.src.LastTokenResponse()
	f.mu.Lock()
	f.atPost = append(f.atPost, heldState{access: access.Value, refresh: refresh, last: last})
	f.mu.Unlock()
}

func (f *revokeFixture) changeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.changes)
}

// signIn completes a code exchange whose response carries launch context,
// so the source has a last token response, then makes the source hold
// access and refresh. It forgets the exchange's token change.
func (f *revokeFixture) signIn(t *testing.T, access auth.Token, refresh string) {
	t.Helper()
	f.as.answerToken(0, launchBody(t, "at-0", map[string]any{"patient": "P1", "scope": "openid launch/patient"}, map[string]any{"refresh_token": "rt-0"}))
	req, err := f.src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if _, _, err := f.src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	if f.src.LastTokenResponse().Patient != "P1" {
		t.Fatal("sign-in left no last token response")
	}
	f.src.SetTokens(access, refresh)
	f.mu.Lock()
	f.changes, f.held = nil, nil
	f.mu.Unlock()
}

// revokeWithin calls src.Revoke(ctx) and fails the test at once if it does
// not return, which here means it deadlocked against the source.
func revokeWithin(t *testing.T, ctx context.Context, src *smart.Source) error {
	t.Helper()
	var err error
	withinDeadline(t, "Revoke", func() { err = src.Revoke(ctx) })
	return err
}

// checkSignedOut reports an error unless the source holds no token and no
// last token response, Token asks for re-authentication, the hook ran once,
// with the zero TokenChange, after the tokens were cleared, and every
// revocation request was sent after the tokens and the last token response
// were dropped.
func (f *revokeFixture) checkSignedOut(t *testing.T) {
	t.Helper()
	if access, refresh := f.src.HeldTokens(); !access.IsZero() || refresh != "" {
		t.Errorf("after Revoke the source holds %+v, %q; want no token", access, refresh)
	}
	if last := f.src.LastTokenResponse(); !reflect.DeepEqual(last, smart.TokenResponse{}) {
		t.Errorf("LastTokenResponse() after Revoke = %+v, want the zero value", last)
	}
	if tok, err := f.src.Token(t.Context()); !errors.Is(err, auth.ErrReauthRequired) {
		t.Errorf("Token() after Revoke = %q, %v; want ErrReauthRequired", tok.Value, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(f.changes) != 1:
		t.Errorf("token changes = %d, want one for Revoke", len(f.changes))
	case !reflect.DeepEqual(f.changes[0], smart.TokenChange{}):
		t.Errorf("Revoke's token change = %+v, want the zero TokenChange", f.changes[0])
	case f.held[0] != [2]string{}:
		t.Errorf("the hook ran while the source held %q, want it to run after the tokens were cleared", f.held[0])
	}
	for _, st := range f.atPost {
		if st.access != "" || st.refresh != "" || !reflect.DeepEqual(st.last, smart.TokenResponse{}) {
			t.Errorf("while the revocation request was sent the source held %q, %q and last token response %+v; want them dropped before it is sent",
				st.access, st.refresh, st.last)
		}
	}
}

// TestRevokeSendsTheTokenWithItsHint pins REQ-167: Revoke posts the refresh
// token with token_type_hint refresh_token when the source holds one, and
// otherwise the access token with access_token, form-encoded; a public
// client also sends its client_id. The tokens and the last token response
// are dropped before the request is sent.
func TestRevokeSendsTheTokenWithItsHint(t *testing.T) { // REQ-167
	tests := []struct {
		name      string
		access    auth.Token
		refresh   string
		wantToken string
		wantHint  string
	}{
		{name: "access and refresh token", access: freshAccess("at-1"), refresh: "rt-1", wantToken: "rt-1", wantHint: "refresh_token"},
		{name: "refresh token only", refresh: "rt-1", wantToken: "rt-1", wantHint: "refresh_token"},
		{name: "access token only", access: freshAccess("at-1"), wantToken: "at-1", wantHint: "access_token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newRevokeFixture(t, nil)
			f.signIn(t, tc.access, tc.refresh)
			tokenReqs := len(f.as.tokenRequests())
			if err := revokeWithin(t, t.Context(), f.src); err != nil {
				t.Fatalf("Revoke() error = %v, want nil on a 200 answer", err)
			}
			reqs := f.as.revokeRequests()
			if len(reqs) != 1 {
				t.Fatalf("revocation requests = %d, want 1", len(reqs))
			}
			form := reqs[0].form
			checkFormField(t, form, "token", tc.wantToken, true)
			checkFormField(t, form, "token_type_hint", tc.wantHint, true)
			checkFormField(t, form, "client_id", "client-id", true)
			if keys := slices.Sorted(maps.Keys(form)); !slices.Equal(keys, []string{"client_id", "token", "token_type_hint"}) {
				t.Errorf("revocation form fields = %q, want [client_id token token_type_hint]", keys)
			}
			if n := len(f.as.tokenRequests()) - tokenReqs; n != 0 {
				t.Errorf("token requests during Revoke = %d, want none", n)
			}
			f.mu.Lock()
			observed := len(f.atPost)
			f.mu.Unlock()
			if observed != 1 {
				t.Errorf("revocation requests observed on their way = %d, want 1", observed)
			}
			f.checkSignedOut(t)
		})
	}
}

// TestRevokeAuthenticatesLikeTheTokenEndpoint pins REQ-167 and REQ-068: the
// revocation request authenticates the client exactly as the refresh did. A
// private_key_jwt client signs a new assertion for it, built like the token
// endpoint's, whose aud is the token endpoint URL.
func TestRevokeAuthenticatesLikeTheTokenEndpoint(t *testing.T) { // REQ-167 REQ-068
	key := newRSAKey(t)
	const secret = "s3cret"
	clients := []struct {
		name    string
		opts    []smart.Option
		methods []string
		// want* describe both requests.
		wantClientID, wantSecret, wantBasic, wantAssertion bool
	}{
		{name: "public", wantClientID: true},
		{name: "client_secret_basic", opts: []smart.Option{smart.WithClientSecret(secret)}, wantBasic: true},
		{
			name: "client_secret_post", opts: []smart.Option{smart.WithClientSecret(secret)}, methods: []string{"client_secret_post"},
			wantClientID: true, wantSecret: true,
		},
		{name: "private_key_jwt", opts: []smart.Option{smart.WithClientAssertionKey(key, "RS384", "kid-1")}, wantAssertion: true},
	}
	for _, cc := range clients {
		t.Run(cc.name, func(t *testing.T) {
			f := newRevokeFixture(t, func(ep *discovery.AuthEndpoints) { ep.TokenEndpointAuthMethodsSupported = cc.methods }, cc.opts...)
			f.as.answerToken(0, tokenBody(t, "at-2", "rt-2", ""))
			f.src.SetTokens(staleAccess("at-1"), "rt-1")
			if _, err := f.src.Token(t.Context()); err != nil {
				t.Fatalf("Token() (refresh) error = %v", err)
			}
			if err := revokeWithin(t, t.Context(), f.src); err != nil {
				t.Fatalf("Revoke() error = %v", err)
			}
			toks, revs := f.as.tokenRequests(), f.as.revokeRequests()
			if len(toks) != 1 || len(revs) != 1 {
				t.Fatalf("token requests = %d, revocation requests = %d; want 1 each", len(toks), len(revs))
			}
			for _, r := range []struct {
				endpoint string
				req      stubRequest
			}{{"token", toks[0]}, {"revocation", revs[0]}} {
				t.Run(r.endpoint, func(t *testing.T) {
					checkFormField(t, r.req.form, "client_id", "client-id", cc.wantClientID)
					checkFormField(t, r.req.form, "client_secret", secret, cc.wantSecret)
					if r.req.basic != cc.wantBasic || (cc.wantBasic && (r.req.user != "client-id" || r.req.pass != secret)) {
						t.Errorf("HTTP Basic = %t %q:%q, want %t client-id:%s", r.req.basic, r.req.user, r.req.pass, cc.wantBasic, secret)
					}
					if got := r.req.form.Get("client_assertion") != ""; got != cc.wantAssertion {
						t.Errorf("client_assertion sent = %t, want %t", got, cc.wantAssertion)
					}
					if cc.wantAssertion {
						checkFormField(t, r.req.form, "client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", true)
					}
				})
			}
			if !cc.wantAssertion {
				return
			}
			tokClaims, revClaims := assertionClaims(t, toks[0].form.Get("client_assertion")), assertionClaims(t, revs[0].form.Get("client_assertion"))
			if revClaims.Jti == "" || revClaims.Jti == tokClaims.Jti {
				t.Errorf("revocation assertion jti = %q, token assertion jti = %q; want a new assertion", revClaims.Jti, tokClaims.Jti)
			}
			wantAud := f.as.srv.URL + "/token"
			if revClaims.Iss != "client-id" || revClaims.Sub != "client-id" || revClaims.Aud != wantAud {
				t.Errorf("revocation assertion iss, sub, aud = %q, %q, %v; want client-id, client-id, %q", revClaims.Iss, revClaims.Sub, revClaims.Aud, wantAud)
			}
		})
	}
}

type assertionClaimSet struct {
	Iss string `json:"iss"`
	Sub string `json:"sub"`
	Aud any    `json:"aud"`
	Jti string `json:"jti"`
}

func assertionClaims(t *testing.T, jwt string) assertionClaimSet {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("client_assertion %q has %d segments, want 3", jwt, len(parts))
	}
	var c assertionClaimSet
	decodeSegment(t, parts[1], &c)
	return c
}

// checkRevocationError reports an error unless err is an *auth.ExchangeError
// matching only auth.ErrRevocationFailed, with status and OAuth2 code
// (empty for none), a cause, and no token in its text.
func checkRevocationError(t *testing.T, err error, status int, code string) {
	t.Helper()
	ex, ok := errors.AsType[*auth.ExchangeError](err)
	if !ok || ex == nil {
		t.Fatalf("Revoke() error = %v (%T), want an *auth.ExchangeError", err, err)
	}
	if !errors.Is(err, auth.ErrRevocationFailed) || !errors.Is(ex.Sentinel, auth.ErrRevocationFailed) {
		t.Errorf("Revoke() error = %v, want its sentinel auth.ErrRevocationFailed", err)
	}
	for _, other := range []error{auth.ErrInvalidConfig, auth.ErrTokenExchangeFailed, auth.ErrRefreshFailed, auth.ErrReauthRequired} {
		if errors.Is(err, other) {
			t.Errorf("Revoke() error = %v also matches %v, want only auth.ErrRevocationFailed", err, other)
		}
	}
	if ex.StatusCode != status {
		t.Errorf("ExchangeError.StatusCode = %d, want %d", ex.StatusCode, status)
	}
	switch {
	case code == "" && ex.OAuth2 != nil:
		t.Errorf("ExchangeError.OAuth2 = %v, want none", ex.OAuth2)
	case code != "" && (ex.OAuth2 == nil || ex.OAuth2.Code != code):
		t.Errorf("ExchangeError.OAuth2 = %v, want code %q", ex.OAuth2, code)
	}
	if ex.Inner == nil {
		t.Error("ExchangeError.Inner = nil, want the cause")
	}
	if msg := err.Error(); !strings.HasPrefix(msg, "auth: token revocation failed") || strings.Contains(msg, "rt-1") || strings.Contains(msg, "at-1") {
		t.Errorf("Revoke() error text = %q, want it to start with the sentinel's and carry no token", msg)
	}
}

// TestRevokeOutcome pins REQ-167 and REQ-063: Revoke returns nil on a 200
// answer and otherwise an *auth.ExchangeError matching
// auth.ErrRevocationFailed, with the status and the RFC 6749 §5.2 error
// when the body has one; whatever the answer, the source is signed out and
// the hook runs once with the zero TokenChange.
func TestRevokeOutcome(t *testing.T) { // REQ-167 REQ-063
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int // 0: Revoke returns nil
		wantCode   string
	}{
		{name: "200", status: http.StatusOK},
		{name: "200 with a body", status: http.StatusOK, body: `{"error":"ignored"}`},
		{
			name: "400 unsupported_token_type", status: http.StatusBadRequest,
			body:       `{"error":"unsupported_token_type","error_description":"refresh tokens cannot be revoked"}`,
			wantStatus: http.StatusBadRequest, wantCode: "unsupported_token_type",
		},
		{
			name: "503 with an error body", status: http.StatusServiceUnavailable, body: `{"error":"temporarily_unavailable"}`,
			wantStatus: http.StatusServiceUnavailable, wantCode: "temporarily_unavailable",
		},
		{name: "503 without an error body", status: http.StatusServiceUnavailable, body: "try later", wantStatus: http.StatusServiceUnavailable},
		{name: "204", status: http.StatusNoContent, wantStatus: http.StatusNoContent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newRevokeFixture(t, nil)
			f.as.answerRevoke(tc.status, tc.body)
			f.signIn(t, freshAccess("at-1"), "rt-1")

			err := revokeWithin(t, t.Context(), f.src)
			if tc.wantStatus == 0 {
				if err != nil {
					t.Errorf("Revoke() error = %v, want nil", err)
				}
			} else {
				checkRevocationError(t, err, tc.wantStatus, tc.wantCode)
			}
			if n := len(f.as.revokeRequests()); n != 1 {
				t.Errorf("revocation requests = %d, want 1", n)
			}
			f.checkSignedOut(t)
		})
	}
}

// TestRevokeTransportFailure pins REQ-167: a revocation request that gets no
// answer, because the endpoint cannot be reached or the context ended,
// fails with auth.ErrRevocationFailed and signs the source out all the same.
func TestRevokeTransportFailure(t *testing.T) { // REQ-167
	t.Run("unreachable endpoint", func(t *testing.T) {
		dead := unreachableURL(t)
		f := newRevokeFixture(t, func(ep *discovery.AuthEndpoints) {
			ep.RevocationEndpoint = discovery.MustParseURL(dead + "/revoke")
		})
		f.signIn(t, freshAccess("at-1"), "rt-1")
		checkRevocationError(t, revokeWithin(t, t.Context(), f.src), 0, "")
		f.checkSignedOut(t)
	})
	t.Run("context ended", func(t *testing.T) {
		f := newRevokeFixture(t, nil)
		f.signIn(t, freshAccess("at-1"), "rt-1")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := revokeWithin(t, ctx, f.src)
		checkRevocationError(t, err, 0, "")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Revoke(ended context) error = %v, want it to match context.Canceled", err)
		}
		if n := len(f.as.revokeRequests()); n != 0 {
			t.Errorf("revocation requests = %d, want none", n)
		}
		f.checkSignedOut(t)
	})
	t.Run("client assertion cannot be signed", func(t *testing.T) {
		f := newRevokeFixture(t, nil, smart.WithClientAssertionKey(newRSAKey(t), "RS384", "kid-1"))
		f.signIn(t, freshAccess("at-1"), "rt-1")
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // the assertion is refused before any request is built
		err := revokeWithin(t, ctx, f.src)
		checkRevocationError(t, err, 0, "")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Revoke(ended context) error = %v, want it to match context.Canceled", err)
		}
		f.mu.Lock()
		sent := len(f.atPost)
		f.mu.Unlock()
		if sent != 0 {
			t.Errorf("revocation requests on their way = %d, want none: the request cannot be built", sent)
		}
		f.checkSignedOut(t)
	})
	t.Run("answer whose body cannot be read", func(t *testing.T) {
		errBody := errors.New("connection reset while reading the body")
		var f *revokeFixture
		f = newRevokeFixture(t, nil, smart.WithHTTPClient(&http.Client{Transport: brokenBodyTransport{
			next: observingTransport{next: http.DefaultTransport, observe: func() { f.observe() }},
			err:  errBody,
		}}))
		f.signIn(t, freshAccess("at-1"), "rt-1")
		err := revokeWithin(t, t.Context(), f.src)
		checkRevocationError(t, err, http.StatusServiceUnavailable, "")
		if !errors.Is(err, errBody) {
			t.Errorf("Revoke() error = %v, want it to keep the body read error", err)
		}
		f.checkSignedOut(t)
	})
}

// brokenBodyTransport answers the revocation endpoint with 503 and a body
// whose read fails with err, after passing the request through next; it
// passes every other request on to next.
type brokenBodyTransport struct {
	next http.RoundTripper
	err  error
}

func (b brokenBodyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := b.next.RoundTrip(r)
	if err != nil || r.URL.Path != "/revoke" {
		return resp, err
	}
	_ = resp.Body.Close()
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(iotest.ErrReader(b.err)),
		Request:    r,
	}, nil
}

// TestRevokeSendsBeforeTheHookRuns pins REQ-063 and REQ-167: Revoke reports
// its token change only after the revocation request has been sent, so a
// hook that blocks or panics cannot stop the request.
func TestRevokeSendsBeforeTheHookRuns(t *testing.T) { // REQ-063 REQ-167
	signedOut := func(c smart.TokenChange) bool { return reflect.DeepEqual(c, smart.TokenChange{}) }

	t.Run("hook that blocks", func(t *testing.T) {
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		f := newRevokeFixture(t, nil, smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
			if signedOut(c) {
				entered <- struct{}{}
				<-release
			}
		}))
		f.signIn(t, freshAccess("at-1"), "rt-1")
		done := make(chan error, 1)
		go func() { done <- f.src.Revoke(t.Context()) }()
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("Revoke did not call the token-change hook")
		}
		if n := len(f.as.revokeRequests()); n != 1 {
			t.Errorf("revocation requests while the hook blocks = %d, want 1: the request goes out before the hook runs", n)
		}
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Revoke() error = %v, want nil", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Revoke did not return after the hook was released")
		}
	})

	t.Run("hook that panics", func(t *testing.T) {
		f := newRevokeFixture(t, nil, smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
			if signedOut(c) {
				panic("hook failed")
			}
		}))
		f.signIn(t, freshAccess("at-1"), "rt-1")
		var recovered any
		withinDeadline(t, "Revoke", func() {
			defer func() { recovered = recover() }()
			_ = f.src.Revoke(t.Context())
		})
		if recovered == nil {
			t.Error("Revoke returned, want the hook's panic to reach its caller")
		}
		if n := len(f.as.revokeRequests()); n != 1 {
			t.Errorf("revocation requests = %d, want 1: a panicking hook does not stop the request", n)
		}
		if access, refresh := f.src.HeldTokens(); !access.IsZero() || refresh != "" {
			t.Errorf("held tokens = %+v, %q; want none", access, refresh)
		}
	})
}

// TestRevokeWithoutEndpoint pins REQ-167: a source whose server advertises
// no revocation endpoint clears its tokens and drops its last token
// response, then fails with auth.ErrInvalidConfig, sending nothing.
func TestRevokeWithoutEndpoint(t *testing.T) { // REQ-167
	f := newRevokeFixture(t, func(ep *discovery.AuthEndpoints) { ep.RevocationEndpoint = nil })
	f.signIn(t, freshAccess("at-1"), "rt-1")
	tokenReqs := len(f.as.tokenRequests())
	err := revokeWithin(t, t.Context(), f.src)
	if !errors.Is(err, auth.ErrInvalidConfig) || errors.Is(err, auth.ErrRevocationFailed) {
		t.Errorf("Revoke() error = %v, want auth.ErrInvalidConfig only", err)
	}
	if n, m := len(f.as.revokeRequests()), len(f.as.tokenRequests())-tokenReqs; n+m != 0 {
		t.Errorf("requests sent = %d, want none", n+m)
	}
	f.checkSignedOut(t)
}

// TestRevokeWithoutToken pins REQ-167: a source that holds no token drops
// its last token response and returns nil from Revoke without sending a
// request or calling the hook, with or without a revocation endpoint. An
// access token without a value counts as no token, and Revoke clears what
// is left of it, so Token asks for re-authentication.
func TestRevokeWithoutToken(t *testing.T) { // REQ-167
	tests := []struct {
		name  string
		edit  func(*discovery.AuthEndpoints)
		setup func(t *testing.T, f *revokeFixture)
	}{
		{name: "never held a token"},
		{name: "no revocation endpoint", edit: func(ep *discovery.AuthEndpoints) { ep.RevocationEndpoint = nil }},
		{
			name:  "a last token response but no token",
			setup: func(t *testing.T, f *revokeFixture) { f.signIn(t, auth.Token{}, "") },
		},
		{
			name:  "a last token response but no token, no revocation endpoint",
			edit:  func(ep *discovery.AuthEndpoints) { ep.RevocationEndpoint = nil },
			setup: func(t *testing.T, f *revokeFixture) { f.signIn(t, auth.Token{}, "") },
		},
		{
			name: "an access token without a value",
			setup: func(t *testing.T, f *revokeFixture) {
				f.signIn(t, auth.Token{Type: auth.TokenTypeBearer, Scope: "openid", ExpiresAt: time.Now().Add(time.Hour)}, "")
			},
		},
		{
			name: "already revoked",
			setup: func(t *testing.T, f *revokeFixture) {
				f.signIn(t, freshAccess("at-1"), "rt-1")
				if err := revokeWithin(t, t.Context(), f.src); err != nil {
					t.Fatalf("first Revoke() error = %v", err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newRevokeFixture(t, tc.edit)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			reqsBefore, changesBefore := len(f.as.revokeRequests()), f.changeCount()
			if err := revokeWithin(t, t.Context(), f.src); err != nil {
				t.Errorf("Revoke() error = %v, want nil", err)
			}
			if last := f.src.LastTokenResponse(); !reflect.DeepEqual(last, smart.TokenResponse{}) {
				t.Errorf("LastTokenResponse() after Revoke = %+v, want the zero value", last)
			}
			if access, refresh := f.src.HeldTokens(); access != (auth.Token{}) || refresh != "" {
				t.Errorf("after Revoke the source holds %+v, %q; want nothing", access, refresh)
			}
			if tok, err := f.src.Token(t.Context()); !errors.Is(err, auth.ErrReauthRequired) {
				t.Errorf("Token() after Revoke = %+v, %v; want auth.ErrReauthRequired", tok, err)
			}
			if n := len(f.as.revokeRequests()) - reqsBefore; n != 0 {
				t.Errorf("revocation requests = %d, want none", n)
			}
			if n := f.changeCount() - changesBefore; n != 0 {
				t.Errorf("token changes = %d, want none", n)
			}
		})
	}
}

// revokingTransport answers the revocation endpoint with 200 and passes
// every other request on to next.
type revokingTransport struct {
	next http.RoundTripper

	mu    sync.Mutex
	forms []url.Values
}

func (rt *revokingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path != "/revoke" {
		return rt.next.RoundTrip(r)
	}
	raw, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, err
	}
	rt.mu.Lock()
	rt.forms = append(rt.forms, form)
	rt.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
}

// TestRevokeDiscardsARefreshInFlight pins REQ-167 and REQ-063: a refresh
// still running when Revoke clears the tokens has its result discarded, so
// the source stays signed out and reports no change for it.
func TestRevokeDiscardsARefreshInFlight(t *testing.T) { // REQ-167 REQ-063
	key := newRSAKey(t)
	synctest.Test(t, func(t *testing.T) {
		h := newHeldRefreshTransport(t, key)
		rt := &revokingTransport{next: h}
		var log changeLog
		src, err := newSource("client-id", discovery.AuthEndpoints{
			AuthorizationEndpoint: discovery.MustParseURL("https://idp.test/authorize"),
			TokenEndpoint:         discovery.MustParseURL("https://idp.test/token"),
			RevocationEndpoint:    discovery.MustParseURL("https://idp.test/revoke"),
		},
			smart.WithHTTPClient(&http.Client{Transport: rt}),
			smart.WithRedirectURI("https://app.example/callback"),
			smart.WithTokenChange(log.record),
		)
		if err != nil {
			t.Fatalf("newSource: %v", err)
		}
		src.SetTokens(staleAccess("A-1"), "rt-A")

		leader := make(chan tokenResult, 1)
		go func() {
			tok, err := src.Token(t.Context())
			leader <- tokenResult{tok, err}
		}()
		<-h.arrived // the refresh is held at the server
		if err := src.Revoke(t.Context()); err != nil {
			t.Fatalf("Revoke() error = %v, want nil", err)
		}
		h.release <- heldResponse{http.StatusOK, `{"access_token":"A-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-A2"}`}

		if r := <-leader; !errors.Is(r.err, auth.ErrReauthRequired) || r.tok.Value != "" {
			t.Errorf("Token() whose refresh Revoke overtook = %q, %v; want ErrReauthRequired", r.tok.Value, r.err)
		}
		if access, refresh := src.HeldTokens(); !access.IsZero() || refresh != "" {
			t.Errorf("held tokens = %+v, %q; want none", access, refresh)
		}
		if _, err := src.Token(t.Context()); !errors.Is(err, auth.ErrReauthRequired) {
			t.Errorf("Token() afterwards error = %v, want ErrReauthRequired", err)
		}
		if last := src.LastTokenResponse(); !reflect.DeepEqual(last, smart.TokenResponse{}) {
			t.Errorf("LastTokenResponse() = %+v, want the zero value: nothing of the discarded refresh is kept", last)
		}
		if n := len(h.refreshes()); n != 1 {
			t.Errorf("refresh grants = %d, want 1", n)
		}
		if changes := log.all(); len(changes) != 1 || !reflect.DeepEqual(changes[0], smart.TokenChange{}) {
			t.Errorf("token changes = %+v, want only Revoke's zero change", changes)
		}
		rt.mu.Lock()
		defer rt.mu.Unlock()
		if len(rt.forms) != 1 || rt.forms[0].Get("token") != "rt-A" || rt.forms[0].Get("token_type_hint") != "refresh_token" {
			t.Errorf("revocation forms = %v, want one revoking rt-A as a refresh_token", rt.forms)
		}
	})
}

// TestRevokeEndsTheSessionIdentity pins REQ-167 and REQ-064: signing out
// also ends the identity of the ID token the source verified, so a refresh
// of tokens imported afterwards may carry an ID token naming another user.
// Without Revoke, the same refresh is refused.
func TestRevokeEndsTheSessionIdentity(t *testing.T) { // REQ-167 REQ-064
	tests := []struct {
		name   string
		revoke bool
		// noToken makes the source hold no token when Revoke is called.
		noToken bool
		wantErr error
	}{
		{name: "after Revoke", revoke: true, wantErr: auth.ErrInvalidConfig},
		{name: "after Revoke on a source holding no token", revoke: true, noToken: true},
		{name: "control without Revoke"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newOIDCProvider(t)
			src := p.source(t, p.endpoints()) // no revocation endpoint
			exchangeSession(t, p, src, "user-1")
			if tc.noToken {
				src.SetTokens(auth.Token{}, "")
			}
			if tc.revoke {
				err := revokeWithin(t, t.Context(), src)
				if (tc.wantErr == nil && err != nil) || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
					t.Fatalf("Revoke() error = %v, want %v", err, tc.wantErr)
				}
			}
			src.SetTokens(staleAccess("at-9"), "rt-9")
			p.setBody(tokenBody(t, "at-10", "rt-10", p.sign(t, refreshClaims("user-2"))))
			tok, err := src.Token(t.Context())
			switch {
			case tc.revoke && (err != nil || tok.Value != "at-10"):
				t.Errorf("Token() = %q, %v; want at-10: Revoke ended user-1's identity", tok.Value, err)
			case !tc.revoke && !errors.Is(err, auth.ErrJWKSValidationFailed):
				t.Errorf("Token() = %q, %v; want the user-2 ID token refused while user-1's identity holds", tok.Value, err)
			}
		})
	}
}
