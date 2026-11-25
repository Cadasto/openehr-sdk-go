---
kind: adr
id: ADR-0015
title: "FLAT composition metadata: `ctx/` for the six respelled scalars, real paths for the composer and the EVENT_CONTEXT optionals"
status: accepted
date: 2026-08-03
---

# ADR 0015 — FLAT composition metadata: `ctx/` for the six respelled scalars, real paths for the composer and the EVENT_CONTEXT optionals

- **Status:** Accepted, 2026-08-03 — maintainer decision.
- **Amended:** 2026-11-25: six respelled fields, not five; decision 5 gives the composer its real path, an exception to decision 2; decision 6 takes in the underscore spelling of the EVENT_CONTEXT optionals.
- **Amends:** [REQ-053](../specifications/wire.md#req-053) (*Composition-level metadata*, *The composer*), [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes) (the composer leaves its out-of-scope classes), [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) (the `ctx/` keys no field carries). **Applies:** the round-trip comparison of [REQ-052](../specifications/wire.md#req-052), PROBE-086, PROBE-089, PROBE-105.
- **Related:** [ADR 0014](0014-webtemplate-reference-implementation-lock.md) pins the reference whose spellings this admits; the package deviation register is [`simplified/deviations.md`](../../openehr/serialize/simplified/deviations.md).

## Context

Composition-level metadata has two spellings in circulation, carrying the same information on different surfaces.
The ITS-REST *Simplified Formats* specification puts it under the `ctx/` prefix; the reference implementation writes it at the real paths under the template root (`<root>/language|code`, `<root>/composer|name`, `<root>/context/start_time`).
Both are in the wild, and a codec that refuses one blocks adoption, while one that lets a real path through ordinary leaf placement bypasses the check that two spellings agree.

Two families do not fit a one-to-one respelling:

- **The composer.** `ctx/composer_name` carries a name and `ctx/composer_self` the fact that the composer is the subject, and nothing else.
  A composer's `external_ref`, its `identifiers` and a PARTY_RELATED composer's `relationship` have no `ctx/` carrier.
  The reference and the *Simplified Formats* example write them at the composer's real path, `<root>/composer|id`, `|id_scheme`, `|id_namespace`, beside `<root>/composer|name`.
- **The EVENT_CONTEXT optionals** (`health_care_facility`, `participations`, `end_time`, `location`).
  The *Simplified Formats* specification lists `ctx/` forms for them with no suffix vocabulary; the reference writes and documents `<root>/context/_health_care_facility`, `_participation:N`, `_end_time` and `_location`, which is the underscore grammar of [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes).

The carrier chosen for each field fixes what decode accepts and encode writes, so it fixes the FLAT a caller stores, and cannot be taken back without breaking stored FLAT on one side.

## Decision

**Six respelled scalars: read both spellings, write `ctx/`. The composer and the EVENT_CONTEXT optionals ride their real paths.**

1. **Decode accepts either spelling** of language, territory, composer name, composer self, time and setting: the `ctx/` short form or the reference's real path.
   A real path is normalised onto its `ctx/` field before any Web Template resolution, so the rest of decode sees one vocabulary; one alias table holds the respellings, so adding one is a reviewed edit.
2. **Encode writes the `ctx/` form only,** except a composer that the `ctx/` forms cannot carry whole (decision 5).
3. **A `|terminology` witness is checked, then discarded.**
   The `ctx/` form carries only the code and implies its terminology, so a real path carrying another terminology is refused, not rewritten.
4. **Two spellings that disagree are an error, not a precedence rule.**
   They are refused, naming both: silently preferring either would corrupt composition metadata.
5. **The composer's real path carries the whole composer.**
   Decode reads it, in the party grammar of [REQ-140](../specifications/wire.md#req-140--underscore-prefixed-rm-attributes), beside the two `ctx/` forms, and a field given in both spellings must agree.
   Encode writes a composer that one `ctx/` form carries whole in that form, and any other composer whole at its real path with no `ctx/` composer key, so one body spells its composer one way.
6. **The EVENT_CONTEXT optionals ride the underscore grammar,** on encode and decode, under the real `context` segment, with no special-case machinery and no `ctx/` short forms.
   The `ctx/` sketches for them are unknown `ctx/` keys, unmatched content ([REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode)) that decode ignores by default and reports or refuses on request.
   If a producer that writes them appears, accepting them is an addition to the alias table: accept, normalise onto the underscore spelling, refuse on disagreement.

The mechanics are [wire.md § REQ-053](../specifications/wire.md#req-053) *Composition-level metadata* and *The composer*.

## Consequences

- **Interop.** FLAT whose only unmodelled surface was the metadata spelling decodes, and the reference's composer and EVENT_CONTEXT keys round-trip; PROBE-086 and PROBE-105 compare them.
- **One vocabulary per field.** A FLAT author linter can state its required keys in `ctx/` terms with the real paths as accepted aliases, and has one spelling for the context optionals.
- **The alias table shadows Web Template paths.** A template that constrains a COMPOSITION-level `language` node has that key routed to `ctx/language`; it is the same RM attribute, which is why the table holds only attributes that are composition metadata.
- **Two output spellings of the composer across bodies.** A bare-name composer writes `ctx/composer_name`; a referenced one writes `<root>/composer|name` and its reference.
  A consumer that reads only `ctx/composer_name` misses the second kind.
  A PARTY_IDENTIFIED composer with nothing to write, or with an empty name, is refused: written as nothing, it would read back as PARTY_SELF.
- **The `ctx/` sketches are not honoured on input.** A hand-written body that uses `ctx/participation…` loses those fields unless the caller asks for the report or the refusal; no known producer writes them.

## Alternatives considered

- **`ctx/` only.** Rejected: it refuses reference-written FLAT and leaves a linter unable to state its contract.
- **Real paths only.** Rejected: `ctx/` is what the ITS-REST specification describes and what the SDK writes; dropping it breaks every stored body for no interop gain.
- **Accept both and write both.** Rejected: two keys for one value invite divergence and make the codec non-idempotent unless always written in step.
- **Write the reference's real paths for every field.** Rejected: a breaking output change that contradicts the specification's `ctx/` guidance; parity on those keys, if ever wanted, belongs behind an output-dialect option.
- **Prefer one spelling silently when both appear.** Rejected: a body carrying two languages is defective, and guessing corrupts metadata invisibly.
- **New `ctx/` short forms for the EVENT_CONTEXT optionals and the composer's reference.** Rejected: it invents a suffix vocabulary the specification does not define, diverges from the reference, and forks the party grammar into two spellings.

## Traceability

- Amends: REQ-053, REQ-140, REQ-170
- Applies: REQ-052
