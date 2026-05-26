# Implementation plans

Active and archived implementation plans for `openehr-sdk-go`. Plans derive from [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md). **Completed or superseded plans:** [`archive/README.md`](archive/README.md).

## Active plans

| Plan | Scope | Covers REQs / strands |
|---|---|---|
| [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) | openEHR REST 1.1.0-development typed client family | REQ-050..057, REQ-013..026, REQ-060..072, REQ-090..092; PROBE-010..013, PROBE-040..049; STRAND-01 |
| [2026-05-25-req094-prefer-followups.md](2026-05-25-req094-prefer-followups.md) | REQ-094 write-path gaps (**not landed**) | REQ-094; PROBE-065 |
| [2026-05-26-contribution-submission-shape.md](2026-05-26-contribution-submission-shape.md) | `contribution.Commit` request body must be `Contribution_create` (inline `data`, not `OBJECT_REF`) — **not landed** (SDK-GAP-10) | REQ-050; PROBE-072 (proposed) |
| [2026-05-25-versioning-strategy.md](2026-05-25-versioning-strategy.md) | Go module semver, tags, release gates, compatibility metadata | REQ-001, REQ-004 |

### Phase 2 — clinical building blocks (in flight)

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-05-21-phase-2-clinical-building-blocks.md](2026-05-21-phase-2-clinical-building-blocks.md) | Umbrella — sequencing and dependency rules | REQ-013, REQ-014 |
| [2026-05-22-template-req100-followups.md](2026-05-22-template-req100-followups.md) | REQ-100 hardening, compiled template, REQ-103–105 (Phases 7+ open) | REQ-100, REQ-103; PROBE-022, PROBE-024 |
| [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md) | WebTemplate JSON export (deferred) | proposed REQ-106 |
| [2026-05-26-c-primitive-object-wire-parser.md](2026-05-26-c-primitive-object-wire-parser.md) | C_PRIMITIVE_OBJECT inner-item parsing + REQ-107 UID emission (Phases 1–2 **not landed**; PR #20 follow-up landed PROBE-027 extension) | REQ-100, REQ-107, REQ-101; PROBE-023 widening, PROBE-027 extension |
| [2026-05-21-aql-builders.md](2026-05-21-aql-builders.md) | AQL struct + verb builders | REQ-055; PROBE-020, PROBE-021 |

**Landed (archived):** OPT parser, composition validation (REQ-102), composition builder (REQ-101), template-driven instance generator (REQ-107), BMM codegen, canonical JSON/XML — see [archive/](archive/README.md). **Remaining validation scope** (demographic, AQL lint) is noted in the archived [umbrella validation plan](archive/2026-05-21-validation.md) and tracked under the Phase 2 umbrella.

## Header convention (load-bearing)

Every plan MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — REQ-NNN / PROBE-NNN / STRAND-NN identifiers
- **`Status:`** / **`Implementation:`** — mirror [`../specifications/README.md`](../specifications/README.md#status-header)
- **`Depends on:`** / **`Defers:`** — explicit scope boundaries

Naming: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md). When a plan lands, move it to [`archive/`](archive/README.md) and update [`../specifications/traceability.yaml`](../specifications/traceability.yaml).
