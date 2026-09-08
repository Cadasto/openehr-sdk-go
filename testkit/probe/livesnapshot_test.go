package probe_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/basic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// newUUIDv4 builds a random RFC 4122 v4 UUID from crypto/rand, so the per-run
// EHR id needs no third-party uuid dependency.
func newUUIDv4(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("read random: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func liveFail(err error) probe.Result {
	return probe.Result{Status: probe.StatusFail, Detail: err.Error()}
}

func liveFailf(format string, a ...any) probe.Result {
	return probe.Result{Status: probe.StatusFail, Detail: fmt.Sprintf(format, a...)}
}

// coreLiveEntries is the self-scoping core read/write suite for a Live snapshot:
// one mutating create against the per-run id, then read-only probes against
// that same id. They share id by closure — declared here, never observed from
// the transport (REQ-082) — and the runner executes them in order against one
// deployment. A read probe would need to skip when its precondition is absent;
// here the create earlier in the run is that precondition.
func coreLiveEntries(id openehrclient.EHRID) []probe.Entry {
	rw := []probe.Mode{probe.ModeSandbox, probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-EHR-CREATE",
			Effect: probe.EffectMutating,
			Modes:  rw,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				rec, meta, err := openehrclient.Create(ctx, c, openehrclient.WithEHRID(id))
				if err != nil {
					return liveFail(err), nil
				}
				if rec == nil || rec.EHRID.Value != string(id) {
					return liveFailf("create returned id %v, want %s", rec, id), nil
				}
				if meta == nil || meta.ETag == "" {
					return liveFailf("create returned no ETag"), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: rec.EHRID.Value}, nil
			},
		},
		{
			ID:     "LIVE-EHR-GET",
			Effect: probe.EffectReadOnly,
			Modes:  rw,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				got, _, err := openehrclient.Get(ctx, c, id)
				if err != nil {
					return liveFail(err), nil
				}
				if got == nil || got.EHRID.Value != string(id) {
					return liveFailf("get returned id %v, want %s", got, id), nil
				}
				return probe.Result{Status: probe.StatusPass}, nil
			},
		},
		{
			ID:     "LIVE-EHR-EXISTS",
			Effect: probe.EffectReadOnly,
			Modes:  rw,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				ok, err := openehrclient.Exists(ctx, c, id)
				if err != nil {
					return liveFail(err), nil
				}
				if !ok {
					return liveFailf("exists = false for the id just created"), nil
				}
				return probe.Result{Status: probe.StatusPass}, nil
			},
		},
		{
			ID:     "LIVE-EHR-STATUS-GET",
			Effect: probe.EffectReadOnly,
			Modes:  rw,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				st, _, err := ehrstatus.Get(ctx, c, id)
				if err != nil {
					return liveFail(err), nil
				}
				if st == nil {
					return liveFailf("ehr_status is nil"), nil
				}
				return probe.Result{Status: probe.StatusPass}, nil
			},
		},
		{
			ID:     "LIVE-SYSTEM-VERSION",
			Effect: probe.EffectReadOnly,
			Modes:  rw,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				v, err := system.Version(ctx, c)
				if err != nil {
					// The OPTIONS / capabilities operation is optional; a
					// deployment that does not implement it answers 404.
					// That is a skip with a named precondition, not a
					// failure — the probe passes against a deployment that
					// does serve it (REQ-082 Live).
					if errors.Is(err, transport.ErrNotFound) {
						return probe.Result{Status: probe.StatusSkip, Detail: "deployment does not implement the OPTIONS / capabilities operation"}, nil
					}
					return liveFail(err), nil
				}
				if v == "" {
					return liveFailf("empty REST API spec version"), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: v}, nil
			},
		},
	}
}

// TestLiveCoreSnapshot runs the core read/write suite against a live openEHR
// deployment and logs each probe's verdict — a conformance snapshot of the
// core EHR surface. It is opt-in and skipped in CI: OPENEHR_LIVE_EHRBASE names
// the target, and OPENEHR_LIVE_ALLOW_MUTATING is the separate write opt-in
// REQ-082 requires. Run: it fails if any probe fails, so it doubles as a Live
// gate for the core flows.
func TestLiveCoreSnapshot(t *testing.T) {
	base := os.Getenv("OPENEHR_LIVE_EHRBASE")
	if base == "" {
		t.Skip("set OPENEHR_LIVE_EHRBASE to a live openEHR REST base to run the core Live snapshot; not part of CI")
	}
	if os.Getenv(envAllowMutating) == "" {
		t.Skipf("set %s to let the snapshot's create probe write to %s", envAllowMutating, base)
	}

	var src auth.TokenSource
	if cred := os.Getenv("OPENEHR_LIVE_EHRBASE_BASIC"); cred != "" {
		user, pass, ok := strings.Cut(cred, ":")
		if !ok || user == "" {
			t.Fatal("OPENEHR_LIVE_EHRBASE_BASIC must be user:pass")
		}
		s, err := basic.New(user, pass)
		if err != nil {
			t.Fatal(err)
		}
		src = s
	}

	id := openehrclient.EHRID(newUUIDv4(t))
	t.Logf("per-run EHR id %s", id)

	sum, err := probe.Run(t.Context(), probe.Config{
		Mode:          probe.ModeLive,
		Endpoint:      base,
		HTTPClient:    &http.Client{Timeout: 10 * time.Second},
		TokenSource:   src,
		AllowMutating: true,
	}, coreLiveEntries(id))

	t.Logf("live core snapshot vs %s — %d passed, %d skipped, %d failed of %d",
		base, sum.Passed, sum.Skipped, sum.Failed, sum.Selected)
	for _, r := range sum.Results {
		detail := r.Detail
		if detail == "" {
			detail = "-"
		}
		t.Logf("  %-20s %-4s %s", r.Probe, r.Status, detail)
	}

	if err != nil && !errors.Is(err, probe.ErrAllSkipped) {
		t.Errorf("live core snapshot probe error(s): %v", err)
	}
	if sum.Failed > 0 {
		t.Errorf("live core snapshot: %d of %d probes failed against %s", sum.Failed, sum.Selected, base)
	}
}
