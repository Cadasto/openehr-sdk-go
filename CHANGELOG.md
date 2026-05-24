# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Pre-1.0: nothing has been released as a tagged version yet, so only `### Added` is in use. Internal renames, fix-ups, and dropped experiments are folded into the relevant Added bullet (or omitted) rather than carried as separate `### Changed` / `### Fixed` / `### Removed` entries.

Entries under `## [Unreleased]` are **short and high-level**: one-line bullets naming the artefact class and scope. Detail belongs in commit messages and PR bodies — see [`AGENTS.md`](AGENTS.md#changelogmd).

## [Unreleased]

### Added

- Specs SDD structure: requirement registry, topic specs (`packaging`, `transport`), `traceability.yaml`, and `make spec-check`.
- Repository scaffolding: module layout, Makefile/Docker toolchain, AI docs, and CI.
- Normative `docs/specifications/` tree (REQ / PROBE / STRAND) and implementation plans under `docs/plans/`.
- Pinned BMM corpus under `resources/bmm/` and version-bump tooling (`bmmgen`, `bmmdiff`, drift workflow).
- BMM loader (`openehr/bmm/`) and generated RM + AOM 1.4 types with type registry.
- Canonical JSON codec (`openehr/serialize/canjson/`) and vendored cassettes; serialize conformance probes (PROBE-030/031).
- Canonical XML codec (`openehr/serialize/canxml/`): per-class `MarshalXML`/`UnmarshalXML`/`BMMName` companions; `xsi:type` polymorphic dispatch via typereg (top-level concrete generics like `DV_INTERVAL` included); `archetype_node_id` as XSD attribute per openEHR ITS-XML; cross-format JSON↔XML invariant; conformance probes PROBE-033/034.
- Vendored ehrbase RM cassettes under `testkit/cassettes/canonical_{json,xml}/ehrbase/` (Apache-2.0 with per-directory provenance + license attribution).
- ADRs 0001–0005 (BMM runbook, codegen decisions, EVENT polymorphism, numeric wire tolerance, compiled-template foundation).
- Authentication providers (`auth/`, `clientcreds/`, `jwtbearer/`, `basic/`, `auth/smart/`): client_credentials, JWT-bearer, basic, and SMART authorization-code with PKCE, JWKS rotation, refresh, and `Source.LastTokenResponse()` post-refresh accessor (REQ-061..063, REQ-069).
- SMART platform integration (`smart/`): launch context, RS256 ID-token validation via JWKS, principal claim extraction with `WithPrincipalClaimNames`, nbf/iat skew enforcement (REQ-064, REQ-067).
- Service discovery (`smart/discovery/`).
- Transport layer (`transport/`) with openEHR headers (incl. `openehr-item-tag`, `openehr-version-item-tag`), retry, OTel, error envelope mapping, caller-header override safety, and absolute-URL `Location` parsing for version/template IDs.
- Transport `RetryPolicy.Disabled` + `NoRetry` sentinel (REQ-096): unambiguous opt-out for benchmark / load-tool consumers.
- Transport request-level `Observer` hook (REQ-098): `WithObserver` + `WithObservationTag` deliver retry-aware `Observation` records per logical call.
- Transport `Client.HTTPClient()` accessor (REQ-021) — exposes the injected `*http.Client` so non-`transport` packages can reuse it outside the catalog-routed `Do` pipeline (mirrors the existing `Catalog()` accessor).
- REST clients: System API; EHR read/write (composition, ehrstatus, directory, contribution) plus item-tag get/set (`openehr/client/ehr/itemtags/`, REQ-059); Definition ADL 1.4 template lifecycle and stored AQL CRUD (REQ-057); AQL Query execute (`openehr/client/query/`, `openehr/aql/`, REQ-055); Admin `/admin/*` housekeeping — `DeleteEHR`, `DeleteAllEHRs`, `PurgeTemplates`, `Repository` (`openehr/client/admin/`, REQ-099).
- REQ-103 primitive constraint introspection (`openehr/template/constraints/`, follow-up Phase 6) — sealed `PrimitiveConstraint` interface with closed-set Go types for every ADL 1.4 OPT primitive `xsi:type` (`CBoolean`, `CInteger`, `CReal`, `CString`, `CDate`, `CTime`, `CDateTime`, `CDuration`, `CodePhrase`, `DvQuantity`, `CDvOrdinal`); typed `Violation` + `ViolationCode` payloads; `NumericRange` with inclusive / unbounded semantics; pure-function `Validate(value any) []Violation` per type. Wire-parse extension on `openehr/template/parse.go` decodes primitive children and attaches the typed value via `ComplexObject.PrimitiveConstraint()`; threaded through to `templatecompile.CompiledNode.PrimitiveConstraint()`. PROBE-024 sandbox probe (`testkit/probes/template/`) exercises parse → resolve → validate against fixture cases.
- Clinical modeling foundation (REQ-100 + follow-up Phases 1–4, [ADR 0005](docs/adr/0005-compiled-template-foundation.md)) — ADL 1.4 operational template (OPT) parser at `openehr/template/`: `OperationalTemplate` with closed `Node` taxonomy (`ComplexObject` / `ArchetypeRoot` / `Attribute` / `Slot`), openEHR path utilities (`ParsePath` / `NodeAt` / `ValidatePath`) with archetype-id and at-code predicates, strict-mode parse (`ParseOPTStrict` / `ParseFileStrict`) and strict path resolution (`WithStrictPaths` / `ErrAmbiguousPath`), OPT provenance metadata (`Description`, `Annotations`), ontology capture (`ArchetypeRoot.Terms` / `TermBindings`), walker dispatch via `ObjectNode` supertype, `Cardinality.String` / `IsValid`. BMM-driven RM structural lookup at `openehr/rm/rminfo/` (`Lookup` interface + `Default` populated by codegen). Compiled foundation at `internal/templatecompile/` — walker-friendly tree with cached AQL paths, O(1) reverse indexes, implicit RM-mandatory attribute injection, per-archetype-root term scope; consumed by the composition builder (REQ-101) and validator (REQ-102). PROBE-022 OPT path resolution probe.
- Cadasto health probes (`cadasto/admin/`, SDK-GAP-07) — `Live` / `Ready` deployment-level probes with typed per-probe path overrides (`WithLivePath` / `WithReadyPath`); URL derives from the openEHR REST catalog entry's origin (scheme + host); errors-Is-compatible mapping for 401/403/404/5xx via `transport` sentinels; deliberately bypasses `transport.Do` (no envelope decoding, no OTel spans, no retries).
- Vendored ITS-REST and SMART cassettes; versioned-write (010–013), definition, query, serialize (030–031, 033–034), discovery (040–041), admin (PROBE-070), and OPT path resolution (PROBE-022) conformance probes.
- Worked examples (`cmd/examples/{canonical_json,canxml_roundtrip,ehr_create,opt-parse}`) demonstrating canjson, JSON↔XML round-trip, end-to-end EHR creation against an httptest backend, and local OPT path resolution.
- Implementation roadmap (`docs/roadmap.md`).
