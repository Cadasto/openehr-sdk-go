# Plan — path-parameter encoding fix + class-predicate ParsedPath

**Date:** 2026-07-13
**Status:** In progress
**Owner:** SDK maintainers
**Covers:**
- **REQ-095** (OpenAPI authoritative source — [wire.md](../specifications/wire.md#req-095)): a defect fix. The SDK's request path MUST conform to the OAS path template, which requires each path parameter to be percent-encoded **exactly once**. Today several leaf clients pre-escape with `url.PathEscape` into `transport.Request.Path`, which the transport re-encodes — a double-encode that 404s any id with an encodable character (e.g. a space in a template id).
- **REQ-113** (execution-oriented parsed AQL AST — [clinical-modeling.md](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast)): an implementation-aligned completeness change. Populate `aql.Comparison.ParsedPath` for a **class standing predicate** (`EHR e[ehr_id/value = $x]`) with an empty `Alias` and structured `Segments`, so a consumer need not re-split the relative path text — the last raw-text re-tokenization the WHERE side already eliminated.

**Probes:** PROBE-082 (already pins structured standing-predicate access in `openehr/aql/parse/structured_test.go`) is extended to assert the class-predicate `ParsedPath`. No new PROBE for the encoding fix — it is a wire-URL correctness defect covered by unit + round-trip tests in the leaf clients and transport.

**Depends on:** REQ-113 landed (PR #58); REQ-055/057 stored-query + definition clients landed.

**Defers / out of scope:**
- ~~The `ehr` / `composition` / `admin` / `demographic` / `directory` clients share the same `url.PathEscape`-into-`Request.Path` pattern~~ — **done in the follow-up** (commit 3): the pre-escape pattern is now retired across *every* `openehr/client` leaf client (UUID / `::`-delimited `OBJECT_VERSION_ID` ids carry no `/`, so the decoded path round-trips). Enforced tree-wide by `TestNoPathEscapeInClientPathParams`.
- Ids that legitimately contain a literal `/` (would need `RawPath`) — openEHR ids (template ids, archetype ids, qualified query names) do not contain `/`, so the minimal decoded-path contract round-trips correctly.
- Alias resolution / binding the relative class-predicate path to a concrete RM type — the consumer's job.

## Source (inbound)

Two gaps filed by the consuming CDR project against SDK v0.14.0:
- Double-encoding: a recurring `GET /definition/template/{}` → 404 on OPT ids with spaces (`Referral Request.v1`, `Weird Types 1`); the consumer works around it by building the raw decoded path itself instead of calling `definition.GetTemplate`.
- Class-predicate ParsedPath: the residual raw-text split left after the structured standing-predicate work landed; the CDR still splits `ehr_id/value` from `Comparison.Path` because `ParsedPath` is nil for a class predicate.

## Root cause (encoding)

`transport.joinTarget` assigns `Request.Path` to `url.URL.Path` — the **decoded** field — and relies on `url.URL.String()` as the single canonical encoder. A client that `url.PathEscape`es first turns a space into `%20`; the transport then encodes the `%` to `%25`, yielding `%2520` on the wire. The two layers disagree on whether `Request.Path` is decoded or pre-encoded. Resolution: the transport is the single canonical encoder; `Request.Path` is a **decoded** path; leaf clients interpolate the raw id.

## Definition of Ready

- [x] Both gaps map to existing REQs (REQ-095, REQ-113) — no new identifier, no new GAP doc.
- [x] Root cause reproduced from the code paths (double-encode; nil ParsedPath).
- [x] Fix option chosen: minimal (no pre-escape) over `RawPath` — openEHR ids carry no `/`.

## Tasks (TDD — red first)

1. **REQ-113 / GAP-20**
   - Test (RED): `EHR e[ehr_id/value = $x]` → `PredicateComparison.ParsedPath != nil`, `Alias == ""`, `Segments == [{ehr_id},{value}]`, `ParsedPath.Raw == Path`. Archetype/version predicate still nil.
   - Impl: in `standingComparison`, decompose the relative `objectPath` into `aql.PathSegment`s (mirror `extractIdentifiedPath`); set `ParsedPath`.
   - Docs: update `ast.go` `PredicateComparison` doc + REQ-113 canonical prose (drop "ParsedPath not populated for class predicate").

2. **REQ-095 / GAP-21**
   - Test (RED): `GetTemplate(ctx, c, "Referral Request.v1", …)` → server sees decoded path `.../adl1.4/Referral Request.v1` and wire `EscapedPath()` `.../Referral%20Request.v1` (not `%2520`). Same for `DeleteTemplate`, `ExampleComposition`, stored-query PUT/GET, and `query.RunStored`.
   - Guard test: no `url.PathEscape(` in non-test sources of `openehr/client/definition` and `openehr/client/query`.
   - Impl: drop `url.PathEscape` from the `Request.Path` construction in `definition/template.go`, `definition/stored_query.go`, `query/execute.go` (keep `url.PathUnescape` for decoding the `Location` header). Document `transport.Request.Path` as a decoded path. **No `joinTarget` change needed** — the transport already assigns `Request.Path` to the decoded `url.URL.Path` and lets `String()` encode once (base URLs carry no `RawPath`), so it is already the single canonical encoder; a regression test (`transport/path_encoding_test.go`) pins that contract.
   - Docs: add a normative "Path-parameter encoding" statement under REQ-095.

3. **Traceability + gates:** add `openehr/client/definition`, `openehr/client/query`, `transport` packages + new tests to REQ-095; keep PROBE-082 note for REQ-113. `make fmt` → `make spec-check` → `make ci`.

## Definition of Done

- [ ] All new tests pass; `make ci` green (includes `spec-check`, codegen/aqlgen drift, race).
- [ ] `traceability.yaml` updated; `make spec-check` clean.
- [ ] No `url.PathEscape` into `Request.Path` in the definition/query clients; guard test enforces it.
- [ ] Round-trip: upload a spaced-id template → `GetTemplate` by that id → 200 with body (was 404).
