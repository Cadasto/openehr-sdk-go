# openehr-sdk-go

[![CI](https://github.com/cadasto/openehr-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/cadasto/openehr-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cadasto/openehr-sdk-go.svg)](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/cadasto/openehr-sdk-go)](https://goreportcard.com/report/github.com/cadasto/openehr-sdk-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Cadasto/openehr-sdk-go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Status: v0 preview](https://img.shields.io/badge/status-v0--preview-orange)](docs/roadmap.md)

First-party **Go SDK for openEHR** — covers openEHR REST `1.1.0-development`, the Reference Model, AQL, OPT/OET, and SMART-on-openEHR auth, with Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care aggregates) shipped in the same module for v1.

> **Status:** early implementation (v0 preview) — see [`docs/roadmap.md`](docs/roadmap.md) for the landed-vs-planned matrix.

## Use cases

The four primary consumers:

1. **Benchmark and load tools** — high-concurrency CRUD against the openEHR API; the existing **openehr-cdr** benchmark is the first consumer.
2. **Synthetic data seeders** — OPT-guided fakers driving bulk Compositions and demographic records.
3. **MCP servers** — exposing openEHR operations as MCP tools for agentic clients, with token-forwarded auth.
4. **Federative API clients** — fan-out over multiple openEHR backends with per-node spec pinning and partial-failure handling.

Plus building-block use cases that import a single sub-package (RM modeling, codec, validation, AQL string construction, OPT parsing) without constructing an authenticated client.

## Quickstart

```bash
make help        # grouped targets (toolchain, test, lint, CI, …)
make doctor      # check host Go vs Docker fallback
make ci          # full PR gate (see docs/ci.md)
make test        # unit tests (+ codegen drift check)
make fmt         # gofmt -w -s on the tree
```

Go `1.25.x` on the host is the fast path. If host Go is missing, build the Docker dev image once (`make image-dev`) and the Makefile transparently routes `fmt / vet / test / build` through it. See [`Dockerfile`](Dockerfile) and [`docker-compose.yml`](docker-compose.yml).

## Documentation

| Reading order | Doc |
|---|---|
| 1 — entry point for any agent | [AGENTS.md](AGENTS.md) |
| 2 — **normative specifications** (REQ / PROBE / STRAND) | [specs/](specs/) |
| 3 — design narrative, dependency mermaid | [docs/architecture.md](docs/architecture.md) |
| 4 — AI agent conventions, MCP skills | [docs/ai-workflow.md](docs/ai-workflow.md) |
| 5 — CI / contributor checks | [docs/ci.md](docs/ci.md) |
| 6 — ADRs (closed) | [docs/adr/](docs/adr/) |
| 6b — landed vs planned | [docs/roadmap.md](docs/roadmap.md) |
| 7 — release log | [CHANGELOG.md](CHANGELOG.md) |

The source of truth for module design is the in-repo [`specs/`](specs/) tree. Open research strands live in [`specs/research-strands.md`](specs/research-strands.md) until promoted ADRs land in [`docs/adr/`](docs/adr/).

## Equivalent SDK

Cadasto's **PHP SDK** targets the same openEHR REST surface and the same SMART-on-openEHR conformance probe set, with an idiomatic PHP API. Cross-language parity is enforced by the shared probe set, not by source-code mirroring.

## License

MIT — see [LICENSE](LICENSE).
