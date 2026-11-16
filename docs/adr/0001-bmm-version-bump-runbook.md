---
kind: adr
id: ADR-0001
title: "BMM version-bump runbook"
status: accepted
date: 2026-05-16
---

# ADR 0001 — BMM version-bump runbook

- **Status:** Accepted, 2026-05-16.
- **Requirements:** [REQ-041](../specifications/reference-model.md#req-041--pinned-bmm-sources) (the pinned BMM sources a version bump moves).

## Context

The pinned BMM files under [`resources/bmm/`](../../resources/bmm/) are the SDK's **source of truth** for the openEHR Reference Model, Archetype Object Model, and Base types (see [`resources/bmm/README.md`](../../resources/bmm/README.md) and [`docs/specifications/reference-model.md`](../../docs/specifications/reference-model.md)). The BMM-driven generator (`cmd/bmmgen`, `internal/bmmgen`) emits `openehr/rm/`, `openehr/aom/aom14/`, and the typereg `_gen.go` files deterministically from those inputs.

Bumping any pinned BMM file touches the generated tree at a scale beyond casual hand-review: the RM has ~146 classes; AOM 1.4 has 39. Two failure modes follow:

1. **Hand-edits to generated files** silently re-introduce themselves on every regeneration cycle, causing CI noise and trust erosion.
2. **Accidental misalignment** between the BMM file, the conformance contract pins, the resources table, and the CHANGELOG entry makes the bump opaque to downstream consumers.

We need a deterministic, reviewable procedure so that any maintainer (or AI-driven contributor) can execute a BMM bump with the same outcome.

## Decision

A BMM version bump is one pull request with a deterministic, reviewable shape.
The new schema is added beside the old one and the old one removed in the same commit, so the diff shows a paired add and remove, and the SDK pins exactly one version per schema id.
Generated files are never edited by hand: `make codegen-check`, which `make test` runs, holds them to the generator, and a weekly drift bot (`.github/workflows/codegen-drift.yml`) re-runs that check on a clean checkout and tracks a failure in one `bmm-drift` issue.
The steps, roles and tooling are in [`resources/bmm/README.md` § Updating](../../resources/bmm/README.md#updating), beside the files they move.

## Consequences

**Positive.**

- A BMM version bump becomes a single pull request with a deterministic shape: the BMM file rename, the `_gen.go` diff, the version pins and one CHANGELOG bullet proposed in the pull request body.
- The drift bot catches three failure modes: accidental hand-edits to `_gen.go`, generator-template regressions, and BMM ingestion bugs introduced after the bump.
- The simulated-bump test in [`internal/bmmgen/sim_bump_test.go`](../../internal/bmmgen/sim_bump_test.go) exercises the regeneration path on every CI run, so the procedure is continuously verified.

**Negative.**

- Multi-bump batching is now harder: if the team wants to land RM 1.2.1 and AOM 1.4.1 in the same PR, the procedure runs twice. This is intentional — paired bumps are not the common case and "small reviewable diff" wins over "fewer PRs".
- The drift bot consumes a small amount of weekly CI budget (one Ubuntu runner-minute for the verify, plus a few seconds for the issue API). Acceptable.

**Neutral.**

- The procedure does not prescribe a SemVer impact (major / minor / patch); that is decided under [`docs/specifications/module-layout.md § Versioning`](../../docs/specifications/module-layout.md#versioning).

## See also

- commit [e1c6633c](https://github.com/Cadasto/openehr-sdk-go/commit/e1c6633c): the BMM version-bump tooling and the weekly codegen drift workflow this runbook governs.
- [`resources/bmm/README.md`](../../resources/bmm/README.md) — pinned BMM file inventory; § Updating holds the procedure.
- [`docs/specifications/reference-model.md`](../../docs/specifications/reference-model.md) — normative conformance contract; § Schema → Go package set carries the version pins.
- [`.github/workflows/codegen-drift.yml`](../../.github/workflows/codegen-drift.yml) — weekly drift bot implementation.
- [`cmd/bmmdiff`](../../cmd/bmmdiff/) — semantic BMM diff CLI.
