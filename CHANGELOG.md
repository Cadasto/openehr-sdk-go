# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Pre-1.0: nothing has been released as a tagged version yet, so only `### Added` is in use. Internal renames, fix-ups, and dropped experiments are folded into the relevant Added bullet (or omitted) rather than carried as separate `### Changed` / `### Fixed` / `### Removed` entries.

Entries under `## [Unreleased]` are **short and high-level**: one-line bullets naming the artefact class and scope. Detail belongs in commit messages and PR bodies — see [`AGENTS.md`](AGENTS.md#changelogmd).

## [Unreleased]

### Added

- Specs SDD structure: requirement registry, topic specs (`packaging`, `transport`), `traceability.yaml`, and `make spec-check`.
- Repository scaffolding: module layout, Makefile/Docker toolchain, AI docs, and CI.
- Normative `specs/` tree (REQ / PROBE / STRAND) and implementation plans under `docs/plans/`.
- Pinned BMM corpus under `resources/bmm/` and version-bump tooling (`bmmgen`, `bmmdiff`, drift workflow).
- BMM loader (`openehr/bmm/`) and generated RM + AOM 1.4 types with type registry.
- Canonical JSON codec (`openehr/serialize/canjson/`) and vendored cassettes; serialize conformance probes (PROBE-030/031).
- Canonical XML codec (`openehr/serialize/canxml/`): per-class `MarshalXML`/`UnmarshalXML`/`BMMName` companions; `xsi:type` polymorphic dispatch via typereg (top-level concrete generics like `DV_INTERVAL` included); `archetype_node_id` as XSD attribute per openEHR ITS-XML; cross-format JSON↔XML invariant; conformance probes PROBE-033/034.
- Vendored ehrbase RM cassettes under `testkit/cassettes/canonical_{json,xml}/ehrbase/` (Apache-2.0 with per-directory provenance + license attribution).
- ADRs 0001–0004 (BMM runbook, codegen decisions, EVENT polymorphism, numeric wire tolerance).
- Authentication providers (`auth/`, `clientcreds/`, `jwtbearer/`, `basic/`, `auth/smart/`): client_credentials, JWT-bearer, basic, and SMART authorization-code with PKCE, JWKS rotation, refresh, and `Source.LastTokenResponse()` post-refresh accessor (REQ-061..063, REQ-069).
- SMART platform integration (`smart/`): launch context, RS256 ID-token validation via JWKS, principal claim extraction with `WithPrincipalClaimNames`, nbf/iat skew enforcement (REQ-064, REQ-067).
- Service discovery (`smart/discovery/`).
- Transport layer (`transport/`) with openEHR headers (incl. `openehr-item-tag`, `openehr-version-item-tag`), retry, OTel, error envelope mapping, caller-header override safety, and absolute-URL `Location` parsing for version/template IDs.
- Transport `RetryPolicy.Disabled` + `NoRetry` sentinel (REQ-096): unambiguous opt-out for benchmark / load-tool consumers.
- Transport request-level `Observer` hook (REQ-098): `WithObserver` + `WithObservationTag` deliver retry-aware `Observation` records per logical call.
- REST clients: System API; EHR read/write (composition, ehrstatus, directory, contribution) plus item-tag get/set (`openehr/client/ehr/itemtags/`, REQ-059); Definition ADL 1.4 template lifecycle and stored AQL CRUD (REQ-057); AQL Query execute (`openehr/client/query/`, `openehr/aql/`, REQ-055); Admin `/admin/*` housekeeping — `DeleteEHR`, `DeleteAllEHRs`, `PurgeTemplates`, `Repository` (`openehr/client/admin/`, REQ-099).
- Vendored ITS-REST and SMART cassettes; versioned-write (010–013), definition, query, serialize (030–031, 033–034), discovery (040–041), and admin (PROBE-070) conformance probes.
- Worked examples (`cmd/examples/{canonical_json,canxml_roundtrip,ehr_create}`) demonstrating canjson, JSON↔XML round-trip, and end-to-end EHR creation against an httptest backend.
- Implementation roadmap (`docs/roadmap.md`).
