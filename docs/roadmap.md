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
| Core | Template-less RM validation floor | **Partial** | `openehr/validation/`, `openehr/validation/rmread/` REQ-112 | `ValidateRM` and typed sugars walk any RM root with `rminfo` as sole driver (no template required), checking RM-mandatory absences and a per-type invariant catalogue, including `TERM_MAPPING.match`'s value set and `DV_TEXT.mappings`'s `Mappings_valid` ([PR 145](https://github.com/Cadasto/openehr-sdk-go/pull/145)). The archetype-root and ARCHETYPED catalogue rows are spec-first and await code ([plan](plans/2026-09-24-rm-floor-archetype-roots.md)). PROBE-077 deferred ([conformance.md § PROBE-077](specifications/conformance.md#probe-077--rm-floor-invariant-matrix)) |
| Core | Synthesis `medium`/`detail_level` level | **Planned** | `openehr/instance/` REQ-107 | Representative optional-subset fill between `Minimal` and full population |
| Core | LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| Core | EHR Extract RM | **Deferred** | — | Out of v1 scope |
| Core | Canonical XML `DV_MULTIMEDIA` byte fields | **Partial** | `openehr/serialize/canxml/` REQ-056 | The generated XML codec writes `data` and `integrity_check` as one decimal element per byte and reads them back the same way, so a document carrying the bytes as base64 text fails to decode (`strconv.ParseUint`). Encode at `openehr/rm/data_types_encapsulated_xmlmar_gen.go:54, :67`, decode at `openehr/rm/data_types_encapsulated_xmlunmar_gen.go:56, :72`; no canxml test covers DV_MULTIMEDIA |
| Core | Decoding an inline-version CONTRIBUTION | **Deferred** | `openehr/serialize/canjson/`, `openehr/client/ehr/contribution/` | A CONTRIBUTION body that carries its versions inline, as the 47 vendored `testkit/corpus/submissions/` bodies do, is refused by `canjson.Unmarshal` into `rm.Contribution` at `/versions/0`, because RM `CONTRIBUTION.versions` holds references. The SDK writes that shape through `contribution.Submission` but has no type that reads it; `TestCorpusParityV1V2` counts these bodies as refused by both codecs |
| Core | `bmm.Load` context | **Deferred** | `openehr/bmm/` | `Load` and `LoadAll` take no `context.Context` (`openehr/bmm/load.go:23`, `openehr/bmm/loadall.go:46`); adding one changes the API, so it waits for the pre-1.0 API review |
| Core | Test fixtures through `go:embed` | **Deferred** | `testkit/fixtures/` | `CassettesRoot` finds the cassette tree through `runtime.Caller` (`testkit/fixtures/paths.go:32`); moving to `go:embed` is test-only work |
| Clinical modeling | `<OPERATIONAL_TEMPLATE>` OPT root | **Deferred** | `openehr/template/` REQ-100 | `ParseOPT` refuses an `<OPERATIONAL_TEMPLATE>` root with `ErrInvalidOPT` (`openehr/template/parse.go:113`); the test corpus renames the root before parsing (`testkit/fixtures/parse_opt.go:13`) |
| Clinical modeling | Malformed `C_STRING` pattern at parse time | **Deferred** | `openehr/template/constraints/` REQ-103 | `NewCString` keeps a pattern that does not compile, and `Validate` reports it as `CodeInvalidValue` (`openehr/template/constraints/string.go:30-37`). Reporting it from `ParseOPT` needs a signature change through the primitive builders (`openehr/template/parse_primitives.go:205`) |
| Clinical modeling | RM floor coded invariants | **Deferred** | `openehr/validation/` REQ-112 | `Setting_valid`, `Category_validity`, `Change_type_valid`, `Mode_valid` and `Normal_status_validity` are not evaluated. `openehr/terminology` (REQ-034) ships the groups they check, but `openehr/validation` does not import it ([known gap](specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)) |
| Clinical modeling | Interval templates in the constraint-cassette axis | **Deferred** | `testkit/fixtures/`, `openehr/validation/` | `isConstraintTemplateID` holds the four `Test_dv_interval_*` templates out (`testkit/fixtures/constraint_templates.go:46`, pinned at `constraint_templates_test.go:20`). Enrolling them flips that pin and adds two `primitive_out_of_range` assertions to `openehr/validation/constraint_cassettes_test.go` for the `lower_upper` instances, whose bounds `-10` and `200` fall outside the template's `0..100`; PROBE-076 already passes on all four |
| Simplified formats | Multi-composition FLAT batches | **Deferred** | `openehr/serialize/simplified/` REQ-053 | `MarshalFlat` and `UnmarshalFlat` handle one composition per call (`flat_encode.go:24`, `flat_decode.go:58`); there is no batch form |
| REST clients | ItemTags | **Partial** | `openehr/client/ehr/itemtags/` | REQ-059 is **partial** overall (PROBE-062 implemented (Sandbox) under `testkit/probes/rest/`; dedicated ITEM_TAG endpoints still deferred); header codec + composition/ehrstatus/directory GET and composition PUT are landed |
| REST clients | Definition — ADL 2 | **Deferred** | — | `FormatADL14` is the only registered format (stage 4) |
| REST clients | Demographic contribution | **Deferred** | `openehr/client/demographic/` | The pinned OpenAPI carries `contribution_create` and `contribution_get` on `/demographic/contribution` (`resources/its-rest/demographic-validation.openapi.yaml:806, :842`); the demographic client has no contribution leaf. It would mirror `openehr/client/ehr/contribution/` |
| REST clients | FLAT / STRUCTURED over REST | **Deferred** | `openehr/client/ehr/composition/` REQ-053 | The composition leaf reads and writes canonical JSON only: `Get` returns and `Save` takes an `*rm.Composition` (`openehr/client/ehr/composition/composition.go:36, :180`). Sending or receiving `application/openehr.wt.flat+json` or `application/openehr.wt.structured+json` is left to the caller, who runs `openehr/serialize/simplified` one call before |
| AQL | Typed result columns | **Deferred** | `openehr/aql/`, `openehr/client/query/` | Resolved SELECT paths know their leaf data-value type, which could type result cells. The value is unproven, so it waits for a consumer that asks ([ADR 0017](adr/0017-aql-semantic-layer.md) § Consequences) |
| AQL | Template-derived FROM builder | **Deferred** | `openehr/aql/` | Exploratory only. AQL scopes by archetype, and one archetype's data is stored under several templates, so a chain derived from one template over-fits the query; template identity belongs in a `WHERE …/archetype_details/template_id/value` condition. A prototype would emit an archetype-scoped chain and add the template condition only on request |
| AQL | Shared semantic resolver | **Deferred** | `openehr/aql/` | One resolution pass producing a typed query model for the linter, the builder, an executor and a CDR's plan-and-lower stage; it needs its own requirement ([ADR 0017](adr/0017-aql-semantic-layer.md) § Consequences) |
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
| Conformance | Cadasto API conformance | **Planned** | `testkit/corpus/cadasto/` | REQ-083, anchored to the Cadasto platform API contract (stage 4) |
| Conformance | OpenAPI cassettes | **Partial** | `testkit/corpus/` REQ-095 | Coverage table in [`testkit/corpus/its_rest/README.md`](../testkit/corpus/its_rest/README.md). The named gaps are stored-query metadata bodies, a persisted CONTRIBUTION response, ITEM_TAG bodies, and the `Identifier` write-response body |

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
