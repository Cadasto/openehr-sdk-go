---
kind: guide
---

# AI workflow

How AI assistants (Claude Code, Cursor, Copilot, Codex, …) work in this repo: recommended tooling, openEHR ground-truth lookups, the loop, orchestration, examples and review.
The contract is the specifications in [specifications/](specifications/) and the vendored contracts under [`resources/`](../resources/), and [AGENTS.md](../AGENTS.md) is where to start; this file adds only the AI-specific layer.
The public disclosure that this project is built with AI assistance is in the [README](../README.md#ai-assisted-development); the contributor rules, including the `Assisted-by:` commit trailer, are in [CONTRIBUTING.md](../CONTRIBUTING.md#ai-assisted-contributions).

## Recommended tooling (Claude Code / Cursor)

**Binding for Go work:** the [go-coding plugin](https://github.com/Cadasto/go-coding-plugin) (`go-coding@cadasto`, Claude Code and Cursor). It encodes idiomatic-Go judgment and ties its advice to the deterministic toolchain (gofumpt, `go vet`, golangci-lint v2 + `modernize`, `go test -race`), the same tools the [Makefile](../Makefile) runs. Before writing or reviewing Go, load `go-coding:go-coding` and then the focused skill that matches the diff. Loading the router alone doesn't count.

**Which skill for which change:**

| Change touches… | Load |
|---|---|
| any Go change (start here) | `go-coding:go-coding` (router) |
| error paths | `go-errors` |
| any `_test.go` | `go-testing` |
| modernizing, or loops/maps/strings | `go-idioms` |
| goroutines, channels, or context lifetimes | `go-concurrency` |
| a new package or exported API | `go-layout` |
| golangci-lint v2 config/adoption | `go-lint-setup`; not needed in this repo, because the config is already pinned (`make lint`) |
| reviewing a Go diff | the plugin's `go-reviewer` agent, unless the workflow already supplies a single reviewer seat |

**Orchestrators: brief every subagent.** Subagents don't inherit the parent session's skills:

- Implementer brief: load the router plus the matching focused skill(s) above before writing.
- Reviewer brief: load them before reviewing and cite the rule a finding rests on.
- A reviewer seat that already exists (the SDD subagent-driven loop's own reviewer) applies the skills itself instead of spawning `go-reviewer`.

Pair with the gopls-lsp plugin for code intelligence (defs/refs/rename/vulncheck). The plugin is there so you run the deterministic tool instead of working a rule out by hand. The LSP tool sees only the session's starting folder, so from a worktree run `gopls` in the shell instead: `cd .worktrees/<branch> && gopls references <file>:<line>:<col>` (also `definition` and `implementation`).

The codebase-memory-mcp knowledge graph (or the `codebase-memory` skill) indexes the main checkout, not `.worktrees/`, so use it for structural questions about `main`: the architecture overview (`get_architecture`), hotspots, files that change together, and the callers of a distinctive name (`search_graph`, `trace_path`, `get_code_snippet`). It matches calls by bare name, so a common name such as `Run` or `Write` gives false callers. On a branch, `gopls` answers symbol questions. Run `index_repository` once if the project isn't indexed yet.

## openEHR ground truth (MCP / skills)

This repo is an openEHR workspace. Before guessing an RM path, terminology code, or ITS-JSON shape, look it up with the openehr-assistant plugin. Load its skills with the Skill tool (`openehr-assistant:<name>`) and type its commands as `/<name>`:

| Skill / command | Use when |
|---|---|
| `/openehr-explain` | start here for any lookup: RM/AM/BASE type, RM structural concept, archetype, template, ADL idiom, AQL keyword, or terminology code |
| `openehr-assistant` | routing + the guide corpus: spec-lookup methodology (`howto/spec-lookup`), ITS-REST envelopes, simplified formats |
| `aql-authoring` | write, optimize, or review AQL for `openehr/aql/` |
| `composition-builder` | build or check a Composition instance |
| `template-authoring` | OET/OPT authoring and constraint review |
| `archetype-authoring` · `archetype-lint` | author / review / lint an archetype |
| `demographic-modeling` | PARTY and demographic structures |
| `/ckm-search` | find a published CKM archetype or template before modelling one |

For an exact attribute list, invariant, or signature, call the MCP tool `type_specification_get` (BMM-backed) **before locking goldens or types**. Resolve a numeric code with `terminology_resolve`.

## The loop

0. **Assemble context small:** `make spec-context REQ=094 OUTLINE=1` first. It prints the registry row, the `traceability.yaml` block (packages, probes, tests), the plans and ADRs that name the REQ, the research strands that touch it, and the canonical section's outline with each rule's name and size.
   Then `make spec-context REQ=094 RULE=<name>` prints the one rule the task needs.
   Without either flag the bundle prints the whole section, about 80 KB for the largest, so read it whole only when the task spans the section.
1. **Locate** your task's REQ via the [REQ registry](specifications/REQ.md), then follow the row to its **canonical** topic spec (don't read prose out of `REQ.md` itself).
2. **Inspect ground truth before editing.** Check RM shapes with MCP `type_specification_get` and terminology with `terminology_resolve`. Never hardcode a path or numeric literal without verifying it. Before writing the Go itself, load the matching go-coding skill (§ Recommended tooling above).
3. **Cite identifiers** in tests and maintainer comments as [development-process.md § The ladder](development-process.md#the-ladder-full-lane) says, never in what SDK users read ([AGENTS.md § Spec-driven workflow](../AGENTS.md#spec-driven-workflow-agents)).
4. **Don't decide open questions in code** ([AGENTS.md § Source of truth](../AGENTS.md#source-of-truth)), and don't add a normative `MUST`/`SHOULD`/`MAY` without a REQ to anchor it.
   A rule missing from the spec goes into its topic spec, under the next free REQ with a `traceability.yaml` entry and `make spec-generate`, before the code: never leave a rule that exists only in code.
   For an ambiguous one, find the canonical wording with `/openehr-explain` or the `openehr-assistant` skill's `howto/spec-lookup` guide.
5. **Verify.** Run `make ci` (includes `make spec-check`) before claiming done. See [ci.md](ci.md). **For wire/client changes, green tests aren't enough.** Read the `probes:` on the REQ's traceability entry (or `make spec-context`) and open each `#### PROBE-NNN` in [conformance.md](specifications/conformance.md). The task is done only when each probe is **Implemented (Sandbox)**, or **Implemented (inline)** for an in-repo probe, or its deferral is recorded in its conformance.md entry. `make probe-status` lists each probe's status and whether its test file exists.

`/sdd-deliver` runs this loop as a pipeline to a ready PR, and `/sdd-close` closes the PR out and, after the merge, cleans up; the sdd plugin documents both.

## Orchestration

The sdd plugin sets the orchestrator and worker rules. This repository adds:

- **Code index in a worktree:** workers answer Go symbol questions with `gopls` run in the shell inside the worktree, plus `go doc` and `go list` (§ Recommended tooling), and the brief repeats this.
- **The maintainer merges.** Agents open PRs, a draft only when asked.
- The worker model, parallelism, worktrees, the per-task review gate, the reviewers and the paths they skip are declared under `agents:` in [`.sdd.yaml`](.sdd.yaml). The model is passed per dispatch.

## Examples

When you add, rename, remove, or materially change a [`cmd/examples/`](../cmd/examples/) program, keep its docs in sync **in the same PR**. That means [`cmd/examples/doc.go`](../cmd/examples/doc.go), [examples.md](examples.md), the site's [examples page](../pages/examples.md) and [package map](../pages/packages.md), and, when the onboarding path changes, [quick-start.md](quick-start.md). See also [AGENTS.md § Spec-driven workflow](../AGENTS.md#spec-driven-workflow-agents). If `doc.go` and the markdown disagree, the runnable code wins.

An example change is done, beyond `make ci` and `make docs-check`, when:

- the program imports public packages only, runs offline by default, and opens with its purpose and its exact run commands;
- its flags are spelled with one dash (`-corpus`), in prose, run commands and comments alike;
- its errors are wrapped and classified through the SDK's public error contracts;
- no secret and no value that changes between runs (a token, a PKCE verifier, a uid, a timestamp) is copied into a document;
- its output is compared by a transcript or mirror test, or its exclusion records why not (**Transcripts**, below).

**Mirrored copies.** examples.md is canonical, its learning order included; the README and the site show shorter copies. When an example's output appears in another prominent place, add that place to `mirrors` in [`mirrors_test.go`](../cmd/examples/mirrors_test.go), which also holds the site's learning order to the catalogue's. A snippet that cannot be compared mechanically names its source program in a comment and is a Go `Example` or a transcript test with deterministic output. Never copy a whole program into several documents.

**Transcripts.** [`transcripts_test.go`](../cmd/examples/transcripts_test.go) checks the **Sample output:** blocks of examples.md that its allowlist names against real runs. That allowlist alone decides which blocks are checked, and its exclusion census accounts for every other example. A deliberate output change therefore means copying the block again from `go run ./cmd/examples/<name>`, not editing it by hand. An example that gains a verbatim sample-output block joins the allowlist in the same PR: `TestSampleMarkerCensus` fails on a bare sample-output marker with no allowlist entry. The one recorded exception is smart-launch, whose PKCE state, verifier and `expires_at` differ every run. `bareMarkerException` names it, and the test fails if that exception outlives the marker it excuses.

## Review

Findings for a branch live in one file in the clone's git directory, which `sdd-pr status` names; the line format, the severities and the evidence rules are in the sdd plugin's `references/review.md`.
In short, three severities, graded by harm:

- **Critical** and **important** findings carry evidence (the command run, the failing test, or the two sentences that disagree) and are resolved before merge, fixed or declined with the reason.
- One about code or a document the change did not touch keeps its severity, is marked `outside` and blocks nothing.
- **Suggestions** name an input and a weaker result, never taste or wrong behaviour; they never block, number at most five per reviewer per pass, and the close-out asks which to carry to `docs/backlog.md`.

In anything posted to the PR, never write a number with a leading hash sign, which GitHub turns into a link to an unrelated issue.

A reviewer that loads the sdd plugin runs `/sdd-review --pr <N>`, in whichever host the orchestrator chooses; a second pass lands in the same findings file. One that does not load the plugin gets this request, filled in (`sdd-pr scope` prints the range), so what it is given does not drift from what the repo defines:

```text
── review request · <branch> · range <a>..<b> · profile formal ──
Review commits <a>..<b> of Cadasto/openehr-sdk-go, and only those. Read docs/ai-workflow.md § Review.
Report critical and important findings only, each with evidence (what you ran, or the two
sentences that disagree) and a one-line fix, ending a defect in what the range did not change with
` · outside`; a lead with no behaviour claim is a suggestion, at most five.
If you work on this machine, pipe your lines in that grammar to: <plugin root>/tools/sdd-pr.py add -
If you work on the pull request, post one review: one inline comment per finding about the change,
its first word **critical** or **important** and its last line `By: <your name>`, and a defect in
what the range did not change in the review body, in the grammar above; post nothing else, and do not
restate the PR body.
──────────────────────────────────────────────────────────────────────────────
```
