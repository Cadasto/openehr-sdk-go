# Continuous integration

How `openehr-sdk-go` is checked on GitHub and how to reproduce those checks locally. CI is **operational process** — it is not part of the normative `docs/specifications/` contract (wire semantics and conformance probes live there).

## Workflows

| Workflow | File | When it runs |
|---|---|---|
| **CI** | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) | Every pull request; every push to `main` |
| **CodeQL** | [`.github/workflows/codeql.yml`](../.github/workflows/codeql.yml) | Every pull request; every push to `main`; Tuesdays 06:00 UTC. Findings appear under **Security → Code scanning** |
| **Codegen drift** | [`.github/workflows/codegen-drift.yml`](../.github/workflows/codegen-drift.yml) | Mondays 06:00 UTC; `workflow_dispatch` |
| **Release** | [`.github/workflows/release.yml`](../.github/workflows/release.yml) | Push of a `v*` tag; `workflow_dispatch` dry-run. Process in [releases.md](releases.md) |

### CI jobs (`ci.yml`)

Jobs run in parallel. All use Go **1.27.x** (`actions/setup-go@v7` with module cache).

| Job | Makefile targets | Purpose |
|---|---|---|
| **Verify** | `fmt-check`, `mod-tidy-check`, `codegen-verify`, `aqlgen-verify`, `vet`, `spec-check`, `flat-conformance-verify`, `build` | Static checks and compile-all without running tests |
| **Test** | `test` | Unit tests; `test` already depends on `codegen-verify` and `aqlgen-verify` |
| **Lint** | (via `golangci-lint-action` v2.13.2, config [`.golangci.yml`](../.golangci.yml)) | Same rules as `make lint` / `make lint-ci` |
| **Race** | `test-race` | **Push to `main` only** — `-race` is slower; catches data races in `typereg` and codecs |

PRs do not run the **Race** job. Merge to `main` triggers it on the post-merge push.

### CodeQL (`codeql.yml`)

GitHub's CodeQL analysis for Go, with `build-mode: autobuild`. Go 1.27.x is set up before CodeQL initialises so autobuild can compile the `go 1.27.0` module. It runs on the same PR and `main` events as CI, plus a weekly schedule that re-checks unchanged code against newly released queries.

### Codegen drift bot

The weekly workflow re-runs `make codegen-verify` on a clean checkout. On failure it opens or comments on a single tracking issue labelled `bmm-drift`, then fails the workflow run. Follow [ADR 0001 — BMM version-bump runbook](adr/0001-bmm-version-bump-runbook.md) when triaging.

This complements PR CI: it catches generator-template drift between human-driven PRs.

## Local reproduction

```bash
make doctor    # toolchain diagnosis
make ci        # full PR gate (fmt, mod tidy, vet, test, lint, spec-check, fixture integrity, build)
make test-race # optional; matches main-branch Race job
```

Run `make help` for the full grouped list. Common targets:

| Group | Target | What it does |
|---|---|---|
| CI | `make ci` | Full PR gate (fmt, mod tidy, vet, test, lint, spec-check, flat-conformance-verify, build) |
| Test | `make test` | Unit tests; depends on `codegen-verify` |
| Test | `make test-race` | `-race` detector (main-branch job only) |
| Format | `make fmt-check` | Fail if `golangci-lint fmt --diff` (gofumpt + goimports) would change any file |
| Modules | `make mod-tidy-check` | Fail if `go mod tidy` would change `go.mod` / `go.sum` |
| Codegen | `make codegen-verify` | BMM-generated tree matches `resources/bmm/` |
| Codegen | `make aqlgen-verify` | Committed AQL parser matches `resources/aql/grammar/active/` (needs Docker) |
| Specs | `make spec-check` | `docs/specifications/traceability.yaml` paths and probes match the tree |
| Fixtures | `make flat-conformance-verify` | Offline `sha256` integrity of the pinned upstream FLAT corpus against its `MANIFEST.txt`; no network and no `curl`/`jq` needed, so it is safe in the gate. Catches a hand-edit to a vendored fixture whose whole value is being byte-identical to upstream |
| Fixtures | `make flat-conformance-check` | The above **plus** a best-effort upstream-drift report (needs network; degrades with a note when offline). Dev helper, not a CI gate |
| Specs | `make spec-context REQ=NNN` | Assemble the SDD context bundle for a REQ (dev/agent helper; not a CI gate) |
| Specs | `make probe-status` | Each PROBE's status and whether its test file exists (dev helper; not a CI gate) |
| Lint | `make lint` | `golangci-lint` on host if the binary was built with Go 1.27, else Docker (`LINT_IMAGE`) |

**Policy:** extend the [Makefile](../Makefile), not ad-hoc shell in workflows. CI and contributors share the same entry points ([AGENTS.md](../AGENTS.md) Tooling policy).

### Lint configuration

- Config: [`.golangci.yml`](../.golangci.yml)
- Pin: `golangci/golangci-lint:v2.13.2` (Makefile `LINT_IMAGE` and GitHub Action `version`). Official release binaries are built with Go 1.27; a host `go install` from an older toolchain cannot load this module (golangci-lint refuses when its build Go is below the `go.mod` floor). `make lint` / `make fmt-check` then fall back to the pinned image.
- Generated files (the `// Code generated … DO NOT EDIT.` set) are skipped via `exclusions: generated: lax` in `.golangci.yml`

## Dependency updates

[`.github/dependabot.yml`](../.github/dependabot.yml) opens weekly PRs for `go.mod` and GitHub Actions version bumps.

## Future CI (not yet wired)

| Check | When |
|---|---|
| `govulncheck ./...` | Before v1.0.0 or when non-stdlib deps land |
| Conformance probe runner (`testkit/probes/…`) | Dedicated job when live-backend modes are needed |

## See also

- [docs/releases.md](releases.md) — version policy, tag checklist, `v1.0.0` gate
- [docs/ai-workflow.md](ai-workflow.md) — agent pre-merge checklist
- [resources/README.md](../resources/README.md) — BMM pin and update procedure
- [docs/specifications/conformance.md](../docs/specifications/conformance.md) — PROBE-NNN definitions (tests run via `make test` today)
