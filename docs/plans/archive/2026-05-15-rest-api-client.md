# Plan — openEHR REST API client (1.1.0-development)

**Date:** 2026-05-15
**Status:** Archived — Phases 1–6 + 8 landed (the openEHR REST client family). Phase 7 (Demographic) split out to the active plan [`2026-06-14-demographic-rest-client.md`](2026-06-14-demographic-rest-client.md); Phase 9 (benchmark harness) is optional/deferred.
**Owner:** SDK maintainers
**Covers:** REQ-050, REQ-051, REQ-054, REQ-055, REQ-057, REQ-058, **REQ-059 (openEHR custom headers)**, REQ-013, REQ-014, REQ-020, REQ-021, REQ-022, REQ-023, REQ-024, REQ-025, REQ-026, REQ-060..068 (auth integration), REQ-070..072 (discovery integration), REQ-090..092 (observability/retry/TLS), **REQ-093 (error envelope)**, **REQ-094 (`Prefer` negotiation)**, **REQ-095 (OpenAPI authoritative source)**; PROBE-010..013 implement; reserves PROBE-040..049 (REST-binding probes)
**Depends on:** BMM codegen complete ([`2026-05-15-bmm-codegen.md`](2026-05-15-bmm-codegen.md)); canonical JSON codec ([`2026-05-15-canonical-json-serialization.md`](2026-05-15-canonical-json-serialization.md)) Phases 0–3; canonical XML codec ([`2026-05-15-canonical-xml-serialization.md`](2026-05-15-canonical-xml-serialization.md)) optional for v1 (clients negotiate JSON by default)
**Defers:** SMART OAuth2/PKCE flow implementation (covered separately by the SMART plan when it lands); federator policy (separate proposal); FLAT/STRUCTURED simplified-format clients (separate plan); cassette recording infrastructure (testkit work); REQ-094 write-path gaps (`Prefer=identifier`, `representation` + empty body) — **[`2026-05-25-req094-prefer-followups.md`](2026-05-25-req094-prefer-followups.md)** (**landed**)

## Implementation progress (normative detail stays in `docs/specifications/`)

| Plan phase | Outcome | Status |
|---|---|---|
| 1 — `transport/` | HTTP client, headers, retry, OTel, error envelope | **Done** |
| 2 — `openehr/client/system/` | Capabilities, version, health | **Done** |
| 3 — EHR read path | `ehr`, `ehrstatus`, `composition`, `directory` GET | **Done** |
| 4 — EHR versioned writes | PUT/POST/DELETE, `contribution`, PROBE-010–012 | **Done** |
| 5 — Query API | `openehr/client/query/` | **Done** |
| 6 — Definition API | templates, stored queries | **Done** (ADL 1.4 templates + stored AQL CRUD; PROBE-067) |
| 7 — Demographic API | **split out** → [`2026-06-14-demographic-rest-client.md`](2026-06-14-demographic-rest-client.md) | Open |
| 8 — Admin API | `openehr/client/admin/` ITS-REST housekeeping | **Done** (REQ-099; PROBE-070) |
| 9 — Benchmark harness | load/benchmark consumer over the client | Open |

## Goal

Implement the typed openEHR REST 1.1.0-development client family under `openehr/client/{system,ehr,query,definition,demographic,admin}/`, layered on `transport/`, `auth/`, `smart/discovery/`, `openehr/rm/`, and `openehr/serialize/canjson/`. The client is the wire-binding of the openEHR Platform Service Model — it MUST be the surface every downstream consumer (benchmark, seeder, MCP server, federator) uses to reach a Cadasto CDR or any conformant openEHR backend.

Consumers import the leaves they need (REQ-013) — e.g. `openehr/client/ehr/composition` for composition CRUD without pulling in AQL or Definition.

## Integration with existing stack

| Piece | Location | Role for this plan |
|---|---|---|
| Generated RM types | `openehr/rm/*_gen.go` | Request and response payload types |
| Canonical JSON codec | `openehr/serialize/canjson/` | Default request body encoding + response body decoding |
| Canonical XML codec | `openehr/serialize/canxml/` | Optional via `Accept: application/xml` negotiation |
| Type registry | `openehr/rm/typereg` | Polymorphic response decoding |
| Transport wrapper | `transport/` | HTTP `*http.Client` injection, headers, error mapping, retries, OTel |
| Auth | `auth/` + `auth/<provider>/` | `TokenSource` provides bearer tokens |
| Discovery | `smart/discovery/` | `ServiceCatalog` resolves base URLs per service id (REQ-070) |
| Sandbox | `sandbox/` | In-memory + cassette transports that implement the same client interfaces |
| Testkit | `testkit/` | Test doubles, cassette runners, conformance probes |

The client does **not** introduce new building-block dependencies. It composes existing ones per the SDK's dependency direction (REQ-014, [`docs/specifications/module-layout.md § Dependency direction`](../../../docs/specifications/module-layout.md#dependency-direction)).

## What "REST 1.1.0-development" means here

Per the openEHR ITS-REST overview and per the per-area OpenAPI YAML sources (the SDK's source of truth):

- **Six functional API areas:** System, EHR, Query, Definition, Demographic, Admin.
- **Versioned object semantics:** every versioned resource (Composition, EHR_STATUS, Directory, Folder, Contribution-derived Versions) carries an `ETag` on responses and requires `If-Match` on PUT (REQ-054).
- **Custom headers (the `openehr-*` family):**
  - `openehr-version` — explicit RM version negotiation on a per-request basis.
  - `openehr-audit-details` — commit-time audit envelope for write paths.
  - `openehr-template-id` — declares the template the payload conforms to (write paths).
  - `openehr-uri` — opaque resource pointer (some endpoints).
  - `openehr-item-tag` — ItemTag operations (REST 1.1.0 new).
- **`Prefer` response-shape negotiation:** `Prefer: return=minimal|identifier|representation`. The SDK exposes this as a per-call option; default is `minimal` for writes and `representation` for reads.
- **Media types:**
  - Canonical JSON: `application/json` (default).
  - Canonical XML: `application/xml` (negotiable, requires `canxml`).
  - Simplified formats: `application/openehr.wt.flat+json`, `application/openehr.wt.structured+json`, `application/openehr.wt+json` — **deferred** (separate plan).
- **Error model:** structured response body with `message`, `code`, and an optional coded-text array; mapped onto the SDK's typed `transport.WireError` hierarchy with the openEHR-specific detail attached.
- **Status code matrix:** standard plus `412 Precondition Failed`, `428 Precondition Required`, `409 Conflict` mapped to the `transport.Err*` sentinels per REQ-054.

> **Source of truth for endpoint shapes:** the OpenAPI YAML files at https://github.com/openEHR/specifications-ITS-REST/tree/master/computable/OAS. Whenever this plan and the OpenAPI disagree, the OpenAPI wins. The plan does not duplicate every endpoint; it lays out the package structure and conventions.

## Why now

- The BMM codegen, canonical JSON codec, and `transport/` building blocks are complete (or scheduled). The client is the bridge that ties them to consumers.
- The four primary use cases (benchmark, seeder, MCP server, federator — [`docs/specifications/use-cases.md § Primary use cases`](../../../docs/specifications/use-cases.md#primary-use-cases)) **all** need the REST client. Without it, every use case rebuilds HTTP, auth, RM serialization, and error mapping in isolation — exactly the drift problem the SDK was created to prevent.
- A benchmark harness exercises the client under realistic load — deferred to Phase 9 (optional).
- PROBE-010..013 (versioned-write conformance — REQ-054) cannot be implemented in code without a real client.

## Out of scope

- **SMART OAuth2/PKCE flow** — `auth/smart/` is a separate plan. This plan integrates `auth.TokenSource` but does not implement specific providers.
- **Federator policy** (authority, merge, partial-failure semantics) — separate proposal pending MPI research outcomes. The client provides per-node primitives; policy is the consumer's.
- **MCP framework integration** — the client is consumable from an MCP server; the wiring is the MCP server's concern.
- **FLAT / STRUCTURED / Web-Template format clients** — separate plan. Each format has different semantics, different content types, different OPT-driven assembly logic. This plan covers canonical JSON (default) and canonical XML (negotiable).
- **EHR Extract API** — deferred per existing scope decision (RM `ehr_extract` package skipped — see [`docs/specifications/scope.md`](../../../docs/specifications/scope.md) and the BMM codegen plan).
- **AQL builder design** — that's `openehr/aql/`. This plan's `client/query` package consumes the builder but does not redesign it.
- **Cassette recording infrastructure** — depends on the testkit work; this plan declares the dependency but does not implement the recorder.
- **Cadasto Extra / Datamap / Care / MPI / Admin** — those live under `cadasto/...` and have their own plans. This plan covers only standard openEHR ITS-REST.

## Package layout (additive to existing module layout)

Already present as stubs (per [`docs/specifications/module-layout.md`](../../../docs/specifications/module-layout.md)):

```
openehr/client/
├── doc.go               # already written (see user edit)
├── system/              # System API
├── ehr/                 # EHR API + sub-resources
├── query/               # Query API (AQL)
├── definition/          # Definition API
└── demographic/         # Demographic API
```

This plan adds:

```
openehr/client/
├── admin/               # Admin API — to be added to module-layout.md in Phase 0
└── ehr/
    ├── composition/     # leaf — Composition CRUD
    ├── contribution/    # leaf — multi-version atomic commits
    ├── directory/       # leaf — Folder/Directory CRUD
    ├── ehrstatus/       # leaf — EHR_STATUS read/update
    └── itemtags/        # leaf — ItemTag operations (REST 1.1.0)
```

Rationale for leaf sub-packages under `client/ehr/`: each resource has its own CRUD surface and the consumer typically imports one leaf (`composition.Save(ctx, ...)`). The parent `client/ehr/` package exposes the common types (`EhrID`, `VersionedObjectID`, etc.) and the EHR-itself operations (`ehr.Get`, `ehr.Create`).

## Canonical client shape

Each leaf package follows the same shape — package-level functions over a `Client` struct, both styles available (REQ-023):

```go
package composition

// Package-level surface (primary, REQ-023):
func Save(ctx context.Context, c *transport.Client, ehrID rm.EhrID, comp *rm.Composition, opts ...SaveOption) (*rm.Composition, *VersionMetadata, error)
func Get(ctx context.Context, c *transport.Client, ehrID rm.EhrID, ref Ref, opts ...GetOption) (*rm.Composition, *VersionMetadata, error)
func Update(ctx context.Context, c *transport.Client, ehrID rm.EhrID, voID rm.VersionedObjectID, ifMatch string, comp *rm.Composition, opts ...UpdateOption) (*rm.Composition, *VersionMetadata, error)
func Delete(ctx context.Context, c *transport.Client, ehrID rm.EhrID, versionUID rm.VersionUID, ifMatch string, opts ...DeleteOption) error

// Repository convenience for DI seams (REQ-023):
type Repository interface { /* mirrors the package-level functions */ }
func NewRepository(c *transport.Client) Repository
```

Every method takes `ctx` first (REQ-020), accepts functional options (REQ-022), returns typed errors (REQ-025), and is goroutine-safe (REQ-026). Generics carry typed responses through `transport.Decode[T]` (REQ-024).

## Phases

Sequenced so each phase delivers a runnable surface (or framework gate) and the SDK build stays green throughout.

### Phase 0 — Normative alignment, fixtures, probe reservation

**Outcome:** Specs and test inputs are pinned before client code lands.

**Tasks:**

1. **Specs updates landed alongside this plan (already in `docs/specifications/`):** REQ-059 (openEHR custom header family), REQ-093 (error envelope), REQ-094 (`Prefer` negotiation), REQ-095 (OpenAPI YAML authoritative source); [`wire.md`](../../../docs/specifications/wire.md) sections "Functional API areas", "Authoritative source", "openEHR custom header family", "`Prefer` negotiation", "Error envelope"; [`module-layout.md`](../../../docs/specifications/module-layout.md) — added `openehr/client/admin/` and `openehr/client/ehr/<sub-leaves>/` (composition, contribution, directory, ehrstatus, itemtags); [`glossary.md`](../../../docs/specifications/glossary.md) — `Prefer`, `openehr-audit-details`, expanded ItemTag, error envelope, OpenAPI authoritative source entries.
2. **Reserve probes in [`docs/specifications/conformance.md`](../../../docs/specifications/conformance.md)** (Draft placeholders):
   - **PROBE-040** — EHR creation round-trip (POST `/ehr` → EHR_STATUS body → 201 → `ehr_id` extracted).
   - **PROBE-041** — Composition versioned write — `Prefer: return=representation` returns a bare `COMPOSITION` body with new `ETag` (REQ-094 (+052)); renumbered to PROBE-061 + PROBE-071 in the REST-binding range. See [`conformance.md`](../../specifications/conformance.md#probe-061--composition-versioned-write-with-prefer-returnrepresentation).
   - **PROBE-042** — `openehr-audit-details` header round-trip (write → read back via Contribution).
   - **PROBE-043** — Discovery-routed request — client uses `org.openehr.rest` base URL from `ServiceCatalog`, not a hard-coded value.
   - **PROBE-044** — Per-request `auth.TokenSource` (via `ctx`) overrides client-default `TokenSource` for the duration of one request.
   - **PROBE-045** — `Prefer: return=minimal` on POST returns identifier only; subsequent GET returns full payload.
   - **PROBE-046** — Stored AQL query (`query.RunStored`) hits `/query/{qualified_name}` and returns a typed `ResultSet`.
   - **PROBE-047** — Template upload (ADL1.4) round-trips; subsequent template GET returns the same OPT bytes.
   - **PROBE-048** — Error envelope: a 400 with `{message, code}` JSON body decodes into `transport.WireError.OpenEHR` and `errors.As` to a typed error.
   - **PROBE-049** — Spec-version mismatch at discovery time fails with `DiscoveryError{Reason: spec_version_mismatch}` — already covered by PROBE-003, listed here for completeness.
3. **Vendor REST fixtures** → `testkit/cassettes/its_rest/`:
   - Composition POST/GET/PUT/DELETE request+response pairs (from a Cadasto reference deployment or hand-crafted to the OpenAPI shapes).
   - System API capabilities response.
   - AQL execute request+response (one ad-hoc, one stored).
   - Template upload + retrieval.
   - Error envelopes for 400, 401, 403, 404, 409, 412, 428.
   - Provenance README citing the OpenAPI YAML release and the source deployment commit.
4. **Confirm `transport/` API surface** — the leaf clients in this plan depend on it. Block until `transport/` is at the documented surface (next phase).

**Definition of done:**

- PROBE-040..048 exist in `conformance.md` as Draft with the conditions listed above.
- `testkit/cassettes/its_rest/` populated with at least the System + EHR + Composition + Query subsets.
- `module-layout.md` includes `openehr/client/admin/`.

### Phase 1 — `transport/` HTTP foundation

**Outcome:** A working, building-block `transport.Client` that every per-area client wraps. No openEHR-specific endpoints yet — just the HTTP plumbing.

**Tasks:**

1. **`transport.Client` core:**
   ```go
   package transport

   type Client struct {
       httpClient *http.Client
       catalog    *discovery.ServiceCatalog
       tokenSrc   auth.TokenSource
       opts       options
       // ... unexported
   }

   func New(opts ...Option) (*Client, error)

   // Per-call request builder
   func (c *Client) Do(ctx context.Context, req *Request) (*Response, error)

   // Typed wrapper using generics (REQ-024)
   func Decode[T any](ctx context.Context, c *Client, req *Request) (*T, *Metadata, error)
   ```
2. **`Request` type** carries: HTTP method, service ID (`org.openehr.rest`), path with `{ehr_id}` placeholders, query parameters, headers (typed setters for `openehr-*` family + `Prefer` + `If-Match`), body payload (`any`, marshalled via the configured codec).
3. **`Response` type** carries: status code, body bytes (lazy-decoded), `ETag` (quote-stripped), `Location`, `openehr-*` response headers, and the OTel span context.
4. **Functional options** (REQ-022):
   - `WithHTTPClient(*http.Client)` — required for non-trivial use (REQ-021).
   - `WithServiceCatalog(*discovery.ServiceCatalog)` — required (REQ-070).
   - `WithTokenSource(auth.TokenSource)` — defaults to anonymous.
   - `WithUserAgent(string)`, `WithSpecVersion(string)` (REQ-051), `WithRetry(retry.Policy)` (REQ-091, off by default), `WithLogger(slog.Logger)`.
   - `WithCallerAttribution(transport.CallerAttribution)` (REQ-066, opt-in).
   - `WithDefaultCodec(serialize.Codec)` — default `canjson`.
5. **Header plumbing:**
   - `Accept` / `Content-Type` from the active codec (default `application/json`).
   - `Authorization: Bearer <token>` from `tokenSrc` per request — supports per-request `TokenSource` from `ctx` (REQ-066 hook, REQ-060 + REQ-020).
   - `Cadasto-OpenEhr-Spec-Version` opt-in (REQ-051) — only set when the discovery document indicates a Cadasto deployment OR `WithCadastoSpecVersionHeader(true)` is set.
   - `If-Match` / `ETag` round-trip (REQ-054) — quote-stripping on read, quote-wrapping on write (the CDR benchmark client got this right; we lift the convention).
   - `openehr-audit-details` set from a `transport.WithAuditDetails(...)` per-call option.
   - `Prefer` from a `transport.WithPrefer("return=representation")` option.
   - W3C `traceparent`/`tracestate` from OTel propagation (REQ-090).
6. **Error mapping** — status code → typed sentinel from [`docs/specifications/idiom.md § Errors`](../../../docs/specifications/idiom.md#errors-req-025):
   - `404` → `ErrNotFound`
   - `401` → `ErrUnauthorized`
   - `403` → `ErrForbidden`
   - `409` → `ErrVersionConflict`
   - `412` → `ErrPreconditionFailed`
   - `428` → `ErrPreconditionRequired`
   - `5xx` → wrapped `WireError` with the openEHR error envelope decoded into `OpenEHRErrorDetail{Message, Code, CodedTextArray}`.
7. **Tests** (using `httptest.Server`):
   - Round-trip of every typed header.
   - Error decoding from a 400 envelope.
   - `If-Match` is sent on PUT; `ETag` is captured on response.
   - Per-request `auth.TokenSource` via `ctx` overrides the client-default.
   - Retry policy off by default; explicit `WithRetry(...)` retries on 503 with backoff.
   - OTel span created with sanitised URL (no bearer in attributes).

**Definition of done:**

- `go test ./transport/...` passes.
- A test request against a stub HTTP server completes round-trip with all expected headers.
- `transport.Client` is documented; its API stable for downstream packages.

### Phase 2 — System API (`openehr/client/system/`)

**Outcome:** The smallest typed client, serves as the integration smoke test for the whole stack.

**Tasks:**

1. `system.Capabilities(ctx, c) (*rm.Capabilities, error)` — `GET /` or `/openehr/v1/` (per OpenAPI).
2. `system.Version(ctx, c) (string, error)` — surfaces deployment-declared version metadata.
3. `system.Health(ctx, c) (*HealthStatus, error)` — optional helper around the deployment's health endpoint (Cadasto-specific path may differ; the SDK MAY emit it via a Cadasto-platform Extra option but the base method targets the standard ITS-REST capabilities endpoint).
4. No versioning, no auth complications, no polymorphic bodies — perfect smoke test.
5. Tests against the cassette fixtures from Phase 0.

**Definition of done:**

- A consumer can call `system.Capabilities(ctx, c)` against the sandbox and the cassette runner returns a typed `Capabilities`.

### Phase 3 — EHR API: read paths

**Outcome:** Read-only EHR resource access. No versioned writes yet. ETag capture works end-to-end.

**Tasks:**

1. `openehr/client/ehr/`:
   - `ehr.Get(ctx, c, ehrID) (*rm.Ehr, *VersionMetadata, error)` — `GET /ehr/{ehr_id}`.
   - `ehr.Exists(ctx, c, ehrID) (bool, error)` — `HEAD /ehr/{ehr_id}`.
   - `ehr.GetBySubject(ctx, c, subjectNamespace, subjectID) (*rm.Ehr, error)` — `GET /ehr?subject_id=...&subject_namespace=...`.
2. `openehr/client/ehr/ehrstatus/`:
   - `ehrstatus.Get(ctx, c, ehrID, opts ...GetOption) (*rm.EhrStatus, *VersionMetadata, error)` — `GET /ehr/{ehr_id}/ehr_status` (latest) or at-time / at-version variants.
   - `ehrstatus.GetVersioned(ctx, c, ehrID, versionUID) (...)`.
3. `openehr/client/ehr/composition/`:
   - `composition.Get(ctx, c, ehrID, ref Ref) (*rm.Composition, *VersionMetadata, error)`.
   - `Ref` is a typed union (sealed interface) with `ByVersionedObjectID` (latest) and `ByVersionUID` (specific) — REQ-024 generics not required since the result type is fixed.
4. `openehr/client/ehr/directory/`:
   - `directory.Get(ctx, c, ehrID, opts ...GetOption) (*rm.Folder, *VersionMetadata, error)`.
   - `directory.GetVersioned(ctx, c, ehrID, versionUID) (...)`.
   - `directory.GetAtTime(ctx, c, ehrID, t time.Time) (...)`.
5. `VersionMetadata` holds `ETag`, `Location`, `LastModified`, and the parsed `version_uid` extracted from response envelope (mirroring the CDR benchmark's `extractVersionUID` — promoted to a typed helper).
6. Tests against cassettes.

**Definition of done:**

- All read paths pass on cassettes.
- ETags propagate through to `VersionMetadata` for use in subsequent writes (Phase 4).

### Phase 4 — EHR API: versioned writes

**Outcome:** Versioned-write surface complete. REQ-054 enforced; PROBE-010, PROBE-011, PROBE-012 implemented.

**Tasks:**

1. `ehr.Create(ctx, c, opts) (*rm.Ehr, error)` — POST `/ehr` (server-assigned ID) or PUT `/ehr/{ehr_id}` (client-supplied ID).
   - Options: `WithInitialStatus(*rm.EhrStatus)`, `WithSubject(rm.PartyRef)`.
2. `ehrstatus.Put(ctx, c, ehrID, ifMatch string, status *rm.EhrStatus, opts ...PutOption) (*VersionMetadata, error)` — REQ-054.
3. `composition.Save(ctx, c, ehrID, comp *rm.Composition, opts ...SaveOption) (*rm.Composition, *VersionMetadata, error)` — body decoded per the ITS-REST OpenAPI `201_COMPOSITION` schema as a bare COMPOSITION (REQ-094 (+052)); the ORIGINAL_VERSION envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}`.
   - Options: `WithPrefer("return=representation"|"return=minimal"|"return=identifier")`, `WithAuditDetails(*rm.AuditDetails)`, `WithTemplateID(string)`.
4. `composition.Update(ctx, c, ehrID, voID, ifMatch, comp, opts...)` — REQ-054.
5. `composition.Delete(ctx, c, ehrID, versionUID, ifMatch, opts...)` — REQ-054.
6. `directory.Save(...)`, `directory.Update(...)`, `directory.Delete(...)` — same shape.
7. `contribution.Commit(ctx, c, ehrID, batch *rm.Contribution, opts...)` — multi-version atomic commit; carries `openehr-audit-details` at the envelope level.
8. `itemtags.Set(ctx, c, ehrID, target ItemTagTarget, tag rm.ItemTag, opts...)`, `itemtags.Get(...)`, `itemtags.Delete(...)` — REST 1.1.0 new.
9. **Typed errors propagated:** `transport.ErrPreconditionFailed`, `transport.ErrPreconditionRequired`, `transport.ErrVersionConflict` — verified via probes.
10. Tests:
    - Round-trip POST + PUT (with `If-Match` from initial POST response) + GET (returns updated body) + DELETE (returns 204).
    - PROBE-010 / PROBE-011 / PROBE-012 implemented as probe code (per [`docs/specifications/conformance.md`](../../../docs/specifications/conformance.md)).

**Definition of done:**

- Full versioned-write surface compiles and passes cassette tests.
- PROBE-010..012 land as runnable code in `testkit/probes/versioned/`.
- A consumer can replicate the CDR benchmark's write flow via the SDK with no raw HTTP.

### Phase 5 — Query API (AQL)

**Outcome:** AQL execution surface — ad-hoc and stored. REQ-055, REQ-057 implemented.

**Tasks:**

1. `openehr/client/query/`:
   - `query.Execute(ctx, c, q *aql.Query, opts ...ExecuteOption) (*ResultSet, error)` — `POST /query/aql`.
   - `query.ExecuteString(ctx, c, raw string, params map[string]any, opts...) (*ResultSet, error)` — escape hatch for raw AQL strings.
   - `query.RunStored(ctx, c, qualifiedName string, version string, params map[string]any, opts...) (*ResultSet, error)` — `GET /query/{qualified_query_name}` or `POST /query/{qualified_query_name}` with body params per the OpenAPI.
2. `ResultSet` carries `Name`, `Q` (the executed AQL string), `Columns []ColumnDef`, `Rows [][]any` (row values typed `any` because their schema depends on the query — REQ-024 escape hatch is fine here; consumers cast or use a generic helper).
3. Generic helper: `query.ExecuteTyped[T any](ctx, c, q, opts...) ([]T, error)` — for queries whose row shape is known at the call site (e.g. one column = one struct).
4. Error mapping for AQL-level failures (parse error, path resolution) into a typed `query.AQLError` distinct from `transport.WireError`.
5. Tests: ad-hoc query, stored query by ID, stored query with bind params, AQL syntax error → typed `AQLError`.

**Definition of done:**

- `query.Execute` and `query.RunStored` pass cassette tests including the `ResultSet` shape.
- PROBE-046 lands.

### Phase 6 — Definition API

**Outcome:** Template (OPT) and stored-query lifecycle. REQ-057 (stored queries) integrates here.

**Tasks:**

1. `openehr/client/definition/`:
   - `definition.UploadTemplate(ctx, c, format TemplateFormat, body io.Reader, opts ...UploadOption) (*TemplateMetadata, error)` — `POST /definition/template/{format}` where `format` is `adl1.4` or `adl2`. Body content type follows the OpenAPI (`application/xml` for OPT, `text/plain` for ADL2 source).
   - `definition.GetTemplate(ctx, c, templateID, version string, format TemplateFormat) ([]byte, *TemplateMetadata, error)`.
   - `definition.ListTemplates(ctx, c, opts ...ListOption) ([]TemplateMetadata, error)`.
   - `definition.DeleteTemplate(ctx, c, templateID string) error` — where supported by the deployment.
   - **Example generation:** `definition.ExampleComposition(ctx, c, templateID string, format TemplateFormat) (*rm.Composition, error)` — uses the deployment's `?format=...` example endpoint.
2. **Stored AQL queries:**
   - `definition.PutStoredQuery(ctx, c, qualifiedName, version string, aql string, opts...) (*StoredQueryMetadata, error)`.
   - `definition.GetStoredQuery(ctx, c, qualifiedName, version string) (*StoredQueryMetadata, error)`.
   - `definition.ListStoredQueries(ctx, c) ([]StoredQueryMetadata, error)`.
   - `definition.DeleteStoredQuery(ctx, c, qualifiedName, version string) error`.
3. Tests: upload-then-read cycle for OPT and ADL2 sources; example generation; stored-query CRUD.

**Definition of done:**

- Template + stored-query lifecycle complete.
- PROBE-047 lands.

### Phase 7 — Demographic API

**Split out** to its own active plan: [`2026-06-14-demographic-rest-client.md`](2026-06-14-demographic-rest-client.md). The Demographic API (PARTY-hierarchy CRUD) was `doc.go`-only when this plan was archived; it is tracked there.

### Phase 8 — Admin API

**Outcome:** Standard ITS-REST Admin API — EHR physical delete and other administrative-lifecycle ops.

**Tasks:**

1. `openehr/client/admin/` (REQ-099 — landed surface):
   - `admin.DeleteEHR(ctx, c, ehrID, opts ...DeleteOption) error` — `DELETE /admin/ehr/{ehr_id}`.
   - `admin.DeleteAllEHRs(ctx, c, opts ...) error` — bulk delete when the deployment exposes it.
   - `admin.PurgeTemplates(ctx, c, opts ...) error` — template housekeeping per ITS-REST admin paths.
   - **Not in v1:** `PhysicalDeleteEHR`, per-composition admin delete, per-contribution admin delete (were draft API names in an earlier plan revision).
2. The Admin API is `Status: development` in ITS-REST 1.1.0-development; same maturity caveat as Demographic.
3. **Distinction from `cadasto/admin/`** — that package handles Cadasto-platform admin (tenant, env). This package handles the **openEHR** Admin API (REST surface).

**Definition of done:**

- Admin API compiles; cassette tests exercise physical-delete flows.

### Phase 9 — Benchmark harness (optional)

**Outcome:** A load/benchmark harness runs via this SDK with no measurable percentile regression vs a raw-HTTP baseline.

**Tasks:**

1. Migrate the CDR benchmark harness's client layer to use:
   - `transport.Client` with the same HTTP transport tuning (MaxIdleConnsPerHost, KeepAlive — the SDK's `transport` MUST accept these via the injected `*http.Client`).
   - The leaf clients here (`system`, `ehr/composition`, `ehr/ehrstatus`, `ehr/directory`, `definition`).
2. Confirm: the `BuildEHRStatusBody` / `BuildDirectoryBody` `map[string]any` body builders disappear from the benchmark; the benchmark builds typed `rm.EhrStatus` / `rm.Folder` values and the codec emits bytes.
3. Run the full benchmark; capture p50/p95/p99 against the raw-HTTP baseline. Document the comparison.

**Definition of done:**

- Benchmark builds on the SDK.
- p50, p95, p99 within 5% of the raw-HTTP baseline (or a documented justification for any larger delta).

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| ITS-REST 1.1.0-development endpoint paths drift between development and final 1.1.0 | The SDK pins to the OpenAPI YAML at a specific upstream commit (recorded in `testkit/cassettes/its_rest/README.md`). When 1.1.0 final ships, bump the pin in one commit; CI catches drift via the cassette set. |
| Cadasto deployment headers (`X-Tenant-Id`, `X-Subject-Id`) leak into the openEHR-core client | The standard client never sets Cadasto headers. Tenant routing is handled by the deployment gateway (per [`docs/specifications/auth.md § Per-client tenant binding`](../../../docs/specifications/auth.md) — one client = one tenant). If a Cadasto-specific header is required, it goes via `transport.WithCadastoSpecVersionHeader(true)` (REQ-051) or under `cadasto/extra/`. |
| `Prefer: return=representation` doubles wire bandwidth on writes | Documented; default for writes is `minimal`. Consumers opt in per call. The benchmark's POST shape (return-minimal-then-GET-for-ETag) is preserved by default. |
| Versioned-write semantics confuse consumers (when to send If-Match vs not) | Typed PUT functions REQUIRE `ifMatch string` as a non-optional parameter — the type system enforces it. Forgetting it is a compile error, not a 428 at runtime. |
| The `_type` field appears in the wrong RM shape after decoding via typereg | Caught by canjson Phase 2 round-trip tests; orthogonal to this plan but a dependency. |
| Discovery refresh racing with a PUT causes a stale base URL to be used mid-request | `transport.Client` snapshots the catalog at request start; refresh applies to the next request. Documented. PROBE-041 in the discovery plan covers the case. |
| AQL bind parameters with `any` row values lose type safety | Documented in `query.ExecuteTyped[T]` — consumers reach for the generic helper when the row shape is known. Otherwise `any` is the honest answer. |
| Cassette drift when the deployment's response shape evolves | The cassette README documents the source deployment commit; refreshing is an explicit step with CHANGELOG. |
| OPT XML upload via `Content-Type: application/xml` while the rest of the client uses JSON | `definition.UploadTemplate` takes a `TemplateFormat` enum that drives `Content-Type`; the typed surface prevents mixing. |
| Goroutine-safety pitfalls in the shared `transport.Client` | REQ-026 mandates safety; tested with `go test -race`. `Decode[T]` does not mutate the client; per-request `ctx` carries any per-call state. |
| `openehr-audit-details` schema evolves between RM versions | The SDK accepts a typed `*rm.AuditDetails` and serialises via canjson; the codec follows the BMM; RM-version bumps regenerate `AuditDetails`. No header-format hand-coding. |

## Mapping to specs

- [docs/specifications/wire.md § REST version pin](../../../docs/specifications/wire.md#req-050) — REQ-050; the contract.
- [docs/specifications/wire.md § Cadasto spec-version header](../../../docs/specifications/wire.md#req-051) — REQ-051; opt-in plumbing in `transport/`.
- [docs/specifications/wire.md § Optimistic concurrency](../../../docs/specifications/wire.md#req-054) — REQ-054; the versioned-write contract.
- [docs/specifications/wire.md § AQL](../../../docs/specifications/wire.md#req-055--wire-boundary) — REQ-055; consumed by `client/query/`.
- [docs/specifications/wire.md § Stored AQL](../../../docs/specifications/wire.md#req-057) — REQ-057; consumed by `client/definition/` and `client/query/`.
- [docs/specifications/idiom.md](../../../docs/specifications/idiom.md) — REQ-020..026; followed by every leaf client.
- [docs/specifications/auth.md](../../../docs/specifications/auth.md) — REQ-060..068; integrated via `transport.WithTokenSource` and per-request `auth.WithTokenSource(ctx, ts)`.
- [docs/specifications/service-discovery.md](../../../docs/specifications/service-discovery.md) — REQ-070..072; integrated via `transport.WithServiceCatalog`.
- [docs/specifications/conformance.md](../../../docs/specifications/conformance.md) — PROBE-010..013 implemented; PROBE-040..049 reserved/implemented.
- [docs/specifications/research-strands.md § STRAND-01](../../../docs/specifications/research-strands.md) — Resolved; the SDK is independent, so the Phase 9 benchmark harness is optional.

## Out-of-band considerations

- **Cross-SDK parity (REQ-080, REQ-081).** The PHP SDK's wire request for the same SDK call MUST be byte-equivalent (modulo header order / case). Cassettes are shared across SDKs once the cassette format stabilises.
- **CDR benchmark as the reference consumer.** The CDR benchmark's client layer is the closest existing code to what this plan produces. It demonstrates: HTTP transport tuning per worker, ETag round-trip, `If-Match` quoting, version-UID extraction, and Cadasto-specific `X-Tenant-Id` / `X-Subject-Id` headers. The SDK extracts the **patterns** (transport tuning is consumer config; ETag/If-Match plumbing is in `transport/`); it does not lift Cadasto-specific headers into the openEHR-core surface.
- **Sandbox transport equivalence.** Every leaf client MUST be testable against `sandbox/` without code changes — `sandbox.NewInMemory()` implements the same `transport.Client`-like interface so the leaf clients can swap transports at construction time. This is the building block for hermetic SDK-consumer tests.
- **Federator implications.** Phase 1's `transport.Client` is per-issuer / per-tenant (REQ-065). The federator constructs multiple clients with independent `*http.Client`, `ServiceCatalog`, and `TokenSource` configurations — see [`docs/specifications/research-strands.md § STRAND-06`](../../../docs/specifications/research-strands.md). This plan does not resolve STRAND-06 but it does provide the per-node primitive.
- **Future: streaming bodies.** Out of scope. Large composition payloads (>10 MB) are rare in v1; if a use case appears, add a `transport.WithStreaming(...)` option later.
- **Future: HTTP/2 multiplexing.** The injected `*http.Client` transport governs HTTP/2 use. The SDK does not configure it.
- **Future: Conformance test against EHRbase.** The openEHR-core part of this client is in theory vendor-neutral. STRAND-08 (Cadasto extras boundary) tracks whether to formalise EHRbase as a tested backend; this plan does not commit to it but does not preclude it (the standard openEHR REST endpoints, headers, and error envelopes are EHRbase-compatible).
