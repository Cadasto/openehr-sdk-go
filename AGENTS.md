# AGENTS.md

**Entry point for every coding agent and contributor.** Pair with [`README.md`](README.md). Claude Code loads this file at session start only while the repository has no `CLAUDE.md`, `.claude/CLAUDE.md` or `CLAUDE.local.md`, so do not add one: Claude-specific notes go in § Tooling & workflow.

## Project

A first-party **Go SDK for openEHR** — `github.com/cadasto/openehr-sdk-go`, MIT. **openEHR-first**: openEHR REST `1.1.0`, the Reference Model, AQL, ADL 1.4 OPT, and SMART-on-openEHR auth are the normative scope. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care) ship in the same module for v1, behind a clean `cadasto/` cut line so later extraction is a subtree move, not a rewrite.

Go `1.27.x`, module floor `1.27.0` ([releases.md § Minimum Go version](docs/releases.md#minimum-go-version)). **Pre-1.0**: a minor release can change the public API ([releases.md § Pre-1.0](docs/releases.md#pre-10)). Status per REQ in [REQ.md](docs/specifications/REQ.md), open work in [docs/roadmap.md](docs/roadmap.md).

## Source of truth

The normative spec lives **in this repo** under [`docs/specifications/`](docs/specifications/), self-contained: implementing or reviewing the SDK needs no external sources. Its conventions (RFC-2119 keywords, status headers, identifiers, traceability) are in [`docs/specifications/README.md`](docs/specifications/README.md).

- `docs/specifications/` carries the RFC-2119 statements that code, plans and tests are measured against; [`docs/architecture.md`](docs/architecture.md) carries the design narrative.
- **The specs record the contract; the maintainer's instruction outranks them.** When a request contradicts a sentence, name the sentence once, do the work and amend it in the same change.
- Without the maintainer, anything that disagrees with the specs is a finding, and the specs win.
- A `draft` section binds once it merges. No specification here is marked *implementation-aligned*, the sdd plugin's mode in which code leads and the spec is reconciled in the same PR.
- Never silently resolve an open [research strand](docs/specifications/research-strands.md) in code: surface the decision or land an [ADR](docs/adr/).

## Where to look

The specialised docs are **canonical**: defer to them rather than restating.

| When you… | Read |
|---|---|
| use the SDK | [quick-start.md](docs/quick-start.md), [examples.md](docs/examples.md) and the runnable [`cmd/examples/`](cmd/examples/) |
| fix a bug or build a capability | the [REQ registry](docs/specifications/REQ.md) to find the REQ, then its bundle (§ Spec-driven workflow) |
| decide the lane of a change | [development-process.md](docs/development-process.md) |
| move a package or change its imports | [module-layout.md](docs/specifications/module-layout.md), then [architecture.md](docs/architecture.md) for the narrative |
| write or run a conformance probe (probes run under `make test`) | [conformance.md](docs/specifications/conformance.md) for the catalogue, [probe-harness.md](docs/probe-harness.md) for the harness |
| meet an open question | [research-strands.md](docs/specifications/research-strands.md) and [docs/adr/](docs/adr/) |
| orchestrate agents, brief a worker or review a branch | [ai-workflow.md](docs/ai-workflow.md) |
| pick up open work | [roadmap.md](docs/roadmap.md), and [backlog.md](docs/backlog.md) for leads to verify before acting |
| run the gate | [ci.md](docs/ci.md) |
| edit the Makefile or a script | § Tooling & workflow (portability), then [ci.md](docs/ci.md) |
| commit, push or open a pull request | § Git and pull requests |
| cut a release or edit the CHANGELOG | [releases.md](docs/releases.md) |
| check a spec convention (keywords, kinds, identifiers, editing rules) | [specifications/README.md](docs/specifications/README.md) |
| contribute without an agent | [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), [LICENSING.md](LICENSING.md) |

## Before you hand back

Check every change against this list before reporting it done:

1. **The gate passes:** `make ci`, or the Docker-free subset in § Tooling & workflow. Say which platform it ran on (macOS arm64, Debian or WSL amd64, or CI): a check on one does not cover the others.
2. **The spec indexes are current:** after a test's `REQ-NNN` citations change, `make spec-generate` has run and what it wrote is committed; `make spec-check` passes.
3. **SDK users see no process ids,** and **a changed example ships with its docs** (both in § Spec-driven workflow).
4. **A changed Makefile or script still runs** on GNU Make 3.81, under `dash`, and on bash 3.2 (§ Tooling & workflow).
5. **Commits follow § Git and pull requests,** and `CHANGELOG.md` is untouched unless you are cutting a release or were asked to.

## Spec-driven workflow (agents)

The sdd plugin carries the method. This repository's form of it is in [development-process.md](docs/development-process.md) (two lanes, the ladder), [specifications/README.md](docs/specifications/README.md) (conventions) and [`docs/.sdd.yaml`](docs/.sdd.yaml), and the loop in [ai-workflow.md § The loop](docs/ai-workflow.md#the-loop).
**Start small:** `make spec-context REQ=NNN OUTLINE=1` prints the registry row, the map entry, the plans, ADRs and strands that name the REQ, and the canonical section's outline with each rule's name and size; then `make spec-context REQ=NNN RULE=<name>` prints the one rule the task needs.
Without either flag the bundle prints the whole section, which runs to about 80 KB for the largest.
**Finish with `make spec-check`** (`make ci` includes it).
Rules the plugin does not know, or that reverse its defaults:

- Normative text lives once, in its canonical topic spec. `traceability.yaml` is a pure index, and `REQ.md` and each map row's `tests:` list are generated from the tests' REQ citations ([development-process.md § The ladder](docs/development-process.md#the-ladder-full-lane)).
- **Amend in place.** A change edits the sentence that already holds its topic and adds no paragraph, note or new document beside it. A new ADR records only a decision no ADR or spec section covers; an accepted ADR's stale part is rewritten, with one `**Amended:**` line ([adr/README.md § Status](docs/adr/README.md#status)).
- **What SDK users read carries no process.**
  - What pkg.go.dev shows (package docs, doc comments on exported identifiers, the values of exported variables and constants, examples, README files) carries no spec-process identifiers (REQ, ADR, STRAND, plan or spec paths) and no RFC-2119 capitals; a probe's own `PROBE-NNN` and public openEHR specification citations are fine.
  - User-facing error text, `docs/quick-start.md`, `docs/examples.md` and the site under `pages/` carry no REQ, ADR, STRAND or PROBE id either.
  - Maintainer comments may cite them.
  - `TestGodocCarriesNoSpecProcessIdentifiers` ([`internal/godoc_ids_guard_test.go`](internal/godoc_ids_guard_test.go)) fails on a REQ, ADR or STRAND id in a package comment or the doc comment of an exported identifier in any non-test Go file, `internal/`, `cmd/` and `testkit/probes/` included. Put the citation in a comment go doc does not show, such as one in the function body.
- No `SDK-GAP` identifier: a gap is worked under a REQ with a PROBE.
- Keep [`cmd/examples/`](cmd/examples/) docs in sync **in the same PR** as the program ([ai-workflow.md § Examples](docs/ai-workflow.md#examples)); a new program fails `TestExampleDirectoryCensus` until `transcripts_test.go` classifies it.
- Plans go in [`docs/plans/`](docs/plans/), never superpowers' default `docs/superpowers/` tree ([development-process.md § superpowers + SDD](docs/development-process.md#superpowers--sdd)).
- **Backlog size.** [`docs/backlog.md`](docs/backlog.md) is kept at about 80 `- ` entries, not the sdd plugin's 40: the session hook's "over the 40" line is informational until the count passes 80.
- **Retiring a requirement** moves its entry to [retired-requirements.md](docs/specifications/retired-requirements.md) and deletes its `traceability.yaml` row ([specifications/README.md § Editing rules](docs/specifications/README.md#editing-rules)), not the sdd plugin's default of keeping the section and a `retired` row. That page is the only place a retired number appears, and `make spec-check` refuses a row that reuses one; a test that cited it is re-cited to the requirement that took its rule.
- **Write short.** Lead with the result; a finding is one sentence of about 30 words, a PR body about 450 words; quote only the output that proves a claim. A specification sentence over about 60 words that a change touches is split in that change: a chain of conditions becomes a list of condition and behaviour, or a small table, saying no more and no less than before.

## Module layout & boundaries

Full taxonomy and the package tree are in [module-layout.md](docs/specifications/module-layout.md) (normative) and [architecture.md](docs/architecture.md) (narrative). The **load-bearing rules** — a violation forfeits the option of extracting `cadasto/` later:

- Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` imports `cadasto/…`.
- No `cadasto/<X>` imports another `cadasto/<Y>` directly — share through openEHR-core types or interface contracts.
- `auth/` is layered: generic `TokenSource` at the bottom; SMART (`auth/smart`) and other providers on top.
- `internal/…` is consumer-invisible and excluded from semver promises.
- **Building-block independence (REQ-013):** the offline and AQL building blocks never import `transport/`, `auth/` or `openehr/client/*`, directly or through any package they import. The block list and each block's own import rule are in the table in [module-layout.md § REQ-013](docs/specifications/module-layout.md#req-013--building-block-independence).

## Code style and conventions

Go rules live in [`idiom.md`](docs/specifications/idiom.md) (normative) and the go-coding plugin, whose skills are loaded before writing or reviewing Go as [ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor) says. Where the plugin's general advice differs, this repository's rule wins:

- **I/O surface:** `context.Context` first, all network I/O through the caller's `*http.Client`, functional options on every constructor, and no reflection beyond [idiom.md § API conventions](docs/specifications/idiom.md#api-conventions).
- **No panics:** library code never panics on wire or caller input; it returns an error or fails closed. Only a `Must`-prefixed function may panic, beside a non-panicking alternative ([idiom.md § No panics](docs/specifications/idiom.md#no-panics)).
- **JSON:** non-test code decodes RM and AOM 1.4 values with `encoding/json/v2` (or `canjson`), never v1 (REQ-052); `internal/v1_rm_decode_guard_test.go` scans for it.
- **Tests:** stdlib `testing` and [`testkit/`](testkit/) helpers only, no assertion libraries; behaviour tests for a public surface go in the external `_test` package.
- **Commits** follow § Git and pull requests.

**CHANGELOG.md** — update only on request or when cutting a release, and read the format rule in [releases.md § CHANGELOG](docs/releases.md#changelog) first: one single-sentence bullet per artefact class, no `### Fixed`.

## Git and pull requests

- **Branches:** start each change on a branch from an up-to-date `origin/main`, named after its Conventional Commit type (`feat/…`, `fix/…`, `docs/…`). When `main` moves, rebase onto it; never merge it in.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/), scoped to the touched area (`auth`, `rm`, `transport`, `client/ehr`, `docs`, `build`, …), staging files by path. AI-assisted commits end with an `Assisted-by:` trailer ([CONTRIBUTING § AI-assisted contributions](CONTRIBUTING.md#ai-assisted-contributions)).
- **Pull requests:** open every PR as a draft; `/sdd-close` marks it ready when it pushes the close-out. The body follows [`.github/pull_request_template.md`](.github/pull_request_template.md): about 450 words, its Summary ending with the `Lane:` line ([development-process.md § The PR body](docs/development-process.md#the-pr-body)).
- **The maintainer merges.** An agent never merges or closes a pull request and never deletes a remote branch.
- **In anything posted to GitHub,** write a pull request or issue as a link, never a bare `#N`, which GitHub links to whatever has that number.
- **Worktrees** go under the gitignored `.worktrees/`. After `git worktree remove`, run `golangci-lint cache clean`; otherwise lint reports phantom issues under the removed path.
- **Release cut** follows [releases.md § Tag checklist](docs/releases.md#tag-checklist) and commits straight to `main`, no branch or PR. The same commit moves the `go get …@vX.Y.Z` pin in `README.md`, `docs/quick-start.md`, `pages/install.md` and `pages/index.md`; `make docs-check` fails until all four match the newest `## [X.Y.Z]` heading in `CHANGELOG.md`.

## Tooling & workflow

Host Go `1.27.x` is the fast path; the Makefile auto-routes through a Docker dev image when host Go is missing, and `make test-race` also when the host has no C compiler ([Dockerfile](Dockerfile), [docker-compose.yml](docker-compose.yml)). **Use the Makefile as the single entry point** — extend it, don't add ad-hoc scripts.

| Task | Command |
|---|---|
| Discover every target | `make help`, by section; `make doctor` shows the toolchain the Makefile will actually use |
| **Full PR / CI gate** | `make ci` — see [docs/ci.md](docs/ci.md) |
| One package, fast loop | `go test ./openehr/serialize/canjson/... -run TestName -count=1`; skips the codegen checks `make test` runs first, so `make ci` stays the gate |
| Spec gate | `make spec-check` prints the verdict; `make sdd-check` prints every warning |
| Regenerate spec indexes | `make spec-generate`: the map's `tests:` lists, the REQ.md registry and the ADR index |
| Spec context bundle | `make spec-context REQ=NNN OUTLINE=1`, then `RULE=<name>` (§ Spec-driven workflow); `PROBE=NNN` prints one probe's catalog entry |
| Keyword diff against a base | `make spec-kwdiff BASE=<ref>`; `CHECK=1` fails when a file's keyword counts change |

**Gotchas worth the reading:**

- **Never hand-edit a vendored fixture** under `resources/` or `testkit/corpus/` — being byte-identical to upstream is its whole value. Re-sync instead. CODE24's own fixtures have no upstream to re-sync from, so they may be edited; record each edit in [`testkit/corpus/THIRD_PARTY_LICENSES.md`](testkit/corpus/THIRD_PARTY_LICENSES.md), except the removal of personal or account details, which is made without a note.
- `make probe-status` prints `MISSING` for any probe covered inline or in a sibling's file. That is the filename heuristic, **not** drift — `make spec-check` is the real gate.
- A test file (`*_test.go`, or any `.go` file under `testkit/probes/`) whose set of cited `REQ-NNN` ids changes, by a comment edit too, makes `make spec-check` fail with "tests list is stale" until `make spec-generate` rewrites the map's `tests:` list; commit what it writes. A citation counts only as `REQ-NNN`, with its hyphen.
- A non-test file that imports `reflect` fails `TestREQ024ReflectOnlyForReviewedUses` until it is added to the reviewed list in [`internal/reflect_guard_test.go`](internal/reflect_guard_test.go), for a use [idiom.md § API conventions](docs/specifications/idiom.md#api-conventions) allows, such as a typed-nil check.
- **`make ci` cannot complete without Docker:** `test` → `aqlgen-check` → `build-antlr` always needs it; everything else falls back to Docker only when a host tool is missing or unsuitable (routing in [ci.md](docs/ci.md)).
  Without Docker, run `fmt-check`, `vet`, `spec-check`, `flat-check`, `build` and `go test ./... -count=1`, and let PR CI be the gate.
- **Portability.** The Makefile and scripts run on macOS (GNU Make 3.81 as `/usr/bin/make`, bash 3.2, BSD `sed` and `xargs`) and on Linux, where make recipes run under `dash`.
  - POSIX sh in recipes and no Make 4 features (`$(file …)`, `!=`, `.ONESHELL`); a literal `#` inside `$(shell …)` breaks 3.81, so use `$(HASH)`.
  - A bash 4 feature (`declare -A`, `mapfile`) sits behind a `BASH_VERSINFO` check that names the fix, as in `scripts/spec-check.sh`; `sed`, `xargs` and `grep` use only options BSD and GNU share.
  - `make spec-check` needs Python 3.11 or later. `make doctor` shows the make, bash, sed and `python3` in use, and `make test-fused` reproduces arm64's fused multiply-add on amd64.
- **Public docs never name downstream consuming projects.** Describe what a consumer needs, not who the consumer is.

**Runtime dependencies** are deliberately minimal and reviewed — adding one is a decision, not a convenience. The current set, each confined to the package it serves, is in [architecture.md § Dependencies](docs/architecture.md#dependencies).

**Agent tooling:**

- **Code intelligence:** for Go symbol questions run `gopls` in the shell from the checkout or worktree being changed. codebase-memory-mcp, when installed, answers questions about `main` only ([ai-workflow.md § Recommended tooling](docs/ai-workflow.md#recommended-tooling-claude-code--cursor)).
- **Claude Code hook:** [`.claude/settings.json`](.claude/settings.json) runs [`.claude/hooks/goformat-on-save.sh`](.claude/hooks/goformat-on-save.sh) (gofumpt and goimports, skipping `*_gen.go`) on each `*.go` file a **Write** or **Edit** touches; `make fmt` is the full-tree pass.
- **Plugins:** `.claude/settings.json` enables go-coding, sdd, superpowers, gopls-lsp, pr-review-toolkit, feature-dev and claude-md-management; look there before assuming a skill is missing. The openehr-assistant plugin and codebase-memory-mcp are per-user installs that the repository does not enable.
- **Local config:** personal permission grants go in the gitignored `.claude/settings.local.json`; never add a `permissions` block to the checked-in `.claude/settings.json`.

## openEHR knowledge

The openehr-assistant plugin, where installed, and its MCP tools answer RM paths, class definitions, terminology codes and ITS-JSON shapes: look them up rather than guess ([ai-workflow.md § openEHR ground truth](docs/ai-workflow.md#openehr-ground-truth-mcp--skills)). This repository's pinned contracts outrank a general answer, and are the source to read when the plugin is absent:

- **Wire semantics:** the conformance probe suite ([conformance.md](docs/specifications/conformance.md)).
- **REST:** the vendored OpenAPI pin in [`resources/its-rest/`](resources/its-rest/README.md) (`*-validation.openapi.yaml`) for every path, body, header and status code.
- **XML:** the pinned ITS-XML schemas in [`resources/its-xml/`](resources/its-xml/README.md), which the generator reads for child-element order; they declare the `v2` namespace where the SDK writes `v1`.
- **Web Template and FLAT:** the EHRbase `openEHR_SDK` reference ([ADR 0014](docs/adr/0014-webtemplate-reference-implementation-lock.md)), read at the commit [`testkit/corpus/UPSTREAM_PINS.txt`](testkit/corpus/UPSTREAM_PINS.txt) pins.
- **Classes and invariants:** the openEHR specifications, as the pinned BMM schemas in [`resources/bmm/`](resources/bmm/) the generator reads.

## Do not touch (yet)

- Published REQ-NNN / PROBE-NNN / STRAND-NN identifiers ([REQ.md § Numbering policy](docs/specifications/REQ.md#numbering-policy)).
- Module path — locked at `github.com/cadasto/openehr-sdk-go` (REQ-001).
- The `go.mod` `go` directive — the minor line's `.0` patch, never the toolchain patch you happen to run ([releases.md § Minimum Go version](docs/releases.md#minimum-go-version) says why). Dev-image pins ([Dockerfile](Dockerfile)) move independently.
- `internal/bmmgen` and `internal/bmmdiff` — generator tooling, not public API; structural changes need rationale in [architecture.md](docs/architecture.md) and [ADR 0002](docs/adr/0002-bmm-codegen-decisions.md).
- A numbered ADR whose header does not name the REQs it amends: the map lists no ADRs, so the header is how `make spec-context` finds it, and `make spec-generate` writes the index in [`docs/adr/README.md`](docs/adr/README.md) from it.
