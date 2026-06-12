# Plan — BMM upstream alignment (`openEHR/BMM-publisher`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Date:** 2026-06-11
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-041](../../specifications/bmm-conformance.md#req-041--pinned-bmm-sources), [REQ-042](../../specifications/bmm-conformance.md#req-042--generated-code-drift-detected), [REQ-045](../../specifications/bmm-conformance.md#req-045--bmm-loader)
**Probes:** — (existing PROBE-030..038 cover generated RM/canjson; extend only if a bump changes wire semantics)
**Implementation:** planned
**Depends on:** [ADR 0001 — BMM version-bump runbook](../adr/0001-bmm-version-bump-runbook.md) (landed procedure for executing a bump once inputs are chosen)
**Defers:** AOM 2.4 / LANG / TERM code generation (deferred BMM files stay pinned for reference only); automated nightly sync from upstream (optional follow-up)

## Goal

Establish a repeatable workflow to **import, verify, and land** openEHR BMM `P_BMM` JSON from the upstream publisher repo ([`openEHR/BMM-publisher`](https://github.com/openEHR/BMM-publisher)) into [`resources/bmm/`](../../resources/bmm/), then regenerate and review the SDK's generated tree (`openehr/rm/`, `openehr/aom/aom14/`, `openehr/rm/rminfo/`, `openehr/rm/typereg/`).

Today the SDK pins byte-identical copies with manual provenance notes in [`resources/bmm/README.md`](../../resources/bmm/README.md). Upstream ships the same `.bmm.json` files under `resources/` and publishes them via Docker (`ghcr.io/openehr/bmm-publisher`). This plan closes the gap between "we know upstream exists" and "we can confidently diff, import, and bump with CI gates."

**Consumers:** SDK maintainers and AI agents executing BMM bumps; downstream apps indirectly via regenerated RM types and `rminfo` lookup tables.

## Upstream source of truth

| Upstream | Role |
|---|---|
| [`openEHR/BMM-publisher`](https://github.com/openEHR/BMM-publisher) `resources/*.bmm.json` | Canonical **P_BMM JSON** inputs shipped with the publisher tool |
| [`ghcr.io/openehr/bmm-publisher`](https://ghcr.io/openehr/bmm-publisher) | Reproducible extraction (`docker run … yaml` / `split-json`) when comparing publisher output |
| openEHR LANG / RM / AM specification releases | Human-readable authority; BMM JSON is the computable binding |

**SDK v1 primary pins (today):**

| SDK file | Upstream file (same schema id) | Notes |
|---|---|---|
| `openehr_base_1.3.0.bmm.json` | `openehr_base_1.3.0.bmm.json` | Match |
| `openehr_rm_1.2.0.bmm.json` | `openehr_rm_1.2.0.bmm.json` | Match |
| `openehr_am_1.4.0.bmm.json` | `openehr_am_1.4.0.bmm.json` | Match |
| `openehr_am_2.4.0.bmm.json` | `openehr_am_2.4.0.bmm.json` | Deferred — reference pin only |
| `openehr_lang_1.1.0.bmm.json` | `openehr_lang_1.1.0.bmm.json` | Deferred — upstream also ships `openehr_lang_1.1.0-bmm3.bmm.json`; do not import blindly |
| `openehr_term_3.1.0.bmm.json` | `openehr_term_3.1.0.bmm.json` | Deferred |

Upstream additionally carries **older schema ids** (e.g. `openehr_rm_1.1.0`, `openehr_base_1.2.0`) that the SDK does not pin — ignore unless doing historical archaeology.

## Implementation checklist

| Step | Status |
|---|---|
| Upstream sync helper documented / scripted | |
| Baseline checksum manifest for pinned files | |
| First alignment PR (verify current pins = upstream bytes, or land drift) | |
| `resources/bmm/README.md` + `bmm-conformance.md` cross-links | |
| `make ci` after any bump | |

## Phases

### Phase 0 — Baseline audit (no codegen change)

**Outcome:** Know whether current SDK pins are byte-identical to upstream `main`.

**Tasks:**

- [ ] Clone or shallow-fetch [`openEHR/BMM-publisher`](https://github.com/openEHR/BMM-publisher) alongside this repo (sibling under `/src/cadasto/` or temp dir).
- [ ] For each **primary** SDK pin, `cmp` / `sha256sum` against upstream `resources/<file>`.
- [ ] Record results in this plan's PR description (table: file → match / drift / upstream-only newer id).
- [ ] If drift exists on a primary pin without a schema-id bump, treat as **mid-version `schema_revision`** per [`resources/bmm/README.md § Integrity`](../../resources/bmm/README.md#integrity) — replace file, note in CHANGELOG, skip codegen if `bmmdiff` shows no semantic delta.

**Definition of done:** Written audit; no silent mismatch between "byte-identical copies" claim and reality.

### Phase 1 — Upstream import workflow (tooling + docs)

**Outcome:** Any maintainer can run one documented path from upstream file → SDK `resources/bmm/` → ADR 0001 bump.

**Tasks:**

- [ ] Add `scripts/bmm-upstream-sync.sh` (or Makefile target `make bmm-upstream-check`) that:
  - Accepts `BMM_PUBLISHER_ROOT` (default: sibling clone path).
  - Compares checksums for all pinned files.
  - Prints `go run ./cmd/bmmdiff old new` command lines when a newer schema id is staged.
  - Exits non-zero on unexpected drift (CI optional gate).
- [ ] Extend [`resources/bmm/README.md`](../../resources/bmm/README.md) § Provenance with:
  - Upstream repo URL + tagged release policy ("track BMM-publisher releases that correspond to openEHR spec publication").
  - Pointer to this plan + ADR 0001 (procedure stays in ADR; sync discovery stays here).
- [ ] Document optional Docker path:
  ```bash
  docker run --rm -v ./staging:/app/output ghcr.io/openehr/bmm-publisher split-json all
  ```
  for extracting per-type JSON when validating publisher output — not required for SDK codegen, useful when disputing a semantic diff.

**Definition of done:** `make bmm-upstream-check` (or script) runs locally; README points to upstream.

### Phase 2 — Land a version bump (when upstream publishes)

**Outcome:** SDK primary pins move to a new schema id (e.g. `openehr_rm_1.2.1`) with full regen.

**Tasks:**

- [ ] Follow [ADR 0001](../adr/0001-bmm-version-bump-runbook.md) steps 1–11 verbatim.
- [ ] Run `go run ./cmd/bmmdiff <old> <new> -suggest-changelog` and attach summary to PR.
- [ ] Update [`docs/specifications/bmm-conformance.md`](../../specifications/bmm-conformance.md) schema table.
- [ ] Audit `openehr/rm/*_ext.go` companions and SDK-GAP-11 `*Like` interfaces if `bmmdiff` reports Added classes under substitution parents.
- [ ] Run `make codegen-verify`, `make test`, `make ci`.

**Definition of done:** Single PR: old BMM removed, new BMM added, generated tree regen'd, CHANGELOG bullet, conformance table updated.

### Phase 3 — CI guard (optional)

**Outcome:** PRs that hand-edit `_gen.go` or drift from pinned BMM are caught (partially exists via `codegen-verify` + weekly drift bot).

**Tasks:**

- [ ] Evaluate adding `make bmm-upstream-check` to CI as **informational** or **required** once upstream clone is stable in GitHub Actions (may defer — network + submodule policy).
- [ ] Store checksum manifest at `resources/bmm/checksums.txt` (generated, not hand-maintained) if Phase 1 script emits it.

**Defers:** Wiring CI to clone BMM-publisher on every PR — cost/noise trade-off left to maintainer decision.

## Mapping to specs

- [REQ-041](../../specifications/bmm-conformance.md#req-041--pinned-bmm-sources) — pinned sources in `resources/bmm/`
- [REQ-042](../../specifications/bmm-conformance.md#req-042--generated-code-drift-detected) — `make codegen` + `make codegen-verify`
- [ADR 0001](../adr/0001-bmm-version-bump-runbook.md) — execution runbook once inputs are selected
- [`resources/bmm/README.md`](../../resources/bmm/README.md) — file inventory + short update pointer

## Open questions

1. **Lang BMM3 variant** — upstream ships both `openehr_lang_1.1.0.bmm.json` and `openehr_lang_1.1.0-bmm3.bmm.json`. Which is authoritative for future LANG codegen? Track as STRAND candidate if unresolved at first bump.
2. **Release cadence** — align SDK bumps with BMM-publisher git tags vs openEHR specification release announcements. Prefer publisher tag that matches the spec release note.

## References

- Upstream: https://github.com/openEHR/BMM-publisher
- SDK pins: [`resources/bmm/`](../../resources/bmm/)
- Generator: [`internal/bmmgen/`](../../internal/bmmgen/), [`cmd/bmmgen/`](../../cmd/bmmgen/), [`cmd/bmmdiff/`](../../cmd/bmmdiff/)
- Archived codegen plan: [`archive/2026-05-15-bmm-codegen.md`](archive/2026-05-15-bmm-codegen.md)
