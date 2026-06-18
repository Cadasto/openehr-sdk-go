package authprobes_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/auth"
)

// smartConfigCassette loads the canonical SMART configuration cassette
// shared with smart/discovery's own tests. Path resolution uses
// runtime.Caller so the helper works regardless of CWD — the conformance
// harness invokes probes outside `go test`.
func smartConfigCassette(t *testing.T) []byte {
	t.Helper()
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve cassette path: runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(src), "..", "..", "cassettes", "its_rest", "discovery", "smart-configuration.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

// mismatchedSpecCassette returns the canonical cassette with the
// org.openehr.rest spec_version rewritten to an incompatible value, for
// PROBE-003's fail-fast assertion.
func mismatchedSpecCassette(t *testing.T) []byte {
	t.Helper()
	b := smartConfigCassette(t)
	out := bytes.ReplaceAll(b, []byte(`"spec_version": "1.1.0-development"`), []byte(`"spec_version": "1.0.3"`))
	if bytes.Equal(out, b) {
		t.Fatal("mismatchedSpecCassette: spec_version replacement did not match the cassette")
	}
	return out
}

func assertPass(t *testing.T, r probes.Result, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s framework error: %v", r.Probe, err)
	}
	if r.Status != "pass" {
		t.Errorf("%s status = %q (detail: %s); want pass", r.Probe, r.Status, r.Detail)
	}
}

// TestProbe001 runs PROBE-001 and asserts discovery declares
// response_type=code and code_challenge_method=S256 (REQ-061).
func TestProbe001(t *testing.T) {
	r, err := probes.Probe001DiscoveryCodePKCE(context.Background(), smartConfigCassette(t))
	assertPass(t, r, err)
}

// TestProbe002 runs PROBE-002 and asserts the resolved catalog advertises
// org.openehr.rest with a parseable base URL and a spec_version (REQ-070).
func TestProbe002(t *testing.T) {
	r, err := probes.Probe002OpenEHRRestService(context.Background(), smartConfigCassette(t))
	assertPass(t, r, err)
}

// TestProbe003 runs PROBE-003 and asserts a spec-version mismatch fails
// fast at resolution with a typed DiscoveryError (REQ-072).
func TestProbe003(t *testing.T) {
	r, err := probes.Probe003SpecVersionMismatch(context.Background(), mismatchedSpecCassette(t))
	assertPass(t, r, err)
}

// TestProbe004 runs PROBE-004 and asserts the S256 PKCE verifier
// round-trip plus the G-7 RFC 7636 / x/oauth2 parity properties (REQ-061).
func TestProbe004(t *testing.T) {
	r, err := probes.Probe004PKCEVerifierRoundTrip(context.Background())
	assertPass(t, r, err)
}

// TestProbe005 runs PROBE-005 and asserts a configured openEHR scope
// survives the authorize -> token-response round trip (REQ-061).
func TestProbe005(t *testing.T) {
	r, err := probes.Probe005ScopeRoundTrip(context.Background())
	assertPass(t, r, err)
}

// TestProbe006 runs PROBE-006 and asserts a JWKS key rotation triggers
// exactly one refresh and validates the id_token transparently (REQ-062).
func TestProbe006(t *testing.T) {
	r, err := probes.Probe006JWKSRotationTransparent(context.Background())
	assertPass(t, r, err)
}

// TestProbe007 runs PROBE-007 (transport half) and asserts that a wire
// 401 with a configured Reauther triggers exactly one Reauth call,
// retries with the refreshed bearer, and succeeds (REQ-063).
func TestProbe007(t *testing.T) {
	r, err := probes.Probe007TransportTokenRefresh(context.Background())
	assertPass(t, r, err)
}

// TestProbe007Proactive runs PROBE-007 (proactive half) and asserts an
// expired token is refreshed silently via grant_type=refresh_token before
// the next acquisition (REQ-063).
func TestProbe007Proactive(t *testing.T) {
	r, err := probes.Probe007ProactiveTokenRefresh(context.Background())
	assertPass(t, r, err)
}

// TestProbe008 runs PROBE-008 and asserts platform principal claims
// surface verbatim, with absent claims surfacing as nil (REQ-067).
func TestProbe008(t *testing.T) {
	r, err := probes.Probe008PrincipalClaimsVerbatim(context.Background())
	assertPass(t, r, err)
}

// TestProbe009 runs PROBE-009 and asserts caller attribution is emitted
// (header + caller.agent_id OTel attribute) only when configured (REQ-066).
func TestProbe009(t *testing.T) {
	r, err := probes.Probe009CallerAttributionOptIn(context.Background())
	assertPass(t, r, err)
}

// TestLaunchModeStandalone proves REQ-068's standalone launch mode:
// AuthorizeURL without a launch parameter.
func TestLaunchModeStandalone(t *testing.T) {
	r, err := probes.LaunchModeStandalone(context.Background())
	assertPass(t, r, err)
}

// TestLaunchModeEmbedded proves REQ-068's embedded (EHR) launch mode:
// AuthorizeURL forwards the EHR-supplied launch parameter.
func TestLaunchModeEmbedded(t *testing.T) {
	r, err := probes.LaunchModeEmbedded(context.Background())
	assertPass(t, r, err)
}

// TestLaunchModeBackend proves REQ-068's backend launch mode: client
// credentials (symmetric + private_key_jwt) and jwt-bearer produce the
// expected token requests.
func TestLaunchModeBackend(t *testing.T) {
	r, err := probes.LaunchModeBackend(context.Background())
	assertPass(t, r, err)
}
