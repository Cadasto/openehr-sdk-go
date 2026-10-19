---
kind: guide
---

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
| | Validation: template-driven, non-COMPOSITION roots, RM floor | **Landed** (RM floor: partial, see open work) |
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
| Core | Template-less RM validation floor | **Partial** | `openehr/validation/`, `internal/rmaccess/` REQ-112 | `ValidateRM` and its typed wrappers validate any RM root without a template: the walk of [§ REQ-171](specifications/clinical-modeling.md#req-171--validation-engine) with every attribute unconstrained, reading every registered class through generated access ([ADR 0028](adr/0028-generated-rm-attribute-access.md)), checking RM-mandatory absences and each invariant the inventory classifies as evaluated ([conformance.md § PROBE-077](specifications/conformance.md#probe-077--rm-floor-invariant-matrix)), and naming the rest in `Coverage.Unevaluated`. Template-driven validation applies the same rules in the same walk, after the template's admission ([ADR 0025](adr/0025-template-validation-includes-the-rm-floor.md), [ADR 0027](adr/0027-template-admission-before-rm-rules.md); REQ-171 landed in [PR 271](https://github.com/Cadasto/openehr-sdk-go/pull/271)). Known gaps ([§ REQ-112](specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)): a bound beside its own open flag, or beside a bound it cannot be compared with, is not reported; and on a decoded value a by-value mandatory member whose zero value is itself valid cannot be told absent from present, except a root EHR_STATUS's `subject`, which `ValidateRMEHRStatusBytes` reads from the source keys ([Value-typed mandatory presence](specifications/clinical-modeling.md#value-typed-mandatory-presence)). Presence recorded at decode is [STRAND-15](specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types) |
| Core | Validation during decode; validator policy options | **Deferred** | `openehr/validation/`, `openehr/serialize/` REQ-171 | Out of [REQ-171](specifications/clinical-modeling.md#req-171--validation-engine)'s scope: presence recorded by a decoder and unmatched members captured on the value ([STRAND-15](specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types), which gets its own requirement once resolved), and options for descent under refused content, a severity policy per rule or a partial-instance mode ([STRAND-16](specifications/research-strands.md#strand-16--validator-policy-beyond-the-limits)) |
| Core | Synthesis `complete`/`detail_level` level | **Planned** | `openehr/instance/` REQ-107 | The ITS-REST `detail_level=complete`: every data point the template allows, as reference guidance and not expected to be committable, beyond `Large`. `Required` (`detail_level=required`) and `Medium` (`detail_level=medium`) have landed |
| Core | LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| Core | EHR Extract RM | **Deferred** | — | Out of v1 scope |
| Core | Decoding an inline-version CONTRIBUTION | **Deferred** | `openehr/serialize/canjson/`, `openehr/client/ehr/contribution/` | A CONTRIBUTION body that carries its versions inline, as the 47 vendored `testkit/corpus/submissions/` bodies do, is refused by `canjson.Unmarshal` into `rm.Contribution` at `/versions/0`, because RM `CONTRIBUTION.versions` holds references. The SDK writes that shape through `contribution.Submission` but has no type that reads it; `TestCorpusParityV1V2` counts these bodies as refused by both codecs |
| Core | `bmm.Load` context | **Deferred** | `openehr/bmm/` | `Load` and `LoadAll` take no `context.Context` (`openehr/bmm/load.go:23`, `openehr/bmm/loadall.go:46`); adding one changes the API, so it waits for the pre-1.0 API review |
| Core | Test fixtures through `go:embed` | **Deferred** | `testkit/fixtures/` | `CorpusRoot` finds the fixture tree through `runtime.Caller` (`testkit/fixtures/paths.go:34`); moving to `go:embed` is test-only work |
| Clinical modeling | `<OPERATIONAL_TEMPLATE>` OPT root | **Deferred** | `openehr/template/` REQ-100 | `ParseOPT` refuses an `<OPERATIONAL_TEMPLATE>` root with `ErrInvalidOPT` (`openehr/template/parse.go:113`); the test corpus renames the root before parsing (`testkit/fixtures/parse_opt.go:13`) |
| Clinical modeling | Malformed `C_STRING` pattern at parse time | **Deferred** | `openehr/template/constraints/` REQ-103 | `NewCString` keeps a pattern that does not compile, and `Validate` reports it as `CodeInvalidValue` (`NewCString` in `openehr/template/constraints/string.go`). Reporting it from `ParseOPT` needs a signature change through the primitive builders (`openehr/template/parse_primitives.go:205`) |
| Clinical modeling | RM floor change-control and resource-description coded invariants | **Deferred** | `openehr/validation/` REQ-112 | The floor checks the coded invariants its *Coded invariants* table lists, but not VERSION `Lifecycle_state_valid`, AUDIT_DETAILS `Change_type_valid`, ATTESTATION `Reason_valid`, or the `Language_valid` of TRANSLATION_DETAILS and RESOURCE_DESCRIPTION_ITEM, even on an ORIGINAL_VERSION handed to `ValidateRM` as the root ([Coded invariants](specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)) |
| Clinical modeling | Interval templates in the constraint-fixture axis | **Deferred** | `testkit/fixtures/`, `openehr/validation/` | `isConstraintTemplateID` holds the four `Test_dv_interval_*` templates out (`testkit/fixtures/constraint_templates.go:46`, pinned at `constraint_templates_test.go:20`). Enrolling them flips that pin and adds two `primitive_out_of_range` assertions to `openehr/validation/constraint_fixtures_test.go` for the `lower_upper` instances, whose bounds `-10` and `200` fall outside the template's `0..100`; PROBE-076 already passes on all four |
| Simplified formats | Multi-composition FLAT batches | **Deferred** | `openehr/serialize/simplified/` REQ-053 | `MarshalFlat` and `UnmarshalFlat` handle one composition per call (`flat_encode.go:24`, `flat_decode.go:58`); there is no batch form |
| REST clients | ItemTags | **Partial** | `openehr/client/ehr/itemtags/` | REQ-059 is **partial** overall (PROBE-062 implemented (Sandbox) under `testkit/probes/rest/`; dedicated ITEM_TAG endpoints still deferred); header codec + composition/ehrstatus/directory GET and composition PUT are landed |
| REST clients | Definition — ADL 2 | **Deferred** | — | `FormatADL14` is the only registered format (stage 4) |
| REST clients | Demographic contribution | **Deferred** | `openehr/client/demographic/` | The pinned OpenAPI carries `contribution_create` and `contribution_get` on `/demographic/contribution` (`resources/its-rest/demographic-validation.openapi.yaml:806, :842`); the demographic client has no contribution leaf. It would mirror `openehr/client/ehr/contribution/` |
| REST clients | FLAT / STRUCTURED over REST | **Deferred** | `openehr/client/ehr/composition/` REQ-053 | The composition leaf reads and writes canonical JSON only: `Get` returns and `Save` takes an `*rm.Composition` (`openehr/client/ehr/composition/composition.go:36, :180`). Sending or receiving `application/openehr.wt.flat+json` or `application/openehr.wt.structured+json` is left to the caller, who runs `openehr/serialize/simplified` one call before |
| AQL | Typed result columns | **Deferred** | `openehr/aql/`, `openehr/client/query/` | Resolved SELECT paths know their leaf data-value type, which could type result cells. The value is unproven, so it waits for a consumer that asks ([ADR 0017](adr/0017-aql-semantic-layer.md) § Consequences) |
| AQL | Template-derived FROM builder | **Deferred** | `openehr/aql/` | Exploratory only. AQL scopes by archetype, and one archetype's data is stored under several templates, so a chain derived from one template over-fits the query; template identity belongs in a `WHERE …/archetype_details/template_id/value` condition. A prototype would emit an archetype-scoped chain and add the template condition only on request |
| AQL | Shared semantic resolver | **Deferred** | `openehr/aql/` | One resolution pass producing a typed query model for the linter, the builder, an executor and a CDR's plan-and-lower stage; it needs its own requirement ([ADR 0017](adr/0017-aql-semantic-layer.md) § Consequences) |
| SMART / Cadasto | Discovery cache shared across resolver options | **Deferred** | `smart/discovery/` REQ-071 | A fresh cache hit passes the calling resolver's own checks ([PR 270](https://github.com/Cadasto/openehr-sdk-go/pull/270)), but not the OpenID configuration cross-check, which needs a request; and entries are keyed by the Platform base URL alone, so a catalog fetched from one resolver's `WithWellKnownPath` document reaches a resolver configured with another ([known gap](specifications/service-discovery.md#req-071)) |
| SMART / Cadasto | AppContext / launch helpers | **Partial** | `smart/` | LaunchContext + ID-token validation (REQ-064/067) landed; App Registration helpers are not built, and dynamic client registration is left to a later helper ([auth.md § What is NOT in scope here](specifications/auth.md#what-is-not-in-scope-here)) |
| SMART / Cadasto | Cadasto Extra API | **Planned** | `cadasto/extra/` |  |
| SMART / Cadasto | Datamap V2 | **Planned** | `cadasto/datamap/` REQ-058 |  |
| SMART / Cadasto | MPI preview | **Planned** | `cadasto/mpi/` |  |
| SMART / Cadasto | Cadasto admin | **Partial** | `cadasto/admin/` | Health probes (`Live`, `Ready`) landed per REQ-083; tenant/env/system-info planned. Distinct from the ITS Admin client |
| SMART / Cadasto | Care aggregates | **Planned** | `cadasto/care/` |  |
| Conformance | Auth / REST probes | **Partial** | `testkit/probes/auth/`, `testkit/probes/rest/` | PROBE-001…009 all implemented (Sandbox) plus launch-mode coverage; of the REST-binding probes, PROBE-060 / 061 / 062 / 065 / 067 and PROBE-102…104 are implemented (Sandbox), PROBE-060, PROBE-065, and PROBE-104 also have a Live snapshot, PROBE-066 / 079 are witnessed Live with Sandbox still planned, three scenario recordings replay the PROBE-060, 065, 066 and 079 exchanges outside the runner (none is yet a probe's Cassette mode), and PROBE-063 / 064 / 068 remain Draft |
| Conformance | Sandbox transport | **Partial** | `sandbox/` | In-memory backend (`backend.go`, `script.go`) landed, covering EHR create/get/head plus scripted routes. Versioned, definition, demographic, and transport probes run on it instead of hand-written `httptest` servers. Auth/discovery probes still stand up `httptest` identity servers (OIDC/JWKS, not CDR), and only the EHR resource is modelled in memory; other routes are scripted. Open REQ-082 work: [Known gaps](specifications/conformance.md#cross-mode-agreement) |
| Conformance | Testkit helpers + probe runner | **Partial** | `testkit/` | The runner (`testkit/probe/run.go`) executes the catalog, a subset, or one probe; the per-package `Result` types under `testkit/probes/*` are `type Result = probe.Result` aliases, not duplicates. REQ-082 specifies the modes and result contract. The recording format is settled ([ADR 0020](adr/0020-cassette-recording-har.md); [STRAND-11](specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) resolved). The Cassette recorder, replayer, the `cmd/probe-record` capture harness and five checked-in recordings (`ehr-create`, `ehr-lifecycle`, `ehr-status`, `composition-minimal`, `stored-query`) have landed. PROBE-060 and PROBE-065 now have a Live snapshot (`TestLiveEHRStatusAndMinimalSnapshot`), and PROBE-104 has one (`TestLiveDefinitionExample`). The five recordings are keyed by scenario, not by probe id, and replay in their own tests, so the runner serves no catalog probe in Cassette mode from them yet. What remains of [REQ-082](specifications/conformance.md#req-082--runnability) is runner-served Cassette recordings captured by the probes themselves, the full REQ-082 replay key, and further Live runs against a reachable CDR |
| Conformance | openEHR conformance ratification | **Partial** | `testkit/conformance/webtemplate/` | REQ-080/082. PROBE-086 round-trips the pinned upstream EHRbase FLAT corpus (34 bodies this SDK did not write) exact on the modelled subset: **1467 of 1824 keys (80.4%)**; remaining refusals are censused in [SKIPPED.md](../testkit/conformance/webtemplate/SKIPPED.md). Live-CDR ratification and the Cassette/Live modes remain open |
| Conformance | Cadasto API conformance | **Planned** | `testkit/corpus/cadasto/` | REQ-083, anchored to the Cadasto platform API contract (stage 4) |
| Conformance | OpenAPI fixtures | **Partial** | `testkit/corpus/` REQ-095 | Coverage table in [`testkit/corpus/its_rest/README.md`](../testkit/corpus/its_rest/README.md). The named gaps are stored-query metadata bodies, a persisted CONTRIBUTION response, ITEM_TAG bodies, and the `Identifier` write-response body |

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

Update a row here in the PR that changes it: remove it when the feature lands, add it when new open work is accepted. Keep notes to a sentence or two and link to the spec or ADR for detail, never to a plan: a plan is a working note, deleted once its work lands. Per-requirement status belongs in `traceability.yaml`, not here.
