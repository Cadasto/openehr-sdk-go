---
description: >-
  Runnable programs under cmd/examples/ — decode, validate, synthesise, build
  AQL, create an EHR, and run a SMART PKCE launch. All of them work offline.
---

# Examples

The programs under
[`cmd/examples/`](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples)
are short programs to copy from, not products. Between them they cover the
building blocks end to end — decode, parse, compile, validate, synthesise,
encode, and AQL — plus the smallest REST create and a SMART launch. The REST
examples talk to an in-process mock, so none of them needs a clinical data
repository (CDR).

The catalogue with flags, fixtures, and notes on what to copy into your own
application is
[docs/examples.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md)
in the repository. This page is the showcase, and a good place to start.

Fixture paths resolve relative to the source file, so each `go run` works from
any working directory inside a clone.

## Decode canonical JSON {#canonical_json}

This is the smallest building-block program there is. It decodes a
canonical-JSON Composition from the repository fixtures into a typed
`rm.Composition`, with no transport and no auth in sight.

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

Packages: `openehr/rm`, `openehr/serialize/canjson`. Fixture:
`testkit/cassettes/compositions/body_weight.json`.

## Validate JSON against a template {#validate-from-json}

This is the shape a CI check takes. The program reads the bytes, decodes them
into Reference Model objects, compiles the operational template (OPT), and
prints either `OK` or the constraint violations it found.

```bash
go run ./cmd/examples/validate-from-json
```

Packages: `canjson`, `template`, `validation`.

## Build an AQL query {#aql-build}

The struct builder and the verb functions emit the same AQL string. Use this
when the query is assembled in code rather than pasted in as text.

```bash
go run ./cmd/examples/aql-build
```

Packages: `openehr/aql`.

## Create an EHR {#create-an-ehr}

This is the smallest REST create: a static service catalog, an injected
client, and one call to `ehr.Create`. The program starts a loopback `httptest`
server in the same process, so it needs neither a CDR nor credentials.

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
| [Run a SMART PKCE launch](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/smart-launch) | `smart-launch` | In-process mock | Standalone PKCE launch; state and verifier persistence |

Build every example:

```bash
go build ./cmd/examples/...
```

If you are new to the SDK, this order works well:

1. Decode canonical JSON
2. Validate JSON against a template
3. Build an AQL query
4. Create an EHR
5. Run a SMART PKCE launch
