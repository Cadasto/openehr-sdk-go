# Plan — BMM-driven code generation for openEHR domain types

**Date:** 2026-05-15
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-041, REQ-042, REQ-043, REQ-044, REQ-045, REQ-046, REQ-047
**Resolves:** part of STRAND-04 (RM polymorphism + codec performance — the polymorphism side)

## Goal

Generate the openEHR domain-model Go code from the pinned **primary** BMM schemas under `resources/bmm/` (REQ-041 table), with reproducible output and CI drift detection. Hand-written method bodies and helpers live in clearly separated companion files (`<file>_ext.go`).

**v1 generation targets:**

- `openehr/rm/` — from `openehr_rm_1.2.0` + `openehr_base_1.3.0`, **excluding** the `org.openehr.rm.ehr_extract` package.
- `openehr/rm/typereg/` — type registry covering generated `openehr/rm/` concrete types.
- `openehr/aom/aom14/` — from `openehr_am_1.4.0` + `openehr_base_1.3.0`. **Sibling of `openehr/rm/`** (not under `template/`) — see [bmm-conformance.md § Schema → Go package set](../../specs/bmm-conformance.md#schema--go-package-set) for the rationale.

**Out of v1 (BMM files kept in `resources/bmm/`, no generated code):** AOM 2, RM EHR Extract package, LANG, TERM. Each becomes a future plan phase when a consumer demands it.

## Why now

- The BMM files are the openEHR Foundation's computable form of the spec — using them as source-of-truth eliminates one whole class of transcription drift.
- The Cadasto PHP `openehr-bmm` library already validates this approach for PHP; mirroring it in Go gives cross-language conformance for free.
- The RM has ~146 classes; AM 2 has ~75. Hand-writing them is **possible** but locks in transcription errors and creates a maintenance burden every BMM micro-release.
- The CDR-extraction MVP (Phase 1 per [specs/use-cases.md § POC extraction](../../specs/use-cases.md#poc-extraction-scope)) needs real RM types. Writing them by hand is the wrong starting point if generation is also viable.

## Out of scope

- Generating wire codecs (`openehr/serialize/`) — those are hand-written; the codec consults the generated types and the generated type registry.
- Generating validators (`openehr/validation/`) — also hand-written.
- Implementing function bodies (`DV_QUANTITY.add`, `is_strictly_comparable_to`, etc.). The generator emits **method stubs** with documentation; bodies are hand-implemented as needed (see REQ-044).
- Generating archetype / template parsers. The AOM **types** are generated; the ADL / OET parser is hand-written and consumes the AOM types.
- Generating from anything other than the pinned BMM files (no network fetch, no on-the-fly schema discovery).

## Phases

The plan is sequenced so each phase delivers a runnable artefact and the SDK build stays green throughout. **Five phases for v1.**

| Phase | Title | Status |
|---|---|---|
| 1 | BMM loader (`openehr/bmm/`) | not started |
| 2 | Code generator skeleton + drift detection | not started |
| 3 | Function stubs and documentation | not started |
| 4 | AOM 1.4 generation | not started |
| 5 | Drift bot + version-bump runbook | not started |

Deferred to a later plan: AOM 2 generation, LANG generation, TERM generation, RM EHR Extract generation. The BMM files for these stay in `resources/bmm/` and the conformance contract continues to apply when those phases are picked up.

### Phase 1 — BMM loader (`openehr/bmm/`)

**Outcome:** A public, reusable, building-block BMM loader. No code generation yet.

**Tasks:**

1. Implement `bmm.Schema`, `bmm.Package`, `bmm.Class`, `bmm.Property`, `bmm.Type`, `bmm.FunctionParameter` as plain Go structs mirroring the P_BMM persistence shape — one struct family per `_type` variant:
   - Property variants: `SingleProperty`, `SinglePropertyOpen`, `ContainerProperty`, `GenericProperty` — implement a common `Property` interface (with `isProperty()` marker — see `rm-modeling.md` § Abstract categories).
   - Type variants: `SimpleType`, `GenericType`, `ContainerType` — implement a common `Type` interface.
   - Class variants: `SimpleClass`, `GenericClass`, `Enumeration` (with string or int item codes), `Interface`.
   - Function parameter variants: `SingleFunctionParameter`, `SingleFunctionParameterOpen`, `GenericFunctionParameter`, `ContainerFunctionParameter`.
2. Implement `bmm.Load(r io.Reader) (*Schema, error)`. Uses `encoding/json` with a custom decoder that dispatches on `_type` per the type registry pattern (REQ-040 applied here too).
3. Implement `bmm.LoadAll(rootID string, resolver Resolver) (*Schema, error)`. Follows `includes` transitively. The merge semantics: ancestor schemas contribute primitive types and base classes; descendant schemas contribute domain classes. Conflicts (same class name in two schemas) **MUST** error explicitly.
4. Implement a `FSResolver` (reads from `resources/bmm/`) and a `MapResolver` (reads from an in-memory map) for tests.
5. Tests:
   - Load each of the 6 BMM files individually; assert expected class counts, package counts, primitive type counts.
   - Load `openehr_rm_1.2.0` with `includes` resolution; assert that primitives from `openehr_base_1.3.0` are merged.
   - Round-trip test: load → marshal → reload → equal model.
   - Negative tests: malformed JSON, unknown `_type`, missing required field, circular includes.

**Definition of done:**

- `go test ./openehr/bmm/...` passes.
- `go doc github.com/cadasto/openehr-sdk-go/openehr/bmm` shows the public API.
- Loader is consumed in Phase 2 without modification.
- Lines of code budget: ~1500–2000 lines including tests.

### Phase 2 — Code generator skeleton (`internal/bmmgen`, `cmd/bmmgen`)

**Outcome:** A working generator that emits a deterministic **one-file-per-package** scaffold for `openehr/rm/` — types only, no methods yet. Drift detection wired in CI.

**Tasks:**

1. Implement the emit pipeline in `internal/bmmgen`:
   - **Resolver step:** load the BMM via `openehr/bmm`, resolve includes, build a flat class table.
   - **Plan step:** group classes by BMM package; emit one Go file per BMM top-level package (`org.openehr.rm.data_types.quantity` → `openehr/rm/data_types_quantity_gen.go`).
   - **Render step:** Go `text/template` templates per construct (struct, interface, enum, type registry). Run `go/format` on the output to canonicalise.
   - **Write step:** write atomically; preserve hand-written companion files (`*_ext.go`) untouched.
2. Mapping coverage in this phase:
   - Concrete classes → structs with embedded ancestors + JSON tags. (REQ-031)
   - Abstract classes → interfaces with `is<X>()` marker. (REQ-032)
   - Enumerations → typed `string` / `int` with const block.
   - Single properties (mandatory → `T`, optional → `*T`).
   - Container properties → `[]T` or `map[K]V` per the table in [bmm-conformance.md § Container mapping](../../specs/bmm-conformance.md#container-mapping).
   - Generic properties → generic instantiation `Foo[Bar]`.
   - Open generic parameters → typed via the surrounding class's type parameter.
   - Primitive type mapping → per [bmm-conformance.md § Primitive type mapping](../../specs/bmm-conformance.md#primitive-type-mapping).
3. Emit `openehr/rm/typereg_gen.go` (and `openehr/aom/aom14/typereg_gen.go`): an `init()` that registers every concrete `_type` with its constructor. **Path note:** the original plan called for `openehr/rm/typereg/registry_gen.go`; the canonical location is one level up to avoid a `typereg → rm` import cycle (see ADR 0002 D3 / [`../adr/0002-bmm-codegen-decisions.md`](../adr/0002-bmm-codegen-decisions.md)).
4. Implement `cmd/bmmgen` flags: `-resources`, `-out`, `-verify` (CI mode — diff against working tree, exit non-zero on drift).
5. CI step: add a `make codegen-verify` target that runs `go run ./cmd/bmmgen -verify`. Wire into the existing `make test` pipeline.
6. Tests for the generator:
   - Golden file tests for a small subset (e.g. `data_types_quantity` package). The generated file is checked into the repo; the test re-generates and `diff`s.
   - Validation that running the generator twice produces byte-identical output.
   - Validation that a deliberate manual edit to a generated file fails `-verify`.

**Definition of done:**

- `go test ./internal/bmmgen/...` passes.
- `go run ./cmd/bmmgen` produces output that compiles (`go build ./...` passes).
- `go run ./cmd/bmmgen -verify` exits zero on a fresh checkout, non-zero after a hand-edit to a `*_gen.go` file.
- CI runs `-verify` and fails on drift.

### Phase 3 — Function stubs and documentation

**Outcome:** Generated method stubs for class functions (`DV_QUANTITY.add`, `is_strictly_comparable_to`, etc.) with the BMM `documentation`, `pre_conditions`, and `post_conditions` reflected as Go doc comments. Method bodies remain unimplemented — they panic with a clear message.

**Tasks:**

1. Extend the templates to emit method declarations with proper Go-doc style — first sentence from the BMM `documentation`, subsequent paragraphs preserved.
2. Translate `pre_conditions` and `post_conditions` to `// Pre:` and `// Post:` comments verbatim (these are short OCL-ish expressions in the BMM, e.g. `is_strictly_comparable_to (other)`).
3. Bodies: `panic("not implemented: <Class>.<method> — implement in a non-generated file")`. The companion `<file>_ext.go` is where consumers/maintainers add real bodies as needed.
4. Operator aliases (e.g. `"aliases": ["+"]` on `DV_QUANTITY.add`) are emitted as Go doc comments only — Go does not support operator overloading.

**Definition of done:**

- The generated code documents every BMM function present in the source.
- A panicking method that nobody implemented is a clear, debuggable signal — not a silent fallback to a wrong value.

### Phase 4 — AOM 1.4

**Outcome:** Generated AOM 1.4 type set under `openehr/aom/aom14/` — sibling of `openehr/rm/`.

**Tasks:**

1. Re-run the generator on `openehr_am_1.4.0.bmm.json` + `openehr_base_1.3.0.bmm.json`.
2. Resolve sub-package layout — emit one Go file per AOM 1.4 BMM package under `openehr/aom/aom14/`.
3. The package sits **next to** `openehr/rm/`, not inside `openehr/template/`. Rationale (per [bmm-conformance.md § Schema → Go package set](../../specs/bmm-conformance.md#schema--go-package-set)): AOM is the model of an *archetype*; templates *consume* archetypes (an OPT contains flattened archetype definitions). The future `openehr/template/` parser will import `openehr/aom/aom14/`, but does not own it.
4. Tests: same golden-file approach as Phase 2; targeted at a small AOM subset (e.g. `ARCHETYPE`, `C_OBJECT`, `C_ATTRIBUTE`, `ARCHETYPE_SLOT`).

**Definition of done:**

- `openehr/aom/aom14/` compiles and passes golden-diff checks.
- The (later) ADL 1.4 parser in `openehr/template/` can construct AOM 1.4 values without further generation.

**Deferred (not in v1):**

- **AOM 2** (`openehr_am_2.4.0`) — no v1 consumer. The BMM file stays in `resources/`. When wired in, the package will be `openehr/aom/aom2/` (parallel to `aom14`).
- **LANG** code generation (`openehr_lang_1.1.0`) — the BMM meta-classes used by `openehr/bmm/` are hand-written for v1; auto-generating them is a future option.
- **TERM** (`openehr_term_3.1.0`) — would target `openehr/rm/terminology/`. No v1 consumer; integration happens directly with deployment-specific terminology backends.
- **RM EHR Extract** (`org.openehr.rm.ehr_extract` package inside `openehr_rm_1.2.0`) — the generator skips this BMM package in Phase 2.

### Phase 5 — Drift bot + version-bump runbook

**Outcome:** Operational tooling that makes BMM version bumps low-risk.

**Tasks:**

1. CI weekly job: re-run the generator on a fresh checkout and post a comment to a tracking issue if drift would occur (catches accidental hand-edits or generator-template changes).
2. Runbook in [`../adr/`](../adr/) or this plan describing how to bump a BMM file: drop new version next to old, run generator, inspect diff, update [bmm-conformance.md](../../specs/bmm-conformance.md) and [`../../resources/bmm/README.md`](../../resources/bmm/README.md), CHANGELOG entry, remove old file in same PR.
3. Optional: a small `cmd/bmmdiff` tool that compares two BMM files and prints a human-readable "what changed" diff (added classes, removed classes, property changes per class, cardinality changes). Distinct from `git diff` because it understands the BMM structure.

**Definition of done:**

- A simulated version bump (e.g. fake `openehr_rm_1.2.1.bmm.json` with one added property) regenerates without manual intervention; the diff in `openehr/rm/` is small and reviewable; tests pass; CHANGELOG entry template is auto-suggested.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Generator emits Go code that doesn't compile for some BMM edge case | Golden-file tests on a representative subset; CI verifies the entire RM compiles after each generator change. |
| BMM file declares something the mapping rules don't cover | The generator fails loudly with the unhandled construct named. Then either: a) extend the mapping rules + bmm-conformance.md, or b) raise an upstream issue if it's a BMM bug. |
| Hand-written method bodies in `*_ext.go` get out of sync with a regenerated `*_gen.go` (e.g. a BMM-renamed parameter) | The `-ext.go` file fails to compile after regeneration. CI catches it. |
| AOM 1.4 and (future) AOM 2 share enough naming that consumers mix them | Separate Go packages (`aom14` vs `aom2`). Cross-imports prohibited. AOM 2 is deferred for v1; the layout already reserves the slot. |
| Generated documentation becomes stale because the BMM upstream is slow to fix typos | Documented in REQ-047: the BMM is authoritative; upstream the fix. The SDK does not patch documentation in-tree. |
| The generated RM is too large for a typical consumer (build time, IDE load) | Acceptable cost. Each top-level BMM package is its own Go file, so go tooling can prune. |

## Mapping to specs

- [specs/bmm-conformance.md](../../specs/bmm-conformance.md) is the contract this plan implements.
- [specs/rm-modeling.md](../../specs/rm-modeling.md) defines the Go shape the generator emits.
- [specs/module-layout.md](../../specs/module-layout.md) places `openehr/bmm/`, `internal/bmmgen/`, `cmd/bmmgen/` in the package taxonomy.
- [specs/idiom.md § Generics policy](../../specs/idiom.md#generics-policy) governs how generic classes (`Interval<T>`, `DVInterval<T>`) are emitted.

## Out-of-band considerations

- **Cross-SDK parity (REQ-080, REQ-081).** The PHP SDK's openehr-bmm library is the existing reference for what a BMM-driven Go library should look like. Cross-check the Go shape against the PHP class names where the wire format demands parity (it does not — parity is at the wire, not the source — but consistency helps maintainers fluent in both).
- **Future ADRs to write once the plan executes.** The shared contract source-of-truth decision (STRAND-02) intersects this plan: if STRAND-02 eventually selects an OpenAPI-generated wire-DTO layer, the BMM-generated RM types and the OpenAPI-generated DTOs need to coexist. Plan reviews the boundary at Phase 2 close.
