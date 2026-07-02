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

**Status:** Resolved.

**Decision:** The SDK is specified and built as an independent module — its surface is defined by the specs, not derived from or gated on any existing codebase. Where a reference CDR already implements a primitive (an HTTP wrapper, RM mapping, auth scaffolding), reuse is opportunistic and case-by-case: generalisable primitives belong in the SDK; consumer-specific concerns stay in the consumer.

**Codified in:** the building-block and boundary rules (REQ-010..014).

---

## STRAND-02 — Shared contract source-of-truth (PHP ↔ Go)

**Status:** Cancelled.

**Rationale:** Not pursued. The SDK is hand-written and independent; wire-level conformance is guaranteed by the openEHR conformance probe suite (REQ-080), so no shared contract source or code-generation pipeline is needed.

---

## STRAND-03 — Go-idiomatic surface validation

**Status:** Resolved.

**Decision:** Package-level functions are the primary surface; repository structs are offered as a convenience for injection seams. Settled by the landed client surface and codified in REQ-021..024 ([idiom.md](idiom.md)) — the SDK follows idiomatic Go rather than mirroring another SDK's source shape.

---

## STRAND-04 — RM polymorphism and codec performance

**Status:** Partially resolved.

**Question:** The RM modeling rules in [rm-modeling.md](rm-modeling.md) (concrete structs + embedded base + interfaces + central type registry) need validation against the full RM 1.1.0-development surface. And: which JSON codec — `encoding/json`, `sonic`, `easyjson` — is the default?

### Resolved sub-questions

| Sub-question | Resolution | ADR |
|---|---|---|
| Abstract generic `EVENT` polymorphism (`History.events`) | Promote `EVENT` to a Go interface; `POINT_EVENT` / `INTERVAL_EVENT` concrete; whitelist in generator | [ADR 0003](../adr/0003-rm-event-polymorphism.md) |
| `Real` / `Integer` wire tolerance (quoted vs numeric JSON) | Strict encode, permissive decode via `rm.Real` / `rm.Integer` defined types | [ADR 0004](../adr/0004-numeric-wire-tolerance.md) |
| Polymorphic round-trip fidelity (SDK-GAP-13) | Value-in-interface `_type` on encode via `openehr/internal/jsonpoly`; round-tripped `DV_INTERVAL<T>` validated from its bounds' runtime types; corpus round-trips byte-stable | [archived plan](../plans/archive/2026-06-23-polymorphic-encode-decode.md) |

### Still open

- **Full RM inventory:** decode every BMM type through the registry; identify sites that resist the pattern (e.g. further `VERSION[T]` whitelist decisions beyond `EVENT`).
- **Default codec benchmark:** `encoding/json` (current, via generator-emitted `MarshalJSON`) vs `sonic` vs `easyjson` under seeder/benchmark workloads — a *performance* axis (throughput/allocations), codec swapped behind the same generated methods.
- **`encoding/json/v2` as a *simplification* axis (Go 1.25):** distinct from the performance candidates above. The generator emits bespoke `MarshalJSON`/`UnmarshalJSON` (`internal/bmmgen/render_json{mar,unmar}.go`) and `openehr/internal/jsonpoly` largely to obtain what `encoding/json` v1 lacks: deterministic field order, correct zero/omit semantics (`omitzero`), and value-in-interface `_type` handling. `encoding/json/v2` (`GOEXPERIMENT=jsonv2`) provides these natively (`jsontext`, `MarshalerTo`/`UnmarshalerFrom`, marshal options), so it could *retire* a large share of that generated + hand-written marshaling surface rather than merely swap the codec behind it. Blocked on experimental status.
- **Validation independence:** confirm `openehr/validation` can validate without taking on the codec's dependencies (REQ-013).

**Evidence needed (remaining):**

- Benchmark throughput, allocations, and memory residency for codec candidates.
- Document any remaining abstract-generic classes requiring ADR whitelist (generator policy today: `EVENT` only).
- `encoding/json/v2` fit-gap: whether `jsontext` + marshal options reproduce byte-stable canonical JSON (PROBE-030/031/038) and the polymorphic `_type` round-trip (SDK-GAP-13) without the generator's marshaler emit — and quantify the generated + `jsonpoly` LOC it would remove.
- `encoding/json/v2` stability/timeline: experimental behind `GOEXPERIMENT=jsonv2` in Go 1.25; gate any adoption on a stable, un-gated API so the SDK's public-API and wire-stability promises hold.

**Resolution form (remaining):** ADR choosing the default codec (with tuning-knob notes for swapping). Amends REQ-052, REQ-053, possibly REQ-040 if registry shape needs tweaking. A `encoding/json/v2` resolution would additionally touch the codegen policy in [ADR 0002](../adr/0002-bmm-codegen-decisions.md), since it changes what the generator emits.

**Implementation gate:** Phase 1b — affects every read path in `openehr/client/*` and openEHR wire conformance (REQ-080).

---

## STRAND-05 — SMART-on-openEHR auth library

**Status:** Resolved. See [ADR 0009](../adr/0009-smart-auth-library-scope.md).

**Question (resolved):** There is no batteries-included Go substrate for OAuth2 / SMART-on-openEHR; implementing `auth/smart` is first-party work. What's the implementation plan and how is it validated?

**Decision summary:**

- Built the full SMART-on-openEHR auth library across `auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic`, `auth/introspect`, and `smart/discovery`. Four flows (PKCE public, confidential symmetric, confidential asymmetric `private_key_jwt`, Backend Services) and three launch modes (standalone, embedded, backend) are covered and exercised by PROBE-001..009.
- Relaxed the OTel-only dependency rule: adopted `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3`, and `github.com/go-jose/go-jose/v4` (directly imported for JWS signing; also required by `go-oidc/v3`) for security-sensitive JOSE/OIDC crypto, scoped to `auth/` and `smart/`. Hand-rolling JWS signing and ID-token verification at the RS384/ES384/RS256/ES256 multi-alg level was rejected as a correctness and maintenance risk.
- `auth.FromOAuth2TokenSource` adapter and an issuer-matching multi-EHR helper are recorded as available follow-ups (not built — no current consumer need). See ADR 0009 § (c).

**Codified in:** [ADR 0009](../adr/0009-smart-auth-library-scope.md). Amends REQ-061..064.

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

**Rationale:** Locked early to avoid late-cycle import-path churn.

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

## STRAND-09 — ITS-REST conformance follow-ups

**Status:** Open (deferred) — opened by the [ITS-REST conformance remediation](../plans/archive/2026-06-19-its-rest-conformance-remediation.md).

**Two follow-ups carried out of that plan:**

1. **Dedicated REST conformance probes (deferred).** The plan called for four `testkit/probes/rest/*` probes (audit-details header grammar, System `OPTIONS`, Admin bulk-delete, Definition `/example`). The corrected wire shapes are currently asserted by package `httptest` tests (request-capture), which provide equivalent coverage for CI but are not part of the ratifiable probe suite (REQ-080/082). **Open until:** the sandbox/cassette probe runner (REQ-082) is the gate for these surfaces; then promote the unit assertions to `rest/*` probes. Affects REQ-059, REQ-095, REQ-099.

2. **Stored-query `fetch` omission vs OpenAPI `required` (decision recorded).** The Definition/Query `Query` schema marks `offset`, `fetch`, and `query_parameters` `required`, but `fetch` has no spec default ("depends on the implementation") and `fetch: 0` means *zero rows*. **Decision:** the SDK always emits `offset` (documented default 0) and `query_parameters`, but omits `fetch` unless the caller sets it (`fetchSet`), so the server applies its own default. This is a deliberate, documented deviation from the literal `required` list (`openehr/client/query/execute.go` `storedBody`). **Revisit if:** a conformant backend rejects a body without `fetch`, in which case escalate to an ADR (emit a sentinel/`-1`, or send the server's advertised default).

---

## Index

| Strand | Title | Status | Affects |
|---|---|---|---|
| [STRAND-01](#strand-01--extraction-from-reference-cdr) | Extraction from reference CDR | **Resolved** | REQ-010..014 |
| [STRAND-02](#strand-02--shared-contract-source-of-truth-php--go) | Shared contract source-of-truth | **Cancelled** | — |
| [STRAND-03](#strand-03--go-idiomatic-surface-validation) | Go-idiomatic surface | **Resolved** | REQ-021..024 |
| [STRAND-04](#strand-04--rm-polymorphism-and-codec-performance) | RM polymorphism + codec perf | **Partially resolved** | REQ-024, REQ-040, REQ-052..053 |
| [STRAND-05](#strand-05--smart-on-openehr-auth-library) | SMART-on-openEHR auth library | **Resolved** ([ADR 0009](../adr/0009-smart-auth-library-scope.md)) | REQ-061..064 |
| [STRAND-06](#strand-06--concurrency-and-transport-hygiene) | Concurrency / transport hygiene | Open | REQ-021, REQ-026 |
| [STRAND-07](#strand-07--versioning-and-module-path) | Versioning + module path | **Resolved** | REQ-001, REQ-004, REQ-005 |
| [STRAND-08](#strand-08--cadasto-extras-boundary-criteria-conditional-extraction) | Cadasto-extras extraction | Open (long-term) | REQ-010, REQ-011 |
| [STRAND-09](#strand-09--its-rest-conformance-follow-ups) | ITS-REST conformance follow-ups (REST probes; stored-query `fetch`) | Open (deferred) | REQ-059, REQ-095, REQ-099 |
