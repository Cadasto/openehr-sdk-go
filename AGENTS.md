# AGENTS.md

**Entry point for every coding agent and contributor.** Pair with [`README.md`](README.md); Claude Code also loads [`.claude/CLAUDE.md`](.claude/CLAUDE.md) (Claude-specific notes only).

## Project

A first-party **Go SDK for openEHR** — `github.com/cadasto/openehr-sdk-go`, MIT. **openEHR-first**: openEHR REST `1.1.0-development`, the Reference Model, AQL, ADL 1.4 OPT, and SMART-on-openEHR auth are the normative scope. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care) ship in the same module for v1, behind a clean `cadasto/` cut line so later extraction is a subtree move, not a rewrite.

Go `1.26.x`, module floor `1.26.0` ([REQ-002](docs/specifications/packaging.md#req-002--go-version)). **Early implementation, pre-1.0** — landed-vs-planned in [docs/roadmap.md](docs/roadmap.md).

## Source of truth

The normative spec lives **in this repo** under [`docs/specifications/`](docs/specifications/) — self-contained; implementing or reviewing the SDK needs no external sources. Conventions (RFC-2119 keywords, status headers, identifiers, traceability): [`docs/specifications/README.md`](docs/specifications/README.md).

`docs/specifications/` carries the RFC-2119 statements code, plans, and tests are measured against; `docs/architecture.md` carries the design narrative. **When anything disagrees with the specs, the specs win.** Never silently resolve an open [research strand](docs/specifications/research-strands.md) in code — surface the decision or land an [ADR](docs/adr/).

## Documentation

Reading order — the specialized docs are **canonical**; defer to them rather than restating:

| # | Doc | Scope |
|---|---|---|
| 0 | [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) | **Developer onboarding** — install, integration paths, runnable `cmd/examples/` catalog |
| 1 | [docs/specifications/](docs/specifications/) | **Normative specs** — REQ/PROBE/STRAND in [REQ.md](docs/specifications/REQ.md); machine map in [traceability.yaml](docs/specifications/traceability.yaml); process + descriptor in [development-process.md](docs/development-process.md) / [.sdd.yaml](docs/.sdd.yaml) |
| 2 | [docs/architecture.md](docs/architecture.md) | Design narrative — package organization, dependencies, integration, mermaid diagrams |
| 3 | [docs/ai-workflow.md](docs/ai-workflow.md) | **AI conventions** — the working loop, recommended plugins/skills, openEHR ground-truth lookups, hooks |
| 4 | [docs/adr/](docs/adr/) | Closed architectural decisions |
| 5 | [docs/plans/](docs/plans/) + [docs/roadmap.md](docs/roadmap.md) | Implementation plans and landed-vs-planned checklist |
| 6 | [CHANGELOG.md](CHANGELOG.md) + [docs/releases.md](docs/releases.md) | Release log and version policy |
| 7 | [CONTRIBUTING.md](CONTRIBUTING.md) + [SECURITY.md](SECURITY.md) | Contributor flow and vulnerability reporting |

### Spec-driven workflow (agents)

**Start with `make spec-context REQ=NNN`** — one bundle with the registry row, traceability block, canonical excerpt, and touching strands. **Finish with `make spec-check`** (`make ci` includes it). The step-by-step loop lives in [ai-workflow.md § The loop](docs/ai-workflow.md#the-loop); the rules that bind regardless of how you got there:

- New normative text goes in the **canonical topic spec** first, then the REQ registry row — never as duplicate prose in `REQ.md`, and never as a rule that exists only in code.
- Cite `REQ-NNN` / `PROBE-NNN` in tests and `doc.go`; update `traceability.yaml` in the same change that lands the code.
- **`REQ`/`PROBE` is the feature register; there is no `SDK-GAP` identifier.** A discovered gap is worked under a REQ (extend or create via `sdd-specify`) with a `PROBE` for wire conformance. A GAP-style label may appear only as an ephemeral in-flight plan filename — never in `traceability.yaml`, test names, `doc.go`, or normative prose ([ADR 0012](docs/adr/0012-retire-sdk-gap-identifier.md)).
- Keep [`cmd/examples/`](cmd/examples/) docs in sync **in the same PR** as the program — checklist in [ai-workflow.md § Examples](docs/ai-workflow.md#examples).

**Descriptor & process.** Machine-readable conventions (REQ style, document paths, `make` targets, `PROBE`/`STRAND` toggles, ground-truth source) live in [`docs/.sdd.yaml`](docs/.sdd.yaml) — the descriptor the `sdd-*` skills read first. The end-to-end loop and the Definition of Ready / Done are mapped in [`docs/development-process.md`](docs/development-process.md).

**superpowers + SDD.** SDD owns the spec/traceability layer; the superpowers loop owns build/verify/branch. Brainstorming design docs are *narrative input* that feeds the canonical specs (not a normative source), and plans belong in [`docs/plans/`](docs/plans/) with the `**Covers:**` header + DoR/DoD — never a parallel `docs/superpowers/` tree. Full redirect: [development-process.md § superpowers + SDD](docs/development-process.md#superpowers--sdd).

## Module layout & boundaries

Full taxonomy and the package tree are in [module-layout.md](docs/specifications/module-layout.md) (normative) and [architecture.md](docs/architecture.md) (narrative). The **load-bearing rules** — a violation forfeits the option of extracting `cadasto/` later:

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports `cadasto/…`.
- No `cadasto/<X>` imports another `cadasto/<Y>` directly — share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART (`auth/smart`) and other providers on top.
- `internal/…` is consumer-invisible and excluded from semver promises.
- **Building-block independence (REQ-013):** `openehr/{rm,serialize,validation,template}` and the AQL blocks `openehr/aql` + `aql/parse` + `aql/lint` + `aql/contain` MUST be usable standalone, with no `transport/` or `auth/` import. `openehr/aql` is no longer models-only — it imports `aql/contain` and the Go-internal `aql/internal/semcheck` for REQ-162 containment verification.

## Code style and conventions

The elaborate, normative idiom spec is [`idiom.md`](docs/specifications/idiom.md) (context propagation, `*http.Client` injection, functional options, generics-no-reflection, errors, concurrency, naming, public-API stability) — read it. The quick version:

- **Format / lint:** `make fmt` (gofumpt + goimports via `golangci-lint fmt`) and `make lint` (golangci-lint v2 + `modernize` / `errorlint`), both pinned in the [Makefile](Makefile); `make ci` gates them.
- **Idioms:** `context.Context` first on every I/O method; inject `*http.Client` (never allocate one); functional options; package-level functions as the primary surface; generics only to remove a reflection hop; **no reflection** and no inheritance emulation (concrete structs + `typereg` for `_type` decoding).
- **Errors:** wrap with `fmt.Errorf("…: %w", err)`; typed sentinels at boundaries; no panics in library code.
- **Tests:** stdlib `testing` only — no assertion libraries — plus helpers in [`testkit/`](testkit/); behaviour tests for a public surface belong in the external `_test` package, so they exercise what consumers can reach. Guards carry the bar their spec sets — typically *removing the guard MUST fail a named test*.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) — scope is the touched area (`auth`, `rm`, `transport`, `client/ehr`, `docs`, `build`, …).

**CHANGELOG.md** — update only on request or when cutting a release. **One single-sentence bullet (~35 words max) per artefact class**: artefact + scope + key REQ/PROBE, never API inventories or per-REQ breakdowns (those live in `traceability.yaml`, commits, and PR bodies). Release notes are generated verbatim from the block, so a long entry is a defect — err short. Pre-1.0: `### Added` only.

## Tooling & workflow

Host Go `1.26.x` is the fast path; the Makefile auto-routes through a Docker dev image when host Go is missing ([Dockerfile](Dockerfile), [docker-compose.yml](docker-compose.yml)). **Use the Makefile as the single entry point** — extend it, don't add ad-hoc scripts.

| Task | Command |
|---|---|
| Discover all targets | `make help` |
| Diagnose the environment | `make doctor` — host Go, Docker, and which toolchain the Makefile will actually use |
| **Full PR / CI gate** | `make ci` — see [docs/ci.md](docs/ci.md) |
| Format / check | `make fmt` / `make fmt-check` |
| Vet / lint | `make vet` / `make lint` |
| Unit / race tests | `make test` / `make test-race` |
| BMM codegen verify | `make codegen-verify` |
| AQL parser codegen verify | `make aqlgen-verify` — fails if `openehr/aql/parse/gen/` drifts from the `active/` grammar (needs Docker, not a host JRE); regenerate with `make aqlgen` |
| Spec traceability | `make spec-check` |
| Spec context bundle | `make spec-context REQ=NNN` — registry row + traceability + canonical excerpt + strands |
| Probe status | `make probe-status` — each PROBE's status and whether its test file exists |
| FLAT corpus integrity | `make flat-conformance-verify` — offline `sha256` of the vendored EHRbase FLAT corpus (PROBE-086's input); `…-check` adds a network drift report (dev helper, not a gate) |
| Build Docker dev image | `make image-dev` (only when host Go is missing) |

**Gotchas worth the reading:**

- **Never hand-edit a vendored fixture** under `resources/` or `testkit/cassettes/` — being byte-identical to upstream is its whole value. Re-sync instead.
- `make probe-status` prints `MISSING` for any probe covered inline or in a sibling's file. That is the filename heuristic, **not** drift — `make spec-check` is the real gate.
- **`make ci` cannot complete without Docker** — `test` → `aqlgen-verify` → `antlr-image` is the only *unconditional* Docker link. The rest reaches for Docker only when host tooling is missing: `vet`/`build`/`test` need host Go `1.26.x`, `fmt-check`/`lint` a host `golangci-lint` (routing: [ci.md](docs/ci.md)). With those installed, run `fmt-check`, `vet`, `spec-check`, `flat-conformance-verify`, `build` and `go test ./... -count=1`, and let PR CI be the gate.

**Runtime dependencies** are deliberately minimal and reviewed — adding one is a decision, not a convenience. The current set, each confined to the package it serves:

| Dependency | Scope |
|---|---|
| **OpenTelemetry** | tracing, confined to `transport/` |
| **antlr4-go** | the AQL parser, `openehr/aql/parse` |
| **`golang.org/x/oauth2`**, **`github.com/coreos/go-oidc/v3`** | SMART/auth crypto correctness ([ADR 0009](docs/adr/0009-smart-auth-library-scope.md)) — scoped to `auth/` and `smart/` |
| **`go-jose/v4`** | required transitively by go-oidc; `auth/jwtbearer` + `smart` also import it directly for JWS signing |

Rationale and the wider picture: [architecture.md § Dependencies](docs/architecture.md#dependencies). Conformance probes (`testkit/probes/…`) run via `make test`; inventory in [conformance.md](docs/specifications/conformance.md).

**Recommended agent tooling:** the **go-coding** plugin (Go skills + the `go-reviewer` agent), **gopls-lsp** (code intelligence), and **codebase-memory-mcp** (structural exploration / impact) — see [ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor).

**Local agent config:** personal permission grants belong in the gitignored `.claude/settings.local.json` — never add a `permissions` block to the checked-in `.claude/settings.json` (shared hook/plugin config only).

## openEHR knowledge

Use the openEHR MCP skills before guessing RM paths, terminology codes, or ITS-JSON shapes — see [ai-workflow.md § openEHR ground truth](docs/ai-workflow.md#openehr-ground-truth-mcp--skills). The openEHR conformance probe suite is the source of truth for wire-level semantics; the openEHR spec is authoritative for class invariants.

**REST API schema.** For any endpoint path, request/response body, header, or status code, read the vendored OpenAPI pin in [`resources/its-rest/`](resources/its-rest/README.md) (`*-validation.openapi.yaml`) rather than guessing — it is the machine-readable contract. Refresh/verify with `make its-rest-sync` / `make its-rest-check`. [`resources/ehrbase/`](resources/ehrbase/README.md) holds EHRbase deployment extensions and is **not** the normative contract.

## Do not touch (yet)

- Promoting new numbered ADRs without updating [`docs/adr/README.md`](docs/adr/README.md), [`REQ.md`](docs/specifications/REQ.md), and [`traceability.yaml`](docs/specifications/traceability.yaml). Open decisions stay as [research strands](docs/specifications/research-strands.md) until an ADR lands.
- Duplicating normative REQ prose in `REQ.md` — the registry is index-only; canonical text lives in the topic specs.
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling, not public API; structural changes need rationale in [architecture.md](docs/architecture.md) and [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- The `go.mod` `go` directive — the minor line's `.0` patch, never the toolchain patch you happen to run (REQ-002): a mid-line floor makes every consumer and CI image on an earlier patch fetch a new toolchain, breaking air-gapped builds. Dev-image pins ([Dockerfile](Dockerfile)) move independently.
- REQ-NNN / PROBE-NNN / STRAND-NN identifiers — **stable** once published; never renumber or reuse.
