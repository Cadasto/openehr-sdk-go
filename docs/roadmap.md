# SDK roadmap — landed vs planned

**What this is:** a living checklist of **implementation reality**, so you can tell at a glance what you can build on today. The normative contract lives in [`docs/specifications/`](specifications/) — when this file and the specs disagree, **the specs win** and this file is the one to fix.

**Looking for something else?** Requirement index → [`REQ.md`](specifications/REQ.md) · machine-readable traceability → [`traceability.yaml`](specifications/traceability.yaml) · sequenced delivery → [`plans/`](plans/) · getting started → [`quick-start.md`](quick-start.md).

## Legend

| Symbol | Meaning |
|--------|---------|
| **Landed** | Code + tests in tree; usable (may still be v1-preview quality) |
| **Partial** | Subset implemented, or spec-only with traceability incomplete |
| **Planned** | Normative in `docs/specifications/`; directory may exist with `doc.go` only |
| **Deferred** | Explicitly out of v1 scope in specs or ADRs |

---

## Delivery stages

Each stage groups several deliverables; a stage is only as done as its weakest row. Per-area detail is in the tables that follow.

| Stage | Deliverable | Status |
|---|---|---|
| **1 — Foundations** | Module scaffolding, specs, Makefile, CI | **Landed** |
| | BMM loader, codegen (RM + AOM 1.4), type registry | **Landed** |
| | Canonical JSON + XML serialization | **Landed** |
| | Transport (HTTP, retry, OTel, errors) and auth providers | **Landed** |
| | Service discovery + SMART PKCE and ID-token validation | **Landed** |
| | openEHR REST clients — System, EHR, Query, Definition, Admin, Demographic | **Landed** |
| | Benchmark harness | **Deferred** |
| **2 — Clinical building blocks** | ADL 1.4 OPT parser + compiled-template foundation | **Landed** |
| | Composition builder | **Landed** |
| | Validation — template-driven, non-COMPOSITION roots, RM floor | **Landed** |
| | OPT → RM instance synthesis | **Landed** (`medium` detail level open) |
| | AQL — builders, parsed AST, static lint | **Landed** |
| | Simplified formats (FLAT / STRUCTURED) + WebTemplate export | **Landed** |
| **3 — Deployment & adoption** | Application SMART (`smart/` AppContext) on discovery | **Partial** |
| | EHRbase CDR support | **Partial** |
| | Worked examples (`cmd/examples/`) | **Landed** |
| | Documentation website | **Planned** |
| **4 — Platform extras & ADL 2** | Cadasto extras (`cadasto/*`) | **Planned** |
| | ADL 2 / AOM 2.4 — codegen and the Definition ADL 2 format | **Deferred** |
| **5 — Conformance ratification** | Sandbox transport + probe runner | **Partial** |
| | Cassette / Live probe ratification against a real CDR | **Partial** |

Stage 2's per-REQ delivery history is in its [archived phase plan](plans/archive/2026-05-21-phase-2-clinical-building-blocks.md); the tables below carry the current state.

---

## Core openEHR building blocks

| Feature | Status | Package / REQ | Notes |
|---------|--------|---------------|-------|
| BMM loader | **Landed** | `openehr/bmm/` REQ-045 | |
| RM types (generated) | **Landed** | `openehr/rm/` REQ-030–033, 041–047 | From pinned `resources/bmm/` |
| Type registry | **Landed** | `openehr/rm/typereg/` REQ-040 | |
| AOM 1.4 (generated) | **Landed** | `openehr/aom/aom14/` | |
| AOM 2.4 | **Deferred** | — | BMM pinned in `resources/bmm/`; no codegen and no package yet (stage 4) |
| Canonical JSON | **Landed** | `openehr/serialize/canjson/` REQ-052 | PROBE-030/031/038. Narrow polymorphic decode via `<Parent>Like` interfaces; `_type` encode/decode round-trips byte-stable, with `DV_INTERVAL<T>` bounds validation ([plan](plans/archive/2026-06-23-polymorphic-encode-decode.md)) |
| Canonical XML | **Landed** | `openehr/serialize/canxml/` REQ-056 | PROBE-033/034 |
| FLAT / STRUCTURED | **Landed** | `openehr/serialize/simplified/` REQ-053, REQ-140 | Bidirectional FLAT + STRUCTURED codecs plus OPT-free interconversion, driven by the REQ-106 Web Template. Full `DV_*` set, `ctx/` metadata, `\|other` open value-sets, strict fail-loud decode; `Unmarshal*(…, WithTemplate(compiled))` yields a composition that **validates against the OPT**. REQ-140's underscore-prefixed RM attribute grammar (`_uid`, `_link:N`, `_feeder_audit`, party families, value decorations) took upstream EHRbase corpus parity to **80.4%** (PROBE-086, PROBE-089). Refusals and the two registered projection losses are listed in the package's `deviations.md` ([ADR 0015](adr/0015-flat-metadata-spelling.md), [plan](plans/archive/2026-07-14-flat-structured-codecs.md)) |
| WebTemplate JSON export | **Landed** | `openehr/template/webtemplate/` REQ-106 | Compiled OPT → EHRbase `openEHR_SDK` v2.3 WebTemplate JSON. Structural and input parity against three vendored references (PROBE-075); archetype-reuse-under-slot landed via REQ-116 ([ADR 0014](adr/0014-webtemplate-reference-implementation-lock.md), [plan](plans/archive/2026-05-22-webtemplate-export.md)) |
| Template-level node naming | **Landed** | `openehr/template/`, `openehr/template/webtemplate/` REQ-116 | The `C_STRING` an OPT pins on a node's `name` is parsed, exposed (`NodeName()`), emitted as the reference's `[archetype_id,'Name']` predicate, and preferred for the WebTemplate `id` — so a template reusing one archetype among siblings is addressable. Paths through unnamed nodes are byte-identical to before ([plan](plans/archive/2026-07-29-template-node-naming.md)) |
| OPT parser (ADL 1.4 `.opt`) | **Landed** | `openehr/template/` REQ-100 | Parse + path utilities + PROBE-022; strict-mode parse, `WithStrictPaths`, `ValidatePath`, `Description()`/`Annotations()`. OET out of scope ([plan](plans/archive/2026-05-22-template-req100-followups.md)) |
| Primitive constraint introspection | **Landed** | `openehr/template/constraints/` REQ-103 | Closed-set `PrimitiveConstraint` types with typed `Violation` payloads and a pure `Validate(value any)`; PROBE-024. AOM partial-pattern enforcement deferred |
| Slot assertion grammar | **Landed** | `openehr/template/constraints/`, `openehr/validation/`, `openehr/instance/` REQ-104 | `SlotAssertion`/`SlotRules` parse anchored `archetype_id matches {regex}` from OPT includes/excludes; validator and generator enforce slot-fit; PROBE-027 |
| Terminology bindings | **Landed** | `openehr/template/` REQ-105 | `Terms()`/`TermBindings()` accessors surface OPT term definitions and external bindings. External terminology lookup deferred |
| RM structural lookup | **Landed** | `openehr/rm/rminfo/` | BMM-derived `Lookup` (`RequiredAttributes`, `AttributeRMType`, `IsContainer`, `KnownRMTypes`); stdlib-only, no runtime BMM ([ADR 0005](adr/0005-compiled-template-foundation.md)) |
| RM meta-model introspection | **Landed** | `openehr/rm/rminfo/` REQ-048 | The RM **class graph** beside the attribute tables: abstractness, parents, transitive ancestors, conformance, concrete-descendant expansion, attribute declaration site — on an optional `Hierarchy` interface, so no external `Lookup` implementer breaks. Compiled-in only; PROBE-094 checks it against an independent reduction of the pinned BMM ([plan](plans/archive/2026-08-18-rminfo-class-hierarchy.md)) |
| RM behavioural functions | **Landed** | `openehr/rm/`, `openehr/rm/rmpath/` REQ-120–123 | Identifier parsing/derivation, `VERSION.is_branch`, temporal `DV_*` compare/convert, and locatable path read access (`ItemAtPath`, `PathExists`, …) over a reflection-free walker. Arithmetic, `parent`, and VERSIONED_OBJECT container ops deferred ([ADR 0011](adr/0011-rm-behavioural-functions-surface.md), [plan](plans/archive/2026-06-19-rm-functions.md)) |
| Compiled OPT foundation | **Landed (internal)** | `internal/templatecompile/` | Walker-friendly tree with cached AQL paths, implicit RM-attribute injection, per-archetype-root term scope. Engine stays internal; public access is the REQ-111 bridge ([ADR 0005](adr/0005-compiled-template-foundation.md)) |
| Public compiled-template bridge | **Landed** | `openehr/templatecompile/` REQ-111 | `Compile(opt)` re-exports the compiled form so external modules reach the builder, synthesiser, validator, and AQL lint through public packages ([ADR 0010](adr/0010-public-compiled-template-bridge.md)) |
| Composition vs OPT validation | **Landed** | `openehr/validation/` REQ-102 | Template-driven `ValidateComposition`; PROBE-025/026 |
| Validation beyond COMPOSITION | **Landed** | `openehr/validation/` REQ-110 | Generic `Validate(root, c)` plus typed `ValidateDemographic` (PARTY hierarchy), `ValidateFolder`, `ValidateEHRStatus`; PROBE-074 |
| Template-less RM validation floor | **Landed** | `openehr/validation/`, `openehr/validation/rmread/` REQ-112 | `ValidateRM` and typed sugars walk any RM root with `rminfo` as sole driver — no template required — checking RM-mandatory absences and a per-type invariant catalogue. PROBE-077 deferred ([plan](plans/archive/2026-06-29-rm-floor-validation.md)) |
| AQL static lint | **Landed** | `openehr/aql/lint/`, `openehr/validation/` REQ-109 | Parse against the SDK grammar profile ([ADR 0007](adr/0007-aql-antlr-grammar-profile.md)) → 3-layer lint → `validation.ValidateAQL` bridge; PROBE-028 |
| AQL wire models + builders | **Landed** | `openehr/aql/` REQ-055 | Literal AQL + ResultSet; struct-builder and verb-functions emit byte-identical canonical AQL (PROBE-020); `ErrPathResolution` mapping (PROBE-021) |
| Parsed AQL AST + round-trip emitter | **Landed** | `openehr/aql/parse/`, `openehr/aql/` REQ-113 | `parse.Query` is the read-side mirror of `aql.Builder`, sharing one `WhereExpr`/`Value` vocabulary; `(*Query).Emit` closes the round-trip. Out-of-catalogue shapes surface `aql.ErrIncompleteAST` rather than dropping a clause. `Document.Dropped()` and `lint.Issue.Span` report findings **value-free** (PROBE-096), and `PathSegment.Parsed` types every bracketed predicate (PROBE-095) |
| AQL expression-catalogue completion | **Landed** | `openehr/aql/` REQ-117 | The AST covers the whole SDK grammar profile — mixed-star SELECT, function calls, path operands, `MATCHES TERMINOLOGY(…)`/`{uri}`, FROM-root junctions (PROBE-087). Builder gains the containment algebra and opt-in in-text paging (PROBE-088) |
| AQL `SELECT TOP` + literal source text | **Landed** | `openehr/aql/` REQ-118 | The deprecated `SELECT TOP n [FORWARD\|BACKWARD]` is carried on both sides, so a query the SDK did not author round-trips. `LiteralExpr.Raw` keeps a literal's source text (the result schema names unaliased columns by it) while emission stays canonical; `TOP` with `LIMIT` is refused |
| Re-parseable canonical AQL emission | **Landed** | `openehr/aql/` REQ-119 | Every validating write path guards the **value** positions, not just clause shapes: grammar-correct string escaping, mandatory real fractional part, positional `MATCHES {uri}` checks, function-name and arity checks. `Emit` re-parses its own output and compares a skeleton. **Wire-visible:** a literal carrying `'` or `\` and a whole-valued real (`2` → `2.0`) emit differently, and predicate/path text is re-emitted verbatim — normalise through `aql.StripPredicateTrivia` when comparing (PROBE-090) |
| OPT → RM instance synthesis | **Landed** | `openehr/instance/` REQ-107 | `Generate(ctx, c, opts)` with closed-root accessors, a `UIDSource` test seam, and seeded synthetic value fill; PROBE-027 over `vital_signs.opt`, `clinical_note.opt`, and a real-world corpus ([plan](plans/archive/2026-05-24-template-instance-example-generator.md)) |
| Synthesis `medium`/`detail_level` level | **Planned** | `openehr/instance/` REQ-107 | Representative optional-subset fill between `Minimal` and full population |
| Composition builder | **Landed** | `openehr/composition/` REQ-101 | `NewSkeleton` + `Builder.Set/SetText/SetQuantity/SetCodedText/Build`; PROBE-023 full unmarshal round-trip |
| LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| EHR Extract RM | **Deferred** | — | Out of v1 scope |

---

## Auth and transport

| Feature | Status | Package / REQ | Notes |
|---------|--------|---------------|-------|
| `TokenSource` + per-request ctx | **Landed** | `auth/` REQ-060 | |
| Client credentials | **Landed** | `auth/clientcreds/` REQ-068 | Symmetric `client_secret` + SMART Backend Services asymmetric (`WithClientAssertion`) |
| JWT Bearer | **Landed** | `auth/jwtbearer/` REQ-068 | RS384 default + ES384/RS256/ES256; `private_key_jwt` and SMART Backend Services |
| HTTP Basic on openEHR REST | **Landed** | `auth/basic/` REQ-069 | |
| Caller attribution | **Landed** | `transport/` REQ-066 | PROBE-009 — opt-in header + `caller.agent_id` OTel attribute |
| SMART PKCE + launch | **Landed** | `auth/smart/` REQ-061–063 | PKCE, code exchange, refresh, JWKS cache; PROBE-001/004/005 |
| Application launch context | **Landed** | `smart/` REQ-064, REQ-067 | LaunchContext, ID-token validation, principal claims (PROBE-008); openEHR-native `ehrId`/`episodeId` |
| JWKS rotation | **Landed** | `auth/smart/` REQ-062 | Cache + refresh-on-miss; PROBE-006 |
| Token refresh (SMART provider) | **Landed** | `auth/smart/` REQ-063 | Proactive expiry refresh + transport 401→reauth; PROBE-007 covers both halves |
| SMART flows + launch modes | **Landed** | `auth/smart/`, `auth/clientcreds/`, `auth/jwtbearer/` REQ-068 | All 4 flows × 3 launch modes probe-covered in Sandbox; Inferno STU2.2 cross-check recorded in [conformance.md](specifications/conformance.md). Cassette/Live ratification deferred |
| Transport (HTTP, retry, OTel, errors) | **Landed** | `transport/` REQ-090–093, REQ-096, REQ-098 | |
| `Prefer` negotiation | **Landed** | `transport/`, `openehr/client/ehr/*` REQ-094 | All three write-path modes: `return=representation` bare-body decode (PROBE-061/071), `return=identifier` slot population, and representation + empty body → `ErrInvalidShape`. Result-contract amendment planned ([plan](plans/2026-08-18-write-result-contract.md)); PROBE-065 round-trip deferred |
| Path-parameter segment validation | **Landed** | `transport/`, `openehr/client/*` REQ-150 | Refuses `.`/`..`/empty/`\`/control-character segments and a path whose segment count contradicts its `Route` template — fails closed with no request. Service root exempt; PROBE-091 across ten leaves, with an AST tripwire holding every leaf to setting `Route` ([plan](plans/archive/2026-08-18-path-segment-validation.md)) |
| Transport `NoRetry` / `Disabled` | **Landed** | `transport/` REQ-096 | Bench-friendly retry opt-out |
| Transport observer hook | **Landed** | `transport/` REQ-098 | `WithObserver` + `WithObservationTag` |
| Service discovery | **Landed** | `smart/discovery/` REQ-070–072 | `services` wire-shape fix plus endpoint/alg metadata ([ADR 0008](adr/0008-smart-discovery-services-shape.md), [plan](plans/archive/2026-06-16-auth-smart-conformance-audit.md)) |

---

## REST clients (`openehr/client/*`)

| API area | Status | Package | Notes |
|----------|--------|---------|-------|
| System | **Landed** | `openehr/client/system/` | Capabilities, version |
| EHR (create, get, delete) | **Landed** | `openehr/client/ehr/` | |
| EHR_STATUS | **Landed** | `openehr/client/ehr/ehrstatus/` | |
| Composition CRUD | **Landed** | `openehr/client/ehr/composition/` | REQ-054 If-Match; PROBE-071 representation decode; REQ-094 `Prefer` write path complete |
| Directory | **Landed** | `openehr/client/ehr/directory/` | Same REQ-094 / PROBE-071 notes as composition |
| Contribution | **Landed** | `openehr/client/ehr/contribution/` | **Write:** submission body is `Contribution_create`, response the persisted `rm.Contribution`; the commit audit drops server-assigned `time_committed` and keeps a `DV_CODED_TEXT` `change_type` (PROBE-072). **Read (REQ-142):** `Get` issues `GET /ehr/{ehr_id}/contribution/{contribution_uid}`; empty ids fail closed before any request and 404 maps to `ErrNotFound` (PROBE-092) |
| ItemTags | **Landed** | `openehr/client/ehr/itemtags/` | REQ-059; header codec + composition/ehrstatus/directory GET, composition PUT |
| Query (AQL execute) | **Landed** | `openehr/client/query/` | Ad-hoc + stored execute; REQ-055 verb-aware `openehr-ehr-id` POST scoping. PROBE-078 deferred |
| Definition — ADL 1.4 templates | **Landed** | `openehr/client/definition/` | Upload/list/get/delete + example composition. List filters `template_id`/`concept`/`version`/`offset`/`fetch` (REQ-143, PROBE-093); explicit zero paging is sent, negative refused |
| Definition — stored AQL | **Landed** | `openehr/client/definition/` | Put/get/list/delete; REQ-057 `PutStoredQuery` recovers `{name, version}` from the `Location` header on a body-less reply. PROBE-079 deferred |
| Definition — ADL 2 | **Deferred** | — | `FormatADL14` is the only registered format (stage 4) |
| Demographic | **Landed** | `openehr/client/demographic/` | PARTY hierarchy CRUD (five typed resources) + read-only `versioned_party`; polymorphic decode via `typereg.DecodeAs[rm.Party]`; PROBE-073 |
| Admin (ITS-REST) | **Landed** | `openehr/client/admin/` | `DeleteEHR`, `DeleteAllEHRs`, `PurgeTemplates` (REQ-099) |

Deferred Tier-3 gaps — dedicated ITEM_TAG endpoints, `VERSIONED_*` read families, content negotiation — and the REST-probe follow-ups are tracked in [STRAND-09](specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups). Delivery history: [REST client](plans/archive/2026-05-15-rest-api-client.md) · [demographic](plans/archive/2026-06-14-demographic-rest-client.md) · [ITS-REST remediation](plans/archive/2026-06-19-its-rest-conformance-remediation.md).

---

## Deployment targets

| Target | Status | Notes |
|---|---|---|
| EHRbase CDR | **Partial** | Vendored EHRbase OpenAPI specs in [`resources/ehrbase/`](../resources/ehrbase/README.md) (deployment extensions, **not** the normative contract); WebTemplate export matches `openEHR_SDK` v2.3 (PROBE-075); the FLAT codec round-trips EHRbase's own corpus at 80.4% (PROBE-086); the EHRbase-specific template purge is wired (REQ-099). Ratification against a **running** deployment is open — see stage 5 |
| Any ITS-REST 1.1.0 CDR | **Partial** | The clients target the vendored [`resources/its-rest/`](../resources/its-rest/README.md) pin; conformance is asserted in Sandbox, not yet against a live third-party CDR |
| Static / non-discovering backend | **Landed** | Build a `discovery.ServiceCatalog` by hand — no base-URL parameter (REQ-070); see [quick-start.md](quick-start.md) |

---

## Application SMART and Cadasto

| Feature | Status | Package | Notes |
|---------|--------|---------|-------|
| Discovery resolver + cache | **Landed** | `smart/discovery/` | |
| AppContext / launch helpers | **Partial** | `smart/` | LaunchContext + ID-token validation (REQ-064/067) landed; App Registration open ([STRAND-05](specifications/research-strands.md)) |
| Cadasto Extra API | **Planned** | `cadasto/extra/` | |
| Datamap V2 | **Planned** | `cadasto/datamap/` REQ-058 | |
| MPI preview | **Planned** | `cadasto/mpi/` | |
| Cadasto admin | **Partial** | `cadasto/admin/` | Health probes (`Live`, `Ready`) landed per REQ-083; tenant/env/system-info planned. Distinct from the ITS Admin client |
| Care aggregates | **Planned** | `cadasto/care/` | |

---

## Test infrastructure and conformance

| Feature | Status | Location | Notes |
|---------|--------|----------|-------|
| Serialize probes | **Landed** | `testkit/probes/serialize/` | PROBE-030/031, 033/034 |
| Versioned-write probes | **Landed** | `testkit/probes/versioned/` | PROBE-010–013, 071, 092 |
| Validation probes | **Landed** | `testkit/probes/validation/` | PROBE-025/026 |
| Instance synthesis probe | **Landed** | `testkit/probes/instance/` | PROBE-027 |
| Composition builder probe | **Landed** | `testkit/probes/composition/` | PROBE-023 — full marshal → unmarshal → re-marshal round-trip |
| AQL probes | **Landed** | `testkit/probes/aql/` | PROBE-020 (struct vs verb byte-identical), PROBE-021, PROBE-088 |
| Definition probes | **Landed** | `testkit/probes/definition/` | PROBE-067, PROBE-093 |
| Discovery probes | **Landed** | `testkit/probes/discovery/` | PROBE-040/041 |
| Transport probes | **Landed** | `testkit/probes/transport/` | PROBE-091 |
| Auth / REST probes | **Partial** | `testkit/probes/auth/` | PROBE-001…009 all implemented (Sandbox) plus launch-mode coverage; the PROBE-060+ REST-binding probes are mostly Draft |
| Sandbox transport | **Planned** | `sandbox/` | Reserved name, `doc.go` only — the suite hand-writes an `httptest` server per test. Phase 2 of the [runnability plan](plans/2026-08-18-probe-runnability.md) |
| Testkit helpers + probe runner | **Partial** | `testkit/` | Probe packages landed but there is **no runner**: the `Result` type is duplicated across 11 packages and each probe is reached only from its own bespoke test. REQ-082 specified the modes, result contract, and per-mode rules; phases 1–2 of the [runnability plan](plans/2026-08-18-probe-runnability.md) build them. Recording format open ([STRAND-11](specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml)) |
| openEHR conformance ratification | **Partial** | `testkit/conformance/webtemplate/` | REQ-080/082. **PROBE-086** round-trips the pinned upstream EHRbase FLAT corpus — 34 bodies this SDK did not write — exact on the modelled subset: **1466 of 1824 keys (80.4%)**, up from 10.5% on landing. Remaining refusals are censused in [SKIPPED.md](../testkit/conformance/webtemplate/SKIPPED.md). Live-CDR ratification and the Cassette/Live modes remain open |
| Cadasto API conformance | **Planned** | `testkit/cassettes/cadasto/` | REQ-083 — anchored to the Cadasto platform API contract (stage 4) |
| OpenAPI cassettes | **Partial** | `testkit/cassettes/` REQ-095 | Not all surfaces covered |

---

## Tooling, docs and examples

| Feature | Status | Notes |
|---------|--------|-------|
| `make ci` + grouped `make help` | **Landed** | Gate detail in [ci.md](ci.md) |
| `make spec-check` | **Landed** | Traceability subset only |
| Release / semver strategy | **Landed** | Tag-driven [`release.yml`](../.github/workflows/release.yml); policy in [releases.md](releases.md) |
| `cmd/bmmgen` / `cmd/bmmdiff` | **Landed** | Codegen and BMM-corpus diff tooling |
| Developer onboarding | **Landed** | [quick-start.md](quick-start.md) — install, two integration paths, REST wiring |
| Worked examples | **Landed** | [`cmd/examples/`](../cmd/examples/) — 16 runnable programs, catalogued in [examples.md](examples.md) (the single list) |
| Documentation website | **Planned** | No site generator in the tree yet; docs are read from `docs/` and on GitHub |

---

## How to update this file

1. **After landing a feature:** flip the status here, set `Impl. landed` in [`REQ.md`](specifications/REQ.md), and add the paths to [`traceability.yaml`](specifications/traceability.yaml).
2. **After closing a plan phase:** update the plan's progress table **and** the stage row above.
3. **Keep it a checklist, not a spec.** Do not duplicate normative REQ prose or per-release history here — link to `docs/specifications/` and the archived plan instead. A Notes cell that outgrows a couple of sentences belongs in the plan it links to.
