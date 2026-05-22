# Plan — REQ-100 template parser follow-ups (post-#10)

**Date:** 2026-05-22
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-100 (hardening); PROBE-022 (breadth)
**Implementation:** planned
**Depends on:** [2026-05-21-template-parser.md](2026-05-21-template-parser.md) (PR #10 landing)
**Defers:** REQ-101 composition builder (consumes hardened paths); AOM 2 / OET

## Goal

Close gaps identified in PR #10 review **without** blocking merge of the initial REQ-100 landing. Work in small PRs or commits on `main` after #10 merges; follow this checklist in order.

**Do not duplicate** the fix-now items called out in PR #10 review comments (error `%w` chain, `Root()` spec/godoc, stdlib-only dependency text, fixture README, `opt-parse` example errors).

## When to run

| Trigger | Action |
|---|---|
| After PR #10 merges | Open follow-up branch `feat/template-req100-followups` from `main` |
| Before REQ-100 `implementation: landed` in traceability | Complete **Phase 1** below |
| Before composition builder Phase 1 | Complete **Phase 2** path/slot coverage |

## Phase 1 — Tests and traceability honesty

**Outcome:** REQ-100 test surface matches normative claims; traceability `landed` is defensible.

**Tasks:**

1. **`TestNodeAt_PredicateAtCode`** — `path_test.go`: resolve a stable at-code from `vital_signs.opt` via `NodeAt` (not only `ParsePath` accept table).
2. **`TestParseFile_VitalSigns_ContainsSlot`** — find `*template.Slot` in tree; assert `Includes()` non-empty where fixture has `<includes>`.
3. **`TestNodeAt_CannotDescendSlot`** — path into slot child → `errors.Is(err, ErrPathNotFound)`.
4. **`TestParseFile_ClinicalNote_Path`** — at least one deep `/content/...` assertion on `clinical_note.opt`.
5. **`TestParseOPT_UnsupportedAttributeType`** — minimal XML with unknown attribute `xsi:type`; assert `errors.Is(..., ErrUnsupportedNode)` **after** parent PR fixes `%w` in `parse.go`.
6. **PROBE-022** — extend `probes_test.go` assertions: one at-code path; optional second fixture body for `clinical_note.opt`.
7. **Align status labels** — pick one ladder and sync `traceability.yaml`, `roadmap.md`, and template parser plan header:
   - Recommended: `partial` until Phase 1 done; `landed` only when tests + spec edits complete.
8. **`conformance.md`** — add coverage-matrix row: Clinical modeling / PROBE-022 → `testkit/probes/template/`.

**Definition of done:** `make ci` green; REQ-100 `implementation: landed` in `traceability.yaml` only if Phase 1 complete.

## Phase 2 — API hardening (optional strict modes)

**Outcome:** Safer defaults for production callers; no breaking change to default parse behavior.

**Tasks:**

1. **Getter immutability** — `Attributes()`, `Children()`, `Includes()`, `Excludes()` return `slices.Clone` of internal slices (or document copy-on-read in godoc if semver prefers deferral).
2. **`NodeKind()`** — add `NodeKind` iota + `func (n Node) Kind() NodeKind` on sealed implementations to reduce consumer type switches.
3. **`TrimSpace` on `ArchetypeID()`** — parse path in `buildArchetypeRoot` / promotion branch (`parse.go`).
4. **Unknown child `xsi:type` with children** — choose and document:
   - **A)** recurse via `buildComplexObject` when attributes present, or
   - **B)** return `ErrUnsupportedNode` when unknown type has nested XML, or
   - **C)** add `ParseOPTStrict(...)` option (default remains forward-compatible leaf).
   Update REQ-100 § Node taxonomy accordingly.
5. **Trailing XML** — after `Decode`, reject non-whitespace tokens until EOF (`ErrInvalidOPT`).
6. **BOM handling** — propagate `Peek`/`Discard` errors as `ErrInvalidOPT` wrap.
7. **`ParseFile` I/O** — wrap `os.Open` with context path; preserve `fs.ErrNotExist` via `%w`.
8. **Parse edge tests** — `ParseOPT(nil)`, non-`<template>` root, `.OPT` extension case-insensitive acceptance.

**Definition of done:** New tests for each behavior; CHANGELOG bullet only if public API adds options/types.

## Phase 3 — Ergonomics (before REQ-101)

**Outcome:** Composition builder consumers hit fewer footguns.

**Tasks:**

1. **`ErrAmbiguousPath`** (new sentinel) — when predicate-less segment has `len(children) > 1`, or duplicate predicate match; optional `WithStrictPaths()` on `OperationalTemplate` resolution (default: current first-child rule per REQ-100).
2. **`ValidatePath(p Path) error`** — optional walk that checks segment names exist on tree (today `ParsePath` is syntax-only).
3. **`Multiplicity` validation** — reject `lower > upper` at parse time if both set (or document opaque interval until validation REQ).

## Out of scope (this plan)

- OET parse; ADL 2 OPT; Archie linker; terminology expansion (unchanged REQ-100 v1 bounds).
- Importing `openehr/aom/aom14/` into parser (defer until constraint payloads are needed for REQ-102 validation).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 tests + PROBE-022 breadth | |
| Traceability `landed` + conformance matrix row | |
| Phase 2 immutability / strict parse (if adopted) | |
| Phase 3 ambiguity / ValidatePath (if needed for composition) | |
| `make ci` | |

## Mapping to specs

- [docs/specifications/clinical-modeling.md § REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths)
- [docs/specifications/conformance.md § PROBE-022](../specifications/conformance.md)
