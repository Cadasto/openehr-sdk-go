# Plan — WebTemplate export (JSON-format simplified template representation)

**Date:** 2026-05-22
**Status:** Draft (deferred — child of the [simplified-formats umbrella](2026-06-23-simplified-formats.md); no committed delivery window)
**Owner:** SDK maintainers
**Parent / shared model:** Phase 2 of [`2026-06-23-simplified-formats.md`](2026-06-23-simplified-formats.md). The WebTemplate node tree (`id` / `aqlPath` / `inputs`+suffixes / `:index`) is the **same shared simplified-template model** that REQ-053 (FLAT/STRUCTURED) consumes — build it once (umbrella Phase 1); this plan serialises it to JSON.
**Covers:** proposed REQ-106 (WebTemplate JSON export). **REQ-107** is reserved for OPT→RM instance synthesis — [`2026-05-24-template-instance-example-generator.md`](archive/2026-05-24-template-instance-example-generator.md).
**Implementation:** planned
**Depends on:** the landed compiled-template foundation (`openehr/template/` + public bridge `openehr/templatecompile/` REQ-111) + REQ-103 primitive constraints; sequenced after the umbrella's Phase 1 shared model.
**Defers:** Round-trip from WebTemplate JSON back to OPT; JSON-schema-conformant validation against a reference implementation

## Goal

Export the SDK's internal compiled template (REQ-100 + Phase 4 of the follow-up plan) as a JSON-format simplified template representation suitable for UI / form-generation consumers. This format is widely used in the openEHR ecosystem for client-side rendering, AQL builder UIs, and form-driven data entry. The originating reference is the [better-care WebTemplate format](https://github.com/better-care/web-template).

## Why a separate plan

The compiled template (`template.Compiled` in the follow-up plan Phase 4) is **internal SDK infrastructure** consumed by:

- Composition builder (REQ-101)
- Composition validator (REQ-102)
- Example data generator
- Eventually, AQL-from-template synthesis

It is **not** wire-stable, **not** a public-API JSON contract, **not** consumer-facing.

The JSON-format simplified template (a.k.a. "WebTemplate") is the **opposite** — a **consumer-facing** projection:

- A **vendor de-facto** serialisation (Better → EHRbase) consumed by browsers, forms, third-party tools. It is **not** a normative openEHR template artefact — only the downstream FLAT/STRUCTURED *serialization* is standardised (ITS-REST *Simplified Formats*). Treat it as a public contract we must keep stable for consumers, not as a spec we can appeal to.
- Has its own field naming convention (camelCase `id`, suffixed inputs, computed flags) that does not match the AOM 1.4 element shapes.
- Includes computed values (AQL paths, choice indicators, input lists per primitive) that the internal compiled tree carries but in a different representation.
- Crosses a wire boundary — once exposed, breaking changes are painful for consumers in other languages.

Conflating the two would couple SDK internals to a public JSON contract and force every internal refactor through a JSON-compatibility lens. Keeping them separate lets the compiled template evolve freely while the JSON export remains a stable consumer surface. (The intermediate **shared simplified-template model** — umbrella Phase 1 — is where the projection lives; this plan only serialises it.)

## What WebTemplate carries (informational)

The format is well-documented externally. Each template element carries:

- `id` — a sanitised camelCase "web id" from RM type + node id/term, with sibling disambiguation. **The exact `id`-generation algorithm is implementation-specific (EHRbase vs Better differ) and consumer-critical** — FLAT path keys are built from it, so it MUST mirror the chosen reference exactly. This is the hardest part of the model.
- `name` — language-resolved display text from term definitions.
- `rmType` — RM class name.
- `nodeId` — archetype id or at-code.
- `min` / `max` — cardinality bounds.
- `aqlPath` / `path` — stable AQL path string.
- `children` — recursive structure.
- `inputs` — per-primitive constraint, one per logical input slot (e.g. a quantity has two inputs: `magnitude` decimal, `unit` coded-text).
- `termBindings` — flattened term bindings per language.
- `annotations` — UI hints from `<annotations path="...">` blocks.
- `archetypeSlot` flag for slot-fill points.
- `cardinalities` — container-side cardinality on `CLUSTER.items`, `ITEM_TREE.items`, etc.

## Approach (high-level, not committed)

1. **Spec REQ-106** in `docs/specifications/clinical-modeling.md` + a `REQ.md` registry row (REQ-106 is **not yet registered**): pin which reference implementation and version we mirror (**recommend EHRbase `openEHR_SDK`, `version "2.3"`** — Better's repo is frozen/build-rotted), what we add, what we deliberately omit, and the stability guarantees to consumers.
2. **Shared simplified-template model** (umbrella Phase 1) — pure transform from `*template.Compiled` to the node tree (`id`/`aqlPath`/`inputs`/`:index`), consumed by this plan **and** REQ-053. No mutation of the compiled tree. The input is the **flattened compiled OPT** — never `.oet`/`.t.json` (authoring artefacts the SDK does not parse).
3. **`Marshal(model) ([]byte, error)`** — serialise the shared model to the WebTemplate JSON shape. Deterministic field order; stable across SDK versions.
4. **Round-trip golden files** — for each fixture OPT, store a checked-in JSON output. Tests assert byte-equality on regeneration.
5. **Conformance probe PROBE-075** (PROBE-026 is already REQ-102's) — compare SDK-emitted JSON against the EHRbase `openEHR_SDK` `webtemplate/*.json` fixtures for the same OPT. **Structural parity, not byte-exact** (id sanitisation / version / field ordering differ) — maintain a documented-deviations list.

## What this plan does NOT do

- **Round-trip from JSON back to OPT** — the simplified format is lossy by design (e.g. ontology details are flattened, expression-level slot assertions are dropped). Recovering an OPT from the JSON is not a goal.
- **Schema-conformance certification** against any reference implementation — initial scope is structural alignment; pixel-for-pixel parity is a later goal subject to consumer demand.
- **UI rendering** — that's the consumer's job. This plan only emits the data.

## Trigger conditions

Open this plan for active development when **any** of:

- A direct consumer requests JSON-format template output (typical demand: form-generation tooling, FHIR-mapping UIs).
- Composition builder (REQ-101) is landed and a UI consumer needs the same template metadata client-side.
- A conformance probe against an existing reference implementation is needed.

Until then this plan is a placeholder reserving the design space, the proposed REQ-106 identifier, and a clear deferral path for reviewers asking "where does WebTemplate fit?". It is now sequenced as **Phase 2 of the [simplified-formats umbrella](2026-06-23-simplified-formats.md)** (shared model → this JSON export → REQ-053 FLAT/STRUCTURED).

## Out of scope (this plan)

- Round-trip from WebTemplate JSON back to OPT.
- Multi-version output (the reference format has evolved; this plan picks one version).
- Direct JSON-schema validation of incoming data against a WebTemplate (validation is REQ-102's concern, against the OPT, not the WebTemplate JSON).

## Implementation checklist

| Step | Status |
|---|---|
| REQ-106 spec authored | not started |
| `openehr/template/jsonexport/` sub-package | not started |
| `Marshal(*template.Compiled) ([]byte, error)` | not started |
| Round-trip golden files for fixture OPTs | not started |
| Cross-implementation conformance probe (PROBE-075) | not started |

## Mapping to specs

- Pending: REQ-106 (JSON-format simplified template export) — **not yet in `REQ.md`; register before implementing** (umbrella DoR)
- Foundation: [REQ-100 follow-up plan](archive/2026-05-22-template-req100-followups.md) Phase 4 (compiled template) + Phase 6 (REQ-103 primitives)

## References (informational)

- **better-care WebTemplate** — [`github.com/better-care/web-template`](https://github.com/better-care/web-template). Originating reference for the JSON-format simplified template; includes JSON schema and worked examples.
- **ehrbase openEHR_SDK WebTemplate generation** — [`github.com/ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK), `web-template/` Maven module. A Java reference that compiles raw OPT to WebTemplate JSON; covers the format's edge cases on `DV_QUANTITY` units, `CODE_PHRASE` external code lists, and `EVENT` choice handling. **The recommended target** (actively maintained; `version "2.3"`).
- **openehr-kb format note** — `openehr-kb/reference/notes/openehr-template-and-composition-formats.md` (sibling repo): layered format map, the WebTemplate de-facto schema, FLAT/STRUCTURED path grammar, and the media-type table (incl. EHRbase's non-conformant `.schema` variant — do **not** copy it), all with commit-pinned sources.
- **openEHR ITS-REST Simplified Formats** (current, STABLE → 1.1.0; canonical media types, supersedes "Simplified Data Template") — <https://specifications.openehr.org/releases/ITS-REST/development/simplified_formats.md>
