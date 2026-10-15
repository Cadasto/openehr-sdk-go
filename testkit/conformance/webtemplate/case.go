// Package webtemplate runs the PROBE-086 upstream FLAT serialisation
// conformance harness: for each body in the pinned EHRbase FLAT corpus,
// decode it through the SDK's FLAT codec and re-encode it, then compare the
// result against the upstream-authored original.
//
// What this adds over the FLAT round-trip probe is the input. That probe
// round-trips the SDK's own FLAT output, so it cannot catch a path this SDK
// never emits, a suffix it names differently, or a leaf it drops
// symmetrically. Here the input is FLAT this SDK did not write.
//
// # What is asserted
//
// The corpus exercises a great deal the FLAT codec does not model yet, so a
// whole-body comparison would be all noise. Instead [Run] establishes the
// modelled subset and asserts on that:
//
//  1. Decode the upstream body with the codec's report option. The codec
//     ignores a key it does not read and reports it as unmatched content
//     (see simplified.WithUnmatched); it refuses, with an error, a key it
//     reads but cannot carry, and never drops one silently. Exclude each
//     reported key, and for each refusal record it and its reason, remove
//     exactly what that refusal names (see [dropRefused]: one key, one suffix,
//     one leaf, or a whole subtree, depending on the shape), and retry. The
//     resulting [Report.Excluded] count is the unmodelled surface, derived
//     from the codec's own report and errors instead of from a hand-kept table
//     that would go stale as gaps close.
//  2. Re-encode what decoded, and compare against the surviving upstream
//     keys. Inside that subset a missing key, an extra key, or a changed
//     value is a failure. There is no list of tolerated drops.
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
// while `|id` / `|id_scheme` / `|id_namespace` / `|type` (the composer's
// `external_ref`) and its `_identifier:N` and `/relationship` sub-paths are no
// respelling: no `ctx/` short form carries them. Decode reads them and encode
// writes them at the same real path (ADR 0029), so they reach the comparison and
// the census compares them. Base-matching `composer` once absorbed the
// external_ref into the hold-out and hid 12 corpus keys the codec then refused
// (PR #86 review, round 3).
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
// The match is suffix-aware: `language` and `territory` hold out every
// suffix ([metaLeaves]), while the composer holds out only the exact spellings
// the codec respells ([metaSpellings]). A key counts as metadata because of
// what the codec does with that spelling, whatever leaf it sits on.
//
// This is the one place the comparison holds a key out on both sides, and
// every hold-out is a respelling. There are no waivers, and
// TestHoldOutMatchesCodecAliases fails if one reappears:
//
//   - `language|*`, `territory|*`, `composer|name`, `composer_self`,
//     `context/start_time` and `context/setting|*` are respellings. Upstream
//     writes them as real paths under the template root (`<root>/language|code`,
//     `<root>/context/setting|code`); the FLAT codec reads and writes the `ctx/`
//     short forms (`ctx/language`, `ctx/setting|code` + `|value`). That is the
//     same information on a different surface, a documented codec deviation
//     with no data lost. Comparing across the two spellings would report every
//     such key as both missing and extra.
//   - `composer|id`, `|id_scheme`, `|id_namespace` and `|type`, and the
//     composer's `_identifier:N` and `/relationship`, are not held out, which
//     is why the match is suffix-aware. No `ctx/` short form carries them:
//     they carry the rest of the composer at its real path, which decode
//     reads and encode writes (ADR 0029), so they are compared like any
//     content key instead of being absorbed by a base match.
//
// Keys are held out even when decode happens to accept them, so a metadata
// key that survives decode does not resurface as a phantom "missing" when
// re-encode writes it back in ctx/ form.
//
// The match reads a key through [unindexedSpelling] first, because the codec
// does: a metadata path whose segments carry `:0` (`<root>/language:0|code`,
// `<root>:0/composer:0|name`, `<root>/context:0/start_time:0`, the spelling
// OPT-free STRUCTURED → FLAT conversion writes) is the same real path as the
// unindexed one, the template root's own segment included, as the codec reads
// it. Any other index is refused by the codec, so it is not held out: it
// reaches decode and is counted there.
//
// This creates an asymmetry on the emitted side: the `ctx/` prefix test is
// unbounded, so every ctx/ key the encoder writes is skipped and a bogus one
// would be invisible here. The decode leg of the FLAT round-trip probe covers
// that case, because it feeds this SDK's own ctx/ output back through decode.
func IsCompositionMeta(key, root string) bool {
	if strings.HasPrefix(key, ctxPrefix) {
		return true
	}
	rel, rooted := strings.CutPrefix(key, root+"/")
	if !rooted {
		// The template root's own segment takes `:0` too (REQ-053).
		rel, rooted = strings.CutPrefix(key, root+zeroIndex+"/")
	}
	if !rooted {
		return false
	}
	rel = unindexedSpelling(rel)
	// Suffix-aware first, on the spelling as written once its `:0` indexes are
	// gone: `composer|name` is a respelling, `composer|id` is the composer's
	// external_ref at its real path, and only an exact match can tell them
	// apart.
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

// zeroIndex is the one index a composition metadata segment takes. Each such
// segment is a single-valued RM attribute, so the codec reads `:0` as no index
// at all and refuses any other.
const zeroIndex = ":0"

// unindexedSpelling drops a `:0` index from every segment of a root-relative
// FLAT key's path and keeps the |suffix as written: `context:0/start_time:0`
// becomes `context/start_time` and `language:0|code` becomes `language|code`,
// the spellings simplified.MetadataAliasSpellings reports. Any other index is
// left in place, so the key matches no metadata spelling.
func unindexedSpelling(rel string) string {
	path, suffix, piped := strings.Cut(rel, "|")
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = strings.TrimSuffix(s, zeroIndex)
	}
	out := strings.Join(segs, "/")
	if piped {
		out += "|" + suffix
	}
	return out
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
