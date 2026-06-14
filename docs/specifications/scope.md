# Scope

**Status:** Draft

What the `openehr-sdk-go` v1 surface includes and excludes. Out-of-scope items are not gaps — they are deliberate boundaries with named handling elsewhere.

## In scope

| Area | Coverage |
|---|---|
| openEHR REST `1.1.0-development` — primary surface | EHR, Composition, Contribution, Directory, EHR_STATUS, ItemTags, Template/Definition, Demographic, AQL, System endpoints |
| openEHR Reference Model | Concrete types for clinical and demographic RM (excluding the `ehr_extract` package — deferred), type registry, embedded base structs, interfaces for abstract categories ([rm-modeling.md](rm-modeling.md)) — **generated** from the pinned BMM schema in [`../resources/bmm/`](../resources/bmm/) per [bmm-conformance.md](bmm-conformance.md) |
| Archetype Object Model 1.4 | `openehr/aom/aom14/` — generated from `openehr_am_1.4.0.bmm.json`; sibling of `openehr/rm/`. Consumed by `openehr/template/` for parsing ADL 1.4 archetypes embedded in OPTs. |
| BMM loader | Public `openehr/bmm/` (hand-written for v1) — P_BMM JSON parser, includes resolution, queryable in-memory model. Building block (REQ-045). |
| Code generator | In-tree `internal/bmmgen` + `cmd/bmmgen` emitting RM + AOM 1.4 + type registry — drift-checked in CI |
| openEHR Archetype Query Language (AQL) | Struct-builder and verb-function builders, request/result models, executor in `openehr/client/query` ([wire.md § AQL](wire.md#aql)) |
| Operational template (OPT) handling | ADL 1.4 `.opt` parse and path utilities (`openehr/template/`); OPT-driven generic Composition builder. OET (`.oet`) parsing is out of scope for v1. |
| Canonical JSON / FLAT / STRUCTURED codecs | All three openEHR serialization shapes, independently usable ([wire.md § Canonical JSON](wire.md#canonical-json), [§ Simplified formats](wire.md#simplified-formats)) |
| SMART-on-openEHR authentication | PKCE, JWKS rotation, token refresh, launch context ([auth.md](auth.md)) |
| Auth providers — alternative grants | Client Credentials, JWT Bearer; abstracted under `auth.TokenSource` |
| Service discovery | First-class `ServiceCatalog`, cached, refresh-able, with hand-built catalogs for non-discovering backends ([service-discovery.md](service-discovery.md)) |
| Cadasto-platform extras | Cadasto Extra API, **Datamap V2** (REQ-058), minimal MPI search (preview), Admin endpoints, Care aggregates — shipped in the same module under `cadasto/` ([module-layout.md § Cadasto extras](module-layout.md#cadasto-extras)) |
| Sandbox + recorded fixtures | In-memory and cassette-replay transports for hermetic SDK-consumer tests |
| Testkit + conformance probes | Test doubles, fluent builders, the cross-SDK probe runner ([conformance.md](conformance.md)) |
| Cross-SDK parity contract | Wire-level parity with the Cadasto PHP SDK via the shared probe set (REQ-080, REQ-081) |
| Examples per primary use case | Worked example programs under `cmd/examples/` for benchmark, seeder, MCP, federator |

## Out of scope (v1)

| Excluded | Where handled |
|---|---|
| FHIR resources, FHIR query, FHIR façade clients | A sibling Cadasto FHIR SDK proposal — not this module. |
| openEHR EHR Extract (RM `ehr_extract` package) | Deferred — no v1 consumer. BMM definitions remain in `resources/bmm/openehr_rm_1.2.0.bmm.json`; generator skips this package. Wire in when a consumer needs the extract exchange surface. |
| AOM 2 / ADL 2 / OPT-2 | Deferred — `openehr_am_2.4.0.bmm.json` kept in `resources/bmm/` but not generated. AOM 1.4 covers v1's archetype/template parsing needs. |
| openEHR LANG types as generated code | Deferred — `openehr_lang_1.1.0.bmm.json` kept as reference. The BMM meta-classes used by `openehr/bmm/` are hand-written against the P_BMM persistence shape for v1; auto-generating them is a future option. |
| openEHR TERM service interface | Deferred — `openehr_term_3.1.0.bmm.json` kept in `resources/bmm/`. No v1 consumer requires a typed terminology-service interface; consumers integrate with a deployment-specific terminology backend directly. |
| Full Master Patient Index design beyond a preview search surface | Cadasto MPI research track. The `cadasto/mpi` preview is a placeholder shape only. |
| OIDC provider and Authorization Server **design** | Cadasto authorization-server research track. The SDK *consumes* the AS via `auth/smart`; it does not implement or define one. |
| Production federator policy (authority, merge strategy, partial-failure semantics) | A separate proposal pending MPI research outcomes. The SDK provides per-node primitives; policy is bespoke per deployment. |
| MCP framework selection and MCP tool catalog | A separate proposal. This SDK only guarantees the surface is *consumable* from an MCP server. |
| Webhooks client | Deferred to a later phase. |
| Extraction of Cadasto-platform extras (`cadasto/…`) into a separate Go module | Conditional later step driven by STRAND-08. v1 keeps everything together; the cut line (REQ-010, REQ-011) ensures extraction would be mechanical. |
| Code generation from a shared contract source (PHP ↔ Go OpenAPI) | Not pursued (STRAND-02, cancelled). SDKs are hand-written and independent; cross-SDK conformance is the shared probe set. |
| Template-specific generated structs (e.g. a typed Go struct for a vital-signs OPT) | Belongs in the consuming project; the generic OPT-driven builder lives in `openehr/composition`. |
| Connection-pooling, transport-level TLS, proxy config | Owned by the consumer's injected `*http.Client` (REQ-021); SDK does not allocate transports. |
| Multi-tenant routing, per-tenant credential storage | A consumer concern. The SDK exposes per-call auth via `TokenSource`; storage and selection of credentials is application-side. |

## Adjacent components — clarifications

Items occasionally confused with SDK scope:

- The SDK is a **library**, not a service. No `main` package, no listening sockets, no database driver. The `cmd/examples/` programs are illustrative only.
- A CDR implements the *server* side of openEHR REST; this SDK implements the *client* side. They share no runtime code.
- The MCP server use case is a target, not a deliverable. The SDK ships method signatures that map 1:1 to MCP tool definitions; the MCP framework integration is the consuming MCP server's responsibility.
