# Requirements

**Status:** Draft

Enumerated normative requirements for `github.com/cadasto/openehr-sdk-go`. Each `REQ-NNN` is stable once published; the requirement itself may be amended (with a CHANGELOG entry), but the ID does not change.

Conventions: RFC 2119 keywords — see [README.md § How to read these specs](README.md#how-to-read-these-specs).

The "Covered by" field cross-references the spec file or section that elaborates the requirement, and (once implementation lands) the package and tests that demonstrate compliance.

---

## Module identity and packaging

### REQ-001 — Module path

The SDK **MUST** be published as the Go module `github.com/cadasto/openehr-sdk-go`.

- **Rationale:** Idiomatic lowercase Go module path aligned with the GitHub organisation login; cross-language discoverability with the Cadasto PHP SDK; locked to avoid late module-path churn.
- **Covered by:** [module-layout.md § Module identity](module-layout.md#module-identity)

### REQ-002 — Go version

The SDK **MUST** declare `go 1.25` (or later patch within the 1.25 line) in `go.mod` and **MUST NOT** require a more recent Go release than is currently on the upstream supported line (N-1 policy).

- **Rationale:** Tracks Go's officially supported releases; pins the language features available (generics with type sets, `errors.Is/As/Unwrap`, etc.).
- **Covered by:** [module-layout.md § Versioning](module-layout.md#versioning)

### REQ-003 — License

The SDK **MUST** be distributed under the MIT License.

- **Covered by:** [`../LICENSE`](../LICENSE)

### REQ-004 — Semantic versioning

The SDK **MUST** follow Semantic Versioning 2.0.0. Major versions `v2` and beyond **MUST** use Go's semantic-import-versioning convention (`…/v2/`).

- **Covered by:** [module-layout.md § Versioning](module-layout.md#versioning)

### REQ-005 — Internal boundary

Anything under `internal/` **MUST** be considered outside the public API surface; consumers **MUST NOT** import from it and the SDK **MAY** change it without notice.

- **Covered by:** [module-layout.md § The `internal/` boundary](module-layout.md#the-internal-boundary)

---

## Module layout and boundaries

### REQ-010 — `cadasto/` cut line

No package outside `cadasto/…` **MAY** import from `cadasto/…`. The packages allowed to be importers of `cadasto/<X>` are: the consuming application (in `cmd/` and downstream repos) and other `cadasto/<X>` packages **only via** openEHR-core types or interface contracts (see REQ-011).

- **Rationale:** Preserves the option of extracting `cadasto/` into a sibling module without a rewrite.
- **Covered by:** [module-layout.md § Boundary rules](module-layout.md#boundary-rules)

### REQ-011 — No sideways imports inside `cadasto/`

No `cadasto/<X>` package **MAY** import another `cadasto/<Y>` package directly. Shared types **MUST** live in openEHR-core packages (under `openehr/…`, `auth/…`, `smart/…`, or `transport/…`), or be expressed as interfaces consumed by both.

- **Covered by:** [module-layout.md § Boundary rules](module-layout.md#boundary-rules)

### REQ-012 — Auth layering

`auth/` **MUST** define a generic `TokenSource` abstraction. Provider-specific implementations (`auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, …) **MUST** be sub-packages and **MUST NOT** appear in the public surface of `auth/` itself.

- **Covered by:** [auth.md § TokenSource contract](auth.md#tokensource-contract)

### REQ-013 — Building-block independence

Each of `openehr/rm`, `openehr/serialize`, `openehr/validation`, `openehr/template`, and `openehr/aql` (models only) **MUST** be importable and useful without constructing an authenticated client or instantiating `transport/` or `auth/`.

- **Rationale:** Authoring tools, CI validators, FHIR-mapping prototypes, AQL linters — these consumers do not touch HTTP and the SDK must not force the dependency.
- **Covered by:** [use-cases.md § Building-block use cases](use-cases.md#building-block-use-cases)

### REQ-014 — Dependency direction

Imports between SDK packages **MUST** flow strictly downward through the dependency graph defined in [module-layout.md § Dependency direction](module-layout.md#dependency-direction). Upward or cyclic imports are prohibited.

- **Covered by:** [module-layout.md § Dependency direction](module-layout.md#dependency-direction)

---

## Idiomatic surface

### REQ-020 — Context-first I/O

Every method, function, or constructor that performs I/O (HTTP, file, disk-cached lookup) **MUST** take `context.Context` as its first parameter.

- **Covered by:** [idiom.md § Context propagation](idiom.md#context-propagation)

### REQ-021 — Injected `*http.Client`

The SDK **MUST NOT** allocate its own `*http.Client`. Constructors and clients **MUST** accept an injected `*http.Client` (or a higher-level type that wraps one); zero-value or `nil` **MAY** be permitted to mean "use `http.DefaultClient`" but **SHOULD** be discouraged in documentation.

- **Rationale:** Connection-pool, timeout, and TLS configuration belong to the application, not the SDK. The federator use case in particular requires per-node transport control.
- **Covered by:** [idiom.md § HTTP client injection](idiom.md#http-client-injection)

### REQ-022 — Functional options for configuration

SDK constructors **SHOULD** accept configuration via functional options (`sdk.WithX(...)`, `sdk.WithY(...)`). Public config structs **MAY** exist as a convenience for serialised configuration but **MUST** be normalised through the same option chain.

- **Covered by:** [idiom.md § Functional options](idiom.md#functional-options)

### REQ-023 — Package-level functions as primary surface

The primary call-site surface **SHOULD** be package-level functions; repository-style struct surfaces **MAY** be offered as an injection-seam convenience but **MUST NOT** be the only entry point.

- **Covered by:** [idiom.md § Surface shape](idiom.md#surface-shape)

### REQ-024 — Generics, no reflection

Typed REST clients, validators, repositories, and template bindings **MUST** carry types via Go generics. Reflection **MUST NOT** be used to project the openEHR `_type` discriminator onto a Go type — the type registry (REQ-040) is the only sanctioned mechanism.

- **Covered by:** [idiom.md § Generics policy](idiom.md#generics-policy), [rm-modeling.md § Type registry](rm-modeling.md#type-registry)

### REQ-025 — Error wrapping

Errors crossing package boundaries **MUST** be wrapped (`fmt.Errorf("...: %w", err)`) when context is added, and **MUST** be unwrappable with `errors.Is` / `errors.As`. Library code **MUST NOT** panic on input that originated from the wire or from a consumer; panics are reserved for programmer errors (nil-pointer, broken invariant).

- **Covered by:** [idiom.md § Errors](idiom.md#errors)

### REQ-026 — Goroutine-safe clients

All public SDK clients **MUST** be safe for concurrent use by multiple goroutines without external synchronisation. Exceptions **MUST** be documented in the package `doc.go`.

- **Covered by:** [idiom.md § Concurrency](idiom.md#concurrency)

---

## Reference Model

### REQ-030 — Concrete structs for concrete RM types

Each concrete openEHR RM type (e.g. `COMPOSITION`, `OBSERVATION`, `DV_QUANTITY`) **MUST** be expressed as a concrete Go struct. The SDK **MUST NOT** emulate inheritance via "base struct + flag" patterns.

- **Covered by:** [rm-modeling.md § Concrete types](rm-modeling.md#concrete-types)

### REQ-031 — Embedded base structs for shared fields

Shared RM fields (e.g. `LOCATABLE`'s `name`, `archetype_node_id`, `uid`; `PATHABLE`'s parent reference) **MUST** be expressed as embedded structs in concrete types, not duplicated per type.

- **Covered by:** [rm-modeling.md § Embedded base structs](rm-modeling.md#embedded-base-structs)

### REQ-032 — Interfaces for abstract RM categories

Abstract RM categories (`DATA_VALUE`, `ITEM_STRUCTURE`, `ENTRY`, …) **MUST** be expressed as Go interfaces. A concrete struct implementing one of these categories **MUST** satisfy the corresponding interface by virtue of its method set.

- **Covered by:** [rm-modeling.md § Abstract categories](rm-modeling.md#abstract-categories)

### REQ-033 — No inheritance emulation

The SDK **MUST NOT** implement the openEHR `_type` discriminator via runtime tag-magic alone. Polymorphic JSON decoding **MUST** consult the type registry (REQ-040).

- **Covered by:** [rm-modeling.md § No inheritance emulation](rm-modeling.md#no-inheritance-emulation)

### REQ-040 — Type registry

A central type registry **MUST** live in `openehr/rm/typereg` and **MUST** map each openEHR `_type` discriminator to its concrete Go type. JSON decoding of polymorphic RM fields **MUST** consult the registry. The registry **MUST** be append-only at runtime and panic on duplicate registration of a `_type` to two different Go types.

- **Covered by:** [rm-modeling.md § Type registry](rm-modeling.md#type-registry)

### REQ-041 — Pinned BMM sources

The SDK **MUST** treat the BMM (Basic Meta-Model) files pinned in [`../resources/bmm/`](../resources/bmm/) as the canonical source of truth for the openEHR domain model. Hand-written code **MUST NOT** embed openEHR-type knowledge that is not derivable from these files.

- **Covered by:** [bmm-conformance.md § REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources)

### REQ-042 — Generated code with drift detection

The packages `openehr/rm/`, `openehr/rm/typereg/`, and `openehr/aom/aom14/` **MUST** be generated from the BMM files by the in-tree generator (`internal/bmmgen`, `cmd/bmmgen`). The generator **MUST** be reproducible (same input → byte-identical output) and CI **MUST** fail when the working tree diverges from the regenerated output.

- **Covered by:** [bmm-conformance.md § REQ-042](bmm-conformance.md#req-042--generated-code-drift-detected)

### REQ-043 — P_BMM → Go mapping rules

The generator **MUST** apply the mapping rules in [bmm-conformance.md § Mapping rules](bmm-conformance.md#mapping-rules) for every BMM concept. Deviations require an ADR and an update to that section.

- **Covered by:** [bmm-conformance.md § Mapping rules](bmm-conformance.md#mapping-rules)

### REQ-044 — Hand-written extensions isolated from generated files

Hand-written code in BMM-derived packages **MUST** live in separate files clearly marked as non-generated (`<file>_ext.go` convention). Generated files **MUST** carry the `// Code generated by bmmgen; DO NOT EDIT.` header and **MUST NOT** be hand-edited.

- **Covered by:** [bmm-conformance.md § REQ-044](bmm-conformance.md#req-044--hand-written-extensions-are-isolated)

### REQ-045 — BMM loader as building block

A public `openehr/bmm/` package **MUST** exist that loads and resolves BMM schemas (P_BMM JSON) without depending on `transport/`, `auth/`, or any HTTP machinery. Per REQ-013, this package is independently importable.

- **Covered by:** [bmm-conformance.md § REQ-045](bmm-conformance.md#req-045--bmm-loader-is-a-building-block), [module-layout.md](module-layout.md)

### REQ-046 — Primitive type mapping

The 29 BMM primitive types **MUST** map to Go types per the table in [bmm-conformance.md § Primitive type mapping](bmm-conformance.md#primitive-type-mapping). Mappings are fixed; alternative widenings are not permitted.

- **Covered by:** [bmm-conformance.md § Primitive type mapping](bmm-conformance.md#primitive-type-mapping)

### REQ-047 — BMM is authoritative on divergence

When a BMM file declares a class, property, or cardinality that contradicts non-BMM openEHR specification prose, the **BMM file is authoritative** for SDK conformance. Suspected BMM bugs are raised upstream rather than worked around in the SDK.

- **Covered by:** [bmm-conformance.md § REQ-047](bmm-conformance.md#req-047--bmm-spec-divergence-resolution)

---

## Wire format and spec version

### REQ-050 — openEHR REST 1.1.0-development pin

The SDK **MUST** target openEHR REST `1.1.0-development` as its primary wire contract. Spec-version compatibility checks **MUST** happen at discovery time (REQ-070), not on first request.

- **Covered by:** [wire.md § REST version pin](wire.md#rest-version-pin)

### REQ-051 — Versioning header (Cadasto-specific, opt-in)

The SDK **MAY** send a `Cadasto-OpenEhr-Spec-Version` request header on every outgoing request, but **MUST** keep this off by default. The header is a Cadasto-platform-specific signal and **MUST NOT** be sent to non-Cadasto backends.

- **Covered by:** [wire.md § Cadasto spec-version header](wire.md#cadasto-spec-version-header)

### REQ-052 — Canonical JSON

The SDK **MUST** produce openEHR canonical JSON for write paths and accept it for read paths. The canonical-JSON contract is defined in [wire.md § Canonical JSON](wire.md#canonical-json).

- **Covered by:** [wire.md § Canonical JSON](wire.md#canonical-json)

### REQ-053 — FLAT and STRUCTURED formats

The SDK **MUST** provide codecs for openEHR simplified formats FLAT and STRUCTURED in `openehr/serialize`. The codecs **MUST** be usable independently of the HTTP client (REQ-013).

- **Covered by:** [wire.md § Simplified formats](wire.md#simplified-formats)

### REQ-054 — Optimistic concurrency

For versioned resources (Composition, EHR_STATUS, Directory, Contribution), the SDK **MUST** treat `If-Match` / `ETag` as load-bearing; missing or stale `If-Match` on a PUT **MUST** be reported as a typed error mapped from the server's response code (typically `409` / `412` / `428`).

- **Covered by:** [wire.md § Optimistic concurrency](wire.md#optimistic-concurrency)

### REQ-055 — AQL wire boundary

`openehr/aql` **MUST** produce AQL strings on the wire that are semantically equivalent regardless of the builder style used (struct-builder vs verb-functions). Both styles **MUST** be testable against the same wire-output golden cassettes.

- **Covered by:** [wire.md § AQL](wire.md#aql)

### REQ-056 — Canonical XML

`openehr/serialize/` **MUST** include a canonical XML codec for openEHR data, symmetric to the canonical JSON codec (REQ-052). The canonical XML codec **MUST** be usable independently of the HTTP client (REQ-013).

- **Rationale:** The platform contract advertises canonical XML alongside canonical JSON; consumers exchanging openEHR artefacts with XML-pinned legacy systems need it.
- **Covered by:** [wire.md § Canonical XML](wire.md#canonical-xml)

### REQ-057 — Stored AQL queries

`openehr/client/definition/` **MUST** expose CRUD operations for openEHR stored AQL queries (register, list, get, delete). `openehr/client/query/` **MUST** support execution of a stored query by ID in addition to ad-hoc AQL execution.

- **Rationale:** Stored queries are a first-class part of the openEHR REST surface and a Cadasto-platform performance feature.
- **Covered by:** [wire.md § Stored AQL](wire.md#stored-aql), [module-layout.md](module-layout.md)

### REQ-059 — openEHR custom header family

The SDK **MUST** support sending and receiving the openEHR REST 1.1.0-development custom-header family on every request/response path where the OpenAPI declares them:

- `openehr-version` — explicit RM version negotiation on a per-request basis.
- `openehr-audit-details` — commit-time audit envelope on versioned writes (Composition, Directory, EHR_STATUS, Contribution).
- `openehr-template-id` — declares the template id the payload conforms to (Composition write paths).
- `openehr-uri` — opaque openEHR resource pointer (used by selected endpoints).
- `openehr-item-tag` — ItemTag operations (REST 1.1.0 new resource).

The headers **MUST** be set via typed per-call options on the relevant client method (not raw string maps). Reception of these headers on responses **MUST** be exposed on the typed response metadata.

- **Covered by:** [wire.md § openEHR custom header family](wire.md#openehr-custom-header-family)

### REQ-058 — Datamap V2 format

`cadasto/datamap/` **MUST** target the **Datamap V2** wire format. Older Datamap versions are out of scope for v1 of the SDK.

- **Covered by:** [module-layout.md](module-layout.md), [scope.md](scope.md)

---

## Authentication

### REQ-060 — TokenSource interface

The SDK **MUST** define an `auth.TokenSource` interface that returns a token and an expiry (or an indication of non-expiry). Every authenticated request path **MUST** acquire its bearer token through a `TokenSource`.

- **Covered by:** [auth.md § TokenSource contract](auth.md#tokensource-contract)

### REQ-061 — SMART-on-openEHR PKCE

`auth/smart` **MUST** implement the SMART-on-openEHR authorization-code-with-PKCE flow per the SMART App Launch specification, parameterised for openEHR scopes and launch context.

- **Covered by:** [auth.md § SMART-on-openEHR](auth.md#smart-on-openehr)

### REQ-062 — JWKS rotation

`auth/smart` **MUST** support JWKS key rotation: cached keys **MUST** expire on a documented TTL and **MUST** be refreshed on a verification miss before the validation is reported as a failure.

- **Covered by:** [auth.md § JWKS rotation](auth.md#jwks-rotation)

### REQ-063 — Token refresh

`auth/smart` **MUST** transparently refresh access tokens using the refresh token when one is available; consumers **MUST NOT** be required to drive the refresh manually.

- **Covered by:** [auth.md § Token refresh](auth.md#token-refresh)

### REQ-064 — Launch context

The application-level `smart/` package **MUST** expose the SMART launch context (patient, user, encounter, launch-parameters) as typed values derived from the token / launch-id response. Consumers **MUST NOT** be required to parse JWT claims by hand.

- **Covered by:** [auth.md § Launch context](auth.md#launch-context)

### REQ-065 — Per-client tenant / issuer binding

Each SDK client instance **MUST** bind to exactly one issuer (and therefore one tenant context). Multi-issuer or multi-tenant fan-out **MUST** be achieved by constructing one client per issuer; the SDK **MUST NOT** internally multiplex issuers behind a single client.

- **Rationale:** Tenant is an architectural boundary in the platform. Sharing a client across issuers would mean sharing connection pool, discovery cache, token cache — each of which must be tenant-scoped.
- **Covered by:** [auth.md § Per-client binding](auth.md#per-client-binding), [service-discovery.md](service-discovery.md)

### REQ-066 — Caller attribution for AI-facing audit

When the SDK is consumed by an AI-facing surface (MCP server, agent integration), it **SHOULD** support forwarding caller-attribution metadata (AI-agent identifier, model provider, optional trace correlation) to the backend so audit traces can record the AI-mediated provenance of an action. The mechanism **MUST** be transport-level (custom header or OTel attribute), opt-in, and **MUST NOT** be set automatically.

- **Rationale:** Platform-side audit requirements (REQ-083 in the Cadasto platform requirements catalogue) record the endpoint, context, model provider, and downstream action trace for AI-assisted actions. The SDK provides the carriage; the application provides the values.
- **Covered by:** [auth.md § AI caller attribution](auth.md#ai-caller-attribution)

### REQ-067 — Surface platform principal claims

When the token contains platform principal claims (typically `principal_uid` and `principal_type` carrying `PERSON` or `AGENT`), the SDK **MUST** surface them on `LaunchContext` (or its non-SMART equivalent) without coercion. The SDK **MUST NOT** assume principal type or invent values; missing claims surface as `nil` / zero.

- **Rationale:** Platforms model human and machine principals distinctly. The SDK is the consumer; it reflects what the token carries.
- **Covered by:** [auth.md § Platform principal claims](auth.md#platform-principal-claims)

### REQ-068 — SMART flow and launch-mode coverage

`auth/smart/` (in concert with `auth/clientcreds/` and `auth/jwtbearer/`) **MUST** support the four SMART grant flows: Authorization Code + PKCE, Authorization Code + client_secret, Client Credentials, JWT Bearer. The SDK **MUST** also support the three launch modes: **standalone**, **embedded** (iFrame), and **backend service**.

- **Covered by:** [auth.md § SMART flows](auth.md#smart-flows), [auth.md § Launch modes](auth.md#launch-modes)

---

## Service discovery

### REQ-070 — First-class discovery

SDK constructors **MUST** accept a `smart/discovery.ServiceCatalog` rather than a single base URL. For non-discovering backends (e.g. a static EHRbase deployment), consumers **MUST** be able to inject a hand-built catalog without depending on a discovery transport.

- **Covered by:** [service-discovery.md § ServiceCatalog](service-discovery.md#servicecatalog)

### REQ-071 — Discovery cache

Resolved service catalogs **MUST** be cached. The cache **MUST** honour TTL and conditional refresh (`ETag` / `If-None-Match`) and **MUST** be invalidated on `401`/`403` against a previously-working endpoint.

- **Covered by:** [service-discovery.md § Caching](service-discovery.md#caching)

### REQ-072 — Discovery validation

On resolution, the SDK **MUST** verify that the catalog advertises every service the client intends to use. `org.openehr.rest` **MUST** be present for any openEHR REST consumer; missing or version-incompatible services **MUST** fail fast with a typed `DiscoveryError`.

- **Covered by:** [service-discovery.md § Validation](service-discovery.md#validation)

---

## Cross-SDK conformance

### REQ-080 — Conformance probe parity

The conformance probe set defined in [conformance.md](conformance.md) **MUST** be implementable identically in both the Go SDK and the Cadasto PHP SDK against the same reference deployment. Probe IDs **MUST** be stable across languages.

- **Covered by:** [conformance.md § Probe catalog](conformance.md#probe-catalog)

### REQ-081 — Wire-level parity, not source-level

Cross-SDK parity is enforced at the **wire** level (HTTP request/response bytes, AQL string, JSON shape) — not at the source-code level. Per-language API idioms (Go's `context.Context` + functional options, PHP's repositories + exceptions) **MAY** diverge.

- **Covered by:** [conformance.md § Parity scope](conformance.md#parity-scope)

### REQ-082 — Probe runnability

Every probe **MUST** be runnable against (a) the sandbox transport (`sandbox/`), (b) a recorded cassette, and (c) a live Cadasto reference deployment, with a single source of truth for the probe definition.

- **Covered by:** [conformance.md § Runnability](conformance.md#runnability)

---

## Observability and reliability

### REQ-090 — OpenTelemetry hooks

`transport/` **MUST** expose OpenTelemetry hooks (trace span, attributes, propagation) on every outgoing request, and **MUST NOT** require consumers to use OTel — the absence of an OTel `TracerProvider` **MUST** be a silent no-op, not an error.

- **Covered by:** [wire.md § Observability hooks](wire.md#observability-hooks)

### REQ-091 — Retry policy

`transport/` **MUST** offer a default retry/backoff policy that is **off by default**. Enabling retries **MUST** be an explicit functional option; the retry budget and the set of retriable status codes **MUST** be configurable.

- **Rationale:** Benchmark and federator use cases need precise control over retry semantics; defaulting to "retry on" produces misleading latency tails.
- **Covered by:** [wire.md § Retry policy](wire.md#retry-policy)

### REQ-093 — openEHR error envelope mapping

The SDK **MUST** decode the openEHR REST structured error envelope (`{message, code, coded_text[]}` per ITS-REST 1.1.0-development) on non-2xx responses into a typed `transport.OpenEHRErrorDetail` attached to the corresponding `transport.WireError`. Consumers detect the error class via `errors.Is` and inspect the openEHR-specific detail via `errors.As`.

The mapping from HTTP status to typed sentinel (`ErrNotFound`, `ErrUnauthorized`, `ErrForbidden`, `ErrVersionConflict`, `ErrPreconditionFailed`, `ErrPreconditionRequired`) **MUST** be deterministic and documented in [idiom.md § Errors](idiom.md#errors).

- **Covered by:** [wire.md § Error envelope](wire.md#error-envelope), [idiom.md § Errors](idiom.md#errors)

### REQ-094 — `Prefer` response-shape negotiation

The SDK **MUST** support the openEHR REST `Prefer: return={minimal|identifier|representation}` header on write paths:

- **`minimal`** — server returns no body (just `Location` + `ETag`); the SDK's typed return value carries only the metadata.
- **`identifier`** — server returns only the new identifier; the SDK populates the identifier slot of the typed return value.
- **`representation`** — server returns the full new resource representation; the SDK populates the full typed return value.

The header **MUST** be exposed as a per-call option (typically `WithPrefer(Prefer)` where `Prefer` is a typed enum). Defaults: `minimal` for writes, `representation` for reads (where applicable).

- **Covered by:** [wire.md § `Prefer` negotiation](wire.md#prefer-negotiation)

### REQ-095 — OpenAPI as authoritative endpoint source

When per-endpoint shapes (paths, parameters, status codes, response bodies) disagree between [wire.md](wire.md), the SDK's plans, and the upstream openEHR OpenAPI YAML at `https://github.com/openEHR/specifications-ITS-REST/tree/master/computable/OAS`, the **OpenAPI YAML wins**. Plans and specs pin to a specific upstream commit; bumping that commit is an explicit, reviewable change.

- **Rationale:** the OpenAPI files are the openEHR Foundation's machine-readable form of the REST contract — the same role the BMM files play for the RM (REQ-041). Treating them as authoritative gives the SDK the same drift-detection discipline for the wire surface as for the type surface.
- **Covered by:** [wire.md § Authoritative source](wire.md#authoritative-source)

### REQ-092 — TLS posture

The SDK **MUST NOT** allocate its own `*http.Client` (REQ-021) and therefore **MUST NOT** dictate TLS policy. However, it **SHOULD** warn (via a configurable logger or returned error on construction) when it detects a clearly unsafe configuration: plaintext HTTP endpoints in `ServiceCatalog` entries marked as production, expired CA roots, or a discovery document fetched over `http://`.

- **Rationale:** Platform regulatory requirements call for TLS 1.3+ on every API path. The SDK is not the enforcement point, but it **MUST NOT** silently enable insecure traffic.
- **Covered by:** [wire.md § TLS posture](wire.md#tls-posture)

---

## Index by topic

| Topic | REQs |
|---|---|
| Module identity / packaging | 001, 002, 003, 004, 005 |
| Boundaries | 010, 011, 012, 013, 014 |
| Idiomatic surface | 020, 021, 022, 023, 024, 025, 026 |
| Reference Model | 030, 031, 032, 033, 040 |
| BMM conformance | 041, 042, 043, 044, 045, 046, 047 |
| Wire format | 050, 051, 052, 053, 054, 055, 056, 057, 058, 059 |
| Authentication | 060, 061, 062, 063, 064, 065, 066, 067, 068 |
| Service discovery | 070, 071, 072 |
| Cross-SDK conformance | 080, 081, 082 |
| Observability / reliability | 090, 091, 092 |
| REST binding | 059, 093, 094, 095 (cross-listed; wire-binding semantics) |

Number ranges leave headroom (10-99 per topic, decadal gaps between topics) so new requirements slot in without renumbering the catalog.
