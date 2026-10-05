---
kind: specification
---

# Module layout

**Status:** Draft

Authoritative package taxonomy, dependency rules, and versioning policy for `github.com/cadasto/openehr-sdk-go`. Implements REQ-010 through REQ-014, REQ-058 and REQ-099.

## Module identity

The module path is **`github.com/cadasto/openehr-sdk-go`**, all lowercase; [packaging.md § REQ-001](packaging.md#req-001--module-path) owns its spelling rule.

The module is licensed **MIT** (REQ-003) and targets **Go 1.27.x** (REQ-002), tracking the current stable Go release line (N) as a deliberate project policy.

## Package taxonomy

The SDK is divided into two top-level layers — **openEHR core** and **Cadasto extras** — plus orthogonal support trees (`auth/`, `transport/`, `smart/`, `sandbox/`, `testkit/`, `cmd/examples/`, `internal/`).

### openEHR core

Generic openEHR primitives. No application-specific healthcare models live here.

| Package | Scope |
|---|---|
| `auth/` | Generic `TokenSource` abstraction and shared OAuth2 primitives (JWKS, discovery, scope builder). |
| `auth/smart/` | SMART-on-openEHR provider — PKCE, authorization-code launch flow, token refresh, JWKS rotation, ID-token verification. |
| `auth/clientcreds/` | OAuth2 Client Credentials grant provider. |
| `auth/jwtbearer/` | OAuth2 JWT Bearer (RFC 7523) grant provider. |
| `auth/basic/` | HTTP Basic (RFC 7617) credentials for openEHR REST (REQ-069). |
| `transport/` | HTTP client wrapper around an injected `*http.Client`. Hosts interceptors, retry/backoff, OTel hooks, error mapping, optional spec-version pinning. Named `transport/` (not `http/`) to avoid collision with `net/http` at consumer call sites. |
| `openehr/` | Namespace marker; exports nothing. |
| `openehr/rm/` | RM types (clinical + demographic) as concrete structs with embedded base types; abstract RM categories as Go interfaces. **Generated** from the pinned `openehr_rm_*.bmm.json` schema (REQ-042). |
| `openehr/rm/typereg/` | Central type registry mapping `_type` discriminator → concrete Go type. **Generated** as part of the RM emission. |
| `openehr/bmm/` | Public BMM loader and in-memory model (`bmm.Schema`, `bmm.Class`, `bmm.Property`, …). Parses P_BMM JSON; resolves `includes`. Importable as a building block (REQ-013, REQ-045). |
| `openehr/terminology/` | The openEHR Terminology's `openehr` groups and code sets as compiled-in, closed tables — **generated** from the pinned `resources/terminology/openehr_terminology.xml` by `cmd/termgen` (REQ-034). Stdlib-only; sits below `openehr/rm` and joins the REQ-013 set. |
| `openehr/serialize/` | Canonical JSON / XML, FLAT, STRUCTURED codecs. |
| `openehr/validation/` | Validation interfaces and implementations: Composition vs OPT, demographic structural validation, AQL syntax / path resolution. |
| `openehr/template/` | ADL 1.4 operational template (OPT: `.opt` / `OPERATIONAL_TEMPLATE`) parse and path utilities. **Consumes** `openehr/aom/` types but does not own them. OET (`.oet`) is out of scope for v1. |
| `openehr/templatecompile/` | Public compiled-template bridge (REQ-111): `Compile(opt)` turns a parsed OPT into the `Compiled` form the composition builder, instance synthesiser, validator, and AQL lint accept. Thin alias re-export over `internal/templatecompile`; sibling of `openehr/template/` because it needs `openehr/rm/rminfo` ([ADR 0010](../adr/0010-public-compiled-template-bridge.md)). |
| `openehr/instance/` | Template-driven RM instance example generator (REQ-107). |
| `openehr/template/webtemplate/` | WebTemplate JSON export of a compiled template (REQ-106). |
| `openehr/aom/` | Archetype Object Model — the in-memory form of an archetype after parsing ADL. Sibling of `openehr/rm/` (both are top-level openEHR information models). |
| `openehr/aom/aom14/` | AOM 1.4 types (ADL 1.4). **Generated** from `openehr_am_1.4.0.bmm.json` + `openehr_base_1.3.0.bmm.json` (REQ-042). |
| `openehr/aom/aom2/` | AOM 2 types (ADL 2). **Deferred for v1** — BMM file kept in `resources/bmm/`. |
| `openehr/aql/` | AQL builders (struct-builder + verb-functions) and request / result models, independent of an executor, plus the REQ-162 opt-in containment verification over the builder's own algebra — sharing `openehr/aql/internal/semcheck` with `openehr/aql/lint` so read and write cannot drift — and the REQ-163 write-side version-predicate, standing-predicate and typed-projection carriers; `Build()` verifies its emitted `SELECT` after emission. |
| `openehr/aql/parse/` | AQL syntax parser against the SDK grammar profile (ADR 0007) → generated-type-free AST (REQ-109). The generated ANTLR parser lives under `openehr/aql/parse/gen/`; the pure-Go ANTLR runtime is its only third-party dependency. |
| `openehr/aql/lint/` | Three-layer AQL static lint (syntax, shape, template-aware archetype/path checks) over `parse` (REQ-109), plus the REQ-161 semantic and portability checks over `openehr/aql/contain`'s containment relation and the always-on, Warning-only REQ-164 path-shape and paging group. Bridged into the validation model by `validation.ValidateAQL`; never imports `openehr/validation`. |
| `openehr/aql/contain/` | Containment admissibility relation (REQ-160): verdicts derived at runtime from the pinned BMM via `openehr/rm/rminfo` plus cited overlay edges; REQ-164 added the `Unavoidable` route-forcing query — is every route from one class to another forced through a third — consumed by `aql_contains_redundant_step`. Sits below `openehr/aql` and `openehr/aql/lint`; imports only `openehr/rm`, `openehr/rm/rminfo`, and the standard library. |
| `openehr/composition/` | Generic OPT-driven Composition builder (path-value assignment). Template-specific generated structs do **not** live here — they belong in the consuming project. |
| `openehr/client/` | REST clients grouped per openEHR resource. |
| `openehr/client/system/` | System API — capabilities, version, infrastructure discovery. |
| `openehr/client/ehr/` | EHR API — EHR identity, common sub-resource types (`EhrID`, `VersionedObjectID`, `VersionMetadata`). Sub-leaves below carry the per-resource CRUD. |
| `openehr/client/ehr/composition/` | Composition CRUD (REQ-054 optimistic concurrency). |
| `openehr/client/ehr/contribution/` | Multi-version atomic commits. Submission body is the ITS-REST `Contribution_create` shape (inline `ORIGINAL_VERSION`/`IMPORTED_VERSION` with `data: T`); the audit envelope is carried in the JSON body (REQ-059) — unlike single-resource writes there is no `openehr-audit-details` header. Response decodes as persisted `rm.Contribution`. PROBE-072 (REQ-050/095). |
| `openehr/client/ehr/directory/` | Folder / Directory CRUD. |
| `openehr/client/ehr/ehrstatus/` | EHR_STATUS read/update. |
| `openehr/client/ehr/itemtags/` | ItemTag operations (REST 1.1.0 new resource — REQ-059). |
| `openehr/client/query/` | Query API — AQL executor (ad-hoc + stored, REQ-055 / REQ-057). |
| `openehr/client/definition/` | Definition API — templates (ADL1.4 + ADL2), stored queries, example generation. |
| `openehr/client/demographic/` | Demographic API — parties, relationships, identities (upstream Status: development). |
| `openehr/client/admin/` | Admin API — EHR physical delete, administrative-lifecycle operations (upstream Status: development). Distinct from `cadasto/admin/` (Cadasto-platform admin). |
| `smart/` | Application-level SMART AppContext (patient, user, encounter, launch parameters) and App Registration helpers. Distinct from `auth/smart` (OAuth2 flow). |
| `smart/discovery/` | Service catalog resolver. |
| `sandbox/` | In-memory openEHR backend ([`sandbox.Backend`](../../sandbox/doc.go)) that implements `http.RoundTripper` for REQ-082 Sandbox mode. No network listener, no credentials, no `transport/` or `auth/` import (REQ-082; `TestNoListenerImports` pins it). It serves the EHR resource built in, and any other route through scripted handlers. |
| `testkit/` | Conformance probes (`testkit/probes/`), the shared result type and catalog runner (`testkit/probe/`, REQ-082), vendored fixtures, Cassette HAR recordings, and fixture-path resolution. Vendored fixture documents under `testkit/corpus/` (`templates/`, `compositions/`, `rm/`, `its_rest/`); Cassette-mode HAR 1.2 recordings under `testkit/recordings/` (ADR 0020; replay via `probe.Replayer`); path resolution in `testkit/fixtures/`; corpus-scale parity harnesses under `testkit/conformance/` (a probe's shared runner plus its counted-exclusion census — e.g. `webtemplate/` for PROBE-086). Named `testkit/` (not `testing/`) to avoid `testing` package collision. |

### Cadasto extras

Application-specific layer. Shipped in the same module in v1 for adoption convenience; the `cadasto/` subtree is a single cut line for later conditional extraction (STRAND-08).

| Package | Scope |
|---|---|
| `cadasto/extra/` | Cadasto Extra API client. |
| `cadasto/datamap/` | Datamap **V2** codec: payload to and from canonical JSON against an OPT, with skeleton, schema and validation (REQ-058). |
| `cadasto/care/` | Application aggregates over EHR + Demographic: Patient, User, CaseLoad, CareTeam, Episode. |
| `cadasto/mpi/` | Minimal MPI search (preview surface). |
| `cadasto/admin/` | Tenant, env, system info, healthcheck (health-probe contract: REQ-083). |

### Support trees

| Package | Scope |
|---|---|
| `cmd/examples/` | Worked example programs for each named use case. |
| `cmd/bmmgen/` | CLI entry point for the BMM-driven code generator (REQ-042). |
| `internal/` | Implementation helpers excluded from BC promises (Go convention). |
| `internal/bmmgen/` | BMM code-generator implementation. Reads `resources/bmm/*.bmm.json` via `openehr/bmm/` and emits `openehr/rm/`, `openehr/aom/aom14/`, and the `typereg` registry. Not part of the public API. |
| `cmd/termgen/` | CLI entry point for the openEHR terminology code generator (REQ-034): `-resources ./resources/terminology -out . [-verify]`. Driven by `make termgen` / `make termgen-verify`. |
| `internal/termgen/` | Terminology code-generator implementation. Parses the pinned `resources/terminology/openehr_terminology.xml` and renders `openehr/terminology/openehr_gen.go`. Go-internal, consumed only by `cmd/termgen/`. |
| `resources/` | Pinned SDK assets (BMM schemas under `resources/bmm/`, the openEHR Terminology under `resources/terminology/`, future XSDs and similar). See [`../resources/README.md`](../../resources/README.md), [`../resources/bmm/README.md`](../../resources/bmm/README.md) and [`../resources/terminology/README.md`](../../resources/terminology/README.md). |
| `docs/` | Narrative documentation (architecture, AI workflow, ADRs, plans). |
| `docs/specifications/` | Normative specifications — this tree. |

## Dependency direction

This is the graph [REQ-014](#req-014--dependency-direction) holds imports to: they flow strictly downward through it, never upward or in a cycle.

```
Application code (cmd/examples, downstream consumers)
    ├──→ cadasto/{care, extra, datamap, mpi, admin}
    ├──→ smart/                     (AppContext, discovery)
    ├──→ openehr/composition/
    ├──→ openehr/aql/
    ├──→ openehr/client/*
    │       └──→ transport/         openehr/rm/         openehr/serialize/
    │              └──→ auth/        └──→ openehr/rm/typereg/
    │                     └──→ net/http.Client (injected)
    │
    ├─ (building-block use, no transport) ──→ openehr/serialize/simplified/  ──→ openehr/rm/
    ├─ (building-block use, no transport) ──→ openehr/rm/  ──→ openehr/serialize/canxml/  (generated XML marshal code)
    ├─ (building-block use, no transport) ──→ openehr/serialize/{canjson,canxml}/  ──→ openehr/rm/typereg/   (neither imports openehr/rm)
    ├─ (building-block use, no transport) ──→ openehr/validation/   ──→ openehr/rm/  openehr/template/
    └─ (building-block use, no transport) ──→ openehr/template/

openehr/{serialize, instance, client/*} ──→ openehr/terminology/   (stdlib-only; sits below openehr/rm, which may import it later — REQ-034)

cadasto/care      ──→ openehr/client/*
cadasto/{extra, mpi, admin} ──→ transport/
cadasto/datamap   ──→ openehr/template/   (no transport/ or auth/: REQ-058)

sandbox/  -. implements .-→ openehr/client/*   cadasto/*
testkit/  -. helpers for .-→ all of the above
```

**Key invariants:**

- `transport/` depends on `auth/`, never the reverse.
- `openehr/client/*` depends on `transport/`, `openehr/rm/`, `openehr/serialize/`, never on `cadasto/…`.
- `openehr/rm/`'s generated marshal files import `openehr/serialize/canxml/`. That edge does not cycle: `canxml` imports only `openehr/rm/typereg/` and `openehr/serialize/internal/poly` from this module.
- `cadasto/<X>` may depend on `openehr/client/*`, `transport/`, `openehr/rm/`, etc. — but never on another `cadasto/<Y>`. `cadasto/datamap` is the exception on the wire side: REQ-058 keeps it off `transport/` and `auth/`.
- `openehr/bmm/` depends on no `transport/`, `auth/` or HTTP package; the rule is [REQ-045](bmm-conformance.md#req-045--bmm-loader-is-a-building-block)'s.
- `internal/bmmgen` depends on `openehr/bmm/` and the standard `text/template` / `go/format` packages — no SDK runtime packages.
- `openehr/terminology/` is stdlib-only — the rule is REQ-034's, enforced by `TestTerminologyForbiddenImports`; it sits *below* `openehr/rm`, so `openehr/rm` may import it later without a cycle.
- `internal/termgen` is a generator tool consumed only by `cmd/termgen` at build time — no library package imports it.

## REQ-010 — `cadasto/` cut line

No package outside `cadasto/…` **MAY** import from `cadasto/…`. The packages allowed to import `cadasto/<X>` are: the consuming application (in `cmd/` and downstream repos) and other `cadasto/<X>` packages **only via** openEHR-core types or interface contracts (see REQ-011).

Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` **MAY** import from `cadasto/…`. The `cadasto/` subtree is the single cut line for an optional later extraction (STRAND-08).

## REQ-011 — No sideways imports inside `cadasto/`

No `cadasto/<X>` package **MAY** import another `cadasto/<Y>` package directly. Shared types **MUST** live in openEHR-core packages (under `openehr/…`, `auth/…`, `smart/…`, or `transport/…`), or be expressed as interfaces consumed by both.

## REQ-012 — Auth layering

`auth/` **MUST** define a generic `TokenSource` abstraction. Provider-specific implementations (`auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic`, …) **MUST** be sub-packages and **MUST NOT** appear in the public surface of `auth/` itself.

## REQ-013 — Building-block independence

Each of `openehr/rm`, `openehr/bmm`, `openehr/serialize` and its sub-packages, `openehr/validation`, `openehr/instance`, `openehr/composition`, `openehr/template`, `openehr/templatecompile`, `openehr/template/webtemplate`, `openehr/terminology`, `openehr/aql`, and the AQL building blocks `openehr/aql/parse`, `openehr/aql/lint`, `openehr/aql/contain` and `openehr/aql/internal/semcheck` **MUST** be importable and useful without constructing an authenticated client or instantiating `transport/` or `auth/`.

Each of those blocks, and every package of this module it imports directly or indirectly, **MUST NOT** import `transport/`, `auth/` or `openehr/client/*`.

The template-side building blocks `openehr/validation` (with `openehr/validation/rmread`), `openehr/instance`, `openehr/composition`, `openehr/templatecompile` and `openehr/template/webtemplate` **MUST NOT** import `openehr/serialize/` in their own files: a caller that wants wire bytes imports a codec itself. The rule is on their own files because `openehr/rm`'s generated marshal code imports `openehr/serialize/canxml`, so the closure of every block that uses `openehr/rm` reaches it.

A package with a narrower import rule states it in its own section: `openehr/bmm` in [REQ-045](bmm-conformance.md#req-045--bmm-loader-is-a-building-block), `openehr/rm/rminfo` in [REQ-048](bmm-conformance.md#req-048--rm-meta-model-introspection-surface), `openehr/terminology` in [REQ-034](rm-modeling.md#openehr-terminology-vocabulary-req-034), `openehr/template` in [REQ-100](clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths), `openehr/instance` in [REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), `openehr/aql/parse` and `openehr/aql/lint` in [REQ-109](clinical-modeling.md#req-109--aql-static-lint), `openehr/aql/contain` in [REQ-160](clinical-modeling.md#req-160--aql-containment-admissibility-relation), and `openehr/aql` and `openehr/aql/internal/semcheck` in [REQ-162](clinical-modeling.md#req-162--builder-containment-verification).

- **Enforced by:** `openehr/rm` → `TestRMForbiddenImports`; `openehr/bmm` → `TestBMMForbiddenImports`; `openehr/serialize` → `TestSerializeForbiddenImports`; `openehr/serialize/canjson` → `TestCanJSONForbiddenImports`; `openehr/serialize/canxml` → `TestCanXMLForbiddenImports`; `openehr/serialize/simplified` → `TestBuildingBlockIndependence`; `openehr/validation` and `openehr/validation/rmread` → `TestValidationForbiddenImports`; `openehr/instance` → `TestInstanceForbiddenImports`; `openehr/composition` → `TestCompositionForbiddenImports`; `openehr/template` → `TestTemplateForbiddenImports`; `openehr/templatecompile` → `TestTemplatecompileForbiddenImports`; `openehr/template/webtemplate` → `TestWebtemplateForbiddenImports`; `openehr/terminology` → `TestTerminologyForbiddenImports`; `openehr/aql` → `TestAQLForbiddenImports`; `openehr/aql/parse` → `TestAQLParseForbiddenImports`; `openehr/aql/lint` → `TestAQLLintForbiddenImports`; `openehr/aql/contain` → `TestContainForbiddenImports`; `openehr/aql/internal/semcheck` → `TestSemcheckForbiddenImports`. Each check walks the block's in-module import closure for the closure rules and reads the block's own imports for the own-files rules, through the shared helper `internal/importguard`, whose own tests are the can-fail control. A non-test file of the package counts when some build could compile it, whatever its build tags, except `//go:build ignore`.

See [use-cases.md § Building-block use cases](use-cases.md#building-block-use-cases).

## REQ-014 — Dependency direction

Imports between SDK packages **MUST** flow strictly downward through the dependency graph in [§ Dependency direction](#dependency-direction). Upward or cyclic imports are prohibited.

## REQ-058 — Datamap V2

`cadasto/datamap/` **MUST** implement version 2 of Datamap, the Cadasto payload format for reading and writing clinical and demographic data without building Reference Model instances ([glossary.md](glossary.md)). Earlier Datamap versions are out of scope. The format (its keys, value shapes and coded-value forms) is the Cadasto platform's, and [REQ-083](conformance.md#req-083--cadasto-platform-api-conformance) names the authority the package conforms to.

The package is a codec between Datamap V2 and openEHR canonical JSON, driven by the operational template that governs the data. It has two profiles, chosen by the template's root type:

- **Composition profile:** a template rooted on `COMPOSITION`.
- **Party profile:** a template rooted on `PERSON`, `ORGANISATION`, `GROUP`, `AGENT` or `ROLE`, or on one of the archetypeable demographic components `ADDRESS`, `CONTACT`, `PARTY_IDENTITY` or `PARTY_RELATIONSHIP`.

Every other root is out of scope, including `EHR_STATUS`, which the format also covers.

- For each profile the package **MUST** convert in both directions. A conversion for one profile **MUST** refuse a template of the other, and every operation **MUST** refuse a template outside both profiles.
- A conversion that refuses its template, or meets a value it cannot map to its template node (a value of the wrong shape, a malformed coded value), **MUST** fail with an error and **MUST NOT** return a converted document alongside it.
- Conversion walks the template and takes only the keys it defines: a payload key the template does not define **MUST NOT** reach the converted document. Validation is the operation that refuses such a key.
- For a template of either profile the package **MUST** provide an empty payload skeleton, a JSON Schema of the payload, and a validation of a payload against the template. Validation **MUST** report every key the template does not define, every required key that is missing, and every value that breaks a constraint the template states itself. A payload that validation accepts **MUST** convert without error.
- The canonical JSON a conversion returns **MUST** decode as the template root's RM type and pass template validation ([REQ-110](clinical-modeling.md#req-110--template-driven-validation-beyond-composition)). The Datamap payload a conversion returns **MUST** pass the package's own validation.
- The codec **MUST** be usable without a client, like the building blocks in [REQ-013](#req-013--building-block-independence): it takes the template and the payload as values, performs no network I/O, and imports neither `transport/` nor `auth/`. Its entry points read untrusted input, so the bounds of [REQ-108](clinical-modeling.md#req-108--untrusted-document-bounds) apply to them.

## REQ-099 — ITS-REST Admin client surface

`openehr/client/admin/` **MUST** expose typed package-level functions for the ITS-REST `/admin/*` housekeeping endpoints that deployments commonly ship for test setup/teardown:

- `DeleteEHR(ctx, c, ehrID) error` — admin-mode delete on `DELETE /admin/ehr/{ehr_id}` (404 surfaces as `transport.ErrNotFound`).
- `DeleteAllEHRs(ctx, c, ...) error` — wholesale reset on `DELETE /admin/ehr/all` (the literal `/all` segment per `resources/its-rest/admin-validation.openapi.yaml` line 78), with the optional repeatable `ehr_id` subset query parameter; deployments **MAY** disable it (failures surface as the typed wire error).
- `PurgeTemplates(ctx, c) error` — clear the template registry. This is **not** part of the ITS-REST Admin contract (which defines only `/admin/ehr/{ehr_id}` and `/admin/ehr/all`); it is an EHRbase-specific extension (`DELETE admin/template/all`, `deleteAllTemplates`) and **MUST** be documented as such on its godoc rather than presented as ITS-REST-conformant.

The Admin API is upstream `x-status: DEVELOPMENT`; this client therefore ships as **Draft** and **MAY** change between minor versions.

The package **MUST** mirror the `Repository` pattern used by `openehr/client/ehr/*` so it composes with the dependency-injection seams in REQ-023. The Cadasto admin extras (`cadasto/admin/`) **MUST NOT** be conflated with this surface — they target distinct endpoint families and the module-layout cut line under `cadasto/` (REQ-010, REQ-011) keeps them separated.

Out of scope at v1: bulk operations, async-job admin endpoints, ITS-REST capability negotiation (lives in `openehr/client/system`).

- **Lives in:** [`openehr/client/admin/`](../../openehr/client/admin)
- **Probes:** PROBE-103 (`DELETE /admin/ehr/all` with the repeatable `ehr_id` parameter, Sandbox) at [`testkit/probes/rest/probe_103_admin_bulk_delete.go`](../../testkit/probes/rest/probe_103_admin_bulk_delete.go); unit tests `TestDeleteEHR*`, `TestDeleteAllEHRs`, `TestPurgeTemplates`, `TestRepositoryRoundTrip` in `openehr/client/admin/admin_test.go`

## Boundary rules (summary)

Five load-bearing rules — normative detail in REQ-010 through REQ-014 above and REQ-070 in [service-discovery.md](service-discovery.md). A violation forfeits future options that the cut lines preserve.

1. **No upward imports into `cadasto/`** — REQ-010.
2. **No sideways imports inside `cadasto/`** — REQ-011.
3. **Layered `auth/`** — REQ-012.
4. **Building-block independence** — REQ-013.
5. **Service discovery is first-class** — REQ-070: constructors take a `smart/discovery.ServiceCatalog`, not a single base URL.

## The `internal/` boundary

Anything under `internal/` is **outside** the public API surface (REQ-005). Per Go convention, external consumers cannot import it; the SDK **MAY** rename, restructure, or delete `internal/` packages between any two patch releases.

When adding to `internal/`:

- Document the rationale in [docs/architecture.md](../architecture.md) — "why is this not on the public surface?".
- Prefer placing helpers in the package that needs them, not in `internal/`, unless reuse across ≥2 packages or a need for cross-package encapsulation justifies the move.
- `internal/` **MUST NOT** be used as a dumping ground for "I don't want to commit to this name yet". If a package is real, name it and place it; if not, do not export it yet.

## Versioning

The SDK follows **Semantic Versioning 2.0.0** (REQ-004). A release **MUST** take the highest bump that any of its changes maps to in the table below. For this table, a field added to an exported struct is not a breaking change, and that includes a slice, map, or function field. Callers still see three effects, and none of them changes the bump: positional struct literals no longer compile, which [idiom.md § Public-API stability](idiom.md#public-api-stability) tells consumers not to write; a slice, map, or function field makes the struct incomparable; a new field can clash with a promoted name. While on `v0.x`, a change the table maps to major takes a minor bump instead, and the release notes **MUST** name it.

| Change | Bump |
|---|---|
| Breaking change to any public type, function, or method (anywhere except `internal/`) | major |
| Deprecation of a public symbol (still works; warns) | minor |
| New public symbol, new package, new spec REQ in `Stable` status | minor |
| Bug fix that preserves all public contracts | patch |
| Change confined to `internal/` | patch |
| Change to `Draft`-status specs | patch. The CHANGELOG follows [AGENTS.md § Code style and conventions](../../AGENTS.md#code-style-and-conventions) |
| Spec `Status:` transition `Draft` → `Stable` | minor |
| Spec `Status:` transition `Stable` → `Deprecated` | minor |
| Spec deletion (removing a `Deprecated` spec after a documented cycle) | major |
| BMM bump that adds generated public types or fields | minor |
| BMM bump that removes or changes a generated public type or field | major (a breaking change) |
| BMM bump with no public type change | patch |
| Raise of the `go.mod` minimum Go version (REQ-002) | minor |
| Module path change (REQ-001) | major, after a deprecation cycle ([§ Module path stability](#module-path-stability)) |
| Tightened validation: an input that passed now fails | major (a breaking change) |

`v0.x` is in motion until the openEHR-core surface and conformance probe set stabilise. `v1.0.0` lands when:

- Every REQ in [REQ.md](REQ.md) is `Impl. landed` or `retired` — the per-REQ axis, with `retired` terminal (it satisfies the gate rather than blocking it). `Status:` is a per-**file** promise per [README.md § Status header](README.md#status-header); promoting each spec file `Draft → Stable` is part of the cut, not a precondition for it.
- The openEHR wire-conformance probe suite in `conformance.md` passes.
- A reference openEHR deployment passes the probe suite.

`v2`+ would live under `…/v2/` per Go's semantic-import-versioning convention. Major-version bumps are deliberate, not accidental — a `v0.x → v1.0.0` doc-only relicense or a missing import-path bump is a release defect.

## Module path stability

The module path (`github.com/cadasto/openehr-sdk-go`) is locked. Renaming it is a change the [§ Versioning](#versioning) table maps to major, and the rename **MUST** follow a deprecation cycle of at least one minor release. The `/vN` suffix that [packaging.md § REQ-004](packaging.md#req-004--semantic-versioning) requires for major versions v2 and beyond is Go's semantic import version of this same module path, not a rename. This deprecation cycle does not apply to it. A patch release **MUST NOT** change the module path.
