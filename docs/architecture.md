# Architecture

**Narrative companion to [`docs/specifications/`](../docs/specifications/).** This document describes the SDK's structure as prose and diagrams; the normative `MUST / SHOULD / MAY` statements live in [`docs/specifications/`](../docs/specifications/). When the two disagree, `docs/specifications/` wins and this document is the one to update.

> **Status: early implementation.** BMM loader, codegen, type registry, canonical JSON, canonical XML, `transport/` (incl. observer / NoRetry — REQ-096, REQ-098), auth providers (`clientcreds`, `jwtbearer`, `basic`, `smart` — PKCE/JWKS/refresh), `smart/discovery/`, `smart/` LaunchContext + RS256 ID-token validation, openEHR REST clients (`openehr/client/system`, `openehr/client/ehr` read/write incl. ItemTags, `openehr/client/definition` ADL 1.4 + stored AQL, `openehr/client/query` AQL execute, `openehr/client/admin`), `openehr/template/` (ADL 1.4 OPT parse + path utilities — REQ-100), `openehr/rm/rminfo/` (BMM lookup — [ADR 0005](adr/0005-compiled-template-foundation.md)), and `internal/templatecompile/` (compiled OPT tree) are landed. Composition builder, AQL builder, App Registration, and Cadasto extras remain open. Sections below describe both the intended shape and what runs today (`make test`, `make codegen`).

## Where to find what

| Need | Place |
|---|---|
| Requirement registry (REQ-NNN index) | [`../docs/specifications/REQ.md`](../docs/specifications/REQ.md) |
| Traceability index (machine-readable) | [`../docs/specifications/traceability.yaml`](../docs/specifications/traceability.yaml) |
| Packaging (REQ-001–005) | [`../docs/specifications/packaging.md`](../docs/specifications/packaging.md) |
| Glossary | [`../docs/specifications/glossary.md`](../docs/specifications/glossary.md) |
| In / out of scope | [`../docs/specifications/scope.md`](../docs/specifications/scope.md) |
| Package taxonomy + dependency rules (normative) | [`../docs/specifications/module-layout.md`](../docs/specifications/module-layout.md) |
| Idiomatic Go surface rules | [`../docs/specifications/idiom.md`](../docs/specifications/idiom.md) |
| RM modeling rules | [`../docs/specifications/rm-modeling.md`](../docs/specifications/rm-modeling.md) |
| Auth & SMART-on-openEHR contract | [`../docs/specifications/auth.md`](../docs/specifications/auth.md) |
| Wire format (REST, AQL, canonical JSON, FLAT, STRUCTURED) | [`../docs/specifications/wire.md`](../docs/specifications/wire.md) |
| Transport (retry, OTel, TLS posture) | [`../docs/specifications/transport.md`](../docs/specifications/transport.md) |
| Service discovery flow | [`../docs/specifications/service-discovery.md`](../docs/specifications/service-discovery.md) |
| Cross-SDK conformance probes (PROBE-NNN) | [`../docs/specifications/conformance.md`](../docs/specifications/conformance.md) |
| Use cases — primary, building-block, POC | [`../docs/specifications/use-cases.md`](../docs/specifications/use-cases.md) |
| Open research strands (STRAND-NN) | [`../docs/specifications/research-strands.md`](../docs/specifications/research-strands.md) |
| Closed architectural decisions | [`adr/`](adr/) |
| Implementation plans (per phase) | [`plans/`](plans/) |

## Package layout (summary)

The full taxonomy with package-level scope notes lives in [`../docs/specifications/module-layout.md`](../docs/specifications/module-layout.md). Landed packages are listed in [Current implementation](#current-implementation); remaining leaves are stubs or planned.

```
openehr-sdk-go/
├── auth/             smart/  clientcreds/  jwtbearer/  basic/
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

Normative rules: REQ-010 through REQ-014 in [`../docs/specifications/REQ.md`](../docs/specifications/REQ.md).

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

Anything under `internal/` is excluded from BC promises (REQ-005). Today this holds generator tooling: `internal/bmmgen` (RM/AOM/canonical JSON emission) and `internal/bmmdiff` (BMM corpus diff for version bumps). When in doubt about whether a helper belongs in a public package or `internal/`, ask: "would a consumer write a meaningful caller against this directly?" If no, it goes in `internal/`; if yes, it goes in a named public package.

## Current implementation

| Area | Location | Notes |
|---|---|---|
| Pinned BMM corpus | [`resources/bmm/`](../resources/bmm/) | Six `openehr_*.bmm.json` files; see [ADR 0001](adr/0001-bmm-version-bump-runbook.md) |
| BMM loader | [`openehr/bmm/`](../openehr/bmm/) | `LoadAll`, `FSResolver`, descendant-shadows-ancestor merge |
| Code generator | [`internal/bmmgen/`](../internal/bmmgen/), [`cmd/bmmgen`](../cmd/bmmgen) | `make codegen` / `make codegen-verify` (chained in `make test`) |
| Generated RM | [`openehr/rm/`](../openehr/rm/) | `*_gen.go`, `*_jsonmar_gen.go`, `*_jsonunmar_gen.go`, `typereg_gen.go` |
| RM structural lookup | [`openehr/rm/rminfo/`](../openehr/rm/rminfo/) | BMM-derived `lookup_gen.go`; [ADR 0005](adr/0005-compiled-template-foundation.md) |
| Compiled OPT (internal) | [`internal/templatecompile/`](../internal/templatecompile/) | `Compile`, AQL paths, implicit attrs; consumed by REQ-101/102 |
| Generated AOM 1.4 | [`openehr/aom/aom14/`](../openehr/aom/aom14/) | One-way import of `rm` for base types |
| OPT wire parser | [`openehr/template/`](../openehr/template/) | REQ-100; PROBE-022 |
| Type registry | [`openehr/rm/typereg/`](../openehr/rm/typereg/) | Hand-written `Registry`; registrations in `typereg_gen.go` per ADR 0002 |
| Canonical JSON | [`openehr/serialize/canjson/`](../openehr/serialize/canjson/) | REQ-052; PROBE-030/031 |
| Canonical XML | [`openehr/serialize/canxml/`](../openehr/serialize/canxml/) | REQ-056; PROBE-033/034; `xsi:type` dispatch via typereg; `archetype_node_id` as XSD attribute |
| Transport | [`transport/`](../transport/) | REQ-021, 054, 059, 066, 090–094, 096–098 |
| Auth | [`auth/`](../auth/), [`auth/clientcreds/`](../auth/clientcreds/), [`auth/jwtbearer/`](../auth/jwtbearer/), [`auth/basic/`](../auth/basic/) | REQ-060, 066, 068, 069 |
| Discovery | [`smart/discovery/`](../smart/discovery/) | REQ-070–072, 092 |
| REST clients | [`openehr/client/system/`](../openehr/client/system/), [`openehr/client/ehr/`](../openehr/client/ehr/) (+ composition, ehrstatus, directory, contribution, itemtags), [`openehr/client/query/`](../openehr/client/query/), [`openehr/client/definition/`](../openehr/client/definition/) (templates + stored AQL), [`openehr/client/admin/`](../openehr/client/admin/) | REQ-054, 055, 057, 059, 099; PROBE-010–013, 067, 070 |
| Conformance probes | [`testkit/probes/`](../testkit/probes/) | `serialize/` (030–031, 033–034), `versioned/` (010–013), `discovery/` (040–041), `definition/` (067), `admin/` (070) |

### BMM codegen pipeline

```mermaid
flowchart LR
  BMM["resources/bmm/*.bmm.json"]
  Load["openehr/bmm LoadAll"]
  Gen["internal/bmmgen"]
  RM["openehr/rm *_gen.go"]
  AOM["openehr/aom/aom14"]
  Reg["typereg_gen.go"]
  JSON["*_jsonmar_gen.go / *_jsonunmar_gen.go"]

  BMM --> Load --> Gen
  Gen --> RM
  Gen --> AOM
  Gen --> Reg
  Gen --> JSON
```

Load-bearing structural choices (flat packages, merge policy, typereg placement, abstract flattening, AOM→RM import, function stubs) are recorded in [ADR 0002 — BMM codegen decisions](adr/0002-bmm-codegen-decisions.md). Normative conformance rules remain in [`docs/specifications/bmm-conformance.md`](../docs/specifications/bmm-conformance.md).

## Versioning

Semver via standard Go module versioning. Module path locked at `github.com/cadasto/openehr-sdk-go` (REQ-001, STRAND-07 resolved). `v2`+ would live under `…/v2/` per Go's semantic-import-versioning convention. The version-bump rules per change kind are in [`../docs/specifications/module-layout.md § Versioning`](../docs/specifications/module-layout.md#versioning).

## Open decisions

Tracked in [`../docs/specifications/research-strands.md`](../docs/specifications/research-strands.md). STRAND-07 resolved (versioning + module path); STRAND-04 partially resolved (EVENT + numeric wire — ADRs 0003–0004). Five ADRs Accepted under [`adr/`](adr/) (0001–0005). Resolutions become ADRs here.
