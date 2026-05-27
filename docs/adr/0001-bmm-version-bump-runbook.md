# ADR 0001 — BMM version-bump runbook

- **Status:** Accepted, 2026-05-16.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** part of [`docs/plans/archive/2026-05-15-bmm-codegen.md`](../plans/archive/2026-05-15-bmm-codegen.md) Phase 5.

## Context

The pinned BMM files under [`resources/bmm/`](../../resources/bmm/) are the SDK's **source of truth** for the openEHR Reference Model, Archetype Object Model, and Base types (see [`resources/bmm/README.md`](../../resources/bmm/README.md) and [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md)). The BMM-driven generator (`cmd/bmmgen`, `internal/bmmgen`) emits `openehr/rm/`, `openehr/aom/aom14/`, and the typereg `_gen.go` files deterministically from those inputs.

Bumping any pinned BMM file touches the generated tree at a scale beyond casual hand-review: the RM has ~146 classes; AOM 1.4 has 39. Two failure modes follow:

1. **Hand-edits to generated files** silently re-introduce themselves on every regeneration cycle, causing CI noise and trust erosion.
2. **Accidental misalignment** between the BMM file, the conformance contract pins, the resources table, and the CHANGELOG entry makes the bump opaque to downstream consumers.

We need a deterministic, reviewable procedure so that any maintainer (or AI-driven contributor) can execute a BMM bump with the same outcome.

## Decision

A BMM version bump MUST follow the numbered procedure below. CI enforces the deterministic-output invariant via `make codegen-verify`; the weekly drift bot (`.github/workflows/codegen-drift.yml`) catches both accidental hand-edits and generator-template changes that would silently break the next bump.

### Procedure

1. **Stage the new BMM file alongside the old.** Drop `openehr_rm_1.2.1.bmm.json` next to `openehr_rm_1.2.0.bmm.json`. **Do not overwrite** the old file in the same commit — the diff must show the rename as a paired add/remove.

2. **Regenerate.** Run:

   ```sh
   make codegen
   ```

   The generator writes (or rewrites) every `_gen.go` file under `openehr/rm/`, `openehr/aom/aom14/`, etc. It does not touch `*_ext.go` companions.

3. **Verify the regen is reproducible.** Run:

   ```sh
   make codegen-verify
   ```

   On a freshly-regenerated tree this MUST exit 0. If it does not, re-run step 2 and investigate — a non-deterministic generator is a bug, not a "bump consequence".

4. **(Optional but recommended) Inspect the semantic diff.** Run:

   ```sh
   go run ./cmd/bmmdiff \
     resources/bmm/openehr_rm_1.2.0.bmm.json \
     resources/bmm/openehr_rm_1.2.1.bmm.json
   ```

   `bmmdiff` understands the BMM structure and produces a human-readable summary (added/removed classes, per-class property add/remove/change, cardinality changes, function changes). For a one-line CHANGELOG suggestion add `-suggest-changelog`.

5. **Audit hand-written companions.** If the diff removes a class or renames a property, every `*_ext.go` companion referencing it WILL fail to compile against the regenerated `_gen.go` file. Run `go build ./...` and fix or remove the affected companions in this PR — do not defer.

6. **Update version pins.** Edit [`docs/specifications/bmm-conformance.md § Schema → Go package set`](../../docs/specifications/bmm-conformance.md#schema--go-package-set) so the table reflects the new schema id and bmm_version.

7. **Update [`resources/bmm/README.md`](../../resources/bmm/README.md)** — both the `Files` table (schema id, bmm_version, class count) and any prose that pins a specific version. The `## Updating` section there defers to this ADR; do not duplicate the procedure.

8. **Add a CHANGELOG entry.** Drop a one-liner under [`CHANGELOG.md`](../../CHANGELOG.md) `## [Unreleased]`, in the appropriate sub-section:

   - **Added** — pure additions (new classes / properties).
   - **Changed** — type changes, cardinality changes, ancestor-chain changes.
   - **Removed** — class or property deletions.

   The `bmmdiff -suggest-changelog` output is a good starting point but MUST be reviewed by a human; it favours brevity over editorial polish. Keep the bullet **short and high-level** per [`AGENTS.md § Code style and conventions`](../../AGENTS.md#code-style-and-conventions) — one line, one artefact class.

9. **Remove the old BMM file in the same commit.** Never leave both versions in `resources/bmm/` — the SDK pins exactly one version per schema id at a time. The paired add/remove makes the rename reviewable.

10. **Audit narrow-interface accessors** (SDK-GAP-11 / PROBE-038). If `bmmdiff -suggest-changelog` reports an Added class whose `ancestors` chain includes any of `DV_TEXT`, `DV_URI`, `AUDIT_DETAILS`, `PARTY_IDENTIFIED`, or `OBJECT_REF`, the generator auto-extends the matching `<Parent>Like` interface via its marker-method walk, but the closed type-switches in [`openehr/rm/like_accessors.go`](../../openehr/rm/like_accessors.go) DO NOT pick the new subtype up — each needs an explicit `case *NewSubtype:` arm to recover the parent payload. Add the arm, then pin a round-trip case for the new subtype under [`openehr/serialize/canjson/polymorphic_decode_test.go`](../../openehr/serialize/canjson/polymorphic_decode_test.go) so PROBE-038's substitution guarantee covers it.

11. **Open the PR.** The weekly drift-bot will pass on subsequent runs since `make codegen-verify` is now green; the PR's normal CI runs `make test` which includes `codegen-verify`.

### Roles

- **Author** runs steps 1–9, opens the PR, requests review.
- **Reviewer** confirms the BMM diff (step 4 output is the ideal artefact to paste into the PR body) matches the Go-side diff scope, and that the CHANGELOG bullet correctly classifies the change.
- **The drift bot** (`.github/workflows/codegen-drift.yml`) acts after merge: a green next-Monday run is the definition of "the bump landed cleanly".

### Tooling guarantees

- `make codegen-verify` is wired into `make test` (Makefile line numbers may drift). A passing local `make test` is sufficient evidence the regen is reproducible.
- `cmd/bmmdiff` is an inspection tool — exit 0 always, output to stdout. Suitable for piping into PR descriptions or CHANGELOG drafts.
- The drift bot's tracking-issue convention is one open issue labelled `bmm-drift`. Subsequent drift runs comment on the open issue rather than spawning duplicates.

## Consequences

**Positive.**

- A BMM version bump becomes a single PR with deterministic shape: BMM file rename + a small `_gen.go` diff + four documentation updates + one CHANGELOG bullet.
- The drift bot catches three failure modes: accidental hand-edits to `_gen.go`, generator-template regressions, and BMM ingestion bugs introduced after the bump.
- The simulated-bump test in [`internal/bmmgen/sim_bump_test.go`](../../internal/bmmgen/sim_bump_test.go) exercises the regen path on every CI run, so the procedure is not just documentation — it is continuously verified.

**Negative.**

- Multi-bump batching is now harder: if the team wants to land RM 1.2.1 and AOM 1.4.1 in the same PR, the procedure runs twice. This is intentional — paired bumps are not the common case and "small reviewable diff" wins over "fewer PRs".
- The drift bot consumes a small amount of weekly CI budget (one Ubuntu runner-minute for the verify, plus a few seconds for the issue API). Acceptable.

**Neutral.**

- The procedure does not prescribe a SemVer impact (major / minor / patch); that lives in [`docs/specifications/module-layout.md § Versioning`](../../docs/specifications/module-layout.md#versioning). The CHANGELOG sub-section choice (Added / Changed / Removed) is the closest the runbook gets to a SemVer signal.

## See also

- [`docs/plans/archive/2026-05-15-bmm-codegen.md`](../plans/archive/2026-05-15-bmm-codegen.md) — Phase 5 ("Drift bot + version-bump runbook").
- [`resources/bmm/README.md`](../../resources/bmm/README.md) — pinned BMM file inventory; the `## Updating` section defers to this ADR.
- [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) — normative conformance contract; § Schema → Go package set carries the version pins.
- [`.github/workflows/codegen-drift.yml`](../../.github/workflows/codegen-drift.yml) — weekly drift bot implementation.
- [`cmd/bmmdiff`](../../cmd/bmmdiff/) — semantic BMM diff CLI.
