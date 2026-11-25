---
kind: adr
id: ADR-0002
title: "BMM code generator structural decisions"
status: accepted
date: 2026-05-16
---

# ADR 0002 — BMM code generator structural decisions

- **Status:** Accepted, 2026-05-16.
- **Amended:** 2026-11-25: D4 states which abstract generics become interfaces, D11 states the version-bump rule, and the JSON encoder design, D8 until now, is [ADR 0022](0022-canonical-json-encoding-json-v2.md)'s.
- **Requirements:** [REQ-041](../specifications/reference-model.md#req-041--pinned-bmm-sources) (the pinned sources, the mapping rules the generator applies and its drift check), [REQ-056](../specifications/wire.md#req-056) (the canonical-XML child order D10 supplies); the type shape D4 sets is summarised in [reference-model.md § The shape of the RM in Go](../specifications/reference-model.md#the-shape-of-the-rm-in-go).
- **Related:** [ADR 0022](0022-canonical-json-encoding-json-v2.md) (the JSON codec the generator emits), [ADR 0034](0034-sdk-authored-root-schemas.md) (the SDK-authored root schemas D2, D5 and D10 serve).

## Context

The BMM-driven generator (`internal/bmmgen`, run by `cmd/bmmgen`) emits the SDK's openEHR domain types from the pinned schemas under `resources/bmm/`.
Go has no inheritance, the RM's class tree is deep, and a schema bump regenerates thousands of lines, so the layout choices below are not obvious from [reference-model.md](../specifications/reference-model.md) alone and would be expensive to reverse after v1.

## Decision

### D1 — Flat single package per generation target

All RM classes emit into one Go package `openehr/rm/`, with one `*_gen.go` file per BMM top-level package (file names strip `org.openehr.rm.` / `org.openehr.base.` and dot-to-underscore the remainder). AOM 1.4 follows the same flat pattern at `openehr/aom/aom14/`. No nested Go sub-packages mirror the BMM tree.

### D2 — Descendant-shadows-ancestor BMM include merge

`bmm.LoadAll` allows a descendant schema's class to override an ancestor schema's class with the same name (descendant wins). Two sibling ancestor schemas containing the same class still return `ErrSchemaConflict`. Matches the real openEHR corpus shape (base + RM refinements). The SDK-authored root schemas of [ADR 0034](0034-sdk-authored-root-schemas.md) rest on this rule: a root amends a pinned class by declaring it again. The rule also holds where two includes meet again, so the AOM 1.4 root, which includes the AOM 1.4 schema and the RM root, takes the RM form of the classes BASE and RM both declare; [§ REQ-045](../specifications/reference-model.md#req-045--bmm-loader-is-a-building-block) states the merge.

### D3 — `typereg_gen.go` in package `rm` / `aom14`, not under `typereg/`

The hand-written `Registry` lives in `openehr/rm/typereg/`. Generated `init()` registrations live in `openehr/rm/typereg_gen.go` (and `openehr/aom/aom14/typereg_gen.go`) so constructors can reference concrete types without an `rm ↔ typereg` import cycle. RM and AOM share `typereg.Default`; discriminator strings are disjoint.

### D4 — Abstract classes flatten into concrete descendants; abstract generics are structs or interfaces

A non-generic abstract class is a Go interface with an unexported `is<X>()` marker, and its properties are flattened into each concrete descendant struct.
An abstract generic class is a generic struct that its concrete descendants embed: `OriginalVersion[T]` and `ImportedVersion[T]` embed `Version[T]`.
A class on the whitelist `codecPolymorphicAbstractGenericNames` in [`internal/bmmgen/render.go`](../../internal/bmmgen/render.go) is emitted the way a non-generic abstract class is: a marker interface with no type parameter, which each descendant implements for every type argument.
The whitelist holds `EVENT` alone, so `Event` is `interface{ isEvent() }`, `PointEvent[T]` and `IntervalEvent[T]` carry the EVENT properties as their own, and `History[T].Events` is `[]Event`, a list that holds either kind and decodes each by its `_type`.
A class joins the whitelist when a slot has to hold its descendants polymorphically; `VERSION` stays a struct until one does.
The whitelist is public API: moving a class onto it or off it turns a struct into an interface or back, a breaking change.

A `P_BMM_INTERFACE` class is emitted as a Go interface with the unexported `is<X>()` marker and no methods: the generator emits nothing for its `functions`. The RM has two, `CODE_SET_ACCESS` and `TERMINOLOGY_ACCESS`, which the BMM declares with four and six functions; nothing in the SDK implements or consumes them, so both are marker-only. Emitting their functions as interface methods was considered and not taken: it would put ten methods on the public surface with no implementer and no reader, and the BMM's signatures would be frozen there. The generator also gives an interface no marker on a descendant, because the pinned BMM sets no `is_abstract` on any interface ([STRAND-12](../specifications/research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag)) and no class in it descends from one; propagating the functions to concrete descendants is left until a BMM bump supplies one.

### D5 — AOM 1.4 references RM for base types (one-way import)

`openehr/aom/aom14/` imports `openehr/rm` for shared base types (`rm.HierObjectID`, etc.). AOM does not duplicate base classes. Dependency is strictly `aom14 → rm`. The AOM 1.4 root of [ADR 0034](0034-sdk-authored-root-schemas.md) binds `AUTHORED_RESOURCE` to the RM `common.resource` form, so the resource classes it references are the ones `openehr/rm` holds.

### D6 — BMM functions become panic stubs; bodies live in `*_ext.go`

Every BMM `function` of a class becomes a Go method whose body is `panic("not implemented: …")` with BMM documentation propagated as godoc; a `P_BMM_INTERFACE`'s functions get no method (D4). Real implementations belong in hand-written companion files only ([REQ-041](../specifications/reference-model.md#req-041--pinned-bmm-sources)) — `*_ext.go`, or `*_funcs.go` for the behavioural-function set realised under [ADR 0011](0011-rm-behavioural-functions-surface.md). The generator never touches non-`_gen.go` files.

### D7 — Manual-implementation skip set suppresses chosen stubs

A curated set ([`internal/bmmgen/manual_impl.go`](../../internal/bmmgen/manual_impl.go)), keyed `OWNER.function` on the BMM declaring class, lists functions that are hand-written in a non-generated file. `renderFunctions` skips stub emission for those keys at both emit sites (the abstract-descendant loop and the concrete / abstract-generic loop), so a hand-written method does not collide with a generated panic stub (`method redeclared`). A declaring-owner key (e.g. `UID_BASED_ID.root`, `PATHABLE.item_at_path`) suppresses the stub on every concrete descendant at once. This realises the [ADR 0011](0011-rm-behavioural-functions-surface.md) surface for REQ-120..123; functions absent from the set keep emitting fail-loud stubs (D6).

### D9 — The generated XML decoders hand what they do not read to an internal runtime

Canonical XML decodes through a generated `UnmarshalXML` per class, and [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode) asks the codec's entry points for options that report or refuse unmatched content. `encoding/xml` passes such a method only the `*xml.Decoder` and the start element, so no option can reach the generated code as an argument.

The generated decoders therefore read nothing they do not model themselves, and hand the rest to helpers in the internal package `openehr/internal/xmlunmatched`:

- on entry, the element's attributes and a static description of its class, which the generator emits beside the method: the class name, the properties carried as XML attributes, the mandatory members and the multi-valued members, so the runtime reports missing members ([REQ-175](../specifications/wire.md#req-175--missing-members-on-decode), [ADR 0033](0033-missing-members-reported-by-decoders.md)) with no RM table of its own;
- in the character-data arm, the text between its child elements;
- in the fallback arm, every child it does not read: an unknown element, an element in a foreign namespace, a `Hash` member, the content of a BMM `Any` member, a member whose type has no decoder of its own;
- in the arm of each single-valued member, a second child element of that member, tracked with one bit per member, and a nilled element.

A primitive leaf and a member typed by a generic parameter decode through the same package.
When a caller sets an option, `canxml` registers the decode's policy with the helpers for the length of each value; without one, each helper does what `encoding/xml` would.
What a decode then reports, refuses or reads as absent is [REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode), [REQ-175](../specifications/wire.md#req-175--missing-members-on-decode) and [REQ-056](../specifications/wire.md#req-056)'s.
The helpers stay off `canxml`'s public API: `canxml` exposes only the options and re-exports the sentinels.
The generator refuses no class for this, bar one with more than 64 single-valued members, which the repeat guard's bits cannot hold and neither the RM nor AOM 1.4 has; every class-typed arm goes either to a decoder the generator emitted or to the helpers, which its test checks.

### D10 — The generated XML encoders take their child order from the ITS-XML schemas

In canonical XML, element order is part of a document's identity ([REQ-056](../specifications/wire.md#req-056)), and a reader that validates against the openEHR ITS-XML schemas refuses children that leave their type's `xs:sequence`. The BMM gives no such order. Each class declares its own properties, and the merged property list the generator builds (D2, D4) puts them in an order the schemas do not follow: OBSERVATION's `protocol` came before LOCATABLE's `name`, and 62 of the 140 generated XML encoders wrote their children out of the schemas' order.

The generator therefore reads a second pinned input beside `resources/bmm/`: the ITS-XML schemas vendored under `resources/its-xml/`, RM Release-1.1.0 with BASE Release-1.2.0, and AM Release-1.4 for the AOM 1.4 classes. For each class it takes the schema type of the same name, or of its nearest ancestor when the schema has none, with that type's `xs:extension` bases, the bases' sequences first, and it emits the class's children in that order. Under [ADR 0034](0034-sdk-authored-root-schemas.md) it also takes from that type which members are XML attributes, in place of its former name check on `archetype_node_id`, a rule the operational template model ([REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model)) needs. A class generated into `openehr/rm` is looked up in the RM and BASE schemas, and an AOM 1.4 class in the AM schemas first. Four BASE resource classes (`AUTHORED_RESOURCE`, `RESOURCE_DESCRIPTION`, `RESOURCE_DESCRIPTION_ITEM`, `TRANSLATION_DETAILS`) are declared in both sets, the AM 1.4 copy without the elements BASE added and with ones BASE dropped. The SDK writes them through one `openehr/rm` encoder, so a property its own set does not declare is placed where the other set's sequence puts it, right after the nearest earlier property both declare: `RESOURCE_DESCRIPTION_ITEM.copyright`, which only AM 1.4 declares, comes after `misuse`. Any other property the schema does not declare follows the declared ones, in the generator's own order, so no property is dropped: one a later release added, such as `EHR.tags`, or an interval's bounds, which the schema declares only on its `IntervalOf…` subtypes. The decoders keep matching children by name in any order. A test holds every generated encoder to the vendored schemas, and `make codegen-check` holds the generated files to the generator.

### D11 — A BMM version bump is one pull request

A BMM version bump is one pull request. The new schema is added beside the old one and the old one removed in the same commit, so the diff shows the pair and the SDK pins one version per schema id. The steps are in [`resources/bmm/README.md` § Updating](../../resources/bmm/README.md#updating), and [`internal/bmmgen/sim_bump_test.go`](../../internal/bmmgen/sim_bump_test.go) runs a simulated bump on every CI run.

## Consequences

- Consumers construct COMPOSITION values from a single `rm` import; no six-package import fan-out.
- Calling an unimplemented BMM function panics by design; see the package doc of `openehr/rm`.
- Generated files are never edited by hand. `make test` runs `make codegen-check`, which holds every `*_gen.go` file to the generator, and the weekly [`codegen-drift`](../../.github/workflows/codegen-drift.yml) workflow re-runs it on a clean checkout and tracks a failure in one `bmm-drift` issue.
- A BMM version bump is one reviewable pull request with one pinned version per schema id (D11).
- D10 makes the vendored ITS-XML schemas a second pinned input to the generator: a re-pin can re-order the generated XML encoders, and `make codegen-check` fails until `make codegen` is rerun.

## References

- [`docs/specifications/reference-model.md`](../specifications/reference-model.md)
- [`docs/architecture.md`](../architecture.md) — narrative companion
- [`internal/bmmgen/`](../../internal/bmmgen/)
- [`resources/bmm/README.md`](../../resources/bmm/README.md) — the pinned schema inventory and the bump procedure

## Traceability

- Applies: REQ-041 (its mapping rules), REQ-056
