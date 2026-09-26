# SDK roadmap — what is not finished

What you can build on today, and what is still open. Per-requirement status is in the [requirements registry](specifications/REQ.md), generated from [`traceability.yaml`](specifications/traceability.yaml); this page adds only the delivery stages, the work that is not finished, and the deployment targets. When this page and the specs disagree, **the specs win** and this page is the one to fix.

## Legend

| Symbol | Meaning |
|--------|---------|
| **Landed** | Code + tests in tree; usable (may still be v1-preview quality) |
| **Partial** | Subset implemented, or spec-only with traceability incomplete |
| **Planned** | Normative in `docs/specifications/`; directory may exist with `doc.go` only |
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
| **2 — Clinical building blocks** | ADL 1.4 OPT parser + compiled-template foundation | **Landed** |
| | Composition builder | **Landed** |
| | Validation: template-driven, non-COMPOSITION roots, RM floor | **Landed** (RM floor archetype-root and ARCHETYPED rows: partial) |
| | OPT → RM instance synthesis | **Landed** (`medium` detail level open) |
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

Everything below is `Partial`, `Planned` or `Deferred`. Gaps that belong to a deployment rather than a feature (EHRbase, Better Platform) are under [Deployment targets](#deployment-targets). Anything in neither table has landed; see the [registry](specifications/REQ.md).

| Area | Feature | Status | Package / REQ | Notes |
|---|---|---|---|---|
| Core | Benchmark harness | **Deferred** | — | A few packages carry `go test -bench` benchmarks; there is no shared harness (stage 1) |
| Core | AOM 2.4 | **Deferred** | — | BMM pinned in `resources/bmm/`; no codegen and no package yet (stage 4) |
| Core | Template-less RM validation floor | **Partial** | `openehr/validation/`, `openehr/validation/rmread/` REQ-112 | `ValidateRM` and typed sugars walk any RM root with `rminfo` as sole driver (no template required), checking RM-mandatory absences and a per-type invariant catalogue, including `TERM_MAPPING.match`'s value set and `DV_TEXT.mappings`'s `Mappings_valid` ([plan](plans/archive/2026-09-01-rm-canonical-json-fidelity.md)). The archetype-root and ARCHETYPED catalogue rows are spec-first and await code ([plan](plans/2026-09-24-rm-floor-archetype-roots.md)). PROBE-077 deferred ([plan](plans/archive/2026-06-29-rm-floor-validation.md)) |
| Core | Synthesis `medium`/`detail_level` level | **Planned** | `openehr/instance/` REQ-107 | Representative optional-subset fill between `Minimal` and full population |
| Core | LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| Core | EHR Extract RM | **Deferred** | — | Out of v1 scope |
| REST clients | ItemTags | **Partial** | `openehr/client/ehr/itemtags/` | REQ-059 is **partial** overall (PROBE-062 implemented (Sandbox) under `testkit/probes/rest/`; dedicated ITEM_TAG endpoints still deferred); header codec + composition/ehrstatus/directory GET and composition PUT are landed |
| REST clients | Definition — ADL 2 | **Deferred** | — | `FormatADL14` is the only registered format (stage 4) |
| SMART / Cadasto | AppContext / launch helpers | **Partial** | `smart/` | LaunchContext + ID-token validation (REQ-064/067) landed; App Registration open ([STRAND-05](specifications/research-strands.md)) |
| SMART / Cadasto | Cadasto Extra API | **Planned** | `cadasto/extra/` |  |
| SMART / Cadasto | Datamap V2 | **Planned** | `cadasto/datamap/` REQ-058 |  |
| SMART / Cadasto | MPI preview | **Planned** | `cadasto/mpi/` |  |
| SMART / Cadasto | Cadasto admin | **Partial** | `cadasto/admin/` | Health probes (`Live`, `Ready`) landed per REQ-083; tenant/env/system-info planned. Distinct from the ITS Admin client |
| SMART / Cadasto | Care aggregates | **Planned** | `cadasto/care/` |  |
| Conformance | Auth / REST probes | **Partial** | `testkit/probes/auth/`, `testkit/probes/rest/` | PROBE-001…009 all implemented (Sandbox) plus launch-mode coverage; of the REST-binding probes, PROBE-060 / 061 / 062 / 065 / 067 and PROBE-102…104 are implemented (Sandbox), PROBE-066 / 079 are witnessed Live, and PROBE-063 / 064 / 068 remain Draft |
| Conformance | Sandbox transport | **Partial** | `sandbox/` | In-memory backend (`backend.go`, `script.go`) landed, covering EHR create/get/head plus scripted routes. Versioned, definition, demographic, and transport probes run on it instead of hand-written `httptest` servers. Auth/discovery probes still stand up `httptest` identity servers (OIDC/JWKS, not CDR). Phase 2 of the [runnability plan](plans/2026-08-18-probe-runnability.md) |
| Conformance | Testkit helpers + probe runner | **Partial** | `testkit/` | The runner (`testkit/probe/run.go`) executes the catalog, a subset, or one probe; the per-package `Result` types under `testkit/probes/*` are `type Result = probe.Result` aliases, not duplicates. REQ-082 specifies the modes and result contract. The recording format is settled ([ADR 0020](adr/0020-cassette-recording-har.md); [STRAND-11](specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) resolved). The Cassette recorder, replayer, the `cmd/probe-record` capture harness and two corpus recordings (`ehr-create`, `ehr-lifecycle`) have landed. What remains of the [runnability plan](plans/2026-08-18-probe-runnability.md) is Cassette coverage beyond those two, the full REQ-082 replay key, and Live runs against a reachable CDR |
| Conformance | openEHR conformance ratification | **Partial** | `testkit/conformance/webtemplate/` | REQ-080/082. PROBE-086 round-trips the pinned upstream EHRbase FLAT corpus (34 bodies this SDK did not write) exact on the modelled subset: **1466 of 1824 keys (80.4%)**; remaining refusals are censused in [SKIPPED.md](../testkit/conformance/webtemplate/SKIPPED.md). Live-CDR ratification and the Cassette/Live modes remain open |
| Conformance | Cadasto API conformance | **Planned** | `testkit/cassettes/cadasto/` | REQ-083, anchored to the Cadasto platform API contract (stage 4) |
| Conformance | OpenAPI cassettes | **Partial** | `testkit/cassettes/` REQ-095 | Coverage table in [`testkit/cassettes/its_rest/README.md`](../testkit/cassettes/its_rest/README.md). The named gaps are stored-query metadata bodies, a persisted CONTRIBUTION response, ITEM_TAG bodies, and the `Identifier` write-response body |

---

## Deployment targets

| Target | Status | Notes |
|---|---|---|
| EHRbase CDR | **Partial** | WebTemplate export matches `openEHR_SDK` v2.3 (PROBE-075) and the FLAT codec round-trips EHRbase's own corpus at 80.4% (PROBE-086). EHRbase-only admin helpers (`PurgeTemplates`) are documented as deployment extensions (REQ-099), not as a second OpenAPI pin. Ratification against a **running** deployment is open (see stage 5) |
| FerroEHR | **Partial** | Live probe target: `OPENEHR_LIVE_FERROEHR` names the deployment and `OPENEHR_LIVE_FERROEHR_BASIC` carries its `user:pass` credential ([`testkit/probe/live_test.go`](../testkit/probe/live_test.go)); the Cassette replay key strips its `/ferroehr/rest/openehr/v1` base ([`testkit/probe/replay.go`](../testkit/probe/replay.go)). Opt-in, not run in CI; no Live snapshot recorded yet |
| Better Platform | **Deferred** | No Live probe target. Web Template export emits EHRbase ids (`blood_pressure`); the Better camelCase variant (`bloodPressure`) is not produced ([ADR 0014](adr/0014-webtemplate-reference-implementation-lock.md)) |
| Any ITS-REST 1.1.0 CDR | **Partial** | The clients target the vendored [`resources/its-rest/`](../resources/its-rest/README.md) pin; conformance is asserted in Sandbox, not yet against a live third-party CDR |
| Static / non-discovering backend | **Landed** | Build a `discovery.ServiceCatalog` by hand, since there is no base-URL parameter (REQ-070); see [quick-start.md](quick-start.md) |

---

## Updating this page

Update a row here in the PR that changes it: remove it when the feature lands, add it when new open work is accepted. Keep notes to a sentence or two and link to the plan or spec for detail. Per-requirement status belongs in `traceability.yaml`, not here.
