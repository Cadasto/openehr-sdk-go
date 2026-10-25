---
kind: adr
id: ADR-0031
title: "The composition and EHR_STATUS version reads refuse an IMPORTED_VERSION, departing from the pinned version schemas"
status: accepted
date: 2026-10-25
---

# ADR 0031 — The composition and EHR_STATUS version reads refuse an IMPORTED_VERSION, departing from the pinned version schemas

- **Status:** Accepted, 2026-10-25.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** —
- **Amends:** [REQ-174](../specifications/wire.md#req-174--versioned-object-reads) (Versioned-object reads), which states the refusal, and [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source), which carries the keyed exception.
- **Related:** [ADR 0024](0024-contribution-deletion-without-data.md) and [ADR 0026](0026-stored-query-paging-fields-only-when-set.md) (the keyed-exception form this one follows).

## Context

**What the pin says.** The vendored EHR API answers each version read of a composition or an EHR_STATUS (`versioned_composition_version_get_at_time`, `…_get_by_id`, and the two `versioned_ehr_status` twins) with `UVersionOfComposition` or `UVersionOfEhrStatus`, each a `oneOf` of `ORIGINAL_VERSION` and `IMPORTED_VERSION` with `_type` as the discriminator ([`resources/its-rest/ehr-validation.openapi.yaml`](../../resources/its-rest/ehr-validation.openapi.yaml), `:3700-3710`, `:3802-3812`).

**What an IMPORTED_VERSION is.** In the RM Common IM's change-control package, an `IMPORTED_VERSION` wraps an `ORIGINAL_VERSION` copied from another system: its `item` is that original version, and its own `commit_audit` and `contribution` record the import, not the original commit. A record holds one when it received versions through an EHR Extract.

**What the SDK does.** The version reads of [§ REQ-174](../specifications/wire.md#req-174--versioned-object-reads) decode the envelope as `ORIGINAL_VERSION`. The canonical JSON decoder refuses any other `_type` on it, so an `IMPORTED_VERSION` body fails as the typed decode failure of [REQ-151](../specifications/transport.md#req-151--typed-2xx-decode-failure). That refuses a body the pin admits, and [§ REQ-095](../specifications/wire.md#req-095) makes every such departure a keyed exception with a decision behind it.

**Why this needs a decision.** Reading an `IMPORTED_VERSION` means choosing what the decoded version reports: the import's audit and contribution, the original's, or both, and how a caller tells an imported version from a local one. The EHR Extract, the exchange that creates imported versions, is deferred in [scope.md](../specifications/scope.md#out-of-scope-v1), so no consumer needs the reading yet to settle that choice.

## Decision

**The composition and EHR_STATUS version reads read `ORIGINAL_VERSION` only, and refuse an `IMPORTED_VERSION` body as a typed decode failure, never as an empty version.** [§ REQ-174](../specifications/wire.md#req-174--versioned-object-reads) *Responses* states the rule, and [§ REQ-095](../specifications/wire.md#req-095) lists the refusal as a keyed exception.

The exception is tied to the EHR Extract scope. It lapses when the extract exchange comes into scope: the version reads then take `IMPORTED_VERSION` and this ADR is superseded by the one that decides what an imported version reports.

## Consequences

- A caller reading a record that holds imported versions gets a typed decode error carrying the response body, and can decode the body itself. It does not get a version whose identifier and payload are silently empty.
- The `VERSIONED_PARTY` version reads in the demographic leaf decode the same way today; this ADR does not bind them, because § REQ-174's response rules cover the composition and EHR_STATUS leaves only.
- Reading imported versions later is an additive change: a new field or type on the decoded version, not a change to what an `ORIGINAL_VERSION` read returns.

## Traceability

- Amends: REQ-095, REQ-174
