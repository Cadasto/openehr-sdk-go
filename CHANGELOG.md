# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — release policy in [`docs/releases.md`](docs/releases.md).

Pre-1.0 (`v0.x`): only `### Added` is in use; fix-ups and dropped experiments fold into the relevant bullet. `v0.x` minors *may* break public API — **pin to the exact tag**; breaking changes are called out per release below. Bullets are **short and high-level** (artefact class + scope + key REQs/probes) — detail lives in commit messages and PR bodies (see [`AGENTS.md`](AGENTS.md#code-style-and-conventions)).

## [Unreleased]

### Added

- **RM behavioural functions (REQ-120..123).** Hand-written realisations of the pure/derived openEHR RM functions `bmmgen` had emitted as panic stubs: identifier parsing/derivation with canonical `Parse*` entry points for `UID_BASED_ID` / `OBJECT_VERSION_ID` / `VERSION_TREE_ID` / `ARCHETYPE_ID` / `TERMINOLOGY_ID` / `LOCATABLE_REF` (`openehr/rm`, REQ-120; `client/ehr` version-uid helpers delegate to it); `VERSION.is_branch` (REQ-122); temporal `DV_DATE` / `DV_TIME` / `DV_DATE_TIME` / `DV_DURATION` component access, partial-form inspection, magnitude/compare, and `ToTime` / `ToDuration` (REQ-123); and locatable path read access `ItemAtPath` / `ItemsAtPath` / `PathExists` / `PathUnique` as a new `openehr/rm/rmpath` building block with its own reflection-free walker (REQ-121). A `bmmgen` manual-implementation skip set suppresses the corresponding stubs ([ADR 0002 D7](docs/adr/0002-bmm-codegen-decisions.md)); surface & fallibility per [ADR 0011](docs/adr/0011-rm-behavioural-functions-surface.md). No panics on malformed input; temporal arithmetic, `PATHABLE.parent` / `path_of_item`, and `VERSIONED_OBJECT` container ops remain deferred stubs. Minor behaviour change: `ehr.VersionUID.{VersionedObjectID,CreatingSystemID,VersionNumber}` now return `""` for a non-canonical (non-three-part) version-uid string, where the previous lenient splitter returned a partial segment — well-formed server-issued uids are unaffected. Additive.

## [0.9.0] - 2026-06-18

Ninth `v0.x` minor — the **SMART-on-openEHR auth conformance cycle**: the full client-auth matrix, SMART Backend Services, ID-token algorithm agility, and RFC 7662 token introspection land alongside the public compiled-template bridge (REQ-111) and the SDD tooling alignment. Additive only — no public API breaks this cycle; safe to upgrade from `v0.8.0`.

### Added

- **SMART-on-openEHR auth conformance audit (REQ-061..064, REQ-067, REQ-069..072).** Full client-auth matrix — public PKCE, confidential `client_secret_basic`/`client_secret_post`, asymmetric `private_key_jwt` (`auth/smart`); Backend Services `client_credentials` + `private_key_jwt` (`auth/clientcreds`); JWT Bearer grant RFC 7523 (`auth/jwtbearer`, RS384/ES384 baseline); ID-token alg agility RS256/RS384/ES256/ES384 (`smart/idtoken`); RFC 7662 token introspection (`auth/introspect`); transport 401→reauth safety net with terminal-vs-transient token-error classification (REQ-063) and configurable early-expiry; launch-context scope helpers. Retires the OTel-only dependency rule — adopts `golang.org/x/oauth2` + `github.com/coreos/go-oidc/v3` (+ direct `go-jose/v4`) for JOSE/OIDC crypto, scoped to `auth/`+`smart/` ([ADR 0009](docs/adr/0009-smart-auth-library-scope.md)); SMART discovery `services`-map shape ([ADR 0008](docs/adr/0008-smart-discovery-services-shape.md)). PROBE-001..009; runnable `cmd/examples/smart-launch`. STRAND-05 resolved.
- **Public compiled-template bridge (REQ-111).** New `openehr/templatecompile` re-exports `Compile(opt)`, the `Compiled` handle, and its introspection tree (`CompiledNode`/`CompiledAttribute`) so external modules can build the compiled template the builder (REQ-101), instance synthesiser (REQ-107), validator (REQ-102/110), and AQL lint (REQ-109) accept — and navigate it (form generation, path discovery) — without an `internal/` import ([ADR 0010](docs/adr/0010-public-compiled-template-bridge.md)). New examples `cmd/examples/{compile-build-validate,template-explore}`. Additive.
- **SDD tooling alignment (docs).** Added the machine-readable `docs/.sdd.yaml` descriptor and the `docs/development-process.md` constitution; reconciled the `go-jose/v4` dependency framing (direct import; also required by `go-oidc/v3`) across AGENTS.md / ADR 0009 / research-strands. No code or API change.

## [0.8.0] - 2026-06-17

Eighth `v0.x` minor — the **template modelling cycle** (REQ-104 slot assertions, REQ-105 terminology bindings, REQ-110 validation beyond COMPOSITION). Additive only; safe to upgrade from `v0.7.0`.

### Added

- **Slot assertion grammar (REQ-104).** `openehr/template/constraints` gains `SlotAssertion`/`SlotRules`, parsing anchored `archetype_id matches {regex}` from OPT `<includes>`/`<excludes>` (plain-text + Ocean operator-2007 XML); `validation` and `instance` (`ErrSlotFillUnsupported`) enforce parsed slot-fit, RM-type-prefix fallback only when no includes were parsed. PROBE-027.
- **Terminology bindings (REQ-105).** `ArchetypeRoot.Terms()`/`TermBindings()` deep-copy OPT `term_definitions`/`term_bindings`; compiled `Term`/`TermLang`/`TermBindings`/`TermBindingsForNode` surface per-node display text and external code bindings (single document language).
- **Validation beyond COMPOSITION (REQ-110).** Generic `validation.Validate(root, c)` + typed wrappers `ValidateDemographic` (PERSON/ORGANISATION/GROUP/AGENT/ROLE), `ValidateFolder`, `ValidateEHRStatus`; the lockstep walker (and `rmread`) extend to the demographic PARTY hierarchy and FOLDER/EHR_STATUS, plus DataValue leaf readers (DV_DATE/TIME/DATE_TIME/DURATION/BOOLEAN, DV_IDENTIFIER, DV_MULTIMEDIA). `ValidateComposition` now delegates, unchanged. PROBE-074.

## [0.7.0] - 2026-06-16

Seventh `v0.x` minor — the **AQL building-block cycle** (REQ-055 builders, REQ-109 parse + lint). Additive; safe to upgrade from `v0.6.0`. One new runtime dependency: the pure-Go ANTLR runtime (`github.com/antlr4-go/antlr/v4`), confined to `openehr/aql/parse` — the Java generator is containerised and never on the build/test path.

### Added

- **AQL builders (REQ-055).** `openehr/aql` struct-builder (`NewBuilder`) + verb-functions (`Select`/`From`/`FromEHR`/`Where`) emit byte-identical canonical AQL (PROBE-020); typed values/`Param` (injection guard), comparisons, `And`/`Or`; `client/query` maps path-resolution failures to `aql.ErrPathResolution` (PROBE-021).
- **AQL static lint (REQ-109).** New `openehr/aql/parse` (generated-type-free AST against the SDK grammar profile — official openEHR AQL plus documented `SDK-AQL-NNN` deltas, [ADR 0007](docs/adr/0007-aql-antlr-grammar-profile.md)) and `openehr/aql/lint` (three-layer collect-all lint), bridged by `validation.ValidateAQL`; new `aql.ErrSyntax`, `Compiled.AllByArchetypeID`; PROBE-028.

## [0.6.0] - 2026-06-15

Sixth `v0.x` minor — a documentation and developer-tooling cycle. No public API or behaviour change (REQ-073/REQ-108 formalise behaviour already shipped in `v0.4.0`).

### Added

- **Go developer tooling — go-coding adoption.** gofumpt + goimports via `golangci-lint fmt`; a golangci-lint v2 reference set (`modernize`, `errorlint`, `bodyclose`, `noctx`, `contextcheck`, …) through one pinned shim (`make fmt`/`fmt-check`/`lint`; dev image carries v2.11.4). Behaviour-preserving `modernize` sweep (generated `*_gen.go` untouched).
- **Documentation — SDD overhaul.** Restructured README/SECURITY/contributor & agent guides; expanded architecture/roadmap; resolved research strands 01–03; dropped PHP-SDK parity framing for openEHR wire conformance; added REQ-083 and formalised security hardening as REQ-073 + REQ-108 ([ADR 0006](docs/adr/0006-composition-validation-walker-placement.md)). New `make spec-context`/`probe-status`; stricter `spec-check`.

## [0.5.0] - 2026-06-13

Fifth `v0.x` minor — completes REQ-094 `Prefer` write-path negotiation. One behaviour change (stricter empty-`representation`); no signature changes, no in-tree callers affected.

### Added

- **REQ-094 `Prefer` completed.** Versioned writes (`composition`/`directory`/`ehr_status`) honour `Prefer: return=identifier` — the ITS-REST `Identifier` body populates the `VersionMetadata` slot via `ehr.ResolveIdentifierBody` (`Location` header stays canonical) — and `return=representation` with an empty body now returns `transport.ErrInvalidShape` instead of a silent nil. New surface: `ehr.Identifier`, `(*VersionMetadata).ResolveIdentifierBody`.

## [0.4.0] - 2026-06-12

Fourth `v0.x` minor — security hardening (auth/transport/SMART/OPT+BMM input), SMART launch-state verification (REQ-061), and the contribution write-audit shape correction. **Two breaking changes, both with no in-tree callers** (see the SMART and contribution bullets).

### Added

- **Developer onboarding** — quick-start and runnable examples catalog under `docs/`.
- **Security hardening (auth/transport/SMART/OPT+BMM input).** SMART discovery rejects issuer mismatch + non-https catalog endpoints; `transport.WireError` keeps the openEHR message/raw body out of `Error()` by default (`transport.WithRawErrorBodies`); response bodies capped (`transport.WithMaxResponseBody`, default 64 MiB); OPT/BMM/upload reads size-bounded (`bmm.ErrInputTooLarge`) and tree/path/polymorphic-decode depth-bounded (`typereg.ErrMaxDepthExceeded`); item-tag headers reject control chars; principal/launch claim maps defensively copied; `crypto/rand` JTI; `C_STRING` patterns compile once.
- **SMART launch state verification (REQ-061).** *Breaking:* `smart.Source.ExchangeAuthorizationCode` gains a `callbackState` parameter, verified against the issued state *before* any token-endpoint call (`smart.ErrLaunchInvalidState` on mismatch); `BeginAuthorization("")` self-generates random state. Migrate by passing the redirect's `state` query parameter as `callbackState`.
- **Contribution write-audit shape (SPECITS-95 / [ITS-REST PR 131](https://github.com/openEHR/specifications-ITS-REST/pull/131)).** Write-side commit-audit shape — `time_committed` omitted, `change_type` kept as `DV_CODED_TEXT`, defaulting to `_type:"AUDIT_DETAILS"` with a settable `UPDATE_AUDIT` fallback. *Breaking:* `Submission.Audit` → `contribution.UpdateAudit`; `Submission.Versions` entries → `OriginalVersion[T]`/`ImportedVersion[T]` (`WrapOriginalVersion`/`WrapImportedVersion`); no in-tree callers. PROBE-072; REQ-050/095.
- **Simplification & tooling.** Deduplicated the OAuth2-error parser and audit marshaller (`ehr.MarshalAuditDetails`); cached `rminfo.KnownRMTypes`; release workflow split into verify + publish jobs; `bmmgen` confines output paths; Dockerfile pins the Go patch version.
- **BMM `lang` pin synced upstream.** Canonical modular publisher form (now includes `base`); the BMM loader gains diamond-include support (a definition reached via two include paths merges once when identical). Reference-only — no generated-code change.

## [0.3.0] - 2026-05-28

Third `v0.x` minor — RM polymorphic decode coverage (SDK-GAP-11), `Get*` accessors on the narrow `*Like` interfaces, and rounded-out testkit cassettes. One breaking change (field-type lift on substitution slots).

### Added

- **RM polymorphic decode coverage (SDK-GAP-11).** `bmmgen` emits narrow `<Parent>Like` interfaces (`DVTextLike`, `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`) so concrete-typed RM slots admit substitution (`LOCATABLE.name` carrying `DV_CODED_TEXT` round-trips losslessly); `DV_INTERVAL[T: DV_ORDERED]` decodes via typereg. *Breaking:* those fields are now interfaces — migrate via `rm.DVTextValueOf`/`rm.AsDVText`/`rm.AuditDetailsBase`. PROBE-038.
- **`*Like` interface ergonomics.** Narrow interfaces expose `Get*` accessors (`GetValue`/`GetDefiningCode`, `GetSystemID`/`GetTimeCommitted`/`GetChangeType`/…, `GetName`/`GetIdentifiers`/…). Additive; pre-existing `rm.DVTextValueOf`/`rm.DVURIValueOf` stay as compat shims. See [`openehr/rm/doc.go`](openehr/rm/doc.go).
- **Testkit cassettes** — ehrbase Robot fixtures (minimal-entry, `Test_dv_*`, EHR_STATUS, FOLDER, persistent compositions, CONTRIBUTION) under `testkit/cassettes/` with `fixtures.SubmissionJSON` and `scripts/ingest-robot-cassettes.sh`.

## [0.2.0] - 2026-05-26

Second `v0.x` minor — the Contribution write path, AOM 1.4 primitive wrappers, and tag-driven release tooling. One breaking change (`contribution.Commit` signature, no in-tree callers).

### Added

- **C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission** — AOM 1.4 primitive short-name wrappers flow through; `Composition.uid` emits `_type:"HIER_OBJECT_ID"`; PROBE-023 widened to full round-trip.
- **Contribution submission shape (SDK-GAP-10)** — `contribution.Commit` now takes `*Submission` (ITS-REST `Contribution_create`: inline `ORIGINAL_VERSION`/`IMPORTED_VERSION` with `data: T`), not `*rm.Contribution`. *Breaking;* no in-tree callers. PROBE-072.
- **Repo hygiene + release workflow** — issue/PR templates, refined `CONTRIBUTING.md`/`SECURITY.md`; tag-driven [`release.yml`](.github/workflows/release.yml) with auto-generated compatibility table.

## [0.1.0] - 2026-05-26

First tagged release — the openEHR-first Go SDK adoption slice: REST 1.1.0-development client family + auth + discovery + canonical codecs + the ADL 1.4 template/validation/instance/composition stack.

### Added

- **Module scaffolding & process** — module layout, Makefile/Docker toolchain, AI workflow docs, CI (REQ-001..005); SDD tree at [`docs/specifications/`](docs/specifications/) (REQ/PROBE/STRAND registry + [`traceability.yaml`](docs/specifications/traceability.yaml) + `make spec-check`); plans; CONTRIBUTING/SECURITY/releases; ADRs [0001–0005](docs/adr/).
- **BMM & codegen** — pinned BMM corpus (`base_1.3.0`, `rm_1.2.0`, `am_1.4.0`, `am_2.4.0`, `lang_1.1.0`, `term_3.1.0`); BMM loader (REQ-045); generated RM + AOM 1.4 types with `typereg` (REQ-040..047); `bmmgen`/`bmmdiff` + weekly drift workflow ([ADR 0001](docs/adr/0001-bmm-version-bump-runbook.md)); BMM-driven RM lookup at `openehr/rm/rminfo/`.
- **Serialization** — canonical JSON (REQ-052) and canonical XML (REQ-056) codecs with `xsi:type` polymorphic dispatch and a cross-format JSON↔XML invariant.
- **Transport, auth, discovery** — `transport/` with openEHR custom headers (REQ-059), retry (REQ-091, `NoRetry`), OTel hooks (REQ-090), error-envelope mapping (REQ-093), `Prefer` (REQ-094), `Idempotency-Key` (REQ-097), request `Observer` (REQ-098); auth providers `auth/{clientcreds,jwtbearer,basic,smart}` (REQ-060..063, REQ-069); SMART-on-openEHR with JWKS-validated ID tokens (REQ-064, REQ-067); service discovery (REQ-070..072).
- **REST clients** — System; EHR read/write (`composition`/`ehrstatus`/`directory`/`contribution`/`itemtags`, REQ-050..057, 059); Definition (ADL 1.4 templates + stored AQL, REQ-057); AQL Query (REQ-055); Admin (REQ-099). `Prefer: return=representation` bodies decode as bare RM types (SDK-GAP-09).
- **Clinical modeling** — OPT parser at `openehr/template/` (REQ-100); compiled walker tree at `internal/templatecompile/` ([ADR 0005](docs/adr/0005-compiled-template-foundation.md)); primitive constraints (REQ-103); composition validator at `openehr/validation/` (REQ-102); RM instance generator (REQ-107); generic composition builder at `openehr/composition/` (REQ-101).
- **Cadasto extras** — `cadasto/admin/` Live/Ready health probes (SDK-GAP-07).
- **Conformance probes** (`testkit/probes/`) and **worked examples** (`cmd/examples/`: `canonical_json`, `canxml_roundtrip`, `ehr_create`, `opt-parse`, `validate-composition`, `validate-from-json`, `primitive-validate`, `generate-example`).

### Compatibility

| Concept | Value |
|---|---|
| Go toolchain (minimum) | `1.25.0` |
| openEHR REST | `1.1.0-development` |
| BMM corpus | `openehr_base_1.3.0`, `openehr_rm_1.2.0`, `openehr_am_1.4.0`, `openehr_am_2.4.0`, `openehr_lang_1.1.0`, `openehr_term_3.1.0` |

### Known follow-ups (not landed at 0.1.0)

- [REQ-094 write-path gaps](docs/plans/archive/2026-05-25-req094-prefer-followups.md) · AQL verb builders ([plan](docs/plans/archive/2026-05-21-aql-builders.md)) · Demographic REST client ([plan](docs/plans/archive/2026-06-14-demographic-rest-client.md)) · Benchmark harness ([plan](docs/plans/archive/2026-05-15-rest-api-client.md)).
