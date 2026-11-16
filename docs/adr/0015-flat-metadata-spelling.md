---
kind: adr
id: ADR-0015
title: "Composition-level FLAT metadata: accept both spellings, emit `ctx/`"
status: accepted
date: 2026-08-03
---

# ADR 0015 — Composition-level FLAT metadata: accept both spellings, emit `ctx/`

- **Status:** Accepted, 2026-08-03 — maintainer decision.
- **Amended by:** [ADR 0029](0029-composer-real-path-carrier.md): decision 5's first bullet is reversed, and decision 2 gains one exception. The composer's real path carries the whole composer, decode reads `composer|id`, `|id_scheme` and `|id_namespace`, and encode writes a composer the `ctx/` short forms cannot carry at its real path.
- **Introduces:** —. **Amends:** [REQ-053](../specifications/wire.md#req-053) (FLAT/STRUCTURED codecs — decode input surface). **Applies:** REQ-080 / PROBE-086 (the probe that surfaced it).
- **Related:** [ADR 0014](0014-webtemplate-reference-implementation-lock.md) pins the reference whose spelling this admits; the package deviation register is [`simplified/deviations.md`](../../openehr/serialize/simplified/deviations.md).

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

1. **A real rejection that had turned into a silent success.** `UnmarshalFlat` never handled these keys correctly. They failed with `ErrUnsupportedDatatype` or `ErrUnknownPath` until a `CODE_PHRASE` leaf mapping made the coded pair **decode silently through ordinary leaf placement**, writing the RM node directly and bypassing the `ctx/` conflict check. A hard error over a pure respelling blocked adoption; an unnoticed bypass of the conflict check was worse. Both argue for one explicit, reviewed alias table.
2. **A FLAT author linter is blocked on it.** Its specification must first define "the required composition-level FLAT key set" as normative prose. That set cannot be written down without knowing which spelling is canonical, which is accepted, and which is an error.

## Decision

**Accept both spellings on input; emit exactly one — `ctx/` — on output.** An asymmetric codec: liberal in what it reads, single-valued in what it writes.

1. **Decode accepts either spelling** for the five respelled fields in the table above. A real path is normalised onto its `ctx/` equivalent before any Web Template resolution, so the rest of decode sees one vocabulary.
2. **Encode is unchanged** — `ctx/` only. No consumer's stored FLAT changes shape, and there is exactly one output spelling to test and document.

3. **A `|terminology` witness is checked, then discarded.** The `ctx/` form carries only the code, and `applyContext` rebuilds the CODE_PHRASE with a hardcoded terminology (`ISO_639-1`, `ISO_3166-1`). A real path carrying a *different* terminology is therefore an error, not a value to silently rewrite. The rule's normative home is [wire.md § REQ-053](../specifications/wire.md#req-053); this ADR records the decision behind it, not the requirement itself.
4. **Two spellings that disagree are an error, not a precedence rule.** `ctx/language: en` beside `<root>/language|code: de` fails loudly naming both. Silently preferring either would corrupt composition metadata — the same stance the codec already takes on an index collision.
5. **Respellings only.** Two composition-level families are deliberately *not* admitted to the alias table, because accepting them would mean adding a field rather than accepting a spelling. They behave differently at the codec boundary, and the difference matters:
   - `composer|id` / `|id_scheme` / `|id_namespace` — the composer's `external_ref`, which the `ctx/` short forms structurally cannot carry. Admitting and dropping it would breach REQ-053's semantics-preserving contract, so decode **refused** these suffixes (`PARTY_PROXY`), until [ADR 0029](0029-composer-real-path-carrier.md) gave the composer its real path.

   - `context/setting|*` — the **`ctx/setting` short form** was then implemented on neither surface, so admitting the real path would have added a field, not a spelling. [REQ-053](../specifications/wire.md#req-053) has since given `ctx/setting` both directions, so `context/setting` is an ordinary respelling.

## Consequences

- **Positive — interop.** A body whose only unmodelled surface was the metadata spelling decodes; that was a hard adoption blocker and the main point of the change.
- **Positive — a FLAT author linter is unblocked.** It can state its required-key set in `ctx/` terms and names the real-path spellings as accepted aliases.
- **No output change, no break.** Encode emits `ctx/` only, so nothing downstream shifts; this is additive on the input surface. [ADR 0029](0029-composer-real-path-carrier.md) later made one exception, a composer the `ctx/` short forms cannot carry whole.
- **Alias table is load-bearing.** Every accepted alias must be in one table with its `ctx/` target, so adding a sixth is a reviewed edit rather than scattered string comparisons.
- **A template constraining a COMPOSITION-level `language` node is intercepted.** Such a key routes to `ctx/language` rather than through leaf placement. That is correct — it is the same RM attribute either way — but it means the alias table shadows those Web Template paths, which is why the table is restricted to attributes that genuinely *are* composition metadata.

## Alternatives considered

- **`ctx/` only (status quo).** Rejected: leaves a hard failure on reference-authored FLAT, and leaves a FLAT author linter unable to state its contract.
- **Real paths only, dropping `ctx/`.** Rejected: `ctx/` is what the ITS-REST spec describes and what the SDK already emits; switching would break every existing consumer's stored FLAT for no interop gain.
- **Accept both *and* emit both.** Rejected: duplicate keys for one value invite divergence between them, double the surface every consumer must handle, and would make the codec non-idempotent unless the pair is always written in lockstep.
- **Accept both on input, emit the reference's real paths.** Rejected: it is a breaking output change for existing consumers, and it contradicts the spec's own `ctx/` guidance. If parity on those keys is ever wanted, it belongs behind an explicit output-dialect option, not as a default flip.
- **Prefer one spelling silently when both appear.** Rejected: a payload carrying two different languages is a defect in the payload, and guessing which is meant corrupts metadata invisibly.

## Traceability

- Amends: REQ-053
- Applies: REQ-080
