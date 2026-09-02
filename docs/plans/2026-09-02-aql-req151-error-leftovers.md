# AQL + REQ-151 error leftovers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax. The three task groups are independent — pick any subset.

**Date:** 2026-09-02
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-055/057](../specifications/wire.md#req-057) (stored-query client path hygiene), [REQ-025](../specifications/idiom.md#errors-req-025) (nil-safe error inspection on the AQL/decode surface), and a coverage extension for [PROBE-099](../specifications/conformance.md) under REQ-160. No new id — amendments + tests.
**Probes:** PROBE-099 (extended with a discriminating row).
**Implementation:** planned
**Depends on:** landed `*transport.DecodeError` / `typereg.DecodeError` (REQ-151), `parse.SyntaxError`, the stored-query client.
**Defers:** nothing — this drains the residual AQL + REQ-151 error follow-ups recorded from the #137/#140/#151 review rounds.

**Goal:** Clear the small, verified AQL-side and decode-error leftovers that never got filed: trim the stored-query `version` path parameter consistently, make the `parse.SyntaxError` / `typereg.DecodeError` inspection sites nil-safe (an `AsType` match can carry a typed-nil pointer), and add the discriminating PROBE-099 row that makes containment-relation threading mutation-detectable at probe level.

**Architecture:** Three unrelated small fixes grouped as a cleanup sweep. Each is independently shippable; split if delivery prefers.

**Tech Stack:** Go (`errors.AsType`, `strings.TrimSpace`), `openehr/client/query`, `openehr/client/definition`, `openehr/aql/lint`, `openehr/aql/parse`, `testkit/probes/aql`.

## Global Constraints

- **No panics on bad input** (REQ-025): an `AsType` match that yields a typed-nil pointer MUST NOT be dereferenced.
- **Value-free diagnostics** (REQ-093) on any new error text.

## Definition of Done

- Each group's stubs/gaps closed with `// REQ-` citations and a test that fails before the fix.
- `make spec-check` / `make ci` pass. Plan archived.

## Phases

### Group 1: Trim the stored-query `version` path parameter — REQ-055/057

**Files:** `openehr/client/query/execute.go` (`runStoredAtVersion`, `:101-102`), `openehr/client/definition/stored_query.go` (`DeleteStoredQuery`, `:390`); tests in the sibling `_test.go`.

**State:** `qualifiedName` is trimmed (`stored_query.go:391` `name := strings.TrimSpace(qualifiedName)`), but `version` is used **raw** in the path: `runStoredAtVersion` does `path += "/" + version` with no trim (`execute.go:102`), and `DeleteStoredQuery` trims `version` only for the empty-check yet builds `Path` from the raw value (`:390` region). A `version` with surrounding whitespace passes the guard and reaches the wire un-trimmed — an asymmetry with `name`.

- [ ] **Step 1: Failing test.** `RunStoredVersion(…, " v1 ", …)` and `DeleteStoredQuery(…, " v1 ")` build a path containing `/v1`, not `/%20v1%20` or `/ v1 ` (capture the `Request.Path` via a stub transport).
- [ ] **Step 2:** Run → FAIL (raw value reaches the path).
- [ ] **Step 3:** Trim `version` once at the top of `runStoredAtVersion` and `DeleteStoredQuery` (mirroring `name`); reuse the trimmed value for both the empty-check and the path.
- [ ] **Step 4:** Run → PASS. `make ci`. Commit `fix(client): trim stored-query version path parameter (REQ-057)`.

### Group 2: Nil-safe `SyntaxError` / `DecodeError` inspection — REQ-025

**Files:** `openehr/aql/lint/lint.go` (`spanForError` ~`:204`, `syntaxDetail` ~`:210`), `openehr/aql/parse/emit_verify.go` (`:77`), and the `typereg` `Inner.Error()` site; tests in the sibling `_test.go`.

**State:** these sites do `if se, ok := errors.AsType[*parse.SyntaxError](err); ok { … se.Pos … }`. `errors.AsType` can report `ok == true` with a **typed-nil** pointer (recorded in the REQ-025 work — `AsType` is not nil-proof), so `se.Pos` panics if a typed-nil `*parse.SyntaxError` reaches the chain. Same shape for the `typereg.DecodeError` `Inner.Error()` call (loses `%v` panic recovery) and the `parse` dangling `"at 0:0:"` on nil/zero text.

- [ ] **Step 1: Failing test.** Feed each site an error chain carrying a typed-nil `*parse.SyntaxError` (`var se *parse.SyntaxError; err := fmt.Errorf("x: %w", se)`); assert no panic and a sensible fallback (`err.Error()` / a value-free message).
- [ ] **Step 2:** Run → PANIC/FAIL today.
- [ ] **Step 3:** Add `&& se != nil` to each `ok`-guarded branch (or switch to `errors.As` with an explicit nil check); guard the `typereg` `Inner.Error()` behind a nil check; suppress the dangling `"at 0:0:"` when position is zero/unknown.
- [ ] **Step 4:** Run → PASS. `make ci`. Commit `fix(aql): nil-safe SyntaxError/DecodeError inspection (REQ-025)`.

### Group 3: Discriminating PROBE-099 row — REQ-160

**Files:** `testkit/probes/aql/probes_test.go` (the PROBE-099 fire corpus; the non-discriminating supplied-relation row is annotated there today).

**State:** the existing supplied-relation fire row uses a **deliberately inert** overlay (`EHR→VERSIONED_PARTY`), so ignoring the threaded relation would still pass — the row does not detect relation-threading regressions at probe level (the unit test `TestRedundantStepReadsTheSuppliedRelation` does, which is why this was only ever a nice-to-have). This adds the probe twin that *is* mutation-detectable.

- [ ] **Step 1:** Add a fire row supplying a **discriminating** overlay — one that silences a finding that otherwise stands (mirror the unit test's `EHR_STATUS→OBSERVATION` overlay) — so a stubbed-out relation threading fails the probe.
- [ ] **Step 2:** Run the probe suite → the new row passes with threading, and (verify locally by temporarily ignoring `tc.Relation`) fails without it.
- [ ] **Step 3:** `make ci`. Commit `test(probes): discriminating supplied-relation PROBE-099 row (REQ-160)`.

## Self-review

- **Coverage:** version-trim → Group 1; nil-safety → Group 2; PROBE-099 twin → Group 3. All three recorded leftovers mapped.
- **Independence:** the groups share no files; ship any subset.
- **Note:** Group 2 is the same REQ-025 nil-safety axis as `transport`'s `WireError`/`DecodeError` guards — the [decode-error-surface plan](archive/2026-09-02-decode-error-surface-typing.md) has landed, so match the guard idiom it left behind.

## Mapping to specs

- [wire.md § REQ-057](../specifications/wire.md#req-057) — stored-query routes (Group 1).
- [idiom.md § Errors (REQ-025)](../specifications/idiom.md#errors-req-025) — nil-safe inspection (Group 2).
- [conformance.md § PROBE-099](../specifications/conformance.md) under REQ-160 — probe corpus (Group 3).
