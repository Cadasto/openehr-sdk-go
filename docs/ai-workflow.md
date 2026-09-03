# AI workflow

How AI assistants (Claude Code, Cursor, Copilot, Codex, …) work in this repo. Ground truth lives in [AGENTS.md](../AGENTS.md) and [architecture.md](architecture.md) — read those first. This file adds only the AI-specific layer: recommended tooling, openEHR ground-truth lookups, and the loop to follow. It does **not** restate the idiom, boundary, or spec rules — those are canonical elsewhere and linked below.

## Recommended tooling (Claude Code / Cursor)

**Binding for Go work:** the **[go-coding plugin](https://github.com/Cadasto/go-coding-plugin)** (`go-coding@cadasto`, Claude Code and Cursor). It encodes idiomatic-Go judgment and ties advice to the deterministic toolchain (gofumpt, `go vet`, golangci-lint v2 + `modernize`, `go test -race`) — the same tools the [Makefile](../Makefile) runs. Before writing or reviewing Go, load `go-coding:go-coding` then the focused skill matching the diff — loading the router alone doesn't count.

**Which skill for which change:**

| Change touches… | Load |
|---|---|
| any Go change — start here | `go-coding:go-coding` (router) |
| error paths | `go-errors` |
| any `_test.go` | `go-testing` |
| modernizing, or loops/maps/strings | `go-idioms` |
| goroutines, channels, or context lifetimes | `go-concurrency` |
| a new package or exported API | `go-layout` |
| a one-shot idiom/tool question, no editing | `/go-explain <topic>` |
| golangci-lint v2 config/adoption | `go-linting`, `go-lint-setup` — not needed in this repo, the config is already pinned (`make lint`) |
| reviewing a Go diff | the plugin's `go-reviewer` agent, unless the workflow already supplies a single reviewer seat |

**Orchestrators: brief every subagent** — subagents don't inherit the parent session's skills:

- Implementer brief: load the router plus the matching focused skill(s) above before writing.
- Reviewer brief: load them before reviewing and cite the rule a finding rests on.
- A reviewer seat that already exists (the SDD subagent-driven loop's own reviewer) applies the skills itself instead of spawning `go-reviewer`.

Pair with the **gopls-lsp** plugin for code intelligence (defs/refs/rename/vulncheck). Run the deterministic tool rather than reasoning a rule out by hand — that's the whole point of the plugin.

For **code exploration, call-chain tracing, and impact analysis**, query the **codebase-memory-mcp** knowledge graph (or the `codebase-memory` skill) before grepping the whole tree: `search_graph` (find functions / types / routes), `trace_path` (call chains and data flow), `get_code_snippet` (exact symbol source), `get_architecture` (structure overview). Run `index_repository` once if the project isn't indexed yet. This is the fast way to answer "who calls this?" before a refactor, or to map an unfamiliar subsystem.

## openEHR ground truth (MCP / skills)

This repo is an openEHR workspace. Before guessing an RM path, terminology code, or ITS-JSON shape, look it up with the **openehr-assistant** plugin — skills via the Skill tool (`openehr-assistant:<name>`), commands typed as `/<name>`:

| Skill / command | Use when |
|---|---|
| `/openehr-explain` | **start here for any lookup** — RM/AM/BASE type, RM structural concept, archetype, template, ADL idiom, AQL keyword, or terminology code |
| `openehr-assistant` | routing + the guide corpus: spec-lookup methodology (`howto/spec-lookup`), ITS-REST envelopes, simplified formats |
| `aql-authoring` | write, optimize, or review AQL for `openehr/aql/` |
| `composition-builder` | build or check a Composition instance |
| `template-authoring` | OET/OPT authoring and constraint review |
| `archetype-authoring` · `archetype-lint` | author / review / lint an archetype |
| `demographic-modeling` | PARTY and demographic structures |
| `/ckm-search` | find a published CKM archetype or template before modelling one |

For an exact attribute list, invariant, or signature — **before locking goldens or types** — call the MCP tool `type_specification_get` (BMM-backed); resolve a numeric code with `terminology_resolve`.

## The loop

0. **Assemble context in one shot:** `make spec-context REQ=094` — bundles the registry row, the `traceability.yaml` block (packages, probes, tests, plans), the canonical spec excerpt, and any research strands that touch the REQ. Start here; it routes you to the canonical sources instead of grepping.
1. **Locate** your task's REQ via the [REQ registry](specifications/REQ.md) → follow the row to its **canonical** topic spec (don't read prose out of `REQ.md` itself).
2. **Inspect ground truth before editing** — RM shapes via MCP `type_specification_get`, terminology via `terminology_resolve`. Never hardcode a path or numeric literal without verifying. Before writing the Go itself, load the matching go-coding skill (§ Recommended tooling above).
3. **Cite identifiers** — tests and `doc.go` reference REQ-NNN / PROBE-NNN; update [traceability.yaml](specifications/traceability.yaml) when landing packages or probes; never renumber published IDs.
4. **Don't decide open questions in code** — don't silently resolve a [research strand](specifications/research-strands.md), and don't add a normative MUST/SHOULD/MAY without a REQ to anchor it. Surface it or draft an [ADR](adr/).
5. **Verify** — `make ci` (includes `make spec-check`) before claiming done. See [ci.md](ci.md). **For wire/client changes, green tests aren't enough:** read the `probes:` on the REQ's traceability entry (or `make spec-context`), open each `#### PROBE-NNN` in [conformance.md](specifications/conformance.md), and treat the task as done only when each is **Implemented (Sandbox)** or explicitly deferred in the plan. `make probe-status` lists each probe's status and whether its test file exists.

The full editing rules — idiomatic surface, the `cadasto/` boundary contract, and the do-not-touch list — are canonical in [AGENTS.md](../AGENTS.md) and [specifications/idiom.md](specifications/idiom.md). Follow those; this file intentionally doesn't duplicate them.

## Examples

When you add, rename, remove, or materially change a [`cmd/examples/`](../cmd/examples/) program, keep its docs in sync **in the same PR** — [`cmd/examples/doc.go`](../cmd/examples/doc.go), [examples.md](examples.md), and [quick-start.md](quick-start.md) when the onboarding path changes. See also [AGENTS.md § Spec-driven workflow](../AGENTS.md#spec-driven-workflow-agents). If `doc.go` and the markdown disagree, the runnable code wins.

## Hooks

After Write/Edit on a `*.go` file, Claude Code formats it via [`.claude/hooks/goformat-on-save.sh`](../.claude/hooks/goformat-on-save.sh) (gofumpt + goimports, host-only, skips `*_gen.go`). Details in [`.claude/CLAUDE.md`](../.claude/CLAUDE.md); `make fmt` is the authoritative full-tree pass.

## When stuck

- **Open decision** (STRAND-NN) → draft an [ADR](adr/) or ask the user; don't settle it in a PR.
- **Ambiguous spec** → `/openehr-explain`, or the `openehr-assistant` skill's `howto/spec-lookup` guide, for the canonical wording.
- **Missing normative rule** → add a `Status: Draft` REQ in [REQ.md](specifications/REQ.md) and elaborate in the topic spec before coding — never a rule that exists only in code.
