---
description: >-
  Runnable programs under cmd/examples/ — decode, validate, synthesise,
  build AQL, create an EHR, and run a SMART PKCE launch. All work offline.
---

# Examples

Programs under
[`cmd/examples/`](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples)
cover each major SDK surface. They are reference shapes, not products.
REST examples use an in-process mock, so nothing needs a CDR.

The catalog with flags, fixtures, and “what to copy into your app” notes is
[docs/examples.md](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/examples.md)
in the repository. This page is the showcase — start here.

Fixture paths resolve relative to the source file, so each `go run` works from
any working directory inside a clone.

## Decode canonical JSON {#canonical_json}

Smallest building-block path: decode vendored canonical JSON into a typed
`rm.Composition`. No transport, no auth.

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

CI shape: bytes → RM → compiled OPT → validation issues.

```bash
go run ./cmd/examples/validate-from-json
```

Packages: `canjson`, `template`, `validation`. Prints `OK` or the constraint
violations.

## Build an AQL query {#aql-build}

Struct builder and verb functions emit the same AQL string.

```bash
go run ./cmd/examples/aql-build
```

Packages: `openehr/aql`. Use this when the query is assembled in code rather
than pasted as text.

## Create an EHR {#create-an-ehr}

Smallest REST create: a static catalog, an injected client, `ehr.Create`.
The example stands up an in-process handler — no listener, no credentials.

```bash
go run ./cmd/examples/ehr_create
```

Packages: `smart/discovery`, `transport`, `openehr/client/ehr`. For the same
call through `sandbox.Backend` as the transport, see
[quick-start Path B](https://github.com/cadasto/openehr-sdk-go/blob/main/docs/quick-start.md).

## The rest of the catalog

| Example | Program | Network | Demonstrates |
|---|---|---|---|
| [JSON and XML round-trip](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/canxml_roundtrip) | `canxml_roundtrip` | No | JSON ↔ XML cross-format invariant |
| [Parse an operational template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/opt-parse) | `opt-parse` | No | Parse ADL 1.4 OPT, walk paths |
| [Validate primitive constraints](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/primitive-validate) | `primitive-validate` | No | Primitive constraint validation |
| [Validate a composition against a template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/validate-composition) | `validate-composition` | No | In-memory composition vs OPT |
| [Synthesise a composition from a template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/generate-example) | `generate-example` | No | OPT → synthesised RM instance → JSON |
| [Parse AQL into a structured tree](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/aql-parse-structured) | `aql-parse-structured` | No | Parse AQL → structured AST + emit |
| [Lint an AQL query](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/lint-aql) | `lint-aql` | No | AQL static lint + `ValidateAQL` |
| [Compile, build, and validate](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/compile-build-validate) | `compile-build-validate` | No | Public compile → build → validate |
| [Explore a compiled template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/template-explore) | `template-explore` | No | Compiled OPT structure + leaf paths |
| [Export a Web Template](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/webtemplate-export) | `webtemplate-export` | No | Compiled OPT → EHRbase WebTemplate JSON |
| [FLAT and STRUCTURED round-trip](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/flat-roundtrip) | `flat-roundtrip` | No | COMPOSITION ↔ FLAT / STRUCTURED |
| [Build a contribution](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/contribution-build) | `contribution-build` | Optional mock | Fluent `Contribution_create` assembly |
| [Run a SMART PKCE launch](https://github.com/cadasto/openehr-sdk-go/tree/main/cmd/examples/smart-launch) | `smart-launch` | Mock | Standalone PKCE launch; state + verifier persistence |

Build every example:

```bash
go build ./cmd/examples/...
```

Suggested order if you are new: Decode canonical JSON → Validate JSON
against a template → Build an AQL query → Create an EHR → Run a SMART
PKCE launch.
