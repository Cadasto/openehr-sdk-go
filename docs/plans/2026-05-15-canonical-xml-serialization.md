# Plan — Canonical XML serialization

**Date:** 2026-05-15
**Status:** Implemented (Sandbox) — 2026-05-17. All six phases landed; PROBE-033/034 in `testkit/probes/serialize/`; cross-format invariant in `openehr/serialize/canxml/crossformat_test.go`. Hash<K,V> XML emission/decode deferred (v1 limitation; see `canxml/doc.go`).
**Owner:** SDK maintainers
**Covers:** REQ-056, REQ-040 (type registry consumption), REQ-013 (building-block), REQ-024 (no reflection on `xsi:type` dispatch); PROBE-033, PROBE-034 (reserved — see Phase 0)
**Depends on:** BMM codegen complete ([`2026-05-15-bmm-codegen.md`](2026-05-15-bmm-codegen.md)); canonical JSON plan Phase 0 (`internal/poly`, typereg sentinels, vendored fixtures, `wire.md` ordering); JSON plan Phase 2+ for shared cross-format tests
**Defers:** FLAT and STRUCTURED simplified formats

## Goal

Implement the openEHR canonical XML codec under `openehr/serialize/canxml/`. Same determinism and fail-fast polymorphic dispatch as canonical JSON, using **`xsi:type`** instead of **`_type`**.

Consumers import `openehr/serialize/canxml` directly (REQ-013).

## Integration with existing stack

| Piece | Role |
|---|---|
| `openehr/rm/typereg` | **`Lookup(typeName)`** for `xsi:type` → constructor; do **not** use `typereg.Decode` (JSON-only API) |
| `openehr/serialize/internal/poly` | Shared discriminator resolution + `DecodeError` (from JSON plan Phase 0) |
| `internal/bmmgen` | Emits `*_xmlmar_gen.go` |
| RM composition fixtures | Vendored JSON cassettes from JSON plan; XML-specific fragments vendored separately (Phase 0) |

Dispatch logic is shared with JSON via `poly`; only attribute/element reading differs.

## What "canonical XML" means here

Grounded in openEHR RM XSDs (pinned release — see References) and observed RM/OPT XML conventions:

- Default namespace: **`http://schemas.openehr.org/v1`**
- `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` on document root when any descendant carries `xsi:type`
- Polymorphism: **`xsi:type`** = unprefixed openEHR class name (`xsi:type="OBSERVATION"`). Foundation primitives: `xsi:type="xsd:string"` etc. with `http://www.w3.org/2001/XMLSchema`
- **Element names are snake_case BMM property / class names** — same as JSON keys (e.g. `magnitude_status`, `composition`). Do **not** lower-case class names (`composition` ≠ `COMPOSITION` unless the BMM name is already lower case).
- **Polymorphic site:** same definition as the JSON plan (abstract BMM type at the field). Decoder requires `xsi:type` at polymorphic sites unless relaxed mode is on. Encoder emits `xsi:type` on every concrete value boundary (SDK deterministic profile); only one discriminator per concrete element.
- Containers: repeated sibling elements (one per item), not a wrapper element.
- ISO 8601 and numbers: element **text** content; not parsed at codec layer (REQ-046).
- `xsi:schemaLocation` optional; decoder ignores; encoder does not emit.

> **`xsi:type` vs `xmi:type`:** Reject `xmi:type` with `ErrInvalidShape` and an explicit message.

## Canonical ordering

Same normative rule as canonical JSON (and amended REQ-052 / REQ-056 narrative):

1. Child elements follow **BMM property declaration order**.
2. **`xsi:type` is the first attribute** when present.
3. Compact XML (no insignificant inter-element whitespace) is the byte-equality target for round-trip tests.

## Why now

- REQ-056 requires symmetric XML support.
- `xsi:type` dispatch mirrors JSON `_type`; shared `poly` + typereg amortises design.
- Cross-format invariant: JSON ↔ XML ↔ JSON preserves Go values (Phase 3).

## Out of scope

- **FLAT / STRUCTURED**
- **XSD validation** at codec layer
- **XMI**
- **Archetype/template/OPT XML authoring** — `openehr/template/`; this codec is **RM instance** data (compositions, demographics, etc.)
- **AOM XML in v1 probes** — OPT fragments MAY seed unit tests for `xsi:type` parsing, but PROBE-033/034 target **RM composition** XML only
- Cross-tool whitespace equality with third-party pretty-printers

## Phases

### Phase 0 — Normative hooks, fixtures, probes

**Outcome:** XML plan is wired to specs and test inputs before implementation.

**Tasks:**

1. **Amend [`specs/wire.md`](../../specs/wire.md) REQ-056** — add element/attribute ordering aligned with JSON plan (BMM order, `xsi:type` first). Same CHANGELOG bullet as JSON Phase 0 if not already done.
2. **Reserve probes in [`specs/conformance.md`](../../specs/conformance.md)** (Draft placeholders):
   - **PROBE-033** — canonical-XML round trip (modulo compact whitespace)
   - **PROBE-034** — unknown `xsi:type` → `typereg.ErrUnknownType`
3. **Vendor XML fixtures** → `testkit/cassettes/canonical_xml/`:
   - RM composition XML (convert or capture from CDR where available)
   - Small hand-crafted fragments for `OBSERVATION` / `DV_QUANTITY` polymorphism
   - Provenance README (sibling-repo path + refresh procedure)
4. Confirm **`openehr/serialize/internal/poly`** from JSON plan Phase 0 is format-agnostic (discriminator string in, ctor out).

**Definition of done:** PROBE-033/034 exist in `conformance.md` as Draft; at least one RM XML fixture in-repo.

### Phase 1 — Package skeleton and encoder

**Outcome:** `canxml.Marshal` produces deterministic compact XML with correct namespaces and `xsi:type` at polymorphic sites.

**Tasks:**

1. `openehr/serialize/canxml/doc.go` — namespaces, snake_case element names, strict vs relaxed decode.
2. Public API: `Marshal`, `MarshalIndent` (same shape as `canjson`).
3. `bmmgen` emits `MarshalXML(e *xml.Encoder, start xml.StartElement) error` in `*_xmlmar_gen.go`:
   - Element local name = **snake_case** BMM name (from parent field or type)
   - Emit `xsi:type` when encoding a value at a **polymorphic** parent field
   - Children in BMM order; optional nil → omit element; empty container → omit
4. Root namespace declarations per § What canonical XML means.
5. Tests (**encode-only goldens** — do not require `UnmarshalXML` yet):
   - `DV_QUANTITY` at root: no `xsi:type` on root element in isolation test
   - `COMPOSITION` with `OBSERVATION` in `content`: `xsi:type="OBSERVATION"` on child
   - Nil optionals omitted; empty containers omitted

**Definition of done:**

- `go test ./openehr/serialize/canxml/...` passes encode goldens.
- Phase 1 DoD does **not** require stdlib round-trip (that needs Phase 2 `UnmarshalXML`).

### Phase 2 — Decoder + polymorphic dispatch

**Outcome:** `Unmarshal` reads XML into concrete RM types.

**Tasks:**

1. Public API: `Unmarshal`, `NewDecoder`, `Decode`, `WithRelaxedTypeDispatch` (parallel to JSON — default strict).
2. **Errors:** wrap `typereg.ErrMissingType`, `ErrUnknownType`, `ErrTypeMismatch` in `poly.DecodeError` with element paths. `canxml`-local: `ErrInvalidShape`, `ErrNamespace`.
3. **Dispatch:**
   ```go
   func DecodeAs[T any](d *xml.Decoder, start xml.StartElement) (T, error)
   func DecodeSliceAs[T any](d *xml.Decoder, start xml.StartElement) ([]T, error)
   ```
   - Read `xsi:type` from `start.Attr` (XML namespace `http://www.w3.org/2001/XMLSchema-instance`)
   - Strip `xsd:` prefix for primitives; map to typereg name
   - `ctor, ok := typereg.Default.Lookup(name)` — then `xml.Decoder` into new `*Concrete`
   - Generated `UnmarshalXML` on parents calls these helpers
4. Namespace rules: reject foreign namespaces → `ErrNamespace`; tolerate redundant `xmlns` on inner elements.
5. Reject `xmi:type`; mixed content → `ErrInvalidShape`.
6. Tests:
   - RM fragment `xsi:type="OBSERVATION"` → `*rm.Observation`
   - Unknown / missing / mismatch cases (typereg sentinels)
   - **Scope:** RM instance XML only in conformance path; OPT-shaped fragments are unit-test inputs for dispatch, not PROBE-033 payloads

**Definition of done:**

- RM XML fixtures decode to expected types.
- Five error classes covered (including `ErrNamespace`).

### Phase 3 — Round-trip, probes, cross-format

**Outcome:** XML round-trip stable; cross-format invariant with JSON.

**Tasks:**

1. **Round-trip:** vendored XML fixtures → `Unmarshal` → `Marshal` → byte-equal compact XML. Do **not** rely on pretty-print → compact; encoder always emits compact form for equality tests.
2. **PROBE-033 / PROBE-034** in `testkit/probes/serialize/`.
3. **Cross-format regression** (shared with JSON plan):
   ```
   JSON bytes → Unmarshal → value A
   Marshal XML(A) → XML bytes → Unmarshal → value C
   cmp.Equal(A, C)  // google/go-cmp
   Marshal JSON(C) → D; Compact(D) == Compact(original JSON)
   ```
   Use vendored **JSON** composition cassettes as the source of truth for RM graphs; XML leg validates the XML codec against the same logical data.
4. Pin **ITS-XML / XSD release** in `doc.go` and `wire.md` when amended (same bump process as BMM files).

**Definition of done:**

- PROBE-033/034 pass (sandbox).
- Cross-format test passes on all vendored JSON composition cassettes.

### Phase 4 — Edge cases

**Tasks:**

1. Self-closing vs empty element pairs — document encoder choice; decoder accepts both.
2. `xsd:` foundation types — strip on decode; emit only where BMM requires.
3. Empty containers / nil optionals — absent elements only.
4. Numeric / ISO8601 / `CodePhrase` — same posture as JSON plan.
5. Deep `FOLDER` trees — stack safety.

**Definition of done:** Each case in `doc.go` + test.

### Phase 5 — Performance baseline

**Tasks:**

1. `bench_test.go` — encode/decode ~50 KiB composition XML; 10k batch.
2. Compare to JSON plan benchmarks; default **`encoding/xml`** unless evidence warrants a build-tag alternative.
3. Update STRAND-04 with XML numbers.

**Definition of done:** Benchmarks runnable; note in plan or ADR.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| snake_case vs lower-case element names | Normative rule: BMM snake_case; tests pin goldens |
| stdlib `encoding/xml` namespace quirks | Fixture-driven tests; document workaround if bypass needed |
| Phase 1 DoD claimed stdlib round-trip too early | Encode-only DoD in Phase 1 |
| OPT XML confused with RM scope | PROBE-033 uses RM XML; OPT fragments = unit tests only |
| Missing `xsi:type` in the wild | `WithRelaxedTypeDispatch` (off by default); JSON plan parity |
| Whitespace breaks byte diff | Compact encoder output only in equality tests |
| Duplicate error sentinels | typereg owns Unknown/Missing/Mismatch; canxml wraps |
| ITS-XML / XSD drift | Pin release in References + `wire.md`; explicit bump process |

## Mapping to specs

- [specs/wire.md § Canonical XML](../../specs/wire.md#canonical-xml) — REQ-056 (amend in Phase 0)
- [specs/rm-modeling.md § Type registry](../../specs/rm-modeling.md#type-registry) — REQ-040; use `Lookup`, not `Decode`
- [specs/idiom.md § Generics policy](../../specs/idiom.md#generics-policy) — REQ-024
- [specs/conformance.md PROBE-033, PROBE-034](../../specs/conformance.md) — reserved Phase 0
- [Canonical JSON plan](2026-05-15-canonical-json-serialization.md) — shared ordering, `poly`, cross-format tests
- [`.codebase-memory/adr.md`](../../.codebase-memory/adr.md) — typereg layout (D3), flattening (D4)

## References

- openEHR ITS-REST — [Simplified Formats](https://specifications.openehr.org/releases/ITS-REST/development/simplified_formats.md) (defers XML detail to XSDs)
- openEHR RM XSDs — normative element shape (pin release alongside BMM in `resources/bmm/README.md`)
- Pinned BMM: same as JSON plan
- Golden inputs: `testkit/cassettes/canonical_xml/`; cross-format source graphs from `testkit/cassettes/canonical_json/`
- Sibling reference (provenance only): `openehr-cdr` `cmd/benchmark/internal/fixtures/templates/` for `xsi:type` examples in OPT XML

## Out-of-band considerations

- **Cross-format round-trip** — strongest shared invariant with JSON plan; failures indicate a bug in either codec.
- **Cross-SDK parity (REQ-080, REQ-081):** shared cassettes + BMM element order when XML probes ratify.
- **`xsi:schemaLocation`:** decode tolerant; encode omitted.
- **Future: streaming XML** — `encoding/xml` supports it; out of v1.
