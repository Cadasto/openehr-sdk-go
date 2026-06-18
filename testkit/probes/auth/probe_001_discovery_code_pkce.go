package authprobes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// Probe001DiscoveryCodePKCE implements PROBE-001: the SMART configuration
// document MUST declare the "code" response type and the "S256" PKCE
// challenge method (REQ-061). The probe serves the SMART configuration
// cassette from an in-process server, resolves it through the real
// discovery.Resolver, and asserts both lists on the resolved
// AuthEndpoints.
//
// `cassetteBody` is the SMART configuration JSON the upstream server
// returns — typically the vendored
// testkit/cassettes/its_rest/discovery/smart-configuration.json read by
// the caller.
//
// Pass conditions:
//  1. Resolve succeeds.
//  2. AuthEndpoints.ResponseTypesSupported contains "code".
//  3. AuthEndpoints.CodeChallengeMethodsSupported contains "S256".
func Probe001DiscoveryCodePKCE(ctx context.Context, cassetteBody []byte) (Result, error) { // PROBE-001 (REQ-061)
	r := Result{Probe: "PROBE-001"}
	if len(cassetteBody) == 0 {
		return r, errors.New("PROBE-001: cassetteBody is empty")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cassetteBody)
	}))
	defer srv.Close()

	cat, err := resolveCassette(ctx, srv)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Resolve failed: %v", err)
		return r, nil
	}
	if !slices.Contains(cat.Auth.ResponseTypesSupported, "code") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("response_types_supported = %v; want it to contain \"code\"", cat.Auth.ResponseTypesSupported)
		return r, nil
	}
	if !slices.Contains(cat.Auth.CodeChallengeMethodsSupported, "S256") {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("code_challenge_methods_supported = %v; want it to contain \"S256\"", cat.Auth.CodeChallengeMethodsSupported)
		return r, nil
	}
	r.Status = "pass"
	r.Detail = "discovery declares response_type=code and code_challenge_method=S256"
	return r, nil
}

// resolveCassette builds a strict-pin discovery.Resolver against srv and
// resolves it. Shared by the discovery-declaration probes (001/002/003)
// so each exercises the real resolver, not a hand-built catalog.
func resolveCassette(ctx context.Context, srv *httptest.Server) (*discovery.ServiceCatalog, error) {
	res, err := discovery.NewResolver(
		discovery.NewMemoryCache(),
		discovery.WithHTTPClient(srv.Client()),
		discovery.WithAllowInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("build resolver: %w", err)
	}
	return res.Resolve(ctx, srv.URL)
}
