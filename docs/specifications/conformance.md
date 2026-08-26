# Conformance probes

**Status:** Draft

The conformance probe suite for this SDK. Covers openEHR wire conformance (REQ-080, REQ-082) and Cadasto-platform API conformance for the `cadasto/*` extras (REQ-083).

A **conformance probe** is an executable assertion that the SDK exercises against either:

- the sandbox transport (`sandbox/`),
- a replayed recording (`testkit/recordings/`), or
- a live Cadasto reference deployment,

to verify wire-level conformance to openEHR REST + SMART-on-openEHR.

Each probe has a stable `PROBE-NNN` ID, a single normative definition (here), and a pass/fail outcome against a reference deployment.

## Conformance scope

### REQ-080 — openEHR wire conformance

The probe suite verifies the SDK against the **openEHR wire contract**, not against any other implementation:

- A probe's **assertion** is wire-level: the HTTP request bytes (method, path, headers, body), the response status, the response body shape.
- A probe's **definition** lives once, here, and is implemented in the SDK's test suite.
- A probe **MUST NOT** assert on source-level idioms (function names, error types).
- Decode→encode round-trips **MUST** be byte-stable (modulo documented field ordering).

### REQ-081 — Wire-level parity (retired)

**Status: Deprecated.** This requirement once framed conformance as byte-for-byte parity with the Cadasto PHP SDK. The SDK no longer pursues cross-SDK parity: wire-level correctness is defined against the openEHR spec (REQ-080), and the idiomatic-Go surface stands on its own (REQ-023, [idiom.md](idiom.md)). The identifier is retained (stable; never reused) but carries no active requirement.

### REQ-082 — Runnability

Every probe that asserts against a **backend** **MUST** be runnable in three modes:

| Mode | Backend | Artefact | Use |
|---|---|---|---|
| **Sandbox** | `sandbox/` in-memory transport | none | Fast unit tests; CI default |
| **Cassette** | a replayed **recording** | `testkit/recordings/` | Deterministic CI against captured real-deployment traffic |
| **Live** | a reference openEHR deployment | none | Pre-release verification against a real backend |

The probe definition is the single source; the runner picks the backend at invocation time.

**Not every probe is backend-facing.** An **in-repo** probe asserts a property over vendored inputs or over the SDK's own output — the AQL round-trip and catalogue properties, the upstream FLAT parity harness, the codec and validation multiset probes — and reaches no server in any mode. Such a probe **MUST** declare `In-repo` in its **Modes** line, and the three-mode rule above does **not** bind it: there is no backend for a recording to capture or a deployment to confirm. This is a declared class, not a shortfall, and it is why a blanket three-mode reading of this requirement is wrong — 13 of the 68 catalog entries are in-repo by construction.

For a backend-facing probe, its **Modes** line is the authoritative statement of which of the three it currently supports, and any mode missing from that line is an open gap in *this* requirement rather than a defect in the probe. Today 15 entries declare all three; the rest are the work [REQ-082's plan](../plans/2026-08-18-probe-runnability.md) sequences.

A **recording** is a captured HTTP exchange — method, URL, request and response headers, status, and both bodies. It is a different artefact from the vendored **fixture documents** under `testkit/cassettes/` (§ Vendored fixtures below), which are bodies only and carry no exchange. The two **MUST NOT** share a directory: a fixture is hand-curated input, a recording is captured evidence, and only the second can go stale against a deployment.

#### Mode selection belongs to the runner, never to the probe

A backend-facing probe **MUST** receive an already-configured client and **MUST NOT** observe which mode it runs in (an in-repo probe receives no client — there is nothing to configure) — no mode parameter, no environment read, no type assertion on the transport. A probe that cannot be written without knowing its backend is asserting something other than wire behaviour, and **MUST** be reformulated rather than given an escape hatch.

The runner **MUST** expose a single entry point taking the mode plus that mode's backend configuration, and **MUST** be able to run the whole catalog, a named subset, or one probe. Selecting a mode the invocation cannot satisfy (Cassette with no recording for a probe, Live with no endpoint) **MUST** fail loudly; it **MUST NOT** silently fall back to another mode.

#### The probe contract

Every probe **MUST** report through one shared result type with a single canonical home, carrying the `PROBE-NNN` id, the mode, the status, and a detail string. Per-package copies of that type **MUST NOT** exist — a probe result compared across modes has to be the same type in every package, or the runner cannot aggregate it.

Status is the closed set `pass` / `fail` / `skip`:

- `skip` **MUST** name the unmet precondition in its detail, and **MUST NOT** be reported or counted as a pass.
- The runner's summary **MUST** distinguish "every probe passed" from "some probes skipped". A run whose probes all skipped **MUST NOT** read as green — vacuous success is the failure mode this rule exists to prevent, the same reasoning [PROBE-086](#probe-086--upstream-flat-serialisation-parity) applies to an empty compared set.

Every backend-facing probe **MUST** declare its effect as **read-only** or **mutating**, as an **Effect** field in its catalog entry — part of the probe's definition here, not a runner-side annotation.

The catalog predates this field and no entry carries one yet. Until an entry is classified, a probe **MUST** be treated as **mutating**: the unclassified default is the restrictive one, so a writer cannot reach a live deployment merely because nobody got round to labelling it. Populating the catalog is phase 1 of the plan; an in-repo probe needs no declaration, having no backend to affect.

#### Sandbox mode

The `sandbox/` transport **MUST** serve every backend-facing probe in the catalog without a network listener or credentials (an in-repo probe reaches no transport), and **MUST** be reachable by SDK consumers testing their own applications — it is a published building block, not test-only scaffolding (REQ-013: no `auth/` or live-transport dependency).

Sandbox state **MUST** be per-run and isolated: two probes running against one sandbox instance **MUST NOT** observe each other's writes unless the probe definition says they share an EHR.

#### Cassette mode

The recording **encoding** is deliberately not fixed here: whether recordings are HTTP Archive (`.har`) or a purpose-built YAML schema is open as [STRAND-11](research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml), to be resolved by ADR against a real capture. Every rule below binds regardless of the encoding chosen.

Recordings are checked-in evidence and are held to the same standard as any vendored corpus:

- A recording **MUST** carry its provenance — the deployment it came from, the date, and the SDK commit that captured it. A recording whose provenance cannot be stated **MUST** be discarded rather than replayed.
- Recordings **MUST** be redacted **at capture time**, never at review time. Credentials (`Authorization`, cookies, tokens, client secrets, JWKS private material) and patient-identifying data **MUST NOT** reach disk. A recording **MUST** record that redaction ran, so an unredacted capture is detectable rather than merely unlikely.
- Replay matching **MUST** be on a normalised request key — method, path, and the headers and body fields the probe's assertion depends on — **not** byte-exact equality: a capture necessarily carries timestamps, generated UUIDs, and `ETag` values that differ on every run, and byte-exact matching would make every recording single-use.
- An unmatched request **MUST** fail the probe. The replayer **MUST NOT** pass a request through to a network, and **MUST NOT** synthesise a plausible response — a recording with a gap is a recording that must be recaptured.
- Where one probe issues the same normalised request more than once and expects different responses (a write followed by a read of what it wrote), the recording **MUST** preserve exchange order and the replayer **MUST** consume matches in that order.

#### Live mode

- A **read-only** probe **MAY** run against any reachable deployment.
- A **mutating** probe **MUST** be self-scoping: it creates the resources it needs, derived from a per-run identifier, and **MUST NOT** depend on server state it did not create. A probe requiring a pre-seeded EHR **MUST** `skip` when that precondition is absent rather than fail.
- The runner **MUST** refuse to execute mutating probes in Live mode without an explicit per-invocation opt-in. Writing to a deployment **MUST** be something the operator asked for, never a default that a mode flag turned on.
- Cleanup is best-effort — a deployment may forbid deletion. The per-run identifier **MUST** appear in every resource a mutating probe creates, so anything it leaves behind is attributable and removable by hand.

#### Cross-mode agreement

The same probe **MUST** reach the same verdict in every mode it is exercised in, with recording captured once against the live backend. A probe that passes in Sandbox and fails on replay indicates the sandbox models the deployment wrongly; **the recording is authoritative and the sandbox MUST be corrected**, never the reverse. A `skip` for an absent precondition is not a disagreement.

For a backend-facing probe, a mode absent from its **Modes** line is an open gap in this requirement rather than a defect in the probe. An in-repo probe reaching no server in any mode is neither.

### REQ-083 — Cadasto platform API conformance

The openEHR surface conforms to the openEHR spec (REQ-080). The Cadasto-platform extras under `cadasto/` (Extra API, Datamap, MPI, Care, admin) have **no openEHR spec**; their wire contract is the **Cadasto platform API** itself.

- The authority is the Cadasto platform's API definition (its OpenAPI document where one exists) or, failing that, recorded fixtures from a reference Cadasto deployment.
- `cadasto/*` probes assert the SDK's request/response wire shape against that contract — **not** against any other SDK. This is the first-party replacement for the retired cross-SDK parity check (REQ-081): the platform is the authority, so a divergence both SDKs shared can no longer pass silently.
- Vendored fixture documents live under `testkit/cassettes/cadasto/` (bodies and reference responses, not REQ-082 Cassette-mode recordings — [§ Vendored fixtures](#vendored-fixtures-testkitcassettes)); per-fixture provenance (deployment, commit/date) is recorded in that directory's README.

**Status: planned.** The `cadasto/*` surfaces are Phase 4; the conformance fixtures land with them.

#### Health probes (`cadasto/admin`)

`cadasto/admin` exposes deployment liveness/readiness probes (`Live`, `Ready`) with a wire contract independent of the (planned) Cadasto platform API surface:

- **Default paths** `DefaultLivePath = "/health/live"`, `DefaultReadyPath = "/health/ready"`, each overridable per call via `WithLivePath` / `WithReadyPath` (e.g. `/healthz`).
- **URL derivation.** The probe URL derives from the **origin (scheme + host) of the openEHR REST service entry** — the openEHR REST API path prefix is **NOT** inherited.
- **Public / no auth.** Health endpoints are public: the SDK **MUST NOT** send an `Authorization` header on a probe.
- **Status mapping** (`errors.Is`-compatible): `2xx → nil`; `401 → transport.ErrUnauthorized`; `403 → transport.ErrForbidden`; `404 → transport.ErrNotFound`; `5xx → transport.ErrServerError`. Other non-2xx codes (400, 405, 408, 429, …) surface as a plain formatted error with no sentinel.
- Probes borrow `transport`'s sentinel taxonomy (REQ-093) but **bypass** `transport.Client.Do` — no openEHR error-envelope decoding, no OTel spans, no retries. Use `openehr/client/admin` when those concerns matter.

**Status:** landed (`cadasto/admin`). Distinct from the ITS-REST Admin client (`openehr/client/admin`). Platform-API conformance fixtures remain Phase 4 per above.

### Vendored fixtures (`testkit/cassettes/`)

This tree holds **fixture documents** — bodies, not exchanges. It is not the Cassette-mode recording corpus (REQ-082); the directory name predates that distinction and is kept because paths resolve through [`testkit/fixtures`](../../testkit/fixtures/).

Serialization and clinical-modeling probes that need reference RM bytes or OPT bodies **MUST** use the checked-in tree under `testkit/cassettes/`. Paths **MUST** be resolved via [`testkit/fixtures`](../../testkit/fixtures/) (`TemplateOpt`, `CompositionJSON`, `CompositionXML`, `RMJSON`, `RMXML`, `SubmissionJSON`) — not hard-coded legacy directory names.

**Layout** (vendor provenance is indexed in [`testkit/cassettes/README.md`](../../testkit/cassettes/README.md); it is not encoded in directory names):

```
testkit/cassettes/
  templates/{template-id}.opt
  compositions/{template-id}.json
  compositions/{template-id}.xml     # when vendored
  rm/{name}.json | {name}.xml        # RM-only samples (ehrbase, leaf XML, …)
  submissions/{name}.json            # CONTRIBUTION POST wire (inline ORIGINAL_VERSION)
  its_rest/                          # ITS-REST wire records (REQ-095)
```

| Kind | Role | Typical probes |
|---|---|---|
| `templates/` + `compositions/` | Operational template + canonical instance for a `template_id` | PROBE-022–027, PROBE-030 (JSON), PROBE-033 (XML when paired) |
| `rm/` | RM root samples without a paired OPT (ehrbase COMPOSITION/EHR_STATUS/FOLDER, leaf `DV_QUANTITY`, …) | PROBE-030, PROBE-033 |
| `submissions/` | CONTRIBUTION create payloads for the EHR contribution client (not `rm.Contribution` decode) | contribution client tests (REQ-059) |
| `its_rest/` | ITS-REST request and response **bodies** (no method, URL, headers, or status — see REQ-082) | PROBE-010+, discovery probes (REQ-095) |

Discovery for PROBE-030 / PROBE-033 walks `compositions/` and `rm/` via [`fixtures.ListCompositionJSON`](../../testkit/fixtures/discover.go) and [`fixtures.ListRMXML`](../../testkit/fixtures/discover.go). Templates with JSON or XML on disk but known codec gaps **MAY** be listed in `compositionJSONExcluded`, `compositionXMLExcluded`, or `rmJSONExcluded` in that package so probes stay green while the files remain available for template and validation work.

**Legacy paths** (`testkit/cassettes/canonical_json/`, `canonical_xml/`, `fixtures/`, vendor subdirectories under `cassettes/`) are **retired** — do not reference them in new spec text, plans, or code comments.

## Probe catalog

The catalog is the normative list. Each entry has:

- **ID** — stable, never renumbered.
- **Title** — one-line description.
- **Preconditions** — what state the system must be in.
- **Wire assertion** — what's checked at the byte / status level.
- **Modes** — Sandbox / Cassette / Live for a backend-facing probe, or In-repo for a probe that reaches no server in any mode (REQ-082). For a backend-facing probe this line is the authoritative record of which modes it supports today.
- **Effect** — read-only or mutating (REQ-082). Absent means *treated as* mutating; the catalog is not yet populated.
- **Status** — Draft (in this spec), Implemented (in code), Ratified (passes against a reference openEHR deployment), Deferred (the requirement it covers is landed and unit-covered, but this dedicated probe is not written), Deprecated (scheduled removal; may be unrunnable when implementation is already gone pre-v1.0).
- **Satisfies** — REQ-IDs this probe exercises (inverse of the [REQ registry](REQ.md)).

### Authentication and discovery

#### PROBE-001 — Discovery declares `code+pkce`

- **Title:** SMART configuration document declares `code` response type and `S256` PKCE method.
- **Preconditions:** A SMART-on-openEHR deployment is reachable.
- **Wire assertion:** GET `<issuer>/.well-known/smart-configuration` (or equivalent) returns 200 with a JSON body containing `"response_types_supported"` including `"code"` and `"code_challenge_methods_supported"` including `"S256"`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_001_discovery_code_pkce.go`](../../testkit/probes/auth/probe_001_discovery_code_pkce.go). Resolves the canonical SMART configuration cassette through the real `discovery.Resolver` and asserts `code` + `S256` on the resolved `AuthEndpoints`.
- **Satisfies:** REQ-061.

#### PROBE-002 — Discovery advertises `org.openehr.rest`

- **Title:** Service catalog includes the openEHR REST service with a parseable base URL and a declared spec version.
- **Preconditions:** SMART discovery resolved.
- **Wire assertion:** The discovery document's service catalog contains an entry with id `"org.openehr.rest"`, a parseable `base_url`, and a `spec_version` field.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_002_openehr_rest_service.go`](../../testkit/probes/auth/probe_002_openehr_rest_service.go). Asserts `catalog.OpenEHRRest()` resolves an entry with an absolute `base_url` and a declared `spec_version`.
- **Satisfies:** REQ-070, REQ-072.

#### PROBE-003 — Spec-version mismatch fails fast

- **Title:** A discovery document advertising an incompatible spec version is rejected at resolution, not at first request.
- **Preconditions:** SDK is configured to require `1.1.0-development`; deployment advertises `1.0.3`.
- **Wire assertion:** Construction-time discovery returns a `DiscoveryError` with reason `spec_version_mismatch`. No request to the openEHR REST endpoint is made.
- **Modes:** Sandbox, Cassette (constructed-mismatch cassette).
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_003_spec_version_mismatch.go`](../../testkit/probes/auth/probe_003_spec_version_mismatch.go). Serves a cassette whose `org.openehr.rest` declares `1.0.3` and asserts the resolver fails fast with `DiscoveryError(spec_version_mismatch)`, returning no catalog.
- **Satisfies:** REQ-072.

#### PROBE-004 — PKCE verifier round-trip

- **Title:** A SMART launch using `S256` PKCE successfully exchanges code for token.
- **Preconditions:** Deployment registers the SDK as a SMART app with PKCE required.
- **Wire assertion:** Authorization request carries `code_challenge` and `code_challenge_method=S256`; token exchange carries `code_verifier`; token response is 200 with an `access_token`.
- **G-7 PKCE parity:** the SDK's verifier additionally satisfies RFC 7636 / `x/oauth2`: ≥ 32 bytes of decoded entropy, `base64.RawURLEncoding` (URL-safe, unpadded), and `code_challenge == base64url(SHA256(verifier))` with method `S256` — cross-checked against `golang.org/x/oauth2.S256ChallengeFromVerifier`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_004_pkce_verifier_round_trip.go`](../../testkit/probes/auth/probe_004_pkce_verifier_round_trip.go). Drives a full `auth/smart` authorization-code + PKCE launch against an httptest token endpoint and asserts the wire round-trip plus the G-7 parity properties.
- **Satisfies:** REQ-061.

#### PROBE-005 — Scope round-trip

- **Title:** Configured openEHR scope (`<compartment>/<resource>.<permission>`) survives token exchange and lands in the JWT scope claim or the response `scope` field.
- **Preconditions:** Scope `patient/COMPOSITION.read` is requested.
- **Wire assertion:** Authorization request `scope` parameter contains `patient/COMPOSITION.read`; token response `scope` field contains it (or the JWT `scope` claim does).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_005_scope_round_trip.go`](../../testkit/probes/auth/probe_005_scope_round_trip.go). Asserts the configured scope appears on the authorization request and survives into the SDK-parsed token-response `scope` field.
- **Satisfies:** REQ-061.

#### PROBE-008 — Platform principal claims surface verbatim

- **Title:** When the token carries `principal_uid` and `principal_type` claims (per REQ-067), the SDK surfaces them on `LaunchContext.Principal` without coercion.
- **Preconditions:** A token with `principal_uid = "u-123"`, `principal_type = "AGENT"`.
- **Wire assertion:** SDK exposes `LaunchContext.Principal = {UID: "u-123", Type: PrincipalTypeAgent}`. Missing claims surface as nil/zero, not as guessed defaults.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_008_principal_claims.go`](../../testkit/probes/auth/probe_008_principal_claims.go). Mints a signed id_token, validates it via `smart.LaunchContextFromTokenResponse`, and asserts the principal claims surface verbatim while an absent-claims token yields a nil `Principal`.
- **Satisfies:** REQ-067.

#### PROBE-009 — Caller attribution forwarded on opt-in

- **Title:** When caller attribution is configured (REQ-066), the SDK emits the configured header and OTel attributes; when not configured, no attribution data appears on the wire.
- **Preconditions:** One client with `WithCallerAttribution(...)`, one without.
- **Wire assertion:** Configured client emits the `X-Cadasto-Caller-Attribution` header and `caller.agent_id` OTel attribute; unconfigured client emits neither.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_009_caller_attribution.go`](../../testkit/probes/auth/probe_009_caller_attribution.go). Runs one `transport.Client` with `WithCallerAttribution` and one without against the same httptest endpoint, capturing the request header at the server and the span attributes via an in-memory `TracerProvider` (no OTel-SDK dependency).
- **Satisfies:** REQ-066.

#### PROBE-006 — JWKS rotation transparent to caller

- **Title:** A signing-key rotation on the authorization server triggers exactly one JWKS refresh in the SDK; subsequent requests succeed without consumer intervention.
- **Preconditions:** A cached JWKS does not contain the `kid` of the issued token (simulating rotation).
- **Wire assertion:** SDK fetches JWKS once, validates the token, and proceeds. No double-refresh, no double-validation failure surfaced.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/auth/probe_006_jwks_rotation.go`](../../testkit/probes/auth/probe_006_jwks_rotation.go). Signs an id_token with a rotated key whose `kid` is absent from the seeded JWKS cache, then asserts `smart.ValidateIDToken` recovers via exactly one refresh (seed + one fetch on the miss) and validates transparently.
- **Satisfies:** REQ-062.

#### PROBE-007 — Token refresh transparent to caller

- **Title:** An expired access token with a valid refresh token is refreshed silently before the next request.
- **Preconditions:** Cached token has `expires_at < now`; refresh token is valid.
- **Wire assertion:** Token endpoint receives `grant_type=refresh_token`; the next outgoing request carries the new access token.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — both halves. The transport-side safety net (wire 401 → `Reauth` → retry once with refreshed bearer) is asserted in [`testkit/probes/auth/probe_007_transport_refresh.go`](../../testkit/probes/auth/probe_007_transport_refresh.go) (REQ-063, Phase 4b). The proactive expiry-based half — `Source.Token` refreshing a stale token via a `grant_type=refresh_token` exchange on the wire — landed in Phase 5 as [`testkit/probes/auth/probe_007_proactive_refresh.go`](../../testkit/probes/auth/probe_007_proactive_refresh.go); it seeds an expired access token + valid refresh token and asserts the token endpoint receives exactly one `grant_type=refresh_token` request and the SDK returns the freshly issued bearer. (`auth/smart` unit tests `TestRefreshIfNeeded` / `TestSourceReauthForcesRefresh` remain.) Full cassette/live ratification is deferred.
- **Satisfies:** REQ-063

#### Launch-mode coverage (REQ-068)

The four SMART grant flows × three launch modes (REQ-068) are exercised as
named coverage functions alongside the auth probes in
[`testkit/probes/auth/launch_modes.go`](../../testkit/probes/auth/launch_modes.go)
(run via `TestLaunchModeStandalone` / `TestLaunchModeEmbedded` /
`TestLaunchModeBackend`). They are coverage proofs rather than catalogued
`PROBE-NNN` IDs, but they are part of the auth-suite definition of done:

- **Standalone** — `Source.AuthorizeURL` builds an authorization URL with
  `response_type=code` + the `S256` PKCE challenge and **no** `launch`
  parameter.
- **Embedded (EHR launch)** — an EHR-supplied `launch` parameter is
  forwarded verbatim to the authorization endpoint.
- **Backend service** — three confidential backend flows produce the
  expected token request on the wire: `auth/clientcreds` with a symmetric
  `client_secret` (HTTP Basic), `auth/clientcreds` with
  `WithClientAssertion` (`client_credentials` + signed `client_assertion`,
  no Basic, no `client_secret` — SMART Backend Services asymmetric), and
  `auth/jwtbearer` (RFC 7523 JWT Bearer grant).

Together with the PKCE public flow (PROBE-004) and the confidential-code
auth-method selection (covered by `auth/smart`'s `TestExchangeWithPrivateKeyJWT`
/ `TestG3CrossCheckRejectsUnsupportedMethod` / `TestExchangeWithClientSecretBasic`
unit pins), this exercises all four flows across all three launch modes.

##### Inferno SMART App Launch (STU2.2) Client-suite cross-check

The HL7 Inferno **SMART App Launch Test Kit STU2.2 _Client_** suite is used
as an external checklist (it cannot be executed here). Mapping of its
client scenarios to SDK coverage:

| Inferno client scenario | SDK coverage | Status |
|---|---|---|
| **Public client** (authorization-code + PKCE, no secret) | PROBE-004 (PKCE + G-7 parity), PROBE-005 (scope), standalone/embedded launch modes | Covered (Sandbox) |
| **Confidential Symmetric** (`client_secret_basic`) | `auth/smart` `client_secret_basic` selection + backend symmetric arm of `LaunchModeBackend`; positive wire test `TestExchangeWithClientSecretBasic` (asserts `Authorization: Basic base64(clientID:secret)`, `grant_type=authorization_code`, no `client_assertion`) | Covered (Sandbox) |
| **Confidential Asymmetric** (`private_key_jwt`) | `auth/smart` `WithClientAssertionKey` (`TestExchangeWithPrivateKeyJWT`, G-3 cross-check) + private_key_jwt backend arm of `LaunchModeBackend` | Covered (Sandbox) |
| **Backend Services Asymmetric** (`client_credentials` + `client_assertion`) | backend arm of `LaunchModeBackend` (`auth/clientcreds.WithClientAssertion`); `auth/jwtbearer` for the RFC 7523 grant | Covered (Sandbox) |

**Recorded gaps (follow-ups, not silent skips):**

- The SDK is the SMART **client**, so Inferno scenarios that assert the SDK
  *responds* as an authorization server (e.g. token-introspection responder
  behaviour) are out of scope by construction.
- Cassette/Live ratification of all auth probes against a reference
  authorization server (and a real Inferno run against a deployed Cadasto
  endpoint) is deferred — the probes are Sandbox-only today.
- ES384/ES256 asymmetric client-assertion paths are unit-tested in
  `auth/jwtbearer` but the launch-mode backend coverage exercises the RS384
  default only; an ES* backend coverage arm is a possible follow-up.

### Versioned writes and optimistic concurrency

#### PROBE-010 — PUT Composition without If-Match

- **Title:** A PUT against a versioned Composition without an `If-Match` header is rejected with `428 Precondition Required`.
- **Preconditions:** An existing Composition with a known `version_uid`.
- **Wire assertion:** PUT `/ehr/{ehr_id}/composition/{versioned_object_id}` without `If-Match` returns `428`; the SDK maps this to `transport.ErrPreconditionRequired`. The Go SDK additionally short-circuits empty `ifMatch` at the call site with `transport.ErrInvalidConfig` per the typed-write-path guard.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_010_put_without_if_match.go`](../../testkit/probes/versioned/probe_010_put_without_if_match.go).
- **Satisfies:** REQ-054, REQ-093

#### PROBE-011 — PUT Composition with stale If-Match

- **Title:** A PUT with a stale `If-Match` (referencing an old version_uid) is rejected with `412 Precondition Failed` or `409 Conflict` depending on backend convention.
- **Preconditions:** Composition has been updated since the SDK's cached `version_uid`.
- **Wire assertion:** PUT returns `412` or `409`; SDK maps to `ErrPreconditionFailed` or `ErrVersionConflict` accordingly.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_011_put_stale_if_match.go`](../../testkit/probes/versioned/probe_011_put_stale_if_match.go).
- **Satisfies:** REQ-054, REQ-093

#### PROBE-012 — ETag survives round trip

- **Title:** A GET Composition followed by a PUT with the captured `ETag` as `If-Match` succeeds.
- **Preconditions:** Read-then-write workflow.
- **Wire assertion:** GET response carries `ETag`; PUT carries the same value as `If-Match`; PUT returns `204` or `200`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_012_etag_round_trip.go`](../../testkit/probes/versioned/probe_012_etag_round_trip.go).
- **Satisfies:** REQ-054

#### PROBE-013 — Cross-EHR isolation

- **Title:** A `version_uid` belonging to EHR A cannot be read via EHR B's path.
- **Preconditions:** Two distinct EHRs; a Composition known to belong to EHR A.
- **Wire assertion:** GET `/ehr/{ehr_b_id}/composition/{version_uid_from_a}` returns `404 Not Found`, never `200`, never the EHR A data.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_013_cross_ehr_isolation.go`](../../testkit/probes/versioned/probe_013_cross_ehr_isolation.go).
- **Satisfies:** REQ-054

### AQL

#### PROBE-020 — AQL builder string stability

- **Title:** The struct-builder and verb-functions produce byte-identical AQL strings for the same logical query.
- **Preconditions:** A reference query ("all OBSERVATIONs of archetype `body_temperature` for a given EHR"); canonical golden in [`openehr/aql/testdata/wire/`](../../openehr/aql/testdata/wire/).
- **Wire assertion:** The struct-builder (`aql.NewBuilder().Select(…).FromEHR(…).Contains(…).Build()`) and the verb-functions (`aql.Select(…).FromEHR(…).Contains(…).Build()`) emit equal strings, byte for byte, and both equal the golden — `SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[…] WHERE e/ehr_id/value = $ehr_id` (the builders emit EHR scoping via WHERE; the standing-predicate form is equally valid AQL).
- **Modes:** Sandbox (no network).
- **Status:** Implemented (Sandbox) — [`testkit/probes/aql/`](../../testkit/probes/aql/).

#### PROBE-021 — AQL parse error mapping

- **Title:** A syntactically invalid AQL string produced by a typed builder is impossible; a syntactically valid but semantically invalid one produces a typed `AQLError` on execution.
- **Preconditions:** Reference deployment that validates AQL against templates.
- **Wire assertion:** The typed builders cannot emit syntactically invalid AQL (structural guarantee). Execution of a query referencing a non-existent path returns the backend's AQL error envelope; the SDK maps it to `*query.AQLError`, which satisfies `errors.Is(err, aql.ErrPathResolution)`. A **501** response maps to `*query.AQLError` satisfying `errors.Is(err, aql.ErrEngineCapability)` and never `aql.ErrPathResolution` — the capability gap is read from the status alone, with an envelope or bare, never from message text (REQ-055).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — error mapping tested against synthesised backend responses: a path-resolution envelope, and a 501 capability gap both with an envelope and bare ([`openehr/client/query/`](../../openehr/client/query/)); Cassette/Live ratification pending a reference deployment.

#### PROBE-028 — AQL lint stability

- **Title:** Linting fixed AQL strings against the SDK grammar profile (and, for Layer 3, a compiled OPT) yields a stable issue-code multiset.
- **Preconditions:** A compiled OPT (`vital_signs.opt`) and cassette query strings in [`testkit/cassettes/aql/lint/`](../../testkit/cassettes/aql/lint/) (`valid.aql`, `missing_archetype.aql`, `bad_syntax.aql`).
- **Wire assertion:** Sandbox-only — `lint.LintString(q, &lint.Options{Compiled: c})` over each cassette MUST produce exactly the expected `lint.Issue.Code` multiset: `valid.aql` → none; `missing_archetype.aql` → `aql_archetype_not_in_template`; `bad_syntax.aql` → `aql_syntax`. Any implementation of REQ-109 over the same grammar profile + template MUST report the same codes. Detail text and path strings are not asserted (observable-behaviour level).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/aql/probe_028_aql_lint.go`](../../testkit/probes/aql/probe_028_aql_lint.go).
- **Satisfies:** REQ-109.

#### PROBE-080 — AQL parse/emit round-trip property

- **Title:** Parsing a catalogue AQL string into `parse.Query` and re-emitting via `(*Query).Emit` is stable — canonical inputs are preserved, re-emission is idempotent, and out-of-catalogue shapes surface `aql.ErrIncompleteAST` at both parse and emit rather than round-tripping silently.
- **Preconditions:** The AQL catalogue exercised in [`openehr/aql/parse/roundtrip_test.go`](../../openehr/aql/parse/roundtrip_test.go) — 87 idempotence cases, 43 canonical-input preservation cases, and the residual-gap suite (the unrepresentable-numeric guard, which an out-of-range `TOP` count now falls under). The corpus grew when REQ-117 closed the v1 catalogue gaps and again when REQ-118 moved the `top` clause into the catalogue; see [PROBE-087](#probe-087--aql-structured-ast-catalogue-completeness).
- **Wire assertion:** In-repo property — for every catalogue query, `parse.ParseQuery(q)` → `(*Query).Emit()` MUST equal the canonical form of `q`, and a second `Emit` MUST equal the first (idempotence). Out-of-catalogue shapes MUST return `aql.ErrIncompleteAST` from both `ParseQuery` and `Emit`, never a partial emit. The `WhereExpr`/`Value` vocabulary is identical across the read (`parse`) and write (`aql.Builder`) sides.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — see [`openehr/aql/parse/roundtrip_test.go`](../../openehr/aql/parse/roundtrip_test.go).
- **Satisfies:** REQ-113.

#### PROBE-082 — AQL structured path access (standing predicate + WHERE path)

- **Title:** A class standing predicate is readable as a structured `{path, operator, value}` (`ClassExpr.PredicateComparison`) and a WHERE comparison's alias-qualified path as alias + segments (`aql.Comparison.ParsedPath`), without importing `parse/gen` or re-tokenizing raw text (REQ-113).
- **Preconditions:** A parsed query carrying a standing class predicate (`EHR e[ehr_id/value=$ehr]`) and a WHERE comparison over an alias path (`o/data[at0001]/events[at0006]/value/magnitude > $threshold`).
- **Wire assertion:** In-repo property — `ParseQuery`'s `From.Root.PredicateComparison` MUST expose `{ehr_id/value, =, $ehr}` for the standing comparison and be nil for an archetype-HRID (non-comparison) predicate; the WHERE `aql.Comparison.ParsedPath` MUST expose alias `o` + the ordered segments, with `ParsedPath.Raw == Comparison.Path`. Emission is unaffected — the standing predicate and path survive a parse→emit→parse round-trip, and Emit is idempotent.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — see [`openehr/aql/parse/structured_test.go`](../../openehr/aql/parse/structured_test.go).
- **Satisfies:** REQ-113.

#### PROBE-087 — AQL structured-AST catalogue completeness

- **Title:** Every shape in the REQ-117 catalogue list — primitive SELECT literal, mixed `SELECT *, col`, function-call WHERE LHS, `MATCHES` with `TERMINOLOGY(...)`/`{URI}`, path-vs-path comparison, FROM-root boolean junction, parameter/primitive/nested-function arguments in function calls, and junctions over any in-catalogue operand — plus the REQ-118 additions (the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause, and a projected literal's source text) parses into `parse.Query` without `aql.ErrIncompleteAST` and round-trips through `(*Query).Emit` under the PROBE-080 fixed-point property.
- **Preconditions:** The REQ-117 catalogue corpus in [`openehr/aql/parse/roundtrip_test.go`](../../openehr/aql/parse/roundtrip_test.go) / [`openehr/aql/parse/query_test.go`](../../openehr/aql/parse/query_test.go), including the former v1 gap corpus asserting the gap is closed, extended by the REQ-118 `top`-clause and literal-source-text corpus.
- **Wire assertion:** In-repo property — for each catalogue shape, `ParseQuery` MUST return a structurally faithful AST (pinned per shape) and `Emit` MUST be a fixed point. Since REQ-118 the complete residual `ErrIncompleteAST` list is ONE class — an unrepresentable numeric literal (a `LIMIT`/`OFFSET` beyond Go `int`, an INTEGER beyond `int64`, a REAL beyond `float64` in either direction — overflow, or underflow collapsing to zero — or an out-of-range `TOP` count, in any value position) — and it MUST still fire. A `top` clause MUST carry its count and direction, and a projected literal MUST carry its source text as written whenever that text diverges from the canonical rendering.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — the nine round-trip cases covering the eight closed gap shapes are asserted modelled by `TestFormerCatalogueGapsModelled` and the residual guard by `TestParseQuerySurfacesIncompleteAST` in [`openehr/aql/parse/roundtrip_test.go`](../../openehr/aql/parse/roundtrip_test.go), with per-shape structural pins in [`openehr/aql/parse/query_test.go`](../../openehr/aql/parse/query_test.go) and vocabulary introspection in [`openehr/aql/introspect_test.go`](../../openehr/aql/introspect_test.go). The REQ-118 additions are pinned by `TestParseQueryTopClause`, `TestParseQueryTopClauseOutOfRange`, and `TestParseQueryLiteralSourceText` in the same files.
- **Satisfies:** REQ-117, REQ-118.

#### PROBE-088 — AQL builder containment and paging stability

- **Title:** The builder's negated containment (`NOT CONTAINS`), sibling containment junctions (`AND`/`OR` with precedence-driven parenthesisation), opt-in in-text `LIMIT`/`OFFSET`, and the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause (REQ-118) emit byte-stable canonical AQL, and a builder program using none of the new API produces byte-identical output to the pre-REQ-117 golden.
- **Preconditions:** Golden fixtures under [`openehr/aql/testdata/wire/`](../../openehr/aql/testdata/wire/) covering the new constructs plus the untouched PROBE-020 golden.
- **Wire assertion:** Sandbox golden comparison (no backend) — built query strings MUST match the committed goldens byte-for-byte; requesting both in-text and envelope paging MUST be a build-time error, never a silently combined emission, and so MUST a `TOP` combined with either row-limit channel (QUERY Release-1.1.0 § 4.4.3 forbids `TOP` with `LIMIT`) or a negative `TOP` count.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/aql/probe_088_builder_containment_paging.go`](../../testkit/probes/aql/probe_088_builder_containment_paging.go), run by `TestProbe088BuilderContainmentAndPaging`; goldens in [`openehr/aql/testdata/wire/`](../../openehr/aql/testdata/wire/). Every golden is additionally re-parsed and re-emitted (`TestProbe088GoldensRoundTripThroughParse`), which ties the write-side canonicaliser to PROBE-087; the paging channel-exclusivity refusals — including the REQ-118 `TOP` rules — are pinned in [`openehr/aql/paging_test.go`](../../openehr/aql/paging_test.go).
- **Satisfies:** REQ-117, REQ-118.

#### PROBE-090 — AQL emission round-trip closure

- **Title:** Every value a supported write path emits — string, real, and integer literals, a `MATCHES {uri}` operand, a value-position function call, and a containment tree — re-parses into an equal value, and the write-side guards that refuse an unspellable value agree with the vendored grammar rather than with a hand-written expectation.
- **Preconditions:** The escape / numeric / URI / function-name corpora in [`openehr/aql/parse/literal_roundtrip_test.go`](../../openehr/aql/parse/literal_roundtrip_test.go) and [`openehr/aql/parse/emit_parity_test.go`](../../openehr/aql/parse/emit_parity_test.go), the write-side unit corpus in [`openehr/aql/value_test.go`](../../openehr/aql/value_test.go), the predicate corpora in [`openehr/aql/parse/predicate_parity_test.go`](../../openehr/aql/parse/predicate_parity_test.go), [`openehr/aql/parse/predicate_confrontation_test.go`](../../openehr/aql/parse/predicate_confrontation_test.go), [`openehr/aql/parse/predicate_guard_test.go`](../../openehr/aql/parse/predicate_guard_test.go) and [`openehr/aql/predicate_test.go`](../../openehr/aql/predicate_test.go), the identifier and archetype-id corpora in [`openehr/aql/parse/identifier_parity_test.go`](../../openehr/aql/parse/identifier_parity_test.go), and the vendored lexer at `resources/aql/grammar/active/AqlLexer.g4`, which the grammar-confrontation tests read directly.
- **Wire assertion:** In-repo property — for every value in the corpus, `ParseQuery(emit(v))` MUST succeed and MUST recover a value equal to `v` under `aql.EqualValues`, in EVERY value position (comparison operand, `MATCHES` member, `LIKE` pattern, SELECT item, SELECT function argument), on BOTH carriers (a pointer-shaped node MUST emit byte-identically to the value-shaped one, or both MUST refuse); a value the guards refuse MUST NOT round-trip intact. For a literal read from source text the property MUST hold TWICE (parse → emit → parse recovers the same value), which is what an arbitrary byte and a surrogate pair break and mere parseability does not catch. A position that splices exactly ONE grammar token verbatim — a SELECT `AS` alias, a class alias, a class RM type, an archetype predicate — MUST be guarded at EVERY position of that class, and its accept set MUST equal "the parser reads the string back unchanged". Emission is additionally verified AFTER emission, over the whole query — bound by [clinical-modeling.md § Emission verified after emission](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) — the closure that reaches a token boundary no per-position rule can decide, because it depends on text the position does not contain. The rules live there and are not restated here; what this probe carries is that section's oracle construction: the output must re-parse, and the re-parse must carry the same encoding-INDEPENDENT skeleton (a structural reduction, not field-by-field AST equality — which refuses most hand-built ASTs; which encodings are factored out as erased and which nesting stays structural is that section's inventory, not restated here). A position that splices a whole bracket REGION verbatim — a class or VERSION standing predicate — is bound by [clinical-modeling.md § The class predicate positions](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission), the ONE normative home for that position's rules: the bracket-escape guard and its region states, the whole decidable `versionPredicate` production, whole-TOKEN longest match, the refusal of every region whose end the supplied text does not fix, the structural value-free naming of a refused predicate, and the two-sided generated confrontation with its base requirements — none of them restated here. What this probe carries is that section's oracle CONSTRUCTION: round-trip identity is the oracle (a spliced predicate parses perfectly well as something else); the generated corpus is spliced at each of the two bracket positions into bases at least one of which carries material after the bracket and a later line break (a bare base rides beside it for the end-of-query adjacency); every base is shown able to fail by some splice in it re-parsing as a different query; and the VERSION shape rule — whose arm the shared junk-operand corpus caps at "never a different query" — gets a corpus of its own with operands legal by construction, over which accepted text re-parses outright. The converse MUST hold too: an AST field the emitter never renders in the shape it was given — an archetype or RM type on a `VERSION` class, a root beside a root junction — MUST be refused rather than DROPPED; the full refusal inventory, including the containment junction node's, is [clinical-modeling.md § Emit-side structural parity](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission)'s, since emitting less than the AST holds returns more rows than the caller asked for, and a guard that validates such a field reports the malformed value while losing the well-formed one. The grammar-derived guards MUST agree with the grammar: for every token name in the lexer — plus the keyword spellings carried only in lexer `fragment`s — the builder emits a function of that name if and only if the parser accepts it; for every URI in the corpus, the builder emits the operand if and only if it re-parses as one `MATCHES` on the same URI (round-trip IDENTITY, not parseability — an injected operand yields text that parses cleanly as a DIFFERENT query). The pointer-twin invariant MUST additionally hold at the SOURCE level: no type assertion or switch on a sealed vocabulary outside a function that normalises. Every guard MUST be mutation-detectable: removing it MUST fail a named test, and a guard that could be TIGHTENED into refusing valid AQL MUST have a positive control that fails when it is.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — across twelve suites. The STRUCTURAL guards are [`openehr/aql/parse/dispatch_tripwire_test.go`](../../openehr/aql/parse/dispatch_tripwire_test.go) (`TestSealedVocabularyDispatchSitesNormalise` — source-level sweep, shape list derived from marker-method receivers — and `TestDerefSwitchesCoverEveryShape`, which holds the deref switches themselves exhaustive over the same derived shape sets, both carrier forms) and [`openehr/aql/parse/value_position_parity_test.go`](../../openehr/aql/parse/value_position_parity_test.go) (`TestEveryValueKindInEveryPositionRoundTripsOrRefuses` — the position × kind × carrier sweep). The grammar confrontations are `TestReservedFuncNamesTrackTheGrammar`, `TestMatchesURIGuardTracksTheGrammar`, and — in [`openehr/aql/parse/identifier_parity_test.go`](../../openehr/aql/parse/identifier_parity_test.go) — `TestIdentifierGuardTracksTheGrammar`, `TestArchetypeIDGuardTracksTheGrammar` and its generated counterpart `TestArchetypeIDGuardAgreesOverGeneratedHRIDs`, which hold the single-token identifier and archetype-id guards to the same standard, with the Emit- and Build-side refusals beside them (`TestEmitRefusesFieldsItWouldSilentlyDrop` for the drop direction, `TestBuildEmitParityModuloTrim` for the normalised-string parity claim, and `TestBuilderExpressesTheVersionAlternative` for the one carve-out in the RM-type position, whose "built/" half is the positive control that fails if the carve-out is removed). The class and VERSION bracket positions are held by [`openehr/aql/predicate_test.go`](../../openehr/aql/predicate_test.go) (the scanner-state table, one row per state the escape scan tracks) and, in `openehr/aql/parse`, [`predicate_guard_test.go`](../../openehr/aql/parse/predicate_guard_test.go) (`TestEmitVersionPredicateHoldsItsOwnSubGrammar` for the two-sub-grammar split, `TestEmitClassPredicateRefusesBracketEscape`, `TestEmitClassPredicateGuardRefusesNothingTheParserAccepts` for the tightening property, `TestEmitClassPredicateAcceptsLoudMalformations` for the negative space no generated corpus can reach, and `TestEmitClassPredicateGuardsEveryClassPosition` for the containment call site), [`predicate_confrontation_test.go`](../../openehr/aql/parse/predicate_confrontation_test.go) (the generated two-sided confrontation, spliced into a class base and a VERSION base, with the per-base rail that fails when no splice in that base parses as a DIFFERENT query, plus `TestVersionPredicateShapeAgreesWithTheParser` — the legal-operand corpus over which the VERSION position's accepted text must re-parse outright, which is the arm the shared corpus cannot carry) and [`predicate_parity_test.go`](../../openehr/aql/parse/predicate_parity_test.go) (round-trip IDENTITY over the whitespace-, comment- and TERM_CODE-bearing corpus). The after-emission verification is held by [`emit_verify_test.go`](../../openehr/aql/parse/emit_verify_test.go) — `TestEmitRefusesACrossBracketRegexSubstitution` (the cross-bracket residual, on the structural arm, with the redaction assertion), `TestEmitVerificationAcceptsEveryReNestedEncoding` (the anti-tightening arm over the AST encodings the parser is free to re-nest), `TestEmitVerificationPinsEachCarriedCoordinate` (the splice-reachable half of the per-coordinate table) and `TestEmitNeverReturnsUnparseableText` (the underlying property, over a corpus that reports itself if it goes one-sided) — and by [`emit_verify_internal_test.go`](../../openehr/aql/parse/emit_verify_internal_test.go) (`TestSkeletonDistinguishesEveryCoordinate`, the white-box census half, plus the fail-closed switch arms no external test reaches). The per-rule corpora live in [`openehr/aql/parse/literal_roundtrip_test.go`](../../openehr/aql/parse/literal_roundtrip_test.go) (string escaping both directions and both delimiters, `FuzzStringLiteralRoundTrip`, numeric boundaries and spellings) and [`openehr/aql/parse/emit_parity_test.go`](../../openehr/aql/parse/emit_parity_test.go) (Emit-side structural refusals, no-panic boundaries, Build/Emit/FormatWhere agreement); the write-side unit refusals (narrow operand slots, parameter spelling, TERMINOLOGY arity, `EqualValues`) in [`openehr/aql/value_test.go`](../../openehr/aql/value_test.go). File-level citation is deliberate: the suites are the probe's surface, and per-test names live in the files themselves, where a rename cannot rot this entry.
- **Satisfies:** REQ-119.

#### PROBE-095 — AQL predicate structuring

- **Title:** Every bracketed path predicate the vendored grammar admits at a path-segment position arrives as a typed, trivia-free, delimiter-free structured value beside its untouched verbatim text — or as a member of the enumerated unstructured set, nil beside the verbatim text.
- **Preconditions:** The vendored grammar at `resources/aql/grammar/active/AqlParser.g4`, from which the corpus is **generated** — a hand-written case list is the "list a maintainer must remember to update" [clinical-modeling.md § REQ-119 § Acceptance](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) forbids, and the failure it forbids is a grammar alternative silently uncovered.
- **Wire assertion:** In-repo property. The RULES are [clinical-modeling.md § REQ-113 § Structured node predicates](clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast)'s — the kinds and their components, the form-not-parse-node rule, the five name spellings on both carrying alternatives, the enumerated unstructured set, nil-not-partial, the untouched class position, emission ignoring the model — and are not restated here. What this probe carries is the oracle CONSTRUCTION: the corpus is generated from the grammar's `pathPredicate` alternatives **including the `pathPredicateOperand` production they reach** (whose members decide structured-versus-enumerated-unstructured), and the sweep MUST fail when the grammar admits a form neither a kind nor the enumerated set covers, shown able to fail by a control. Every form is presented at BOTH positions it can occupy — bracket top level and junction operand — and MUST yield the same kind at each; each enumerated unstructured form MUST assert nil-beside-populated-verbatim at both positions. Trivia and escape independence is asserted as a property driven from the same generated corpus — a mechanically padded spelling MUST produce components equal to the bare spelling's WHILE the verbatim text differs, both arms firing — with the interior-trivia spellings no mechanical padding generates (inside an operand path, a name slot, regex braces) carried as named rows beside it. Emission parity is asserted rather than assumed: the REQ-119 round-trip suites stay green and emitted text byte-identical.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — [`openehr/aql/parse/predicate_structure_test.go`](../../openehr/aql/parse/predicate_structure_test.go). `TestEveryGrammarAlternativeIsCovered` reads every alternative of the three predicate productions and `pathPredicateOperand` out of the vendored grammar, `TestTheGrammarSweepCanFail` is its able-to-fail control, `TestEveryFormStructures` + `TestTheSameFormYieldsTheSameKindInBothPositions` hold the both-positions rule (enumerated-nil rows included), `TestTriviaIndependenceOverTheGeneratedCorpus` drives the padding property from the corpus with `TestTriviaAndEscapesDoNotReachTheComponents` carrying the interior spellings, `TestEveryNameSpellingIsDiscriminated` covers the five name spellings on both carriers, `TestBothExtractorsAgreeOnTheStructure` keeps the two path extraction sites in step, and `TestEmissionIsUnaffected` asserts the REQ-119 fixed point.
- **Satisfies:** REQ-113.

#### PROBE-096 — AQL value-free structured diagnostics

- **Title:** Every construct the extractor drops carries a structured `{kind, clause, span}` record, every lint issue carries a span, no field the contract classifies value-free contains any substring of the input's value spans, and each span points at the construct the diagnostic is about.
- **Preconditions:** A corpus reaching every **reachable** drop class — as of [clinical-modeling.md § REQ-119 § Amends REQ-117](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) that is a numeric literal outside the value vocabulary, in each clause that admits one — plus a representative lint-issue set spanning the layers `lint` runs (parameter-binding cases run with `Options.Query` set, since their codes fire on no other path), and, for each corpus query, the **value spans** of its input identified independently of the diagnostic under test.
- **Wire assertion:** In-repo property, three arms. The RULES are [clinical-modeling.md § REQ-113 § Value-free structured drop records](clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast)'s and [§ REQ-109 § Value-free lint diagnostics](clinical-modeling.md#req-109--aql-static-lint)'s — the two-class field contract, the accessor's four properties, the classes-not-sites rule, the clause and span reuse, additivity — and are not restated here. What this probe carries is the oracle construction per arm. **(a) Record per drop:** completeness is held by a sweep derived from the SOURCE of every non-test file in the package — so a site added later, in any file, is covered the day it lands — and the sweep MUST be shown able to fail; the defensive arms' unreachability by a legal query is asserted at the unit level, since no corpus can reach them and a probe demanding one would be weakened instead of satisfied; the source-order rule is exercised by one query carrying drops in two clauses. **(b) Value-free fields:** for every corpus query, no value-free field of either surface — rendered WHOLE, span included, so a future string field cannot slip past — may contain any substring of that input's value spans, asserted against the INPUT and never against expected strings; the value-bearing fields are deliberately NOT asserted clean, and each case pins the code it exists to exercise so it cannot pass on bystander issues. Additivity is asserted over the corpus: codes, severities and `OK()` outcomes match their pre-change values. **(c) Span attribution:** each span is verified by slicing it back out of the input and matching the offending construct rather than its enclosing clause, in one span type across both packages (a compile-time alias check), with an unattributable span zero rather than approximated.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — [`openehr/aql/parse/dropped_test.go`](../../openehr/aql/parse/dropped_test.go) (arm (a): the reachable-drop corpus, the accessor's four properties, the package-wide source sweep and its able-to-fail control, the defensive-arm unreachability claim, source order; arm (c): the span sliced back out of the source) with the latent span-helper guards in [`openehr/aql/parse/span_internal_test.go`](../../openehr/aql/parse/span_internal_test.go), and [`openehr/aql/lint/span_test.go`](../../openehr/aql/lint/span_test.go) (arm (b) on the lint surface with per-case must-find codes and the additivity pin, the alias span, the zero-width syntax span agreeing with `Detail`'s line:col, and the one-span-type rule held by a compile-time check).
- **Satisfies:** REQ-113, REQ-109.

#### PROBE-097 — AQL semantic and portability lint corpus

- **Title:** Every REQ-161 issue code fires on a corpus query built to carry exactly that defect, with the REQ-161 severity and a span on the offending construct; queries with no REQ-161 defect yield no REQ-161 code; and the builder's verification agrees with the linter on the containment-check subset (REQ-162; the five containment codes, not the lint-only portability codes).
- **Preconditions:** The vendored grammar profile (parse) and the pinned BMM via `rminfo` — no CDR, no OPT. The [REQ-160 acceptance table](clinical-modeling.md#req-160--aql-containment-admissibility-relation) is the relation oracle; the EHRbase admissibility matrix (maintainer's knowledge base) is a neutral observed-behaviour cross-check, including the EHRbase-compatibility guard (an RM-valid pair a conformant engine admits never verdicts Never).
- **Wire assertion:** In-repo property, three arms. **(a) Per-code corpus:** for each REQ-161 code, at least one corpus query yields it with the specified severity at the span of the offending construct, and at least one near-miss query stays silent — the unknown-class suppression rule (an unknown operand suppresses the pair checks), the archetype-mismatch suppression (a literal archetype predicate whose declared class or HRID type segment is unknown yields `aql_unknown_rm_class` only, never `aql_archetype_class_mismatch`) and the `aql_fanout_row_grain` conservative firing rule each pinned by their own negative case. **(b) Additivity guard:** every query of the pre-REQ-161 corpus that carried no REQ-161 defect yields an unchanged issue-code multiset; corpus queries that genuinely carry a newly-checked defect are re-baselined deliberately, recorded here. **(c) Read/write parity (REQ-162):** for corpus queries expressible through the builder, the verification entry point's code multiset equals the containment-check subset of `lint.LintString` over the emitted text.
- **Modes:** In-repo (unit-level; no backend).
- **Status:** Implemented (inline) — see [`testkit/probes/aql/probe_097_semantic_lint.go`](../../testkit/probes/aql/probe_097_semantic_lint.go), run by `TestProbe097SemanticLint` in [`testkit/probes/aql/probes_test.go`](../../testkit/probes/aql/probes_test.go). Arm (a) pins all eight REQ-161 codes with severity and span, plus the three mandatory negative cases (unknown-class suppression, archetype-mismatch suppression, the `aql_fanout_row_grain` conservative firing rule). The unknown-class suppression negative is additionally backed by a Never-verdict sibling row that observably arms the operand-level suppression rule; the pure unknown-operand case is redundant-by-construction under REQ-160's pair-question totality and is pinned at engine level instead by `semcheck_test.go`'s `TestPairSuppressedByOperandVerdict`. Arm (b) re-ran PROBE-028's three-cassette corpus under the completed linter: zero REQ-161 codes gained on any of them — **no re-baseline required**, confirming the controller's precomputed prediction. Arm (c) holds read/write parity over the corpus's builder-expressible rows (the five containment codes) and asserts the corpus is not vacuous (every containment code is exercised by at least one row, and at least one row is clean); `VerifyContainment` never emits the three portability codes, and REQ-162 § Contract scopes parity to the containment-check subset — a FROM-root archetype predicate is separately unreachable from the builder (the write-side FROM clause carries no archetype field) — so both are out of scope by REQ-162 § Contract, not by omission.
- **Satisfies:** REQ-160, REQ-161, REQ-162.

#### PROBE-022 — OPT path resolution

- **Title:** Parsing an ADL 1.4 operational template (OPT) and resolving a fixture-defined list of openEHR paths returns nodes whose RM type, archetype node id, and (for archetype roots) archetype id match the expected values; explicitly unknown attributes and unmatched predicates produce `ErrPathNotFound`.
- **Preconditions:** A reference OPT body (XML bytes) and an assertion list mapping paths to expected node identity.
- **Wire assertion:** Sandbox-only — `template.ParseOPT` + `template.ParsePath` + `OperationalTemplate.NodeAt` against the fixture body MUST match every assertion in the list. Negative assertions (`ExpectNotFound`) MUST surface `ErrPathNotFound` (wrapped).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/template/probe_022_opt_path_resolution.go`](../../testkit/probes/template/probe_022_opt_path_resolution.go).

#### PROBE-023 — Composition builder round-trip

- **Title:** Building a composition via `composition.NewBuilder` + `Set` → `Build` → `canjson.Marshal` → `canjson.Unmarshal` → re-marshal preserves the values supplied through `Set` at their addressed paths.
- **Preconditions:** A compiled OPT and a list of (path, value) assignments addressed against it.
- **Wire assertion:** Sandbox-only — `composition.NewBuilder(ctx, c, opts...)` + per-path `Set` + `Build` MUST succeed; `canjson.Marshal` of the result MUST contain the assigned primitive values (magnitude / units for DV_QUANTITY, value string for DV_TEXT, code / terminology for DV_CODED_TEXT) as byte fragments. `canjson.Unmarshal` into a fresh `*rm.Composition` MUST succeed (proving the polymorphic dispatch on `Composition.uid` + nested DataValues works symmetrically); re-marshalling the decoded composition MUST preserve the same fragments.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/composition/probe_023_builder_round_trip.go`](../../testkit/probes/composition/probe_023_builder_round_trip.go). In-memory verification of the built `*rm.Composition` (without canjson) is additionally covered by `TestBuilder_SetQuantity_systolic` in [`openehr/composition/builder_test.go`](../../openehr/composition/builder_test.go).
- **Satisfies:** REQ-101, REQ-082.

#### PROBE-024 — Primitive constraint validate

- **Title:** Parsing an OPT and resolving a fixture-defined list of leaf paths, calling `PrimitiveConstraint.Validate` with a supplied Go value, returns the expected multiset of `ViolationCode` values per case.
- **Preconditions:** A reference OPT body (XML bytes) carrying at least one primitive-constraint child (C_BOOLEAN, C_INTEGER, C_REAL, C_STRING, C_DATE, C_TIME, C_DATE_TIME, C_DURATION, C_CODE_PHRASE, C_DV_QUANTITY, C_DV_ORDINAL) and a case list with positive (no violations) and negative (specific code expectations) entries.
- **Wire assertion:** Sandbox-only — `template.ParseOPT` + path resolution + `(*ComplexObject).PrimitiveConstraint().Validate(value)` MUST match every case's `WantCodes` multiset. Cases with `ExpectNoConstraint` MUST address nodes whose `PrimitiveConstraint()` returns nil.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/template/probe_024_primitive_validate.go`](../../testkit/probes/template/probe_024_primitive_validate.go).
- **Satisfies:** REQ-103, REQ-082

#### PROBE-025 — Composition validate

- **Title:** Parsing an OPT, compiling it, and running `ValidateComposition(comp, c)` over a fixture-defined list of (OPT, composition, expected codes) tuples returns the expected multiset of [`validation.Issue.Code`](../../openehr/validation/issue.go) values per case.
- **Preconditions:** A reference OPT body (XML bytes) and a hand-built or fixture-decoded `*rm.Composition`; each case carries a `WantCodes []string` that captures the multiset semantics (order irrelevant, duplicates count).
- **Wire assertion:** Sandbox-only — `template.ParseOPT` + `templatecompile.Compile` + `validation.ValidateComposition` MUST produce an `Issue.Code` multiset that matches each case's `WantCodes`. Positive cases assert `WantCodes` is empty; primitive / structural mismatches assert specific codes (`primitive_out_of_range`, `primitive_unit_unknown`, `primitive_not_in_list`, `slot_fill`, …).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/validation/probe_025_composition_validate.go`](../../testkit/probes/validation/probe_025_composition_validate.go).
- **Satisfies:** REQ-102, REQ-103, REQ-082

#### PROBE-026 — Missing required nodes / cardinality

- **Title:** Sharpens PROBE-025 with negative structural cases — missing required nodes, empty multi-valued attributes with `existence ≥ 1`, occurrences upper-bound violations, RM-type mismatches under C_SINGLE_ATTRIBUTE alternatives — and asserts the issue-code multiset (`required`, `cardinality`, `rm_type_mismatch`, `alternative_mismatch`) is stable across conformant implementations.
- **Preconditions:** Same OPT + composition tuple shape as PROBE-025; cases focus on the v2 template-driven structural completion surface that the RM-guided intermediate could not detect.
- **Wire assertion:** Sandbox-only — same pipeline as PROBE-025. A composition with the systolic ELEMENT removed surfaces `required` at the ITEM_LIST `/items` path; an empty `events` slice surfaces `required` + `cardinality`; an unmatched alternative surfaces `alternative_mismatch`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/validation/probe_025_composition_validate.go`](../../testkit/probes/validation/probe_025_composition_validate.go).
- **Satisfies:** REQ-102, REQ-082

#### PROBE-027 — Generated instance validates clean

- **Title:** `instance.Generate(c, opts)` followed by `validation.ValidateComposition(out, c)` returns `Result.OK = true` for both `Minimal` and `Example` policies on the same OPT.
- **Preconditions:** Compiled OPT for a fixture template; valid composer + territory for COMPOSITION roots.
- **Wire assertion:** Cross-package round-trip — generator and validator agree on the same template-driven contract.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/instance/probe_027_generated_validates.go`](../../testkit/probes/instance/probe_027_generated_validates.go). Probe runs against both `vital_signs.opt` and `clinical_note.opt` for `Minimal` and `Example` policies. Slot fills draw a conforming archetype id from the parsed REQ-104 include grammar when a safe example can be synthesized, falling back to `openEHR-EHR-<RMType>.example.v1` (the validator's RM-type-prefix fallback path) only when the OPT carried no parseable includes.
- **Satisfies:** REQ-107.

#### PROBE-074 — Template-driven validation of non-COMPOSITION roots

- **Title:** `validation.Validate(root, c)` over a fixture-defined list of (OPT, RM root, expected codes) tuples returns the expected [`validation.Issue.Code`](../../openehr/validation/issue.go) multiset for archetypeable roots outside the COMPOSITION content set — the demographic PARTY hierarchy (`PERSON`/`ORGANISATION`/`GROUP`/`AGENT`/`ROLE` + sub-components) and the EHR-IM roots (`FOLDER`/`EHR_STATUS`).
- **Preconditions:** An OPT body rooted at the target RM type (real `Address.v2.opt` / `TestPerson.v2.opt`, or a synthetic root) and an in-memory or fixture-decoded RM root; each case carries a `WantCodes []string` multiset.
- **Wire assertion:** Sandbox-only — `template.ParseOPT` + `templatecompile.Compile` + `validation.Validate` MUST produce an `Issue.Code` multiset matching each case's `WantCodes`. A conformant ADDRESS instance validates clean; a `PERSON` under an `ORGANISATION` OPT surfaces `rm_type_mismatch`; a `PERSON` missing its OPT-pinned `identities` surfaces `required` + `cardinality`; a `FOLDER` whose archetype id differs surfaces `archetype_id_mismatch`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/validation/probe_074_noncomposition_validate.go`](../../testkit/probes/validation/probe_074_noncomposition_validate.go).
- **Satisfies:** REQ-110, REQ-102, REQ-103.

#### PROBE-075 — WebTemplate structural parity

- **Title:** `webtemplate.Marshal(templatecompile.Compile(opt))` structurally matches a vendored EHRbase `openEHR_SDK` v2.3 reference WebTemplate JSON for the same OPT — the `id` set, `rmType`, `aqlPath`, `min`/`max`, and per-node input `suffix`/`type` — against a documented-deviations list.
- **Preconditions:** Commit-pinned EHRbase `openEHR_SDK` reference WebTemplate JSON + OPT pairs, vendored under [`testkit/cassettes/`](../../testkit/cassettes/) with provenance + Apache-2.0 attribution. Three fixtures are asserted: `constrain_test` (pins **no** template-level node name, so its golden carries **zero** name predicates across all 104 nodes — that, not an absence of sibling collision, is why it reached parity first), and the two [REQ-116](clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) oracles `Corona_Anamnese` (four `SECTION.adhoc.v1` siblings reusing one archetype; the loud failure mode, formerly `ErrIDCollision`) and `GECCO_Diagnose` (the silent one — it always built, but its golden name-predicates 24 `aqlPath`s this builder once emitted bare). REQ-116 closed both: `id` derives from the pinned template-level node name and `aqlPath` carries the name predicate, so all three now hold exact structural parity — 104/104, 230/230, 34/34 — with residual input and `min` deltas documented and count-pinned in [`deviations.md`](../../openehr/template/webtemplate/deviations.md). The earlier reading of the corona failure as "does not compile — archetype reuse under a slot" was incorrect: the cause was shared-path subtrees, now admitted.
- **Wire assertion:** Not backend-facing — an in-repo parity property: `template.ParseOPT` + `templatecompile.Compile` + `webtemplate.Build`/`Marshal` MUST reproduce the reference node structure (id / rmType / aqlPath / min / max + input suffix/type per node); every accepted difference (field ordering, absent optional fields, localized-string packaging, known id edge cases) MUST appear on the documented-deviations list, and any difference not listed is a failure.
- **Modes:** In-repo (parity property against a vendored fixture; no backend).
- **Status:** Implemented (inline) — `TestStructuralParity` + `TestInputParity` in [`openehr/template/webtemplate/`](../../openehr/template/webtemplate/) hold exact structural parity on node structure (id/rmType/nodeId/aqlPath/min/max) and input suffix/type against all three vendored references — `constrain_test` 104/104, `Corona_Anamnese` 230/230, `GECCO_Diagnose` 34/34 — with the documented residual deltas count-pinned per fixture; catalogue in [`deviations.md`](../../openehr/template/webtemplate/deviations.md).
- **Satisfies:** REQ-106; REQ-116 (the two name-pinning oracles are its acceptance fixtures).

#### PROBE-076 — FLAT / STRUCTURED composition round-trip

- **Title:** For a vendored upstream (OPT + canonical COMPOSITION) pair, the simplified codecs round-trip the composition without losing the data the formats carry — FLAT is idempotent (`MarshalFlat → UnmarshalFlat → MarshalFlat`), STRUCTURED re-encodes to the same FLAT, OPT-free interconversion preserves it — **and**, when the source composition is itself OPT-valid, a `WithTemplate` decode of the emitted FLAT validates against the OPT (the conformance leg).
- **Preconditions:** Vendored EHRbase (Apache-2.0) `Test_dv_*` datatype OPTs + matching canonical composition JSON under [`testkit/cassettes/`](../../testkit/cassettes/) (provenance in [`THIRD_PARTY_LICENSES.md`](../../testkit/cassettes/THIRD_PARTY_LICENSES.md)); the WebTemplate is built via `templatecompile.Compile` + `webtemplate.Build` (REQ-106).
- **Wire assertion:** Not backend-facing — an in-repo property across the datatype corpus (DV_TEXT/CODED_TEXT/DATE_TIME/QUANTITY/COUNT/BOOLEAN/ORDINAL/PROPORTION/DURATION/IDENTIFIER/URI + the `|raw`-carried MULTIMEDIA/PARSABLE/INTERVAL): round-trip idempotence plus the OPT-validation leg, which catches dropped or mistyped leaves that a symmetric omission would hide from idempotence alone. **Scope limit:** symmetric omission of *optional* data can still pass, and the probe does **not** compare emitted FLAT/STRUCTURED byte-for-byte against vendored upstream simplified output — that upstream-conformance probe landed as [PROBE-086](#probe-086--upstream-flat-serialisation-parity); see [`deviations.md`](../../openehr/serialize/simplified/deviations.md) § Conformance. A template the WebTemplate builder cannot yet model is a documented skip, never a fail.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (Sandbox) — [`testkit/probes/serialize/probe_076_simplified_round_trip.go`](../../testkit/probes/serialize/probe_076_simplified_round_trip.go), run by `TestProbe076` over the vendored constraint-template corpus (24 pass, 1 skip).
- **Satisfies:** REQ-053.

#### PROBE-077 — RM-floor invariant matrix

- **Title:** A vendored-cassette matrix exercises `ValidateRM` (+ typed sugars) across the REQ-112 per-RM-type invariant catalogue and the RM-mandatory required-set walk, the same way PROBE-025/026 exercise the template-driven floor.
- **Preconditions:** Vendored RM cassettes covering the required-set breaches and the four named leaf invariants (CODE_PHRASE, DV_QUANTITY, DV_INTERVAL, OBJECT_REF-family) documented in [clinical-modeling.md § REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor).
- **Wire assertion:** Not yet defined at cassette granularity — the unit-test cassette matrix in [`openehr/validation/rmfloor_test.go`](../../openehr/validation/rmfloor_test.go) carries first-cycle coverage (required-set absences, the four named invariants, the unbounded-skip negative, and the nil-guard contract) inline, ahead of a dedicated vendored-cassette probe.
- **Modes:** Sandbox (planned); Cassette, Live not yet scoped.
- **Status:** Deferred — REQ-112 is landed and unit-covered; the dedicated PROBE-077 vendored-cassette matrix is deferred to a follow-up cycle (tracked in [roadmap.md](../roadmap.md)).
- **Satisfies:** REQ-112.

#### PROBE-081 — EHR_STATUS value-typed mandatory presence (subject)

- **Title:** `validation.ValidateRMEHRStatusBytes(data)` flags an omitted RM-mandatory `subject` (typed `rm.PartySelf`, a value struct) from JSON-key presence, without false-positiving on a valid bare `PARTY_SELF`.
- **Preconditions:** Canonical-JSON EHR_STATUS bodies — one omitting the `subject` key; one supplying `subject` as a bare `{"_type":"PARTY_SELF"}` (no external_ref); one omitting the interface-typed `name`.
- **Wire assertion:** In-repo property — an EHR_STATUS whose top-level `subject` key is absent (or present but JSON `null`) MUST surface `required` at `/subject`; a present non-null subject (even the bare `PARTY_SELF` that decodes to the Go zero value) MUST NOT; the interface-typed mandatory `name`, when absent, MUST still surface `required` at `/name` (no regression). A non-object / malformed input surfaces a single `invalid_shape` at `/`.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — see [`openehr/validation/rmfloor_bytes_test.go`](../../openehr/validation/rmfloor_bytes_test.go).
- **Satisfies:** REQ-112.

#### PROBE-086 — Upstream FLAT serialisation parity

- **Title:** For each body in the pinned upstream EHRbase FLAT conformance corpus, decoding it and re-encoding it through the SDK's REQ-053 codec reproduces the upstream FLAT — same path set, same leaf values — over the subset of the corpus that the codec models, with everything outside that subset counted rather than waived.
- **Preconditions:** The commit-pinned corpus at [`testkit/cassettes/flat-conformance/`](../../testkit/cassettes/flat-conformance/) — the `conformance_ehrbase.de.v0` OPT plus 34 `ehrbase_conformance_*.json` FLAT bodies from `composition/flat/simSDT/conformance/`, integrity-checked against `MANIFEST.txt` by `make flat-conformance-verify` (the offline gate `make ci` runs; `make flat-conformance-check` adds a network upstream-drift report) (Apache-2.0; provenance in [`THIRD_PARTY_LICENSES.md`](../../testkit/cassettes/THIRD_PARTY_LICENSES.md)). Resolved via `fixtures.FlatConformanceOpt` / `ListFlatConformance`.
- **Wire assertion:** Not backend-facing — an in-repo parity property. `template.ParseFile` + `templatecompile.Compile` + `webtemplate.Build`, then per fixture `simplified.UnmarshalFlat(upstream, wt, WithTemplate(c))` + `simplified.MarshalFlat`. The comparison is scoped to the **modelled subset** — the upstream keys left after (a) removing what the codec refuses on decode and (b) holding a **named allow-list** of composition-level metadata out on *both* sides. That allow-list carries two different things and MUST distinguish them. **Respellings** — `language`, `territory`, `composer|name`, `composer_self`, `context/start_time` and `context/setting`, which REQ-053 reads and writes as the `ctx/` short forms (`ctx/language`, `ctx/composer_name`, `ctx/time`, `ctx/setting|code` + `|value`) where upstream writes real paths: comparing across the two spellings would report every such key as **Missing**, since the `ctx/` key the encoder wrote in its place is itself skipped on the emitted side and so never surfaces as Extra. The hold-out is therefore **suffix-aware**, not per-path: the composer's `external_ref` suffixes (`composer|id`, `|id_scheme`, `|id_namespace`) are not respellings and MUST NOT be held out — decode refuses them (`PARTY_PROXY`), which is exactly where the census should show them, in the excluded set. `context/setting` was this list's one documented **waiver** of a real encode-side drop while `ctx/setting` emission was deferred; with that emission landed (REQ-053, [ADR 0015](../adr/0015-flat-metadata-spelling.md)'s left-open gap closed) it is an ordinary respelling — the real path decodes, re-encodes as `ctx/setting|code` + `|value`, and its keys are held out on both sides exactly like `context/start_time`. The hold-out therefore carries **no waiver**: every held-out spelling MUST resolve to an accepted alias or a terminology witness. The hold-out allow-list MUST stay in step with the codec's accepted-alias table, and that agreement MUST be enforced mechanically rather than by review: the harness derives its expected hold-out from the codec's exported alias accessors (`simplified.MetadataAliasSpellings` / `MetadataWitnessSpellings`) and asserts the reverse direction as well — every held-out spelling MUST resolve to an accepted alias or a terminology witness (the waiver class is empty since `ctx/setting` emission landed, and MUST stay empty — a new waiver is a spec decision, not a harness edit). So an alias accepted on decode but missing from the hold-out (a spurious Missing) and a hold-out with no alias behind it (a real key silently suppressed) both fail the harness's own tests rather than being caught by eye. The hold-out MUST stay an explicit allow-list rather than a prefix test: `context/other_context` carries archetyped data and MUST NOT be held out, and `category` MUST be compared — it is a template-constrained Web Template leaf riding its own FLAT path, spelled identically on both sides, not composition metadata. Within that subset the re-encode MUST reproduce the upstream path set with equal leaf values: a dropped, invented, or altered key is a failure, and there MUST NOT be a tolerated-drop list — an upstream key that decodes and then does not re-encode is a defect, not a skip. The excluded set MUST be derived from the codec's own decode refusals rather than hand-maintained, so that closing a gap widens coverage without a list to update; each refusal MUST remove only what it names, so a single unmodelled `|suffix` does not withdraw the modelled suffixes beside it. Excluded and compared counts MUST be pinned per fixture — in the harness package's own tests, which own that ratchet — so the unmodelled surface can shrink deliberately but never grow unnoticed. Independently of the pins, a fixture that compares **nothing** MUST fail: exact agreement over an empty compared set is vacuous, so a total coverage collapse MUST NOT read as a pass. This is strictly stronger than [PROBE-076](#probe-076--flat--structured-composition-round-trip), whose input is the SDK's *own* output and so cannot catch a path this SDK never emits, a suffix it names differently, or a leaf it silently drops — the upstream-conformance follow-up PROBE-076's scope limit names.
- **Modes:** In-repo (parity property against vendored fixtures; no backend) — REQ-082's declared class for a probe that asserts no wire exchange, not a mode shortfall.
- **Status:** Implemented (Sandbox) — harness at [`testkit/conformance/webtemplate/`](../../testkit/conformance/webtemplate/) (one sub-test per corpus fixture in `runner_test.go`), probe wrapper at [`probe_086_upstream_flat_parity.go`](../../testkit/probes/serialize/probe_086_upstream_flat_parity.go), run by `TestProbe086`. The probe is **exact** on what it compares — a dropped, invented, or altered key inside the modelled subset fails, with no tolerated-drop list — and contributes the **coverage floor**: it fails a fixture that compared zero keys, which `Report.Clean` would otherwise report as vacuously clean. The per-fixture excluded/compared **ratchet** is the harness package's own pins in `runner_test.go`, not the probe's; that is what keeps the unmodelled surface shrinking deliberately and never growing silently. The measured census — modelled-subset size, refusal inventory, root causes, and what would move the number — lives in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md), regenerated with the harness's `-census` flag; its first-run catch (a silent encode-side data loss, fixed in `rmpath` under REQ-121) is recorded in the plan close-out.
- **Satisfies:** REQ-080 (advances); exercises REQ-053 and REQ-106.

#### PROBE-089 — Underscore-attribute round-trip

- **Title:** Every [REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes) underscore-attribute family round-trips FLAT → canonical RM → FLAT byte-identically, and an RM composition populated with an in-scope attribute emits it without loss — per family, including the recursive shapes.
- **Preconditions:** The PROBE-086 corpus (underscore keys are ~60% of its 1824 keys), plus SDK-authored per-family fixtures against the vendored OPTs covering each row of the REQ-140 grammar table — including recursion (`_feeder_audit/…/provider/_identifier:N`, `dv_multimedia/_thumbnail`), `_null_flavour` beside an absent bare value, and the deliberate refusals.
- **Wire assertion:** In-repo, not backend-facing. Per grammar family: (a) **decode** — a FLAT body carrying the family's keys decodes into the typed RM attribute (never a `|raw` detour), and re-encoding reproduces the input byte-for-byte; (b) **encode** — a composition populated with the attribute emits exactly the family's key set: no silent drop, and no `|raw` fallback where the grammar carries the value; (c) **refusals** — the composer `external_ref` / composer `_identifier:N`, `_instruction_details`, and `_wf_definition` keys fail with typed errors naming the key, and MUST NOT decode-and-drop; (d) **STRUCTURED** — the same fixtures interconvert FLAT ↔ STRUCTURED preserving the underscore vocabulary (`_`-keys as members, arrays for `:N`). The PROBE-086 census MUST move in step: the `path not in web template` excluded family shrinks by the landed underscore keys and the per-fixture compared/excluded pins are re-baselined in the same change — a landed family whose corpus keys stay excluded is a failure of this probe, not a census footnote.
- **Modes:** In-repo (Sandbox); no backend.
- **Status:** Implemented (Sandbox) — [`testkit/probes/serialize/probe_089_underscore_round_trip.go`](../../testkit/probes/serialize/probe_089_underscore_round_trip.go), run by `TestProbe089` (14 SDK-authored per-family fixtures, one per grammar-table row, against the vendored corpus OPT) and `TestProbe089Refusals` (the deliberate exclusions), with `TestProbe089FrameworkMisuse` separating "could not run" from "the codec is wrong". What each leg pins: **(a)** byte-exactness over the *whole* fixture body, so a family cannot be carried at the cost of a key beside it; **(b)** the decoded composition goes out through canonical JSON and back before the re-encode, which is what makes this an encode assertion rather than a second reading of (a) — an underscore value parked anywhere the canonical form does not model vanishes there — and the emitted family key set must match exactly, with a `|raw` at a base the grammar carries reported as its own failure; **(c)** each exclusion must fail with the sentinel its boundary declares and name the offending key, a successful decode being the decode-and-drop this REQ forbids; **(d)** the OPT-free interconversion must carry the `_` vocabulary as array-valued members and the OPT-driven STRUCTURED round-trip must return the same FLAT. The census movement (a) requires is recorded in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md) — 360 → 1466 compared keys over Phases C0–C3, with the per-fixture pins re-baselined in each landing commit. The probe's bite is verified by mutation, not assumed: disabling the LOCATABLE owner walk fails 8 fixtures and disabling the value-decoration emitter 5, so deleting the behaviour fails the probe and not only a package test.
- **Satisfies:** REQ-140; advances REQ-080 (census); exercises REQ-053.

### Canonical JSON and formats

#### PROBE-030 — Canonical-JSON round trip

- **Title:** Decoding a canonical-JSON Composition and re-encoding produces byte-identical output (modulo documented field ordering).
- **Preconditions:** A reference Composition cassette.
- **Wire assertion:** `serialize.Decode → struct → serialize.Encode` produces output that, after the SDK's canonical-ordering pass, matches the input.
- **Modes:** Sandbox (no network).
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_030_canjson_round_trip.go`](../../testkit/probes/serialize/probe_030_canjson_round_trip.go).
- **Satisfies:** REQ-052, REQ-040, REQ-082

#### PROBE-031 — `_type` discriminator decoded via registry

- **Title:** A `_type` not in the type registry decodes to a typed `UnknownTypeError`, not silently to `map[string]any`.
- **Preconditions:** A cassette containing an unregistered `_type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown `_type` value.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_031_typereg_unknown_type.go`](../../testkit/probes/serialize/probe_031_typereg_unknown_type.go).
- **Satisfies:** REQ-040, REQ-052

#### PROBE-038 — RM polymorphic decode coverage

- **Title:** `canjson.Unmarshal[Composition]` decodes every BMM-admissible `_type` discriminator at every substitutable slot — including (a) substitutable subtypes in concrete-typed slots (e.g. `LOCATABLE.name` carrying `DV_CODED_TEXT`) per openEHR RM Liskov substitution, and (b) generic types with abstract type parameters (e.g. `DV_INTERVAL[T: DV_ORDERED]`).
- **Preconditions:** Vendored RM cassettes under `testkit/cassettes/rm/polymorphic/` covering both failure modes.
- **Wire assertion:** Decode succeeds; the recovered tree preserves every original `_type` discriminator (no silent narrowing on substitutable slots); re-marshalling produces wire-equivalent JSON for the same logical content (canonical JSON ordering wins ties).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — [`testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go`](../../testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go) exercises decode → re-marshal → `_type`-preservation on canjson across [`testkit/cassettes/rm/polymorphic/`](../../testkit/cassettes/rm/polymorphic/). The probe's scope is canjson; the underlying narrow-interface generator emission (`<Parent>Like` per BMM ancestors graph) lifts both canjson and canxml dispatch in the same change, but only canjson is asserted here. Plan: [`docs/plans/archive/2026-05-26-rm-polymorphic-decode-coverage.md`](../plans/archive/2026-05-26-rm-polymorphic-decode-coverage.md).
- **Satisfies:** REQ-040, REQ-052

#### PROBE-033 — Canonical-XML round trip

- **Title:** Decoding a canonical-XML Composition and re-encoding produces byte-identical compact XML (modulo documented element/attribute ordering).
- **Preconditions:** A reference Composition XML fixture under `testkit/cassettes/compositions/` or `testkit/cassettes/rm/` (see [Vendored fixtures](#vendored-fixtures-testkitcassettes)).
- **Wire assertion:** `canxml.Unmarshal → struct → canxml.Marshal` produces output that matches the input after the SDK's compact-XML canonicalisation pass.
- **Modes:** Sandbox (no network).
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_033_canxml_round_trip.go`](../../testkit/probes/serialize/probe_033_canxml_round_trip.go).
- **Satisfies:** REQ-056, REQ-040, REQ-082

#### PROBE-034 — `xsi:type` discriminator decoded via registry

- **Title:** An `xsi:type` not in the type registry decodes to `typereg.ErrUnknownType`, not silently to an untyped value.
- **Preconditions:** A cassette (or hand-crafted XML) containing an unregistered `xsi:type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown type value, wrapped in `*typereg.DecodeError`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_034_typereg_xsi_unknown.go`](../../testkit/probes/serialize/probe_034_typereg_xsi_unknown.go).
- **Satisfies:** REQ-040, REQ-056

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
- **Status:** Implemented (Sandbox) — see [`testkit/probes/discovery/probe_040_catalog_ttl.go`](../../testkit/probes/discovery/probe_040_catalog_ttl.go).
- **Satisfies:** REQ-070, REQ-072

#### PROBE-041 — Catalog refresh on 401

- **Title:** A `401` from a previously-working endpoint triggers exactly one discovery refresh and one retry; failure to recover surfaces a typed error.
- **Preconditions:** Cached catalog; backend rotates and returns `401` on the cached token.
- **Wire assertion:** SDK refreshes JWKS/catalog once, retries once. On second `401`, returns `transport.ErrUnauthorized`.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — discovery-layer half — see [`testkit/probes/discovery/probe_041_catalog_refresh_on_401.go`](../../testkit/probes/discovery/probe_041_catalog_refresh_on_401.go). The probe asserts the resolver's `Refresh` against a 401 upstream issues exactly one fetch and returns a typed `*discovery.DiscoveryError(fetch_failed)`. The transport-layer 401→reauth hook is wired (opt-in via `transport.WithReauthOn401` + `auth.ReautherFunc`; see PROBE-007 in [`testkit/probes/auth/probe_007_transport_refresh.go`](../../testkit/probes/auth/probe_007_transport_refresh.go)); the REQ-071-bullet-3 application pattern (discovery-catalog-refresh on 401) remains the pending closure.
- **Satisfies:** REQ-071 (discovery half), REQ-072

### REST binding

The REST-binding probes assert the openEHR-REST 1.1.0-development wire contract above `transport/` and the typed leaf clients under `openehr/client/`. PROBE-040 and PROBE-041 are taken by the service-discovery range; the REST-binding range starts at PROBE-060 (next free range after Observability 050–059) per the [Adding probes](#adding-probes) rule.

#### PROBE-060 — EHR creation round-trip

- **Title:** `POST /ehr` with an initial `EHR_STATUS` body returns `201`, surfaces the assigned `ehr_id`, and a follow-up `GET` returns the same status.
- **Preconditions:** Backend supports server-assigned `ehr_id`.
- **Wire assertion:** POST returns `201` with `Location` header; SDK extracts `ehr_id`; a subsequent GET returns the same EHR_STATUS.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft.

#### PROBE-061 — Composition versioned write with `Prefer: return=representation`

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` returns a bare `COMPOSITION` body plus a new `ETag` (REQ-094).
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Request carries `Prefer: return=representation`; response body decodes as bare `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` schema (oneOf: `Composition` | `Identifier`) — **not** an `ORIGINAL_VERSION<COMPOSITION>` envelope, which lives at `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`). The response `ETag` is captured into `VersionMetadata`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) via PROBE-071 — the bare-body wire assertion (and the symmetric PUT path) is exercised by [`testkit/probes/versioned/probe_071_composition_write_response_shape.go`](../../testkit/probes/versioned/probe_071_composition_write_response_shape.go) and the strict-against-spec unit pins `TestSaveRepresentationDecodesBareComposition`, `TestSaveRepresentationRejectsOriginalVersionShape`, `TestUpdateRepresentationDecodesBareComposition`, and `TestUpdateRepresentationRejectsOriginalVersionShape` in [`openehr/client/ehr/composition/composition_test.go`](../../openehr/client/ehr/composition/composition_test.go). PROBE-061 stays as the named "Composition versioned write with `Prefer: return=representation`" probe in the REST-binding range; PROBE-071 is the REQ-094-anchored superset covering both POST and PUT with the strict-rejection assertion.
- **Satisfies:** REQ-094 (`return=representation` arm only). The `Prefer=identifier` slot and empty-body strictness are now landed (leaf unit tests) — see the archived [`2026-05-25-req094-prefer-followups.md`](../plans/archive/2026-05-25-req094-prefer-followups.md).

#### PROBE-062 — `openehr-audit-details` header round-trip

- **Title:** A write carrying `openehr-audit-details` is reflected in the resulting Contribution's audit envelope on read-back.
- **Preconditions:** Existing EHR; a known `*rm.AuditDetails` value.
- **Wire assertion:** Write request carries `openehr-audit-details` in the openEHR dotted-attribute grammar (`change_type.code_string="…",committer.name="…",system_id="…"` — REQ-059, **not** JSON); subsequent Contribution GET returns the same audit fields (committer name, change-type, system_id).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Draft. Encoder unit-covered by `openehr/client/ehr/audit_test.go` (dotted-grammar golden); the read-back round-trip remains to be probe-ratified.

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
- **Status:** Implemented (Sandbox) — see [`testkit/probes/definition/probe_067_template_upload_round_trip.go`](../../testkit/probes/definition/probe_067_template_upload_round_trip.go).

#### PROBE-068 — Error envelope decodes into `WireError.OpenEHR`

- **Title:** A `400 Bad Request` carrying a `{message, code}` JSON body surfaces as a `transport.WireError` whose `OpenEHR` detail is populated and which matches a typed error via `errors.As`.
- **Preconditions:** Cassette of a real 400 error envelope.
- **Wire assertion:** `errors.As(err, &wire)` succeeds; `wire.OpenEHR.Message`, `wire.OpenEHR.Code` are set from the envelope; `wire.RawBody` preserves the raw bytes.
- **Modes:** Sandbox, Cassette.
- **Status:** Draft.

#### PROBE-069 — `Idempotency-Key` header round-trip

- **Title:** A POST/PUT write that carries `Request.IdempotencyKey` emits the `Idempotency-Key` HTTP header verbatim and surfaces it on the OTel span as `http.request.idempotency_key`.
- **Preconditions:** Backend accepts the header (no server-side dedup behaviour required for the SDK-side assertion).
- **Wire assertion:** Captured request headers include `Idempotency-Key: <value>` exactly as supplied; absent when `IdempotencyKey` is empty.
- **Modes:** Sandbox.
- **Status:** Deprecated — REQ-097 deprecated; Cadasto openEHR services no longer accept `Idempotency-Key`. Removal target: v1.0.0. Sandbox assertion removed from the tree pre-1.0 (was `TestDoIdempotencyKey` in `transport/client_test.go`).
- **Satisfies:** REQ-097

#### PROBE-070 — Admin `DeleteEHR` round-trip

- **Title:** `DELETE /admin/ehr/{ehr_id}` returns 2xx; a subsequent `GET /ehr/{ehr_id}` returns 404 surfaced as `transport.ErrNotFound`.
- **Preconditions:** Backend exposes the ITS-REST `/admin/*` surface; admin deletion is enabled for the tenant.
- **Wire assertion:** `admin.DeleteEHR` succeeds; `errors.Is(ehr.Get(...), transport.ErrNotFound)` is true after the delete.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — happy-path delete + missing-EHR variants covered by [`openehr/client/admin/admin_test.go`](../../openehr/client/admin/admin_test.go). A standalone probe file (`testkit/probes/admin/`) is deferred.
- **Satisfies:** REQ-099

#### PROBE-071 — Composition POST/PUT response body is bare COMPOSITION

- **Title:** `POST /ehr/{ehr_id}/composition` and `PUT /ehr/{ehr_id}/composition/{vo_uid}` with `Prefer: return=representation` return a bare `COMPOSITION` body — not an `ORIGINAL_VERSION<COMPOSITION>` envelope.
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Response body decodes cleanly as `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` / `200_COMPOSITION_updated` schemas. A server that returns `{"_type":"ORIGINAL_VERSION", ...}` on these paths is non-conformant; the SDK surfaces that as a decode error (strict-against-spec posture per REQ-094). The full version envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_071_composition_write_response_shape.go`](../../testkit/probes/versioned/probe_071_composition_write_response_shape.go) which exercises both POST and PUT arms in a single invocation when `voID` and `ifMatch` are supplied; otherwise the PUT arm is skipped and the probe still passes on POST alone. Unit-level pins covering both verbs and both halves of the strict-against-spec contract: `TestSaveRepresentationDecodesBareComposition`, `TestSaveRepresentationRejectsOriginalVersionShape`, `TestUpdateRepresentationDecodesBareComposition`, and `TestUpdateRepresentationRejectsOriginalVersionShape` in [`openehr/client/ehr/composition/composition_test.go`](../../openehr/client/ehr/composition/composition_test.go). The same shape applies to `directory.Save` / `directory.Update` per `201_directory` / `200_FOLDER_retrieved`; covered by `TestSaveRepresentationDecodesBareFolder`, `TestSaveRepresentationRejectsOriginalVersionShape`, `TestUpdateRepresentationDecodesBareFolder`, and `TestUpdateRepresentationRejectsOriginalVersionShape` in [`openehr/client/ehr/directory/directory_test.go`](../../openehr/client/ehr/directory/directory_test.go).
- **Satisfies:** REQ-094 (`return=representation` arm only).

#### PROBE-072 — Contribution submission body matches `Contribution_create`

- **Title:** `POST /ehr/{ehr_id}/contribution` request body is the ITS-REST `Contribution_create` shape — `{audit, versions: [ORIGINAL_VERSION<T>|IMPORTED_VERSION<T> with inline data]}` — not the persisted `rm.Contribution` shape whose `versions[]` is `[]OBJECT_REF`.
- **Preconditions:** Existing EHR; at least one resource payload (`Composition` / `EHRStatus` / `Folder` / `EHRAccess`) to commit.
- **Wire assertion:** Captured request body has `versions[i]._type ∈ {"ORIGINAL_VERSION","IMPORTED_VERSION"}` and carries the resource payload inline under `data`. A request body whose `versions[]` contains `{"_type":"OBJECT_REF", ...}` is non-conformant per the ITS-REST OpenAPI `Contribution_create` schema (the persisted `OBJECT_REF` shape returns at read time only). **Commit-audit shape (SPECITS-95 / ITS-REST PR 131):** the batch `audit` and each `versions[i].commit_audit` MUST omit the server-assigned `time_committed` and carry a `DV_CODED_TEXT`-shaped `change_type` (nested `defining_code`), not the erroneous flat `TERMINOLOGY_CODE`. The SDK emits `_type:"AUDIT_DETAILS"` by default (conformant servers accept it); the `UPDATE_AUDIT` form is available as a caller-selected fallback.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — `contribution.Submission` lands in [`openehr/client/ehr/contribution/submission.go`](../../openehr/client/ehr/contribution/submission.go) and `contribution.Commit` now takes `*Submission`; the probe is at [`testkit/probes/versioned/probe_072_contribution_submission_shape.go`](../../testkit/probes/versioned/probe_072_contribution_submission_shape.go). The commit-audit DTO (`contribution.UpdateAudit` + write-version wrappers `OriginalVersion`/`ImportedVersion`) drops the server-assigned `time_committed`; the probe asserts both the version shape and the audit shape. Unit-level pins `TestCommitSubmissionShape` and the `update_audit` / `version` tests cover the SDK leaf. Plans: [`docs/plans/archive/2026-05-26-contribution-submission-shape.md`](../plans/archive/2026-05-26-contribution-submission-shape.md) (REQ-050/095) and the UPDATE_AUDIT/DvCodedText follow-up (SPECITS-95).
- **Satisfies:** REQ-050, REQ-095.

#### PROBE-073 — Demographic PARTY polymorphic round-trip

- **Title:** A PARTY of each concrete type (PERSON / ORGANISATION / GROUP / AGENT / ROLE) round-trips through create → get → get-version with its `_type` discriminator decoded back into the same concrete type at every hop.
- **Preconditions:** A server (Sandbox: fake) serving the typed PARTY body for `POST|GET /demographic/{type}[/{uid_based_id}]` and the `ORIGINAL_VERSION<PARTY>` envelope for `GET /demographic/versioned_party/{vo_uid}/version`.
- **Wire assertion:** Sandbox — `demographic.Create` (Prefer=representation), `demographic.Get`, and `demographic.GetVersion` each MUST decode the response into the same concrete Go type as the input PARTY (REQ-040). The VERSION hop additionally exercises the `ORIGINAL_VERSION<PARTY>` envelope whose generic `data` cannot decode into the abstract `rm.Party` interface and is re-decoded by `_type` through the type registry. A body whose `_type` decodes to a different concrete type is non-conformant.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/demographic/probe_073_demographic_round_trip.go`](../../testkit/probes/demographic/probe_073_demographic_round_trip.go); the leaf is covered by the `openehr/client/demographic` unit tests. Demographic API maturity is Draft (upstream ITS-REST `x-status: DEVELOPMENT`).
- **Satisfies:** REQ-040, REQ-050.

#### PROBE-078 — `POST /query/aql` scopes via `openehr-ehr-id` header

- **Title:** An EHR-scoped ad-hoc AQL execution via `POST /query/aql` carries the scope on the `openehr-ehr-id` request header, per the verb-aware scoping rule in [wire.md § AQL executor](wire.md#aql-executor); `GET /query/aql/{qualified_query_name}` continues to carry the `ehr_id` query parameter.
- **Preconditions:** A stored or ad-hoc query executed with an EHR scope, against a backend that honours the ITS-REST OAS distinction between the two verbs (no `ehr_id` query parameter or body field on the POST operations).
- **Wire assertion:** Captured POST request carries `openehr-ehr-id: <ehr_id>` and no `ehr_id` query parameter or request-body field; the same scope on a GET request carries `ehr_id` as a query parameter and no header. A backend that only honours the header would run a query lacking it population-wide — the assertion catches an SDK regression that scopes POST via the query parameter instead.
- **Modes:** Sandbox (planned); Cassette, Live not yet scoped.
- **Status:** Deferred — REQ-055 verb-aware scoping is landed and unit-covered ([`openehr/client/query/query_test.go`](../../openehr/client/query/query_test.go)); the dedicated wire-level PROBE-078 is deferred to a follow-up cycle (tracked in [roadmap.md](../roadmap.md)).
- **Satisfies:** REQ-055.

#### PROBE-079 — `PutStoredQuery` recovers `{name, version}` from `Location`

- **Title:** A body-less `PUT /definition/query/{qualified_query_name}[/{version}]` store reply recovers the server-assigned `{name, version}` from the `Location` response header, per [wire.md § Stored AQL](wire.md#req-057).
- **Preconditions:** A store operation whose response is the canonical `200_StoredQuery_stored` shape — empty body, `Location` header carrying the qualified name and version.
- **Wire assertion:** `definition.PutStoredQuery` MUST recover `{name, version}` by parsing `Location` first; a JSON body, when present, is a lenient fallback; the caller's input `{name, version}` is the last-resort fallback. A malformed `Location` MUST NOT fail the call.
- **Modes:** Sandbox (planned); Cassette, Live not yet scoped.
- **Status:** Deferred — REQ-057 recovery order is landed and unit-covered ([`openehr/client/definition/stored_query_test.go`](../../openehr/client/definition/stored_query_test.go)); the dedicated wire-level PROBE-079 is deferred to a follow-up cycle (tracked in [roadmap.md](../roadmap.md)).
- **Satisfies:** REQ-057.

#### PROBE-084 — Built contribution body matches `Contribution_create`

- **Title:** A `Contribution_create` body assembled by the contribution builder reaches the wire with the requested operation's change-type code, a lifecycle state on every version, `preceding_version_uid` exactly where the operation requires it, and none of the server-assigned fields the pin's `UpdateVersion` does not declare.
- **Preconditions:** One payload per operation the batch commits — `COMPOSITION`, `EHR_STATUS` and `FOLDER` on the assertion below, `EHR_ACCESS` being reachable only through the same generic entry points and covered at unit level — plus a preceding-version uid per non-creation; a sandbox server accepting `POST /ehr/{ehr_id}/contribution`.
- **Wire assertion:** The captured request body — built through the builder and committed through `contribution.Commit`, so the assertion covers the real marshal path rather than a hand-marshalled copy — satisfies, per version: `commit_audit.change_type.defining_code.code_string` is the [REQ-130 § Change types](wire.md#req-130--contribution-builder) code for the operation requested (`249` / `250` / `251` / `523`) with `terminology_id.value = "openehr"`; `lifecycle_state` is present and `DV_CODED_TEXT`-shaped; `preceding_version_uid` is present for an amendment / modification / deletion and **absent** for a creation; `data` carries the payload inline with the `_type` its operation asked for; `contribution` is never emitted; and `uid` is emitted **only** where the caller supplied one, verbatim, and is absent everywhere else. The batch `audit` carries the caller's `change_type` verbatim — a value the probe deliberately sets to a code that matches no version's, since [REQ-130](wire.md#req-130--contribution-builder) forbids deriving it — and no `time_committed` appears anywhere in the body. **The vendored corpus ([`testkit/cassettes/submissions/`](../../testkit/cassettes/README.md)) is the shape witness, compared structurally, not byte-for-byte:** its records carry a top-level `_type:"CONTRIBUTION"` envelope that `Contribution_create` omits and RM payloads this SDK does not author, so a byte comparison would assert the fixture rather than the contract. The byte-level pin is a golden of the builder's own output, checked in beside the probe; the corpus arm asserts that every version field a corpus record carries is one the builder also emits, and that the fields it omits are omitted for the reason REQ-130 names.
- **Effect:** mutating — the Sandbox arm issues `POST /ehr/{ehr_id}/contribution` against the fake.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — [`testkit/probes/versioned/probe_084_built_contribution_body.go`](../../testkit/probes/versioned/probe_084_built_contribution_body.go); harness in [`probes_test.go`](../../testkit/probes/versioned/probes_test.go). One batch exercises the whole code table — a creation, an amendment, a modification and a deletion, over `COMPOSITION`, `EHR_STATUS` and `FOLDER` payloads whose `data._type` is asserted per version — under a batch audit declaring `unknown` (`253`). That code is deliberately outside the four the builder authors, so once the versions between them carry all four, no derivation rule over the versions could reproduce the batch value and the non-derivation arm cannot pass by coincidence; it also puts `WithAudit`, the escape hatch for an unauthored code, on the wire. Both halves of the server-assigned-`uid` rule are asserted there too: three versions must carry no `uid` key at all — asserting *absent* rather than *non-empty*, since a builder that synthesised one would satisfy the looser check — and the modification must carry exactly the uid its caller named. Eleven planted bodies, each violating exactly one clause and each **matched against the failure detail it must produce** so a case cannot rot into failing for an unrelated reason, pin that every arm still bites; the corpus arm is confronted with a version field the SDK cannot emit, and a missing input or empty corpus is an **error**, never a pass. The byte-level golden is [`testdata/probe_084_built_body.json`](../../testkit/probes/versioned/testdata/probe_084_built_body.json) (regenerate with `-update`). Plan: [`docs/plans/archive/2026-07-16-contribution-builder.md`](../plans/archive/2026-07-16-contribution-builder.md).
- **Satisfies:** REQ-130.

#### PROBE-091 — Path-parameter traversal is refused

- **Title:** A path parameter that is `.` or `..`, is empty, or contains `/`, `\`, or a control character is refused before any HTTP request is issued, on every path-interpolating leaf package.
- **Preconditions:** A transport client whose `*http.Client` records whether any request was issued (httptest or a tripwire RoundTripper).
- **Wire assertion:** For each leaf package that interpolates a path parameter, at least two hostile inputs — one of which MUST be the separator-smuggling id (`foo/bar` on a `Route`-set request), the only input that exercises the route-arity rule; the other drawn from a traversal id (`a/../../definition/query/evil`, `..`), a backslash-bearing id, or a control character — each fail with a non-nil error and a captured request count of zero: the call fails closed with no bytes on the wire. A well-formed id that only needs percent-encoding (a space or dots in a template id, `Blood Pressure.v1`) still issues exactly one request whose path is encoded once (REQ-095), and the service root `"/"` — the System API's only operation — passes validation and issues its request (the REQ-150 exemption's positive control). Sentinel identity (`ErrInvalidPathSegment`, `ErrInvalidConfig`) is pinned by `transport/` unit tests, not by this probe (REQ-080: no error-type assertions in probes).
- **Effect:** read-only — the hostile arms assert that NO request reaches the wire, and the positive control issues a read.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — [`testkit/probes/transport/probe_091_path_segment_validation.go`](../../testkit/probes/transport/probe_091_path_segment_validation.go); harness in [`probes_test.go`](../../testkit/probes/transport/probes_test.go). Plan: [`docs/plans/archive/2026-08-18-path-segment-validation.md`](../plans/archive/2026-08-18-path-segment-validation.md).
- **Satisfies:** REQ-150.

#### PROBE-092 — Contribution read matches `contribution_get`

- **Title:** `GET /ehr/{ehr_id}/contribution/{contribution_uid}` is issued by the contribution read leaf and the 200 body decodes as the persisted `CONTRIBUTION`.
- **Preconditions:** A known EHR id and contribution uid; a sandbox server returning a canonical-JSON contribution body (or 404).
- **Wire assertion:** The captured request method and path match the ITS-REST template; a 200 decodes to the contribution type (version metadata is **not** asserted — the pin defines only `Content-Type` on `200_CONTRIBUTION`); empty ids issue no request; a 404 fails with a non-nil error. Sentinel identity (`ErrNotFound`, `ErrInvalidConfig`) is pinned by `contribution` unit tests, not by this probe (REQ-080).
- **Effect:** read-only (`GET`, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — [`testkit/probes/versioned/probe_092_contribution_get.go`](../../testkit/probes/versioned/probe_092_contribution_get.go); harness in [`probes_test.go`](../../testkit/probes/versioned/probes_test.go). Plan: [`docs/plans/archive/2026-08-18-contribution-get.md`](../plans/archive/2026-08-18-contribution-get.md).
- **Satisfies:** REQ-142.

#### PROBE-093 — Template list filters reach the wire

- **Title:** `ListTemplates` emits the ITS-REST list query parameters `template_id`, `concept`, `version`, `offset`, and `fetch` when the corresponding options are set, and omits them when unset.
- **Preconditions:** A sandbox Definition server on `GET /definition/template/adl1.4` (the only `TemplateFormat` v1 registers; the pin's ADL 2 list operation shares the same five parameter components).
- **Wire assertion:** Each option appears as the named query key; an explicit `WithOffset(0)` / `WithFetch(0)` is present on the wire; a negative offset or fetch fails with a non-nil error and no request (sentinel identity is pinned by `definition` unit tests, not by this probe — REQ-080); no options yields an empty query. The response still decodes as the existing template-metadata slice.
- **Effect:** read-only (`GET` list, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — [`testkit/probes/definition/probe_093_template_list_filters.go`](../../testkit/probes/definition/probe_093_template_list_filters.go); harness in [`probes_test.go`](../../testkit/probes/definition/probes_test.go). Plan: [`docs/plans/archive/2026-08-18-template-list-filters.md`](../plans/archive/2026-08-18-template-list-filters.md).
- **Satisfies:** REQ-143.

### RM model introspection

#### PROBE-094 — RM meta-model introspection equals the pinned BMM

- **Title:** The compiled-in RM class graph — class universe, abstractness, immediate parents, transitive ancestors, concrete-descendant expansion, and per-attribute declaring class — equals an independent reduction of the pinned RM BMM, and every answer it gives is closed over the universe it reports.
- **Preconditions:** The pinned primary RM schema and its includes under [`resources/bmm/`](../../resources/bmm) (REQ-041), reduced in-test through the `openehr/bmm` loader (REQ-045) — deliberately **not** through the generator's own reduction, which would make the comparison tautological and would pass on a generator whose walk is itself wrong.
- **Wire assertion:** In-repo property, not backend-facing.
  (a) **Universe** — the compiled-in class set equals the class universe [REQ-048 § The class universe](bmm-conformance.md#the-class-universe) defines: the classes the RM generation target emits from the primary RM schema **and the BASE classes it includes**, minus enumerations and the foundation typing classes that target skips — with the two exclusions that section places **outside** the rule (the `org.openehr.rm.ehr_extract` package, excluded upstream of emission, and the `primitive_types` entries, out even where the target emits a Go type for them). A class present on one side only is a failure **in either direction**: a missing class is an unanswerable question, an extra one is a class name the pinned schemas do not define.
  (b) **Per class** — abstractness equals the BMM `is_abstract` flag *verbatim*, including the six classes that carry no flag (`MEASUREMENT_SERVICE`, `TERMINOLOGY_SERVICE`, `CODE_SET_ACCESS`, `TERMINOLOGY_ACCESS`, `OPENEHR_CODE_SET_IDENTIFIERS`, `OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS`) and are therefore reported non-abstract — REQ-047 forbids substituting a locally-derived verdict, and this arm MUST fail if one is substituted, not merely if the flag is misread. Immediate parents equal the BMM `ancestors` list filtered to the universe, in declaration order; transitive ancestors equal the closure of that filtered edge set; concrete descendants equal the non-abstract members of the inverse closure, plus the class itself when it is not abstract.
  (c) **Closure of the class graph** — every class name the class-graph answers return (parents, ancestors, concrete descendants) is itself in the universe: none of the excluded `primitive_types` entries (`Any`, `Ordered`, `Interval`, the `Iso8601_*` types), no enumeration (`PROPORTION_KIND`), and no `ehr_extract` class leaks in through an ancestor or descendant edge. The **declaration site is out of scope here by construction** — REQ-048 exempts it, and arm (d) checks it against the *unfiltered* BMM chain instead. The converse arm is the load-bearing one: the filter MUST NOT cost an RM edge — for every class whose BMM ancestors mix an RM class with a filtered one (`DV_ORDERED`, `DV_INTERVAL`, `DV_DATE`, `DV_PROPORTION`, …), the RM ancestor MUST survive, since an edge lost here silently shrinks every descendant expansion above it. Every dropped edge MUST additionally be accountable to one of the universe rule's declared exclusions, named per edge; a dropped ancestor the probe cannot account for is a failure, because that is what an RM class falling out of the universe looks like. The classes left with no in-universe ancestor MUST be pinned in both directions — an unpinned new root fails, and a pinned one that regains a parent fails too.
  (d) **Declaration site** — for every class and every attribute the flattened tables report on it: the declaring class MUST be that class or one of its **unfiltered** BMM ancestors (the site may lie outside the universe — REQ-048 exempts it, and on the pinned RM `Interval` is the only such site, pinned by name); its own BMM declaration MUST carry the attribute; and where the site is *in* the universe, the attribute's reported type / required / container triple MUST equal the one that class reports. The container flag is checked against the raw BMM property here, because type and required would mean re-implementing REQ-043's mapping inside the probe; the full triple-versus-site comparison is asserted beside it in the behaviour suite. **Completeness too**: the shipped attribute-name set per class MUST equal the effective property set of the same class re-derived from the BMM, minus the probe's **named exemption pins** (`unshippedProperties`), and the list is held two-sidedly: an unpinned drop fails, and a stale pin whose drop has since been fixed fails too. The pin rule — each pin cites an open research strand, and the pinned drop is not folded in ahead of that strand's resolution — is normative in [REQ-048 § The attribute tables are complete against the BMM](bmm-conformance.md#the-attribute-tables-are-complete-against-the-bmm), which also names the list's one current entry (`Iso8601_timezone.value`, [STRAND-13](research-strands.md#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped)); this arm is its oracle. An attribute the generator silently declines to translate therefore fails rather than going unnoticed. How the site is derived is an implementation decision, not something a probe can assert.
  (e) **Negative space** — a name outside the universe MUST be distinguishable from every in-universe answer on every question; an attribute a class does not carry MUST report not-found rather than a guessed class; and a root class and a dead-end abstract class MUST each report as known with an empty answer, never as unknown. The pinned RM cannot supply every shape — a dead-end abstract class with a *named* identity, a class whose parent is absent from the data set — so those are exercised through REQ-048's synthetic-model seam in the behaviour suite beside this probe, not in the probe itself.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — six suites in [`openehr/rm/rminfo/probe_094_test.go`](../../openehr/rm/rminfo/probe_094_test.go) — one per arm, two for arm (d): `TestProbe094UniverseEqualsThePinnedBMM` (a, both directions), `TestProbe094PerClassFactsEqualThePinnedBMM` (b, over all 139 classes), `TestProbe094FilterCostsNoRMEdge` (c, accounting for every dropped edge — all nine are `primitive_types` entries or the one enumeration, none an RM class — with the four filter-roots pinned two-sidedly and a fatal guard if no ancestor was filtered at all), `TestProbe094DeclarationSitesAreRealBMMDeclarations` (d, 728 sites), `TestProbe094AttributeSetsAreComplete` (d, the attribute-set completeness with its STRAND-13 exemption pin) — with the full type/required/container-versus-site comparison beside them in `TestDeclaredOnAgreesWithTheFlattenedTables` — and `TestProbe094NegativeSpace` (e, with fatal guards on the vacuous cases). The reduction reads the pinned schemas through `openehr/bmm` and restates the generator's exclusion list as a literal, so a class silently added to or dropped from either list fails the probe. Behaviour-level coverage of the same surface — including the synthetic-model seam the pinned RM cannot supply — is in [`openehr/rm/rminfo/hierarchy_test.go`](../../openehr/rm/rminfo/hierarchy_test.go).
- **Satisfies:** REQ-048.

#### PROBE-098 — Absence reasons account for the universe's negative space

- **Title:** For every name the pinned schemas declare and the class universe omits, the compiled-in absence table reports the reason an independent reduction of the pinned BMM derives — in both directions, under REQ-049's fixed precedence — while universe members report *none* and names no schema declares report *undeclared* with no stored entry backing them.
- **Preconditions:** As PROBE-094: the pinned primary RM schema and its includes under [`resources/bmm/`](../../resources/bmm) (REQ-041), reduced in-test through the `openehr/bmm` loader (REQ-045) — never through the generator's own reduction, which would make the comparison tautological — with the generator's exclusion lists restated as literals so a silent change to either side fails the probe.
- **Wire assertion:** In-repo property, not backend-facing.
  (a) **Accounting, both directions** — every declared name the universe omits gets the reduction's reason from the shipped surface, and every stored table entry is a declared-but-omitted name carrying that same reason; an entry on one side only is a failure either way.
  (b) **No overlap, nothing computed stored** — the stored table contains no universe member and no *undeclared* or *none* entry; every `KnownRMTypes()` member reports *none*.
  (c) **Undeclared** — a name no pinned schema declares reports *undeclared*.
  (d) **Precedence** — at least one name matching two rules is pinned by name and MUST report per the REQ-049 order (a primitive-mapped name the named-class exclusion list restates as belt-and-braces reports *primitive*, not *excluded class*).
  (e) **Consistency with the negative space** — every out-of-universe name this probe exercises reports not-known on every `Lookup`/`Hierarchy` question (PROBE-094 arm (e) holds the converse for universe members).
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — five suites in [`openehr/rm/rminfo/probe_098_test.go`](../../openehr/rm/rminfo/probe_098_test.go), one per arm: `TestProbe098AbsenceAccountsForTheNegativeSpace` (a, walking every declared name so both directions are answered, with the census pinned per reason — 29 primitive, 3 enumeration, 12 excluded class, 30 excluded package — so a rule that quietly stops matching fails on the counts rather than leaving its names unasked-about), `TestProbe098NoOverlapWithTheUniverse` (b, membership versus absence in both directions, then the stored key set held to a set identity against the reduction's declared-minus-universe set, read through the test-only accessor in [`export_test.go`](../../openehr/rm/rminfo/export_test.go) because the shipped accessor answers membership first and would mask an entry for a universe member, and no stored value *none* or *undeclared*), `TestProbe098UndeclaredIsComputedNotStored` (c, names the pinned schemas mention without declaring, plus synthetic near-misses of a real table key and the empty string), `TestProbe098PrecedenceIsFixed` (d, the two-rule witness pinned by name on `Ordered` and both adjacent ranks pinned as name sets rather than counted, with the enumeration rank's absent witness asserted rather than assumed — that rank is covered by `internal/bmmgen`'s `TestRMInfoAbsenceReasonsUnderTheFixedPrecedence` on a synthetic enumeration), and `TestProbe098AbsentNamesAreUnknownEverywhere` (e, all 74 declared-but-omitted names plus both undeclared witness sets, over every `Lookup` / `Hierarchy` question). The reduction reads the pinned schemas through `openehr/bmm` and restates the generator's exclusion lists as literals — `internal/bmmgen` is never imported — so a name silently added to or dropped from either side fails the probe. Behaviour-level coverage of the same surface — the zero value, the synthetic seam, the `String()` form — is in [`openehr/rm/rminfo/absence_test.go`](../../openehr/rm/rminfo/absence_test.go), and the generator half in [`internal/bmmgen/render_rminfo_absence_test.go`](../../internal/bmmgen/render_rminfo_absence_test.go).
- **Satisfies:** REQ-049.

### Observability

#### PROBE-050 — OTel span carries openEHR attributes

- **Title:** Every outgoing request opens an OTel span with `openehr.spec_version`, `openehr.resource_type`, and a sanitised URL.
- **Preconditions:** OTel `TracerProvider` injected in context.
- **Wire assertion:** Captured span has the expected attribute set; URL does not contain the bearer token.
- **Modes:** Sandbox.
- **Status:** Draft.
- **Satisfies:** REQ-090

#### PROBE-051 — No-OTel is a silent no-op

- **Title:** Absence of a `TracerProvider` in context produces no error, no warning, and no allocated spans.
- **Preconditions:** Default context.
- **Wire assertion:** Request succeeds; no global state mutation.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — covered by [`transport/client_test.go`](../../transport/client_test.go).
- **Satisfies:** REQ-090

## Adding probes

A new probe **MUST**:

- Be assigned the next available `PROBE-NNN`. The original rule was *next in the probe's topic range, with a gap of 10 between topics*; that rule was exhausted once the catalog crossed 080 and allocation has been **sequential across topics** ever since — 086 and 089 are formats probes, 087/088/090 AQL, 091–093 REST binding, 094 RM model introspection. A new topic therefore takes the next free number and adds its own catalog section rather than opening a decade. Renumbering remains prohibited either way.
- Have a definition in this catalog *before* any implementation lands.
- Be runnable in at least Sandbox mode; Cassette and Live modes follow when fixtures are recorded.
- Carry a `Status:` transition (Draft → Implemented → Ratified, or Deprecated before removal) in this spec when its state changes; transitions go in the CHANGELOG.

## Removing probes

A probe **MUST NOT** be silently removed. The lifecycle is:

1. Mark `Status: Deprecated` with a reason and a removal target version.
2. Keep the probe runnable for at least one minor version.
3. Remove in the next major version.

Renumbering is prohibited — once a `PROBE-NNN` is published, it stays.

## Coverage matrix

| Topic | Probes | Lives in (test code) |
|---|---|---|
| Auth + discovery | PROBE-001 … 009 | **all implemented (Sandbox)** — [`testkit/probes/auth/`](../../testkit/probes/auth/). PROBE-001/002/003 drive the real `discovery.Resolver`; PROBE-004 (PKCE + G-7 parity) / PROBE-005 (scope) drive a full `auth/smart` authorization-code launch; PROBE-006 (JWKS rotation), PROBE-007 (transport + proactive refresh halves), PROBE-008 (principal claims), PROBE-009 (caller attribution). Launch-mode coverage (standalone / embedded / backend, REQ-068) lives alongside in [`launch_modes.go`](../../testkit/probes/auth/launch_modes.go). |
| Versioned writes | PROBE-010 … 013 | [`testkit/probes/versioned/`](../../testkit/probes/versioned) — all implemented (Sandbox) |
| AQL | PROBE-020 … 021, PROBE-028, PROBE-080, PROBE-082, PROBE-087, PROBE-088, PROBE-090, PROBE-095, PROBE-096, PROBE-097 | PROBE-097 (REQ-160/161/162 semantic and portability lint corpus) implemented (inline) — [`testkit/probes/aql/probe_097_semantic_lint.go`](../../testkit/probes/aql/probe_097_semantic_lint.go); PROBE-020 implemented (Sandbox) — [`testkit/probes/aql/`](../../testkit/probes/aql/); PROBE-021 structural guarantee + `aql.ErrPathResolution` / `aql.ErrEngineCapability` mapping tested under [`openehr/client/query/`](../../openehr/client/query/), Cassette/Live pending; PROBE-028 (REQ-109 AQL lint stability) implemented (Sandbox) — [`testkit/probes/aql/probe_028_aql_lint.go`](../../testkit/probes/aql/probe_028_aql_lint.go); PROBE-080 (REQ-113 parse/emit round-trip) + PROBE-082 (REQ-113 structured predicate + WHERE path) implemented inline — [`openehr/aql/parse/roundtrip_test.go`](../../openehr/aql/parse/roundtrip_test.go), [`openehr/aql/parse/structured_test.go`](../../openehr/aql/parse/structured_test.go); PROBE-087 (REQ-117 + REQ-118 structured-AST catalogue completeness — the REQ-118 `SELECT TOP` carrier and literal source text extend it) implemented inline — [`openehr/aql/parse/query_test.go`](../../openehr/aql/parse/query_test.go) + the same round-trip suite; PROBE-088 (REQ-117 + REQ-118 builder containment algebra, in-text paging, and the deprecated `SELECT TOP` clause) implemented (Sandbox) — [`testkit/probes/aql/probe_088_builder_containment_paging.go`](../../testkit/probes/aql/probe_088_builder_containment_paging.go) with goldens in [`openehr/aql/testdata/wire/`](../../openehr/aql/testdata/wire/); PROBE-090 (REQ-119 emission round-trip closure — every value kind survives emit -> parse -> equal value in SELECT as well as WHERE, and the grammar-derived write-side guards are confronted with the vendored lexer rather than a hand-written expectation) implemented inline — [`openehr/aql/parse/literal_roundtrip_test.go`](../../openehr/aql/parse/literal_roundtrip_test.go) + [`openehr/aql/parse/emit_parity_test.go`](../../openehr/aql/parse/emit_parity_test.go) + [`openehr/aql/value_test.go`](../../openehr/aql/value_test.go), + [`openehr/aql/parse/identifier_parity_test.go`](../../openehr/aql/parse/identifier_parity_test.go) (the single-token identifier and archetype-id guards, the drop-direction refusals, and the generated HRID sweep), held structurally by [`openehr/aql/parse/dispatch_tripwire_test.go`](../../openehr/aql/parse/dispatch_tripwire_test.go) + [`openehr/aql/parse/value_position_parity_test.go`](../../openehr/aql/parse/value_position_parity_test.go), and the class / VERSION bracket positions by [`openehr/aql/predicate_test.go`](../../openehr/aql/predicate_test.go) + [`openehr/aql/parse/predicate_guard_test.go`](../../openehr/aql/parse/predicate_guard_test.go) + [`openehr/aql/parse/predicate_parity_test.go`](../../openehr/aql/parse/predicate_parity_test.go) + [`openehr/aql/parse/predicate_confrontation_test.go`](../../openehr/aql/parse/predicate_confrontation_test.go), with the after-emission verification held by [`openehr/aql/parse/emit_verify_test.go`](../../openehr/aql/parse/emit_verify_test.go) + [`openehr/aql/parse/emit_verify_internal_test.go`](../../openehr/aql/parse/emit_verify_internal_test.go), and the read-side consequence for template resolution pinned in [`openehr/aql/lint/lint_test.go`](../../openehr/aql/lint/lint_test.go). PROBE-095 (predicate structuring, REQ-113) implemented inline — [`openehr/aql/parse/predicate_structure_test.go`](../../openehr/aql/parse/predicate_structure_test.go), over a corpus generated from the vendored grammar down to `pathPredicateOperand`; PROBE-096 (value-free structured diagnostics, REQ-113/109) implemented inline — [`openehr/aql/parse/dropped_test.go`](../../openehr/aql/parse/dropped_test.go) + [`openehr/aql/lint/span_test.go`](../../openehr/aql/lint/span_test.go). |
| Clinical modeling | PROBE-022, PROBE-023, PROBE-024, PROBE-025, PROBE-026, PROBE-027, PROBE-074, PROBE-075, PROBE-077, PROBE-081 | [`testkit/probes/template/`](../../testkit/probes/template/) — PROBE-022 / PROBE-024 implemented (Sandbox); PROBE-023 implemented (Sandbox) under [`testkit/probes/composition/`](../../testkit/probes/composition/); PROBE-025 / PROBE-026 / PROBE-074 under [`testkit/probes/validation/`](../../testkit/probes/validation/); PROBE-027 implemented (Sandbox) under [`testkit/probes/instance/`](../../testkit/probes/instance/) — REQ-107 Phases 1–3 landed; PROBE-074 (REQ-110) extends validation to demographic + EHR-IM roots; PROBE-075 (REQ-106/REQ-116 WebTemplate structural parity) implemented inline — exact node + input-suffix/type parity against three vendored references (`constrain_test` 104/104, `Corona_Anamnese` 230/230, `GECCO_Diagnose` 34/34; residuals count-pinned) in [`openehr/template/webtemplate/`](../../openehr/template/webtemplate/); PROBE-077 (REQ-112 RM-floor invariant matrix) deferred — unit-cassette matrix carries first-cycle coverage in [`openehr/validation/rmfloor_test.go`](../../openehr/validation/rmfloor_test.go); PROBE-081 (REQ-112) pins EHR_STATUS value-typed `subject` presence, implemented inline — [`openehr/validation/rmfloor_bytes_test.go`](../../openehr/validation/rmfloor_bytes_test.go). |
| Canonical JSON / formats | PROBE-030 … 034, PROBE-038, PROBE-076, PROBE-086, PROBE-089 | [`testkit/probes/serialize/`](../../testkit/probes/serialize) — 030–031, 033–034, 038 implemented; 032 not yet. PROBE-038 (REQ-052/040 polymorphic decode coverage) at [`testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go`](../../testkit/probes/serialize/probe_038_canjson_rm_polymorphic_decode.go); PROBE-076 (REQ-053 FLAT/STRUCTURED round-trip) at [`probe_076_simplified_round_trip.go`](../../testkit/probes/serialize/probe_076_simplified_round_trip.go); PROBE-086 (REQ-080 upstream FLAT parity) implemented (Sandbox) — harness at [`testkit/conformance/webtemplate/`](../../testkit/conformance/webtemplate/), wrapper at [`probe_086_upstream_flat_parity.go`](../../testkit/probes/serialize/probe_086_upstream_flat_parity.go); 34/34 fixtures exact on the modelled subset, with the unmodelled surface counted per fixture ([`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md)). PROBE-089 (REQ-140 underscore-attribute round-trip) implemented (Sandbox) at [`probe_089_underscore_round_trip.go`](../../testkit/probes/serialize/probe_089_underscore_round_trip.go) — 14 per-family fixtures over the four legs (byte-exact decode→re-encode, canonical-transit encode, the deliberate refusals, STRUCTURED vocabulary), the census movement it requires being 19.7% → 80.4% of the corpus. |
| RM model introspection | PROBE-094, PROBE-098 | PROBE-098 (REQ-049 absence reasons vs the same independent reduction) implemented inline — [`openehr/rm/rminfo/probe_098_test.go`](../../openehr/rm/rminfo/probe_098_test.go), five suites (one per arm), beside the behaviour suite [`absence_test.go`](../../openehr/rm/rminfo/absence_test.go) and the generator's [`render_rminfo_absence_test.go`](../../internal/bmmgen/render_rminfo_absence_test.go) (archived plan: [`2026-08-26-rminfo-absence-reason.md`](../plans/archive/2026-08-26-rminfo-absence-reason.md)); PROBE-094 (REQ-048 compiled-in RM class graph vs an independent reduction of the pinned BMM) implemented inline — [`openehr/rm/rminfo/probe_094_test.go`](../../openehr/rm/rminfo/probe_094_test.go), six suites (one per arm, two for arm (d)), beside the behaviour suite [`hierarchy_test.go`](../../openehr/rm/rminfo/hierarchy_test.go). |
| Service discovery | PROBE-040 … 041 | [`testkit/probes/discovery/`](../../testkit/probes/discovery) — both implemented (Sandbox) |
| Observability | PROBE-050 … 051 | partial — PROBE-051 in [`transport/client_test.go`](../../transport/client_test.go); *planned* — `testkit/probes/observability/` |
| REST binding | PROBE-060 … 068, PROBE-071, PROBE-072, PROBE-073, PROBE-078, PROBE-079, PROBE-084, PROBE-091, PROBE-092, PROBE-093 | partial — PROBE-061/071 (`Prefer: return=representation`, REQ-094) implemented (Sandbox) at [`testkit/probes/versioned/probe_071_composition_write_response_shape.go`](../../testkit/probes/versioned/probe_071_composition_write_response_shape.go) + leaf unit tests; PROBE-072 (REQ-050/095 contribution submission shape) implemented (Sandbox) at [`testkit/probes/versioned/probe_072_contribution_submission_shape.go`](../../testkit/probes/versioned/probe_072_contribution_submission_shape.go); PROBE-073 (Demographic PARTY polymorphic round-trip) implemented (Sandbox) at [`testkit/probes/demographic/probe_073_demographic_round_trip.go`](../../testkit/probes/demographic/probe_073_demographic_round_trip.go); REQ-094 `identifier` / empty-body follow-ups landed (leaf unit tests, archived [`2026-05-25-req094-prefer-followups.md`](../plans/archive/2026-05-25-req094-prefer-followups.md)); PROBE-065 (`minimal`→GET round-trip), PROBE-078 (REQ-055 `openehr-ehr-id` POST scoping), and PROBE-079 (REQ-057 `PutStoredQuery` `Location` recovery) still deferred. PROBE-092 (REQ-142 contribution read) is implemented (Sandbox) at [`testkit/probes/versioned/probe_092_contribution_get.go`](../../testkit/probes/versioned/probe_092_contribution_get.go); PROBE-091 (REQ-150 path-segment validation) is implemented (Sandbox) at [`testkit/probes/transport/probe_091_path_segment_validation.go`](../../testkit/probes/transport/probe_091_path_segment_validation.go); PROBE-093 (REQ-143 template list filters) is implemented (Sandbox) at [`testkit/probes/definition/probe_093_template_list_filters.go`](../../testkit/probes/definition/probe_093_template_list_filters.go); PROBE-084 (REQ-130 built contribution body) is implemented (Sandbox) at [`testkit/probes/versioned/probe_084_built_contribution_body.go`](../../testkit/probes/versioned/probe_084_built_contribution_body.go), with a byte golden beside it and the vendored submission corpus as its shape witness. |
