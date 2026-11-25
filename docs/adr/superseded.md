---
kind: reference
---

# Superseded ADRs

Nothing on this page binds: each entry names where its decision lives now.
ADR numbers are never reused, so a new ADR takes the next number above the highest in the [index](README.md#index) or on this page.

## ADR-0001 — BMM version-bump runbook

Folded into [ADR 0002 § D11](0002-bmm-codegen-decisions.md#d11--a-bmm-version-bump-is-one-pull-request), the version-bump rule, on 2026-11-25; the steps are in [`resources/bmm/README.md` § Updating](../../resources/bmm/README.md#updating).

## ADR-0003 — Codec polymorphism for abstract generic RM classes

Folded into [ADR 0002](0002-bmm-codegen-decisions.md) D4 on 2026-11-25.

## ADR-0004 — Strict-encode, permissive-decode for BMM `Real` and `Integer`

Folded into [ADR 0022](0022-canonical-json-encoding-json-v2.md) § Numbers on 2026-11-25.

## ADR-0005 — Compiled OPT foundation (rminfo + internal templatecompile)

Folded into [ADR 0035](0035-compile-step-reads-operational-template-model.md) on 2026-11-25; its rules are now [REQ-048](../specifications/reference-model.md#req-048--rm-meta-model-introspection-surface) and [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge).

## ADR-0006 — Composition validation walker package placement

Folded into [ADR 0025](0025-template-validation-includes-the-rm-floor.md) clause 5 on 2026-11-25.

## ADR-0008 — SMART discovery: canonical `services` map shape

Folded into [ADR 0023](0023-smart-platform-base-url-and-oidc-issuer.md) decisions 1 and 2 on 2026-11-25.

## ADR-0010 — Public compiled-template bridge placement

Folded into [ADR 0035](0035-compile-step-reads-operational-template-model.md) decision 4 on 2026-11-25.

## ADR-0012 — Retire SDK-GAP as a durable identifier; REQ/PROBE is the feature register

Its rule is in [AGENTS.md § Spec-driven workflow](../../AGENTS.md#spec-driven-workflow-agents), "No `SDK-GAP` identifier", since 2026-11-25.

## ADR-0016 — EVENT_CONTEXT optionals ride the underscore grammar, not new `ctx/` short forms

Folded into [ADR 0015](0015-flat-metadata-spelling.md) decision 6 on 2026-11-25.

## ADR-0021 — Encoded JSON member order is not part of the canonical JSON contract

Folded into [ADR 0022](0022-canonical-json-encoding-json-v2.md) § Member order on 2026-11-25.

## ADR-0024 — A contribution deletion built without a payload carries no data, departing from the pinned UpdateVersion schema

Its reason now sits in the owning section; see [wire.md § Keyed exceptions](../specifications/wire.md#keyed-exceptions).

## ADR-0026 — Stored-query execution sends offset and fetch only when the caller sets them, departing from the pinned Query schema

Its reason now sits in the owning section; see [wire.md § Keyed exceptions](../specifications/wire.md#keyed-exceptions).

## ADR-0027 — Template admission precedes the RM rules at every node

Folded into [ADR 0025](0025-template-validation-includes-the-rm-floor.md) clauses 1, 2 and 4 on 2026-11-25.

## ADR-0029 — FLAT: the composer rides its real path when the `ctx/` short forms cannot carry it

Folded into [ADR 0015](0015-flat-metadata-spelling.md) decisions 2 and 5 on 2026-11-25.

## ADR-0030 — Composition calls send the format query parameters only when the caller opts in, departing from the pinned composition operations

Its reason now sits in the owning section; see [wire.md § Keyed exceptions](../specifications/wire.md#keyed-exceptions).

## ADR-0031 — The composition and EHR_STATUS version reads refuse an IMPORTED_VERSION, departing from the pinned version schemas

Its reason now sits in the owning section; see [wire.md § Keyed exceptions](../specifications/wire.md#keyed-exceptions).

## ADR-0032 — The composition and EHR_STATUS version reads accept an ORIGINAL_VERSION without data, the RM's logical deletion, departing from the pinned version schemas

Its reason now sits in the owning section; see [wire.md § Keyed exceptions](../specifications/wire.md#keyed-exceptions).

## ADR-0036 — Web Template ids: a duplicate sibling takes the format's `_1` suffix and an interval alternative the IPS spelling, departing from the reference's code

Folded into [ADR 0014](0014-webtemplate-reference-implementation-lock.md) decision 2 on 2026-11-25.
