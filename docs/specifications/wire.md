# Wire format

**Status:** Draft

The normative contract between the SDK and any conformant openEHR backend (Cadasto CDR, EHRbase, others). Covers REQ-050 through REQ-059 (wire surface and openEHR headers) and REQ-095 (OpenAPI authoritative source). Transport hygiene (REQ-090–094) lives in [transport.md](transport.md).

The premise: correctness is wire-level (REQ-080). The bytes on the wire and the AQL strings conform to the openEHR spec; the Go source shape is independent.

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

When formatting `openehr-item-tag` header values, the SDK **MUST** reject keys, values, and target paths that contain control characters (bytes `< 0x20` except tab, and `DEL` `0x7F`) — a caller-supplied tag key or value with embedded CR/LF is a caller error, not a sanitisation opportunity (header injection).

Response headers in this family **MUST** be surfaced on the typed response metadata returned by each method (alongside `ETag`, `Location`).

The SDK **MUST NOT** require consumers to construct the audit envelope by hand — `*rm.AuditDetails` is a generated RM type per REQ-042, serialised via canonical JSON / canonical XML at the codec boundary.

## `Prefer` negotiation and error envelope

REQ-094 (`Prefer`) and REQ-093 (structured error envelope) are normative in [transport.md](transport.md). Leaf clients under `openehr/client/*` consume them via `transport.Client`.

## Canonical JSON

### REQ-052

The SDK's primary write payload **MUST** be openEHR canonical JSON. Read payloads **MUST** be accepted in canonical JSON; FLAT and STRUCTURED inputs flow through codec conversion in `openehr/serialize`.

Canonical-JSON properties:

- Every RM type instance carries `_type`. The encoder **MUST** emit it; the decoder **MUST** consult the type registry (REQ-040).
- Field order **SHOULD** follow the openEHR canonical-JSON specification when one is published; until then the SDK **MUST** use this deterministic profile (see [`docs/plans/archive/2026-05-15-canonical-json-serialization.md`](../plans/archive/2026-05-15-canonical-json-serialization.md)):
  - `_type` is always the first key on every encoded concrete RM value.
  - Remaining object keys follow **BMM property declaration order** (the order code generation emits struct fields).
  - `Hash` (`map[K]V`) keys are serialized in **lexicographic key order** (independent of struct field order).
- Numbers, booleans, strings, arrays, objects are JSON-vanilla — no openEHR-flavoured encoding tricks.
- `DV_QUANTITY` magnitudes are emitted as JSON numbers, not strings, unless the spec mandates otherwise (some implementations have used strings to avoid float-precision loss; the SDK takes a position — see § Floating-point precision below).

### Floating-point precision

Numeric magnitudes are serialised as IEEE 754 double-precision JSON numbers. The SDK **MUST NOT** silently coerce a magnitude through `float32` or a similarly lossy intermediate. If a wire value exceeds JSON's number precision (rare in clinical data), the SDK **MUST** report this on decode as a typed error rather than silently rounding.

Some upstream producers (notably legacy CDR exporters) emit `Real` / `Integer` magnitudes as quoted decimal strings. The SDK adopts **asymmetric tolerance**: encode is strict (numbers only); decode accepts either a JSON number or a quoted decimal string. The full rule and its rationale live in [`docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md). The asymmetric profile is part of the openEHR wire contract this SDK follows (REQ-080).

Golden canonical-JSON composition inputs for codec and PROBE-030 live under `testkit/cassettes/compositions/` and `testkit/cassettes/rm/` (see [Vendored cassettes](conformance.md#vendored-cassettes-testkitcassettes)). Example: `compositions/BMI.json` for quoted-number magnitudes ([ADR 0004](../adr/0004-numeric-wire-tolerance.md)).

### Polymorphic substitution (SDK-GAP-11)

The openEHR RM permits Liskov substitution at every property slot: a slot whose declared type is `T` admits any concrete subtype of `T` as a runtime instance, by AOM `valid_value` semantics. Two cases the canonical-JSON codec **MUST** handle losslessly:

1. **Substitutable subtype in a concrete-typed slot.** When a property's declared type is itself concrete but has registered subtypes per the BMM `ancestors` graph (`LOCATABLE.name: DV_TEXT` admitting `DV_CODED_TEXT`, `EVENT_CONTEXT.health_care_facility: PARTY_IDENTIFIED` admitting `PARTY_RELATED`, etc.), the wire `_type` discriminator drives dispatch. The SDK surfaces this via **narrow Go interfaces** (`<Parent>Like` — `DVTextLike`, `PartyIdentifiedLike`, `AuditDetailsLike`, `DVURILike`, `ObjectRefLike`) generated by `bmmgen` from the ancestors graph; the wire decoder routes through `typereg.DecodeAs[<Like>]`.
2. **Generic type parameterised over an abstract bound.** When a generic class instantiates over an abstract bound (`DV_INTERVAL[T: DV_ORDERED]`), the field is dispatched via `typereg.DecodeAs[T]` at decode time; this handles both interface-T instantiations (`DVInterval[DVOrdered]` used by reference ranges) and concrete value-T instantiations (`DVInterval[DVQuantity]` used by `DVQuantity.NormalRange`).

**Missing-`_type` tolerance:** canonical JSON SHOULD carry `_type` everywhere, but real-world cassettes elide it on concrete-typed slots where the static field fixes the subtype (e.g. `"name": {"value": "Tree"}` on an `ITEM_TREE`). The decoder falls back to the **declared parent's concrete type** when the wire omits `_type` on a narrow-interface slot; this preserves backward compatibility with permissive producers without compromising the strict-abstract-slot rule (`DATA_VALUE`, `DV_ORDERED`, `ITEM_STRUCTURE`, `PARTY_PROXY` still require `_type`).

The full substitution semantics are pinned by [PROBE-038](conformance.md#probe-038--rm-polymorphic-decode-coverage-sdk-gap-11) (decode + re-marshal preserves every input `_type` discriminator). On BMM bumps that introduce new subtypes ([ADR 0001](../adr/0001-bmm-version-bump-runbook.md) step 10), `make codegen` auto-extends the relevant `<Parent>Like` interface (marker methods on the new concrete class); the closed type-switches in [`openehr/rm/like_accessors.go`](../../openehr/rm/like_accessors.go) still need an explicit `case *NewSubtype:` arm per new descendant, plus a round-trip case in [`openehr/serialize/canjson/polymorphic_decode_test.go`](../../openehr/serialize/canjson/polymorphic_decode_test.go), so PROBE-038's substitution guarantee covers it.

## Canonical XML

### REQ-056

The SDK **MUST** provide a canonical XML codec in `openehr/serialize`, symmetric to the canonical JSON codec — same type-registry consultation (REQ-040), same OPT-driven validation hooks, same independence from `transport/` (REQ-013).

Canonical XML applies to the same RM surface as canonical JSON: Composition, EHR_STATUS, Directory, Contribution, demographic resources. Polymorphic discrimination uses the `xsi:type` attribute (XML Schema Instance namespace), not the JSON `_type` property. Element names **MUST** be snake_case BMM names (same as canonical JSON keys). The codec **MUST** carry the namespace declarations the openEHR XML schemas require (`http://schemas.openehr.org/v1` default namespace; `xmlns:xsi` when `xsi:type` is present).

Canonical ordering for XML **MUST** mirror the JSON profile (see [`docs/plans/archive/2026-05-15-canonical-xml-serialization.md`](../plans/archive/2026-05-15-canonical-xml-serialization.md)):

- Child elements follow **BMM property declaration order** (same order code generation emits struct fields).
- `xsi:type` is the **first attribute** on every encoded concrete RM value where a polymorphic site is being resolved; the encoder emits it on every concrete value boundary (deterministic profile), the decoder requires it at polymorphic sites unless [`WithRelaxedTypeDispatch`] is set.
- Nil-pointer optional fields and empty containers with `cardinality.lower == 0` are emitted as **ABSENT** (no element). Both ABSENT and an empty self-closing element are accepted on decode.
- ISO 8601 dates/times/durations are passed through as element text content; the codec does not parse them at codec layer (REQ-046).
- Numeric magnitudes use IEEE 754 double-precision (same posture as canonical JSON); decode also accepts quoted decimal strings per [`docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md).
- Compact XML (no insignificant inter-element whitespace) is the byte-equality target for round-trip tests.
- `xmi:type` is **rejected** on decode with `ErrInvalidShape` and an explicit message — only `xsi:type` is recognised.

XML is a second-class format on the wire today (REST 1.1.0-development is JSON-first), but several integration scenarios pin to XML for legacy reasons. The SDK supports it without forcing it.

Golden canonical-XML inputs for codec and PROBE-033 live under `testkit/cassettes/compositions/` and `testkit/cassettes/rm/` (same layout as REQ-052; see [Vendored cassettes](conformance.md#vendored-cassettes-testkitcassettes)).

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

### Request vs response shape asymmetry

Two endpoints carry **distinct request and response shapes** that are easy to conflate because the RM ships only the persisted (response) form:

- **`POST /ehr/{ehr_id}/composition`** and **`PUT /ehr/{ehr_id}/composition/{vo_uid}`** (SDK-GAP-09, [PROBE-071](conformance.md#probe-071--composition-postput-response-body-is-bare-composition-sdk-gap-09)). Request body: a bare `COMPOSITION` payload. Response body under `Prefer: return=representation`: a bare `COMPOSITION` per ITS-REST `201_COMPOSITION` / `200_COMPOSITION_updated`, **not** the persisted `ORIGINAL_VERSION<COMPOSITION>` envelope. The persisted envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`). Same shape applies to `directory.Save` / `Update`.
- **`POST /ehr/{ehr_id}/contribution`** (SDK-GAP-10, [PROBE-072](conformance.md#probe-072--contribution-submission-body-matches-contribution_create-sdk-gap-10)). Request body: ITS-REST `Contribution_create` — `{audit, versions: [ORIGINAL_VERSION<T> with inline data: T]}` for `T ∈ {COMPOSITION, EHR_STATUS, FOLDER, EHR_ACCESS}`. Response body: persisted `CONTRIBUTION` whose `versions[]` is `[]OBJECT_REF` (the references the server assigned). A submission body shaped like the persisted `CONTRIBUTION` is rejected by spec-conformant CDRs because its `OBJECT_REF`s point at versions that do not yet exist.
  - **Commit-audit DTO asymmetry (SPECITS-95 / [ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131)).** The request-side commit audit (the batch `audit` and each version's `commit_audit`) is the `UPDATE_AUDIT` DTO, **not** the persisted `AUDIT_DETAILS`: it MUST omit the server-assigned `time_committed`, treats `system_id` as optional, and types `change_type` (and `UpdateVersion.lifecycle_state`) as `DV_CODED_TEXT` — never the withdrawn flat `TERMINOLOGY_CODE`. A client SHOULD send `_type:"UPDATE_AUDIT"`; servers SHOULD accept `AUDIT_DETAILS` or an omitted `_type`. The Go SDK emits `AUDIT_DETAILS` by default (`contribution.UpdateAudit`) and exposes `AuditType` to fall back to `UPDATE_AUDIT` for non-conformant servers.

Implementations **MUST NOT** serialise the persisted shape on either submission path. The Go SDK enforces this via [`contribution.Submission`](../../openehr/client/ehr/contribution/submission.go) (distinct from `rm.Contribution`) and the composition / directory write surfaces that take bare RM types.

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

**AQL injection.** `ExecuteString` (raw AQL escape hatch) **MUST** be documented as unsafe for interpolating caller-supplied values into the query text — bind parameters via the typed `params` map (named placeholders the CDR binds server-side). String-built AQL from untrusted input is injectable.

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

## Transport cross-cutting concerns

REQ-090 (OpenTelemetry), REQ-091 (retry), REQ-092 (TLS posture), REQ-093 (error envelope), and REQ-094 (`Prefer`) are specified in [transport.md](transport.md).

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
| OpenAPI authoritative source | REQ-095 | `testkit/cassettes/its_rest/` (records upstream commit) |
| Shared RM / OPT cassettes | REQ-052, REQ-056, REQ-082 | `testkit/cassettes/{templates,compositions,rm}/` — resolve via `testkit/fixtures/`; index in [`testkit/cassettes/README.md`](../../testkit/cassettes/README.md) |
| Transport (OTel, retry, TLS, errors, Prefer) | REQ-090–094 | [transport.md](transport.md) → `transport/`, `smart/discovery/` |
