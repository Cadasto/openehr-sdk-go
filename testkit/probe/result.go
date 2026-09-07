package probe

import (
	"fmt"
	"strings"
)

// Mode is the backend the runner selected for a probe invocation.
// A backend-facing probe must not read this; the runner stamps it
// onto [Result] after the probe returns (REQ-082).
type Mode string

// The four modes the runner can select. Sandbox, Cassette, and Live
// are the three backends REQ-082 requires of a backend-facing probe;
// ModeInRepo is the declared class for a probe that reaches no server.
const (
	// ModeSandbox serves probes from the in-memory sandbox backend.
	// The runner builds this client itself.
	ModeSandbox Mode = "sandbox"
	// ModeCassette replays a checked-in recording.
	ModeCassette Mode = "cassette"
	// ModeLive reaches a real openEHR deployment.
	ModeLive Mode = "live"
	// ModeInRepo runs probes that assert over vendored inputs or the
	// SDK's own output and reach no backend.
	ModeInRepo Mode = "in-repo"
)

// Status is the closed set a probe may report (REQ-082).
type Status string

// The closed status vocabulary. Anything else a probe reports is
// rewritten to StatusFail by the runner.
const (
	// StatusPass means the probe's assertion held.
	StatusPass Status = "pass"
	// StatusFail means the assertion did not hold, or the probe
	// returned an error.
	StatusFail Status = "fail"
	// StatusSkip means a precondition was absent. The detail must name
	// it; a skip with no named precondition is rewritten to a failure.
	StatusSkip Status = "skip"
)

// Effect is the write posture of a backend-facing probe (REQ-082).
// An unclassified probe is treated as [EffectMutating] so it cannot
// reach a live deployment by default.
type Effect string

// The two effect classifications. The zero value is not one of them:
// see [ResolveEffect].
const (
	// EffectReadOnly means the probe writes nothing to the backend.
	EffectReadOnly Effect = "read-only"
	// EffectMutating means the probe writes, so Live mode needs the
	// per-invocation opt-in.
	EffectMutating Effect = "mutating"
)

// ResolveEffect returns e when it is a classified effect, and
// [EffectMutating] for any other value — including the zero value.
// Removing this default must fail [TestUnclassifiedEffectIsMutating].
func ResolveEffect(e Effect) Effect {
	if e == EffectReadOnly {
		return EffectReadOnly
	}
	return EffectMutating
}

// Result is the shared probe outcome. Status is the closed set
// pass / fail / skip; Detail is supplementary text for failures and
// skips. Mode is filled by the runner.
type Result struct {
	Probe  string
	Mode   Mode
	Status Status
	Detail string
}

// ParseModes splits a catalog Modes line into the mode tokens it
// names, in order. Parenthetical notes and trailing dash clauses are
// stripped first, so "Sandbox, Cassette, Live.", "In-repo (unit-level;
// no backend)" and "Sandbox, Cassette, Live (planned)." all parse. An
// empty line — once that stripping leaves nothing — returns (nil,
// nil): there is nothing to parse, which is not an error.
//
// A token that survives stripping but names none of the four modes
// returns [ErrInvalidEntry]: a catalog typo (e.g. "Sanbdox") must fail
// loudly rather than silently narrow the probe's declared modes to
// whatever recognisable tokens happened to be left.
func ParseModes(line string) ([]Mode, error) {
	line = strings.TrimSpace(line)
	if i := strings.Index(line, "—"); i >= 0 {
		line = line[:i]
	}
	if i := strings.Index(line, "("); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimRight(line, ". \t")
	if line == "" {
		return nil, nil
	}
	var out []Mode
	for part := range strings.SplitSeq(line, ",") {
		token := strings.ToLower(strings.TrimSpace(part))
		switch token {
		case "sandbox":
			out = append(out, ModeSandbox)
		case "cassette":
			out = append(out, ModeCassette)
		case "live":
			out = append(out, ModeLive)
		case "in-repo":
			out = append(out, ModeInRepo)
		default:
			return nil, fmt.Errorf("%w: unknown mode token %q in %q", ErrInvalidEntry, token, line)
		}
	}
	return out, nil
}
