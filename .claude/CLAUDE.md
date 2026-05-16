# Claude Code — repository-local notes

**Project rules, plans, stack, and tooling policy are in [`AGENTS.md`](../AGENTS.md)** (and [`README.md`](../README.md)). Read those first; they apply to every agent and human contributor.

This file only records **Claude Code–specific** behavior in *this* repo:

## Native instruction loading

Claude Code loads `CLAUDE.md` from `.claude/` when working in this project. It does **not** replace `AGENTS.md` — it supplements it for the Claude product.

## Hook (`.claude/settings.json`)

After **Write** or **Edit** on a `*.go` file, **PostToolUse** runs [`.claude/hooks/gofmt-on-save.sh`](hooks/gofmt-on-save.sh): host `gofmt -w -s` on the single edited file. **Host-only** — a Docker round-trip per save would dominate latency. If host `gofmt` is missing, the hook is a silent no-op; `make fmt` (which routes through the Dockerfile `dev` stage when host Go is absent) catches the tree on next invocation.

## openEHR & MCP skills

This repo is an openEHR workspace. Prefer the `openehr-assistant:*` skills (type-spec, terminology, rm-structure, format-data, guide) over guessing — see [`docs/ai-workflow.md`](../docs/ai-workflow.md) for the full list and when to use each.

## Source of truth for module design

The **Cadasto SDK Specification proposal** (private) governs the module layout, boundary rules, and idiomatic surface. If a code change pulls in a direction the proposal does not cover, stop and surface the decision before implementing — do not silently resolve open research strands.

## Commits and CHANGELOG

Follow **Conventional Commits** for commit messages and keep `CHANGELOG.md` `## [Unreleased]` bullets **short and high-level** (one line per artefact class and scope). See [`AGENTS.md` § Code style and conventions](../AGENTS.md#code-style-and-conventions).
