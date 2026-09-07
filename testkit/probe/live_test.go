package probe_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/basic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// liveTargets are opt-in Live-mode CDRs. A target runs only when its
// env var is set; GitHub CI and `make test` leave them unset, so these
// subtests skip without dialing anyone (REQ-082: Live is pre-release,
// Sandbox is the CI default).
var liveTargets = []struct {
	name string
	env  string
	user string
	pass string
}{
	{name: "ehrbase", env: "OPENEHR_LIVE_EHRBASE"},
	{name: "ferroehr", env: "OPENEHR_LIVE_FERROEHR", user: "ferroehr", pass: "ferroehr"},
}

func createEHRProbe() probe.Entry {
	return probe.Entry{
		ID:     "PROBE-LIVE-EHR-CREATE",
		Effect: probe.EffectMutating,
		Run: func(ctx context.Context, cl *transport.Client) (probe.Result, error) {
			rec, _, err := openehrclient.Create(ctx, cl)
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
			base := os.Getenv(target.env)
			if base == "" {
				t.Skipf("set %s to a live openEHR REST base to run this probe; it is not part of CI", target.env)
			}
			var src auth.TokenSource
			if target.user != "" {
				s, err := basic.New(target.user, target.pass)
				if err != nil {
					t.Fatal(err)
				}
				src = s
			}
			c, err := probe.NewClient(base, &http.Client{Timeout: 5 * time.Second}, src)
			if err != nil {
				t.Fatal(err)
			}
			if !liveReachable(t, c) {
				t.Skipf("%s not reachable at %s", target.name, base)
			}
			sum, err := probe.Run(t.Context(), probe.Config{
				Mode:          probe.ModeLive,
				Client:        c,
				Endpoint:      base,
				AllowMutating: true,
			}, []probe.Entry{createEHRProbe()})
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
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", sandbox.New().HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox, Client: c}, []probe.Entry{createEHRProbe()})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Green() {
		t.Fatalf("sandbox runner: %s", sum.Results[0].Detail)
	}
}

func liveReachable(t *testing.T, c *transport.Client) bool {
	t.Helper()
	_, err := openehrclient.Exists(t.Context(), c, "00000000-0000-4000-8000-000000000000")
	if err == nil {
		return true
	}
	msg := err.Error()
	return !strings.Contains(msg, "connection refused") &&
		!strings.Contains(msg, "no such host") &&
		!strings.Contains(msg, "timeout") &&
		!strings.Contains(msg, "dial tcp")
}
