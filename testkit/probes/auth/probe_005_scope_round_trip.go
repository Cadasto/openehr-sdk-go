package authprobes

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Probe005ScopeRoundTrip implements PROBE-005: a configured openEHR scope
// (`<compartment>/<resource>.<permission>`) survives token exchange and
// lands in the token response `scope` field (REQ-061).
//
// Pass conditions:
//  1. The authorization-request `scope` parameter contains the configured scope.
//  2. The token response `scope` field (parsed by the SDK) contains it.
func Probe005ScopeRoundTrip(ctx context.Context) (Result, error) { // PROBE-005 (REQ-061)
	r := Result{Probe: "PROBE-005"}
	const want = "patient/COMPOSITION.read"
	capture, err := runPKCEFlow(ctx, []string{want, "openid"}, "")
	if err != nil {
		return r, fmt.Errorf("PROBE-005: %w", err)
	}

	authScopes := strings.Fields(capture.authQuery.Get("scope"))
	if !slices.Contains(authScopes, want) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("authorization scope = %v; want it to contain %q", authScopes, want)
		return r, nil
	}

	// The token response (parsed by the SDK) MUST carry the granted scope.
	respScopes := strings.Fields(capture.tokenResp.Scope)
	if !slices.Contains(respScopes, want) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("token response scope = %q; want it to contain %q", capture.tokenResp.Scope, want)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("scope %q survived authorize -> token-response round trip", want)
	return r, nil
}
