# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Authentication foundation and providers (`auth/`, `clientcreds/`, `jwtbearer/`, `basic/` REQ-069).
- Service discovery (`smart/discovery/`).
- Transport layer (`transport/`) with openEHR headers, retry, OTel, and error envelope mapping.
- REST clients: System API; EHR read/write (composition, ehrstatus, directory, contribution); Definition ADL 1.4 template lifecycle.
- Vendored ITS-REST and SMART cassettes; versioned-write (010–013), definition, serialize (030–031, 033–034), and discovery (040–041) conformance probes.
- Worked examples (`cmd/examples/{canonical_json,canxml_roundtrip,ehr_create}`) demonstrating canjson, JSON↔XML round-trip, and end-to-end EHR creation against an httptest backend.
- Implementation roadmap (`docs/roadmap.md`).
- Transport `RetryPolicy.Disabled` + `NoRetry` sentinel (REQ-096): unambiguous opt-out for benchmark / load-tool consumers.
- Transport `Request.IdempotencyKey` field (REQ-097): first-class `Idempotency-Key` header with OTel attribute pickup; PROBE-069.
- Transport request-level `Observer` hook (REQ-098): `WithObserver` + `WithObservationTag` deliver retry-aware `Observation` records per logical call.
- REST client `openehr/client/admin/`: ITS-REST `/admin/*` housekeeping — `DeleteEHR`, `DeleteAllEHRs`, `PurgeTemplates`, `Repository` (REQ-099); PROBE-070.

### Changed

- BMM resources relocated to `resources/bmm/`.
- Module path normalized to `github.com/cadasto/openehr-sdk-go`.
- Spec prose aligned with ADR-backed codegen and numeric wire rules.
- Makefile grouped `make help`; agent docs and package `doc.go` REQ citations synced with landed code.

### Fixed

- Transport caller-header overrides (canonical keys); Location parsing for version/template IDs from absolute URLs.
