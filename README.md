# openehr-sdk-go

[![CI](https://github.com/cadasto/openehr-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/cadasto/openehr-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cadasto/openehr-sdk-go.svg)](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/cadasto/openehr-sdk-go)](https://goreportcard.com/report/github.com/cadasto/openehr-sdk-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Cadasto/openehr-sdk-go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Version: v0.3.0](https://img.shields.io/badge/version-v0.3.0-blue)](docs/releases.md)
[![Release notes](https://img.shields.io/badge/release_notes-CHANGELOG-blue)](CHANGELOG.md)

First-party **Go SDK for openEHR** — covers openEHR REST `1.1.0-development`, the Reference Model, AQL, ADL 1.4 OPT parsing with typed primitive constraints (REQ-100 / REQ-103), template-driven composition validation (REQ-102), template-driven RM instance synthesis (REQ-107), an OPT-driven composition builder (REQ-101), and SMART-on-openEHR auth, with Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care aggregates) shipped in the same module for v1. (OET / ADL 2 are out of v1 scope.)

> **Status:** `v0.3.0` — third `v0.x` minor. Pre-1.0 minors may break public API; pin to the exact tag. See [`docs/releases.md`](docs/releases.md) for the version policy and [`docs/roadmap.md`](docs/roadmap.md) for the landed-vs-planned matrix.

```bash
go get github.com/cadasto/openehr-sdk-go@v0.3.0
```

## Use cases

The four primary consumers:

1. **Benchmark and load tools** — high-concurrency CRUD against the openEHR API; the reference CDR load harness is the first consumer.
2. **Synthetic data seeders** — OPT-guided fakers driving bulk Compositions and demographic records.
3. **MCP servers** — exposing openEHR operations as MCP tools for agentic clients, with token-forwarded auth.
4. **Federative API clients** — fan-out over multiple openEHR backends with per-node spec pinning and partial-failure handling.

Plus building-block use cases that import a single sub-package (RM modeling, codec, validation, AQL string construction, OPT parsing) without constructing an authenticated client.

## Quickstart

**New to the SDK?** Start with [docs/quick-start.md](docs/quick-start.md) and the runnable catalog in [docs/examples.md](docs/examples.md).

```bash
go get github.com/cadasto/openehr-sdk-go@v0.3.0
go run ./cmd/examples/canonical_json   # first building-block example (no network)
```

Contributors:

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
| 0 — **developer onboarding** | [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) |
| 1 — entry point for any agent | [AGENTS.md](AGENTS.md) |
| 2 — **normative specifications** (REQ / PROBE / STRAND) | [docs/specifications/](docs/specifications/) |
| 3 — design narrative, dependency mermaid | [docs/architecture.md](docs/architecture.md) |
| 4 — AI agent conventions, MCP skills, example-doc maintenance | [docs/ai-workflow.md](docs/ai-workflow.md) |
| 5 — CI / contributor checks | [docs/ci.md](docs/ci.md) |
| 6 — ADRs (closed) | [docs/adr/](docs/adr/) |
| 6b — landed vs planned | [docs/roadmap.md](docs/roadmap.md) |
| 7 — release log | [CHANGELOG.md](CHANGELOG.md) |
| 8 — release process + version policy | [docs/releases.md](docs/releases.md) |
| 9 — how to contribute | [CONTRIBUTING.md](CONTRIBUTING.md) |
| 10 — vulnerability reporting | [SECURITY.md](SECURITY.md) |

The source of truth for module design is the in-repo [`docs/specifications/`](docs/specifications/) tree. Open research strands live in [`docs/specifications/research-strands.md`](docs/specifications/research-strands.md) until promoted ADRs land in [`docs/adr/`](docs/adr/).

## Equivalent SDK

Cadasto's **PHP SDK** targets the same openEHR REST surface and the same SMART-on-openEHR conformance probe set, with an idiomatic PHP API. Cross-language parity is enforced by the shared probe set, not by source-code mirroring.

## License

MIT — see [LICENSE](LICENSE).
