---
kind: guide
---

# AI workflow

How AI assistants (Claude Code, Cursor, Copilot, Codex, …) work in this repo. Ground truth lives in [AGENTS.md](../AGENTS.md) and [architecture.md](architecture.md), so read those first. The specification records the contract and the maintainer's instruction outranks a sentence ([AGENTS.md § Source of truth](../AGENTS.md#source-of-truth)). This file adds only the AI-specific layer: recommended tooling, openEHR ground-truth lookups, and the loop to follow. It does not restate the idiom, boundary, or spec rules, which have their canonical homes elsewhere (linked below). The public disclosure that this project is built with AI assistance is in the [README](../README.md#ai-assisted-development); the contributor rules, including the `Assisted-by:` commit trailer, are in [CONTRIBUTING.md](../CONTRIBUTING.md#ai-assisted-contributions).

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

0. **Assemble context in one shot:** `make spec-context REQ=094`. It bundles the registry row, the `traceability.yaml` block (packages, probes, tests), the canonical spec excerpt, any plan that names the REQ, the ADRs whose header cites it, and any research strands that touch the REQ. Start here: the bundle points you to the canonical sources, so you don't have to grep for them.
1. **Locate** your task's REQ via the [REQ registry](specifications/REQ.md), then follow the row to its **canonical** topic spec (don't read prose out of `REQ.md` itself).
2. **Inspect ground truth before editing.** Check RM shapes with MCP `type_specification_get` and terminology with `terminology_resolve`. Never hardcode a path or numeric literal without verifying it. Before writing the Go itself, load the matching go-coding skill (§ Recommended tooling above).
3. **Cite identifiers.** Tests and maintainer comments reference REQ-NNN / PROBE-NNN; godoc on exported API is written for SDK users and does not. Update [traceability.yaml](specifications/traceability.yaml) when landing packages or probes and run `make spec-generate` (the registry is generated from it); never renumber published IDs.
4. **Don't decide open questions in code.** Don't silently resolve a [research strand](specifications/research-strands.md), and don't add a normative `MUST`/`SHOULD`/`MAY` without a REQ to anchor it. Raise it or draft an [ADR](adr/).
5. **Verify.** Run `make ci` (includes `make spec-check`) before claiming done. See [ci.md](ci.md). **For wire/client changes, green tests aren't enough.** Read the `probes:` on the REQ's traceability entry (or `make spec-context`) and open each `#### PROBE-NNN` in [conformance.md](specifications/conformance.md). The task is done only when each probe is **Implemented (Sandbox)**, or **Implemented (inline)** for an in-repo probe, or its deferral is recorded in its conformance.md entry. `make probe-status` lists each probe's status and whether its test file exists.

The full editing rules (idiomatic surface, the `cadasto/` boundary contract, and the do-not-touch list) are canonical in [AGENTS.md](../AGENTS.md) and [specifications/idiom.md](specifications/idiom.md). Follow those; this file does not repeat them.

`/sdd-deliver` runs this loop as a pipeline: workers per task, the first review pass, the PR. `/sdd-close <PR>` then closes out in the same PR: it updates the requirement status, writes the PR body, carries the `outside` defects it does not fix to `docs/backlog.md` and asks which suggestions to carry, by name. After the merge, `/sdd-close` again cleans up the branch; now and then, `/sdd-triage --backlog` on `main` picks up what was not carried and consolidates the backlog.

## Orchestration

- **One orchestrator, bounded workers.** The main session orchestrates, on the strongest model available. It reads what binds, briefs and dispatches workers, gates each task, adjudicates spec questions, opens the PR, runs triage and closes out.
- **The orchestrator does not write product code**, except for a task that cannot be made self-contained. That task is done in-session, and the PR body says so.
- **A brief is self-contained.** It holds the task, what it cites, the clauses it must satisfy (quoted), the files it may touch, the verification command, en-route findings, the skills to load (§ Recommended tooling), and no subagents.
- **Code index:** for Go symbol questions (where it is defined, who uses it, who implements it), workers run `gopls definition|references|implementation <file>:<line>:<col>` from inside the worktree, plus `go doc` and `go list`; `grep` covers literals and prose, and the brief repeats this. The codebase-memory-mcp graph indexes the main checkout, not `.worktrees/`, so it answers only questions about `main` (§ Recommended tooling).
- **Workers do not spawn workers.** Tasks run in sequence on the branch; a parallel wave (large tasks, disjoint files) shares it, each worker committing only its own files.
- **Completion is accounted for.** A worker that dies is re-dispatched, or the gap is named. A task is not done because a dispatch ended.
- **Findings state is read from the findings file (§ Review), never remembered.**
- **The maintainer merges.** Agents open PRs, a draft only when asked.
- The worker model, parallelism, worktrees, the per-task review gate, the reviewers and the paths they skip are declared under `agents:` in [`.sdd.yaml`](.sdd.yaml). The model is passed per dispatch.

## Examples

When you add, rename, remove, or materially change a [`cmd/examples/`](../cmd/examples/) program, keep its docs in sync **in the same PR**. That means [`cmd/examples/doc.go`](../cmd/examples/doc.go), [examples.md](examples.md), and, when the onboarding path changes, [quick-start.md](quick-start.md). See also [AGENTS.md § Spec-driven workflow](../AGENTS.md#spec-driven-workflow-agents). If `doc.go` and the markdown disagree, the runnable code wins.

## Hooks

The Claude Code format-on-save hook is documented in [AGENTS.md § Tooling & workflow](../AGENTS.md#tooling--workflow). `make fmt` is the authoritative full-tree pass.

## Review

Findings for a branch live in one file in the clone's git directory, `<git-common-dir>/sdd/findings/<branch-slug>.md`, named after the branch with `/` replaced by `--`. Every worktree and every agent on this machine sees it, git never commits it, and removing a worktree keeps it; `sdd-pr status` prints its path. `sdd-pr` makes every write to it (`add`, `flip`, `record`, `rename`), so nobody edits it by hand. It is the only list of findings, with or without a PR, and `sdd-pr status` names it for deletion once the PR merges. When a PR exists, `sdd-pr` keeps the file, the PR's inline review threads, one summary per pass and the suggestion comments in step; nothing else about findings is posted, the PR body included. `sdd-pr status` lists what is open, prints `Mergeable: yes|no` and names the next command. The line format, the severities and the evidence rules are in the sdd plugin's `references/review.md`.

There are three severities, graded by harm. **Critical** and **important** findings carry evidence: the command run, the failing test, or the two sentences that disagree, quoted. A claim of wrong code behaviour is settled by running it, or by a named input, then graded by its harm; a missing specification sentence never lowers it. Those about the change are resolved before merge, either fixed or declined with the reason on the finding's line, and they are the only ones that get inline threads on the PR. One about code or a document the change did not touch keeps its severity, is marked `outside`, blocks nothing, and is fixed here when the fix is small and carried after merge otherwise. **Suggestions** never block, number at most five per reviewer per pass, and stay on the PR in its suggestion comments; the close-out (`/sdd-close`) asks which of them to carry to `docs/backlog.md` in the PR, by name, and after the merge `/sdd-triage --backlog` carries any it did not. This repository keeps `docs/backlog.md` at about 80 lines, not the plugin's 40, so a consolidation stops there. A suggestion names an input and a weaker result and outlives the change; taste (naming, wording, style) never does, and wrong behaviour is never a suggestion. A slip in a document the branch wrote is fixed in the pass, not filed. In anything posted to the PR, never write a number with a leading hash sign, which GitHub turns into a link to an unrelated issue.

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

A finding is a claim, and so is a reviewer's proposed correction; both are checked against the code and the spec before either is applied. The findings file goes when the branch merges, so a declined finding whose reason should hold for later changes becomes a sentence in the canonical spec or an ADR.

## When stuck

- **Open decision** (STRAND-NN) → draft an [ADR](adr/) or ask the user. Don't settle it in a PR.
- **Ambiguous spec** → use `/openehr-explain`, or the `openehr-assistant` skill's `howto/spec-lookup` guide, to find the canonical wording.
- **Missing normative rule** → write it in the topic spec, add a `traceability.yaml` entry with the next free REQ number, and run `make spec-generate` before coding. Never leave a rule that exists only in code.
