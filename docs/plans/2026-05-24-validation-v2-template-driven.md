# Plan — REQ-102 v2: template-driven composition validation

**Date:** 2026-05-24
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-102 (structural completion); REQ-103 (primitive reuse); REQ-104 (slot grammar — partial overlap); REQ-013 (building-block independence)
**Probes:** PROBE-025 (extend); proposed **PROBE-026** (missing required node / cardinality negative cases)
**Implementation:** landed (Phases 0–4 complete; see [Implementation checklist](#implementation-checklist) for per-phase status and recorded deviations from the original plan)
**Depends on:** [`2026-05-21-validation.md`](2026-05-21-validation.md) Phase 1 (landed — RM-guided primitive pass); [`2026-05-22-template-req100-followups.md`](2026-05-22-template-req100-followups.md) Phases 4–6 (compiled template + walker + REQ-103)
**Defers:** External terminology lookup (REQ-105); full ADL2 / AOM 2; validate wire bytes; federated archetype repository for slot-fill resolution

## Goal

Close the structural validation gaps in `ValidateComposition` so the SDK enforces what the **OPT and RM spec require**, not only what happens to be present in an incoming composition graph.

The public signature stays stable:

```go
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result
```

v2 completes REQ-102's deferred dimensions (existence, cardinality, alternatives, missing nodes, RM-type match) by making the **compiled OPT the walk driver** and the composition the value source.

---

## Problem statement

### What v1 does today

Phase 1 landed an **RM-guided constraint lookup**:

1. Descend the composition via typed switches (`validateObservation`, `walkItemsAttribute`, …).
2. Build AQL paths using the composition's `archetype_node_id` values as predicates.
3. Call `c.NodeAt(path)` to find OPT constraints at those paths.
4. Apply REQ-103 primitive checks and spot identity checks when a path resolves.

Documented honestly in [`openehr/validation/doc.go`](../../openehr/validation/doc.go) § Validation trust model.

### Why that is insufficient

The OPT defines a **required shape**. Validation must be able to fail when the composition is **missing** nodes the template mandates, even when no RM subtree exists to start a walk from.

| Scenario | v1 behaviour | Spec expectation |
|---|---|---|
| OPT requires `ELEMENT` at `/…/items[at0004]`; composition omits it | No issue — path never visited | `required` / `cardinality` |
| OPT requires `ELEMENT.value` (existence ≥ 1); element present, `Value == nil` | No issue — explicitly skipped | `required` |
| OPT allows `DV_TEXT \| DV_CODED_TEXT`; composition carries wrong alternative | May skip if path/type mismatch | `type_mismatch` / alternative failure |
| OPT declares min/max on multi-valued attribute; composition has too few/many children | Not checked (except empty `/content` special case) | `cardinality` |
| Composition carries extra RM node not in OPT | Not flagged | policy-dependent (v2: optional `warning` or out-of-scope) |
| Wrong at-code on existing node | `node_id_mismatch` when path absent; may miss constraints at correct OPT path | identity + constraint failure |

**Root cause:** paths are computed from **untrusted composition metadata**, then used as lookup keys. Missing composition nodes produce missing paths, so required OPT nodes are invisible.

### What the spec implies (normative intent)

ADL 1.4 OPT validation semantics (AOM 1.4 `C_COMPLEX_OBJECT` / `C_ATTRIBUTE`):

- **Existence** on an attribute — must the RM property be non-null?
- **Cardinality** on `C_MULTIPLE_ATTRIBUTE` — how many child constraint paths may/must match?
- **Occurrences** on a child object — how many instances of that constrained object?
- **Alternatives** under `C_SINGLE_ATTRIBUTE` — exactly one child constraint path must match.
- **Archetype / node identity** — `archetype_node_id` on LOCATABLE must match pinned ids.
- **RM type** — instance type must match `rm_type_name` on the matching constraint node.
- **Primitive constraints** — leaf values must satisfy REQ-103 (already landed for matched paths).

The validator must treat the **template as authoritative for structure** and the **composition as the instance under test**.

---

## Target architecture

### Template-driven lockstep walk

Replace the RM-guided deep walk with a **lockstep** traversal: at each compiled OPT node, read the corresponding RM value(s) by `rm_attribute_name`, then enforce constraints.

```
CompiledNode (OPT)                    RM instance
─────────────────                    ───────────
For each CompiledAttribute attr:     val := readRM(parent, attr.Name())
  enforce existence / cardinality     compare val against attr + children
  for each child constraint:          recurse into matched RM child(ren)
  at primitive leaf:                  PrimitiveConstraint.Validate(val)
```

**Path strings in issues** come from `CompiledNode.AQLPath()` (compile-time, trusted), not from composition-supplied predicates.

**RM property access** uses the existing typed RM graph — closed switches per parent RM type + attribute name (no reflection; REQ-024). This is the idiomatic Go equivalent of iterating `$attribute->rm_attribute_name` in reference implementations, not a port of their class hierarchy.

### Relationship to v1 code

| v1 artefact | v2 disposition |
|---|---|
| `validateRequiredAttrs` (composition root BMM) | Keep — still valid; may fold into template walk at root |
| `validateRootArchetype` | Keep — or emit from template root node check |
| `validateContent` slot-fit | Keep logic; invoke from template `/content` attribute handler |
| `composition_deep.go` RM walkers | **Replace** for structural + primitive passes; extract shared `dataValueInput` / primitive dispatch |
| `checkLocatableIdentity` | **Fold into** template-node handler (identity is checked when binding RM to OPT child) |
| REQ-103 `constraints.*.Validate` | **Reuse unchanged** |

Single entry point; internally either one unified walk or two sequential passes (structural then primitive) that share an RM cursor — prefer **one walk** to avoid drift.

### Walker placement

Extend [`internal/templatecompile/walk`](../../internal/templatecompile/walk/) with **`WalkComposition`** (name from original validation plan):

- Input: `*templatecompile.Compiled`, `*rm.Composition`
- Maintains an **RM cursor** stack parallel to the OPT node stack
- Visitor accumulates `[]Issue` (collect-all; never abort early)
- OPT-only `walk.Walk` remains for tooling; composition validation uses the lockstep variant

The validator package (`openehr/validation/`) implements the visitor + RM read helpers; the walk machinery stays in `internal/templatecompile/walk` next to the existing OPT walker.

---

## Compile / parser prerequisites

Some OPT metadata needed for v2 is **not yet on the compiled surface**:

| Metadata | Wire location | Parser today | Needed for |
|---|---|---|---|
| Attribute **existence** | `<existence>` on `<attributes>` | Parsed → `CompiledAttribute.Existence()` | Required attribute checks |
| Attribute **single vs multiple** | `xsi:type` on attribute | Parsed → `CompiledAttribute.Cardinality()` | Single vs collection read |
| Attribute **cardinality interval** | `<cardinality><interval>…` under `C_MULTIPLE_ATTRIBUTE` | **Not parsed** (`xmlCAttribute` has no field) | Min/max child counts |
| Object **occurrences** | `<occurrences>` on `<children>` | Parsed → `CompiledNode.Occurrences()` | Per-child instance counts |
| **Alternatives** | Multiple `<children>` under `C_SINGLE_ATTRIBUTE` | Available as `CompiledAttribute.Children()` | AnyOf matching |
| Slot **includes/excludes** | `<includes>` / `<excludes>` | Raw strings on slot nodes | REQ-104 (optional v2 phase) |

**Phase 0 task:** extend `openehr/template` parse + `internal/templatecompile` compile to capture `<cardinality>` on `C_MULTIPLE_ATTRIBUTE` → `CompiledAttribute.Occurrences()` (or `ChildCardinality()`) distinct from existence. Existence answers "must the attribute be filled?"; cardinality answers "how many children under a filled attribute?".

Until that lands, v2 can enforce existence + node occurrences + alternative matching; full multiplicities on container attributes use existence lower bound as a floor and defer upper-bound to Phase 2.

---

## Validation dimensions (v2 completion matrix)

| Dimension | v2 target | Issue code | Sentinel |
|---|---|---|---|
| Missing OPT-required attribute (existence lower ≥ 1) | Template walk finds no RM value | `required` | `ErrRequired` |
| Missing required child object (occurrences lower ≥ 1) | Template child not matched in RM | `required` | `ErrRequired` |
| Too few / too many children (cardinality / occurrences) | Count RM children vs interval | `cardinality` | `ErrCardinality` |
| `C_SINGLE_ATTRIBUTE` — no alternative matches | Try each child constraint against RM value | `alternative_mismatch` | `ErrTypeMismatch` (or new sentinel) |
| RM type ≠ OPT `rm_type_name` | Type switch / BMM name check | `rm_type_mismatch` | `ErrTypeMismatch` |
| Archetype / node id pinning | Compare LOCATABLE id to OPT node | `archetype_id_mismatch` / `node_id_mismatch` | `ErrTypeMismatch` |
| Primitive leaf constraints | REQ-103 at bound RM value | `primitive_*` | `ErrPrimitive` |
| `/content` slot fill | Declared roots + REQ-104 when ready | `slot_fill` | `ErrSlotFill` |
| Extra RM nodes not in OPT | — | **Out of v2** unless product asks | — |

---

## Phases

### Phase 0 — Metadata + spec alignment

**Outcome:** Compiled template carries every interval the validator needs; REQ-102 spec updated to describe v2 as the structural completion of the same entry point.

**Tasks:**

1. **Parse `<cardinality>` on `C_MULTIPLE_ATTRIBUTE`** — extend `xmlCAttribute`, wire `Attribute`, compile into `CompiledAttribute`.
2. **Accessor** — `CompiledAttribute.ChildMultiplicity() *template.Multiplicity` (name TBD; document existence vs child-count semantics in godoc).
3. **Update [`docs/specifications/clinical-modeling.md`](../../docs/specifications/clinical-modeling.md) § REQ-102** — move deferred rows to v2; document template-driven trust model; add issue codes (`alternative_mismatch`, …).
4. **Update [`docs/specifications/traceability.yaml`](../../docs/specifications/traceability.yaml)** — note REQ-102 remains `partial` until v2 Phase 3; add PROBE-026 stub row.
5. **Cross-link** this plan from [`2026-05-21-validation.md`](2026-05-21-validation.md) checklist.

**Definition of done:** Parser/compile tests for cardinality interval; `make spec-check` green; no validator behaviour change yet.

---

### Phase 1 — RM cursor (`rmread` internal package)

**Outcome:** Given `(parentRM, parentRMType, attrName)`, return the RM value(s) at that attribute without reflection.

**Tasks:**

1. Add `openehr/validation/rmread/` (or `internal/validationrm/`) — **stdlib + `openehr/rm` only** for the read table; the parent `validation` package orchestrates.
2. **Core API:**
   ```go
   // ReadSingle returns the RM value for a Single attribute, or (nil, false) when absent.
   ReadSingle(parent any, parentType, attrName string) (val any, ok bool)

   // ReadMultiple returns the RM collection for a Multiple attribute (slice or nil).
   ReadMultiple(parent any, parentType, attrName string) (items []any, ok bool)
   ```
3. Implement rows for every `(RMType, attr)` reachable from `COMPOSITION` down through the Phase 1 content-type closed set (Observation, Evaluation, Instruction, Action, AdminEntry, Section, GenericEntry) plus ItemStructure / Item / Event / DataValue paths already in v1 walkers.
4. **Tests:** table-driven — given fixture RM subgraph + attribute name → expected values; no OPT involved.

**Definition of done:** `rmread` covered for all paths exercised by `vital_signs.opt` and `clinical_note.opt`; `go test ./openehr/validation/...` green.

---

### Phase 2 — Template-driven structural visitor

**Outcome:** Missing required nodes and cardinality violations are reported with OPT-authoritative paths.

**Tasks:**

1. **`walk.WalkComposition(comp, c, visitor)`** — lockstep DFS; visitor receives `(optNode, rmValue, path)`.
2. **Structural handler** per visited OPT node:
   - For each `CompiledAttribute` on the node (skip `Implicit()` unless OPT silent + BMM mandatory policy says otherwise):
     - Read RM via `rmread`.
     - **Existence:** if interval lower ≥ 1 and RM absent → `Issue{Path: node.AQLPath()+"/"+attr.Name(), Code: "required"}`.
     - **Single:** exactly one RM value; match against one of `attr.Children()` (alternative loop).
     - **Multiple:** count RM items; check attribute cardinality interval and each child's occurrences.
   - **Identity:** when binding RM LOCATABLE to OPT child, compare `archetype_node_id` to `child.ArchetypeID()` / `child.NodeID()`.
   - **RM type:** concrete RM type must match `child.RMTypeName()` (with abstract RM interface categories per BMM).
3. **Slot nodes:** delegate to existing slot-fit logic; REQ-104 grammar is a follow-up swap-in.
4. **Collect-all:** visitor appends to `[]Issue`; walk never short-circuits.
5. **Wire into `ValidateComposition`:** run structural visitor; merge issues with any retained root checks.

**Tests:**

- `vital_signs.opt` + complete composition → OK.
- Remove systolic element entirely → `required` at `/content[…]/data/events[at0006]/data/items[at0004]`.
- Set `Element.Value = nil` where OPT existence requires value → `required` at `…/value`.
- Empty events where OPT requires ≥1 → `cardinality` at `…/events`.
- Wrong RM type under single attribute → `rm_type_mismatch`.

**Definition of done:** Missing-node cases above pass; PROBE-026 draft cases green in sandbox.

---

### Phase 3 — Primitive pass on template bindings

**Outcome:** REQ-103 runs at every primitive leaf the template declares, bound to the RM value found by the structural walk — no composition-built path lookup.

**Tasks:**

1. During structural walk, when OPT node has `PrimitiveConstraint() != nil` and RM value is present → `Validate(rmInput)`.
2. Reuse `dataValueInput` dispatch from v1.
3. **Remove** RM-guided `composition_deep.go` walkers once template walk covers the same fixture paths (delete dead code in same PR to avoid dual maintenance).
4. Extend PROBE-025 / PROBE-026 with primitive + structural combined cases.

**Definition of done:** All existing `openehr/validation` tests rewritten against template-driven behaviour; no regression on PROBE-025 code multiset; `composition_deep*.go` removed or reduced to test helpers only.

---

### Phase 4 — Alternatives, occurrences polish, probes

**Outcome:** `C_SINGLE_ATTRIBUTE` AnyOf semantics and occurrences upper bounds match AOM 1.4.

**Tasks:**

1. **Alternative matching:** for Single attributes with N children, try each child constraint against the RM value; succeed on first match; if none match → `alternative_mismatch` listing allowed RM types.
2. **Occurrences upper bound** on child objects and attribute cardinality upper bound.
3. **PROBE-026** — sandbox probe: shared fixture tuples `(opt, composition, wantCodes)` including missing-node and cardinality cases; cross-SDK stable code multiset like PROBE-025.
4. **REQ-102 → `landed`** in traceability when Phase 2–4 complete and spec updated.

**Definition of done:** `make ci` green; REQ-102 structural rows no longer marked deferred in clinical-modeling.md.

---

## RM read + match algorithm (sketch)

For a `CompiledNode` `N` and RM value `R` (nil when missing):

```
for each attr in N.Attributes():
    if attr.Cardinality == Single:
        v, ok := rmread.ReadSingle(R, N.RMTypeName(), attr.Name())
        if attr.Existence.Lower >= 1 && !ok → required
        if !ok → continue
        if !matchOneOf(v, attr.Children()) → alternative_mismatch | rm_type_mismatch | ...
    else: // Multiple
        items, ok := rmread.ReadMultiple(R, N.RMTypeName(), attr.Name())
        if attr.Existence.Lower >= 1 && len(items) == 0 → required
        check count vs attr.ChildMultiplicity()
        for each child constraint C in attr.Children():
            match RM items to C by id / slot rules
            recurse WalkComposition(childOptNode, matchedRM)
```

`matchOneOf` is the Go equivalent of `C_SINGLE_ATTRIBUTE` trying each child validator until one succeeds.

---

## Testing strategy

| Layer | What |
|---|---|
| `rmread` unit tests | Attribute read table in isolation |
| `walk` integration | Lockstep walk on compiled fixture OPT + hand-built RM |
| `validation` unit | End-to-end `ValidateComposition` on `vital_signs.opt` / `clinical_note.opt` |
| PROBE-025 | Extend — existing primitive cases must still pass |
| PROBE-026 (new) | Missing required node, cardinality, alternative failure — **code multiset only** for cross-SDK parity |
| Regression guard | Test that fails if validator only walks RM nodes (e.g. omit entire Observation → must NOT return OK) |

---

## Non-goals (v2)

- **Extra RM nodes** not declared in OPT — report as warning or ignore; not required for REQ-102 landing.
- **External terminology** — REQ-105.
- **Full ARCHETYPE_SLOT assertion grammar** — REQ-104 (slot-fit prefix fallback remains until then).
- **Demographic / AQL validators** — still Phase 2 of [`2026-05-21-validation.md`](2026-05-21-validation.md).
- **Wire-byte validation** — no `serialize/` import.
- **Performance tuning** — correct single-pass collect-all first; memoisation (Archie `APathQueryCache` pattern) only if profiling demands it.

---

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0: parse + compile attribute `<cardinality>` interval | landed |
| Phase 0: REQ-102 spec + traceability update | landed |
| Phase 1: `rmread` attribute access table | landed |
| Phase 2: `walk.WalkComposition` lockstep walker | landed (see deviation 1 below) |
| Phase 2: structural visitor (existence, cardinality, identity, type) | landed (see deviation 2 below) |
| Phase 2: missing-node tests on `vital_signs.opt` | landed |
| Phase 3: primitive pass on template bindings | landed |
| Phase 3: remove RM-guided `composition_deep.go` walkers | n/a — branch was off main; v1 walkers never existed here |
| Phase 4: `C_SINGLE_ATTRIBUTE` alternative matching | landed |
| Phase 4: PROBE-026 | landed |
| REQ-102 `implementation: landed` | landed |

### Deviations from the original plan

1. **Walker placement.** Plan § Phase 2 placed the lockstep walker inside `internal/templatecompile/walk/` next to the existing OPT walker. Implementation lives in `openehr/validation/walk_composition.go` instead — the visitor is tightly coupled to validation issue emission (alternatives, identity, primitive dispatch) and the abstraction was not load-bearing for other consumers. If a future caller surfaces (composition builder, example generator) and wants the same lockstep machinery, extract then.

2. **Implicit attribute policy.** Plan § Phase 2 said "skip `Implicit()` unless OPT silent + BMM mandatory policy says otherwise". Implementation now runs the existence check on every non-skipped attribute including implicits (BMM `Required()` flag drives the emit). Rationale: implicit attrs are precisely the BMM-mandatory ones that the OPT didn't repin — silently skipping them would let COMPOSITION.composer / .language / .territory go unchecked even though BMM mandates them. No descent happens for implicits because `Children()` is empty.

3. **`alternative_mismatch` vs `rm_type_mismatch` disambiguation.** Plan § Phase 4 emits `alternative_mismatch` when no C_SINGLE_ATTRIBUTE child fits. Implementation refines this: with exactly one OPT child the code is `rm_type_mismatch` (plain type constraint, not AnyOf); with two or more children the code stays `alternative_mismatch`. Captured in [`docs/specifications/clinical-modeling.md`](../specifications/clinical-modeling.md) § REQ-102 issue-code taxonomy.

---

## References

- [`openehr/validation/doc.go`](../../openehr/validation/doc.go) — v1 trust model
- [`docs/specifications/clinical-modeling.md`](../../docs/specifications/clinical-modeling.md) § REQ-102 — normative validator contract
- [`internal/templatecompile/walk/doc.go`](../../internal/templatecompile/walk/doc.go) — OPT walker; `WalkComposition` deferred note
- [`docs/plans/2026-05-21-validation.md`](2026-05-21-validation.md) — original plan (template-driven intent; Phase 1 landed as RM-guided partial)
- openEHR AM 1.4 Template XSD — `C_SINGLE_ATTRIBUTE`, `C_MULTIPLE_ATTRIBUTE`, existence vs cardinality
- [openEHR/archie RMObjectValidator](https://github.com/openEHR/archie) — per-dimension validator split (occurrence, multiplicity, primitive) — informational, not parity target
