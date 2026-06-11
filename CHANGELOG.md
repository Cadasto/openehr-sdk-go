# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — release policy in [`docs/releases.md`](docs/releases.md).

Pre-1.0 (`v0.x`): only `### Added` is in use. Internal renames, fix-ups, and dropped experiments fold into the relevant Added bullet (or are omitted) rather than carry separate `### Changed` / `### Fixed` / `### Removed` entries. Bullets are **short and high-level** (artefact class + scope + key REQs/probes) — detail belongs in commit messages and PR bodies (see [`AGENTS.md § CHANGELOG.md`](AGENTS.md#changelogmd)).

## [Unreleased]

### Added

- **Developer onboarding** — quick-start and runnable examples catalog under `docs/`.

## [0.3.0] - 2026-05-28

Third `v0.x` minor — RM polymorphic decode coverage (SDK-GAP-11) closes the wire-shape substitution gap on concrete-typed RM slots (`LOCATABLE.name`, `AUDIT_DETAILS`, `OBJECT_REF`, …); ergonomic `Get*` accessors on the narrow `*Like` interfaces become the preferred read surface; testkit cassette coverage rounds out ehrbase Robot fixtures. Per [`docs/releases.md`](docs/releases.md), `v0.x` minors may break public API — the single breaking change this cycle is the field-type lift on substitution slots (migrate via `rm.DVTextValueOf` / `rm.AsDVText` / `rm.AuditDetailsBase` / `rm.PartyIdentifiedBase` / `rm.ObjectRefBase` or the new `Get*` accessors); pin to the exact tag.

### Added

- **Testkit cassettes** — ehrbase Robot fixtures (minimal-entry, `Test_dv_*`, EHR_STATUS, FOLDER, persistent compositions, CONTRIBUTION submissions) under `testkit/cassettes/` with `fixtures.SubmissionJSON` and `scripts/ingest-robot-cassettes.sh`.
- **RM polymorphic decode coverage (SDK-GAP-11)** — `bmmgen` emits narrow `<Parent>Like` interfaces (`DVTextLike`, `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`) so concrete-typed RM slots admit Liskov substitution per the RM (e.g. `LOCATABLE.name DV_TEXT` carrying `DV_CODED_TEXT` round-trips losslessly). `DV_INTERVAL[T: DV_ORDERED]` decodes its `lower` / `upper` via typereg. Breaking change: fields previously typed as the concrete parent are now interfaces — migrate via `rm.DVTextValueOf`, `rm.AsDVText`, `rm.AuditDetailsBase`. PROBE-038.
- **`*Like` interface ergonomics** — narrow interfaces now expose Get-prefixed accessor methods (`DVTextLike.GetValue`, `GetDefiningCode`; `DVURILike.GetValue`; `AuditDetailsLike.GetSystemID`/`GetTimeCommitted`/`GetChangeType`/`GetCommitter`/`GetDescription`; `PartyIdentifiedLike.GetName`/`GetIdentifiers`/`GetExternalRef`; `ObjectRefLike.GetID`/`GetNamespace`/`GetType`). Additive (non-breaking) on top of the SDK-GAP-11 lift. Pre-existing `rm.DVTextValueOf` / `rm.DVURIValueOf` helpers stay as thin compat shims. See [`openehr/rm/doc.go`](openehr/rm/doc.go) § Substitution slots.

## [0.2.0] - 2026-05-26

Second `v0.x` minor — Contribution write path lands the spec request shape, AOM 1.4 primitive wrappers now flow through, and the release-tooling is fully wired (tag-driven workflow + auto-compatibility table). Per [`docs/releases.md`](docs/releases.md), `v0.x` minors may break public API — `contribution.Commit`'s signature change is the only one this cycle and has no in-tree callers.

### Added

- **C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission** — AOM 1.4 primitive short-name wrappers now flow through; `Composition.uid` emits `_type:"HIER_OBJECT_ID"`; PROBE-023 widened to full round-trip.
- **GitHub repo hygiene + release workflow** — issue / PR templates, refined `CONTRIBUTING.md` / `SECURITY.md`; tag-driven [`release.yml`](.github/workflows/release.yml) re-runs `make ci` and drafts a GitHub Release with auto-generated compatibility table.
- **Contribution submission shape (SDK-GAP-10)** — `contribution.Commit` now takes [`*Submission`](openehr/client/ehr/contribution/submission.go) (ITS-REST `Contribution_create`: inline `ORIGINAL_VERSION`/`IMPORTED_VERSION` with `data: T`), not the persisted `*rm.Contribution`. Breaking change; no in-tree callers. PROBE-072.

## [0.1.0] - 2026-05-26

First tagged release. Covers the openEHR-first Go SDK adoption slice: REST 1.1.0-development client family + auth + discovery + canonical codecs + the ADL 1.4 template / validation / instance / composition stack. Per [`docs/releases.md`](docs/releases.md), `v0.x` minors may break public API — pin to the exact tag.

### Added

- **Module scaffolding and process** — module layout, Makefile/Docker toolchain, AI workflow docs, GitHub Actions CI (REQ-001..005); SDD tree at [`docs/specifications/`](docs/specifications/) with REQ/PROBE/STRAND registry, machine-readable [`traceability.yaml`](docs/specifications/traceability.yaml), and `make spec-check` enforcement; implementation plans under [`docs/plans/`](docs/plans/); [`CONTRIBUTING.md`](CONTRIBUTING.md), [`SECURITY.md`](SECURITY.md), [`docs/releases.md`](docs/releases.md); ADRs [0001–0005](docs/adr/).
- **BMM and codegen** — pinned BMM corpus (`resources/bmm/openehr_base_1.3.0`, `openehr_rm_1.2.0`, `openehr_am_1.4.0`, `openehr_am_2.4.0`, `openehr_lang_1.1.0`, `openehr_term_3.1.0`); BMM loader (REQ-045); generated RM + AOM 1.4 types with `typereg` (REQ-040..047); `bmmgen` + `bmmdiff` tooling + weekly drift workflow ([ADR 0001](docs/adr/0001-bmm-version-bump-runbook.md)); BMM-driven RM structural lookup at `openehr/rm/rminfo/`.
- **Serialization** — canonical JSON (REQ-052) and canonical XML (REQ-056) codecs with `xsi:type` polymorphic dispatch and cross-format JSON↔XML invariant; vendored RM cassettes under `testkit/cassettes/{templates,compositions,rm}/`.
- **Transport, auth, discovery** — `transport/` with openEHR custom-header family (REQ-059), retry policy (REQ-091, `NoRetry` sentinel), OTel hooks (REQ-090), structured error envelope mapping (REQ-093), `Prefer` negotiation (REQ-094), `Idempotency-Key` (REQ-097), request `Observer` (REQ-098); auth providers `auth/{clientcreds,jwtbearer,basic,smart}` (REQ-060..063, REQ-069); SMART-on-openEHR with JWKS-validated ID tokens (REQ-064, REQ-067); service discovery (`smart/discovery/`, REQ-070..072).
- **REST clients** — System; EHR read/write (`composition`, `ehrstatus`, `directory`, `contribution`, `itemtags`, REQ-050..057, REQ-059); Definition (ADL 1.4 templates + stored AQL CRUD, REQ-057); AQL Query (`openehr/client/query/`, `openehr/aql/`, REQ-055); Admin (`openehr/client/admin/`, REQ-099). Composition + directory writes decode `Prefer: return=representation` bodies as bare RM types per the ITS-REST schemas (SDK-GAP-09) — the `ORIGINAL_VERSION` envelope is reached via `GET /versioned_composition/{vo_uid}/version/{version_uid}`.
- **Clinical modeling** — OPT parser at `openehr/template/` (REQ-100); compiled walker-friendly tree at `internal/templatecompile/` ([ADR 0005](docs/adr/0005-compiled-template-foundation.md)); primitive constraints at `openehr/template/constraints/` (REQ-103) with `Validate(value any)` + `ExampleValue() any`; template-driven composition validator at `openehr/validation/` (REQ-102); template-driven RM instance generator at `openehr/instance/` + `internal/templateinstance/rmwrite/` (REQ-107); generic OPT-driven composition builder at `openehr/composition/` (REQ-101).
- **Cadasto extras** — `cadasto/admin/` Live / Ready deployment health probes (SDK-GAP-07).
- **Conformance probes** (`testkit/probes/`) — versioned writes (PROBE-010..013, PROBE-071), definition (PROBE-067), serialize (PROBE-030/031, PROBE-033/034), discovery (PROBE-040/041), admin (PROBE-070), OPT path resolution (PROBE-022), primitive constraint validate (PROBE-024), composition validate (PROBE-025/026), instance generator round-trip on `vital_signs.opt` + `clinical_note.opt` (PROBE-027), composition builder marshal-fragment parity (PROBE-023).
- **Worked examples** under `cmd/examples/` — `canonical_json`, `canxml_roundtrip`, `ehr_create`, `opt-parse`, `validate-composition`, `validate-from-json`, `primitive-validate`, `generate-example`.

### Compatibility

| Concept | Value |
|---|---|
| Go toolchain (minimum) | `1.25.0` |
| openEHR REST | `1.1.0-development` |
| BMM corpus | `openehr_base_1.3.0`, `openehr_rm_1.2.0`, `openehr_am_1.4.0`, `openehr_am_2.4.0`, `openehr_lang_1.1.0`, `openehr_term_3.1.0` |

### Known follow-ups (not landed)

- [REQ-094 write-path gaps](docs/plans/2026-05-25-req094-prefer-followups.md) — `Prefer=identifier` + `representation`+empty-body guard.
- AQL verb-style builders ([plan](docs/plans/2026-05-21-aql-builders.md)) — Query/ResultSet wire models landed; verb builders open.
- Demographic REST client ([plan §Phase 7](docs/plans/2026-05-15-rest-api-client.md)) — `doc.go` stub only.
- CDR benchmark migration ([plan §Phase 9](docs/plans/2026-05-15-rest-api-client.md), STRAND-01).
