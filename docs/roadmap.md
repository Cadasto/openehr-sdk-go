# SDK roadmap — landed vs planned

**Status:** Living checklist (2026-05). Tracks **implementation reality** against the normative contract in [`specs/`](../specs/). When this file and `specs/` disagree, **`specs/` wins** — update this roadmap.

**Also see:** phase table in [AGENTS.md](../AGENTS.md#status-and-active-scope), REQ registry [`specs/REQ.md`](../specs/REQ.md), machine traceability [`specs/traceability.yaml`](../specs/traceability.yaml), sequenced delivery in [`docs/plans/`](plans/).

## Legend

| Symbol | Meaning |
|--------|---------|
| **Landed** | Code + tests in tree; usable (may still be v1-preview quality) |
| **Partial** | Subset implemented or spec-only traceability incomplete |
| **Planned** | Normative in `specs/`; directory may exist with `doc.go` only |
| **Deferred** | Explicitly out of v1 scope in specs or ADRs |

---

## Milestones (delivery phases)

| Phase | Focus | Status |
|-------|--------|--------|
| **0** | Scaffolding — module layout, specs, Makefile, CI | **Complete** |
| **0.5** | BMM loader, codegen (RM + AOM 1.4), typereg, canonical JSON | **Landed** |
| **1a** | Transport, auth (clientcreds, jwtbearer, basic), discovery, System + EHR REST | **Landed** |
| **1b** | SMART PKCE (`auth/smart`), Query client, Definition stored AQL, CDR benchmark (STRAND-01) | **Partial** (PKCE, Query, stored AQL landed; CDR benchmark deferred) |
| **2** | Composition builder, template parser, AQL builders (+ executor landed) | **Not started** — see [plans/2026-05-21-phase-2-clinical-building-blocks.md](plans/2026-05-21-phase-2-clinical-building-blocks.md) |
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
| Canonical JSON | **Landed** | `openehr/serialize/canjson/` REQ-052 | PROBE-030/031 |
| Canonical XML | **Landed** | `openehr/serialize/canxml/` REQ-056 | PROBE-033/034; traceability indexed |
| FLAT / STRUCTURED | **Planned** | `openehr/serialize/` REQ-053 | Parent package is placeholder |
| OPT parser (ADL 1.4 `.opt`) | **Planned** | `openehr/template/` | [plan](plans/2026-05-21-template-parser.md); OET out of scope |
| Validation (OPT, AQL, demo) | **Planned** | `openehr/validation/` | [plan](plans/2026-05-21-validation.md) |
| AQL wire models | **Landed** | `openehr/aql/` REQ-055 | Literal AQL + ResultSet; [builders plan](plans/2026-05-21-aql-builders.md) |
| Composition builder | **Planned** | `openehr/composition/` | [plan](plans/2026-05-21-composition-builder.md) |
| LANG / TERM BMM | **Deferred** | `resources/bmm/` | Reference pins only |
| EHR Extract RM | **Deferred** | — | Skipped per v1 scope |

---

## Auth and transport

| Feature | Status | Package / REQ | Notes |
|---------|--------|---------------|-------|
| `TokenSource` + per-request ctx | **Landed** | `auth/` REQ-060 | |
| Client credentials | **Landed** | `auth/clientcreds/` REQ-068 | |
| JWT Bearer | **Landed** | `auth/jwtbearer/` REQ-068 | |
| HTTP Basic on openEHR REST | **Landed** | `auth/basic/` REQ-069 | |
| Caller attribution | **Landed** | `transport/` REQ-066 | |
| SMART PKCE + launch | **Partial** | `auth/smart/` REQ-061–063 | PKCE, code exchange, refresh, JWKS cache; wire 401 re-auth open |
| Application launch context | **Landed** | `smart/` REQ-064, REQ-067 | LaunchContext, ID-token validation, principal claims |
| JWKS rotation | **Landed** | `auth/smart/` REQ-062 | Cache + refresh-on-miss |
| Token refresh (SMART provider) | **Partial** | `auth/smart/` REQ-063 | Proactive refresh on `TokenSource`; transport 401 → refresh not wired |
| Transport (HTTP, retry, OTel, errors) | **Landed** | `transport/` REQ-090–094 | |
| Transport `NoRetry` / `Disabled` | **Landed** | `transport/` REQ-096 | Bench-friendly retry opt-out |
| Transport observer hook | **Landed** | `transport/` REQ-098 | `WithObserver` + `WithObservationTag` |
| Service discovery | **Landed** | `smart/discovery/` REQ-070–072 | |

---

## REST clients (`openehr/client/*`)

| API area | Status | Package | Notes |
|----------|--------|---------|-------|
| System | **Landed** | `openehr/client/system/` | Capabilities, version |
| EHR (create, get, delete) | **Landed** | `openehr/client/ehr/` | |
| EHR_STATUS | **Landed** | `openehr/client/ehr/ehrstatus/` | |
| Composition CRUD | **Landed** | `openehr/client/ehr/composition/` | REQ-054 If-Match |
| Directory | **Landed** | `openehr/client/ehr/directory/` | |
| Contribution | **Landed** | `openehr/client/ehr/contribution/` | |
| ItemTags | **Landed** | `openehr/client/ehr/itemtags/` | REQ-059; header codec + composition/ehrstatus/directory GET, composition PUT |
| Query (AQL execute) | **Landed** | `openehr/client/query/` | Ad-hoc + stored execute; REQ-055 |
| Definition — ADL 1.4 templates | **Landed** | `openehr/client/definition/` | Upload/list/get/delete, example composition |
| Definition — stored AQL | **Landed** | `openehr/client/definition/` | Put/get/list/delete; REQ-057 |
| Definition — ADL 2 | **Planned** | — | Deferred in package docs |
| Demographic | **Planned** | `openehr/client/demographic/` | `doc.go` only |
| Admin (ITS-REST) | **Landed** | `openehr/client/admin/` | `DeleteEHR`, `DeleteAllEHRs`, `PurgeTemplates` (REQ-099) |

REST delivery detail: [2026-05-15-rest-api-client.md](plans/2026-05-15-rest-api-client.md) (plan table may lag — this roadmap reflects the tree).

---

## Application SMART and Cadasto

| Feature | Status | Package | Notes |
|---------|--------|---------|-------|
| Discovery resolver + cache | **Landed** | `smart/discovery/` | |
| AppContext / launch helpers | **Partial** | `smart/` | LaunchContext + ID-token validation (REQ-064/067); App Registration open (STRAND-05) |
| Cadasto Extra API | **Planned** | `cadasto/extra/` | |
| Datamap V2 | **Planned** | `cadasto/datamap/` REQ-058 | |
| MPI preview | **Planned** | `cadasto/mpi/` | |
| Cadasto admin | **Planned** | `cadasto/admin/` | Distinct from ITS Admin client |
| Care aggregates | **Planned** | `cadasto/care/` | |

---

## Test infrastructure and conformance

| Feature | Status | Location | Notes |
|---------|--------|----------|-------|
| Serialize probes | **Landed** | `testkit/probes/serialize/` | PROBE-030/031, 033/034 |
| Versioned-write probes | **Landed** | `testkit/probes/versioned/` | PROBE-010–013 |
| Definition probe | **Landed** | `testkit/probes/definition/` | |
| Discovery probes | **Landed** | `testkit/probes/discovery/` | PROBE-040/041 |
| Auth / REST probes | **Planned** | — | PROBE-001–009, 060+ in catalog |
| Sandbox transport | **Planned** | `sandbox/` | `doc.go` only |
| Testkit helpers + probe runner | **Partial** | `testkit/` | Probe packages landed; `sandbox/` cassette runner open (REQ-082) |
| PHP SDK wire parity | **Planned** | — | REQ-080–081 |
| OpenAPI cassettes | **Partial** | `testkit/cassettes/` REQ-095 | Not all surfaces covered |

---

## Tooling and examples

| Feature | Status | Notes |
|---------|--------|-------|
| `make ci` / grouped `make help` | **Landed** | |
| `make spec-check` | **Landed** | Traceability subset only |
| `cmd/bmmgen` / `cmd/bmmdiff` | **Landed** | |
| Worked examples | **Landed** | `cmd/examples/{canonical_json,canxml_roundtrip,ehr_create}` |

---

## How to update this file

1. After landing a feature: flip status here and set `Impl. landed` in [`specs/REQ.md`](../specs/REQ.md); add paths to [`specs/traceability.yaml`](../specs/traceability.yaml).
2. After closing a plan phase: update the plan’s progress table **and** the milestone row above.
3. Do **not** duplicate normative REQ prose here — link to `specs/` instead.
