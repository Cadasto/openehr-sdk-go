---
kind: guide
---

# SDK roadmap — what is not finished

What you can build on today, and what is still open. Per-requirement status is in the [requirements registry](specifications/REQ.md), generated from [`traceability.yaml`](specifications/traceability.yaml); this page adds only the delivery stages, the work that is not finished, and the deployment targets.

## Legend

| Symbol | Meaning |
|--------|---------|
| **Landed** | Code + tests in tree; usable (may still be v1-preview quality) |
| **Partial** | Subset implemented, or spec-only with traceability incomplete |
| **Planned** | Normative in `docs/specifications/`; directory may exist with `doc.go` only |
| **In delivery** | Specified, and being built on a branch whose pull request lands it |
| **Deferred** | Explicitly out of v1 scope in specs or ADRs |

---

## Delivery stages

Each stage groups several deliverables, and a stage is only as done as its weakest row. The Open work table below names what is left in each.

| Stage | Deliverable | Status |
|---|---|---|
| **1 — Foundations** | Module scaffolding, specs, Makefile, CI | **Landed** |
| | BMM loader, codegen (RM + AOM 1.4), type registry | **Landed** |
| | Canonical JSON + XML serialization | **Landed** |
| | Transport (HTTP, retry, OTel, errors) and auth providers | **Landed** |
| | Service discovery + SMART PKCE and ID-token validation | **Landed** |
| | openEHR REST clients: System, EHR, Query, Definition, Admin, Demographic | **Landed** |
| | Benchmark harness | **Deferred** |
| **2 — Clinical building blocks** | ADL 1.4 operational template reader + compiled-template foundation | **Landed** |
| | Composition builder | **Landed** |
| | Validation: template-driven, non-COMPOSITION roots, RM floor | **Landed** (RM floor: partial, see open work) |
| | OPT → RM instance synthesis | **Landed** (`complete` detail level open) |
| | AQL: builders, parsed AST, static lint | **Landed** |
| | Simplified formats (FLAT / STRUCTURED) + WebTemplate export | **Landed** |
| **3 — Deployment & adoption** | Application SMART (`smart/` AppContext) on discovery | **Partial** |
| | EHRbase CDR support | **Partial** |
| | Worked examples (`cmd/examples/`) | **Landed** |
| | Documentation website | **Landed** |
| **4 — Platform extras & ADL 2** | Cadasto extras (`cadasto/*`) | **Planned** |
| | ADL 2 / AOM 2.4: codegen and the Definition ADL 2 format | **Deferred** |
| **5 — Conformance ratification** | Sandbox transport + probe runner | **Partial** |
| | Cassette / Live probe ratification against a real CDR | **Partial** |

---

## Open work

Everything below is `Partial`, `Planned`, `In delivery` or `Deferred`. Gaps that belong to a deployment rather than a feature (EHRbase, Better Platform) are under [Deployment targets](#deployment-targets). Anything in neither table has landed; see the [registry](specifications/REQ.md).

| Area | Feature | Status | Package / REQ | Notes |
|---|---|---|---|---|
| Core | Benchmark harness | **Deferred** | — | A few packages carry `go test -bench` benchmarks, but there is no shared harness (stage 1) |
| Core | AOM 2.4 | **Deferred** | — | The BMM is pinned in `resources/bmm/`, with no codegen and no package yet (stage 4) |
| Core | Template-less RM validation floor | **Partial** | `openehr/validation/` REQ-112 | `ValidateRM` validates any RM root without a template, but does not report a bound beside its own open flag ([known gap](specifications/clinical-modeling.md#known-gap--a-bound-beside-its-own-open-flag)), nor tell an omitted by-value mandatory member of a value built in Go from a present one ([Value-typed mandatory presence](specifications/clinical-modeling.md#value-typed-mandatory-presence)) |
| Core | Validation during decode; validator policy options | **Deferred** | `openehr/validation/`, `openehr/serialize/` REQ-171 | Out of [REQ-171](specifications/clinical-modeling.md#req-171--validation-engine)'s scope: unmatched members captured on the value ([STRAND-15](specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types)), and descent under refused content, a severity per rule or a partial-instance mode ([STRAND-16](specifications/research-strands.md#strand-16--validator-policy-beyond-the-limits)) |
| Core | Synthesis `complete`/`detail_level` level | **Planned** | `openehr/instance/` REQ-107 | The ITS-REST `detail_level=complete`, every data point the template allows; until it lands, `instance.Complete` is an alias of `Large` |
| Core | LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| Core | EHR Extract RM | **Deferred** | — | Out of v1 scope |
| Core | `bmm.Load` context | **Deferred** | `openehr/bmm/` | `Load` and `LoadAll` take no `context.Context`, and adding one changes the API, so it waits for the pre-1.0 API review |
| Core | Test fixtures through `go:embed` | **Deferred** | `testkit/fixtures/` | `CorpusRoot` finds the fixture tree through `runtime.Caller`, and moving to `go:embed` is test-only work |
| Clinical modeling | Malformed `C_STRING` pattern at compile time | **Deferred** | `openehr/template/constraints/` REQ-103 | `NewCString` keeps a pattern that does not compile and `Validate` reports it as `CodeInvalidValue`, where the compile step could refuse it without a signature change ([§ REQ-103](specifications/clinical-modeling.md#req-103--primitive-constraint-introspection)) |
| Clinical modeling | RM floor change-control and resource-description coded invariants | **Deferred** | `openehr/validation/` REQ-112 | The floor checks the coded invariants its table lists, not those of VERSION, AUDIT_DETAILS, ATTESTATION, TRANSLATION_DETAILS or RESOURCE_DESCRIPTION_ITEM ([Coded invariants](specifications/clinical-modeling.md#coded-invariants)) |
| Simplified formats | Multi-composition FLAT batches | **Deferred** | `openehr/serialize/simplified/` REQ-053 | `MarshalFlat` and `UnmarshalFlat` handle one composition per call, with no batch form |
| REST clients | ItemTag routes | **Partial** | `openehr/client/ehr/itemtags/` REQ-059 | The ItemTag headers have landed, but the pinned OpenAPI's `/tags` routes, under `/ehr/{ehr_id}` and `/demographic`, have no client ([wire.md § REQ-059](specifications/wire.md#req-059)) |
| REST clients | Definition — ADL 2 | **Deferred** | — | `FormatADL14` is the only registered format (stage 4) |
| REST clients | Demographic contribution | **Deferred** | `openehr/client/demographic/` | The pinned OpenAPI carries `contribution_create` and `contribution_get` on `/demographic/contribution`, which would mirror `openehr/client/ehr/contribution/` |
| AQL | Typed result columns | **Deferred** | `openehr/aql/`, `openehr/client/query/` | Resolved SELECT paths know their leaf data-value type and could type result cells, which waits for a consumer that asks ([ADR 0017](adr/0017-aql-semantic-layer.md) § Consequences) |
| AQL | Template-derived FROM builder | **Deferred** | `openehr/aql/` | Exploratory only: AQL scopes by archetype, so a prototype would emit an archetype-scoped chain and add a `…/archetype_details/template_id/value` condition only on request |
| AQL | Shared semantic resolver | **Deferred** | `openehr/aql/` | One resolution pass giving a typed query model to the linter, the builder, an executor and a CDR's planner, which needs its own requirement ([ADR 0017](adr/0017-aql-semantic-layer.md) § Consequences) |
| SMART / Cadasto | Discovery cache shared across resolver options | **Deferred** | `smart/discovery/` REQ-071 | A cache entry is keyed by the Platform base URL alone and a hit skips the OpenID configuration cross-check, so it can reach a resolver configured otherwise ([known gap](specifications/auth.md#req-071)) |
| SMART / Cadasto | AppContext / launch helpers | **Partial** | `smart/` | LaunchContext and ID-token validation have landed, but not App Registration helpers or dynamic client registration ([auth.md § What is NOT in scope here](specifications/auth.md#what-is-not-in-scope-here)) |
| SMART / Cadasto | Cadasto Extra API | **Planned** | `cadasto/extra/` |  |
| SMART / Cadasto | Datamap V2 | **Planned** | `cadasto/datamap/` REQ-058 |  |
| SMART / Cadasto | MPI preview | **Planned** | `cadasto/mpi/` |  |
| SMART / Cadasto | Cadasto admin | **Partial** | `cadasto/admin/` REQ-083 | The health checks (`Live`, `Ready`, `ReadyBody`) have landed, but not the tenant, environment and system-info endpoints; distinct from the ITS Admin client |
| SMART / Cadasto | Care aggregates | **Planned** | `cadasto/care/` |  |
| Conformance | Probe modes | **Partial** | `testkit/probes/` REQ-082 | See [Open probe modes](#open-probe-modes-req-082) below |
| Conformance | Sandbox transport | **Partial** | `sandbox/` | The in-memory backend models the EHR resource and scripts the other routes, while the auth and discovery probes still stand up their own `httptest` identity servers ([Known gaps](probe-harness.md#cross-mode-agreement)) |
| Conformance | openEHR conformance ratification | **Partial** | `testkit/conformance/webtemplate/` REQ-053/082 | PROBE-086 round-trips the pinned EHRbase FLAT corpus on the modelled subset, with the rest censused in [SKIPPED.md](../testkit/conformance/webtemplate/SKIPPED.md), and Live-CDR ratification is open |
| Conformance | Cadasto API conformance | **Planned** | `testkit/corpus/cadasto/` REQ-083 | Anchored to the Cadasto platform API contract, for the extras beyond the health checks (stage 4) |
| Conformance | OpenAPI fixtures | **Partial** | `testkit/corpus/` REQ-050 | The gaps are stored-query metadata bodies, a persisted CONTRIBUTION response, ITEM_TAG bodies and the `Identifier` write-response body ([coverage](../testkit/corpus/its_rest/README.md)) |

### Open probe modes (REQ-082)

REQ-082 aims for every backend-facing probe to run in Sandbox, Cassette and Live mode, with the runner serving Cassette recordings the probes capture themselves ([conformance.md § REQ-082](specifications/conformance.md#req-082--runnability)).
Today the runner, the recorder and replayer, `cmd/probe-record` and five recordings have landed, but the recordings are keyed by scenario and replay in their own tests.
A mode missing from a backend-facing entry is open work, not a defect in the probe.
Tracked by name:

- Cassette for PROBE-060, PROBE-065, PROBE-066, PROBE-079 and PROBE-104.
- Cassette and Live for PROBE-062, PROBE-078, PROBE-102 and PROBE-103.
- Sandbox for PROBE-066 and PROBE-079, and a GET arm for PROBE-066.
- Cassette and Live for the auth and discovery probes, which first need to take a configured client ([Known gaps](probe-harness.md#cross-mode-agreement)).
- An **Effect** field on every backend-facing entry that lacks one ([probe-harness.md § Effect](probe-harness.md#effect)).

---

## Deployment targets

| Target | Status | Notes |
|---|---|---|
| EHRbase CDR | **Partial** | The WebTemplate export and the FLAT codec are checked against EHRbase's own references (PROBE-075, PROBE-086), and its admin helpers (`PurgeTemplates`) are deployment extensions (REQ-099), but ratification against a running deployment is open (stage 5) |
| FerroEHR | **Partial** | A Live probe target named by `OPENEHR_LIVE_FERROEHR`, with its credential in `OPENEHR_LIVE_FERROEHR_BASIC`, opt-in and not run in CI; no Live snapshot is recorded yet |
| Better Platform | **Deferred** | No Live probe target, and the Web Template export emits EHRbase ids (`blood_pressure`), not the Better camelCase variant ([ADR 0014](adr/0014-webtemplate-reference-implementation-lock.md)) |
| Any ITS-REST 1.1.0 CDR | **Partial** | The clients target the vendored [`resources/its-rest/`](../resources/its-rest/README.md) pin, with conformance asserted in Sandbox, not yet against a live third-party CDR |
| Static / non-discovering backend | **Landed** | Build a `discovery.ServiceCatalog` by hand, since there is no base-URL parameter (REQ-070); see [quick-start.md](quick-start.md) |

---

## Updating this page

Update a row here in the PR that changes it: remove it when the feature lands, add it when new open work is accepted. Keep notes to a sentence or two and link to the spec or ADR for detail, never to a plan: a plan is a working note, deleted once its work lands. Per-requirement status belongs in `traceability.yaml`, not here.
