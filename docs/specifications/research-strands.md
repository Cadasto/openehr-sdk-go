# Research strands

**Status:** Draft

Open architectural questions that are **not yet decided** but are scoped, named, and tracked. Each strand resolves into an ADR (under `../docs/adr/`) and amends one or more REQs in [REQ.md](REQ.md).

A strand is not a `Draft` REQ — it is an open question whose answer cannot be predicted by reading the spec. Resolving a strand happens by:

1. Producing evidence (a spike, a benchmark, a fit-gap analysis).
2. Writing an ADR proposing the resolution.
3. Amending the affected REQs in this tree.
4. Closing the strand here with a backlink to the ADR.

Strand IDs (`STRAND-NN`) are stable. Renumbering is prohibited.

---

## STRAND-01 — Extraction from reference CDR

**Status:** Open.

**Question:** Which parts of the existing reference CDR Go codebase (HTTP wrapper, RM mapping, auth scaffolding, load-test utilities) move to the SDK, and which remain bespoke in the consumer harness?

**Why it's open:** the cdr code is hand-grown for a server-side use case. Some primitives generalise to a client SDK; others (e.g. PostgreSQL connection pooling, archive triggers, tenant routing) are server-only and stay. The boundary is non-trivial.

**Evidence needed:**

- File-level inventory of the cdr's HTTP layer, RM types, JWT verifier, error envelope code.
- A decision per file: SDK / bespoke / shared via a third package.
- Post-migration benchmark run confirming no measurable percentile regression (target: same p50, p95, p99 within 5%).

**Resolution form:** ADR that lists the moved files, the shared types, and any helpers that needed re-shaping. Amends REQ-001..014.

**Implementation gate:** Phase 1 of the use-cases sequencing (POC milestones 1–3) cannot finish without a decision here.

---

## STRAND-02 — Shared contract source-of-truth (PHP ↔ Go)

**Status:** Open. **Significant decision — warrants a dedicated ADR.**

**Question:** Both SDKs target the same openEHR REST `1.1.0-development` surface. How is the contract maintained?

**Options:**

(a) **Independent hand-written clients with shared test cassettes.** Each SDK is hand-written; the conformance-probe set + shared cassettes catch wire-level drift. Lowest tooling investment; relies on probe coverage being complete.

(b) **Shared OpenAPI document; both SDKs generate their typed REST layer from it.** Highest up-front investment; strongest cross-language conformance guarantee. Risks: generated code complicates Go idiom (functional options, ctx-first) and PHP idiom (repositories, exceptions).

(c) **Hybrid.** Hand-written public surfaces (repositories / package functions, idiomatic options) on top of generated low-level wire DTOs from an OpenAPI document maintained alongside.

**Evidence needed:**

- Spike a generated low-level DTO layer for one resource (e.g. EHR + EHR_STATUS) in each language. Measure idiom impact.
- Estimate the maintenance cost of the OpenAPI document.
- Run the probe set against each option's prototype.

**Resolution form:** ADR-NNN that selects (a), (b), or (c) with reasoning. Amends REQ-050 (REST version pin — the contract source becomes named) and adds (if (b)/(c)) generation-pipeline REQs.

**Implementation gate:** Decision can wait until Phase 2; affects sequencing of cross-SDK probe ratification (Phase 5).

---

## STRAND-03 — Go-idiomatic surface validation

**Status:** Open.

**Question:** Are package-level functions the right primary surface, or should the SDK mirror the PHP SDK's repository-struct surface for cross-language familiarity?

**Why it's open:** REQ-023 currently says "package-level functions SHOULD be the primary surface". But "SHOULD" leaves room for evidence-driven adjustment.

**Evidence needed:**

- Implement the EHR + Composition surface in both shapes against the four named use cases.
- Measure: lines of caller code, IDE autocomplete clarity, mockability for tests.
- Survey the four use-case consumers (benchmark, seeder, MCP, federator) for preference.

**Resolution form:** ADR-NNN confirming or revising REQ-023, REQ-022 (functional options), REQ-021 (HTTP client injection).

**Implementation gate:** Affects Phase 1 (CDR extraction) — best resolved before significant surface lands.

---

## STRAND-04 — RM polymorphism and codec performance

**Status:** Partially resolved.

**Question:** The RM modeling rules in [rm-modeling.md](rm-modeling.md) (concrete structs + embedded base + interfaces + central type registry) need validation against the full RM 1.1.0-development surface. And: which JSON codec — `encoding/json`, `sonic`, `easyjson` — is the default?

### Resolved sub-questions

| Sub-question | Resolution | ADR |
|---|---|---|
| Abstract generic `EVENT` polymorphism (`History.events`) | Promote `EVENT` to a Go interface; `POINT_EVENT` / `INTERVAL_EVENT` concrete; whitelist in generator | [ADR 0003](../docs/adr/0003-rm-event-polymorphism.md) |
| `Real` / `Integer` wire tolerance (quoted vs numeric JSON) | Strict encode, permissive decode via `rm.Real` / `rm.Integer` defined types | [ADR 0004](../docs/adr/0004-numeric-wire-tolerance.md) |

### Still open

- **Full RM inventory:** decode every BMM type through the registry; identify sites that resist the pattern (e.g. further `VERSION[T]` whitelist decisions beyond `EVENT`).
- **Default codec benchmark:** `encoding/json` (current, via generator-emitted `MarshalJSON`) vs `sonic` vs `easyjson` under seeder/benchmark workloads.
- **Validation independence:** confirm `openehr/validation` can validate without taking on the codec's dependencies (REQ-013).

**Evidence needed (remaining):**

- Benchmark throughput, allocations, and memory residency for codec candidates.
- Document any remaining abstract-generic classes requiring ADR whitelist (generator policy today: `EVENT` only).

**Resolution form (remaining):** ADR choosing the default codec (with tuning-knob notes for swapping). Amends REQ-052, REQ-053, possibly REQ-040 if registry shape needs tweaking.

**Implementation gate:** Phase 1b — affects every read path in `openehr/client/*` and cross-SDK parity (REQ-080).

---

## STRAND-05 — SMART-on-openEHR auth library

**Status:** Open.

**Question:** There is no Go equivalent of Laravel Socialite (the PHP SDK's auth substrate). Implementing `auth/smart` is first-party work. What's the implementation plan and how is it validated?

**Why it's open:** REQ-061..064 describe the *contract*. The implementation is non-trivial: PKCE, JWKS rotation, refresh, launch context, error mapping. There is risk of subtle drift from the SMART App Launch specification.

**Evidence needed:**

- Implement against a reference SMART-on-openEHR deployment.
- Validate against PROBE-001..007 (the auth probes).
- Compare wire output to a PHP SDK trace against the same deployment.

**Resolution form:** ADR-NNN documenting the auth library scope, dependencies (e.g. `golang.org/x/oauth2` vs hand-rolled), and the conformance-probe pass evidence. Amends REQ-061..064.

**Implementation gate:** Phase 2 — affects every consumer that isn't already using `auth/clientcreds`.

---

## STRAND-06 — Concurrency and transport hygiene

**Status:** Open.

**Question:** The federator constructs multiple SDK clients. Do they share a single `*http.Transport` (for connection-pool efficiency across nodes) or own independent transports (for cleaner failure isolation per node)?

**Why it's open:** REQ-021 says "inject your `*http.Client`" — but it does not prescribe sharing. Both approaches are defensible; the trade-off is connection-pool reuse vs blast-radius isolation.

**Evidence needed:**

- Federator-style spike (Phase 3) running 4+ clients with both shared and independent transports.
- Measure: connection-establishment latency (cold), pool-exhaustion behaviour under load, failure isolation when one node is slow / unreachable.

**Resolution form:** ADR-NNN documenting the recommendation and the override path. Amends REQ-021 if the default guidance becomes opinionated.

**Implementation gate:** Phase 3 — federator implementation makes this decision real.

---

## STRAND-07 — Versioning and module path

**Status:** **Resolved.**

**Decision:** Module path is `github.com/cadasto/openehr-sdk-go`; semantic-import versioning for v2+; `internal/` boundary per Go convention.

**Rationale:** Locked early to avoid late-cycle import-path churn. Cross-language discoverability with the Cadasto PHP SDK informs the name.

**Codified in:** REQ-001, REQ-004, REQ-005, [module-layout.md § Versioning](module-layout.md#versioning).

---

## STRAND-08 — Cadasto extras: boundary, criteria, conditional extraction

**Status:** Open.

**Question:** Will `cadasto/…` ever be extracted into a sibling Go module? If so, when, and what triggers the decision?

**Why it's open:** the openEHR-core part of this SDK could in principle be vendor-neutral (target EHRbase, other openEHR backends). The Cadasto extras are platform-specific. Whether keeping them in one module or splitting them is right depends on adoption, governance, and cross-backend demand.

**Criteria** (none decisive on its own; jointly assessed):

- **Conceptual.** Is the surface an openEHR concept or a Cadasto-specific concept?
- **Technical.** Does it share types and lifecycle with openEHR core, or is it thinly layered on top?
- **Audience.** Is it needed by every consumer, or by a subset (integration developers using Admin / Datamap)?
- **Governance.** Same release cadence as openEHR core, or faster-churn Cadasto surfaces?
- **Cross-backend demand.** Concrete demand for the openEHR-core to work against EHRbase or another non-Cadasto backend.

**Evidence needed:**

- Adoption data after Phase 4 (Cadasto extras shipped).
- A concrete consumer asking for an EHRbase-compatible build.

**Resolution form:** ADR-NNN either confirming "keep together" or extracting. If extracting: package moves, module path for the sibling, semver implications for both modules.

**Open until:** v1 is in production for at least one minor release. Premature resolution is worse than no resolution.

**Boundary held in v1** (regardless of resolution): REQ-010, REQ-011, REQ-012 enforce the cut line so an extraction would be mechanical, not a rewrite.

---

## Index

| Strand | Title | Status | Affects |
|---|---|---|---|
| [STRAND-01](#strand-01--extraction-from-reference-cdr) | Extraction from reference CDR | Open | REQ-001..014; Phase 1 |
| [STRAND-02](#strand-02--shared-contract-source-of-truth-php--go) | Shared contract source-of-truth | Open | REQ-050; cross-SDK |
| [STRAND-03](#strand-03--go-idiomatic-surface-validation) | Go-idiomatic surface | Open | REQ-021..023 |
| [STRAND-04](#strand-04--rm-polymorphism-and-codec-performance) | RM polymorphism + codec perf | **Partially resolved** | REQ-024, REQ-040, REQ-052..053 |
| [STRAND-05](#strand-05--smart-on-openehr-auth-library) | SMART-on-openEHR auth library | Open | REQ-061..064 |
| [STRAND-06](#strand-06--concurrency-and-transport-hygiene) | Concurrency / transport hygiene | Open | REQ-021, REQ-026 |
| [STRAND-07](#strand-07--versioning-and-module-path) | Versioning + module path | **Resolved** | REQ-001, REQ-004, REQ-005 |
| [STRAND-08](#strand-08--cadasto-extras-boundary-criteria-conditional-extraction) | Cadasto-extras extraction | Open (long-term) | REQ-010, REQ-011 |
