# Implementation plans

Active and historical implementation plans for `openehr-sdk-go`. Plans are derivations of [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md) — feature matrix and milestones (updated when phases land; plans may lag).

| Plan | Scope | Covers REQs / strands |
|---|---|---|
| [2026-05-15-bmm-codegen.md](2026-05-15-bmm-codegen.md) | BMM-driven code generation for `openehr/rm/`, `openehr/aom/aom14/` | REQ-041..047; part of STRAND-04 |
| [2026-05-15-canonical-json-serialization.md](2026-05-15-canonical-json-serialization.md) | Canonical JSON encoder + decoder under `openehr/serialize/canjson/` | REQ-052, REQ-040; PROBE-030, PROBE-031; STRAND-04 (polymorphism side) |
| [2026-05-15-canonical-xml-serialization.md](2026-05-15-canonical-xml-serialization.md) | Canonical XML encoder + decoder under `openehr/serialize/canxml/` | REQ-056, REQ-040 |
| [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) | openEHR REST 1.1.0-development typed client family under `openehr/client/{system,ehr,query,definition,demographic,admin}/` | REQ-050, REQ-051, REQ-054, REQ-055, REQ-057, REQ-058, REQ-013..026, REQ-060..072, REQ-090..092; PROBE-010..013, PROBE-040..049; STRAND-01 |
| [2026-05-25-req094-prefer-followups.md](2026-05-25-req094-prefer-followups.md) | REQ-094 write-path gaps: `Prefer=identifier` slot + `representation` empty-body error (**Implementation: not landed**) | REQ-094; defers PROBE-065 |

### Phase 2 — clinical building blocks (2026-05-21)

Deliver in order; umbrella plan holds sequencing and dependency rules.

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-05-21-phase-2-clinical-building-blocks.md](2026-05-21-phase-2-clinical-building-blocks.md) | Umbrella — template → composition → validation → AQL builders | REQ-013, REQ-014; links REQ-055, REQ-053 (deferred) |
| [2026-05-21-template-parser.md](2026-05-21-template-parser.md) | `openehr/template/` — ADL 1.4 OPT (`.opt`) parse, `OperationalTemplate`, paths; OET out of scope | REQ-100; PROBE-022 |
| [2026-05-22-template-req100-followups.md](2026-05-22-template-req100-followups.md) | REQ-100 hardening + clinical-modeling foundation: tests, parser hardening, compiled template + RMInfoLookup + walker, primitive constraints (REQ-103), slot assertions (REQ-104), terminology bindings (REQ-105) | REQ-100; PROBE-022, PROBE-023, PROBE-024; proposed REQ-103/104/105 |
| [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md) | WebTemplate JSON export (deferred — separate plan to keep internal compiled template decoupled from public JSON contract) | proposed REQ-106; PROBE-026 (proposed) |
| [2026-05-21-composition-builder.md](2026-05-21-composition-builder.md) | `openehr/composition/` — OPT-driven skeleton + path-assigning builder | REQ-101 (proposed); PROBE-023 (proposed) |
| [2026-05-21-validation.md](2026-05-21-validation.md) | Umbrella — demographic + AQL lint still **planned**; composition scope superseded by v2 plan | REQ-102 (umbrella); PROBE-021 (execute) |
| [2026-05-24-validation-v2-template-driven.md](2026-05-24-validation-v2-template-driven.md) | REQ-102 template-driven `ValidateComposition` (**landed**) | REQ-102, REQ-103; PROBE-025, PROBE-026 |
| [2026-05-21-aql-builders.md](2026-05-21-aql-builders.md) | `openehr/aql/` — struct + verb builders (executor already landed) | REQ-055; PROBE-020, PROBE-021 |

## Header convention (load-bearing)

Every plan in this tree MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — list of REQ-NNN / PROBE-NNN / STRAND-NN identifiers this plan implements. A plan without a covered identifier is not landed in the registry.
- **`Status:`** — `Draft` / `Implemented (Sandbox)` / `Implemented (Partial)` / `Implemented` (mirrors topic-spec status; see [`../specifications/README.md` § Status header](../specifications/README.md#status-header)).
- **`Implementation:`** — `planned` / `partial` / `landed` (mirrors the REQ.md `Impl.` column).
- **`Depends on:`** / **`Defers:`** — other plans or landed packages this assumes; explicit out-of-scope.

Plans link to **canonical** spec sections only (one section per REQ); never duplicate the normative prose. See [`../specifications/README.md` § Traceability](../specifications/README.md#traceability) for the full chain.

Naming convention: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md).
