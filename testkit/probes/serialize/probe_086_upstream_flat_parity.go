package serializeprobes

// PROBE-086 — upstream FLAT serialisation parity (REQ-080; exercises REQ-053
// and REQ-106). For one body of the pinned EHRbase FLAT conformance corpus,
// decode it through the REQ-053 codec and re-encode it, then compare against
// the upstream-authored original.
//
// The distinct assertion versus [Probe076SimplifiedRoundTrip] is the input.
// PROBE-076 round-trips the SDK's *own* FLAT output, so it cannot catch a
// path this SDK never emits, a suffix it names differently, or a leaf it
// drops symmetrically. Here the input is FLAT this SDK did not write — which
// is how the EVENT.time / INSTRUCTION.narrative / INSTRUCTION.expiry_time
// encode-side data loss was found (fixed 2026-08-01, REQ-121).
//
// This is a thin wrapper: the engine lives in
// testkit/conformance/webtemplate so the same runner backs both this probe
// and the package's own table-driven tests. What is compared, and the
// substantial part of the corpus that is not yet modelled, are documented in
// that package — SKIPPED.md carries the counted inventory.
//
// Modes: In-repo (Sandbox) only. Cassette / Live are out of scope for v1, so
// REQ-082 runnability is a documented partial.

import (
	"errors"
	"fmt"
	"strings"

	conformance "github.com/cadasto/openehr-sdk-go/testkit/conformance/webtemplate"
)

// Probe086UpstreamFlatParity round-trips one corpus fixture against the
// vendored upstream FLAT body. The caller owns corpus I/O: enumerate with
// [conformance.Cases] once and pass each [conformance.Case] in, the way every
// sibling probe here takes its input directly.
//
// Status is "pass" when the modelled subset round-trips exactly — same keys,
// same values — and that subset is non-empty. It is "fail" when a key inside
// the subset is dropped, invented, or altered, which is a codec defect. A
// fixture whose constructs the codec does not model yet does not fail on that
// account: those keys are excluded by the runner and counted in Detail.
//
// Coverage is guarded in two places, and this probe is only the coarse one.
// The ratchet is the per-fixture excluded/compared pin in
// testkit/conformance/webtemplate's own tests: that is what stops the
// unmodelled surface growing key by key. What the probe adds is a floor — a
// fixture that compared *nothing* fails, because [conformance.Report.Clean] is
// vacuously true over an empty compared set and would otherwise report a total
// coverage collapse (every key refused, or an emptied fixture) as a pass to a
// consumer reading Status alone.
//
// Framework misuse (nil target, a case with no FLAT path) returns a non-nil
// error.
func Probe086UpstreamFlatParity(target *conformance.Target, c conformance.Case) (Result, error) {
	r := Result{Probe: "PROBE-086"}
	if target == nil {
		return r, errors.New("PROBE-086: nil target")
	}
	if c.Flat == "" {
		return r, fmt.Errorf("PROBE-086: case %q has no FLAT body path", c.Name)
	}

	rep, err := conformance.Run(target, c)
	if err != nil {
		// A harness fault (unreadable fixture, a decode that cannot be
		// reduced, a re-encode error) is a probe failure, not a skip: it
		// means the corpus or the codec moved in a way the runner cannot
		// characterise.
		r.Status, r.Detail = "fail", "run: "+err.Error()
		return r, nil
	}

	// Coverage floor. Clean() asserts nothing about how much was compared, and
	// over an empty compared set it is vacuously true — so without this a
	// fixture whose every key got refused, or one that lost its content
	// upstream, would report "pass". Every fixture in the pinned corpus
	// compares at least one key, so zero is never legitimate.
	if rep.Compared == 0 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("compared 0 of %d upstream keys (%d excluded as unmodelled, %d metadata held out): "+
			"a fixture that compares nothing cannot round-trip exactly, it can only be vacuously clean — "+
			"expect corpus drift or a codec regression refusing every key",
			rep.Total, rep.Excluded, rep.Meta)
		return r, nil
	}

	if !rep.Clean() {
		var b strings.Builder
		fmt.Fprintf(&b, "modelled subset diverged: %d missing, %d extra, %d value mismatches (%d/%d keys compared, %d excluded as unmodelled)",
			len(rep.Missing), len(rep.Extra), len(rep.Mismatched), rep.Compared, rep.Total, rep.Excluded)
		for _, m := range rep.Missing {
			fmt.Fprintf(&b, "; missing %s", m)
		}
		for _, e := range rep.Extra {
			fmt.Fprintf(&b, "; extra %s", e)
		}
		for _, m := range rep.Mismatched {
			fmt.Fprintf(&b, "; %s", m)
		}
		r.Status, r.Detail = "fail", b.String()
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("%d/%d upstream keys compared exactly (%d excluded as unmodelled, %d metadata held out)",
		rep.Compared, rep.Total, rep.Excluded, rep.Meta)
	return r, nil
}

// NewProbe086Target compiles and exports the corpus OPT once, for reuse
// across every fixture in a run.
func NewProbe086Target() (*conformance.Target, error) {
	return conformance.NewTarget()
}
