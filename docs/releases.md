# Releases

How `github.com/cadasto/openehr-sdk-go` is versioned, tagged, and announced. Companion to the [versioning-strategy plan](plans/2026-05-25-versioning-strategy.md) (normative reasoning) and [`docs/ci.md`](ci.md) (quality gate before tag).

## Versioning policy

[Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html), per [REQ-004](specifications/packaging.md#req-004--semantic-versioning).

The git tag (`v0.1.0`, `v0.2.0`, …) is the **single authoritative source** of the SDK version. Consumers pin via:

```bash
go get github.com/cadasto/openehr-sdk-go@v0.1.0
```

`go.mod` does **not** carry the SDK's own semver — only the Go language version and dependency pins.

### Four version concepts (only one is "the version")

The repo carries four version concepts; the git tag tracks **only the first**. The others ship as compatibility metadata in the release notes.

| Concept | What it pins | Pin location | Bumps when |
|---|---|---|---|
| **SDK semver** | Public API + wire behaviour | git tag `vX.Y.Z` | Every release |
| Go toolchain | Minimum Go compiler | `go.mod` `go 1.25.0` | N-1 release line (REQ-002) |
| openEHR REST | Wire contract | [REQ-050](specifications/wire.md#req-050) → `1.1.0-development` | Spec bump; discovery mismatch fails fast |
| BMM corpus | Generated RM/AOM shapes | `resources/bmm/openehr_*.bmm.json` | [ADR 0001](adr/0001-bmm-version-bump-runbook.md) (may force minor) |

### Bump matrix

| Change | Bump | Notes |
|---|---|---|
| Breaking change to any exported symbol outside `internal/` | **Major** | Renamed types, removed funcs, changed error semantics |
| New exported package, func, type; spec promotion to Stable | **Minor** | During `v0.x`, may still surface in release notes as a break |
| Bug fix preserving public contracts | **Patch** | |
| Only `internal/`, tests, docs, CI | **Patch** (or no tag) | |
| BMM bump that changes generated public types | **Minor** | Verify via `make codegen-verify` + `bmmdiff` |
| BMM bump with no public type change | **Patch** | |
| `go.mod` minimum Go version raise | **Minor** | REQ-002 |
| Module path change | **Major** + `/v2` import path | REQ-001 |

### Pre-1.0 (`v0.x`) policy

We stay on `v0.x` until the [`v1.0.0` gate](#v100-gate). Per SemVer 2.0, `v0.y.z` allows breaking changes on **minor** `y` — we follow that strictly:

- **`v0.x.y` minor (`y` bumps)**: may break public API. Release notes explicitly list any break.
- **`v0.x.y` patch (`z` bumps)**: always backwards-compatible.
- **`v1.0.0`**: ceremonial — no breaking changes thereafter without a major bump.

Consumers in `v0.x` should pin to a specific tag and read release notes before upgrading minors.

### `v1.0.0` gate

`v1.0.0` lands when **all three** of these hold ([`module-layout.md` § Versioning](specifications/module-layout.md#versioning)):

1. All REQs in [`REQ.md`](specifications/REQ.md) at `Status: Stable` (no remaining `Draft`).
2. Probe parity with the PHP SDK ([REQ-080](specifications/conformance.md#req-080--probe-parity) / [REQ-081](specifications/conformance.md#req-081--wire-level-parity-not-source-level)).
3. A reference Cadasto deployment passes the live probe set ([REQ-082](specifications/conformance.md#req-082--runnability) — Live mode).

Today (2026-05): large surface is **landed** (transport, clients, codecs, template/validation stack, instance generator, composition builder), but the REQ registry is largely **Draft** and probe ratification is open. `v1.0.0` is not honest yet — we cut `v0.1.0` instead as the first adopter slice.

## Release process

### Tag checklist (per release)

1. **CI green on `main`** — `make ci` passes locally (`fmt-check`, `mod-tidy-check`, `codegen-verify`, `vet`, `spec-check`, `test`, `lint`, `build`). The same set is enforced by [`.github/workflows/ci.yml`](../.github/workflows/ci.yml).
2. **CHANGELOG cut** — rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`; open a fresh `## [Unreleased]` above it.
3. **Roadmap milestone** — bump [`docs/roadmap.md`](roadmap.md) if the release crosses a milestone boundary (e.g. Phase 2 closeout).
4. **Preview release notes locally** (optional but recommended):
   ```bash
   bash scripts/release-notes.sh 0.1.0   # prints what the workflow will draft
   ```
   The script extracts the matching `## [X.Y.Z]` block from `CHANGELOG.md` and appends an auto-generated compatibility table (SDK semver, Go minimum, openEHR REST, BMM pins, git revision). For a no-side-effects preview against a future tag, use the workflow's `workflow_dispatch` dry-run from the Actions tab.
5. **Annotated tag** from `main`:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```
   Lightweight tags work but annotated tags carry the release date + author and integrate cleanly with `git describe` and Go module info.
6. **Release workflow runs** — [`.github/workflows/release.yml`](../.github/workflows/release.yml) fires on the `v*` tag push: re-runs `make ci` on the tagged commit, regenerates release notes via `scripts/release-notes.sh`, and creates a **draft** GitHub Release. The workflow does **not** auto-publish — the maintainer reviews and clicks publish manually. If CI fails, the draft is not created and the tag stands without a release until the issue is fixed (delete and re-tag, or push a patch).
7. **Publish the draft** — open the [Releases page](https://github.com/Cadasto/openehr-sdk-go/releases), verify the notes, edit if needed, click **Publish release**.
8. **Announce** — link release notes from the first consumer (and any other in-flight adopter).

### Compatibility metadata (release notes)

Every release notes section ends with a small table identifying the four version concepts:

| Concept | Value at release |
|---|---|
| SDK semver | `v0.1.0` |
| Go toolchain (minimum) | `1.25.0` |
| openEHR REST | `1.1.0-development` |
| BMM corpus | `openehr_base_1.3.0`, `openehr_rm_1.2.0`, `openehr_am_1.4.0`, `openehr_am_2.4.0`, `openehr_lang_1.1.0`, `openehr_term_3.1.0` |
| Git revision | `<short SHA>` |

Reading the BMM pins: `ls resources/bmm/*.bmm.json` — that's the authoritative list. The release workflow ([`release.yml`](../.github/workflows/release.yml)) regenerates this table automatically from `go.mod`, `resources/bmm/`, and `git rev-parse --short HEAD` via [`scripts/release-notes.sh`](../scripts/release-notes.sh), so the CHANGELOG section itself only needs to carry the human narrative.

### Pre-releases

Optional `vX.Y.Z-rc.N` tags before a meaningful minor / major. Standard Go tooling: `go get @v0.2.0-rc.1` selects the pre-release explicitly. No CI rule changes needed.

### Hotfixes

`v0.1.1` (patch) cut from `main` if `main` is acceptable. If `main` has diverged with breaking changes, cherry-pick to a `release/v0.1` branch — but the pre-1.0 support promise is **best-effort only**. Long-lived `release/*` branches are reserved for the `v1.x` era.

### Cadence

- **`v0.x`**: on-demand, milestone-driven. No fixed schedule.
- **`v1.x` and beyond**: TBD when we get there; expect a monthly patch cadence once consumers exist.

## Branch policy

- `main` is always releasable after CI passes.
- Tag **only** from `main` (or a `release/v0.x` branch in the hotfix exception case).
- Branch protection on `main` is enforced via GitHub settings — see [`docs/ci.md`](ci.md#branch-protection).

## Alignment with the PHP SDK

Wire parity (REQ-080 / REQ-081) is the coupling point, NOT the tag. The Go and PHP SDKs version independently:

- Cross-SDK probes (`PROBE-NNN`) define the contract.
- "Compatible with PHP SDK ≥X" appears in release notes once PHP releases.
- No lockstep tagging unless explicitly agreed for marketing reasons.

The Cadasto platform itself versions independently; the openEHR REST `spec_version` returned by discovery is the runtime check.

## Open questions

| Question | Current answer |
|---|---|
| Hand-edit a version constant vs runtime introspection from git tag? | **Neither for v0.1.0** — the git tag IS the version. Runtime introspection via a `version` package is a deferred follow-up, only added when a consumer actually asks for `version.String()`. |
| Sign tags (Sigstore / GPG)? | Nice-to-have for `v1.0.0`; not blocking `v0.1.0`. |
| Who may push tags? | Maintainers only. Branch protection on `main` is the enforcement; tag pushes go through whoever holds repo-write. |

## References

- [REQ-004](specifications/packaging.md#req-004--semantic-versioning) — semantic versioning
- [`module-layout.md` § Versioning](specifications/module-layout.md#versioning) — bump matrix, `v1.0.0` gates
- [`CHANGELOG.md`](../CHANGELOG.md) — pre-1.0 policy
- [`docs/ci.md`](ci.md) — quality gate before tag
- [ADR 0001](adr/0001-bmm-version-bump-runbook.md) — BMM bumps vs codegen
- [`docs/plans/2026-05-25-versioning-strategy.md`](plans/2026-05-25-versioning-strategy.md) — full plan
- Go modules: [Module version numbering](https://go.dev/doc/modules/version-numbers)
