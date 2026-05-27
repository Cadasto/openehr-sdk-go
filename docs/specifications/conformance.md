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

### Vendored cassettes (`testkit/cassettes/`)

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
| `its_rest/` | Recorded HTTP request/response shapes | PROBE-010+, discovery probes (REQ-095) |

Discovery for PROBE-030 / PROBE-033 walks `compositions/` and `rm/` via [`fixtures.ListCompositionJSON`](../../testkit/fixtures/discover.go) and [`fixtures.ListRMXML`](../../testkit/fixtures/discover.go). Templates with JSON or XML on disk but known codec gaps **MAY** be listed in `compositionJSONExcluded`, `compositionXMLExcluded`, or `rmJSONExcluded` in that package so probes stay green while the files remain available for template and validation work.

**Legacy paths** (`testkit/cassettes/canonical_json/`, `canonical_xml/`, `fixtures/`, vendor subdirectories under `cassettes/`) are **retired** — do not reference them in new spec text, plans, or code comments.

## Probe catalog

The catalog is the normative list. Each entry has:

- **ID** — stable, never renumbered.
- **Title** — one-line description.
- **Preconditions** — what state the system must be in.
- **Wire assertion** — what's checked at the byte / status level.
- **Modes** — Sandbox / Cassette / Live.
- **Status** — Draft (in this spec), Implemented (in code), Ratified (cross-SDK pass against reference), Deprecated (scheduled removal; may be unrunnable when implementation is already gone pre-v1.0).
- **Satisfies** — REQ-IDs this probe exercises (inverse of the [REQ registry](REQ.md)).

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
- **Satisfies:** REQ-054, REQ-093

#### PROBE-011 — PUT Composition with stale If-Match

- **Title:** A PUT with a stale `If-Match` (referencing an old version_uid) is rejected with `412 Precondition Failed` or `409 Conflict` depending on backend convention.
- **Preconditions:** Composition has been updated since the SDK's cached `version_uid`.
- **Wire assertion:** PUT returns `412` or `409`; SDK maps to `ErrPreconditionFailed` or `ErrVersionConflict` accordingly.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_011_put_stale_if_match.go`](../testkit/probes/versioned/probe_011_put_stale_if_match.go).
- **Satisfies:** REQ-054, REQ-093

#### PROBE-012 — ETag survives round trip

- **Title:** A GET Composition followed by a PUT with the captured `ETag` as `If-Match` succeeds.
- **Preconditions:** Read-then-write workflow.
- **Wire assertion:** GET response carries `ETag`; PUT carries the same value as `If-Match`; PUT returns `204` or `200`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_012_etag_round_trip.go`](../testkit/probes/versioned/probe_012_etag_round_trip.go).
- **Satisfies:** REQ-054

#### PROBE-013 — Cross-EHR isolation

- **Title:** A `version_uid` belonging to EHR A cannot be read via EHR B's path.
- **Preconditions:** Two distinct EHRs; a Composition known to belong to EHR A.
- **Wire assertion:** GET `/ehr/{ehr_b_id}/composition/{version_uid_from_a}` returns `404 Not Found`, never `200`, never the EHR A data.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_013_cross_ehr_isolation.go`](../testkit/probes/versioned/probe_013_cross_ehr_isolation.go).
- **Satisfies:** REQ-054

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

- **Title:** Sharpens PROBE-025 with negative structural cases — missing required nodes, empty multi-valued attributes with `existence ≥ 1`, occurrences upper-bound violations, RM-type mismatches under C_SINGLE_ATTRIBUTE alternatives — and asserts the issue-code multiset (`required`, `cardinality`, `rm_type_mismatch`, `alternative_mismatch`) is stable across SDKs.
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
- **Status:** Implemented (Sandbox) — see [`testkit/probes/instance/probe_027_generated_validates.go`](../../testkit/probes/instance/probe_027_generated_validates.go). Probe runs against both `vital_signs.opt` and `clinical_note.opt` for `Minimal` and `Example` policies. v1 stop-gap: slot fills synthesise `openEHR-EHR-<RMType>.example.v1` archetype ids matching the validator's RM-type-prefix `slotFitsArchetypeID` heuristic until REQ-104 supplies a parsed slot grammar.
- **Satisfies:** REQ-107.

### Canonical JSON and formats

#### PROBE-030 — Canonical-JSON round trip

- **Title:** Decoding a canonical-JSON Composition and re-encoding produces byte-identical output (modulo documented field ordering).
- **Preconditions:** A reference Composition cassette.
- **Wire assertion:** `serialize.Decode → struct → serialize.Encode` produces output that, after the SDK's canonical-ordering pass, matches the input.
- **Modes:** Sandbox (no network).
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_030_canjson_round_trip.go`](../testkit/probes/serialize/probe_030_canjson_round_trip.go).
- **Satisfies:** REQ-052, REQ-040, REQ-082

#### PROBE-031 — `_type` discriminator decoded via registry

- **Title:** A `_type` not in the type registry decodes to a typed `UnknownTypeError`, not silently to `map[string]any`.
- **Preconditions:** A cassette containing an unregistered `_type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown `_type` value.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_031_typereg_unknown_type.go`](../testkit/probes/serialize/probe_031_typereg_unknown_type.go).
- **Satisfies:** REQ-040, REQ-052

#### PROBE-038 — RM polymorphic decode coverage (SDK-GAP-11)

- **Title:** `canjson.Unmarshal[Composition]` decodes every BMM-admissible `_type` discriminator at every substitutable slot — including (a) substitutable subtypes in concrete-typed slots (e.g. `LOCATABLE.name` carrying `DV_CODED_TEXT`) per openEHR RM Liskov substitution, and (b) generic types with abstract type parameters (e.g. `DV_INTERVAL[T: DV_ORDERED]`).
- **Preconditions:** Vendored RM cassettes under `testkit/cassettes/rm/polymorphic/` covering both failure modes.
- **Wire assertion:** Decode succeeds; the recovered tree preserves every original `_type` discriminator (no silent narrowing on substitutable slots); re-marshalling produces wire-equivalent JSON for the same logical content (canonical JSON ordering wins ties).
- **Modes:** Sandbox.
- **Status:** Draft — implementation tracked in [`docs/plans/2026-05-26-rm-polymorphic-decode-coverage.md`](../plans/2026-05-26-rm-polymorphic-decode-coverage.md). Flips to **Implemented (Sandbox)** once the ancestry-driven narrow-interface generator emission lands.
- **Satisfies:** SDK-GAP-11, REQ-040, REQ-052

#### PROBE-033 — Canonical-XML round trip

- **Title:** Decoding a canonical-XML Composition and re-encoding produces byte-identical compact XML (modulo documented element/attribute ordering).
- **Preconditions:** A reference Composition XML cassette under `testkit/cassettes/compositions/` or `testkit/cassettes/rm/` (see [Vendored cassettes](#vendored-cassettes-testkitcassettes)).
- **Wire assertion:** `canxml.Unmarshal → struct → canxml.Marshal` produces output that matches the input after the SDK's compact-XML canonicalisation pass.
- **Modes:** Sandbox (no network).
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_033_canxml_round_trip.go`](../testkit/probes/serialize/probe_033_canxml_round_trip.go).
- **Satisfies:** REQ-056, REQ-040, REQ-082

#### PROBE-034 — `xsi:type` discriminator decoded via registry

- **Title:** An `xsi:type` not in the type registry decodes to `typereg.ErrUnknownType`, not silently to an untyped value.
- **Preconditions:** A cassette (or hand-crafted XML) containing an unregistered `xsi:type`.
- **Wire assertion:** Decode returns `typereg.ErrUnknownType` with the unknown type value, wrapped in `*typereg.DecodeError`.
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/serialize/probe_034_typereg_xsi_unknown.go`](../testkit/probes/serialize/probe_034_typereg_xsi_unknown.go).
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
- **Status:** Implemented (Sandbox) — see [`testkit/probes/discovery/probe_040_catalog_ttl.go`](../testkit/probes/discovery/probe_040_catalog_ttl.go).
- **Satisfies:** REQ-070, REQ-072

#### PROBE-041 — Catalog refresh on 401

- **Title:** A `401` from a previously-working endpoint triggers exactly one discovery refresh and one retry; failure to recover surfaces a typed error.
- **Preconditions:** Cached catalog; backend rotates and returns `401` on the cached token.
- **Wire assertion:** SDK refreshes JWKS/catalog once, retries once. On second `401`, returns `transport.ErrUnauthorized`.
- **Modes:** Sandbox, Cassette.
- **Status:** Implemented (Sandbox) — discovery-layer half — see [`testkit/probes/discovery/probe_041_catalog_refresh_on_401.go`](../testkit/probes/discovery/probe_041_catalog_refresh_on_401.go). The probe asserts the resolver's `Refresh` against a 401 upstream issues exactly one fetch and returns a typed `*discovery.DiscoveryError(fetch_failed)`. The full transport-driven retry-on-401 + `transport.ErrUnauthorized` mapping (REQ-071 bullet 3) lands once the transport calls into `Resolver.Refresh` on 401 — currently transport surfaces 401 directly without re-driving discovery.
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

- **Title:** `POST /ehr/{ehr_id}/composition` with `Prefer: return=representation` returns a bare `COMPOSITION` body plus a new `ETag` (SDK-GAP-09).
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Request carries `Prefer: return=representation`; response body decodes as bare `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` schema (oneOf: `Composition` | `Identifier`) — **not** an `ORIGINAL_VERSION<COMPOSITION>` envelope, which lives at `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`). The response `ETag` is captured into `VersionMetadata`.
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) via PROBE-071 — the bare-body wire assertion (and the symmetric PUT path) is exercised by [`testkit/probes/versioned/probe_071_composition_write_response_shape.go`](../testkit/probes/versioned/probe_071_composition_write_response_shape.go) and the strict-against-spec unit pins `TestSaveRepresentationDecodesBareComposition`, `TestSaveRepresentationRejectsOriginalVersionShape`, `TestUpdateRepresentationDecodesBareComposition`, and `TestUpdateRepresentationRejectsOriginalVersionShape` in [`openehr/client/ehr/composition/composition_test.go`](../openehr/client/ehr/composition/composition_test.go). PROBE-061 stays as the named "Composition versioned write with `Prefer: return=representation`" probe in the REST-binding range; PROBE-071 is the SDK-GAP-09-anchored superset covering both POST and PUT with the strict-rejection assertion.
- **Satisfies:** REQ-094 (`return=representation` arm only). `Prefer=identifier` and empty-body strictness remain **not landed** — see [`docs/plans/2026-05-25-req094-prefer-followups.md`](../plans/2026-05-25-req094-prefer-followups.md).

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
- **Status:** Implemented (Sandbox) — see [`testkit/probes/definition/probe_067_template_upload_round_trip.go`](../testkit/probes/definition/probe_067_template_upload_round_trip.go).

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
- **Status:** Implemented (Sandbox) — happy-path delete + missing-EHR variants covered by [`openehr/client/admin/admin_test.go`](../openehr/client/admin/admin_test.go). Cross-SDK probe file (`testkit/probes/admin/`) deferred until the PHP SDK lands the admin client.
- **Satisfies:** REQ-099

#### PROBE-071 — Composition POST/PUT response body is bare COMPOSITION (SDK-GAP-09)

- **Title:** `POST /ehr/{ehr_id}/composition` and `PUT /ehr/{ehr_id}/composition/{vo_uid}` with `Prefer: return=representation` return a bare `COMPOSITION` body — not an `ORIGINAL_VERSION<COMPOSITION>` envelope.
- **Preconditions:** Existing EHR; a valid Composition body conforming to a deployed template.
- **Wire assertion:** Response body decodes cleanly as `*rm.Composition` per the ITS-REST OpenAPI `201_COMPOSITION` / `200_COMPOSITION_updated` schemas. A server that returns `{"_type":"ORIGINAL_VERSION", ...}` on these paths is non-conformant; the SDK surfaces that as a decode error (strict-against-spec posture per SDK-GAP-09). The full version envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}` (`UVersionOfComposition`).
- **Modes:** Sandbox, Cassette, Live.
- **Status:** Implemented (Sandbox) — see [`testkit/probes/versioned/probe_071_composition_write_response_shape.go`](../testkit/probes/versioned/probe_071_composition_write_response_shape.go) which exercises both POST and PUT arms in a single invocation when `voID` and `ifMatch` are supplied; otherwise the PUT arm is skipped and the probe still passes on POST alone. Unit-level pins covering both verbs and both halves of the strict-against-spec contract: `TestSaveRepresentationDecodesBareComposition`, `TestSaveRepresentationRejectsOriginalVersionShape`, `TestUpdateRepresentationDecodesBareComposition`, and `TestUpdateRepresentationRejectsOriginalVersionShape` in [`openehr/client/ehr/composition/composition_test.go`](../openehr/client/ehr/composition/composition_test.go). The same shape applies to `directory.Save` / `directory.Update` per `201_directory` / `200_FOLDER_retrieved`; covered by `TestSaveRepresentationDecodesBareFolder`, `TestSaveRepresentationRejectsOriginalVersionShape`, `TestUpdateRepresentationDecodesBareFolder`, and `TestUpdateRepresentationRejectsOriginalVersionShape` in [`openehr/client/ehr/directory/directory_test.go`](../openehr/client/ehr/directory/directory_test.go).
- **Satisfies:** SDK-GAP-09, REQ-094 (`return=representation` arm only).

#### PROBE-072 — Contribution submission body matches `Contribution_create` (SDK-GAP-10)

- **Title:** `POST /ehr/{ehr_id}/contribution` request body is the ITS-REST `Contribution_create` shape — `{audit, versions: [ORIGINAL_VERSION<T>|IMPORTED_VERSION<T> with inline data]}` — not the persisted `rm.Contribution` shape whose `versions[]` is `[]OBJECT_REF`.
- **Preconditions:** Existing EHR; at least one resource payload (`Composition` / `EHRStatus` / `Folder` / `EHRAccess`) to commit.
- **Wire assertion:** Captured request body has `versions[i]._type ∈ {"ORIGINAL_VERSION","IMPORTED_VERSION"}` and carries the resource payload inline under `data`. A request body whose `versions[]` contains `{"_type":"OBJECT_REF", ...}` is non-conformant per the ITS-REST OpenAPI `Contribution_create` schema (the persisted `OBJECT_REF` shape returns at read time only).
- **Modes:** Sandbox.
- **Status:** Implemented (Sandbox) — `contribution.Submission` lands in [`openehr/client/ehr/contribution/submission.go`](../../openehr/client/ehr/contribution/submission.go) and `contribution.Commit` now takes `*Submission`; the probe is at [`testkit/probes/versioned/probe_072_contribution_submission_shape.go`](../../testkit/probes/versioned/probe_072_contribution_submission_shape.go). Unit-level pin `TestCommitSubmissionShape` in [`openehr/client/ehr/contribution/contribution_test.go`](../../openehr/client/ehr/contribution/contribution_test.go) covers the wire-shape assertion at the SDK leaf; the probe wraps the same check for cross-SDK / cassette / live use. Implementation plan: [`docs/plans/archive/2026-05-26-contribution-submission-shape.md`](../plans/archive/2026-05-26-contribution-submission-shape.md).
- **Satisfies:** SDK-GAP-10, REQ-050.

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
- **Status:** Implemented (Sandbox) — covered by [`transport/client_test.go`](../transport/client_test.go).
- **Satisfies:** REQ-090

## Adding probes

A new probe **MUST**:

- Be assigned the next available `PROBE-NNN` for its topic range (gap of 10 between topics).
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
| Auth + discovery | PROBE-001 … 009 | *planned* — `testkit/probes/auth/` (discovery resolver covered by `smart/discovery/resolver_test.go`; formal probes not yet) |
| Versioned writes | PROBE-010 … 013 | [`testkit/probes/versioned/`](../testkit/probes/versioned/) — all implemented (Sandbox) |
| AQL | PROBE-020 … 021 | *planned* — `testkit/probes/aql/` |
| Clinical modeling | PROBE-022, PROBE-023, PROBE-024, PROBE-025, PROBE-026, PROBE-027 | [`testkit/probes/template/`](../../testkit/probes/template/) — PROBE-022 / PROBE-024 implemented (Sandbox); PROBE-023 implemented (Sandbox) under [`testkit/probes/composition/`](../../testkit/probes/composition/); PROBE-025 / PROBE-026 under [`testkit/probes/validation/`](../../testkit/probes/validation/); PROBE-027 implemented (Sandbox) under [`testkit/probes/instance/`](../../testkit/probes/instance/) — REQ-107 Phases 1–3 landed. |
| Canonical JSON / formats | PROBE-030 … 034, PROBE-038 | [`testkit/probes/serialize/`](../testkit/probes/serialize/) — 030–031, 033–034 implemented; PROBE-038 (SDK-GAP-11 polymorphic decode coverage) reserved Draft per [`docs/plans/2026-05-26-rm-polymorphic-decode-coverage.md`](../plans/2026-05-26-rm-polymorphic-decode-coverage.md); 032 not yet |
| Service discovery | PROBE-040 … 041 | [`testkit/probes/discovery/`](../testkit/probes/discovery/) — both implemented (Sandbox) |
| Observability | PROBE-050 … 051 | partial — PROBE-051 in [`transport/client_test.go`](../transport/client_test.go); *planned* — `testkit/probes/observability/` |
| REST binding | PROBE-060 … 068, PROBE-071, PROBE-072 | partial — PROBE-061/071 (`Prefer: return=representation`, SDK-GAP-09) implemented (Sandbox) at [`testkit/probes/versioned/probe_071_composition_write_response_shape.go`](../testkit/probes/versioned/probe_071_composition_write_response_shape.go) + leaf unit tests; PROBE-072 (SDK-GAP-10 contribution submission shape) implemented (Sandbox) at [`testkit/probes/versioned/probe_072_contribution_submission_shape.go`](../../testkit/probes/versioned/probe_072_contribution_submission_shape.go); PROBE-065 (`minimal`) and REQ-094 `identifier` / empty-body follow-ups **not landed** ([`docs/plans/2026-05-25-req094-prefer-followups.md`](../plans/2026-05-25-req094-prefer-followups.md)) |
