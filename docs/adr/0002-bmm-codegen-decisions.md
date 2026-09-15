# ADR 0002 — BMM code generator structural decisions

- **Status:** Accepted, 2026-05-16.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/archive/2026-05-15-bmm-codegen.md`](../plans/archive/2026-05-15-bmm-codegen.md).

## Context

The BMM-driven generator (`internal/bmmgen`, `cmd/bmmgen`) emits the SDK's openEHR domain types from pinned schemas under `resources/bmm/`. Several layout choices are non-obvious from [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) alone and would be expensive to reverse after v1. This ADR records them for reviewers and agents.

Operational BMM bumps are covered by [ADR 0001](0001-bmm-version-bump-runbook.md).

## Decision

### D1 — Flat single package per generation target

All RM classes emit into one Go package `openehr/rm/`, with one `*_gen.go` file per BMM top-level package (file names strip `org.openehr.rm.` / `org.openehr.base.` and dot-to-underscore the remainder). AOM 1.4 follows the same flat pattern at `openehr/aom/aom14/`. No nested Go sub-packages mirror the BMM tree.

### D2 — Descendant-shadows-ancestor BMM include merge

`bmm.LoadAll` allows a descendant schema's class to override an ancestor schema's class with the same name (descendant wins). Two sibling ancestor schemas containing the same class still return `ErrSchemaConflict`. Matches REQ-047 and real openEHR corpus shape (base + RM refinements).

### D3 — `typereg_gen.go` in package `rm` / `aom14`, not under `typereg/`

The hand-written `Registry` lives in `openehr/rm/typereg/`. Generated `init()` registrations live in `openehr/rm/typereg_gen.go` (and `openehr/aom/aom14/typereg_gen.go`) so constructors can reference concrete types without an `rm ↔ typereg` import cycle. RM and AOM share `typereg.Default`; discriminator strings are disjoint.

### D4 — Abstract classes flatten into concrete descendants; abstract generics are structs or interfaces

Non-generic abstract classes become Go interfaces with unexported `is<X>()` markers; their properties flatten into each concrete descendant struct. Abstract **generic** classes **with concrete descendants** (e.g. `EVENT`, `VERSION`) are also emitted as marker interfaces so codec polymorphism works — see [ADR 0003](0003-rm-event-polymorphism.md). Abstract generics **without** concrete descendants remain generic structs.

### D5 — AOM 1.4 references RM for base types (one-way import)

`openehr/aom/aom14/` imports `openehr/rm` for shared base types (`rm.HierObjectID`, etc.). AOM does not duplicate base classes. Dependency is strictly `aom14 → rm`.

### D6 — BMM functions become panic stubs; bodies live in `*_ext.go`

Every BMM `function` becomes a Go method whose body is `panic("not implemented: …")` with BMM documentation propagated as godoc. Real implementations belong in hand-written companion files only (REQ-044) — `*_ext.go`, or `*_funcs.go` for the behavioural-function set realised under [ADR 0011](0011-rm-behavioural-functions-surface.md). The generator never touches non-`_gen.go` files.

### D7 — Manual-implementation skip set suppresses chosen stubs

A curated set ([`internal/bmmgen/manual_impl.go`](../../internal/bmmgen/manual_impl.go)), keyed `OWNER.function` on the BMM declaring class, lists functions that are hand-written in a non-generated file. `renderFunctions` skips stub emission for those keys at both emit sites (the abstract-descendant loop and the concrete / abstract-generic loop), so a hand-written method does not collide with a generated panic stub (`method redeclared`). A declaring-owner key (e.g. `UID_BASED_ID.root`, `PATHABLE.item_at_path`) suppresses the stub on every concrete descendant at once. This realises the [ADR 0011](0011-rm-behavioural-functions-surface.md) surface for REQ-120..123; functions absent from the set keep emitting fail-loud stubs (D6).

### D8 — The generator emits no bespoke JSON codec methods

Superseding the emission policy `internal/bmmgen/render_jsonmar.go` and `internal/bmmgen/render_jsonunmar.go` implemented, the generator no longer emits a `MarshalJSON` / `UnmarshalJSON` pair per generated type. Canonical JSON is produced and consumed by `encoding/json/v2` ([ADR 0022](0022-canonical-json-encoding-json-v2.md)), and the properties the bespoke methods supplied are obtained as follows.

| Property | Was | Is |
|---|---|---|
| `_type` on every concrete RM value | hand-rolled prologue in each generated `MarshalJSON` | a generated `MarshalJSONTo` calling `json.MarshalEncode` on an anonymous struct whose first field is `_type` and whose second embeds a method-free alias of the class, except a class that embeds a marshaler-bearing concrete ancestor, whose alias would promote the ancestor's methods and emit the wrong `_type` (ruling R19); those take a flat wire struct that embeds nothing and copies each field, still leading with `_type` |
| `_type` on a value held in an interface | `openehr/internal/jsonpoly` (80 lines, 177 call sites) | nothing extra. `MarshalJSONTo` has a **value** receiver, so it is in the method set of both the value and the pointer; v2 dispatches to it however the concrete instance was assigned into the slot (ruling R19). The value receiver is load-bearing under a v1 entry point, not v2: `DefaultOptionsV1` sets `CallMethodsWithLegacySemantics`, which skips a pointer-receiver marshal method on an unaddressable value (an interface or map element), and `contribution`, `transport` and `testkit` stay on v1 by design, whereas a v2 entry alone would call a pointer-receiver method regardless of addressability |
| Polymorphic dispatch at a substitutable slot | `typereg.DecodeAs[T]` called from each generated `UnmarshalJSON` | one `json.UnmarshalFromFunc` per polymorphic interface, built from `typereg.Default` at init and supplied through `json.WithUnmarshalers`. Every hook **MUST** pass `dec.Options()` into any nested decode, or a deeper interface slot silently loses its hook |
| Member order | struct field order, fixed by emission order | not a contract ([REQ-052](../specifications/wire.md#req-052)); `_type` first is a SHOULD, and `Hash` sorting is `json.Deterministic(true)` set by the generated marshaler |
| Zero versus omit | `omitempty` on every generated tag | `omitzero` on pointer fields; `omitempty` retained on container fields. See the warning below |
| Shape-failure classification (`canjson.ErrInvalidShape`) | `typereg.WrapShapeError` at each generated funnel | one shared runtime helper called by every generated `UnmarshalJSONFrom`, so 110 copies of the classification collapse into one. The sentinel's contract stays [REQ-052](../specifications/wire.md#req-052) § Decode-side shape sentinel, not this ADR's |
| Nil-receiver refusal (REQ-025) | first statement of each generated `UnmarshalJSON` (`internal/bmmgen/render_jsonunmar.go:345`) | first statement of each generated `UnmarshalJSONFrom`, unchanged in force |

The invariant D8 asserts: **no per-type JSON codec logic is generated into `openehr/rm/*_gen.go` or `openehr/aom/aom14/*_gen.go` beyond the two-method delegation above.**

**Warning, on the `omitzero` row.** `go doc encoding/json` § Migrating to v2 recommends migrating `omitempty` to `omitzero` for a bool, number, pointer or interface value, and that is right for the 42 `*string`, 16 `*bool`, 23 pointer-to-number, 6 `*map[string]T` and 5 `*any` fields the generator emits. It is wrong for container fields. v2's `omitempty` omits a field that encodes as an empty JSON value, so a nil slice and a non-nil empty slice are both omitted, which is the collapse [REQ-052](../specifications/wire.md#req-052)'s `DV_TEXT.mappings` re-encode collapse clause documents and `TestDVTextMappingsDecodePresenceAndEncodeCollapse` pins. Under `omitzero` a non-nil empty slice is not zero and would emit `[]`, which that clause states would be RM-invalid (`Mappings_valid`). A blanket sweep would break an RM invariant silently.

## Consequences

- Consumers construct COMPOSITION values from a single `rm` import; no six-package import fan-out.
- Calling unimplemented BMM functions panics by design — see package doc on `openehr/rm`.
- `make test` chains `codegen-verify`; hand-edits to `*_gen.go` fail CI.
- [ADR 0003](0003-rm-event-polymorphism.md)'s whitelist stands, and its justification is re-homed. It was adopted because `encoding/json` could not select a concrete shape at `HISTORY.events` and the generated `UnmarshalJSON` copied the slice without dispatch. With D8 that generated method is gone, so the *codec* argument no longer applies, but the *type-shape* decision does: `History[T].Events` is `[]Event`, an interface slice, and that is the Go API the SDK ships. The whitelist is a public-surface decision now, not a codec workaround, so changing it would be a breaking API change rather than a codec tuning knob.

## References

- [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md)
- [`docs/architecture.md`](../architecture.md) — narrative companion
- [`internal/bmmgen/`](../../internal/bmmgen/)
