# Plan — SDK-GAP-16: stored-query / query-client REST conformance

**Date:** 2026-06-29
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-055](../specifications/wire.md#req-055--aql-query) (AQL query — execution client) and [REQ-057](../specifications/wire.md#req-057--definition) (Definition — stored-query store/get). No new REQ; both findings are spec-conformance corrections within the existing contracts.
**Probes:** extend the existing query / definition PROBE coverage with two new arms — proposed **PROBE-078** (POST execution with `openehr-ehr-id` header scoping) and **PROBE-079** (no-version `PutStoredQuery` recovers the assigned version from `Location`).
**Implementation:** planned
**Depends on:** nothing new — both fixes are local to [`openehr/client/query/execute.go`](../../openehr/client/query/execute.go) and [`openehr/client/definition/stored_query.go`](../../openehr/client/definition/stored_query.go) and the vendored OAS under [`resources/ehrbase/`](../../resources/ehrbase/).
**Defers:** the `DeleteStoredQuery` operation (intentional EHRbase-aligned extension, explicitly *not* in scope per the dossier); server-side query execution semantics.
**Inbound source:** [SDK-GAP-16 dossier](../../docs/sdk-gap-drafts/SDK-GAP-16.md) (cross-check of the SDK client against `specifications-ITS-REST@master computable/OAS/query-validation.openapi.yaml` + `definition-validation.openapi.yaml`, by a consuming CDR project — both findings are wire-level deviations against a strict-spec third-party server).

## Goal

Bring the SDK's query-execution and definition stored-query clients into conformance with the canonical openEHR ITS-REST OAS on two narrow points:

- **A.** Allow EHR scoping on **POST execution** via the `openehr-ehr-id` request header (the spec's mechanism on POST), not only via the `ehr_id` query parameter (the GET mechanism, which the canonical POST operations do not declare).
- **B.** Recover the server-assigned version from the **`Location` response header** on a no-version `PutStoredQuery`, instead of relying solely on a non-spec response body.

Both are additive on the SDK callsite (existing callers keep working unchanged) and remove a class of silent failure against spec-conformant servers.

## Problem

Today the client diverges from the canonical OAS in two places — both consumer-confirmed.

### Finding A — `applyEHRScope` always sets the query parameter, never the header

[`openehr/client/query/execute.go`](../../openehr/client/query/execute.go) `applyEHRScope` calls `req.Query.Set("ehr_id", …)` on **both** GET and POST paths and never sends the `openehr-ehr-id` header. The OAS declares the `ehr_id` query parameter only on the GET operations; the POST operations (`query_execute_adhoc_query_body`, `query_execute_stored_query_body`, `…_version_body`) declare no `ehr_id` query parameter, and their request bodies (`AdhocQueryExecute`, `Query`) carry no `ehr_id` field — so for POST the **header is the spec's mechanism**. A server that scopes POST execution only via the header receives an unscoped query from the SDK and silently runs population-wide instead of single-EHR.

### Finding B — `putStoredQuery` ignores `Location`, relies on a non-spec body

[`openehr/client/definition/stored_query.go:110`](../../openehr/client/definition/stored_query.go) decodes the response body into `StoredQueryMetadata`, and on an empty body returns `&StoredQueryMetadata{Name: name, Version: version, …}` — where `version` is the caller's input (`""` on the no-version path). The OAS `200_StoredQuery_stored` response defines a `Location` header and no body; the server-assigned version lives in `Location: …/definition/query/{name}/{version}`. Against a body-less spec-conformant server (and against EHRbase when the request was `Content-Type: text/plain`, which the SDK always sends), the no-version `PutStoredQuery` therefore yields `Version: ""` — the caller cannot learn the assigned version.

## Definition of Ready (analysis gate)

- [x] Maintainer sign-off on the **Finding A surface shape** — **A2** (verb-aware default: header on POST, query param on GET) chosen 2026-06-29.
- [ ] PROBE-078 + PROBE-079 cassette pair specified — the two minimal HTTP exchanges captured from the vendored OAS contract.

## Accepted approach (2026-06-29)

### Finding A — `openehr-ehr-id` header on POST (A2)

`applyEHRScope` becomes verb-aware: GET → `ehr_id` query parameter (unchanged); POST → `openehr-ehr-id` request header. Document the precedence in the package doc: an explicit option wins if/when one is later added; then header on POST; then query parameter as fallback only on GET. Removes the silent-failure mode by default for every existing POST caller. Existing in-tree callers do not need to change.

An explicit `query.WithEHRIDHeader(id)` execute-option is **deferred** to a follow-up plan — additive when needed, no need to land it here.

Either option carries the header through [`transport`](../../transport) via the existing custom-header mechanism (REQ-059) — no new transport plumbing.

### Finding B — parse `Location` in `putStoredQuery`

Symmetric, additive change in `putStoredQuery`:

1. If `resp.Metadata.Header.Get("Location")` is set, parse the trailing two path segments to recover `{name, version}` and return `&StoredQueryMetadata{Name: name, Version: parsedVersion, Q: aqlText}`.
2. Else if `len(resp.Body) > 0`, decode as today.
3. Else fall back to the synthesised metadata with the caller's input version (the existing behaviour — keeps a sane result against a deficient server).

The Location parse is forgiving: the host/scheme are ignored, only the last two non-empty path segments are read. If parsing fails (malformed Location), drop through to step 2/3 silently and surface no error — the caller still gets a response, just without the assigned version. (No new error type for this corner; logging at most.)

`GetStoredQuery` and `ListStoredQueries` are not touched.

## Phases

### Phase 1 — analysis & sign-off (this plan)

**Tasks:**
- Record maintainer sign-off on A1 vs A2.
- Lock the PROBE-078 + PROBE-079 cassette pair — capture the OAS-defined headers/bodies; commit the cassettes under [`testkit/cassettes/query/`](../../testkit/cassettes/query/) and [`testkit/cassettes/definition/`](../../testkit/cassettes/definition/).

**Definition of done:** sign-off recorded; cassettes in place; this plan flipped Draft → Ready.

### Phase 2 — implementation

**Tasks:**
- Finding A: implement the chosen surface in `applyEHRScope`. Update `openehr/client/query/doc.go` with the precedence note.
- Finding B: implement the Location-header path in `putStoredQuery`. Keep `GetStoredQuery` unchanged. Add a small helper `parseLocationVersion(loc string) (name, version string, ok bool)` used by `putStoredQuery` only.
- Unit pins for both — `client/query/execute_test.go` and `client/definition/stored_query_test.go` — gating on the spec-canonical wire (POST header set; Location parsed) and the regression cases (POST without explicit scope still works; body-bearing servers still decode; malformed Location falls through cleanly).

**Definition of done:** `make ci` green; unit pins carry `// REQ-055` / `// REQ-057` citations.

### Phase 3 — probes + traceability close-out

**Tasks:**
- Land **PROBE-078** at `testkit/probes/query/` — POST execution with header scoping.
- Land **PROBE-079** at `testkit/probes/definition/` — no-version store recovers version from Location.
- Update [`traceability.yaml`](../specifications/traceability.yaml): map PROBE-078 → REQ-055, PROBE-079 → REQ-057.
- Refresh [`docs/specifications/wire.md`](../specifications/wire.md) REQ-055 / REQ-057 with one-line notes on the header / Location dual mechanism (no behavioural prose duplication).

**Definition of done:** probes pass against the vendored cassettes; `make spec-check` green; CHANGELOG `[Unreleased]` bullet drafted.

## Acceptance criteria

- A POST execution with EHR scope sends `openehr-ehr-id: <id>` on the wire (assert via captured request headers); the GET path keeps the query parameter (unchanged).
- `PutStoredQuery(ctx, c, name, aql)` against a body-less server that returns `Location: …/definition/query/foo/1.2.3` returns `StoredQueryMetadata{Name: "foo", Version: "1.2.3", …}`. The same call against a body-bearing server decodes the body unchanged.
- Malformed Location → fall through to body decode → fall through to synthesised metadata. No error returned in any of the three branches.
- `make ci` and `make spec-check` green; `traceability.yaml` lists PROBE-078 and PROBE-079.

## Out of scope

- `DeleteStoredQuery` on `/definition/query/{name}/{version}` (intentional EHRbase-aligned extension; keep as is per the dossier).
- Any change to the GET execution scoping (`ehr_id` query parameter — spec-correct on GET).
- Server-side query-execution semantics.

## Risks / open questions

- **`WithEHRIDHeader` future option.** If A2 is chosen now, an explicit `WithEHRIDHeader` option may still be useful later (e.g. a caller wants the header on GET too, against a server that honours both). Adding it later is additive; no need to land it in this plan.
- **EHRbase Content-Type branch.** EHRbase's body-only-on-`application/json` behaviour is documented; the SDK always sends `text/plain` on store, so the Location path is the always-applicable branch. No change to Content-Type negotiation in this plan.
- **Cassette source.** The two cassettes should be derived from the OAS examples rather than hand-rolled, to keep the wire shape spec-honest. Pull from `resources/ehrbase/` if a matching example exists; otherwise hand-roll from the OAS schema.

## Mapping to specs

- [docs/specifications/wire.md § REQ-055 / REQ-057](../specifications/wire.md) — REQ rows extended with the header / Location dual-mechanism note.
- [docs/specifications/REQ.md](../specifications/REQ.md) — registry rows unchanged (no new REQ).
- [docs/specifications/traceability.yaml](../specifications/traceability.yaml) — PROBE-078 / PROBE-079 → REQ-055 / REQ-057.
