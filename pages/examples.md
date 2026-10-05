---
description: >-
  Runnable programs under cmd/examples/: decode, validate, synthesise, build
  and run AQL, call the REST API, and authenticate with a SMART PKCE launch or
  client credentials. All of them work offline.
---

# Examples

The programs under
[`cmd/examples/`](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples)
are short programs to copy from. Between them they cover the building blocks
end to end (decode, parse, compile, validate, synthesise, encode, and AQL).
They also cover REST calls for EHRs, compositions, AQL queries, templates and
contributions, and two ways to authenticate: a SMART launch for an app with a
signed-in user, and client credentials for a service with no user. The REST
examples talk to a mock server in the same process, so none of them needs a
clinical data repository (CDR) or an account.

The full catalogue in the repository,
[docs/examples.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md),
lists each program's flags and fixtures, with notes on what to copy into your
own application. This page covers four of them in detail.

Fixture paths resolve relative to the source file, so each `go run` works from
any working directory inside a clone.

## Decode canonical JSON {#canonical_json}

This is the smallest building-block program in the repository. It decodes a
canonical-JSON Composition from the fixtures into a typed `rm.Composition`,
without importing transport or auth.

```bash
go run ./cmd/examples/canonical_json
```

```text
composition: archetype_node_id=openEHR-EHR-COMPOSITION.encounter.v1
  name="body_weight"
  language=nl (terminology=ISO_639-1)
  territory=NL
  category=event
  content items=1
OK: canonical-JSON Composition decoded from body_weight.json
```

`archetype_node_id` names the archetype the document is built on, `language`
and `territory` are codes with the terminology they come from, and
`content items` counts the entries the document carries.

Packages: `openehr/rm`, `openehr/serialize/canjson`. Fixture:
`testkit/corpus/compositions/body_weight.json`.

## Validate JSON against a template {#validate-from-json}

A CI check has this shape. The program reads the bytes, decodes them into
Reference Model objects, compiles the operational template (OPT), and runs two
checks. The RM floor checks the Reference Model's own rules and needs no
template. The template constraints check what the OPT declares. Each check
prints its verdict and any issues it found. The exit status is 1 when either
check finds an issue or the program cannot run, and 2 on a bad flag, so the
command can gate a pipeline.

```bash
go run ./cmd/examples/validate-from-json
go run ./cmd/examples/validate-from-json -corpus
go run ./cmd/examples/validate-from-json composition.json template.opt
```

The first form validates a bundled composition that passes, `-corpus`
validates demo data that reports issues, and two paths validate your own
files.

Packages: `rm`, `canjson`, `template`, `templatecompile`, `validation`.

## Build an AQL query {#aql-build}

The struct builder and the verb functions emit the same AQL string. Use this
when the query is assembled in code rather than pasted in as text. The program
also shows nested CONTAINS clauses with in-text LIMIT and OFFSET, and the
opt-in `VerifyContainment` check that asks whether the classes in a query can
contain one another under the Reference Model. `Build` never asks that.

```bash
go run ./cmd/examples/aql-build
```

Packages: `openehr/aql`, `openehr/aql/contain`.

## Create an EHR {#create-an-ehr}

Three pieces make the smallest REST create: a static service catalog, an
injected client, and one call to `ehr.Create`. The program starts a loopback
`httptest` server in the same process, so it needs neither a CDR nor
credentials.

```bash
go run ./cmd/examples/ehr_create
```

Packages: `smart/discovery`, `transport`, `openehr/client/ehr`. For the same
call made through `sandbox.Backend` as the transport, see
[quick-start Path B](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/quick-start.md).

## The rest of the catalogue

| Example | Program | Network | Demonstrates |
|---|---|---|---|
| [JSON and XML round-trip](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/canxml_roundtrip) | `canxml_roundtrip` | None | JSON ↔ XML cross-format invariant |
| [Parse an operational template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/opt-parse) | `opt-parse` | None | Parse ADL 1.4 OPT, walk paths |
| [Validate primitive constraints](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/primitive-validate) | `primitive-validate` | None | Primitive constraint validation |
| [Validate a Composition against a template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/validate-composition) | `validate-composition` | None | In-memory Composition against an OPT |
| [Synthesise a Composition from a template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/generate-example) | `generate-example` | None | OPT → synthesised RM instance → JSON |
| [Parse AQL into a structured tree](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/aql-parse-structured) | `aql-parse-structured` | None | Parse AQL → structured tree + emit |
| [Lint an AQL query](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/lint-aql) | `lint-aql` | None | AQL static lint + `ValidateAQL` |
| [Compile, build, and validate](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/compile-build-validate) | `compile-build-validate` | None | Public compile → build → validate |
| [Explore a compiled template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/template-explore) | `template-explore` | None | Compiled OPT structure + leaf paths |
| [Export a Web Template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/webtemplate-export) | `webtemplate-export` | None | Compiled OPT → EHRbase Web Template JSON |
| [FLAT and STRUCTURED round-trip](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/flat-roundtrip) | `flat-roundtrip` | None | COMPOSITION ↔ FLAT / STRUCTURED |
| [Build a Contribution](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/contribution-build) | `contribution-build` | In-process mock, with `-commit` | Fluent `Contribution_create` assembly |
| [Manage a template on the server](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/definition-lifecycle) | `definition-lifecycle` | In-process mock | Upload, list, download and compile a template |
| [Save, read and update a Composition](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/composition-crud) | `composition-crud` | In-process mock | Optimistic concurrency with `If-Match`; a stale update refused |
| [Execute an AQL query](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/query-execute) | `query-execute` | In-process mock | Bound parameters, RESULT_SET cells as typed values, a refused query |
| [Run a SMART PKCE launch](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/smart-launch) | `smart-launch` | In-process mock | Standalone PKCE launch; state and verifier persistence |
| [Authenticate a backend service](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/service-auth) | `service-auth` | In-process mock | OAuth 2.0 client credentials; the cached token reused |

Build every example:

```bash
go build ./cmd/examples/...
```

If you are new to the SDK, try them in this order:

1. Decode canonical JSON
2. Validate JSON against a template
3. Build an AQL query
4. Create an EHR
5. Manage a template on the server
6. Save, read and update a Composition
7. Execute an AQL query
8. Run a SMART PKCE launch
9. Authenticate a backend service
