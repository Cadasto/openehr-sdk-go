# Plan — Simplified formats (WebTemplate export + FLAT/STRUCTURED) — umbrella

**Date:** 2026-06-23
**Status:** Draft (proposed umbrella — planned; not yet approved for implementation)
**Owner:** SDK maintainers
**Covers:** [REQ-053](../specifications/wire.md#req-053) (FLAT / STRUCTURED composition codecs); **proposed REQ-106** (WebTemplate JSON export — child plan [`2026-05-22-webtemplate-export.md`](2026-05-22-webtemplate-export.md)). Cross-links the compiled-template foundation [REQ-100](../specifications/clinical-modeling.md)/[REQ-111](../specifications/module-layout.md), [REQ-103](../specifications/clinical-modeling.md) primitives, [REQ-107](../specifications/clinical-modeling.md) instance synthesis, [REQ-102](../specifications/transport.md) validation.
**Probes:** PROBE-075 (WebTemplate export structural conformance), PROBE-076 (FLAT/STRUCTURED composition round-trip) — **reserved** (next free after PROBE-074).
**Implementation:** planned
**Depends on:** landed compiled-template foundation — `openehr/template/` + the public bridge `openehr/templatecompile/` (REQ-111), REQ-103 primitive constraints, REQ-107 `instance.Generate`. All shipped; no new prerequisites.
**Defers:** round-trip WebTemplate JSON → OPT; ADL2 `.opt2` / `.oet` / `.t.json` *authoring*-template parsing (the SDK consumes the flattened `.opt`); byte-exact parity with any single reference implementation; multi-version WebTemplate output.

## Goal

Build the SDK's **simplified-formats** stack on a single shared internal model derived from the compiled OPT, then expose it two ways: (1) **WebTemplate JSON** (REQ-106 — a UI/form-generation schema) and (2) **FLAT (simSDT) + STRUCTURED (structSDT) composition codecs** (REQ-053 — simplified data-entry/ingestion payloads). Consumers: form renderers, AQL-builder UIs, FHIR-mapping tools, and any integration that commits composition data against a known template without hand-building canonical RM.

## Why one umbrella (the key architectural insight)

WebTemplate export (REQ-106) and FLAT/STRUCTURED (REQ-053) were previously filed as unrelated tracks. They are not: per the format research ([openehr-kb note](../../../openehr-kb/reference/notes/openehr-template-and-composition-formats.md) — *if the kb is not checked out, see the source repos in §References*), the **Web Template node tree IS the machinery FLAT/STRUCTURED needs**. Both reference implementations (EHRbase `openEHR_SDK`, Better `web-template`) generate flat paths from exactly this model:

```
*template.Compiled (OPT, flattened)
        │  build once
        ▼
SIMPLIFIED TEMPLATE MODEL   ← shared: id-tree + aqlPath + inputs/suffixes + :index rules
        │                                   │
        ▼ serialize                         ▼ drive path<->value mapping
WebTemplate JSON (REQ-106)          FLAT (simSDT) / STRUCTURED (structSDT) (REQ-053)
application/openehr.wt+json         application/openehr.wt.flat+json / …wt.structured+json
```

A node's `id` (camelCase "web id"), its `aqlPath`, and its leaf `inputs` (with attribute suffixes `|magnitude`, `|unit`, `|code`, …) plus zero-based `:index` rules define both the WebTemplate JSON *and* every FLAT path key. Build the model once: **REQ-106 = serialize it; REQ-053 = use it to encode/decode compositions.** Doing either in isolation rebuilds ~80% of the other.

## Sequencing (phases — implementation detail lives in the child plans)

| Phase | Scope | REQ | Output |
|---|---|---|---|
| **1 — Shared model** | A `*template.Compiled` → simplified-template node tree: id-generation (the hard, consumer-critical part — must mirror the chosen reference), per-leaf input/suffix derivation, `aqlPath`, occurrences/cardinalities, `:index` semantics. Pure transform, no tree mutation. | foundation for REQ-053/106 | internal model package (e.g. `openehr/template/simplified/` or `openehr/simplified/`) |
| **2 — WebTemplate JSON export** | Serialize the model to the Better/EHRbase JSON shape; deterministic field order; golden files; structural conformance vs reference fixtures. | **REQ-106** | child plan [`2026-05-22-webtemplate-export.md`](2026-05-22-webtemplate-export.md); PROBE-075 |
| **3 — FLAT / STRUCTURED codecs** | Encode `*rm.Composition` → FLAT (`path\|attr: value` map) and STRUCTURED (nested objects); decode back (needs the model/OPT). Canonical media types — **not** EHRbase's `.schema` variants. | **REQ-053** | new child plan (to author when Phase 1 lands); PROBE-076 |

Phases 2 and 3 are independent once Phase 1 exists; either can ship first by consumer demand.

## Decisions to lock before implementation (Definition of Ready)

- **Reference implementation:** target **EHRbase `openEHR_SDK`** (Java, actively maintained, `version "2.3"`) over Better `web-template` (Kotlin; frozen 2021, build-rot reports). Record as an [ADR](../adr/) if it forks behaviour.
- **Pin the WebTemplate `version`** we emit (EHRbase emits `"2.3"`).
- **`id`-generation algorithm** is the load-bearing decision (consumers' FLAT paths depend on it): document the exact sanitisation + sibling-disambiguation rules mirroring the reference; ADR if it diverges.
- **Author the specs:** REQ-106 has **no registry row yet** — add canonical prose to `docs/specifications/clinical-modeling.md` + a `REQ.md` row; flesh `REQ-053` in `wire.md`. Do **not** implement against an unregistered REQ.
- **Media types:** emit the canonical openEHR *Simplified Formats* strings `application/openehr.wt.flat+json` / `application/openehr.wt.structured+json` and `application/openehr.wt+json`; treat EHRbase's `.schema`-suffixed strings as a known upstream bug (be liberal on input only).

## Conformance corpus (for PROBE-075/076)

Matched **OPT → WebTemplate → FLAT/STRUCTURED** fixture sets exist upstream and pin behaviour:

- EHRbase `openEHR_SDK` `test-data/.../{operationaltemplate,webtemplate,composition/flat/simSDT,composition/flat/structured}/` — e.g. the `corona_anamnese` trio.
- Better `web-template` `src/test/resources/compatibility/{templates,compositions/{flat,structured,raw}}/` — e.g. the `Vital Signs Pathfinder Demo` trio.

**Parity is structural, not byte-exact** — `id` sanitisation, `version`, and field ordering differ across implementations; maintain an explicit documented-deviations list rather than chasing diff-zero.

## Out of scope (entire umbrella)

- Round-trip from WebTemplate/FLAT/STRUCTURED back to an OPT (the simplified forms are lossy by design).
- Parsing `.oet` / `.t.json` *authoring* templates or ADL2 `.opt2` — the SDK consumes the flattened ADL 1.4 `.opt` ([roadmap](../roadmap.md): AOM 2.4 deferred).
- UI rendering (consumer's job — these plans emit/consume data only).
- Terminology expansion of `inputs[].list` against an external TERM service.

## Definition of Ready

- REQ-053 and proposed REQ-106 each have canonical spec prose + a `REQ.md` registry row.
- Reference implementation + WebTemplate `version` + `id`-algorithm choices recorded (ADR where a fork is irreversible).
- Phase child plans list concrete tasks and the verification command (`make ci`, probes).

## Definition of Done

- Phase 1 model + Phase 2 (REQ-106) and/or Phase 3 (REQ-053) land with `// REQ-` / `// PROBE-` citations.
- `traceability.yaml` + `REQ.md` **Impl.** reflect what shipped; canonical spec prose updated in the same PR.
- PROBE-075/076 pass against the conformance corpus (modulo the documented-deviations list).
- `make spec-check` + `make ci` green.

## Mapping to specs

- [`docs/specifications/wire.md` § REQ-053](../../docs/specifications/wire.md#req-053) — FLAT/STRUCTURED contract (to be fleshed)
- proposed **REQ-106** — `docs/specifications/clinical-modeling.md` + `REQ.md` row (to author)
- [`docs/specifications/REQ.md`](../../docs/specifications/REQ.md) — registry
- [`docs/roadmap.md`](../roadmap.md) — FLAT/STRUCTURED row (REQ-053, Planned)

## References (informational)

- **openehr-kb note** — `openehr-kb/reference/notes/openehr-template-and-composition-formats.md` (sibling repo): the layered format map, WebTemplate de-facto schema, FLAT/STRUCTURED path grammar, media-type table, and commit-pinned sources. Primary design grounding.
- **EHRbase `openEHR_SDK`** — [`github.com/ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK), `web-template/` module (`WebTemplateNode`, `OPTParser`, `FlatPathDto`/`FlatPathParser`, `InputHandler`). The recommended living reference.
- **Better `web-template`** — [`github.com/better-care/web-template`](https://github.com/better-care/web-template) (Kotlin; older, frozen test suite).
- **openEHR ITS-REST Simplified Formats** (current, STABLE → 1.1.0) — <https://specifications.openehr.org/releases/ITS-REST/development/simplified_formats.md>. Canonical media types; supersedes "Simplified Data Template".
