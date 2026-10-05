package authprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
)

// probe106ClientID is the public client PROBE-106 signs in and out.
const probe106ClientID = "probe106-client"

// probe106Request is one request the PROBE-106 authorization server received.
type probe106Request struct {
	method, contentType, authorization string
	form                               url.Values
}

// probe106Server is the in-process authorization server of PROBE-106. It
// publishes a SMART configuration that advertises revocation_endpoint,
// answers the code exchange with an access token and a refresh token, and
// records what its token and revocation endpoints receive.
type probe106Server struct {
	mu     sync.Mutex
	token  []probe106Request
	revoke []probe106Request
}

func (p *probe106Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	base := "https://" + req.Host
	if req.URL.Path == "/.well-known/smart-configuration" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
  "authorization_endpoint": %[1]q,
  "token_endpoint": %[2]q,
  "revocation_endpoint": %[3]q,
  "response_types_supported": ["code"],
  "code_challenge_methods_supported": ["S256"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "capabilities": ["launch-standalone", "client-public"],
  "services": {"org.openehr.rest": {"baseUrl": %[4]q, "spec_version": "1.1.0-development", "capabilities": ["composition"]}}
}`, base+"/authorize", base+"/token", base+"/revoke", base+"/openehr/v1")
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	got := probe106Request{
		method:        req.Method,
		contentType:   req.Header.Get("Content-Type"),
		authorization: req.Header.Get("Authorization"),
		form:          req.PostForm,
	}
	switch req.URL.Path {
	case "/token":
		p.mu.Lock()
		p.token = append(p.token, got)
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-106","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-106"}`))
	case "/revoke":
		p.mu.Lock()
		p.revoke = append(p.revoke, got)
		p.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, req)
	}
}

// requests returns copies of what the token and revocation endpoints have
// received.
func (p *probe106Server) requests() (token, revoke []probe106Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.token), slices.Clone(p.revoke)
}

// Probe106TokenRevocation implements PROBE-106: Source.Revoke sends the
// source's refresh token to the advertised revocation_endpoint (RFC 7009)
// and leaves the source signed out.
//
// Scenario:
//   - An in-process authorization server publishes a SMART configuration
//     that advertises revocation_endpoint. The probe resolves it through the
//     real discovery.Resolver and builds a public-client Source from the
//     catalog.
//   - A code exchange leaves the source holding an access token and a
//     refresh token.
//   - Revoke signs the source out; the revocation endpoint answers 200.
//
// Pass conditions (all must hold):
//  1. The catalog carries the advertised revocation endpoint.
//  2. Revoke returns nil.
//  3. The revocation endpoint received exactly one form-encoded POST
//     carrying token=<the refresh token>, token_type_hint=refresh_token and
//     the same client-authentication form fields the token endpoint
//     received (the public client's client_id, and no secret or assertion),
//     with no Authorization header at either endpoint.
//  4. The next Token call fails with auth.ErrReauthRequired, and the token
//     endpoint receives no further request.
func Probe106TokenRevocation(ctx context.Context) (Result, error) { // PROBE-106 (REQ-167)
	r := Result{Probe: "PROBE-106"}
	as := &probe106Server{}
	srv := httptest.NewTLSServer(as)
	defer srv.Close()

	cat, err := resolveFixture(ctx, srv)
	if err != nil {
		return r, fmt.Errorf("PROBE-106: resolve the SMART configuration: %w", err)
	}
	if cat.Auth.RevocationEndpoint == nil || cat.Auth.RevocationEndpoint.String() != srv.URL+"/revoke" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("catalog revocation endpoint = %v; want the advertised %s/revoke", cat.Auth.RevocationEndpoint, srv.URL)
		return r, nil
	}
	src, err := authsmart.NewFromCatalog(cat, probe106ClientID,
		authsmart.WithHTTPClient(srv.Client()),
		authsmart.WithRedirectURI("https://app.probe106.example/callback"),
	)
	if err != nil {
		return r, fmt.Errorf("PROBE-106: build Source: %w", err)
	}
	areq, err := src.BeginAuthorization("")
	if err != nil {
		return r, fmt.Errorf("PROBE-106: BeginAuthorization: %w", err)
	}
	if _, _, err := src.ExchangeAuthorizationCode(ctx, "code-106", areq.State, areq); err != nil {
		return r, fmt.Errorf("PROBE-106: ExchangeAuthorizationCode: %w", err)
	}

	if err := src.Revoke(ctx); err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Revoke returned %v on a 200 answer; want nil", err)
		return r, nil
	}

	tokenReqs, revokeReqs := as.requests()
	if len(revokeReqs) != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("revocation endpoint received %d requests; want exactly 1", len(revokeReqs))
		return r, nil
	}
	rev := revokeReqs[0]
	switch {
	case rev.method != http.MethodPost || rev.contentType != "application/x-www-form-urlencoded":
		r.Status = "fail"
		r.Detail = fmt.Sprintf("revocation request is %s with Content-Type %q; want a form-encoded POST", rev.method, rev.contentType)
		return r, nil
	case !slices.Equal(rev.form["token"], []string{"rt-106"}):
		r.Status = "fail"
		r.Detail = "revocation request does not carry exactly one token, the refresh token the code exchange issued"
		return r, nil
	case !slices.Equal(rev.form["token_type_hint"], []string{"refresh_token"}):
		r.Status = "fail"
		r.Detail = fmt.Sprintf("revocation request token_type_hint = %q; want exactly [refresh_token]", rev.form["token_type_hint"])
		return r, nil
	case len(tokenReqs) != 1:
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token endpoint received %d requests before the Token call; want only the code exchange", len(tokenReqs))
		return r, nil
	case !slices.Equal(rev.form["client_id"], []string{probe106ClientID}) ||
		!slices.Equal(tokenReqs[0].form["client_id"], []string{probe106ClientID}) ||
		rev.authorization != "" || tokenReqs[0].authorization != "":
		r.Status = "fail"
		r.Detail = fmt.Sprintf("client authentication differs: revocation client_id %q, token client_id %q, Authorization headers present %t/%t; want the public client's client_id at both and no header",
			rev.form["client_id"], tokenReqs[0].form["client_id"], rev.authorization != "", tokenReqs[0].authorization != "")
		return r, nil
	case !clientAuthEqual(tokenReqs[0].form, rev.form):
		r.Status = "fail"
		r.Detail = fmt.Sprintf("client-authentication form fields differ: token %q, revocation %q; want the same keys and the same values",
			clientAuthEncode(tokenReqs[0].form), clientAuthEncode(rev.form))
		return r, nil
	}

	if tok, err := src.Token(ctx); !errors.Is(err, auth.ErrReauthRequired) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Token after Revoke returned a token of %d bytes and error %v; want auth.ErrReauthRequired", len(tok.Value), err)
		return r, nil
	}
	if after, _ := as.requests(); len(after) != len(tokenReqs) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Token after Revoke sent %d token-endpoint requests; want none", len(after)-len(tokenReqs))
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "Revoke posted the refresh token with token_type_hint=refresh_token and the same client authentication as the token request to the advertised revocation_endpoint; the source is signed out"
	return r, nil
}

// clientAuthKeys are the form fields that authenticate the client. The
// Authorization header is compared beside them.
var clientAuthKeys = []string{"client_id", "client_secret", "client_assertion", "client_assertion_type"}

// clientAuthEqual reports whether the two requests carry the same
// client-authentication form fields: the same keys and the same values.
func clientAuthEqual(a, b url.Values) bool {
	for _, key := range clientAuthKeys {
		if !slices.Equal(a[key], b[key]) {
			return false
		}
	}
	return true
}

// clientAuthEncode renders those fields for a failure message. Keys are
// sorted, and a field neither request sent is left out.
func clientAuthEncode(form url.Values) string {
	got := make(url.Values)
	for _, key := range clientAuthKeys {
		if values, ok := form[key]; ok {
			got[key] = values
		}
	}
	return got.Encode()
}
