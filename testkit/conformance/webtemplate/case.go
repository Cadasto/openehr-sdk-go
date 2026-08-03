// Package webtemplate runs the PROBE-086 upstream FLAT serialisation
// conformance harness: for each body in the pinned EHRbase FLAT corpus,
// decode it through the REQ-053 codec and re-encode it, then compare the
// result against the upstream-authored original.
//
// The distinct value over PROBE-076 is the input. PROBE-076 round-trips the
// SDK's *own* FLAT output, so it cannot catch a path this SDK never emits, a
// suffix it names differently, or a leaf it drops symmetrically. Here the
// input is FLAT this SDK did not write.
//
// # What is asserted
//
// The corpus exercises a great deal the REQ-053 codec does not model yet, so
// a whole-body comparison would be all noise. Instead [Run] establishes the
// **modelled subset** and asserts on that:
//
//  1. Decode the upstream body. Where the codec refuses a key — it fails
//     loudly, never silently drops — record the refusal and its reason, remove
//     exactly what that refusal covers (see [dropRefused]: one suffix, one
//     leaf, or a whole subtree, depending on the shape), and retry. The
//     resulting [Report.Excluded] count is the unmodelled surface, *derived
//     from the codec's own errors* rather than from a hand-kept table that
//     would rot as gaps close.
//  2. Re-encode what decoded, and compare against the surviving upstream
//     keys. Inside that subset a missing key, an extra key, or a changed
//     value is a **failure**, not a skip — there is no tolerated-drop list.
//
// The tests pin both halves: the excluded count per fixture (so the
// unmodelled surface can shrink deliberately but never grow unnoticed) and
// exactness of the compared subset. SKIPPED.md beside this file carries the
// per-family inventory and why each family is out of scope today.
package webtemplate

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Case is one conformance fixture: an upstream-authored FLAT body to be
// round-tripped against the corpus OPT. Every fixture in the vendored corpus
// instantiates the same template, so the OPT is shared and not carried here.
type Case struct {
	// Name is the fixture stem (e.g. "ehrbase_conformance_action").
	Name string
	// Flat is the absolute path to the upstream FLAT body.
	Flat string
}

// Cases enumerates the vendored corpus in a stable (sorted) order.
func Cases() ([]Case, error) {
	names, err := fixtures.ListFlatConformance()
	if err != nil {
		return nil, err
	}
	cases := make([]Case, 0, len(names))
	for _, n := range names {
		cases = append(cases, Case{Name: n, Flat: fixtures.FlatConformanceFlat(n)})
	}
	return cases, nil
}

// ctxPrefix marks composition-level metadata in this codec's spelling.
const ctxPrefix = "ctx/"

// metaLeaves are the composition-level attributes upstream writes as real
// paths directly under the template root and REQ-053 respells under `ctx/`.
//
// `category` is deliberately absent: it is not a ctx/ field at all but a
// template-constrained Web Template leaf that rides its own FLAT path (see
// openehr/serialize/simplified/deviations.md § `ctx/` context), so both sides
// spell it identically and it is compared like any other content key.
//
// The list MUST stay in step with simplified.metadataAliases (ADR 0015): that
// table decides which real-path spellings decode accepts, and this one decides
// which the comparison holds out. A spelling accepted there but absent here
// re-creates exactly the false missing/extra noise the hold-out exists to
// prevent — held out as `ctx/…` on the emitted side, compared as a real path on
// the upstream side. `composer_self` is here for that reason, not because the
// present corpus needs it (it writes only the `ctx/` form).
var metaLeaves = map[string]bool{
	"language": true, "territory": true, "composer": true, "composer_self": true,
}

// contextMetaLeaves are the EVENT_CONTEXT attributes upstream writes under
// `<root>/context/`. Deliberately an allow-list of the two metadata leaves
// rather than the whole `context/` subtree: `context/other_context` carries
// *archetyped data*, and swallowing it as metadata would hide a real gap.
// This corpus has no other_context, so nothing is lost today — the narrow
// list is what keeps that true if the corpus grows.
//
// The two are not held out for the same reason: `start_time` is a respelling
// (`ctx/time`), `setting` is a waived encode-side drop. See
// [IsCompositionMeta].
var contextMetaLeaves = map[string]bool{
	"setting": true, "start_time": true,
}

// IsCompositionMeta reports whether key is composition-level metadata rather
// than archetyped content, for the given Web Template root id.
//
// This is the one place the comparison holds a key out on *both* sides, and
// the hold-outs are not all the same kind of thing:
//
//   - `language`, `territory`, `composer` and `context/start_time` are
//     **respellings**. Upstream writes them as real paths under the template
//     root (`<root>/language|code`, `<root>/context/start_time`); REQ-053
//     reads and writes the `ctx/` short forms (`ctx/language`, `ctx/time`) —
//     the same information on a different surface, a documented codec
//     deviation rather than lost data. Comparing across the two spellings
//     would report every such key as both missing and extra, which is noise,
//     not signal.
//   - `context/setting` is a **waiver of a known encode-side drop**, not a
//     respelling. It decodes (a WithTemplate decode even defaults it to
//     `238 other care`) and then re-encodes to *nothing at all*: the
//     `ctx/setting` short form is deferred, so the value is dropped on the way
//     out — openehr/serialize/simplified/deviations.md § `ctx/` context. Held
//     out deliberately. It survived the ctx/-versus-real-path decision
//     ([ADR 0015](../../../docs/adr/0015-flat-metadata-spelling.md), which
//     settled the *spelling*): this is an **emission** gap — `ctx/setting` is
//     not written at all — so it clears when that emission lands, not when a
//     spelling wins. Until then the loss is recorded in SKIPPED.md, not
//     concealed by the hold-out.
//
// Keys are held out even when decode happens to accept them, so a metadata
// key that survives decode does not resurface as a phantom "missing" when
// re-encode writes it back in ctx/ form.
//
// Note the asymmetry this creates on the emitted side: the `ctx/` prefix test
// is unbounded, so *every* ctx/ key the encoder writes is skipped and a bogus
// one would be invisible here. PROBE-076's decode leg is the backstop for
// that — it feeds this SDK's own ctx/ output back through decode.
func IsCompositionMeta(key, root string) bool {
	if strings.HasPrefix(key, ctxPrefix) {
		return true
	}
	segs := strings.Split(baseOf(key), "/")
	if len(segs) < 2 || segs[0] != root {
		return false
	}
	// Explicit length comparisons rather than a switch on len: the bounds
	// analyser does not refine `switch len(segs)` cases into length facts and
	// flags segs[2] as unguarded.
	if len(segs) == 2 {
		return metaLeaves[segs[1]]
	}
	if len(segs) == 3 {
		return segs[1] == "context" && contextMetaLeaves[segs[2]]
	}
	return false
}

// baseOf strips the |suffix from a FLAT key, leaving the leaf path that all
// of a leaf's suffixed entries share.
func baseOf(key string) string {
	base, _, _ := strings.Cut(key, "|")
	return base
}

// There is deliberately no allow-list of tolerated encode-side drops here.
//
// An earlier revision carried one — `knownEncodeGaps`, three entries covering
// `EVENT.time` and `INSTRUCTION.narrative` / `expiry_time`, which this harness
// caught on its first run. All three shared one root cause (rmpath resolved
// none of the affected in-context attributes, and flat_encode routes that
// ErrPathNotFound into skipNotFound alongside genuinely absent optionals), it
// was fixed in rmpath on 2026-08-01 under REQ-121, and the list went empty.
//
// It is not coming back. An empty-but-armed matcher is a fail-open: a
// regression in exactly the area the probe was built to watch would land in a
// tolerated bucket instead of [Report.Missing], and the probe would still pass.
// A decoded key that does not re-encode is a failure. If a future drop needs
// tolerating, the case for it belongs in SKIPPED.md and in a decision to widen
// the refusal surface — which the codec generates itself by failing loudly —
// not in a table here that only the tests would notice going stale.
