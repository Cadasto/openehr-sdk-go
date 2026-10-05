---
description: >-
  Package map for the openEHR Go SDK: building blocks that stay free of
  transport, the REST client packages, auth providers, sandbox, and testkit.
---

# Packages

The module path is `github.com/cadasto/openehr-sdk-go`. The layout below is
the published one, described in
[docs/specifications/module-layout.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/specifications/module-layout.md).
API detail is on [pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go).

These tables name the packages a consumer imports directly. Internal
packages and a few narrower helpers are left out; the layout document has the
full tree. The
[roadmap](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/roadmap.md)
records which parts have landed and which are planned, with their `REQ`
identifiers.

## Building blocks (no HTTP)

None of these import `transport/` or `auth/`. That independence is REQ-013 in
[module-layout.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/specifications/module-layout.md#req-013--building-block-independence).

| Import | Role |
|---|---|
| [`openehr/rm`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/rm) | Reference Model types, generated from the BMM this SDK builds against |
| [`openehr/rm/typereg`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/rm/typereg) | `_type` discriminator → concrete Go type |
| [`openehr/terminology`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/terminology) | openEHR terminology groups and code sets, generated from the openEHR Terminology release |
| [`openehr/serialize/canjson`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson) | Canonical JSON |
| [`openehr/serialize/canxml`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml) | Canonical XML |
| [`openehr/serialize/simplified`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified) | FLAT and STRUCTURED |
| [`openehr/template`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/template) | ADL 1.4 operational template parse and paths |
| [`openehr/templatecompile`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/templatecompile) | Compile an OPT for the builder, validator, and AQL lint |
| [`openehr/template/webtemplate`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate) | Web Template JSON export from a compiled OPT, EHRbase v2.3 ids |
| [`openehr/validation`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/validation) | Reference Model rules with no template (`ValidateRM`), a Composition or other RM root against a compiled OPT, and the AQL lint entry |
| [`openehr/instance`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/instance) | Synthesise an RM instance from a compiled template |
| [`openehr/composition`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/composition) | OPT-driven Composition builder |
| [`openehr/aql`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql) | AQL builders and request / result models |
| [`openehr/aql/parse`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql/parse) | AQL syntax parser |
| [`openehr/aql/lint`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql/lint) | Static lint |
| [`openehr/aql/contain`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql/contain) | Containment admissibility |
| [`openehr/bmm`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/bmm) | BMM loader |
| [`openehr/aom/aom14`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aom/aom14) | Archetype Object Model 1.4 |

## REST client

Typed packages over the ITS-REST Release-1.1.0 OpenAPI files kept in this
repository. [Workflow](workflow.md#which-cdr) lists the CDRs this client has
been run against.

| Import | Role |
|---|---|
| [`transport`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/transport) | Injected `*http.Client`, retries, error mapping, optional OTel |
| [`smart/discovery`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/smart/discovery) | Service catalog |
| [`smart`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/smart) | SMART launch context and ID-token claims after the `auth/smart` exchange (**partial**: app registration is open) |
| [`openehr/client/ehr`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr) | EHR identity and version metadata |
| [`openehr/client/ehr/composition`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition) | Composition CRUD |
| [`openehr/client/ehr/contribution`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution) | Multi-version commits |
| [`openehr/client/ehr/directory`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/directory) | Directory / folder |
| [`openehr/client/ehr/ehrstatus`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus) | EHR_STATUS |
| [`openehr/client/ehr/itemtags`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/itemtags) | `openehr-item-tag` headers on composition, EHR_STATUS and directory reads and on composition writes (**partial**; the dedicated ITEM_TAG endpoints are deferred) |
| [`openehr/client/query`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/query) | Ad-hoc and stored AQL |
| [`openehr/client/definition`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/definition) | Templates and stored queries |
| [`openehr/client/demographic`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/demographic) | Demographic API; `DEVELOPMENT` status inside ITS-REST Release-1.1.0 |
| [`openehr/client/system`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/system) | Capabilities and version |
| [`openehr/client/admin`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/admin) | ITS-REST admin; `DEVELOPMENT` status inside ITS-REST Release-1.1.0 |

## Auth

The generic `auth.TokenSource` sits at the bottom; the providers build on it.

| Import | Role |
|---|---|
| [`auth`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth) | `TokenSource` |
| [`auth/smart`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/smart) | SMART-on-openEHR PKCE |
| [`auth/clientcreds`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/clientcreds) | OAuth2 client credentials |
| [`auth/jwtbearer`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/jwtbearer) | JWT Bearer (RFC 7523) |
| [`auth/basic`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/basic) | HTTP Basic |

## Test and sandbox

| Import | Role |
|---|---|
| [`sandbox`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/sandbox) | In-memory openEHR backend (`http.RoundTripper`) |
| [`testkit/probe`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/testkit/probe) | Shared probe result type and catalog runner |
| `testkit/corpus` | Fixture documents kept in the repository (bodies, not HTTP recordings) |
| `testkit/recordings` | Cassette-mode HAR 1.2 recordings |

## Cadasto extras

The `cadasto/` packages ship in the same module, kept separate so they can
move to their own module later. Most of the surface is **planned**;
`cadasto/admin` has health probes today. None of it is part of the openEHR
REST contract.

| Import | Maturity |
|---|---|
| [`cadasto/admin`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/cadasto/admin) | **partial**, live/ready probes |
| [`cadasto/extra`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/cadasto/extra) | **planned** |
| [`cadasto/datamap`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/cadasto/datamap) | **planned** |
| [`cadasto/mpi`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/cadasto/mpi) | **planned** |
| [`cadasto/care`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/cadasto/care) | **planned** |
