# Plan — `contribution.Commit` submission-shape fix

**Date:** 2026-05-26
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-050](../specifications/wire.md#req-050), [REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation), [REQ-095](../specifications/wire.md#req-095) (OpenAPI authoritative source); proposed addendum to REQ-059
**Probes:** new — proposed **PROBE-072** (contribution submission body matches `Contribution_create`)
**Implementation:** **not landed** — `contribution.Commit` currently serialises the **persisted** shape (`versions: []OBJECT_REF`) and is rejected by every spec-conformant CDR
**Depends on:** [SDK-GAP-09 / PR #17](https://github.com/Cadasto/openehr-sdk-go/pull/17) (bare-response decode contract — already landed); ITS-REST OpenAPI [`ehr-html.openapi.yaml`](https://github.com/openEHR/specifications-ITS-REST/blob/master/computable/OAS/ehr-html.openapi.yaml) §`Contribution_create`
**Defers:** SMART-on-openEHR token forwarding for multi-version commits (orthogonal)

## Goal

Close [SDK-GAP-10](https://github.com/Cadasto/openehr-go-poc/blob/main/docs/sdk-gap-drafts/SDK-GAP-10-contribution-commit-submission-shape.md) — symmetric to the SDK-GAP-09 fix for `composition.Save/Update`. The openEHR ITS-REST `POST /ehr/{ehr_id}/contribution` endpoint expects a `Contribution_create` payload whose `versions[]` carries the **inline** `ORIGINAL_VERSION<T>` (with `data: T` for `T ∈ {Composition, EHRStatus, Folder, EHRAccess}`), not the persisted `rm.Contribution` shape whose `versions[]` is `[]OBJECT_REF`.

## Out of scope

- **Changing the response decode shape** — `*rm.Contribution` is correct for the 201/200 response body; the persisted shape returned by the server matches what the SDK already decodes.
- **Per-version Prefer header** — `Prefer: return=representation` is request-level only per spec.
- **`auth/smart` token forwarding for batch commits** — orthogonal.
- **Generic batch-write abstraction over EHR_STATUS / FOLDER / EHR_ACCESS write paths** — `Contribution.versions[]` is the union; per-resource singular writes already exist.

## Problem in detail

Current state in [`openehr/client/ehr/contribution/contribution.go`](../../openehr/client/ehr/contribution/contribution.go):

```go
func Commit(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID,
    batch *rm.Contribution, opts ...CommitOption) (*rm.Contribution, ...)
```

`batch` is the **persisted** RM `CONTRIBUTION` (`openehr/rm/composition_gen.go`) whose `versions[]` is `[]rm.ObjectRef`. canjson-marshals to:

```json
{ "_type": "CONTRIBUTION",
  "uid":      { "_type": "HIER_OBJECT_ID", "value": "..." },
  "audit":    { "_type": "AUDIT_DETAILS", ... },
  "versions": [ {"_type": "OBJECT_REF", ...} ] }
```

The CDR rejects this — at submission time the OBJECT_REFs point to versions that do not yet exist. The spec's `Contribution_create` shape pins each `versions[i]` as a typed `ORIGINAL_VERSION<T>` (or `IMPORTED_VERSION<T>`) with the resource payload inline under `data`:

```json
{ "audit":    { "_type": "AUDIT_DETAILS", "description": "..." },
  "versions": [ { "_type":             "ORIGINAL_VERSION",
                  "contribution":      null,
                  "data":              { "_type": "COMPOSITION", ... },
                  "lifecycle_state":   { ... },
                  "preceding_version_uid": { ... } } ] }
```

`rm.OriginalVersion[*rm.Composition]` already exists in `openehr/rm/` (used today by `versioned_composition` GET decode). The submission shape is essentially `{audit, versions: []rm.Version}` where `rm.Version` is the abstract VERSION family (ORIGINAL_VERSION or IMPORTED_VERSION) discriminated by `_type`.

## Phases

### Phase 0 — Failing repro + probe stub

**Outcome:** PROBE-072 reserved in `conformance.md`; failing integration test pinned.

**Tasks:**

1. **`docs/specifications/conformance.md`** — reserve PROBE-072 ("Contribution submission body matches `Contribution_create` schema") with **Status: Draft**. Title + Wire assertion only; status flips to Implemented in Phase 2.
2. **Failing repro test** in `openehr/client/ehr/contribution/contribution_test.go` — POST to an httptest stub that asserts the request body's `versions[0]._type == "ORIGINAL_VERSION"` (not `OBJECT_REF`) and contains a `data` field. Pin as the regression gate.

### Phase 1 — Submission request type

**Outcome:** Typed request-side shape distinct from the persisted `rm.Contribution`.

**Tasks:**

1. **New type** in `openehr/client/ehr/contribution/`:
   ```go
   // Submission is the ITS-REST request-side payload for
   // POST /ehr/{ehr_id}/contribution. Distinct from rm.Contribution
   // (the persisted/response shape) because the submission carries
   // inline ORIGINAL_VERSION<T> payloads under data, while the
   // persisted shape carries OBJECT_REFs.
   type Submission struct {
       Audit    rm.AuditDetails
       Versions []rm.Version  // closed type-switch: *rm.OriginalVersion[T] | *rm.ImportedVersion[T]
   }
   ```
   Marshalling lives on `Submission` (custom `MarshalJSON` that emits canjson per-element via existing polymorphic dispatch). No reflection — closed switch over the concrete `T ∈ {*Composition, *EHRStatus, *Folder, *EHRAccess}`.
2. **`Commit` signature change** —
   ```go
   func Commit(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID,
       batch *Submission, opts ...CommitOption) (*rm.Contribution, *VersionMetadata, error)
   ```
   Returns the persisted `*rm.Contribution` (response shape unchanged — that's GAP-09 territory).
3. **`Repository.Commit`** updated; internal callers (none in-tree today, but document the breaking-change call surface in the commit message).
4. **Unit tests**: round-trip `Submission` → canjson bytes → byte-fragment assertions that `_type:ORIGINAL_VERSION` appears, `data._type:COMPOSITION` appears, `OBJECT_REF` does NOT appear.

**Definition of done:**

- `go test ./openehr/client/ehr/contribution/...` green including the Phase 0 failing test (now passing).
- `Submission` documented in package doc + `clinical-modeling.md` cross-reference.

### Phase 2 — PROBE-072 implementation + cassette

**Outcome:** Cross-package round-trip pinned.

**Tasks:**

1. **PROBE-072** at `testkit/probes/versioned/probe_072_contribution_submission_shape.go` (versioned package fits — contribution is a versioned-write surface). Pure-Go probe; assert the request body shape via httptest captured request.
2. **Cassette**: record one in `testkit/cassettes/its_rest/ehr/contribution_submit.json` if not already present (POST request + 201 response with persisted shape) so consumers can replay the round-trip.
3. **Conformance.md PROBE-072 status** flipped Draft → Implemented (Sandbox).
4. **REQ.md + traceability.yaml** — REQ-050 packages list extended with `openehr/client/ehr/contribution`; PROBE-072 added.

**Definition of done:**

- `go test ./testkit/probes/versioned/...` green for PROBE-072.
- `make ci` green; `make spec-check` happy.

### Phase 3 — Consumer alignment + survey

**Outcome:** SDK consumers (openehr-cdr benchmark, integration tests) migrated; spec ambiguity caveat resolved.

**Tasks:**

1. **openehr-cdr round-trip test** that currently bypasses `contribution.Commit` via a hand-rolled `postContribution` helper ([`tests/integration/sdk_roundtrip_test.go`](https://github.com/Cadasto/openehr-go-poc/blob/main/tests/integration/sdk_roundtrip_test.go)) can be migrated to use the new `Submission` type. Coordinate the consumer-side switch in their plan.
2. **Cross-SDK survey** — ehrbase, Better, Ocean, DIPS. If any major implementation accepts only the persisted shape, this gap escalates to "spec ambiguity" per the GAP-10 acceptance criteria. Expectation: the spec is explicit; survey confirms uniform implementation of `Contribution_create`.
3. **CHANGELOG** entry: add to the REST-clients bullet (mirrors SDK-GAP-09's CHANGELOG note).

**Definition of done:**

- openehr-cdr integration tests use `contribution.Commit(ctx, c, ehrID, &Submission{...})` without falling back to a private helper.
- Cross-SDK survey results recorded in this plan or its archive entry.

## Cross-references

- [SDK-GAP-10 draft](https://github.com/Cadasto/openehr-go-poc/blob/main/docs/sdk-gap-drafts/SDK-GAP-10-contribution-commit-submission-shape.md) — consumer report.
- [SDK-GAP-09 / PR #17](https://github.com/Cadasto/openehr-sdk-go/pull/17) — symmetric **response**-side fix; landed.
- [`2026-05-15-rest-api-client.md`](2026-05-15-rest-api-client.md) §Phase 4 — original contribution surface; this plan is the corrective.
- ITS-REST OpenAPI: `Contribution_create` schema definition.

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — PROBE-072 stub + failing test | |
| Phase 1 — `Submission` type + `Commit` signature change | |
| Phase 1 — round-trip unit tests | |
| Phase 2 — PROBE-072 implementation + cassette | |
| Phase 2 — REQ.md + traceability + conformance updates | |
| Phase 3 — openehr-cdr migration | |
| Phase 3 — cross-SDK survey results recorded | |
| `make ci` green | |
