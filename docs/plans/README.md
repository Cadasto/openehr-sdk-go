# Implementation plans

Active and historical implementation plans for `openehr-sdk-go`. Plans are derivations of [`../../specs/`](../../specs/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md) — feature matrix and milestones (updated when phases land; plans may lag).

| Plan | Scope | Covers REQs / strands |
|---|---|---|
| [2026-05-15-bmm-codegen.md](2026-05-15-bmm-codegen.md) | BMM-driven code generation for `openehr/rm/`, `openehr/aom/aom14/` | REQ-041..047; part of STRAND-04 |
| [2026-05-15-canonical-json-serialization.md](2026-05-15-canonical-json-serialization.md) | Canonical JSON encoder + decoder under `openehr/serialize/canjson/` | REQ-052, REQ-040; PROBE-030, PROBE-031; STRAND-04 (polymorphism side) |
| [2026-05-15-canonical-xml-serialization.md](2026-05-15-canonical-xml-serialization.md) | Canonical XML encoder + decoder under `openehr/serialize/canxml/` | REQ-056, REQ-040 |
| [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) | openEHR REST 1.1.0-development typed client family under `openehr/client/{system,ehr,query,definition,demographic,admin}/` | REQ-050, REQ-051, REQ-054, REQ-055, REQ-057, REQ-058, REQ-013..026, REQ-060..072, REQ-090..092; PROBE-010..013, PROBE-040..049; STRAND-01 |

### Phase 2 — clinical building blocks (2026-05-21)

Deliver in order; umbrella plan holds sequencing and dependency rules.

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-05-21-phase-2-clinical-building-blocks.md](2026-05-21-phase-2-clinical-building-blocks.md) | Umbrella — template → composition → validation → AQL builders | REQ-013, REQ-014; links REQ-055, REQ-053 (deferred) |
| [2026-05-21-template-parser.md](2026-05-21-template-parser.md) | `openehr/template/` — ADL 1.4 OPT (`.opt`) parse, `OperationalTemplate`, paths; OET out of scope | REQ-100 (proposed); PROBE-022 (proposed) |
| [2026-05-21-composition-builder.md](2026-05-21-composition-builder.md) | `openehr/composition/` — generic OPT-driven builder | REQ-101 (proposed); PROBE-023 (proposed) |
| [2026-05-21-validation.md](2026-05-21-validation.md) | `openehr/validation/` — comp↔OPT, demographic, AQL lint | REQ-102 (proposed); PROBE-024 (proposed); PROBE-021 (execute) |
| [2026-05-21-aql-builders.md](2026-05-21-aql-builders.md) | `openehr/aql/` — struct + verb builders (executor already landed) | REQ-055; PROBE-020, PROBE-021 |

Naming convention: `YYYY-MM-DD-<short-title>.md`. New plans: copy [_template.md](_template.md). Each plan cites REQ-IDs / STRAND-IDs and links to **canonical** spec sections only (specs/[README.md § Traceability](../../specs/README.md#traceability)).
