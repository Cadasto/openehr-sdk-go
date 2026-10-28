---
kind: adr
id: ADR-0002
title: "BMM code generator structural decisions"
status: accepted
date: 2026-05-16
---

# ADR 0002 — BMM code generator structural decisions

- **Status:** Accepted, 2026-05-16.
- **Supersedes:** —
- **Superseded by:** —
- **Amended:** D4, 2026-10-01 (the `P_BMM_INTERFACE` shape). D8, 2026-10-11 (the interval classes' boundary-flag logic). D9 added, 2026-10-12 (canonical-XML unmatched content). D10 added, 2026-10-26 (canonical-XML child order from the ITS-XML schemas). D9, 2026-10-26 (the decoders hand the runtime a description of their class on entry, [ADR 0033](0033-missing-members-reported-by-decoders.md)). D9 and D10, 2026-10-28 (the AM 1.4 schema type names, and the members the schema declares as an element of another name).
- **Tracks:** the BMM code generator, landed on `main` in commit [b89aa589](https://github.com/Cadasto/openehr-sdk-go/commit/b89aa589).
- **Requirements:** [REQ-043](../specifications/bmm-conformance.md#mapping-rules) (the P_BMM to Go mapping rules the generator applies); [REQ-056](../specifications/wire.md#req-056) (the canonical-XML child order D10 supplies).

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

A `P_BMM_INTERFACE` class is emitted as a Go interface with the unexported `is<X>()` marker and no methods: the generator emits nothing for its `functions`. The RM has two, `CODE_SET_ACCESS` and `TERMINOLOGY_ACCESS`, which the BMM declares with four and six functions; nothing in the SDK implements or consumes them, so both are marker-only. Emitting their functions as interface methods was considered and not taken: it would put ten methods on the public surface with no implementer and no reader, and the BMM's signatures would be frozen there. The generator also gives an interface no marker on a descendant, because the pinned BMM sets no `is_abstract` on any interface ([STRAND-12](../specifications/research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag)) and no class in it descends from one; propagating the functions to concrete descendants is left until a BMM bump supplies one.

### D5 — AOM 1.4 references RM for base types (one-way import)

`openehr/aom/aom14/` imports `openehr/rm` for shared base types (`rm.HierObjectID`, etc.). AOM does not duplicate base classes. Dependency is strictly `aom14 → rm`.

### D6 — BMM functions become panic stubs; bodies live in `*_ext.go`

Every BMM `function` of a class becomes a Go method whose body is `panic("not implemented: …")` with BMM documentation propagated as godoc; a `P_BMM_INTERFACE`'s functions get no method (D4). Real implementations belong in hand-written companion files only (REQ-044) — `*_ext.go`, or `*_funcs.go` for the behavioural-function set realised under [ADR 0011](0011-rm-behavioural-functions-surface.md). The generator never touches non-`_gen.go` files.

### D7 — Manual-implementation skip set suppresses chosen stubs

A curated set ([`internal/bmmgen/manual_impl.go`](../../internal/bmmgen/manual_impl.go)), keyed `OWNER.function` on the BMM declaring class, lists functions that are hand-written in a non-generated file. `renderFunctions` skips stub emission for those keys at both emit sites (the abstract-descendant loop and the concrete / abstract-generic loop), so a hand-written method does not collide with a generated panic stub (`method redeclared`). A declaring-owner key (e.g. `UID_BASED_ID.root`, `PATHABLE.item_at_path`) suppresses the stub on every concrete descendant at once. This realises the [ADR 0011](0011-rm-behavioural-functions-surface.md) surface for REQ-120..123; functions absent from the set keep emitting fail-loud stubs (D6).

### D8 — The generator emits no bespoke JSON codec methods

Superseding the emission policy `internal/bmmgen/render_jsonmar.go` and `internal/bmmgen/render_jsonunmar.go` implemented, the generator no longer emits a `MarshalJSON` / `UnmarshalJSON` pair per generated type. Canonical JSON is produced and consumed by `encoding/json/v2` ([ADR 0022](0022-canonical-json-encoding-json-v2.md)), and the properties the bespoke methods supplied are obtained as follows.

| Property | Was | Is |
|---|---|---|
| `_type` on every concrete RM value | hand-rolled prologue in each generated `MarshalJSON` | a generated `MarshalJSONTo` calling `json.MarshalEncode` on an anonymous struct whose first field is `_type` and whose second embeds a method-free alias of the class, except a class that embeds a marshaler-bearing concrete ancestor, whose alias would promote the ancestor's methods and emit the wrong `_type` (ruling R19); those take a flat wire struct that embeds nothing and copies each field, still leading with `_type` |
| `_type` on a value held in an interface | `openehr/internal/jsonpoly` (80 lines, 177 call sites) | nothing extra. `MarshalJSONTo` has a **value** receiver, so it is in the method set of both the value and the pointer; v2 dispatches to it however the concrete instance was assigned into the slot (ruling R19). The value receiver is load-bearing under a v1 entry point, not v2: `DefaultOptionsV1` sets `CallMethodsWithLegacySemantics`, which skips a pointer-receiver marshal method on an unaddressable value (an interface or map element), and `contribution`, `transport` and `testkit` stay on v1 by design, whereas a v2 entry alone would call a pointer-receiver method regardless of addressability |
| Polymorphic dispatch at a substitutable slot | `typereg.DecodeAs[T]` called from each generated `UnmarshalJSON` | one `json.UnmarshalFromFunc` per polymorphic interface, built from `typereg.Default` at init and supplied through `json.WithUnmarshalers`. Every hook passes `dec.Options()` into any nested decode; without it a deeper interface slot silently loses its hook |
| Member order | struct field order, fixed by emission order | not a contract; [REQ-052](../specifications/wire.md#req-052) recommends `_type` first, and `Hash` sorting is `json.Deterministic(true)` set by the generated marshaler |
| Zero versus omit | `omitempty` on every generated tag | optional: `omitzero` on pointer fields, `omitempty` retained on container fields (see the warning below). Mandatory: `omitzero` on list, map, pointer and `any` fields, `omitempty` on an interface or type-parameter field, so an absent mandatory member is left out ([REQ-052](../specifications/wire.md#req-052) § An absent mandatory member); a bare tag on any other field |
| Shape-failure classification (`canjson.ErrInvalidShape`) | `typereg.WrapShapeError` at each generated funnel | one shared runtime helper called by every generated `UnmarshalJSONFrom`, so 110 copies of the classification collapse into one. The sentinel's contract stays [REQ-052](../specifications/wire.md#req-052) § Decode-side shape sentinel, not this ADR's |
| Nil-receiver refusal (REQ-025) | first statement of each generated `UnmarshalJSON` (`internal/bmmgen/render_jsonunmar.go:345`) | first statement of each generated `UnmarshalJSONFrom`, unchanged in force |

The invariant D8 asserts: **no per-type JSON codec logic is generated into `openehr/rm/*_gen.go` or `openehr/aom/aom14/*_gen.go` beyond the two-method delegation above, except for the interval classes.** An interval's codec has to know which of its four boundary flags the wire carried and whether a side is open: encode omits an open side's empty bound ([REQ-052](../specifications/wire.md#req-052)), and decode gives an absent flag its default ([REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults)), neither of which the delegation can see. That logic is generated for every class carrying the four flags, BASE `Interval` included, so the AOM 1.4 constraint intervals get it too, and the generator refuses a flag-carrying class whose shape it does not handle.

**Warning, on the `omitzero` row.** `go doc encoding/json` § Migrating to v2 recommends migrating `omitempty` to `omitzero` for a bool, number, pointer or interface value, and that is right for the 42 `*string`, 16 `*bool`, 23 pointer-to-number, 6 `*map[string]T` and 5 `*any` fields the generator emits. Those counts omit the `*[]string` field `CodeList`. It is wrong for container fields. v2's `omitempty` omits a field that encodes as an empty JSON value, so a nil slice and a non-nil empty slice are both omitted, which is the collapse [REQ-052](../specifications/wire.md#req-052)'s `DV_TEXT.mappings` re-encode collapse clause documents and `TestDVTextMappingsDecodePresenceAndEncodeCollapse` pins. Under `omitzero` a non-nil empty slice is not zero and would emit `[]`, which that clause states would be RM-invalid (`Mappings_valid`). A blanket sweep would break an RM invariant silently. A mandatory list or map takes `omitzero` on purpose: REQ-052 writes a present empty one as `[]` or `{}`, and leaves out only a nil one. A mandatory interface or type-parameter field takes `omitempty` instead, because a typed-nil pointer is not zero and encodes as `null`, while every concrete RM value encodes with its `_type` and so is never empty; `any` takes `omitzero`, since an empty string or object it holds is a value.

### D9 — The generated XML decoders hand what they do not read to an internal runtime

Canonical XML decodes through a generated `UnmarshalXML` per class, and [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) asks the codec's entry points for options that report or refuse unmatched content. `encoding/xml` passes such a method only the `*xml.Decoder` and the start element, so no option can reach the generated code as an argument.

The generated decoders therefore read nothing they do not model themselves. On entry each hands its element's attributes, together with a static description of its class the generator emits beside the method (the class name, the properties carried as XML attributes, the mandatory members a document must carry, the multi-valued members, and the members also read from a child element of another name), which the runtime reads to report the mandatory members the document leaves out ([REQ-175](../specifications/wire.md#req-175--missing-members-on-decode), [ADR 0033](0033-missing-members-reported-by-decoders.md)) without an RM table of its own; in its character-data arm the text between its child elements, and in its fallback arm every child it does not read (an unknown element, an element in a foreign namespace, a `Hash` member, the content of a BMM `Any` member, a member whose type has no decoder of its own), to helpers in the internal package `openehr/internal/xmlunmatched`. A primitive leaf and a member typed by a generic parameter decode through the same package. When a caller sets an option, `canxml` builds the decoder through that package, which tracks the open elements and registers the decoder with the decode's own policy for the length of each value. Without an option, each helper finds no registration and does what `encoding/xml` would, with one exception: text that a primitive's type refuses comes back from the helpers as a `DecodeError` naming the element, in every decode, never as the conversion's bare error. A tracked decode names the element by its path from the root; `canxml.Unmarshal` without an option finds that path by decoding the input a second time, tracked, once the first decode has failed so. The helpers stay off `canxml`'s public API: `canxml` exposes only the options and re-exports the sentinels. The generator refuses no class for this; every class-typed arm goes either to a decoder the generator emitted or to the helpers, which its test checks.

### D10 — The generated XML encoders take their child order from the ITS-XML schemas

In canonical XML, element order is part of a document's identity ([REQ-056](../specifications/wire.md#req-056)), and a reader that validates against the openEHR ITS-XML schemas refuses children that leave their type's `xs:sequence`. The BMM gives no such order. Each class declares its own properties, and the merged property list the generator builds (D2, D4) puts them in an order the schemas do not follow: OBSERVATION's `protocol` came before LOCATABLE's `name`. On 2026-10-26, 62 of the 140 generated XML encoders wrote their children out of the schemas' order.

The generator therefore reads a second pinned input beside `resources/bmm/`: the ITS-XML schemas vendored under `resources/its-xml/`, RM and BASE Release-1.1.0, and AM Release-1.4 for the AOM 1.4 classes. For each class it takes the schema type of the same name, or of its nearest ancestor when the schema has none, with that type's `xs:extension` bases, the bases' sequences first, and it emits the class's children in that order. A class the AM 1.4 schema names differently takes the schema type of that name (`Target.XMLTypeNames` lists the three). A member the schema declares as an element of another name takes that element's place: `C_CODED_TEXT.terminology` is written where `C_CODE_PHRASE` declares `terminology_id`, before `code_list` (`Target.XMLIdentifierElements`). A class generated into `openehr/rm` is looked up in the RM and BASE schemas, and an AOM 1.4 class in the AM schemas first. Four BASE resource classes (`AUTHORED_RESOURCE`, `RESOURCE_DESCRIPTION`, `RESOURCE_DESCRIPTION_ITEM`, `TRANSLATION_DETAILS`) are declared in both sets, the AM 1.4 copy without the elements BASE added and with ones BASE dropped. The SDK writes them through one `openehr/rm` encoder, so a property its own set does not declare is placed where the other set's sequence puts it, right after the nearest earlier property both declare: `RESOURCE_DESCRIPTION_ITEM.copyright`, which only AM 1.4 declares, comes after `misuse`. Any other property the schema does not declare follows the declared ones, in the generator's own order, so no property is dropped: one a later release added, such as `EHR.tags`, or an interval's bounds, which the schema declares only on its `IntervalOf…` subtypes. The decoders keep matching children by name in any order. A test holds every generated encoder to the vendored schemas, and `make codegen-check` holds the generated files to the generator.

## Consequences

- Consumers construct COMPOSITION values from a single `rm` import; no six-package import fan-out.
- Calling unimplemented BMM functions panics by design — see package doc on `openehr/rm`.
- `make test` chains `codegen-verify`; hand-edits to `*_gen.go` fail CI.
- [ADR 0003](0003-rm-event-polymorphism.md)'s whitelist stands, and its justification is re-homed. It was adopted because `encoding/json` could not select a concrete shape at `HISTORY.events` and the generated `UnmarshalJSON` copied the slice without dispatch. With D8 that generated method is gone, so the *codec* argument no longer applies, but the *type-shape* decision does: `History[T].Events` is `[]Event`, an interface slice, and that is the Go API the SDK ships. The whitelist is a public-surface decision now, not a codec workaround, so changing it would be a breaking API change rather than a codec tuning knob.
- D10 makes the vendored ITS-XML schemas a second pinned input to the generator: a re-pin can re-order the generated XML encoders, and `make codegen-check` fails until `make codegen` is rerun.

## References

- [`docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md)
- [`docs/architecture.md`](../architecture.md) — narrative companion
- [`internal/bmmgen/`](../../internal/bmmgen/)
