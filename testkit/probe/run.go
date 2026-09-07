package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// sandboxBaseURL is the base URL the runner gives a Sandbox-mode
// client. The host is never dialled — [sandbox.Backend] is the HTTP
// transport — but the path must carry a prefix the backend strips, so
// it matches sandbox's /openehr/v1 route prefix.
const sandboxBaseURL = "https://sandbox.local/openehr/v1"

// Sentinel refusals the runner returns instead of a green summary.
// Each one is pinned by a named test that must fail if the guard is
// removed (REQ-082).
var (
	// ErrUnsatisfiableMode is returned when the invocation asked for a
	// mode the selected probes cannot run in — Cassette with no
	// recording, Live with no endpoint, in-repo mode for a
	// backend-facing probe, a probe whose declared Modes exclude the
	// requested one, or a backend wiring that would silently swap one
	// mode's backend for another's. The runner must not fall back to
	// another mode.
	ErrUnsatisfiableMode = errors.New("probe: mode cannot be satisfied")

	// ErrAllSkipped is returned when every selected probe reported
	// skip. Vacuous success is not green.
	ErrAllSkipped = errors.New("probe: all probes skipped")

	// ErrMutatingNotOptedIn is returned when Live mode would execute a
	// mutating (or unclassified) probe without the per-invocation
	// opt-in REQ-082 requires.
	ErrMutatingNotOptedIn = errors.New("probe: mutating live run requires opt-in")

	// ErrEmptySelection is returned when there is nothing to run: no
	// entries handed to [Run], or no ids handed to [Select]. An empty
	// selection is a caller mistake, not a green run and not an
	// all-skipped one — a computed filter that matched nothing must
	// never widen into the whole catalog.
	ErrEmptySelection = errors.New("probe: empty selection")

	// ErrUnknownProbe is returned by [Select] when a requested id is
	// not in the catalog.
	ErrUnknownProbe = errors.New("probe: unknown probe id")

	// ErrInvalidEntry is returned when a catalog entry cannot be run
	// as written: no Run function, no id, an id another entry already
	// uses, or an InRepo flag its declared Modes contradict. The
	// runner refuses the entry rather than panicking on it (REQ-025).
	ErrInvalidEntry = errors.New("probe: invalid catalog entry")
)

// Entry is one catalog probe the runner can invoke.
type Entry struct {
	ID     string
	Effect Effect
	// Modes is the catalog Modes line. When non-empty it is a filter:
	// the runner refuses the entry in any mode it does not name. Empty
	// means the probe has not declared support, and the runner applies
	// only the mode's own backend requirements. An in-repo probe may
	// declare ModeInRepo and nothing else.
	Modes []Mode
	// InRepo is true when the probe reaches no backend (REQ-082's
	// declared class). An in-repo probe needs no client and no
	// recording, and runs under every mode.
	InRepo bool
	// Run executes the probe. c is nil for an in-repo probe. The
	// function must not inspect the runner's mode.
	Run func(ctx context.Context, c *transport.Client) (Result, error)
}

// Config is the runner's mode plus that mode's backend configuration.
// The mode is a real backend choice, not a label: the runner builds
// the Sandbox client itself and refuses a Live or Cassette client that
// is sandbox-backed, so a run cannot report one mode while exercising
// another.
type Config struct {
	Mode Mode

	// Client is a caller-supplied transport, used in Cassette mode
	// (the replayer) and optionally in Live mode. It must be nil in
	// Sandbox mode — set Sandbox instead.
	Client *transport.Client

	// Sandbox is the in-memory backend a Sandbox-mode run serves from.
	// Nil means the runner creates a fresh one per run, so sandbox
	// state stays per-run and isolated (REQ-082).
	Sandbox *sandbox.Backend

	// HTTPClient is the injected transport a Live-mode run uses when
	// Client is nil (REQ-021: the SDK never creates one implicitly).
	HTTPClient *http.Client

	// TokenSource authenticates a Live-mode run built from Endpoint.
	// Nil means anonymous.
	TokenSource auth.TokenSource

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

// add records r and returns the updated summary. It is a value method
// like [Summary.Green]: the type is a plain value, so callers write
// sum = sum.add(r).
func (s Summary) add(r Result) Summary {
	s.Results = append(s.Results, r)
	switch r.Status {
	case StatusPass:
		s.Passed++
	case StatusFail:
		s.Failed++
	case StatusSkip:
		s.Skipped++
	default:
		// Unreachable: Run canonicalises every status before calling
		// add. Counting an unrecognised one as a failure keeps
		// Passed+Failed+Skipped == len(Results) if it ever is reached.
		s.Failed++
	}
	return s
}

// Select resolves ids against catalog, preserving the order the ids
// were given. An id the catalog does not carry returns
// [ErrUnknownProbe]; an empty ids list returns [ErrEmptySelection]
// rather than the whole catalog, so a computed filter that matched
// nothing cannot widen into a full run. To run the whole catalog,
// pass it straight to [Run].
func Select(catalog []Entry, ids ...string) ([]Entry, error) {
	if err := validate(catalog); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: no probe ids given", ErrEmptySelection)
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
		return nil, fmt.Errorf("%w: %s", ErrUnknownProbe, strings.Join(missing, ", "))
	}
	return out, nil
}

// Run executes exactly the entries it is given, in order — the whole
// catalog is spelled Run(ctx, cfg, catalog), a subset is
// [Select]'s output. An empty entries list returns
// [ErrEmptySelection]: it is neither green nor all-skipped.
//
// Run validates every entry before executing any of them
// ([ErrInvalidEntry]) and refuses a mode the selection cannot satisfy
// ([ErrUnsatisfiableMode], [ErrMutatingNotOptedIn]) without running
// anything. It then builds one client for the whole run — for Sandbox
// mode the runner builds it from cfg.Sandbox, and a Live or Cassette
// client that turns out to be sandbox-backed is refused.
//
// A probe that returns an error is recorded as a failure and the run
// continues; Run returns the completed summary alongside an
// [errors.Join] of those errors, so an erroring probe can never leave
// a green partial summary. A run whose probes all skipped returns the
// summary and [ErrAllSkipped].
func Run(ctx context.Context, cfg Config, entries []Entry) (Summary, error) {
	if len(entries) == 0 {
		return Summary{Mode: cfg.Mode}, fmt.Errorf("%w: no entries to run", ErrEmptySelection)
	}
	if err := validate(entries); err != nil {
		return Summary{Mode: cfg.Mode}, err
	}
	refused := Summary{Mode: cfg.Mode, Selected: len(entries)}
	for _, e := range entries {
		if err := satisfiable(cfg, e); err != nil {
			return refused, err
		}
	}

	// One client for the whole run, never one per probe. An all-in-repo
	// selection needs no backend at all, so none is built.
	var client *transport.Client
	if slices.ContainsFunc(entries, func(e Entry) bool { return !e.InRepo }) {
		c, err := buildClient(cfg)
		if err != nil {
			return refused, err
		}
		client = c
	}

	sum := Summary{Mode: cfg.Mode, Selected: len(entries)}
	var probeErrs []error
	for _, e := range entries {
		var c *transport.Client
		if !e.InRepo {
			c = client
		}
		r, err := e.Run(ctx, c)
		if err != nil {
			probeErrs = append(probeErrs, fmt.Errorf("%s: %w", e.ID, err))
			r = Result{Probe: e.ID, Status: StatusFail, Detail: "probe error: " + err.Error()}
		}
		if r.Probe == "" {
			r.Probe = e.ID
		}
		r.Mode = resultMode(cfg, e)
		sum = sum.add(canonicalise(r))
	}
	if len(probeErrs) > 0 {
		return sum, errors.Join(probeErrs...)
	}
	if sum.Passed == 0 && sum.Failed == 0 {
		return sum, fmt.Errorf("%w: %d skipped", ErrAllSkipped, sum.Skipped)
	}
	return sum, nil
}

// canonicalise is the single point where a probe's self-reported
// status is reconciled with the closed vocabulary. A status outside
// pass/fail/skip becomes a failure that keeps the offending text, and
// a skip that names no precondition becomes a failure too — REQ-082
// requires a skip to say what was missing, and an unexplained skip
// would otherwise buy silence for free.
func canonicalise(r Result) Result {
	switch r.Status {
	case StatusPass, StatusFail:
		return r
	case StatusSkip:
		if strings.TrimSpace(r.Detail) == "" {
			r.Status = StatusFail
			r.Detail = "skip without a named precondition (REQ-082)"
		}
		return r
	default:
		r.Detail = fmt.Sprintf("unrecognized status %q", r.Status)
		r.Status = StatusFail
		return r
	}
}

func resultMode(cfg Config, e Entry) Mode {
	if e.InRepo {
		return ModeInRepo
	}
	return cfg.Mode
}

// validate refuses entries the runner cannot execute as written,
// before anything runs. Every case here would otherwise surface as a
// panic or a silently dropped probe (REQ-025).
func validate(entries []Entry) error {
	seen := make(map[string]struct{}, len(entries))
	for i, e := range entries {
		if e.ID == "" {
			return fmt.Errorf("%w: entry %d has no id", ErrInvalidEntry, i)
		}
		if _, dup := seen[e.ID]; dup {
			return fmt.Errorf("%w: duplicate id %s", ErrInvalidEntry, e.ID)
		}
		seen[e.ID] = struct{}{}
		if e.Run == nil {
			return fmt.Errorf("%w: %s has no Run function", ErrInvalidEntry, e.ID)
		}
		if err := validateModes(e); err != nil {
			return err
		}
	}
	return nil
}

// validateModes rejects an entry whose InRepo flag and declared Modes
// contradict each other. An in-repo probe reaches no backend, so it
// cannot declare a backend mode; a backend-facing probe is not
// in-repo, so it cannot declare ModeInRepo.
func validateModes(e Entry) error {
	for _, m := range e.Modes {
		if e.InRepo && m != ModeInRepo {
			return fmt.Errorf("%w: %s is in-repo but declares mode %q", ErrInvalidEntry, e.ID, m)
		}
		if !e.InRepo && m == ModeInRepo {
			return fmt.Errorf("%w: %s declares in-repo but is backend-facing", ErrInvalidEntry, e.ID)
		}
	}
	return nil
}

func satisfiable(cfg Config, e Entry) error {
	if e.InRepo {
		// An in-repo probe reaches no backend, so every mode satisfies
		// it and its Modes line is not a filter.
		return nil
	}
	if len(e.Modes) > 0 && !slices.Contains(e.Modes, cfg.Mode) {
		return fmt.Errorf("%w: %s declares modes %s, not %q", ErrUnsatisfiableMode, e.ID, modeList(e.Modes), cfg.Mode)
	}
	switch cfg.Mode {
	case ModeSandbox:
		return nil
	case ModeCassette:
		if cfg.RecordingDir == "" {
			return fmt.Errorf("%w: cassette mode needs a recording directory for %s", ErrUnsatisfiableMode, e.ID)
		}
		if !recordingExists(cfg.RecordingDir, e.ID) {
			return fmt.Errorf("%w: no recording for %s in %s", ErrUnsatisfiableMode, e.ID, cfg.RecordingDir)
		}
		return nil
	case ModeLive:
		if cfg.Client == nil && cfg.Endpoint == "" {
			return fmt.Errorf("%w: live mode needs an endpoint or a client for %s", ErrUnsatisfiableMode, e.ID)
		}
		if ResolveEffect(e.Effect) == EffectMutating && !cfg.AllowMutating {
			return fmt.Errorf("%w: %s", ErrMutatingNotOptedIn, e.ID)
		}
		return nil
	case ModeInRepo:
		return fmt.Errorf("%w: in-repo mode runs only in-repo probes, and %s is backend-facing", ErrUnsatisfiableMode, e.ID)
	default:
		return fmt.Errorf("%w: unknown mode %q", ErrUnsatisfiableMode, cfg.Mode)
	}
}

// buildClient produces the one transport the run's backend-facing
// probes share. Sandbox mode is built here rather than by the caller,
// so the label and the backend cannot diverge; Live and Cassette use
// the caller's client but only after it is shown not to be
// sandbox-backed.
func buildClient(cfg Config) (*transport.Client, error) {
	switch cfg.Mode {
	case ModeSandbox:
		if cfg.Client != nil {
			return nil, fmt.Errorf("%w: sandbox mode builds its own client; set Sandbox, not Client", ErrUnsatisfiableMode)
		}
		backend := cfg.Sandbox
		if backend == nil {
			backend = sandbox.New()
		}
		return NewClient(sandboxBaseURL, backend.HTTPClient(), nil)
	case ModeCassette:
		if cfg.Client == nil {
			return nil, fmt.Errorf("%w: cassette mode needs a client", ErrUnsatisfiableMode)
		}
		if err := refuseSandboxTransport(cfg.Client.HTTPClient(), ModeCassette); err != nil {
			return nil, err
		}
		return cfg.Client, nil
	case ModeLive:
		if cfg.Client != nil {
			if err := refuseSandboxTransport(cfg.Client.HTTPClient(), ModeLive); err != nil {
				return nil, err
			}
			return cfg.Client, nil
		}
		if err := refuseSandboxTransport(cfg.HTTPClient, ModeLive); err != nil {
			return nil, err
		}
		c, err := NewClient(cfg.Endpoint, cfg.HTTPClient, cfg.TokenSource)
		if err != nil {
			return nil, fmt.Errorf("live mode: %w", err)
		}
		return c, nil
	case ModeInRepo:
		// Unreachable: satisfiable refuses a backend-facing probe in
		// in-repo mode, and buildClient runs only when one is selected.
		return nil, fmt.Errorf("%w: in-repo mode has no backend client", ErrUnsatisfiableMode)
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", ErrUnsatisfiableMode, cfg.Mode)
	}
}

// refuseSandboxTransport rejects a client whose HTTP transport is the
// in-memory sandbox backend. Running that under the Live or Cassette
// label is a silent fallback to Sandbox: the summary would claim a
// deployment or a recording answered when nothing left the process.
func refuseSandboxTransport(hc *http.Client, mode Mode) error {
	if hc == nil {
		return nil
	}
	if _, ok := hc.Transport.(*sandbox.Backend); ok {
		return fmt.Errorf("%w: %s mode with a sandbox transport is a silent fallback to sandbox", ErrUnsatisfiableMode, mode)
	}
	return nil
}

// modeList renders declared modes for a refusal message.
func modeList(modes []Mode) string {
	parts := make([]string, len(modes))
	for i, m := range modes {
		parts[i] = string(m)
	}
	return strings.Join(parts, ", ")
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
