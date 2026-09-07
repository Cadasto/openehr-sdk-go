package probe

import "strings"

// Mode is the backend the runner selected for a probe invocation.
// A backend-facing probe must not read this; the runner stamps it
// onto [Result] after the probe returns (REQ-082).
type Mode string

const (
	ModeSandbox  Mode = "sandbox"
	ModeCassette Mode = "cassette"
	ModeLive     Mode = "live"
	ModeInRepo   Mode = "in-repo"
)

// Status is the closed set a probe may report (REQ-082).
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Effect is the write posture of a backend-facing probe (REQ-082).
// An unclassified probe is treated as [EffectMutating] so it cannot
// reach a live deployment by default.
type Effect string

const (
	EffectReadOnly Effect = "read-only"
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
// names. Parenthetical notes and trailing clauses are ignored so
// "Sandbox, Cassette, Live." and "In-repo (unit-level; no backend)"
// both parse.
func ParseModes(line string) []Mode {
	line = strings.TrimSpace(line)
	if i := strings.Index(line, "—"); i >= 0 {
		line = line[:i]
	}
	if i := strings.Index(line, "("); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimRight(line, ". \t")
	if line == "" {
		return nil
	}
	var out []Mode
	for part := range strings.SplitSeq(line, ",") {
		switch token := strings.ToLower(strings.TrimSpace(part)); token {
		case "sandbox":
			out = append(out, ModeSandbox)
		case "cassette":
			out = append(out, ModeCassette)
		case "live":
			out = append(out, ModeLive)
		case "in-repo":
			out = append(out, ModeInRepo)
		}
	}
	return out
}
