# Wire format

**Status:** Draft

The normative contract between the SDK and any conformant openEHR backend (Cadasto CDR, EHRbase, others). Covers REQ-050 through REQ-059 (wire surface and headers), REQ-090 / REQ-091 / REQ-092 (transport hygiene), REQ-093 / REQ-094 / REQ-095 (REST binding: error envelope, `Prefer` negotiation, OpenAPI authoritative source).

The premise: cross-SDK parity is wire-level (REQ-081). Source-level idioms diverge between Go and PHP SDKs; the bytes on the wire and the AQL strings are identical.

## Functional API areas

openEHR REST 1.1.0-development partitions the surface into **six** functional areas. The SDK provides a typed leaf package per area under `openehr/client/`:

| Area | Scope | SDK package |
|---|---|---|
| **System** | service capabilities, version, infrastructure discovery | `openehr/client/system/` |
| **EHR** | EHR + sub-resources: Composition, Contribution, Directory/Folder, EHR_STATUS, ItemTags | `openehr/client/ehr/` (+ sub-leaves) |
| **Query** | ad-hoc AQL execution; stored-query invocation; `RESULT_SET` shape | `openehr/client/query/` |
| **Definition** | ADL2/ADL1.4 archetypes, OPTs/templates, example generation, stored queries | `openehr/client/definition/` |
| **Demographic** | parties, relationships, identities (upstream Status: development) | `openehr/client/demographic/` |
| **Admin** | EHR physical delete, administrative lifecycle (upstream Status: development) | `openehr/client/admin/` |

The split is normative: a consumer who needs only AQL imports `openehr/client/query/` without pulling in the EHR or Definition surface (REQ-013).

## Authoritative source

### REQ-095

For per-endpoint detail (paths, parameters, request/response schemas, status codes), the **upstream openEHR OpenAPI YAML files** are authoritative — they are the openEHR Foundation's machine-readable form of the REST contract, the analogue of the BMM files (REQ-041) for the type surface.

Pinned source: `https://github.com/openEHR/specifications-ITS-REST/tree/master/computable/OAS`. The SDK's plans and test cassettes record the upstream commit they were validated against; bumping it is an explicit, reviewable change.

When the OpenAPI files and any in-repo prose disagree, the OpenAPI wins; the prose is updated, not the wire behaviour.

## REST version pin

### REQ-050

The SDK targets **openEHR REST `1.1.0-development`** as its primary contract surface. Concretely:

- Endpoint shapes, request/response envelopes, and status-code semantics follow the 1.1.0-development specification.
- The `Cadasto-OpenEhr-Spec-Version` header (REQ-051) is the wire signal that distinguishes Cadasto deployments from generic openEHR backends; it does not change the request bodies or paths.
- Earlier versions (1.0.3, 1.0.2) are not targeted. A consumer who needs to talk to a 1.0.3 deployment can do so against a build of the SDK that has the 1.0.3 quirks documented; v1 does not promise backwards compatibility across REST major/minor versions.

The version pin is enforced at discovery time (REQ-072), not on the first request. A mismatched advertised version **MUST** fail fast with a typed `DiscoveryError`.

## Cadasto spec-version header

### REQ-051

The SDK **MAY** send a `Cadasto-OpenEhr-Spec-Version` header on outgoing requests:

```
Cadasto-OpenEhr-Spec-Version: 1.1.0-development
```

This is a **Cadasto-platform-specific** signal. The SDK **MUST**:

- Keep the header **off by default**.
- Enable it only when a Cadasto deployment is detected (e.g. via the discovery document) or when a functional option (`transport.WithCadastoSpecVersionHeader(true)`) is set.
- Strip the header from any cross-origin (CORS) preflight if browsers are in the request path (the SDK is not directly used in a browser, but the rule documents the intent).

Sending this header to a non-Cadasto openEHR backend **MUST NOT** happen automatically — generic backends may reject unknown headers depending on policy.

## openEHR custom header family

### REQ-059

openEHR REST 1.1.0-development defines a family of `openehr-*` custom headers carrying RM and template-level metadata at the wire layer. The SDK **MUST** support them via **typed per-call options**, never via raw header maps on the consumer's surface.

| Header | Direction | Carries | Typed option |
|---|---|---|---|
| `openehr-version` | request | explicit RM version negotiation for this request | `transport.WithRMVersion("1.1.0")` |
| `openehr-audit-details` | request (writes) | commit-time audit envelope: committer, time, change-type | `transport.WithAuditDetails(*rm.AuditDetails)` |
| `openehr-template-id` | request (composition writes) | declares the template id the payload conforms to | `composition.WithTemplateID(string)` |
| `openehr-uri` | request / response | opaque openEHR resource pointer (selected endpoints) | typed on the affected method |
| `openehr-item-tag` | request / response | ItemTag operations (REST 1.1.0 new resource) | exposed on `openehr/client/ehr/itemtags/` |

Response headers in this family **MUST** be surfaced on the typed response metadata returned by each method (alongside `ETag`, `Location`).

The SDK **MUST NOT** require consumers to construct the audit envelope by hand — `*rm.AuditDetails` is a generated RM type per REQ-042, serialised via canonical JSON / canonical XML at the codec boundary.

## `Prefer` negotiation

### REQ-094

openEHR REST 1.1.0-development uses the standard HTTP `Prefer: return=<mode>` header to negotiate the response body on write paths. The SDK **MUST** support the three documented modes:

| Mode | Server response | SDK return |
|---|---|---|
| `minimal` | empty body; `Location` + `ETag` | typed return value carries only metadata (`*VersionMetadata`) |
| `identifier` | body contains identifier only | typed return value populated to the identifier slot |
| `representation` | body contains the full new resource | typed return value populated fully |

Rules:

- Per-call option: `transport.WithPrefer(Prefer)`. `Prefer` is a typed enum, not a raw string, to keep call sites compile-checked.
- **Defaults:** writes default to `minimal` (matches the CDR benchmark's POST-then-GET pattern and minimises wire bandwidth); reads default to `representation` (no Prefer header sent — `representation` is the natural response).
- The SDK **MUST NOT** silently downgrade `representation` to `minimal` when the server omits the body; that is a server bug and surfaces as `ErrInvalidShape` on decode.

## Error envelope

### REQ-093

openEHR REST 1.1.0-development returns a structured JSON body on non-2xx responses:

```json
{
  "message": "Composition violates template constraints at /content[1]/...",
  "code": "VALIDATION_FAILED",
  "coded_text": [
    {"terminology_id": {"value": "openehr"}, "code_string": "..."}
  ]
}
```

The SDK **MUST**:

- Decode this envelope on every non-2xx response (best-effort; missing fields default to zero values).
- Attach the parsed envelope to `transport.WireError` as `OpenEHRErrorDetail{Message, Code, CodedText}`.
- Map the HTTP status to the typed sentinel per [idiom.md § Errors](idiom.md#errors): `ErrNotFound` (404), `ErrUnauthorized` (401), `ErrForbidden` (403), `ErrVersionConflict` (409), `ErrPreconditionFailed` (412), `ErrPreconditionRequired` (428).
- Preserve the raw response body for diagnostics: `WireError.RawBody` is accessible for logging / forensic use.

Consumers check the error class with `errors.Is(err, transport.ErrVersionConflict)` and reach the openEHR-specific detail with:

```go
var wire *transport.WireError
if errors.As(err, &wire) && wire.OpenEHR != nil {
    log.Printf("openEHR error code=%s message=%s", wire.OpenEHR.Code, wire.OpenEHR.Message)
}
```

## Canonical JSON

### REQ-052

The SDK's primary write payload **MUST** be openEHR canonical JSON. Read payloads **MUST** be accepted in canonical JSON; FLAT and STRUCTURED inputs flow through codec conversion in `openehr/serialize`.

Canonical-JSON properties:

- Every RM type instance carries `_type`. The encoder **MUST** emit it; the decoder **MUST** consult the type registry (REQ-040).
- Field order **SHOULD** follow the openEHR canonical-JSON specification when one is published; until then the SDK **MUST** use this deterministic profile (see [`docs/plans/2026-05-15-canonical-json-serialization.md`](../docs/plans/2026-05-15-canonical-json-serialization.md)):
  - `_type` is always the first key on every encoded concrete RM value.
  - Remaining object keys follow **BMM property declaration order** (the order code generation emits struct fields).
  - `Hash` (`map[K]V`) keys are serialized in **lexicographic key order** (independent of struct field order).
- Numbers, booleans, strings, arrays, objects are JSON-vanilla — no openEHR-flavoured encoding tricks.
- `DV_QUANTITY` magnitudes are emitted as JSON numbers, not strings, unless the spec mandates otherwise (some implementations have used strings to avoid float-precision loss; the SDK takes a position — see § Floating-point precision below).

### Floating-point precision

Numeric magnitudes are serialised as IEEE 754 double-precision JSON numbers. The SDK **MUST NOT** silently coerce a magnitude through `float32` or a similarly lossy intermediate. If a wire value exceeds JSON's number precision (rare in clinical data), the SDK **MUST** report this on decode as a typed error rather than silently rounding.

Some upstream producers (notably legacy CDR exporters) emit `Real` / `Integer` magnitudes as quoted decimal strings. The SDK adopts **asymmetric tolerance**: encode is strict (numbers only); decode accepts either a JSON number or a quoted decimal string. The full rule and its rationale live in [`docs/adr/0004-numeric-wire-tolerance.md`](../docs/adr/0004-numeric-wire-tolerance.md). Cross-SDK parity (REQ-081) requires every SDK to follow the same asymmetric profile.

## Canonical XML

### REQ-056

The SDK **MUST** provide a canonical XML codec in `openehr/serialize`, symmetric to the canonical JSON codec — same type-registry consultation (REQ-040), same OPT-driven validation hooks, same independence from `transport/` (REQ-013).

Canonical XML applies to the same RM surface as canonical JSON: Composition, EHR_STATUS, Directory, Contribution, demographic resources. Polymorphic discrimination uses the `xsi:type` attribute (XML Schema Instance namespace), not the JSON `_type` property. Element names **MUST** be snake_case BMM names (same as canonical JSON keys). The codec **MUST** carry the namespace declarations the openEHR XML schemas require (`http://schemas.openehr.org/v1` default namespace; `xmlns:xsi` when `xsi:type` is present).

Canonical ordering for XML **MUST** mirror the JSON profile where applicable (see [`docs/plans/2026-05-15-canonical-xml-serialization.md`](../docs/plans/2026-05-15-canonical-xml-serialization.md)): child elements in BMM declaration order; `xsi:type` first among attributes when present; compact XML (no insignificant inter-element whitespace) is the byte-equality target for round-trip tests.

XML is a second-class format on the wire today (REST 1.1.0-development is JSON-first), but several integration scenarios pin to XML for legacy reasons. The SDK supports it without forcing it.

## Simplified formats

### REQ-053

The SDK **MUST** provide codecs for the openEHR **FLAT** and **STRUCTURED** simplified formats in `openehr/serialize`:

- **FLAT** — path-keyed, value-flat. Keys are openEHR paths (`/content[openEHR-EHR-OBSERVATION.foo.v1]/data[at0001]/events[at0002]/data[at0003]/items[at0004]/value/magnitude`); values are leaf scalars.
- **STRUCTURED** — nested JSON that preserves RM structure but omits `_type` where the path is unambiguous (because the template constrains the type).

Both codecs **MUST**:

- Be usable independently of the HTTP client (REQ-013) — feeding a FLAT JSON file to a FLAT-to-canonical converter is a valid standalone use case.
- Round-trip cleanly when the source artifact is OPT-aware (FLAT/STRUCTURED ↔ canonical conversion requires the OPT to resolve ambiguous paths and missing `_type`s).
- Report missing OPT context as a typed error when conversion cannot proceed without it.

## ITS-REST envelopes

The openEHR REST 1.1.0-development specification defines envelope shapes for typical responses (collections, errors, version metadata). The SDK **MUST**:

- Decode well-formed envelopes into typed Go structs.
- Surface envelope-level metadata (e.g. paging hints, conformance hints) on the typed response, not as a parallel `map[string]any`.
- Reject malformed envelopes with a typed `WireError` carrying the parse failure.

The exact envelope shapes are openEHR REST 1.1.0-development; this spec does not re-define them, it pins to them.

## AQL

### REQ-055 — Wire boundary

`openehr/aql` ships two builder styles:

- **Struct-builder.** Build an `aql.Query` value by composing typed structs.
- **Verb-functions.** Compose a query via top-level functions (`aql.Select(...)`, `aql.From(...)`, `aql.Where(...)`).

Both styles **MUST** produce the **same AQL string on the wire** for the same logical query. Concrete rules:

- The serialised AQL string is the wire contract. Two queries that produce the same string are equivalent; two that produce different strings are different queries, even if logically equivalent.
- Whitespace, casing, and aliasing are subject to canonicalisation in the wire-output path — the SDK **MUST** produce a stable, canonicalised string so wire-output goldens are deterministic.
- Both styles are tested against the same wire-output golden cassettes; a builder change that produces different output is a breaking change.

### AQL executor

`openehr/client/query` is the AQL executor. It:

- Accepts a built `aql.Query` (or a raw AQL string for advanced use).
- Sends it as an openEHR REST `POST /query/aql` (or `GET /query/aql/{queryId}` for stored queries).
- Decodes the response: `meta`, `columns`, `rows`. Row values are typed via generics where the caller pre-declares column types; otherwise they decode to `any` and the call site casts.
- Surfaces AQL-level errors (parse, path resolution) as typed errors distinct from generic `WireError`.

### Stored AQL

### REQ-057

The platform supports **stored AQL queries** — queries registered ahead of time and executed by ID. The SDK **MUST** support both ends:

- **`openehr/client/definition/`** — register, list, get, delete stored queries. Operations map to the openEHR REST Definition API.
- **`openehr/client/query/`** — execute a stored query by ID (`GET /query/{qualified_query_name}`) in addition to the ad-hoc execution path.

A stored query is identified by a qualified name (typically reverse-DNS, e.g. `org.example.queries.recent-observations`); the SDK passes it through verbatim. Stored queries are expected to be faster than ad-hoc AQL on the same backend (materialised read models, known output schemas), but the SDK does not pre-validate the qualified name — that's the backend's responsibility.

## Optimistic concurrency

### REQ-054

openEHR versioned resources (Composition, EHR_STATUS, Directory, Contribution) are versioned by `version_uid` with `If-Match` / `ETag` optimistic concurrency on the wire.

Rules the SDK **MUST** enforce:

- A PUT against a versioned resource **MUST** include `If-Match: "<preceding_version_uid>"` (the canonical form per the openEHR REST envelope). Omitting it **MUST** result in `428 Precondition Required` from the backend; the SDK **MUST NOT** retry without an `If-Match`.
- A `409 Conflict` response (stale `If-Match`) **MUST** map to `transport.ErrVersionConflict`.
- A `412 Precondition Failed` response **MUST** map to `transport.ErrPreconditionFailed`.
- A `428 Precondition Required` response **MUST** map to `transport.ErrPreconditionRequired`.
- The SDK **MUST NOT** synthesise these statuses client-side — they come from the backend.

ETag handling on reads is symmetric: the SDK **MUST** capture `ETag` from a response and expose it on the typed return value so the caller can use it for the next PUT.

## Observability hooks

### REQ-090

`transport/` **MUST** expose OpenTelemetry hooks:

- **Spans.** Every outgoing request opens an OTel span named `<METHOD> <route_template>` (e.g. `GET /ehr/{ehr_id}`). The span **MUST** carry attributes: HTTP method, URL (sanitised — no tokens), status code, response size, `openehr.spec_version`, `openehr.resource_type` (where applicable).
- **Propagation.** Trace context **MUST** be propagated outbound via the standard W3C `traceparent` / `tracestate` headers (using the OTel `propagation` API).
- **No-op safety.** The absence of a `TracerProvider` in the context **MUST** be a silent no-op — the SDK **MUST NOT** require an OTel setup to function.

Metrics and logs **MAY** be added later (request count, request duration histogram, retry attempts) once the OTel SDK stabilises a metrics surface and the SDK has a benchmarked basis for which metrics to emit.

## TLS posture

### REQ-092

The SDK does not allocate its own `*http.Client` (REQ-021), so TLS configuration is the consumer's responsibility. However, the SDK **SHOULD**:

- Emit a warning (via a configurable logger or a typed result on construction) when a `ServiceCatalog` entry's `BaseURL` uses plaintext `http://` and the entry is not explicitly marked `Insecure: true`.
- Emit a warning when the SMART discovery document is fetched over `http://`.
- Default the *opt-in* discovery fetcher (when the SDK does its own fetching rather than receiving a hand-built catalog) to refusing plaintext URLs unless `discovery.WithAllowInsecure()` is set.

The SDK **MUST NOT** silently override or relax the consumer's `*http.Client` TLS config. A consumer who wants to talk to a local development backend over plaintext does so explicitly; production deployments fail visibly.

## Retry policy

### REQ-091

`transport/` **MUST** offer a default retry / backoff policy that is **off by default**. Enabling retries **MUST** be an explicit functional option:

```go
client, err := transport.New(catalog,
    transport.WithRetry(retry.Policy{
        MaxAttempts:     3,
        InitialBackoff:  100 * time.Millisecond,
        MaxBackoff:      5 * time.Second,
        Multiplier:      2.0,
        RetriableStatus: []int{502, 503, 504},
    }),
)
```

Rules:

- Retries **MUST NOT** be enabled by default for **any** method. Benchmarks and federators need clean latency tails.
- Retries **MUST NOT** be applied to non-idempotent methods (POST, PATCH, DELETE-with-side-effects) unless the consumer explicitly opts in per call.
- Retries **MUST** respect `ctx` cancellation — a cancelled context aborts retry waits immediately.
- The retry budget **MUST** be observable via the OTel span (`retry.attempt`, `retry.backoff_ms`).

## Streaming and large payloads

Out of v1 scope:

- Streaming response bodies. v1 reads complete responses into memory. Streaming **MAY** be added when a documented consumer needs it.
- Multipart uploads beyond what openEHR REST 1.1.0-development requires.
- Range requests, partial reads.

## Coverage matrix

| Topic | REQ | Lives in |
|---|---|---|
| REST 1.1.0-dev pin | REQ-050 | `transport/`, `openehr/client/*` |
| Cadasto spec-version header | REQ-051 | `transport/` |
| Canonical JSON | REQ-052 | `openehr/serialize/canjson/` |
| Canonical XML | REQ-056 | `openehr/serialize/canxml/` |
| FLAT / STRUCTURED | REQ-053 | `openehr/serialize/` (deferred sub-packages) |
| Optimistic concurrency | REQ-054 | `transport/` (error mapping), `openehr/client/*` (header plumbing) |
| AQL wire | REQ-055 | `openehr/aql/`, `openehr/client/query/` |
| Stored AQL | REQ-057 | `openehr/client/definition/`, `openehr/client/query/` |
| openEHR custom header family | REQ-059 | `transport/` (option API), `openehr/client/*` (typed per-method options) |
| Error envelope mapping | REQ-093 | `transport/` (decoding), `openehr/client/*` (typed-error propagation) |
| `Prefer` negotiation | REQ-094 | `transport/` (option), `openehr/client/*` (per-write default) |
| OpenAPI authoritative source | REQ-095 | `testkit/cassettes/its_rest/` (records upstream commit) |
| Observability | REQ-090 | `transport/` |
| Retry policy | REQ-091 | `transport/` |
| TLS posture | REQ-092 | `transport/`, `smart/discovery/` |
