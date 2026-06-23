# Plan — openEHR ITS-REST conformance remediation

**Date:** 2026-06-19
**Status:** Landed (PR #52, merged to `main` 2026-06-22) — archived 2026-06-23. Tiers 1–3 landed; larger Tier-3 subsystems deferred (see Defers). Dedicated `testkit/probes/rest/*` probes deferred to [STRAND-09](../../specifications/research-strands.md).
**Owner:** SDK maintainers
**Covers:** [REQ-095](../../specifications/wire.md#req-095) (umbrella — OpenAPI authoritative source); [REQ-059](../../specifications/wire.md#req-059) (custom headers); [REQ-093](../../specifications/transport.md#req-093--openehr-error-envelope-mapping) (status mapping); [REQ-099](../../specifications/module-layout.md#req-099--its-rest-admin-client-surface) (admin surface); touches [REQ-054](../../specifications/wire.md#req-054), [REQ-057](../../specifications/wire.md#req-057), [REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation)
**Probes:** new wire-conformance probes under `testkit/probes/rest/` (audit-details header grammar, System `OPTIONS`, Admin path, Definition `/example`); anchored to [REQ-080](../../specifications/conformance.md)
**Implementation:** landed (PR #52)
**Source:** ITS-REST conformance audit (2026-06-19) against pinned `resources/its-rest/*.openapi.yaml` (MANIFEST commit per [resources/its-rest/MANIFEST.txt](../../../resources/its-rest/MANIFEST.txt))
**Defers (to named follow-up plans):** dedicated ITEM_TAG endpoint family (`/tags`, `DELETE …/tags/{key}`); VERSIONED_COMPOSITION / VERSIONED_EHR_STATUS / contribution-GET read families; FLAT/STRUCTURED + `wt+json` content negotiation (REQ-053, already planned); ADL2 definition family (already planned). These are capability gaps, not defects in shipped code — each merits its own plan.

## Goal

Bring the already-"Landed" `openehr/client/*` packages into conformance with the authoritative openEHR ITS-REST OpenAPI contract (REQ-095), fixing wire-breaking deviations — chiefly the `openehr-audit-details` header serialised as JSON instead of the spec's dotted-attribute grammar, the System client using `GET` instead of `OPTIONS`, the Admin bulk-delete path, and the Definition example endpoint — and closing the smaller payload/status deviations the audit surfaced.

## Background — why this is conformance debt, not new features

The audit compared every operation in the six vendored specs against the SDK. Three findings are **spec defects** (in-repo normative prose contradicts or under-specifies the authoritative YAML — REQ-095 says the YAML wins and the prose is corrected); the rest are **code deviations** from a contract that is already correct. None of this is new capability — the affected packages are marked **Landed** in [roadmap.md](../../roadmap.md), several under REQs that currently claim conformance.

Severity tiers (priority order):

- **Tier 1 — wire-breaking** (fails against a conformant server today): audit-details header grammar; System `OPTIONS` verb; Admin `/admin/ehr/all` path; Definition `/example` path + params.
- **Tier 2 — real deviations in built features**: `/admin/template` (out of contract); composition `204` treated as error; stored-query required `offset`/`fetch`; `query_type` casing; `created_timestamp` field name; versioned stored-query `PUT` missing.
- **Tier 3 — coverage / capability gaps**: `openehr-version` lifecycle_state option; status mapping (400/422; 428 reconciliation); `directory` `path` param; Query `GET` variants. (Larger Tier-3 items are deferred to follow-up plans — see header.)

## Global constraints (apply to every task)

- **The vendored YAML is authoritative (REQ-095).** Cite the exact `resources/its-rest/*.yaml` line for every wire-shape decision; never guess.
- **No new runtime dependencies.** Header grammar codec is hand-written, stdlib-only (consistent with the item-tag codec already in `openehr/client/ehr/itemtag_wire.go`).
- **Idioms (REQ-020–025):** `context.Context` first; functional options; package-level functions; wrap errors with `%w`; no panics in library code.
- **PHI-safety (REQ-093):** error surfaces must not interpolate response bodies by default.
- **Verification:** every task ends green on `make test`; the final task runs `make ci` (includes `make spec-check`). Wire-shape tasks add/extend a cassette or probe.
- **CHANGELOG:** add one `### Added`/`### Fixed` bullet per artefact class at the end (pre-1.0 uses `### Added`; a conformance-fix bullet is acceptable under a `### Fixed` subhead if introduced).

---

## Phase 0 — Normative corrections (specs first)

These edits correct the contract so the Phase 1–3 code has something true to satisfy. They are small, RFC-2119, and cite the authoritative YAML. Do this phase first; it is reviewable on its own.

### Task 0.1 — Correct REQ-059 audit-details / openehr-version header grammar

**Files:** Modify [docs/specifications/wire.md](../../specifications/wire.md) (REQ-059 section).

**Problem:** REQ-059's closing sentence — *"`*rm.AuditDetails` is … serialised via canonical JSON / canonical XML at the codec boundary"* — is correct for the **contribution request body** but is the root cause of the header bug: the `openehr-audit-details` **request header** must use the flat dotted-attribute grammar in [overview-validation.openapi.yaml:246-263](../../../resources/its-rest/overview-validation.openapi.yaml#L246-L263):

```
openehr-audit-details: change_type.code_string="251"
openehr-audit-details: committer.name="John Doe",committer.external_ref.id="…",committer.external_ref.namespace="demographic",committer.external_ref.type="PERSON"
openehr-audit-details: system_id="example.openehr.systemid"
openehr-version: lifecycle_state.code_string="532"
```

**Change:** Add a normative clause to REQ-059:
- The `openehr-audit-details` request header **MUST** be encoded as the dotted-attribute grammar (comma-separated `attribute.path="value"` pairs, repeatable header lines for `change_type`, `description`, `committer`, `system_id`), **NOT** as a JSON object. Canonical JSON/XML serialisation of `AUDIT_DETAILS` applies only to the contribution request **body** (the `commit_audit`/`UpdateAudit` field), never to this header.
- The `openehr-version` request header carries `lifecycle_state.code_string="<code>"` in the same grammar.
- Clarify the existing "serialised via canonical JSON" sentence to scope it to body codecs.

**Acceptance:** REQ-059 unambiguously distinguishes header grammar from body serialisation; cites the overview YAML lines.

### Task 0.2 — Reconcile REQ-093 status→sentinel table with the contract

**Files:** Modify [docs/specifications/transport.md](../../specifications/transport.md) (REQ-093 section).

**Problem:** REQ-093 mandates an `ErrPreconditionRequired (428)` sentinel, but **428 appears in none of the vendored specs** (`grep` is empty); the overview says a stale `If-Match` is **412** and a missing-but-expected `If-Match` SHOULD yield **400** ([overview:370](../../../resources/its-rest/overview-validation.openapi.yaml#L370)). Meanwhile **422 Unprocessable Entity** — a documented validation-failure status ([overview:400](../../../resources/its-rest/overview-validation.openapi.yaml#L400)) used on EHR/demographic write ops ([ehr:396-397](../../../resources/its-rest/ehr-validation.openapi.yaml#L396), [demographic:580-587](../../../resources/its-rest/demographic-validation.openapi.yaml#L580)) — and **409 already-exists** (distinct from stale-If-Match, [definition:4245](../../../resources/its-rest/definition-validation.openapi.yaml#L4245)) are unaddressed.

**Change:** In the REQ-093 status-mapping bullet:
- Add `ErrUnprocessable` (422) → validation/semantic failure.
- Note that openEHR signals missing-`If-Match` as **400**, and that 428 is retained only as a defensive mapping for non-conformant servers (not an openEHR-canonical status).
- Note that 409 spans both optimistic-concurrency conflict and resource-already-exists; the openEHR error `code` (already surfaced via `OpenEHRErrorDetail`) disambiguates — `ErrVersionConflict` remains the sentinel.

**Acceptance:** the table matches the status codes actually used across the six specs; a new `ErrUnprocessable` sentinel is specified.

### Task 0.3 — Pin admin paths in REQ-099

**Files:** Modify [docs/specifications/module-layout.md](../../specifications/module-layout.md) (REQ-099 section).

**Problem:** REQ-099 names `DeleteAllEHRs`/`PurgeTemplates` but pins no paths. Code hits `/admin/ehr` (spec: `/admin/ehr/all`, [admin:78](../../../resources/its-rest/admin-validation.openapi.yaml#L78)) and `/admin/template` (**absent from the admin contract** — admin defines only `/admin/ehr/{ehr_id}` and `/admin/ehr/all`).

**Change:**
- State that `DeleteAllEHRs` targets `DELETE /admin/ehr/all` with the optional repeatable `ehr_id` subset query param.
- State that template purge is **not** part of the ITS-REST Admin contract; `PurgeTemplates` is a deployment extension (commonly EHRbase `/admin/template`) and **MUST** be documented as such on its godoc (not presented as ITS-REST-conformant).
- Note Admin is upstream `x-status: DEVELOPMENT` → the client ships **Draft**.

**Acceptance:** REQ-099 pins the two real admin paths and labels `PurgeTemplates` as a non-contract extension.

### Task 0.4 — Update REQ registry + traceability

**Files:** Modify [docs/specifications/REQ.md](../../specifications/REQ.md), [docs/specifications/traceability.yaml](../../specifications/traceability.yaml).

- Leave REQ-059 / REQ-095 at `partial` (this plan advances them); add a note that conformance remediation is in progress citing this plan.
- Register the new `ErrUnprocessable` sentinel and new probes in `traceability.yaml` once Phase 1–3 land.

**Verify Phase 0:** `make spec-check` green.

---

## Phase 1 — Tier 1 wire-breaking fixes

### Task 1.1 — Encode `openehr-audit-details` as the dotted-attribute grammar (flagship)

**Files:**
- Modify: [openehr/client/ehr/audit.go](../../../openehr/client/ehr/audit.go) (`MarshalAuditDetails`, lines 13-22)
- Create: `openehr/client/ehr/audit_header.go` (the grammar encoder) + `openehr/client/ehr/audit_header_test.go`
- Callers unchanged in shape: [composition/composition.go](../../../openehr/client/ehr/composition/composition.go), [directory/directory.go:144,183,218](../../../openehr/client/ehr/directory/directory.go#L144), [demographic/party.go:138,184](../../../openehr/client/demographic/party.go#L138) (they call `MarshalAuditDetails`)

**Current (wrong):** `MarshalAuditDetails` does `canjson.Marshal(a)` and the JSON object is set verbatim as the header at [transport/client.go:314](../../../transport/client.go#L314). A spec-conformant server parses the dotted grammar and silently drops all audit metadata.

**Change:** Replace the JSON marshal with an encoder that emits the dotted-attribute grammar from the relevant `AUDIT_DETAILS` fields:
- `change_type` → `change_type.code_string="<code>"` (and `change_type.value="…"` where present)
- `description` → `description.value="…"`
- `committer` → `committer.name="…"`, and if a `PARTY_REF` external_ref is present: `committer.external_ref.id="…",committer.external_ref.namespace="…",committer.external_ref.type="…"`
- `system_id` → `system_id="…"`
Reuse the quoting/escaping + control-char rejection already implemented in [openehr/client/ehr/itemtag_wire.go](../../../openehr/client/ehr/itemtag_wire.go) (extract a shared helper if clean). The server accepts repeated header lines; emitting one comma-joined value is also valid per the grammar — emit a single header value (transport sets one header), grouping `committer.*` together.

**Tests:** golden test asserting the exact header string for a representative `AUDIT_DETAILS` (committer with external_ref + change_type DV_CODED_TEXT + system_id); control-char rejection; nil → `""`.

**Probe:** add `testkit/probes/rest/probe_audit_details_header.go` asserting the header wire format on a composition write cassette. Comment `// REQ-059`.

**Verify:** `go test ./openehr/client/ehr/...`; the existing composition/directory/demographic write tests still pass (they assert call shape, not header JSON — confirm none assert the JSON header form; update any that do).

### Task 1.2 — System client: use `OPTIONS /`, surface `Allow`

**Files:** Modify [openehr/client/system/system.go](../../../openehr/client/system/system.go) (`Capabilities` :144-148, `Version` :170, `Health` :190).

**Current (wrong):** all three issue `http.MethodGet` against `/`. Spec defines exactly one System op: `OPTIONS /` (operationId `options`, [system:52-53](../../../resources/its-rest/system-validation.openapi.yaml#L52)), returning the `Options` body + an `Allow` response header ([system:123](../../../resources/its-rest/system-validation.openapi.yaml#L123)).

**Change:**
- `Capabilities` issues `http.MethodOptions`. (Confirm `transport.Request`/`Client.Do` permit OPTIONS with a decoded body; add support if the verb is gated.)
- `Health` keeps its anonymous liveness probe but switches to `OPTIONS /` too (it reuses the same endpoint).
- Optionally surface the `Allow` header on `ServiceCapabilities`/metadata.

**Tests:** update `system_test.go` cassettes/asserts to expect `OPTIONS`. Add `testkit/probes/rest/probe_system_options.go` (`// REQ-095`).

**Verify:** `go test ./openehr/client/system/...`.

### Task 1.3 — Admin: correct bulk-delete path to `/admin/ehr/all`

**Files:** Modify [openehr/client/admin/admin.go](../../../openehr/client/admin/admin.go) (`DeleteAllEHRs` :37-44), `admin_test.go` (:98-99).

**Current (wrong):** `Path:"/admin/ehr"`, `Route:"/admin/ehr"`. Spec: `DELETE /admin/ehr/all{?ehr_id*}` ([admin:78-80](../../../resources/its-rest/admin-validation.openapi.yaml#L78)).

**Change:**
- Path/Route → `/admin/ehr/all`.
- Add an optional variadic/option to pass the `ehr_id` subset query param (repeatable) per the spec; default omits it (full reset).
- Update the test assertion to `DELETE …/admin/ehr/all`.

**Tests:** update `TestDeleteAllEHRs`; add a subset-`ehr_id` case. Probe `testkit/probes/rest/probe_admin_delete_all.go` (`// REQ-099`).

**Verify:** `go test ./openehr/client/admin/...`.

### Task 1.4 — Definition: correct example endpoint path + params

**Files:** Modify [openehr/client/definition/template.go](../../../openehr/client/definition/template.go) (`ExampleComposition` :339-360, `WithExampleFormat` :329, doc :337) + `doc.go:12`.

**Current (wrong):** builds `…/example_composition` and sends a non-spec `format` query param. Spec: `GET …/adl1.4/{template_id}/example` with params `example_type` (input|output, default input) and `example_detail_level` (required|medium|complete) ([definition:225-257](../../../resources/its-rest/definition-validation.openapi.yaml#L225)).

**Change:**
- Path segment `example_composition` → `example`.
- Replace `WithExampleFormat` with `WithExampleType(input|output)` and `WithExampleDetailLevel(required|medium|complete)` typed options mapping to the two spec query params. Drop the undefined `format` param. (Accept-header format negotiation — `application/xml`, `wt.flat+json` — is a separate Tier-3/content-negotiation concern; keep `application/json` default here.)

**Tests:** update `TestExampleComposition*`; assert path `…/example` and the two params. Probe `testkit/probes/rest/probe_definition_example.go` (`// REQ-095`).

**Verify:** `go test ./openehr/client/definition/...`.

---

## Phase 2 — Tier 2 deviations

### Task 2.1 — Document `PurgeTemplates` as a non-contract deployment extension

**Files:** Modify [openehr/client/admin/admin.go](../../../openehr/client/admin/admin.go) (`PurgeTemplates` :51-57 godoc) + `doc.go:8`.

**Change:** Per Task 0.3 — godoc states `/admin/template` is not part of the ITS-REST admin contract (a deployment extension, commonly EHRbase) and may 404/405 elsewhere. Keep the function (it's useful for test teardown) but stop advertising it as ITS-REST-conformant. No path change (no authoritative path exists to correct it to).

**Verify:** `go test ./openehr/client/admin/...`; godoc reads honestly.

### Task 2.2 — Composition: treat documented `204` (deleted-at-time) as success, not `ErrInvalidShape`

**Files:** Modify [openehr/client/ehr/composition/composition.go](../../../openehr/client/ehr/composition/composition.go) (`Get` :23-58).

**Current (wrong):** `Get` calls `transport.Decode`, which returns `ErrInvalidShape` on any empty 2xx body. Spec `composition_get` documents `204 No Content` as success when the composition was deleted at the requested `version_at_time` ([ehr:423](../../../resources/its-rest/ehr-validation.openapi.yaml#L423)).

**Change:** Before decoding, branch on `resp.StatusCode == 204`: return `(nil, *VersionMetadata, nil)` (or a typed `ErrDeletedAtTime` sentinel — pick the surface that matches how the SDK signals "gone"; document it). Only decode bodies for 200.

**Tests:** add a `204`-cassette case asserting clean success/typed signal, not `ErrInvalidShape`.

**Verify:** `go test ./openehr/client/ehr/composition/...`.

### Task 2.3 — Stored-query body: always emit required `offset`/`fetch`; allow explicit `0`

**Files:** Modify [openehr/client/query/execute.go](../../../openehr/client/query/execute.go) (`storedBody` :135-148, `executeConfig`/options :options.go).

**Current (wrong):** `storedBody` elides `offset`/`fetch` when zero; the `Query` schema marks both **required** ([query:552-558](../../../resources/its-rest/query-validation.openapi.yaml#L552)). `WithOffset(0)` is unrepresentable.

**Change:** Track presence (e.g. `*int` or a `set` bool in `executeConfig`) so an explicit `WithOffset(0)`/`WithFetch(0)` emits the field; for stored queries always include both (schema-required) — default to the server's documented defaults if unset, or send `0`/spec default. Ad-hoc (`adhocBody`) only requires `q`, so its current elision is fine — leave it.

**Tests:** assert the stored-query body always contains `offset` and `fetch`; assert `WithOffset(0)` round-trips.

**Verify:** `go test ./openehr/client/query/...`.

### Task 2.4 — Stored-query `query_type` casing → `AQL`

**Files:** Modify [openehr/client/definition/stored_query.go](../../../openehr/client/definition/stored_query.go) (`storeConfig` default :81, emit :86-88).

**Current (wrong):** defaults to lowercase `"aql"`. Spec `QueryType` enum/default is `AQL` ([definition:4116](../../../resources/its-rest/definition-validation.openapi.yaml#L4116), schema :3852).

**Change:** default `queryType: "AQL"`; if `WithQueryType` accepts user input, upper-case or validate against the enum.

**Tests:** assert emitted `type:"AQL"`.

**Verify:** `go test ./openehr/client/definition/...`.

### Task 2.5 — Definition `TemplateMetadata`: decode `created_timestamp`

**Files:** Modify [openehr/client/definition/template.go](../../../openehr/client/definition/template.go) (`TemplateMetadata` struct + `UnmarshalJSON`/`MarshalJSON`, ~:74-90).

**Current (wrong):** decodes `json:"created_on"`; the spec field is `created_timestamp` ([definition:503-523](../../../resources/its-rest/definition-validation.openapi.yaml#L503)). The real timestamp silently lands in `Extras`; `CreatedOn` stays zero.

**Change:** map the field to `created_timestamp`. If the public field name `CreatedOn` must change for clarity, note the (pre-1.0) breaking change in CHANGELOG; otherwise keep the Go field name and fix only the JSON tag. Drop or keep the non-spec `description` field as `Extras` (it is not in the spec schema).

**Tests:** decode a spec-shaped `TemplateMetadata` cassette; assert the timestamp populates.

**Verify:** `go test ./openehr/client/definition/...`.

### Task 2.6 — Definition: implement versioned stored-query `PUT`

**Files:** Modify [openehr/client/definition/stored_query.go](../../../openehr/client/definition/stored_query.go) (`PutStoredQuery` / `Repository`).

**Current:** only the unversioned `PUT /definition/query/{name}` exists. Spec also defines `PUT /definition/query/{name}/{version}` ([definition:453-454](../../../resources/its-rest/definition-validation.openapi.yaml#L453)) with its own `409_StoredQuery_version`.

**Change:** add `PutStoredQueryVersion(ctx, c, qualifiedName, version, aqlText, opts...)` (+ repository method) targeting the `{version}` path; map 409 to `ErrVersionConflict`.

**Tests:** cassette asserting the versioned path; 409 → sentinel.

**Verify:** `go test ./openehr/client/definition/...`.

---

## Phase 3 — Tier 3 coverage gaps (smaller items)

> Larger Tier-3 items (ITEM_TAG endpoint family, VERSIONED_* reads, content negotiation) are **deferred to named follow-up plans** per the header — they are independent subsystems, each producing its own testable surface. The items below are small and belong with this remediation.

### Task 3.1 — `openehr-version` lifecycle_state write option

**Files:** Modify composition/directory/ehrstatus/demographic write paths to expose `WithLifecycleState(code string)` → sets `transport.Request.RMVersion` as `lifecycle_state.code_string="<code>"` (grammar from Task 0.1). Overview mandates servers accept it ([overview:246,253](../../../resources/its-rest/overview-validation.openapi.yaml#L246)).

**Tests:** assert the `openehr-version` header value on a write. **Verify:** `go test ./openehr/client/...`.

### Task 3.2 — Status sentinel mapping: 422 (+ 400 surface)

**Files:** Modify [transport/client.go](../../../transport/client.go) (`statusToSentinel` ~:420-438), [transport/errors.go](../../../transport/errors.go) (add `ErrUnprocessable`).

**Change:** per Task 0.2 — map 422 → `ErrUnprocessable`; ensure 400 surfaces a `*WireError` with the openEHR code reachable via `errors.As` (no new sentinel required for 400 unless the spec warrants). Keep 428 mapping (defensive, documented non-canonical).

**Tests:** `TestWireError*` cases for 422/400. **Verify:** `go test ./transport/...`.

### Task 3.3 — Directory `path` sub-folder query param

**Files:** Modify [openehr/client/ehr/directory/directory.go](../../../openehr/client/ehr/directory/directory.go) (`Get`/`GetAtTime`/`GetVersioned`).

**Change:** add an optional `WithPath(string)` forwarding the spec `path` query param ([ehr:666,688](../../../resources/its-rest/ehr-validation.openapi.yaml#L666)) to fetch a sub-FOLDER.

**Tests:** assert `?path=` on the request. **Verify:** `go test ./openehr/client/ehr/directory/...`.

### Task 3.4 — Query `GET` variants (optional)

**Files:** Modify [openehr/client/query/execute.go](../../../openehr/client/query/execute.go).

**Change:** add `GET` execution for ad-hoc + stored queries (spec [query:224,273,333](../../../resources/its-rest/query-validation.openapi.yaml#L224)), URL-encoding `q`/`offset`/`fetch`/`query_parameters` (`style: form`). Lower priority — the spec recommends `POST` for long queries; gate behind an explicit option or a separate `ExecuteGET`. May be split into its own plan if it grows.

**Tests:** assert `GET` URL + param encoding. **Verify:** `go test ./openehr/client/query/...`.

---

## Final task — Traceability, roadmap, CI

**Files:** [docs/specifications/traceability.yaml](../../specifications/traceability.yaml), [docs/specifications/REQ.md](../../specifications/REQ.md), [docs/roadmap.md](../../roadmap.md), [CHANGELOG.md](../../../CHANGELOG.md).

- Add the new probes (`testkit/probes/rest/*`) and `ErrUnprocessable` to `traceability.yaml`; cite REQ-059/093/095/099.
- Advance REQ-059/REQ-095 notes; add roadmap notes on the corrected REST-client conformance.
- One CHANGELOG bullet per artefact class (conformance fixes).
- Run `make spec-check` then `make ci` — both green.

## Definition of Ready

- [x] Phase 0 normative edits reviewed (spec wins over prose per REQ-095).
- [x] Pinned spec lines re-confirmed against current `resources/its-rest/MANIFEST.txt` (re-run the audit greps if the pin moved).
- [x] Decision recorded on `created_timestamp` field rename: JSON-tag-only (Go field `CreatedOn` kept; non-breaking).

## Definition of Done

- [x] All Phase 0–3 tasks land with tests; `make ci` green (includes `make spec-check`).
- [ ] New wire-conformance probes pass and are registered in `conformance.md` + `traceability.yaml`. — **deferred to [STRAND-09](../../specifications/research-strands.md)**; wire shapes are covered by package `httptest` tests in the interim.
- [x] REQ-059/093/099 prose matches the authoritative YAML; REQ.md/roadmap reflect the advanced status.
- [x] CHANGELOG updated; no `docs/superpowers/` tree created (plans live here).
- [x] Deferred Tier-3 subsystems captured (see **Defers** header) — named follow-ups, scope cut explicit.

## Self-review notes

- **Coverage:** every audit finding maps to a task — Tier 1 → Phase 1 (+0); Tier 2 → Phase 2; small Tier 3 → Phase 3; large Tier 3 → deferred (header), explicitly. The audit's "confirmed conformant" items (RESULT_SET decode, `_type` polymorphism, If-Match quoting, Prefer branches) need no task.
- **Spec-defect vs code-bug** is called out per task so a reviewer can reject a spec edit independently of a code fix.
- **Probes** are added only for wire-level findings (header grammar, verb, path) where a cassette meaningfully guards conformance — consistent with REQ-080/REQ-082.
