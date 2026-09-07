package probe_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func stub(id string, status probe.Status, inRepo bool, effect probe.Effect) probe.Entry {
	return probe.Entry{
		ID:     id,
		InRepo: inRepo,
		Effect: effect,
		Run: func(context.Context, *transport.Client) (probe.Result, error) {
			return probe.Result{Probe: id, Status: status}, nil
		},
	}
}

func mustClient(t *testing.T) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("https://test.example.com/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(&http.Client{}))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRun_AllSkippedIsNotGreen(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{
		stub("PROBE-001", probe.StatusSkip, true, ""),
		stub("PROBE-002", probe.StatusSkip, true, ""),
	})
	if !errors.Is(err, probe.ErrAllSkipped) {
		t.Fatalf("Run() error = %v, want %v", err, probe.ErrAllSkipped)
	}
	if sum.Green() {
		t.Fatal("Summary.Green() = true for an all-skipped run; vacuous success is not green")
	}
	if sum.Passed != 0 || sum.Skipped != 2 {
		t.Fatalf("passed=%d skipped=%d, want passed=0 skipped=2", sum.Passed, sum.Skipped)
	}
}

func TestRun_UnsatisfiableModeFails(t *testing.T) {
	t.Parallel()
	backend := stub("PROBE-010", probe.StatusPass, false, probe.EffectReadOnly)
	cases := []struct {
		name string
		cfg  probe.Config
		ids  []string
	}{
		{
			name: "cassette without directory",
			cfg:  probe.Config{Mode: probe.ModeCassette, Client: mustClient(t)},
		},
		{
			name: "cassette without recording for probe",
			cfg: probe.Config{
				Mode:         probe.ModeCassette,
				Client:       mustClient(t),
				RecordingDir: t.TempDir(),
			},
		},
		{
			name: "live without endpoint or client",
			cfg:  probe.Config{Mode: probe.ModeLive},
		},
		{
			name: "sandbox without client",
			cfg:  probe.Config{Mode: probe.ModeSandbox},
		},
		{
			name: "unknown mode",
			cfg:  probe.Config{Mode: probe.Mode("tape")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := probe.Run(t.Context(), tc.cfg, []probe.Entry{backend}, tc.ids...)
			if !errors.Is(err, probe.ErrUnsatisfiableMode) {
				t.Fatalf("Run() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
			}
		})
	}
}

func TestRun_SkipNotCountedAsPass(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{
		stub("PROBE-001", probe.StatusPass, true, ""),
		stub("PROBE-002", probe.StatusSkip, true, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Passed != 1 {
		t.Fatalf("Passed = %d, want 1 (skip must not increment pass)", sum.Passed)
	}
	if sum.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", sum.Skipped)
	}
	if !sum.Green() {
		t.Fatal("Summary.Green() = false; one pass and one skip is still green")
	}
}

func TestUnclassifiedEffectIsMutating(t *testing.T) {
	t.Parallel()
	if got := probe.ResolveEffect(""); got != probe.EffectMutating {
		t.Fatalf("ResolveEffect(\"\") = %q, want %q", got, probe.EffectMutating)
	}
	if got := probe.ResolveEffect("unknown"); got != probe.EffectMutating {
		t.Fatalf("ResolveEffect(\"unknown\") = %q, want %q", got, probe.EffectMutating)
	}
	if got := probe.ResolveEffect(probe.EffectReadOnly); got != probe.EffectReadOnly {
		t.Fatalf("ResolveEffect(read-only) = %q, want read-only", got)
	}

	// Live mode refuses an unclassified (hence mutating) probe without opt-in.
	_, err := probe.Run(t.Context(), probe.Config{
		Mode:   probe.ModeLive,
		Client: mustClient(t),
	}, []probe.Entry{
		stub("PROBE-099", probe.StatusPass, false, ""),
	})
	if !errors.Is(err, probe.ErrMutatingNotOptedIn) {
		t.Fatalf("unclassified live run error = %v, want %v", err, probe.ErrMutatingNotOptedIn)
	}
}

func TestRun_SubsetAndSingle(t *testing.T) {
	t.Parallel()
	catalog := []probe.Entry{
		stub("PROBE-001", probe.StatusPass, true, ""),
		stub("PROBE-002", probe.StatusFail, true, ""),
		stub("PROBE-003", probe.StatusPass, true, ""),
	}
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, catalog, "PROBE-001")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 1 || sum.Passed != 1 || len(sum.Results) != 1 {
		t.Fatalf("single-probe run: selected=%d passed=%d results=%d", sum.Selected, sum.Passed, len(sum.Results))
	}
	if sum.Results[0].Mode != probe.ModeInRepo {
		t.Fatalf("in-repo result mode = %q, want %q", sum.Results[0].Mode, probe.ModeInRepo)
	}

	sum, err = probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, catalog, "PROBE-001", "PROBE-003")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 2 || sum.Passed != 2 {
		t.Fatalf("subset run: selected=%d passed=%d", sum.Selected, sum.Passed)
	}
}

func TestRun_CassetteSatisfiedWhenRecordingPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "PROBE-010.yaml")
	if err := os.WriteFile(path, []byte("# recording placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := probe.Run(t.Context(), probe.Config{
		Mode:         probe.ModeCassette,
		Client:       mustClient(t),
		RecordingDir: dir,
	}, []probe.Entry{
		stub("PROBE-010", probe.StatusPass, false, probe.EffectReadOnly),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Green() || sum.Results[0].Mode != probe.ModeCassette {
		t.Fatalf("cassette run green=%v mode=%q", sum.Green(), sum.Results[0].Mode)
	}
}

func TestParseModes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []probe.Mode
	}{
		{"Sandbox, Cassette, Live.", []probe.Mode{probe.ModeSandbox, probe.ModeCassette, probe.ModeLive}},
		{"In-repo (unit-level property; no backend).", []probe.Mode{probe.ModeInRepo}},
		{"Sandbox (planned); Cassette, Live not yet scoped.", []probe.Mode{probe.ModeSandbox}},
	}
	for _, tc := range cases {
		got := probe.ParseModes(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("ParseModes(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("ParseModes(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
