# Use cases

**Status:** Draft

The SDK's primary consumers, building-block consumers, and the delivery sequence that informs early phasing.

The SDK is not built in a vacuum: every public-surface decision is justified by at least one named consumer. If a feature has no consumer, it does not ship in v1.

## Primary use cases

Four named consumers drive the SDK's primary surface. For each: what the SDK provides, what stays bespoke in the consumer.

### Benchmark

A high-concurrency CRUD-against-the-openEHR-API tool for capacity planning and CDR scalability research.

| SDK provides | Stays bespoke |
|---|---|
| Typed REST methods (`composition.Save`, `query.Execute`, …) | Workload shaping (Poisson arrivals, mixed read/write ratios, locality patterns) |
| Injected `*http.Client` for connection-pool tuning | Percentile and tail-latency collection (the SDK exposes the raw timings via OTel; the benchmark aggregates) |
| Retry-off mode (REQ-091 — retries off by default) | Backend storage metrics; not the SDK's job |
| OTel hooks for tracing inside the SDK | Report renderer (HTML/CSV/JSON output) |
| `context.Context` for cancellation / deadline plumbing | Run orchestration (worker pool, ramp-up, ramp-down) |

A high-concurrency benchmark harness is a natural early consumer of the typed REST surface.

### Synthetic data seeder

An OPT-guided faker that produces bulk Compositions and demographic records for staging environments and CDR-load testing.

| SDK provides | Stays bespoke |
|---|---|
| RM types (`openehr/rm`) — concrete structs to fill in | Generation rules per clinical domain (vital-signs value distributions, demographic plausibility) |
| OPT-driven generic Composition builder (`openehr/composition`) | OPT inventory — which templates to seed; the seeder picks |
| `ContributionBuilder` for batched atomic writes | Checkpointing and resume — the seeder's responsibility, not the SDK's |
| Demographic helpers in `cadasto/care` | Identity strategy (deterministic seeds for reproducibility) |
| `testkit/` fluent builders for trivial cases | Faker library bindings (e.g. `gofakeit`) |

### MCP server

A Model Context Protocol server that exposes openEHR operations as MCP tools to agentic clients (Claude, other LLM clients).

| SDK provides | Stays bespoke |
|---|---|
| Typed method signatures that map ~1:1 to MCP tool definitions | MCP framework integration (e.g. `mark3labs/mcp-go`); tool registration; transport bindings |
| Per-request `auth.TokenSource` via context (REQ-060 + ctx) | Mapping incoming MCP auth to a per-request `TokenSource` |
| Idempotent, ctx-cancellable methods | Tool-result serialization for MCP transport |
| Sandbox / recorded transports for tool testing | LLM-side prompt engineering |

The SDK's `context.Context`-first design (REQ-020) makes per-request auth forwarding natural — the incoming token from the MCP transport becomes a `WithTokenSource(ctx, ts)` and propagates through every SDK call without rewiring.

### Federative API client

Fan-out over multiple openEHR backends with per-node spec pinning, partial-failure handling, and a merge / authority policy.

| SDK provides | Stays bespoke |
|---|---|
| Per-node SDK client with independent base URL, issuer, spec version | Federation policy (which node is authoritative; merge vs first-wins) |
| Conformance probes per node (each node verified against the same probe set) | Partial-failure aggregation (degraded vs failed; SLO accounting) |
| Independent `*http.Client` per node (REQ-021) | Identity reconciliation across nodes — see MPI research strand |
| `context.Context` propagation across goroutines | Routing decisions (which subset of nodes to call) |

Federation policy is the subject of a separate research track once MPI lands. The SDK provides the *primitives*; the policy is bespoke.

## Building-block use cases

REQ-013 mandates that each core package be importable and useful without constructing an authenticated client. These five building-block consumers exist today and motivate the rule:

| Building block | Consumer pattern |
|---|---|
| `openehr/rm/` alone | Authoring tools, RM-aware data transforms, FHIR↔openEHR mapping prototypes — model openEHR data in memory without touching a CDR. |
| `openehr/serialize/` alone | Canonical-JSON pre-processors for archival (hashing, deduplication); JSON-to-JSON diff utilities for migration scripts. |
| `openehr/validation/` alone | CI validators that check Composition-vs-OPT conformance on pull-request; webhook handlers that gate uploads; pre-commit hooks in a clinical-modeling repo. |
| `openehr/aql/` (models only) | AQL linters, formatters, static analysers that don't execute the query — they parse, normalise, and report. |
| `openehr/template/` alone | ADL 1.4 OPT (`.opt`) parsing and path utilities for IDE plugins and CI; OET out of scope for v1. |

These consumers **MUST NOT** be forced to import `transport/`, `auth/`, or `smart/`. Their dependency graph stops at the leaf package they use.

## Sequencing

The SDK does not pursue feature parity with the PHP SDK in lockstep — sequencing follows **consumer demand**, one capability slice at a time:

- **Phase 1** — core REST + auth slice: `auth/clientcreds`, `transport/`, `openehr/rm/`, `openehr/client/ehr/` (unblocks the benchmark and any CRUD consumer).
- **Phase 2** — MCP and SMART consumers: `auth/smart`, `smart/`, `smart/discovery`.
- **Phase 3** — federator: multi-issuer, multi-catalog, partial-failure.
- **Phase 4** — Cadasto extras (Extra, Datamap, MPI preview, Admin, Care aggregates).
- **Phase 5** — cross-SDK probe ratification, `v1.0.0`.

Each phase ends with the corresponding probes transitioning `Draft` → `Implemented` → `Ratified` in [conformance.md](conformance.md); each milestone produces a plan in [`../docs/plans/`](../docs/plans/) citing the REQ-IDs and PROBE-IDs it addresses.

## Out-of-scope use cases

Documented to prevent scope-creep PRs:

- **Browser-side openEHR client.** The SDK is server-side / CLI; no browser-side bindings, no WebAssembly export, no fetch-API polyfill. A consumer needing this can build it on top of `openehr/rm/` and `openehr/serialize/`, but the SDK doesn't ship a browser shape.
- **Mobile SDKs.** Not a Go consumer pattern. iOS / Android need different SDKs (Swift / Kotlin) which are out of scope.
- **Realtime / streaming.** No openEHR REST 1.1.0-development streaming endpoints in v1. Server-Sent Events for outbox publication is an openEHR-side concern, not an SDK concern in v1.
- **CLI for end-users.** The SDK ships `cmd/examples/` for illustration only. A "Cadasto CLI" is a separate downstream product that *consumes* this SDK.
