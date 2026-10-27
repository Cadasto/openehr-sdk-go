# Claude Code

All project rules, specs, tooling, and workflow: **[`AGENTS.md`](../AGENTS.md)** (repo root). Read that first.

Claude-only notes:

- **Hook** — after **Write** / **Edit** on `*.go`, [`.claude/settings.json`](settings.json) runs [`hooks/goformat-on-save.sh`](hooks/goformat-on-save.sh) (gofumpt + goimports on that file; falls back to `gofmt -s`; no-op if none on host; skips `*_gen.go`). Use `make fmt` / `make ci` for full-tree checks (`golangci-lint fmt` via the pinned image).
- **Plugins** — the checked-in [`settings.json`](settings.json) enables the project plugins (go-coding, sdd, superpowers, gopls-lsp, pr-review-toolkit, feature-dev, claude-md-management); look there before assuming a skill is missing.
- **Backlog size** — `docs/backlog.md` is kept at about 80 lines here ([ai-workflow.md § Review](../docs/ai-workflow.md#review)). The sdd plugin builds in 40: its session-start line "over the 40 it should hold" is informational until the count passes 80, and `/sdd-triage --backlog` consolidates to about 80, not 40.
- **Retiring a requirement** — the entry moves to `docs/specifications/retired-requirements.md` ([AGENTS.md](../AGENTS.md)). `/sdd-specify` keeps a retired section in its topic spec by default; here it moves the entry to that page instead and points the map row's `canonical` at it.
