# AGENTS.md

**Entry point for every coding agent and contributor.** Pair with [`README.md`](README.md); Claude Code also loads [`.claude/CLAUDE.md`](.claude/CLAUDE.md) (Claude-specific notes only).

## Project

A first-party **Go SDK for openEHR** — `github.com/cadasto/openehr-sdk-go`, MIT. **openEHR-first**: openEHR REST `1.1.0-development`, the Reference Model, AQL, ADL 1.4 OPT, and SMART-on-openEHR auth are the normative scope. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care) ship in the same module for v1, behind a clean `cadasto/` cut line so later extraction is a subtree move, not a rewrite.

Go `1.27.x`, module floor `1.27.0` ([REQ-002](docs/specifications/packaging.md#req-002--go-version)). **Early implementation, pre-1.0** — status per REQ in [REQ.md](docs/specifications/REQ.md), open work in [docs/roadmap.md](docs/roadmap.md).

## Source of truth

The normative spec lives **in this repo** under [`docs/specifications/`](docs/specifications/) — self-contained; implementing or reviewing the SDK needs no external sources. Conventions (RFC-2119 keywords, status headers, identifiers, traceability): [`docs/specifications/README.md`](docs/specifications/README.md).

`docs/specifications/` carries the RFC-2119 statements code, plans, and tests are measured against; `docs/architecture.md` carries the design narrative. **When anything disagrees with the specs, the specs win.** Never silently resolve an open [research strand](docs/specifications/research-strands.md) in code — surface the decision or land an [ADR](docs/adr/).

## Documentation

Reading order — the specialized docs are **canonical**; defer to them rather than restating:

| # | Doc | Scope |
|---|---|---|
| 0 | [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) | **Developer onboarding** — install, integration paths, runnable `cmd/examples/` catalog |
| 1 | [docs/specifications/](docs/specifications/) | **Normative specs** — REQ index in [REQ.md](docs/specifications/REQ.md), PROBE in [conformance.md](docs/specifications/conformance.md), STRAND in [research-strands.md](docs/specifications/research-strands.md); machine map in [traceability.yaml](docs/specifications/traceability.yaml); process + descriptor in [development-process.md](docs/development-process.md) / [.sdd.yaml](docs/.sdd.yaml) |
| 2 | [docs/architecture.md](docs/architecture.md) | Design narrative — package organization, dependencies, integration, mermaid diagrams |
| 3 | [docs/ai-workflow.md](docs/ai-workflow.md) | **AI conventions** — the working loop, recommended plugins/skills, openEHR ground-truth lookups, hooks |
| 4 | [docs/adr/](docs/adr/) | Closed architectural decisions |
| 5 | [docs/plans/](docs/plans/) + [docs/roadmap.md](docs/roadmap.md) + [docs/backlog.md](docs/backlog.md) | Implementation plans, the open-work roadmap, and merged PRs' leftover suggestions (leads to verify, refreshed by `/sdd-triage --backlog` on `main`) |
| 6 | [CHANGELOG.md](CHANGELOG.md) + [docs/releases.md](docs/releases.md) | Release log and version policy |
| 7 | [CONTRIBUTING.md](CONTRIBUTING.md) + [SECURITY.md](SECURITY.md) | Contributor flow and vulnerability reporting |
| 8 | [LICENSING.md](LICENSING.md) | MIT grant plus in-tree third-party inventory |

### Spec-driven workflow (agents)

**Start with `make spec-context REQ=NNN`** — one bundle with the registry row, traceability block, canonical excerpt, and touching strands. **Finish with `make spec-check`** (`make ci` includes it). The step-by-step loop lives in [ai-workflow.md § The loop](docs/ai-workflow.md#the-loop); the rules that bind regardless of how you got there:

- New normative text goes in the **canonical topic spec** first, then its `traceability.yaml` entry (`make spec-gen` writes the registry row) — never as duplicate prose in `REQ.md`, and never as a rule that exists only in code.
- Cite `REQ-NNN` / `PROBE-NNN` in tests and maintainer comments (function bodies, unexported code); update `traceability.yaml` in the same change that lands the code, then `make spec-gen`. The REQ.md registry and each map row's `tests:` list (from the tests' REQ citations, [development-process.md § The ladder](docs/development-process.md#the-ladder-full-lane)) are generated, so never edit them by hand, and the map is a pure index: no notes, no comments.
- **Two lanes.** A change that alters no normative statement (refactor, move, perf, tooling, a fix that restores the spec'd behaviour) owes no spec, registry or plan edits — green `make ci` and one PR-body line. See [development-process.md § Two lanes](docs/development-process.md#two-lanes).
- **Godoc is for SDK users.** Package docs and doc comments on exported identifiers carry no spec-process identifiers (REQ, ADR, STRAND, plan or spec paths) and no RFC-2119 capitals. State the behaviour a caller needs in plain English. A probe's own `PROBE-NNN` id and public openEHR specification citations are fine.
- **`REQ`/`PROBE` is the feature register; there is no `SDK-GAP` identifier.** A discovered gap is worked under a REQ (extend or create via `sdd-specify`) with a `PROBE` for wire conformance. No GAP-style label appears anywhere — not in plan filenames, `traceability.yaml`, test names, `doc.go`, or normative prose ([ADR 0012](docs/adr/0012-retire-sdk-gap-identifier.md)).
- Keep [`cmd/examples/`](cmd/examples/) docs in sync **in the same PR** as the program — checklist in [ai-workflow.md § Examples](docs/ai-workflow.md#examples).

**Descriptor & process.** The `sdd-*` skills read [`docs/.sdd.yaml`](docs/.sdd.yaml) first; the lanes, the ladder and the Definition of Ready / Done are in [`docs/development-process.md`](docs/development-process.md), and the SDD-vs-superpowers split in [development-process.md § superpowers + SDD](docs/development-process.md#superpowers--sdd). Plans go in [`docs/plans/`](docs/plans/), never a parallel `docs/superpowers/` tree. They are committed working notes outside the traceability chain: no gate, generator or map reads them; brainstorming docs are narrative input, not a normative source.

## Module layout & boundaries

Full taxonomy and the package tree are in [module-layout.md](docs/specifications/module-layout.md) (normative) and [architecture.md](docs/architecture.md) (narrative). The **load-bearing rules** — a violation forfeits the option of extracting `cadasto/` later:

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports `cadasto/…`.
- No `cadasto/<X>` imports another `cadasto/<Y>` directly — share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART (`auth/smart`) and other providers on top.
- `internal/…` is consumer-invisible and excluded from semver promises.
- **Building-block independence (REQ-013):** the offline blocks (`openehr/rm`, `bmm`, `serialize/…`, `validation` + `validation/rmread`, `instance`, `composition`, `template`, `templatecompile`, `template/webtemplate`, `terminology`) and the AQL blocks (`openehr/aql`, `aql/parse`, `aql/lint`, `aql/contain`, `aql/internal/semcheck`) MUST NOT import `transport/`, `auth/` or `openehr/client/*`, directly or through any package they import. The template-side blocks also keep `openehr/serialize/` out of their own files. The canonical list and its `Test*ForbiddenImports` guards are in [module-layout.md § REQ-013](docs/specifications/module-layout.md#req-013--building-block-independence); a block with a narrower rule (for example `openehr/terminology`, stdlib-only below `openehr/rm`, REQ-034) states it in its own section.

## Code style and conventions

The elaborate, normative idiom spec is [`idiom.md`](docs/specifications/idiom.md) (context propagation, `*http.Client` injection, functional options, generics-no-reflection, errors, concurrency, naming, public-API stability) — read it. The quick version:

- **Format / lint:** `make fmt` (gofumpt + goimports via `golangci-lint fmt`) and `make lint` (golangci-lint v2 + `modernize` / `errorlint`), both pinned in the [Makefile](Makefile); `make ci` gates them.
- **Idioms:** `context.Context` first on every I/O method; inject `*http.Client` (never allocate one); functional options; package-level functions as the primary surface; generics only to remove a reflection hop; **no reflection** beyond the exceptions in [idiom.md § Generics policy (REQ-024)](docs/specifications/idiom.md#generics-policy-req-024), and no inheritance emulation (concrete structs + `typereg` for `_type` decoding).
- **Errors:** wrap with `fmt.Errorf("…: %w", err)`; typed sentinels at boundaries; no panics in library code.
- **JSON:** non-test code decodes RM and AOM 1.4 values with `encoding/json/v2` (or `canjson`), never v1 — v1's lenient options (duplicate names, case-insensitive matching, invalid UTF-8) would reach the generated decoders (REQ-052). `internal/v1_rm_decode_guard_test.go` scans for it.
- **Tests:** stdlib `testing` only — no assertion libraries — plus helpers in [`testkit/`](testkit/); behaviour tests for a public surface belong in the external `_test` package, so they exercise what consumers can reach. Guards carry the bar their spec sets — typically *removing the guard MUST fail a named test*.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) — scope is the touched area (`auth`, `rm`, `transport`, `client/ehr`, `docs`, `build`, …). AI-assisted commits carry an `Assisted-by:` trailer — see [CONTRIBUTING § AI-assisted contributions](CONTRIBUTING.md#ai-assisted-contributions).
- **Skills:** before writing or reviewing Go, load `go-coding:go-coding` then the focused skill matching the diff — `go-errors` for error paths, `go-testing` for any `_test.go`, `go-idioms` when modernizing or touching loops/maps/strings, `go-concurrency` for goroutines, channels, or context lifetimes, `go-layout` for a new package or exported API; the router alone doesn't count. An orchestrator dispatching implementer or reviewer subagents MUST carry this instruction in every brief — subagents don't inherit the session's skills. Detail: [ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor).

**CHANGELOG.md** — update only on request or when cutting a release. **One single-sentence bullet (~35 words max) per artefact class**: artefact + scope + key REQ/PROBE, never API inventories or per-REQ breakdowns (those live in `traceability.yaml`, commits, and PR bodies); the optional per-release summary line is one sentence too, plus a `**Breaking:**` sentence naming each change [`module-layout.md` § Versioning](docs/specifications/module-layout.md#versioning) maps to major. Release notes are generated verbatim from the block, so a long entry is a defect — err short. Sections, in this order and only when they have entries: `### Added` for new capability; `### Changed` for behaviour a caller may have to adapt to (a tightened check, a different output, a different error); `### Deprecated` and `### Removed` for public symbols. No `### Fixed`: a fix of unreleased work folds into its bullet, and a fix of released behaviour goes under `### Changed` when callers may have to adapt, otherwise into its artefact class's `### Added` bullet.

## Tooling & workflow

Host Go `1.27.x` is the fast path; the Makefile auto-routes through a Docker dev image when host Go is missing ([Dockerfile](Dockerfile), [docker-compose.yml](docker-compose.yml)). **Use the Makefile as the single entry point** — extend it, don't add ad-hoc scripts.

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
| Regenerate spec indexes | `make spec-gen`: the map's `tests:` lists from the tests' REQ citations, the REQ.md registry from `traceability.yaml` |
| Spec context bundle | `make spec-context REQ=NNN` — registry row + traceability + canonical excerpt + strands |
| Probe status | `make probe-status` — each PROBE's status and whether its test file exists |
| FLAT corpus integrity | `make flat-conformance-verify` — offline `sha256` of the vendored EHRbase FLAT corpus (PROBE-086's input); `…-check` adds a network drift report (dev helper, not a gate) |
| Build Docker dev image | `make image-dev` (only when host Go is missing) |
| Docs site | `make docs-check` builds the site from `pages/` and asserts its output; `make docs-serve` previews it. The docs-theme brand layer is pinned by commit + sha256 in `sources.json` (bump recipe in its `$comment`) |

**Gotchas worth the reading:**

- **Never hand-edit a vendored fixture** under `resources/` or `testkit/corpus/` — being byte-identical to upstream is its whole value. Re-sync instead.
- `make probe-status` prints `MISSING` for any probe covered inline or in a sibling's file. That is the filename heuristic, **not** drift — `make spec-check` is the real gate.
- **`make ci` cannot complete without Docker:** `test` → `aqlgen-verify` → `antlr-image` always needs it. Everything else uses Docker only when host tooling is missing (host Go `1.27.x`; a host `golangci-lint` **built with Go 1.27**; routing in [ci.md](docs/ci.md)).
  Without Docker, run `fmt-check`, `vet`, `spec-check`, `flat-conformance-verify`, `build` and `go test ./... -count=1`, and let PR CI be the gate.
- After `git worktree remove`, run `golangci-lint cache clean`; otherwise lint reports phantom issues under the removed `.worktrees/` path.
- **Public docs never name downstream consuming projects.** Describe what a consumer needs, not who the consumer is.
- **Release cut** follows [releases.md § Tag checklist](docs/releases.md#tag-checklist) and commits straight to `main`, no branch or PR. The same commit moves the `go get …@vX.Y.Z` pin in `README.md`, `docs/quick-start.md`, `pages/install.md` and `pages/index.md`; `make docs-check` fails until all four match the newest `## [X.Y.Z]` heading in `CHANGELOG.md`.

**Runtime dependencies** are deliberately minimal and reviewed — adding one is a decision, not a convenience. The current set, each confined to the package it serves:

| Dependency | Scope |
|---|---|
| **OpenTelemetry** | tracing, confined to `transport/` |
| **antlr4-go** | the AQL parser, `openehr/aql/parse` |
| **`golang.org/x/oauth2`**, **`github.com/coreos/go-oidc/v3`** | SMART/auth crypto correctness ([ADR 0009](docs/adr/0009-smart-auth-library-scope.md)) — scoped to `auth/` and `smart/` |
| **`go-jose/v4`** | required transitively by go-oidc; `auth/jwtbearer` + `smart` also import it directly for JWS signing |

Rationale and the wider picture: [architecture.md § Dependencies](docs/architecture.md#dependencies). Conformance probes (`testkit/probes/…`) run via `make test`; inventory in [conformance.md](docs/specifications/conformance.md).

**Agent tooling:** a review of a Go diff goes through the plugin's `go-reviewer` agent, or the reviewer loads the same focused skills itself when the workflow already provides a single reviewer seat (the SDD subagent-driven loop does). `go-lint-setup` is never needed here: golangci-lint v2 is already pinned (`make lint`). Also: **gopls-lsp** (code intelligence) and **codebase-memory-mcp** (structural exploration / impact) — see [ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor).

**Local agent config:** personal permission grants belong in the gitignored `.claude/settings.local.json` — never add a `permissions` block to the checked-in `.claude/settings.json` (shared hook/plugin config only).

## openEHR knowledge

Use the openEHR MCP skills before guessing RM paths, terminology codes, or ITS-JSON shapes — see [ai-workflow.md § openEHR ground truth](docs/ai-workflow.md#openehr-ground-truth-mcp--skills). The openEHR conformance probe suite is the source of truth for wire-level semantics; the openEHR spec is authoritative for class invariants.

**REST API schema.** For any endpoint path, request/response body, header, or status code, read the vendored OpenAPI pin in [`resources/its-rest/`](resources/its-rest/README.md) (`*-validation.openapi.yaml`) rather than guessing — it is the machine-readable contract. Refresh/verify with `make its-rest-sync` / `make its-rest-check`. EHRbase-specific deployment extensions that a call still uses (for example `PurgeTemplates`) are documented on that call and in REQ-099, not by a second OpenAPI pin. In-tree third-party licences are inventoried in [`LICENSING.md`](LICENSING.md).

## Do not touch (yet)

- Promoting new numbered ADRs without updating [`docs/adr/README.md`](docs/adr/README.md) and naming the REQs they amend in their header (the map lists no ADRs). Open decisions stay as [research strands](docs/specifications/research-strands.md) until an ADR lands.
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling, not public API; structural changes need rationale in [architecture.md](docs/architecture.md) and [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- The `go.mod` `go` directive — the minor line's `.0` patch, never the toolchain patch you happen to run (REQ-002): a mid-line floor makes every consumer and CI image on an earlier patch fetch a new toolchain, breaking air-gapped builds. Dev-image pins ([Dockerfile](Dockerfile)) move independently.
- REQ-NNN / PROBE-NNN / STRAND-NN identifiers — **stable** once published; never renumber or reuse.
