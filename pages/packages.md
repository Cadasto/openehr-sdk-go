---
description: >-
  Package map for the openEHR Go SDK — building blocks that stay free of
  transport, the REST client packages, auth providers, sandbox, and testkit.
---

# Packages

The module path is `github.com/cadasto/openehr-sdk-go`. The layout below is
the published one, described in
[docs/specifications/module-layout.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/specifications/module-layout.md).
API detail is on [pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go).

These tables name the packages a consumer imports directly. Internal
packages and a few narrower helpers are left out; the layout document has the
full tree. Which parts have landed and which are planned, with their `REQ`
identifiers, is in the
[roadmap](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/roadmap.md).

## Building blocks (no HTTP)

These packages must remain usable without importing `transport/` or `auth/`
(REQ-013).

| Import | Role |
|---|---|
| `openehr/rm` | Reference Model types, generated from the BMM this SDK builds against |
| `openehr/rm/typereg` | `_type` discriminator → concrete Go type |
| `openehr/terminology` | openEHR terminology groups and code sets, generated from the openEHR Terminology release |
| `openehr/serialize/canjson` | Canonical JSON |
| `openehr/serialize/canxml` | Canonical XML |
| `openehr/serialize/simplified` | FLAT and STRUCTURED |
| `openehr/template` | ADL 1.4 operational template parse and paths |
| `openehr/templatecompile` | Compile an OPT for the builder, validator, and AQL lint |
| `openehr/validation` | Composition against OPT; AQL lint entry |
| `openehr/instance` | Synthesise an RM instance from a compiled template |
| `openehr/composition` | OPT-driven Composition builder |
| `openehr/aql` | AQL builders and request / result models |
| `openehr/aql/parse` | AQL syntax parser |
| `openehr/aql/lint` | Static lint |
| `openehr/aql/contain` | Containment admissibility |
| `openehr/bmm` | BMM loader |
| `openehr/aom/aom14` | Archetype Object Model 1.4 |

## REST client

Typed packages over the ITS-REST Release-1.1.0 OpenAPI files kept in this
repository. [Workflow](workflow.md#which-cdr) lists the CDRs this client has
been run against.

| Import | Role |
|---|---|
| `transport` | Injected `*http.Client`, retries, error mapping, optional OTel |
| `smart/discovery` | Service catalog |
| `openehr/client/ehr` | EHR identity and version metadata |
| `openehr/client/ehr/composition` | Composition CRUD |
| `openehr/client/ehr/contribution` | Multi-version commits |
| `openehr/client/ehr/directory` | Directory / folder |
| `openehr/client/ehr/ehrstatus` | EHR_STATUS |
| `openehr/client/ehr/itemtags` | `openehr-item-tag` headers on composition, EHR_STATUS and directory reads and on composition writes (**partial**; the dedicated ITEM_TAG endpoints are deferred) |
| `openehr/client/query` | Ad-hoc and stored AQL |
| `openehr/client/definition` | Templates and stored queries |
| `openehr/client/demographic` | Demographic API (upstream: development) |
| `openehr/client/system` | Capabilities and version |
| `openehr/client/admin` | ITS-REST admin (upstream: development) |

## Auth

The generic `auth.TokenSource` sits at the bottom; the providers build on it.

| Import | Role |
|---|---|
| `auth` | `TokenSource` |
| `auth/smart` | SMART-on-openEHR PKCE |
| `auth/clientcreds` | OAuth2 client credentials |
| `auth/jwtbearer` | JWT Bearer (RFC 7523) |
| `auth/basic` | HTTP Basic |
| `auth/introspect` | Opt-in RFC 7662 token introspection, for a consumer acting as a resource server |

## Test and sandbox

| Import | Role |
|---|---|
| `sandbox` | In-memory openEHR backend (`http.RoundTripper`) |
| `testkit/probe` | Shared probe result type and catalog runner |
| `testkit/cassettes` | Fixture documents kept in the repository (bodies, not HTTP recordings) |
| `testkit/recordings` | Cassette-mode HAR 1.2 recordings |

## Cadasto extras

The `cadasto/` packages ship in the same module, kept separate so they can
move to their own module later. Most of the surface is **planned**;
`cadasto/admin` has health probes today. None of it is part of the openEHR
REST contract.

| Import | Maturity |
|---|---|
| `cadasto/admin` | **partial** — live/ready probes |
| `cadasto/extra` | **planned** |
| `cadasto/datamap` | **planned** |
| `cadasto/mpi` | **planned** |
| `cadasto/care` | **planned** |
