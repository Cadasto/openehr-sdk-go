# RM function deferred stubs (arithmetic + refs/inverse-navigation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.
>
> **⚠ YAGNI / DoR gate — read first.** Both clusters below are `Out of scope` in [rm-functions.md](../specifications/rm-functions.md) **because no consumer needs them yet**, and neither has a REQ. This plan exists so the work is *ready*, not as a signal to start. **Do not execute** until a real consumer need appears; when it does, run Phase 0 (`sdd-specify`) first — code-first here would violate the SDD no-code-first guardrail.

**Date:** 2026-09-01
**Status:** Draft (parked — awaiting consumer need)
**Owner:** SDK maintainers
**Covers:** proposed **REQ-124** (data-value arithmetic) and proposed **REQ-125** (reference accessors + inverse navigation) — both to be authored in the 120–129 band per [rm-functions.md § Editing rules](../specifications/rm-functions.md#editing-rules). No id is allocated until Phase 0.
**Probes:** TBD at Phase 0 (behavioural — unit-covered).
**Implementation:** planned
**Depends on:** landed REQ-120 (identifiers), REQ-121 (path read access), REQ-123 (temporal helpers); [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) (behavioural-function surface).
**Defers:** the `VERSIONED_OBJECT` container operations and `commit_*` mutators — those are **server-mediated by contract** (REQ-122) and MUST stay fail-loud stubs; they are not in this plan.

**Goal:** Realise the two `rm-functions` clusters currently left as documented, fail-loud panic stubs — (1) arithmetic on the `DV_AMOUNT` datatypes, (2) reference convenience accessors plus inverse path navigation — replacing each `panic("not implemented …")` with spec-anchored behaviour, **when** a consumer needs them.

**Architecture:** Both clusters extend the reflection-free RM behavioural surface (ADR 0011): generated stubs live in `*_gen.go`, real implementations in hand-written sibling files (`_funcs.go`). Cluster 1 is pure value computation; Cluster 2's inverse navigation (`parent`, `path_of_item`) is the hard part — the concrete RM structs carry **no parent back-pointers**, so it needs a design decision (an ADR) before code.

**Tech Stack:** Go, `openehr/rm`, ISO 8601 duration math, `errors` (REQ-025 — no panics on bad input).

## Global Constraints

- **Library code MUST NOT panic** on malformed input or absent paths (rm-functions.md invariant / REQ-025) — failures are a returned error, a zero value, or an empty result.
- **Reflection-free** (REQ-024): typed dispatch only, no `reflect`.
- **Each cluster is independently shippable.** They share nothing; split into two plans if that suits delivery better.

## Definition of Ready (per cluster, at Phase 0)

- A REQ is allocated and its normative section authored (`sdd-specify`) — REQ-124 for arithmetic, REQ-125 for refs/inverse-nav — with acceptance criteria and the value-set/invariant rules copied from the openEHR RM.
- For Cluster 2's `parent`/`path_of_item`: an **Accepted ADR** settling how parent linkage is carried reflection-free (the irreversible fork — see Task 2).

## Definition of Done (per cluster)

- Every named `panic("not implemented …")` stub is replaced by a real method in a `_funcs.go` file, with `// REQ-124`/`// REQ-125` citations.
- Malformed/edge inputs return errors or zero values — proven by tests — never panic.
- rm-functions.md `Out of scope` note for that cluster is removed and the REQ section says `landed`; REQ.md `Impl.` + `traceability.yaml` updated; numbering band table updated (new id consumed).
- `make spec-check` / `make ci` pass. Plan archived.

## Phases

### Phase 0: Specify (both clusters) — `sdd-specify`

- [ ] Allocate **REQ-124** and author its rm-functions.md section: arithmetic on `DV_QUANTITY`, `DV_COUNT`, `DV_PROPORTION`, `DV_DURATION` — `add`, `subtract`, `multiply`, `negative`, and `DV_DURATION` calendar-nominal `add_nominal`/`subtract_nominal` — with the RM guards (e.g. `DV_QUANTITY.add` requires equal `units`; result validity).
- [ ] Allocate **REQ-125** and author its section: `OBJECT_REF`/`PARTY_REF`/`PARTY_PROXY` convenience accessors (SHOULD-level; `LOCATABLE_REF.as_uri` is the one already realised), and `PATHABLE.parent` / `path_of_item` inverse navigation.
- [ ] For REQ-125 inverse navigation, open an ADR (below) and get it **Accepted** before Task 2 code.

### Task 1 (Cluster 1 — REQ-124): DV_AMOUNT arithmetic

**Files:**
- Modify: `openehr/rm/data_types_quantity_gen.go` stubs are *called through*; implement in `openehr/rm/data_types_quantity_funcs.go` (create).
- Test: `openehr/rm/data_types_quantity_funcs_test.go` (create).

**Stubs to realise** (from the grep census): `DV_QUANTITY.add/subtract/multiply` (`data_types_quantity_gen.go:613…`), `DV_COUNT.add/subtract/multiply` (`:122/:144/:151`), `DV_PROPORTION.add/subtract/multiply` (`:397/:434/:441`), `DV_AMOUNT.negative` (`:51…`), and the `DV_DURATION` arithmetic + nominal operators.

- [ ] **Step 1: Failing test** — e.g. `DV_QUANTITY{magnitude:1,units:"mg"}.add(DV_QUANTITY{magnitude:2,units:"mg"})` → `magnitude 3, units "mg"`; adding mismatched units → error, not panic; `DV_DURATION` `add_nominal("P1M")` across a short month follows calendar rules.
- [ ] **Step 2:** Run → panics/fails today.
- [ ] **Step 3:** Implement each operator in `_funcs.go` per the openEHR RM `Quantity` package semantics (units-match guard for `DV_QUANTITY`; ratio math for `DV_PROPORTION`; ISO-8601 duration math incl. nominal Y/M for `DV_DURATION`). Malformed → error.
- [ ] **Step 4:** Run → PASS. `make ci`.
- [ ] **Step 5:** Commit `feat(rm): DV_AMOUNT arithmetic (REQ-124)`.

### Task 2 (Cluster 2 — REQ-125): reference accessors + inverse navigation

**Files:**
- Implement accessors + inverse nav in `openehr/rm/identification_funcs.go` / a new `common_archetyped_funcs.go`.
- Test: sibling `_test.go` files.

**Stubs to realise:** `OBJECT_REF`/`PARTY_REF`/`PARTY_PROXY` convenience accessors (parallel to the realised `LOCATABLE_REF.as_uri`); `PATHABLE.parent` (`common_archetyped_gen.go:1035…`, one per PATHABLE-bearing type) and `path_of_item`.

- [ ] **Step 1 (accessors, easy):** Failing tests for the `OBJECT_REF`/`PARTY_REF`/`PARTY_PROXY` accessors → implement (the underlying fields are already on the structs; these are convenience derivations) → PASS.
- [ ] **Step 2 (ADR, before inverse-nav code):** Decide how `parent`/`path_of_item` obtain parent linkage reflection-free — the RM structs carry no back-pointers. Options: (a) a navigation context/cursor that threads parents during a downward walk; (b) identity-based tree search from a root; (c) leave `parent` a documented stub and provide only `path_of_item(root, target)` via search. Record in `docs/adr/00NN-rm-inverse-navigation.md`; get it Accepted.
- [ ] **Step 3:** Implement per the ADR; malformed/absent → empty/error, never panic (REQ-025). Tests: known node → its path; typed-nil node → no match, no panic.
- [ ] **Step 4:** `make ci`. Commit `feat(rm): reference accessors + inverse navigation (REQ-125)`.

## Self-review

- **Coverage:** the two table rows map to Cluster 1 (REQ-124) and Cluster 2 (REQ-125). Both start behind Phase 0.
- **Independence:** the clusters share no code — a reviewer can accept one and reject the other; splitting into two plans is reasonable if delivery prefers it.
- **Biggest risk:** `parent`/`path_of_item` is an architectural fork (no parent pointers in the RM), gated behind an ADR — do not code it ahead of that decision (the spec's [STRAND-10](../specifications/research-strands.md#strand-10--rmpath-one-sentinel-for-two-not-found-conditions) neighbourhood).

## Mapping to specs

- [rm-functions.md § REQ-123 Out of scope](../specifications/rm-functions.md#req-123--temporal-data-value-helpers) — arithmetic deferral (→ REQ-124).
- [rm-functions.md § REQ-120 / § REQ-121 Out of scope](../specifications/rm-functions.md#req-121--locatable-path-read-access) — accessors + `parent`/`path_of_item` deferral (→ REQ-125).
- [rm-functions.md § Editing rules](../specifications/rm-functions.md#editing-rules) — new behaviour gets a new 120–129 REQ.
