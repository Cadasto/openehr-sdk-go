# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — release policy in [`docs/releases.md`](docs/releases.md).

Pre-1.0 (`v0.x`): only `### Added` is in use; fix-ups and dropped experiments fold into the relevant bullet. `v0.x` minors *may* break public API — **pin to the exact tag**; breaking changes are called out per release below.

**Keep it terse — the release notes are generated verbatim from each version block, so length here is length there.** Each bullet is **one sentence, ~35 words max**: artefact class + scope + the key REQ/PROBE, nothing more. **No inline lists of types, methods, parameters, or per-REQ breakdowns** — that detail lives in [`traceability.yaml`](docs/specifications/traceability.yaml), commit messages, and PR bodies. The optional per-release summary line is **one sentence**. If a bullet runs past two lines on screen it is too long — cut it in half. (See [`AGENTS.md`](AGENTS.md#code-style-and-conventions).)

## [Unreleased]

### Added

- **Canonical-JSON decode shape sentinel (REQ-052).** Every generated RM/AOM `UnmarshalJSON` now classifies a JSON-shape failure with `canjson.ErrInvalidShape`, so `errors.Is` finally matches; the message and the unwrapped cause are unchanged. [plan](docs/plans/archive/2026-09-02-decode-error-surface-typing.md).
- **Value-free diagnostics for unrouted requests (REQ-093).** A request with no route template renders the placeholder `(unrouted)` rather than its resolved path or URL — `WireError.Error()` now shows `(GET (unrouted))`. [plan](docs/plans/archive/2026-09-02-decode-error-surface-typing.md).

## [0.24.0] - 2026-08-30

Twenty-fourth `v0.x` minor — a 2xx body that will not decode is a typed error carrying the raw bytes, Definition catalogs tolerate timestamp layouts and re-emit unknown keys, and AQL gains write-side parity, path-shape lint and a 501 capability sentinel. **Breaking:** `aql.SelectField` is no longer usefully `==`-comparable.

### Added

- **Typed 2xx decode failure (REQ-151, PROBE-101).** A 2xx body that will not decode now fails with `transport.DecodeError` carrying the raw bytes; the non-2xx and empty-body arms are unchanged, and the message no longer echoes the codec's text — unwrap or read `Body`.
- **Canonical-JSON encode sentinel (REQ-052).** A value the canonical-JSON encoder refuses now carries `canjson.ErrInvalidValue`, `errors.Is`-distinguishable from the decode-side shape sentinel and from the transport one — the encode side had none of its own.
- **Catalog descriptor decode and re-encode.** Definition catalog timestamps decode against a closed tolerant layout set and an empty 2xx list body yields a non-nil empty slice (REQ-144); the Definition and System descriptors re-emit unknown keys, documented fields winning a collision.
- **Nil-receiver tolerance on the error axis (REQ-025).** Typed-nil error values left behind by a failed `errors.As` answer instead of panicking across the transport, query, auth, discovery, AQL-parse and type-registry error types, and the SDK sites that dereferenced such a match are guarded.
- **Reserved stored-query name refused client-side (REQ-057).** Store and execute refuse the query-name `aql` before the wire — store any case variant (`transport.ErrInvalidConfig`), execute the exact name (`query.ErrInvalidConfig`); stored-path diagnostics name the invoked operation.
- **AQL 501 capability sentinel (REQ-055, PROBE-021).** HTTP 501 from query execution is `aql.ErrEngineCapability` — a capability gap, classified from the status alone, never from message text, and never also path resolution.
- **AQL containment evidence under CI (REQ-160, PROBE-100).** The vendored EHRbase FROM-combination corpus runs under `go test` as PROBE-100's admissibility ratchet over the REQ-160 relation — purely additive, no SDK behaviour change.
- **AQL path-shape and paging lint (REQ-164, PROBE-099).** A third additive `lint` Layer-2 group — five Warning codes for unpredicated repeating segments, unordered row bounds, unaliased projections, path fan-out and provably inert containment steps.
- **AQL `TOP` plus envelope `fetch` lint (REQ-109, REQ-118).** Lint raises Error `aql_top_with_fetch` when a `TOP` query also carries the request envelope's row limit — the read-side twin of the existing `Build()` refusal.
- **AQL write-side expressivity parity (REQ-163, PROBE-088/097).** The builder spells the `VERSION` predicate bracket, a class-position standing comparison and a typed projection its read side already modelled, and `Build()` verifies what it emitted, refusing a construction that changes the recorded query — semver-minor on REQ-119's footing. *Breaking:* `aql.SelectField` is no longer usefully `==`-comparable — `==` compiles but panics at run time on a call-shaped field, map insertion included; compare built query strings.
- **Dependency pins.** OpenTelemetry 1.45 → 1.46 (`otel` / `otel/trace`); the `go.mod` floor stays `1.26.0`.

## [0.23.0] - 2026-08-26

Twenty-third `v0.x` minor — `rminfo` now answers why a name is *not* in its class universe, so a caller can tell a real openEHR class this SDK does not ship from a name openEHR never defines. Purely additive: no existing signature or answer changes.

### Added

- **RM class-universe absence reasons (REQ-049, PROBE-098).** `rminfo` now reports why a name is *not* in the class universe, as a closed `AbsenceReason` on an optional `AbsenceReporter` backed by a generated table; existing answers are unchanged.

## [0.22.0] - 2026-08-25

Twenty-second `v0.x` minor — AQL gains a semantic layer that tells an RM-impossible query apart from one that merely returns no rows, on the read and write sides alike, and the contribution builder and write-result contract land. **Breaking:** `lint.Options` gains a field, so positional struct literals no longer compile, and `contribution.Commit` stops reporting an empty 2xx representation as success.

### Added

- **Builder containment verification (REQ-162, PROBE-097).** `(*Builder).VerifyContainment` walks the builder's own containment algebra opt-in, reporting REQ-161's codes at read/write parity with the linter; `Build()` and its emitted text are unchanged.
- **AQL semantic and portability lint (REQ-161, PROBE-097).** `lint` gains REQ-160-backed containment checks and spec-gap portability advisories, supplied a relation at every entry point including the validation seam; adding the field breaks positional struct literals.
- **AQL containment admissibility relation (REQ-160, PROBE-097).** `aql/contain` answers whether a containment route connects two AQL class expressions, derived from the pinned BMM plus a cited overlay table and extensible per deployment.
- **Contribution builder (REQ-130, PROBE-084).** `contribution.Builder` assembles a `Contribution_create` body from per-operation changes; write-side version bodies stop emitting the server-assigned `contribution` and an empty `uid`.
- **Write-result contract (REQ-094).** Callers distinguish a successful write with no body from a committed write whose representation is empty or unusable, via `HasResource` and `NoRepresentationError`; `contribution.Commit` no longer treats an empty 2xx representation as success.
- **Toolchain pins.** golangci-lint v2.12.2 → v2.13.0, which carries the `modernize` analyzers forward with `golang.org/x/tools` 0.44 → 0.48, and the Go dev image 1.26.5 → 1.26.6; the `go.mod` floor stays `1.26.0`.

## [0.21.0] - 2026-08-20

Twenty-first `v0.x` minor — the client now fails closed on hostile path segments, nil caller input and typed-nil committers before anything reaches the wire, and the contribution read, template list filters and RM meta-model introspection land. **Breaking:** `contribution.Repository` and `definition.Repository` each grow a member and `parse.Clause` gains four — compile-time breaks for out-of-tree implementers and exhaustive switches.

### Added

- **Path-parameter segment validation (REQ-150).** The transport refuses a traversal, empty, backslash-bearing or control-character path segment, and a path whose segment count contradicts its `Route` template, before any request is issued.
- **Template list filters (REQ-143).** `ListTemplates` takes the ITS-REST list filters as trailing options; `definition.Repository` grows the same variadic — a compile-time break for external implementers.
- **Probe runnability specified (REQ-082, STRAND-11).** The three-mode mandate gains mode selection, a shared probe-result contract, per-mode rules, and write-probe semantics; the `v1.0.0` gate now cites the per-REQ `Impl.` axis.
- **Value-free AQL diagnostics (REQ-113, REQ-109, PROBE-096).** `parse.Document.Dropped()` and `lint.Issue.Span` report a finding's kind, clause and position with no query text in any structured field; `parse.Clause` gains four members, so a consumer's exhaustive switch gains unhandled cases.
- **Structured AQL node predicates (REQ-113, PROBE-095).** `aql.PathSegment.Parsed` types every bracketed path-segment predicate beside its verbatim text — node ids, names, `$param`, comparisons, regex and junctions — with `aql.EqualPredicates` for comparison.
- **RM meta-model introspection (REQ-048, PROBE-094, STRAND-12/13).** `rminfo` gains a compiled-in class graph — abstractness, parents, ancestors, conformance, concrete-descendant expansion, attribute declaration site — and `KnownRMTypes` widens to every emitted class.
- **Contribution read (REQ-142, PROBE-092).** `contribution.Get` reads a persisted contribution by uid and `contribution.Repository` grows the same read — a compile-time break for external implementers.
- **Library code fails closed instead of panicking (REQ-025).** Constructors, client entry points and variadic options refuse nil, typed-nil and zero-value caller input — including `PartyProxy` committers that leaked `"committer":null` — with a `Must`-prefix carve-out.

## [0.20.0] - 2026-08-18

Twentieth `v0.x` minor — REQ-119 closes AQL emission under re-parsing, so every value and predicate the SDK writes it can read back as what it meant. **Breaking:** the AQL write paths refuse text they once emitted, and predicate and path text now emits verbatim.

### Added

- **Re-parseable, verified canonical AQL emission (REQ-119, PROBE-090).** Every emitted value and predicate round-trips — **changes emitted AQL** for escaped strings, whole-valued reals, and verbatim predicate/path text. *Breaking:* the AQL write paths now return `ErrInvalidQuery` where they once wrote structure-changing or unparseable text.

## [0.19.0] - 2026-08-06

Nineteenth `v0.x` minor — REQ-140's underscore-prefixed RM attribute grammar takes upstream FLAT parity from 19.7% to 80.4% of the EHRbase corpus, and AQL gains the deprecated `SELECT TOP` carrier. No public API removals; `instance.Generate` changes the `subject` of every generated ENTRY.

### Added

- **FLAT DV_TEXT substitution and `ctx/setting` emission (REQ-053, PROBE-086).** A captured `DV_CODED_TEXT` at a `DV_TEXT` leaf rides the coded suffix form, `EVENT_CONTEXT.setting` round-trips, and encode refuses a setting it cannot carry — coverage 19.5% → 19.7%.
- **Generated compositions satisfy `Setting_valid` and `Basic_validity` (REQ-107).** `instance.Generate` now pins an openEHR-coded `setting` default and fills an unconstrained `PARTY_PROXY` with `PARTY_SELF`, changing the `subject` of every generated ENTRY.
- **AQL `SELECT TOP` carrier and literal source text (REQ-118, PROBE-087/088).** The deprecated `SELECT TOP n` clause now parses, emits and builds, a projected literal carries its source text, and two lint codes flag `TOP` + `LIMIT`.
- **Underscore-prefixed RM attributes in FLAT/STRUCTURED (REQ-140, REQ-053, ADR 0016, PROBE-089).** The simplified codecs carry the whole `_`-prefixed RM attribute grammar and the remaining datatype leaves, DV_SCALE included, taking upstream FLAT parity from 19.7% to 80.4%.
- **Go idiom modernization (no API or spec change).** A tree-wide pass onto current stdlib idioms, fixing an XML prologue scan that could pick the wrong root type and a nondeterministic FLAT encode diagnostic.
- **Toolchain and dependency pins.** golangci-lint v2.12.2 and the Go 1.26.5 dev image; OpenTelemetry 1.44 → 1.45, whose breaking changes are confined to modules this SDK does not import.

## [0.18.0] - 2026-08-05

Eighteenth `v0.x` minor — the upstream FLAT conformance ratchet nearly doubles PROBE-086 coverage (10.5% → 19.5%) and the AQL expression catalogue closes to the full SDK grammar profile. **Breaking:** `aql.Containment` is no longer `==`-comparable (REQ-117).

### Added

- **CODE_PHRASE FLAT leaves (REQ-053, REQ-121, PROBE-086).** `openehr/serialize/simplified` round-trips standalone `CODE_PHRASE` leaves as `|code` + `|terminology` — the larger half of the 10.5% → 19.5% PROBE-086 coverage ratchet, and **encode emits four new keys per ENTRY**.
- **Reference metadata spellings accepted on decode (REQ-053, REQ-025, ADR 0015).** FLAT decode accepts the reference's real-path composition metadata alongside the `ctx/` short forms — still the only emitted spelling — and a malformed composite context value reports a typed error instead of panicking.
- **Optional datatype suffixes in FLAT (REQ-053, PROBE-086).** Optional `DV_ORDERED` / `QUANTIFIED` / `AMOUNT` decorations round-trip as suffixes instead of `|raw`, and a decorated `DV_TEXT` at an open coded leaf keeps its decoration rather than flattening to a bare `|other`, now refused beside other suffixes.
- **AQL expression-catalogue completion (REQ-117, PROBE-087/088).** The structured AST covers the whole SDK grammar profile, the lint gate accepts two over-rejected shapes, and the builder gains a containment algebra plus opt-in in-text paging. *Breaking:* `aql.Containment` is no longer `==`-comparable.

## [0.17.0] - 2026-08-03

Seventeenth `v0.x` minor — the simplified-formats stack lands: a WebTemplate JSON export and the bidirectional FLAT/STRUCTURED codecs built on it, plus template-level node naming and the first upstream FLAT conformance slice. **Breaking:** REQ-116 changes compiled-path and FLAT-key shapes for templates that pin a node name — read [`webtemplate/deviations.md`](openehr/template/webtemplate/deviations.md) before upgrading.

### Added

- **WebTemplate JSON export (REQ-106, ADR 0014).** New building block `openehr/template/webtemplate` projects a compiled OPT into EHRbase `openEHR_SDK` v2.3 WebTemplate JSON, pinned by structural parity (PROBE-075) against vendored reference fixtures.
- **FLAT / STRUCTURED simplified-format codecs (REQ-053, PROBE-076).** New building block `openehr/serialize/simplified` — bidirectional, Web-Template-driven Flat/Structured codecs with OPT-free interconversion, strict fail-loud decode, and an OPT-validatable `WithTemplate` decode mode.
- **Template-level node naming and name-predicated paths (REQ-116, PROBE-075).** Paths gain `[archetype_id,'Name']` predicates, WebTemplate `id`s derive from pinned names, and single-occurrence abstract EVENTs are lifted — **compiled-path and FLAT-key shapes change** (details: `webtemplate/deviations.md`).
- **In-context RM attribute path resolution (REQ-121).** `openehr/rm/rmpath` now resolves the `EVENT_CONTEXT`, `ACTIVITY.timing` and `ACTION.time` attributes the Web Template synthesizes, fixing a silent FLAT encode drop of template-constrained `other_context` data, with a guard test pinning the class.
- **Ordinal symbol-code parse (REQ-103).** `openehr/template` now also accepts the `symbol/defining_code` nesting the AOM 1.4 XSD mandates, so a `CDvOrdinal`'s coded term is populated instead of silently empty.
- **Upstream FLAT serialisation conformance (REQ-080, PROBE-086).** `testkit/conformance/webtemplate` round-trips the pinned EHRbase FLAT corpus through the REQ-053 codec, exact on the modelled subset (10.5% of upstream keys) with the rest counted per fixture and zero coverage failing.

## [0.16.0] - 2026-07-13

Sixteenth `v0.x` minor — closes two consumer-filed gaps: REST path parameters are now percent-encoded exactly once (fixing a real 404 for ids containing a space, e.g. a spaced template id) and a class standing predicate exposes its relative path as structured segments. Additive API — the only integrator-visible change is the corrected single-encoded wire URL for an id carrying a percent-encodable character; safe to upgrade.

### Added

- **Single-encoded REST path parameters (REQ-095).** Every `openehr/client` leaf client now interpolates the raw id and lets the transport — the single canonical path encoder — escape it once, fixing a double-encode that 404'd template ids containing a space; guarded tree-wide against re-introduction.
- **Structured class-predicate path (REQ-113).** A class standing predicate's `Comparison.ParsedPath` is now populated with an empty alias and structured segments, mirroring the WHERE side, so a consumer reads the relative path's segments without re-splitting the raw `Path`.

## [0.15.0] - 2026-07-12

Fifteenth `v0.x` minor — internal consolidation onto generated and shared surfaces: a generated LOCATABLE identity surface + reverse type registry (ADR 0013; REQ-031/040), the `rm.ObjectIDValue` lexical helper (REQ-120), and shared versioned-write client plumbing (REQ-094). Additive only — no integrator-visible behaviour or API break from v0.14.0; safe to upgrade.

### Added

- **Generated LOCATABLE identity surface (ADR 0013; REQ-031, REQ-040).** `bmmgen` emits `Get*/Set*` identity accessors widening the sealed `rm.Locatable` plus `rm.MutableLocatable`, and the reverse registry (`rm.RMTypeName`, `rm.IsTypedNil`, `typereg.Names`), retiring the hand-maintained identity switches.

- **`rm.ObjectIDValue` lexical helper (REQ-120).** Public `(string, bool)` accessor for any concrete `OBJECT_ID` (value or pointer), alongside the existing `UIDValue`; retires the private, behaviourally-identical switch in `openehr/client/ehr`.

- **Shared versioned-write client plumbing (REQ-094).** `openehr/client/ehr` gains exported generic write-result, write-config, and delete-tail helpers consolidating the four leaf clients' duplicated Prefer-decode and write-option internals; leaf public APIs and error strings unchanged.

## [0.14.0] - 2026-07-02

Fourteenth `v0.x` minor — two consumer-driven gaps closed on the RM-floor and AQL-parse surfaces: presence-aware EHR_STATUS validation (REQ-112) and structured access to AQL standing-predicate + WHERE paths (REQ-113). Additive new public API; one minor source break — `parse.IdentifiedPath` now embeds the relocated `aql.IdentifiedPath`, so field reads are unchanged but composite-literal construction must wrap the embedded struct.

### Added

- **Presence-aware EHR_STATUS RM-floor entry (REQ-112).** New `validation.ValidateRMEHRStatusBytes` flags an omitted value-typed mandatory `subject` via JSON-key presence, which the value-based `ValidateRMEHRStatus` cannot (a bare `PARTY_SELF` decodes identically to an absent one).
- **Structured AQL path access (REQ-113).** `parse.ClassExpr.PredicateComparison` exposes a class standing predicate as a structured `aql.Comparison`, and `aql.Comparison.ParsedPath` exposes a WHERE path's alias+segments; the shared `aql.IdentifiedPath`/`PathSegment` vocabulary moved into `openehr/aql` (parse-side names preserved); emission/round-trip unchanged.

## [0.13.0] - 2026-07-01

Thirteenth `v0.x` minor — raises the minimum Go toolchain to the current stable line (Go 1.26, `go 1.26.0` floor; REQ-002). Toolchain, build, and docs only — no openEHR API, wire, or behaviour change from v0.12.0; upgrade once your build runs Go ≥ 1.26.

### Added

- **Go toolchain minimum raised to Go 1.26 (`go 1.26.0` floor) (REQ-002).** `go.mod`, the Docker/Make toolchain, and CI now track the current stable Go line (N) instead of N-1; consumers must build with Go ≥ 1.26.0.

## [0.12.0] - 2026-07-01

Twelfth `v0.x` minor — three inbound CDR-dossier gaps closed: template-less RM validation (REQ-112), stored-query / query REST conformance (REQ-055/057), and the execution-oriented parsed AQL AST (REQ-113), plus the repo-wide modernization sweep. Additive across the board — no integrator-visible behaviour changes from v0.11.0; the WHERE / Value vocabulary types are now exported on `openehr/aql` so a parsed query is introspectable.

### Added

- **Template-less RM validation floor (REQ-112).** `validation.ValidateRM` + typed sugars (`ValidateRMFolder`/`EHRStatus`/`EHRAccess`/`Demographic`) walk any RM root with the BMM as sole driver and report RM-mandatory absences plus a per-type invariant catalogue (CODE_PHRASE, DV_QUANTITY, DV_INTERVAL bounds, OBJECT_REF type/namespace); template-driven path unchanged.
- **Stored-query/query REST conformance (REQ-055/057).** POST query execution now scopes via the spec's `openehr-ehr-id` request header (verb-aware: GET still uses the `ehr_id` query parameter); `PutStoredQuery` recovers the assigned `{name, version}` from the `Location` response header before falling through to body decode then synthesised metadata.
- **Execution-oriented parsed AQL AST (REQ-113).** Read-side `parse.Query` AST mirrors `aql.Builder`; `WhereExpr`/`Value` vocabulary unified across read and write sides; `Emit` closes the round-trip loop. Out-of-catalogue shapes surface as `aql.ErrIncompleteAST`.
- **Modernization sweep (lint, tests, idioms).** `.golangci.yml` enables `revive` with a curated rule set; tests across `auth/`, `smart/`, `transport/`, `openehr/client/*` and `testkit/probes/*` adopt `t.Context()` instead of `context.Background()`; benchmark loops adopt `b.Loop`; `sort.*` replaced with `slices.Sort`/`SortFunc`. No public-API change.

## [0.11.0] - 2026-06-24

Eleventh `v0.x` minor — polymorphic `_type` round-trip stability (REQ-052) and seeded synthetic value generation (REQ-107), plus the ITS-REST conformance remediation. Additive new `instance`/`composition` value-fill API; one integrator-visible change — `composition.Get` returns the typed `ErrDeletedAtTime` on a 204 deleted read. Safe to upgrade from `v0.10.0` after that note.

### Added

- **ITS-REST conformance remediation (REQ-059/093/095/099).** Corrected wire-level deviations in the landed REST clients against the vendored OpenAPI contract (headers, verbs, paths, status mapping) and vendored the EHRbase specs (`resources/ehrbase/`, Apache-2.0); see [plan](docs/plans/archive/2026-06-19-its-rest-conformance-remediation.md). Integrator note: `composition.Get` now returns the typed `ErrDeletedAtTime` (not a nil error) on a 204 deleted-at-time read.
- **Polymorphic `_type` round-trip stability (REQ-052/102).** Value-in-interface RM fields now emit `_type` on encode (new `openehr/internal/jsonpoly`) and a round-tripped `DV_INTERVAL<T>` re-validates from its bounds' runtime types, restoring strict round-trip template validation. [plan](docs/plans/archive/2026-06-23-polymorphic-encode-decode.md).
- **Seeded synthetic value fill (REQ-103/107).** New orthogonal `instance.ValueFill` (`RandomFill`) + `ValueSource` seam — surfaced as `composition.WithValueFill`/`WithValueSource` — draws in-constraint, varying, reproducible leaf values; `medium` detail_level deferred. [plan](docs/plans/archive/2026-06-23-seeded-synthetic-generation.md).

## [0.10.0] - 2026-06-19

Tenth `v0.x` minor — RM behavioural functions (REQ-120..123) and real-world OPT synthesis/validation coverage (REQ-102/107/110). Additive; one minor `ehr.VersionUID` partial-segment change. Safe to upgrade from `v0.9.0`.

### Added

- **RM behavioural functions (REQ-120..123).** Hand-written `Parse*` identifiers, `VERSION.is_branch`, temporal `DV_*` read/compare/convert, and locatable-path reads (new `openehr/rm/rmpath`) replace the `bmmgen` panic stubs ([ADR 0011](docs/adr/0011-rm-behavioural-functions-surface.md)); `ehr.VersionUID` now returns `""` for non-canonical uids.
- **Real-world OPT synthesis & validation coverage (REQ-102/107/110).** Synthesiser and validator now handle `ELEMENT`/`CLUSTER.name`, generic `DV_INTERVAL<T>`, and colliding optional content roots, exercised by a vendored real-world OPT corpus (PROBE-027 extended).

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

Third `v0.x` minor — RM polymorphic decode coverage (REQ-052), `Get*` accessors on the narrow `*Like` interfaces, and rounded-out testkit cassettes. One breaking change (field-type lift on substitution slots).

### Added

- **RM polymorphic decode coverage (REQ-052).** `bmmgen` emits narrow `<Parent>Like` interfaces (`DVTextLike`, `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`) so concrete-typed RM slots admit substitution (`LOCATABLE.name` carrying `DV_CODED_TEXT` round-trips losslessly); `DV_INTERVAL[T: DV_ORDERED]` decodes via typereg. *Breaking:* those fields are now interfaces — migrate via `rm.DVTextValueOf`/`rm.AsDVText`/`rm.AuditDetailsBase`. PROBE-038.
- **`*Like` interface ergonomics.** Narrow interfaces expose `Get*` accessors (`GetValue`/`GetDefiningCode`, `GetSystemID`/`GetTimeCommitted`/`GetChangeType`/…, `GetName`/`GetIdentifiers`/…). Additive; pre-existing `rm.DVTextValueOf`/`rm.DVURIValueOf` stay as compat shims. See [`openehr/rm/doc.go`](openehr/rm/doc.go).
- **Testkit cassettes** — ehrbase Robot fixtures (minimal-entry, `Test_dv_*`, EHR_STATUS, FOLDER, persistent compositions, CONTRIBUTION) under `testkit/cassettes/` with `fixtures.SubmissionJSON` and `scripts/ingest-robot-cassettes.sh`.

## [0.2.0] - 2026-05-26

Second `v0.x` minor — the Contribution write path, AOM 1.4 primitive wrappers, and tag-driven release tooling. One breaking change (`contribution.Commit` signature, no in-tree callers).

### Added

- **C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission** — AOM 1.4 primitive short-name wrappers flow through; `Composition.uid` emits `_type:"HIER_OBJECT_ID"`; PROBE-023 widened to full round-trip.
- **Contribution submission shape (REQ-050/095)** — `contribution.Commit` now takes `*Submission` (ITS-REST `Contribution_create`: inline `ORIGINAL_VERSION`/`IMPORTED_VERSION` with `data: T`), not `*rm.Contribution`. *Breaking;* no in-tree callers. PROBE-072.
- **Repo hygiene + release workflow** — issue/PR templates, refined `CONTRIBUTING.md`/`SECURITY.md`; tag-driven [`release.yml`](.github/workflows/release.yml) with auto-generated compatibility table.

## [0.1.0] - 2026-05-26

First tagged release — the openEHR-first Go SDK adoption slice: REST 1.1.0-development client family + auth + discovery + canonical codecs + the ADL 1.4 template/validation/instance/composition stack.

### Added

- **Module scaffolding & process** — module layout, Makefile/Docker toolchain, AI workflow docs, CI (REQ-001..005); SDD tree at [`docs/specifications/`](docs/specifications/) (REQ/PROBE/STRAND registry + [`traceability.yaml`](docs/specifications/traceability.yaml) + `make spec-check`); plans; CONTRIBUTING/SECURITY/releases; ADRs [0001–0005](docs/adr/).
- **BMM & codegen** — pinned BMM corpus (`base_1.3.0`, `rm_1.2.0`, `am_1.4.0`, `am_2.4.0`, `lang_1.1.0`, `term_3.1.0`); BMM loader (REQ-045); generated RM + AOM 1.4 types with `typereg` (REQ-040..047); `bmmgen`/`bmmdiff` + weekly drift workflow ([ADR 0001](docs/adr/0001-bmm-version-bump-runbook.md)); BMM-driven RM lookup at `openehr/rm/rminfo/`.
- **Serialization** — canonical JSON (REQ-052) and canonical XML (REQ-056) codecs with `xsi:type` polymorphic dispatch and a cross-format JSON↔XML invariant.
- **Transport, auth, discovery** — `transport/` with openEHR custom headers (REQ-059), retry (REQ-091, `NoRetry`), OTel hooks (REQ-090), error-envelope mapping (REQ-093), `Prefer` (REQ-094), `Idempotency-Key` (REQ-097), request `Observer` (REQ-098); auth providers `auth/{clientcreds,jwtbearer,basic,smart}` (REQ-060..063, REQ-069); SMART-on-openEHR with JWKS-validated ID tokens (REQ-064, REQ-067); service discovery (REQ-070..072).
- **REST clients** — System; EHR read/write (`composition`/`ehrstatus`/`directory`/`contribution`/`itemtags`, REQ-050..057, 059); Definition (ADL 1.4 templates + stored AQL, REQ-057); AQL Query (REQ-055); Admin (REQ-099). `Prefer: return=representation` bodies decode as bare RM types (REQ-094).
- **Clinical modeling** — OPT parser at `openehr/template/` (REQ-100); compiled walker tree at `internal/templatecompile/` ([ADR 0005](docs/adr/0005-compiled-template-foundation.md)); primitive constraints (REQ-103); composition validator at `openehr/validation/` (REQ-102); RM instance generator (REQ-107); generic composition builder at `openehr/composition/` (REQ-101).
- **Cadasto extras** — `cadasto/admin/` Live/Ready health probes (REQ-083).
- **Conformance probes** (`testkit/probes/`) and **worked examples** (`cmd/examples/`: `canonical_json`, `canxml_roundtrip`, `ehr_create`, `opt-parse`, `validate-composition`, `validate-from-json`, `primitive-validate`, `generate-example`).

### Compatibility

| Concept | Value |
|---|---|
| Go toolchain (minimum) | `1.25.0` |
| openEHR REST | `1.1.0-development` |
| BMM corpus | `openehr_base_1.3.0`, `openehr_rm_1.2.0`, `openehr_am_1.4.0`, `openehr_am_2.4.0`, `openehr_lang_1.1.0`, `openehr_term_3.1.0` |

### Known follow-ups (not landed at 0.1.0)

- [REQ-094 write-path gaps](docs/plans/archive/2026-05-25-req094-prefer-followups.md) · AQL verb builders ([plan](docs/plans/archive/2026-05-21-aql-builders.md)) · Demographic REST client ([plan](docs/plans/archive/2026-06-14-demographic-rest-client.md)) · Benchmark harness ([plan](docs/plans/archive/2026-05-15-rest-api-client.md)).
