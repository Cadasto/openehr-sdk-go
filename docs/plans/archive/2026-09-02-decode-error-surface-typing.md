# Decode / error-surface typing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Date:** 2026-09-02
**Status:** Done (2026-09-02) — both tasks executed; [REQ-093](../../specifications/transport.md#req-093--openehr-error-envelope-mapping) stays `landed` (it gained the `(unrouted)` placeholder clause), and [REQ-052](../../specifications/wire.md#req-052) stays **`partial`**: the decode-side shape sentinel now has a producer, but the § Floating-point precision mantissa arm — the one this plan never claimed — is still open.
**Owner:** SDK maintainers
**Covers:** [REQ-052](../../specifications/wire.md#req-052) (the decode-side `canjson.ErrInvalidShape` producer) and [REQ-093](../../specifications/transport.md#req-093--openehr-error-envelope-mapping) (value-free diagnostics — the `effectiveRoute()` path-identifier leak). No new id.
**Probes:** none new (unit-covered).
**Implementation:** landed
**Depends on:** landed `*transport.DecodeError` (REQ-151), `transport.Request.effectiveRoute()`.
**Defers:** the `StoredQueryMetadata` / `TemplateMetadata` Extras re-encode item once considered for this plan — **verified already fixed** (`stored_query.go:78-90` has `MarshalJSON` with delete-then-overlay collision handling, PR #140); nothing to do.

**Goal:** Close two error-surface loose ends from the REQ-151/#140 review rounds: give the decode-only sentinel `canjson.ErrInvalidShape` a producer (or retire it), and stop `transport`'s error strings from printing a real request path — with identifiers like a live `ehr_id` — when a route template is unset.

**Architecture:** Both are about what a returned error *says* and *classifies*. Task 1 is a canjson decision (wire the reserved sentinel, or delete it — YAGNI). Task 2 is a REQ-093 sweep of every `transport` error string built from `effectiveRoute()`, which falls back to `Request.Path`.

**Tech Stack:** Go errors, `openehr/serialize/canjson`, `transport`.

## Global Constraints

- **Value-free diagnostics** (REQ-093): a transport/codec error string carries method + route template + classification, never a populated path, id, or payload value.
- **Sentinel distinctness** (REQ-052): `canjson.ErrInvalidShape`, `canjson.ErrInvalidValue`, and `transport.ErrInvalidShape` stay three distinct values; `errors.Is` against one MUST NOT match the others.

## Definition of Done

- Task 1: either `canjson.ErrInvalidShape` has a producer (some decode path wraps it) with a test asserting `errors.Is`, **or** the sentinel is removed and its doc references cleaned — decided below, not left dangling.
- Task 2: no `transport` error string prints a populated `Request.Path` when `Route` is empty; a test with an unset route template asserts the message is value-free.
- `make spec-check` / `make ci` pass. Plan archived.

## Phases

### Task 1: A producer for `canjson.ErrInvalidShape` (or retire it) — REQ-052

**Files:** `openehr/serialize/canjson/decode.go`, `doc.go`; test `openehr/serialize/canjson/decode_test.go`. Spec: `docs/specifications/wire.md` § REQ-052.

**State:** `ErrInvalidShape` is defined (`decode.go:26`) and documented as "reserved for JSON-level shape errors," but `decode.go:76` / `doc.go:88` state **nothing wraps it** ("producer deferred"). So `errors.Is(err, canjson.ErrInvalidShape)` can never be true today.

**Decision to make first (this is the task's real content):**

- **(A) Wire it (recommended if any caller wants the classification).** Make the in-type JSON-shape decode failures — the `canjson: <RM_TYPE>:` family, where the bytes are valid JSON but the wrong shape for the target type — wrap `ErrInvalidShape`, mirroring how the encode side already wraps `ErrInvalidValue`. `*json.SyntaxError` (malformed JSON) and `typereg.DecodeError` (polymorphic) stay as-is. This gives the sentinel its documented meaning and lets the precision work in the fidelity plan's Task 5 reuse it.
- **(B) Retire it (YAGNI, if no consumer needs it).** Delete the sentinel and its doc references; a caller classifies decode failures via `*json.SyntaxError` / `typereg.DecodeError` / the `canjson:` prefix instead.

**Decision: (A) wire it.** The SDK has no consumer today (Step 1), but the queued fidelity plan's `Real`-precision task needs a decode-side sentinel and names this one — so the near-term need is real and retiring it would only mean re-adding it.

- [x] **Step 1:** Grep the SDK and — where visible — the consuming CDR project for `canjson.ErrInvalidShape` consumers. No consumer + no near-term need → lean toward (B). A consumer classifying shape errors → (A). *(SDK grep: only canjson's own tests and doc prose — no behavioural consumer; the CDR project is not on disk here. Decided (A) on the queued fidelity-plan need.)*
- [x] **Step 2 (if A): failing test** — a valid-JSON-wrong-shape body (e.g. a `DV_QUANTITY` with `magnitude` as an object) decodes to an error for which `errors.Is(err, canjson.ErrInvalidShape)` is true, and `errors.Is(err, canjson.ErrInvalidValue)` / `transport.ErrInvalidShape` are both false.
- [x] **Step 3 (if A):** wrap the `canjson: <TYPE>:` shape errors with `ErrInvalidShape` at the one funnel that builds them; keep `SyntaxError`/`DecodeError` untouched. Update `doc.go` (drop "none wraps"). **(if B):** delete the sentinel + doc lines; add a test asserting the three-way classification still holds via the remaining types. *(The funnel is generated, not hand-written: the template line in `internal/bmmgen/render_jsonunmar.go` now calls the new `typereg.WrapShapeError`, which is also where the sentinel value had to live — `openehr/rm` cannot import `openehr/serialize`. `canjson.ErrInvalidShape` keeps its name as that value.)*
- [x] **Step 4:** `make ci`. Commit `fix(canjson): give ErrInvalidShape a producer (REQ-052)` (A) or `refactor(canjson): retire unused ErrInvalidShape sentinel (REQ-052)` (B).

### Task 2: Stop `effectiveRoute()` leaking a path into error strings — REQ-093

**Files:** `transport/client.go` (`effectiveRoute` + its error-string call sites: `:308`, `:322`, `:325`, and the `DecodeError{Route: req.effectiveRoute()}` at `:618`); `transport/errors.go` (`DecodeError.Error`, and the `WireError` neighbour on the same axis); test `transport/errors_test.go`.

**State:** `effectiveRoute()` "falls back to Path" (`client.go:159`). `DecodeError.Error()` prints `e.Route` (`errors.go:48`), and `DecodeError.Route` is set from `effectiveRoute()` (`client.go:618`). So a request with **no route template** (only a `Path` like `/ehr/7f3e…-live-id`) yields an error string containing that id — a REQ-093 value-free violation. The `fmt.Errorf("transport: %s %s: …", …, req.effectiveRoute(), …)` calls at `:308/:322/:325` share the leak.

- [x] **Step 1: Failing test.** Build a `*transport.Request` with `Path` set to an identifier-bearing path and `Route` empty; drive each error path (decode failure, read-body failure, over-limit); assert the returned `Error()` string does **not** contain the path/id — only method + a route placeholder + classification.
- [x] **Step 2:** Run → FAIL (the id appears today).
- [x] **Step 3: Fix.** Make the error-string surface value-free when `Route` is unset — either `effectiveRoute()` returns a stable placeholder (e.g. the method's generic route, or `"(unrouted)"`) **for diagnostic strings** while telemetry keeps its own resolution, or the call sites stop using `effectiveRoute()` for the human string and use `Route` alone (empty → placeholder). Keep the OTel span attribute (`client.go:180`) on the real resolution — spans are sanitised URLs, not REQ-093 error strings. Sweep the `WireError` neighbour on the same axis in the same change.
- [x] **Step 4:** Run → PASS; existing routed-request tests still show the template. `make ci`.
- [x] **Step 5: Commit** `fix(transport): keep error strings value-free when route template is unset (REQ-093)`.

## Self-review

- **Coverage:** producerless `ErrInvalidShape` → Task 1; `effectiveRoute()` leak → Task 2. The Extras item is verified done and deferred out with evidence.
- **Cross-plan link:** Task 1's outcome (A vs B) decides whether the fidelity plan's Task 5 reuses `canjson.ErrInvalidShape` or mints its own precision sentinel — sequence Task 1 first if both are executed.
- **Biggest risk:** Task 2 must not blind telemetry — the fix is scoped to the human `Error()` string, not the span/`http.route` attribute.

## Mapping to specs

- [wire.md § REQ-052](../../specifications/wire.md#req-052) — the decode-side sentinel (Task 1); the § gained the "Decode-side shape sentinel" clause with this plan.
- [transport.md § REQ-093](../../specifications/transport.md#req-093--openehr-error-envelope-mapping) — value-free diagnostics (Task 2); the § gained the "Unrouted requests render a placeholder" clause with this plan.
