---
description: >-
  Package map for the openEHR Go SDK — building blocks that stay free of
  transport, the REST client leaves, auth providers, sandbox, and testkit.
---

# Packages

The module path is `github.com/cadasto/openehr-sdk-go`. The taxonomy below is
the published layout in
[docs/specifications/module-layout.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/specifications/module-layout.md).
API detail is on [pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go).

Landed-versus-planned status, with `REQ` identifiers, is the
[roadmap](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/roadmap.md).
This page lists what a consumer imports. It does not restate the normative
requirements.

## Building blocks (no HTTP)

These packages must remain usable without importing `transport/` or `auth/`
(REQ-013).

| Import | Role |
|---|---|
| `openehr/rm` | Reference Model types, generated from pinned BMM |
| `openehr/rm/typereg` | `_type` discriminator → concrete Go type |
| `openehr/serialize/canjson` | Canonical JSON |
| `openehr/serialize/canxml` | Canonical XML |
| `openehr/serialize/simplified` | FLAT and STRUCTURED |
| `openehr/template` | ADL 1.4 operational template parse and paths |
| `openehr/templatecompile` | Compile an OPT for the builder, validator, and AQL lint |
| `openehr/validation` | Composition vs OPT; AQL lint entry |
| `openehr/instance` | Synthesise an RM instance from a compiled template |
| `openehr/composition` | OPT-driven Composition builder |
| `openehr/aql` | AQL builders and request / result models |
| `openehr/aql/parse` | AQL syntax parser |
| `openehr/aql/lint` | Static lint |
| `openehr/aql/contain` | Containment admissibility |
| `openehr/bmm` | BMM loader |
| `openehr/aom/aom14` | Archetype Object Model 1.4 |

## REST client

Typed leaves over the vendored
[ITS-REST](https://specifications.openehr.org/releases/ITS-REST/Release-1.1.0/)
pin. Which CDRs this repository records is on
[Workflow](workflow.md#which-cdr).

| Import | Role |
|---|---|
| `transport` | Injected `*http.Client`, retries, error mapping, optional OTel |
| `smart/discovery` | Service catalog |
| `openehr/client/ehr` | EHR identity and version metadata |
| `openehr/client/ehr/composition` | Composition CRUD |
| `openehr/client/ehr/contribution` | Multi-version commits |
| `openehr/client/ehr/directory` | Directory / folder |
| `openehr/client/ehr/ehrstatus` | EHR_STATUS |
| `openehr/client/query` | Ad-hoc and stored AQL |
| `openehr/client/definition` | Templates and stored queries |
| `openehr/client/demographic` | Demographic API (upstream: development) |
| `openehr/client/system` | Capabilities and version |
| `openehr/client/admin` | ITS-REST admin (upstream: development) |

## Auth

Generic `auth.TokenSource` at the bottom; providers on top.

| Import | Role |
|---|---|
| `auth` | `TokenSource` |
| `auth/smart` | SMART-on-openEHR PKCE |
| `auth/clientcreds` | OAuth2 client credentials |
| `auth/jwtbearer` | JWT Bearer (RFC 7523) |
| `auth/basic` | HTTP Basic |

## Test and sandbox

| Import | Role |
|---|---|
| `sandbox` | In-memory openEHR backend (`http.RoundTripper`) |
| `testkit/probe` | Shared probe result type and catalog runner |
| `testkit/cassettes` | Vendored fixture documents (bodies, not HTTP recordings) |
| `testkit/recordings` | Cassette-mode HAR 1.2 recordings |

## Cadasto extras

Shipped in the same module behind a `cadasto/` cut line. Most of this surface
is **planned**; `cadasto/admin` has health probes today (**partial**). Do not
treat these as the openEHR REST contract.

| Import | Maturity |
|---|---|
| `cadasto/admin` | **partial** — live/ready probes |
| `cadasto/extra` | **planned** |
| `cadasto/datamap` | **planned** |
| `cadasto/mpi` | **planned** |
| `cadasto/care` | **planned** |

## Who it was built for

The README names the intended consumers: benchmark and load tools, synthetic
data seeders, MCP servers that forward the caller's token, federative clients
over several backends, and SMART-on-openEHR apps with a Go backend. Importing
one building-block package is enough if that is all the job needs.
