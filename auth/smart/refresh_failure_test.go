package smart_test

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// failingTokenEndpoint is an http.RoundTripper standing in for a token
// endpoint whose refresh answers never carry a usable token. It counts the
// refresh requests it receives.
type failingTokenEndpoint struct {
	t         *testing.T
	answer    func(r *http.Request) (*http.Response, error)
	refreshes atomic.Int32
}

func (e *failingTokenEndpoint) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	if got := form.Get("grant_type"); r.URL.Path != "/token" || got != "refresh_token" {
		e.t.Errorf("request to %s with grant_type %q, want a refresh_token grant to /token", r.URL.Path, got)
	}
	e.refreshes.Add(1)
	return e.answer(r)
}

// okResponse is a 200 answer to r whose body reads from body.
func okResponse(r *http.Request, body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(body),
		Request:    r,
	}
}

// TestREQ063_TransientRefreshFailureMatchesErrRefreshFailed checks that a
// refresh that fails without a usable token response, because the request
// is never answered or because a 2xx answer carries no token, is a refresh
// failure and not a token-exchange failure. Such a failure is transient: the
// source keeps its access and refresh tokens. The held access token is past
// its ExpiresAt, so Token returns the failure, and the next Token call tries
// the refresh again instead of asking for re-authentication.
func TestREQ063_TransientRefreshFailureMatchesErrRefreshFailed(t *testing.T) { // REQ-063
	t.Parallel()
	errUnreachable := errors.New("token endpoint unreachable")
	errBodyCut := errors.New("connection reset while reading the body")
	tests := []struct {
		name   string
		answer func(r *http.Request) (*http.Response, error)
		cause  error // nil when the cause is not the test's own error
	}{
		{
			name:   "network failure",
			answer: func(*http.Request) (*http.Response, error) { return nil, errUnreachable },
			cause:  errUnreachable,
		},
		{
			name: "body cannot be read",
			answer: func(r *http.Request) (*http.Response, error) {
				return okResponse(r, iotest.ErrReader(errBodyCut)), nil
			},
			cause: errBodyCut,
		},
		{
			name: "2xx body is not JSON",
			answer: func(r *http.Request) (*http.Response, error) {
				return okResponse(r, strings.NewReader("<html>maintenance</html>")), nil
			},
		},
		{
			name: "2xx body has no access_token",
			answer: func(r *http.Request) (*http.Response, error) {
				return okResponse(r, strings.NewReader(`{"token_type":"Bearer","expires_in":3600}`)), nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			endpoint := &failingTokenEndpoint{t: t, answer: tc.answer}
			src, err := newSource("client-id", discovery.AuthEndpoints{
				AuthorizationEndpoint: discovery.MustParseURL("https://idp.example/authorize"),
				TokenEndpoint:         discovery.MustParseURL("https://idp.example/token"),
			},
				smart.WithHTTPClient(&http.Client{Transport: endpoint}),
				smart.WithRedirectURI("https://app.example/callback"),
			)
			if err != nil {
				t.Fatalf("newSource: %v", err)
			}
			held := expiredAccess("at-0")
			src.SetTokens(held, "rt-0")

			_, err = src.Token(t.Context())

			if !errors.Is(err, auth.ErrRefreshFailed) {
				t.Errorf("Token() error = %v, want it to match auth.ErrRefreshFailed", err)
			}
			if errors.Is(err, auth.ErrTokenExchangeFailed) || errors.Is(err, auth.ErrReauthRequired) {
				t.Errorf("Token() error = %v, want it to match neither auth.ErrTokenExchangeFailed nor auth.ErrReauthRequired", err)
			}
			if ee, ok := errors.AsType[*auth.ExchangeError](err); !ok || ee == nil || ee.Terminal() {
				t.Errorf("Token() error = %v, want a non-terminal *auth.ExchangeError", err)
			}
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Errorf("Token() error = %v, want it to match its cause %v", err, tc.cause)
			}
			access, refresh := src.HeldTokens()
			if access.Value != held.Value || refresh != "rt-0" {
				t.Errorf("held tokens after the failed refresh = (%q, %q), want (%q, %q)", access.Value, refresh, held.Value, "rt-0")
			}

			_, err = src.Token(t.Context())

			if !errors.Is(err, auth.ErrRefreshFailed) || errors.Is(err, auth.ErrReauthRequired) {
				t.Errorf("second Token() error = %v, want auth.ErrRefreshFailed, not auth.ErrReauthRequired", err)
			}
			if got := endpoint.refreshes.Load(); got != 2 {
				t.Errorf("refresh requests after two Token() calls = %d, want 2: the kept refresh token is tried again", got)
			}
		})
	}
}
