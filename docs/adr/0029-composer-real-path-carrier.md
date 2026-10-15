---
kind: adr
id: ADR-0029
title: "FLAT: the composer rides its real path when the `ctx/` short forms cannot carry it"
status: accepted
date: 2026-10-15
---

# ADR 0029 — FLAT: the composer rides its real path when the `ctx/` short forms cannot carry it

- **Status:** Accepted, 2026-10-15 — maintainer decision, taken to close the composer boundary [ADR 0015](0015-flat-metadata-spelling.md) left open ("closing it needs a `ctx/` or real-path carrier — a new decision, not a codec change").
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — a direct decision.
- **Amends:** [ADR 0015](0015-flat-metadata-spelling.md) decision 5 (its first bullet, the composer's `external_ref`) and decision 2 (encode writes `ctx/` only, now except a composer those forms cannot carry), [REQ-053](../specifications/wire.md#req-053) (the composition-level metadata rule), [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes) (the out-of-scope classes and the deliberate exclusions), [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) (the carrier-less keys). **Applies:** PROBE-086, PROBE-089, PROBE-105.
- **Related:** [ADR 0014](0014-webtemplate-reference-implementation-lock.md) pins the reference whose spelling this adopts; [ADR 0016](0016-event-context-optionals-underscore-spelling.md) took the same route for the EVENT_CONTEXT optionals, a real path rather than new `ctx/` short forms.

## Context

The SDK carries the composition's composer in two `ctx/` short forms, `ctx/composer_name` and `ctx/composer_self`, and accepts the reference's real-path spellings of those two as aliases ([ADR 0015](0015-flat-metadata-spelling.md)). The short forms carry a name, or the fact that the composer is the subject, and nothing else. A composer's `external_ref`, its `identifiers` and a PARTY_RELATED composer's `relationship` have no carrier: decode refuses the reference's keys for them, and encode drops the `external_ref` and `identifiers` of a named composer and refuses every other shape. A properly referenced composer therefore does not survive a FLAT round trip, and FLAT the reference writes for such a composer does not decode.

The reference writes those attributes at the composer's real path, `<root>/composer|id`, `|id_scheme`, `|id_namespace`, beside `<root>/composer|name`. So does the published *Simplified Formats* specification's own example. Five of the cross-format corpus bodies and four of the FLAT conformance bodies carry them; none uses a `ctx/` spelling for the composer's reference.

The choice of carrier fixes what decode accepts and what encode writes for a composer, so it changes the FLAT a caller stores. It cannot be taken back without breaking stored FLAT on one side or the other.

## Decision

The composer's real path is the carrier of the whole composer, in the party grammar § REQ-140 already gives every other party position. Decode reads it beside the two `ctx/` short forms, and a field given in both spellings must agree. Encode writes a composer that one `ctx/` short form carries whole in that form, as before, and any other composer whole at its real path with no `ctx/` composer key, so a body spells its composer one way. The mechanics are [wire.md § REQ-053](../specifications/wire.md#req-053)'s composer rule.

ADR 0015's other decisions stand, and decision 2 still holds for the four other respelled fields (`language`, `territory`, `time`, `setting`) and for a composer a `ctx/` form carries whole: the `ctx/` form remains the only spelling encode writes for them.

## Consequences

- **Positive: a referenced composer round-trips.** The composer leaves the out-of-scope classes of § REQ-140, which drop from four to three, and the reference's composer keys decode instead of being refused. PROBE-086 and PROBE-105 compare those keys where they were excluded.
- **Positive: no new vocabulary.** Every key is the reference's own spelling, read by the party grammar the SDK already implements for `_provider`, `_health_care_facility` and participations.
- **Cost: two output spellings for one attribute across bodies.** A composition whose composer is a bare name still writes `ctx/composer_name`; one whose composer is referenced writes `<root>/composer|name` and its reference. A consumer that reads only `ctx/composer_name` misses the second kind. Within one body the composer has one spelling.
- **Cost: FLAT that encode used to accept changes shape or starts to encode.** A named composer with a reference was written as `ctx/composer_name` alone, and is now written at its real path with the reference; a PARTY_RELATED composer, a PARTY_SELF with a reference and a PARTY_IDENTIFIED with only a reference or identifiers were refused, and now encode. A PARTY_IDENTIFIED with nothing to write, or with an empty name, is still refused: written as nothing, it would read back as PARTY_SELF.

## Traceability

- Amends: ADR-0015, REQ-053, REQ-140, REQ-170
