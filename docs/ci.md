# Continuous integration

How `openehr-sdk-go` is checked on GitHub and how to reproduce those checks locally. CI is **operational process** — it is not part of the normative `docs/specifications/` contract (wire semantics and conformance probes live there).

## Workflows

| Workflow | File | When it runs |
|---|---|---|
| **CI** | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) | Every pull request; every push to `main` |
| **Codegen drift** | [`.github/workflows/codegen-drift.yml`](../.github/workflows/codegen-drift.yml) | Mondays 06:00 UTC; `workflow_dispatch` |

### CI jobs (`ci.yml`)

Jobs run in parallel. All use Go **1.25.x** (`actions/setup-go@v5` with module cache).

| Job | Makefile targets | Purpose |
|---|---|---|
| **Verify** | `fmt-check`, `mod-tidy-check`, `codegen-verify`, `vet`, `spec-check`, `build` | Static checks and compile-all without running tests |
| **Test** | `test` | Unit tests; `test` already depends on `codegen-verify` |
| **Lint** | (via `golangci-lint-action` v2.11.4, config [`.golangci.yml`](../.golangci.yml)) | Same rules as `make lint` / `make lint-ci` |
| **Race** | `test-race` | **Push to `main` only** — `-race` is slower; catches data races in `typereg` and codecs |

PRs do not run the **Race** job. Merge to `main` triggers it on the post-merge push.

### Codegen drift bot

The weekly workflow re-runs `make codegen-verify` on a clean checkout. On failure it opens or comments on a single tracking issue labelled `bmm-drift`, then fails the workflow run. Follow [ADR 0001 — BMM version-bump runbook](adr/0001-bmm-version-bump-runbook.md) when triaging.

This complements PR CI: it catches generator-template drift between human-driven PRs.

## Local reproduction

```bash
make doctor    # toolchain diagnosis
make ci        # full PR gate (fmt, mod tidy, vet, test, lint, build)
make test-race # optional; matches main-branch Race job
```

Run `make help` for the full grouped list. Common targets:

| Group | Target | What it does |
|---|---|---|
| CI | `make ci` | Full PR gate (fmt, mod tidy, vet, test, lint, spec-check, build) |
| Test | `make test` | Unit tests; depends on `codegen-verify` |
| Test | `make test-race` | `-race` detector (main-branch job only) |
| Format | `make fmt-check` | Fail if `gofmt -s` would change any file |
| Modules | `make mod-tidy-check` | Fail if `go mod tidy` would change `go.mod` / `go.sum` |
| Codegen | `make codegen-verify` | BMM-generated tree matches `resources/bmm/` |
| Specs | `make spec-check` | `docs/specifications/traceability.yaml` paths and probes match the tree |
| Lint | `make lint` | `golangci-lint` on host if installed, else Docker (`LINT_IMAGE`) |

**Policy:** extend the [Makefile](../Makefile), not ad-hoc shell in workflows. CI and contributors share the same entry points ([AGENTS.md](../AGENTS.md) Tooling policy).

### Lint configuration

- Config: [`.golangci.yml`](../.golangci.yml)
- Pin: `golangci/golangci-lint:v2.11.4` (Makefile `LINT_IMAGE` and GitHub Action `version`)
- Generated `*_gen.go`, `*_jsonmar_gen.go`, `*_jsonunmar_gen.go` have relaxed duplicate/complexity rules

## Recommended branch protection (`main`)

| Check | Required for merge? |
|---|---|
| Verify | Yes |
| Test | Yes |
| Lint | Yes |
| Race | Optional (informational until the team promotes it) |

Also enable **Require branches to be up to date before merging** when PR volume warrants it.

## Dependency updates

[`.github/dependabot.yml`](../.github/dependabot.yml) opens weekly PRs for `go.mod` and GitHub Actions version bumps.

## Future CI (not yet wired)

| Check | When |
|---|---|
| `govulncheck ./...` | Before v1.0.0 or when non-stdlib deps land |
| Conformance probe runner (`testkit/probes/…`) | Dedicated job when live-backend modes are needed |
| Release on tag | When versioned module publishes are routine |

## See also

- [docs/ai-workflow.md](ai-workflow.md) — agent pre-merge checklist
- [resources/README.md](../resources/README.md) — BMM pin and update procedure
- [docs/specifications/conformance.md](../docs/specifications/conformance.md) — PROBE-NNN definitions (tests run via `make test` today)
