# Plan — Canonical JSON serialization

**Date:** 2026-05-15
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-052, REQ-040 (type registry consumption), REQ-013 (building-block), REQ-024 (no reflection on `_type` dispatch); PROBE-030, PROBE-031
**Depends on:** BMM codegen complete ([`2026-05-15-bmm-codegen.md`](2026-05-15-bmm-codegen.md) Phases 1–5) — generated RM/AOM types, JSON tags, and `openehr/rm/typereg_gen.go` (ADR D3: registrations in package `rm`, not under `typereg/`)
**Defers:** FLAT and STRUCTURED simplified formats (separate later plan)

## Goal

Implement the openEHR canonical JSON codec — encoder and decoder — for the generated RM types under `openehr/serialize/canjson/`. Round-trip byte-stability and a fail-fast policy on unknown polymorphic types (`_type`) are load-bearing.

Consumers import `openehr/serialize/canjson` directly (building-block independence per REQ-013). The parent `openehr/serialize` package documents the codec family; optional thin re-exports (`serialize.CanonicalJSON`) MAY be added later but are not required for v1.

## Integration with existing stack

| Piece | Location | Role for this plan |
|---|---|---|
| Generated RM types + `json` tags | `openehr/rm/*_gen.go` | Encode/decode targets |
| Type registry primitive | `openehr/rm/typereg/registry.go` | `Lookup`, `Decode`, `DecodeAs[T]` — **single dispatch authority** |
| Generated registrations | `openehr/rm/typereg_gen.go` | `init()` populates `typereg.Default` |
| Shared polymorphic helpers | `openehr/serialize/internal/poly` (new, unexported) | Discriminator resolution + shared `DecodeError` shape; used by `canjson` and `canxml` |
| Code generator extension | `internal/bmmgen` | Emits `*_jsonmar_gen.go` per RM file group |

This plan adds wire codecs; it does **not** re-run BMM type generation except to extend `bmmgen` for `MarshalJSON` / `UnmarshalJSON`.

## What "canonical JSON" means here

The wire format used by openEHR REST 1.1.0-development for openEHR data instances. Verified properties (from vendored CDR composition fixtures — see Phase 0):

- Property names are **snake_case** — they match BMM property names exactly (no transformation).
- Polymorphic discrimination uses the **`_type`** JSON property, set to the unprefixed openEHR class name (e.g. `"_type": "COMPOSITION"`, `"_type": "DV_QUANTITY"`).
- **Polymorphic site** (normative for this SDK): a property whose declared BMM type is an abstract class or interface in the merged schema (post descendant-shadows-ancestor merge per ADR D2). At such sites the decoder **MUST** require `_type` unless relaxed mode is enabled (see Phase 2). On encode the SDK **always** emits `_type` at every concrete RM value boundary (deterministic profile); only the **outermost** concrete type emits `_type` once — not on embedded ancestor field groups (ADR D4 flattening).
- Monomorphic concrete nested values MAY omit `_type` on decode when the declared type has no abstract alternatives; the SDK still emits `_type` on encode for determinism.
- Nested objects render as nested JSON objects (no flattening of `name.value`).
- Optional fields are either **absent** or carry `null`. Absent and `null` are both accepted on decode; on encode the SDK emits **absent** (no key) for nil-pointer optional fields.
- Container properties (List/Set/Array) serialize as JSON arrays.
- **Hash** (`map[K]V`) keys serialize in **lexicographic key order** (Go `encoding/json` behaviour), even when struct fields use BMM declaration order.
- ISO 8601 dates/times/durations are JSON strings; the SDK does **not** parse them to `time.Time` at the codec layer (REQ-046).
- `Real` / `Double` magnitudes are JSON numbers (`float64`).
- Empty containers with `cardinality.lower == 0` encode as **absent** (`omitempty`).

> **Note on `_type` vs `@type`.** openEHR canonical JSON uses `_type` (leading underscore), not `@type` (JSON-LD). The discriminator value is the openEHR class name (e.g. `OBSERVATION`), not a URI.

## Canonical ordering (normative for this SDK)

openEHR has not published a strict canonical-JSON field-order rule. **`specs/wire.md` REQ-052 currently defaults to lexicographic order when no spec rule exists; this plan supersedes that default for implementation** and MUST be followed by a matching `wire.md` edit in Phase 0.

**SDK rule:**

1. **`_type` is always the first JSON object key** on every encoded concrete RM value.
2. **Remaining keys follow BMM property declaration order** (the order `bmmgen` emits struct fields).
3. **`Hash` map keys are always lexicographic** (stdlib), independent of rule 2.

Cross-SDK probes (REQ-080) compare against shared cassettes encoded with this ordering until the openEHR Foundation publishes an official canonical order.

## Why now

- Generated RM types have `json` tags but no `_type`-aware `MarshalJSON` / `UnmarshalJSON` yet.
- PROBE-030 and PROBE-031 require the codec.
- `openehr/client/*`, `transport/`, and `openehr/composition/` depend on canonical JSON.

## Out of scope

- **FLAT and STRUCTURED** — separate plan.
- **Canonical XML** — [`2026-05-15-canonical-xml-serialization.md`](2026-05-15-canonical-xml-serialization.md).
- **AQL result envelopes** — `openehr/client/query/` layers its own handling; leaf RM values use this codec.
- **OPT validation** — `openehr/validation/`.
- **JSON-LD** — ITS-JSON canonical form only.
- **Streaming decode** — note for future; REST 1.1.0 does not require it.

## Phases

### Phase 0 — Normative alignment, fixtures, shared dispatch

**Outcome:** Specs and test inputs are pinned before codec code lands.

**Tasks:**

1. **Amend [`specs/wire.md`](../../specs/wire.md) REQ-052** — replace the lexicographic default with BMM declaration order + `_type` first + lexicographic `Hash` keys (per § Canonical ordering above). One CHANGELOG bullet under `## [Unreleased]`.
2. **Vendor golden fixtures** into this repo (REQ-082 cassette independence):
   - Copy a minimal set from `openehr-cdr` `cmd/benchmark/internal/fixtures/compositions/` → `testkit/cassettes/canonical_json/` (or `openehr/serialize/canjson/testdata/`).
   - Record provenance in `testkit/cassettes/README.md` (source commit, refresh command).
   - CI MUST NOT depend on the sibling repo being cloned.
3. **Add `openehr/serialize/internal/poly`** — unexported helpers:
   - `ResolveType(name string) (func() any, error)` → `typereg.Default.Lookup`
   - Shared `DecodeError` with `Path`, `Type`, `Inner`, `Unwrap() error`
   - Map `typereg` failures to stable sentinels (next task)
4. **Add typereg sentinels** in `openehr/rm/typereg/registry.go`:
   ```go
   var (
       ErrMissingType  = errors.New("typereg: _type discriminator missing")
       ErrUnknownType  = errors.New("typereg: _type not in registry")
       ErrTypeMismatch = errors.New("typereg: decoded type does not satisfy target")
   )
   ```
   Update `Decode` / `DecodeAs[T]` to return errors wrapping these (`errors.Is` compatible). PROBE-031 asserts `typereg.ErrUnknownType`.
5. **`canjson` wraps typereg errors** in `poly.DecodeError` with JSON pointer paths — no duplicate `ErrUnknownType` in `canjson`.

**Definition of done:**

- `wire.md` and this plan agree on field order.
- At least `body_weight.json` (and 2–3 peers) exist in-repo.
- `typereg` sentinels covered by unit tests; `poly` package compiles.

### Phase 1 — Package skeleton and encoder

**Outcome:** `openehr/serialize/canjson/` with working `Marshal` for generated RM types. `_type` emitted on every encoded concrete value.

**Tasks:**

1. `openehr/serialize/canjson/doc.go` — scope, ordering rules, building-block import path, strict vs relaxed decode (forward reference).
2. Public API:
   ```go
   func Marshal(v any) ([]byte, error)
   func MarshalIndent(v any, prefix, indent string) ([]byte, error)
   ```
3. Extend `bmmgen` to emit `MarshalJSON` on every **concrete** RM struct into `<file>_jsonmar_gen.go`. **Generator pattern** (do not call `json.Marshal` on `self` recursively):

   ```go
   func (c *Composition) MarshalJSON() ([]byte, error) {
       return json.Marshal(compositionWire{
           Type: "COMPOSITION",
           // fields in BMM order, copied from c
       })
   }
   type compositionWire struct {
       Type string `json:"_type"`
       // remaining fields with same json tags as Composition, declaration order
   }
   ```

   Rules: `_type` first field in wire struct; omit nil pointers; omit empty slices when `cardinality.lower == 0`; never emit `_type` on embedded ancestor-only wire structs.
4. `canjson.Marshal(v)` → `json.Marshal(v)`; per-type `MarshalJSON` does the work. Keeps STRAND-04 swap path (sonic/easyjson) as a one-line change later.
5. Tests (vendored fixtures): `DV_QUANTITY` golden bytes; `COMPOSITION` + polymorphic `content[]`; nil optionals absent; empty containers absent; `body_weight` encode matches fixture after normalising `null` → absent on compare.

**Definition of done:**

- `go test ./openehr/serialize/canjson/...` passes encode tests.
- `go run ./cmd/bmmgen -verify` still passes after generator extension.

### Phase 2 — Decoder + polymorphic dispatch

**Outcome:** `Unmarshal` populates correct concrete types at polymorphic sites via `typereg` + generated `UnmarshalJSON`.

**Tasks:**

1. Public API:
   ```go
   func Unmarshal(data []byte, v any) error
   func NewDecoder(r io.Reader) *Decoder
   func (d *Decoder) Decode(v any) error

   // Decoder options (v1 default: strict)
   func WithRelaxedTypeDispatch(enabled bool) DecoderOption
   ```
   **Strict (default):** missing `_type` at a polymorphic site → `typereg.ErrMissingType`. **Relaxed:** if the parent field's declared type has exactly one concrete descendant in the merged BMM, instantiate it without `_type` (documented escape hatch for legacy producers; off by default).
2. **Errors:** `canjson` adds only `ErrInvalidShape` for JSON shape issues. `ErrMissingType`, `ErrUnknownType`, `ErrTypeMismatch` are **`typereg` sentinels**, wrapped:
   ```go
   type DecodeError struct {
       Path  string // JSON pointer-ish (/content/0/_type)
       Type  string
       Inner error // unwraps to typereg.Err*
   }
   ```
3. **Dispatch (strategy B):** Generated `UnmarshalJSON` on structs with polymorphic fields calls:
   - `typereg.DecodeAs[T](raw)` for single polymorphic values
   - `canjson.DecodeSliceAs[T](raw)` for `[]T` where `T` is an RM interface — implemented as a loop over `typereg.DecodeAs[T]` per element
   - Do **not** add a second `canjson.DecodeAs` that duplicates `typereg.DecodeAs`
4. Generic instantiations (`DVInterval[DVQuantity]`, etc.): plain `json.Unmarshal` — type parameter fixes the concrete type.
5. Tests: vendored fixtures decode to correct concrete types; unknown `_type` → `errors.Is(err, typereg.ErrUnknownType)`; missing `_type` (strict) → `ErrMissingType`; wrong interface → `ErrTypeMismatch`.

**Definition of done:**

- All vendored composition fixtures decode cleanly.
- Negative tests cover typereg sentinels + `DecodeError.Path`.

### Phase 3 — Round-trip + probes

**Outcome:** Decode → re-encode is byte-stable (PROBE-030). Unknown `_type` probe (PROBE-031).

**Tasks:**

1. Round-trip every vendored fixture: `Unmarshal` → `Marshal` → compare to original. Normalise inputs with `json.Compact` (or parse/re-marshal) to strip incidental whitespace before byte diff. SDK `Marshal` emits compact JSON; `MarshalIndent` for human inspection only.
2. When comparing to source fixtures that contain `"field": null`, strip null keys (or run through SDK round-trip) per Phase 4 §1.
3. **Hash-stability test:** encode same value twice → identical bytes; include a struct with a `map` field to assert lexicographic key order.
4. Implement probes:
   - `testkit/probes/serialize/probe_030_canjson_round_trip.go`
   - `testkit/probes/serialize/probe_031_typereg_unknown_type.go` — asserts `typereg.ErrUnknownType`
5. **Cross-format regression** (with canxml plan): JSON → Go → XML → Go → JSON; use `github.com/google/go-cmp/cmp` for Go value equality (not bare `reflect.DeepEqual`). Document in both plans.

**Definition of done:**

- PROBE-030 and PROBE-031 pass in sandbox mode.
- Every vendored composition fixture round-trips.

### Phase 4 — Edge cases and conformance polish

**Tasks:**

1. **`null` handling** — decode `null` → `nil` / zero; encode → absent. Document in `doc.go`.
2. **Numbers** — `float64` / `int32` / `int64`; overflow → `ErrInvalidShape`. Document float precision limits (REQ-052).
3. **ISO 8601** — plain `string` at codec layer; typed helpers in `*_ext.go`.
4. **`CodePhrase` / `TerminologyID`** — nested objects; single `rm.CodePhrase` type post ADR D2/D5.
5. **Recursion** — deep `FOLDER` trees; stack-safe decode.
6. **Empty composition** — `content` encodes as **absent**, not `[]`.
7. **Full cassette sweep** — all files under `testkit/cassettes/canonical_json/`.

**Definition of done:** Each edge case documented in `doc.go` with a focused test.

### Phase 5 — Performance baseline (STRAND-04)

**Tasks:**

1. `bench_test.go`: encode/decode ~50 KiB composition; batch 10k decodes.
2. Compare stdlib vs optional build-tag backends **after** baseline exists — default remains **`encoding/json`** unless evidence says otherwise (document alloc/op costs of per-type `MarshalJSON`).
3. Close STRAND-04 **polymorphism** sub-strand in [`specs/research-strands.md`](../../specs/research-strands.md); leave codec-perf open if benchmarks are inconclusive.

**Definition of done:** `go test -bench=. ./openehr/serialize/canjson/...` runs; short numbers in this plan or an ADR.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `wire.md` lexicographic default conflicts with BMM order | Phase 0 amends `wire.md`; probes document SDK ordering |
| Sibling-repo fixtures missing in CI | Phase 0 vendors cassettes in-repo |
| `MarshalJSON` recursion / wrong key order | Wire-struct pattern; `_type` as first struct field |
| Duplicate `DecodeAs` APIs | `typereg.DecodeAs[T]` only; `canjson.DecodeSliceAs` composes it |
| Map key order vs struct field order | Document: Hash keys always lexicographic |
| Legacy producers omit `_type` | `WithRelaxedTypeDispatch` (default off); XML plan has parallel `WithRelaxedTypeDispatch` for `xsi:type` |
| PROBE-031 expects `typereg.ErrUnknownType` | Phase 0 adds typereg sentinels |
| `null` in CDR fixtures | Decode accepts; encode absent; tests normalise before diff |
| Cross-format `DeepEqual` flakes | `cmp.Equal` with exported-only or generated equalities |

## Mapping to specs

- [specs/wire.md § Canonical JSON](../../specs/wire.md#canonical-json) — REQ-052 (amended in Phase 0)
- [specs/rm-modeling.md § Type registry](../../specs/rm-modeling.md#type-registry) — REQ-040
- [specs/idiom.md § Generics policy](../../specs/idiom.md#generics-policy) — REQ-024: reflection OK for ordinary field mapping; `_type` dispatch only via typereg
- [specs/conformance.md PROBE-030, PROBE-031](../../specs/conformance.md)
- [`.codebase-memory/adr.md`](../../.codebase-memory/adr.md) — D3 typereg layout, D4 flattening, D5 `rm.CodePhrase`
- [specs/research-strands.md STRAND-04](../../specs/research-strands.md)

## References

- openEHR ITS-REST — [Simplified Formats](https://specifications.openehr.org/releases/ITS-REST/development/simplified_formats.md) (canonical JSON role, `_type` discriminator)
- Pinned BMM: [`resources/bmm/openehr_rm_1.2.0.bmm.json`](../../resources/bmm/openehr_rm_1.2.0.bmm.json), [`resources/bmm/openehr_base_1.3.0.bmm.json`](../../resources/bmm/openehr_base_1.3.0.bmm.json)
- Golden inputs: `testkit/cassettes/canonical_json/` (vendored from openehr-cdr; provenance in README)
- Cross-format tests: shared with [canonical XML plan](2026-05-15-canonical-xml-serialization.md)

## Out-of-band considerations

- **Cross-SDK parity (REQ-080, REQ-081):** PHP and Go MUST converge on BMM declaration order + shared cassettes for probe byte comparison.
- **Cross-format round-trip:** Documented in Phase 3; canonical XML plan owns XML-specific probes (PROBE-033+).
- **Future: JSON Schema from BMM** — not in v1.
- **Future: streaming** — `json.Decoder` token limits for very large compositions.
