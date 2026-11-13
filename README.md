# openehr-sdk-go

[![CI](https://github.com/Cadasto/openehr-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Cadasto/openehr-sdk-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cadasto/openehr-sdk-go.svg)](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Cadasto/openehr-sdk-go)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/tag/Cadasto/openehr-sdk-go?sort=semver&label=release&color=blue)](docs/releases.md)

A first-party Go SDK for openEHR. It covers the openEHR REST API, the Reference Model as typed Go structs, building and validating Compositions from operational templates, AQL, and SMART-on-openEHR authentication.

You don't have to take all of it. The building blocks (RM types, serialization, validation, AQL, template parsing) are plain packages with no dependency on the HTTP client or on auth. If all you need is to validate a Composition in a CI job, you import that one package and nothing else.

It's written the way Go code usually is: every I/O call takes a `context.Context`, you pass in your own `*http.Client`, configuration is done with functional options, and RM polymorphism goes through generics and a type registry rather than reflection.

## Try it

A COMPOSITION is the openEHR clinical document, and canonical JSON is how the REST API carries it. This decodes one into a typed struct and reads a few fields, with no network and no CDR involved:

```go
var c rm.Composition
if err := canjson.Unmarshal(body, &c); err != nil {
    log.Fatal(err)
}
fmt.Println(c.ArchetypeNodeID, c.Category.Value, len(c.Content))
```

The runnable version, [`cmd/examples/canonical_json`](cmd/examples/canonical_json/main.go), does the same against a fixture bundled with the repo, so it works straight from a clone:

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

`archetype_node_id` names the archetype the document is built on, `language` and `territory` are codes with the terminology they come from, `category` is the composition category, and `content items` counts the entries the document carries. The [examples catalogue](docs/examples.md) lists the other programs of this size and what each covers, offline and over REST, with a note on what to copy into your own application.

To use it in your own project, pin an exact tag. The SDK is pre-1.0, so a minor release can change the public API:

```bash
go get github.com/cadasto/openehr-sdk-go@v0.49.0
```

The version policy is in [releases.md](docs/releases.md), and the [roadmap](docs/roadmap.md) says what has actually landed.

## Who it's for

It was built with these kinds of consumer in mind:

1. **Benchmark and load tools** running high-concurrency CRUD against the openEHR REST API.
2. **Synthetic data seeders** that use an OPT to drive bulk Compositions and demographic records.
3. **MCP servers** that expose openEHR operations as tools for agentic clients, with a per-request token issued for the openEHR API on the user's behalf.
4. **Federative API clients** that fan out over several openEHR backends, with per-node spec pinning and partial-failure handling.
5. **SMART-on-openEHR apps with a Go backend**: server-side launch, token handling, and CDR calls from a Go web or API service.

If you only need one piece (RM modeling, a codec, validation, AQL string construction, OPT parsing), you can import that package on its own.

## What's in it

The [package map](https://cadasto.github.io/openehr-sdk-go/packages/) gives the import path for each area below, and [pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go) has the API reference.

- **openEHR REST client**: System, EHR, EHR_STATUS, Composition, Directory, Contribution, Query, Definition (stored AQL), Demographic, and Admin, over a versioned transport.
- **Reference Model**: typed RM structs and a central type registry, generated from pinned BMM dictionaries, plus hand-written identifier, temporal, and locatable-path helpers.
- **Serialization**: canonical JSON and XML round-trips, and bidirectional FLAT / STRUCTURED simplified-format codecs driven by a Web Template.
- **Templates (ADL 1.4 OPT)**: an operational template model that reads an OPT and writes it back in canonical JSON or XML, a compile step with typed primitive constraints, and WebTemplate JSON export for form generation.
- **Compositions**: an OPT-driven builder, template-driven validation, and RM-instance synthesis from a template.
- **AQL**: literal AQL wire models and result sets, fluent struct and verb builders, and static parse-and-lint.
- **Authentication**: SMART-on-openEHR (PKCE), client credentials, JWT bearer, and basic token sources, all over one injected `TokenSource`.
- **Service discovery**: a multi-backend service catalog with per-node spec pinning.
- **Cadasto platform extras**: mostly still on the roadmap. Datamap, MPI, an Extra API, and Care aggregates are **planned**. The Cadasto admin package has health probes today, and the rest of it is **partial**. These packages live in the same module behind a `cadasto/` cut line, so they can later move out as a subtree instead of being rewritten.
- **Conformance**: an openEHR wire-conformance probe suite covering round trips that keep every value and spec-correct envelopes.

## Getting started

Start with [quick-start.md](docs/quick-start.md), then the runnable catalog in [examples.md](docs/examples.md). quick-start covers toolchain setup, whether you have host Go or use the Docker fallback.

## Documentation

| Doc | What it covers |
|---|---|
| [docs/quick-start.md](docs/quick-start.md) | Install, toolchain setup, and the ways to integrate the SDK |
| [docs/examples.md](docs/examples.md) | The runnable example programs and what each one covers |
| [docs/roadmap.md](docs/roadmap.md) | What has landed and what is planned |
| [docs/releases.md](docs/releases.md) | Version policy and how releases are cut |
| [CHANGELOG.md](CHANGELOG.md) | Release log |
| [SECURITY.md](SECURITY.md) | Reporting a vulnerability |

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) explains how to report a bug, propose a change and send a pull request, and lists where the project's specifications, design notes and decisions live. If you're working on the SDK itself, `make help` lists the grouped targets and [ci.md](docs/ci.md) explains the PR gate.

## AI-assisted development

Much of this codebase and its documentation was written with AI coding assistants such as Claude Code and Cursor. Every change is measured against the written specification in this repository, has to pass the [`make ci` gate](docs/ci.md), and is reviewed before a maintainer merges it. [ai-workflow.md](docs/ai-workflow.md) explains how the assistants are set up and what they must look up rather than guess. The rules for contributing with AI help, including the `Assisted-by:` commit trailer, are in [CONTRIBUTING.md](CONTRIBUTING.md#ai-assisted-contributions).

## License

The SDK is released under the MIT licence. See [LICENSE](LICENSE).

Vendored third-party artefacts (test fixtures, openEHR specification pins, the AQL grammar) keep their upstream licences. The inventory is [LICENSING.md](LICENSING.md).
