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

// metaLeaves are the composition-level leaves directly under the template root
// where *every* `|suffix` is composition metadata, so the hold-out matches on
// the base path alone.
//
// Only `language` and `territory` qualify, and it is their witness suffixes that
// earn it: the codec accepts `|code` as the respelling and `|terminology` as a
// checked-and-discarded witness (simplified.MetadataWitnessSpellings), and the
// `ctx/` short form carries neither separately — one `ctx/language` stands for
// the whole CODE_PHRASE. Matching per-suffix here would leave the witness keys in
// the comparison, where they have no counterpart on the emitted side at all.
//
// `category` is deliberately absent: it is not a ctx/ field at all but a
// template-constrained Web Template leaf that rides its own FLAT path (see
// openehr/serialize/simplified/deviations.md § `ctx/` context), so both sides
// spell it identically and it is compared like any other content key.
var metaLeaves = map[string]bool{
	"language": true, "territory": true,
}

// metaSpellings are root-relative spellings held out **exactly as spelled,
// suffix and all** — the composer forms REQ-053 respells under `ctx/`, and
// nothing else on that leaf.
//
// `composer` cannot be base-matched the way `language` can, because its suffixes
// do not all mean the same thing: `|name` is a respelling (`ctx/composer_name`),
// while `|id` / `|id_scheme` / `|id_namespace` are the PARTY_PROXY
// `external_ref`, which no `ctx/` short form can carry. ADR 0015 refuses those
// three rather than silently dropping them, so they must reach decode and land in
// the census as an excluded PARTY_PROXY family. Base-matching `composer` absorbed
// all four into the hold-out and hid 12 corpus keys of real data loss (PR #86
// review, round 3).
//
// This set MUST stay in step with simplified.MetadataAliasSpellings and
// simplified.MetadataWitnessSpellings (ADR 0015): those tables decide which
// real-path spellings decode accepts, and the hold-out decides which the
// comparison excuses. A spelling accepted there but absent here re-creates
// exactly the false missing/extra noise the hold-out exists to prevent — held out
// as `ctx/…` on the emitted side, compared as a real path on the upstream side.
// A spelling held out here that the codec does *not* accept is the opposite
// failure, and the more dangerous one: it conceals a refusal. Both directions are
// asserted mechanically against the codec's own tables in
// TestHoldOutMatchesCodecAliases.
//
// `composer_self` is here for the first reason, not because the present corpus
// needs it (it writes only the `ctx/` form).
var metaSpellings = map[string]bool{
	"composer|name": true, "composer_self": true,
}

// contextMetaLeaves are the EVENT_CONTEXT attributes upstream writes under
// `<root>/context/`. Deliberately an allow-list of the two metadata leaves
// rather than the whole `context/` subtree: `context/other_context` carries
// *archetyped data*, and swallowing it as metadata would hide a real gap.
// This corpus has no other_context, so nothing is lost today — the narrow
// list is what keeps that true if the corpus grows.
//
// Both are respellings, base-matched like `language`/`territory` because every
// suffix they carry is either an accepted alias or a checked witness:
// `start_time` respells to `ctx/time`, and `setting` to `ctx/setting|code` +
// `|value` with `|terminology` as an `openehr` witness (REQ-053, amended
// 2026-08-05 — `setting` was the suite's one waived encode-side drop until
// `ctx/setting` emission landed). See [IsCompositionMeta].
var contextMetaLeaves = map[string]bool{
	"setting": true, "start_time": true,
}

// IsCompositionMeta reports whether key is composition-level metadata rather
// than archetyped content, for the given Web Template root id.
//
// The match is **suffix-aware**: `language` and `territory` hold out every
// suffix ([metaLeaves]), the composer holds out only the exact spellings the
// codec respells ([metaSpellings]). A key is metadata because of what the codec
// does with that spelling, not because of which leaf it sits on.
//
// This is the one place the comparison holds a key out on *both* sides, and
// every hold-out is the same kind of thing — a respelling; the waiver class is
// empty and TestHoldOutMatchesCodecAliases fails if one reappears:
//
//   - `language|*`, `territory|*`, `composer|name`, `composer_self`,
//     `context/start_time` and `context/setting|*` are **respellings**. Upstream
//     writes them as real paths under the template root (`<root>/language|code`,
//     `<root>/context/setting|code`); REQ-053 reads and writes the `ctx/` short
//     forms (`ctx/language`, `ctx/setting|code` + `|value`) — the same
//     information on a different surface, a documented codec deviation rather
//     than lost data. Comparing across the two spellings would report every such
//     key as both missing and extra, which is noise, not signal.
//     `context/setting` joined this class on 2026-08-05 (the amended REQ-053):
//     until `ctx/setting` emission landed it was the suite's one documented
//     **waiver** — the real path decoded and then re-encoded to nothing — and
//     ADR 0015 had deliberately left that emission gap open.
//   - `composer|id`, `|id_scheme` and `|id_namespace` are **not** held out, and
//     that is the point of the suffix-awareness. They are the PARTY_PROXY
//     `external_ref`, which the `ctx/` short forms structurally cannot carry, so
//     ADR 0015 refuses them on decode. They flow through to the codec, are
//     refused there, and are counted as an excluded PARTY_PROXY family — real
//     data loss, visible in the census, rather than absorbed by a base match.
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
	rel, rooted := strings.CutPrefix(key, root+"/")
	if !rooted {
		return false
	}
	// Suffix-aware first, on the spelling as written: `composer|name` is a
	// respelling, `composer|id` is refused data loss, and only an exact match can
	// tell them apart.
	if metaSpellings[rel] {
		return true
	}
	segs := strings.Split(baseOf(rel), "/")
	// Explicit length comparisons rather than a switch on len: the bounds
	// analyser does not refine `switch len(segs)` cases into length facts and
	// flags segs[1] as unguarded.
	if len(segs) == 1 {
		return metaLeaves[segs[0]]
	}
	if len(segs) == 2 {
		return segs[0] == "context" && contextMetaLeaves[segs[1]]
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
