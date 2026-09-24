# AI workflow

How AI assistants (Claude Code, Cursor, Copilot, Codex, …) work in this repo. Ground truth lives in [AGENTS.md](../AGENTS.md) and [architecture.md](architecture.md), so read those first. This file adds only the AI-specific layer: recommended tooling, openEHR ground-truth lookups, and the loop to follow. It does **not** restate the idiom, boundary, or spec rules. Those have their canonical homes elsewhere, linked below. The public disclosure that this project is built with AI assistance is in the [README](../README.md#ai-assisted-development); the contributor rules, including the `Assisted-by:` commit trailer, are in [CONTRIBUTING.md](../CONTRIBUTING.md#ai-assisted-contributions).

## Recommended tooling (Claude Code / Cursor)

**Binding for Go work:** the **[go-coding plugin](https://github.com/Cadasto/go-coding-plugin)** (`go-coding@cadasto`, Claude Code and Cursor). It encodes idiomatic-Go judgment and ties its advice to the deterministic toolchain (gofumpt, `go vet`, golangci-lint v2 + `modernize`, `go test -race`), the same tools the [Makefile](../Makefile) runs. Before writing or reviewing Go, load `go-coding:go-coding` and then the focused skill that matches the diff. Loading the router alone doesn't count.

**Which skill for which change:**

| Change touches… | Load |
|---|---|
| any Go change (start here) | `go-coding:go-coding` (router) |
| error paths | `go-errors` |
| any `_test.go` | `go-testing` |
| modernizing, or loops/maps/strings | `go-idioms` |
| goroutines, channels, or context lifetimes | `go-concurrency` |
| a new package or exported API | `go-layout` |
| a one-shot idiom/tool question, no editing | `/go-explain <topic>` |
| golangci-lint v2 config/adoption | `go-linting`, `go-lint-setup`; not needed in this repo, because the config is already pinned (`make lint`) |
| reviewing a Go diff | the plugin's `go-reviewer` agent, unless the workflow already supplies a single reviewer seat |

**Orchestrators: brief every subagent.** Subagents don't inherit the parent session's skills:

- Implementer brief: load the router plus the matching focused skill(s) above before writing.
- Reviewer brief: load them before reviewing and cite the rule a finding rests on.
- A reviewer seat that already exists (the SDD subagent-driven loop's own reviewer) applies the skills itself instead of spawning `go-reviewer`.

Pair with the **gopls-lsp** plugin for code intelligence (defs/refs/rename/vulncheck). Run the deterministic tool instead of working a rule out by hand. That is what the plugin is for.

For **code exploration, call-chain tracing, and impact analysis**, query the **codebase-memory-mcp** knowledge graph (or the `codebase-memory` skill) before grepping the whole tree: `search_graph` (find functions / types / routes), `trace_path` (call chains and data flow), `get_code_snippet` (exact symbol source), `get_architecture` (structure overview). Run `index_repository` once if the project isn't indexed yet. Use it to answer "who calls this?" before a refactor, or to map an unfamiliar subsystem.

## openEHR ground truth (MCP / skills)

This repo is an openEHR workspace. Before guessing an RM path, terminology code, or ITS-JSON shape, look it up with the **openehr-assistant** plugin. Load its skills with the Skill tool (`openehr-assistant:<name>`) and type its commands as `/<name>`:

| Skill / command | Use when |
|---|---|
| `/openehr-explain` | **start here for any lookup**: RM/AM/BASE type, RM structural concept, archetype, template, ADL idiom, AQL keyword, or terminology code |
| `openehr-assistant` | routing + the guide corpus: spec-lookup methodology (`howto/spec-lookup`), ITS-REST envelopes, simplified formats |
| `aql-authoring` | write, optimize, or review AQL for `openehr/aql/` |
| `composition-builder` | build or check a Composition instance |
| `template-authoring` | OET/OPT authoring and constraint review |
| `archetype-authoring` · `archetype-lint` | author / review / lint an archetype |
| `demographic-modeling` | PARTY and demographic structures |
| `/ckm-search` | find a published CKM archetype or template before modelling one |

For an exact attribute list, invariant, or signature, call the MCP tool `type_specification_get` (BMM-backed) **before locking goldens or types**. Resolve a numeric code with `terminology_resolve`.

## The loop

0. **Assemble context in one shot:** `make spec-context REQ=094`. It bundles the registry row, the `traceability.yaml` block (packages, probes, tests, plans), the canonical spec excerpt, and any research strands that touch the REQ. Start here. It points you to the canonical sources, so you don't have to grep for them.
1. **Locate** your task's REQ via the [REQ registry](specifications/REQ.md), then follow the row to its **canonical** topic spec (don't read prose out of `REQ.md` itself).
2. **Inspect ground truth before editing.** Check RM shapes with MCP `type_specification_get` and terminology with `terminology_resolve`. Never hardcode a path or numeric literal without verifying it. Before writing the Go itself, load the matching go-coding skill (§ Recommended tooling above).
3. **Cite identifiers.** Tests and `doc.go` reference REQ-NNN / PROBE-NNN. Update [traceability.yaml](specifications/traceability.yaml) when landing packages or probes, and never renumber published IDs.
4. **Don't decide open questions in code.** Don't silently resolve a [research strand](specifications/research-strands.md), and don't add a normative MUST/SHOULD/MAY without a REQ to anchor it. Raise it or draft an [ADR](adr/).
5. **Verify.** Run `make ci` (includes `make spec-check`) before claiming done. See [ci.md](ci.md). **For wire/client changes, green tests aren't enough.** Read the `probes:` on the REQ's traceability entry (or `make spec-context`) and open each `#### PROBE-NNN` in [conformance.md](specifications/conformance.md). The task is done only when each probe is **Implemented (Sandbox)** or explicitly deferred in the plan. `make probe-status` lists each probe's status and whether its test file exists.

The full editing rules (idiomatic surface, the `cadasto/` boundary contract, and the do-not-touch list) are canonical in [AGENTS.md](../AGENTS.md) and [specifications/idiom.md](specifications/idiom.md). Follow those. This file does not repeat them.

## Examples

When you add, rename, remove, or materially change a [`cmd/examples/`](../cmd/examples/) program, keep its docs in sync **in the same PR**. That means [`cmd/examples/doc.go`](../cmd/examples/doc.go), [examples.md](examples.md), and, when the onboarding path changes, [quick-start.md](quick-start.md). See also [AGENTS.md § Spec-driven workflow](../AGENTS.md#spec-driven-workflow-agents). If `doc.go` and the markdown disagree, the runnable code wins.

## Hooks

The Claude Code format-on-save hook is documented in [`.claude/CLAUDE.md`](../.claude/CLAUDE.md). `make fmt` is the authoritative full-tree pass.

## When stuck

- **Open decision** (STRAND-NN) → draft an [ADR](adr/) or ask the user. Don't settle it in a PR.
- **Ambiguous spec** → use `/openehr-explain`, or the `openehr-assistant` skill's `howto/spec-lookup` guide, to find the canonical wording.
- **Missing normative rule** → add a `Status: Draft` REQ in [REQ.md](specifications/REQ.md) and elaborate it in the topic spec before coding. Never leave a rule that exists only in code.
