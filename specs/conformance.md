# Conformance probes

**Status:** Draft

The cross-SDK contract that enforces wire-level parity between this Go SDK and the Cadasto PHP SDK. Covers REQ-080, REQ-081, REQ-082.

A **conformance probe** is an executable assertion that the SDK exercises against either:

- the sandbox transport (`sandbox/`),
- a recorded HTTP cassette, or
- a live Cadasto reference deployment,

to verify wire-level conformance to openEHR REST + SMART-on-openEHR.

Each probe has a stable `PROBE-NNN` ID. Probes are identical across the Go and PHP SDKs: same ID, same assertion, same pass/fail outcome against a reference deployment.

## Parity scope

### REQ-080 — Probe parity

The probe set defined here **MUST** be implementable identically in both SDKs. Concrete implications:

- A probe's **assertion** is wire-level: the HTTP request bytes (method, path, headers, body), the response status, the response body shape.
- A probe's **identifier** is shared across languages.
- A probe's **definition** lives once (in this spec) and is implemented separately in each SDK's test suite.
- A probe **MUST NOT** assert on source-level idioms (function names, error types, exception classes).

### REQ-081 — Wire-level parity, not source-level

The PHP SDK uses repositories + exceptions; the Go SDK uses package functions + typed errors. Both are correct; the probe set does not care which.

What the probe set **does** care about:

- The wire-format HTTP request is byte-identical when comparing equivalent calls.
- The AQL string emitted by the AQL builder is identical.
- The error class that surfaces to the application maps identically (a `412` produces "precondition failed" in both, however that's named in each language).

### REQ-082 — Runnability

Every probe **MUST** be runnable in three modes:

| Mode | Backend | Use |
|---|---|---|
| **Sandbox** | `sandbox/` in-memory transport | Fast unit tests; CI default |
| **Cassette** | Recorded `.har` or `.yaml` fixture | Deterministic CI against captured real-deployment traffic |
| **Live** | A reference Cadasto deployment | Pre-release verification; cross-SDK parity confirmation |

The probe definition is the single source; the runner picks the backend at invocation time. The same probe MUST pass in all three modes (with cassette recording done once against the live backend).

## Probe catalog

The catalog is the normative list. Each entry has:

- **ID** — stable, never renumbered.
- **Title** — one-line description.
- **Preconditions** — what state the system must be in.
- **Wire assertion** — what's checked at the byte / status level.
- **Modes** — Sandbox / Cassette / Live.
- **Status** — Draft (in this spec), Implemented (in code), Ratified (cross-SDK pass against reference).

### Authentication and discovery

#### PROBE-001 — Discovery declares `code+pkce`

- **Title:** SMART configuration document declares `code` response type and `S256` PKCE method.
- **Preconditions:** A SMART-on-openEHR deployment is reachable.
- **Wire assertion:** GET `<issuer>/.well-known/smart-configuration` (or equivalent) returns 200 with a JSON body containing `"response_types_supported"` including `"code"` and `"code_challenge_methods_supported"` including `"S256"`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-002 — Discovery advertises `org.openehr.rest`

- **Title:** Service catalog includes the openEHR REST service with a parseable base URL and a declared spec version.
- **Preconditions:** SMART discovery resolved.
- **Wire assertion:** The discovery document's service catalog contains an entry with id `"org.openehr.rest"`, a parseable `base_url`, and a `spec_version` field.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-003 — Spec-version mismatch fails fast

- **Title:** A discovery document advertising an incompatible spec version is rejected at resolution, not at first request.
- **Preconditions:** SDK is configured to require `1.1.0-development`; deployment advertises `1.0.3`.
- **Wire assertion:** Construction-time discovery returns a `DiscoveryError` with reason `spec_version_mismatch`. No request to the openEHR REST endpoint is made.
- **Modes:** Sandbox, Cassette (constructed-mismatch cassette).
- **Status:** Draft.

#### PROBE-004 — PKCE verifier round-trip

- **Title:** A SMART launch using `S256` PKCE successfully exchanges code for token.
- **Preconditions:** Deployment registers the SDK as a SMART app with PKCE required.
- **Wire assertion:** Authorization request carries `code_challenge` and `code_challenge_method=S256`; token exchange carries `code_verifier`; token response is 200 with an `access_token`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-005 — Scope round-trip

- **Title:** Configured openEHR scope (`<compartment>/<resource>.<permission>`) survives token exchange and lands in the JWT scope claim or the response `scope` field.
- **Preconditions:** Scope `patient/COMPOSITION.read` is requested.
- **Wire assertion:** Authorization request `scope` parameter contains `patient/COMPOSITION.read`; token response `scope` field contains it (or the JWT `scope` claim does).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-008 — Platform principal claims surface verbatim

- **Title:** When the token carries `principal_uid` and `principal_type` claims (per REQ-067), the SDK surfaces them on `LaunchContext.Principal` without coercion.
- **Preconditions:** A token with `principal_uid = "u-123"`, `principal_type = "AGENT"`.
- **Wire assertion:** SDK exposes `LaunchContext.Principal = {UID: "u-123", Type: PrincipalTypeAgent}`. Missing claims surface as nil/zero, not as guessed defaults.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

#### PROBE-009 — Caller attribution forwarded on opt-in

- **Title:** When caller attribution is configured (REQ-066), the SDK emits the configured header and OTel attributes; when not configured, no attribution data appears on the wire.
- **Preconditions:** One client with `WithCallerAttribution(...)`, one without.
- **Wire assertion:** Configured client emits the `X-Cadasto-Caller-Attribution` header and `caller.agent_id` OTel attribute; unconfigured client emits neither.
- **Modes:** Sandbox.
- **Status:** Draft.

#### PROBE-006 — JWKS rotation transparent to caller

- **Title:** A signing-key rotation on the authorization server triggers exactly one JWKS refresh in the SDK; subsequent requests succeed without consumer intervention.
- **Preconditions:** A cached JWKS does not contain the `kid` of the issued token (simulating rotation).
- **Wire assertion:** SDK fetches JWKS once, validates the token, and proceeds. No double-refresh, no double-validation failure surfaced.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

#### PROBE-007 — Token refresh transparent to caller

- **Title:** An expired access token with a valid refresh token is refreshed silently before the next request.
- **Preconditions:** Cached token has `expires_at < now`; refresh token is valid.
- **Wire assertion:** Token endpoint receives `grant_type=refresh_token`; the next outgoing request carries the new access token.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

### Versioned writes and optimistic concurrency

#### PROBE-010 — PUT Composition without If-Match

- **Title:** A PUT against a versioned Composition without an `If-Match` header is rejected with `428 Precondition Required`.
- **Preconditions:** An existing Composition with a known `version_uid`.
- **Wire assertion:** PUT `/ehr/{ehr_id}/composition/{versioned_object_id}` without `If-Match` returns `428`; the SDK maps this to `transport.ErrPreconditionRequired`. The Go SDK additionally short-circuits empty `ifMatch` at the call site with `transport.ErrInvalidConfig` per the typed-write-path guard.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_010_put_without_if_match.go`](../testkit/probes/versioned/probe_010_put_without_if_match.go).

#### PROBE-011 — PUT Composition with stale If-Match

- **Title:** A PUT with a stale `If-Match` (referencing an old version_uid) is rejected with `412 Precondition Failed` or `409 Conflict` depending on backend convention.
- **Preconditions:** Composition has been updated since the SDK's cached `version_uid`.
- **Wire assertion:** PUT returns `412` or `409`; SDK maps to `ErrPreconditionFailed` or `ErrVersionConflict` accordingly.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_011_put_stale_if_match.go`](../testkit/probes/versioned/probe_011_put_stale_if_match.go).

#### PROBE-012 — ETag survives round trip

- **Title:** A GET Composition followed by a PUT with the captured `ETag` as `If-Match` succeeds.
- **Preconditions:** Read-then-write workflow.
- **Wire assertion:** GET response carries `ETag`; PUT carries the same value as `If-Match`; PUT returns `204` or `200`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_012_etag_round_trip.go`](../testkit/probes/versioned/probe_012_etag_round_trip.go).

#### PROBE-013 — Cross-EHR isolation

- **Title:** A `version_uid` belonging to EHR A cannot be read via EHR B's path.
- **Preconditions:** Two distinct EHRs; a Composition known to belong to EHR A.
- **Wire assertion:** GET `/ehr/{ehr_b_id}/composition/{version_uid_from_a}` returns `404 Not Found`, never `200`, never the EHR A data.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

### AQL

#### PROBE-020 — AQL builder string stability

- **Title:** The struct-builder and verb-functions produce byte-identical AQL strings for the same logical query.
- **Preconditions:** A reference query (e.g. "all OBSERVATIONs of archetype foo for a given EHR").
- **Wire assertion:** `aql.NewQuery(...).String()` and `aql.From(...).Select(...).String()` are equal, byte for byte, for the reference query.
- **Modes:** Sandbox (no network).
- **Status:** Draft.

#### PROBE-021 — AQL parse error mapping

- **Title:** A syntactically invalid AQL string produced by a typed builder is impossible; a syntactically valid but semantically invalid one produces a typed `AQLError` on execution.
- **Preconditions:** Reference deployment that validates AQL against templates.
- **Wire assertion:** Execution of a query referencing a non-existent path returns the backend's AQL error envelope; SDK maps to `aql.ErrPathResolution`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

### Canonical JSON and formats

#### PROBE-030 — Canonical-JSON round trip

- **Title:** Decoding a canonical-JSON Composition and re-encoding produces byte-identical output (modulo documented field ordering).
- **Preconditions:** A reference Composition cassette.
- **Wire assertion:** `serialize.Decode → struct → serialize.Encode` produces output that, after the SDK's canonical-ordering pass, matches the input.
- **Modes:** Sandbox (no network).
- **Status:** Draft.

#### PROBE-031 — `_type` discriminator decoded via registry

- **Title:** A `_type` not in the type registry decodes to a typed `UnknownTypeError`, not silently to `map[string]any`.
- **Preconditions:** A cassette containing an unregistered `_type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown `_type` value.
- **Modes:** Sandbox.
- **Status:** Draft.

#### PROBE-033 — Canonical-XML round trip

- **Title:** Decoding a canonical-XML Composition and re-encoding produces byte-identical compact XML (modulo documented element/attribute ordering).
- **Preconditions:** A reference Composition XML cassette.
- **Wire assertion:** `canxml.Unmarshal → struct → canxml.Marshal` produces output that matches the input after the SDK's compact-XML canonicalisation pass.
- **Modes:** Sandbox (no network).
- **Status:** Draft.

#### PROBE-034 — `xsi:type` discriminator decoded via registry

- **Title:** An `xsi:type` not in the type registry decodes to `typereg.ErrUnknownType`, not silently to an untyped value.
- **Preconditions:** A cassette containing an unregistered `xsi:type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown type value.
- **Modes:** Sandbox.
- **Status:** Draft.

#### PROBE-032 — FLAT → canonical → FLAT round trip

- **Title:** Given an OPT and a FLAT payload, converting FLAT → canonical and back to FLAT produces the original FLAT payload (modulo documented OPT-driven normalisation).
- **Preconditions:** A reference OPT + FLAT pair.
- **Wire assertion:** Round-trip equality after OPT-driven normalisation.
- **Modes:** Sandbox.
- **Status:** Draft.

### Service discovery

#### PROBE-040 — Catalog cache honours TTL

- **Title:** Two SDK constructions within the TTL window of a cached catalog do not produce a second discovery fetch.
- **Preconditions:** Catalog with declared TTL > 0; two constructions in quick succession.
- **Wire assertion:** Exactly one discovery fetch occurs.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

#### PROBE-041 — Catalog refresh on 401

- **Title:** A `401` from a previously-working endpoint triggers exactly one discovery refresh and one retry; failure to recover surfaces a typed error.
- **Preconditions:** Cached catalog; backend rotates and returns `401` on the cached token.
- **Wire assertion:** SDK refreshes JWKS/catalog once, retries once. On second `401`, returns `transport.ErrUnauthorized`.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

### REST binding

The REST-binding probes assert the openEHR-REST 1.1.0-development wire contract above `transport/` and the typed leaf clients under `openehr/client/`. PROBE-040 and PROBE-041 are taken by the service-discovery range; the REST-binding range starts at PROBE-060 (next free range after Observability 050–059) per the [Adding probes](#adding-probes) rule.

#### PROBE-060 — EHR creation round-trip

- **Title:** `POST /ehr` with an initial `EHR_STATUS` body returns `201`, surfaces the assigned `ehr_id`, and a follow-up `GET` returns the same status.
- **Preconditions:** Backend supports server-assigned `ehr_id`.
- **Wire assertion:** POST returns `201` with `Location` header; SDK extracts `ehr_id`; a subsequent GET returns the same EHR_STATUS.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-061 — Composition versioned write with `Prefer: return=representation`

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` returns the full `ORIGINAL_VERSION<COMPOSITION>` plus a new `ETag`.
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Request carries `Prefer: return=representation`; response body decodes as `ORIGINAL_VERSION<COMPOSITION>`; response `ETag` is captured into `VersionMetadata`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-062 — `openehr-audit-details` header round-trip

- **Title:** A write carrying `openehr-audit-details` is reflected in the resulting Contribution's audit envelope on read-back.
- **Preconditions:** Existing EHR; a known `*rm.AuditDetails` value.
- **Wire assertion:** Write request carries `openehr-audit-details: <canonical-JSON>`; subsequent Contribution GET returns the same audit fields (committer name, time-committed, change-type).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-063 — Discovery-routed request

- **Title:** The transport resolves its base URL from `ServiceCatalog`'s `org.openehr.rest` entry, not from a hard-coded value.
- **Preconditions:** Catalog with `org.openehr.rest.base_url = "https://override.example/openehr/v1"`.
- **Wire assertion:** A request made via the leaf client targets `https://override.example/openehr/v1/...`, not the SDK default.
- **Modes:** Sandbox.
- **Status:** Draft.

#### PROBE-064 — Per-request `auth.TokenSource` overrides client default

- **Title:** A `TokenSource` attached to `ctx` via `auth.WithTokenSource` overrides the client-default `TokenSource` for the duration of one request.
- **Preconditions:** Client constructed with `TokenSource` A; request issued with `ctx` carrying `TokenSource` B.
- **Wire assertion:** Outgoing `Authorization` header carries the bearer from B; subsequent requests without the ctx-override fall back to A.
- **Modes:** Sandbox.
- **Status:** Draft.

#### PROBE-065 — `Prefer: return=minimal` on POST returns identifier only

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=minimal` returns an empty body and a `Location` header; a follow-up GET returns the full payload.
- **Preconditions:** Backend honours `Prefer: return=minimal`.
- **Wire assertion:** POST response body is empty; `Location` is set; SDK surfaces only `*VersionMetadata`. Subsequent GET returns the full Composition.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

#### PROBE-066 — Stored AQL query execution

- **Title:** `GET /query/{qualified_query_name}` returns a typed `ResultSet`.
- **Preconditions:** A stored query registered under a known qualified name.
- **Wire assertion:** Request path matches the qualified-name URL template; response decodes as `query.ResultSet` with `Columns` and `Rows` populated.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-067 — Template upload round-trip

- **Title:** `POST /definition/template/adl1.4` with an OPT body succeeds; a subsequent `GET` returns the same OPT bytes.
- **Preconditions:** Backend supports ADL1.4 template upload at the standard path.
- **Wire assertion:** Upload request carries `Content-Type: application/xml`; GET response body equals the uploaded OPT bytes (modulo backend-side reformatting documented per deployment).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-068 — Error envelope decodes into `WireError.OpenEHR`

- **Title:** A `400 Bad Request` carrying a `{message, code}` JSON body surfaces as a `transport.WireError` whose `OpenEHR` detail is populated and which matches a typed error via `errors.As`.
- **Preconditions:** Cassette of a real 400 error envelope.
- **Wire assertion:** `errors.As(err, &wire)` succeeds; `wire.OpenEHR.Message`, `wire.OpenEHR.Code` are set from the envelope; `wire.RawBody` preserves the raw bytes.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

### Observability

#### PROBE-050 — OTel span carries openEHR attributes

- **Title:** Every outgoing request opens an OTel span with `openehr.spec_version`, `openehr.resource_type`, and a sanitised URL.
- **Preconditions:** OTel `TracerProvider` injected in context.
- **Wire assertion:** Captured span has the expected attribute set; URL does not contain the bearer token.
- **Modes:** Sandbox.
- **Status:** Draft.

#### PROBE-051 — No-OTel is a silent no-op

- **Title:** Absence of a `TracerProvider` in context produces no error, no warning, and no allocated spans.
- **Preconditions:** Default context.
- **Wire assertion:** Request succeeds; no global state mutation.
- **Modes:** Sandbox.
- **Status:** Draft.

## Adding probes

A new probe **MUST**:

- Be assigned the next available `PROBE-NNN` for its topic range (gap of 10 between topics).
- Have a definition in this catalog *before* any implementation lands.
- Be runnable in at least Sandbox mode; Cassette and Live modes follow when fixtures are recorded.
- Carry a `Status:` transition (Draft → Implemented → Ratified) in this spec when its state changes; transitions go in the CHANGELOG.

## Removing probes

A probe **MUST NOT** be silently removed. The lifecycle is:

1. Mark `Status: Deprecated` with a reason and a removal target version.
2. Keep the probe runnable for at least one minor version.
3. Remove in the next major version.

Renumbering is prohibited — once a `PROBE-NNN` is published, it stays.

## Coverage matrix

| Topic | Probes | Lives in (test code, TBD) |
|---|---|---|
| Auth + discovery | PROBE-001 … 009 | `testkit/probes/auth/` |
| Versioned writes | PROBE-010 … 013 | `testkit/probes/versioned/` |
| AQL | PROBE-020 … 021 | `testkit/probes/aql/` |
| Canonical JSON / formats | PROBE-030 … 034 | `testkit/probes/serialize/` |
| Service discovery | PROBE-040 … 041 | `testkit/probes/discovery/` |
| Observability | PROBE-050 … 051 | `testkit/probes/observability/` |
| REST binding | PROBE-060 … 068 | `testkit/probes/rest/` |
