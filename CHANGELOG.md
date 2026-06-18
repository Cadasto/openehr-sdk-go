# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — release policy in [`docs/releases.md`](docs/releases.md).

Pre-1.0 (`v0.x`): only `### Added` is in use. Internal renames, fix-ups, and dropped experiments fold into the relevant Added bullet (or are omitted) rather than carry separate `### Changed` / `### Fixed` / `### Removed` entries. Bullets are **short and high-level** (artefact class + scope + key REQs/probes) — detail belongs in commit messages and PR bodies (see [`AGENTS.md § Code style and conventions`](AGENTS.md#code-style-and-conventions)).

## [Unreleased]

### Added

- **Public compiled-template bridge (REQ-111).** New `openehr/templatecompile` package re-exports `Compile(opt)`, the `Compiled` handle, and its introspection tree (`CompiledNode` / `CompiledAttribute`) so external modules can both construct the compiled template the composition builder (REQ-101), instance synthesiser (REQ-107), validator (REQ-102/110), and AQL lint (REQ-109) accept — now callable outside this module without any `internal/` import — and navigate it (form generation, path discovery, custom mapping). Sibling of `openehr/template` to avoid an import cycle and REQ-100's stdlib-only contract ([ADR 0010](docs/adr/0010-public-compiled-template-bridge.md)); new examples `cmd/examples/compile-build-validate` (public compile → build → validate) and `cmd/examples/template-explore` (introspection: structure + leaf-path discovery) show the public-only path. Additive.

## [0.8.0] - 2026-06-17

Eighth `v0.x` minor — the **template modelling cycle**: REQ-104 slot assertion grammar and REQ-105 terminology bindings land together with REQ-110 validation beyond COMPOSITION, completing the deferred tail of the REQ-100 follow-up plan. **Additive only — no public API or behaviour breaks this cycle**; per [`docs/releases.md`](docs/releases.md), `v0.x` minors *may* break public API, but none do here — safe to upgrade from `v0.7.0`.

### Added

- **Slot assertion grammar (REQ-104).** `openehr/template/constraints` gains `SlotAssertion` / `SlotRules` with anchored `archetype_id matches {regex}` parsing from OPT `<includes>` / `<excludes>` (plain-text and Ocean operator-2007 XML shapes); `Slot.SlotRules()` and `CompiledNode.SlotRules()` return defensive copies; `openehr/validation` and `openehr/instance` (`ErrSlotFillUnsupported`) enforce parsed slot-fit with RM-type-prefix fallback only when no includes were parsed; catch-all `.*` exclude ignored when closed includes are present. PROBE-027 slot-fill path updated. Additive.
- **Terminology bindings (REQ-105).** `ArchetypeRoot.Terms()` / `TermBindings()` on `openehr/template` deep-copy OPT `term_definitions` and `term_bindings`; compiled `Term`, `TermLang`, `TermBindings`, and `TermBindingsForNode` on `internal/templatecompile` surface per-node display text and external code bindings (single document language — `lang` accepted for forward compatibility). Additive.
- **Template-driven validation beyond COMPOSITION (REQ-110).** `openehr/validation` gains a generic `Validate(root, c)` plus typed wrappers `ValidateDemographic` (PERSON / ORGANISATION / GROUP / AGENT / ROLE), `ValidateFolder`, and `ValidateEHRStatus`; `ValidateComposition` now delegates to `Validate`. The lockstep walker (and `rmread`) extend to the demographic PARTY hierarchy + sub-components (ADDRESS / CONTACT / PARTY_IDENTITY / PARTY_RELATIONSHIP / CAPABILITY) and the EHR-IM roots FOLDER / EHR_STATUS, plus primitive-bearing DataValue leaf readers (DV_DATE/TIME/DATE_TIME/DURATION/BOOLEAN, DV_IDENTIFIER, DV_MULTIMEDIA) so explicit-`value` C_PRIMITIVE constraints validate rather than report a false `required`. PROBE-074. Additive — no public API or behaviour break (`ValidateComposition` unchanged).

## [0.7.0] - 2026-06-16

Seventh `v0.x` minor — the **AQL building-block cycle**: the typed builders (REQ-055) and the static parse + lint pipeline (REQ-109) land together, completing the Phase 2 AQL surface. **Additive only — no public API or behaviour breaks this cycle**; per [`docs/releases.md`](docs/releases.md), `v0.x` minors *may* break public API, but none do here — safe to upgrade from `v0.6.0`. One new runtime dependency: the pure-Go ANTLR runtime (`github.com/antlr4-go/antlr/v4`), confined to `openehr/aql/parse` — the generator (Java) is containerised and never on the build/test path (see [architecture.md § Dependencies](docs/architecture.md#dependencies)).

### Added

- **AQL builders (REQ-055).** `openehr/aql` gains a struct-builder (`NewBuilder`) and verb-functions (`Select` / `From` / `FromEHR` / `Where`) that emit byte-identical, canonical AQL (PROBE-020); typed values (`Param` + literals, the injection guard), comparisons, and `And` / `Or`; `openehr/client/query` maps backend path-resolution failures to `aql.ErrPathResolution` (PROBE-021). Completes the Phase 2 clinical building blocks.
- **AQL static lint (REQ-109).** New building-block packages `openehr/aql/parse` (syntax → generated-type-free AST against the SDK grammar profile — official openEHR AQL plus documented `SDK-AQL-NNN` deltas, [ADR 0007](docs/adr/0007-aql-antlr-grammar-profile.md)) and `openehr/aql/lint` (three-layer collect-all lint: syntax, shape, and template-aware archetype/path checks), bridged into the shared validation model by `validation.ValidateAQL`; new `aql.ErrSyntax` sentinel and `Compiled.AllByArchetypeID`; PROBE-028. The ANTLR Go runtime (`github.com/antlr4-go/antlr/v4`) is a new, narrowly-scoped runtime dependency confined to `aql/parse`; the Java generator is containerised (`make aqlgen`) and never on the build/test path.

## [0.6.0] - 2026-06-15

Sixth `v0.x` minor — a documentation and developer-tooling cycle: the SDD documentation overhaul plus adoption of the first-party **go-coding** toolchain. **No public API or behaviour change this cycle** — the security-hardening specs REQ-073 / REQ-108 formalise behaviour already shipped in `v0.4.0`. Per [`docs/releases.md`](docs/releases.md), `v0.x` minors may break public API — none this cycle; safe to upgrade from `v0.5.0`.

### Added

- **Go developer tooling — go-coding plugin adoption.** Formatting moves to gofumpt + goimports via `golangci-lint fmt`, and linting to a golangci-lint v2 reference set (`modernize`, `errorlint`, `bodyclose`, `noctx`, `contextcheck`, …), both routed through one pinned shim (`make fmt` / `fmt-check` / `lint`; the dev image carries golangci-lint v2.11.4). Includes a behaviour-preserving `modernize` sweep across the hand-written tree (generated `*_gen.go` untouched). No consumer-visible change.
- **Documentation — SDD overhaul + agent context routing.** Restructured README / SECURITY / contributor & agent guides; expanded the architecture and roadmap narratives; decoupled the specs from the PoC (research strands 01–03 resolved); dropped PHP-SDK parity framing in favour of openEHR wire conformance; added REQ-083 (Cadasto platform API conformance) and formalised the already-shipped security hardening as REQ-073 (discovery trust posture) + REQ-108 (untrusted document bounds), with [ADR 0006](docs/adr/0006-composition-validation-walker-placement.md) (composition-validation walker placement). New agent helpers `make spec-context REQ=NNN` and `make probe-status`, plus a stricter `spec-check` (full REQ coverage; Draft-is-binding; REQ.md Impl. == traceability implementation).

## [0.5.0] - 2026-06-13

Fifth `v0.x` minor — completes REQ-094 `Prefer` write-path negotiation on versioned writes (`composition` / `directory` / `ehr_status`): `return=identifier` populates the version identifier slot from the ITS-REST `Identifier` body, and `return=representation` with an empty body now returns `transport.ErrInvalidShape` instead of silently yielding a nil resource. New exported surface: `ehr.Identifier` + `(*VersionMetadata).ResolveIdentifierBody`. Per [`docs/releases.md`](docs/releases.md), `v0.x` minors may break public API — the only behavioural change this cycle is the stricter empty-`representation` handling (previously a silent `nil`); no signature changes and no in-tree callers affected. Pin to the exact tag.

### Added

- **REQ-094 `Prefer` write-path negotiation completed.** Versioned writes (`composition` / `directory` / `ehr_status`) now honour `Prefer: return=identifier` — the ITS-REST `Identifier` body (`{"uid": …}`) populates the `VersionMetadata` identifier slot (via `ehr.ResolveIdentifierBody`, with the `Location` header staying canonical) — and `return=representation` with an empty body now returns `transport.ErrInvalidShape` instead of silently yielding a nil resource ("MUST NOT silently downgrade").

## [0.4.0] - 2026-06-12

Fourth `v0.x` minor — security hardening across auth / transport / SMART / OPT + BMM input, SMART launch-state verification (REQ-061), the contribution write-audit shape correction (SPECITS-95 / ITS-REST PR 131), plus deduplication and least-privilege release tooling and a `lang` BMM pin sync. Per [`docs/releases.md`](docs/releases.md), `v0.x` minors may break public API — two breaks this cycle, both with no in-tree callers: (1) `smart.Source.ExchangeAuthorizationCode` gains a `callbackState` argument verified before any token-endpoint call (migrate by passing the redirect's `state` query parameter; `BeginAuthorization("")` now self-generates state); (2) the contribution submission write shape — `Submission.Audit` is now `contribution.UpdateAudit` and `Submission.Versions` entries are the `OriginalVersion[T]` / `ImportedVersion[T]` write-wrappers (migrate via `WrapOriginalVersion` / `WrapImportedVersion`). Pin to the exact tag.

### Added

- **Developer onboarding** — quick-start and runnable examples catalog under `docs/`.
- **Security hardening (auth / transport / SMART / OPT + BMM input).** SMART discovery rejects issuer mismatch and non-https catalog endpoints; `transport.WireError` keeps the openEHR error *message* and raw body out of `Error()` by default (opt in via `transport.WithRawErrorBodies`); transport caps response bodies (`transport.WithMaxResponseBody`, default `transport.DefaultMaxResponseBody` 64 MiB); OPT / BMM / template-upload reads are size-bounded (`bmm.ErrInputTooLarge`) and OPT tree, path, and polymorphic-JSON decode are depth-bounded (`typereg.ErrMaxDepthExceeded`); item-tag header values reject control characters; principal / launch-context claim maps are defensively copied; `crypto/rand` entropy added to the JWT-bearer JTI; `C_STRING` patterns compile once (`constraints.NewCString`).
- **SMART launch state verification (REQ-061).** Breaking change: `smart.Source.ExchangeAuthorizationCode` gains a `callbackState` parameter and verifies it against the issued state *before* any token-endpoint call, returning the new `smart.ErrLaunchInvalidState` on mismatch; `BeginAuthorization("")` now generates a cryptographically random state. Migrate by passing the `state` query parameter received at the redirect URI as `callbackState`.
- **Simplification & tooling.** Deduplicated the OAuth2-error parser and audit-details marshaller (`ehr.MarshalAuditDetails`); cached `rminfo.KnownRMTypes`; release workflow split into a read-only verify job and a write-scoped publish job; `bmmgen` confines generated output paths; Dockerfile pins the Go patch version.
- **BMM `lang` pin synced to upstream.** Replaced the pinned `openehr_lang_1.1.0` schema with the canonical modular publisher form (it now includes `base` rather than inlining it). The BMM loader gained diamond-include support: a definition reached via two include paths (e.g. `am2.4 → base` and `am2.4 → lang → base`) merges once when identical, instead of erroring. Reference-only schema — no generated code changes.
- **Contribution write-audit shape (SPECITS-95 / [ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131)).** The contribution submission request now carries the write-side commit-audit shape — server-assigned `time_committed` omitted, `change_type` kept as `DV_CODED_TEXT` — defaulting to `_type:"AUDIT_DETAILS"` with a settable `UPDATE_AUDIT` fallback for non-conformant servers. Breaking change: `Submission.Audit` becomes `contribution.UpdateAudit`, and `Submission.Versions` entries become the `OriginalVersion[T]` / `ImportedVersion[T]` write-wrappers (`WrapOriginalVersion` / `WrapImportedVersion`); no in-tree callers outside the package. PROBE-072 extended; REQ-050 / REQ-095.

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

- [REQ-094 write-path gaps](docs/plans/archive/2026-05-25-req094-prefer-followups.md) — `Prefer=identifier` + `representation`+empty-body guard.
- AQL verb-style builders ([plan](docs/plans/archive/2026-05-21-aql-builders.md)) — Query/ResultSet wire models landed; verb builders open.
- Demographic REST client ([plan](docs/plans/archive/2026-06-14-demographic-rest-client.md)) — `doc.go` stub only.
- Benchmark harness migration ([plan §Phase 9](docs/plans/archive/2026-05-15-rest-api-client.md)).
