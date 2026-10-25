---
kind: adr
id: ADR-0032
title: "The composition and EHR_STATUS version reads accept an ORIGINAL_VERSION without data, the RM's logical deletion, departing from the pinned version schemas"
status: accepted
date: 2026-10-26
---

# ADR 0032 — The composition and EHR_STATUS version reads accept an ORIGINAL_VERSION without data, the RM's logical deletion, departing from the pinned version schemas

- **Status:** Accepted, 2026-10-26.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** —
- **Amends:** [REQ-174](../specifications/wire.md#req-174--versioned-object-reads) (Versioned-object reads), which states the reading, and [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source), which carries the keyed exception.
- **Related:** [ADR 0024](0024-contribution-deletion-without-data.md), the write-side twin: a contribution deletion built without a payload carries no data.

## Context

**What the pin says.** The vendored EHR API's `UMOriginalVersionOfComposition` and `UMOriginalVersionOfEhrStatus`, the `ORIGINAL_VERSION` arm of each version read's response, list `data` among their `required` members ([`resources/its-rest/ehr-validation.openapi.yaml`](../../resources/its-rest/ehr-validation.openapi.yaml), `:3641-3646`, `:3743-3748`).

**What the Reference Model says.** A logical deletion is a new `ORIGINAL_VERSION` whose `data` is Void, with lifecycle state `deleted`; `ORIGINAL_VERSION.data` is optional (`0..1`) so that a deletion can be recorded. The vendored BMM declares `data` without `is_mandatory`. [ADR 0024](0024-contribution-deletion-without-data.md) records the same rule and that servers split on it: one follows the RM and refuses a deleted version that carries data.

**What the SDK does.** The version reads of [§ REQ-174](../specifications/wire.md#req-174--versioned-object-reads) decode a version whose `data` is absent as a version with no payload and no error. That accepts a body the pin's schema excludes, and [§ REQ-095](../specifications/wire.md#req-095) makes every such departure a keyed exception with a decision behind it.

**Why this needs a decision.** Following the pin would make every logically deleted version unreadable through the SDK when the server follows the RM: the revision history names the version, and reading it would fail.

## Decision

**The composition and EHR_STATUS version reads accept an `ORIGINAL_VERSION` without `data` and return its envelope with no payload.** [§ REQ-174](../specifications/wire.md#req-174--versioned-object-reads) *Responses* states the reading, and [§ REQ-095](../specifications/wire.md#req-095) lists it as a keyed exception.

The exception is tied to the pin's current text. It lapses when the pin makes `data` optional on the version schemas, as it is in the RM.

## Consequences

- A caller reads a deleted version's identifier, lifecycle state, commit audit and contribution, and finds no payload; the lifecycle state tells a deletion from a server that dropped the data.
- A body that carries `data` is read exactly as before; the exception only widens what the reads accept.
- The PROBE-109 conformance probe does not treat a version without `data` as a failure.

## Traceability

- Amends: REQ-095, REQ-174
