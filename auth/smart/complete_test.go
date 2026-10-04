package smart_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

const completeIssuer = "https://idp.example/realms/clinic"

// tokenEndpoint is a stub token endpoint that answers every request with
// body and records the forms it received.
type tokenEndpoint struct {
	srv  *httptest.Server
	mu   sync.Mutex
	body string
	got  []url.Values
}

func newTokenEndpoint(t *testing.T, body string) *tokenEndpoint {
	t.Helper()
	te := &tokenEndpoint{body: body}
	te.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		te.mu.Lock()
		te.got = append(te.got, r.PostForm)
		body := te.body
		te.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(te.srv.Close)
	return te
}

// forms returns the forms the endpoint has received.
func (te *tokenEndpoint) forms() []url.Values {
	te.mu.Lock()
	defer te.mu.Unlock()
	return append([]url.Values(nil), te.got...)
}

// endpoints returns authorization-server endpoints whose token endpoint is
// te.
func (te *tokenEndpoint) endpoints() discovery.AuthEndpoints {
	return discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://idp.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL(te.srv.URL + "/token"),
	}
}

// completeSource builds a source bound to completeIssuer whose token
// endpoint is te; issAdvertised sets the server's RFC 9207 flag.
func completeSource(t *testing.T, te *tokenEndpoint, issAdvertised bool, opts ...smart.Option) *smart.Source {
	t.Helper()
	ep := te.endpoints()
	ep.AuthorizationResponseIssParameterSupported = issAdvertised
	src, err := newSource("client-id", ep, append([]smart.Option{
		smart.WithHTTPClient(te.srv.Client()),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithIssuer(completeIssuer),
	}, opts...)...)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	return src
}

// completeSentinels are the classes a CompleteAuthorization failure can
// match; each refusal matches exactly one of them.
var completeSentinels = []error{
	smart.ErrLaunchInvalidState,
	smart.ErrLaunchIssuerMismatch,
	smart.ErrAuthorizationRejected,
	auth.ErrInvalidConfig,
	auth.ErrTokenExchangeFailed,
}

// TestCompleteAuthorizationRefusesBeforeExchange pins REQ-061: a redirect
// that repeats state, iss, code or error is rejected first; then it is
// checked for state, then for the RFC 9207 issuer, then for an error
// response, and a redirect that fails any check is refused with that
// check's sentinel and no token-endpoint call. Where a redirect fails more
// than one check, the earlier check decides. An empty iss counts as absent,
// and a request without an issuer cannot check one that is sent or
// advertised, which is a configuration error.
func TestCompleteAuthorizationRefusesBeforeExchange(t *testing.T) { // REQ-061
	const state = "state-1"
	tests := []struct {
		name          string
		callback      url.Values
		issAdvertised bool
		noIssuer      bool // the source, and so the request, has no issuer
		want          error
	}{
		{name: "state repeated", callback: url.Values{"state": {state, state}, "code": {"c"}}, want: smart.ErrAuthorizationRejected},
		{name: "state repeated, the first one wrong", callback: url.Values{"state": {"other", state}, "code": {"c"}}, want: smart.ErrAuthorizationRejected},
		{name: "iss repeated", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {completeIssuer, completeIssuer}}, want: smart.ErrAuthorizationRejected},
		{name: "iss repeated, the second one foreign", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {completeIssuer, "https://evil.example"}}, want: smart.ErrAuthorizationRejected},
		{name: "code repeated", callback: url.Values{"state": {state}, "code": {"c1", "c2"}}, want: smart.ErrAuthorizationRejected},
		{name: "error repeated", callback: url.Values{"state": {state}, "error": {"access_denied", "server_error"}}, want: smart.ErrAuthorizationRejected},
		{name: "error repeated with a wrong state", callback: url.Values{"state": {"other"}, "error": {"access_denied", "access_denied"}}, want: smart.ErrAuthorizationRejected},
		{name: "state differs", callback: url.Values{"state": {"other"}, "code": {"c"}, "iss": {completeIssuer}}, want: smart.ErrLaunchInvalidState},
		{name: "state missing", callback: url.Values{"code": {"c"}, "iss": {completeIssuer}}, want: smart.ErrLaunchInvalidState},
		{name: "state differs and issuer differs", callback: url.Values{"state": {"other"}, "code": {"c"}, "iss": {"https://evil.example"}}, want: smart.ErrLaunchInvalidState},
		{name: "state differs and an error response", callback: url.Values{"state": {"other"}, "error": {"access_denied"}}, want: smart.ErrLaunchInvalidState},
		{name: "issuer differs", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {"https://evil.example"}}, want: smart.ErrLaunchIssuerMismatch},
		{name: "issuer differs by a trailing slash", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {completeIssuer + "/"}}, want: smart.ErrLaunchIssuerMismatch},
		{name: "issuer differs in letter case", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {"https://IDP.example/realms/clinic"}}, want: smart.ErrLaunchIssuerMismatch},
		{name: "issuer empty where the server advertises it", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {""}}, issAdvertised: true, want: smart.ErrLaunchIssuerMismatch},
		{name: "no issuer configured, iss sent", callback: url.Values{"state": {state}, "code": {"c"}, "iss": {completeIssuer}}, noIssuer: true, want: auth.ErrInvalidConfig},
		{name: "no issuer configured, parameter advertised", callback: url.Values{"state": {state}, "code": {"c"}}, issAdvertised: true, noIssuer: true, want: auth.ErrInvalidConfig},
		{name: "no issuer configured, state differs", callback: url.Values{"state": {"other"}, "code": {"c"}, "iss": {completeIssuer}}, noIssuer: true, want: smart.ErrLaunchInvalidState},
		{name: "issuer differs and an error response", callback: url.Values{"state": {state}, "error": {"access_denied"}, "iss": {"https://evil.example"}}, want: smart.ErrLaunchIssuerMismatch},
		{name: "issuer missing where the server advertises it", callback: url.Values{"state": {state}, "code": {"c"}}, issAdvertised: true, want: smart.ErrLaunchIssuerMismatch},
		{name: "issuer missing where advertised, with an error response", callback: url.Values{"state": {state}, "error": {"access_denied"}}, issAdvertised: true, want: smart.ErrLaunchIssuerMismatch},
		{name: "error response", callback: url.Values{"state": {state}, "error": {"access_denied"}, "iss": {completeIssuer}}, want: smart.ErrAuthorizationRejected},
		{name: "error response without iss", callback: url.Values{"state": {state}, "error": {"access_denied"}}, want: smart.ErrAuthorizationRejected},
		{name: "error response that also carries a code", callback: url.Values{"state": {state}, "error": {"server_error"}, "code": {"c"}}, want: smart.ErrAuthorizationRejected},
		{name: "neither error nor code", callback: url.Values{"state": {state}, "iss": {completeIssuer}}, want: smart.ErrAuthorizationRejected},
		{name: "empty code", callback: url.Values{"state": {state}, "code": {""}}, want: smart.ErrAuthorizationRejected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			te := newTokenEndpoint(t, `{"access_token":"at-1","token_type":"Bearer","expires_in":3600}`)
			var opts []smart.Option
			if tc.noIssuer {
				opts = append(opts, smart.WithIssuer(""))
			}
			src := completeSource(t, te, tc.issAdvertised, opts...)
			req, err := src.BeginAuthorization(state)
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			tok, _, err := src.CompleteAuthorization(t.Context(), tc.callback, req)
			for _, s := range completeSentinels {
				if is, want := errors.Is(err, s), errors.Is(tc.want, s); is != want {
					t.Errorf("CompleteAuthorization(%v) error = %v; errors.Is(%v) = %v, want %v", tc.callback, err, s, is, want)
				}
			}
			if !tok.IsZero() {
				t.Errorf("CompleteAuthorization(%v) returned token %q on refusal, want none", tc.callback, tok.Value)
			}
			if n := len(te.forms()); n != 0 {
				t.Errorf("CompleteAuthorization(%v) made %d token-endpoint calls, want none before the checks pass", tc.callback, n)
			}
		})
	}
}

// TestCompleteAuthorizationErrorResponse pins REQ-061: an error response
// fails with ErrAuthorizationRejected, and errors.As extracts an
// *auth.OAuth2Error holding the error, error_description and error_uri
// parameters; a redirect with neither error nor code carries no such
// envelope.
func TestCompleteAuthorizationErrorResponse(t *testing.T) { // REQ-061
	te := newTokenEndpoint(t, `{"access_token":"at-1"}`)
	src := completeSource(t, te, false)
	req, err := src.BeginAuthorization("state-1")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}

	callback := url.Values{
		"state":             {"state-1"},
		"error":             {"access_denied"},
		"error_description": {"The user declined"},
		"error_uri":         {"https://idp.example/errors/access_denied"},
	}
	_, _, err = src.CompleteAuthorization(t.Context(), callback, req)
	if !errors.Is(err, smart.ErrAuthorizationRejected) {
		t.Fatalf("CompleteAuthorization(%v) error = %v, want ErrAuthorizationRejected", callback, err)
	}
	var oe *auth.OAuth2Error
	if !errors.As(err, &oe) || oe == nil {
		t.Fatalf("CompleteAuthorization(%v) error = %v, want an *auth.OAuth2Error inside", callback, err)
	}
	want := auth.OAuth2Error{Code: "access_denied", Description: "The user declined", URI: "https://idp.example/errors/access_denied"}
	if *oe != want {
		t.Errorf("OAuth2Error = %+v, want %+v", *oe, want)
	}

	_, _, err = src.CompleteAuthorization(t.Context(), url.Values{"state": {"state-1"}}, req)
	if !errors.Is(err, smart.ErrAuthorizationRejected) {
		t.Fatalf("CompleteAuthorization without code or error: error = %v, want ErrAuthorizationRejected", err)
	}
	if oe, ok := errors.AsType[*auth.OAuth2Error](err); ok {
		t.Errorf("CompleteAuthorization without code or error: error carries %+v, want no OAuth2Error the server did not send", oe)
	}
}

// TestCompleteAuthorizationExchanges pins REQ-061: a redirect that passes
// every check exchanges its code as ExchangeAuthorizationCode does, with
// the request's PKCE verifier, and the source then holds the token. An
// issuer is accepted when it equals the request's exactly, and its absence,
// or an empty value, is accepted when the server does not advertise the
// parameter, also by a source that has no issuer to compare with.
func TestCompleteAuthorizationExchanges(t *testing.T) { // REQ-061
	tests := []struct {
		name          string
		iss           []string // nil leaves iss out
		issAdvertised bool
		noIssuer      bool // the source, and so the request, has no issuer
	}{
		{name: "matching issuer", iss: []string{completeIssuer}},
		{name: "matching issuer, advertised", iss: []string{completeIssuer}, issAdvertised: true},
		{name: "no issuer, not advertised"},
		{name: "empty issuer, not advertised", iss: []string{""}},
		{name: "no issuer configured, none sent or advertised", noIssuer: true},
		{name: "no issuer configured, empty iss, not advertised", iss: []string{""}, noIssuer: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			te := newTokenEndpoint(t, `{"access_token":"at-1","token_type":"Bearer","expires_in":3600}`)
			var opts []smart.Option
			if tc.noIssuer {
				opts = append(opts, smart.WithIssuer(""))
			}
			src := completeSource(t, te, tc.issAdvertised, opts...)
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			callback := url.Values{"state": {req.State}, "code": {"code-xyz"}}
			if tc.iss != nil {
				callback["iss"] = tc.iss
			}
			tok, tr, err := src.CompleteAuthorization(t.Context(), callback, req)
			if err != nil {
				t.Fatalf("CompleteAuthorization(%v) error = %v, want success", callback, err)
			}
			if tok.Value != "at-1" || tr.AccessToken != "at-1" {
				t.Errorf("CompleteAuthorization() token = %q, response access_token = %q, want at-1", tok.Value, tr.AccessToken)
			}
			forms := te.forms()
			if len(forms) != 1 {
				t.Fatalf("token-endpoint calls = %d, want 1", len(forms))
			}
			f := forms[0]
			if f.Get("grant_type") != "authorization_code" || f.Get("code") != "code-xyz" || f.Get("code_verifier") != req.PKCE.Verifier {
				t.Errorf("token request = %v, want grant_type=authorization_code, code=code-xyz and the request's verifier", f)
			}
			held, err := src.Token(t.Context())
			if err != nil || held.Value != "at-1" {
				t.Errorf("Source.Token() after CompleteAuthorization = %q, %v; want at-1", held.Value, err)
			}
		})
	}
}

// TestCompleteAuthorizationRequiresRequest pins that CompleteAuthorization
// refuses a request that did not come from BeginAuthorization before it
// reads the redirect, so an empty state on both sides cannot pass the state
// check and the redirect's content does not decide the error.
func TestCompleteAuthorizationRequiresRequest(t *testing.T) { // REQ-061
	for _, callback := range []url.Values{
		{"code": {"c"}},
		{"error": {"access_denied"}},
		{"code": {"c1", "c2"}},
		{},
	} {
		te := newTokenEndpoint(t, `{"access_token":"at-1"}`)
		src := completeSource(t, te, false)
		_, _, err := src.CompleteAuthorization(t.Context(), callback, smart.AuthorizationRequest{})
		if !errors.Is(err, auth.ErrInvalidConfig) {
			t.Errorf("CompleteAuthorization(%v, empty request) error = %v, want auth.ErrInvalidConfig", callback, err)
		}
		if n := len(te.forms()); n != 0 {
			t.Errorf("CompleteAuthorization(%v, empty request) made %d token-endpoint calls, want none", callback, n)
		}
	}
}
