---
kind: specification
---

# Scope

**Status:** Draft

What the `openehr-sdk-go` v1 surface includes and excludes, and the consumers that justify it. Out-of-scope items are not gaps — they are deliberate boundaries with named handling elsewhere.

## In scope

| Area | Coverage |
|---|---|
| openEHR REST `1.1.0` — primary surface | EHR, Composition, Contribution, Directory, EHR_STATUS, ItemTags, Template/Definition, Demographic, AQL, System endpoints, and the versioned-object reads of COMPOSITION and EHR_STATUS ([wire.md § REQ-174](wire.md#req-174--versioned-object-reads)) and of PARTY |
| openEHR Reference Model | Concrete types for clinical and demographic RM (excluding the `ehr_extract` package — deferred), type registry, embedded base structs, interfaces for abstract categories ([reference-model.md](reference-model.md)) — **generated** from the pinned BMM schema in [`../resources/bmm/`](../../resources/bmm) per [reference-model.md](reference-model.md) |
| Archetype Object Model 1.4 | `openehr/aom/aom14/` — the AOM 1.4 classes and the ADL 1.4 operational template model (REQ-176), generated from the SDK-authored `openehr_operational_template_1.4.0` root, which includes `openehr_am_1.4.0.bmm.json` and the RM root (REQ-041); sibling of `openehr/rm/`; `openehr/templatecompile` compiles the model it reads (REQ-111). |
| BMM loader | Public `openehr/bmm/` (hand-written for v1) — P_BMM JSON parser, includes resolution, queryable in-memory model. Building block (REQ-045). |
| Code generator | In-tree `internal/bmmgen` + `cmd/bmmgen` emitting RM + AOM 1.4 + type registry — drift-checked in CI |
| openEHR Archetype Query Language (AQL) | Struct-builder and verb-function builders, request/result models, executor in `openehr/client/query` ([wire.md § AQL](wire.md#aql)) |
| Operational template (OPT) handling | ADL 1.4 `.opt` reading into the AOM 1.4 model (`openehr/aom/aom14/`), its compiled form and paths (`openehr/templatecompile/`, `openehr/template/`); OPT-driven generic Composition builder. OET (`.oet`) parsing is out of scope for v1. |
| Canonical JSON / FLAT / STRUCTURED codecs | All three openEHR serialization shapes, independently usable ([wire.md § Canonical JSON](wire.md#canonical-json), [§ Simplified formats](wire.md#simplified-formats)) |
| SMART-on-openEHR authentication | PKCE, JWKS rotation, token refresh, launch context ([auth.md](auth.md)) |
| Auth providers — alternative grants | Client Credentials, JWT Bearer; abstracted under `auth.TokenSource` |
| Service discovery | First-class `ServiceCatalog`, cached, refresh-able, with hand-built catalogs for non-discovering backends ([auth.md](auth.md)) |
| Cadasto-platform extras | Admin endpoints today; Cadasto Extra API, **Datamap V2** (REQ-058), minimal MPI search (preview) and Care aggregates reserved — in the same module under `cadasto/` ([module-layout.md § Cadasto extras](module-layout.md#cadasto-extras)) |
| Sandbox + recordings | In-memory `sandbox.Backend` (`http.RoundTripper`) for hermetic SDK-consumer tests ([conformance.md § REQ-082](conformance.md#req-082--runnability)); Cassette HAR 1.2 recordings under `testkit/recordings/` ([ADR 0020](../adr/0020-cassette-recording-har.md)) |
| Testkit + conformance probes | Conformance probes, the shared result type and catalog runner (`testkit/probe/`), vendored fixture documents, and fixture-path resolution under `testkit/` ([conformance.md](conformance.md)) |
| openEHR wire conformance | The probe suite verifies wire-level correctness against the openEHR spec ([conformance.md](conformance.md)) |
| Examples per primary use case | Worked example programs under `cmd/examples/`, catalogued in [`../examples.md`](../examples.md); the five primary consumers of [§ Primary use cases](#primary-use-cases) are downstream products that follow those shapes, not programs in this tree |

## Out of scope (v1)

| Excluded | Where handled |
|---|---|
| FHIR resources, FHIR query, FHIR façade clients | A sibling Cadasto FHIR SDK proposal — not this module. |
| openEHR EHR Extract (RM `ehr_extract` package) | Deferred — no v1 consumer. BMM definitions remain in `resources/bmm/openehr_rm_1.2.0.bmm.json`; generator skips this package. Wire in when a consumer needs the extract exchange surface. |
| ITS-REST `IMPORTED_VERSION` bodies on the composition and EHR_STATUS version reads | Deferred with the EHR Extract, the exchange that creates imported versions: the version reads refuse them as a typed decode failure ([wire.md § REQ-174](wire.md#req-174--versioned-object-reads), [ADR 0031](../adr/0031-version-reads-refuse-imported-version.md)). |
| AOM 2 / ADL 2 / OPT-2 | Deferred — `openehr_am_2.4.0.bmm.json` kept in `resources/bmm/` but not generated. AOM 1.4 covers v1's archetype/template parsing needs. |
| openEHR LANG types as generated code | Deferred — `openehr_lang_1.1.0.bmm.json` kept as reference. The BMM meta-classes used by `openehr/bmm/` are hand-written against the P_BMM persistence shape for v1; auto-generating them is a future option. |
| openEHR TERM service interface | Deferred — `openehr_term_3.1.0.bmm.json` kept in `resources/bmm/`. No v1 consumer requires a typed terminology-service interface; consumers integrate with a deployment-specific terminology backend directly. |
| Full Master Patient Index design beyond a preview search surface | Cadasto MPI research track. The `cadasto/mpi` preview is a placeholder shape only. |
| OIDC provider and Authorization Server **design** | Cadasto authorization-server research track. The SDK *consumes* the AS via `auth/smart`; it does not implement or define one. |
| Production federator policy (authority, merge strategy, partial-failure semantics) | A separate proposal pending MPI research outcomes. The SDK provides per-node primitives; policy is bespoke per deployment. |
| MCP framework selection and MCP tool catalog | A separate proposal. This SDK only guarantees the surface is *consumable* from an MCP server. |
| Webhooks client | Not provided. |
| Extraction of Cadasto-platform extras (`cadasto/…`) into a separate Go module | Conditional later step driven by STRAND-08. v1 keeps everything together; the cut line ([module-layout.md § Boundary rules](module-layout.md#boundary-rules)) ensures extraction would be mechanical. |
| Code generation from a shared contract source (OpenAPI-generated wire DTOs) | Not pursued (STRAND-02, cancelled). The SDK is hand-written; openEHR wire conformance is the probe suite. |
| Template-specific generated structs (e.g. a typed Go struct for a vital-signs OPT) | Belongs in the consuming project; the generic OPT-driven builder lives in `openehr/composition`. |
| Connection-pooling, transport-level TLS, proxy config | Owned by the consumer's injected `*http.Client` (REQ-021); SDK does not allocate transports. |
| Multi-tenant routing, per-tenant credential storage | A consumer concern. The SDK exposes per-call auth via `TokenSource`; storage and selection of credentials is application-side. |

## Adjacent components — clarifications

Items occasionally confused with SDK scope:

- The SDK is a **library**, not a service: no listening sockets, no database driver. The `cmd/` programs are developer tools and illustrative examples.
- A CDR implements the *server* side of openEHR REST; this SDK implements the *client* side. They share no runtime code.
- The MCP server use case is a target, not a deliverable. The SDK ships method signatures that map 1:1 to MCP tool definitions; the MCP framework integration is the consuming MCP server's responsibility.

## Use cases

The SDK is not built in a vacuum: every public-surface decision is justified by at least one named consumer. If a feature has no consumer, it does not ship in v1.

### Primary use cases

Five named consumers drive the SDK's primary surface. For each: what the SDK provides, what stays bespoke in the consumer.

#### Benchmark

A high-concurrency CRUD-against-the-openEHR-API tool for capacity planning and CDR scalability research.

| SDK provides | Stays bespoke |
|---|---|
| Typed REST methods (`composition.Save`, `query.Execute`, …) | Workload shaping (Poisson arrivals, mixed read/write ratios, locality patterns) |
| Injected `*http.Client` for connection-pool tuning | Percentile and tail-latency collection (the SDK exposes the raw timings via OTel; the benchmark aggregates) |
| Retry-off mode (REQ-091 — retries off by default) | Backend storage metrics; not the SDK's job |
| OTel hooks for tracing inside the SDK | Report renderer (HTML/CSV/JSON output) |
| `context.Context` for cancellation / deadline plumbing | Run orchestration (worker pool, ramp-up, ramp-down) |

A high-concurrency benchmark harness is a natural early consumer of the typed REST surface.

#### Synthetic data seeder

An OPT-guided faker that produces bulk Compositions and demographic records for staging environments and CDR-load testing.

| SDK provides | Stays bespoke |
|---|---|
| RM types (`openehr/rm`) — concrete structs to fill in | Generation rules per clinical domain (vital-signs value distributions, demographic plausibility) |
| OPT-driven generic Composition builder (`openehr/composition`) | OPT inventory — which templates to seed; the seeder picks |
| `contribution.Builder` for batched atomic writes ([REQ-130](wire.md#req-130--contribution-builder)) | Checkpointing and resume — the seeder's responsibility, not the SDK's |
| The demographic client (`openehr/client/demographic`) | Identity strategy (deterministic seeds for reproducibility) |
| The template-driven instance generator (`openehr/instance`) | Faker library bindings (e.g. `gofakeit`) |

#### MCP server

A Model Context Protocol server that exposes openEHR operations as MCP tools to agentic clients (Claude, other LLM clients).

| SDK provides | Stays bespoke |
|---|---|
| Typed method signatures that map ~1:1 to MCP tool definitions | MCP framework integration (e.g. `mark3labs/mcp-go`); tool registration; transport bindings |
| Per-request `auth.TokenSource` via context (REQ-060 + ctx) | Mapping incoming MCP auth to a per-request `TokenSource` |
| Idempotent, ctx-cancellable methods | Tool-result serialization for MCP transport |
| Sandbox / recorded transports for tool testing | LLM-side prompt engineering |

The SDK's `context.Context`-first design (REQ-020) makes per-request auth forwarding natural — the incoming token from the MCP transport becomes a `WithTokenSource(ctx, ts)` and propagates through every SDK call without rewiring.

#### Federative API client

Fan-out over multiple openEHR backends with per-node spec pinning, partial-failure handling, and a merge / authority policy.

| SDK provides | Stays bespoke |
|---|---|
| Per-node SDK client with independent base URL, issuer, spec version | Federation policy (which node is authoritative; merge vs first-wins) |
| Conformance probes per node (each node verified against the same probe set) | Partial-failure aggregation (degraded vs failed; SLO accounting) |
| Independent `*http.Client` per node (REQ-021) | Identity reconciliation across nodes — see MPI research strand |
| `context.Context` propagation across goroutines | Routing decisions (which subset of nodes to call) |

Federation policy is the subject of a separate research track once MPI lands. The SDK provides the *primitives*; the policy is bespoke.

#### openEHR SMART app (Go backend)

A server-side web or API service that performs a SMART-on-openEHR launch, handles tokens, and makes CDR calls on the user's behalf — the Go backend behind a clinical app.

| SDK provides | Stays bespoke |
|---|---|
| SMART-on-openEHR launch + PKCE flow (`auth/smart`, REQ-060–069) | Web framework, routing, and the redirect/callback wiring for the deployment |
| Discovery of service base URLs and auth endpoints (`smart/discovery`, REQ-070–073) | Session store and how tokens are persisted across a user session |
| ID-token validation / JWKS handling and coalesced token refresh (`auth/`, `smart/`) | Scope-to-feature authorization policy inside the app |
| Typed REST methods for CDR calls behind the launch (`openehr/client/ehr/…`) | Front-end / UI and product-specific workflow |

### Building-block use cases

The building blocks [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence) lists are importable and useful without an authenticated client. Consumers such as these motivate the rule:

| Building block | Consumer pattern |
|---|---|
| `openehr/rm/` alone | Authoring tools, RM-aware data transforms, FHIR↔openEHR mapping prototypes — model openEHR data in memory without touching a CDR. |
| `openehr/serialize/` alone | Canonical-JSON pre-processors for archival (hashing, deduplication); JSON-to-JSON diff utilities for migration scripts. |
| `openehr/validation/` alone | CI validators that check Composition-vs-OPT conformance on pull-request; webhook handlers that gate uploads; pre-commit hooks in a clinical-modeling repo. |
| `openehr/aql/` alone | AQL builders, linters, formatters, and static analysers that don't execute the query — they parse, normalise, judge RM containment, and report. |
| `openehr/aom/aom14/` and `openehr/templatecompile/` alone | Reading an ADL 1.4 OPT (`.opt`), compiling it and resolving its paths, for IDE plugins and CI; OET out of scope for v1. |
| `openehr/terminology/` alone | Code ↔ rubric and group / code-set membership lookups over the pinned openEHR Terminology, for validators, form renderers and mapping tools that never touch a CDR (REQ-034). |

### Out-of-scope use cases

Documented to prevent scope-creep PRs:

- **Browser-side openEHR client.** The SDK is server-side / CLI; no browser-side bindings, no WebAssembly export, no fetch-API polyfill. A consumer needing this can build it on top of `openehr/rm/` and `openehr/serialize/`, but the SDK doesn't ship a browser shape.
- **Mobile SDKs.** Not a Go consumer pattern. iOS / Android need different SDKs (Swift / Kotlin) which are out of scope.
- **Realtime / streaming.** No openEHR REST 1.1.0 streaming endpoints in v1. Server-Sent Events for outbox publication is an openEHR-side concern, not an SDK concern in v1.
- **CLI for end-users.** The SDK ships `cmd/examples/` for illustration only. A "Cadasto CLI" is a separate downstream product that *consumes* this SDK.
