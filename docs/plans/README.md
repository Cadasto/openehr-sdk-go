# Implementation plans

Active and archived implementation plans for `openehr-sdk-go`. Plans derive from [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md). **Completed or superseded plans:** [`archive/README.md`](archive/README.md).

## Active plans

### SDK-GAP-15/16/17 — v0.11.0 dossiers (Draft, 2026-06-29)

Three independent gaps filed by a consuming CDR project after v0.11.0; each plan stands alone (no shared code path) and may land in any order, on the same branch or separate ones.

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-06-29-sdk-gap-15-rm-floor-validation.md](2026-06-29-sdk-gap-15-rm-floor-validation.md) | Template-less RM validation entry (`ValidateRM`) — RM-invariant floor beneath the template-driven path | proposed REQ-112; PROBE-077 |
| [2026-06-29-sdk-gap-16-stored-query-rest-conformance.md](2026-06-29-sdk-gap-16-stored-query-rest-conformance.md) | `openehr-ehr-id` header scoping on POST execution; `Location` parsing in `PutStoredQuery` | REQ-055 / REQ-057 (no new REQ); PROBE-078 / PROBE-079 |
| [2026-06-29-sdk-gap-17-aql-execution-ast.md](2026-06-29-sdk-gap-17-aql-execution-ast.md) | Stable, generated-type-free read AST for parsed AQL (`parse.Query`); interim `Document.Tree()` accessor | proposed REQ-113; PROBE-080 |

### Simplified formats — WebTemplate + FLAT/STRUCTURED (planned umbrella)

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-06-23-simplified-formats.md](2026-06-23-simplified-formats.md) | Umbrella — shared simplified-template model → WebTemplate JSON export + FLAT/STRUCTURED codecs | REQ-053, proposed REQ-106; PROBE-075/076 |
| [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md) | WebTemplate JSON export — umbrella Phase 2 (deferred) | proposed REQ-106; PROBE-075 |

The Phase 2 clinical-building-blocks umbrella **landed and was archived** ([archive/2026-05-21-phase-2-clinical-building-blocks.md](archive/2026-05-21-phase-2-clinical-building-blocks.md)); its remaining deferred scope (simplified formats) is now sequenced by the umbrella above. The two AQL plans also landed and moved to `archive/` — AQL builders ([REQ-055](archive/2026-05-21-aql-builders.md)) and AQL parse + lint ([REQ-109](archive/2026-06-15-aql-lint.md)).

**Landed (archived):** OPT parser, REQ-100 follow-ups (Phases 1–8), composition validation (REQ-102), composition builder (REQ-101), template-driven instance generator (REQ-107), REQ-104 slot assertions + REQ-105 terminology bindings (PR #43), C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission, BMM codegen, canonical JSON/XML, AQL builders (REQ-055), AQL static lint (REQ-109), validation beyond COMPOSITION (REQ-110 — demographic PARTY hierarchy + FOLDER / EHR_STATUS), the public compiled-template bridge (REQ-111, ADR 0010), the SMART-on-openEHR auth conformance audit (REQ-061..064/068, REQ-070..072; ADR 0008/0009; STRAND-05), polymorphic `_type` round-trip stability (SDK-GAP-13; REQ-052/040/102/107), and seeded synthetic value generation (SDK-GAP-14 value-fill + seed; REQ-103/107/101) — see [archive/](archive/README.md). The umbrella validation scope first sketched in the archived [umbrella validation plan](archive/2026-05-21-validation.md) is now complete.

## Header convention (load-bearing)

Every plan MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — REQ-NNN / PROBE-NNN / STRAND-NN identifiers
- **`Status:`** / **`Implementation:`** — mirror [`../specifications/README.md`](../specifications/README.md#status-header)
- **`Depends on:`** / **`Defers:`** — explicit scope boundaries

Naming: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md). When a plan lands, move it to [`archive/`](archive/README.md) and update [`../specifications/traceability.yaml`](../specifications/traceability.yaml).
