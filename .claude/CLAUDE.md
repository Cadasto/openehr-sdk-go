# Claude Code

All project rules, specs, tooling, and workflow: **[`AGENTS.md`](../AGENTS.md)** (repo root). Read that first.

Claude-only notes:

- **Hook** — after **Write** / **Edit** on `*.go`, [`.claude/settings.json`](settings.json) runs [`hooks/gofmt-on-save.sh`](hooks/gofmt-on-save.sh) (`gofmt -w -s` on that file; no-op if host `gofmt` missing). Use `make fmt` / `make ci` for full-tree checks.
