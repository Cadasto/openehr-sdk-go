# AGENTS.md

**Entry point for every coding agent and contributor.** Pair with [`README.md`](README.md). Claude Code loads this file at session start only while the repository has no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md`, so do not add one: Claude-specific notes go in § Tooling & workflow.

## Project

A first-party **Go SDK for openEHR** — `github.com/cadasto/openehr-sdk-go`, MIT. **openEHR-first**: openEHR REST `1.1.0`, the Reference Model, AQL, ADL 1.4 OPT, and SMART-on-openEHR auth are the normative scope. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care) ship in the same module for v1, behind a clean `cadasto/` cut line so later extraction is a subtree move, not a rewrite.

Go `1.27.x`, module floor `1.27.0` ([releases.md § Minimum Go version](docs/releases.md#minimum-go-version)). **Pre-1.0**: a minor release can change the public API ([releases.md § Pre-1.0](docs/releases.md#pre-10)). Status per REQ in [REQ.md](docs/specifications/REQ.md), open work in [docs/roadmap.md](docs/roadmap.md).

## Source of truth

The normative spec lives **in this repo** under [`docs/specifications/`](docs/specifications/) — self-contained; implementing or reviewing the SDK needs no external sources. Conventions (RFC-2119 keywords, status headers, identifiers, traceability): [`docs/specifications/README.md`](docs/specifications/README.md).

`docs/specifications/` carries the RFC-2119 statements code, plans, and tests are measured against; `docs/architecture.md` carries the design narrative. **The specs record the contract; the maintainer's instruction outranks them.** When a request contradicts a sentence, name the sentence once, do the work and amend it in the same change. Without the maintainer, anything that disagrees with the specs is a finding and the specs win, except a section marked *implementation-aligned*, where code leads and the spec is reconciled in the same PR. A `draft` section binds once it merges. Never silently resolve an open [research strand](docs/specifications/research-strands.md) in code — surface the decision or land an [ADR](docs/adr/).

## Documentation

Reading order — the specialized docs are **canonical**; defer to them rather than restating:

| # | Doc | Scope |
|---|---|---|
| 0 | [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) | **Developer onboarding** — install, integration paths, runnable `cmd/examples/` catalog |
| 1 | [docs/specifications/](docs/specifications/) | **Normative specs** — REQ index in [REQ.md](docs/specifications/REQ.md), PROBE in [conformance.md](docs/specifications/conformance.md), STRAND in [research-strands.md](docs/specifications/research-strands.md), retired REQ in [retired-requirements.md](docs/specifications/retired-requirements.md); machine map in [traceability.yaml](docs/specifications/traceability.yaml); process + descriptor in [development-process.md](docs/development-process.md) / [.sdd.yaml](docs/.sdd.yaml) |
| 2 | [docs/architecture.md](docs/architecture.md) | Design narrative — package organization, dependencies, integration, mermaid diagrams |
| 3 | [docs/ai-workflow.md](docs/ai-workflow.md) | **AI conventions** — the working loop, recommended plugins/skills, openEHR ground-truth lookups, review and findings |
| 4 | [docs/adr/](docs/adr/) | Closed architectural decisions |
| 5 | [docs/plans/](docs/plans/) + [docs/roadmap.md](docs/roadmap.md) + [docs/backlog.md](docs/backlog.md) | Implementation plans, the open-work roadmap, and merged branches' leftover defects and suggestions (leads to verify before acting) |
| 6 | [CHANGELOG.md](CHANGELOG.md) + [docs/releases.md](docs/releases.md) | Release log and version policy |
| 7 | [CONTRIBUTING.md](CONTRIBUTING.md) + [SECURITY.md](SECURITY.md) | Contributor flow and vulnerability reporting |
| 8 | [LICENSING.md](LICENSING.md) | MIT grant plus in-tree third-party inventory |

### Spec-driven workflow (agents)

The sdd plugin carries the method. This repository's form of it is in [development-process.md](docs/development-process.md) (two lanes, the ladder, Definition of Ready / Done), [specifications/README.md](docs/specifications/README.md) (conventions) and [`docs/.sdd.yaml`](docs/.sdd.yaml), and the loop in [ai-workflow.md § The loop](docs/ai-workflow.md#the-loop). **Start with `make spec-context REQ=NNN`** (registry row, traceability block, canonical excerpt, touching strands); **finish with `make spec-check`** (`make ci` includes it). Rules the plugin does not know, or that reverse its defaults:

- Normative text lives once, in its canonical topic spec. `REQ.md` and each map row's `tests:` list are generated (`make spec-generate`), `traceability.yaml` is a pure index, and tests and maintainer comments cite `REQ-NNN` / `PROBE-NNN`.
- **Godoc is for SDK users.** Package docs and doc comments on exported identifiers carry no spec-process identifiers (REQ, ADR, STRAND, plan or spec paths) and no RFC-2119 capitals; a probe's own `PROBE-NNN` and public openEHR specification citations are fine. Error text an SDK user sees carries no spec-process identifiers either; maintainer comments may cite them.
- No `SDK-GAP` identifier: a gap is worked under a REQ with a PROBE ([ADR 0012](docs/adr/0012-retire-sdk-gap-identifier.md)).
- Keep [`cmd/examples/`](cmd/examples/) docs in sync **in the same PR** as the program ([ai-workflow.md § Examples](docs/ai-workflow.md#examples)).
- Plans go in [`docs/plans/`](docs/plans/), never superpowers' default `docs/superpowers/` tree ([development-process.md § superpowers + SDD](docs/development-process.md#superpowers--sdd)).
- **Backlog size.** [`docs/backlog.md`](docs/backlog.md) is kept at about 80 lines ([ai-workflow.md § Review](docs/ai-workflow.md#review)), which outranks the 40 built into the sdd plugin's hook and its `/sdd-triage --backlog` consolidation: the hook's session-start line "over the 40 it should hold" is informational until the count passes 80.
- **Retiring a requirement.** Its entry moves to [retired-requirements.md](docs/specifications/retired-requirements.md) and the map row's `canonical` points there ([specifications/README.md § Editing rules](docs/specifications/README.md#editing-rules)). This outranks the sdd plugin's default, which `/sdd-specify` otherwise follows, of keeping the section in its topic spec.
- **Write short.** Lead with the result; a finding is one sentence of about 30 words, a PR body about 450 words; quote only the output that proves a claim.

## Module layout & boundaries

Full taxonomy and the package tree are in [module-layout.md](docs/specifications/module-layout.md) (normative) and [architecture.md](docs/architecture.md) (narrative). The **load-bearing rules** — a violation forfeits the option of extracting `cadasto/` later:

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports `cadasto/…`.
- No `cadasto/<X>` imports another `cadasto/<Y>` directly — share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART (`auth/smart`) and other providers on top.
- `internal/…` is consumer-invisible and excluded from semver promises.
- **Building-block independence (REQ-013):** the offline and AQL building blocks never import `transport/`, `auth/` or `openehr/client/*`, directly or through any package they import. The block list and each block's own import rule are in the table in [module-layout.md § REQ-013](docs/specifications/module-layout.md#req-013--building-block-independence).

## Code style and conventions

Go rules live in [`idiom.md`](docs/specifications/idiom.md) (normative) and the go-coding plugin. Before writing or reviewing Go, load `go-coding:go-coding` and then the focused skill for the diff; an orchestrator names them in every subagent brief, because subagents don't inherit skills ([ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor)). Where the plugin's general advice differs, this repository's rule wins:

- **I/O surface:** `context.Context` first, all network I/O through the caller's `*http.Client`, functional options on every constructor, and no reflection beyond [idiom.md § API conventions](docs/specifications/idiom.md#api-conventions).
- **JSON:** non-test code decodes RM and AOM 1.4 values with `encoding/json/v2` (or `canjson`), never v1 (REQ-052); `internal/v1_rm_decode_guard_test.go` scans for it.
- **Tests:** stdlib `testing` and [`testkit/`](testkit/) helpers only, no assertion libraries; behaviour tests for a public surface go in the external `_test` package.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/), scoped to the touched area (`auth`, `rm`, `transport`, `client/ehr`, `docs`, `build`, …); AI-assisted commits end with an `Assisted-by:` trailer ([CONTRIBUTING § AI-assisted contributions](CONTRIBUTING.md#ai-assisted-contributions)).

**CHANGELOG.md** — update only on request or when cutting a release, and read the format rule in [releases.md § CHANGELOG](docs/releases.md#changelog) first: one single-sentence bullet per artefact class, no `### Fixed`.

## Tooling & workflow

Host Go `1.27.x` is the fast path; the Makefile auto-routes through a Docker dev image when host Go is missing, and `make test-race` also when the host has no C compiler ([Dockerfile](Dockerfile), [docker-compose.yml](docker-compose.yml)). **Use the Makefile as the single entry point** — extend it, don't add ad-hoc scripts.

| Task | Command |
|---|---|
| Discover all targets | `make help` — by section, from the Makefile's `##@ Section` and `target: ## description` annotations ([`make/help.mk`](make/help.mk) renders them) |
| Diagnose the environment | `make doctor` — host Go, Docker, and which toolchain the Makefile will actually use |
| **Full PR / CI gate** | `make ci` — see [docs/ci.md](docs/ci.md) |
| Format / check | `make fmt` / `make fmt-check` |
| Vet / lint | `make vet` / `make lint` |
| Unit / race tests | `make test` / `make test-race` |
| One package, fast loop | `go test ./openehr/serialize/canjson/... -run TestName -count=1`; skips the codegen checks `make test` runs first, so `make ci` stays the gate |
| BMM codegen verify | `make codegen-check`; `make bmm-check` verifies the pinned schemas' `sha256` (offline), `make bmm-drift` reports ITS-BMM changes |
| AQL parser codegen verify | `make aqlgen-check` — fails if `openehr/aql/parse/gen/` drifts from the `active/` grammar (needs Docker, not a host JRE); regenerate with `make aqlgen` |
| Spec traceability | `make spec-check` |
| Full gate report | `make sdd-check` — the SDD gate with every warning (`make spec-check` prints only the verdict) |
| Regenerate spec indexes | `make spec-generate`: the map's `tests:` lists from the tests' REQ citations, the REQ.md registry from `traceability.yaml` |
| Spec context bundle | `make spec-context REQ=NNN` — registry row + traceability + canonical excerpt + strands |
| Probe status | `make probe-status` — each PROBE's status and whether its test file exists |
| Live CDR (opt-in, not in `make ci`) | `make probe-record ARGS=…` records a cassette; `make cdr-crosscheck` checks generated documents against live CDRs. Environment variables are in [ci.md](docs/ci.md) |
| FLAT corpus integrity | `make flat-check` — offline `sha256` of the vendored EHRbase FLAT corpus (PROBE-086's input); `make flat-drift` adds a network drift report (dev helper, not a gate). `make corpus-pins-check` holds every EHRbase corpus to the one commit per repository in `testkit/corpus/UPSTREAM_PINS.txt` |
| Build Docker dev image | `make build-dev` (when host Go is missing, or for `make test-race` on a host without a C compiler; rebuild an image made before the dev stage gained gcc) |
| Docs site | `make docs-check` builds the site from `pages/` and asserts its output; `make docs-serve` previews it. The docs-theme brand layer is pinned by commit + sha256 in `sources.json` (bump recipe in its `$comment`) |

**Gotchas worth the reading:**

- **Never hand-edit a vendored fixture** under `resources/` or `testkit/corpus/` — being byte-identical to upstream is its whole value. Re-sync instead.
- `make probe-status` prints `MISSING` for any probe covered inline or in a sibling's file. That is the filename heuristic, **not** drift — `make spec-check` is the real gate.
- **`make ci` cannot complete without Docker:** `test` → `aqlgen-check` → `build-antlr` always needs it; everything else falls back to Docker only when a host tool is missing or unsuitable (routing in [ci.md](docs/ci.md)).
  Without Docker, run `fmt-check`, `vet`, `spec-check`, `flat-check`, `build` and `go test ./... -count=1`, and let PR CI be the gate.
- Worktrees go under the gitignored `.worktrees/`. After `git worktree remove`, run `golangci-lint cache clean`; otherwise lint reports phantom issues under the removed `.worktrees/` path.
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

**Agent tooling:**

- **Go review:** the plugin's `go-reviewer` agent, or the reviewer loads the same focused skills itself when the workflow already provides a single reviewer seat (the SDD loop does). `go-lint-setup` is never needed: golangci-lint v2 is already pinned.
- **Code intelligence:** **gopls** (from a worktree, run it in the shell) and **codebase-memory-mcp** (structure and impact questions about `main`); see [ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor).
- **Claude Code hook:** after a **Write** or **Edit** on a `*.go` file, [`.claude/settings.json`](.claude/settings.json) runs [`.claude/hooks/goformat-on-save.sh`](.claude/hooks/goformat-on-save.sh) (gofumpt + goimports on that file; `gofmt -s` fallback; no-op without a formatter; skips `*_gen.go`). `make fmt` is the full-tree pass.
- **Plugins:** `.claude/settings.json` enables go-coding, sdd, superpowers, gopls-lsp, pr-review-toolkit, feature-dev and claude-md-management; look there before assuming a skill is missing.
- **Local config:** personal permission grants go in the gitignored `.claude/settings.local.json`; never add a `permissions` block to the checked-in `.claude/settings.json`.

## openEHR knowledge

The openehr-assistant plugin and its MCP tools answer RM paths, class definitions, terminology codes and ITS-JSON shapes: look them up rather than guess ([ai-workflow.md § openEHR ground truth](docs/ai-workflow.md#openehr-ground-truth-mcp--skills)). This repository's pinned contracts outrank a general answer:

- **Wire semantics:** the conformance probe suite ([conformance.md](docs/specifications/conformance.md)).
- **REST:** the vendored OpenAPI pin in [`resources/its-rest/`](resources/its-rest/README.md) (`*-validation.openapi.yaml`) for every path, body, header and status code.
- **XML:** the pinned ITS-XML schemas in [`resources/its-xml/`](resources/its-xml/README.md), which the generator reads for child-element order; they declare the `v2` namespace where the SDK writes `v1`.
- **Classes and invariants:** the openEHR specifications, as the pinned BMM schemas in [`resources/bmm/`](resources/bmm/) the generator reads.

## Do not touch (yet)

- Promoting new numbered ADRs without updating [`docs/adr/README.md`](docs/adr/README.md) and naming the REQs they amend in their header (the map lists no ADRs). Open decisions stay as [research strands](docs/specifications/research-strands.md) until an ADR lands.
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling, not public API; structural changes need rationale in [architecture.md](docs/architecture.md) and [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- The `go.mod` `go` directive — the minor line's `.0` patch, never the toolchain patch you happen to run ([releases.md § Minimum Go version](docs/releases.md#minimum-go-version)): a mid-line floor makes every consumer and CI image on an earlier patch fetch a new toolchain, breaking air-gapped builds. Dev-image pins ([Dockerfile](Dockerfile)) move independently.
- REQ-NNN / PROBE-NNN / STRAND-NN identifiers — **stable** once published; never renumber or reuse.
