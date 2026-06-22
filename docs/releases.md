# Releases

How `github.com/cadasto/openehr-sdk-go` is versioned, tagged, and announced. Quality gate before any tag: [`docs/ci.md`](ci.md). Landed reasoning: [versioning-strategy plan](plans/archive/2026-05-25-versioning-strategy.md) (archived).

## Versioning

[SemVer 2.0.0](https://semver.org/spec/v2.0.0.html) ([REQ-004](specifications/packaging.md#req-004--semantic-versioning)). The **git tag** (`vX.Y.Z`) is the single authoritative version — `go.mod` carries only the Go language version, never the SDK's own semver, and there is no runtime `version` package. Consumers pin:

```bash
go get github.com/cadasto/openehr-sdk-go@vX.Y.Z
```

Standard SemVer applies (breaking → major, additive → minor, fix → patch). The cases worth spelling out because they're specific to this SDK:

| Change | Bump |
|---|---|
| BMM bump that changes generated public types (verify: `make codegen-verify` + `bmmdiff`) | Minor |
| BMM bump with no public type change | Patch |
| `go.mod` minimum Go version raise | Minor (REQ-002) |
| Module path change | Major + `/vN` import path (REQ-001) |

### Four version concepts

The repo pins four versions independently; the git tag tracks only the first. The rest ship as a compatibility table in each release's notes, auto-generated from `go.mod`, `resources/bmm/`, and the git SHA by [`scripts/release-notes.sh`](../scripts/release-notes.sh) — so nothing here needs hand-updating.

| Concept | Pin location | Bumps when |
|---|---|---|
| **SDK semver** | git tag `vX.Y.Z` | every release |
| Go toolchain (minimum) | `go.mod` `go` line | N-1 release line (REQ-002) |
| openEHR REST | [REQ-050](specifications/wire.md#req-050) → `1.1.0-development` | spec bump; discovery mismatch fails fast |
| BMM corpus | `resources/bmm/*.bmm.json` | [ADR 0001](adr/0001-bmm-version-bump-runbook.md) |

### Pre-1.0

While on `v0.x`: **minor** bumps may break the public API (release notes list every break); **patch** bumps stay compatible. Pin an exact tag and read the notes before upgrading a minor.

### `v1.0.0` gate

Cut when all three hold ([`module-layout.md` § Versioning](specifications/module-layout.md#versioning)):

1. All REQs in [`REQ.md`](specifications/REQ.md) at `Status: Stable`.
2. The openEHR wire-conformance probe suite passes ([REQ-080](specifications/conformance.md#req-080--openehr-wire-conformance)).
3. A reference openEHR deployment passes the live probe suite ([REQ-082](specifications/conformance.md#req-082--runnability)).

Until then we ship `v0.x` adopter slices; current progress is in [`docs/roadmap.md`](roadmap.md).

## Release process

### Tag checklist

1. **CI green on `main`** — `make ci` (the same gate as [`ci.yml`](../.github/workflows/ci.yml)).
2. **CHANGELOG** — rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD`; open a fresh `## [Unreleased]` above it. Keep it terse — one-sentence bullets and a one-sentence summary per the [CHANGELOG brevity rule](../CHANGELOG.md); notes are generated verbatim from this block, so trim *here*, not in the GitHub draft. Commit this (with step 3) **directly to `main`** — no branch or PR for a release bump (see [Branch & tag policy](#branch--tag-policy)).
3. **Roadmap** — bump [`docs/roadmap.md`](roadmap.md) if the release crosses a milestone.
4. **Annotated tag from `main`** and push:
   ```bash
   git tag -a vX.Y.Z -m "Release vX.Y.Z" && git push origin vX.Y.Z
   ```
5. **Release workflow** — [`release.yml`](../.github/workflows/release.yml) fires on the `v*` tag: re-runs `make ci` on the tagged commit, regenerates notes via [`scripts/release-notes.sh`](../scripts/release-notes.sh) (CHANGELOG block + auto compatibility table), and creates a **draft** Release. It never auto-publishes; if CI fails, no draft is created.
6. **Publish** — review the draft on the [Releases page](https://github.com/Cadasto/openehr-sdk-go/releases), edit if needed, click **Publish release**.

Preview notes locally without side effects: `bash scripts/release-notes.sh X.Y.Z`, or the workflow's `workflow_dispatch` dry-run.

### Pre-releases & hotfixes

- **Pre-release:** optional `vX.Y.Z-rc.N` tag; `go get @vX.Y.Z-rc.1` selects it explicitly.
- **Hotfix:** patch from `main` if releasable; otherwise cherry-pick to a `release/v0.x` branch — pre-1.0 support is best-effort, and long-lived `release/*` branches are a `v1.x` concern.
- **Cadence:** `v0.x` is on-demand, milestone-driven.

## Branch & tag policy

- `main` is always releasable after CI; tag **only** from `main` (or a `release/v0.x` hotfix branch).
- Tags are pushed by maintainers only; branch protection on `main` is the enforcement.
- **Substantive work** (features, fixes, docs of record) lands via branch + PR. **Mechanical release bookkeeping** — the version-bump CHANGELOG cut (steps 2–3 above) and any milestone roadmap bump — is committed **directly to `main`** by a maintainer; no branch or PR detour. If a direct push is ever rejected by branch protection, stop and surface it rather than silently routing the bump through a PR.

## References

- [REQ-004](specifications/packaging.md#req-004--semantic-versioning) — semantic versioning
- [`module-layout.md` § Versioning](specifications/module-layout.md#versioning) — bump matrix + `v1.0.0` gates
- [ADR 0001](adr/0001-bmm-version-bump-runbook.md) — BMM bumps vs codegen
- [`docs/ci.md`](ci.md) — quality gate before tag
- [versioning-strategy plan](plans/archive/2026-05-25-versioning-strategy.md) (archived)
- Go modules: [version numbering](https://go.dev/doc/modules/version-numbers)
