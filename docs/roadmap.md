# SDK roadmap — landed vs planned

**Status:** Living checklist — tracks **implementation reality** against the normative contract in [`docs/specifications/`](../docs/specifications/). When this file and `docs/specifications/` disagree, **`docs/specifications/` wins** — update this roadmap.

**Also see:** [AGENTS.md § Project](../AGENTS.md#project), REQ registry [`docs/specifications/REQ.md`](../docs/specifications/REQ.md), machine traceability [`docs/specifications/traceability.yaml`](../docs/specifications/traceability.yaml), sequenced delivery in [`docs/plans/`](plans/).

## Legend

| Symbol | Meaning |
|--------|---------|
| **Landed** | Code + tests in tree; usable (may still be v1-preview quality) |
| **Partial** | Subset implemented or spec-only traceability incomplete |
| **Planned** | Normative in `docs/specifications/`; directory may exist with `doc.go` only |
| **Deferred** | Explicitly out of v1 scope in specs or ADRs |

---

## Milestones (delivery phases)

| Phase | Focus | Status |
|-------|--------|--------|
| **0** | Scaffolding — module layout, specs, Makefile, CI | **Complete** |
| **0.5** | BMM loader, codegen (RM + AOM 1.4), typereg, canonical JSON | **Landed** |
| **1a** | Transport, auth (clientcreds, jwtbearer, basic), discovery, System + EHR REST | **Landed** |
| **1b** | SMART PKCE (`auth/smart`), Query client, Definition stored AQL, ID-token validation, benchmark harness | **Partial** (PKCE, Query, stored AQL, ID-token validation landed; benchmark harness deferred) |
| **2** | Composition builder, template parser, validation, AQL builders (+ executor landed) | **Landed** — OPT parser (REQ-100) + follow-ups + REQ-103 + compiled template foundation; **REQ-102 composition validation** ([archive](plans/archive/2026-05-24-composition-validation-template-driven.md)); **REQ-107 template-driven instance generator** ([archive](plans/archive/2026-05-24-template-instance-example-generator.md)); **REQ-101 composition builder** ([archive](plans/archive/2026-05-21-composition-builder.md)); **C_PRIMITIVE_OBJECT wire-parser + UID emission** ([archive](plans/archive/2026-05-26-c-primitive-object-wire-parser.md)) — PROBE-023 full unmarshal round-trip; **REQ-104 slot assertion grammar** + **REQ-105 terminology bindings** ([archive](plans/archive/2026-06-12-template-req104-req105-deferred.md), PR #43); **REQ-055 AQL builders** (struct + verb, PROBE-020/021); **REQ-109 AQL parse + static lint** (`openehr/aql/parse` + `openehr/aql/lint` + `validation.ValidateAQL`, SDK grammar profile, PROBE-028, [archive](plans/archive/2026-06-15-aql-lint.md)); **REQ-110 validation beyond COMPOSITION** — demographic PARTY hierarchy + FOLDER / EHR_STATUS through the same walker (PROBE-074, [archive](plans/archive/2026-06-17-validation-non-composition-roots.md)) — all child Phase 1 rows done. See [plans/2026-05-21-phase-2-clinical-building-blocks.md](plans/2026-05-21-phase-2-clinical-building-blocks.md). |
| **3** | Application SMART (`smart/` AppContext) on discovery | **Partial** (discovery + launch context REQ-064/067) |
| **4** | Cadasto extras (`cadasto/*`) | **Not started** |
| **5** | Sandbox transports + full conformance probe ratification | **Partial** |

---

## Core openEHR building blocks

| Feature | Status | Package / REQ | Notes |
|---------|--------|---------------|-------|
| BMM loader | **Landed** | `openehr/bmm/` REQ-045 | |
| RM types (generated) | **Landed** | `openehr/rm/` REQ-030–033, 041–047 | From pinned `resources/bmm/` |
| Type registry | **Landed** | `openehr/rm/typereg/` REQ-040 | |
| AOM 1.4 (generated) | **Landed** | `openehr/aom/aom14/` | |
| AOM 2.4 | **Deferred** | `openehr/aom/aom2/` | BMM pinned; no codegen yet |
| Canonical JSON | **Landed** | `openehr/serialize/canjson/` REQ-052 | PROBE-030/031/038; SDK-GAP-11 narrow polymorphic decode (`<Parent>Like` interfaces) [archived plan](plans/archive/2026-05-26-rm-polymorphic-decode-coverage.md) |
| Canonical XML | **Landed** | `openehr/serialize/canxml/` REQ-056 | PROBE-033/034; traceability indexed |
| FLAT / STRUCTURED | **Planned** | `openehr/serialize/` REQ-053 | Parent package is placeholder |
| OPT parser (ADL 1.4 `.opt`) | **Landed** | `openehr/template/` REQ-100 | Parse + path utilities + PROBE-022; follow-up Phases 1–3 landed: strict-mode parse (`ParseOPTStrict` / `ParseFileStrict`), `WithStrictPaths` + `ErrAmbiguousPath`, `ValidatePath`, `Description()` / `Annotations()`, `ObjectNode` walker supertype, `Cardinality.String`/`IsValid`; OET out of scope. [plan](plans/archive/2026-05-22-template-req100-followups.md) |
| Primitive constraint introspection | **Landed** | `openehr/template/constraints/` REQ-103 | Closed-set `PrimitiveConstraint` types (`CBoolean`, `CInteger`, `CReal`, `CString`, `CDate`, `CTime`, `CDateTime`, `CDuration`, `CodePhrase`, `DvQuantity`, `CDvOrdinal`); typed `Violation` payloads; pure `Validate(value any)`. Threaded through `ComplexObject.PrimitiveConstraint()` + `CompiledNode.PrimitiveConstraint()`; PROBE-024. AOM partial-pattern enforcement deferred. [plan](plans/archive/2026-05-22-template-req100-followups.md) |
| Slot assertion grammar (REQ-104) | **Landed** | `openehr/template/constraints/`, `openehr/template/`, `internal/templatecompile/`, `openehr/validation/`, `openehr/instance/` | `SlotAssertion` / `SlotRules` with anchored `archetype_id matches {regex}` parsing from OPT includes/excludes; validator + instance generator enforce parsed slot-fit (RM-type-prefix fallback only when no includes parsed); PROBE-027 slot-fill path updated. [plan](plans/archive/2026-06-12-template-req104-req105-deferred.md) |
| Terminology bindings (REQ-105) | **Landed** | `openehr/template/`, `internal/templatecompile/` | `ArchetypeRoot.Terms()` / `TermBindings()` deep-copy accessors; compiled `Term`, `TermLang`, `TermBindings`, `TermBindingsForNode` surface OPT term definitions and external bindings (single document language; `lang` forward-compat). External terminology lookup deferred. [plan](plans/archive/2026-06-12-template-req104-req105-deferred.md) |
| RM structural lookup | **Landed** | `openehr/rm/rminfo/` | BMM-derived `Lookup` (`RequiredAttributes`, `AttributeRMType`, `IsContainer`, `KnownRMTypes`); stdlib-only, no runtime BMM dependency. [ADR 0005](adr/0005-compiled-template-foundation.md) |
| Compiled OPT foundation | **Landed (internal)** | `internal/templatecompile/` | `Compile` produces walker-friendly tree with cached AQL paths, implicit RM-attribute injection, per-archetype-root term scope; consumed by composition builder (REQ-101) and validator (REQ-102). Internal until public shape stabilises. [ADR 0005](adr/0005-compiled-template-foundation.md) |
| Composition vs OPT validation (REQ-102) | **Landed** | `openehr/validation/` | Template-driven `ValidateComposition`; PROBE-025/026. [plan](plans/archive/2026-05-24-composition-validation-template-driven.md) |
| AQL static lint (REQ-109) | **Landed** | `openehr/aql/parse/`, `openehr/aql/lint/`, `openehr/validation/` | Parse against the SDK grammar profile (ADR 0007) → 3-layer lint (`lint.LintString` / `lint.Lint`) → `validation.ValidateAQL` bridge; PROBE-028. [plan](plans/archive/2026-06-15-aql-lint.md) |
| Validation beyond COMPOSITION (REQ-110) | **Landed** | `openehr/validation/` | Generic `Validate(root, c)` + typed `ValidateDemographic` (PARTY hierarchy + ADDRESS/CONTACT/PARTY_IDENTITY/PARTY_RELATIONSHIP/CAPABILITY), `ValidateFolder`, `ValidateEHRStatus`; DataValue-leaf readers; PROBE-074. [plan](plans/archive/2026-06-17-validation-non-composition-roots.md) |
| AQL wire models + builders | **Landed** | `openehr/aql/` REQ-055 | Literal AQL + ResultSet; struct-builder + verb-functions emit byte-identical canonical AQL (PROBE-020); `ErrPathResolution` mapping (PROBE-021). [builders plan](plans/archive/2026-05-21-aql-builders.md) |
| OPT → RM instance synthesis | **Landed** | `openehr/instance/` REQ-107 | `Generate(ctx, c, opts)` + closed-root accessors + `internal/templateinstance/rmwrite/`; PROBE-027 on `vital_signs.opt` + `clinical_note.opt`; `Options.UIDSource` test-determinism seam; canjson-polymorphic `Composition.uid`. [archive](plans/archive/2026-05-24-template-instance-example-generator.md) + [wire-parser archive](plans/archive/2026-05-26-c-primitive-object-wire-parser.md). |
| Composition builder | **Landed** | `openehr/composition/` REQ-101 | `NewSkeleton` + `Builder.Set/SetText/SetQuantity/SetCodedText/Build`; PROBE-023 (full unmarshal round-trip). [archive](plans/archive/2026-05-21-composition-builder.md) |
| LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| EHR Extract RM | **Deferred** | — | Skipped per v1 scope |

---

## Auth and transport

| Feature | Status | Package / REQ | Notes |
|---------|--------|---------------|-------|
| `TokenSource` + per-request ctx | **Landed** | `auth/` REQ-060 | |
| Client credentials | **Landed** | `auth/clientcreds/` REQ-068 | Symmetric `client_secret` + SMART Backend Services asymmetric (`WithClientAssertion`); backend launch-mode probe coverage in `testkit/probes/auth/launch_modes.go` |
| JWT Bearer | **Landed** | `auth/jwtbearer/` REQ-068 | RS384 default + ES384/RS256/ES256; `private_key_jwt` + SMART Backend Services landed; backend launch-mode probe coverage |
| HTTP Basic on openEHR REST | **Landed** | `auth/basic/` REQ-069 | |
| Caller attribution | **Landed** | `transport/` REQ-066 | PROBE-009 (opt-in header + `caller.agent_id` OTel attribute) |
| SMART PKCE + launch | **Landed** | `auth/smart/` REQ-061–063 | PKCE, code exchange, refresh, JWKS cache. Phase 5 probes: PROBE-001 (discovery code+S256), PROBE-004 (PKCE + G-7 parity), PROBE-005 (scope round-trip) |
| Application launch context | **Landed** | `smart/` REQ-064, REQ-067 | LaunchContext, ID-token validation, principal claims (PROBE-008). openEHR-native `ehrId`/`episodeId` + id-token alg agility (RS384/ES384) landed |
| JWKS rotation | **Landed** | `auth/smart/` REQ-062 | Cache + refresh-on-miss; PROBE-006 (one refresh on kid rotation, transparent) |
| Token refresh (SMART provider) | **Landed** | `auth/smart/` REQ-063 | Proactive expiry refresh + transport 401→reauth. PROBE-007 covers both halves in Sandbox (`probe_007_transport_refresh.go` + `probe_007_proactive_refresh.go`) |
| SMART flows + launch modes | **Landed** | `auth/smart/`, `auth/clientcreds/`, `auth/jwtbearer/` REQ-068 | All 4 flows × 3 launch modes (standalone / embedded / backend) probe-covered in Sandbox; Inferno STU2.2 Client-suite cross-check + recorded gaps in [conformance.md](specifications/conformance.md). Cassette/Live ratification deferred |
| Transport (HTTP, retry, OTel, errors) | **Landed** | `transport/` REQ-090–093, REQ-096–098 | |
| `Prefer` negotiation (REQ-094) | **Landed** | `transport/`, `openehr/client/ehr/composition/`, `directory/`, `ehrstatus/` | All three write-path modes landed: `return=representation` bare-body decode (SDK-GAP-09), `return=identifier` slot population, and `representation` + empty body → `ErrInvalidShape`. [Archived plan](plans/archive/2026-05-25-req094-prefer-followups.md); PROBE-065 round-trip deferred |
| Transport `NoRetry` / `Disabled` | **Landed** | `transport/` REQ-096 | Bench-friendly retry opt-out |
| Transport observer hook | **Landed** | `transport/` REQ-098 | `WithObserver` + `WithObservationTag` |
| Service discovery | **Landed** | `smart/discovery/` REQ-070–072 | `services` wire-shape fix (object vs array) + extra endpoint/alg metadata sequenced in [plans/2026-06-16-auth-smart-conformance-audit.md](plans/2026-06-16-auth-smart-conformance-audit.md) (ADR 0008) |

---

## REST clients (`openehr/client/*`)

| API area | Status | Package | Notes |
|----------|--------|---------|-------|
| System | **Landed** | `openehr/client/system/` | Capabilities, version |
| EHR (create, get, delete) | **Landed** | `openehr/client/ehr/` | |
| EHR_STATUS | **Landed** | `openehr/client/ehr/ehrstatus/` | |
| Composition CRUD | **Landed** | `openehr/client/ehr/composition/` | REQ-054 If-Match; SDK-GAP-09 representation decode; REQ-094 `Prefer` write-path complete |
| Directory | **Landed** | `openehr/client/ehr/directory/` | Same REQ-094 / SDK-GAP-09 notes as composition |
| Contribution | **Landed** | `openehr/client/ehr/contribution/` | Submission body is `Contribution_create` (inline `ORIGINAL_VERSION`/`IMPORTED_VERSION` with `data: T`); response remains persisted `rm.Contribution`. Write-side commit audit drops server-assigned `time_committed`, keeps `DV_CODED_TEXT` `change_type`, defaults to `AUDIT_DETAILS` with an `UPDATE_AUDIT` fallback (SPECITS-95 / ITS-REST PR 131). PROBE-072 / SDK-GAP-10. Archived plans: [submission shape](plans/archive/2026-05-26-contribution-submission-shape.md), [write-audit](plans/archive/2026-06-11-contribution-update-audit-dv-coded-text.md). |
| ItemTags | **Landed** | `openehr/client/ehr/itemtags/` | REQ-059; header codec + composition/ehrstatus/directory GET, composition PUT |
| Query (AQL execute) | **Landed** | `openehr/client/query/` | Ad-hoc + stored execute; REQ-055 |
| Definition — ADL 1.4 templates | **Landed** | `openehr/client/definition/` | Upload/list/get/delete, example composition |
| Definition — stored AQL | **Landed** | `openehr/client/definition/` | Put/get/list/delete; REQ-057 |
| Definition — ADL 2 | **Planned** | — | Deferred in package docs |
| Demographic | **Planned** | `openehr/client/demographic/` | `doc.go` only |
| Admin (ITS-REST) | **Landed** | `openehr/client/admin/` | `DeleteEHR`, `DeleteAllEHRs`, `PurgeTemplates` (REQ-099) |

REST delivery detail: [2026-05-15-rest-api-client.md](plans/archive/2026-05-15-rest-api-client.md) (archived; this roadmap reflects the tree). Demographic follow-up: [2026-06-14-demographic-rest-client.md](plans/archive/2026-06-14-demographic-rest-client.md).

---

## Application SMART and Cadasto

| Feature | Status | Package | Notes |
|---------|--------|---------|-------|
| Discovery resolver + cache | **Landed** | `smart/discovery/` | |
| AppContext / launch helpers | **Partial** | `smart/` | LaunchContext + ID-token validation (REQ-064/067); App Registration open (STRAND-05) — to be resolved via ADR 0009 in [plans/2026-06-16-auth-smart-conformance-audit.md](plans/2026-06-16-auth-smart-conformance-audit.md) |
| Cadasto Extra API | **Planned** | `cadasto/extra/` | |
| Datamap V2 | **Planned** | `cadasto/datamap/` REQ-058 | |
| MPI preview | **Planned** | `cadasto/mpi/` | |
| Cadasto admin | **Partial** | `cadasto/admin/` | Health probes (`Live`, `Ready`) landed per SDK-GAP-07; tenant/env/system-info planned. Distinct from ITS Admin client. |
| Care aggregates | **Planned** | `cadasto/care/` | |

---

## Test infrastructure and conformance

| Feature | Status | Location | Notes |
|---------|--------|----------|-------|
| Serialize probes | **Landed** | `testkit/probes/serialize/` | PROBE-030/031, 033/034 |
| Versioned-write probes | **Landed** | `testkit/probes/versioned/` | PROBE-010–013; PROBE-071 (SDK-GAP-09 representation writes) |
| Validation probes | **Landed** | `testkit/probes/validation/` | PROBE-025/026 (REQ-102) |
| Instance synthesis probe | **Landed** | `testkit/probes/instance/` | PROBE-027 (REQ-107 + REQ-104 slot-fill grammar) on `vital_signs.opt` + `clinical_note.opt` |
| Composition builder probe | **Landed** | `testkit/probes/composition/` | PROBE-023 (REQ-101) — full marshal → unmarshal → re-marshal round-trip |
| AQL builder probe | **Landed** | `testkit/probes/aql/` | PROBE-020 (REQ-055) — struct vs verb byte-identical, both match golden; PROBE-021 mapping sandbox-tested |
| Definition probe | **Landed** | `testkit/probes/definition/` | PROBE-067 |
| Discovery probes | **Landed** | `testkit/probes/discovery/` | PROBE-040/041 |
| Auth / REST probes | **Partial** | `testkit/probes/auth/`, `testkit/probes/versioned/`, leaf `*_test.go` | Auth suite PROBE-001…009 all implemented (Sandbox) in `testkit/probes/auth/` + launch-mode coverage (standalone/embedded/backend); PROBE-061/071 landed; PROBE-060+ REST-binding probes mostly Draft |
| Sandbox transport | **Planned** | `sandbox/` | `doc.go` only |
| Testkit helpers + probe runner | **Partial** | `testkit/` | Probe packages landed; `sandbox/` cassette runner open (REQ-082) |
| openEHR conformance ratification | **Planned** | — | REQ-080, REQ-082 |
| Cadasto API conformance | **Planned** | `testkit/cassettes/cadasto/` | REQ-083 — `cadasto/*` extras anchored to the Cadasto platform API contract (Phase 4) |
| OpenAPI cassettes | **Partial** | `testkit/cassettes/` REQ-095 | Not all surfaces covered |

---

## Tooling and examples

| Feature | Status | Notes |
|---------|--------|-------|
| `make ci` / grouped `make help` | **Landed** | |
| `make spec-check` | **Landed** | Traceability subset only |
| Release / semver strategy | **Landed** | Tag-driven [`release.yml`](../.github/workflows/release.yml); policy in [`releases.md`](releases.md), `v1.0.0` ceremony tracked separately ([archived plan](plans/archive/2026-05-25-versioning-strategy.md)) |
| Developer onboarding | **Landed** | [`quick-start.md`](quick-start.md) + [`examples.md`](examples.md) — install, REST wiring, catalog of all `cmd/examples/` programs |
| `cmd/bmmgen` / `cmd/bmmdiff` | **Landed** | |
| Worked examples | **Landed** | [`cmd/examples/`](../cmd/examples/) — `canonical_json`, `canxml_roundtrip`, `ehr_create`, `generate-example`, `opt-parse`, `primitive-validate`, `validate-composition`, `validate-from-json`; catalog in [`examples.md`](examples.md) |

---

## How to update this file

1. After landing a feature: flip status here and set `Impl. landed` in [`docs/specifications/REQ.md`](../docs/specifications/REQ.md); add paths to [`docs/specifications/traceability.yaml`](../docs/specifications/traceability.yaml).
2. After closing a plan phase: update the plan’s progress table **and** the milestone row above.
3. Do **not** duplicate normative REQ prose here — link to `docs/specifications/` instead.
