package discoveryprobes_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/discovery"
)

// cassetteBytes loads the canonical SMART configuration cassette
// shared with `smart/discovery`'s own tests. Path resolution uses
// runtime.Caller so the helper works regardless of CWD — the
// conformance harness invokes probes outside `go test`.
func cassetteBytes(t *testing.T) []byte {
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

// TestProbe040 runs PROBE-040 and asserts a TTL-cached resolver
// produces exactly one upstream fetch across two Resolve calls.
func TestProbe040(t *testing.T) {
	r, err := probes.Probe040CatalogTTL(context.Background(), cassetteBytes(t))
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
	}
}

// TestProbe041 runs PROBE-041 and asserts Refresh against a 401
// upstream surfaces a typed DiscoveryError(fetch_failed) after
// exactly one fetch.
func TestProbe041(t *testing.T) {
	r, err := probes.Probe041CatalogRefreshOn401(context.Background(), cassetteBytes(t))
	if err != nil {
		t.Fatalf("probe framework error: %v", err)
	}
	if r.Status != "pass" {
		t.Errorf("status = %q (detail: %s); want pass", r.Status, r.Detail)
	}
}
