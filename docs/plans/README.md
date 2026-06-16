# Implementation plans

Active and archived implementation plans for `openehr-sdk-go`. Plans derive from [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md). **Completed or superseded plans:** [`archive/README.md`](archive/README.md).

## Active plans

| Plan | Scope | Covers REQs / strands |
|---|---|---|
| [2026-06-14-demographic-rest-client.md](2026-06-14-demographic-rest-client.md) | openEHR Demographic API (PARTY-hierarchy CRUD) — split from the archived REST client plan | REQ-013, REQ-020..026, REQ-040, REQ-054 |

### Phase 2 — clinical building blocks (in flight)

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-05-21-phase-2-clinical-building-blocks.md](2026-05-21-phase-2-clinical-building-blocks.md) | Umbrella — sequencing and dependency rules | REQ-013, REQ-014 |
| [2026-06-12-template-req104-req105-deferred.md](2026-06-12-template-req104-req105-deferred.md) | REQ-104 slot assertions, REQ-105 terminology bindings (deferred) | REQ-104, REQ-105 |
| [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md) | WebTemplate JSON export (deferred) | proposed REQ-106 |

The two AQL plans landed and moved to `archive/` — AQL builders ([REQ-055](archive/2026-05-21-aql-builders.md)) and AQL parse + lint ([REQ-109](archive/2026-06-15-aql-lint.md)). The umbrella now carries only deferred scope (REQ-104/105/106) plus the still-planned demographic validator.

**Landed (archived):** OPT parser, REQ-100 follow-ups (Phases 1–6), composition validation (REQ-102), composition builder (REQ-101), template-driven instance generator (REQ-107), C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission, BMM codegen, canonical JSON/XML, AQL builders (REQ-055), AQL static lint (REQ-109) — see [archive/](archive/README.md). **Remaining validation scope** (demographic validator) is noted in the archived [umbrella validation plan](archive/2026-05-21-validation.md) and tracked under the Phase 2 umbrella.

## Header convention (load-bearing)

Every plan MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — REQ-NNN / PROBE-NNN / STRAND-NN identifiers
- **`Status:`** / **`Implementation:`** — mirror [`../specifications/README.md`](../specifications/README.md#status-header)
- **`Depends on:`** / **`Defers:`** — explicit scope boundaries

Naming: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md). When a plan lands, move it to [`archive/`](archive/README.md) and update [`../specifications/traceability.yaml`](../specifications/traceability.yaml).
