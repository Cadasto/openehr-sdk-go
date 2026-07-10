# Plan — SDK-GAP-15: template-less Reference Model validation

**Date:** 2026-06-29
**Status:** Landed (PR #57, 2026-06-30)
**Owner:** SDK maintainers
**Covers:** **REQ-112** (template-less RM validation floor) — extends the existing template-driven contract in [REQ-102](../../specifications/clinical-modeling.md#req-102--composition-validation) and [REQ-110](../../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition); reuses [REQ-120..123](../../specifications/rm-functions.md) and the BMM introspection at [`openehr/rm/rminfo`](../../../openehr/rm/rminfo).
**Probes:** PROBE-077 (RM-floor invariant matrix) — deferred to a follow-up cycle; unit-test cassette matrix in [`rmfloor_test.go`](../../../openehr/validation/rmfloor_test.go) covers the first-cycle catalogue.
**Implementation:** landed
**Depends on:** REQ-110 walker generalisation already landed (`openehr/validation/validate.go`, `walk_composition.go`, `rmread/`); REQ-120..123 RM behavioural helpers landed in v0.10.0.
**Defers:** terminology / external-code validation; archetype-level constraints (already owned by REQ-102/110); consumer-side gating and the HTTP 422 surface.
**Source (inbound):** a consuming CDR project — filed against v0.11.0 for RM-structural validation of non-template-bound writes (FOLDER, EHR_STATUS, EHR_ACCESS, untemplated demographic PARTY) where the consumer had been running on a strict-canjson-decode proxy that catches type-shape but not RM invariants.

## Goal

Expose a **template-less** Reference Model validation entry point — `validation.ValidateRM(root any) Result` (and typed sugars for the closed RM root set) — that walks any RM root and reports `Issue`s for BMM-derived existence/cardinality breaches and RM type invariants, in the same `Result`/`Issue` model the OPT-driven validators already use. The OPT-driven path stays the authoritative template-conformance layer; this is the **RM-only floor beneath it** for writes that bind to no template (FOLDER, EHR_STATUS, EHR_ACCESS, untemplated demographic PARTY).

## Problem

Every entry point in [`openehr/validation`](../../../openehr/validation) ([`validate.go`](../../../openehr/validation/validate.go), [`ValidateComposition`](../../../openehr/validation/composition.go), `ValidateFolder` / `ValidateEHRStatus` / `ValidateDemographic`) accepts a `*templatecompile.Compiled` as the authoritative driver — the package doc states *"Validation is template-driven."* A CDR persisting non-template-bound resources therefore has no SDK call to assert RM conformance. Its strongest substitute today is a strict `canjson` typed decode, which proves JSON↔type correctness but **not** RM invariants — RM-mandatory-attribute omissions decode cleanly, as do `DV_INTERVAL` lower>upper, empty `CODE_PHRASE.code_string`, mis-paired `DV_ORDINAL` symbol/value, and cardinality violations on container attributes.

The SDK already carries every building block required: `rminfo` (`RequiredAttributes` / `AttributeRMType` / `KnownRMTypes`), the RM behavioural functions, and the value-source-generic walker `rmread`. Only an OPT-independent driver is missing.

## Definition of Ready (analysis gate)

Implementation may start when:

- [x] Maintainer sign-off on the **driver split** — **Option A** (second driver alongside `*Compiled`) chosen 2026-06-29.
- [x] **Covers:** finalized — promote new REQ-112 section under [`clinical-modeling.md`](../../specifications/clinical-modeling.md) 2026-06-29; REQ-110 stays exactly as-is. REQ.md row added at status `Draft` in Phase 2 alongside the prose.
- [ ] PROBE-077 cassette set agreed — minimum: an `EHR_STATUS` missing `subject`, a `FOLDER` missing `name`, a `DV_INTERVAL` with `lower>upper`, a `CODE_PHRASE` with empty `code_string`. Each cassette: input JSON + expected `Issue` (path + code).

## Accepted approach (2026-06-29)

Two drivers are under consideration; both share the **walker** (the existing `walk_composition` shape generalised) and the **invariant evaluators** (small, declarative — one per RM type).

### Option A — second driver alongside `*Compiled` (chosen 2026-06-29)

A new internal driver type — e.g. `validate.rmFloorDriver` — that satisfies the same walk contract as the template-driven driver but sources mandatory-attribute and type-membership facts from `rminfo` instead of an OPT. The walker reuses `rmread/` for the value-side and dispatches per-RM-type invariant evaluators alongside the existing structural checks. The public API forks once at the entry layer:

- `validation.ValidateRM(root any) Result` — generic.
- Typed sugar: `ValidateRMFolder(f *rm.Folder) Result`, `ValidateRMEHRStatus(s *rm.EHRStatus) Result`, `ValidateRMEHRAccess`, `ValidateRMDemographic`.

**Pros.** Zero risk to the OPT-driven walkers; the invariant catalogue is shared (template-driven gets the RM-invariant checks for free if not already present); narrow blast radius.
**Cons.** Two driver types to keep in sync if the walker ever grows new hook points.

### Option B — make the walker driver an interface, parameterise

Refactor `walk_composition` so its driver dependency is an interface satisfied by both `*templatecompile.Compiled` and a new `rmFloor` value. The public entries route to the same walker with different drivers.

**Pros.** One walker, two drivers cleanly switched at the seam.
**Cons.** Touches the validated REQ-102/110 path; the interface surface has to be carved out carefully; bigger diff.

**Lean.** Option A unless review surfaces a strong reason to consolidate. Either option lands the same public surface and the same invariant evaluators.

### Invariant evaluators (shared, declarative)

A first-cycle set, each a tiny function over the RM-typed value:

- `DV_INTERVAL[T]` — `lower` ≤ `upper` when both bounds are set; per-end `*_included` defaults respected.
- `CODE_PHRASE` — `code_string` non-empty (RM spec) and `terminology_id` set.
- `DV_QUANTITY` — `magnitude`/`units` co-presence; precision non-negative when set.
- `DV_ORDINAL` — `value` integer; `symbol` set; pair coherent (no orphan symbol/value).
- `DV_CODED_TEXT` — `value` non-empty when `defining_code` is set (already partially checked in REQ-102 path).
- `OBJECT_REF` family — `id`, `type` set; `id_scheme` non-empty.
- Container attribute lower bounds — driven by `rminfo.RequiredAttributes` (e.g. `EHR_STATUS.subject`, `FOLDER.name`).

Each evaluator returns `[]Issue` with the standard path + code; codes reuse the existing taxonomy where applicable, with a new `RMInvariant` family code for type-level invariants.

## Phases

### Phase 1 — analysis & sign-off (this plan)

**Tasks:**
- Record maintainer sign-off on Option A vs B.
- Finalise REQ-112 prose under `clinical-modeling.md` (or REQ-110 extension); register the row in [`REQ.md`](../../specifications/REQ.md) at status `Draft`.
- Pick the PROBE-077 cassette matrix; add the stub cassettes to `testkit/cassettes/rm/floor/`.

**Definition of done:** REQ-112 row exists; cassette directory in place; this plan flipped Draft → Ready.

### Phase 2 — invariant evaluators + driver

**Tasks:**
- Land the per-RM-type invariant evaluators as a sub-package (`openehr/validation/rminvariant/` or top-level functions in `openehr/validation/`).
- Land the chosen driver (Option A or B).
- Wire the entry points: `ValidateRM` + the four typed sugars. Public package doc updated to distinguish *template-driven* vs *RM-floor* surfaces.

**Definition of done:** `make ci` green; invariant evaluators carry `// REQ-112` citations; entries documented in `doc.go`.

### Phase 3 — PROBE-077 + traceability close-out

**Tasks:**
- Land the PROBE-077 test driving the cassette matrix; assert path + code per Issue.
- Update [`traceability.yaml`](../../specifications/traceability.yaml): REQ-112 → `openehr/validation` + PROBE-077.
- Add a paragraph to `openehr/validation/doc.go` distinguishing the two surfaces; flip REQ-112 row status to `Stable` once probes pass.

**Definition of done:** PROBE-077 green; `make spec-check` green; CHANGELOG `[Unreleased]` bullet drafted.

## Acceptance criteria

- For a structurally-decodable but RM-invalid root (e.g. `EHR_STATUS` missing `subject`, `FOLDER` missing `name`, `DV_INTERVAL` with `lower>upper`, `CODE_PHRASE` with empty `code_string`), `ValidateRM` returns a `Result` carrying the invariant violation(s) with stable `path` + `code`.
- For a valid root, `ValidateRM` returns `Result{OK: true}`.
- No new dependency on `*templatecompile.Compiled` for the floor; the template-driven entries are unchanged and pass their existing tests verbatim.
- `make ci` and `make spec-check` green; `traceability.yaml` lists REQ-112.

## Out of scope

- Terminology binding / external-code validation (separate concern, future REQ).
- Archetype-level constraints — already covered by the template-driven path.
- Consumer-side gating (the `TEMPLATE_VALIDATION` posture and HTTP 422 mapping live in the consumer).

## Risks / open questions

- **`Issue` code stability.** Reusing the existing taxonomy is preferable; introducing a `RMInvariant` family is fine but must be additive (no renumbering of existing codes). Confirm at sign-off.
- **Invariant catalogue completeness.** First cycle covers the dossier's named cases plus the obvious neighbours. A follow-up sweep against the RM spec (data types section) likely surfaces more — track as a successor plan once REQ-112 lands.
- **Performance.** The floor walker runs over the whole RM root; for large FOLDERs this may need the same short-circuiting hooks REQ-102 has. Measure during Phase 2.

## Mapping to specs

- [docs/specifications/clinical-modeling.md § REQ-112 (to be added)](../../specifications/clinical-modeling.md) — normative contract.
- [docs/specifications/REQ.md](../../specifications/REQ.md) — registry row.
- [docs/specifications/traceability.yaml](../../specifications/traceability.yaml) — REQ → package / probe.
