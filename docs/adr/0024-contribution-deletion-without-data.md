---
kind: adr
id: ADR-0024
title: "A contribution deletion built without a payload carries no data, departing from the pinned UpdateVersion schema"
status: accepted
date: 2026-10-07
---

# ADR 0024 — A contribution deletion built without a payload carries no data, departing from the pinned UpdateVersion schema

- **Status:** Accepted, 2026-10-07.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — direct decision, recorded from the maintainer's issue [#246](https://github.com/Cadasto/openehr-sdk-go/issues/246) (its *Proposed solution* and *Alternatives considered*).
- **Amends:** [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source) — one keyed exception, carried by § REQ-095 itself rather than asserted from here — and [REQ-130](../specifications/wire.md#req-130--contribution-builder) (Contribution builder), whose § *Deletion* carries the mechanics.
- **Related:** [ADR 0019](0019-definition-timestamp-tolerance.md) (the first keyed exception to REQ-095, and the form this one follows).

## Context

**What the Reference Model says.** A logical deletion is a new `ORIGINAL_VERSION` whose `data` is Void, with change type `deleted` (`523`) and lifecycle state `deleted` (`523`) (RM Common, *Contributions* and *Logical Deletion*). `ORIGINAL_VERSION.data` is optional (`0..1`, kept optional "to enable logical deletion") and `lifecycle_state` is mandatory. The vendored BMM agrees: [`resources/bmm/openehr_rm_1.2.0.bmm.json`](../../resources/bmm/openehr_rm_1.2.0.bmm.json) declares `data` without `is_mandatory` and `lifecycle_state` with it.

**What the pin says.** [`resources/its-rest/ehr-validation.openapi.yaml:3905-3908`](../../resources/its-rest/ehr-validation.openapi.yaml) lists `data` among the `required` members of `UpdateVersion`, the version type of `Contribution_create`. The issue cites an upstream change request, SPECPR-489, asking the pin's authors to make it optional; it was not re-verified here.

**Why this is a REQ-095 question.** REQ-095 makes the OpenAPI files authoritative: "the OpenAPI wins; the prose is updated, not the wire behaviour", and a departure it does not key is "a defect, not an exception". The contribution builder (REQ-130) cannot build the RM's deletion without emitting a version the pin's schema rejects.

**What is observed.** The two disagree in practice, and servers split. One server follows the RM and refuses the pin's form: it answers `422` to a `data` member on a deleted version and `400` to a lifecycle state that contradicts the change type. Another accepts the pin's body and drops the data. The vendored submission corpus ([`testkit/corpus/submissions/`](../../testkit/corpus/README.md)) records 12 versions with change type `523` (counting the two nested in a CONTRIBUTION payload), all carrying `data`, six with lifecycle `deleted` and six with `complete`, because the pin asks for it. So today the only way to send the RM's deletion is to hand-assemble the version, which is the wiring REQ-130 exists to remove.

## Decision

**The builder's `Deletion` accepts a call with no payload and then emits a version with no `data` member.** This is the second departure from the authoritative-source rule that REQ-095 grants and names. It is opt-in per call: a caller who passes a payload gets the pin-conformant version as before, and a caller whose server enforces the schema passes the payload. The other three operations still refuse a call with no payload. § REQ-130 *Deletion* states the behaviour and the lifecycle default that goes with it.

The exception is tied to the pin's current text. It lapses when the pin makes `data` optional, and if the upstream request is refused, the exception stays as the SDK's deliberate choice of the RM over a schema the RM contradicts.

## Consequences

- **Easier:** a caller builds the RM's deletion with the builder, without hand-wiring the change type, audit and lifecycle state, and against servers that follow the RM.
- **Harder:** the SDK can now emit a body the pinned schema rejects, so a schema-validating server refuses a deletion built without a payload. The default path is unchanged and the departure is a visible choice at the call (a nil payload).
- **Permanently constrained:** REQ-095's keyed list now has two entries, each needing the evidence a deployment produced. The data-less version was already expressible by hand: `Submission.Validate` has never required `data`.
- **Cheap to undo:** refusing the nil payload again is one condition in the builder, a CHANGELOG entry and a minor release, which is why this ADR records a deliberate choice and not an irreversible fork.

## Traceability

- Amends: REQ-095, REQ-130
