package probe_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/basic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus"
	"github.com/cadasto/openehr-sdk-go/openehr/client/system"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func liveFail(err error) probe.Result {
	return probe.Result{Status: probe.StatusFail, Detail: err.Error()}
}

func liveFailf(format string, a ...any) probe.Result {
	return probe.Result{Status: probe.StatusFail, Detail: fmt.Sprintf(format, a...)}
}

// safeBase strips any userinfo from a base URL before it reaches a test log:
// the SDK's live credential path is OPENEHR_LIVE_EHRBASE_BASIC, kept out of the
// URL, so a credential embedded in OPENEHR_LIVE_EHRBASE never leaks to output.
func safeBase(base string) string {
	if u, err := url.Parse(base); err == nil && u.User != nil {
		u.User = nil
		return u.String()
	}
	return base
}

// coreLiveEntries is the self-scoping core read/write suite for a Live snapshot:
// one mutating create against the per-run id, then read-only probes against
// that same id. They share id by closure — declared here, never observed from
// the transport (REQ-082) — and the runner executes them in order against one
// deployment. A read probe would need to skip when its precondition is absent;
// here the create earlier in the run is that precondition.
//
// Every entry is Live-only. The closure-shared EHR would not survive the
// runner's per-probe sandbox isolation, and ehr_status / OPTIONS / are not
// sandbox routes, so declaring Sandbox would be a false Modes claim against
// REQ-082's cross-mode-agreement rule (the runner requires the same verdict in
// every mode a probe lists). The sandbox create/get/head path is exercised by
// TestSandboxCreateEHRThroughRunner.
func coreLiveEntries(id openehrclient.EHRID) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-EHR-CREATE",
			Effect: probe.EffectMutating,
			Modes:  live,
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
			Modes:  live,
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
			Modes:  live,
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
			Modes:  live,
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
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				v, err := system.Version(ctx, c)
				if err != nil {
					// The probe's precondition is that the deployment exposes the
					// System capabilities endpoint (OPTIONS /) — the spec-defined
					// "Options and Conformance" operation (system-validation.openapi.yaml,
					// SHOULD-level). A 404 is that endpoint being absent, so it is
					// an unmet precondition (REQ-082 skip), not a failed assertion.
					// EHRbase 2.35.1 does not implement it — an accepted deployment
					// deviation recorded in conformance.md § REQ-082. The probe
					// passes against a deployment that serves it.
					if errors.Is(err, transport.ErrNotFound) {
						return probe.Result{Status: probe.StatusSkip, Detail: "precondition absent: deployment does not expose the System capabilities endpoint (OPTIONS /) — spec-defined, EHRbase 2.35.1 does not implement it (accepted deviation)"}, nil
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
// REQ-082 requires. It fails unless the run is green — an all-skipped or
// failing run is not a pass (REQ-082) — so it doubles as a Live gate for the
// core flows.
func TestLiveCoreSnapshot(t *testing.T) {
	base := os.Getenv("OPENEHR_LIVE_EHRBASE")
	if base == "" {
		t.Skip("set OPENEHR_LIVE_EHRBASE to a live openEHR REST base to run the core Live snapshot; not part of CI")
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

	id := openehrclient.EHRID(uuid.NewV4().String())
	t.Logf("per-run EHR id %s", id)

	hc := &http.Client{Timeout: 10 * time.Second}
	cfg := probe.Config{Mode: probe.ModeLive, Endpoint: base, HTTPClient: hc, TokenSource: src}

	// A live base alone does not authorise writing: exercise the mutating
	// refusal before skipping, so the opt-in guard is pinned even when the env
	// switch is unset (mirrors TestLiveCreateEHR — the create entry is mutating,
	// so Run refuses the whole suite up front).
	if os.Getenv(envAllowMutating) == "" {
		if _, err := probe.Run(t.Context(), cfg, coreLiveEntries(id)); !errors.Is(err, probe.ErrMutatingNotOptedIn) {
			t.Fatalf("Run without the mutating opt-in = %v, want %v", err, probe.ErrMutatingNotOptedIn)
		}
		t.Skipf("set %s to let the snapshot's create probe write to %s", envAllowMutating, safeBase(base))
	}
	cfg.AllowMutating = true

	// A bad OPENEHR_LIVE_EHRBASE is a clean skip, not a wall of probe failures.
	reach, err := probe.NewClient(base, hc, src)
	if err != nil {
		t.Fatal(err)
	}
	if !liveReachable(t, reach) {
		t.Skipf("deployment not reachable at %s", safeBase(base))
	}

	sum, err := probe.Run(t.Context(), cfg, coreLiveEntries(id))

	t.Logf("live core snapshot vs %s — %d passed, %d skipped, %d failed of %d",
		safeBase(base), sum.Passed, sum.Skipped, sum.Failed, sum.Selected)
	for _, r := range sum.Results {
		detail := r.Detail
		if detail == "" {
			detail = "-"
		}
		t.Logf("  %-20s %-4s %s", r.Probe, r.Status, detail)
	}

	// Green() is Passed>0 && Failed==0, so this catches both a vacuous all-skip
	// (sum.Failed==0 alone would miss it) and any probe failure; err carries the
	// run's ErrAllSkipped / joined probe errors for the message (REQ-082).
	if !sum.Green() {
		t.Errorf("live core snapshot vs %s is not green: %d passed, %d skipped, %d failed of %d (run err: %v)",
			safeBase(base), sum.Passed, sum.Skipped, sum.Failed, sum.Selected, err)
	}
}
