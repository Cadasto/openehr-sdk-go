---
kind: adr
id: ADR-0036
title: "Web Template ids: a duplicate sibling takes the format's `_1` suffix and an interval alternative the IPS spelling, departing from the reference's code"
status: accepted
date: 2026-11-11
---

# ADR 0036 — Web Template ids: a duplicate sibling takes the format's `_1` suffix and an interval alternative the IPS spelling, departing from the reference's code

- **Status:** Accepted, 2026-11-11. Maintainer decisions taken while settling the Simplified Formats issues: the duplicate-sibling suffix on 2026-11-10, the interval alternative's id and the decode of the earlier spelling on 2026-11-11.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — a direct decision.
- **Amends:** [ADR 0014](0014-webtemplate-reference-implementation-lock.md) decision 2 (the reference's sibling-disambiguation rule) and its consequence "we adopt EHRbase's id choices verbatim", [REQ-106](../specifications/clinical-modeling.md#id-generation-adr-0014) (`id` generation), [REQ-053](../specifications/wire.md#req-053) (path segments). **Applies:** PROBE-075, PROBE-086, PROBE-105.
- **Related:** [ADR 0029](0029-composer-real-path-carrier.md) took the same route for the composer: the reference's spelling is read on decode beside the one the SDK writes.

## Context

ADR 0014 locks the Web Template `id` to the EHRbase `openEHR_SDK` reference, its sibling-disambiguation rule included, because the `id` is the FLAT path segment a consumer binds to. Two `id`s the reference's code writes are not the ones the *Simplified Formats* text or the reference's own test material give:

- **A duplicate sibling.** Rule 7 of § Node ID Generation Rules says only "Append a numeric suffix if needed to ensure uniqueness among siblings"; its example table gives "Blood Pressure (duplicate)" as `blood_pressure_1`. The reference writes the second of two siblings that share an `id` as `blood_pressure2`, as do the format's RM-mapping examples and the upstream FLAT corpus. A client that follows the table keys `x_1`, which the SDK left unmatched and, by default, ignored.
- **An interval value alternative.** When an ELEMENT admits several value types, each gets a child `<type>_value`. For `DV_INTERVAL<DV_DATE_TIME>` the reference's code writes `interval<dv_date_time>_value`, a key EHRbase does not read itself; the reference's IPS test Web Template writes `interval_of_date_time_value`.

## Decision

1. **A duplicate sibling's `id` is `<id>_N`**, the first claimant keeping the bare `id` and each later one taking the next free N from 1, as [REQ-106](../specifications/clinical-modeling.md#id-generation-adr-0014) sets out.
2. **An interval value alternative's `id` is `interval_of_<type>_value`**, the IPS test Web Template's spelling.
3. **Decode also reads a duplicate sibling's earlier spelling**, `<id><N+1>` for `<id>_N`, where no sibling has that `id` of its own; encode writes only decision 1's spelling. The conditions are [REQ-053](../specifications/wire.md#req-053)'s.

ADR 0014's other decisions stand: every other `id` mirrors the reference, and PROBE-075 compares the export to the reference's goldens with each of these two spellings mapped through a recorded deviation.

## Consequences

- **Positive: a body written to the format's table decodes.** The keys a client builds from the *Simplified Formats* text resolve, and a body written by the reference or by an earlier release of this SDK still decodes.
- **Cost: the reference does not read what the SDK writes.** EHRbase reads neither `blood_pressure_1` nor `interval_of_date_time_value` at those nodes. A consumer that sends the SDK's FLAT to the reference must respell them.
- **Cost: one shape is read on a guess.** Beside a sibling whose own `id` is `<id>_K` (named "Blood Pressure 1"), the reference's numbering and the SDK's part, and decode can read the reference's `blood_pressure2` as that sibling. No vendored template has that shape; the codec's deviations register records it.
- **Probes compare in one spelling.** PROBE-075 maps the reference goldens' ids, and PROBE-086 and PROBE-105 respell the reference's FLAT keys, each pinning what it respelled.

## Traceability

- Amends: ADR-0014, REQ-106, REQ-053
