package authprobes_test

import (
	"context"
	"testing"

	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/auth"
)

// TestProbe007 runs PROBE-007 (transport half) and asserts that a wire
// 401 with a configured Reauther triggers exactly one Reauth call,
// retries with the refreshed bearer, and succeeds (REQ-063).
func TestProbe007(t *testing.T) {
	r, err := probes.Probe007TransportTokenRefresh(context.Background())
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-007 status = %q (detail: %s); want pass", r.Status, r.Detail)
	}
}
