---
kind: adr
id: ADR-0030
title: "Composition calls send the format query parameters only when the caller opts in, departing from the pinned composition operations"
status: accepted
date: 2026-10-25
---

# ADR 0030 — Composition calls send the format query parameters only when the caller opts in, departing from the pinned composition operations

- **Status:** Accepted, 2026-10-25 — maintainer decision on issue [#307](https://github.com/Cadasto/openehr-sdk-go/issues/307), whose proposed solution names the parameter form as an option of its own.
- **Supersedes:** —
- **Superseded by:** —
- **Strand:** none — a direct decision.
- **Amends:** [REQ-173](../specifications/wire.md#req-173--composition-representation-formats) (Composition representation formats), which states the rule, and [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source), which carries the fourth keyed exception.
- **Related:** [ADR 0026](0026-stored-query-paging-fields-only-when-set.md) and [ADR 0024](0024-contribution-deletion-without-data.md) (the keyed-exception form this one follows).

## Context

**What the pin says.** [`resources/its-rest/ehr-validation.openapi.yaml`](../../resources/its-rest/ehr-validation.openapi.yaml) selects a composition's representation by media type alone: `composition_create`, `composition_update` and `composition_get` take `Content-Type` and `Accept` from `ContentType_LOCATABLE` and `Accept_LOCATABLE` (`:4103-4145`), whose values are `application/json`, `application/xml`, `application/openehr.wt.flat+json` and `application/openehr.wt.structured+json`. The overview requires the `openehr-template-id` header on a composition committed in a simplified format ([`overview-validation.openapi.yaml:295-297`](../../resources/its-rest/overview-validation.openapi.yaml)). The pin defines no query parameter that selects a format or names a template on these operations.

**What is observed.** EHRbase's composition controller (its `rest-openehr` module at commit `580ea750b`, 2026-10-16) declares only the deprecated `.schema`-suffixed simplified media types among those it consumes and produces, so a request with the canonical FLAT or STRUCTURED type is not routed to it. It also selects a simplified format from a `format` query parameter beside `application/json`, and reads the template id of a simplified write from a `templateId` query parameter; it does not read the `openehr-template-id` header.

**What binds the SDK.** [§ REQ-053](../specifications/wire.md#req-053) forbids emitting the deprecated `.schema`-suffixed types, and [§ REQ-095](../specifications/wire.md#req-095) refuses any departure from the pin that it does not name. So with media types alone, a FLAT or STRUCTURED write from the SDK does not reach such a deployment, and the caller is back to building the request by hand.

**Why this needs a decision.** The ways out are to emit the deprecated types, which § REQ-053 refuses; to send the parameter form, which § REQ-095 refuses unless it is keyed; or to leave such deployments to hand-built requests, which is the duplication issue #307 asks to remove.

## Decision

**The composition leaf sends the `format` and `templateId` query parameters only when the caller opts in, beside the standard `openehr-template-id` header, and never sends the deprecated media types.** [§ REQ-173](../specifications/wire.md#req-173--composition-representation-formats) *The format query parameters* states the rule, and [§ REQ-095](../specifications/wire.md#req-095) lists it as its fourth keyed exception.

The exception is tied to the pin's current text. It lapses if the pin defines these parameters on the composition operations.

## Consequences

- **Easier:** a caller whose deployment selects the simplified formats by parameter writes and reads FLAT and STRUCTURED through the typed leaf, keeping its `If-Match`, `Prefer`, audit and version-metadata handling.
- **Harder:** the leaf has two request forms for the simplified formats, and each needs its own tests. The caller has to know which form its deployment takes; the SDK does not detect it.
- **Consumer-visible:** nothing changes for a caller who does not set the option.
- **Permanently constrained:** the parameter form is never the default, and the SDK does not send the deprecated media types in either form.
- **Cheap to undo:** dropping the options is a removal of two public symbols, a CHANGELOG entry and a minor release before 1.0, which is why this records a deliberate choice and not an irreversible fork.

## Traceability

- Amends: REQ-173, REQ-095
