# Plan — Contribution `UPDATE_AUDIT` wire shape (ITS-REST PR 131 / SPECITS-95)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-06-11
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-050](../../specifications/wire.md#req-050), [REQ-095](../../specifications/wire.md#req-095) (OpenAPI authoritative source); proposed addendum to REQ-059 (commit audit on write paths)
**Probes:** **PROBE-072** (extend or sibling probe for `UPDATE_AUDIT` / `change_type` shape); possible **PROBE-073** (lifecycle_state on `UpdateVersion`)
**Implementation:** planned
**Depends on:** Landed [`contribution.Submission`](../../openehr/client/ehr/contribution/submission.go) + [archive plan](archive/2026-05-26-contribution-submission-shape.md); upstream merge of [openEHR/specifications-ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131)
**Defers:** Demographic contribution client (same schema family — mirror EHR changes when `openehr/client/demographic/` lands); re-pinning bundled OAS artefacts in-repo (SDK consumes spec by reference, not vendored OAS today)

## Goal

Adapt the SDK's **contribution write path** and related version-commit audit fields to the corrected ITS-REST 1.1.0 wire types once [PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131) lands ([SPECITS-95](https://specifications.openehr.org/tickets/SPECITS-95)).

**Problem (upstream):** Release 1.0.3 OAS typed `UPDATE_AUDIT.change_type` as `TERMINOLOGY_CODE`, while:

- the schema's own inline example used a `DV_CODED_TEXT`-shaped object;
- the read side (`AUDIT_DETAILS` on `GET …/contribution/{uid}`) already uses `DV_CODED_TEXT`;
- tested CDRs (Better, EHRbase) reject `TERMINOLOGY_CODE` and expect `DV_CODED_TEXT`.

PR 131 fixes the source schemas:

| Schema | Change |
|---|---|
| `schemas/common/UpdateAudit.yaml` | `change_type` → `DvCodedText`; optional `system_id`; `_type: UPDATE_AUDIT`; clarifies DTO vs persisted `AUDIT_DETAILS` |
| `schemas/ehr/UpdateVersion.yaml` | `lifecycle_state` → `DvCodedText` |
| `schemas/demographic/UpdateVersion.yaml` | same |
| `schemas/base_types/TerminologyCode.yaml` | **removed** (no remaining OAS consumers) |
| `contribution_create` operations | document `_type` handling (`UPDATE_AUDIT` SHOULD; servers SHOULD accept `AUDIT_DETAILS` / omitted) |

**SDK today:**

- [`contribution.Submission`](../../openehr/client/ehr/contribution/submission.go) marshals `{audit, versions[]}` for `Contribution_create`.
- `Submission.Audit` is [`rm.AuditDetails`](../../openehr/rm/common_generic_gen.go) — the **persisted RM class**, not the REST `UPDATE_AUDIT` DTO.
- `Version.commit_audit` inside each inline version is also `rm.AuditDetails`.
- Test helpers and [`testkit/cassettes/submissions/`](../../testkit/cassettes/submissions/) already use **`DV_CODED_TEXT`-shaped** `change_type` (nested `defining_code.terminology_id.value`) — aligned with PR 131's fix, not with the erroneous 1.0.3 `TERMINOLOGY_CODE` schema.
- RM-generated types already model `change_type` as `rm.DVCodedText` on `AUDIT_DETAILS` — the mismatch is **REST DTO semantics** (`UPDATE_AUDIT` vs `AUDIT_DETAILS`, optional/forbidden server fields, `_type` discriminator), not RM codegen.

**Consumers:** reference CDR load harness, seeder tools, MCP servers posting contributions; PROBE-072 conformance consumers.

## Implementation checklist

| Step | Status |
|---|---|
| Track ITS-REST PR 131 merge + Release-1.1.0 amendment 5.9 | |
| Normative text in [`wire.md`](../../specifications/wire.md) (REQ-050 / REQ-059 addendum) | |
| Wire DTO types + `Submission` marshalling | |
| PROBE-072 extended or PROBE-073 added | |
| `traceability.yaml` + `conformance.md` | |
| Submission cassettes audited (no `TERMINOLOGY_CODE` shape) | |
| `make spec-check` + `make ci` | |

## Phases

### Phase 0 — Spec tracking + gap analysis

**Outcome:** SDK normative docs describe the target wire shape before code changes.

**Tasks:**

- [ ] Watch [ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131) until merged; record merged commit / Release-1.1.0 amendment reference in this plan header.
- [ ] Read merged `UpdateAudit.yaml`, `UpdateVersion.yaml`, and `contribution_create.yaml` operation text.
- [ ] Audit current `contribution.Submission` JSON output:
  - Top-level `audit._type` — today emits `AUDIT_DETAILS` via RM marshal?
  - `audit.time_committed` / `audit.system_id` — spec says server-assigned / optional on commit; document whether SDK should omit them on write.
  - Per-version `commit_audit` — same DTO question for nested audits.
  - `lifecycle_state` on `ORIGINAL_VERSION` / `IMPORTED_VERSION` — confirm `DVCodedText` emission matches `DvCodedText` OAS (terminology_id nesting).
- [ ] Grep repo for `TERMINOLOGY_CODE` on contribution write paths — expect **none** on wire; RM `TerminologyCode` type remains for other contexts.

**Definition of done:** Gap table (field → current SDK → PR 131 target → action) attached to Phase 1 PR.

### Phase 1 — Wire DTO types (distinct from persisted RM)

**Outcome:** Typed request-side audit envelopes that marshal to `UPDATE_AUDIT`, not full `AUDIT_DETAILS`.

**Tasks:**

- [ ] Introduce `openehr/client/ehr/contribution` (or `openehr/wire/` if shared with demographic later) DTOs mirroring OAS:
  ```go
  // UpdateAudit is the ITS-REST commit DTO (not rm.AUDIT_DETAILS).
  // REQ-059 addendum / SPECITS-95.
  type UpdateAudit struct {
      Type        string // json:"_type,omitempty" — default UPDATE_AUDIT
      SystemID    *string
      ChangeType  rm.DVCodedText
      Description *rm.DVText
      Committer   rm.PartyProxy // existing PartyIdentified / PartySelf / …
  }
  ```
- [ ] Change `Submission.Audit` from `rm.AuditDetails` to `UpdateAudit` (or embed with custom marshal). **Breaking change** for any in-tree caller — acceptable pre-1.0; document in CHANGELOG.
- [ ] Change `rm.Version[T].CommitAudit` usage on **write path only** — options:
  - **A (preferred):** `OriginalVersion` / `ImportedVersion` submission wrapper types with `CommitAudit UpdateAudit` for marshalling only; keep RM types for GET decode.
  - **B:** Custom `MarshalJSON` on version types that emits `UPDATE_AUDIT` when nested under `Submission` (harder — avoid context-sensitive marshal).
- [ ] `MarshalJSON` rules:
  - Emit `_type: "UPDATE_AUDIT"` by default; document that servers MAY accept `AUDIT_DETAILS` / omitted (interop note from PR 131).
  - Omit `time_committed` on write (server assigns).
  - Treat `system_id` as optional pointer — omit when unset unless caller explicitly sets.
- [ ] Conversion helpers: `UpdateAuditFromAuditDetails(ad rm.AuditDetails) UpdateAudit` for callers that already hold persisted-shaped audits (strip server fields).

**Definition of done:** Unit tests assert JSON fragments: `"_type":"UPDATE_AUDIT"`, `change_type` has `defining_code` nesting, no `TERMINOLOGY_CODE` envelope, no required `time_committed`.

### Phase 2 — PROBE + spec registry

**Outcome:** Conformance pin prevents regression to wrong audit DTO or terminology-code shape.

**Tasks:**

- [ ] Extend **PROBE-072** or add **PROBE-073** in [`docs/specifications/conformance.md`](../../specifications/conformance.md):
  - Request body top-level `audit._type` is `UPDATE_AUDIT` (or document accepted alternates if SDK deliberately omits `_type`).
  - `audit.change_type` matches `DvCodedText` shape (has `defining_code`, not flat terminology-code triple).
  - At least one version's `lifecycle_state` uses coded-text shape when present.
- [ ] Implement probe in [`testkit/probes/versioned/`](../../testkit/probes/versioned/) — httptest body capture (same pattern as PROBE-072).
- [ ] Update [`docs/specifications/wire.md`](../../specifications/wire.md) § Request vs response asymmetry — add bullet for `UPDATE_AUDIT` vs `AUDIT_DETAILS` on contribution create.
- [ ] Update [`docs/specifications/REQ.md`](../../specifications/REQ.md) + [`traceability.yaml`](../../specifications/traceability.yaml) if new PROBE or REQ-059 addendum is registered.

**Definition of done:** `go test ./testkit/probes/versioned/...` green; `make spec-check` green.

### Phase 3 — Cassettes + consumer migration

**Outcome:** Vendored submission JSON and callers use the DTO types consistently.

**Tasks:**

- [ ] Audit [`testkit/cassettes/submissions/*.json`](../../testkit/cassettes/submissions/) — most already use DV_CODED_TEXT `change_type`; update any that emit wrong `_type` or server-only fields to match PR 131 examples.
- [ ] Update [`testkit/cassettes/submissions/README.md`](../../testkit/cassettes/submissions/README.md) with `UPDATE_AUDIT` note + link to this plan.
- [ ] Migrate [`contribution_test.go`](../../openehr/client/ehr/contribution/contribution_test.go) helpers (`newAudit`) to `UpdateAudit` builders.
- [ ] Coordinate private reference CDR harness if it constructs `Submission` with `rm.AuditDetails` (out-of-tree — note in PR).

**Definition of done:** `go test ./openehr/client/ehr/contribution/...` + probe tests green; cassette README accurate.

## Mapping to specs

- [REQ-050](../../specifications/wire.md#req-050) — REST wire surface; contribution create payload
- [REQ-095](../../specifications/wire.md#req-095) — OpenAPI is authoritative; track ITS-REST amendment 5.9
- [PROBE-072](../../specifications/conformance.md#probe-072--contribution-submission-body-matches-contribution_create-sdk-gap-10) — submission shape (extend)
- Archived: [2026-05-26-contribution-submission-shape.md](archive/2026-05-26-contribution-submission-shape.md) — landed `Submission` vs `rm.Contribution`
- Upstream: [specifications-ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131), [Discourse thread](https://discourse.openehr.org/t/contribution-update-audit-change-type-in-rest-api-vs-vendor-implementations/16928)

## Risk notes

| Risk | Mitigation |
|---|---|
| **Breaking public API** — `Submission.Audit` type change | Pre-1.0; CHANGELOG + migration helpers |
| **Dual audit shapes** — read vs write | Keep `rm.AuditDetails` for GET decode; DTO only on write path |
| **PR 131 not merged yet** | Phase 0–1 can land DTO + tests against merged YAML snapshot; pin spec ref in tests |
| **`_type` interop** | Document SHOULD/SHOULD accept matrix from PR 131; do not reject server leniency in client marshal |

## Open questions

1. **Shared wire package** — place `UpdateAudit` under `contribution/` vs new `openehr/wire/` for reuse by demographic contribution (deferred client).
2. **PROBE numbering** — extend 072 vs new 073 for audit DTO — decide when implementing Phase 2.
3. **Composition / directory write paths** — do singular `Save`/`Update` endpoints share `UpdateAudit` for any audit fields? Audit scope in Phase 0 gap table.

## References

- ITS-REST PR: https://github.com/openEHR/specifications-ITS-REST/pull/131
- Ticket: https://specifications.openehr.org/tickets/SPECITS-95
- SDK submission type: [`openehr/client/ehr/contribution/submission.go`](../../openehr/client/ehr/contribution/submission.go)
- Example cassette (already DV_CODED_TEXT): [`testkit/cassettes/submissions/contributions_valid_minimal_minimal_admin.contribution.json`](../../testkit/cassettes/submissions/contributions_valid_minimal_minimal_admin.contribution.json)
