package probe_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// stubDetail is stub with a detail string, for the skip rules where an
// empty detail is the thing under test.
func stubDetail(id string, status probe.Status, detail string) probe.Entry {
	e := stub(id, status, true, "")
	e.Run = func(context.Context, *transport.Client) (probe.Result, error) {
		return probe.Result{Probe: id, Status: status, Detail: detail}, nil
	}
	return e
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
		stubDetail("PROBE-001", probe.StatusSkip, "no template loaded"),
		stubDetail("PROBE-002", probe.StatusSkip, "no template loaded"),
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
			name: "cassette without client",
			cfg: probe.Config{
				Mode:         probe.ModeCassette,
				RecordingDir: recordingDir(t, "PROBE-010"),
			},
		},
		{
			name: "live without endpoint or client",
			cfg:  probe.Config{Mode: probe.ModeLive},
		},
		{
			name: "unknown mode",
			cfg:  probe.Config{Mode: probe.Mode("tape")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := probe.Run(t.Context(), tc.cfg, []probe.Entry{backend})
			if !errors.Is(err, probe.ErrUnsatisfiableMode) {
				t.Fatalf("Run() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
			}
		})
	}
}

// TestRun_InRepoModeRefusesBackendProbe pins the ModeInRepo arm of the
// mode switch: in-repo mode has no backend, so a backend-facing entry
// asked for it is unsatisfiable rather than silently client-less.
func TestRun_InRepoModeRefusesBackendProbe(t *testing.T) {
	t.Parallel()
	_, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeInRepo}, []probe.Entry{
		stub("PROBE-010", probe.StatusPass, false, probe.EffectReadOnly),
	})
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("Run() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
	}
	// The refusal must come from the per-probe check and name the
	// offending entry, not from the later client-building fallback:
	// the caller needs to know which probe cannot run here.
	if !strings.Contains(err.Error(), "PROBE-010") {
		t.Fatalf("Run() error = %q, want it to name the backend-facing probe", err)
	}
}

// TestRun_DeclaredModesAreEnforced pins Entry.Modes as a filter: an
// entry that names only Sandbox must be refused under Live rather than
// run against a deployment it never declared support for.
func TestRun_DeclaredModesAreEnforced(t *testing.T) {
	t.Parallel()
	e := stub("PROBE-020", probe.StatusPass, false, probe.EffectReadOnly)
	e.Modes = []probe.Mode{probe.ModeSandbox}

	_, err := probe.Run(t.Context(), probe.Config{
		Mode:   probe.ModeLive,
		Client: mustClient(t),
	}, []probe.Entry{e})
	if !errors.Is(err, probe.ErrUnsatisfiableMode) {
		t.Fatalf("Run() error = %v, want %v", err, probe.ErrUnsatisfiableMode)
	}

	// The same entry under the mode it declares runs.
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{e})
	if err != nil {
		t.Fatalf("Run() under a declared mode: %v", err)
	}
	if !sum.Green() {
		t.Fatal("Summary.Green() = false for a probe run under its declared mode")
	}
}

// TestRun_InvalidEntryRefused pins the catalog validation: each shape
// the runner cannot execute is refused with ErrInvalidEntry before
// anything runs, and a nil Run function does not panic (REQ-025).
func TestRun_InvalidEntryRefused(t *testing.T) {
	t.Parallel()
	noRun := probe.Entry{ID: "PROBE-030", InRepo: true}
	inRepoWithBackendMode := stub("PROBE-031", probe.StatusPass, true, "")
	inRepoWithBackendMode.Modes = []probe.Mode{probe.ModeSandbox}
	backendClaimingInRepo := stub("PROBE-032", probe.StatusPass, false, probe.EffectReadOnly)
	backendClaimingInRepo.Modes = []probe.Mode{probe.ModeInRepo}

	cases := []struct {
		name    string
		entries []probe.Entry
	}{
		{name: "nil Run", entries: []probe.Entry{noRun}},
		{name: "empty id", entries: []probe.Entry{stub("", probe.StatusPass, true, "")}},
		{name: "duplicate id", entries: []probe.Entry{
			stub("PROBE-033", probe.StatusPass, true, ""),
			stub("PROBE-033", probe.StatusPass, true, ""),
		}},
		{name: "in-repo declaring a backend mode", entries: []probe.Entry{inRepoWithBackendMode}},
		{name: "backend-facing declaring in-repo", entries: []probe.Entry{backendClaimingInRepo}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, tc.entries)
			if !errors.Is(err, probe.ErrInvalidEntry) {
				t.Fatalf("Run() error = %v, want %v", err, probe.ErrInvalidEntry)
			}
			if len(sum.Results) != 0 {
				t.Fatalf("Run() recorded %d results for an invalid catalog; nothing must run", len(sum.Results))
			}
		})
	}
}

// TestSelect_Sentinels pins the two selection refusals: an id the
// catalog does not carry, and an empty id list that must not widen
// into the whole catalog.
func TestSelect_Sentinels(t *testing.T) {
	t.Parallel()
	catalog := []probe.Entry{
		stub("PROBE-001", probe.StatusPass, true, ""),
		stub("PROBE-002", probe.StatusPass, true, ""),
	}

	if _, err := probe.Select(catalog, "PROBE-404"); !errors.Is(err, probe.ErrUnknownProbe) {
		t.Fatalf("Select(unknown id) error = %v, want %v", err, probe.ErrUnknownProbe)
	}
	got, err := probe.Select(catalog)
	if !errors.Is(err, probe.ErrEmptySelection) {
		t.Fatalf("Select(no ids) error = %v, want %v", err, probe.ErrEmptySelection)
	}
	if len(got) != 0 {
		t.Fatalf("Select(no ids) returned %d entries; an empty filter must not mean the whole catalog", len(got))
	}

	sel, err := probe.Select(catalog, "PROBE-002")
	if err != nil {
		t.Fatal(err)
	}
	if len(sel) != 1 || sel[0].ID != "PROBE-002" {
		t.Fatalf("Select(\"PROBE-002\") = %v, want the single named entry", sel)
	}
}

// TestRun_EmptySelectionRefused pins that Run runs exactly what it is
// given: no entries is a refusal, not a whole-catalog run and not a
// green one.
func TestRun_EmptySelectionRefused(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, nil)
	if !errors.Is(err, probe.ErrEmptySelection) {
		t.Fatalf("Run(no entries) error = %v, want %v", err, probe.ErrEmptySelection)
	}
	if errors.Is(err, probe.ErrAllSkipped) {
		t.Fatal("Run(no entries) reported all-skipped; an empty selection is a caller mistake, not a skip")
	}
	if sum.Green() {
		t.Fatal("Summary.Green() = true for an empty selection")
	}
}

// TestRun_ProbeErrorIsRecordedAndRunContinues pins that a probe that
// returns an error cannot leave a green partial summary: it is
// recorded as a failure, the remaining probes still run, and the
// error itself reaches the caller.
func TestRun_ProbeErrorIsRecordedAndRunContinues(t *testing.T) {
	t.Parallel()
	boom := errors.New("backend exploded")
	second := false
	entries := []probe.Entry{
		{
			ID:     "PROBE-040",
			InRepo: true,
			Run: func(context.Context, *transport.Client) (probe.Result, error) {
				return probe.Result{}, boom
			},
		},
		{
			ID:     "PROBE-041",
			InRepo: true,
			Run: func(context.Context, *transport.Client) (probe.Result, error) {
				second = true
				return probe.Result{Status: probe.StatusPass}, nil
			},
		},
	}
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, entries)
	if !errors.Is(err, boom) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, boom)
	}
	if !second {
		t.Fatal("the second probe did not run; an erroring probe must not abandon the rest of the selection")
	}
	if len(sum.Results) != 2 {
		t.Fatalf("Summary.Results has %d entries, want 2 (both probes recorded)", len(sum.Results))
	}
	if sum.Green() {
		t.Fatal("Summary.Green() = true after a probe error; a partial summary must not read as green")
	}
	if sum.Failed != 1 || sum.Passed != 1 {
		t.Fatalf("failed=%d passed=%d, want failed=1 passed=1", sum.Failed, sum.Passed)
	}
	if got := sum.Results[0]; got.Probe != "PROBE-040" || got.Status != probe.StatusFail {
		t.Fatalf("first result = %+v, want PROBE-040 recorded as a failure", got)
	}
	if want := "probe error: " + boom.Error(); sum.Results[0].Detail != want {
		t.Fatalf("first result detail = %q, want %q", sum.Results[0].Detail, want)
	}
}

// TestRun_UnrecognizedStatusBecomesFailure pins the canonicalisation
// point: a status outside pass/fail/skip is a failure whose detail
// keeps the offending text.
func TestRun_UnrecognizedStatusBecomesFailure(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{
		stubDetail("PROBE-050", probe.Status("inconclusive"), ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Failed != 1 || sum.Passed != 0 || sum.Skipped != 0 {
		t.Fatalf("failed=%d passed=%d skipped=%d, want failed=1", sum.Failed, sum.Passed, sum.Skipped)
	}
	if got, want := sum.Results[0].Detail, `unrecognized status "inconclusive"`; got != want {
		t.Fatalf("detail = %q, want %q (the offending text must survive)", got, want)
	}
	if sum.Green() {
		t.Fatal("Summary.Green() = true with an unrecognised status")
	}
}

// TestRun_SkipWithoutPreconditionBecomesFailure pins REQ-082's rule
// that a skip must name what was missing: an unexplained skip would
// otherwise buy silence for free.
func TestRun_SkipWithoutPreconditionBecomesFailure(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{
		stubDetail("PROBE-060", probe.StatusSkip, "   "),
		stubDetail("PROBE-061", probe.StatusSkip, "no live endpoint configured"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Failed != 1 || sum.Skipped != 1 {
		t.Fatalf("failed=%d skipped=%d, want failed=1 skipped=1", sum.Failed, sum.Skipped)
	}
	if got, want := sum.Results[0].Detail, "skip without a named precondition (REQ-082)"; got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
	if sum.Results[1].Status != probe.StatusSkip {
		t.Fatalf("a skip that names its precondition = %q, want it left as a skip", sum.Results[1].Status)
	}
}

func TestRun_SkipNotCountedAsPass(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, []probe.Entry{
		stubDetail("PROBE-001", probe.StatusPass, ""),
		stubDetail("PROBE-002", probe.StatusSkip, "nothing to compare against"),
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
	single, err := probe.Select(catalog, "PROBE-001")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, single)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 1 || sum.Passed != 1 || len(sum.Results) != 1 {
		t.Fatalf("single-probe run: selected=%d passed=%d results=%d", sum.Selected, sum.Passed, len(sum.Results))
	}
	if sum.Results[0].Mode != probe.ModeInRepo {
		t.Fatalf("in-repo result mode = %q, want %q", sum.Results[0].Mode, probe.ModeInRepo)
	}

	subset, err := probe.Select(catalog, "PROBE-001", "PROBE-003")
	if err != nil {
		t.Fatal(err)
	}
	sum, err = probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, subset)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 2 || sum.Passed != 2 {
		t.Fatalf("subset run: selected=%d passed=%d", sum.Selected, sum.Passed)
	}

	// The whole catalog is spelled Run(ctx, cfg, catalog).
	sum, err = probe.Run(t.Context(), probe.Config{Mode: probe.ModeSandbox}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 3 {
		t.Fatalf("whole-catalog run: selected=%d, want 3", sum.Selected)
	}
}

func TestRun_CassetteSatisfiedWhenRecordingPresent(t *testing.T) {
	t.Parallel()
	sum, err := probe.Run(t.Context(), probe.Config{
		Mode:         probe.ModeCassette,
		Client:       mustClient(t),
		RecordingDir: recordingDir(t, "PROBE-010"),
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

// recordingDir returns a temp directory holding a placeholder
// recording for each id.
func recordingDir(t *testing.T, ids ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, id := range ids {
		path := filepath.Join(dir, id+".yaml")
		if err := os.WriteFile(path, []byte("# recording placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
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
