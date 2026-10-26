---
kind: specification
---

# Conformance probes

**Status:** Draft

The conformance probe catalogue for this SDK, and the requirements that hold the probe harness to the people who use it: the sandbox backend, recording safety and the live-write opt-in (REQ-082), and the `cadasto/admin` health probes (REQ-083).

A **conformance probe** is an executable assertion that the SDK exercises against either:

- the sandbox transport (`sandbox/`),
- a replayed recording (`testkit/recordings/`), or
- a live reference deployment,

to verify wire-level conformance to openEHR REST and SMART-on-openEHR, or an in-repo property over vendored inputs or the SDK's own output that reaches no server.

Each probe has a stable `PROBE-NNN` ID, one catalogue entry here, and a pass/fail outcome. How probes are written, run and kept is [development-process.md § Conformance probes](../development-process.md#conformance-probes).

## Conformance scope

### REQ-080 — openEHR wire conformance

**Retired.** Its test-writing rules moved to [development-process.md § Writing a probe](../development-process.md#writing-a-probe), and the round-trip comparison it restated is [wire.md § REQ-052](wire.md#req-052)'s. The identifier is never reused.

### REQ-081 — Wire-level parity (retired)

**Retired.** The SDK checks no parity with another SDK: its wire output is checked against the openEHR specification by the [probe catalog](#probe-catalog), and its Go surface follows [idiom.md § Surface shape](idiom.md#surface-shape). The identifier is never reused.

### REQ-082 — Runnability

A backend-facing probe runs in up to three modes: **Sandbox**, against the in-memory `sandbox/` transport; **Cassette**, against a replayed recording; and **Live**, against a reference openEHR deployment. An **in-repo** probe asserts a property over vendored inputs or over the SDK's own output and reaches no server in any mode. Each catalogue entry's **Modes** line says which applies: 35 of the 80 catalog entries are in-repo by construction. Today 12 entries declare all three modes; a mode missing from a backend-facing entry is open work in this requirement, not a defect in the probe ([roadmap.md](../roadmap.md)).

How the runner selects a mode, what a probe reports and each mode's working rules are in [development-process.md § Conformance probes](../development-process.md#conformance-probes). This section holds what an SDK consumer, or the operator of a deployment a probe writes to, relies on.

#### Sandbox mode

The `sandbox/` transport is a published building block, not test-only scaffolding.

- It **MUST** serve every backend-facing probe in the catalogue without a network listener or credentials.
- It **MUST** be reachable by an SDK consumer testing their own application, injected as the transport of the HTTP client the consumer hands the SDK.
- Sandbox state **MUST** be per-run and isolated: two probes running against one sandbox instance observe each other's writes only when the probe definition says they share an EHR.

Its non-test source imports neither `auth/` nor `transport/`, and neither of the listener packages `net` and `net/http/httptest`.

#### Cassette mode

A recording is a captured HTTP exchange: method, URL, request and response headers, status and both bodies. Its encoding is HTTP Archive 1.2 (`.har`), settled by [ADR 0020](../adr/0020-cassette-recording-har.md); provenance and the redaction record sit on `log` as a `_req082` object, since HAR has no native slot for them.

- Credentials (`Authorization`, cookies, tokens, client secrets, JWKS private material) and patient-identifying data **MUST NOT** reach disk. The recorder redacts them at capture time, never at review time, and the recording records that redaction ran, so an unredacted capture is detectable rather than merely unlikely.
- A recording **MUST** carry its provenance: the deployment it came from, the date, and the SDK commit that captured it. A recording whose provenance cannot be stated **MUST** be refused rather than replayed.
- The replayer **MUST** fail an unmatched request, and **MUST NOT** pass a request through to a network or synthesise a plausible response: a recording with a gap is recaptured.

#### Live mode

The runner **MUST** refuse to execute a mutating probe in Live mode without an explicit per-invocation opt-in, and a probe whose catalogue entry declares no **Effect** counts as mutating: writing to a deployment is something the operator asked for, never a default a mode flag turned on or a label nobody wrote.

#### The CDR cross-check tool

The CDR cross-check tool (`make cdr-crosscheck`) measures live CDRs and the SDK against each other; it records numbers and asserts nothing, so it is a tool and not a probe ([development-process.md § The CDR cross-check tool](../development-process.md#the-cdr-cross-check-tool)). It **MUST NOT** write to a CDR without an explicit per-invocation opt-in, `OPENEHR_LIVE_ALLOW_MUTATING`.

### REQ-083 — Cadasto platform API conformance

The `cadasto/` extras have no openEHR specification: their wire contract is the Cadasto platform API, its OpenAPI document where one exists, and otherwise fixture documents captured from a reference Cadasto deployment. Their probes check the SDK's wire shape against that contract, not against another SDK.

#### Health probes (`cadasto/admin`)

`cadasto/admin` exposes deployment liveness/readiness probes (`Live`, `Ready`, `ReadyBody`) with a wire contract independent of the (planned) Cadasto platform API surface. They are distinct from the ITS-REST Admin client, `openehr/client/admin`.

- **Default paths** `DefaultLivePath = "/health/live"`, `DefaultReadyPath = "/health/ready"`, each overridable per call via `WithLivePath` / `WithReadyPath` (e.g. `/healthz`).
- **URL derivation.** The probe URL derives from the **origin (scheme + host) of the openEHR REST service entry** — the openEHR REST API path prefix is **NOT** inherited.
- **Public / no auth.** Health endpoints are public: the SDK **MUST NOT** send an `Authorization` header on a probe.
- **Status mapping** (`errors.Is`-compatible): `2xx → nil`; `401 → transport.ErrUnauthorized`; `403 → transport.ErrForbidden`; `404 → transport.ErrNotFound`; `5xx → transport.ErrServerError`. Other non-2xx codes (400, 405, 408, 429, …) surface as a plain formatted error with no sentinel.
- Probes borrow `transport`'s sentinel taxonomy (REQ-093) but **bypass** `transport.Client.Do` — no openEHR error-envelope decoding, no OTel spans, no retries. Use `openehr/client/admin` when those concerns matter.
- **Bounded drain.** A probe drains the 2xx body `Live` and `Ready` discard, and any body that is not 2xx, so the connection can be reused. It **MUST** read at most a fixed number of bytes of that body, for at most a fixed time, before closing it: a body that never ends, streamed fast or trickled, **MUST NOT** hold a probe until its context ends. A `ReadyBody` 2xx body is read under *The readiness body* below instead.

**The readiness body.** A readiness endpoint often answers with a small document that names the service and its version, and `Ready` discards it. `ReadyBody` sends the request `Ready` sends, takes the same options, and has `Ready`'s path guard and status mapping; on a 2xx it **MUST** return the response body instead of `nil`.

- **Bounded read.** `ReadyBody` **MUST** cap the bytes it reads from the body. The default limit is `DefaultMaxReadyBody` (1 MiB). `WithMaxReadyBody(n)` overrides it with the values [§ REQ-093](transport.md#req-093--openehr-error-envelope-mapping) *Bounded response reads* gives `transport.WithMaxResponseBody`: `0` means the default, a positive value sets the limit, a negative value disables it. `Ready` reads no body it returns, so the option does not change it.
- **Read failures.** A 2xx body longer than the limit **MUST** fail with an error that `errors.Is` matches to `transport.ErrResponseTooLarge` and whose text names the limit. Any other 2xx body that cannot be read **MUST** fail with an error that unwraps to the read failure, such as `io.ErrUnexpectedEOF` for a connection cut mid-body. Either failure **MUST** return no bytes: a partial body is not the document.
- **Value-free text.** The text of these read-failure errors **MUST NOT** carry a body byte, a header value or the read failure's own text, which can quote bytes the server sent, as [§ REQ-151](transport.md#req-151--typed-2xx-decode-failure) *`Error()` stays value-free* holds for `transport.BodyReadError`. The limit is the caller's own setting, not a payload value.
- **Non-2xx.** A status that is not 2xx returns no bytes, beside the error the status mapping gives `Ready`.

**Acceptance.**

- A 2xx readiness response with the body `{"status":"UP","version":"1.2.3"}` makes `ReadyBody` return exactly those bytes; `ReadyBody` with `WithReadyPath("/ehrbase/rest/status")` sends its `GET` to that path at the origin, with no `Authorization` header.
- A 2xx body of exactly `n` bytes under `WithMaxReadyBody(n)` is returned whole; one byte more returns no bytes and an error matching `transport.ErrResponseTooLarge` whose text names `n`. Without the option, a body over `DefaultMaxReadyBody` fails the same way.
- A 2xx body cut mid-read returns no bytes and an error matching `io.ErrUnexpectedEOF`. None of the bytes read before the cut, and no value the server sends as a malformed chunked trailer, appears in the error text.
- A `503` returns no bytes and an error matching `transport.ErrServerError`; an empty or relative `WithReadyPath` returns `ErrInvalidPath` without sending a request; a nil `*transport.Client` returns an error matching `transport.ErrInvalidConfig`.
- A 2xx body that never ends makes `Live` and `Ready` return `nil`, and a `503` body that never ends makes `ReadyBody` return no bytes and an error matching `transport.ErrServerError`, each after reading a bounded number of body bytes. When that body (a 2xx for `Live` and `Ready`, a `503` for `ReadyBody`) sends a byte and then stalls, each call returns the same result before its context ends.

### Vendored fixtures (`testkit/corpus/`)

The probes read their fixture documents (bodies, not recorded exchanges) from `testkit/corpus/`; the layout, the ingests and the integrity checks are [development-process.md § Vendored fixtures](../development-process.md#vendored-fixtures). The cross-format sets come from Apache-2.0 upstreams only, as [LICENSING.md](../../LICENSING.md) records for each vendored corpus ([licensing.md](../licensing.md)).

## Probe catalog

Each entry says what its probe checks and which requirement it satisfies; the requirement's own section holds the rule. An entry has:

- **ID** — stable, never renumbered.
- **Title** — one-line description.
- **Preconditions** — the state the system is in.
- **Wire assertion** — what the probe checks at the byte, status or value level.
- **Modes** — Sandbox / Cassette / Live for a backend-facing probe, or In-repo for a probe that reaches no server in any mode ([§ REQ-082](#req-082--runnability)). For a backend-facing probe this line is the record of which modes it supports today.
- **Effect** — read-only or mutating ([§ REQ-082 *Live mode*](#live-mode)). Absent means *treated as* mutating; few entries carry one yet.
- **Status** — Draft (in this spec), Implemented (in code), Ratified (passes against a reference openEHR deployment), Deferred (the requirement it covers is landed and unit-covered, but this dedicated probe is not written), Deprecated (scheduled removal; possibly unrunnable when its implementation is already gone before 1.0). The parenthesis after Implemented names what runs the probe's committed test: (Sandbox) or (Live snapshot) for a backend-facing probe, (inline) for an in-repo probe that a Go test runs directly rather than the runner. Which modes a probe supports is for the Modes line alone to say.
- **Satisfies** — the REQ-IDs this probe exercises (inverse of the [REQ registry](REQ.md)).

### Terms

**Wire-equivalent.** Two JSON documents are *wire-equivalent* when parsing each into a generic JSON value (object as a map, array as a slice, plus string, number, boolean and null) yields equal trees. The comparison is by member name, not by position, so it is insensitive to member order and to the encoder's choice of string escaping. Array element order **is** compared: it is significant in JSON and in the RM. Numbers are compared as their literal text, so an integer past 2^53 compares exactly rather than through `float64`. Wire-equivalence is the only check on encoded JSON a probe in this catalog makes: none checks byte equality of encoded JSON, for the reason [§ REQ-052](wire.md#req-052) gives.

### Authentication and discovery

#### PROBE-001 — Discovery declares `code+pkce`

- **Title:** SMART configuration document declares `code` response type and `S256` PKCE method.
- **Preconditions:** A SMART-on-openEHR deployment is reachable.
- **Wire assertion:** GET `<base URL>/.well-known/smart-configuration` (or equivalent) returns 200 with a JSON body containing `"response_types_supported"` including `"code"` and `"code_challenge_methods_supported"` including `"S256"`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — resolves the canonical SMART configuration fixture through `discovery.Resolver` and checks `code` and `S256` on the resolved `AuthEndpoints`.
- **Satisfies:** REQ-061.

#### PROBE-002 — Discovery advertises `org.openehr.rest`

- **Title:** Service catalog includes the openEHR REST service with a parseable base URL and a declared spec version.
- **Preconditions:** SMART discovery resolved.
- **Wire assertion:** The discovery document's service catalog contains an entry with id `"org.openehr.rest"`, a parseable `base_url`, and a `spec_version` field.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — checks that `catalog.OpenEHRRest()` resolves an entry with an absolute `base_url` and a declared `spec_version`.
- **Satisfies:** REQ-070, REQ-072.

#### PROBE-003 — Spec-version mismatch fails fast

- **Title:** A discovery document advertising an incompatible spec version is rejected at resolution, not at first request.
- **Preconditions:** SDK is configured to require `1.1.0`; deployment advertises `1.0.3`.
- **Wire assertion:** Construction-time discovery returns a `DiscoveryError` with reason `spec_version_mismatch`. No request to the openEHR REST endpoint is made.
- **Modes:** Sandbox, Cassette (constructed-mismatch cassette).
- **Status:** Implemented (Sandbox) — serves a fixture whose `org.openehr.rest` declares `1.0.3` and checks that the resolver fails fast with `DiscoveryError(spec_version_mismatch)` and returns no catalog.
- **Satisfies:** REQ-072.

#### PROBE-004 — PKCE verifier round-trip

- **Title:** A SMART launch using `S256` PKCE successfully exchanges code for token.
- **Preconditions:** Deployment registers the SDK as a SMART app with PKCE required.
- **Wire assertion:** Authorization request carries `code_challenge`, `code_challenge_method=S256` and exactly one `aud` naming the configured audience (REQ-061); token exchange carries `code_verifier`; token response is 200 with an `access_token`.
- **PKCE parity:** the SDK's verifier also satisfies RFC 7636 and `x/oauth2`: ≥ 32 bytes of decoded entropy, `base64.RawURLEncoding` (URL-safe, unpadded), and `code_challenge == base64url(SHA256(verifier))` with method `S256`, cross-checked against `golang.org/x/oauth2.S256ChallengeFromVerifier`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — drives a full `auth/smart` authorization-code and PKCE launch against a test token endpoint and checks the wire round trip and the PKCE parity properties.
- **Satisfies:** REQ-061.

#### PROBE-005 — Scope round-trip

- **Title:** A configured scope string survives token exchange verbatim and lands in the JWT scope claim or the response `scope` field. The SDK passes scopes through whatever their syntax ([REQ-165](auth.md#req-165--openehr-scope-syntax)), so the probe's scope is not an openEHR resource scope.
- **Preconditions:** Scope `patient/COMPOSITION.read` is requested.
- **Wire assertion:** Authorization request `scope` parameter contains `patient/COMPOSITION.read`; token response `scope` field contains it (or the JWT `scope` claim does).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — checks that the configured scope appears on the authorization request and survives into the token response's `scope` field as the SDK parses it.
- **Satisfies:** REQ-061.

#### PROBE-008 — Platform principal claims surface verbatim

- **Title:** When the token carries `principal_uid` and `principal_type` claims (per REQ-067), the SDK surfaces them on `LaunchContext.Principal` without coercion.
- **Preconditions:** A token with `principal_uid = "u-123"`, `principal_type = "AGENT"`.
- **Wire assertion:** SDK exposes `LaunchContext.Principal = {UID: "u-123", Type: PrincipalTypeAgent}`. Missing claims surface as nil/zero, not as guessed defaults.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — mints a signed id_token, validates it through `smart.LaunchContextFromTokenResponse`, and checks that the principal claims surface verbatim while a token without them yields a nil `Principal`.
- **Satisfies:** REQ-067.

#### PROBE-009 — Caller attribution forwarded on opt-in

- **Title:** When caller attribution is configured (REQ-066), the SDK emits the configured header and OTel attributes; when not configured, no attribution data appears on the wire.
- **Preconditions:** One client with `WithCallerAttribution(...)`, one without.
- **Wire assertion:** Configured client emits the `X-Cadasto-Caller-Attribution` header and `caller.agent_id` OTel attribute; unconfigured client emits neither.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — runs one `transport.Client` with `WithCallerAttribution` and one without against the same test endpoint, capturing the request header at the server and the span attributes through an in-memory `TracerProvider`, with no OTel SDK dependency.
- **Satisfies:** REQ-066.

#### PROBE-006 — JWKS rotation transparent to caller

- **Title:** A signing-key rotation on the authorization server triggers exactly one JWKS refresh in the SDK; subsequent requests succeed without consumer intervention.
- **Preconditions:** A cached JWKS does not contain the `kid` of the issued token (simulating rotation).
- **Wire assertion:** SDK fetches JWKS once, validates the token, and proceeds. No double-refresh, no double-validation failure surfaced.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — signs an id_token with a rotated key whose `kid` the seeded JWKS cache lacks, and checks that `smart.ValidateIDToken` recovers through exactly one refresh (the seed plus one fetch on the miss) and validates the token.
- **Satisfies:** REQ-062.

#### PROBE-007 — Token refresh transparent to caller

- **Title:** An expired access token with a valid refresh token is refreshed silently before the next request.
- **Preconditions:** Cached token has `expires_at < now`; refresh token is valid.
- **Wire assertion:** Token endpoint receives `grant_type=refresh_token`; the next outgoing request carries the new access token.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — both halves: the transport's safety net (a 401 on the wire, then `Reauth`, then one retry with the refreshed bearer, REQ-063), and the proactive half, in which an expired access token beside a valid refresh token makes the token endpoint receive exactly one `grant_type=refresh_token` request and `Source.Token` return the freshly issued bearer. Cassette and Live ratification are open.
- **Satisfies:** REQ-063

#### PROBE-106 — Sign-out revokes at the advertised revocation endpoint

- **Title:** `Source.Revoke` sends the source's refresh token to the advertised `revocation_endpoint` (RFC 7009) and leaves the source signed out.
- **Preconditions:** The SMART configuration advertises `revocation_endpoint`; the source holds an access token and a refresh token from a code exchange.
- **Wire assertion:** The revocation endpoint receives exactly one form-encoded `POST` carrying `token=<refresh token>` and `token_type_hint=refresh_token`, authenticated as the token endpoint authenticates the same client (a public client sends `client_id`); a `200` answer makes `Revoke` return nil, and afterwards the source holds no token and its next `Token` call fails with `auth.ErrReauthRequired` without a token-endpoint request.
- **Effect:** mutating (the revoked grant).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-167.

#### Launch-mode coverage

The three launch modes of [REQ-068](auth.md#req-068--flow-and-launch-mode-coverage) are exercised by coverage tests beside the auth probes. They are coverage proofs rather than catalogued `PROBE-NNN` IDs, but they are part of the auth suite's definition of done:

- **Standalone** — `Source.AuthorizeURL` builds an authorization URL with `response_type=code` and the `S256` PKCE challenge, and **no** `launch` parameter.
- **Embedded (EHR launch)** — an EHR-supplied `launch` parameter is forwarded verbatim to the authorization endpoint.
- **Backend service** — three confidential backend flows produce the expected token request on the wire: `auth/clientcreds` with `WithClientAssertion` (`client_credentials` and a signed `client_assertion`, no Basic, no `client_secret`: SMART Backend Services), and, outside the SMART asymmetric profile, `auth/clientcreds` with a symmetric `client_secret` (HTTP Basic) and `auth/jwtbearer` (the RFC 7523 §2.1 JWT Bearer authorization grant, not a SMART flow).

With the PKCE public flow (PROBE-004), confidential-client PKCE, and the `auth/smart` unit tests of the confidential-code auth-method selection, this exercises every flow in REQ-068's table across all three launch modes.

##### Inferno SMART App Launch (STU2.2) Client-suite cross-check

The HL7 Inferno **SMART App Launch Test Kit STU2.2 _Client_** suite serves as an external checklist (it cannot be executed here). Its client scenarios map to SDK coverage as follows:

| Inferno client scenario | SDK coverage | Status |
|---|---|---|
| **Public client** (authorization-code + PKCE, no secret) | PROBE-004 (PKCE and its RFC 7636 parity), PROBE-005 (scope), standalone and embedded launch modes | Covered (Sandbox) |
| **Confidential Symmetric** (`client_secret_basic`) | The authorization-code `client_secret_basic` selection in `auth/smart`, which sends `Authorization: Basic base64(clientID:secret)`, `grant_type=authorization_code` and no `client_assertion` | Covered (Sandbox) |
| **Confidential Asymmetric** (`private_key_jwt`) | `auth/smart` `WithClientAssertionKey`, with the discovery-driven method cross-check, and the `private_key_jwt` backend arm of the launch-mode coverage | Covered (Sandbox) |
| **Backend Services Asymmetric** (`client_credentials` + `client_assertion`) | The backend arm of the launch-mode coverage (`auth/clientcreds.WithClientAssertion`) | Covered (Sandbox) |

**Recorded gaps:**

- The SDK is the SMART **client**, so Inferno scenarios that check the SDK *responds* as an authorization server (token-introspection responder behaviour, for example) are out of scope by construction.
- Cassette and Live ratification of the auth probes against a reference authorization server, and a real Inferno run against a deployed endpoint, are open: the probes run in Sandbox only today.
- The ES384 and ES256 client-assertion paths are unit-tested in `auth/jwtbearer`, but the launch-mode backend coverage exercises the RS384 default only.

### Versioned writes and optimistic concurrency

#### PROBE-010 — PUT Composition without If-Match

- **Title:** A PUT against a versioned resource without an `If-Match` is never sent: the SDK refuses it at the call site.
- **Preconditions:** An existing Composition with a known `version_uid`.
- **Wire assertion:** The SDK refuses an empty `ifMatch` at the call site with `transport.ErrInvalidConfig`, so no request leaves the process. The probe sends nothing, so it does not observe how a backend answers a missing `If-Match`; that status mapping is [REQ-093](transport.md#req-093--openehr-error-envelope-mapping)'s.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-054, REQ-093

#### PROBE-011 — PUT Composition with stale If-Match

- **Title:** A PUT with a stale `If-Match` (referencing an old version_uid) is rejected with `412 Precondition Failed`.
- **Preconditions:** Composition has been updated since the SDK's cached `version_uid`.
- **Wire assertion:** PUT returns `412`, and the SDK maps it to `ErrPreconditionFailed`. The vendored ITS-REST pin answers a stale `If-Match` on a PUT with `412` and lists no `409` for it, so a `409` fails the probe.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-054, REQ-093

#### PROBE-012 — ETag survives round trip

- **Title:** A GET Composition followed by a PUT with the captured `ETag` as `If-Match` succeeds.
- **Preconditions:** Read-then-write workflow.
- **Wire assertion:** GET response carries `ETag`; PUT carries the same value as `If-Match`; PUT returns `204` or `200`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-054

#### PROBE-013 — Cross-EHR isolation

- **Title:** A `version_uid` belonging to EHR A cannot be read via EHR B's path.
- **Preconditions:** Two distinct EHRs; a Composition known to belong to EHR A.
- **Wire assertion:** GET `/ehr/{ehr_b_id}/composition/{version_uid_from_a}` returns `404 Not Found`, never `200`, never the EHR A data.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-054

### AQL

#### PROBE-020 — AQL builder string stability

- **Title:** The struct-builder and verb-functions produce byte-identical AQL strings for the same logical query.
- **Preconditions:** A reference query ("all OBSERVATIONs of archetype `body_temperature` for a given EHR") and its committed golden.
- **Wire assertion:** The struct builder (`aql.NewBuilder().Select(…).FromEHR(…).Contains(…).Build()`) and the verb functions (`aql.Select(…).FromEHR(…).Contains(…).Build()`) emit equal strings, byte for byte, and both equal the golden: `SELECT o FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o[…] WHERE e/ehr_id/value = $ehr_id` (the builders scope to the EHR through `WHERE`; the standing-predicate form is equally valid AQL).
- **Modes:** In-repo (golden comparison of the SDK's own output; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-055.

#### PROBE-021 — AQL parse error mapping

- **Title:** A syntactically invalid AQL string produced by a typed builder is impossible; a syntactically valid but semantically invalid one produces a typed `AQLError` on execution, and a valid query the deployment does not implement produces one distinguishable as a capability gap rather than as bad AQL.
- **Preconditions:** Reference deployment that validates AQL against templates.
- **Wire assertion:** The typed builders cannot emit syntactically invalid AQL (structural guarantee). Execution of a query referencing a non-existent path returns the backend's AQL error envelope; the SDK maps it to `*query.AQLError`, which satisfies `errors.Is(err, aql.ErrPathResolution)`. A **501** response maps to `*query.AQLError` satisfying `errors.Is(err, aql.ErrEngineCapability)` and never `aql.ErrPathResolution`: the capability gap is read from the status alone, with an envelope or bare, never from message text (REQ-055).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — the error mapping is tested against synthesised backend responses: a path-resolution envelope, and a 501 capability gap with an envelope and bare. Cassette and Live ratification wait for a reference deployment.
- **Satisfies:** REQ-055.

#### PROBE-028 — AQL lint stability

- **Title:** Linting fixed AQL strings against the SDK grammar profile (and, for Layer 3, a compiled OPT) yields a stable issue-code multiset.
- **Preconditions:** A compiled OPT (`vital_signs.opt`) and fixture query strings under `testkit/corpus/aql/lint/` (`valid.aql`, `missing_archetype.aql`, `bad_syntax.aql`).
- **Wire assertion:** In-repo property: `lint.LintString(q, &lint.Options{Compiled: c})` over each fixture produces exactly the expected `lint.Issue.Code` multiset: `valid.aql` → `aql_select_no_alias`; `missing_archetype.aql` → `aql_archetype_not_in_template` and `aql_select_no_alias`; `bad_syntax.aql` → `aql_syntax`. The two `aql_select_no_alias` entries are REQ-164's deliberate re-baseline of this corpus (both queries project a column with no `AS` alias), recorded in [PROBE-099](#probe-099--aql-path-shape-lint-corpus)'s entry; one baseline table serves this probe, PROBE-099 and [PROBE-097](#probe-097--aql-semantic-and-portability-lint-corpus) arm (b), so the corpus is re-baselined once. The cross-implementation claim covers REQ-109's own codes: another implementation of REQ-109 over the same grammar profile and template reports the same `aql_archetype_not_in_template` and `aql_syntax`. A code a later additive Layer-2 group contributes is held by that group's probe, so this multiset grows when such a group rightly flags a fixture, and that growth is a re-baseline recorded there, not a stability breach. Detail text and path strings are not checked.
- **Modes:** In-repo (issue-code multiset over a fixture corpus; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-109.

#### PROBE-080 — AQL parse/emit round-trip property

- **Title:** Parsing a catalogue AQL string into `parse.Query` and re-emitting via `(*Query).Emit` is stable — canonical inputs are preserved, re-emission is idempotent, and out-of-catalogue shapes surface `aql.ErrIncompleteAST` at both parse and emit rather than round-tripping silently.
- **Preconditions:** The AQL catalogue corpus: idempotence cases, canonical-input preservation cases, and the residual cases of [§ REQ-113 § Catalogue and residual](clinical-modeling.md#catalogue-and-residual).
- **Wire assertion:** In-repo property over every catalogue query: the fixed point of [§ REQ-113 § Round-trip property](clinical-modeling.md#round-trip-property), canonical inputs preserved, and the residual refused by both `ParseQuery` and `Emit` with `aql.ErrIncompleteAST`, never emitted partially.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113.

#### PROBE-082 — AQL structured path access (standing predicate + WHERE path)

- **Title:** A class standing predicate is readable as a structured `{path, operator, value}` (`ClassExpr.PredicateComparison`) and a WHERE comparison's alias-qualified path as alias + segments (`aql.Comparison.ParsedPath`), without importing `parse/gen` or re-tokenizing raw text (REQ-113).
- **Preconditions:** A parsed query carrying a standing class predicate (`EHR e[ehr_id/value=$ehr]`) and a WHERE comparison over an alias path (`o/data[at0001]/events[at0006]/value/magnitude > $threshold`).
- **Wire assertion:** In-repo property: the accessors of [§ REQ-113 § Structured path access](clinical-modeling.md#structured-path-access) on these inputs. `From.Root.PredicateComparison` gives `{ehr_id/value, =, $ehr}` for the standing comparison and is nil for an archetype-HRID predicate; the WHERE comparison's `ParsedPath` gives alias `o` and the ordered segments, with `ParsedPath.Raw == Comparison.Path`; and both survive a parse, emit and parse round trip, with `Emit` idempotent.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113.

#### PROBE-087 — AQL structured-AST catalogue completeness

- **Title:** Every shape in the list of [§ REQ-113 § Catalogue and residual](clinical-modeling.md#catalogue-and-residual), the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause and a projected literal's source text included, parses into `parse.Query` without `aql.ErrIncompleteAST` and round-trips through `(*Query).Emit` under the PROBE-080 fixed point.
- **Preconditions:** The catalogue corpus: queries for each catalogue shape, the former gap shapes among them, and the `TOP`-clause and literal-source-text cases.
- **Wire assertion:** In-repo property: for each catalogue shape, `ParseQuery` returns an AST pinned per shape and `Emit` is a fixed point; the residual of [§ REQ-113 § Catalogue and residual](clinical-modeling.md#catalogue-and-residual) still fires; a `TOP` clause carries its count and direction ([§ The `TOP` carrier](clinical-modeling.md#the-top-carrier)), and a projected literal carries its source text where that text differs from the canonical rendering ([§ Literal source text](clinical-modeling.md#literal-source-text)).
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — the closed gap shapes are checked modelled, the residual guard fires, each shape has a structural pin, and the `TOP` clause, its out-of-range count and literal source text have their own cases.
- **Satisfies:** REQ-113.

#### PROBE-088 — AQL builder containment and paging stability

- **Title:** The builder's negated containment (`NOT CONTAINS`), sibling containment junctions (`AND`/`OR`, parenthesised by precedence), opt-in in-text `LIMIT`/`OFFSET`, the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause, and the write-side carriers of [REQ-163](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) (the `VERSION` predicate bracket, the class-position standing comparison, and the typed projection: `DISTINCT`, `AS`, aggregates, function calls, literals, star) emit byte-stable canonical AQL, and a builder program using none of them emits what it emitted before they existed.
- **Preconditions:** Committed goldens covering the constructs: one per `versionPredicate` alternative; one per standing-predicate operand kind the write side can express (`pathPredicateOperand`'s `primitive`, `objectPath` and `PARAMETER` alternatives; its `ID_CODE` and `AT_CODE` alternatives have no `aql.Value` shape, so the builder cannot reach them); REQ-161's suppression shape; and one per `columnExpr` alternative plus the clause-level flags; beside the unchanged PROBE-020 golden.
- **Wire assertion:** In-repo golden comparison (no backend): built query strings match the committed goldens byte for byte, and each golden re-parses and re-emits unchanged, which ties the write-side canonical form to PROBE-087. Each build-time refusal of [§ REQ-163](clinical-modeling.md#req-163--aql-write-side-expressivity-parity) (a second row-bound channel, a `TOP` beside a row limit or with a negative count, a `SELECT` that does not read back as the projection the builder recorded, a typed construction the grammar does not admit where it renders) has a row that expects an error and no emission. The check is over the class of refusal rather than a fixed count of rows, so a shape added to either class extends it without amending this entry.
- **Modes:** In-repo (golden comparison of the SDK's own output; no backend).
- **Status:** Implemented (inline) — the goldens and their parse round trip, the row-bound refusals, and the REQ-163 carriers' accept and refusal rows, with the read-side identity round trip for each construct.
- **Satisfies:** REQ-163.

#### PROBE-090 — AQL emission round-trip closure

- **Title:** Every value a supported write path emits — string, real, and integer literals, a `MATCHES {uri}` operand, a value-position function call, and a containment tree — re-parses into an equal value, and the write-side guards that refuse an unspellable value agree with the vendored grammar rather than with a hand-written expectation.
- **Preconditions:** The escape, numeric, URI and function-name corpora; the write-side unit corpus; the predicate, identifier and archetype-id corpora; and the vendored lexer at `resources/aql/grammar/active/AqlLexer.g4`, which the grammar-confrontation tests read directly.
- **Wire assertion:** In-repo property: the checks [§ REQ-119](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) *Acceptance* names, over the corpora above; the rules are REQ-119's and this entry does not restate them. Every value kind survives emit, parse and compare under `aql.EqualValues` in every value position and on both carriers, or the guards refuse it; a literal read from source text survives twice (parse, emit, parse). The single-token positions are held to "the parser reads the string back unchanged" at every position of their class. Round-trip identity is the oracle at the bracket positions: a generated corpus is spliced at both positions into bases, at least one carrying material after the bracket and a later line break, and every base shows itself able to fail by some splice re-parsing as a different query; the `VERSION` shape rule has its own corpus of operands legal by construction. The grammar-derived guards are confronted with the grammar: for every token name in the lexer, and the keyword spellings carried only in lexer fragments, the builder emits a function of that name exactly when the parser accepts it, and for every URI in the corpus the builder emits the operand exactly when it re-parses as one `MATCHES` on the same URI. A source sweep finds no type assertion or switch on a sealed vocabulary outside a function that normalises. Every guard has a test that fails when it is removed, and a guard that could be tightened into refusing valid AQL has a positive control that fails when it is.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — across twelve suites: the structural sweeps, the grammar confrontations, the bracket-position confrontations, the after-emission verification and the per-rule corpora, with string-literal closure fuzzed.
- **Satisfies:** REQ-119.

#### PROBE-095 — AQL predicate structuring

- **Title:** Every bracketed path predicate the vendored grammar admits at a path-segment position arrives as a typed, trivia-free, delimiter-free structured value beside its untouched verbatim text — or as a member of the enumerated unstructured set, nil beside the verbatim text.
- **Preconditions:** The vendored grammar at `resources/aql/grammar/active/AqlParser.g4`, from which the corpus is generated rather than hand-listed, so no grammar alternative goes uncovered unnoticed ([§ REQ-119](clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) *Acceptance*).
- **Wire assertion:** In-repo property over the rules of [§ REQ-113 § Structured node predicates](clinical-modeling.md#structured-node-predicates), which this entry does not restate; it carries the oracle's construction. The corpus is generated from the grammar's `pathPredicate` alternatives, down to the `pathPredicateOperand` production whose members decide structured against enumerated-unstructured, and the sweep fails when the grammar admits a form that neither a kind nor the enumerated set covers, which a control shows it able to do. Every form is presented at both positions it can occupy, bracket top level and junction operand, and yields the same kind at each; each enumerated unstructured form gives nil beside populated verbatim text at both. Trivia and escape independence is a property driven from the same corpus: a mechanically padded spelling gives components equal to the bare spelling's while its verbatim text differs, both arms firing, with the interior-trivia spellings padding cannot generate (inside an operand path, a name slot, regex braces) as named rows beside it. Emission is checked unaffected: the REQ-119 round-trip suites stay green and the emitted text is byte-identical.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — the grammar sweep and its control, both-position structuring with the enumerated-nil rows, trivia independence over the generated corpus with the interior spellings, the five name spellings on both carriers, agreement of the two path extraction sites, and the emission fixed point.
- **Satisfies:** REQ-113.

#### PROBE-096 — AQL value-free structured diagnostics

- **Title:** Every construct the extractor drops carries a structured `{kind, clause, span}` record, every lint issue carries a span, no field the contract classifies value-free contains any substring of the input's value spans, and each span points at the construct the diagnostic is about.
- **Preconditions:** A corpus reaching every **reachable** drop class (today the numeric literal outside the value vocabulary, the residual of [§ REQ-113 § Catalogue and residual](clinical-modeling.md#catalogue-and-residual), in each clause that admits one); a representative lint-issue set spanning the layers `lint` runs (parameter-binding cases run with `Options.Query` set, since their codes fire on no other path); and, for each corpus query, the **value spans** of its input, identified independently of the diagnostic under test.
- **Wire assertion:** In-repo property in three arms, over the rules of [§ REQ-113 § Value-free structured drop records](clinical-modeling.md#value-free-structured-drop-records) and [§ REQ-109 § Value-free lint diagnostics](clinical-modeling.md#value-free-lint-diagnostics), which this entry does not restate; it carries each arm's oracle. **(a) Record per drop:** a sweep derived from the source of every non-test file in the package holds completeness, so a site added later, in any file, is covered the day it lands, and a control shows the sweep able to fail; the defensive arms' unreachability by a legal query is checked at unit level, since no corpus can reach them; one query carrying drops in two clauses exercises the source-order rule. **(b) Value-free fields:** for every corpus query, no value-free field of either surface, rendered whole with its span so a future string field cannot slip past, contains a substring of that input's value spans, checked against the input and never against expected strings; the value-bearing fields are deliberately not checked, and each case pins the code it exists to exercise, so it cannot pass on bystander issues. Additivity is checked over the corpus: codes, severities and `OK()` outcomes match their earlier values. **(c) Span attribution:** each span, sliced back out of the input, matches the offending construct rather than its enclosing clause, in one span type across both packages (a compile-time alias check), and an unattributable span is zero rather than approximated.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — arm (a) with its source sweep and control, arm (c) on the parse surface with the span helpers' guards, and arm (b) on the lint surface with per-case codes, the additivity pin, the alias span and the one-span-type compile-time check.
- **Satisfies:** REQ-113, REQ-109.

#### PROBE-097 — AQL semantic and portability lint corpus

- **Title:** Every REQ-161 issue code fires on a corpus query built to carry exactly that defect, with the REQ-161 severity and a span on the offending construct; queries with no REQ-161 defect yield no REQ-161 code; and the builder's verification agrees with the linter on the containment codes ([§ REQ-161 § Builder adapter](clinical-modeling.md#builder-adapter); the five containment codes, not the lint-only portability codes).
- **Preconditions:** The vendored grammar profile (parse) and the pinned BMM via `rminfo` — no CDR, no OPT. The [REQ-160 acceptance table](clinical-modeling.md#req-160--aql-containment-admissibility-relation) is the relation oracle; the EHRbase admissibility matrix is a neutral observed-behaviour cross-check, including the EHRbase-compatibility guard (an RM-valid pair a conformant engine admits never verdicts Never).
- **Wire assertion:** In-repo property, three arms. **(a) Per-code corpus:** for each REQ-161 code, at least one corpus query yields it with the specified severity at the span of the offending construct, and at least one near-miss query stays silent — the unknown-class suppression rule (an unknown operand suppresses the pair checks), the archetype-mismatch suppression (a literal archetype predicate whose declared class or HRID type segment is unknown yields `aql_unknown_rm_class` only, never `aql_archetype_class_mismatch`) and the `aql_fanout_row_grain` conservative firing rule each pinned by their own negative case. **(b) Additivity guard:** every query of the pre-REQ-161 corpus that carried no REQ-161 defect yields an unchanged issue-code multiset; corpus queries that genuinely carry a newly-checked defect are re-baselined deliberately, recorded here. **(c) Read/write parity** ([§ REQ-161 § Builder adapter](clinical-modeling.md#builder-adapter)): for corpus queries the builder can express, the verification's code multiset equals the containment-code subset of `lint.LintString` over the emitted text. The class-position `VERSION` predicate, the class-position standing comparison and the typed projection are builder constructs, so the corpus carries them through the same comparison; what stays unspellable is the FROM root's own bracket in every spelling (archetype, standing or version), since the write-side FROM clause carries an RM type and an alias and nothing else.
- **Modes:** In-repo (unit-level; no backend).
- **Status:** Implemented (inline) — arm (a) pins all eight REQ-161 codes with severity and span, and the three negative cases (unknown-class suppression, archetype-mismatch suppression, the `aql_fanout_row_grain` conservative firing rule). The unknown-class negative also has a Never-verdict sibling row that arms the operand-level suppression; the pure unknown-operand case, redundant at corpus level under REQ-160's pair-question totality, is pinned at engine level. Arm (b) re-ran PROBE-028's three fixtures under the full linter: they gained no REQ-161 code, so **no re-baseline was required**. Arm (c) holds read/write parity over the builder-expressible rows, among them REQ-161's own suppression shape (both class-position carriers at once), an explicit version tier in `CONTAINS` position, a standing predicate on an impossible pair and a typed projection over an unknown class, and checks that the corpus is not vacuous: every containment code is exercised by a row, and at least one row is clean. `VerifyContainment` never emits the three portability codes, which [§ REQ-161 § Builder adapter](clinical-modeling.md#builder-adapter) leaves out of parity.
- **Satisfies:** REQ-160, REQ-161, REQ-163.

#### PROBE-099 — AQL path-shape lint corpus

- **Title:** Every REQ-164 issue code fires on a corpus query built to carry exactly that defect, with Warning severity and a span on the offending segment, projection item, path or class expression; every firing rule's negative near-miss stays silent; and no query free of REQ-164 defects changes its issue-code multiset.
- **Preconditions:** The vendored grammar profile (parse) and the pinned BMM via `rminfo` — no CDR, no OPT. The [REQ-164 catalogue](clinical-modeling.md#req-164--aql-path-shape-and-paging-lint) is the oracle for each code's firing and silence rules; the [REQ-160 relation](clinical-modeling.md#req-160--aql-containment-admissibility-relation) is the oracle for the redundant-step rows.
- **Wire assertion:** In-repo property in two arms, PROBE-097's structure without the read/write-parity arm, since these codes have no builder analogue.
  (a) **Per-code corpus:** the firing witnesses and the near-miss rows that [§ REQ-164](clinical-modeling.md#req-164--aql-path-shape-and-paging-lint) *Acceptance* lists, each code at Warning severity with its span on the offending construct; the conservative walk's silent stops are pinned by name, and a `Result` carrying only this group's findings reports `OK()` true.
  (b) **Additivity guard and PROBE-028 re-baseline:** every query of the pre-REQ-164 corpus that carries no REQ-164 defect yields an unchanged issue-code multiset; [PROBE-028](#probe-028--aql-lint-stability)'s three fixture queries are re-run and any code they gain is recorded here as a deliberate re-baseline, per [REQ-161 § Additivity](clinical-modeling.md#additivity).
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — arm (a) pins all five REQ-164 codes with Warning severity and a span on the offending construct (a path segment name, a whole identified path, a projection item or a class token), and the deliberate zero span of `aql_paging_no_order_by`, whose two channels have no position in the query text. The named firing rows and negatives are each claimed by name, so the corpus cannot shrink below them and still pass, and a control that mutates the shipping corpus shows each of those guards able to fail. Beyond that floor the corpus fires the redundant-step row under a supplied relation, a repeating path reached only as a function-call argument, and a single `aql_paging_no_order_by` for an in-text `LIMIT` beside an envelope `Fetch`, and it silences the same redundant-step witness under a discriminating supplied relation (`EHR_STATUS`→`OBSERVATION`), which shows the probe reads `Options.Relation`. Arm (b) re-ran PROBE-028's three fixtures under the full linter and **re-baselined them deliberately**: `valid.aql` and `missing_archetype.aql` each gained exactly `aql_select_no_alias` and nothing else (both project a column with no `AS` alias, a defect they genuinely carry), while `bad_syntax.aql` gained nothing at all, failing at Layer 1 before Layer 2 runs. Neither fixture gained `aql_path_repeating_unpredicated` (`valid.aql` predicates every repeating segment), a row bound, a second projected path under one alias or a redundant step. That is the deliberate re-baseline [REQ-161 § Additivity](clinical-modeling.md#additivity) defines, recorded here as [§ REQ-164](clinical-modeling.md#req-164--aql-path-shape-and-paging-lint) *Additivity* requires.
- **Satisfies:** REQ-164.

#### PROBE-100 — Upstream AQL admissibility corpus ratchet

- **Title:** Every FROM/CONTAINS variant a vendored FROM-family combination corpus recorded as engine-accepted parsed under the SDK grammar profile and raised no Error-severity containment code.
- **Modes:** In-repo (no longer run).
- **Status:** Deprecated, and removed in v0.38.0 — superseded by [PROBE-107](#probe-107--upstream-aql-suite-admissibility-corpus), which rebuilds the FROM family's answered queries from the vendored suites and holds every answered query to a stricter rule: no Error-severity lint issue, not only no containment Error. The identifier is retained, carries no active requirement and is not reused.
- **Satisfies:** none (formerly REQ-160).

#### PROBE-107 — Upstream AQL suite admissibility corpus

- **Title:** Every query the vendored upstream AQL suites send and a conformant engine answers with rows parses under the SDK grammar profile and draws no Error-severity lint code, unless a reviewed decline names it and why — so the REQ-109 grammar profile and the REQ-160 compatibility guard are held to every clause family an engine answers.
- **Preconditions:** The vendored suite corpus under `testkit/corpus/aql/suites/`, with its provenance pin `AQL_SOURCE.txt` and the ingest-generated `EXCLUDED.txt` beside it ([development-process.md § Vendored fixtures](../development-process.md#vendored-fixtures)). No query template is transcribed into the reader: the suites are vendored, so the reader takes each template from the suite that sends it. The vendored grammar profile and the pinned BMM; no CDR, no OPT.
- **Wire assertion:** In-repo property.
  - **Rebuild.** The reader takes each test's query text from the vendored suites alone: a data-driven suite's single query template filled from every row of the CSV the suite names, or a test case's own query. Which argument of an upstream sending keyword carries the query is reader knowledge, transcribed from the upstream keyword resources at the pin, which the corpus does not vendor. The reader resolves Robot's escapes and the `${EMPTY}` and `${SPACE}` built-ins, and replaces a run-time variable that sits inside a quoted literal with one fixed literal, because the probe checks admissibility, not results. A test it cannot rebuild (a variable left outside a quoted literal, a query-building form the reader does not know, a data-driven suite with no template or more than one) is counted under a named reason, never dropped.
  - **Classify.** Each rebuilt query is classed by what its test asserts: **answered** (the test reads the returned rows), **answered-empty** (the test asserts that no row is returned), **refused** (the test expects an error response), **not-ready** (an active `not-ready` tag on the test or its suite) or **skipped** (a skipped suite or test). The class is what the test asserts, not what the engine returned: the upstream sending keyword turns some refusals into a pass or a skip, so before a decline is recorded for an answered query, its suite is read to confirm that the engine answered it.
  - **Answered.** For every answered query, `parse.ParseQuery` succeeds and `lint.LintString(q, nil)` raises no Error-severity issue, unless a decline in the reader's decline table covers the query. A decline names the Error code, the construct it fires on and one of two reasons: an engine grammar extension outside foundation AQL ([REQ-109 § Syntax floor](clinical-modeling.md#syntax-floor--the-sdk-grammar-profile)), or a `CONTAINS` operand an engine binds by an engine extension ([REQ-160 § Containable operands](clinical-modeling.md#containable-operands)).
  - **Declines exposed.** Each answered query the exported reader returns carries the declines that cover it (`SuiteQuery.Declines`), each with its Error code, construct and reason, and none where no decline does, so a consumer that gates queries through the same lint can tell a reviewed decline from a defect without copying the decline table.
  - **Answered-empty.** Every answered-empty query parses, and any Error-severity issue it raises is one of [REQ-161](clinical-modeling.md#req-161--aql-semantic-and-portability-lint)'s containment Errors: an engine that accepts a shape the RM rules out and answers it with no rows is consistent with a Never verdict (REQ-160 § Overlay edges).
  - **Not asserted.** Refused, not-ready and skipped queries are counted and not checked. The SDK stays the looser of the two, and the corpus is not a rejection catalogue: a not-ready test's expected-result file is the author's expectation, not an engine's answer.
  - **Ratchet.** The running test checks the tally per class and per family, and the number of queries each decline covers, so a corpus refresh that moves any query re-baselines this entry and the pin together. A decline that covers no query fails, and so does a family directory that contributes no answered query. Each guard has a control that fails when the guard is removed.
- **Modes:** In-repo (ratchet property over a vendored corpus; no backend).
- **Status:** Implemented (inline) — at the pin `AQL_SOURCE.txt` records, the reader rebuilds **953 queries from all 130 suites in all twelve families**: 909 answered, 31 answered-empty, 6 refused, 7 not-ready and 0 skipped. It counts 16 more that it cannot rebuild, all under `query-text-not-vendored`: the VERSION suites call `Get Composition Time Committed` (14 times) and `Get EHR Status Time Committed` (twice), whose query text lives in the upstream keyword resource, not in the suites. It reads six sending forms, each as the engine receives it: `Set AQL And Execute Ad Hoc Query` (920 queries), as the `q` member of the JSON body the keyword builds, once decoded; the stored-query `PUT /definition/query/…` keywords (9), the same way unless the suite stores plain text; `Send Ad Hoc Request` with the `q` member of its JSON body (18); `Send Ad Hoc Request Through GET` (2); `GET /query/aql?q={query}` with the test or suite variable `${payload}` (3); and a `GET On Session` to `/query/aql` with the `q` entry of its parameters (1). Which upstream keyword sends a query, and how, is the reader's census of the upstream keyword resources at the pin, each entry naming its file and line. A keyword that the census does not name and the suite does not define is counted, never run as a no-op; so is a library request to a query endpoint or to a URL known only at run time, and a Robot construct the reader does not run. Every answered query parses. The only Error-severity issue an answered query draws is `aql_contains_not_containable` on the `CONTAINS` operand `EVENT_CONTEXT`, on 25 queries in four suites of the ORDER_BY and WHERE families; the decline table's one entry covers exactly those, with the reason that an engine binds that operand by an engine extension ([REQ-160 § Containable operands](clinical-modeling.md#containable-operands)). Two answered-empty queries draw `aql_impossible_containment` (`FROM EHR_STATUS eq CONTAINS EHR_STATUS eq` and `FROM COMPOSITION c0[uid/value=…] CONTAINS COMPOSITION c1`), which an empty answer is consistent with. The FOLDER version-tier queries (`FROM FOLDER f CONTAINS VERSION cv0[LATEST_VERSION] CONTAINS COMPOSITION c`) draw none. The running test checks the tally per class, per family and per unrebuilt reason, the decline's 25, and that those 25 carry the one decline while no other query carries any; each guard has its control.
- **Satisfies:** REQ-109, REQ-160.

### Clinical modeling

#### PROBE-022 — OPT path resolution

- **Title:** Parsing an ADL 1.4 operational template (OPT) and resolving a fixture-defined list of openEHR paths returns nodes whose RM type, archetype node id, and (for archetype roots) archetype id match the expected values; explicitly unknown attributes and unmatched predicates produce `ErrPathNotFound`.
- **Preconditions:** A reference OPT body (XML bytes) and an assertion list mapping paths to expected node identity.
- **Wire assertion:** In-repo property: `template.ParseOPT`, `template.ParsePath` and `OperationalTemplate.NodeAt` against the fixture body match every assertion in the list, and each negative assertion (`ExpectNotFound`) surfaces `ErrPathNotFound`, wrapped.
- **Modes:** In-repo (path-resolution property against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-100.

#### PROBE-023 — Composition builder round-trip

- **Title:** Building a composition via `composition.NewBuilder` + `Set` → `Build` → `canjson.Marshal` → `canjson.Unmarshal` → re-marshal preserves the values supplied through `Set` at their addressed paths.
- **Preconditions:** A compiled OPT and a list of (path, value) assignments addressed against it.
- **Wire assertion:** In-repo property: `composition.NewBuilder(ctx, c, opts...)` with a `Set` per path and `Build` succeeds; `canjson.Marshal` of the result contains the assigned primitive values (magnitude and units for DV_QUANTITY, the value string for DV_TEXT, code and terminology for DV_CODED_TEXT) as byte fragments; `canjson.Unmarshal` into a fresh `*rm.Composition` succeeds, which shows the polymorphic dispatch on the nested data values works in both directions; and re-marshalling the decoded composition preserves the same fragments.
- **Modes:** In-repo (round-trip property over a compiled template; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-101.

#### PROBE-024 — Primitive constraint validate

- **Title:** Parsing an OPT and resolving a fixture-defined list of leaf paths, calling `PrimitiveConstraint.Validate` with a supplied Go value, returns the expected multiset of `ViolationCode` values per case.
- **Preconditions:** A reference OPT body (XML bytes) carrying at least one primitive-constraint child (C_BOOLEAN, C_INTEGER, C_REAL, C_STRING, C_DATE, C_TIME, C_DATE_TIME, C_DURATION, C_CODE_PHRASE, C_DV_QUANTITY, C_DV_ORDINAL) and a case list with positive (no violations) and negative (specific code expectations) entries.
- **Wire assertion:** In-repo property: `template.ParseOPT`, path resolution and `(*ComplexObject).PrimitiveConstraint().Validate(value)` match every case's `WantCodes` multiset, and a case marked `ExpectNoConstraint` addresses a node whose `PrimitiveConstraint()` returns nil.
- **Modes:** In-repo (issue-code multiset against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-103

#### PROBE-025 — Composition validate

- **Title:** Parsing an OPT, compiling it, and running `ValidateComposition(comp, c)` over a fixture-defined list of (OPT, composition, expected codes) tuples returns the expected multiset of `validation.Issue.Code` values per case.
- **Preconditions:** A reference OPT body (XML bytes) and a hand-built or fixture-decoded `*rm.Composition`; each case carries a `WantCodes []string` that captures the multiset semantics (order irrelevant, duplicates count).
- **Wire assertion:** In-repo property: `template.ParseOPT`, `templatecompile.Compile` and `validation.ValidateComposition` produce an `Issue.Code` multiset that matches each case's `WantCodes`. Positive cases expect an empty `WantCodes`; primitive and structural mismatches expect specific codes (`primitive_out_of_range`, `primitive_unit_unknown`, `primitive_not_in_list`, `slot_fill`, …).
- **Modes:** In-repo (issue-code multiset against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-102, REQ-103

#### PROBE-026 — Missing required nodes / cardinality

- **Title:** Sharpens PROBE-025 with negative structural cases — missing required nodes, empty multi-valued attributes with `existence ≥ 1` and `cardinality ≥ 1`, occurrences upper-bound violations, RM-type mismatches under C_SINGLE_ATTRIBUTE alternatives — and asserts the issue-code multiset (`required`, `cardinality`, `rm_type_mismatch`, `alternative_mismatch`) is stable across conformant implementations.
- **Preconditions:** The same OPT and composition tuple shape as PROBE-025; the cases are structural ones the template-driven validator detects.
- **Wire assertion:** In-repo property, with the pipeline of PROBE-025. A composition with the systolic ELEMENT removed surfaces no issue, since the template leaves that ELEMENT optional and the ITEM_LIST's `items` carry cardinality `0..*`, so their existence asks for no member ([§ REQ-102](clinical-modeling.md#req-102--composition-validation) *An existence lower bound alone asks for no member*); an empty `events` list surfaces `required` + `cardinality`, and an absent one `required` alone; an unmatched alternative surfaces `alternative_mismatch`.
- **Modes:** In-repo (issue-code multiset against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-102

#### PROBE-027 — Generated instance validates clean

- **Title:** `instance.Generate(c, opts)` followed by `validation.ValidateComposition(out, c)` and `validation.ValidateRM(out)` each return `Result.OK = true` under every policy (`Large`, `Medium` and `Required`) and both `ValueFill` values, on an OPT the generator can synthesise, outside the census rows in the status.
- **Preconditions:** Compiled OPT for a fixture template; valid composer + territory for COMPOSITION roots.
- **Wire assertion:** Cross-package round-trip — generator and validator agree on the same template-driven contract. On the compiling vendored corpus the same generated value also passes `validation.ValidateRM`, the output rule of [§ REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), outside the cases its *Exceptions and known gaps* lists.
- **Modes:** In-repo (generator and validator agreement over fixture OPTs; no backend).
- **Status:** Implemented (inline) — beside the probe, a corpus census runs `instance.Generate` (every policy, both fills, `RandomFill` under three fixed seeds) and `composition.NewBuilder` over every compiling COMPOSITION-rooted OPT (the ADDRESS and PERSON roots run `instance.Generate` only), then `validation.ValidateRM` and, for a COMPOSITION root, `validation.ValidateComposition` (otherwise `validation.Validate`). A failure that is not in the census table fails the test, and a table row that now passes fails it too. One OPT does not compile and is allowlisted: `definition/body_weight` (no `template_id`). What remains in the table is not a validator miss: `TestPerson.v2` codes its two `DV_MULTIMEDIA` media types in terminology openEHR with codes outside the media-type group, which the generator keeps over the RM default (REQ-107 § Exceptions and known gaps), under `Large`. `Medium` builds at most one optional archetype under an attribute, the first that needs no undescribed content, so it reaches no `TestPerson.v2` identifier cluster, and has no row. `family_history.v.1.2.3` has none either: its unfillable slot sits in an optional CLUSTER whose required items admit only slots, which no policy builds (REQ-107 § Slot fills). A second census lists every member no OPT node describes (a slot fill, or a LOCATABLE built from its BMM type) in the `Large` and `Medium` runs under `ExampleFill` and fails on one under an optional node; the one that remains sits below a node the RM requires, the `item` of the ITEM_SINGLE in `clinical_content_validation`'s EVALUATION data, which the template leaves without a child, and is pinned with that reason. A list that only its existence requires gets no undescribed member (REQ-107 § Contract *What a visit writes*), so the protocols and event states of `vital_signs`, whose items the template leaves without a child or with slots only at an existence of 1..1, hold none. Under `Required` the table has no row: that policy reaches none of the optional archetypes those rows fail in. Since `Large` gives each optional archetype that needs no undescribed content its member, no template's body is hollow under `Large`, so a `hollow_body` failure there fails the census. Under `Medium` a template whose kept optional archetype holds no ELEMENT gets a hollow body by design, so the census pins the templates whose body is hollow under it instead (`templates/clinical_content_validation`, whose first optional content archetype that needs no undescribed content is a SECTION with no items): a listed template that stops being hollow fails the census, and so does an unlisted one that becomes hollow. Under `Required` a body with no ELEMENT is the expected result wherever the template makes no ELEMENT mandatory, so the census pins the templates whose body holds one under it instead (`templates/TestPerson.v2`, `templates/test_template_rename_node_2`): a listed template that becomes hollow fails the census, and so does an unlisted one that stops being hollow. `Required` leaves out an optional `content` that only its cardinality asks a member of, at existence `0..1` and cardinality `1..*` over optional archetype roots, as most vendored COMPOSITION templates write it, and visits an optional list for a child whose occurrences lower bound is 1 or more (REQ-107 § Contract *Visit rule*), which the template validator then asks for (REQ-102 *A mandatory child under an optional list*). The archetype-id refusal (`instance.ErrArchetypeIDMissing`) has no row: no vendored OPT asks for an archetype root it names no archetype for.
- **Satisfies:** REQ-107.

#### PROBE-074 — Template-driven validation of non-COMPOSITION roots

- **Title:** `validation.Validate(root, c)` over a fixture-defined list of (OPT, RM root, expected codes) tuples returns the expected `validation.Issue.Code` multiset for archetypeable roots outside the COMPOSITION content set — the demographic PARTY hierarchy (`PERSON`/`ORGANISATION`/`GROUP`/`AGENT`/`ROLE` + sub-components) and the EHR-IM roots (`FOLDER`/`EHR_STATUS`).
- **Preconditions:** An OPT body rooted at the target RM type (real `Address.v2.opt` / `TestPerson.v2.opt`, or a synthetic root) and an in-memory or fixture-decoded RM root; each case carries a `WantCodes []string` multiset.
- **Wire assertion:** In-repo property: `template.ParseOPT`, `templatecompile.Compile` and `validation.Validate` produce an `Issue.Code` multiset matching each case's `WantCodes`. A conformant ADDRESS instance validates clean; a `PERSON` under an `ORGANISATION` OPT surfaces `rm_type_mismatch`; a `PERSON` whose OPT-pinned `identities` list is empty surfaces `required` + `cardinality`, and one without the list `required` alone; a `FOLDER` whose archetype id differs surfaces `archetype_id_mismatch`.
- **Modes:** In-repo (issue-code multiset against fixture OPTs; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-102, REQ-103.

#### PROBE-075 — WebTemplate structural parity

- **Title:** `webtemplate.Marshal(templatecompile.Compile(opt))` structurally matches a vendored EHRbase `openEHR_SDK` v2.3 reference WebTemplate JSON for the same OPT — the `id` set, `rmType`, `aqlPath`, `min`/`max`, and per-node input `suffix`/`type` — against a documented-deviations list.
- **Preconditions:** Commit-pinned EHRbase `openEHR_SDK` reference WebTemplate JSON and OPT pairs, vendored under `testkit/corpus/` with provenance and Apache-2.0 attribution. Three fixtures: `constrain_test`, which pins no template-level node name, so its golden carries no name predicate across its 104 nodes; and the two [REQ-116](clinical-modeling.md#req-116--template-level-node-naming-and-name-predicated-paths) oracles, `Corona_Anamnese` (four `SECTION.adhoc.v1` siblings reusing one archetype) and `GECCO_Diagnose` (whose golden name-predicates 24 `aqlPath`s). Residual input and `min` deltas are documented and count-pinned in [`deviations.md`](../../openehr/template/webtemplate/deviations.md).
- **Wire assertion:** Not backend-facing: an in-repo parity property. `template.ParseOPT`, `templatecompile.Compile` and `webtemplate.Build`/`Marshal` reproduce the reference node structure (id, rmType, aqlPath, min, max, and input suffix and type per node); every accepted difference (field ordering, absent optional fields, localized-string packaging, known id edge cases) is on the documented-deviations list, and any difference not listed is a failure.
- **Modes:** In-repo (parity property against a vendored fixture; no backend).
- **Status:** Implemented (inline) — exact structural parity on node structure (id, rmType, nodeId, aqlPath, min, max) and input suffix and type against all three references: `constrain_test` 104/104, `Corona_Anamnese` 230/230, `GECCO_Diagnose` 34/34, with the residual deltas count-pinned per fixture.
- **Satisfies:** REQ-106; REQ-116 (the two name-pinning oracles are its acceptance fixtures).

#### PROBE-077 — RM-floor invariant matrix

- **Title:** For every invariant the inventory of [clinical-modeling.md § REQ-171](clinical-modeling.md#req-171--validation-engine) *Where the RM rules come from* classifies as evaluated, and every registered class it makes subject to that invariant, `ValidateRM` reports the invariant on a breaking value and does not report it on a conforming one.
- **Preconditions:** The generated invariant inventory and its classification, and, per (invariant, class) pair, a breaking and a conforming value written as canonical-JSON members and decoded through the type registry.
- **Wire assertion:** In-repo property. For each pair, `ValidateRM` on the breaking value reports an issue whose `Rule` is the invariant, or, for an invariant another check decides, that check's issue at the pair's path; on the conforming value it reports neither. A newly evaluated invariant, or a class newly subject to one, without a pair fails the probe, and so does a pair for an invariant the inventory does not classify as evaluated.
- **Modes:** In-repo (unit-level property over the inventory; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-112, REQ-171.

#### PROBE-081 — EHR_STATUS value-typed mandatory presence (subject and ARCHETYPED)

- **Title:** `validation.ValidateRMEHRStatusBytes(data)` decides value-typed mandatory presence from JSON-key presence: it flags an omitted RM-mandatory `subject` (typed `rm.PartySelf`, a value struct) without false-positiving on a valid bare `PARTY_SELF`, and the same for the root's ARCHETYPED keys (`archetype_details.archetype_id` / `rm_version`).
- **Preconditions:** Canonical-JSON EHR_STATUS bodies. Subject arm: one omitting the `subject` key; one with `subject: null`; one supplying `subject` as a bare `{"_type":"PARTY_SELF"}` (no external_ref); one omitting the interface-typed `name`. ARCHETYPED arm, each on the root `archetype_details`: one omitting, and one nulling, `archetype_id`; one omitting, and one nulling, `rm_version`; one with an empty `rm_version` (`""`); and a complete root `archetype_details` with an attribute-less ARCHETYPED nested under `other_details`. Shape: a non-object body, and one whose root `archetype_details` is not an object.
- **Wire assertion:** In-repo property: each body draws exactly the findings [§ REQ-112 § Value-typed mandatory presence](clinical-modeling.md#value-typed-mandatory-presence) gives it: the subject arm's `required` at `/subject`, absent for the bare `PARTY_SELF`; the ARCHETYPED arm's one finding per path, the value walk's findings alone for a present key and for an `archetype_details` below the root; and a single `invalid_shape` at `/` for each shape body. The interface-typed `name`, when absent, still draws `required` at `/name`.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — both arms.
- **Satisfies:** REQ-112.

### Simplified formats and upstream parity

#### PROBE-076 — FLAT / STRUCTURED composition round-trip

- **Title:** For a vendored upstream (OPT + canonical COMPOSITION) pair, the simplified codecs round-trip the composition without losing the data the formats carry — FLAT is idempotent (`MarshalFlat → UnmarshalFlat → MarshalFlat`), STRUCTURED re-encodes to the same FLAT, OPT-free interconversion preserves it — **and**, when the source composition is itself OPT-valid, a `WithTemplate` decode of the emitted FLAT validates against the OPT (the conformance leg).
- **Preconditions:** Vendored EHRbase (Apache-2.0) `Test_dv_*` datatype OPTs and matching canonical composition JSON under `testkit/corpus/` (provenance in [`THIRD_PARTY_LICENSES.md`](../../testkit/corpus/THIRD_PARTY_LICENSES.md)); the WebTemplate is built with `templatecompile.Compile` and `webtemplate.Build` (REQ-106).
- **Wire assertion:** Not backend-facing: an in-repo property across the datatype corpus (DV_TEXT/CODED_TEXT/DATE_TIME/QUANTITY/COUNT/BOOLEAN/ORDINAL/PROPORTION/DURATION/IDENTIFIER/URI + the `|raw`-carried MULTIMEDIA/PARSABLE/INTERVAL): round-trip idempotence plus the OPT-validation leg, which catches dropped or mistyped leaves that a symmetric omission would hide from idempotence alone. **Scope limit:** symmetric omission of *optional* data can still pass, and the probe does **not** compare emitted FLAT/STRUCTURED against vendored upstream simplified output, which is [PROBE-086](#probe-086--upstream-flat-serialisation-parity)'s check; see [`deviations.md`](../../openehr/serialize/simplified/deviations.md) § Conformance. A template the WebTemplate builder cannot yet model is a documented skip, never a fail.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — over the vendored constraint-template corpus (24 pass, 1 skip).
- **Satisfies:** REQ-053.

#### PROBE-086 — Upstream FLAT serialisation parity

- **Title:** For each body in the pinned upstream EHRbase FLAT conformance corpus, decoding it and re-encoding it through the SDK's REQ-053 codec reproduces the upstream FLAT — same path set, same leaf values — over the subset of the corpus that the codec models, with everything outside that subset counted rather than waived.
- **Preconditions:** The commit-pinned corpus under `testkit/corpus/flat-conformance/`: the `conformance_ehrbase.de.v0` OPT plus 34 `ehrbase_conformance_*.json` FLAT bodies from `composition/flat/simSDT/conformance/`, integrity-checked against `MANIFEST.txt` by `make flat-check` (the offline gate `make ci` runs; `make flat-drift` adds a network upstream-drift report) (Apache-2.0; provenance in [`THIRD_PARTY_LICENSES.md`](../../testkit/corpus/THIRD_PARTY_LICENSES.md)).
- **Wire assertion:** Not backend-facing: an in-repo parity property. `template.ParseFile`, `templatecompile.Compile` and `webtemplate.Build`, then per fixture `simplified.UnmarshalFlat(upstream, wt, WithTemplate(c), WithUnmatched(report))` and `simplified.MarshalFlat`. The comparison is scoped to the **modelled subset**: the upstream keys left after (a) removing what the codec reports as unmatched content ([§ REQ-170](wire.md#req-170--unmatched-content-on-decode)) or refuses on decode, and (b) holding a **named allow-list** of composition-level metadata out on *both* sides.

  The allow-list holds out **respellings**: `language`, `territory`, `composer|name`, `composer_self`, `context/start_time` and `context/setting`, which REQ-053 reads and writes as the `ctx/` short forms (`ctx/language`, `ctx/composer_name`, `ctx/time`, `ctx/setting|code` and `|value`) where upstream writes real paths. Compared across the two spellings, every such key would report as **Missing**, since the `ctx/` key the encoder wrote in its place is skipped on the emitted side and so never surfaces as Extra. `composer|name` is held out on both sides whichever spelling encode writes: a composer a `ctx/` form carries whole comes back as `ctx/composer_name`, and a composer with a reference keeps `composer|name` at its real path ([ADR 0029](../adr/0029-composer-real-path-carrier.md)), where the hold-out removes it too, so the name is not compared. The hold-out is **suffix-aware**, not per path: the composer's `external_ref` suffixes (`composer|id`, `|id_scheme`, `|id_namespace`) and its party sub-structure (`composer/_identifier:N`, `composer/relationship`) are not respellings and are not held out, since decode reads them and encode writes them at the same real path ([ADR 0029](../adr/0029-composer-real-path-carrier.md)), so the census compares them. `context/setting` is an ordinary respelling: the real path decodes, re-encodes as `ctx/setting|code` and `|value`, and its keys are held out on both sides like `context/start_time`. The hold-out carries **no waiver**: every held-out spelling resolves to an accepted alias or a terminology witness.

  The harness derives its expected hold-out from the codec's exported alias accessors (`simplified.MetadataAliasSpellings`, `MetadataWitnessSpellings`) and checks the reverse direction too, so an alias accepted on decode but missing from the hold-out (a spurious Missing) and a hold-out with no alias behind it (a real key silently suppressed) both fail the harness's own tests rather than being caught by eye. The waiver class is empty, and a new waiver is a spec decision, not a harness edit. The hold-out is an explicit allow-list rather than a prefix test: `context/other_context` carries archetyped data and is compared, and so is `category`, a template-constrained Web Template leaf riding its own FLAT path, spelled identically on both sides, not composition metadata.

  Within the modelled subset the re-encode reproduces the upstream path set with equal leaf values: a dropped, invented or altered key is a failure, and there is no tolerated-drop list, so an upstream key that decodes and then does not re-encode is a defect, not a skip. The excluded set is derived from what the codec itself reports as unmatched content and from its own decode refusals, never kept by hand, so closing a gap widens coverage without a list to update; each refusal removes only what it names, so one unmodelled `|suffix` does not withdraw the modelled suffixes beside it. The harness package's own tests pin the excluded and compared counts per fixture, so the unmodelled surface can shrink deliberately but never grow unnoticed. Apart from the pins, a fixture that compares **nothing** fails: exact agreement over an empty compared set is vacuous, so a total coverage collapse never reads as a pass. This is strictly stronger than [PROBE-076](#probe-076--flat--structured-composition-round-trip), whose input is the SDK's *own* output and so cannot catch a path this SDK never emits, a suffix it names differently, or a leaf it silently drops.
- **Modes:** In-repo (parity property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — the probe is **exact** on what it compares and adds the **coverage floor**: it fails a fixture that compared zero keys, which the harness's report would otherwise call clean. The per-fixture excluded and compared ratchet is the harness package's own pins. The measured census (modelled-subset size, refusal inventory, root causes, and what would move the number) is in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md), regenerated with the harness's `-census` flag.
- **Satisfies:** REQ-053, REQ-106, REQ-170.

#### PROBE-089 — Underscore-attribute round-trip

- **Title:** Every [REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes) underscore-attribute family round-trips FLAT → canonical RM → FLAT as a [wire-equivalent](#terms) document, and an RM composition populated with an in-scope attribute emits it without loss — per family, including the recursive shapes.
- **Preconditions:** The PROBE-086 corpus (underscore keys are ~60% of its 1824 keys), plus SDK-authored per-family fixtures against the vendored OPTs covering each row of the REQ-140 grammar table — including recursion (`_feeder_audit/…/provider/_identifier:N`, `dv_multimedia/_thumbnail`), `_null_flavour` beside an absent bare value, and the deliberate refusals.
- **Wire assertion:** In-repo, not backend-facing. Per grammar family: (a) **decode**: a FLAT body carrying the family's keys decodes into the typed RM attribute (never a `|raw` detour), and re-encoding reproduces the input as a [wire-equivalent](#terms) document; (b) **encode**: a composition populated with the attribute emits exactly the family's key set: no silent drop, and no `|raw` fallback where the grammar carries the value; (c) **refusals**: the `_instruction_details` and `_wf_definition` keys fail with typed errors naming the key, and are never decoded and dropped; (d) **STRUCTURED**: the same fixtures interconvert FLAT ↔ STRUCTURED preserving the underscore vocabulary (`_`-keys as members, arrays for `:N`). The PROBE-086 census moves in step: the `path not in web template` excluded family shrinks by the underscore keys a family carries, and the per-fixture compared and excluded pins are re-baselined in the same change, so a family whose corpus keys stay excluded fails this probe rather than becoming a census footnote.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — 18 SDK-authored fixtures against the vendored corpus OPT (one per grammar-table row, and one per shape of the composer at its real path) and the deliberate exclusions, with a misused framework reported apart from a codec failure. What each leg pins: **(a)** [wire-equivalence](#terms) over the *whole* fixture body, so a family cannot be carried at the cost of a key beside it; **(b)** the decoded composition goes out through canonical JSON and back before the re-encode, which makes this an encode check rather than a second reading of (a), since an underscore value parked anywhere the canonical form does not model vanishes there; the emitted family key set matches exactly, and a `|raw` at a base the grammar carries is reported as its own failure; **(c)** each exclusion fails with the sentinel its boundary declares and names the offending key, a successful decode being the decode-and-drop REQ-140 forbids; **(d)** the OPT-free interconversion carries the `_` vocabulary as array-valued members, and the OPT-driven STRUCTURED round trip returns the same FLAT. The census movement is recorded in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md).
- **Satisfies:** REQ-140, REQ-053.

#### PROBE-105 — Upstream cross-format parity

- **Title:** For each vendored upstream set that gives one composition in two or more of canonical JSON, canonical XML, FLAT and STRUCTURED, the SDK codecs carry the composition across every leg below whose two formats the set gives and agree with the upstream sibling over the subset they model; every place they do not agree is a recorded outcome with its reason, never a silent waiver.
- **Preconditions:** The sets under `testkit/corpus/crossformat/` ([development-process.md § Vendored fixtures](../development-process.md#vendored-fixtures)), each with the OPT of its template, from which the probe builds the Web Template (`templatecompile.Compile` + `webtemplate.Build`).
- **Wire assertion:** In-repo, not backend-facing. Per set, every leg whose two formats the set carries:
  - (a) **JSON and XML:** the canonical JSON and the canonical XML each decode, and the two decoded compositions, re-encoded through `canjson`, are compared leaf by leaf: a comparison of decoded values, never of bytes ([§ REQ-052](wire.md#req-052)).
  - (b) **canonical to FLAT:** `simplified.MarshalFlat` over the decoded upstream canonical document is compared key by key with the upstream FLAT. That document is the set's canonical JSON when the set has one, and its canonical XML otherwise.
  - (c) **FLAT to canonical:** `simplified.UnmarshalFlat` with `WithTemplate` over the upstream FLAT is compared leaf by leaf, in canonical JSON, with the decoded upstream canonical document. That document is the set's canonical JSON when the set has one, and its canonical XML otherwise.
  - (d) **FLAT to STRUCTURED:** `simplified.FlatToStructured` over the upstream FLAT, which needs no template, is compared leaf by leaf with the upstream STRUCTURED.
  - (e) **STRUCTURED to FLAT:** the upstream STRUCTURED, decoded and re-encoded as FLAT, is compared with the upstream FLAT decoded and re-encoded the same way.

  Every FLAT comparison holds composition-level metadata out on both sides with the [PROBE-086](#probe-086--upstream-flat-serialisation-parity) hold-out, the same allow-list derived from the codec's alias accessors, so an allow-listed `ctx/` short form and its upstream real path are not reported as a difference. Every FLAT and STRUCTURED decode uses the PROBE-086 exclusion: the keys the codec reports as unmatched content ([§ REQ-170](wire.md#req-170--unmatched-content-on-decode)) are excluded, and a key family the codec refuses is removed, no wider than the refusal names, and the decode retried, so one refusal does not hide the rest of the document. Each leg's outcome is either a refusal (the codec's error) or the counts compared, missing, extra, altered and excluded, and it equals the outcome recorded for that set and leg. Excluded counts the upstream FLAT keys the PROBE-086 exclusion removed before the comparison, those the codec reported as unmatched and those removed after a refusal, in the legs that decode the upstream FLAT, (c) and (e): in (e) both sides are decoded, so a removed key family can drop out of both, and only this count keeps that visible. A leg whose outcome changes fails until its record changes in the same commit, so a gap opens or closes only deliberately. A recorded refusal, difference or non-zero excluded count states why it exists. A leg whose recorded outcome is not a refusal compares at least one key or leaf, since agreement over an empty set is vacuous. The recorded outcomes and their reasons are published in a census beside the harness that the harness regenerates, not one kept by hand.
- **Modes:** In-repo (parity property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — the harness runs the legs, holds the recorded outcomes and their reasons, and generates [`CENSUS.md`](../../testkit/conformance/crossformat/CENSUS.md), with a test failing when the committed census is stale; the probe fails any leg whose outcome differs from its record. The counts of sets, legs and outcomes live only in that census.
- **Satisfies:** REQ-052, REQ-053, REQ-056, REQ-169, REQ-170.

### Canonical JSON and formats

#### PROBE-030 — Canonical-JSON round trip

- **Title:** Decoding a canonical-JSON RM value and re-encoding it, then decoding and encoding that output again, recovers the same value and a document wire-equivalent to the first, and the recovered value satisfies the reference-model floor.
- **Preconditions:** A canonical-JSON RM root fixture from the top-level files of `testkit/corpus/compositions/` and `testkit/corpus/rm/` ([development-process.md § Vendored fixtures](../development-process.md#vendored-fixtures)): a COMPOSITION, or another root sample such as an EHR_STATUS or a FOLDER. The probe's leaf values, a DV_QUANTITY and a DV_CODED_TEXT, are inline in the probe.
- **Wire assertion:** Decode, Encode (`b1`), Decode (`A`), Encode (`b2`), Decode (`B`), then the comparison [§ REQ-052](wire.md#req-052) gives a round trip: `A` and `B` equal by typed deep comparison (`reflect.DeepEqual` over the decoded RM values, which compares an interface-typed field by its dynamic type and value, so it covers every substitutable slot and every `DV_INTERVAL[T]` bound with no comparison options); `B` clean under `validation.ValidateRM` ([REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)); `b1` and `b2` [wire-equivalent](#terms); and `b1` wire-equivalent to the input's expected form, so a member the decoder reads with the wrong meaning, a wrongly defaulted interval flag among them, fails the probe. No byte equality is checked anywhere. An input whose vendored content carries an RM-floor finding unrelated to the round trip is held out of the `ValidateRM` leg only, named in the probe with the finding, while the fidelity legs still run on it.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-052, REQ-040, REQ-169

#### PROBE-031 — `_type` discriminator decoded via registry

- **Title:** A `_type` not in the type registry decodes to a typed `UnknownTypeError`, not silently to `map[string]any`.
- **Preconditions:** A fixture containing an unregistered `_type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown `_type` value.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-040, REQ-052

#### PROBE-038 — RM polymorphic decode coverage

- **Title:** `canjson.Unmarshal[Composition]` decodes every BMM-admissible `_type` discriminator at every substitutable slot — including (a) substitutable subtypes in concrete-typed slots (e.g. `LOCATABLE.name` carrying `DV_CODED_TEXT`) per openEHR RM Liskov substitution, and (b) generic types with abstract type parameters (e.g. `DV_INTERVAL[T: DV_ORDERED]`).
- **Preconditions:** Vendored RM fixtures under `testkit/corpus/rm/polymorphic/` covering both failure modes.
- **Wire assertion:** Decode succeeds; the recovered value preserves every original `_type` discriminator that sits on a member the RM defines, counted as a multiset, so a lost substitution fails while an elided duplicate does not, and a `_type` inside a member the decoder ignores ([§ REQ-052](wire.md#req-052)) is not counted; re-marshalling produces a document [wire-equivalent](#terms) to the same logical content. No member order is asserted.
- **Modes:** In-repo (decode property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — decode, re-marshal and `_type` preservation over `testkit/corpus/rm/polymorphic/`. The probe checks canonical JSON only, though the generated `<Parent>Like` interfaces serve canonical XML dispatch too.
- **Satisfies:** REQ-040, REQ-052

#### PROBE-033 — Canonical-XML round trip

- **Title:** Decoding a canonical-XML RM value and re-encoding it, then decoding and encoding that output again, produces two byte-identical compact-XML SDK encodes.
- **Preconditions:** A canonical-XML RM value fixture from the top-level files of `testkit/corpus/compositions/` and `testkit/corpus/rm/` ([development-process.md § Vendored fixtures](../development-process.md#vendored-fixtures)): a COMPOSITION, or an RM root or leaf sample such as a FOLDER or a DV_QUANTITY. The probe also builds a DV_QUANTITY, a DV_TEXT and a COMPOSITION inline.
- **Wire assertion:** Decode → Encode → Decode → Encode; the two compact-XML SDK encodes are byte-identical. Equality with the input fixture is not checked: the fixture's whitespace and serialisation choices differ from the SDK's compact form ([§ REQ-056](wire.md#req-056)), so the stability checked is that of the SDK's own output.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-056, REQ-040

#### PROBE-034 — `xsi:type` discriminator decoded via registry

- **Title:** An `xsi:type` not in the type registry decodes to `typereg.ErrUnknownType`, not silently to an untyped value.
- **Preconditions:** A fixture (or hand-crafted XML) containing an unregistered `xsi:type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown type value, wrapped in `*typereg.DecodeError`.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-040, REQ-056

#### PROBE-032 — FLAT → canonical → FLAT round trip

- **Title:** Given an OPT and a FLAT payload, converting FLAT → canonical and back to FLAT produces the original FLAT payload (modulo documented OPT-driven normalisation).
- **Preconditions:** A reference OPT + FLAT pair.
- **Wire assertion:** Round-trip equality after OPT-driven normalisation.
- **Modes:** In-repo (round-trip property over an OPT and FLAT pair; no backend).
- **Status:** Draft.

### Service discovery

#### PROBE-040 — Catalog cache honours TTL

- **Title:** Two SDK constructions within the TTL window of a cached catalog do not produce a second discovery fetch.
- **Preconditions:** Catalog with declared TTL > 0; two constructions in quick succession.
- **Wire assertion:** Exactly one discovery fetch occurs.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-070, REQ-072

#### PROBE-041 — Catalog refresh on 401

- **Title:** A `401` from a previously-working endpoint triggers exactly one discovery refresh and one retry; failure to recover surfaces a typed error.
- **Preconditions:** Cached catalog; backend rotates and returns `401` on the cached token.
- **Wire assertion:** SDK refreshes JWKS/catalog once, retries once. On second `401`, returns `transport.ErrUnauthorized`.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — the discovery-layer half: the resolver's `Refresh` against a 401 upstream issues exactly one fetch and returns a typed `*discovery.DiscoveryError(fetch_failed)`. The transport's 401 reauth hook is opt-in (`transport.WithReauthOn401` with an `auth.ReautherFunc`, as PROBE-007 uses it); refreshing the discovery catalog on a 401 in an application (REQ-071) is the open half.
- **Satisfies:** REQ-071 (discovery half), REQ-072

### REST binding

The REST-binding probes check the openEHR REST 1.1.0 wire contract above `transport/` and the typed leaf clients under `openehr/client/`.

#### PROBE-060 — EHR creation round-trip

- **Title:** `POST /ehr` with an initial `EHR_STATUS` body answers with a `Location` naming the assigned `ehr_id`, the SDK surfaces that id, and a follow-up `GET` of the EHR_STATUS returns the status that was committed.
- **Preconditions:** Backend supports server-assigned `ehr_id`.
- **Wire assertion:** The create is one `POST` on the exact service-relative `/ehr` path whose captured body decodes back (through the SDK codec, not the write path) as the submitted EHR_STATUS (`archetype_node_id`, `is_queryable`, `is_modifiable`); the response carries a `Location` ending in `/ehr/{ehr_id}` for the id the SDK recovered; the read-back is one `GET` on `/ehr/{ehr_id}/ehr_status` for that id and returns the same fields. The `201` status itself is consumed by the transport (a non-2xx fails the call) and is not surfaced by the leaf, so the probe does not assert the number.
- **Effect:** mutating.
- **Modes:** Sandbox, Live (snapshot); Cassette not yet served by the runner.
- **Status:** Implemented (Sandbox) — a server-assigned create with an initial EHR_STATUS carrying a distinctive archetype and queryable and modifiable flags; the SDK recovers the id, and the `ehr_status` read-back on that id returns the fields. Can-fail plants: a divergent read-back status, a create answered without `Location`, a submitted body that is not the EHR_STATUS, and a read-back issued on another EHR's path. A Live snapshot of the create and the read-back runs too. A scenario recording of them (`ehr-status.har`) is replayed by its own test, outside the runner, so it is not yet this probe's Cassette mode.
- **Satisfies:** REQ-095.

#### PROBE-061 — Composition versioned write with `Prefer: return=representation`

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` returns a bare `COMPOSITION` body plus a new `ETag` (REQ-094).
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Request carries `Prefer: return=representation`; response body decodes as bare `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` schema (oneOf: `Composition` | `Identifier`) — **not** an `ORIGINAL_VERSION<COMPOSITION>` envelope, which lives at `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`). The response `ETag` is captured into `VersionMetadata`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) via PROBE-071 — PROBE-071 checks the bare-body response for the POST and the symmetric PUT, with the strict rejection of an `ORIGINAL_VERSION` body; this entry stays as the named POST probe.
- **Satisfies:** REQ-094 (`return=representation` arm only).

#### PROBE-062 — `openehr-audit-details` header round-trip

- **Title:** A write carrying `openehr-audit-details` is reflected in the resulting Contribution's audit envelope on read-back.
- **Preconditions:** Existing EHR; a known `*rm.AuditDetails` value; the uid of the Contribution the write creates — server-assigned and not returned by a single-resource write, so the Sandbox fake serves it and a Live caller recovers it from the versioned object's revision history.
- **Wire assertion:** Write request carries `openehr-audit-details` in the openEHR dotted-attribute grammar (`change_type.code_string="…",committer.name="…",system_id="…"` — REQ-059, **not** JSON), judged by a parser in the probe that is independent of the SDK encoder: only the seven attributes the upstream contract documents appear, and `change_type.code_string`, `committer.name`, `system_id` (and `description.value` when set) equal the input. The subsequent Contribution GET returns the same audit fields (committer name, change-type, system_id), and its `versions` list names the version uid the write returned (REQ-054), which binds the read-back to the write.
- **Effect:** mutating.
- **Modes:** Sandbox; Cassette, Live not yet scoped.
- **Status:** Implemented (Sandbox) — the probe's parser has its own tests (escapes, duplicates and stray whitespace, which no plant can reach), and the encoder has a dotted-grammar golden. Can-fail plants: a JSON header, a `;` separator, an unquoted value, a rewritten committer, a dropped `system_id`, an undocumented attribute, and read-backs with a divergent system_id, a divergent committer, and a `versions` list that does not name the committed version.
- **Satisfies:** REQ-059.

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

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=minimal` returns an empty body and names the committed version in its `ETag` or `Location` header; a follow-up GET of that version returns the full payload.
- **Preconditions:** Backend honours `Prefer: return=minimal`.
- **Wire assertion:** POST response body is empty; the `ETag` or `Location` names the committed version, read under [REQ-054](wire.md#req-054); SDK surfaces only `*VersionMetadata`. Subsequent GET of that version returns the full Composition.
- **Modes:** Sandbox, Live (snapshot); Cassette not yet served by the runner.
- **Status:** Implemented (Sandbox) — the default `Save` (Prefer `return=minimal`) recovers the version id into the `VersionUID` in the order [REQ-054](wire.md#req-054) states and returns a nil Composition; a follow-up `Get`, which the harness answers only on the path naming that `VersionUID`, returns the full body. Can-fail plants strip the `ETag` and `Location`, answer the read-back with a 404, and serve a read-back with no `archetype_node_id`, each pinning the failure detail it expects. A Live snapshot of the default minimal save and the follow-up GET runs too. A scenario recording of them (`composition-minimal.har`) is replayed by its own test, outside the runner, so it is not yet this probe's Cassette mode.
- **Satisfies:** REQ-094, REQ-054.

#### PROBE-066 — Stored AQL query execution

- **Title:** `GET|POST /query/{qualified_query_name}` returns a typed `ResultSet`.
- **Preconditions:** A stored query registered under a known qualified name.
- **Wire assertion:** Request path matches the qualified-name URL template; response decodes as `query.ResultSet` with `Columns` and `Rows` populated.
- **Modes:** Live (snapshot); Sandbox planned; Cassette not yet served by the runner.
- **Status:** Implemented (Live snapshot) — the stored execution returns a typed `ResultSet` with `Columns` and `Rows` populated against a live CDR; a bound parameter scopes the result to the run's own EHR, so the row read back is this run's, not merely a decodable shape. The Sandbox arm is planned. A scenario recording of the store and the execution (`stored-query.har`) is replayed by its own test, outside the runner, so it is not yet this probe's Cassette mode. The Live witness executes through POST, which REQ-057 permits; a GET arm is planned.
- **Satisfies:** REQ-057.

#### PROBE-067 — Template upload round-trip

- **Title:** `POST /definition/template/adl1.4` with an OPT body succeeds; a subsequent `GET` returns the same OPT bytes.
- **Preconditions:** Backend supports ADL1.4 template upload at the standard path.
- **Wire assertion:** `POST` with an OPT body and `Content-Type: application/xml` returns `201` with a `Location` header; a subsequent `GET` returns the same OPT bytes (modulo backend-side reformatting documented per deployment). The SDK's `Accept` negotiation and 201-response-body handling are the client contract in [wire.md § REQ-144](wire.md#req-144--definition-metadata-decoding) — *Template upload negotiation and response* — not asserted here.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).

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
- **Status:** Deprecated — REQ-097 is retired: Cadasto openEHR services no longer accept `Idempotency-Key`, and the SDK no longer sends it, so the sandbox assertion is removed. The identifier is retained and carries no active requirement.
- **Satisfies:** REQ-097

#### PROBE-070 — Admin `DeleteEHR` round-trip

- **Title:** `DELETE /admin/ehr/{ehr_id}` returns 2xx; a subsequent `GET /ehr/{ehr_id}` returns 404 surfaced as `transport.ErrNotFound`.
- **Preconditions:** Backend exposes the ITS-REST `/admin/*` surface; admin deletion is enabled for the tenant.
- **Wire assertion:** `admin.DeleteEHR` succeeds; `errors.Is(ehr.Get(...), transport.ErrNotFound)` is true after the delete.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — the happy-path delete and the missing-EHR variants are covered by the `openehr/client/admin` unit tests; a standalone probe is deferred.
- **Satisfies:** REQ-099

#### PROBE-071 — Composition POST/PUT response body is bare COMPOSITION

- **Title:** `POST /ehr/{ehr_id}/composition` and `PUT /ehr/{ehr_id}/composition/{vo_uid}` with `Prefer: return=representation` return a bare `COMPOSITION` body — not an `ORIGINAL_VERSION<COMPOSITION>` envelope.
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Response body decodes cleanly as `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` / `200_COMPOSITION_updated` schemas. A server that returns `{"_type":"ORIGINAL_VERSION", ...}` on these paths is non-conformant; the SDK surfaces that as a decode error (strict-against-spec posture per REQ-094). The full version envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — the probe exercises the POST and PUT arms in one invocation when a versioned-object id and an `If-Match` are supplied; otherwise it skips the PUT arm and passes on POST alone. Leaf unit tests cover both verbs and both halves of the strict-against-spec contract, and the same shape for `directory.Save` and `directory.Update` (`201_directory`, `200_FOLDER_retrieved`).
- **Satisfies:** REQ-094 (`return=representation` arm only).

#### PROBE-072 — Contribution submission body matches `Contribution_create`

- **Title:** `POST /ehr/{ehr_id}/contribution` request body is the ITS-REST `Contribution_create` shape — `{audit, versions: [ORIGINAL_VERSION<T>|IMPORTED_VERSION<T> with inline data]}`, bar a deletion built without a payload — not the persisted `rm.Contribution` shape whose `versions[]` is `[]OBJECT_REF`.
- **Preconditions:** Existing EHR; at least one resource payload (`Composition` / `EHRStatus` / `Folder` / `EHRAccess`) to commit.
- **Wire assertion:** Captured request body has `versions[i]._type ∈ {"ORIGINAL_VERSION","IMPORTED_VERSION"}` and carries the resource payload inline under `data`, except an `ORIGINAL_VERSION` with change type `523` built without a payload ([§ REQ-130](wire.md#req-130--contribution-builder) *Deletion*, keyed by [ADR 0024](../adr/0024-contribution-deletion-without-data.md)), which carries no `data` member; a `"data":null` fails on every version. A request body whose `versions[]` contains `{"_type":"OBJECT_REF", ...}` is non-conformant per the ITS-REST OpenAPI `Contribution_create` schema (the persisted `OBJECT_REF` shape returns at read time only). **Commit-audit shape (SPECITS-95 / ITS-REST PR 131):** the batch `audit` and each `versions[i].commit_audit` omit the server-assigned `time_committed` and carry a `DV_CODED_TEXT`-shaped `change_type` (nested `defining_code`), not the erroneous flat `TERMINOLOGY_CODE`. The SDK emits `_type:"AUDIT_DETAILS"` by default (conformant servers accept it); the `UPDATE_AUDIT` form is available as a caller-selected fallback.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — the probe checks both the version shape and the commit-audit shape (`contribution.UpdateAudit` and the write-version wrappers `OriginalVersion` and `ImportedVersion`, which drop the server-assigned `time_committed`). A deletion built without a payload passes ([§ REQ-130](wire.md#req-130--contribution-builder) *Deletion*, keyed by [ADR 0024](../adr/0024-contribution-deletion-without-data.md)): the probe judges it on the `ORIGINAL_VERSION`'s change code `523` and the absence of a `data` key, `"data":null` fails on every version, and eleven planted bodies pin each edge of the exception.
- **Satisfies:** REQ-050, REQ-095.

#### PROBE-073 — Demographic PARTY polymorphic round-trip

- **Title:** A PARTY of each concrete type (PERSON / ORGANISATION / GROUP / AGENT / ROLE) round-trips through create → get → get-version with its `_type` discriminator decoded back into the same concrete type at every hop.
- **Preconditions:** A server (Sandbox: fake) serving the typed PARTY body for `POST|GET /demographic/{type}[/{uid_based_id}]` and the `ORIGINAL_VERSION<PARTY>` envelope for `GET /demographic/versioned_party/{vo_uid}/version`.
- **Wire assertion:** Sandbox — `demographic.Create` (Prefer=representation), `demographic.Get`, and `demographic.GetVersion` each decode the response into the same concrete Go type as the input PARTY (REQ-040). The VERSION hop also exercises the `ORIGINAL_VERSION<PARTY>` envelope, whose generic `data` cannot decode into the abstract `rm.Party` interface and is re-decoded by `_type` through the type registry. A body whose `_type` decodes to a different concrete type is non-conformant.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — the leaf has its own unit tests. The Demographic API is Draft upstream (ITS-REST `x-status: DEVELOPMENT`).
- **Satisfies:** REQ-040, REQ-050.

#### PROBE-078 — verb-aware EHR scoping on query execution

- **Title:** An EHR-scoped ad-hoc AQL execution via `POST /query/aql` carries the scope on the `openehr-ehr-id` request header, per the verb-aware scoping rule in [wire.md § AQL executor](wire.md#aql-executor); every GET query operation — `GET /query/aql` and `GET /query/{qualified_query_name}[/{version}]` — continues to carry the `ehr_id` query parameter.
- **Preconditions:** A stored or ad-hoc query executed with an EHR scope, against a backend that honours the ITS-REST OAS distinction between the two verbs (no `ehr_id` query parameter or body field on the POST operations).
- **Wire assertion:** Captured POST request carries `openehr-ehr-id: <ehr_id>` and no `ehr_id` query parameter or request-body field; the same scope on a GET request carries `ehr_id` as a query parameter and no header. A backend that only honours the header would run a query lacking it population-wide — the assertion catches an SDK regression that scopes POST via the query parameter instead.
- **Modes:** Sandbox; Cassette, Live not yet scoped.
- **Status:** Implemented (Sandbox) — drives all three endpoints (`/query/aql`, `/query/{qualified_query_name}` and `/query/{qualified_query_name}/{version}`) over both verbs, six calls, checking that each request scopes on the verb's channel and carries no other: a POST's query parameter and request body are both checked, a GET's header. Seven can-fail plants move the scope onto a wrong channel per verb (one into the POST body); two of them fire only on the stored and stored-versioned arms, so dropping an arm fails a test. REQ-055's verb-aware scoping also has unit tests. Live is not scoped: EHRbase does not restrict a top-level `FROM EHR e` by the `openehr-ehr-id` header, so request capture (Sandbox) witnesses the header and query-parameter distinction, not a Live result delta.
- **Satisfies:** REQ-055.

#### PROBE-079 — `PutStoredQuery` recovers `{name, version}` from `Location`

- **Title:** A body-less `PUT /definition/query/{qualified_query_name}[/{version}]` store reply recovers the server-assigned `{name, version}` from the `Location` response header, per [wire.md § Stored AQL](wire.md#req-057).
- **Preconditions:** A store operation whose response is the canonical `200_StoredQuery_stored` shape — empty body, `Location` header carrying the qualified name and version.
- **Wire assertion:** `definition.PutStoredQuery` recovers `{name, version}` in the order [wire.md § REQ-057](wire.md#req-057) gives, `Location` first, and a malformed `Location` does not fail the call.
- **Modes:** Live (snapshot); Sandbox planned; Cassette not yet served by the runner.
- **Status:** Implemented (Live snapshot) — against a live CDR: EHRbase answers the body-less store with `200` and `Location: …/definition/query/{name}/{version}`, and the recovered `{name, version}` match the name stored. REQ-057's recovery order also has unit tests. The `stored-query.har` recording replays the store in its own test, outside the runner, so it is not yet this probe's Cassette mode. A Sandbox probe is planned.
- **Satisfies:** REQ-057.

#### PROBE-084 — Built contribution body matches `Contribution_create`

- **Title:** A `Contribution_create` body assembled by the contribution builder reaches the wire with the requested operation's change-type code, a lifecycle state on every version, `preceding_version_uid` exactly where the operation requires it, and none of the server-assigned fields the pin's `UpdateVersion` does not declare.
- **Preconditions:** One payload per operation the batch commits, except the deletion built without one — `COMPOSITION`, `EHR_STATUS` and `FOLDER` on the assertion below, `EHR_ACCESS` being reachable only through the same generic entry points and covered at unit level — plus a preceding-version uid per non-creation; a sandbox server accepting `POST /ehr/{ehr_id}/contribution`.
- **Wire assertion:** The captured request body — built through the builder and committed through `contribution.Commit`, so the assertion covers the real marshal path rather than a hand-marshalled copy — satisfies, per version: `commit_audit.change_type.defining_code.code_string` is the [REQ-130 § Change types](wire.md#req-130--contribution-builder) code for the operation requested (`249` / `250` / `251` / `523`) with `terminology_id.value = "openehr"`; `lifecycle_state` is present and `DV_CODED_TEXT`-shaped, and a deletion's carries the `deleted` code (`523`) unless the caller named another; `preceding_version_uid` is present for an amendment / modification / deletion and **absent** for a creation; `data` carries the payload inline with the `_type` its operation asked for, except on a deletion built without a payload, which carries **no** `data` member (not `null`); `contribution` is never emitted; and `uid` is emitted **only** where the caller supplied one, verbatim, and is absent everywhere else. The batch `audit` carries the caller's `change_type` verbatim — a value the probe deliberately sets to a code that matches no version's, since [REQ-130](wire.md#req-130--contribution-builder) forbids deriving it — and no `time_committed` appears anywhere in the body. **The vendored corpus ([`testkit/corpus/submissions/`](../../testkit/corpus/README.md)) is the shape witness, compared structurally, not byte-for-byte:** its records carry a top-level `_type:"CONTRIBUTION"` envelope that `Contribution_create` omits and RM payloads this SDK does not author, so a byte comparison would assert the fixture rather than the contract. The byte-level pin is a golden of the builder's own output, checked in beside the probe; the corpus arm asserts that every version field a corpus record carries is one the builder also emits, and that the fields it omits are omitted for the reason REQ-130 names.
- **Effect:** mutating — the Sandbox arm issues `POST /ehr/{ehr_id}/contribution` against the fake.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — one batch exercises the whole code table: a creation, an amendment, a modification, a deletion that keeps its payload and a deletion built without one, over `COMPOSITION`, `EHR_STATUS` and `FOLDER` payloads whose `data._type` is checked per version, and whose lifecycle code is checked too (`532`, and `523` on both deletions), under a batch audit declaring `unknown` (`253`). That code is a member of the *audit change type* group ([§ REQ-034](rm-modeling.md#openehr-terminology-vocabulary-req-034)) deliberately outside the four the builder's constructors carry per operation ([§ REQ-130](wire.md#req-130--contribution-builder)), so once the versions between them carry all four, no derivation rule over the versions could reproduce the batch value and the non-derivation arm cannot pass by coincidence. It also puts `WithAudit`, the caller-supplied-audit path, on the wire; `Build` gates it on group membership and the pinned rubric exactly as it gates `WithChangeType`, so no path carries a code outside the group. Both halves of the server-assigned-`uid` rule are checked: four versions carry no `uid` key at all (*absent* rather than *non-empty*, since a builder that synthesised one would satisfy the looser check), and the modification carries exactly the uid its caller named. Sixteen planted bodies, each violating exactly one clause (among them a payload-less deletion that emits `"data":null`, a deletion that has a payload but no `data`, a deletion that carries `532`, and a creation that carries `523`), are each matched against the failure detail it produces, so a case cannot rot into failing for an unrelated reason; the corpus arm is confronted with a version field the SDK cannot emit, and a missing input or empty corpus is an error, never a pass. The byte-level golden is regenerated with `-update`.
- **Satisfies:** REQ-130.

#### PROBE-091 — Path-parameter traversal is refused

- **Title:** A path parameter that is `.` or `..`, is empty, or contains `/`, `\`, or a control character is refused before any HTTP request is issued, on every path-interpolating leaf package.
- **Preconditions:** A transport client whose `*http.Client` records whether any request was issued (httptest or a tripwire RoundTripper).
- **Wire assertion:** For each leaf package that interpolates a path parameter, at least two hostile inputs — one of them the separator-smuggling id (`foo/bar` on a `Route`-set request), the only input that exercises the route-arity rule; the other drawn from a traversal id (`a/../../definition/query/evil`, `..`), a backslash-bearing id, or a control character — each fail with a non-nil error and a captured request count of zero: the call fails closed with no bytes on the wire. A well-formed id that only needs percent-encoding (a space or dots in a template id, `Blood Pressure.v1`) still issues exactly one request whose path is encoded once (REQ-095), and the service root `"/"` — the System API's only operation — passes validation and issues its request (the REQ-150 exemption's positive control). Sentinel identity (`ErrInvalidPathSegment`, `ErrInvalidConfig`) is pinned by `transport/` unit tests, not by this probe.
- **Effect:** read-only — the hostile arms assert that NO request reaches the wire, and the positive control issues a read.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-150.

#### PROBE-092 — Contribution read matches `contribution_get`

- **Title:** `GET /ehr/{ehr_id}/contribution/{contribution_uid}` is issued by the contribution read leaf and the 200 body decodes as the persisted `CONTRIBUTION`.
- **Preconditions:** A known EHR id and contribution uid; a sandbox server returning a canonical-JSON contribution body (or 404).
- **Wire assertion:** The captured request method and path match the ITS-REST template; a 200 decodes to the contribution type (version metadata is **not** asserted — the pin defines only `Content-Type` on `200_CONTRIBUTION`); empty ids issue no request; a 404 fails with a non-nil error. Sentinel identity (`ErrNotFound`, `ErrInvalidConfig`) is pinned by `contribution` unit tests, not by this probe.
- **Effect:** read-only (`GET`, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-174.

#### PROBE-093 — Template list filters reach the wire

- **Title:** `ListTemplates` emits the ITS-REST list query parameters `template_id`, `concept`, `version`, `offset`, and `fetch` when the corresponding options are set, and omits them when unset.
- **Preconditions:** A sandbox Definition server on `GET /definition/template/adl1.4` (the only `TemplateFormat` v1 registers; the pin's ADL 2 list operation shares the same five parameter components).
- **Wire assertion:** Each option appears as the named query key; an explicit `WithOffset(0)` / `WithFetch(0)` is present on the wire; a negative offset or fetch fails with a non-nil error and no request (sentinel identity is pinned by `definition` unit tests, not by this probe); no options yields an empty query. The response still decodes as the existing template-metadata slice.
- **Effect:** read-only (`GET` list, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-143.

#### PROBE-101 — 2xx decode failure is surfaced, not swallowed

- **Title:** A 200 whose body cannot be decoded as the requested representation fails the call rather than returning a silently zero-valued result — on the shared transport decode and on a hand-rolled Definition list decode alike — while a non-2xx on the same route keeps failing as it always did.
- **Preconditions:** A sandbox server that can serve, on demand, a 200 whose JSON body does not match the requested type, a 404, and a list route answering a JSON object where an array is expected.
- **Wire assertion:** Three arms, kept at observable-behaviour level. (a) A read served a 200 with an undecodable body fails with a non-nil error, and the returned resource is never a zero value paired with a nil error — a silent zero-valued success is the exact defect this probe exists to catch — and nothing is handed back beside that error: the returned resource is nil, since a failed decode has nothing to return. (b) The same read served a 404 fails with a non-nil error, likewise with a nil resource beside it. (c) A Definition list leaf served a JSON object where an array is expected fails with a non-nil error, and the slice beside that error carries zero entries — reporting an empty catalog for a body that did not decode would read a failure as success, which [REQ-151](transport.md#req-151--typed-2xx-decode-failure)'s keyed exclusion admits only for an empty *body*. Each arm also issues exactly one request: without that count, a leaf refusing the call before it reached the wire would credit the probe with a failure the server never provoked, and the probe would pass against an SDK that decodes nothing at all. **That count rule is scoped to `Modes: Sandbox`**, where no retry or redirect policy is in play; a later Cassette or Live promotion restates it against that mode's transport policy rather than inheriting it, so the probe keeps failing on decode behaviour and never on transport policy. Error-type identity (`*transport.DecodeError` versus `*transport.WireError`), raw-`Body` recovery, and the value-free `Error()` string are pinned by `transport/` and `openehr/client/definition/` unit tests, not by this probe.
- **Effect:** read-only — every arm issues a read and asserts on how the call fails.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-151.

#### PROBE-102 — System capabilities via `OPTIONS /`

- **Title:** The System API's single operation is `OPTIONS /`; its response decodes into the typed service capabilities with a declared `restapi_specs_version`.
- **Preconditions:** A backend that answers the ITS-REST *Options and Conformance* operation at the service root.
- **Wire assertion:** The request uses `OPTIONS` (not `GET`) against the service root — the client's configured base path, with or without its trailing slash — exactly one request is issued, and the response decodes into `system.ServiceCapabilities` with a non-empty `restapi_specs_version`. The verb is the assertion — a deployment or SDK that issued a `GET` would miss the operation the spec defines.
- **Effect:** read-only.
- **Modes:** Sandbox; Cassette, Live not yet scoped.
- **Status:** Implemented (Sandbox) — can-fail plants a `GET` verb, a path off the service root, and a version-less body.
- **Satisfies:** REQ-095.

#### PROBE-103 — Admin bulk-delete via `DELETE /admin/ehr/all`

- **Title:** The Admin bulk-delete surface is `DELETE /admin/ehr/all`; a subset delete restricts to the named EHRs via the repeatable `ehr_id` query parameter — not a body, and not a path segment per id.
- **Preconditions:** A backend exposing the ITS-REST Admin `/admin/ehr/all` operation (the Admin API is upstream `x-status: DEVELOPMENT`).
- **Wire assertion:** The request uses `DELETE` against the exact service-relative `/admin/ehr/all` path, carries one `ehr_id` query parameter per id supplied, no id appearing as a path segment, and no request body (the OAS `admin_ehr_delete_all` operation declares parameters only). A 2xx (including `202` Accepted and `204` No Content) is success.
- **Effect:** mutating — the Sandbox arm issues the delete against the fake.
- **Modes:** Sandbox; Cassette, Live not yet scoped.
- **Status:** Implemented (Sandbox) — can-fail plants strip the `ehr_id` parameters, append an id as a path segment, and add a request body.
- **Satisfies:** REQ-099.

#### PROBE-104 — Definition example via `GET …/{template_id}/example`

- **Title:** The Definition example endpoint is `GET /definition/template/{format}/{template_id}/example`; its response decodes into a full COMPOSITION for the named template.
- **Preconditions:** A backend exposing the ITS-REST Definition example operation for a loaded template.
- **Wire assertion:** The request uses `GET` against the exact `…/definition/template/{format}/{template_id}/example` route, with the requested format's path segment and the template id in place, and the response decodes into a `*rm.Composition` with a non-empty `archetype_node_id`.
- **Effect:** read-only.
- **Modes:** Sandbox, Live (snapshot); Cassette not yet scoped.
- **Status:** Implemented (Sandbox) — can-fail plants serve a body with no `archetype_node_id` and rewrite the format and template-id segments. A Live snapshot of the example read-back runs too.
- **Satisfies:** REQ-095.

#### PROBE-108 — Composition formats reach the wire

- **Title:** `composition.Save` and `composition.Get` in canonical XML, FLAT and STRUCTURED send the format's media types and the template id, and decode the server's body in the format asked for.
- **Preconditions:** A backend that answers `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` and `GET /ehr/{ehr_id}/composition/{uid_based_id}` in the requested format; a Web Template and a composition that round-trip through FLAT and STRUCTURED.
- **Wire assertion:** For each of canonical XML, FLAT and STRUCTURED, the write carries the format's `Content-Type` and `Accept`, a FLAT or STRUCTURED write carries `openehr-template-id` equal to the Web Template's id, and the representation and the read's `200` body decode into a composition equal on the paths the probe sets to the one written. Under the format query options a FLAT write carries `application/json` with `format=FLAT` and `templateId`, and its read carries `Accept: application/json` with `format=FLAT`. No request carries a `.schema`-suffixed media type.
- **Effect:** mutating (writes one composition per arm in the sandbox and reads it back).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — can-fail plants serve a backend that ignores `Accept`, tamper with the representation or the read-back, drop or change `openehr-template-id`, send wrong media types on a write or a read, drop `format` or `templateId` from a write or `format` from the format-query read, send a body that is not the composition, and send a `.schema`-suffixed media type.
- **Satisfies:** REQ-173.

#### PROBE-109 — EHR versioned-object reads reach the pinned routes

- **Title:** The composition and EHR_STATUS leaves issue the eight `versioned_composition_*` and `versioned_ehr_status_*` reads of the vendored EHR API and decode their bodies, and a `version_at_time` the SDK sends keeps its fraction of a second.
- **Preconditions:** A sandbox server answering each route with a canonical-JSON body: a `VERSIONED_COMPOSITION`, a `VERSIONED_EHR_STATUS`, a `REVISION_HISTORY`, an `ORIGINAL_VERSION` of each payload family, and an `IMPORTED_VERSION` for the refusal row.
- **Wire assertion:** Each captured request's method, path and `Accept: application/json` match the pinned operation; the latest-version read carries no `version_at_time`, and the version-at-time read carries the instant in UTC with every sub-second digit it holds (the pin's example time carried to nanoseconds, `2015-01-20T19:30:22.765432101+01:00`, is sent as `2015-01-20T18:30:22.765432101Z`, so a value cut to milliseconds fails). Each 200 decodes to its RM type, and a version read's `data` to a `COMPOSITION` or an `EHR_STATUS` (a version without `data` passes only when its lifecycle state is `deleted`, code `523`, the RM's logical deletion; any other version without `data` fails the probe, so a leaf that drops the data it was served fails too); an `IMPORTED_VERSION` body fails as a typed decode error, its request held to the same method, path and `Accept`. The same sub-second value reaches the wire from the composition read by a `Ref` with a time, the EHR_STATUS and directory reads at a time, and the `VERSIONED_PARTY` version-at-time read; those four rows judge the request only, not the read's result. Sentinel identity (`ErrInvalidConfig`, `ErrNotFound`, `ErrInvalidShape`) is pinned by the leaf unit tests, not by this probe.
- **Effect:** read-only (`GET`, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — can-fail plants drop the fraction or cut it to milliseconds, send `version_at_time` on a read that takes none, change a read's method, path or `Accept` (the refusal rows' requests included), serve an `ORIGINAL_VERSION` on the refusal row, serve version `data` of the wrong type, and serve a version that is not deleted without `data`; a 404 on each of the four other-leaf rows still passes, since those rows judge the request only.
- **Satisfies:** REQ-174.

### RM model introspection

#### PROBE-094 — RM meta-model introspection equals the pinned BMM

- **Title:** The compiled-in RM class graph — class universe, abstractness, immediate parents, transitive ancestors, concrete-descendant expansion, and per-attribute declaring class — equals an independent reduction of the pinned RM BMM, and every answer it gives is closed over the universe it reports.
- **Preconditions:** The pinned primary RM schema and its includes under [`resources/bmm/`](../../resources/bmm) (REQ-041), reduced in-test through the `openehr/bmm` loader (REQ-045) — deliberately **not** through the generator's own reduction, which would make the comparison tautological and would pass on a generator whose walk is itself wrong.
- **Wire assertion:** In-repo property, not backend-facing, over the rules of [§ REQ-048](bmm-conformance.md#req-048--rm-meta-model-introspection-surface), which this entry does not restate; each arm compares the compiled-in answers with the independent reduction.
  (a) **Universe** — the compiled-in class set equals [§ The class universe](bmm-conformance.md#the-class-universe), and a class on one side only fails in either direction: a missing class is an unanswerable question, an extra one a class name the pinned schemas do not define.
  (b) **Per class** — abstractness equals the BMM `is_abstract` flag verbatim, the six unflagged classes included, and the arm fails if a locally derived verdict is substituted, not only if the flag is misread ([REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources): the BMM wins). Immediate parents equal the BMM `ancestors` list filtered to the universe, in declaration order; transitive ancestors equal the closure of that filtered edge set; concrete descendants equal the non-abstract members of the inverse closure, plus the class itself when it is not abstract.
  (c) **Closure of the class graph** — every class name a class-graph answer returns is in the universe. The converse arm is the load-bearing one: for every class whose BMM ancestors mix an RM class with a filtered one (`DV_ORDERED`, `DV_INTERVAL`, `DV_DATE`, `DV_PROPORTION`, …), the RM ancestor survives, since an edge lost here silently shrinks every descendant expansion above it. Every dropped edge is accounted to one of the universe rule's declared exclusions, named per edge, because an unaccountable dropped ancestor is what an RM class falling out of the universe looks like. The classes left with no in-universe ancestor are pinned in both directions: an unpinned new root fails, and a pinned one that regains a parent fails too. The declaration site is out of this arm's scope; arm (d) checks it against the unfiltered BMM chain.
  (d) **Declaration site and completeness** — for every class and every attribute the flattened tables report on it, the three observable properties of [§ The flattened attribute tables stay flattened](bmm-conformance.md#the-flattened-attribute-tables-stay-flattened) hold, `Interval` being the one out-of-universe site on the pinned RM, pinned by name. The container flag is read from the raw BMM property here, because type and required would mean re-implementing REQ-041's [§ Mapping rules](bmm-conformance.md#mapping-rules) inside the probe; the full triple is compared with the site in the behaviour suite beside it. The shipped attribute-name set per class equals the BMM's effective property set minus the named exemption pins of [§ The attribute tables are complete against the BMM](bmm-conformance.md#the-attribute-tables-are-complete-against-the-bmm), held two-sidedly: an unpinned drop fails, and so does a stale pin whose drop has since been fixed. How the site is derived is an implementation decision, not something a probe can assert.
  (e) **Negative space** — [§ Three answers, never conflated](bmm-conformance.md#three-answers-never-conflated) over the pinned RM. The shapes the pinned RM cannot supply (a dead-end abstract class with a *named* identity, a class whose parent is absent from the data set) are exercised through REQ-048's synthetic-model seam in the behaviour suite beside this probe.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — six suites, one per arm and two for arm (d): (a) in both directions; (b) over all 139 classes; (c) accounting for every dropped edge (all nine are `primitive_types` entries or the one enumeration, none an RM class), with the four filter roots pinned two-sidedly and a fatal guard if no ancestor was filtered at all; (d) over 728 sites, and the attribute-set completeness with its STRAND-13 exemption pin; (e) with fatal guards on the vacuous cases. The reduction reads the pinned schemas through `openehr/bmm` and restates the generator's exclusion list as a literal, so a class silently added to or dropped from either list fails the probe. A behaviour suite covers the same surface, including the synthetic-model seam the pinned RM cannot supply.
- **Satisfies:** REQ-048.

#### PROBE-098 — Absence reasons account for the universe's negative space

- **Title:** For every name the pinned schemas declare and the class universe omits, the compiled-in absence table reports the reason an independent reduction of the pinned BMM derives — in both directions, under the fixed precedence of [§ REQ-048 § Absence reasons](bmm-conformance.md#absence-reasons) — while universe members report *none* and names no schema declares report *undeclared* with no stored entry backing them.
- **Preconditions:** As PROBE-094: the pinned primary RM schema and its includes under [`resources/bmm/`](../../resources/bmm) (REQ-041), reduced in-test through the `openehr/bmm` loader (REQ-045) — never through the generator's own reduction, which would make the comparison tautological — with the generator's exclusion lists restated as literals so a silent change to either side fails the probe.
- **Wire assertion:** In-repo property, not backend-facing, over the rules of [§ REQ-048 § Absence reasons](bmm-conformance.md#absence-reasons), which this entry does not restate.
  (a) **Accounting, both directions** — every declared name the universe omits gets the reduction's reason from the shipped surface, and every stored table entry is a declared-but-omitted name carrying that same reason; an entry on one side only fails either way.
  (b) **No overlap, nothing computed stored** — the stored table contains no universe member and no *undeclared* or *none* entry; every `KnownRMTypes()` member reports *none*.
  (c) **Undeclared** — a name no pinned schema declares reports *undeclared*.
  (d) **Precedence** — at least one name two rules match is pinned by name and reports the reason the fixed precedence ranks first (a primitive-mapped name the named-class exclusion list restates reports *primitive*, not *excluded class*).
  (e) **Consistency with the negative space** — every out-of-universe name this probe exercises reports not-known on every `Lookup`/`Hierarchy` question (PROBE-094 arm (e) holds the converse for universe members).
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline) — five suites, one per arm: (a) walks every declared name, so both directions are answered, with the census pinned per reason (29 primitive, 3 enumeration, 12 excluded class, 30 excluded package), so a rule that quietly stops matching fails on the counts rather than leaving its names unasked-about; (b) checks membership against absence in both directions, then holds the stored key set to a set identity with the reduction's declared-minus-universe set, read through a test-only accessor because the shipped accessor answers membership first and would mask an entry for a universe member; (c) uses names the pinned schemas mention without declaring, synthetic near-misses of a real table key, and the empty string; (d) pins the two-rule witness by name on `Ordered` and both adjacent ranks as name sets, the enumeration rank, which has no witness on the pinned RM, being covered by a generator test on a synthetic enumeration; (e) covers all 74 declared-but-omitted names and both undeclared witness sets, over every `Lookup` / `Hierarchy` question. The reduction never imports the generator, so a name silently added to or dropped from either side fails the probe. Behaviour-level tests cover the zero value, the synthetic seam and the `String()` form, and the generator half has its own.
- **Satisfies:** REQ-048.

### Observability

#### PROBE-050 — OTel span carries openEHR attributes

- **Title:** Every outgoing request opens an OTel span with `openehr.spec_version` and a sanitised URL.
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
- **Status:** Implemented (Sandbox) — covered by the `transport` unit tests.
- **Satisfies:** REQ-090
