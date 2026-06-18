# AGENTS.md

**Entry point for every coding agent and contributor.** Pair with [`README.md`](README.md). Claude Code also loads [`.claude/CLAUDE.md`](.claude/CLAUDE.md) (Claude-specific notes only). This is the 1-page map; the specialized docs it links are **canonical** — defer to them rather than duplicating.

## Project

A first-party **Go SDK for openEHR** — `github.com/cadasto/openehr-sdk-go`. **openEHR-first**: openEHR REST `1.1.0-development`, the Reference Model, AQL, ADL 1.4 OPT, and SMART-on-openEHR auth are the normative scope. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care) ship in the same module for v1, behind a clean `cadasto/` cut line so later extraction is a subtree move, not a rewrite.

| Aspect | Setting |
|---|---|
| Module path | `github.com/cadasto/openehr-sdk-go` |
| License | MIT |
| Go version | `1.25.x` (N-1 release line) |
| openEHR REST | `1.1.0-development` |
| Status | **Early implementation, pre-1.0** — landed-vs-planned in [docs/roadmap.md](docs/roadmap.md) |

## Source of truth

The normative spec lives **in this repo** under [`docs/specifications/`](docs/specifications/) — self-contained; implementing or reviewing the SDK needs no external sources. Conventions (RFC-2119 keywords, status headers, identifiers, traceability): [`docs/specifications/README.md`](docs/specifications/README.md).

When code and specs disagree, **the specs win**. Never silently resolve an open [research strand](docs/specifications/research-strands.md) in code — surface the decision or land an [ADR](docs/adr/).

## Documentation

Reading order:

| # | Doc | Scope |
|---|---|---|
| 0 | [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) | **Developer onboarding** — install, integration paths, runnable `cmd/examples/` catalog |
| 1 | [AGENTS.md](AGENTS.md) (this file) | 1-page entry point |
| 2 | [docs/specifications/](docs/specifications/) | **Normative specs** — REQ/PROBE/STRAND in [REQ.md](docs/specifications/REQ.md); machine map in [traceability.yaml](docs/specifications/traceability.yaml) |
| 3 | [docs/architecture.md](docs/architecture.md) | Design narrative — package organization, dependencies, integration, mermaid diagrams |
| 4 | [docs/ai-workflow.md](docs/ai-workflow.md) | AI conventions — recommended plugins/skills (go-coding, gopls-lsp, codebase-memory), MCP/openEHR skills, hooks |
| 5 | [docs/adr/](docs/adr/) | Closed architectural decisions |
| 6 | [docs/plans/](docs/plans/) + [docs/roadmap.md](docs/roadmap.md) | Implementation plans and landed-vs-planned checklist |
| 7 | [CHANGELOG.md](CHANGELOG.md) + [docs/releases.md](docs/releases.md) | Release log and version policy |
| 8 | [CONTRIBUTING.md](CONTRIBUTING.md) + [SECURITY.md](SECURITY.md) | Contributor flow and vulnerability reporting |

**Normative vs narrative.** `docs/specifications/` carries the RFC-2119 statements code/plans/tests are measured against; `docs/architecture.md` carries the design narrative. If they disagree, the specs win.

### Spec-driven workflow (agents)

When implementing or reviewing against a REQ:

0. Run `make spec-context REQ=NNN` — one bundle with the registry row, traceability block (packages/probes/tests/plans), canonical excerpt, and touching strands.
1. Open the row in [`docs/specifications/REQ.md`](docs/specifications/REQ.md) → follow the **Canonical** link (don't read prose out of `REQ.md` itself).
2. Check [`docs/specifications/traceability.yaml`](docs/specifications/traceability.yaml) for landed packages, probes, and tests.
3. Cite `REQ-NNN` / `PROBE-NNN` in tests and `doc.go`; update `traceability.yaml` when landing code.
4. Run `make spec-check` before claiming spec compliance (`make ci` includes it).

New normative text goes in the **canonical topic spec** first, then the REQ registry row — never as duplicate prose in `REQ.md` or as a rule that exists only in code.

**Descriptor & process.** Machine-readable conventions (REQ style, document paths, `make` targets, `PROBE`/`STRAND` toggles, ground-truth source) live in [`docs/.sdd.yaml`](docs/.sdd.yaml) — the descriptor the `sdd-*` skills read first. The end-to-end loop and the Definition of Ready / Done are mapped in [`docs/development-process.md`](docs/development-process.md).

**superpowers + SDD.** SDD owns the spec/traceability layer; the superpowers loop owns build/verify/branch. Brainstorming design docs are *narrative input* that feeds the canonical specs (not a normative source), and plans belong in [`docs/plans/`](docs/plans/) with the `**Covers:**` header + DoR/DoD — never a parallel `docs/superpowers/` tree. Full redirect: [development-process.md § superpowers + SDD](docs/development-process.md#superpowers--sdd).

**Examples:** when you add, rename, remove, or materially change a [`cmd/examples/`](cmd/examples/) program, keep its docs in sync **in the same PR** — checklist in [ai-workflow.md § Examples](docs/ai-workflow.md#examples).

## Module layout & boundaries

Full taxonomy and the package tree are in [module-layout.md](docs/specifications/module-layout.md) (normative) and [architecture.md](docs/architecture.md) (narrative). The **load-bearing rules** — a violation forfeits the option of extracting `cadasto/` later:

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports `cadasto/…`.
- No `cadasto/<X>` imports another `cadasto/<Y>` directly — share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART (`auth/smart`) and other providers on top.
- `internal/…` is consumer-invisible and excluded from semver promises.
- **Building-block independence (REQ-013):** `openehr/{rm,serialize,validation,template}` and `openehr/aql` (models only) MUST be usable standalone, with no `transport/` or `auth/` import.

## Code style and conventions

The elaborate, normative idiom spec is [`idiom.md`](docs/specifications/idiom.md) (context propagation, `*http.Client` injection, functional options, generics-no-reflection, errors, concurrency, naming, public-API stability) — read it. The quick version:

- **Format / lint:** `make fmt` (gofumpt + goimports via `golangci-lint fmt`) and `make lint` (golangci-lint v2 + `modernize` / `errorlint`), both pinned in the [Makefile](Makefile); `make ci` gates them.
- **Idioms:** `context.Context` first on every I/O method; inject `*http.Client` (never allocate one); functional options; package-level functions as the primary surface; generics only to remove a reflection hop; **no reflection** and no inheritance emulation (concrete structs + `typereg` for `_type` decoding).
- **Errors:** wrap with `fmt.Errorf("…: %w", err)`; typed sentinels at boundaries; no panics in library code.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) — scope is the touched area (`auth`, `rm`, `transport`, `client/ehr`, `docs`, `build`, …).

**CHANGELOG.md** — agents update it only on request or when cutting a release / merging a milestone. One short bullet per artefact class (not per file, REQ, or commit); no API inventories (those live in `traceability.yaml`). Pre-1.0: only `### Added` is used.

## Tooling & workflow

Host Go `1.25.x` is the fast path; the Makefile auto-routes through a Docker dev image when host Go is missing ([Dockerfile](Dockerfile), [docker-compose.yml](docker-compose.yml)). **Use the Makefile as the single entry point** — extend it, don't add ad-hoc scripts.

| Task | Command |
|---|---|
| Discover all targets | `make help` |
| **Full PR / CI gate** | `make ci` — see [docs/ci.md](docs/ci.md) |
| Format / check | `make fmt` / `make fmt-check` |
| Vet / lint | `make vet` / `make lint` |
| Unit / race tests | `make test` / `make test-race` |
| BMM codegen verify | `make codegen-verify` |
| AQL parser codegen verify | `make aqlgen-verify` — fails if `openehr/aql/parse/gen/` drifts from the `active/` grammar (needs Docker, not a host JRE); regenerate with `make aqlgen`. Both run under `make test` / `make ci`. |
| Spec traceability | `make spec-check` |
| Spec context bundle | `make spec-context REQ=NNN` — registry row + traceability + canonical excerpt + strands |
| Probe status | `make probe-status` — each PROBE's status and whether its test file exists |
| Build Docker dev image | `make image-dev` (only when host Go is missing) |

Test framework is stdlib `testing` + helpers in `testkit/`. Runtime dependencies are kept deliberately minimal and reviewed; the current set is: **OpenTelemetry** (tracing, confined to `transport/`), **antlr4-go** (AQL parser, `openehr/aql/parse`), and — adopted for SMART/auth crypto correctness ([ADR 0009](docs/adr/0009-smart-auth-library-scope.md)) — **`golang.org/x/oauth2`** and **`github.com/coreos/go-oidc/v3`** (the latter brings `go-jose/v4` transitively), scoped to `auth/` and `smart/` — see [architecture.md § Dependencies](docs/architecture.md#dependencies). Conformance probes (`testkit/probes/…`) run via `make test`; inventory in [conformance.md](docs/specifications/conformance.md).

**Recommended agent tooling:** the **go-coding** plugin (Go skills + the `go-reviewer` agent), **gopls-lsp** (code intelligence), and **codebase-memory-mcp** (structural exploration / impact) — see [ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor).

## openEHR knowledge

Use the openEHR MCP skills before guessing RM paths, terminology codes, or ITS-JSON shapes — see [ai-workflow.md § openEHR ground truth](docs/ai-workflow.md#openehr-ground-truth-mcp--skills). The openEHR conformance probe suite is the source of truth for wire-level semantics; the openEHR spec is authoritative for class invariants.

**REST API schema.** The machine-readable openEHR REST API contract is vendored in [`resources/its-rest/`](resources/its-rest/README.md) — the upstream `*-validation.openapi.yaml` OpenAPI 3.0 documents (EHR, Query, Definition, Admin, Demographic, System). When you need endpoint paths, request/response bodies, headers, or status codes for any REST resource, read those files rather than guessing. Refresh / verify the pin with `make its-rest-sync` / `make its-rest-check`.

## Do not touch (yet)

- Promoting new numbered ADRs without updating [`docs/adr/README.md`](docs/adr/README.md), [`REQ.md`](docs/specifications/REQ.md), and [`traceability.yaml`](docs/specifications/traceability.yaml). Open decisions stay as [research strands](docs/specifications/research-strands.md) until an ADR lands.
- Duplicating normative REQ prose in `REQ.md` — the registry is index-only; canonical text lives in the topic specs.
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling, not public API; structural changes need rationale in [architecture.md](docs/architecture.md) and [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- REQ-NNN / PROBE-NNN / STRAND-NN identifiers — **stable** once published; never renumber or reuse.
