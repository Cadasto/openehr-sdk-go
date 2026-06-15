# Claude Code

All project rules, specs, tooling, and workflow: **[`AGENTS.md`](../AGENTS.md)** (repo root). Read that first.

Claude-only notes:

- **Hook** — after **Write** / **Edit** on `*.go`, [`.claude/settings.json`](settings.json) runs [`hooks/goformat-on-save.sh`](hooks/goformat-on-save.sh) (gofumpt + goimports on that file; falls back to `gofmt -s`; no-op if none on host; skips `*_gen.go`). Use `make fmt` / `make ci` for full-tree checks (`golangci-lint fmt` via the pinned image).
