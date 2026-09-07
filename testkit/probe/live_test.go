package probe_test

import (
	"context"
	"errors"
	"net"
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
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// envAllowMutating is the separate, explicit opt-in a mutating Live
// run needs. Pointing OPENEHR_LIVE_* at a deployment says where to
// look; this says the run may write there.
const envAllowMutating = "OPENEHR_LIVE_ALLOW_MUTATING"

// liveTargets are opt-in Live-mode CDRs. A target runs only when its
// env var is set; GitHub CI and `make test` leave them unset, so these
// subtests skip without dialing anyone (REQ-082: Live is pre-release,
// Sandbox is the CI default). credEnv, when set, names the variable
// carrying that target's basic-auth credential as user:pass — no
// credential is hard-coded here.
var liveTargets = []struct {
	name    string
	env     string
	credEnv string
}{
	{name: "ehrbase", env: "OPENEHR_LIVE_EHRBASE"},
	{name: "ferroehr", env: "OPENEHR_LIVE_FERROEHR", credEnv: "OPENEHR_LIVE_FERROEHR_BASIC"},
}

// createEHRProbe creates one EHR under a caller-supplied identifier,
// so a Live run is self-scoping: everything it writes is derived from
// the per-run id the test generated and logged (REQ-082).
func createEHRProbe(id openehrclient.EHRID) probe.Entry {
	return probe.Entry{
		ID:     "PROBE-LIVE-EHR-CREATE",
		Effect: probe.EffectMutating,
		Modes:  []probe.Mode{probe.ModeSandbox, probe.ModeLive},
		Run: func(ctx context.Context, cl *transport.Client) (probe.Result, error) {
			rec, _, err := openehrclient.Create(ctx, cl, openehrclient.WithEHRID(id))
			if err != nil {
				return probe.Result{Status: probe.StatusFail, Detail: err.Error()}, nil
			}
			if rec == nil || rec.EHRID.Value == "" {
				return probe.Result{Status: probe.StatusFail, Detail: "create returned no ehr_id"}, nil
			}
			return probe.Result{Status: probe.StatusPass, Detail: rec.EHRID.Value}, nil
		},
	}
}

func TestLiveCreateEHR(t *testing.T) {
	for _, target := range liveTargets {
		t.Run(target.name, func(t *testing.T) {
			// The env check comes before any client is built, so an
			// unset variable skips without a dial.
			base := os.Getenv(target.env)
			if base == "" {
				t.Skipf("set %s to a live openEHR REST base to run this probe; it is not part of CI", target.env)
			}
			var src auth.TokenSource
			if target.credEnv != "" {
				cred := os.Getenv(target.credEnv)
				if cred == "" {
					t.Skipf("set %s to %s's basic-auth credential as user:pass", target.credEnv, target.name)
				}
				user, pass, ok := strings.Cut(cred, ":")
				if !ok || user == "" {
					t.Fatalf("%s must be user:pass", target.credEnv)
				}
				s, err := basic.New(user, pass)
				if err != nil {
					t.Fatal(err)
				}
				src = s
			}

			// One identifier per run, logged, so anything left on the
			// deployment is attributable to this run.
			id := openehrclient.EHRID(uuid.NewV4().String())
			t.Logf("%s: per-run EHR id %s", target.name, id)

			hc := &http.Client{Timeout: 5 * time.Second}
			cfg := probe.Config{
				Mode:        probe.ModeLive,
				Endpoint:    base,
				HTTPClient:  hc,
				TokenSource: src,
			}

			// A live base alone does not authorise writing. Exercise the
			// refusal, then skip naming the switch that lifts it.
			if os.Getenv(envAllowMutating) == "" {
				_, err := probe.Run(t.Context(), cfg, []probe.Entry{createEHRProbe(id)})
				if !errors.Is(err, probe.ErrMutatingNotOptedIn) {
					t.Fatalf("Run() without the opt-in error = %v, want %v", err, probe.ErrMutatingNotOptedIn)
				}
				t.Skipf("set %s (non-empty) to let the mutating live probe write to %s", envAllowMutating, target.env)
			}
			cfg.AllowMutating = true

			reach, err := probe.NewClient(base, hc, src)
			if err != nil {
				t.Fatal(err)
			}
			if !liveReachable(t, reach) {
				t.Skipf("%s not reachable at %s", target.name, base)
			}

			sum, err := probe.Run(t.Context(), cfg, []probe.Entry{createEHRProbe(id)})
			if err != nil {
				t.Fatal(err)
			}
			if !sum.Green() {
				t.Fatalf("live %s: %s", target.name, sum.Results[0].Detail)
			}
			t.Logf("%s created EHR %s", target.name, sum.Results[0].Detail)
		})
	}
}

func TestSandboxCreateEHRThroughRunner(t *testing.T) {
	t.Parallel()
	id := openehrclient.EHRID(uuid.NewV4().String())
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{createEHRProbe(id)})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Green() {
		t.Fatalf("sandbox runner: %s", sum.Results[0].Detail)
	}
	if got := sum.Results[0].Detail; got != string(id) {
		t.Fatalf("created EHR id = %q, want the per-run id %q", got, id)
	}
}

// liveReachable reports whether the deployment answered at all. The
// distinction is made on error type, never on message text: a
// transport-level failure means nothing reached a server, while any
// error that came back over the wire — a typed 404, an auth refusal —
// proves one is there.
func liveReachable(t *testing.T, c *transport.Client) bool {
	t.Helper()
	_, err := openehrclient.Exists(t.Context(), c, "00000000-0000-4000-8000-000000000000")
	if err == nil {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if ue, ok := errors.AsType[*url.Error](err); ok && ue != nil {
		return false
	}
	if oe, ok := errors.AsType[*net.OpError](err); ok && oe != nil {
		return false
	}
	if ne, ok := errors.AsType[net.Error](err); ok && ne != nil && ne.Timeout() {
		return false
	}
	return true
}
