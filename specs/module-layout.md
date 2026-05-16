# Module layout

**Status:** Draft

Authoritative package taxonomy, dependency rules, and versioning policy for `github.com/cadasto/openehr-sdk-go`. Implements REQ-001 through REQ-014.

## Module identity

The module path is **`github.com/cadasto/openehr-sdk-go`** (REQ-001). The path is all lowercase, matching idiomatic Go module naming and the GitHub organisation login; consumer imports MUST use this exact spelling.

The module is licensed **MIT** (REQ-003) and targets **Go 1.25.x** (REQ-002), tracking the N-1 release line per the Go team's support policy.

## Package taxonomy

The SDK is divided into two top-level layers — **openEHR core** and **Cadasto extras** — plus orthogonal support trees (`auth/`, `transport/`, `smart/`, `sandbox/`, `testkit/`, `cmd/examples/`, `internal/`).

### openEHR core

Generic openEHR primitives. No application-specific healthcare models live here.

| Package | Scope |
|---|---|
| `auth/` | Generic `TokenSource` abstraction and shared OAuth2 primitives (JWKS, discovery, scope builder). |
| `auth/smart/` | SMART-on-openEHR provider — PKCE, authorization-code launch flow, token refresh, JWKS rotation. |
| `auth/clientcreds/` | OAuth2 Client Credentials grant provider. |
| `auth/jwtbearer/` | OAuth2 JWT Bearer (RFC 7523) grant provider. |
| `transport/` | HTTP client wrapper around an injected `*http.Client`. Hosts interceptors, retry/backoff, OTel hooks, error mapping, optional spec-version pinning. Named `transport/` (not `http/`) to avoid collision with `net/http` at consumer call sites. |
| `openehr/` | Namespace marker; exports nothing. |
| `openehr/rm/` | RM types (clinical + demographic) as concrete structs with embedded base types; abstract RM categories as Go interfaces. **Generated** from the pinned `openehr_rm_*.bmm.json` schema (REQ-042). |
| `openehr/rm/typereg/` | Central type registry mapping `_type` discriminator → concrete Go type. **Generated** as part of the RM emission. |
| `openehr/bmm/` | Public BMM loader and in-memory model (`bmm.Schema`, `bmm.Class`, `bmm.Property`, …). Parses P_BMM JSON; resolves `includes`. Importable as a building block (REQ-013, REQ-045). |
| `openehr/serialize/` | Canonical JSON / XML, FLAT, STRUCTURED codecs. |
| `openehr/validation/` | Validation interfaces and implementations: Composition vs OPT, demographic structural validation, AQL syntax / path resolution. |
| `openehr/template/` | OPT / OET parsing, template/package deployment helpers, path utilities. **Consumes** `openehr/aom/` types but does not own them. |
| `openehr/aom/` | Archetype Object Model — the in-memory form of an archetype after parsing ADL. Sibling of `openehr/rm/` (both are top-level openEHR information models). |
| `openehr/aom/aom14/` | AOM 1.4 types (ADL 1.4). **Generated** from `openehr_am_1.4.0.bmm.json` + `openehr_base_1.3.0.bmm.json` (REQ-042). |
| `openehr/aom/aom2/` | AOM 2 types (ADL 2). **Deferred for v1** — BMM file kept in `resources/bmm/`. |
| `openehr/aql/` | AQL builders (struct-builder + verb-functions) and request / result models, independent of an executor. |
| `openehr/composition/` | Generic OPT-driven Composition builder (path-value assignment). Template-specific generated structs do **not** live here — they belong in the consuming project. |
| `openehr/client/` | REST clients grouped per openEHR resource. |
| `openehr/client/system/` | System API — capabilities, version, infrastructure discovery. |
| `openehr/client/ehr/` | EHR API — EHR identity, common sub-resource types (`EhrID`, `VersionedObjectID`, `VersionMetadata`). Sub-leaves below carry the per-resource CRUD. |
| `openehr/client/ehr/composition/` | Composition CRUD (REQ-054 optimistic concurrency). |
| `openehr/client/ehr/contribution/` | Multi-version atomic commits (`openehr-audit-details` envelope per REQ-059). |
| `openehr/client/ehr/directory/` | Folder / Directory CRUD. |
| `openehr/client/ehr/ehrstatus/` | EHR_STATUS read/update. |
| `openehr/client/ehr/itemtags/` | ItemTag operations (REST 1.1.0 new resource — REQ-059). |
| `openehr/client/query/` | Query API — AQL executor (ad-hoc + stored, REQ-055 / REQ-057). |
| `openehr/client/definition/` | Definition API — templates (ADL1.4 + ADL2), stored queries, example generation. |
| `openehr/client/demographic/` | Demographic API — parties, relationships, identities (upstream Status: development). |
| `openehr/client/admin/` | Admin API — EHR physical delete, administrative-lifecycle operations (upstream Status: development). Distinct from `cadasto/admin/` (Cadasto-platform admin). |
| `smart/` | Application-level SMART AppContext (patient, user, encounter, launch parameters) and App Registration helpers. Distinct from `auth/smart` (OAuth2 flow). |
| `smart/discovery/` | Service catalog resolver. |
| `sandbox/` | In-memory and recorded-fixture transports implementing the same client interfaces as the production REST clients. |
| `testkit/` | Test doubles, fluent builders, clock abstraction, JWKS fixture, recorder/replay, conformance-probe runner. Named `testkit/` (not `testing/`) to avoid `testing` package collision. |

### Cadasto extras

Application-specific layer. Shipped in the same module in v1 for adoption convenience; the `cadasto/` subtree is a single cut line for later conditional extraction (STRAND-08).

| Package | Scope |
|---|---|
| `cadasto/extra/` | Cadasto Extra API client. |
| `cadasto/datamap/` | Datamap **V2** client and builder (REQ-058). |
| `cadasto/care/` | Application aggregates over EHR + Demographic: Patient, User, CaseLoad, CareTeam, Episode. |
| `cadasto/mpi/` | Minimal MPI search (preview surface). |
| `cadasto/admin/` | Tenant, env, system info, healthcheck. |

### Support trees

| Package | Scope |
|---|---|
| `cmd/examples/` | Worked example programs for each named use case. |
| `cmd/bmmgen/` | CLI entry point for the BMM-driven code generator (REQ-042). |
| `internal/` | Implementation helpers excluded from BC promises (Go convention). |
| `internal/bmmgen/` | BMM code-generator implementation. Reads `resources/bmm/*.bmm.json` via `openehr/bmm/` and emits `openehr/rm/`, `openehr/aom/aom14/`, and the `typereg` registry. Not part of the public API. |
| `resources/` | Pinned SDK assets (BMM schemas under `resources/bmm/`, future XSDs and similar). See [`../resources/README.md`](../resources/README.md) and [`../resources/bmm/README.md`](../resources/bmm/README.md). |
| `docs/` | Narrative documentation (architecture, AI workflow, ADRs, plans). |
| `specs/` | Normative specifications — this tree. |

## Dependency direction

Imports between SDK packages **MUST** flow strictly downward through the graph below (REQ-014). Upward or cyclic imports are prohibited.

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
    ├─ (building-block use, no transport) ──→ openehr/rm/
    ├─ (building-block use, no transport) ──→ openehr/serialize/
    ├─ (building-block use, no transport) ──→ openehr/validation/   ──→ openehr/rm/  openehr/template/
    └─ (building-block use, no transport) ──→ openehr/template/

cadasto/care      ──→ openehr/client/*
cadasto/{extra, datamap, mpi, admin} ──→ transport/

sandbox/  -. implements .-→ openehr/client/*   cadasto/*
testkit/  -. helpers for .-→ all of the above
```

**Key invariants:**

- `transport/` depends on `auth/`, never the reverse.
- `openehr/client/*` depends on `transport/`, `openehr/rm/`, `openehr/serialize/`, never on `cadasto/…`.
- `cadasto/<X>` may depend on `openehr/client/*`, `transport/`, `openehr/rm/`, etc. — but never on another `cadasto/<Y>`.
- `openehr/validation/` MUST NOT take on `openehr/serialize/`'s codec dependencies — validation is structural over the in-memory RM, not over the wire bytes.
- `openehr/bmm/` MUST NOT depend on `transport/`, `auth/`, or any HTTP package — it is a building block (REQ-045).
- `internal/bmmgen` depends on `openehr/bmm/` and the standard `text/template` / `go/format` packages — no SDK runtime packages.

## Boundary rules

Five load-bearing rules. A violation forfeits future options that the cut lines preserve.

1. **No upward imports into `cadasto/`** (REQ-010). Nothing under `openehr/`, `auth/`, `smart/`, `transport/`, `sandbox/`, or `testkit/` **MAY** import from `cadasto/…`. The `cadasto/` subtree is the single cut line for an optional later extraction (STRAND-08).

2. **No sideways imports inside `cadasto/`** (REQ-011). No `cadasto/<name>` package **MAY** import another `cadasto/<other>` directly. Shared types live in openEHR-core packages, or are expressed as interfaces consumed by both.

3. **Layered `auth/`** (REQ-012). `auth/` exposes only the generic `TokenSource` abstraction and shared OAuth2 primitives. SMART-on-openEHR, Client Credentials, JWT Bearer, and any future provider live in sub-packages and are not re-exported from `auth/`.

4. **Building-block independence** (REQ-013). Each of `openehr/rm`, `openehr/serialize`, `openehr/validation`, `openehr/template`, `openehr/aql` (models only) **MUST** be importable and useful without instantiating `transport/`, `auth/`, or any HTTP client.

5. **Service discovery is first-class** (REQ-070). Constructors **MUST** take a `smart/discovery.ServiceCatalog`, not a single base URL.

## The `internal/` boundary

Anything under `internal/` is **outside** the public API surface (REQ-005). Per Go convention, external consumers cannot import it; the SDK **MAY** rename, restructure, or delete `internal/` packages between any two patch releases.

When adding to `internal/`:

- Document the rationale in [docs/architecture.md](../docs/architecture.md) — "why is this not on the public surface?".
- Prefer placing helpers in the package that needs them, not in `internal/`, unless reuse across ≥2 packages or a need for cross-package encapsulation justifies the move.
- `internal/` **MUST NOT** be used as a dumping ground for "I don't want to commit to this name yet". If a package is real, name it and place it; if not, do not export it yet.

## Versioning

The SDK follows **Semantic Versioning 2.0.0** (REQ-004). The mapping of changes → bump:

| Change | Bump |
|---|---|
| Breaking change to any public type, function, or method (anywhere except `internal/`) | major |
| Deprecation of a public symbol (still works; warns) | minor |
| New public symbol, new package, new spec REQ in `Stable` status | minor |
| Bug fix that preserves all public contracts | patch |
| Change confined to `internal/` | patch |
| Change to `Draft`-status specs | patch — but flag in CHANGELOG |
| Spec `Status:` transition `Draft` → `Stable` | minor |
| Spec `Status:` transition `Stable` → `Deprecated` | minor |
| Spec deletion (removing a `Deprecated` spec after a documented cycle) | major |

`v0.x` is in motion until the openEHR-core surface and conformance probe set stabilise. `v1.0.0` lands when:

- All REQs in this catalog are `Status: Stable`.
- The probe set in `conformance.md` reaches the parity bar with the PHP SDK.
- A reference Cadasto deployment passes the probe set.

`v2`+ would live under `…/v2/` per Go's semantic-import-versioning convention. Major-version bumps are deliberate, not accidental — a `v0.x → v1.0.0` doc-only relicense or a missing import-path bump is a release defect.

## Module path stability

The module path (`github.com/cadasto/openehr-sdk-go`) is locked. Renaming the module path requires (a) a major version bump and (b) a deprecation cycle of at least one minor release. There is no scenario in which a patch release changes the module path.
