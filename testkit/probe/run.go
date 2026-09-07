package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// Sentinel refusals the runner returns instead of a green summary.
// Each one is pinned by a named test that must fail if the guard is
// removed (REQ-082).
var (
	// ErrUnsatisfiableMode is returned when the invocation asked for a
	// mode the selected probes cannot run in — Cassette with no
	// recording, Live with no endpoint, Sandbox with no transport for
	// a backend-facing probe. The runner must not fall back to another
	// mode.
	ErrUnsatisfiableMode = errors.New("probe: mode cannot be satisfied")

	// ErrAllSkipped is returned when every selected probe reported
	// skip. Vacuous success is not green.
	ErrAllSkipped = errors.New("probe: all probes skipped")

	// ErrMutatingNotOptedIn is returned when Live mode would execute a
	// mutating (or unclassified) probe without the per-invocation
	// opt-in REQ-082 requires.
	ErrMutatingNotOptedIn = errors.New("probe: mutating live run requires opt-in")
)

// Entry is one catalog probe the runner can invoke.
type Entry struct {
	ID     string
	Effect Effect
	// Modes is the catalog Modes line. Empty means the probe has not
	// declared support; the runner then treats a backend-facing probe
	// as requiring the requested mode to be satisfiable, and an
	// in-repo probe as ModeInRepo.
	Modes []Mode
	// InRepo is true when the probe reaches no backend (REQ-082's
	// declared class). An in-repo probe needs no client and no
	// recording.
	InRepo bool
	// Run executes the probe. c is nil for an in-repo probe. The
	// function must not inspect the runner's mode.
	Run func(ctx context.Context, c *transport.Client) (Result, error)
}

// Config is the runner's mode plus that mode's backend configuration.
type Config struct {
	Mode Mode

	// Client is the already-configured transport the probes receive.
	// The runner does not construct one: the caller wires Sandbox
	// (sandbox.Backend as the HTTP client's RoundTripper), Cassette
	// (replayer), or Live (injected *http.Client against Endpoint).
	// Required for every selected backend-facing probe.
	Client *transport.Client

	// RecordingDir is the Cassette-mode corpus root
	// (testkit/recordings/). A selected backend-facing probe is
	// unsatisfiable unless a file whose name starts with the probe id
	// exists in this directory.
	RecordingDir string

	// Endpoint is the Live-mode openEHR REST base URL. Required for
	// every selected backend-facing probe when Mode is ModeLive and
	// Client is nil.
	Endpoint string

	// AllowMutating is the Live-mode opt-in for probes whose
	// resolved effect is mutating. Off by default.
	AllowMutating bool
}

// Summary is the runner's aggregation. Passes, skips, and failures
// are counted separately so a skip cannot read as a pass (REQ-082).
type Summary struct {
	Mode     Mode
	Results  []Result
	Passed   int
	Failed   int
	Skipped  int
	Selected int
}

// Green reports whether the run is a non-vacuous success: at least
// one pass, no failures. An all-skipped run is not green even if
// the caller ignores [ErrAllSkipped].
func (s Summary) Green() bool {
	return s.Failed == 0 && s.Passed > 0
}

// Run executes the catalog, a named subset, or one probe. ids, when
// present, are PROBE-NNN identifiers; an empty ids list runs every
// entry. Selecting a mode the invocation cannot satisfy returns
// [ErrUnsatisfiableMode] without running anything. A run whose
// probes all skipped returns the summary and [ErrAllSkipped].
func Run(ctx context.Context, cfg Config, catalog []Entry, ids ...string) (Summary, error) {
	selected, err := selectEntries(catalog, ids)
	if err != nil {
		return Summary{Mode: cfg.Mode}, err
	}
	sum := Summary{Mode: cfg.Mode, Selected: len(selected)}
	if len(selected) == 0 {
		return sum, fmt.Errorf("%w: empty selection", ErrAllSkipped)
	}
	for _, e := range selected {
		if err := satisfiable(cfg, e); err != nil {
			return Summary{Mode: cfg.Mode, Selected: len(selected)}, err
		}
	}
	for _, e := range selected {
		r, err := e.Run(ctx, cfg.Client)
		if err != nil {
			return sum, fmt.Errorf("%s: %w", e.ID, err)
		}
		if r.Probe == "" {
			r.Probe = e.ID
		}
		r.Mode = resultMode(cfg, e)
		// A probe that reports skip as pass is rewritten to skip: the
		// status vocabulary is closed and skip must stay skip.
		if r.Status != StatusPass && r.Status != StatusFail && r.Status != StatusSkip {
			r.Status = StatusFail
			if r.Detail == "" {
				r.Detail = "unrecognized status"
			}
		}
		sum.add(r)
	}
	if sum.Passed == 0 && sum.Failed == 0 {
		return sum, fmt.Errorf("%w: %d skipped", ErrAllSkipped, sum.Skipped)
	}
	return sum, nil
}

func (s *Summary) add(r Result) {
	s.Results = append(s.Results, r)
	switch r.Status {
	case StatusPass:
		s.Passed++
	case StatusFail:
		s.Failed++
	case StatusSkip:
		s.Skipped++
	default:
		s.Failed++
	}
}

func resultMode(cfg Config, e Entry) Mode {
	if e.InRepo {
		return ModeInRepo
	}
	return cfg.Mode
}

func selectEntries(catalog []Entry, ids []string) ([]Entry, error) {
	if len(ids) == 0 {
		return catalog, nil
	}
	index := make(map[string]Entry, len(catalog))
	for _, e := range catalog {
		index[e.ID] = e
	}
	out := make([]Entry, 0, len(ids))
	var missing []string
	for _, id := range ids {
		e, ok := index[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		out = append(out, e)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: unknown probe %s", ErrUnsatisfiableMode, strings.Join(missing, ", "))
	}
	return out, nil
}

func satisfiable(cfg Config, e Entry) error {
	if e.InRepo {
		return nil
	}
	switch cfg.Mode {
	case ModeSandbox:
		if cfg.Client == nil {
			return fmt.Errorf("%w: sandbox mode needs a client for %s", ErrUnsatisfiableMode, e.ID)
		}
		return nil
	case ModeCassette:
		if cfg.RecordingDir == "" {
			return fmt.Errorf("%w: cassette mode needs a recording directory", ErrUnsatisfiableMode)
		}
		if !recordingExists(cfg.RecordingDir, e.ID) {
			return fmt.Errorf("%w: no recording for %s in %s", ErrUnsatisfiableMode, e.ID, cfg.RecordingDir)
		}
		if cfg.Client == nil {
			return fmt.Errorf("%w: cassette mode needs a client for %s", ErrUnsatisfiableMode, e.ID)
		}
		return nil
	case ModeLive:
		if cfg.Client == nil && cfg.Endpoint == "" {
			return fmt.Errorf("%w: live mode needs an endpoint", ErrUnsatisfiableMode)
		}
		if cfg.Client == nil {
			return fmt.Errorf("%w: live mode needs a client for %s", ErrUnsatisfiableMode, e.ID)
		}
		if ResolveEffect(e.Effect) == EffectMutating && !cfg.AllowMutating {
			return fmt.Errorf("%w: %s", ErrMutatingNotOptedIn, e.ID)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown mode %q", ErrUnsatisfiableMode, cfg.Mode)
	}
}

func recordingExists(dir, id string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	prefix := strings.ToLower(id)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
