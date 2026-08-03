# ADR 0015 — Composition-level FLAT metadata: accept both spellings, emit `ctx/`

- **Status:** Accepted, 2026-08-03 — maintainer decision, taken to unblock REQ-115 (the FLAT author linter cannot state its required-key set until the spelling question is settled) and to close a real interop rejection found by PROBE-086.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** —
- **Introduces:** —. **Amends:** [REQ-053](../specifications/wire.md#req-053) (FLAT/STRUCTURED codecs — decode input surface). **Applies:** REQ-115 (FLAT author linter — consumes this decision), REQ-080 / PROBE-086 (the probe that surfaced it).
- **Plan:** [2026-08-03-flat-coverage-ratchet.md](../plans/2026-08-03-flat-coverage-ratchet.md) Phase 3.
- **Related:** [ADR 0014](0014-webtemplate-reference-implementation-lock.md) pins the reference whose spelling this admits; the package deviation register is [`simplified/deviations.md`](../../openehr/serialize/simplified/deviations.md); the census is [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md).

## Context

Composition-level metadata has **two spellings in circulation**, and they are not a disagreement about meaning — they carry identical information on different surfaces:

| The reference's real path (relative to the template root) | REQ-053's `ctx/` short form |
|---|---|
| `language\|code`, `language\|terminology` | `ctx/language` |
| `territory\|code`, `territory\|terminology` | `ctx/territory` |
| `composer\|name` | `ctx/composer_name` |
| `composer_self` | `ctx/composer_self` |
| `context/start_time` | `ctx/time` |

The ITS-REST *Simplified Formats* specification describes the `ctx/` prefix for composition metadata, and REQ-053 reads and writes those short forms. EHRbase's own FLAT output writes the real paths. Both are in the wild.

Two independent facts forced the decision now:

1. **A real rejection.** Before this ADR, feeding an EHRbase-authored FLAT body to `UnmarshalFlat` failed with `ErrUnknownPath` on `<root>/language|code` — a hard failure over a pure respelling. Integrators hand-authoring FLAT from EHRbase documentation hit the same wall.
2. **REQ-115 is blocked on it.** The [FLAT author linter plan](../plans/2026-07-16-flat-author-linter.md) Phase 0 must define "the required composition-level FLAT key set" as normative prose. That set cannot be written down without knowing which spelling is canonical, which is accepted, and which is an error.

PROBE-086 held these 318 corpus keys out of its comparison **on both sides** precisely because the question was open — and one of them, `context/setting`, was holding out a real encode-side drop rather than a respelling.

## Decision

**Accept both spellings on input; emit exactly one — `ctx/` — on output.** An asymmetric codec: liberal in what it reads, single-valued in what it writes.

1. **Decode accepts either spelling** for the five respelled fields in the table above. A real path is normalised onto its `ctx/` equivalent before any Web Template resolution, so the rest of decode sees one vocabulary.
2. **Encode is unchanged** — `ctx/` only. No consumer's stored FLAT changes shape, and there is exactly one output spelling to test and document.
3. **A `|terminology` witness is checked, then discarded.** The `ctx/` form carries only the code, and `applyContext` rebuilds the CODE_PHRASE with a hardcoded terminology (`ISO_639-1`, `ISO_3166-1`). A real path carrying a *different* terminology is therefore an error, not a value to silently rewrite.
4. **Two spellings that disagree are an error, not a precedence rule.** `ctx/language: en` beside `<root>/language|code: de` fails loudly naming both. Silently preferring either would corrupt composition metadata — the same stance the codec already takes on an index collision.
5. **Respellings only.** Two composition-level families are deliberately *not* admitted, because accepting them would mean adding a field rather than accepting a spelling:
   - `context/setting|*` — `ctx/setting` is unsupported on **decode too**, so this is an unimplemented field on both surfaces.
   - `composer|id` / `|id_scheme` / `|id_namespace` — the composer's `external_ref`, which the `ctx/` short forms structurally cannot carry. Admitting and dropping it would breach REQ-053's semantics-preserving contract.

   Both stay refused and therefore stay visible in the PROBE-086 census, rather than being quietly absorbed.

## Consequences

- **Positive — interop.** An EHRbase-authored composition, or one hand-written from EHRbase docs, decodes. This was a hard adoption blocker and it is the main point of the change.
- **Positive — REQ-115 unblocked.** Phase 0 can now state the required-key set in `ctx/` terms and name the real-path spellings as accepted aliases, which is exactly the prose that was impossible to write before.
- **No output change, no break.** Encode still emits `ctx/` only, so nothing downstream shifts. This is additive on the input surface.
- **The PROBE-086 census does not move.** Worth stating plainly, because it is the natural thing to expect and it is wrong: the probe decodes upstream FLAT and **re-encodes** it, so a real-path key still comes back respelled as `ctx/`, still reads as one missing plus one extra key, and stays held out. The 318 keys remain held out and coverage remains **18.0%**. What changed is that a body carrying them now *decodes* instead of erroring — which the round-trip metric structurally cannot show.
- **`context/setting`'s waiver survives.** The one hold-out that hides a real encode-side drop is an *emission* gap (`ctx/setting` is not written), not a spelling gap, so this decision does not clear it. It needs `ctx/setting` emission on both sides; tracked in `deviations.md` and `SKIPPED.md`.
- **Alias table is load-bearing.** Every accepted alias must be in one table with its `ctx/` target, so adding a sixth is a reviewed edit rather than scattered string comparisons.
- **A template constraining a COMPOSITION-level `language` node is intercepted.** Such a key routes to `ctx/language` rather than through leaf placement. That is correct — it is the same RM attribute either way — but it means the alias table shadows those Web Template paths, which is why the table is restricted to attributes that genuinely *are* composition metadata.

## Alternatives considered

- **`ctx/` only (status quo).** Rejected: leaves a hard failure on reference-authored FLAT, and leaves REQ-115 unable to state its contract.
- **Real paths only, dropping `ctx/`.** Rejected: `ctx/` is what the ITS-REST spec describes and what the SDK already emits; switching would break every existing consumer's stored FLAT for no interop gain.
- **Accept both *and* emit both.** Rejected: duplicate keys for one value invite divergence between them, double the surface every consumer must handle, and would make the codec non-idempotent unless the pair is always written in lockstep.
- **Accept both on input, emit the reference's real paths.** This *would* move the 318 census keys into the compared set — the only option that does. Rejected anyway: it is a breaking output change for existing consumers, and it contradicts the spec's own `ctx/` guidance. A census number is not worth either. If parity on those keys is ever wanted, it belongs behind an explicit output-dialect option, not as a default flip.
- **Prefer one spelling silently when both appear.** Rejected: a payload carrying two different languages is a defect in the payload, and guessing which is meant corrupts metadata invisibly.
