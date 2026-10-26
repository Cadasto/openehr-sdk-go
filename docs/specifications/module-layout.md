---
kind: specification
---

# Module layout

**Status:** Draft

Authoritative package taxonomy, dependency rules, and versioning policy for `github.com/cadasto/openehr-sdk-go`. Implements REQ-013, REQ-058 and REQ-099; the rules of the retired REQ-010, REQ-011, REQ-012 and REQ-014 are [§ Boundary rules](#boundary-rules).

## Module identity

The module path is **`github.com/cadasto/openehr-sdk-go`**, all lowercase; [packaging.md § REQ-001](packaging.md#req-001--module-path) owns its spelling and its stability.

The module is licensed **MIT** ([licensing.md](../licensing.md)) and targets **Go 1.27.x**, tracking the current stable Go release line (N) as a deliberate project policy ([releases.md § Minimum Go version](../releases.md#minimum-go-version)).

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
| `transport/` | HTTP client wrapper around an injected `*http.Client`. Hosts retry/backoff, OTel hooks, the request observer, error mapping, optional spec-version pinning. Named `transport/` (not `http/`) to avoid collision with `net/http` at consumer call sites. |
| `openehr/` | Namespace marker; exports nothing. |
| `openehr/rm/` | RM types (clinical + demographic) as concrete structs with embedded base types; abstract RM categories as Go interfaces. **Generated** from the pinned `openehr_rm_*.bmm.json` schema (REQ-041). |
| `openehr/rm/typereg/` | Central type registry mapping `_type` discriminator → concrete Go type. **Generated** as part of the RM emission. |
| `openehr/bmm/` | Public BMM loader and in-memory model (`bmm.Schema`, `bmm.Class`, `bmm.Property`, …). Parses P_BMM JSON; resolves `includes`. Importable as a building block (REQ-013, REQ-045). |
| `openehr/terminology/` | The openEHR Terminology's `openehr` groups and code sets, the external code sets the Foundation publishes beside them, and the property-unit table, as compiled-in, closed tables — **generated** from the pinned `resources/terminology/openehr_terminology.xml`, `openehr_external_terminologies.xml` and `PropertyUnitData.xml` by `cmd/termgen` (REQ-034). Stdlib-only; sits below `openehr/rm` and joins the REQ-013 set. |
| `openehr/serialize/` | Canonical JSON / XML, FLAT, STRUCTURED codecs. |
| `openehr/validation/` | Validation interfaces and implementations: Composition vs OPT, demographic structural validation, AQL syntax / path resolution. |
| `openehr/template/` | ADL 1.4 operational template (OPT: `.opt` / `OPERATIONAL_TEMPLATE`) parse and path utilities. **Consumes** `openehr/aom/` types but does not own them. OET (`.oet`) is out of scope for v1. |
| `openehr/templatecompile/` | Public compiled-template bridge (REQ-111): `Compile(opt)` turns a parsed OPT into the `Compiled` form the composition builder, instance synthesiser, validator, and AQL lint accept. Thin alias re-export over `internal/templatecompile`; sibling of `openehr/template/` because it needs `openehr/rm/rminfo` ([ADR 0010](../adr/0010-public-compiled-template-bridge.md)). |
| `openehr/instance/` | Template-driven RM instance example generator (REQ-107). |
| `openehr/template/webtemplate/` | WebTemplate JSON export of a compiled template (REQ-106). |
| `openehr/aom/` | Archetype Object Model — the in-memory form of an archetype after parsing ADL. Sibling of `openehr/rm/` (both are top-level openEHR information models). |
| `openehr/aom/aom14/` | AOM 1.4 types (ADL 1.4). **Generated** from `openehr_am_1.4.0.bmm.json` + `openehr_base_1.3.0.bmm.json` (REQ-041). |
| `openehr/aom/aom2/` | AOM 2 types (ADL 2). **Deferred for v1** — BMM file kept in `resources/bmm/`. |
| `openehr/aql/` | AQL builders (struct-builder + verb-functions) and request / result models, independent of an executor, plus the REQ-161 opt-in containment verification over the builder's own algebra — sharing `openehr/aql/internal/semcheck` with `openehr/aql/lint` so read and write cannot drift — and the REQ-163 write-side version-predicate, standing-predicate and typed-projection carriers; `Build()` verifies its emitted `SELECT` after emission. |
| `openehr/aql/parse/` | AQL syntax parser against the SDK grammar profile (ADR 0007) → generated-type-free AST (REQ-109). The generated ANTLR parser lives under `openehr/aql/parse/gen/`; the pure-Go ANTLR runtime is its only third-party dependency. |
| `openehr/aql/lint/` | Three-layer AQL static lint (syntax, shape, template-aware archetype/path checks) over `parse` (REQ-109), plus the REQ-161 semantic and portability checks over `openehr/aql/contain`'s containment relation and the always-on, Warning-only REQ-164 path-shape and paging group. Bridged into the validation model by `validation.ValidateAQL`; never imports `openehr/validation`. |
| `openehr/aql/contain/` | Containment admissibility relation (REQ-160): verdicts derived at runtime from the pinned BMM via `openehr/rm/rminfo` plus cited overlay edges, and the `Unavoidable` route-forcing query (REQ-164) — is every route from one class to another forced through a third — consumed by `aql_contains_redundant_step`. Sits below `openehr/aql` and `openehr/aql/lint`; imports only `openehr/rm`, `openehr/rm/rminfo`, and the standard library. |
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
| `sandbox/` | In-memory openEHR backend (`sandbox.Backend`) that implements `http.RoundTripper` for REQ-082 Sandbox mode. No network listener, no credentials, no `transport/` or `auth/` import (REQ-082). It serves the EHR resource built in, and any other route through scripted handlers. |
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
| `cmd/bmmgen/` | CLI entry point for the BMM-driven code generator (REQ-041). |
| `cmd/cdrcheck/` | Opt-in cross-check of the example generator (REQ-107) and the template validator (REQ-102) against live CDRs ([§ REQ-082 *The CDR cross-check tool*](conformance.md#the-cdr-cross-check-tool)), driven by `make cdr-crosscheck`. Reads a directory of OPTs and the CDRs from the environment, and writes its output outside the repository. Not part of `make ci`. |
| `internal/` | Implementation helpers excluded from BC promises (Go convention). |
| `internal/bmmgen/` | BMM code-generator implementation. Reads `resources/bmm/*.bmm.json` via `openehr/bmm/` and emits `openehr/rm/`, `openehr/aom/aom14/`, and the `typereg` registry. Not part of the public API. |
| `internal/noredirect/` | The HTTP client for requests that carry a credential: a copy of the caller's client that returns a 3xx answer instead of following it (REQ-060). Shared by `auth/smart`, `auth/clientcreds` and `auth/jwtbearer`; not part of the public API. |
| `cmd/termgen/` | CLI entry point for the openEHR terminology code generator (REQ-034): `-resources ./resources/terminology -out . [-verify]`. Driven by `make termgen` / `make termgen-check`. |
| `internal/termgen/` | Terminology code-generator implementation. Parses the pinned `resources/terminology/openehr_terminology.xml`, `openehr_external_terminologies.xml` and `PropertyUnitData.xml` and renders `openehr/terminology/openehr_gen.go`. Go-internal, consumed only by `cmd/termgen/`. |
| `resources/` | Pinned SDK assets (BMM schemas under `resources/bmm/`, the openEHR Terminology under `resources/terminology/`, future XSDs and similar). See [`../resources/README.md`](../../resources/README.md), [`../resources/bmm/README.md`](../../resources/bmm/README.md) and [`../resources/terminology/README.md`](../../resources/terminology/README.md). |
| `docs/` | Narrative documentation (architecture, AI workflow, ADRs, plans). |
| `docs/specifications/` | Normative specifications — this tree. |

## Dependency direction

Imports between the SDK's packages flow strictly downward through this graph, never upward or in a cycle ([§ Boundary rules](#boundary-rules)).

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

openehr/{serialize, instance, client/*, template, validation} ──→ openehr/terminology/   (stdlib-only; sits below openehr/rm, which may import it later — REQ-034)
openehr/{rm, rm/typereg, serialize/canjson} ──→ openehr/internal/refused/   (stdlib-only; carries a refused literal to DecodeError.Value)
openehr/{rm/typereg, serialize/canjson} ──→ openehr/template/constraints/   (stdlib + internal/iso8601 only; the Redacted type DecodeError.Value holds)
openehr/client/ehr/composition ──→ openehr/template/webtemplate/   (the Web Template its FLAT and STRUCTURED formats serialise against — REQ-173)

cadasto/care      ──→ openehr/client/*
cadasto/{extra, mpi, admin} ──→ transport/
cadasto/datamap   ──→ openehr/template/   (no transport/ or auth/: REQ-058)

sandbox/  -. implements .-→ openehr/client/*   cadasto/*
testkit/  -. helpers for .-→ all of the above
```

**Key invariants:**

- `transport/` depends on `auth/`, never the reverse.
- `openehr/client/*` depends on `transport/`, `openehr/rm/`, `openehr/serialize/`, never on `cadasto/…`. `openehr/client/ehr/composition` also depends on `openehr/template/webtemplate/`, which its FLAT and STRUCTURED formats serialise against ([wire.md § REQ-173](wire.md#req-173--composition-representation-formats)); the template-side blocks never import a client ([§ REQ-013](#req-013--building-block-independence)), so the edge points down.
- `openehr/rm/`'s generated marshal files import `openehr/serialize/canxml/` and `openehr/internal/xmlunmatched/`. That edge does not cycle: `canxml` imports only `openehr/rm/typereg/`, `openehr/serialize/internal/poly` and `openehr/internal/xmlunmatched/` from this module, and `xmlunmatched` imports only the standard library ([ADR 0002](../adr/0002-bmm-codegen-decisions.md) D9).
- `openehr/rm/typereg/` imports `openehr/template/constraints/`, for the `Redacted` type that `DecodeError.Value` holds ([§ REQ-052 *Value-free decode errors*](wire.md#req-052)), and `openehr/internal/refused/`, which carries a literal an RM primitive codec refused to the codecs' entry points. `openehr/rm/` (its `Real`, `Integer` and `Character` codecs) imports `openehr/internal/refused/` to hand the literal over, and `openehr/serialize/canjson/` imports both packages to read it into `Value`. `refused` imports only the standard library, and `constraints` only the standard library and `internal/iso8601`, which itself imports only the standard library, so none of these edges cycles back into the RM or the codecs. An import-guard test in each package holds its edges.
- `openehr/serialize/canjson/` reaches the RM's attribute tables through `internal/rmjsonwalk/`, which imports `openehr/rm/rminfo/` and `internal/rmjsonwalk/polyreg/`, never `openehr/rm/`; `openehr/rm/`'s generated init registers its interface table into `polyreg`, which imports only the standard library. An import-guard test holds this.
- `cadasto/<X>` may depend on `openehr/client/*`, `transport/`, `openehr/rm/`, etc. — but never on another `cadasto/<Y>`. `cadasto/datamap` is the exception on the wire side: REQ-058 keeps it off `transport/` and `auth/`.
- `openehr/bmm/` depends on no `transport/`, `auth/` or HTTP package; the rule is [REQ-045](bmm-conformance.md#req-045--bmm-loader-is-a-building-block)'s, and its import rule is in the table of [§ REQ-013](#req-013--building-block-independence).
- `internal/bmmgen` depends on `openehr/bmm/` and the standard `text/template` / `go/format` packages — no SDK runtime packages.
- `openehr/terminology/` imports only the standard library (the table of [§ REQ-013](#req-013--building-block-independence)); it sits *below* `openehr/rm`, so `openehr/rm` can import it later without a cycle.
- `internal/termgen` is a generator tool consumed only by `cmd/termgen` at build time — no library package imports it.

## REQ-013 — Building-block independence

Each building block in the table below **MUST** be importable and useful without an authenticated client: no package in its import closure, the packages of this module it imports directly or indirectly, is `transport/`, `auth/` or `openehr/client/*`.

The table is the one home of each block's own import rule beside that closure rule. A closure rule holds for the block and every package of this module it imports, directly or indirectly. An own-files rule holds for the block's own non-test files, whatever their build tags, except a file marked `//go:build ignore`. The template-side blocks and the AQL blocks keep `openehr/serialize/` out of their own files rather than their closure, because `openehr/rm`'s generated marshal code imports `openehr/serialize/canxml`, so the closure of every block that uses `openehr/rm` reaches it; a caller that wants wire bytes imports a codec itself.

| Building block | Its own import rule |
|---|---|
| `openehr/rm` | The closure rule only. |
| `openehr/rm/rminfo` | Its own files import only the standard library, so its compiled-in tables never become a runtime reduction of the BMM ([bmm-conformance.md § REQ-048](bmm-conformance.md#req-048--rm-meta-model-introspection-surface)). |
| `openehr/bmm` | No package in its closure is `net/http` or a package under it. Here the closure runs through this module's packages and the standard library's, so no HTTP machinery arrives through a standard-library package such as `expvar` or `net/rpc`; it stops at a package of another module, the standard library's vendored copies included ([bmm-conformance.md § REQ-045](bmm-conformance.md#req-045--bmm-loader-is-a-building-block)). |
| `openehr/serialize` and its sub-packages | The closure rule only. |
| `openehr/terminology` | It imports only the standard library, so it sits below `openehr/rm` and RM-level consumers can adopt it without a cycle ([rm-modeling.md § REQ-034](rm-modeling.md#openehr-terminology-vocabulary-req-034)). |
| `openehr/template` | No package in its closure is `openehr/rm`, `openehr/aom/aom14` or a package under either: RM class names reach it as strings from the OPT XML, never as Go types ([clinical-modeling.md § REQ-100](clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths)). |
| `openehr/template/constraints` | It imports only the standard library and `internal/iso8601`, which itself imports only the standard library ([clinical-modeling.md § REQ-103](clinical-modeling.md#req-103--primitive-constraint-introspection)). |
| `openehr/templatecompile` | Its own files import no package under `openehr/serialize/`. |
| `openehr/template/webtemplate` | Its own files import no package under `openehr/serialize/`. |
| `openehr/validation` | Its own files import no package under `openehr/serialize/`. Its closure includes the generated attribute access in `internal/rmaccess`. |
| `openehr/composition` | Its own files import no package under `openehr/serialize/`. |
| `openehr/instance` | Its own files import no package under `openehr/serialize/`, and no package in its closure is `openehr/composition`, `openehr/validation` or a package under either: the composition builder calls the generator, and the validator checks the generator's output, which is an independent check only while the generator does not use the validator ([clinical-modeling.md § REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)). |
| `openehr/aql` | Its own files import no package under `openehr/serialize/`, and no package in its closure is `openehr/validation`: the arrow runs from validation to aql. |
| `openehr/aql/internal/semcheck` | Its own files import only `openehr/aql/contain` and the standard library, so the read-side lint and the write-side builder can share it without a cycle. |
| `openehr/aql/parse` | Its own files import no package under `openehr/serialize/`, and its only direct third-party import is the pure-Go ANTLR runtime ([clinical-modeling.md § REQ-109](clinical-modeling.md#req-109--aql-static-lint)). |
| `openehr/aql/lint` | Its own files import no package under `openehr/serialize/`, and no package in its closure is `openehr/validation` ([clinical-modeling.md § REQ-109](clinical-modeling.md#req-109--aql-static-lint)). |
| `openehr/aql/contain` | Its own files import only `openehr/rm`, `openehr/rm/rminfo` and the standard library, so it sits below both `openehr/aql` and `openehr/aql/lint` ([clinical-modeling.md § REQ-160](clinical-modeling.md#req-160--aql-containment-admissibility-relation)). |

The closure and own-files rules of each row are held by that package's import-guard test, which walks the block's in-module import closure for a closure rule and reads its own imports for an own-files rule. The note on `openehr/aql/parse`'s one third-party import describes the dependency; no guard checks it.

See [use-cases.md § Building-block use cases](use-cases.md#building-block-use-cases).

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
- The canonical JSON a conversion returns **MUST** decode as the template root's RM type and pass template validation ([REQ-102](clinical-modeling.md#req-102--composition-validation)). The Datamap payload a conversion returns **MUST** pass the package's own validation.
- The codec **MUST** be usable without a client, like the building blocks in [REQ-013](#req-013--building-block-independence): it takes the template and the payload as values, performs no network I/O, and imports neither `transport/` nor `auth/`. Its entry points read untrusted input, so the bounds of [REQ-108](clinical-modeling.md#req-108--untrusted-document-bounds) apply to them.

## REQ-099 — ITS-REST Admin client surface

`openehr/client/admin/` **MUST** expose typed package-level functions for the ITS-REST `/admin/*` housekeeping endpoints that deployments commonly ship for test setup/teardown:

- `DeleteEHR(ctx, c, ehrID) error` — admin-mode delete on `DELETE /admin/ehr/{ehr_id}` (404 surfaces as `transport.ErrNotFound`).
- `DeleteAllEHRs(ctx, c, ...) error` — wholesale reset on `DELETE /admin/ehr/all` (the literal `/all` segment per `resources/its-rest/admin-validation.openapi.yaml` line 78), with the optional repeatable `ehr_id` subset query parameter; deployments **MAY** disable it (failures surface as the typed wire error).
- `PurgeTemplates(ctx, c) error` — clear the template registry. This is **not** part of the ITS-REST Admin contract (which defines only `/admin/ehr/{ehr_id}` and `/admin/ehr/all`); it is an EHRbase-specific extension (`DELETE admin/template/all`, `deleteAllTemplates`), and its godoc says so rather than presenting it as ITS-REST-conformant.

The Admin API is upstream `x-status: DEVELOPMENT`; this client therefore ships as **Draft** and **MAY** change between minor versions.

The package mirrors the `Repository` pattern of `openehr/client/ehr/*`, so it composes with the same dependency-injection seams ([idiom.md § API conventions](idiom.md#api-conventions)). The Cadasto admin extras (`cadasto/admin/`) are a different surface: they target distinct endpoint families, and the `cadasto/` cut line ([§ Boundary rules](#boundary-rules)) keeps the two apart.

Out of scope at v1: bulk operations, async-job admin endpoints, ITS-REST capability negotiation (lives in `openehr/client/system`).

- **Probes:** [PROBE-103](conformance.md#probe-103--admin-bulk-delete-via-delete-adminehrall) (`DELETE /admin/ehr/all` with the repeatable `ehr_id` parameter, Sandbox)

## Boundary rules

These rules keep the cut lines open: breaking one forfeits an option they preserve, such as extracting `cadasto/` later. Import-guard tests hold them where such a test exists.

1. **The `cadasto/` cut line.** Nothing outside `cadasto/…` imports from it: no package under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/` or `testkit/` does. Only the consuming application, in `cmd/` and downstream repositories, imports a `cadasto/<X>` package; another `cadasto/<Y>` reaches it only through openEHR-core types or interface contracts (rule 2). The `cadasto/` subtree is the single cut line for an optional later extraction (STRAND-08).
2. **No sideways imports inside `cadasto/`.** No `cadasto/<X>` package imports another `cadasto/<Y>` package directly. Types they share live in openEHR-core packages (under `openehr/…`, `auth/…`, `smart/…` or `transport/…`), or are interfaces both consume.
3. **Layered `auth/`.** `auth/` defines the generic `TokenSource` abstraction. Each provider (`auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic`, …) is a sub-package, and none appears in the public surface of `auth/` itself.
4. **Building-block independence** — [REQ-013](#req-013--building-block-independence).
5. **Downward dependency direction.** Imports between SDK packages flow strictly downward through the graph in [§ Dependency direction](#dependency-direction), never upward and never in a cycle.
6. **Service discovery is first-class** — [REQ-070](service-discovery.md#req-070): constructors take a `smart/discovery.ServiceCatalog`, not a single base URL.

## The `internal/` boundary

Anything under `internal/` is **outside** the public API surface ([packaging.md § REQ-005](packaging.md#req-005--internal-boundary)). Per Go convention, external consumers cannot import it, and the SDK can rename, restructure, or delete `internal/` packages between any two patch releases.

When adding to `internal/`:

- Document the rationale in [docs/architecture.md](../architecture.md) — "why is this not on the public surface?".
- Prefer placing helpers in the package that needs them, not in `internal/`, unless reuse across ≥2 packages or a need for cross-package encapsulation justifies the move.
- `internal/` is not a dumping ground for "I don't want to commit to this name yet". If a package is real, name it and place it; if not, do not export it yet.

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
| Raise of the `go.mod` minimum Go version ([releases.md § Minimum Go version](../releases.md#minimum-go-version)) | minor |
| Module path change ([packaging.md § REQ-001](packaging.md#req-001--module-path)) | major, after a deprecation cycle |
| Tightened validation: an input that passed now fails | major (a breaking change) |

`v0.x` is in motion until the openEHR-core surface and conformance probe set stabilise. `v1.0.0` lands when:

- Every REQ in [REQ.md](REQ.md) is `Impl. landed` or `retired` — the per-REQ axis, with `retired` terminal (it satisfies the gate rather than blocking it). `Status:` is a per-**file** promise per [README.md § Status header](README.md#status-header); promoting each spec file `Draft → Stable` is part of the cut, not a precondition for it.
- The openEHR wire-conformance probe suite in `conformance.md` passes.
- A reference openEHR deployment passes the probe suite.

`v2`+ would live under `…/v2/` per Go's semantic-import-versioning convention. Major-version bumps are deliberate, not accidental — a `v0.x → v1.0.0` doc-only relicense or a missing import-path bump is a release defect.
