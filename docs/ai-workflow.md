# AI workflow

How AI assistants (Claude Code, Cursor, Copilot, Codex, …) work in this repo. Ground truth lives in [AGENTS.md](../AGENTS.md) and [architecture.md](architecture.md) — read those first. This file adds only the AI-specific layer: recommended tooling, openEHR ground-truth lookups, and the loop to follow. It does **not** restate the idiom, boundary, or spec rules — those are canonical elsewhere and linked below.

## Recommended tooling (Claude Code / Cursor)

**Strongly recommended:** the **[go-coding plugin](https://github.com/Cadasto/go-coding-plugin)** (`go-coding@cadasto`, Claude Code and Cursor). It encodes idiomatic-Go judgment and ties advice to the deterministic toolchain (gofumpt, `go vet`, golangci-lint v2 + `modernize`, `go test -race`) — the same tools the [Makefile](../Makefile) runs. Reach for these skills:

| Skill / agent | Use for |
|---|---|
| `go-coding` | router — when unsure which standard or tool applies |
| `go-errors` | wrapping (`%w`), `errors.Is`/`As`, sentinel vs typed errors |
| `go-concurrency` | goroutine lifetimes/leaks, context, atomics, `-race` |
| `go-testing` | table tests, `t.Parallel`, `synctest`, golden files |
| `go-idioms` | modern idioms (the `modernize` set) / `go fix` |
| `go-linting`, `go-lint-setup` | golangci-lint v2 config and adoption |
| `go-layout` | package and module structure |
| `go-explain` | one-shot lookup of a single idiom or tool |
| `go-reviewer` (agent) | pre-PR review for the bugs linters miss (concurrency, error-swallowing, ctx misuse) |

Pair with the **gopls-lsp** plugin for code intelligence (defs/refs/rename/vulncheck). Run the deterministic tool rather than reasoning a rule out by hand — that's the whole point of the plugin.

For **code exploration, call-chain tracing, and impact analysis**, query the **codebase-memory-mcp** knowledge graph (or the `codebase-memory` skill) before grepping the whole tree: `search_graph` (find functions / types / routes), `trace_path` (call chains and data flow), `get_code_snippet` (exact symbol source), `get_architecture` (structure overview). Run `index_repository` once if the project isn't indexed yet. This is the fast way to answer "who calls this?" before a refactor, or to map an unfamiliar subsystem.

## openEHR ground truth (MCP / skills)

This repo is an openEHR workspace. Before guessing an RM path, terminology code, or ITS-JSON shape, use the **openehr-assistant** skills (Skill tool, `openehr-assistant:<name>`) — the openEHR analogue of looking it up in the spec:

| Skill | Use when |
|---|---|
| `type-spec` | exact attribute list / invariant / signature for an RM class — **before locking goldens or types** |
| `terminology` | resolve a numeric terminology code to a term, or vice versa |
| `format-data` | validate the shape of a sample Composition / FLAT / STRUCTURED instance |
| `guide` | how-to: spec-lookup methodology (`howto/spec-lookup`), ITS-REST envelopes, simplified formats |
| `rm-structure` | domain overview (composition categories, ISM states, versioning, PARTY hierarchy) |
| `archetype-explain`, `template-explain` | semantics of an archetype / OPT — input to builders and validation tests |
| `aql-designer` | design / explain / review AQL for `openehr/aql/` |

The cross-SDK conformance probe set is the source of truth for wire-level semantics; the openEHR spec is authoritative for class invariants.

## The loop

1. **Locate** your task's REQ via the [REQ registry](specifications/REQ.md) → follow the row to its **canonical** topic spec (don't read prose out of `REQ.md` itself).
2. **Inspect ground truth before editing** — RM shapes via MCP `type_specification_get`, terminology via `terminology_resolve`. Never hardcode a path or numeric literal without verifying.
3. **Cite identifiers** — tests and `doc.go` reference REQ-NNN / PROBE-NNN; update [traceability.yaml](specifications/traceability.yaml) when landing packages or probes; never renumber published IDs.
4. **Don't decide open questions in code** — don't silently resolve a [research strand](specifications/research-strands.md), and don't add a normative MUST/SHOULD/MAY without a REQ to anchor it. Surface it or draft an [ADR](adr/).
5. **Verify** — `make ci` (includes `make spec-check`) before claiming done. See [ci.md](ci.md).

The full editing rules — idiomatic surface, the `cadasto/` boundary contract, and the do-not-touch list — are canonical in [AGENTS.md](../AGENTS.md) and [specifications/idiom.md](specifications/idiom.md). Follow those; this file intentionally doesn't duplicate them.

## Examples

When you add, rename, remove, or materially change a [`cmd/examples/`](../cmd/examples/) program, keep its docs in sync **in the same PR** — [`cmd/examples/doc.go`](../cmd/examples/doc.go), [examples.md](examples.md), and [quick-start.md](quick-start.md) when the onboarding path changes. Full checklist: [AGENTS.md § Runnable examples](../AGENTS.md#runnable-examples-agents). If `doc.go` and the markdown disagree, the runnable code wins.

## Hooks

After Write/Edit on a `*.go` file, Claude Code formats it via [`.claude/hooks/goformat-on-save.sh`](../.claude/hooks/goformat-on-save.sh) (gofumpt + goimports, host-only, skips `*_gen.go`). Details in [`.claude/CLAUDE.md`](../.claude/CLAUDE.md); `make fmt` is the authoritative full-tree pass.

## When stuck

- **Open decision** (STRAND-NN) → draft an [ADR](adr/) or ask the user; don't settle it in a PR.
- **Ambiguous spec** → `openehr-assistant:guide` (`howto/spec-lookup`) for the canonical wording.
- **Missing normative rule** → add a `Status: Draft` REQ in [REQ.md](specifications/REQ.md) and elaborate in the topic spec before coding — never a rule that exists only in code.

_Cross-SDK parity with the PHP SDK is **wire-level only** (REQ-081): match the wire, not the source shape._
