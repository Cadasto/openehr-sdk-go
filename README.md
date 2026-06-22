# openehr-sdk-go

[![CI](https://github.com/cadasto/openehr-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/cadasto/openehr-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cadasto/openehr-sdk-go.svg)](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/cadasto/openehr-sdk-go)](https://goreportcard.com/report/github.com/cadasto/openehr-sdk-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Cadasto/openehr-sdk-go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/tag/Cadasto/openehr-sdk-go?sort=semver&label=release&color=blue)](docs/releases.md)
[![Release notes](https://img.shields.io/badge/release_notes-CHANGELOG-blue)](CHANGELOG.md)

A first-party, idiomatic **Go SDK for openEHR**. It lets you talk to openEHR REST CDRs, model Reference Model data in typed Go, build and validate clinical Compositions from operational templates, construct AQL, and authenticate with SMART-on-openEHR — all from one module. Cadasto-platform extras (Datamap, MPI, Extra API, Admin, Care aggregates) ride along in the same module for v1 convenience, behind a clean boundary so they can be split out later.

The SDK is **openEHR-first** and idiomatic Go: `context.Context` on every I/O call, an injected `*http.Client`, functional options, and generics instead of reflection. Its core building blocks — RM modeling, serialization, validation, AQL, and template parsing — are usable standalone, without constructing an authenticated client. Every behaviour traces back to an in-repo normative specification (see [Spec-driven design](#spec-driven-design-sdd)).

> **Pre-1.0.** Public API may change across minor releases — pin to an exact tag. See [`docs/releases.md`](docs/releases.md) for the version policy and [`docs/roadmap.md`](docs/roadmap.md) for the landed-vs-planned matrix.

```bash
go get github.com/cadasto/openehr-sdk-go@latest   # pre-1.0: pin an exact tag for production
```

## Use cases

The primary consumers:

1. **Benchmark and load tools** — high-concurrency CRUD against the openEHR REST API for capacity planning.
2. **Synthetic data seeders** — OPT-guided fakers driving bulk Compositions and demographic records.
3. **MCP servers** — exposing openEHR operations as MCP tools for agentic clients, with token-forwarded auth.
4. **Federative API clients** — fan-out over multiple openEHR backends with per-node spec pinning and partial-failure handling.
5. **openEHR SMART apps with a Go backend** — server-side SMART-on-openEHR launch, token handling, and CDR calls behind a Go web or API service.

Plus building-block use cases that import a single sub-package (RM modeling, codec, validation, AQL string construction, OPT parsing) without constructing an authenticated client.

## Functionality

What the SDK provides today and what's planned. The authoritative landed-vs-planned status and the REQ / PROBE identifiers live in the [roadmap matrix](docs/roadmap.md) and the [REQ registry](docs/specifications/REQ.md).

- **openEHR REST client** — System, EHR, EHR_STATUS, Composition, Directory, Contribution, Query, Definition (stored AQL), and Admin operations over a versioned transport. → [wire](docs/specifications/wire.md), [transport](docs/specifications/transport.md)
- **Reference Model** — typed RM structs with a central type registry, generated from pinned BMM dictionaries, plus hand-written identifier, temporal, and locatable-path helpers. → [rm-modeling](docs/specifications/rm-modeling.md)
- **Serialization** — canonical JSON and XML round-trips; FLAT / STRUCTURED planned. → [wire](docs/specifications/wire.md)
- **Templates (ADL 1.4 OPT)** — operational-template parsing with typed primitive constraints and a compiled-template foundation. → [rm-modeling](docs/specifications/rm-modeling.md)
- **Compositions** — OPT-driven builder, template-driven validation, and RM-instance synthesis from a template. → [wire](docs/specifications/wire.md)
- **AQL** — literal AQL wire models and result sets, fluent struct/verb builders, and static parse-and-lint. → [wire](docs/specifications/wire.md)
- **Authentication** — SMART-on-openEHR (PKCE), client-credentials, JWT-bearer, and basic token sources, layered over a generic injected `TokenSource`. → [auth](docs/specifications/auth.md)
- **Service discovery** — multi-backend service catalog with per-node spec pinning and partial-failure handling. → [service-discovery](docs/specifications/service-discovery.md)
- **Cadasto platform extras** — Datamap, MPI, Extra API, Admin, and Care aggregates, shipped in-module for v1 behind a clean cut line. → [module-layout](docs/specifications/module-layout.md)
- **Conformance** — an openEHR wire-conformance probe suite (round-trip byte-stability, spec-correct envelopes). → [conformance](docs/specifications/conformance.md)

_OET and ADL 2 are out of v1 scope._

## Quickstart

**New to the SDK?** Start with [docs/quick-start.md](docs/quick-start.md) and the runnable catalog in [docs/examples.md](docs/examples.md).

```bash
go get github.com/cadasto/openehr-sdk-go@latest
go run ./cmd/examples/canonical_json   # first building-block example (no network)
```

Contributors:

```bash
make help        # grouped targets (toolchain, test, lint, CI, …)
make ci          # full PR gate (see docs/ci.md)
make test        # unit tests (+ codegen drift check)
make fmt         # gofumpt + goimports (via golangci-lint)
```

Toolchain setup — host Go vs. the Docker fallback — is covered in [docs/quick-start.md](docs/quick-start.md).

## Documentation

### Spec-driven design (SDD)

The normative source of truth and the design that realises it. When code and specs disagree, the specs win.

| Doc | Scope |
|---|---|
| [docs/specifications/](docs/specifications/) | **Normative specs** — REQ / PROBE / STRAND topic specs |
| [docs/specifications/REQ.md](docs/specifications/REQ.md) | Requirement registry (index → canonical topic spec) |
| [docs/specifications/traceability.yaml](docs/specifications/traceability.yaml) | Machine-readable REQ → package / probe / test map |
| [docs/architecture.md](docs/architecture.md) | Design narrative + dependency mermaid |
| [docs/adr/](docs/adr/) | Closed architectural decisions |
| [docs/roadmap.md](docs/roadmap.md) | Landed-vs-planned matrix |

Open research strands live in [research-strands.md](docs/specifications/research-strands.md) until promoted ADRs land in [docs/adr/](docs/adr/).

### Onboarding & process

| Doc | Scope |
|---|---|
| [docs/quick-start.md](docs/quick-start.md) · [docs/examples.md](docs/examples.md) | Developer onboarding + runnable catalog |
| [AGENTS.md](AGENTS.md) | Entry point for coding agents |
| [docs/ai-workflow.md](docs/ai-workflow.md) | AI agent conventions, MCP skills, example-doc upkeep |
| [docs/ci.md](docs/ci.md) | CI and contributor checks |
| [CHANGELOG.md](CHANGELOG.md) | Release log |
| [docs/releases.md](docs/releases.md) | Release process + version policy |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting |

## License

MIT — see [LICENSE](LICENSE).
