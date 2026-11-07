---
description: >-
  Package map for the openEHR Go SDK: building blocks that stay free of
  transport, the REST client packages, auth providers, sandbox, and testkit.
---

# Packages

The module path is `github.com/cadasto/openehr-sdk-go`. API detail is on [pkg.go.dev](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go).

These tables name the packages a consumer imports directly. Internal
packages and a few narrower helpers are left out; pkg.go.dev lists every
package. The
[roadmap](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/roadmap.md)
records which parts have landed and which are planned. Where a runnable
program shows a package at work, the Example column links its entry in the
[examples catalogue](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md).

## Building blocks (no HTTP)

None of these import `transport/` or `auth/`, directly or through another
package.

| Import | Role | Example |
|---|---|---|
| [`openehr/rm`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/rm) | Reference Model types, generated from the BMM this SDK builds against | [`canonical_json`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#canonical_json) |
| [`openehr/rm/typereg`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/rm/typereg) | `_type` discriminator → concrete Go type | [`query-execute`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#query-execute) |
| [`openehr/terminology`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/terminology) | openEHR terminology groups and code sets, generated from the openEHR Terminology release | [`validate-composition`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#validate-composition) |
| [`openehr/serialize/canjson`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson) | Canonical JSON | [`canonical_json`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#canonical_json) |
| [`openehr/serialize/canxml`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml) | Canonical XML | [`canxml_roundtrip`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#canxml_roundtrip) |
| [`openehr/serialize/simplified`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified) | FLAT and STRUCTURED | [`flat-roundtrip`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#flat-roundtrip) |
| [`openehr/aom/aom14`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aom/aom14) | Archetype Object Model 1.4 and the ADL 1.4 operational template model: `ReadOperationalTemplate` reads an OPT, and both canonical codecs write it back | [`opt-parse`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#opt-parse) |
| [`openehr/template`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/template) | ADL 1.4 operational template path grammar and the small types a compiled template shares | [`opt-parse`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#opt-parse) |
| [`openehr/templatecompile`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/templatecompile) | Compile an OPT for the builder, validator, and AQL lint | [`compile-build-validate`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#compile-build-validate) |
| [`openehr/template/webtemplate`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate) | Web Template JSON export from a compiled OPT, EHRbase v2.3 ids | [`webtemplate-export`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#webtemplate-export) |
| [`openehr/validation`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/validation) | Reference Model rules with no template (`ValidateRM`), a Composition or other RM root against a compiled OPT, a canonical JSON or XML document decoded and validated in one call (`ValidateJSON`, `ValidateXML`), and the AQL lint entry | [`validate-from-json`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#validate-from-json) |
| [`openehr/instance`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/instance) | Synthesise an RM instance from a compiled template | [`generate-example`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#generate-example) |
| [`openehr/composition`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/composition) | OPT-driven Composition builder | [`compile-build-validate`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#compile-build-validate) |
| [`openehr/aql`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql) | AQL builders and request / result models | [`aql-build`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#aql-build) |
| [`openehr/aql/parse`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql/parse) | AQL syntax parser | [`aql-parse-structured`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#aql-parse-structured) |
| [`openehr/aql/lint`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql/lint) | Static lint | [`lint-aql`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#lint-aql) |
| [`openehr/aql/contain`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/aql/contain) | Containment admissibility | [`aql-build`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#aql-build) |
| [`openehr/bmm`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/bmm) | BMM loader | |

## REST client

Typed packages over the ITS-REST Release-1.1.0 OpenAPI files kept in this
repository. [Workflow](workflow.md#which-cdr) lists the CDRs this client has
been run against.

| Import | Role | Example |
|---|---|---|
| [`transport`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/transport) | Injected `*http.Client`, retries, error mapping, optional OTel | [`ehr_create`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#ehr_create) |
| [`smart/discovery`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/smart/discovery) | Service catalog | [`ehr_create`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#ehr_create) |
| [`smart`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/smart) | SMART launch context and ID-token claims after the `auth/smart` exchange (**partial**: app registration is open) | |
| [`openehr/client/ehr`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr) | EHR identity and version metadata | [`ehr_create`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#ehr_create) |
| [`openehr/client/ehr/composition`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition) | Composition CRUD in canonical JSON, canonical XML, FLAT or STRUCTURED, revision history and version reads | [`composition-crud`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#composition-crud) |
| [`openehr/client/ehr/contribution`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution) | Multi-version commits | [`contribution-build`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#contribution-build) |
| [`openehr/client/ehr/directory`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/directory) | Directory / folder | |
| [`openehr/client/ehr/ehrstatus`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/ehrstatus) | EHR_STATUS, its revision history and version reads | |
| [`openehr/client/ehr/itemtags`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/ehr/itemtags) | `openehr-item-tag` headers on composition, EHR_STATUS and directory reads and on composition writes (**partial**; the dedicated ITEM_TAG endpoints are deferred) | |
| [`openehr/client/query`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/query) | Ad-hoc and stored AQL | [`query-execute`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#query-execute) |
| [`openehr/client/definition`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/definition) | Templates and stored queries | [`definition-lifecycle`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#definition-lifecycle) |
| [`openehr/client/demographic`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/demographic) | Demographic API, including PARTY revision history and version reads; `DEVELOPMENT` status inside ITS-REST Release-1.1.0 | |
| [`openehr/client/system`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/system) | Capabilities and version | [`service-auth`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#service-auth) |
| [`openehr/client/admin`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/openehr/client/admin) | ITS-REST admin; `DEVELOPMENT` status inside ITS-REST Release-1.1.0 | |

## Auth

The generic `auth.TokenSource` sits at the bottom; the providers build on it.

| Import | Role | Example |
|---|---|---|
| [`auth`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth) | `TokenSource` | [`service-auth`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#service-auth) |
| [`auth/smart`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/smart) | SMART-on-openEHR PKCE | [`smart-launch`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#smart-launch) |
| [`auth/clientcreds`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/clientcreds) | OAuth2 client credentials | [`service-auth`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#service-auth) |
| [`auth/jwtbearer`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/jwtbearer) | JWT Bearer (RFC 7523) | |
| [`auth/basic`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/auth/basic) | HTTP Basic | |

## Test and sandbox

| Import | Role | Example |
|---|---|---|
| [`sandbox`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/sandbox) | In-memory openEHR backend (`http.RoundTripper`) | [`composition-crud`](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md#composition-crud) |
| [`testkit/probe`](https://pkg.go.dev/github.com/cadasto/openehr-sdk-go/testkit/probe) | Shared probe result type and catalog runner | |
| `testkit/corpus` | Fixture documents kept in the repository (bodies, not HTTP recordings) | |
| `testkit/recordings` | Cassette-mode HAR 1.2 recordings | |

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
