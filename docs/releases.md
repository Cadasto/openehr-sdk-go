---
kind: guide
---

# Releases

How `github.com/cadasto/openehr-sdk-go` is versioned, tagged, and announced. Quality gate before any tag: [`docs/ci.md`](ci.md).

## Versioning

[SemVer 2.0.0](https://semver.org/spec/v2.0.0.html) ([REQ-004](specifications/packaging.md#req-004--semantic-versioning)). The git tag (`vX.Y.Z`) is the single authoritative version. `go.mod` carries only the Go language version, never the SDK's own semver, and there is no runtime `version` package. Consumers pin:

```bash
go get github.com/cadasto/openehr-sdk-go@vX.Y.Z
```

Standard SemVer applies: a breaking change is a major bump, an addition a minor one, a fix a patch. The full change-to-bump table, including the cases specific to this SDK (BMM bumps, the minimum Go version, the module path, tightened validation), is in [`module-layout.md` § Versioning](specifications/module-layout.md#versioning).

### Four version concepts

The repo pins four versions independently, and the git tag tracks only the first. The rest ship as a compatibility table in each release's notes. [`scripts/release-notes.sh`](../scripts/release-notes.sh) generates that table from `go.mod`, `resources/bmm/`, and the git SHA, so nothing here needs updating by hand.

| Concept | Pin location | Bumps when |
|---|---|---|
| **SDK semver** | git tag `vX.Y.Z` | every release |
| Go toolchain (minimum) | `go.mod` `go` line | a new stable Go minor line ([§ Minimum Go version](#minimum-go-version)) |
| openEHR REST | [REQ-050](specifications/wire.md#req-050) → `1.1.0` | spec bump; discovery mismatch fails fast |
| BMM corpus | `resources/bmm/*.bmm.json` | [ADR 0001](adr/0001-bmm-version-bump-runbook.md) |

### Minimum Go version

The `go.mod` `go` directive names the current stable Go minor line at its `.0` patch, for example `go 1.27.0`. It states the minimum Go the module needs, not a patch to build with. A Go patch release adds no language or standard-library API, so a mid-line patch floor buys nothing, and it would make every consumer, linter image and CI runner on an earlier patch of the same minor fetch a newer toolchain (`GOTOOLCHAIN=auto`), which breaks offline and air-gapped builds. When a newer Go minor line becomes stable, the directive moves to that line's `.0` patch in a minor SDK release ([`module-layout.md` § Versioning](specifications/module-layout.md#versioning)). It never names a mid-line patch or a pre-release (rc or beta) toolchain. Dev-image pins in the [Dockerfile](../Dockerfile) move independently of it.

### Pre-1.0

The SDK tags `v0.x` from the first usable adopter slice rather than waiting for `v1.0.0`. Without a tag, adopters would pin pseudo-versions for months; an alpha/beta ladder was weighed and left optional (see [Pre-releases & hotfixes](#pre-releases--hotfixes)). `v1.0.0` stays reserved for the gate below.

While on `v0.x`, minor bumps may break the public API (release notes list every break) and patch bumps stay compatible; the rule is in [`module-layout.md` § Versioning](specifications/module-layout.md#versioning). Pin an exact tag and read the notes before upgrading a minor.

### `v1.0.0` gate

Cut when all three hold ([`module-layout.md` § Versioning](specifications/module-layout.md#versioning)):

1. Every REQ in [`REQ.md`](specifications/REQ.md) is `Impl. landed` or `retired`, with no `partial` and no `planned`. (`retired` is terminal: a retired requirement carries no active rule and will never land, so a gate demanding `landed` for every row could not be met.)
2. The openEHR wire-conformance probe suite in [`conformance.md`](specifications/conformance.md) passes.
3. A reference openEHR deployment passes the live probe suite ([REQ-082](specifications/conformance.md#req-082--runnability)).

Promoting each spec file `Status: Draft → Stable` then happens as part of the `v1.0.0` cut. It is not a fourth precondition. `Status:` is a per-**file** stability promise ([README.md § Status header](specifications/README.md#status-header)). Whether a requirement is *built* is recorded in the per-REQ `Impl.` column, which condition 1 checks.

Until then the project ships `v0.x` adopter slices. Current progress is in [`docs/roadmap.md`](roadmap.md).

## Release process

### Tag checklist

1. **CI green on `main`.** Run `make ci` (the same gate as [`ci.yml`](../.github/workflows/ci.yml)).
2. **CHANGELOG.** Rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and open a fresh `## [Unreleased]` above it. Keep it terse: one-sentence bullets and a one-sentence summary, per [§ CHANGELOG](#changelog). The release notes are copied verbatim from this block, so trim *here*, not in the GitHub draft. Commit this (with step 3) **directly to `main`**, with no branch or PR for a release bump (see [Branch & tag policy](#branch--tag-policy)). The same commit bumps the `go get …@vX.Y.Z` line on [`pages/install.md`](../pages/install.md), [`pages/index.md`](../pages/index.md), the [README](../README.md) and [`docs/quick-start.md`](quick-start.md). `make docs-check` reads the release straight out of this CHANGELOG heading and fails until all four name it.
3. **Roadmap.** Bump [`docs/roadmap.md`](roadmap.md) if the release crosses a milestone.
4. **Annotated tag from `main`.** Create it and push:
   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z" && git push origin vX.Y.Z
   ```
5. **Release workflow.** [`release.yml`](../.github/workflows/release.yml) fires on the `v*` tag. It re-runs `make ci` on the tagged commit, regenerates notes via [`scripts/release-notes.sh`](../scripts/release-notes.sh) (CHANGELOG block + auto compatibility table), and creates a **draft** Release. It never publishes on its own, and if CI fails it creates no draft.
6. **Publish.** Review the draft on the [Releases page](https://github.com/Cadasto/openehr-sdk-go/releases), edit if needed, and click **Publish release**.

Preview notes locally without side effects: `bash scripts/release-notes.sh X.Y.Z`, or the workflow's `workflow_dispatch` dry-run.

### CHANGELOG

[`CHANGELOG.md`](../CHANGELOG.md) is updated only on request or when cutting a release. **One single-sentence bullet (~35 words max) per artefact class**: artefact + scope + key REQ/PROBE, never API inventories or per-REQ breakdowns (those live in `traceability.yaml`, commits, and PR bodies); the optional per-release summary line is one sentence too, plus a `**Breaking:**` sentence naming each change [`module-layout.md` § Versioning](specifications/module-layout.md#versioning) maps to major. Release notes are generated verbatim from the block, so a long entry is a defect — err short. Sections, in this order and only when they have entries: `### Added` for new capability; `### Changed` for behaviour a caller may have to adapt to (a tightened check, a different output, a different error); `### Deprecated` and `### Removed` for public symbols. No `### Fixed`: a fix of unreleased work folds into its bullet, and a fix of released behaviour goes under `### Changed` when callers may have to adapt, otherwise into its artefact class's `### Added` bullet.

### Pre-releases & hotfixes

- **Pre-release:** optional `vX.Y.Z-rc.N` tag; `go get @vX.Y.Z-rc.1` selects it explicitly.
- **Hotfix:** patch from `main` if releasable; otherwise cherry-pick to a `release/v0.x` branch. Pre-1.0 support is best-effort, and long-lived `release/*` branches are a `v1.x` concern.
- **Cadence:** `v0.x` is on-demand, milestone-driven.

## Branch & tag policy

- `main` is always releasable after CI. Tag **only** from `main` (or a `release/v0.x` hotfix branch).
- Only maintainers push tags, and branch protection on `main` enforces this.
- Tags are annotated, not signed. Tag signing (Sigstore or GPG) is deferred: a candidate for the `v1.0.0` cut, not one of its gates.
- **Substantive work** (features, fixes, docs of record) lands via branch + PR. A maintainer commits **mechanical release bookkeeping** directly to `main`, with no branch or PR. That covers the version-bump CHANGELOG cut (steps 2 and 3 above) and any milestone roadmap bump. If branch protection ever rejects a direct push, stop and report it. Do not quietly route the bump through a PR instead.

## References

- [REQ-004](specifications/packaging.md#req-004--semantic-versioning): semantic versioning
- [`module-layout.md` § Versioning](specifications/module-layout.md#versioning): bump matrix + `v1.0.0` gates
- [ADR 0001](adr/0001-bmm-version-bump-runbook.md): BMM bumps vs codegen
- [`docs/ci.md`](ci.md): quality gate before tag
- Release tooling: commit [0ee78296](https://github.com/Cadasto/openehr-sdk-go/commit/0ee78296) (the tag-driven release workflow and notes script)
- Go modules: [version numbering](https://go.dev/doc/modules/version-numbers)
