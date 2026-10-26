---
kind: guide
---

# Continuous integration

How `openehr-sdk-go` is checked on GitHub and how to reproduce those checks locally. CI is operational process. It is not part of the normative `docs/specifications/` contract, which holds the wire semantics and conformance probes.

## Workflows

| Workflow | File | When it runs |
|---|---|---|
| **CI** | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) | Every pull request; every push to `main` |
| **CodeQL** | GitHub [default setup](https://docs.github.com/en/code-security/code-scanning/enabling-code-scanning/configuring-default-setup-for-code-scanning) (no in-repo workflow) | Every pull request and every push to `main` (plus GitHub's periodic re-scan), for Go and Actions. Findings appear under **Security → Code scanning** |
| **Codegen drift** | [`.github/workflows/codegen-drift.yml`](../.github/workflows/codegen-drift.yml) | Mondays 06:00 UTC; `workflow_dispatch` |
| **Release** | [`.github/workflows/release.yml`](../.github/workflows/release.yml) | Push of a `v*` tag; `workflow_dispatch` dry-run. Process in [releases.md](releases.md) |
| **Docs CI** | [`.github/workflows/docs-ci.yml`](../.github/workflows/docs-ci.yml) | Every pull request into `main`; `workflow_dispatch`. Runs `make docs-check` |
| **Docs site** | [`.github/workflows/docs-site.yml`](../.github/workflows/docs-site.yml) | Push to `main`; Mondays 06:00 UTC; `workflow_dispatch`. Builds the MkDocs site and deploys it to GitHub Pages once the repository's Pages source is set to GitHub Actions, which is not enabled yet (as of 2026-09-10) |

### CI jobs (`ci.yml`)

Jobs run in parallel. All use Go 1.27.x (`actions/setup-go@v7` with module cache).

| Job | Makefile targets | Purpose |
|---|---|---|
| **Verify** | `fmt-check`, `mod-tidy-check`, `codegen-check`, `aqlgen-check`, `termgen-check`, `vet`, `spec-check`, `bmm-check`, `its-xml-check`, `flat-check`, `corpus-pins-check`, `terminology-check`, `build` | Static checks and compile-all without running tests |
| **Test** | `test` | Unit tests; `test` already depends on `codegen-check`, `aqlgen-check` and `termgen-check` |
| **Lint** | (via `golangci-lint-action` v2.14.0, config [`.golangci.yml`](../.golangci.yml)) | Same rules as `make lint` |
| **Race** | `test-race` | **Push to `main` only**, because `-race` is slower. Catches data races in `typereg` and codecs. The runner has a C compiler, so the race run uses the host `go` |

PRs do not run the **Race** job. Merge to `main` triggers it on the post-merge push.

### CodeQL (default setup)

CodeQL runs through GitHub's [default setup](https://docs.github.com/en/code-security/code-scanning/enabling-code-scanning/configuring-default-setup-for-code-scanning), enabled in **Settings → Code security**, not from an in-repo workflow. It scans Go and GitHub Actions on every PR and every push to `main`, and its extractor handles the `go 1.27.0` module. Results land under **Security → Code scanning**.

An in-repo advanced workflow was tried and dropped: an advanced configuration and the default setup cannot both publish results, and the default setup already covers Go and Actions. If per-repo control is ever needed, disable the default setup first, then add a pinned `github/codeql-action` workflow that keeps the `actions` language alongside `go`.

### Codegen drift bot

The weekly workflow re-runs `make codegen-check` on a clean checkout. On failure it opens or comments on a single tracking issue labelled `bmm-drift`, then fails the workflow run. Follow [ADR 0001 (BMM version-bump runbook)](adr/0001-bmm-version-bump-runbook.md) when triaging. The workflow complements PR CI by catching generator-template drift between human-driven PRs.

## Local reproduction

```bash
make doctor      # toolchain diagnosis
make ci          # full PR gate (fmt, mod tidy, vet, test, lint, spec-check, fixture integrity, build)
make test-race   # optional; matches main-branch Race job (needs a C compiler, else Docker)
make docs-serve  # documentation site on http://127.0.0.1:8000 (needs Docker)
make docs-check  # strict MkDocs build plus the published-output assertions
```

Run `make help` for the full grouped list. Common targets:

| Group | Target | What it does |
|---|---|---|
| CI | `make ci` | Full PR gate (fmt, mod tidy, vet, test, lint, spec-check, bmm-check, its-xml-check, flat-check, corpus-pins-check, terminology-check, build) |
| Test | `make test` | Unit tests; depends on `codegen-check`, `aqlgen-check` and `termgen-check` |
| Test | `make test-race` | `-race` detector (main-branch job only). The detector needs cgo, so it runs on host Go 1.27.x only when the C compiler `go env CC` names is on `PATH`. Otherwise it runs in the dev container with cgo turned on for that run only (`make build-dev` builds the image) |
| Format | `make fmt-check` | Fail if `golangci-lint fmt --diff` (gofumpt + goimports) would change any file |
| Modules | `make mod-tidy-check` | Fail if `go mod tidy` would change `go.mod` / `go.sum` |
| Codegen | `make codegen-check` | BMM-generated tree matches `resources/bmm/` |
| Codegen | `make aqlgen-check` | Committed AQL parser matches `resources/aql/grammar/active/` (needs Docker) |
| Codegen | `make termgen-check` | `openehr/terminology/openehr_gen.go` matches the pinned `resources/terminology/openehr_terminology.xml`, `openehr_external_terminologies.xml` and `PropertyUnitData.xml`; regenerate with `make termgen` |
| Specs | `make spec-check` | `docs/specifications/traceability.yaml` paths and probes match the tree, and the generated REQ.md registry and `tests:` lists are current; also runs the vendored `sdd-check` gate (`scripts/sdd-check.py`) |
| Specs | `make sdd-check` | Prints the vendored gate's full report, warnings included (`spec-check` runs it too) |
| Specs | `make spec-generate` | Regenerate the REQ.md registry and the map's `tests:` lists (not a gate; `spec-check` fails until it is run) |
| Docs | `make docs-check` | Strict MkDocs build plus published-output assertions (needs Docker; fetches docs-theme). The `go get` tag it asserts on the install and landing pages is read from the first release heading in [`CHANGELOG.md`](../CHANGELOG.md); `DOCS_SYNC=docs-sync-offline` builds from the cached brand layer instead of refetching |
| Docs | `make docs-serve` | Live preview on `http://127.0.0.1:8000` |
| Fixtures | `make flat-check` | Offline `sha256` integrity of the pinned upstream FLAT corpus against its `MANIFEST.txt`; no network and no `curl`/`jq` needed, so it is safe in the gate. Catches a hand-edit to a vendored fixture whose whole value is being byte-identical to upstream. `make crossformat-check` does the same for the PROBE-105 cross-format sets, whose integrity also runs in `make test` |
| Fixtures | `make flat-drift` | The above **plus** a best-effort upstream-drift report (needs network; degrades with a note when offline). Dev helper, not a CI gate |
| Fixtures | `make corpus-pins-check` | Fails when an EHRbase corpus provenance file (FLAT and cross-format manifests, `AQL_SOURCE.txt`, `ROBOT_SOURCE.txt`) records a commit other than the one [`testkit/corpus/UPSTREAM_PINS.txt`](../testkit/corpus/UPSTREAM_PINS.txt) names for its repository. Offline |
| Fixtures | `make corpus-pins-drift` | The above **plus** how far each pin is behind its upstream branch (`git ls-remote`; needs network). Dev helper, not a CI gate |
| Fixtures | `make bmm-check` | Offline `sha256` integrity of the pinned BMM schemas against `resources/bmm/MANIFEST.txt`; also fails on a `*.bmm.json` the manifest does not list |
| Fixtures | `make bmm-drift` | The above **plus** a best-effort report of pinned files changed on ITS-BMM `master` and of newer schema versions (needs network). Dev helper, not a CI gate |
| Fixtures | `make its-xml-check` | Offline `sha256` integrity of the pinned ITS-XML schemas (a reference only) against `resources/its-xml/MANIFEST.txt` |
| Fixtures | `make its-xml-drift` | The above **plus** a best-effort report of changed files on ITS-XML `master` and of a newer release directory for a pinned component (needs network). Dev helper, not a CI gate |
| Fixtures | `make terminology-check` | Offline `sha256` integrity of the pinned openEHR Terminology against its `MANIFEST.txt` |
| Fixtures | `make terminology-drift` | The above **plus** a best-effort report of a newer upstream TERM release (needs network). Dev helper, not a CI gate |
| Specs | `make spec-context REQ=NNN` | Assemble the SDD context bundle for a REQ (dev/agent helper; not a CI gate) |
| Specs | `make probe-status` | Each PROBE's status and whether its test file exists (dev helper; not a CI gate) |
| Test | `make cdr-crosscheck` | Opt-in, and `make ci` does not run it: cross-checks the SDK's generated documents and the CDRs' own template examples against live CDRs ([§ REQ-082 *The CDR cross-check tool*](specifications/conformance.md#the-cdr-cross-check-tool)). Reads `OPENEHR_XC_TEMPLATES` (a directory of OPTs), `OPENEHR_LIVE_EHRBASE` and `OPENEHR_LIVE_FERROEHR` with their `_BASIC` credentials, and `OPENEHR_LIVE_ALLOW_MUTATING`. Without `OPENEHR_XC_TEMPLATES` it prints that it skipped and exits 0; with templates but no CDR, or without `OPENEHR_LIVE_ALLOW_MUTATING`, it generates and validates offline and skips the CDR stages. `-only REGEXP` in `ARGS` checks only the templates whose id matches, and the summary of such a run says it was filtered. Writes to a directory outside the repository that no other run has claimed (`-out` in `ARGS`, or `OPENEHR_XC_OUT`; by default a new one under the system temp directory), each record as it completes, so an interrupted run keeps them and writes a partial summary |
| Lint | `make lint` | `golangci-lint` on host if the binary was built with Go 1.27, else Docker (`LINT_IMAGE`) |

**Policy:** extend the [Makefile](../Makefile), not ad-hoc shell in workflows. CI and contributors share the same entry points ([AGENTS.md](../AGENTS.md) Tooling policy).

### Lint configuration

- Config: [`.golangci.yml`](../.golangci.yml)
- Pin: `golangci/golangci-lint:v2.14.0-alpine` (Makefile `LINT_IMAGE` and the Dockerfile's `GOLANGCI_IMAGE`), and `v2.14.0` as the GitHub Action `version`. It is the first release that reads the export data Go 1.27.2 writes (version 5); v2.13.2 fails every package with `could not load export data` against a 1.27.2 toolchain, which the CI job's `go-version: '1.27.x'` resolves to. Official release binaries are built with Go 1.27; a host `go install` from an older toolchain cannot load this module (golangci-lint refuses when its build Go is below the `go.mod` floor). `make lint` / `make fmt-check` then fall back to the pinned image.
- Generated files (the `// Code generated … DO NOT EDIT.` set) are skipped via `exclusions: generated: lax` in `.golangci.yml`

## Dependency updates

[`.github/dependabot.yml`](../.github/dependabot.yml) opens weekly PRs for `go.mod` and GitHub Actions version bumps.

## Future CI (not yet wired)

| Check | When |
|---|---|
| `govulncheck ./...` | Before v1.0.0 or when non-stdlib deps land |
| Conformance probe runner (`testkit/probes/…`) | Dedicated job when live-backend modes are needed |

## See also

- [docs/releases.md](releases.md): version policy, tag checklist, `v1.0.0` gate
- [docs/ai-workflow.md](ai-workflow.md): agent pre-merge checklist
- [resources/README.md](../resources/README.md): BMM pin and update procedure
- [docs/specifications/conformance.md](../docs/specifications/conformance.md): PROBE-NNN definitions (tests run via `make test` today)
