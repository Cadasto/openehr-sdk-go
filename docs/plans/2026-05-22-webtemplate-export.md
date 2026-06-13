# Plan — WebTemplate export (JSON-format simplified template representation)

**Date:** 2026-05-22
**Status:** Draft (deferred — no committed delivery window)
**Owner:** SDK maintainers
**Covers:** proposed REQ-106 (WebTemplate JSON export). **REQ-107** is reserved for OPT→RM instance synthesis — [`2026-05-24-template-instance-example-generator.md`](2026-05-24-template-instance-example-generator.md).
**Implementation:** planned
**Depends on:** [archive/2026-05-22-template-req100-followups.md](2026-05-22-template-req100-followups.md) Phase 4 (compiled template) + Phase 6 (REQ-103 primitive constraints) — both are prerequisites
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

The JSON-format simplified template (a.k.a. "WebTemplate") is the **opposite**:

- A **wire-stable serialisation** consumed by browsers, forms, third-party tools.
- Has its own field naming convention (camelCase, suffixed inputs, computed flags) that does not match the AOM 1.4 element shapes.
- Includes computed values (AQL paths, choice indicators, input lists per primitive) that the internal compiled tree carries but in a different representation.
- Crosses a wire boundary — once exposed, breaking changes are painful for consumers in other languages.

Conflating the two would couple SDK internals to a public JSON contract and force every internal refactor through a JSON-compatibility lens. Keeping them separate lets the compiled template evolve freely while the JSON export remains a stable consumer surface.

## What WebTemplate carries (informational)

The format is well-documented externally. Each template element carries:

- `id` — derived from RM type + node id (e.g. `dv_quantity` → `quantity_value`).
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

1. **Spec REQ-106** in `docs/specifications/clinical-modeling.md` documenting the JSON-format contract: which version of the reference format we mirror, what we add, what we deliberately omit, and the stability guarantees we make to consumers.
2. **`openehr/template/jsonexport/`** (new sub-package) — pure transformation from `*template.Compiled` to a typed Go struct mirroring the JSON shape. No mutation of the compiled tree.
3. **`Marshal(c *template.Compiled) ([]byte, error)`** — emits canonical JSON. Deterministic field order. Stable across SDK versions.
4. **Round-trip golden files** — for each fixture OPT, store a checked-in JSON output. Tests assert byte-equality on regeneration.
5. **Conformance probe** (PROBE-026 candidate) — compare SDK-emitted JSON against a reference-implementation-emitted JSON for the same OPT, modulo documented deviations.

## What this plan does NOT do

- **Round-trip from JSON back to OPT** — the simplified format is lossy by design (e.g. ontology details are flattened, expression-level slot assertions are dropped). Recovering an OPT from the JSON is not a goal.
- **Schema-conformance certification** against any reference implementation — initial scope is structural alignment; pixel-for-pixel parity is a later goal subject to consumer demand.
- **UI rendering** — that's the consumer's job. This plan only emits the data.

## Trigger conditions

Open this plan for active development when **any** of:

- A direct consumer requests JSON-format template output (typical demand: form-generation tooling, FHIR-mapping UIs).
- Composition builder (REQ-101) is landed and a UI consumer needs the same template metadata client-side.
- A cross-SDK conformance probe is needed against an existing reference implementation.

Until then this plan is a placeholder reserving the design space, the proposed REQ-106 identifier, and a clear deferral path for reviewers asking "where does WebTemplate fit?".

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
| Cross-implementation conformance probe (PROBE-026) | not started |

## Mapping to specs

- Pending: REQ-106 (JSON-format simplified template export)
- Foundation: [REQ-100 follow-up plan](archive/2026-05-22-template-req100-followups.md) Phase 4 (compiled template) + Phase 6 (REQ-103 primitives)

## References (informational)

- **better-care WebTemplate** — [`github.com/better-care/web-template`](https://github.com/better-care/web-template). Originating reference for the JSON-format simplified template; includes JSON schema and worked examples.
- **ehrbase openEHR_SDK WebTemplate generation** — [`github.com/ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK), `web-template/` Maven module. A Java reference that compiles raw OPT to WebTemplate JSON; covers the format's edge cases on `DV_QUANTITY` units, `CODE_PHRASE` external code lists, and `EVENT` choice handling.
