---
kind: specification
---

# Conformance probes

**Status:** Draft

The conformance probe catalogue for this SDK, and the requirements that hold the probe harness to the people who use it: the sandbox backend, recording safety and the live-write opt-in (REQ-082), and the `cadasto/admin` health checks (REQ-083).

A **conformance probe** is an executable assertion that the SDK exercises against either:

- the sandbox transport (`sandbox/`),
- a replayed recording (`testkit/recordings/`), or
- a live reference deployment,

to verify wire-level conformance to openEHR REST and SMART-on-openEHR, or an in-repo property over vendored inputs or the SDK's own output that reaches no server.

Each probe has a stable `PROBE-NNN` ID, one catalogue entry here, and a pass/fail outcome.
How probes are written, run and kept is [probe-harness.md](../probe-harness.md).

## Conformance scope

### REQ-082 — Runnability

A backend-facing probe runs in up to three modes: **Sandbox**, against the in-memory `sandbox/` transport; **Cassette**, against a replayed recording; and **Live**, against a reference openEHR deployment.
An **in-repo** probe asserts a property over vendored inputs or over the SDK's own output and reaches no server in any mode.
Each catalogue entry's **Modes** line says which applies: 37 of the 83 catalog entries are in-repo by construction.
Today 8 entries declare all three modes.

How the runner selects a mode, what a probe reports and each mode's working rules are in [probe-harness.md](../probe-harness.md).
This section holds what an SDK consumer, or the operator of a deployment a probe writes to, relies on.

#### Sandbox mode

The `sandbox/` transport is a published building block, not test-only scaffolding, and serves a probe without a network listener or credentials.

- It **MUST** be reachable by an SDK consumer testing their own application, injected as the transport of the HTTP client the consumer hands the SDK.
- Sandbox state **MUST** be per-run and isolated: two probes running against one sandbox instance observe each other's writes only when the probe definition says they share an EHR.

Its non-test source imports neither `auth/` nor `transport/`, and neither of the listener packages `net` and `net/http/httptest`.

#### Cassette mode

A recording is a captured HTTP exchange: method, URL, request and response headers, status and both bodies.
Its encoding is HTTP Archive 1.2 (`.har`), settled by [ADR 0020](../adr/0020-cassette-recording-har.md); provenance and the redaction record sit on `log` as a `_req082` object, since HAR has no native slot for them.

- Credentials (`Authorization`, cookies, tokens, client secrets, JWKS private material) and patient-identifying data **MUST NOT** reach disk.
  The recorder redacts them at capture time, never at review time, and the recording records that redaction ran, so an unredacted capture is detectable rather than merely unlikely.
- The replayer **MUST** refuse a recording that does not record that redaction ran, or that still carries a credential.
- A recording carries its provenance: the deployment it came from, the date, and the SDK commit that captured it.
  The replayer **MUST** refuse a recording whose provenance cannot be stated.
- The replayer **MUST** fail an unmatched request rather than pass it through to a network or synthesise a plausible response: a recording with a gap is recaptured.

#### Live mode

The runner **MUST** refuse to execute a mutating probe in Live mode without an explicit per-invocation opt-in, and a probe whose catalogue entry declares no **Effect** counts as mutating: writing to a deployment is something the operator asked for, never a default a mode flag turned on or a label nobody wrote.

#### The CDR cross-check tool

The CDR cross-check tool (`make cdr-crosscheck`) measures live CDRs and the SDK against each other; it records numbers and asserts nothing, so it is a tool and not a probe ([probe-harness.md § The CDR cross-check tool](../probe-harness.md#the-cdr-cross-check-tool)).
It **MUST NOT** write to a CDR without an explicit per-invocation opt-in, `OPENEHR_LIVE_ALLOW_MUTATING`.

### REQ-083 — `cadasto/admin` health checks

`cadasto/admin` exposes deployment liveness and readiness checks (`Live`, `Ready`, `ReadyBody`) with a wire contract of their own, independent of the rest of the Cadasto platform API.
They are distinct from the ITS-REST Admin client, `openehr/client/admin`.

- **Default paths** `DefaultLivePath = "/health/live"`, `DefaultReadyPath = "/health/ready"`, each overridable per call via `WithLivePath` / `WithReadyPath` (e.g. `/healthz`).
- **Guards.** A path that is empty or does not start with `/` returns `ErrInvalidPath`, and a nil `*transport.Client` an error matching `transport.ErrInvalidConfig`; either way no request is sent.
- **URL derivation.** The check's URL derives from the **origin (scheme + host) of the openEHR REST service entry** — the openEHR REST API path prefix is **NOT** inherited.
- **Public / no auth.** Health endpoints are public: the SDK **MUST NOT** send an `Authorization` header on a health check.
- **Status mapping** (`errors.Is`-compatible): `2xx → nil`.
  Every non-2xx returns a `*admin.StatusError` carrying the check's path and the status code, which unwraps to `transport.ErrUnauthorized` for `401`, `transport.ErrForbidden` for `403`, `transport.ErrNotFound` for `404` and `transport.ErrServerError` for `5xx`, or to `admin.ErrUnexpectedStatus` for any other code (400, 405, 408, 429, …).
- The checks borrow `transport`'s sentinel taxonomy (REQ-093) but **bypass** `transport.Client.Do` — no openEHR error-envelope decoding, no OTel spans, no retries.
  Use `openehr/client/admin` when those concerns matter.
- **Bounded drain.** A check drains the 2xx body `Live` and `Ready` discard, and any body that is not 2xx, so the connection can be reused.
  It **MUST** read at most a fixed number of bytes of that body, for at most a fixed time, before closing it: a body that never ends, streamed fast or trickled, **MUST NOT** hold a check until its context ends.
  A `ReadyBody` 2xx body is read under *The readiness body* below instead.

#### The readiness body

A readiness endpoint often answers with a small document that names the service and its version, and `Ready` discards it.
`ReadyBody` sends the request `Ready` sends, takes the same options, and has `Ready`'s path guard and status mapping; on a 2xx it **MUST** return the response body instead of `nil`.

- **Bounded read.** `ReadyBody` **MUST** cap the bytes it reads from the body.
  The default limit is `DefaultMaxReadyBody` (1 MiB).
  `WithMaxReadyBody(n)` overrides it with the values [§ REQ-093](transport.md#req-093--openehr-error-envelope-mapping) *Bounded response reads* gives `transport.WithMaxResponseBody`: `0` means the default, a positive value sets the limit, a negative value disables it.
  `Ready` reads no body it returns, so the option does not change it.
- **Read failures.** A 2xx body longer than the limit **MUST** fail with an error that `errors.Is` matches to `transport.ErrResponseTooLarge` and whose text names the limit.
  Any other 2xx body that cannot be read **MUST** fail with an error that unwraps to the read failure, such as `io.ErrUnexpectedEOF` for a connection cut mid-body.
  Either failure **MUST** return no bytes: a partial body is not the document.
- **Value-free text.** The text of these read-failure errors **MUST NOT** carry a body byte, a header value or the read failure's own text, which can quote bytes the server sent, as [§ REQ-151](transport.md#req-151--typed-2xx-decode-failure) *`Error()` stays value-free* holds for `transport.BodyReadError`.
  The limit is the caller's own setting, not a payload value.
- **Non-2xx.** A status that is not 2xx returns no bytes, beside the error the status mapping gives `Ready`.

#### Acceptance

- A 2xx readiness response with the body `{"status":"UP","version":"1.2.3"}` makes `ReadyBody` return exactly those bytes; `ReadyBody` with `WithReadyPath("/ehrbase/rest/status")` sends its `GET` to that path at the origin, with no `Authorization` header.
- A 2xx body of exactly `n` bytes under `WithMaxReadyBody(n)` is returned whole; one byte more returns no bytes and an error matching `transport.ErrResponseTooLarge` whose text names `n`.
  Without the option, a body over `DefaultMaxReadyBody` fails the same way.
- A 2xx body cut mid-read returns no bytes and an error matching `io.ErrUnexpectedEOF`.
  None of the bytes read before the cut, and no value the server sends as a malformed chunked trailer, appears in the error text.
- A `503` returns no bytes and an error matching `transport.ErrServerError`; an empty or relative `WithReadyPath` returns `ErrInvalidPath` without sending a request; a nil `*transport.Client` returns an error matching `transport.ErrInvalidConfig`.
- A `429` gives an error that `errors.AsType[*admin.StatusError]` reaches with `StatusCode` 429, and that matches `admin.ErrUnexpectedStatus`.
- A 2xx body that never ends makes `Live` and `Ready` return `nil`, and a `503` body that never ends makes `ReadyBody` return no bytes and an error matching `transport.ErrServerError`, each after reading a bounded number of body bytes.
  When that body (a 2xx for `Live` and `Ready`, a `503` for `ReadyBody`) sends a byte and then stalls, each call returns the same result before its context ends.

### Vendored fixtures (`testkit/corpus/`)

The probes read their fixture documents (bodies, not recorded exchanges) from `testkit/corpus/`; the layout, the ingests and the integrity checks are [testkit/corpus/README.md](../../testkit/corpus/README.md).
The cross-format sets come from Apache-2.0 upstreams only, as [LICENSING.md](../../LICENSING.md) records for each vendored corpus ([licensing.md](../licensing.md)).

## Probe catalog

Each entry says what its probe checks and which requirement it satisfies; the requirement's own section holds the rule.
An entry has:

- **ID** — stable, never renumbered.
- **Title** — one-line description.
- **Preconditions** — the state the system is in.
- **Wire assertion** — what the probe checks at the byte, status or value level.
- **Modes** — Sandbox / Cassette / Live for a backend-facing probe, or In-repo for a probe that reaches no server in any mode ([§ REQ-082](#req-082--runnability)).
  For a backend-facing probe this line is the record of which modes it supports today.
- **Effect** — read-only or mutating ([§ REQ-082 *Live mode*](#live-mode)).
  Absent means *treated as* mutating.
- **Status** — Draft (in this spec), Implemented (in code), Ratified (passes against a reference openEHR deployment), Deferred (the requirement it covers is landed and unit-covered, but this dedicated probe is not written), Deprecated (scheduled removal; possibly unrunnable when its implementation is already gone before 1.0).
  The parenthesis after Implemented names what runs the probe's committed test: (Sandbox) or (Live snapshot) for a backend-facing probe, (inline) for an in-repo probe that a Go test runs directly rather than the runner.
  Which modes a probe supports is for the Modes line alone to say.
- **Satisfies** — the REQ-IDs this probe exercises (inverse of the [REQ registry](REQ.md)).

### Terms

**Wire-equivalent.** Two JSON documents are *wire-equivalent* when parsing each into a generic JSON value (object as a map, array as a slice, plus string, number, boolean and null) yields equal trees.
The comparison is by member name, not by position, so it is insensitive to member order and to the encoder's choice of string escaping.
Array element order **is** compared: it is significant in JSON and in the RM.
Numbers are compared as their literal text, so an integer past 2^53 compares exactly rather than through `float64`.
Wire-equivalence is the only check on encoded JSON a probe in this catalog makes: none checks byte equality of encoded JSON, for the reason [§ REQ-052](wire.md#req-052) gives.

### Authentication and discovery

#### PROBE-001 — Discovery declares `code+pkce`

- **Title:** SMART configuration document declares `code` response type and `S256` PKCE method.
- **Preconditions:** A SMART-on-openEHR deployment is reachable.
- **Wire assertion:** GET `<base URL>/.well-known/smart-configuration` (or equivalent) returns 200 with a JSON body containing `"response_types_supported"` including `"code"` and `"code_challenge_methods_supported"` including `"S256"`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-061.

#### PROBE-002 — Discovery advertises `org.openehr.rest`

- **Title:** Service catalog includes the openEHR REST service with a parseable base URL.
- **Preconditions:** SMART discovery resolved.
- **Wire assertion:** The discovery document's service catalog holds a `services` member keyed `"org.openehr.rest"` with a parseable `baseUrl`.
  An entry without a `spec_version` member passes, as [auth.md § REQ-072](auth.md#req-072) allows.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-070, REQ-072.

#### PROBE-003 — Spec-version mismatch fails fast

- **Title:** A discovery document advertising an incompatible spec version is rejected at resolution, not at first request.
- **Preconditions:** SDK is configured to require `1.1.0`; deployment advertises `1.0.3`.
- **Wire assertion:** Construction-time discovery returns a `DiscoveryError` with reason `spec_version_mismatch`.
  No request to the openEHR REST endpoint is made.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-072.

#### PROBE-004 — PKCE verifier round-trip

- **Title:** A SMART launch using `S256` PKCE successfully exchanges code for token.
- **Preconditions:** Deployment registers the SDK as a SMART app with PKCE required.
- **Wire assertion:** Authorization request carries `code_challenge`, `code_challenge_method=S256` and exactly one `aud` naming the configured audience (REQ-061); token exchange carries `code_verifier`; token response is 200 with an `access_token`.
- **PKCE parity:** the SDK's verifier also satisfies RFC 7636 and `x/oauth2`: ≥ 32 bytes of decoded entropy, `base64.RawURLEncoding` (URL-safe, unpadded), and `code_challenge == base64url(SHA256(verifier))` with method `S256`, cross-checked against `golang.org/x/oauth2.S256ChallengeFromVerifier`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-061, REQ-068.

#### PROBE-005 — Scope round-trip

- **Title:** A configured scope string survives token exchange verbatim and lands in the JWT scope claim or the response `scope` field.
  The SDK passes scopes through whatever their syntax ([REQ-165](auth.md#req-165--openehr-scope-syntax)), so the probe's scope is not an openEHR resource scope.
- **Preconditions:** Scope `patient/COMPOSITION.read` is requested.
- **Wire assertion:** Authorization request `scope` parameter contains `patient/COMPOSITION.read`; token response `scope` field contains it (or the JWT `scope` claim does).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-061, REQ-068.

#### PROBE-008 — Platform principal claims surface verbatim

- **Title:** When the token carries `principal_uid` and `principal_type` claims (per REQ-064), the SDK surfaces them on `LaunchContext.Principal` without coercion.
- **Preconditions:** A token with `principal_uid = "u-123"`, `principal_type = "AGENT"`.
- **Wire assertion:** SDK exposes `LaunchContext.Principal = {UID: "u-123", Type: PrincipalTypeAgent}`.
  Missing claims surface as nil/zero, not as guessed defaults.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-064.

#### PROBE-009 — Caller attribution forwarded on opt-in

- **Title:** When caller attribution is configured (REQ-066), the SDK emits the configured header and OTel attributes; when not configured, no attribution data appears on the wire.
- **Preconditions:** One client with `WithCallerAttribution(...)`, one without.
- **Wire assertion:** Configured client emits the `X-Cadasto-Caller-Attribution` header and `caller.agent_id` OTel attribute; unconfigured client emits neither.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-066.

#### PROBE-006 — JWKS rotation transparent to caller

- **Title:** A signing-key rotation on the authorization server triggers exactly one JWKS refresh in the SDK; subsequent requests succeed without consumer intervention.
- **Preconditions:** A cached JWKS does not contain the `kid` of the issued token (simulating rotation), and is at least `JWKS.MinRefreshInterval` old; the probe sets that interval to zero, so the miss right after the seed counts as one.
- **Wire assertion:** SDK fetches JWKS once, validates the token, and proceeds.
  No double-refresh, no double-validation failure surfaced.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-062.

#### PROBE-007 — Token refresh transparent to caller

- **Title:** An expired access token with a valid refresh token is refreshed silently before the next request.
- **Preconditions:** Cached token has `expires_at < now`; refresh token is valid.
- **Wire assertion:** Token endpoint receives `grant_type=refresh_token`; the next outgoing request carries the new access token.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — both halves: the transport's re-authentication retry ([transport.md § REQ-166 *Re-authentication retry*](transport.md#re-authentication-retry)) and the proactive refresh of an expired access token.
- **Satisfies:** REQ-063, REQ-166

#### PROBE-106 — Sign-out revokes at the advertised revocation endpoint

- **Title:** `Source.Revoke` sends the source's refresh token to the advertised `revocation_endpoint` (RFC 7009) and leaves the source signed out.
- **Preconditions:** The SMART configuration advertises `revocation_endpoint`; the source holds an access token and a refresh token from a code exchange.
- **Wire assertion:** The revocation endpoint receives exactly one form-encoded `POST` carrying `token=<refresh token>` and `token_type_hint=refresh_token`, authenticated as the token endpoint authenticates the same client (a public client sends `client_id`); a `200` answer makes `Revoke` return nil, and afterwards the source holds no token and its next `Token` call fails with `auth.ErrReauthRequired` without a token-endpoint request.
- **Effect:** mutating (the revoked grant).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-063.

#### Launch-mode coverage

The three launch modes of [REQ-068](auth.md#req-068--flow-and-launch-mode-coverage) are exercised by coverage tests beside the auth probes.
They are coverage proofs rather than catalogued `PROBE-NNN` IDs, but they are part of the auth suite's definition of done:

- **Standalone** — `Source.AuthorizeURL` builds an authorization URL with `response_type=code` and the `S256` PKCE challenge, and **no** `launch` parameter.
- **Embedded (EHR launch)** — an EHR-supplied `launch` parameter is forwarded verbatim to the authorization endpoint.
- **Backend service** — three confidential backend flows produce the expected token request on the wire: `auth/clientcreds` with `WithClientAssertion` (`client_credentials` and a signed `client_assertion`, no Basic, no `client_secret`: SMART Backend Services), and, outside the SMART asymmetric profile, `auth/clientcreds` with a symmetric `client_secret` (HTTP Basic) and `auth/jwtbearer` (the RFC 7523 §2.1 JWT Bearer authorization grant, not a SMART flow).

With the PKCE public flow (PROBE-004), confidential-client PKCE, and the `auth/smart` unit tests of the confidential-code auth-method selection, this exercises every flow in REQ-068's table across all three launch modes.

##### Inferno SMART App Launch (STU2.2) Client-suite cross-check

The HL7 Inferno **SMART App Launch Test Kit STU2.2 _Client_** suite serves as an external checklist (it cannot be executed here).
Its client scenarios map to SDK coverage as follows:

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
- **Wire assertion:** The SDK refuses an empty `ifMatch` at the call site with `transport.ErrInvalidConfig`, so no request leaves the process.
  The probe sends nothing, so it does not observe how a backend answers a missing `If-Match`; that status mapping is [REQ-093](transport.md#req-093--openehr-error-envelope-mapping)'s.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-054, REQ-093

#### PROBE-011 — PUT Composition with stale If-Match

- **Title:** A PUT with a stale `If-Match` (referencing an old version_uid) is rejected with `412 Precondition Failed`.
- **Preconditions:** Composition has been updated since the SDK's cached `version_uid`.
- **Wire assertion:** PUT returns `412`, and the SDK maps it to `ErrPreconditionFailed`.
  The vendored ITS-REST pin answers a stale `If-Match` on a PUT with `412` and lists no `409` for it, so a `409` fails the probe.
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
- **Wire assertion:** The typed builders cannot emit syntactically invalid AQL (structural guarantee).
  Execution of a query referencing a non-existent path returns the backend's AQL error envelope; the SDK maps it to `*query.AQLError`, which satisfies `errors.Is(err, aql.ErrPathResolution)`.
  A **501** response maps to `*query.AQLError` satisfying `errors.Is(err, aql.ErrEngineCapability)` and never `aql.ErrPathResolution`: the capability gap is read from the status alone, with an envelope or bare, never from message text (REQ-055).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — against synthesised backend responses: a path-resolution envelope, and a 501 capability gap with an envelope and bare.
- **Satisfies:** REQ-057.

#### PROBE-028 — AQL lint stability

- **Title:** Linting fixed AQL strings against the SDK grammar profile (and, for Layer 3, a compiled OPT) yields a stable issue-code multiset.
- **Preconditions:** A compiled OPT (`vital_signs.opt`) and fixture query strings under `testkit/corpus/aql/lint/` (`valid.aql`, `missing_archetype.aql`, `bad_syntax.aql`).
- **Wire assertion:** In-repo property: `lint.LintString(q, &lint.Options{Compiled: c})` over each fixture produces exactly the expected `lint.Issue.Code` multiset: `valid.aql` → `aql_select_no_alias`; `missing_archetype.aql` → `aql_archetype_not_in_template` and `aql_select_no_alias`; `bad_syntax.aql` → `aql_syntax`.
  The two `aql_select_no_alias` entries come from the REQ-161 path-shape checks that [PROBE-099](#probe-099--aql-path-shape-lint-corpus) holds (both queries project a column with no `AS` alias); one baseline table serves this probe, PROBE-099 and [PROBE-097](#probe-097--aql-semantic-and-portability-lint-corpus) arm (b), so the corpus is re-baselined once.
  The cross-implementation claim covers REQ-109's own codes: another implementation of REQ-109 over the same grammar profile and template reports the same `aql_archetype_not_in_template` and `aql_syntax`.
  A code a later additive Layer-2 group contributes is held by that group's probe, so this multiset grows when such a group rightly flags a fixture, and that growth is a re-baseline recorded there, not a stability breach.
  Detail text and path strings are not checked.
- **Modes:** In-repo (issue-code multiset over a fixture corpus; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-109.

#### PROBE-080 — AQL parse/emit round-trip property

- **Title:** Parsing a catalogue AQL string into `parse.Query` and re-emitting via `(*Query).Emit` is stable — canonical inputs are preserved, re-emission is idempotent, and out-of-catalogue shapes surface `aql.ErrIncompleteAST` at both parse and emit rather than round-tripping silently.
- **Preconditions:** The AQL catalogue corpus: idempotence cases, canonical-input preservation cases, and the residual cases of [§ REQ-113 § Catalogue and residual](aql.md#catalogue-and-residual).
- **Wire assertion:** In-repo property over every catalogue query: the fixed point of [§ REQ-119 § Round-trip property](aql.md#round-trip-property), canonical inputs preserved, and the residual refused by both `ParseQuery` and `Emit` with `aql.ErrIncompleteAST`, never emitted partially.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113.

#### PROBE-082 — AQL structured path access (standing predicate + WHERE path)

- **Title:** A class standing predicate is readable as a structured `{path, operator, value}` (`ClassExpr.PredicateComparison`), and the alias-qualified path of a WHERE comparison, `EXISTS`, `LIKE` and `MATCHES` as alias + segments (`ParsedPath`), without importing `parse/gen` or re-tokenizing raw text (REQ-113).
- **Preconditions:** A parsed query carrying a standing class predicate (`EHR e[ehr_id/value=$ehr]`) and a WHERE comparison over an alias path (`o/data[at0001]/events[at0006]/value/magnitude > $threshold`).
  Queries pairing a comparison with `EXISTS`, `LIKE` and each `MATCHES` operand form over the same path, one with an escaped node name (`c/content[at0001, 'O\'Brien']/name/value`), also spelled with whitespace between its parts.
- **Wire assertion:** In-repo property: the accessors of [§ REQ-113 § Structured path access](aql.md#structured-path-access) on these inputs.
  `From.Root.PredicateComparison` gives `{ehr_id/value, =, $ehr}` for the standing comparison and is nil for an archetype-HRID predicate; the WHERE comparison's `ParsedPath` gives alias `o` and the ordered segments, with `ParsedPath.Raw == Comparison.Path`; and both survive a parse, emit and parse round trip, with `Emit` idempotent.
  The `ParsedPath` of `EXISTS`, `LIKE` and `MATCHES` equals the comparison's, with `Raw == Path`, the node name resolved to `O'Brien` and the whitespace between path parts absent from the segments; the write-side constructors leave it nil, and `Emit` gives the same text when it is nil or wrong.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113.

#### PROBE-087 — AQL structured-AST catalogue completeness

- **Title:** Every shape in the list of [§ REQ-113 § Catalogue and residual](aql.md#catalogue-and-residual), the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause and a projected literal's and function call's source text included, parses into `parse.Query` without `aql.ErrIncompleteAST` and round-trips through `(*Query).Emit` under the PROBE-080 fixed point.
- **Preconditions:** The catalogue corpus: queries for each catalogue shape, the former gap shapes among them, and the `TOP`-clause and projection-source-text cases.
- **Wire assertion:** In-repo property: for each catalogue shape, `ParseQuery` returns an AST pinned per shape and `Emit` is a fixed point.
  The residual of [§ REQ-113 § Catalogue and residual](aql.md#catalogue-and-residual) still fires.
  A `TOP` clause carries its count and direction ([§ The `TOP` carrier](aql.md#the-top-carrier)), and a projected literal or function call carries its source text where that text differs from the canonical rendering ([§ Projection source text](aql.md#projection-source-text)).
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113.

#### PROBE-088 — AQL builder containment and paging stability

- **Title:** The builder's negated containment (`NOT CONTAINS`), sibling containment junctions (`AND`/`OR`, parenthesised by precedence), opt-in in-text `LIMIT`/`OFFSET`, the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause, and the write-side carriers of [REQ-055](aql.md#req-055--aql-builder) emit byte-stable canonical AQL, and a builder program using none of them emits what it emitted before they existed.
  The write-side carriers are the `VERSION` predicate bracket, the class-position standing comparison, and the typed projection: `DISTINCT`, `AS`, aggregates, function calls, literals, star.
- **Preconditions:** Committed goldens covering the constructs: one per `versionPredicate` alternative; one per standing-predicate operand kind the write side can express (`pathPredicateOperand`'s `primitive`, `objectPath` and `PARAMETER` alternatives; its `ID_CODE` and `AT_CODE` alternatives have no `aql.Value` shape, so the builder cannot reach them); REQ-161's suppression shape; and one per `columnExpr` alternative plus the clause-level flags; beside the unchanged PROBE-020 golden.
- **Wire assertion:** In-repo golden comparison (no backend): built query strings match the committed goldens byte for byte, and each golden re-parses and re-emits unchanged, which ties the write-side canonical form to PROBE-087.
  Each build-time refusal of [§ REQ-055](aql.md#req-055--aql-builder) (a second row-bound channel, a `TOP` beside a row limit or with a negative count, a `SELECT` that does not read back as the projection the builder recorded, a typed construction the grammar does not admit where it renders) has a row that expects an error and no emission.
  The check is over the class of refusal rather than a fixed count of rows, so a shape added to either class extends it without amending this entry.
- **Modes:** In-repo (golden comparison of the SDK's own output; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-055.

#### PROBE-090 — AQL emission round-trip closure

- **Title:** Every value a supported write path emits — string, real, and integer literals, a `MATCHES {uri}` operand, a value-position function call, and a containment tree — re-parses into an equal value, and the write-side guards that refuse an unspellable value agree with the vendored grammar rather than with a hand-written expectation.
- **Preconditions:** The escape, numeric, URI and function-name corpora; the write-side unit corpus; the predicate, identifier and archetype-id corpora; and the vendored lexer at `resources/aql/grammar/active/AqlLexer.g4`, which the grammar-confrontation tests read directly.
- **Wire assertion:** In-repo property: the checks [§ REQ-119](aql.md#req-119--re-parseable-canonical-aql-emission) *Acceptance* names, over the corpora above; the rules are REQ-119's and this entry does not restate them.
  Every value kind survives emit, parse and compare under `aql.EqualValues` in every value position and on both carriers, or the guards refuse it; a literal read from source text survives twice (parse, emit, parse).
  The single-token positions are held to "the parser reads the string back unchanged" at every position of their class.
  Round-trip identity is the oracle at the bracket positions: a generated corpus is spliced at both positions into bases, at least one carrying material after the bracket and a later line break, and every base shows itself able to fail by some splice re-parsing as a different query.
  The `VERSION` shape rule has its own corpus of operands legal by construction.
  The grammar-derived guards are confronted with the grammar: for every token name in the lexer, and the keyword spellings carried only in lexer fragments, the builder emits a function of that name exactly when the parser accepts it.
  For every URI in the corpus the builder emits the operand exactly when it re-parses as one `MATCHES` on the same URI.
  A source sweep finds no type assertion or switch on a sealed vocabulary outside a function that normalises.
  Every guard has a test that fails when it is removed, and a guard that could be tightened into refusing valid AQL has a positive control that fails when it is.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-119.

#### PROBE-095 — AQL predicate structuring

- **Title:** Every bracketed path predicate the vendored grammar admits at a path step, after a class or on an alias root arrives as a typed, trivia-free, delimiter-free structured value beside its untouched verbatim text — or as a member of the enumerated unstructured set, nil beside the verbatim text.
- **Preconditions:** The vendored grammar at `resources/aql/grammar/active/AqlParser.g4`, from which the corpus is generated rather than hand-listed, so no grammar alternative goes uncovered unnoticed ([§ REQ-119](aql.md#req-119--re-parseable-canonical-aql-emission) *Acceptance*).
- **Wire assertion:** In-repo property over the rules of [§ REQ-113 § Structured node predicates](aql.md#structured-node-predicates), which this entry does not restate; it carries the oracle's construction.
  The corpus is generated from the grammar's `pathPredicate` alternatives, down to the `pathPredicateOperand` production whose members decide structured against enumerated-unstructured, and the sweep fails when the grammar admits a form that neither a kind nor the enumerated set covers, which a control shows it able to do.
  Every form is presented at both positions it can occupy, bracket top level and junction operand, and yields the same kind at each; each enumerated unstructured form gives nil beside populated verbatim text at both.
  Every form is presented at the three brackets the grammar gives the production, a path step, a class and an alias root, and yields the path step's structure at each, both extractors agreeing.
  Trivia and escape independence is a property driven from the same corpus: a mechanically padded spelling gives components equal to the bare spelling's while its verbatim text differs, both arms firing, with the interior-trivia spellings padding cannot generate (inside an operand path, a name slot, regex braces) as named rows beside it.
  Emission is checked unaffected at every bracket: the REQ-119 round-trip suites stay green and the emitted text is byte-identical with the model cleared.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113.

#### PROBE-096 — AQL value-free structured diagnostics

- **Title:** Every construct the extractor drops carries a structured `{kind, clause, span}` record, every lint issue carries a span, no field the contract classifies value-free contains any substring of the input's value spans, and each span points at the construct the diagnostic is about.
- **Preconditions:** A corpus reaching every **reachable** drop class (today the numeric literal outside the value vocabulary, the residual of [§ REQ-113 § Catalogue and residual](aql.md#catalogue-and-residual), in each clause that admits one).
  A representative lint-issue set spanning the layers `lint` runs (parameter-binding cases run with `Options.Query` set, since their codes fire on no other path).
  For each corpus query, the **value spans** of its input, identified independently of the diagnostic under test.
- **Wire assertion:** In-repo property in three arms, over the two-class rule of [§ REQ-168 § The two field classes](clinical-modeling.md#the-two-field-classes) as [§ REQ-113 § Value-free structured drop records](aql.md#value-free-structured-drop-records) and [§ REQ-109 § Value-free lint diagnostics](aql.md#value-free-lint-diagnostics), which this entry does not restate; it carries each arm's oracle.
  **(a) Record per drop:** a sweep derived from the source of every non-test file in the package holds completeness, so a site added later, in any file, is covered the day it lands, and a control shows the sweep able to fail.
  The defensive arms' unreachability by a legal query is checked at unit level, since no corpus can reach them.
  One query carrying drops in two clauses exercises the source-order rule.
  **(b) Value-free fields:** for every corpus query, no value-free field of either surface, rendered whole with its span so a future string field cannot slip past, contains a substring of that input's value spans, checked against the input and never against expected strings.
  The value-bearing fields are deliberately not checked, and each case pins the code it exists to exercise, so it cannot pass on bystander issues.
  Additivity is checked over the corpus: codes, severities and `OK()` outcomes match their earlier values.
  **(c) Span attribution:** each span, sliced back out of the input, matches the offending construct rather than its enclosing clause, in one span type across both packages (a compile-time alias check), and an unattributable span is zero rather than approximated.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-113, REQ-109.

#### PROBE-097 — AQL semantic and portability lint corpus

- **Title:** Every REQ-161 issue code fires on a corpus query built to carry exactly that defect, with the REQ-161 severity and a span on the offending construct.
  Queries with no REQ-161 defect yield no REQ-161 code.
  The builder's verification agrees with the linter on the containment codes ([§ REQ-161 § Builder adapter](aql.md#builder-adapter); the five containment codes, not the lint-only portability codes).
- **Preconditions:** The vendored grammar profile (parse) and the pinned BMM via `rminfo` — no CDR, no OPT.
  The [REQ-160 acceptance table](aql.md#req-160--aql-containment-admissibility-relation) is the relation oracle; the EHRbase admissibility matrix is a neutral observed-behaviour cross-check, including the EHRbase-compatibility guard (an RM-valid pair a conformant engine admits never verdicts Never).
- **Wire assertion:** In-repo property, three arms.
  **(a) Per-code corpus:** for each REQ-161 code, at least one corpus query yields it with the specified severity at the span of the offending construct, and at least one near-miss query stays silent.
  The unknown-class suppression rule (an unknown operand suppresses the pair checks), the archetype-mismatch suppression (a literal archetype predicate whose declared class or HRID type segment is unknown yields `aql_unknown_rm_class` only, never `aql_archetype_class_mismatch`) and the `aql_fanout_row_grain` conservative firing rule are each pinned by their own negative case.
  **(b) Additivity guard:** every query of the pre-REQ-161 corpus that carried no REQ-161 defect yields an unchanged issue-code multiset; corpus queries that genuinely carry a newly-checked defect are re-baselined deliberately, recorded here.
  **(c) Read/write parity** ([§ REQ-161 § Builder adapter](aql.md#builder-adapter)): for corpus queries the builder can express, the verification's code multiset equals the containment-code subset of `lint.LintString` over the emitted text.
  The class-position `VERSION` predicate, the class-position standing comparison and the typed projection are builder constructs, so the corpus carries them through the same comparison; what stays unspellable is the FROM root's own bracket in every spelling (archetype, standing or version), since the write-side FROM clause carries an RM type and an alias and nothing else.
- **Modes:** In-repo (unit-level; no backend).
- **Status:** Implemented (inline) — PROBE-028's three fixtures gain none of the REQ-161 codes this probe pins, so arm (b) needed no re-baseline.
- **Satisfies:** REQ-160, REQ-161, REQ-055.

#### PROBE-099 — AQL path-shape lint corpus

- **Title:** Every path-shape issue code of REQ-161 fires on a corpus query built to carry exactly that defect, with Warning severity and a span on the offending segment, projection item, path or class expression; every firing rule's negative near-miss stays silent; and no query free of path-shape defects changes its issue-code multiset.
- **Preconditions:** The vendored grammar profile (parse) and the pinned BMM via `rminfo` — no CDR, no OPT.
  The [REQ-161 path-shape catalogue](aql.md#path-shape-checks) is the oracle for each code's firing and silence rules; the [REQ-160 relation](aql.md#req-160--aql-containment-admissibility-relation) is the oracle for the redundant-step rows.
- **Wire assertion:** In-repo property in two arms, PROBE-097's structure without the read/write-parity arm, since these codes have no builder analogue.
  (a) **Per-code corpus:** the firing witnesses and the near-miss rows that [§ REQ-161 *Path-shape acceptance*](aql.md#path-shape-acceptance) lists, each code at Warning severity with its span on the offending construct; the conservative walk's silent stops are pinned by name, and a `Result` carrying only this group's findings reports `OK()` true.
  (b) **Additivity guard:** every corpus query that carries no path-shape defect yields an unchanged issue-code multiset, and [PROBE-028](#probe-028--aql-lint-stability)'s three fixture queries keep theirs.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-161.

#### PROBE-100 — Upstream AQL admissibility corpus ratchet

- **Title:** Every FROM/CONTAINS variant a vendored FROM-family combination corpus recorded as engine-accepted parsed under the SDK grammar profile and raised no Error-severity containment code.
- **Modes:** In-repo (no longer run).
- **Status:** Deprecated — superseded by [PROBE-107](#probe-107--upstream-aql-suite-admissibility-corpus), which holds every answered query to no Error-severity lint issue at all; the identifier is retained and not reused.
- **Satisfies:** none (formerly REQ-160).

#### PROBE-107 — Upstream AQL suite admissibility corpus

- **Title:** Every query the vendored upstream AQL suites send and a conformant engine answers with rows parses under the SDK grammar profile and draws no Error-severity lint code, unless a reviewed decline names it and why — so the REQ-109 grammar profile and the REQ-160 compatibility guard are held to every clause family an engine answers.
- **Preconditions:** The vendored suite corpus under `testkit/corpus/aql/suites/`, with its provenance pin `AQL_SOURCE.txt` and the ingest-generated `EXCLUDED.txt` beside it ([probe-harness.md § Vendored fixtures](../probe-harness.md#vendored-fixtures)); the reader takes each query template from the suite that sends it and transcribes none.
  The vendored grammar profile and the pinned BMM; no CDR, no OPT.
- **Wire assertion:** In-repo property.
  - **Rebuild.** The reader takes each test's query text from the vendored suites alone: a data-driven suite's single query template filled from every row of the CSV the suite names, or a test case's own query.
    It replaces a run-time variable that sits inside a quoted literal with one fixed literal, because the probe checks admissibility, not results.
    A test it cannot rebuild is counted under a named reason, never dropped.
  - **Classify.** Each rebuilt query is classed by what its test asserts, not by what the engine returned: **answered** (the test reads the returned rows), **answered-empty** (the test asserts that no row is returned), **refused** (the test expects an error response), **not-ready** (an active `not-ready` tag on the test or its suite) or **skipped** (a skipped suite or test).
  - **Answered.** For every answered query, `parse.ParseQuery` succeeds and `lint.LintString(q, nil)` raises no Error-severity issue, unless a decline in the reader's decline table covers the query.
    A decline names the Error code, the construct it fires on and one of two reasons: an engine grammar extension outside foundation AQL ([REQ-109 § Syntax floor](aql.md#syntax-floor--the-sdk-grammar-profile)), or a `CONTAINS` operand an engine binds by an engine extension ([REQ-160 § Containable operands](aql.md#containable-operands)).
  - **Declines exposed.** Each answered query the exported reader returns carries the declines that cover it (`SuiteQuery.Declines`), each with its Error code, construct and reason, and none where no decline does, so a consumer that gates queries through the same lint can tell a reviewed decline from a defect without copying the decline table.
  - **Answered-empty.** Every answered-empty query parses, and any Error-severity issue it raises is one of [REQ-161](aql.md#req-161--aql-semantic-and-portability-lint)'s containment Errors: an engine that accepts a shape the RM rules out and answers it with no rows is consistent with a Never verdict (REQ-160 § Overlay edges).
  - **Not asserted.** Refused, not-ready and skipped queries are counted and not checked: the SDK stays the looser of the two, and the corpus is not a rejection catalogue.
  - **Ratchet.** The running test checks the tally per class and per family, and the number of queries each decline covers, so a corpus refresh that moves any query re-baselines the test and the pin together.
    A decline that covers no query fails, and so does a family directory that contributes no answered query.
- **Modes:** In-repo (ratchet property over a vendored corpus; no backend).
- **Status:** Implemented (inline) — the one decline, `aql_contains_not_containable` on the `CONTAINS` operand `EVENT_CONTEXT`, rests on [REQ-160 § Containable operands](aql.md#containable-operands).
- **Satisfies:** REQ-109, REQ-160.

### Clinical modeling

#### PROBE-022 — OPT path resolution

- **Title:** Reading an ADL 1.4 operational template (OPT) into the operational template model, compiling it, and resolving a fixture-defined list of openEHR paths on the compiled template returns nodes whose RM type, archetype node id, and (for archetype roots) archetype id match the expected values; explicitly unknown attributes and unmatched predicates produce `ErrPathNotFound`.
- **Preconditions:** A reference OPT body (XML bytes) and an assertion list mapping paths to expected node identity.
- **Wire assertion:** In-repo property: `aom14.ReadOperationalTemplate`, `templatecompile.Compile`, `template.ParsePath` and `Compiled.Resolve` against the fixture body match every assertion in the list, and each negative assertion (`ExpectNotFound`) surfaces `ErrPathNotFound`, wrapped.
- **Modes:** In-repo (path-resolution property against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-111.

#### PROBE-023 — Composition builder round-trip

- **Title:** Building a composition via `composition.NewBuilder` + `Set` → `Build` → `canjson.Marshal` → `canjson.Unmarshal` → re-marshal preserves the values supplied through `Set` at their addressed paths.
- **Preconditions:** A compiled OPT and a list of (path, value) assignments addressed against it.
- **Wire assertion:** In-repo property:
  - `composition.NewBuilder(ctx, c, opts...)` with a `Set` per path and `Build` succeeds;
  - `canjson.Marshal` of the result contains the assigned primitive values (magnitude and units for DV_QUANTITY, the value string for DV_TEXT, code and terminology for DV_CODED_TEXT) as byte fragments;
  - `canjson.Unmarshal` into a fresh `*rm.Composition` succeeds, which shows the polymorphic dispatch on the nested data values works in both directions;
  - re-marshalling the decoded composition preserves the same fragments.
- **Modes:** In-repo (round-trip property over a compiled template; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-101.

#### PROBE-024 — Primitive constraint validate

- **Title:** Reading an OPT, compiling it and resolving a fixture-defined list of leaf paths on the compiled template, calling `PrimitiveConstraint.Validate` with a supplied Go value, returns the expected multiset of `ViolationCode` values per case.
- **Preconditions:** A reference OPT body (XML bytes) carrying at least one primitive-constraint child (C_BOOLEAN, C_INTEGER, C_REAL, C_STRING, C_DATE, C_TIME, C_DATE_TIME, C_DURATION, C_CODE_PHRASE, C_DV_QUANTITY, C_DV_ORDINAL) and a case list with positive (no violations) and negative (specific code expectations) entries.
- **Wire assertion:** In-repo property: `aom14.ReadOperationalTemplate`, `templatecompile.Compile`, `Compiled.Resolve` and `(*CompiledNode).PrimitiveConstraint().Validate(value)` match every case's `WantCodes` multiset, and a case marked `ExpectNoConstraint` addresses a node whose `PrimitiveConstraint()` returns nil.
- **Modes:** In-repo (issue-code multiset against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-103

#### PROBE-025 — Composition validate

- **Title:** Reading an OPT, compiling it, and running `ValidateComposition(comp, c)` over a fixture-defined list of (OPT, composition, expected codes) tuples returns the expected multiset of `validation.Issue.Code` values per case.
- **Preconditions:** A reference OPT body (XML bytes) and a hand-built or fixture-decoded `*rm.Composition`; each case carries a `WantCodes []string` that captures the multiset semantics (order irrelevant, duplicates count).
- **Wire assertion:** In-repo property: `aom14.ReadOperationalTemplate`, `templatecompile.Compile` and `validation.ValidateComposition` produce an `Issue.Code` multiset that matches each case's `WantCodes`.
  Positive cases expect an empty `WantCodes`; primitive and structural mismatches expect specific codes (`primitive_out_of_range`, `primitive_unit_unknown`, `primitive_not_in_list`, `slot_fill`, …).
- **Modes:** In-repo (issue-code multiset against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-102, REQ-103

#### PROBE-026 — Missing required nodes / cardinality

- **Title:** Sharpens PROBE-025 with negative structural cases — missing required nodes, empty multi-valued attributes with `existence ≥ 1` and `cardinality ≥ 1`, occurrences upper-bound violations, RM-type mismatches under C_SINGLE_ATTRIBUTE alternatives — and asserts the issue-code multiset (`required`, `cardinality`, `rm_type_mismatch`, `alternative_mismatch`) is stable across conformant implementations.
- **Preconditions:** The same OPT and composition tuple shape as PROBE-025; the cases are structural ones the template-driven validator detects.
- **Wire assertion:** In-repo property, with the pipeline of PROBE-025.
  A composition with the systolic ELEMENT removed, its ITEM_LIST's `items` present and empty, surfaces no issue, since the template leaves that ELEMENT optional and those `items` carry cardinality `0..*`, so their existence asks for no member, while the same `items` left out surface `required` ([§ REQ-102](clinical-modeling.md#req-102--composition-validation) *An existence lower bound alone asks for no member*).
  An empty `events` list surfaces `required` + `cardinality`, and an absent one `required` alone.
  An unmatched alternative surfaces `alternative_mismatch`.
- **Modes:** In-repo (issue-code multiset against a fixture OPT; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-102

#### PROBE-027 — Generated instance validates clean

- **Title:** `instance.Generate(c, opts)` followed by `validation.ValidateComposition(out, c)` and `validation.ValidateRM(out)` each return `Result.OK = true` under every policy (`Large`, `Medium` and `Required`) and both `ValueFill` values, on an OPT the generator can synthesise, outside the rows of its corpus census.
- **Preconditions:** Compiled OPT for a fixture template; valid composer + territory for COMPOSITION roots.
- **Wire assertion:** Cross-package round-trip — generator and validator agree on the same template-driven contract.
  On the compiling vendored corpus the same generated value also passes `validation.ValidateRM`, the output rule of [§ REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), outside the cases its *Exceptions and known gaps* lists.
- **Modes:** In-repo (generator and validator agreement over fixture OPTs; no backend).
- **Status:** Implemented (inline) — a corpus census beside the probe runs every vendored OPT through the generator, the composition builder and both validators, and pins each remaining failure and each hollow body with its reason.
- **Satisfies:** REQ-107.

#### PROBE-074 — Template-driven validation of non-COMPOSITION roots

- **Title:** `validation.Validate(root, c)` over a fixture-defined list of (OPT, RM root, expected codes) tuples returns the expected `validation.Issue.Code` multiset for archetypeable roots outside the COMPOSITION content set — the demographic PARTY hierarchy (`PERSON`/`ORGANISATION`/`GROUP`/`AGENT`/`ROLE` + sub-components) and the EHR-IM roots (`FOLDER`/`EHR_STATUS`).
- **Preconditions:** An OPT body rooted at the target RM type (real `Address.v2.opt` / `TestPerson.v2.opt`, or a synthetic root) and an in-memory or fixture-decoded RM root; each case carries a `WantCodes []string` multiset.
- **Wire assertion:** In-repo property: `aom14.ReadOperationalTemplate`, `templatecompile.Compile` and `validation.Validate` produce an `Issue.Code` multiset matching each case's `WantCodes`.
  A conformant ADDRESS instance validates clean; a `PERSON` under an `ORGANISATION` OPT surfaces `rm_type_mismatch`; a `PERSON` whose OPT-pinned `identities` list is empty surfaces `required` + `cardinality`, and one without the list `required` alone; a `FOLDER` whose archetype id differs surfaces `archetype_id_mismatch`.
- **Modes:** In-repo (issue-code multiset against fixture OPTs; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-102, REQ-103.

#### PROBE-075 — WebTemplate structural parity

- **Title:** `webtemplate.Marshal(templatecompile.Compile(opt))` structurally matches a vendored EHRbase `openEHR_SDK` v2.3 reference WebTemplate JSON for the same OPT — the `id` set, `rmType`, `aqlPath`, `min`/`max`, and per-node input `suffix`/`type` — against a documented-deviations list.
- **Preconditions:** Commit-pinned EHRbase `openEHR_SDK` reference WebTemplate JSON and OPT pairs, vendored under `testkit/corpus/` with provenance and Apache-2.0 attribution.
  Four fixtures:
  - `constrain_test`, which pins no template-level node name, so its golden carries no name predicate;
  - the two [node-naming](clinical-modeling.md#node-names-and-name-predicates) oracles, `Corona_Anamnese` (four `SECTION.adhoc.v1` siblings reusing one archetype) and `GECCO_Diagnose` (whose golden name-predicates its `aqlPath`s);
  - `test_all_types`, whose OPT is the cross-format set's, compared on two subtrees: its ACTION, the only one any golden holds, and its EVALUATION's `choice` ELEMENT, whose value has two alternatives.

  Residual input, `min` and `rmType` deltas are documented and count-pinned in [`deviations.md`](../../openehr/template/webtemplate/deviations.md); `test_all_types`'s are also pinned by name.
- **Wire assertion:** Not backend-facing: an in-repo parity property.
  `aom14.ReadOperationalTemplate`, `templatecompile.Compile` and `webtemplate.Build`/`Marshal` reproduce the reference node structure (id, rmType, aqlPath, min, max, and input suffix and type per node); every accepted difference (field ordering, absent optional fields, localized-string packaging, known id edge cases) is on the documented-deviations list, and any difference not listed is a failure.
  The compare first maps the reference's duplicate sibling ids (`test_all_types2`) to the export's spelling (`test_all_types_1`), and pins each rename per golden.
- **Modes:** In-repo (parity property against a vendored fixture; no backend).
- **Status:** Implemented (inline) — exact on the three whole references, with `test_all_types`'s deltas pinned by name.
- **Satisfies:** REQ-106; REQ-111 (the two name-pinning oracles are the acceptance fixtures of its *Node names and name predicates*).

#### PROBE-077 — RM-floor invariant matrix

- **Title:** For every invariant the inventory of [clinical-modeling.md § REQ-171](clinical-modeling.md#req-171--validation-engine) *Where the RM rules come from* classifies as evaluated, and every registered class it makes subject to that invariant, `ValidateRM` reports the invariant on a breaking value and does not report it on a conforming one.
- **Preconditions:** The generated invariant inventory and its classification, and, per (invariant, class) pair, a breaking and a conforming value written as canonical-JSON members and decoded through the type registry.
- **Wire assertion:** In-repo property.
  For each pair, `ValidateRM` on the breaking value reports an issue whose `Rule` is the invariant, or, for an invariant another check decides, that check's issue at the pair's path; on the conforming value it reports neither.
  A newly evaluated invariant, or a class newly subject to one, without a pair fails the probe, and so does a pair for an invariant the inventory does not classify as evaluated.
- **Modes:** In-repo (unit-level property over the inventory; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-112, REQ-171.

#### PROBE-081 — EHR_STATUS value-typed mandatory presence (subject and ARCHETYPED)

- **Title:** `validation.ValidateJSON(data, new(rm.EHRStatus), nil)`, and `ValidateRMEHRStatusBytes(data)` as its deprecated alias, decide value-typed mandatory presence from the document: they flag an omitted RM-mandatory `subject` (typed `rm.PartySelf`, a value struct) without false-positiving on a valid bare `PARTY_SELF`, and the same for an ARCHETYPED's `archetype_id` and `rm_version`, at the root and below it.
- **Preconditions:** Canonical-JSON EHR_STATUS bodies.
  Subject arm: one omitting the `subject` key; one with `subject: null`; one supplying `subject` as a bare `{"_type":"PARTY_SELF"}` (no external_ref); one omitting the interface-typed `name`.
  ARCHETYPED arm, each on the root `archetype_details`: one omitting, and one nulling, `archetype_id`; one omitting, and one nulling, `rm_version`; one with an empty `rm_version` (`""`); and a complete root `archetype_details` with an attribute-less ARCHETYPED nested under `other_details`.
  Shape: a non-object body, and one whose root `archetype_details` is not an object.
- **Wire assertion:** In-repo property: each body draws exactly the findings [§ REQ-112 § Value-typed mandatory presence](clinical-modeling.md#value-typed-mandatory-presence) gives it:
  - the subject arm's `required` at `/subject`, absent for the bare `PARTY_SELF`;
  - the ARCHETYPED arm's one finding per missing member, `required` at the member's own path and nothing below it, at the root and at the nested ARCHETYPED alike, and the value walk's findings alone for a present key (an empty `rm_version` draws `rm_version_valid`);
  - a single `invalid_shape` at `/` for each shape body.

  The interface-typed `name`, when absent, still draws `required` at `/name`.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-112, REQ-175.

#### PROBE-110 — Missing mandatory members at every level, in both canonical formats

- **Title:** `canjson.WithMissing` and `canxml.WithMissing` report every RM-mandatory member a document leaves out, at any depth, by the same JSON Pointer in both formats, as [wire.md § REQ-175](wire.md#req-175--missing-members-on-decode) defines.
  `canxml.WithPresence` reports every member a canonical-XML document sends as `xsi:nil` or empty, mandatory or optional, by the same pointer with the form it took.
  `validation.ValidateJSON` and `ValidateXML` report each missing member as `required` at its own issue path, and `WithMissing`, given the report's text, and `WithMissingPointers`, given its pointer values, hand a caller's own decode report to the value-based entries with the same result.
- **Preconditions:** One COMPOSITION instance in canonical JSON and in its canonical-XML form, with members left out at the root (`language`), in an ENTRY (`subject`), in a HISTORY (`origin`), and in data values of each value-typed kind: a `DV_QUANTITY` without `magnitude`, a `DV_COUNT` without `magnitude`, a `DV_BOOLEAN` without `value`, a `DV_PROPORTION` without `denominator`, a `DV_ORDINAL` without `symbol`, an `ELEMENT` whose ARCHETYPED lacks `rm_version`.
  Rows sending a member as `null` (JSON), with `xsi:nil="true"` and as an empty primitive element (XML).
  Control rows with a present bare `PARTY_SELF`, a `magnitude` of 0, an empty `units` (canonical JSON only: an empty string element is missing in canonical XML), an absent interval flag, an absent EVENT `offset` and an `archetype_node_id` written as an XML attribute.
  In canonical XML only, rows sending an optional member empty and nilled (a `DV_QUANTITY` `precision`, an ELEMENT `null_flavour`).
- **Wire assertion:** In-repo property.
  Each codec's report option receives, once each, exactly the pointers of the members left out, null, nil or empty, and none for a control row; the pointers the canonical-XML form reports equal the canonical-JSON form's; the default decode of each row returns the same value with or without the option, and strict refuses none of them.
  `canxml.WithPresence` receives, once each, the pointer and form of every nil and empty row, mandatory and optional, and nothing for an absent or control row, while `WithMissing` on the same rows is unchanged.
  `ValidateJSON` and `ValidateXML` on each row report `required` at the issue path of each missing member and no other finding at that path, and `validation.New().Validate(root, nil, WithMissing(pointers))` with the codec's own pointers reports the same multiset of issues.
  `WithMissingPointers(pointers)` with the pointer values either codec's report option passes reports that multiset too.
  A row of each kind is also a can-fail control: with the pointer list withheld, the number and boolean rows validate clean, which is the blind spot the probe pins closed.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-175, REQ-112.

#### PROBE-111 — Operational template corpus round trip

- **Title:** Every ADL 1.4 OPT the corpus vendors decodes into the generated operational template types with no unmatched content, and encodes back to a document that validates against the pinned AM Release-1.4 schemas and holds the content of the input, as [clinical-modeling.md § REQ-176](clinical-modeling.md#req-176--adl-14-operational-template-model) defines.
- **Preconditions:** Every `.opt` file under `testkit/corpus/`, found by walking the tree rather than from `fixtures.ListAllOPTs`, which leaves out those under `crossformat/`; and the pinned ITS-XML closure under `resources/its-xml/` with `Template.xsd` among its roots.
  An OPT the pinned schemas refuse as written is listed in a ratchet with its reason and goes through the round-trip leg only, until a re-pin admits it.
- **Wire assertion:** In-repo property, two legs.
  Round trip: `canxml.Unmarshal` with the report option returns no unmatched item, and with `WithStrict` refuses nothing.
  `canxml.Marshal` of the value and a second decode give an equal value.
  The re-encoded document, read as elements, attributes and text, equals the input as [§ REQ-176 *Round trip*](clinical-modeling.md#round-trip) compares them, after the rewrites its *Shapes* section names: `match_negated` removed, a `timezone_validity` or `OPERATOR_KIND` word written as its integer code, an `ORDINAL` or `SCALE` `symbol` whose `value` is empty written without it, and `T_ARCHETYPE_ROOT` written as `C_ARCHETYPE_ROOT`.
  Schema: the re-encoded document of every OPT not in the ratchet validates against the pinned schemas, their `targetNamespace` read as `http://schemas.openehr.org/v1`, through `make operational-template-schema-check`, which `make ci` runs: it writes the re-encoded documents to a temporary folder and runs `xmllint` against `Template.xsd`.
  A can-fail control: with one operational template member withheld from the decoder, the round-trip leg fails on a corpus file that carries it.
- **Modes:** In-repo (property across the vendored corpus; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-176.

### Simplified formats and upstream parity

#### PROBE-076 — FLAT / STRUCTURED composition round-trip

- **Title:** The simplified codecs round-trip a vendored upstream (OPT + canonical COMPOSITION) pair without losing the data the formats carry, and when the source composition is OPT-valid, a `WithTemplate` decode of the emitted FLAT validates against the OPT (the conformance leg).
  For every COMPOSITION OPT the corpus vendors, under both `instance.Required` and `instance.Medium`, the skeleton `composition.NewSkeleton` writes survives a FLAT and a STRUCTURED round trip still valid against its OPT (the skeleton leg).
- **Preconditions:** Round-trip and conformance legs: vendored EHRbase (Apache-2.0) `Test_dv_*` datatype OPTs and matching canonical composition JSON under `testkit/corpus/` (provenance in [`THIRD_PARTY_LICENSES.md`](../../testkit/corpus/THIRD_PARTY_LICENSES.md)).
  Skeleton leg: every OPT `fixtures.ListAllOPTs` lists whose root is COMPOSITION, one per template id, the skeleton written with a composer, a territory and a fixed clock.
  Both: the WebTemplate is built with `templatecompile.Compile` and `webtemplate.Build` (REQ-106).
- **Wire assertion:** Not backend-facing: an in-repo property across the datatype corpus (DV_TEXT/CODED_TEXT/DATE_TIME/QUANTITY/COUNT/BOOLEAN/ORDINAL/PROPORTION/DURATION/IDENTIFIER/URI + the `|raw`-carried MULTIMEDIA/PARSABLE/INTERVAL).
  FLAT is idempotent (`MarshalFlat → UnmarshalFlat → MarshalFlat`), STRUCTURED re-encodes to the same FLAT, and OPT-free interconversion preserves it.
  The OPT-validation leg catches dropped or mistyped leaves that a symmetric omission would hide from idempotence alone.
  **Scope limit:** symmetric omission of *optional* data can still pass, and the probe does **not** compare emitted FLAT/STRUCTURED against vendored upstream simplified output, which is [PROBE-086](#probe-086--upstream-flat-serialisation-parity)'s check; see [`deviations.md`](../../openehr/serialize/simplified/deviations.md) § Conformance.
  In these two legs a template the WebTemplate builder cannot yet model is a documented skip, never a fail.

  The skeleton leg first requires the written skeleton to validate (`validation.ValidateComposition`); a skeleton that does not fails the pair, named, and no round trip is judged from it.
  It then runs `MarshalFlat → UnmarshalFlat` and `MarshalStructured → UnmarshalStructured`, both decoding with `WithTemplate(c)` and `WithStrict()`, and requires `validation.ValidateComposition` of each decoded composition to report nothing.
  Every encode error, decode error and issue fails the pair, named by template, policy, format, and the error text or the issue code and path; so does an OPT that does not compile, a WebTemplate that does not build, and a skeleton the generator refuses.
  A finding passes only when an exclusion matches it exactly: template id, policy, format, stage, and the error text or the issue code and path.
  An exclusion that no longer fires fails as stale, and so does one naming a template the corpus no longer holds.
  A floor on the number of (template, policy) pairs the corpus gives fails a corpus that silently empties.
  The exclusions, each in FLAT and in STRUCTURED, are `Probe076SkeletonExclusions`, which names the reason for each; each is registered in [`deviations.md`](../../openehr/serialize/simplified/deviations.md#conformance) § Conformance.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-053.

#### PROBE-086 — Upstream FLAT serialisation parity

- **Title:** For each body in the pinned upstream EHRbase FLAT conformance corpus, decoding it and re-encoding it through the SDK's REQ-053 codec reproduces the upstream FLAT — same path set, same leaf values — over the subset of the corpus that the codec models, with everything outside that subset counted rather than waived.
- **Preconditions:** The commit-pinned corpus under `testkit/corpus/flat-conformance/`: the `conformance_ehrbase.de.v0` OPT plus the `ehrbase_conformance_*.json` FLAT bodies from `composition/flat/simSDT/conformance/`, integrity-checked against `MANIFEST.txt` by `make flat-check` (Apache-2.0; provenance in [`THIRD_PARTY_LICENSES.md`](../../testkit/corpus/THIRD_PARTY_LICENSES.md)).
- **Wire assertion:** Not backend-facing: an in-repo parity property.
  `aom14.ReadOperationalTemplate` over the OPT file, `templatecompile.Compile` and `webtemplate.Build`, then per fixture `simplified.UnmarshalFlat(upstream, wt, WithTemplate(c), WithUnmatched(report))` and `simplified.MarshalFlat`.
  The comparison is scoped to the **modelled subset**: the upstream keys left after (a) removing what the codec reports as unmatched content ([§ REQ-170](wire.md#req-170--unmatched-content-on-decode)) or refuses on decode, and (b) holding a **named allow-list** of composition-level metadata out on *both* sides.
  The upstream keys left are compared in the export's spelling of a duplicate sibling id: the reference keys the second of two siblings that share an id `<id>2` where the export writes `<id>_1` ([§ REQ-106](clinical-modeling.md#req-106--webtemplate-json-export) *`id` generation*).
  The harness respells only the upstream side, only where the Web Template holds the duplicate group, and pins each fixture's respelled keys.

  The allow-list holds out **respellings** only: `language`, `territory`, `composer|name`, `composer_self`, `context/start_time` and `context/setting`, which REQ-053 reads and writes as the `ctx/` short forms where upstream writes real paths.
  The composer's `external_ref` suffixes (`composer|id`, `|id_scheme`, `|id_namespace`) and its party sub-structure (`composer/_identifier:N`, `composer/relationship`) are not respellings: decode reads them and encode writes them at the same real path ([ADR 0029](../adr/0029-composer-real-path-carrier.md)), so the census compares them.
  The harness derives its expected hold-out from the codec's exported alias accessors (`simplified.MetadataAliasSpellings`, `MetadataWitnessSpellings`) and checks both directions, so an alias accepted on decode but missing from the hold-out, and a hold-out with no alias behind it, both fail the harness's own tests.
  The hold-out carries **no waiver**, and a new waiver is a spec decision, not a harness edit.
  Why each spelling is held out, and why the match is suffix-aware, is in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md) § How the exclusion list is produced.

  Within the modelled subset the re-encode reproduces the upstream path set with equal leaf values: a dropped, invented or altered key is a failure, and there is no tolerated-drop list.
  The one key decode reads and encode does not write, a DV_PROPORTION's bare value ([wire.md § REQ-053](wire.md#req-053) *Leaf datatypes*), is compared with the `|numerator` divided by the `|denominator` the re-encode wrote, to the codec's tolerance, so a value that disagrees is a failure too.
  The excluded set is derived from what the codec itself reports as unmatched content and from its own decode refusals, never kept by hand, and each refusal removes only what it names.
  The harness package's own tests pin the excluded and compared counts per fixture, so the unmodelled surface can shrink deliberately but never grow unnoticed.
  A fixture that compares **nothing** fails: exact agreement over an empty compared set is vacuous.
- **Modes:** In-repo (parity property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — the measured census is in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md), regenerated with the harness's `-census` flag.
- **Satisfies:** REQ-053, REQ-106, REQ-170.

#### PROBE-089 — Underscore-attribute round-trip

- **Title:** Every [REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes) underscore-attribute family round-trips FLAT → canonical RM → FLAT as a [wire-equivalent](#terms) document, and an RM composition populated with an in-scope attribute emits it without loss — per family, including the recursive shapes.
- **Preconditions:** The PROBE-086 corpus, plus SDK-authored per-family fixtures against the vendored OPTs covering each row of the REQ-140 grammar table — including recursion (`_feeder_audit/…/provider/_identifier:N`, `dv_multimedia/_thumbnail`), `_null_flavour` beside an absent bare value, and the deliberate refusals.
- **Wire assertion:** In-repo, not backend-facing.
  Per grammar family:
  (a) **decode**: a FLAT body carrying the family's keys decodes into the typed RM attribute (never a `|raw` detour), and re-encoding reproduces the input as a [wire-equivalent](#terms) document.
  (b) **encode**: a composition populated with the attribute emits exactly the family's key set: no silent drop, and no `|raw` fallback where the grammar carries the value.
  (c) **refusals**: the keys of a deliberate exclusion fail with typed errors naming the key, and are never decoded and dropped.
  (d) **STRUCTURED**: the same fixtures interconvert FLAT ↔ STRUCTURED preserving the underscore vocabulary (`_`-keys as members, arrays for `:N`).
  The PROBE-086 census moves in step: the `path not in web template` excluded family shrinks by the underscore keys a family carries, and the per-fixture compared and excluded pins are re-baselined in the same change, so a family whose corpus keys stay excluded fails this probe rather than becoming a census footnote.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — the census movement is recorded in [`SKIPPED.md`](../../testkit/conformance/webtemplate/SKIPPED.md).
- **Satisfies:** REQ-140, REQ-053.

#### PROBE-105 — Upstream cross-format parity

- **Title:** For each vendored upstream set that gives one composition in two or more of canonical JSON, canonical XML, FLAT and STRUCTURED, the SDK codecs carry the composition across every leg below whose two formats the set gives and agree with the upstream sibling over the subset they model.
  Every place they do not agree is a recorded outcome with its reason, never a silent waiver.
- **Preconditions:** The sets under `testkit/corpus/crossformat/` ([probe-harness.md § Vendored fixtures](../probe-harness.md#vendored-fixtures)), each with the OPT of its template, from which the probe builds the Web Template (`templatecompile.Compile` + `webtemplate.Build`).
- **Wire assertion:** In-repo, not backend-facing.
  Per set, every leg whose two formats the set carries:
  - (a) **JSON and XML:** the canonical JSON and the canonical XML each decode, and the two decoded compositions, re-encoded through `canjson`, are compared leaf by leaf: a comparison of decoded values, never of bytes ([§ REQ-052](wire.md#req-052)).
  - (b) **canonical to FLAT:** `simplified.MarshalFlat` over the decoded upstream canonical document is compared key by key with the upstream FLAT.
    That document is the set's canonical JSON when the set has one, and its canonical XML otherwise.
  - (c) **FLAT to canonical:** `simplified.UnmarshalFlat` with `WithTemplate` over the upstream FLAT is compared leaf by leaf, in canonical JSON, with the decoded upstream canonical document.
    That document is the set's canonical JSON when the set has one, and its canonical XML otherwise.
  - (d) **FLAT to STRUCTURED:** `simplified.FlatToStructured` over the upstream FLAT, which needs no template, is compared leaf by leaf with the upstream STRUCTURED.
  - (e) **STRUCTURED to FLAT:** the upstream STRUCTURED, decoded and re-encoded as FLAT, is compared with the upstream FLAT decoded and re-encoded the same way.

  Every FLAT comparison and every FLAT and STRUCTURED decode uses the [PROBE-086](#probe-086--upstream-flat-serialisation-parity) hold-out, respelling and exclusion, so an allow-listed `ctx/` short form and its upstream real path are not reported as a difference, and one refusal does not hide the rest of the document.
  Each leg's outcome is either a refusal (the codec's error) or the counts compared, missing, extra, altered and excluded, and it equals the outcome recorded for that set and leg.
  A leg whose outcome changes fails until its record changes in the same commit, so a gap opens or closes only deliberately.
  A recorded refusal, difference or non-zero excluded count states why it exists, and a leg whose recorded outcome is not a refusal compares at least one key or leaf, since agreement over an empty set is vacuous.
  The harness regenerates [`CENSUS.md`](../../testkit/conformance/crossformat/CENSUS.md), which publishes the recorded outcomes, their reasons and what each count means.
- **Modes:** In-repo (parity property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — the counts live only in the generated census, which a test holds to the committed copy.
- **Satisfies:** REQ-052, REQ-053, REQ-056, REQ-169, REQ-170.

### Canonical JSON and formats

#### PROBE-030 — Canonical-JSON round trip

- **Title:** Decoding a canonical-JSON RM value and re-encoding it, then decoding and encoding that output again, recovers the same value and a document wire-equivalent to the first, and the recovered value satisfies the reference-model floor.
- **Preconditions:** A canonical-JSON RM root fixture from the top-level files of `testkit/corpus/compositions/` and `testkit/corpus/rm/` ([probe-harness.md § Vendored fixtures](../probe-harness.md#vendored-fixtures)): a COMPOSITION, or another root sample such as an EHR_STATUS or a FOLDER.
  The probe's leaf values, a DV_QUANTITY and a DV_CODED_TEXT, are inline in the probe.
- **Wire assertion:** Decode, Encode (`b1`), Decode (`A`), Encode (`b2`), Decode (`B`), then the comparison [§ REQ-052](wire.md#req-052) gives a round trip:
  - `A` and `B` equal by typed deep comparison (`reflect.DeepEqual` over the decoded RM values, which compares an interface-typed field by its dynamic type and value, so it covers every substitutable slot and every `DV_INTERVAL[T]` bound with no comparison options);
  - `B` clean under `validation.ValidateRM` ([REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor));
  - `b1` and `b2` [wire-equivalent](#terms);
  - `b1` wire-equivalent to the input's expected form, so a member the decoder reads with the wrong meaning, a wrongly defaulted interval flag among them, fails the probe.

  No byte equality is checked anywhere.
  An input whose vendored content carries an RM-floor finding unrelated to the round trip is held out of the `ValidateRM` leg only, named in the probe with the finding, while the fidelity legs still run on it.
- **Modes:** In-repo (round-trip property against vendored fixtures; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-052, REQ-040, REQ-169, REQ-170

#### PROBE-031 — `_type` discriminator decoded via registry

- **Title:** A `_type` not in the type registry fails with `typereg.ErrUnknownType`, not silently decoding to `map[string]any`.
- **Preconditions:** A fixture containing an unregistered `_type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown `_type` value.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-040, REQ-052

#### PROBE-038 — RM polymorphic decode coverage

- **Title:** `canjson.Unmarshal[Composition]` decodes every BMM-admissible `_type` discriminator at every substitutable slot — including (a) substitutable subtypes in concrete-typed slots (e.g. `LOCATABLE.name` carrying `DV_CODED_TEXT`) per openEHR RM Liskov substitution, and (b) generic types with abstract type parameters (e.g. `DV_INTERVAL[T: DV_ORDERED]`).
- **Preconditions:** Vendored RM fixtures under `testkit/corpus/rm/polymorphic/` covering both failure modes.
- **Wire assertion:** Decode succeeds; the recovered value preserves every original `_type` discriminator that sits on a member the RM defines, counted as a multiset, so a lost substitution fails while an elided duplicate does not, and a `_type` inside a member the decoder ignores ([§ REQ-052](wire.md#req-052)) is not counted; re-marshalling produces a document [wire-equivalent](#terms) to the same logical content.
  No member order is asserted.
- **Modes:** In-repo (decode property against vendored fixtures; no backend).
- **Status:** Implemented (inline) — canonical JSON only.
- **Satisfies:** REQ-040, REQ-052

#### PROBE-033 — Canonical-XML round trip

- **Title:** Decoding a canonical-XML RM value and re-encoding it, then decoding and encoding that output again, produces two byte-identical compact-XML SDK encodes.
- **Preconditions:** A canonical-XML RM value fixture from the top-level files of `testkit/corpus/compositions/` and `testkit/corpus/rm/` ([probe-harness.md § Vendored fixtures](../probe-harness.md#vendored-fixtures)): a COMPOSITION, or an RM root or leaf sample such as a FOLDER or a DV_QUANTITY.
  The probe also builds a DV_QUANTITY, a DV_TEXT and a COMPOSITION inline.
- **Wire assertion:** Decode → Encode → Decode → Encode; the two compact-XML SDK encodes are byte-identical.
  Equality with the input fixture is not checked: the fixture's whitespace and serialisation choices differ from the SDK's compact form ([§ REQ-056](wire.md#req-056)), so the stability checked is that of the SDK's own output.
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

- **Modes:** In-repo (never run).
- **Status:** Deprecated — never implemented, since [PROBE-086](#probe-086--upstream-flat-serialisation-parity) decodes upstream FLAT and re-encodes it, checking this round trip at least as strictly; the identifier is retained and not reused.

### Service discovery

#### PROBE-040 — Catalog cache honours TTL

- **Title:** Two SDK constructions within the TTL window of a cached catalog do not produce a second discovery fetch.
- **Preconditions:** Catalog with declared TTL > 0; two constructions in quick succession.
- **Wire assertion:** Exactly one discovery fetch occurs.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-071

#### PROBE-041 — Catalog refresh on 401

- **Title:** A `401` from a previously-working endpoint triggers exactly one discovery refresh and one retry; failure to recover surfaces a typed error.
- **Preconditions:** Cached catalog; backend rotates and returns `401` on the cached token.
- **Wire assertion:** SDK refreshes JWKS/catalog once, retries once.
  On second `401`, returns `transport.ErrUnauthorized`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — the discovery-layer half only: one `Refresh` fetch against a 401 upstream, failing with a typed `*discovery.DiscoveryError(fetch_failed)`.
- **Satisfies:** REQ-071 (discovery half), REQ-072

### REST binding

The REST-binding probes check the openEHR REST 1.1.0 wire contract above `transport/` and the typed leaf clients under `openehr/client/`.

#### PROBE-060 — EHR creation round-trip

- **Title:** `POST /ehr` with an initial `EHR_STATUS` body answers with a `Location` naming the assigned `ehr_id`, the SDK surfaces that id, and a follow-up `GET` of the EHR_STATUS returns the status that was committed.
- **Preconditions:** Backend supports server-assigned `ehr_id`.
- **Wire assertion:** The create is one `POST` on the exact service-relative `/ehr` path whose captured body decodes back (through the SDK codec, not the write path) as the submitted EHR_STATUS (`archetype_node_id`, `is_queryable`, `is_modifiable`).
  The response carries a `Location` ending in `/ehr/{ehr_id}` for the id the SDK recovered.
  The read-back is one `GET` on `/ehr/{ehr_id}/ehr_status` for that id and returns the same fields.
  The `201` status itself is consumed by the transport (a non-2xx fails the call) and is not surfaced by the leaf, so the probe does not assert the number.
- **Effect:** mutating.
- **Modes:** Sandbox, Live (snapshot).
- **Status:** Implemented (Sandbox) — a Live snapshot of the create and the read-back runs too.
- **Satisfies:** REQ-050.

#### PROBE-061 — Composition versioned write with `Prefer: return=representation`

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` returns a bare `COMPOSITION` body plus a new `ETag` (REQ-094).
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Request carries `Prefer: return=representation`; response body decodes as bare `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` schema (oneOf: `Composition` | `Identifier`) — **not** an `ORIGINAL_VERSION<COMPOSITION>` envelope, which lives at `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`).
  The response `ETag` is captured into `VersionMetadata`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) via PROBE-071 — PROBE-071 checks the bare-body response of this POST.
- **Satisfies:** REQ-094 (`return=representation` arm only).

#### PROBE-062 — `openehr-audit-details` header round-trip

- **Title:** A write carrying `openehr-audit-details` is reflected in the resulting Contribution's audit envelope on read-back.
- **Preconditions:** Existing EHR; a known `*rm.AuditDetails` value; the uid of the Contribution the write creates — server-assigned and not returned by a single-resource write, so the Sandbox fake serves it and a Live caller recovers it from the versioned object's revision history.
- **Wire assertion:** Write request carries `openehr-audit-details` in the openEHR dotted-attribute grammar (`change_type.code_string="…",committer.name="…",system_id="…"` — REQ-059, **not** JSON), judged by a parser in the probe that is independent of the SDK encoder: only the seven attributes the upstream contract documents appear, and `change_type.code_string`, `committer.name`, `system_id` (and `description.value` when set) equal the input.
  The subsequent Contribution GET returns the same audit fields (committer name, change-type, system_id), and its `versions` list names the version uid the write returned (REQ-054), which binds the read-back to the write.
- **Effect:** mutating.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-059.

#### PROBE-063 — Discovery-routed request

- **Title:** The transport resolves its base URL from `ServiceCatalog`'s `org.openehr.rest` entry, not from a hard-coded value.
- **Preconditions:** Catalog whose `org.openehr.rest` service has `baseUrl` `https://override.example/openehr/v1`.
- **Wire assertion:** A request made via the leaf client targets `https://override.example/openehr/v1/...`, not the SDK default.
- **Modes:** Sandbox.
- **Status:** Deferred — the `transport` unit tests cover a request URL built from the catalog's `org.openehr.rest` base.
- **Satisfies:** REQ-070.

#### PROBE-064 — Per-request `auth.TokenSource` overrides client default

- **Title:** A `TokenSource` attached to `ctx` via `auth.WithTokenSource` overrides the client-default `TokenSource` for the duration of one request.
- **Preconditions:** Client constructed with `TokenSource` A; request issued with `ctx` carrying `TokenSource` B.
- **Wire assertion:** Outgoing `Authorization` header carries the bearer from B; subsequent requests without the ctx-override fall back to A.
- **Modes:** Sandbox.
- **Status:** Deferred — the `transport` unit tests cover the override and the fallback.
- **Satisfies:** REQ-060.

#### PROBE-065 — `Prefer: return=minimal` on POST returns identifier only

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=minimal` returns an empty body and names the committed version in its `ETag` or `Location` header; a follow-up GET of that version returns the full payload.
- **Preconditions:** Backend honours `Prefer: return=minimal`.
- **Wire assertion:** POST response body is empty; the `ETag` or `Location` names the committed version, read under [REQ-054](wire.md#req-054); SDK surfaces only `*VersionMetadata`.
  Subsequent GET of that version returns the full Composition.
- **Modes:** Sandbox, Live (snapshot).
- **Status:** Implemented (Sandbox) — a Live snapshot of the default minimal save and the follow-up GET runs too.
- **Satisfies:** REQ-094, REQ-054.

#### PROBE-066 — Stored AQL query execution

- **Title:** `GET|POST /query/{qualified_query_name}` returns a typed `ResultSet`.
- **Preconditions:** A stored query registered under a known qualified name.
- **Wire assertion:** Request path matches the qualified-name URL template; response decodes as `aql.ResultSet` with `Columns` and `Rows` populated.
- **Modes:** Live (snapshot).
- **Status:** Implemented (Live snapshot) — executed through POST, with a bound parameter scoping the result to the run's own EHR.
- **Satisfies:** REQ-057.

#### PROBE-067 — Template upload round-trip

- **Title:** `POST /definition/template/adl1.4` with an OPT body succeeds; a subsequent `GET` returns the same OPT bytes.
- **Preconditions:** Backend supports ADL1.4 template upload at the standard path.
- **Wire assertion:** `POST` with an OPT body and `Content-Type: application/xml` returns `201` with a `Location` header; a subsequent `GET` returns the same OPT bytes (modulo backend-side reformatting documented per deployment).
  The SDK's `Accept` negotiation and 201-response-body handling are the client contract in [wire.md § REQ-177 *Template upload negotiation and response*](wire.md#template-upload-negotiation-and-response), not asserted here.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-177.

#### PROBE-068 — Error envelope decodes into `WireError.OpenEHR`

- **Title:** A `400 Bad Request` carrying a `{message, code}` JSON body surfaces as a `transport.WireError` whose `OpenEHR` detail is populated and which matches a typed error via `errors.As`.
- **Preconditions:** Cassette of a real 400 error envelope.
- **Wire assertion:** `errors.As(err, &wire)` succeeds and `wire.OpenEHR.Code` is set from the envelope; with `transport.WithRawErrorBodies(true)`, `wire.OpenEHR.Message` is set from the envelope and `wire.RawBody` preserves the raw bytes.
- **Modes:** Sandbox, Cassette.
- **Status:** Deferred — the `transport` error-envelope unit tests cover the decode.
- **Satisfies:** REQ-093.

#### PROBE-069 — `Idempotency-Key` header round-trip

- **Modes:** Sandbox.
- **Status:** Deprecated — its requirement, REQ-097, is retired and the SDK no longer sends `Idempotency-Key`; the identifier is retained and not reused.

#### PROBE-070 — Admin `DeleteEHR` round-trip

- **Title:** `DELETE /admin/ehr/{ehr_id}` returns 2xx; a subsequent `GET /ehr/{ehr_id}` returns 404 surfaced as `transport.ErrNotFound`.
- **Preconditions:** Backend exposes the ITS-REST `/admin/*` surface; admin deletion is enabled for the tenant.
- **Wire assertion:** `admin.DeleteEHR` succeeds; `errors.Is(ehr.Get(...), transport.ErrNotFound)` is true after the delete.
- **Modes:** Sandbox.
- **Status:** Deferred — the `openehr/client/admin` unit tests cover the delete and the missing-EHR variants.
- **Satisfies:** REQ-099

#### PROBE-071 — Composition POST/PUT response body is bare COMPOSITION

- **Title:** `POST /ehr/{ehr_id}/composition` and `PUT /ehr/{ehr_id}/composition/{vo_uid}` with `Prefer: return=representation` return a bare `COMPOSITION` body — not an `ORIGINAL_VERSION<COMPOSITION>` envelope.
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Response body decodes cleanly as `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` / `200_COMPOSITION_updated` schemas.
  A server that returns `{"_type":"ORIGINAL_VERSION", ...}` on these paths is non-conformant; the SDK surfaces that as a decode error (strict-against-spec posture per REQ-094).
  The full version envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — the PUT arm runs when a versioned-object id and an `If-Match` are supplied, and is skipped otherwise.
- **Satisfies:** REQ-094 (`return=representation` arm only).

#### PROBE-072 — Contribution submission body matches `Contribution_create`

- **Title:** `POST /ehr/{ehr_id}/contribution` request body is the ITS-REST `Contribution_create` shape — `{audit, versions: [ORIGINAL_VERSION<T>|IMPORTED_VERSION<T> with inline data]}`, bar a deletion built without a payload — not the persisted `rm.Contribution` shape whose `versions[]` is `[]OBJECT_REF`.
- **Preconditions:** Existing EHR; at least one resource payload (`Composition` / `EHRStatus` / `Folder` / `EHRAccess`) to commit.
- **Wire assertion:** Captured request body has `versions[i]._type ∈ {"ORIGINAL_VERSION","IMPORTED_VERSION"}` and carries the resource payload inline under `data`, except an `ORIGINAL_VERSION` with change type `523` built without a payload ([§ REQ-130](wire.md#req-130--contribution-builder) *Deletion*, keyed by [ADR 0024](../adr/0024-contribution-deletion-without-data.md)), which carries no `data` member; a `"data":null` fails on every version.
  A request body whose `versions[]` contains `{"_type":"OBJECT_REF", ...}` is non-conformant per the ITS-REST OpenAPI `Contribution_create` schema (the persisted `OBJECT_REF` shape returns at read time only).
  **Commit-audit shape (SPECITS-95 / ITS-REST PR 131):** the batch `audit` and each `versions[i].commit_audit` omit the server-assigned `time_committed` and carry a `DV_CODED_TEXT`-shaped `change_type` (nested `defining_code`), not the erroneous flat `TERMINOLOGY_CODE`.
  The SDK emits `_type:"AUDIT_DETAILS"` by default, a form the pin asks servers to accept; `UPDATE_AUDIT`, the form the pin asks clients to send, is a caller-selected alternative.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-050.

#### PROBE-073 — Demographic PARTY polymorphic round-trip

- **Title:** A PARTY of each concrete type (PERSON / ORGANISATION / GROUP / AGENT / ROLE) round-trips through create → get → get-version with its `_type` discriminator decoded back into the same concrete type at every hop.
- **Preconditions:** A server (Sandbox: fake) serving the typed PARTY body for `POST|GET /demographic/{type}[/{uid_based_id}]` and the `ORIGINAL_VERSION<PARTY>` envelope for `GET /demographic/versioned_party/{vo_uid}/version`.
- **Wire assertion:** Sandbox — `demographic.Create` (Prefer=representation), `demographic.Get`, and `demographic.GetVersion` each decode the response into the same concrete Go type as the input PARTY (REQ-040).
  The VERSION hop also exercises the `ORIGINAL_VERSION<PARTY>` envelope, whose generic `data` cannot decode into the abstract `rm.Party` interface and is re-decoded by `_type` through the type registry.
  A body whose `_type` decodes to a different concrete type is non-conformant.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — the Demographic API is Draft upstream (ITS-REST `x-status: DEVELOPMENT`).
- **Satisfies:** REQ-040, REQ-050.

#### PROBE-078 — verb-aware EHR scoping on query execution

- **Title:** An EHR-scoped ad-hoc AQL execution via `POST /query/aql` carries the scope on the `openehr-ehr-id` request header, per the verb-aware scoping rule in [wire.md § AQL executor](wire.md#aql-executor); every GET query operation — `GET /query/aql` and `GET /query/{qualified_query_name}[/{version}]` — continues to carry the `ehr_id` query parameter.
- **Preconditions:** A stored or ad-hoc query executed with an EHR scope, against a backend that honours the ITS-REST OAS distinction between the two verbs (no `ehr_id` query parameter or body field on the POST operations).
- **Wire assertion:** Captured POST request carries `openehr-ehr-id: <ehr_id>` and no `ehr_id` query parameter or request-body field; the same scope on a GET request carries `ehr_id` as a query parameter and no header.
  A backend that only honours the header would run a query lacking it population-wide — the assertion catches an SDK regression that scopes POST via the query parameter instead.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-057.

#### PROBE-079 — `PutStoredQuery` recovers `{name, version}` from `Location`

- **Title:** A body-less `PUT /definition/query/{qualified_query_name}[/{version}]` store reply recovers the server-assigned `{name, version}` from the `Location` response header, per [wire.md § Stored AQL](wire.md#req-057).
- **Preconditions:** A store operation whose response is the canonical `200_StoredQuery_stored` shape — empty body, `Location` header carrying the qualified name and version.
- **Wire assertion:** `definition.PutStoredQuery` recovers `{name, version}` in the order [wire.md § REQ-057](wire.md#req-057) gives, `Location` first, and a malformed `Location` does not fail the call.
- **Modes:** Live (snapshot).
- **Status:** Implemented (Live snapshot).
- **Satisfies:** REQ-057.

#### PROBE-084 — Built contribution body matches `Contribution_create`

- **Title:** A `Contribution_create` body assembled by the contribution builder reaches the wire with the requested operation's change-type code, a lifecycle state on every version, `preceding_version_uid` exactly where the operation requires it, and none of the server-assigned fields the pin's `UpdateVersion` does not declare.
- **Preconditions:** One payload per operation the batch commits, except the deletion built without one — `COMPOSITION`, `EHR_STATUS` and `FOLDER` on the assertion below, `EHR_ACCESS` being reachable only through the same generic entry points and covered at unit level — plus a preceding-version uid per non-creation; a sandbox server accepting `POST /ehr/{ehr_id}/contribution`.
- **Wire assertion:** The captured request body, built through the builder and committed through `contribution.Commit` so the assertion covers the real marshal path, satisfies per version the rules of [§ REQ-130](wire.md#req-130--contribution-builder) *Change types*, *Preceding version*, *Lifecycle state*, *Deletion* and *Server-assigned fields*, and its `data` keeps the `_type` its operation asked for.
  The batch `audit` carries the caller's `change_type` verbatim, set to a code that matches no version's so that a value derived from the versions fails, and no `time_committed` appears anywhere in the body (*Audit*).
  The byte-level pin is a golden of the builder's own output, checked in beside the probe.
  The vendored corpus ([`testkit/corpus/submissions/`](../../testkit/corpus/README.md)) is a shape witness, compared structurally, not byte for byte, since its records carry a `CONTRIBUTION` envelope and payloads this SDK does not author: every version field a corpus record carries is one the builder also emits, and a field the builder omits is omitted for the reason REQ-130 names.
- **Effect:** mutating — the Sandbox arm issues `POST /ehr/{ehr_id}/contribution` against the fake.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — one batch covers every operation of the change-type table.
- **Satisfies:** REQ-130.

#### PROBE-091 — Path-parameter traversal is refused

- **Title:** A path parameter that is `.` or `..`, is empty, or contains `/`, `\`, or a control character is refused before any HTTP request is issued, on every path-interpolating leaf package.
- **Preconditions:** A transport client whose `*http.Client` records whether any request was issued (httptest or a tripwire RoundTripper).
- **Wire assertion:** For each leaf package that interpolates a path parameter, at least two hostile inputs each fail with a non-nil error and a captured request count of zero: the call fails closed with no bytes on the wire.
  One of them is the separator-smuggling id (`foo/bar` on a `Route`-set request), the only input that exercises the route-arity rule.
  The other is drawn from a traversal id (`a/../../definition/query/evil`, `..`), a backslash-bearing id, or a control character.
  A well-formed id that only needs percent-encoding (a space or dots in a template id, `Blood Pressure.v1`) still issues exactly one request whose path is encoded once (REQ-050), and the service root `"/"` — the System API's only operation — passes validation and issues its request (the REQ-150 exemption's positive control).
  Sentinel identity (`ErrInvalidPathSegment`, `ErrInvalidConfig`) is pinned by `transport/` unit tests, not by this probe.
- **Effect:** read-only — the hostile arms assert that NO request reaches the wire, and the positive control issues a read.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-150.

#### PROBE-092 — Contribution read matches `contribution_get`

- **Title:** `GET /ehr/{ehr_id}/contribution/{contribution_uid}` is issued by the contribution read leaf and the 200 body decodes as the persisted `CONTRIBUTION`.
- **Preconditions:** A known EHR id and contribution uid; a sandbox server returning a canonical-JSON contribution body (or 404).
- **Wire assertion:** The captured request method and path match the ITS-REST template; a 200 decodes to the contribution type (version metadata is **not** asserted — the pin defines only `Content-Type` on `200_CONTRIBUTION`); empty ids issue no request; a 404 fails with a non-nil error.
  Sentinel identity (`ErrNotFound`, `ErrInvalidConfig`) is pinned by `contribution` unit tests, not by this probe.
- **Effect:** read-only (`GET`, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-174.

#### PROBE-093 — Template list filters reach the wire

- **Title:** `ListTemplates` emits the ITS-REST list query parameters `template_id`, `concept`, `version`, `offset`, and `fetch` when the corresponding options are set, and omits them when unset.
- **Preconditions:** A sandbox Definition server on `GET /definition/template/adl1.4` (the only registered `TemplateFormat`; the pin's ADL 2 list operation shares the same five parameter components).
- **Wire assertion:** Each option appears as the named query key; an explicit `WithOffset(0)` / `WithFetch(0)` is present on the wire; a negative offset or fetch fails with a non-nil error and no request (sentinel identity is pinned by `definition` unit tests, not by this probe); no options yields an empty query.
  The response still decodes as the existing template-metadata slice.
- **Effect:** read-only (`GET` list, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-177.

#### PROBE-101 — 2xx decode failure is surfaced, not swallowed

- **Title:** A 200 whose body cannot be decoded as the requested representation fails the call rather than returning a silently zero-valued result — on the shared transport decode and on a hand-rolled Definition list decode alike — while a non-2xx on the same route keeps failing as it always did.
- **Preconditions:** A sandbox server that can serve, on demand, a 200 whose JSON body does not match the requested type, a 404, and a list route answering a JSON object where an array is expected.
- **Wire assertion:** Three arms, kept at observable-behaviour level.
  (a) A read served a 200 with an undecodable body fails with a non-nil error, and the returned resource is never a zero value paired with a nil error.
  A silent zero-valued success is the exact defect this probe exists to catch.
  Nothing is handed back beside that error: the returned resource is nil, since a failed decode has nothing to return.
  (b) The same read served a 404 fails with a non-nil error, likewise with a nil resource beside it.
  (c) A Definition list leaf served a JSON object where an array is expected fails with a non-nil error, and the slice beside that error carries zero entries — reporting an empty catalog for a body that did not decode would read a failure as success, which [REQ-151](transport.md#req-151--typed-2xx-decode-failure)'s keyed exclusion admits only for an empty *body*.
  Each arm also issues exactly one request: without that count, a leaf refusing the call before it reached the wire would credit the probe with a failure the server never provoked, and the probe would pass against an SDK that decodes nothing at all.
  **That count rule is scoped to `Modes: Sandbox`**, where no retry or redirect policy is in play; a later Cassette or Live promotion restates it against that mode's transport policy rather than inheriting it, so the probe keeps failing on decode behaviour and never on transport policy.
  Error-type identity (`*transport.DecodeError` versus `*transport.WireError`), raw-`Body` recovery, and the value-free `Error()` string are pinned by `transport/` and `openehr/client/definition/` unit tests, not by this probe.
- **Effect:** read-only — every arm issues a read and asserts on how the call fails.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-151.

#### PROBE-102 — System capabilities via `OPTIONS /`

- **Title:** The System API's single operation is `OPTIONS /`; its response decodes into the typed service capabilities with a declared `restapi_specs_version`.
- **Preconditions:** A backend that answers the ITS-REST *Options and Conformance* operation at the service root.
- **Wire assertion:** The request uses `OPTIONS` (not `GET`) against the service root — the client's configured base path, with or without its trailing slash — exactly one request is issued, and the response decodes into `system.ServiceCapabilities` with a non-empty `restapi_specs_version`.
  The verb is the assertion — a deployment or SDK that issued a `GET` would miss the operation the spec defines.
- **Effect:** read-only.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-050.

#### PROBE-103 — Admin bulk-delete via `DELETE /admin/ehr/all`

- **Title:** The Admin bulk-delete surface is `DELETE /admin/ehr/all`; a subset delete restricts to the named EHRs via the repeatable `ehr_id` query parameter — not a body, and not a path segment per id.
- **Preconditions:** A backend exposing the ITS-REST Admin `/admin/ehr/all` operation (the Admin API is upstream `x-status: DEVELOPMENT`).
- **Wire assertion:** The request uses `DELETE` against the exact service-relative `/admin/ehr/all` path, carries one `ehr_id` query parameter per id supplied, no id appearing as a path segment, and no request body (the OAS `admin_ehr_delete_all` operation declares parameters only).
  A 2xx (including `202` Accepted and `204` No Content) is success.
- **Effect:** mutating — the Sandbox arm issues the delete against the fake.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-099.

#### PROBE-104 — Definition example via `GET …/{template_id}/example`

- **Title:** The Definition example endpoint is `GET /definition/template/{format}/{template_id}/example`; its response decodes into a full COMPOSITION for the named template.
- **Preconditions:** A backend exposing the ITS-REST Definition example operation for a loaded template.
- **Wire assertion:** The request uses `GET` against the exact `…/definition/template/{format}/{template_id}/example` route, with the requested format's path segment and the template id in place, and the response decodes into a `*rm.Composition` with a non-empty `archetype_node_id`.
- **Effect:** read-only.
- **Modes:** Sandbox, Live (snapshot).
- **Status:** Implemented (Sandbox) — a Live snapshot of the example read-back runs too.
- **Satisfies:** REQ-050.

#### PROBE-108 — Composition formats reach the wire

- **Title:** `composition.Save` and `composition.Get` in canonical XML, FLAT and STRUCTURED send the format's media types and the template id, and decode the server's body in the format asked for.
- **Preconditions:** A backend that answers `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` and `GET /ehr/{ehr_id}/composition/{uid_based_id}` in the requested format; a Web Template and a composition that round-trip through FLAT and STRUCTURED.
- **Wire assertion:** For each of canonical XML, FLAT and STRUCTURED, the write carries the format's `Content-Type` and `Accept`, a FLAT or STRUCTURED write carries `openehr-template-id` equal to the Web Template's id, and the representation and the read's `200` body decode into a composition equal on the paths the probe sets to the one written.
  Under the format query options a FLAT write carries `application/json` with `format=FLAT` and `templateId`, and its read carries `Accept: application/json` with `format=FLAT`.
  No request carries a `.schema`-suffixed media type.
- **Effect:** mutating (writes one composition per arm in the sandbox and reads it back).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-173.

#### PROBE-109 — EHR versioned-object reads reach the pinned routes

- **Title:** The composition and EHR_STATUS leaves issue the eight `versioned_composition_*` and `versioned_ehr_status_*` reads of the vendored EHR API and decode their bodies, and a `version_at_time` the SDK sends keeps its fraction of a second.
- **Preconditions:** A sandbox server answering each route with a canonical-JSON body: a `VERSIONED_COMPOSITION`, a `VERSIONED_EHR_STATUS`, a `REVISION_HISTORY`, an `ORIGINAL_VERSION` of each payload family, and an `IMPORTED_VERSION` for the refusal row.
- **Wire assertion:** Each captured request's method, path and `Accept: application/json` match the pinned operation; the latest-version read carries no `version_at_time`, and the version-at-time read carries the instant in UTC with every sub-second digit it holds (the pin's example time carried to nanoseconds, `2015-01-20T19:30:22.765432101+01:00`, is sent as `2015-01-20T18:30:22.765432101Z`, so a value cut to milliseconds fails).
  Each 200 decodes to its RM type, and a version read's `data` to a `COMPOSITION` or an `EHR_STATUS` (a version without `data` passes only when its lifecycle state is `deleted`, code `523`, the RM's logical deletion; any other version without `data` fails the probe, so a leaf that drops the data it was served fails too).
  An `IMPORTED_VERSION` body fails as a typed decode error, its request held to the same method, path and `Accept`.
  The same sub-second value reaches the wire from the composition read by a `Ref` with a time, the EHR_STATUS and directory reads at a time, and the `VERSIONED_PARTY` version-at-time read; those four rows judge the request only, not the read's result.
  Sentinel identity (`ErrInvalidConfig`, `ErrNotFound`, `ErrInvalidShape`) is pinned by the leaf unit tests, not by this probe.
- **Effect:** read-only (`GET`, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-174.

#### PROBE-112 — Template representations reach the wire

- **Title:** `GetOperationalTemplate` and `GetWebTemplate` read the ADL 1.4 template route with their representation's `Accept` and hand back the body the server sent.
- **Preconditions:** Two sandbox Definition servers on `GET /definition/template/adl1.4/{template_id}` that hold the same template.
  The first answers `application/xml` with an operational template, `application/openehr.wt+json` with a Web Template, and any other `Accept` with `406`.
  The second offers no Web Templates and answers `application/openehr.wt+json` with `406`.
  The probe takes the template id and the bytes of both representations as inputs.
- **Wire assertion:** Each call issues exactly one `GET` on the route with the template id in place and its own `Accept` (`application/xml`, `application/openehr.wt+json`), and returns the served body byte for byte.
  A server that answers `406` to the Web Template's `Accept` makes `GetWebTemplate` fail with a non-nil error and non-nil metadata.
  Sentinel identity (`ErrInvalidConfig`, `*transport.WireError`) is pinned by `definition` unit tests, not by this probe.
- **Effect:** read-only (`GET`, no state change).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox).
- **Satisfies:** REQ-177.

### RM model introspection

#### PROBE-094 — RM meta-model introspection equals the pinned BMM

- **Title:** The compiled-in RM class graph — class universe, abstractness, immediate parents, transitive ancestors, concrete-descendant expansion, and per-attribute declaring class — equals an independent reduction of the pinned RM BMM, and every answer it gives is closed over the universe it reports.
- **Preconditions:** The pinned primary RM schema and its includes under [`resources/bmm/`](../../resources/bmm) (REQ-041), reduced in-test through the `openehr/bmm` loader (REQ-045) — deliberately **not** through the generator's own reduction, which would make the comparison tautological and would pass on a generator whose walk is itself wrong.
- **Wire assertion:** In-repo property, not backend-facing, over the rules of [§ REQ-048](reference-model.md#req-048--rm-meta-model-introspection-surface), which this entry does not restate; each arm compares the compiled-in answers with the independent reduction.
  (a) **Universe** — the compiled-in class set equals [§ The class universe](reference-model.md#the-class-universe), and a class on one side only fails in either direction: a missing class is an unanswerable question, an extra one a class name the pinned schemas do not define.
  (b) **Per class** — abstractness equals the BMM `is_abstract` flag verbatim, the six unflagged classes included, and the arm fails if a locally derived verdict is substituted, not only if the flag is misread ([REQ-041](reference-model.md#req-041--pinned-bmm-sources): the BMM wins).
  Immediate parents equal the BMM `ancestors` list filtered to the universe, in declaration order; transitive ancestors equal the closure of that filtered edge set; concrete descendants equal the non-abstract members of the inverse closure, plus the class itself when it is not abstract.
  (c) **Closure of the class graph** — every class name a class-graph answer returns is in the universe, and for every class whose BMM ancestors mix an RM class with a filtered one (`DV_ORDERED`, `DV_INTERVAL`, `DV_DATE`, `DV_PROPORTION`, …), the RM ancestor survives.
  Every dropped edge is accounted to one of the universe rule's declared exclusions, and the classes left with no in-universe ancestor are pinned in both directions.
  (d) **Declaration site and completeness** — for every class and every attribute the flattened tables report on it, the three observable properties of [§ The flattened attribute tables stay flattened](reference-model.md#the-flattened-attribute-tables-stay-flattened) hold, `Interval` being the one out-of-universe site on the pinned RM, pinned by name.
  The probe reads the container flag from the raw BMM, and the behaviour suite beside it compares the full triple with the site.
  The shipped attribute-name set per class equals the BMM's effective property set minus the named exemption pins of [§ The attribute tables are complete against the BMM](reference-model.md#the-attribute-tables-are-complete-against-the-bmm), held two-sidedly: an unpinned drop fails, and so does a stale pin whose drop has since been fixed.
  (e) **Negative space** — [§ Three answers, never conflated](reference-model.md#three-answers-never-conflated) over the pinned RM.
  The shapes the pinned RM cannot supply (a dead-end abstract class with a *named* identity, a class whose parent is absent from the data set) are exercised through REQ-048's synthetic-model seam in the behaviour suite beside this probe.
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-048.

#### PROBE-098 — Absence reasons account for the universe's negative space

- **Title:** For every name the pinned schemas declare and the class universe omits, the compiled-in absence table reports the reason an independent reduction of the pinned BMM derives, in both directions, under the fixed precedence of [§ REQ-048 § Absence reasons](reference-model.md#absence-reasons).
  Universe members report *none*, and names no schema declares report *undeclared* with no stored entry backing them.
- **Preconditions:** As PROBE-094: the pinned primary RM schema and its includes under [`resources/bmm/`](../../resources/bmm) (REQ-041), reduced in-test through the `openehr/bmm` loader (REQ-045) — never through the generator's own reduction, which would make the comparison tautological — with the generator's exclusion lists restated as literals so a silent change to either side fails the probe.
- **Wire assertion:** In-repo property, not backend-facing, over the rules of [§ REQ-048 § Absence reasons](reference-model.md#absence-reasons), which this entry does not restate.
  (a) **Accounting, both directions** — every declared name the universe omits gets the reduction's reason from the shipped surface, and every stored table entry is a declared-but-omitted name carrying that same reason; an entry on one side only fails either way.
  (b) **No overlap, nothing computed stored** — the stored table contains no universe member and no *undeclared* or *none* entry; every `KnownRMTypes()` member reports *none*.
  (c) **Undeclared** — a name no pinned schema declares reports *undeclared*.
  (d) **Precedence** — at least one name two rules match is pinned by name and reports the reason the fixed precedence ranks first (a primitive-mapped name the named-class exclusion list restates reports *primitive*, not *excluded class*).
  (e) **Consistency with the negative space** — every out-of-universe name this probe exercises reports not-known on every `Lookup`/`Hierarchy` question (PROBE-094 arm (e) holds the converse for universe members).
- **Modes:** In-repo (unit-level property; no backend).
- **Status:** Implemented (inline).
- **Satisfies:** REQ-048.

### Observability

#### PROBE-050 — OTel span carries openEHR attributes

- **Title:** Every outgoing request opens an OTel span with `openehr.spec_version` and a sanitised URL.
- **Preconditions:** An OTel `TracerProvider` given to `transport.WithTracerProvider`.
- **Wire assertion:** Captured span has the expected attribute set; URL does not contain the bearer token.
- **Modes:** Sandbox.
- **Status:** Deferred — the `transport` span unit tests cover the sanitised URL; none yet asserts `openehr.spec_version`.
- **Satisfies:** REQ-090

#### PROBE-051 — No-OTel is a silent no-op

- **Title:** With no `TracerProvider` given to `transport.WithTracerProvider` and none set globally, a request produces no error, no warning, and no allocated spans.
- **Preconditions:** A client built without `transport.WithTracerProvider`, and no global provider set.
- **Wire assertion:** Request succeeds; no global state mutation.
- **Modes:** Sandbox.
- **Status:** Deferred — the `transport` unit tests cover it.
- **Satisfies:** REQ-090
