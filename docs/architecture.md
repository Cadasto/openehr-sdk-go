# Architecture

**Narrative companion to [`specs/`](../specs/).** This document describes the SDK's structure as prose and diagrams; the normative `MUST / SHOULD / MAY` statements live in [`specs/`](../specs/). When the two disagree, `specs/` wins and this document is the one to update.

> **Status: scaffolding.** The directory tree is in place via `doc.go` stubs and the spec set; concrete types and methods are not yet written. Sections below describe the *intended* shape — when implementation lands, this doc gets concrete request/response flow examples.

## Where to find what

| Need | Place |
|---|---|
| Normative requirements (REQ-NNN) | [`../specs/REQ.md`](../specs/REQ.md) |
| Glossary | [`../specs/glossary.md`](../specs/glossary.md) |
| In / out of scope | [`../specs/scope.md`](../specs/scope.md) |
| Package taxonomy + dependency rules (normative) | [`../specs/module-layout.md`](../specs/module-layout.md) |
| Idiomatic Go surface rules | [`../specs/idiom.md`](../specs/idiom.md) |
| RM modeling rules | [`../specs/rm-modeling.md`](../specs/rm-modeling.md) |
| Auth & SMART-on-openEHR contract | [`../specs/auth.md`](../specs/auth.md) |
| Wire format (REST, AQL, canonical JSON, FLAT, STRUCTURED) | [`../specs/wire.md`](../specs/wire.md) |
| Service discovery flow | [`../specs/service-discovery.md`](../specs/service-discovery.md) |
| Cross-SDK conformance probes (PROBE-NNN) | [`../specs/conformance.md`](../specs/conformance.md) |
| Use cases — primary, building-block, POC | [`../specs/use-cases.md`](../specs/use-cases.md) |
| Open research strands (STRAND-NN) | [`../specs/research-strands.md`](../specs/research-strands.md) |
| Closed architectural decisions | [`adr/`](adr/) |
| Implementation plans (per phase) | [`plans/`](plans/) |

## Package layout (summary)

The full taxonomy with package-level scope notes lives in [`../specs/module-layout.md`](../specs/module-layout.md). Each leaf has a `doc.go` stub today:

```
openehr-sdk-go/
├── auth/             smart/  clientcreds/  jwtbearer/
├── transport/
├── openehr/
│   ├── rm/           typereg/
│   ├── serialize/
│   ├── validation/
│   ├── template/
│   ├── aql/
│   ├── composition/
│   └── client/       ehr/  query/  definition/  demographic/  system/
├── smart/            discovery/
├── sandbox/
├── testkit/
├── cadasto/          extra/  datamap/  care/  mpi/  admin/
├── cmd/examples/
└── internal/
```

## Dependency direction

```mermaid
flowchart TD
  App["Application code<br/>(benchmark, seeder, MCP, federator)"]
  Care["cadasto/care/"]
  Cadasto["cadasto/extra, datamap, mpi, admin"]
  Smart["smart/<br/>(AppContext, discovery)"]
  Composition["openehr/composition/"]
  Aql["openehr/aql/"]
  Client["openehr/client/*"]
  Rm["openehr/rm/"]
  Serialize["openehr/serialize/"]
  Validation["openehr/validation/"]
  Template["openehr/template/"]
  Http["transport/"]
  Auth["auth/ (+ providers)"]
  StdHttp["net/http.Client (injected)"]
  Sandbox["sandbox/"]

  App --> Care
  App --> Smart
  App --> Composition
  App --> Aql
  App --> Client
  App --> Cadasto
  App -. building-block .-> Rm
  App -. building-block .-> Serialize
  App -. building-block .-> Validation
  App -. building-block .-> Template

  Care --> Client
  Cadasto --> Http
  Smart --> Auth
  Composition --> Template
  Composition --> Rm
  Aql --> Rm
  Client --> Http
  Client --> Rm
  Client --> Serialize
  Validation --> Rm
  Validation --> Template
  Http --> Auth
  Http --> StdHttp
  Sandbox -. implements .-> Client
  Sandbox -. implements .-> Cadasto
```

Normative rules: REQ-010 through REQ-014 in [`../specs/REQ.md`](../specs/REQ.md).

## Why it's shaped this way (narrative)

### Two cut lines, two purposes

The package tree has two named boundaries:

- **The `cadasto/` cut line** (REQ-010, REQ-011) — preserves the option of extracting Cadasto-platform extras into a sibling Go module later. Open question tracked in STRAND-08. The cut is held now regardless of resolution, because reversing it after v1 ships is expensive.
- **The building-block boundary** (REQ-013) — `openehr/rm`, `openehr/serialize`, `openehr/validation`, `openehr/template`, and `openehr/aql` (models only) must work *without* `transport/` or `auth/`. CI validators, FHIR-mapping prototypes, and AQL linters don't need HTTP; the SDK must not force the dependency.

The first cut is about future-proofing module structure; the second is about present-day consumer ergonomics.

### Idiomatic Go, not a PHP port

The PHP SDK uses repositories + exceptions; this SDK uses package-level functions + typed errors + `context.Context`-first + injected `*http.Client` + functional options. Cross-SDK parity is enforced at the **wire** (the bytes on the HTTP request, the AQL string), not at the source level (REQ-080, REQ-081). Two consumers picking the same logical operation will produce byte-identical HTTP traffic; they will not produce similar-looking source code.

### Type registry, not reflection

openEHR's RM has deep polymorphism (LOCATABLE → ENTRY → COMPOSITION; DATA_VALUE → DV_QUANTITY). Go does not have inheritance. The SDK solves this with concrete structs + embedded base structs + interfaces for abstract categories + a central type registry for `_type` decoding (REQ-030..040). No reflection-heavy tag-magic, no "generic RM node" superset type.

### Discovery is first-class

The SDK does not take a "base URL". It takes a `smart/discovery.ServiceCatalog` (REQ-070). For non-discovering backends — a static EHRbase deployment, a local CDR for testing — consumers build the catalog by hand without invoking a discovery transport.

### `internal/` is invisible

Anything under `internal/` is excluded from BC promises (REQ-005). When in doubt about whether a helper belongs in a public package or `internal/`, ask: "would a consumer write a meaningful caller against this directly?" If no, it goes in `internal/`; if yes, it goes in a named public package.

## Versioning

Semver via standard Go module versioning. Module path locked at `github.com/cadasto/openehr-sdk-go` (REQ-001, STRAND-07 resolved). `v2`+ would live under `…/v2/` per Go's semantic-import-versioning convention. The version-bump rules per change kind are in [`../specs/module-layout.md § Versioning`](../specs/module-layout.md#versioning).

## Open decisions

Tracked in [`../specs/research-strands.md`](../specs/research-strands.md). Eight strands at v0; one resolved (STRAND-07: versioning + module path); seven open. Resolutions become ADRs under [`adr/`](adr/).
