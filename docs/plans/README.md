# Implementation plans

Active and archived implementation plans for `openehr-sdk-go`. Plans derive from [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md). **Completed or superseded plans:** [`archive/README.md`](archive/README.md).

## Active plans

No standalone active plans. The SMART-on-openEHR auth conformance audit landed and moved to [`archive/`](archive/2026-06-16-auth-smart-conformance-audit.md) (REQ-061..064/068, REQ-070..072; ADR 0008/0009; STRAND-05). The next planned body of work is the simplified-formats umbrella below.

### Accepted dossiers — in implementation (branch `fix/sdk-gap-13-14`)

Both approved 2026-06-23; each carries an "Accepted approach" section recording the chosen fix.

| Plan | Scope | Covers REQs |
|---|---|---|
| [2026-06-23-sdk-gap-13-polymorphic-encode-decode.md](2026-06-23-sdk-gap-13-polymorphic-encode-decode.md) | Polymorphic `_type` encode/decode round-trip stability (sub-gap A encode via `jsonpoly` helper + sub-gap B validator bounds check) | REQ-052, REQ-040, REQ-102, REQ-107 |
| [2026-06-23-sdk-gap-14-seeded-synthetic-generation.md](2026-06-23-sdk-gap-14-seeded-synthetic-generation.md) | Seeded / varied synthetic value generation for `instance` / `NewSkeleton` (value-fill + seed; `medium` deferred) | REQ-103, REQ-107, REQ-101 |

### Simplified formats — WebTemplate + FLAT/STRUCTURED (planned umbrella)

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-06-23-simplified-formats.md](2026-06-23-simplified-formats.md) | Umbrella — shared simplified-template model → WebTemplate JSON export + FLAT/STRUCTURED codecs | REQ-053, proposed REQ-106; PROBE-075/076 |
| [2026-05-22-webtemplate-export.md](2026-05-22-webtemplate-export.md) | WebTemplate JSON export — umbrella Phase 2 (deferred) | proposed REQ-106; PROBE-075 |

The Phase 2 clinical-building-blocks umbrella **landed and was archived** ([archive/2026-05-21-phase-2-clinical-building-blocks.md](archive/2026-05-21-phase-2-clinical-building-blocks.md)); its remaining deferred scope (simplified formats) is now sequenced by the umbrella above. The two AQL plans also landed and moved to `archive/` — AQL builders ([REQ-055](archive/2026-05-21-aql-builders.md)) and AQL parse + lint ([REQ-109](archive/2026-06-15-aql-lint.md)).

**Landed (archived):** OPT parser, REQ-100 follow-ups (Phases 1–8), composition validation (REQ-102), composition builder (REQ-101), template-driven instance generator (REQ-107), REQ-104 slot assertions + REQ-105 terminology bindings (PR #43), C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission, BMM codegen, canonical JSON/XML, AQL builders (REQ-055), AQL static lint (REQ-109), validation beyond COMPOSITION (REQ-110 — demographic PARTY hierarchy + FOLDER / EHR_STATUS), the public compiled-template bridge (REQ-111, ADR 0010), and the SMART-on-openEHR auth conformance audit (REQ-061..064/068, REQ-070..072; ADR 0008/0009; STRAND-05) — see [archive/](archive/README.md). The umbrella validation scope first sketched in the archived [umbrella validation plan](archive/2026-05-21-validation.md) is now complete.

## Header convention (load-bearing)

Every plan MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — REQ-NNN / PROBE-NNN / STRAND-NN identifiers
- **`Status:`** / **`Implementation:`** — mirror [`../specifications/README.md`](../specifications/README.md#status-header)
- **`Depends on:`** / **`Defers:`** — explicit scope boundaries

Naming: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md). When a plan lands, move it to [`archive/`](archive/README.md) and update [`../specifications/traceability.yaml`](../specifications/traceability.yaml).
