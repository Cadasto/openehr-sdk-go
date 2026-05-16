# Changelog

All notable changes to `github.com/cadasto/openehr-sdk-go` are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entries under `## [Unreleased]` are **short and high-level**: one-line bullets naming the artefact class and scope. File-level detail belongs in commit messages and PR bodies — see `AGENTS.md` > Code style and conventions.

## [Unreleased]

### Added

- CI: GitHub Actions PR/push workflow ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) — parallel verify, test, and lint jobs; race detector on `main` pushes; Dependabot for `gomod` and Actions.
- Contributor CI guide ([`docs/ci.md`](docs/ci.md)); Makefile targets `fmt-check`, `mod-tidy-check`, `lint-ci`, and `ci` (full local PR gate).
- [`.golangci.yml`](.golangci.yml) (golangci-lint v2.11.4, aligned with Makefile `LINT_IMAGE`).

### Changed

- Pinned BMM schemas moved to `resources/bmm/`; codegen defaults and docs updated.
- Module path normalized to lowercase `github.com/cadasto/openehr-sdk-go` across code, specs, and docs (REQ-001).
- Type registry (`openehr/rm/typereg/`): exported sentinel errors `ErrMissingType`, `ErrUnknownType`, `ErrTypeMismatch`; `Decode` / `DecodeAs` now wrap these for `errors.Is` (PROBE-031).

### Added

- Canonical JSON codec scaffolding (`openehr/serialize/canjson/`): `Marshal`, `MarshalIndent`, package doc pinning the deterministic wire profile (REQ-052).
- Shared polymorphism helpers (`openehr/serialize/internal/poly/`): `ResolveType`, `DecodeError` envelope wrapping typereg sentinels.
- Vendored canonical-JSON cassettes (`testkit/cassettes/canonical_json/`) with provenance README — used by codec and probe tests (REQ-082).
- BMM generator emits per-class canonical-JSON `MarshalJSON` companions (`openehr/rm/*_jsonmar_gen.go`, `openehr/aom/aom14/*_jsonmar_gen.go`): `_type` first, fields in encoding/json declaration order, generator-managed wire structs with descendant-shadows-ancestor handling.
- Canonical JSON decoder (`openehr/serialize/canjson/`): `Unmarshal`, `NewDecoder` / `Decode`, `WithRelaxedTypeDispatch` (strict-default), `DecodeError` (alias of `typereg.DecodeError`), `ErrInvalidShape`. Decoder tests cover leaf decode, polymorphic dispatch on `Composition.content` and `composer`, unknown / missing `_type` wrapping `typereg.ErrUnknownType` / `ErrMissingType`, and `DecodeError.Path` (JSON-pointer-ish) for locating failures.
- BMM generator emits per-class canonical-JSON `UnmarshalJSON` companions (`openehr/rm/*_jsonunmar_gen.go`) for every concrete class with at least one polymorphic field (single or container). Per-field dispatch via `typereg.DecodeAs[T]`; failures return `*typereg.DecodeError` with a JSON-pointer-ish path.
- `typereg.DecodeError` envelope: `Path`, `Type`, `Inner` (Unwrap-compatible); shared with the future canonical-XML codec.
- Canonical-JSON round-trip tests on simple RM values and a no-history Composition (byte-stability + structural equivalence); cassette round-trip test guards a known limitation (abstract-generic `Event[T]` is rendered as a Go struct, blocking history-bearing fixtures until STRAND-04).
- Conformance probes (`testkit/probes/serialize/`): PROBE-030 (canonical-JSON round-trip stability) and PROBE-031 (`_type` not in registry wraps `typereg.ErrUnknownType`).
- Canonical-JSON edge-case tests (`openehr/serialize/canjson/`): null⇄absent equivalence on decode/encode, ISO 8601 string passthrough, empty-container omitempty, deep recursive FOLDER decode.
- Canonical-JSON benchmarks (`openehr/serialize/canjson/bench_test.go`): leaf-type (DV_QUANTITY) and width-400 Composition encode/decode; baseline for STRAND-04 codec-perf sub-strand.
- REST API client implementation plan (`docs/plans/2026-05-15-rest-api-client.md`): ten-phase build for `openehr/client/{system,ehr,query,definition,demographic,admin}` over `transport/`; integrates `auth.TokenSource`, `smart/discovery.ServiceCatalog`, and the canonical JSON/XML codecs; closes STRAND-01 (CDR-extraction milestone).
- Wire-binding spec extensions: REQ-059 (openEHR custom header family — `openehr-version`, `openehr-audit-details`, `openehr-template-id`, `openehr-uri`, `openehr-item-tag`), REQ-093 (openEHR error envelope mapping), REQ-094 (`Prefer` return-shape negotiation), REQ-095 (upstream OpenAPI YAML as authoritative endpoint source). New `specs/wire.md` sections: functional API areas (six groups), authoritative OpenAPI source, custom header family, `Prefer` negotiation, error envelope.
- `specs/module-layout.md` — added `openehr/client/admin/` and EHR sub-leaves (composition, contribution, directory, ehrstatus, itemtags).
- `specs/glossary.md` — entries for `Prefer`, `openehr-audit-details`, openEHR custom header family, error envelope; ItemTags and ITS-REST entries expanded.
- BMM version-bump tooling (`cmd/bmmdiff`, `internal/bmmdiff/`): semantic diff between two BMM files (added/removed classes, per-class property and function changes, cardinality changes, primitives) plus a one-line CHANGELOG-suggestion helper. Weekly CI drift bot (`.github/workflows/codegen-drift.yml`) re-runs `make codegen-verify` on a clean checkout and posts to a `bmm-drift` tracking issue on failure. Version-bump runbook landed as [`docs/adr/0001-bmm-version-bump-runbook.md`](docs/adr/0001-bmm-version-bump-runbook.md); `resources/README.md § Updating` now defers to it. Simulated-bump integration test in `internal/bmmgen/sim_bump_test.go` exercises the end-to-end regen path (Phase 5).
- BMM code generator (`internal/bmmgen/`, `cmd/bmmgen/`): deterministic types-only emission of the openEHR RM under `openehr/rm/` from pinned BMM sources; one-file-per-BMM-package flat layout; `-verify` drift detection and atomic writes; `make codegen` and `make codegen-verify` (the latter chained into `make test`) (REQ-042, REQ-043, REQ-046).
- Generated method stubs for openEHR RM class functions (`render_function.go`): every BMM `function` becomes a panicking Go method with BMM `documentation`, `pre_conditions`, `post_conditions`, and operator `aliases` propagated as Go-doc comments; abstract-class functions are emitted on each concrete descendant (REQ-044).
- Multi-target generation in `internal/bmmgen` (new `Target` type with `TargetRM` and `TargetAOM14`); CLI `-target` flag; generator now drives both targets by default. `openehr/aom/aom14/` is generated alongside `openehr/rm/`, sharing base types via a one-way `aom14 → rm` import (Option C) and the shared `openehr/rm/typereg.Default` registry.
- Generated openEHR AOM 1.4 (`openehr/aom/aom14/*_gen.go`): 6 package files + `typereg_gen.go`, covering all 39 AOM 1.4 classes (`ARCHETYPE`, `C_OBJECT`, `C_ATTRIBUTE`, `ARCHETYPE_SLOT`, etc.) plus 122 method stubs; mutual-recursion break via `CyclicSingleProps` for the `ARCHETYPE ↔ ARCHETYPE_ONTOLOGY` pair (REQ-042).
- Type registry primitive (`openehr/rm/typereg/`): goroutine-safe `Registry` with `Default`, panic-on-duplicate `Register`, polymorphic `Decode` and generic `DecodeAs[T]` (REQ-040).
- Generated openEHR RM (`openehr/rm/*_gen.go`): concrete structs with embedded ancestors and snake_case JSON tags, abstract `is<X>()` marker interfaces, generic `Interval[T]` / `DVInterval[T]`, primitive type aliasing per `specs/bmm-conformance.md`; ~99 concrete types registered with `typereg.Default` via generated `init()`. EHR Extract, foundation builtins and functional packages skipped per spec.
- BMM loader (`openehr/bmm/`): public building-block `Load` / `LoadAll` with `Resolver`, `FSResolver`, `MapResolver`; full P_BMM persistence model with discriminator-driven decoding and round-trippable polymorphic marshalling; descendant-shadows-ancestor include merge with sibling-conflict detection (REQ-045).
- Repository scaffolding: module layout, package-level `doc.go` stubs for every planned sub-package.
- AI-assistant documentation set: `AGENTS.md`, `.claude/CLAUDE.md`, `docs/architecture.md`, `docs/ai-workflow.md`.
- Normative specifications tree under `specs/`: requirements (REQ-NNN), glossary, scope, module layout, idiom, RM modeling, auth, wire format, service discovery, conformance probes (PROBE-NNN), use cases, research strands (STRAND-NN).
- Spec extensions covering Cadasto platform requirements not in the original SDK proposal: canonical XML (REQ-056), stored AQL queries (REQ-057), Datamap V2 (REQ-058), per-client tenant binding (REQ-065), AI caller attribution (REQ-066), platform principal claims (REQ-067), full SMART flow + launch-mode coverage (REQ-068), TLS posture (REQ-092); corresponding PROBE-008 and PROBE-009.
- Pinned openEHR BMM schemas under `resources/` (base 1.3.0, rm 1.2.0, am 1.4.0 as **primary** v1 inputs; am 2.4.0, lang 1.1.0, term 3.1.0 kept as **deferred**) with provenance and update runbook (`resources/README.md`).
- BMM-conformance specification (`specs/bmm-conformance.md`) and REQs REQ-041..047 — pinned BMM sources, generator + drift detection, P_BMM → Go mapping rules, primitive type mapping.
- Package stubs for the BMM ecosystem: public `openehr/bmm/` loader, internal `internal/bmmgen/` generator, `cmd/bmmgen/` CLI placeholder.
- AOM placed as sibling of RM at `openehr/aom/aom14/` (not under `openehr/template/`) — rationale in `specs/bmm-conformance.md`.
- Implementation plan for the generator: `docs/plans/2026-05-15-bmm-codegen.md` (5 phases for v1; AOM 2, LANG, TERM, RM EHR Extract deferred).
- Build tooling: `Makefile` (host Go fast path, Docker fallback), `Dockerfile` (`dev` stage), `docker-compose.yml` (`dev` profile).
- `gofmt`-on-save hook for Claude Code.
