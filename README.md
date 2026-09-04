# openehr-sdk-go

[![CI](https://github.com/Cadasto/openehr-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Cadasto/openehr-sdk-go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/Cadasto/openehr-sdk-go/actions/workflows/codeql.yml/badge.svg)](https://github.com/Cadasto/openehr-sdk-go/actions/workflows/codeql.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cadasto/openehr-sdk-go.svg)](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Cadasto/openehr-sdk-go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/tag/Cadasto/openehr-sdk-go?sort=semver&label=release&color=blue)](docs/releases.md)

A first-party Go SDK for openEHR. It covers the openEHR REST API, the Reference Model as typed Go structs, building and validating Compositions from operational templates, AQL, and SMART-on-openEHR authentication.

You don't have to take all of it. The building blocks (RM types, serialization, validation, AQL, template parsing) are plain packages with no dependency on the HTTP client or on auth. If all you need is to validate a Composition in a CI job, you import that one package and nothing else.

Everything the SDK does traces back to a specification that lives in this repository. When the code and a spec disagree, the spec wins.

It's written the way Go code usually is: every I/O call takes a `context.Context`, you pass in your own `*http.Client`, configuration is done with functional options, and RM polymorphism goes through generics and a type registry rather than reflection.

## Try it

This decodes a canonical-JSON Composition into a typed struct and reads a few fields. No network, no CDR:

```go
var c rm.Composition
if err := canjson.Unmarshal(body, &c); err != nil {
    log.Fatal(err)
}
fmt.Println(c.ArchetypeNodeID, c.Category.Value, len(c.Content))
```

The runnable version uses a fixture bundled with the repo, so it works straight from a clone:

```console
$ go run ./cmd/examples/canonical_json
composition: archetype_node_id=openEHR-EHR-COMPOSITION.encounter.v1
  name="body_weight"
  language=nl (terminology=ISO_639-1)
  territory=NL
  category=event
  content items=1
OK: canonical-JSON Composition decoded from body_weight.json
```

To use it in your own project:

```bash
go get github.com/cadasto/openehr-sdk-go@latest
```

The SDK is pre-1.0. A minor release can change the public API, so pin an exact tag in anything you ship. The version policy is in [releases.md](docs/releases.md), and the [roadmap](docs/roadmap.md) says what has actually landed.

## Who it's for

It was built with a few kinds of consumer in mind:

1. **Benchmark and load tools** running high-concurrency CRUD against the openEHR REST API.
2. **Synthetic data seeders** that use an OPT to drive bulk Compositions and demographic records.
3. **MCP servers** that expose openEHR operations as tools for agentic clients, forwarding the caller's token.
4. **Federative API clients** that fan out over several openEHR backends, with per-node spec pinning and partial-failure handling.
5. **SMART-on-openEHR apps with a Go backend**: server-side launch, token handling, and CDR calls from a Go web or API service.

If you only need one piece (RM modeling, a codec, validation, AQL string construction, OPT parsing), you can import that package on its own.

## What's in it

The definitive landed-vs-planned status, with REQ and PROBE identifiers, is in the [roadmap](docs/roadmap.md) and the [REQ registry](docs/specifications/REQ.md).

- **openEHR REST client** — System, EHR, EHR_STATUS, Composition, Directory, Contribution, Query, Definition (stored AQL), and Admin, over a versioned transport. [wire](docs/specifications/wire.md), [transport](docs/specifications/transport.md)
- **Reference Model** — typed RM structs and a central type registry, generated from pinned BMM dictionaries, plus hand-written identifier, temporal, and locatable-path helpers. [rm-modeling](docs/specifications/rm-modeling.md)
- **Serialization** — canonical JSON and XML round-trips, and bidirectional FLAT / STRUCTURED simplified-format codecs driven by a Web Template. [wire](docs/specifications/wire.md)
- **Templates (ADL 1.4 OPT)** — operational-template parsing with typed primitive constraints, a compiled-template foundation, and WebTemplate JSON export for form generation. [rm-modeling](docs/specifications/rm-modeling.md)
- **Compositions** — an OPT-driven builder, template-driven validation, and RM-instance synthesis from a template. [wire](docs/specifications/wire.md)
- **AQL** — literal AQL wire models and result sets, fluent struct and verb builders, and static parse-and-lint. [wire](docs/specifications/wire.md)
- **Authentication** — SMART-on-openEHR (PKCE), client credentials, JWT bearer, and basic token sources, all over one injected `TokenSource`. [auth](docs/specifications/auth.md)
- **Service discovery** — a multi-backend service catalog with per-node spec pinning and partial-failure handling. [service-discovery](docs/specifications/service-discovery.md)
- **Cadasto platform extras** — Datamap, MPI, Extra API, Admin, and Care aggregates. These ship in the same module for v1, behind a `cadasto/` cut line so they can be split out later as a subtree move rather than a rewrite. [module-layout](docs/specifications/module-layout.md)
- **Conformance** — an openEHR wire-conformance probe suite covering round-trip byte-stability and spec-correct envelopes. [conformance](docs/specifications/conformance.md)

## Getting started

Start with [quick-start.md](docs/quick-start.md), then the runnable catalog in [examples.md](docs/examples.md). Both cover toolchain setup, whether you have host Go or use the Docker fallback.

If you're working on the SDK itself, `make help` lists the grouped targets and [ci.md](docs/ci.md) explains the PR gate.

## AI-assisted development

Much of this codebase and its documentation was written with AI coding assistants such as Claude Code and Cursor. The written specification is what keeps that honest: every change is measured against the specs in this repository, has to pass the [`make ci` gate](docs/ci.md), and is reviewed before a maintainer merges it. How the assistants are set up, and what they must look up rather than guess, is in [ai-workflow.md](docs/ai-workflow.md). The rules for contributing with AI help, including the `Assisted-by:` commit trailer, are in [CONTRIBUTING.md](CONTRIBUTING.md#ai-assisted-contributions).

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
| [docs/development-process.md](docs/development-process.md) | How work flows: the SDD ladder and its gates |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [docs/ci.md](docs/ci.md) | CI and contributor checks |
| [docs/releases.md](docs/releases.md) | Release process + version policy |
| [CHANGELOG.md](CHANGELOG.md) | Release log |
| [AGENTS.md](AGENTS.md) | Entry point for coding agents |
| [docs/ai-workflow.md](docs/ai-workflow.md) | AI agent conventions, MCP skills, example-doc upkeep |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting |

## License

MIT — see [LICENSE](LICENSE).
