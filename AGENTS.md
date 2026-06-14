# AGENTS.md

**Entry point for every coding agent and contributor.** Pair with [`README.md`](README.md). Claude Code additionally loads [`.claude/CLAUDE.md`](.claude/CLAUDE.md) — that file stays small and only carries Claude-specific notes.

## Project

A first-party **Go SDK for openEHR** — package `github.com/cadasto/openehr-sdk-go`. The SDK is **openEHR-first**: openEHR REST `1.1.0-development`, Reference Model, AQL, OPT/OET, and SMART-on-openEHR auth are the normative scope. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care aggregates) ship in the same module in v1 for adoption convenience, with a clean cut line under `cadasto/` so later extraction would be a subtree move rather than a rewrite.

| Aspect | Setting |
|---|---|
| Module path | `github.com/cadasto/openehr-sdk-go` |
| License | MIT |
| Go version | `1.25.x` (N-1 release line) |
| openEHR REST | `1.1.0-development` |
| Equivalent SDK | Cadasto PHP SDK (semantic parity, identical conformance probe set) |
| Status | **Early implementation, pre-1.0** — landed-vs-planned scope tracked in [docs/roadmap.md](docs/roadmap.md) |

## Source of truth

The normative specification for this SDK lives **in this repo** under [`docs/specifications/`](docs/specifications/). That tree is self-contained — implementing or reviewing the SDK does not require access to external architecture sources. Read [`docs/specifications/README.md`](docs/specifications/README.md) for the conventions (RFC-2119 keywords, status headers, identifier scheme, traceability).

`docs/specifications/` reflects and supersedes the upstream **Cadasto SDK Specification proposal**: when the two disagree, this tree wins until the upstream is reconciled. Open research strands in [`docs/specifications/research-strands.md`](docs/specifications/research-strands.md) MUST NOT be silently resolved by code — surface the decision and record an in-repo ADR under [`docs/adr/`](docs/adr/).

Related Cadasto proposals (referred to by role, not by identifier):

- **PHP SDK Specification proposal** — equivalent SDK; semantic parity contract.
- **MPI / identity federation research** — feeds the `cadasto/mpi/` preview shape.
- **Cadasto authorization-server design** — the SDK consumes its outcome via `auth/`.
- **Cadasto SMART-on-openEHR decision** — the basis for `auth/smart/` and `smart/`.

Local reference CDR (private; cloned under `/src/cadasto/` alongside this SDK). Its load-test harness is the first consumer.

## Documentation

Reading order for any contributor or agent:

| # | Doc | Scope |
|---|---|---|
| 0 | [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) | **Developer onboarding** — install, integration paths, runnable `cmd/examples/` catalog |
| 1 | [AGENTS.md](AGENTS.md) (this file) | 1-page entry point |
| 2 | [docs/specifications/](docs/specifications/) | **Normative specs** — REQ/PROBE/STRAND in [REQ.md](docs/specifications/REQ.md); machine-readable map in [traceability.yaml](docs/specifications/traceability.yaml) |
| 3 | [docs/architecture.md](docs/architecture.md) | Design narrative — package map + mermaid diagram |
| 4 | [docs/ai-workflow.md](docs/ai-workflow.md) | AI agent conventions, MCP / openEHR skills, hooks, **example-doc maintenance** |
| 5 | [docs/adr/](docs/adr/) | Closed architectural decisions (0001–0005 Accepted) |
| 6 | [docs/plans/](docs/plans/) + [docs/roadmap.md](docs/roadmap.md) | Implementation plans and landed-vs-planned checklist |
| 7 | [CHANGELOG.md](CHANGELOG.md) | High-level release log (`## [Unreleased]` rolls forward) |
| 8 | [docs/releases.md](docs/releases.md) | Version policy, tag checklist, `v1.0.0` gate |
| 9 | [CONTRIBUTING.md](CONTRIBUTING.md) + [SECURITY.md](SECURITY.md) | Contributor flow and vulnerability reporting |

**Normative vs narrative.** `docs/specifications/` carries RFC-2119 statements that code, plans, and tests are measured against. `docs/architecture.md` carries the design narrative. If they disagree, `docs/specifications/` wins.

### Spec-driven workflow (agents)

When implementing or reviewing against a REQ:

1. Open the row in [`docs/specifications/REQ.md`](docs/specifications/REQ.md) → follow the **Canonical** link.
2. Check [`docs/specifications/traceability.yaml`](docs/specifications/traceability.yaml) for landed packages, probes, and tests.
3. Cite `REQ-NNN` / `PROBE-NNN` in tests and `doc.go`; update `traceability.yaml` when landing new code.
4. Run `make spec-check` before claiming spec compliance (`make ci` includes it).

New normative text goes in the **canonical topic spec** first, then the REQ registry row — not duplicate bodies in `REQ.md`.

### Runnable examples (agents)

[`cmd/examples/`](cmd/examples/) holds worked programs for each major SDK surface. [`docs/examples.md`](docs/examples.md) is the **developer-facing catalog**; [`docs/quick-start.md`](docs/quick-start.md) is the onboarding path that links into it.

When you **add, rename, remove, or materially change** an example (CLI flags, packages shown, fixtures, or the integration path it demonstrates), update the docs in the **same PR**:

1. [`cmd/examples/doc.go`](cmd/examples/doc.go) — package comment bullet list (canonical inventory for agents).
2. [`docs/examples.md`](docs/examples.md) — summary table, per-example section, learning order if the story changed.
3. [`docs/quick-start.md`](docs/quick-start.md) — only when the change affects onboarding (new first-run path, new REST wiring pattern, or a snippet worth copying).
4. [`README.md`](README.md) — Quickstart block if the recommended first command changes.

Run the example (`go run ./cmd/examples/<name>`) and align any sample output in the docs with what it actually prints. Full checklist: [docs/ai-workflow.md § Developer examples](docs/ai-workflow.md#developer-examples--docs).

## Module layout

Normative taxonomy and dependency rules: [`docs/specifications/module-layout.md`](docs/specifications/module-layout.md). Top-level shape:

- `auth/` + providers `auth/{smart,clientcreds,jwtbearer,basic}/` — TokenSource + OAuth2 primitives
- `transport/` — HTTP wrapper around injected `*http.Client`
- `openehr/` — `rm/` (+ `rm/rminfo/` BMM structural lookup), `aom/aom14/`, `serialize/`, `validation/`, `template/`, `aql/`, `composition/`, and `client/{system,ehr,query,definition,demographic,admin}/`
- `smart/` + `smart/discovery/` — application-level SMART LaunchContext + ID-token validation + service catalog resolver
- `cadasto/` — platform extras behind the single cut line (`extra/`, `datamap/`, `care/`, `mpi/`, `admin/`)
- `sandbox/`, `testkit/`, `cmd/examples/`, `internal/`, `docs/`

**Boundary rules** (load-bearing — a violation forfeits the option of extracting `cadasto/` later):

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports from `cadasto/…`.
- No `cadasto/<name>` package imports another `cadasto/<other>` package directly — they share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART-on-openEHR (`auth/smart`) and other providers layered on top.
- `internal/…` is consumer-invisible and excluded from semver promises.

## Idiomatic surface

The SDK is **idiomatic Go**, not a port of the PHP SDK. Semantic parity is enforced by the shared conformance probe set; per-language API is independent. Normative rules in [`docs/specifications/idiom.md`](docs/specifications/idiom.md).

- `context.Context` is the first parameter on every method that does I/O.
- `*http.Client` is **injected**, never allocated by the SDK.
- Functional options for configuration (per package), e.g. `transport.New(catalog, transport.WithHTTPClient(hc), transport.WithTokenSource(ts))`.
- Package-level functions for the primary surface; repository structs offered as a convenience for injection seams.
- Generics for typed REST responses, validators, repositories, template bindings — **no reflection** to carry types.
- Concrete structs for concrete RM types + embedded base structs for shared fields; interfaces for abstract RM categories; central type registry for `_type` decoding. **No inheritance emulation.**
- **Building-block independence** (REQ-013, [`docs/specifications/module-layout.md`](docs/specifications/module-layout.md#req-013--building-block-independence)): `openehr/{rm,serialize,validation,template}/` and `openehr/aql/` (models only) MUST be usable standalone without constructing an authenticated client.

## Code style and conventions

- **Formatting:** gofumpt + goimports via `make fmt` (`golangci-lint fmt`, image pinned in [`Makefile`](Makefile)); the Claude Code save hook applies them per-file. `make fmt-check` gates it.
- **Lint:** `golangci-lint` v2 via `make lint` (image pinned in [`Makefile`](Makefile)); reference linter set incl. `modernize` + `errorlint` in [`.golangci.yml`](.golangci.yml).
- **Imports:** standard library first, then third-party, then internal — separated by a blank line; `goimports` enforces the ordering.
- **Naming:** idiomatic Go — exported `CamelCase`, unexported `camelCase`; package names short, lowercase, no underscores.
- **Errors:** wrapped with `fmt.Errorf("...: %w", err)` for upward context; typed sentinel errors at boundary checks. No panics in library code.
- **Generics:** use them where they remove a reflection hop or a type assertion — not as decoration. If a generic API is harder to read than a `T`-specific one, drop the generic.
- **Concurrency:** clients are goroutine-safe by construction. Document any exception in the package doc.
- **Public API:** anything outside `internal/` is part of the semver contract. Adding to it is fine; renaming/removing requires a major bump.
- **Commit messages:** [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) — scope is a short noun phrase for the touched area (`auth`, `rm`, `transport`, `client/ehr`, `docs`, `agents`, `build`, etc.).

**CHANGELOG.md** — agents **do not need to update** this file for every change. Update only when the user asks, or when cutting a release / merging a milestone PR. When you do:

- **One bullet per artefact class** (e.g. "Transport layer", "EHR REST client") — not per file, type, REQ-ID, probe, or commit.
- **Short.** Aim for **one line** per bullet; two at most. If a bullet wraps to a third line, it's too detailed — trim.
- **No** API inventories, option lists, file paths, struct names, or "implements REQ-NNN" traceability — that lives in `docs/specifications/traceability.yaml`, commit messages, and PR bodies. Readers chasing detail follow the commit log and `traceability.yaml`; the changelog is the headline, not the changelog of the changelog.
- **Pre-1.0:** only `### Added` is used; `### Changed` / `### Fixed` / `### Removed` are reserved for post-v1.0 entries. Pre-release renames, fix-ups, and dropped experiments fold into the relevant Added bullet or are dropped entirely.

## Tooling policy

1. **Go 1.25.x** — `go.mod` pins the line; bump PATCH when a stable release ships.
2. **Use the Makefile** as the single entry point — `make help`. Extend it; don't sprinkle shell scripts.
3. **Host Go is the fast path.** A Docker fallback (the `dev` stage in [`Dockerfile`](Dockerfile), wired through [`docker-compose.yml`](docker-compose.yml)) exists for contributors without a host Go install and for CI runners that prefer a single image. The Makefile auto-detects host Go `1.25.x` and switches to `docker compose run --rm go …` when it is missing.
4. **Released library versions** — avoid pseudo-versions unless a security backport requires it.

| Component | Pin |
|---|---|
| Go | `1.25.x` |
| Test framework | stdlib `testing` + lightweight helpers in `testkit/` |
| Lint | `golangci-lint` (image pinned in `Makefile`; also baked into the dev Dockerfile) |
| HTTP | stdlib `net/http` — no client library dependency |
| Auth | hand-rolled `auth/smart` (no Go equivalent of Socialite) |

## Workflow

| Task | Command |
|---|---|
| Discover targets | `make help` |
| Diagnose toolchain | `make doctor` |
| **Full PR / CI gate** | `make ci` — see [docs/ci.md](docs/ci.md) |
| Build Docker dev image (only when host Go is missing) | `make image-dev` |
| Format | `make fmt` |
| Format check (no write) | `make fmt-check` |
| Vet | `make vet` |
| Lint | `make lint` (host `golangci-lint` or Docker; same config as GitHub) |
| Unit tests | `make test` |
| Race tests | `make test-race` |
| Tidy modules | `make mod-tidy` |
| Verify `go.mod` tidy | `make mod-tidy-check` |
| BMM codegen verify | `make codegen-verify` |
| Spec traceability check | `make spec-check` |
| Build examples | `make build` |

GitHub Actions workflows and branch-protection guidance: [docs/ci.md](docs/ci.md). Conformance probes (`testkit/probes/…`) run via `make test`; landed inventory in [`docs/specifications/conformance.md`](docs/specifications/conformance.md).

## openEHR knowledge

Use the openEHR MCP skills before guessing RM paths, terminology codes, or ITS-JSON shapes. See [docs/ai-workflow.md § MCP & openEHR skills](docs/ai-workflow.md#mcp--openehr-skills). The cross-SDK conformance probe set is the source of truth for wire-level semantics; the openEHR spec itself is authoritative for class invariants.

## Status and active scope

Current landed-vs-planned phases live in [docs/roadmap.md](docs/roadmap.md). Sequencing is informed by reference-CDR extraction (STRAND-01 in [`docs/specifications/research-strands.md`](docs/specifications/research-strands.md)) — the existing CDR HTTP layer and RM mapping are the first source.

## Do not touch (yet)

- Promoting new numbered ADRs without updating [`docs/adr/README.md`](docs/adr/README.md), [`docs/specifications/REQ.md`](docs/specifications/REQ.md), and [`docs/specifications/traceability.yaml`](docs/specifications/traceability.yaml). Open decisions stay as research strands in [`docs/specifications/research-strands.md`](docs/specifications/research-strands.md) until an ADR lands.
- Duplicating normative REQ prose in `REQ.md` — the registry is index-only; canonical text lives in topic specs ([`docs/specifications/packaging.md`](docs/specifications/packaging.md), [`docs/specifications/transport.md`](docs/specifications/transport.md), etc.).
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling only; not public API. Changes need rationale in [`docs/architecture.md`](docs/architecture.md) and, for structural choices, [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- REQ-NNN, PROBE-NNN, STRAND-NN identifiers are **stable** once published — never renumber, never reuse.

## Cross-references

- Cadasto architecture (private) — source of truth for SDK and platform proposals. Linked by role, not by path.
- Sibling repos cloned under `/src/cadasto/`: reference CDR (private), `openehr-bmm`, `openehr-assistant-mcp`, `openehr-assistant-plugin` (plus an internal Cadasto architecture repo, referenced by role above rather than by path).
