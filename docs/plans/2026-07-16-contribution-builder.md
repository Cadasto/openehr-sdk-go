# Plan — ContributionBuilder (fluent Contribution_create assembly)

**Date:** 2026-07-16
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **REQ-130** (Contribution builder) — proposed; canonical home to be authored at [wire.md § REQ-130](../specifications/wire.md#req-130--contribution-builder) in Phase 0. The wire band (050–059) is exhausted, so REQ-130 opens a **new "SDK authoring & client tooling" numbering band (130–139)** — that policy row is a Phase 0 task (see below).
**Builds on:** landed [REQ-050](../specifications/wire.md#req-050) (REST pin); [REQ-059](../specifications/wire.md#req-059) (openEHR custom headers, **Impl. `partial`** — the lifecycle-state header this builder leans on is not fully landed); [use-cases.md § Synthetic data seeder](../specifications/use-cases.md#synthetic-data-seeder) (names ContributionBuilder as SDK-provided)
**Probes:** **PROBE-084** (ContributionBuilder wire shape)
**Implementation:** planned
**Depends on:** landed `openehr/client/ehr/contribution/` (`Submission`, `WrapOriginalVersion`, `UpdateAudit`, `Commit`); the already-landed submission-shape contract (REQ-050/094/095, PROBE-072)
**Defers:** IMPORTED_VERSION authoring helpers beyond one `AddImported` sugar; multi-EHR batching; checkpoint/resume (the seeder's job per use-cases.md)

## Goal

Provide a **fluent builder** that assembles a spec-conformant `Contribution_create` body (`contribution.Submission`) without hand-wiring `ORIGINAL_VERSION` wrappers, change-type codes, and write-side audit fields. Named explicitly in [use-cases.md](../specifications/use-cases.md) as SDK-provided for the synthetic data seeder; closes an ergonomics gap flagged (against peer openEHR SDKs) in the peer-SDK ecosystem fit-gap review.

## Architecture

```
Builder (mutable accumulator)
  .SetAudit(UpdateAudit | functional options)
  .AddCreation[*T](data T, opts...)
  .AddAmendment[*T](precedingUID, data T, opts...)
  .AddModification[*T](...)
  .AddDeletion[*T](...)
        │
        ▼
Build() → *contribution.Submission  (Validate() passes)
        │
        ▼
contribution.Commit(ctx, client, ehrID, submission)
```

- Builder lives in **`openehr/client/ehr/contribution/`** next to `Submission` — same package, no new import cycle.
- Generic over `T ∈ {rm.Composition, rm.EHRStatus, rm.Folder, rm.EHRAccess}` matching the closed type-set in `submission.go`.
- Reuses existing `WrapOriginalVersion` / `WrapImportedVersion`, `UpdateAudit`, and the openEHR `audit_change_type` terminology codes (`249` creation, `250` amendment, `251` modification, `523` deleted — note `523`, not `253`, is the "deleted" code).
- **Immutable after `Build()`** — `Build()` returns a copy; a second `Build()` on the same builder is idempotent, or a documented panic-free `Reset()`.

**Open question for REQ-130 to resolve (do not settle it here):** whether a per-version `lifecycle_state` is carried in the `ORIGINAL_VERSION` body or via the `openehr-version` header at Commit time. This is a wire-shape decision — REQ-130 (with REQ-059) must answer it; the builder then follows the spec, rather than the plan pre-empting it.

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed REQ-130**:

- `Covers:` names the REQ this plan implements (REQ-130) and the REQs it builds on (REQ-050 landed, REQ-059 partial).
- Canonical normative prose for REQ-130 exists — a `wire.md § REQ-130` section + a `REQ.md` registry row + the new numbering-policy band — authored via `sdd-specify` (Phase 0). Until then this DoR item is **pending**, not satisfied.
- The change-type code table and the builder's normative surface live **once**, in REQ-130 (not duplicated in this plan).
- The lifecycle-state placement question (above) is resolved in REQ-130.
- PROBE-084 wire assertion defined in `conformance.md` (Draft).

## Definition of Done

- `contribution.Builder` landed with tests + `// REQ-130`.
- `cmd/examples/contribution-build/` example (create + amend two compositions in one batch).
- PROBE-084 compares the marshalled body to golden `testkit/cassettes/submissions/`.
- `use-cases.md` **Impl.** note; `traceability.yaml` + the REQ.md **Impl.** column (REQ-130 `planned → landed`) updated.
- `make spec-check` + `make ci` green; plan archived (or **Status:** complete).

## Implementation checklist

| Step | Status |
|---|---|
| REQ-130 § + registry row + numbering-policy band (`wire.md`, `REQ.md`) | |
| PROBE-084 defined in `conformance.md` (Draft) | |
| Builder code | |
| Tests with `// REQ-130` / `// PROBE-084` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Spec, numbering band & registry (the specify gate)

The MUST bullets below are a **draft seed to author into REQ-130** — the canonical home is `wire.md`, not this plan.

**Tasks:**

1. Amend the `REQ.md` numbering policy: add the **"SDK authoring & client tooling" band (130–139)** row (the wire band 050–059 is exhausted).
2. Author **REQ-130** in `wire.md` (via `sdd-specify`). Draft normative surface:
   - the SDK exposes a builder producing `*Submission` that passes `Validate()` and `MarshalJSON`;
   - it sets per-version `commit_audit.change_type` and batch-level `audit` consistently;
   - it drops `time_committed` from the write-side audit (inheriting the existing `UpdateAudit` rules);
   - it uses no reflection (REQ-024) — explicit methods per `T` or explicit generic instantiations;
   - the lifecycle-state placement decision (see Architecture) is stated normatively.
3. Add the `REQ.md` registry row (**Impl.:** `planned`; spec section **Status:** `Draft`) + define PROBE-084 in `conformance.md` (Draft) + `traceability.yaml` row.

**Definition of done:** `make spec-check` passes with the new rows and band.

### Phase 1 — Builder API

**Tasks:**

1. Add `builder.go` in `openehr/client/ehr/contribution/`:

```go
type Builder struct { /* audit UpdateAudit; versions []CommitVersion */ }

func NewBuilder() *Builder
func (b *Builder) SetAudit(a UpdateAudit) *Builder
func (b *Builder) WithCommitter(name string) *Builder
func (b *Builder) WithDescription(desc string) *Builder
func (b *Builder) WithSystemID(id string) *Builder

func (b *Builder) AddCreation(comp *rm.Composition, opts ...VersionOption) *Builder
func (b *Builder) AddAmendment(precedingUID string, comp *rm.Composition, opts ...VersionOption) *Builder
// … EHRStatus, Folder, EHRAccess overloads OR single generic with type constraint

func (b *Builder) Build() (*Submission, error)
```

2. `version_option.go` — per-version description override and the lifecycle-state hint, carried per REQ-130's resolution of the open question (not decided here).
3. Internal helper `wrapCreation[T](data *T) (*OriginalVersion[T], error)` building a minimal `rm.OriginalVersion[T]` from the canonical payload (uid optional on create — server assigns).
4. Unit tests `builder_test.go`:
   - Single creation → valid Submission.
   - Creation + amendment → two versions, distinct change types.
   - Empty builder → `Build()` error.
   - Marshalled JSON matches the existing `submission_test.go` golden patterns.

**Definition of done:** `go test ./openehr/client/ehr/contribution/...` green.

### Phase 2 — Example & seeder hook

**Tasks:**

1. `cmd/examples/contribution-build/main.go` — build a batch from two canonical compositions (fixture), print JSON, optional `-commit` against a sandbox URL.
2. Cross-link from `docs/examples.md`.
3. Add one test in `testkit/probes/versioned/` (or extend the contribution tests) showing Builder → Commit sandbox round-trip (if a cassette exists).

**Definition of done:** `make ci` green.

### Phase 3 — PROBE-084 & docs

**Tasks:**

1. PROBE-084: byte-stable marshal of Builder output vs checked-in `testkit/cassettes/submissions/*.json` (modulo documented ordering).
2. Update `roadmap.md`, flip the REQ.md **Impl.** column for REQ-130 to `landed`, archive plan.

**Definition of done:** PROBE-084 in `make test`; REQ-130 **Impl.** = `landed`; `make spec-check` green.

## Mapping to specs

- [wire.md § REQ-130](../specifications/wire.md#req-130--contribution-builder) — the requirement this plan implements (registry row: [REQ.md](../specifications/REQ.md))
- [wire.md § REQ-050](../specifications/wire.md#req-050) — REST pin
- [wire.md § REQ-059](../specifications/wire.md#req-059) — openEHR custom headers (lifecycle-state; partial)
- [conformance.md § PROBE-072](../specifications/conformance.md) — the landed submission-shape contract this builds on
- Archived [2026-05-26-contribution-submission-shape.md](archive/2026-05-26-contribution-submission-shape.md)

## References

- Peer openEHR SDKs' contribution builders (a Python peer SDK; the EHRbase Java SDK `ContributionBuilder`) — the pattern this adapts; see the peer-SDK ecosystem fit-gap review.
- Existing: `submission.go`, `version.go`, `contribution.go`.
