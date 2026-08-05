# ADR 0016 — EVENT_CONTEXT optionals ride the underscore grammar, not new `ctx/` short forms

- **Status:** Accepted, 2026-08-05 — maintainer decision, taken with REQ-140 (underscore-prefixed RM attributes) so the grammar has one spelling per attribute before implementation starts.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** —
- **Introduces:** —. **Amends:** [REQ-053](../specifications/wire.md#req-053) (scopes the ctx-only emission rule to the six respelled scalar fields). **Applies:** [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes) (the grammar these attributes ride), REQ-080 / PROBE-086 (the parity probe whose census moves).
- **Plan:** [2026-08-05-flat-rm-attributes.md](../plans/2026-08-05-flat-rm-attributes.md).
- **Related:** [ADR 0015](0015-flat-metadata-spelling.md) settled the composition-metadata spelling this decision deliberately does **not** reopen; [ADR 0014](0014-webtemplate-reference-implementation-lock.md) pins the reference whose spelling wins here.

## Context

The optional `EVENT_CONTEXT` attributes — `health_care_facility`, `participations`, `end_time`, `location` — have two candidate FLAT spellings, and unlike the ADR 0015 fields they had **neither** implemented:

| The reference's spelling (what the PROBE-086 corpus writes) | The ITS `ctx/` sketch |
|---|---|
| `<root>/context/_health_care_facility\|id\|id_scheme\|id_namespace\|name` | `ctx/health_care_facility…` |
| `<root>/context/_participation:N\|function\|mode\|name\|id…` | `ctx/participation…` |
| `<root>/context/_end_time` | `ctx/end_time` |
| `<root>/context/_location` | `ctx/location` |

The ITS-REST *Simplified Formats* spec lists these among the optional `ctx/` context fields, but gives them no normative suffix vocabulary; the reference implementation both emits and documents the `context/_*` underscore spelling, which is exactly the REQ-140 grammar (`_`-prefixed optional RM attributes at the node they belong to — here, the `EVENT_CONTEXT` under the real `context` segment). Until now the codec dropped these values on encode and refused the keys on decode.

Deciding now, together with REQ-140's Phase 0, avoids the failure mode ADR 0015 was written to close: two spellings implemented ad hoc in different corners of the codec, with the conflict rules discovered afterwards.

## Decision

**The EVENT_CONTEXT optionals are carried in the reference's `context/_*` underscore spelling, on both encode and decode, as ordinary REQ-140 grammar.** No new `ctx/` short forms are introduced for them.

1. **One spelling, both directions.** `context/_health_care_facility`, `context/_participation:N`, `context/_end_time`, `context/_location` are emitted when populated and accepted on decode — the same recursive, RM-type-keyed grammar every other `_`-attribute uses. They get no special-case machinery.
2. **REQ-053's ctx-only emission rule is scoped, not weakened.** The "encode MUST emit only the `ctx/` short form" rule applies to the six respelled scalar fields (`language`, `territory`, `composer_name`, `composer_self`, `time`, `setting`) — the fields where two spellings of one scalar are in circulation. The EVENT_CONTEXT optionals never had an emitted spelling to preserve, so choosing the underscore form breaks no consumer.
3. **The `ctx/` sketches stay unaccepted.** `ctx/participation…`, `ctx/health_care_facility…`, `ctx/end_time`, `ctx/location` remain `ErrUnknownPath` on decode. They are recorded as *deferred input-alias candidates*: if a producer that writes them materialises, accepting them is an ADR 0015-style alias-table addition (accept, normalise onto the underscore spelling, error on disagreement) — an additive follow-up, not a blocker.

## Consequences

- **Positive — census keys actually move.** Unlike the ADR 0015 respellings (held out on both sides, net zero), these keys join the PROBE-086 *compared* set: the corpus writes `context/_*`, the codec now emits `context/_*`, and the round-trip is byte-comparable (~290 keys across `_health_care_facility`, `_participation`, `_end_time`, `_location`).
- **Positive — one grammar, no fork.** PARTICIPATION and PARTY_IDENTIFIED decompose identically at `context/_participation`, `_other_participation` (ENTRY), and inside `_feeder_audit` — a single implementation surface instead of a `ctx/` special case beside a `_` general case.
- **Consistent with ADR 0015's own reasoning.** ADR 0015 chose `ctx/` for the scalar fields because that was the SDK's *already-emitted* spelling and the ITS-documented one; here the already-documented, reference-emitted spelling is the underscore form. Both decisions preserve the emitted surface consumers can already rely on and admit the other spelling only as input.
- **Negative — the ITS `ctx/` sketch is not honoured on input.** A hand-authored payload using `ctx/participation…` is refused. Accepted: no known producer emits it, the refusal is loud and typed, and the alias path stays open.
- **The REQ-115 FLAT author linter** (draft plan) inherits one vocabulary for context optionals — the underscore spelling — rather than two.

## Alternatives considered

- **New `ctx/` short forms (emit + accept).** Rejected: requires inventing a suffix vocabulary the ITS spec does not define (structured participations with performer identifiers do not flatten onto the scalar `ctx/` pattern), diverges from the reference's emitted surface, moves no census key, and forks the PARTICIPATION grammar into two spellings.
- **Accept both spellings now, emit underscore.** Rejected for scope: the alias table, disagreement rules, and witness handling exist for the scalar fields but would need per-suffix extension for structured values; no producer of the `ctx/` sketch is known. Deferred, not foreclosed.
- **Emit `ctx/`, accept both (mirror ADR 0015 exactly).** Rejected: symmetry with ADR 0015 is superficial — ADR 0015 preserved an existing emitted surface; this would *create* a non-reference emitted surface and permanently hold ~290 corpus keys out of the parity comparison.
