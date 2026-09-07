package probe_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// sandboxClient is a caller-built client whose transport is the
// in-memory sandbox — the shape the runner must refuse under the Live
// and Cassette labels.
func sandboxClient(t *testing.T) *transport.Client {
	t.Helper()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", sandbox.New().HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// capture records the client each probe was handed.
func capture(id string, seen *[]*transport.Client) probe.Entry {
	return probe.Entry{
		ID:     id,
		Effect: probe.EffectReadOnly,
		Run: func(_ context.Context, c *transport.Client) (probe.Result, error) {
			*seen = append(*seen, c)
			return probe.Result{Status: probe.StatusPass}, nil
		},
	}
}

// TestSandboxModeBuildsItsOwnClient pins that Sandbox mode is a real
// backend choice: with no Client and no Sandbox configured the runner
// still hands the probe a working sandbox-backed transport, and it
// builds exactly one for the whole run.
func TestSandboxModeBuildsItsOwnClient(t *testing.T) {
	t.Parallel()
	var seen []*transport.Client
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{
		capture("PROBE-070", &seen),
		capture("PROBE-071", &seen),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Green() {
		t.Fatal("Summary.Green() = false for a sandbox run the runner wired itself")
	}
	if len(seen) != 2 {
		t.Fatalf("probes saw %d clients, want 2", len(seen))
	}
	if seen[0] == nil {
		t.Fatal("the probe was handed a nil client; sandbox mode must build one")
	}
	if seen[0] != seen[1] {
		t.Fatal("each probe got its own client; the runner must build one per run")
	}
	if _, ok := seen[0].HTTPClient().Transport.(*sandbox.Backend); !ok {
		t.Fatalf("sandbox-mode transport = %T, want *sandbox.Backend", seen[0].HTTPClient().Transport)
	}
}

// TestSandboxModeUsesTheConfiguredBackend pins that Config.Sandbox is
// the backend the run serves from: a second run against the same
// backend sees what the first wrote. A runner that ignored the field
// and made a fresh backend would report the EHR absent.
func TestSandboxModeUsesTheConfiguredBackend(t *testing.T) {
	t.Parallel()
	backend := sandbox.New()
	const id openehrclient.EHRID = "6c1a09d6-2b1e-4a0f-8d4d-0f2a9c7b31aa"
	cfg := probe.Config{Mode: probe.ModeSandbox, Sandbox: backend}

	create := probe.Entry{
		ID:     "PROBE-072",
		Effect: probe.EffectMutating,
		Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
			if _, _, err := openehrclient.Create(ctx, c, openehrclient.WithEHRID(id)); err != nil {
				return probe.Result{}, err
			}
			return probe.Result{Status: probe.StatusPass}, nil
		},
	}
	if _, err := probe.Run(t.Context(), cfg, []probe.Entry{create}); err != nil {
		t.Fatal(err)
	}

	readBack := probe.Entry{
		ID:     "PROBE-073",
		Effect: probe.EffectReadOnly,
		Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
			ok, err := openehrclient.Exists(ctx, c, id)
			if err != nil {
				return probe.Result{}, err
			}
			if !ok {
				return probe.Result{Status: probe.StatusFail, Detail: "the configured backend did not keep the EHR"}, nil
			}
			return probe.Result{Status: probe.StatusPass}, nil
		},
	}
	sum, err := probe.Run(t.Context(), cfg, []probe.Entry{readBack})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Green() {
		t.Fatalf("second run against the configured backend: %s", sum.Results[0].Detail)
	}
}

// TestSandboxModeRefusesACallerClient pins that Sandbox mode will not
// accept a client the caller built: a live client under the Sandbox
// label would reach a real deployment while the summary said sandbox.
func TestSandboxModeRefusesACallerClient(t *testing.T) {
	t.Parallel()
	var seen []*transport.Client
	_, err := probe.Run(t.Context(), probe.Config{
		Mode:   probe.ModeSandbox,
		Client: mustClient(t),
	}, []probe.Entry{capture("PROBE-074", &seen)})
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("Run() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
	}
	if len(seen) != 0 {
		t.Fatal("the probe ran despite the refusal")
	}
}

// TestSandboxTransportRefusedUnderOtherModes pins that a sandbox-backed
// client cannot ride under the Live or Cassette label. Without the
// guard the run would report a deployment or a recording answered when
// nothing left the process.
func TestSandboxTransportRefusedUnderOtherModes(t *testing.T) {
	t.Parallel()
	entry := stub("PROBE-010", probe.StatusPass, false, probe.EffectReadOnly)
	cases := []struct {
		name string
		cfg  probe.Config
	}{
		{
			name: "live with a sandbox client",
			cfg: probe.Config{
				Mode:          probe.ModeLive,
				Client:        sandboxClient(t),
				AllowMutating: true,
			},
		},
		{
			name: "live built from a sandbox HTTP client",
			cfg: probe.Config{
				Mode:          probe.ModeLive,
				Endpoint:      "https://cdr.example.com/openehr/v1",
				HTTPClient:    sandbox.New().HTTPClient(),
				AllowMutating: true,
			},
		},
		{
			name: "cassette with a sandbox client",
			cfg: probe.Config{
				Mode:         probe.ModeCassette,
				Client:       sandboxClient(t),
				RecordingDir: recordingDir(t, "PROBE-010"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := probe.Run(t.Context(), tc.cfg, []probe.Entry{entry})
			if !errors.Is(err, probe.ErrUnsatisfiableMode) {
				t.Fatalf("Run() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
			}
		})
	}
}

// TestLiveModeBuildsFromEndpoint pins that Live mode no longer needs a
// caller-built client: Endpoint plus the injected HTTP client is
// enough, and Config.Endpoint is actually wired rather than decorative.
func TestLiveModeBuildsFromEndpoint(t *testing.T) {
	t.Parallel()
	var seen []*transport.Client
	sum, err := probe.Run(t.Context(), probe.Config{
		Mode:       probe.ModeLive,
		Endpoint:   "https://cdr.example.com/openehr/v1",
		HTTPClient: &http.Client{},
	}, []probe.Entry{capture("PROBE-075", &seen)})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Green() {
		t.Fatal("Summary.Green() = false for a live run built from the endpoint")
	}
	if len(seen) != 1 || seen[0] == nil {
		t.Fatalf("probes saw %v, want one non-nil client built from Endpoint", seen)
	}
	svc, ok := seen[0].Catalog().OpenEHRRest()
	if !ok {
		t.Fatal("the built client carries no openEHR REST service entry")
	}
	if got := svc.BaseURL.String(); got != "https://cdr.example.com/openehr/v1" {
		t.Fatalf("client base URL = %q, want the configured endpoint", got)
	}
}

// TestInRepoProbeGetsNoClient pins that an in-repo probe is handed
// nothing to reach a backend with, whatever mode the run is in.
func TestInRepoProbeGetsNoClient(t *testing.T) {
	t.Parallel()
	var seen []*transport.Client
	e := capture("PROBE-076", &seen)
	e.InRepo = true
	e.Effect = ""
	if _, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{e}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != nil {
		t.Fatalf("in-repo probe saw %v, want a nil client", seen)
	}
}

// TestNewClientEmptyBaseURL pins that a missing base URL is a
// configuration error, not a mode that cannot be satisfied.
func TestNewClientEmptyBaseURL(t *testing.T) {
	t.Parallel()
	_, err := probe.NewClient("", &http.Client{}, nil)
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Fatalf("NewClient(\"\") error = %v, want %v", err, transport.ErrInvalidConfig)
	}
	if errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatal("NewClient(\"\") reported an unsatisfiable mode; an empty base URL is invalid configuration")
	}
}
