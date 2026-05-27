# Plan — RM `*Like` interface ergonomics (post SDK-GAP-11)

**Date:** 2026-05-27
**Status:** **Landed 2026-05-27.** Phases 1–3 shipped on PR #25. Method names use the `Get*` prefix (not bare `Value()` as originally drafted) because Go forbids a struct field and a method with the same name — see [§ Recommended direction](#recommended-direction-this-sdk).
**Owner:** SDK maintainers
**Covers:** SDK-GAP-11 follow-up (consumer ergonomics); [REQ-024](../../specifications/idiom.md#generics-policy-req-024), [REQ-040](../../specifications/rm-modeling.md#type-registry-req-040), [REQ-052](../../specifications/wire.md#req-052)
**Probes:** PROBE-038 (regression guard — no behaviour change required if decode/re-marshal unchanged)
**Implementation:** **landed** — narrow `<Parent>Like` interface declarations moved out of the generator into hand-written [`openehr/rm/like_interfaces.go`](../../../openehr/rm/like_interfaces.go) with Get-prefixed accessor methods on all five interfaces. Compat helpers in [`openehr/rm/like_accessors.go`](../../../openehr/rm/like_accessors.go) delegate to the methods.
**Depends on:** [archive/2026-05-26-rm-polymorphic-decode-coverage.md](2026-05-26-rm-polymorphic-decode-coverage.md) (landed — `DVTextLike`, `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`)
**Defers:** Replacing `typereg` + `*Like` interfaces with hand-rolled `*Union{Kind, Value}` structs across the whole RM tree (see [gopenehr survey](#survey-cadastogopenehr) — evaluate separately if ergonomics insufficient after Phase 3)

## Goal

Improve **consumer ergonomics** after SDK-GAP-11 without weakening lossless decode/re-marshal. Callers should reach RM-common attributes on substitution slots (especially `LOCATABLE.name`) in a way that matches openEHR intuition (“`DV_TEXT` slot always has a `value`”) while staying idiomatic Go and keeping `typereg` as the single `_type` dispatch path for generated codecs.

**Non-goals:** Reverting to concrete `Name DVText` fields; reflection-based dynamic property access; changing PROBE-038 wire semantics.

## Delivery scope (Phases 1–3)

This plan is intentionally **three phases** — no archive/PROBE flip required; wire behaviour is unchanged.

| Phase | Outcome (one line) |
|-------|-------------------|
| **1** | `DVTextLike.Value()` / `DefiningCode()` generated; example uses `c.Name.GetValue()`; tests prove methods match `DVTextValueOf` |
| **2** | Same method pattern for `DVURILike`, `AuditDetailsLike`, `PartyIdentifiedLike`, `ObjectRefLike`; `openehr/rm/doc.go` documents substitution vs abstract interfaces |
| **3** | `idiom.md` + CHANGELOG + `.gitignore` (`*.tmp`); optional gopenehr-style typed getters on `DVTextLike` if still needed |

**Deferred:** Phase 4 (optional) — gopenehr-style `*Union{Kind, any}` codegen spike; not part of the initial delivery.

## Problem statement

Today, `Composition.Name` (and every `LOCATABLE.name`) is `DVTextLike` — a sealed marker interface. Decode **already** uses `_type` via `typereg.DecodeAs[DVTextLike]`; the pain is **static access**:

- `c.Name.Value` does not compile.
- `rm.DVTextValueOf(c.Name)` works but feels foreign to RM-familiar users and PHP/SDK parity expectations.

The same pattern applies to other `*Like` families, but **`DVTextLike` is the hot path** (`name` on almost every locatable). `DataValue`, `Item`, `ContentItem`, and `UIDBasedID` were already interfaces before GAP-11 — users type-assert those; only concrete-with-subtype slots changed.

## Survey: `Cadasto/gopenehr`

**Note:** There is no `Cadasto/goopenehr` repo; the relevant private codebase is **`Cadasto/gopenehr`** (`github.com/freekieb7/gopenehr`).

### Pattern: tagged union struct + custom `UnmarshalJSON`

Polymorphic RM fields use a **closed union**, not a Go `interface{}`:

```go
type DvTextUnion struct {
    Kind  DvTextKind   // DV_TEXT | DV_CODED_TEXT | unknown
    Value any          // *DV_TEXT | *DV_CODED_TEXT
}
```

Discriminator handling (representative — `internal/openehr/rm/dv_text.go`):

1. `t := util.UnsafeTypeFieldExtraction(data)` — peek `_type` without full parse.
2. `switch t` — allocate concrete struct, `sonic.Unmarshal(data, d.Value)`.
3. Missing/empty `_type` on text → **`DV_TEXT`** (same tolerance as this SDK’s narrow-slot fallback).

**Accessors** (not unified `Value()` on the union):

- `(*DvTextUnion).DV_TEXT() *DV_TEXT`
- `(*DvTextUnion).DV_CODED_TEXT() *DV_CODED_TEXT`
- Callers use `name.DV_CODED_TEXT().Value` after a nil check — still not `name.Value`, but concrete fields are one hop away.

### Polymorphic slots in gopenehr (mapping)

| RM slot | gopenehr field type | Dispatch | Typed getters |
|--------|---------------------|----------|----------------|
| `LOCATABLE.name`, `null_reason`, … | `DvTextUnion` | `_type` → `DV_TEXT` / `DV_CODED_TEXT` | `DV_TEXT()`, `DV_CODED_TEXT()` |
| `ELEMENT.value` | `DataValueUnion` | `_type` → full `DATA_VALUE` family | `DV_QUANTITY()`, `DV_CODED_TEXT()`, … |
| `COMPOSITION.content[]` | `ContentItemUnion` | `_type` → entry types | `Observation()`, `Section()`, … (per kind) |
| `CLUSTER.items[]` | `ItemUnion` | `_type` → `CLUSTER` / `ELEMENT` | `CLUSTER()`, `ELEMENT()` |
| `LOCATABLE.uid` | `UIDBasedIDUnion` | `_type` | `HIER_OBJECT_ID()`, `OBJECT_VERSION_ID()` |
| `OBJECT_REF.id` | `ObjectIDUnion` | `_type` | per-id-type getters |
| `PARTY_PROXY` (e.g. composer) | `PartyProxyUnion` | `_type` | `PARTY_SELF()`, `PARTY_IDENTIFIED()`, `PARTY_RELATED()` |
| `AUDIT_DETAILS` on version | concrete `AUDIT_DETAILS` | — | struct fields |
| `EVENT_CONTEXT.health_care_facility` | concrete `PARTY_IDENTIFIED` | — | **no `PARTY_RELATED` union** (possible gap vs RM substitution) |

### Comparison to openehr-sdk-go (this repo)

| Aspect | `gopenehr` | `openehr-sdk-go` (GAP-11) |
|--------|------------|---------------------------|
| Storage | `Kind` + `any` union struct | Sealed `*Like` interface + `typereg` |
| Decode | Per-union `UnmarshalJSON` + `_type` peek | Generated `json.RawMessage` + `typereg.DecodeAs[<Like>]` |
| Registry | Ad hoc per union file | Central `typereg.Default` (REQ-040) |
| Text access | `DV_TEXT()` / `DV_CODED_TEXT()` then `.Value` | `DVTextValueOf` / `AsDVText` / type assert |
| Codec symmetry | sonic only (in surveyed files) | canjson + canxml shared typereg |

**Takeaway:** Both codebases are **discriminator-first** and **lossless**. gopenehr optimises for **named concrete getters** on unions; this SDK optimises for **one registry** and **narrow interfaces**. The ergonomics gap is closable by adding **interface methods** (or generated getters) without adopting gopenehr’s per-type `UnmarshalJSON` duplication.

## Recommended direction (this SDK)

**Keep** `*Like` + `typereg` (building-block independence, PROBE-038, cross-codec symmetry).

**Add** RM-shaped methods on each narrow interface (generated), mirroring what gopenehr achieves via `DV_TEXT()` / `DV_CODED_TEXT()` but without a second dispatch layer:

| Interface | Methods (minimum) |
|-----------|-------------------|
| `DVTextLike` | `GetValue() string`, `GetDefiningCode() (CodePhrase, bool)` |
| `DVURILike` | `GetValue() string` |
| `AuditDetailsLike` | embed accessors via `AuditDetailsBase`-equivalent methods or promote `AuditDetailsBase` to methods |
| `PartyIdentifiedLike` | `Name() string`, `Identifiers() []DVIdentifier` (optional; narrower API) |
| `ObjectRefLike` | `ID() ObjectID`, `Namespace() string`, `Type() string` via `ObjectRefBase` shape |

Method names must avoid clashes with RM attribute names used as struct fields on the underlying concrete types. Since `DVText.Value` is already a field, `Value()` as a method name on the same struct is illegal — the shipped methods use a **`Get*`** prefix (`GetValue`, `GetDefiningCode`, …) to step out of the field/method namespace.

**Do not** add `*.tmp` only in docs — track in hygiene commit (see Phase 3).

## Phases

### Phase 1 — `DVTextLike` methods + example (highest impact)

**Outcome:** Callers use `c.Name.GetValue()` (method) on any `DVTextLike` field; decode/re-marshal path unchanged; `DVTextValueOf` remains as a thin compatibility wrapper.

**Tasks:**

1. Extend `internal/bmmgen` to emit on `DVTextLike`:
   - `GetValue() string`
   - `GetDefiningCode() (CodePhrase, bool)`
   - Concrete method bodies on `DVText`, `DVCodedText` (value + pointer receivers per interface satisfaction).
   - Closed variant set from `plan.ConcreteSubtypes["DV_TEXT"]`.
2. Regenerate RM if interface definition moves into generated files; otherwise add methods in `like_accessors.go` / a small `like_methods.go` (non-generated) — **pick one place** and document in PR.
3. `openehr/rm/like_accessors.go` — implement `DVTextValueOf` / `AsDVText` as delegates to interface methods (no behaviour change).
4. `cmd/examples/canonical_json/main.go` — `c.Name.GetValue()` instead of `DVTextValueOf`.
5. Tests:
   - `openehr/rm/*_test.go` — `GetValue()` / `GetDefiningCode()` for `*DVText`, `*DVCodedText`, nil `DVTextLike`.
   - `openehr/serialize/canjson/polymorphic_decode_test.go` — unchanged wire assertions (regression only).
   - Optional: `testkit/probes/serialize/probes_test.go` — fail fast on `Probe038Input.loadErr` (clearer cassette-miss signal).

**Definition of done:** `c.Name.GetValue()` compiles and equals prior `DVTextValueOf` semantics; PROBE-038 still passes; `make test` + `make ci` green.

---

### Phase 2 — Remaining `*Like` families

**Outcome:** All five narrow interfaces expose RM-common accessors as methods; package docs explain when to use methods vs type assert vs legacy funcs.

**Tasks:**

1. Emit (or hand-write, same pattern as Phase 1) minimal method sets:

   | Interface | Methods |
   |-----------|---------|
   | `DVURILike` | `Value() string` (URI value) |
   | `AuditDetailsLike` | `SystemID() string`, `TimeCommitted() DVDateTime`, `ChangeType() DVCodedText`, … (minimal set used by clients) |
   | `PartyIdentifiedLike` | accessors matching `PartyIdentifiedBase` fields used in-tree |
   | `ObjectRefLike` | `Namespace() string`, `Type() string`, `ID() ObjectID` (via embedded `ObjectRef` shape) |

   Derive the exact method list from `like_accessors.go` call sites + `grep` across `openehr/`, `testkit/`, `cmd/examples/`.
2. `openehr/rm/doc.go` — § substitution slots:
   - `*Like` = concrete parent + allowed subtypes (GAP-11); use **methods** for shared parent attributes.
   - `DataValue` / `Item` / `ContentItem` / `UIDBasedID` = abstract interfaces; use **type assert** (unchanged).
3. Update [`docs/adr/0001-bmm-version-bump-runbook.md`](../../adr/0001-bmm-version-bump-runbook.md) step 10 cross-link: new subtype → new interface method implementations (in addition to `like_accessors` switch arms until removed).
4. Migrate in-repo call sites from `DVURIValueOf`, `AuditDetailsBase`, etc. to methods where it improves readability (optional within phase; at minimum new code uses methods).

**Definition of done:** Each `*Like` has documented methods; in-repo validation/client code compiles without new func-only patterns; `make ci` green.

---

### Phase 3 — Docs, hygiene, and consumer-facing polish

**Outcome:** Normative/idempotent docs match the method-based API; repo hygiene; release notes for adopters.

**Tasks:**

1. `docs/specifications/idiom.md` — new subsection **Substitution slots (`*Like`)**:
   - Discriminator-driven decode via `typereg` (unchanged).
   - Prefer `name.Value()` over `DVTextValueOf(name)` for `DVTextLike`.
   - Parallel table: `*Like` (methods) vs `DataValue` / `Item` (type assert).
2. `CHANGELOG.md` `[Unreleased]` — one bullet: additive `*Like` interface methods (non-breaking); point to `openehr/rm/doc.go`.
3. `.gitignore` — add `*.tmp` (bmmgen atomic writes; complements `tmp/`).
4. **Optional (same phase, only if Phase 1–2 insufficient):** gopenehr-style typed getters on `DVTextLike` — e.g. `AsDVTextPtr() *DVText`, `AsDVCodedTextPtr() *DVCodedText` — for callers who want concrete structs without a type switch.
5. PR #24 / plan cross-links: note `wire.md` BMM-bump wording already aligned with ADR-0001 ([54d3f57](https://github.com/Cadasto/openehr-sdk-go/commit/54d3f57)) — no further wire.md edit required unless idiom cross-ref needs it.

**Definition of done:** `make spec-check` green; `idiom.md` + CHANGELOG + `.gitignore` landed; examples and `doc.go` readable without reading GAP-11 archive.

---

### Phase 4 (optional spike) — gopenehr-style union codegen

**Outcome:** ADR or research strand comparing generated `XxxUnion{Kind, any}` vs current `XxxLike` + `typereg`. **Out of scope for Phases 1–3** — do not implement unless method-based ergonomics fail acceptance review.

## Implementation checklist

| Step | Status |
|------|--------|
| **Phase 1** | |
| Phase 1 — `DVTextLike` interface: `Value()`, `DefiningCode()` | |
| Phase 1 — concrete method bodies + `like_accessors` delegates | |
| Phase 1 — `canonical_json` example `c.Name.GetValue()` | |
| Phase 1 — unit tests (`DVText` / `DVCodedText` / nil) | |
| Phase 1 — optional `TestProbe038` `loadErr` fast-fail | |
| **Phase 2** | |
| Phase 2 — `DVURILike` methods | |
| Phase 2 — `AuditDetailsLike` methods | |
| Phase 2 — `PartyIdentifiedLike` methods | |
| Phase 2 — `ObjectRefLike` methods | |
| Phase 2 — `openehr/rm/doc.go` substitution § | |
| Phase 2 — ADR-0001 step 10 cross-link | |
| Phase 2 — in-repo call-site migration (as needed) | |
| **Phase 3** | |
| Phase 3 — `docs/specifications/idiom.md` § `*Like` | |
| Phase 3 — `CHANGELOG.md` `[Unreleased]` bullet | |
| Phase 3 — `.gitignore` `*.tmp` | |
| Phase 3 — optional gopenehr-style typed getters | |
| **Gate** | |
| `make ci` green after Phases 1–3 | |
| Phase 4 — union spike (optional, not blocking) | |

## Mapping to specs

- [REQ-024](../../specifications/idiom.md) — methods replace reflection; keep typereg dispatch in codecs.
- [REQ-040](../../specifications/rm-modeling.md) — registry unchanged.
- [REQ-052](../../specifications/wire.md) — wire.md BMM-bump note (ADR-0001 step 10) already aligned ([54d3f57](https://github.com/Cadasto/openehr-sdk-go/commit/54d3f57)).
- [PROBE-038](../../specifications/conformance.md#probe-038--rm-polymorphic-decode-coverage-sdk-gap-11) — no wire assertion change.

## Cross-references

- [archive/2026-05-26-rm-polymorphic-decode-coverage.md](2026-05-26-rm-polymorphic-decode-coverage.md) — why `*Like` exists.
- [openehr/rm/like_accessors.go](../../../openehr/rm/like_accessors.go) — migration helpers (may thin after Phase 1).
- **External:** `Cadasto/gopenehr` — `internal/openehr/rm/{dv_text,content_item,item,data_value,uid_based_id,party_proxy,object_id}.go`.
