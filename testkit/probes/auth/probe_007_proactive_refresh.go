package authprobes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	authsmart "github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// Probe007ProactiveTokenRefresh implements PROBE-007 (proactive,
// expiry-based half): an expired access token with a valid refresh token
// is refreshed silently before the next token acquisition. The token
// endpoint receives a `grant_type=refresh_token` exchange on the wire and
// the SDK's TokenSource returns the newly issued access token (REQ-063).
//
// This complements the transport half (Probe007TransportTokenRefresh,
// wire 401 -> Reauth -> retry). Here the trigger is the proactive
// expiry-based refresh path in Source.Token / Source.RefreshIfNeeded.
//
// Scenario:
//   - The Source is seeded with an already-expired access token and a
//     valid refresh token (as if imported from a prior launch).
//   - Source.Token (called transparently before the next request) sees the
//     token is stale and exchanges the refresh token at the token endpoint.
//
// Pass conditions (all must hold):
//  1. Token returns no error.
//  2. The token endpoint received exactly one request carrying
//     grant_type=refresh_token with the seeded refresh_token value.
//  3. The returned access token is the freshly issued one (not the expired
//     seed).
func Probe007ProactiveTokenRefresh(ctx context.Context) (Result, error) { // PROBE-007 (REQ-063)
	r := Result{Probe: "PROBE-007"}

	var (
		mu        sync.Mutex
		gotForm   url.Values
		callCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		callCount++
		gotForm = req.Form
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2"}`))
	}))
	defer srv.Close()

	authEP := discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://auth.probe007.example/authorize"),
		TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
	}
	src, err := authsmart.New(
		"probe007-client",
		authEP,
		authsmart.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		return r, fmt.Errorf("PROBE-007: build Source: %w", err)
	}

	// Seed an already-expired access token plus a valid refresh token.
	src.SetTokens(auth.Token{
		Value:     "expired-access",
		Type:      "Bearer",
		ExpiresAt: time.Now().Add(-time.Minute),
	}, "rt-1")

	tok, err := src.Token(ctx)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Token returned error instead of refreshing: %v", err)
		return r, nil
	}

	mu.Lock()
	calls := callCount
	form := gotForm
	mu.Unlock()

	if calls != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token endpoint received %d calls; want exactly 1", calls)
		return r, nil
	}
	if gt := form.Get("grant_type"); gt != "refresh_token" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token request grant_type = %q; want refresh_token", gt)
		return r, nil
	}
	if rt := form.Get("refresh_token"); rt != "rt-1" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token request refresh_token = %q; want rt-1 (the seeded token)", rt)
		return r, nil
	}
	if tok.Value != "fresh-access" {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Token returned %q; want the freshly issued fresh-access", tok.Value)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = "expired token proactively refreshed via grant_type=refresh_token; fresh bearer returned"
	return r, nil
}
