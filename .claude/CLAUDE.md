# Claude Code

All project rules, specs, tooling, and workflow: **[`AGENTS.md`](../AGENTS.md)** (repo root). Read that first.

Claude-only notes:

- **Hook** — after **Write** / **Edit** on `*.go`, [`.claude/settings.json`](settings.json) runs [`hooks/gofmt-on-save.sh`](hooks/gofmt-on-save.sh) (`gofmt -w -s` on that file; no-op if host `gofmt` missing). Use `make fmt` / `make ci` for full-tree checks.
- **Examples** — when adding or changing [`cmd/examples/`](../cmd/examples/), update [`docs/examples.md`](../docs/examples.md), [`cmd/examples/doc.go`](../cmd/examples/doc.go), and [`docs/quick-start.md`](../docs/quick-start.md) when onboarding changes — same PR. See [AGENTS.md § Runnable examples](../AGENTS.md#runnable-examples-agents).
