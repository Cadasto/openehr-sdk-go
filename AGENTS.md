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
| Sister SDK | Cadasto PHP SDK (semantic parity, identical conformance probe set) |
| Status | **Early implementation** — see [Status and active scope](#status-and-active-scope); BMM/canjson + transport/EHR REST landed; `auth/smart` PKCE and Query/Definition clients open |

## Source of truth

The normative specification for this SDK lives **in this repo** under [`specs/`](specs/). That tree is self-contained — implementing or reviewing the SDK does not require access to external architecture sources. Read [`specs/README.md`](specs/README.md) for the conventions (RFC-2119 keywords, status headers, identifier scheme, traceability).

`specs/` reflects and supersedes the upstream **Cadasto SDK Specification proposal**: when the two disagree, this tree wins until the upstream is reconciled. Open research strands in [`specs/research-strands.md`](specs/research-strands.md) MUST NOT be silently resolved by code — surface the decision and record an in-repo ADR under [`docs/adr/`](docs/adr/).

Related Cadasto proposals (referred to here by role, not by identifier):

- **PHP SDK Specification proposal** — sister SDK; semantic parity contract.
- **MPI / identity federation research** — feeds the `cadasto/mpi/` preview shape.
- **Cadasto authorization-server design** — the SDK consumes its outcome via `auth/`.
- **Cadasto SMART-on-openEHR decision** — the basis for `auth/smart/` and `smart/`.

Local sibling for extraction work: the `openehr-cdr` repo (cloned under `/src/cadasto/`). Its benchmark CLI is the SDK's first consumer.

## Documentation

Reading order for any contributor or agent:

| # | Doc | Scope |
|---|---|---|
| 1 | [AGENTS.md](AGENTS.md) (this file) | 1-page entry point |
| 2 | [specs/](specs/) | **Normative specifications** — REQ/PROBE/STRAND identifiers; canonical prose in topic specs |
| 2a | [specs/REQ.md](specs/REQ.md) | Requirement **registry** (index only — one row per REQ-NNN) |
| 2b | [specs/traceability.yaml](specs/traceability.yaml) | Machine-readable REQ → package / probe / test map |
| 3 | [docs/architecture.md](docs/architecture.md) | Design narrative — package map, dependency mermaid, why-it's-shaped-this-way |
| 4 | [docs/ai-workflow.md](docs/ai-workflow.md) | AI agent conventions, MCP / openEHR skills, hooks |
| 5 | [docs/adr/](docs/adr/) | Closed architectural decisions — see [docs/adr/README.md](docs/adr/README.md) (0001–0004 Accepted) |
| 6 | [docs/plans/](docs/plans/) | Implementation plans — see [docs/plans/README.md](docs/plans/README.md) |
| 7 | [CHANGELOG.md](CHANGELOG.md) | High-level release log (`## [Unreleased]` rolls forward) |

**Normative vs narrative.** `specs/` carries RFC-2119 `MUST/SHOULD/MAY` statements that code, plans, and tests are measured against. `docs/architecture.md` carries the design *narrative* — the same information re-told as prose with a mermaid diagram. If they disagree, `specs/` wins and the narrative is updated.

### Spec-driven workflow (agents)

When implementing or reviewing against a REQ:

1. Open the row in [`specs/REQ.md`](specs/REQ.md) → follow the **Canonical** link (e.g. [`specs/wire.md`](specs/wire.md), [`specs/transport.md`](specs/transport.md)).
2. Check [`specs/traceability.yaml`](specs/traceability.yaml) for landed packages, probes, and tests.
3. Cite `REQ-NNN` / `PROBE-NNN` in tests and `doc.go`; update `traceability.yaml` when landing new code.
4. Run `make spec-check` before claiming spec compliance (`make ci` includes it).

New normative text goes in the **canonical topic spec** first, then the REQ registry row — not duplicate bodies in `REQ.md`.

## Module layout

The normative taxonomy and dependency rules live in [`specs/module-layout.md`](specs/module-layout.md). Summary, with the dependency rule **"strictly downward, never sideways inside `cadasto/`"**:

```
github.com/cadasto/openehr-sdk-go/
├── auth/                      # generic TokenSource + OAuth2 primitives
│   ├── smart/                 # SMART-on-openEHR provider (PKCE, launch) — planned
│   ├── clientcreds/           # Client Credentials provider
│   └── jwtbearer/             # JWT Bearer provider
├── transport/                 # HTTP wrapper around injected *http.Client
├── openehr/
│   ├── rm/                    # RM types + type registry (typereg)
│   ├── serialize/             # canonical JSON/XML, FLAT, STRUCTURED
│   ├── validation/            # Composition vs OPT, demographic, AQL
│   ├── template/              # OPT/OET parsing, path utilities
│   ├── aql/                   # struct-builder + verb-functions
│   ├── composition/           # OPT-driven generic builder
│   └── client/                # REST clients grouped per openEHR resource
│       ├── ehr/               # EHR, Composition, Contribution, Directory, EHR_STATUS, ItemTags
│       ├── query/             # AQL executor (stub)
│       ├── definition/        # templates, stored queries (stub)
│       ├── demographic/       # stub
│       └── system/            # landed
├── smart/                     # application-level SMART AppContext + App Registration
│   └── discovery/             # service catalog resolver
├── sandbox/                   # in-memory + recorded-fixture transports
├── testkit/                   # test doubles, fluent builders, conformance probes
├── cadasto/                   # Cadasto-platform extras — single cut line
│   ├── extra/                 # Cadasto Extra API
│   ├── datamap/               # Datamap v1
│   ├── care/                  # Patient, User, CaseLoad, CareTeam, Episode aggregates
│   ├── mpi/                   # minimal MPI search (preview)
│   └── admin/                 # tenant, env, system, healthcheck
├── cmd/examples/              # worked examples per use case
├── internal/                  # implementation helpers — excluded from BC promises
└── docs/                      # architecture, ai-workflow, ADRs, plans
```

**Boundary rules** (load-bearing — a violation forfeits the option of extracting `cadasto/` later):

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports from `cadasto/…`.
- No `cadasto/<name>` package imports another `cadasto/<other>` package directly — they share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART-on-openEHR (`auth/smart`) and other providers layered on top.
- `internal/…` is consumer-invisible and excluded from semver promises.

## Idiomatic surface

The SDK is **idiomatic Go**, not a port of the PHP SDK. Semantic parity is enforced by the shared conformance probe set; per-language API is independent. Normative rules in [`specs/idiom.md`](specs/idiom.md).

- `context.Context` is the first parameter on every method that does I/O.
- `*http.Client` is **injected**, never allocated by the SDK.
- Functional options for configuration (per package), e.g. `transport.New(catalog, transport.WithHTTPClient(hc), transport.WithTokenSource(ts))`.
- Package-level functions for the primary surface; repository structs offered as a convenience for injection seams.
- Generics for typed REST responses, validators, repositories, template bindings — **no reflection** to carry types.
- Concrete structs for concrete RM types + embedded base structs for shared fields; interfaces for abstract RM categories; central type registry for `_type` decoding. **No inheritance emulation.**

## Building-block use cases

Each core package stands on its own — applications must not be forced to construct an authenticated client to use the RM, codecs, or template parser. Normative rule: REQ-013 ([`specs/module-layout.md`](specs/module-layout.md#req-013--building-block-independence)). Constructors and zero-values must be ergonomic for these cases:

- `openehr/rm/` alone — model openEHR data in memory.
- `openehr/serialize/` alone — canonicalize / reformat / hash.
- `openehr/validation/` alone — validate a Composition or OPT in CI or a webhook.
- `openehr/aql/` (models only) — construct or parse AQL strings without an executor.
- `openehr/template/` alone — OPT/OET parsing and path utilities.

## Code style and conventions

- **Formatting:** `gofmt -w -s` (the Makefile's `make fmt` and the Claude Code save hook both apply it).
- **Lint:** `golangci-lint` via `make lint` (image pinned in [`Makefile`](Makefile)).
- **Imports:** standard library first, then third-party, then internal — separated by a blank line; let `gofmt` / `goimports` handle ordering.
- **Naming:** idiomatic Go — exported `CamelCase`, unexported `camelCase`; package names are short, lowercase, no underscores.
- **Errors:** wrapped with `fmt.Errorf("...: %w", err)` for upward context; typed sentinel errors at boundary checks. No panics in library code.
- **Generics:** use them where they remove a reflection hop or a type assertion — not as decoration. If a generic API is harder to read than a `T`-specific one, drop the generic.
- **Concurrency:** clients are goroutine-safe by construction. Document any exception in the package doc.
- **Public API:** anything outside `internal/` is part of the semver contract. Adding to it is fine; renaming/removing requires a major bump.

- **Commit messages:**
  - Follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) conventions, e.g. `fix(resources): refreshed BMM definitions in resources`, `feat(tools): added new tool for operational templates`.
  - Scope is a short noun phrase identifying the module/area touched: `auth`, `rm`, `transport`, `client/ehr`, `docs`, `agents`, `build`, etc.

- **CHANGELOG.md** — agents **do not need to update** this file for every change. Update it only when the user asks, or when cutting a release / merging a milestone PR. When you do touch it:
  - **One bullet per artefact class** (e.g. “Transport layer”, “EHR REST client”) — not per file, type, REQ-ID, probe, or commit.
  - **No** API inventories, option lists, test descriptions, ADR summaries, or copy-paste from PR bodies.
  - **No** file paths, struct names, or “implements REQ-NNN” traceability (that lives in `specs/traceability.yaml` and commit messages).
  - Example good bullet: `REST clients: EHR read/write surface (composition, ehrstatus, directory, contribution).`
  - Example bad bullet: a paragraph listing every `With*` option on `transport.Client`.
  - Put implementation detail in **Conventional Commits** and the PR description instead.

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

GitHub Actions workflows and branch-protection guidance: [docs/ci.md](docs/ci.md). Conformance probes (`testkit/probes/…`) run via `make test`; PROBE-030/031 (serialize) and PROBE-010–012 (versioned writes) are implemented — see [`specs/conformance.md`](specs/conformance.md).

## openEHR knowledge

Use the openEHR MCP skills before guessing RM paths, terminology codes, or ITS-JSON shapes. See [docs/ai-workflow.md § MCP & openEHR skills](docs/ai-workflow.md#mcp--openehr-skills). The cross-SDK conformance probe set is the source of truth for wire-level semantics; the openEHR spec itself is authoritative for class invariants.

## Status and active scope

| Phase | Description | Status |
|---|---|---|
| 0 | Repo scaffolding — module layout, AI-assistant docs, Makefile, Dockerfile, `specs/` tree | **complete** |
| 0.5 | BMM loader, codegen (RM + AOM 1.4), typereg, canonical JSON | **landed** — [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md), [ADR 0004](docs/adr/0004-numeric-wire-tolerance.md) |
| 1a | Transport, auth providers (`clientcreds`, `jwtbearer`), discovery, System + EHR REST (read/write) | **landed** — see [REST client plan](docs/plans/2026-05-15-rest-api-client.md) Phases 2–4 |
| 1b | `auth/smart` PKCE end-to-end, Query/Definition clients, CDR benchmark (STRAND-01) | **open** |
| 2 | Composition builder + Templates + AQL builder/executor | not started |
| 3 | Application-level SMART (`smart/` AppContext) on top of landed discovery | partial — discovery landed; launch flow open (STRAND-05) |
| 4 | Cadasto extras (Extra, Datamap, MPI preview, Admin, Care) | not started |
| 5 | Sandbox + full conformance probe ratification | partial — serialize + versioned probes landed |

Sequencing is informed by the openehr-cdr extraction (STRAND-01 in [`specs/research-strands.md`](specs/research-strands.md)) — the existing CDR HTTP layer and RM mapping are the first source.

## Do not touch (yet)

- Promoting new numbered ADRs without updating [`docs/adr/README.md`](docs/adr/README.md), [`specs/REQ.md`](specs/REQ.md), and [`specs/traceability.yaml`](specs/traceability.yaml). Open decisions stay as research strands in [`specs/research-strands.md`](specs/research-strands.md) until an ADR lands.
- Duplicating normative REQ prose in `REQ.md` — the registry is index-only; canonical text lives in topic specs ([`specs/packaging.md`](specs/packaging.md), [`specs/transport.md`](specs/transport.md), etc.).
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling only; not public API. Changes need rationale in [`docs/architecture.md`](docs/architecture.md) and, for structural choices, [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- REQ-NNN, PROBE-NNN, STRAND-NN identifiers are **stable** once published — never renumber, never reuse.

## Cross-references

- Cadasto architecture (private) — source of truth for SDK and platform proposals. Linked by role, not by path.
- Sibling repos cloned under `/src/cadasto/`: `architecture`, `openehr-cdr`, `openehr-bmm`, `openehr-assistant-mcp`, `openehr-assistant-plugin`.
