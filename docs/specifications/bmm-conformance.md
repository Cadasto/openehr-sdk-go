---
kind: specification
---

# BMM conformance

**Status:** Draft

The SDK's domain types (Reference Model, Archetype Object Model, base + language + terminology types) are **derived from openEHR BMM schemas** pinned in [`../resources/bmm/`](../../resources/bmm). This document is the normative contract between those BMM inputs and the Go code in `openehr/rm/`, `openehr/aom/aom14/`, and related packages.

Covers REQ-041, REQ-045, REQ-046 and REQ-048; the retired REQ-042, REQ-043, REQ-044 and REQ-047 are prose under REQ-041, and the retired REQ-049 is part of REQ-048.

## The BMM substrate

openEHR distributes its information and meta-models as **BMM schemas** — Basic Meta-Model files in the `P_BMM` JSON persistence format. Every class, property, generic parameter, function signature, primitive type, cardinality, and documentation string the SDK needs is in those files. The BMM is machine-readable and lossless with respect to UML; it is the substrate the openEHR ecosystem uses for archetype tooling, AQL path resolution, and code generation across implementations.

Pinned schemas live in [`../resources/bmm/`](../../resources/bmm):

| File | Schema ID | v1 status | Role |
|---|---|---|---|
| `openehr_base_1.3.0.bmm.json` | base | **primary** | Primitive types + foundational classes |
| `openehr_rm_1.2.0.bmm.json` | rm | **primary** (excluding `ehr_extract` package) | Reference Model (clinical + demographic) |
| `openehr_am_1.4.0.bmm.json` | am-1.4 | **primary** | Archetype Object Model 1.4 (ADL 1.4) |
| `openehr_am_2.4.0.bmm.json` | am-2 | deferred | Archetype Object Model 2 (ADL 2 / AOM 2) — file kept for future generation |
| `openehr_lang_1.1.0.bmm.json` | lang | deferred | Language types — file kept as reference while the hand-written `openehr/bmm/` loader covers v1 needs |
| `openehr_term_3.1.0.bmm.json` | term | deferred | Terminology service interface — file kept for future generation |

**Primary** files drive v1 code generation. **Deferred** files stay in `resources/bmm/` (no removal) but the generator does not emit Go code from them in v1 — the BMM-conformance contract continues to apply if and when a future phase wires them in.

## Requirements

### REQ-041 — Pinned BMM sources

The BMM files in `resources/bmm/` are the source of truth for the openEHR domain model. The generated RM and AOM 1.4 types, their primitive mappings and their type registry entries **MUST** correspond to the classes and properties of the pinned BMM files, mapped by [§ Mapping rules](#mapping-rules) and [§ Primitive type mapping](#primitive-type-mapping); hand-written code embeds no out-of-band information about openEHR types.

How the SDK keeps that correspondence:

- **Generated, reproducibly.** An in-tree generator (`internal/bmmgen`, run by `cmd/bmmgen`) generates `openehr/rm/`, `openehr/rm/typereg/`, `openehr/rm/rminfo/` and `openehr/aom/aom14/` from the **primary** BMM files in the table above. It is reproducible: two runs over the same inputs give byte-identical output.
- **Drift-checked.** The build gate re-runs the generator and fails when the working tree differs from its output, which is the mechanical guarantee against a hand edit silently diverging from the BMM.
- **Scope.** The generator skips the BMM package `org.openehr.rm.ehr_extract` and its sub-packages, and the deferred BMM files (`am 2.4.0`, `lang 1.1.0`, `term 3.1.0`) are not its inputs in v1.
- **Generated and hand-written files stay apart.** A generated file carries the standard generated-code header ([§ Generator output conventions](#generator-output-conventions)) and is never edited by hand. Hand-written code in a BMM-derived package lives in its own files, documented in a `doc.go` or named by the `<file>_ext.go` convention; it consumes the generated types, never redefines a generated type or method, and never weakens or overrides an invariant the BMM declares. A hand-written package can hold a generated `*_gen.go` file that the drift check covers like any other output, such as the generated attribute access in `internal/rmaccess` and the invariant inventory in `openehr/validation`, which a hand-kept `invariants.json` classifies.
- **The BMM wins.** Where a BMM file declares a class, property or cardinality that contradicts a statement elsewhere in the openEHR specifications, the SDK follows the BMM. The BMM is the computable form the openEHR ecosystem uses for validation, AQL path typing and tooling, so diverging from it would make archetype validation disagree with templates compiled from it, and it is the form the openEHR Foundation actively maintains while spec prose lags. A suspected bug in a BMM file is raised upstream, in the openEHR specifications repository, rather than worked around in the SDK. A deviation from the mapping rules takes an ADR and an update to [§ Mapping rules](#mapping-rules).

The openEHR Terminology vocabulary is pinned the same way under `resources/terminology/` ([rm-modeling.md § REQ-034](rm-modeling.md#openehr-terminology-vocabulary-req-034)).

### REQ-042 — Generated code, drift-detected

**Retired.** Generation and its drift check are now prose under [REQ-041](#req-041--pinned-bmm-sources). The identifier is never reused.

### REQ-043 — Mapping rules: P_BMM → Go

**Retired.** The mapping rules now bind through [REQ-041](#req-041--pinned-bmm-sources) and live in [§ Mapping rules](#mapping-rules). The identifier is never reused.

### REQ-044 — Hand-written extensions are isolated

**Retired.** The separation of generated and hand-written files is now prose under [REQ-041](#req-041--pinned-bmm-sources). The identifier is never reused.

### REQ-045 — BMM loader is a building block

`openehr/bmm/` **MUST** be a public loader for BMM schemas that a consumer — a validator, an archetype tool, a custom code generator, a BMM-aware diff tool — can import without `transport/`, `auth/` or any HTTP machinery. Its import rule is in the table of [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence).

The loader exposes:

```go
package bmm

// Load parses a P_BMM JSON document into an in-memory model.
func Load(r io.Reader) (*Schema, error)

// LoadAll resolves includes by file ID and returns a fully merged schema.
func LoadAll(rootID string, resolver Resolver) (*Schema, error)

// Resolver hands the loader the bytes for a given schema ID.
type Resolver interface {
    Resolve(ctx context.Context, schemaID string) (io.ReadCloser, error)
}
```

Schema, Class, Property, Type, FunctionParameter are the in-memory equivalents of the `P_BMM_*` persistence types — abstract over the four property variants and three type variants (see § Mapping rules).

### REQ-046 — Primitive type mapping

The SDK **MUST** map openEHR primitive types to Go types per the table in [§ Primitive type mapping](#primitive-type-mapping) below. The mapping is fixed; alternative widenings (e.g. mapping `Real` to `float32`) are not permitted.

`Character` is the one primitive whose canonical JSON spelling (a one-character string) disagrees with a numeric Go `rune`, so it maps to a named type carrying its own codec — the mapping is still fixed, this is the fixed target.

### REQ-047 — BMM-spec divergence resolution

**Retired.** The rule that the BMM wins over spec prose is now prose under [REQ-041](#req-041--pinned-bmm-sources). The identifier is never reused.

### REQ-048 — RM meta-model introspection surface

The SDK **MUST** expose the pinned Reference Model's **class graph** — not only the per-class attribute shape it already answers — as a **compiled-in** introspection surface, so that a consumer reasoning about the RM rather than about one template (AQL class-expression expansion and `CONTAINS` conformance, polymorphic slot fit, validation walkers, BMM-faithful re-serialisation and schema diffing) does not have to re-reduce the pinned BMM for itself.

The surface is generated from the **primary** RM BMM file by the generator of [REQ-041](#req-041--pinned-bmm-sources), and it loads, parses and resolves no BMM schema at runtime. Its package, `openehr/rm/rminfo`, keeps to the import rule [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence) gives it: a compiled-in table needs nothing else, and an import of this module's packages, `openehr/bmm` most pointedly, would turn it back into a runtime reduction. This extends [ADR 0005](../adr/0005-compiled-template-foundation.md) C1 from the attribute tables to the whole emitted class universe.

#### The class universe

The surface's **class universe** **MUST** be the classes the **RM generation target** emits from the BMM's `class_definitions` — which spans the primary RM schema *and* the BASE classes it includes, since the target emits both into `openehr/rm/` — minus enumerations and minus the foundation typing classes that target skips as typing constraints rather than data. Deriving the universe from the generation target rather than from a list written here is deliberate: a hand-kept list would rot against the pinned schemas, and the equality is exactly what [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm) checks.

Two exclusions sit *outside* that rule rather than inside it, and conflating them with it would misdescribe the surface:

- The `org.openehr.rm.ehr_extract` package never reaches emission at all ([REQ-041](#req-041--pinned-bmm-sources)), so it is excluded upstream of the universe.
- The `primitive_types` entries of [§ Primitive type mapping](#primitive-type-mapping) are not in the universe, **even where the target does emit a Go type for them** — `Cardinality`, `Interval`, `Multiplicity_interval`, `Terminology_code` and `Terminology_term` are emitted as Go values, and are still out. They are values the RM *uses*, not RM classes to reason *about*. "Not in the universe" therefore means *this surface does not model the class*, which is **not** the same as *no Go type exists for it*.

The universe therefore includes the classes that carry no attributes of their own or inherited, and so appear in no attribute table — the abstract bases `DATA_VALUE` and `PATHABLE`, the `support` terminology/measurement access and service classes, and the definition-constant holders. A class the generation target emits but the universe omitted could not be asked about at all, and a descendant expansion rooted at an abstract class would be unanswerable while that class was absent.

The **class-graph** answers — immediate parents, transitive ancestors, conformance, concrete descendants — **MUST** be closed over that universe without losing an RM edge: every class name they return is itself in it, a BMM ancestor outside it is never reported, and a class whose every BMM ancestor lies outside it is a **root**.

The **attribute declaration site** is deliberately **not** closed, because it belongs to the attribute layer rather than to the graph. It **MUST** name the BMM class that actually declares the attribute, and that class can lie outside the universe when the attribute is inherited from an excluded `primitive_types` entry. On the pinned RM the only such site is `Interval`, which declares the bounds attributes `DV_INTERVAL`, `Point_interval` and `Proper_interval` carry; clamping those to the carrying class would report six attributes as locally declared by three classes that declare none of them, which is the very loss this question exists to prevent. The flattened attribute lookup already behaves this way — it reports primitives and generic parameters (`String`, `Integer`, `T`) as attribute types — so the split is consistent: **the attribute layer is faithful to the BMM, the class graph is navigable.** A caller handed an out-of-universe site can tell, because every class-graph question about that name reports not-known.

What the closing filter removes is exactly the excluded kinds, not RM structure: the [§ Primitive type mapping](#primitive-type-mapping) entries that appear as BMM ancestors — `Any`, `Ordered`, `Interval`, `Iso8601_type` and the four `Iso8601_*` value types — plus the `PROPORTION_KIND` enumeration. The two lists are the same list, which is why the universe rule and the filter cannot drift apart. Every class that names one of those also names an RM ancestor beside it, and that is asserted rather than assumed, since an RM edge lost to the filter would silently shrink every descendant expansion above it. Four classes have no in-universe ancestor left and are therefore roots — `PATHABLE` (its only ancestor is `Any`), `Point_interval` and `Proper_interval` (`Interval`), and `Iso8601_timezone` (`Iso8601_type`). Each loses a foundation edge, none an RM information-model edge; a fifth would be a defect rather than a new root, since it would mean an RM class had lost its RM parent.

#### The questions the surface answers

For a class in the universe, the surface **MUST** answer:

- **Abstractness** — the pinned BMM's own `is_abstract` flag, so a caller can tell that naming the class denotes its concrete descendants rather than one instantiable class; see [§ Abstract is the BMM's flag, not a storability verdict](#abstract-is-the-bmms-flag-not-a-storability-verdict) below for what it therefore does *not* answer.
- **Immediate parents**, in BMM declaration order — the faithful edge set. A transitive closure erases it, and a re-serialiser or schema differ cannot recover it.
- **Transitive ancestors** — every ancestor class, deterministically ordered.
- **Conformance** — whether one class is, or descends from, another.
- **Concrete descendants** — every class the BMM does not mark abstract that naming this class denotes: itself when not abstract, plus every non-abstract strict descendant, deterministically ordered.
- **Attribute declaration site** — for an attribute the class carries, the class whose BMM declaration supplied it.

#### The flattened attribute tables stay flattened

The per-class attribute tables are inheritance-**flattened** and **MUST** stay so: they answer the inheritance-*resolved* question the REQ-112 validation floor and the REQ-111 compiled template depend on. The declaration site is **additive** — it recovers what the fold erases — rather than a narrowing of any table to own-only declarations.

The declaration site **MUST** agree with the fold that produced those tables. Observably, for an attribute a class carries:

- the class named is that class or one of its BMM ancestors — the **unfiltered** ancestor chain, since the site may lie outside the universe (§ The class universe above);
- the named class's own BMM declaration carries the attribute;
- when the named class is **in** the universe, the attribute's reported type, required flag, and container flag equal the ones that class reports. When it is outside, no such row exists and the BMM declaration itself is the reference.

Two answers to "where does this attribute come from" would be the same defect as the erasure this recovers from.

#### The attribute tables are complete against the BMM

Per class, the shipped attribute-name set **MUST** equal the effective property set — own plus inherited — of the same class in the pinned BMM: an attribute the generator silently declines to translate is a defect, not a gap to be discovered later. A known drop is admissible only as a **named exemption pin**, which cites an open research strand, and the pinned drop is not folded in ahead of that strand's resolution. Exactly one exemption exists — `Iso8601_timezone.value`, the emission gap open as [STRAND-13](research-strands.md#strand-13--properties-inherited-from-a-primitive-mapped-ancestor-are-dropped). The equality, and the pin list held two-sidedly (an unpinned drop fails; a stale pin whose drop has since been fixed fails too), are what [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm) arm (d) asserts.

#### Abstract is the BMM's flag, not a storability verdict

`is_abstract` in the pinned BMM answers "does the model forbid instantiating this class". It is **not** the same question as "can a stored instance carry this name as `_type`", and this surface does not conflate them:

- The surface **MUST** report the BMM's flag verbatim, even where the BMM's answer looks wrong. Six classes the universe admits — `MEASUREMENT_SERVICE`, `TERMINOLOGY_SERVICE`, `CODE_SET_ACCESS`, `TERMINOLOGY_ACCESS`, `OPENEHR_CODE_SET_IDENTIFIERS`, `OPENEHR_TERMINOLOGY_GROUP_IDENTIFIERS` — carry no `is_abstract` flag and are reported non-abstract, two of them despite being `P_BMM_INTERFACE` declarations the generator renders as Go interfaces. The BMM wins over a locally derived answer, whether or not the local answer is better ([REQ-041](#req-041--pinned-bmm-sources)).
- The set of `_type` values the SDK can actually decode is the question of the [REQ-040 type registry](rm-modeling.md#type-registry-req-040), which a caller asks instead. This surface answers the model's shape; the registry answers what is instantiable in Go.

Whether abstractness ought to report BMM-interface-ness as well — and whether the missing flags are an upstream BMM defect to raise — is open as [STRAND-12](research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag), and code leaves it open until that strand resolves.

#### Three answers, never conflated

An introspection answer **MUST** keep these states distinct, because a caller refusing a query has to say which one it hit:

| State | Meaning |
|---|---|
| **not in the universe** | this surface does not model the class — see § The class universe, which is not the same as no Go type existing for it |
| **in the universe, abstract, with concrete descendants** | naming it denotes those descendants |
| **in the universe, abstract, with none** | a dead end: it denotes nothing instantiable |

An empty ancestor answer on a class **in** the universe means *root*; an empty concrete-descendant answer on a class in the universe means *dead-end abstract*; neither is the answer for a name the universe does not contain. A question for the declaration site of an attribute the class does not carry **MUST** report not-found rather than a guessed class.

The *out-of-universe* state itself has structure: why a given name is not in the universe is an answerable question, which [§ Absence reasons](#absence-reasons) answers.

#### Absence reasons

The universe answers "does this surface model the class?" with membership. The negative space behind a "no" has structure the generator already computes: a name can be undeclared by the pinned schemas, declared but in a package the target skips wholesale, declared but in the generator's named-class exclusion set, a `primitive_types` value rather than a modelled class ([§ Primitive type mapping](#primitive-type-mapping)), or a BMM enumeration. The surface keeps these apart, for the reason [§ Three answers, never conflated](#three-answers-never-conflated) keeps the in-universe states apart.

For a name **not** in the class universe, the surface **MUST** report exactly one reason, drawn from this closed set:

| Reason | Meaning |
|---|---|
| **undeclared** | no pinned schema of the RM generation target declares the name |
| **excluded package** | declared; its BMM package is one the generator skips wholesale (`org.openehr.rm.ehr_extract` and the functional/builtins foundation packages) |
| **excluded class** | declared; in the generator's named-class exclusion set |
| **primitive** | declared as (or mapped to) a [§ Primitive type mapping](#primitive-type-mapping) entry — a value the RM *uses*, not a class this surface models, even where the target emits a Go type for it |
| **enumeration** | declared as a BMM enumeration, not a class |

For a name the universe **does** contain, the surface **MUST** report a distinguished *none* answer — the reason type's zero value — that equals no absence reason, so the accessor cannot be misread as a membership test with inverted sense.

A name that more than one rule matches **MUST** report the reason ranked first in this fixed precedence, which puts what a name *is* ahead of why it was *skipped*:

| Rank | Reason |
|---|---|
| 1 | primitive |
| 2 | enumeration |
| 3 | excluded class |
| 4 | excluded package |

The generator's named-class exclusion list deliberately restates several primitive-mapped names as belt-and-braces, and the precedence keeps the reported reason independent of that redundancy.

An out-of-universe name that no absence entry backs, the empty string included, **MUST** report *undeclared*: *undeclared* is computed from the table's silence, never stored. The generated table is produced by the run that generates the universe and never names a universe member. It accounts for every name the RM generation target's pinned schemas declare (`class_definitions` and `primitive_types`) that the universe omits, with a reason that is neither *undeclared* nor *none*; a declared name no rule accounts for fails generation rather than being emitted, which keeps a future widening of a package skip from silently reclassifying a real openEHR class as undeclared.

The reason set is **closed and exhaustive-switchable**. Adding a member is an additive change to the public surface: which bump it warrants is [idiom.md § Public-API stability](idiom.md#public-api-stability)'s and REQ-004's question, not this taxonomy's, and what a caller does with a member it does not recognise is the caller's (§ Out of scope below). The reason's string form **MUST** name the kind only.

The question sits on its own optional capability interface, discovered by runtime type assertion, rather than on `Lookup` or `Hierarchy`, so every existing interface, constructor signature and answer stays as it was ([idiom.md § Public-API stability](idiom.md#public-api-stability)); `Default` implements it. It is reachable with synthetic data through a constructor that admits an absence table beside the class data, and a `Lookup` built without one reports *undeclared* for every out-of-universe name, as that constructor's documentation says. A caller-supplied absence table **MUST NOT** contradict the universe: where it and the class data name the same class, the universe wins, and an entry whose value is the *none* reason counts as no entry, so the name reports *undeclared*.

A name reporting an absence reason **MUST** report not-known on every `Lookup` and `Hierarchy` question, and a name reporting *none* is in `KnownRMTypes()`: the two surfaces answer the same membership question and never disagree.

#### Determinism and independence

- Answers **MUST** be deterministic across runs and processes, every returned ordering fixed (BMM declaration order for immediate parents, sorted otherwise), and a returned collection **MUST NOT** alias package state a caller could mutate.
- The surface uses no reflection beyond the exceptions of [idiom.md § Generics and reflection](idiom.md#generics-and-reflection), and keeps to the building-block rules of [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence); that is what makes it importable from any building block.
- Its class-graph questions sit on an **optional capability interface** discovered by assertion, rather than on the existing attribute-lookup interface, so an external implementer of that interface does not break ([idiom.md § Public-API stability](idiom.md#public-api-stability)), following the precedent already set for attribute enumeration. Each question is reachable through the same seam that admits synthetic model data, so the negative space above is testable without the pinned RM.

**Acceptance:** the compiled-in answers equal an independent reduction of the pinned BMM (class universe, abstractness, immediate parents, transitive ancestors, per-attribute declaring class) — the equivalence [PROBE-094](conformance.md#probe-094--rm-meta-model-introspection-equals-the-pinned-bmm) asserts. `ENTRY` reports abstract and expands to exactly the concrete entry classes; a concrete class expands to itself; a dead-end abstract class expands to nothing while still reporting as known. `LOCATABLE`-inherited attributes report `LOCATABLE` (or the redefining descendant) as their declaration site while the flattened lookup keeps answering unchanged. A name outside the universe is distinguishable from every in-universe answer on every question. The compiled-in absence table equals an independent reduction of the pinned BMM, in both directions, under the fixed precedence, with the negative-space consistency held on every out-of-universe name it exercises — what [PROBE-098](conformance.md#probe-098--absence-reasons-account-for-the-universes-negative-space) asserts. Hand-editing a generated hierarchy field fails the drift check.

**Out of scope:** runtime BMM loading (ruled out above, not deferred); AOM/AM class hierarchies; generic-parameter resolution beyond the root-type reduction the attribute tables already apply; selecting among multiple pinned schemas at run time; deriving *instantiability* beyond the BMM's own abstractness flag (an RM invariant that forbids a value is [REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)'s, not this surface's); shipping any excluded model (the exclusions are pinned intent — PROBE-094 — and the absence reasons make them legible, not reversible); naming the *skipped package* for an excluded-package name (deferred until a consumer needs it); and policy — what a caller does with a reason is the caller's, not this surface's.

### REQ-049 — RM class-universe absence reasons

**Retired.** Its absence reasons are now [REQ-048 § Absence reasons](#absence-reasons). The identifier is never reused.

## Mapping rules

**Implements:** REQ-041

How each P_BMM construct becomes Go code.

### Schema → Go package set

| BMM schema | Go package | v1 |
|---|---|---|
| `openehr_base_1.3.0` | shared between `openehr/rm/` (primitives + foundational classes); no dedicated `openehr/base/` package — base types appear in their using packages | ✓ |
| `openehr_rm_1.2.0` | `openehr/rm/` — one struct file per BMM package, e.g. `openehr/rm/data_types_quantity_gen.go`, plus its JSON and XML codec files when the package has concrete classes. The `org.openehr.rm.ehr_extract` package is **skipped** for v1. | ✓ (less ehr_extract) |
| `openehr_am_1.4.0` | `openehr/aom/aom14/`, one struct file per BMM package, e.g. `openehr/aom/aom14/archetype_gen.go`, plus its JSON and XML codec files when the package has concrete classes. The package is a sibling of `openehr/rm/`, **not** a sub-package of `openehr/template/`. Rationale: AOM is the in-memory model of an *archetype*; templates consume archetypes but AOM is the more fundamental sibling of RM. | ✓ |
| `openehr_am_2.4.0` | `openehr/aom/aom2/` (when wired in) | deferred |
| `openehr_lang_1.1.0` | would target `openehr/bmm/` (BMM meta-classes), but v1 hand-writes that loader against the P_BMM persistence shape — generation deferred | deferred |
| `openehr_term_3.1.0` | would target `openehr/rm/terminology/` (terminology service interface) | deferred |

**Why AOM is a sibling of RM, not a sub-package of template.** Archetypes and templates are distinct openEHR knowledge artefacts: archetypes define reusable clinical content (parsed from ADL into AOM); templates compose and constrain archetypes for a use case (parsed from OET into OPT). AOM is the in-memory form of an *archetype*; the operational template (OPT) parser in `openehr/template/` will *consume* AOM types but does not own them. Placing AOM at `openehr/aom/` keeps the parallel with `openehr/rm/`: both are top-level openEHR information models that the rest of the SDK depends on.

### Class → Go type

| BMM class kind | Go output |
|---|---|
| Concrete class (`is_abstract` absent or `false`) | `type X struct { ... }` with embedded ancestor structs |
| Abstract class (`is_abstract: true`) | `type X interface { isX() }` plus a marker method on every concrete descendant. Per [`rm-modeling.md`](rm-modeling.md). |
| `P_BMM_INTERFACE` | `type X interface { isX() }`, a marker only ([ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4). The generator emits no methods for its `functions`: today that leaves the four of `CODE_SET_ACCESS` and the six of `TERMINOLOGY_ACCESS` out, and no class in the pinned BMM descends from an interface ([STRAND-12](research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag)) |
| `P_BMM_ENUMERATION_STRING` | `type X string` + constants for each item |
| `P_BMM_ENUMERATION_INTEGER` | `type X int` + constants for each item |
| Generic class (`generic_parameter_defs` present) | Generic Go type `type X[T constraint] struct { ... }` where the constraint corresponds to the `conforms_to_type` upper bound |

Inheritance via `ancestors` is materialised by **embedding the ancestor struct(s)** in concrete classes, per [rm-modeling.md § The shape of the RM in Go](rm-modeling.md#the-shape-of-the-rm-in-go). Multiple inheritance flattens to multiple embedded structs (Go allows it; the BMM resolves conflicts at the model level).

LOCATABLE concrete descendants additionally receive the **generated identity surface** ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)): four value-receiver `Get<Field>` accessors (listed on the sealed `Locatable` interface) and four pointer-receiver `Set<Field>` mutators (behind the sealed `MutableLocatable` interface), with types resolved by the same property-mapping rules as the struct fields. The RM target's `typereg_gen.go` also emits the reverse registry — `RMTypeName` (Go concrete → bare registration name, generic instantiations enumerated over the parameter bound's owned concrete descendants) and the `IsTypedNil` guard.

### Property → Go field

| P_BMM property kind | Go field type |
|---|---|
| `P_BMM_SINGLE_PROPERTY` + `is_mandatory: true` | `T` (the mapped type) |
| `P_BMM_SINGLE_PROPERTY` + `is_mandatory: false` or absent | `*T` (pointer to express optionality) |
| `P_BMM_SINGLE_PROPERTY_OPEN` | `T` where `T` is the open generic parameter |
| `P_BMM_CONTAINER_PROPERTY` | `[]T` for `List` / `Array`; `[]T` (with a documented uniqueness invariant) for `Set`; `map[string]V` for `Hash` — see § Container mapping |
| `P_BMM_GENERIC_PROPERTY` | The generic instantiation, e.g. `*DVInterval[DVQuantity]` for `DV_INTERVAL<DV_QUANTITY>` |

Three cases depart from the single-property rows. A non-mandatory property whose type the generator emits as a Go interface (an abstract class, or a concrete class with subtypes, emitted as a narrow `…Like` interface) stays `T`, because an interface field is already nilable. A mandatory property that closes a cycle of mandatory struct-typed properties, such as AOM 1.4's `ARCHETYPE.ontology` and `ARCHETYPE_ONTOLOGY.parent_archetype`, becomes `*T`, because Go cannot declare a struct that contains itself by value. A non-mandatory property whose BMM type is `Any` **MUST** be emitted as `*any`, because `Any` is a primitive and not a class the interface exception covers. The pinned schemas emit five such fields, each an `assumed_value` on `C_COMPLEX_OBJECT`, `C_PRIMITIVE_OBJECT`, `C_CODED_TEXT`, `C_ORDINAL` or `C_QUANTITY`. A mandatory `Any` property **MUST** be emitted as `any`.

JSON tags **MUST** preserve the original BMM property name (snake_case), with `omitzero` on a non-mandatory single or generic property held as a pointer (`*T`, `*DVInterval[DVQuantity]`, `*[]T`, `*map[string]V`), so that only a nil pointer is omitted, and `omitempty` on a non-mandatory one that is not a pointer (a Go interface, or a `P_BMM_SINGLE_PROPERTY_OPEN` typed by the open generic parameter). A mandatory property whose Go field can hold no value carries an omit option, so that an absent one is left out of canonical JSON ([REQ-052](wire.md#req-052) § An absent mandatory member): `omitzero` on a list, map or `any` field and on one held as a pointer, a pointer that breaks a cycle included, so that only a nil value is left out and a present empty list or map is still written; and `omitempty` on a Go interface or on a field typed by its class's own generic parameter (a `P_BMM_SINGLE_PROPERTY_OPEN`, or a `P_BMM_SINGLE_PROPERTY` naming that parameter), so that a typed-nil pointer, which encodes as `null`, is left out too. Any other mandatory property, a struct held by value or a primitive, carries no omit option. A `P_BMM_CONTAINER_PROPERTY` field (`[]T`) carries `omitempty` when its cardinality lower bound is 0 or absent, and `omitzero` otherwise. A `P_BMM_GENERIC_PROPERTY` over `Hash` or `List` follows the single-property rule: a mandatory one is `map[string]V` or `[]T` with `omitzero`, and an optional one is `*map[string]V` or `*[]T` with `omitzero`. Why pointer and container fields differ under `encoding/json/v2` is recorded in [ADR 0002 § D8](../adr/0002-bmm-codegen-decisions.md#d8--the-generator-emits-no-bespoke-json-codec-methods) and [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md). The example below is **conceptual** — it shows the BMM property → Go field mapping and the `DV_AMOUNT` ancestor relationship:

```go
// Conceptual mapping only — not the generated struct shape.
type DVQuantity struct {
    DVAmount                            // conceptual ancestor; flattened by generator (ADR 0002 D4)
    Magnitude            rm.Real        `json:"magnitude"`   // BMM Real → rm.Real per REQ-046 / ADR 0004
    Units                string         `json:"units"`
    Precision            *rm.Integer    `json:"precision,omitzero"`
    NormalRange          *DVInterval[DVQuantity] `json:"normal_range,omitzero"`
    OtherReferenceRanges []ReferenceRange[DVQuantity] `json:"other_reference_ranges,omitempty"`
    UnitsSystem          *string        `json:"units_system,omitzero"`
    UnitsDisplayName     *string        `json:"units_display_name,omitzero"`
}
```

**Generator output diverges from this example for abstract ancestors.** Per [`../docs/adr/0002-bmm-codegen-decisions.md`](../adr/0002-bmm-codegen-decisions.md) D4, the generator renders non-generic abstract classes as Go marker interfaces and **flattens their properties** into every concrete descendant struct. So the real `openehr/rm/data_types_quantity_gen.go` carries `DVQuantity` with `DVAmount`'s fields (`Accuracy`, `AccuracyIsPercent`, `MagnitudeStatus`, …) inlined and **no `DVAmount` embed**. Only concrete ancestors and whitelisted abstract-generic ancestors (e.g. `EVENT` per ADR 0003) appear as Go-level embeds.

### Type → Go type expression

| P_BMM type kind | Go expression |
|---|---|
| `P_BMM_SIMPLE_TYPE` with `type: "X"` | `X` (or its Go alias from the primitive map; see § Primitive type mapping) |
| `P_BMM_GENERIC_TYPE` with `root_type: "Foo"`, `generic_parameters: ["Bar"]` | `Foo[Bar]` |
| `P_BMM_CONTAINER_TYPE` with `container_type: "List"`, nested type | `[]T` (see § Container mapping) |

### Container mapping

| BMM container_type | Go shape | Notes |
|---|---|---|
| `List` | `[]T` | Order matters; duplicates allowed |
| `Array` | `[]T` | Same as List in Go; the distinction is historical |
| `Set` | `[]T` | With a generated `Set` invariant check in `Validate()` if validation hooks exist. Go has no native set; modelling as `map[T]struct{}` complicates JSON. |
| `Hash` | `map[string]V` | Keys are `String` |

The wire format JSON does not distinguish List/Set/Array — they all serialise as arrays. Modelling them all as `[]T` keeps round-tripping exact; the Set / Array semantics surface in validators and AQL path-resolution rather than in the type itself.

### Cardinality

`P_BMM_CONTAINER_PROPERTY.cardinality` (with `lower`, `upper`, `upper_unbounded`) reaches the SDK through the generated attribute metadata in `openehr/rm/rminfo`: `AttrMeta.Container` marks a container property, and `AttrMeta.Required` is true when `cardinality.lower` is 1 or more, which the validation floor checks ([REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)). The upper bound is not kept: no container in the pinned RM and BASE BMMs has a finite one.

### Functions

`P_BMM_*` function definitions on classes (e.g. `DV_QUANTITY.add`, `DV_QUANTITY.less_than`) carry pre/post-conditions and operator aliases. The SDK's posture:

- The generator emits **method stubs** for class functions, with the documented `pre_conditions` / `post_conditions` as `// Pre:` / `// Post:` comments. A `P_BMM_INTERFACE`'s functions are the exception: no stub is emitted for them (Class → Go type above, [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4).
- The generator does **not** emit the function body — implementations are hand-written in non-generated files ([REQ-041](#req-041--pinned-bmm-sources)) where they are needed.
- Functions that have not been hand-implemented **SHOULD** panic with a clear message (e.g. `panic("not implemented: DV_QUANTITY.add — implement in a non-generated file")`); this is one of the rare allowed panics in SDK code, justified by the fact that the function is documented in the BMM but not yet wired in.

For SDK v1, the priority is the **types**, not the **methods** — most consumers do not need RM-level arithmetic. The RM functions the SDK implements, and those it leaves out, are specified in [rm-functions.md](rm-functions.md).

## Primitive type mapping

**Implements:** REQ-046

The 29 primitives in `openehr_base_1.3.0.bmm.json` map to Go types per the table below. Mappings are **fixed**; do not introduce alternative widenings.

| BMM primitive | Go type | Notes |
|---|---|---|
| `Boolean` | `bool` | |
| `Integer` | `rm.Integer` (underlying `int32`) | Pinned to 32-bit per the BMM spec. Emitted as the defined type `rm.Integer` so the codec can attach the wire-tolerance rule from [`../docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md). |
| `Integer64` | `rm.Integer64` (underlying `int64`) | Emitted as the defined type `rm.Integer64` so the codec can attach the same wire-tolerance rule as `Integer` ([`../docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md)). `DV_COUNT.magnitude` is the RM's one `Integer64` attribute. |
| `Real` | `rm.Real` (underlying `float64`) | The BMM `Real` is single-precision conceptually but openEHR JSON uses unbounded-precision numbers; `float64` is the safe choice. Emitted as the defined type `rm.Real` so the codec can attach the wire-tolerance rule from [`../docs/adr/0004-numeric-wire-tolerance.md`](../adr/0004-numeric-wire-tolerance.md). |
| `Double` | `float64` | Same as `Real`, no codec tolerance attached. |
| `Character` | `rm.Character` | canonical JSON single-character **string**, not a number; hand-written named type (`openehr/rm/character.go`), like `Integer`/`Real` |
| `String` | `string` | |
| `Octet` | `byte` | |
| `Uri` | `string` | Validated at codec boundary; not `net/url.URL` to preserve round-trip exact representation |
| `Any` | `any` | Optional properties differ: § [Property → Go field](#property--go-field). |
| `Iso8601_date` | `string` (validated) | See § ISO 8601 types below |
| `Iso8601_time` | `string` (validated) | |
| `Iso8601_date_time` | `string` (validated) | |
| `Iso8601_duration` | `string` (validated) | |
| `Iso8601_type` | `string` (validated) | Abstract — supertype of the four above |
| `Temporal` | `string` (validated) | Abstract |
| `Numeric` | (abstract; not directly used) | |
| `Ordered` | (abstract; not directly used) | |
| `Ordered_Numeric` | (abstract; not directly used) | |
| `Cardinality` | `bmm.Cardinality` (struct) | Hand-written in `openehr/bmm/` — lower, upper, upper_unbounded |
| `Multiplicity_interval` | `rm.MultiplicityInterval` (struct) | Generated from base BMM |
| `Interval` | generic `Interval[T]` | Generated; ancestors set on dependent types |
| `List` | `[]T` | See § Container mapping |
| `Set` | `[]T` | See § Container mapping |
| `Array` | `[]T` | See § Container mapping |
| `Hash` | `map[string]V` | See § Container mapping |
| `Container` | abstract; not directly used | |
| `Terminology_code` | `rm.TerminologyCode` (struct) | Generated from base BMM |
| `Terminology_term` | `rm.TerminologyTerm` (struct) | Generated from base BMM |

### ISO 8601 types

The four ISO 8601 BMM types map to Go `string` rather than `time.Time` because:

- The openEHR formats include partial dates (`yyyy`, `yyyy-mm`) and partial times (`HH`, `HH:MM`) that `time.Time` cannot round-trip without information loss.
- Wire-format byte-fidelity matters for canonical JSON conformance (REQ-052).

Helpers in `openehr/rm/` (hand-written, non-generated) **SHOULD** offer typed parsing into `time.Time` and back for consumers who need it, but the storage type is the wire-equivalent `string`.

## Generator output conventions

- File naming: `<package_name>_gen.go` for generated files. Example: `openehr/rm/data_types_quantity_gen.go` corresponds to BMM package `org.openehr.rm.data_types.quantity`.
- Header: every generated file begins with `// Code generated by bmmgen; DO NOT EDIT.` followed by a `// Source: openehr_rm_1.2.0.bmm.json` line for traceability.
- RM release: the public constant `rm.Release` **MUST** carry the primary RM schema's `rm_release`. What the SDK writes into `ARCHETYPED.rm_version` is [wire.md § REQ-053](wire.md#req-053) for FLAT and STRUCTURED decode and [clinical-modeling.md § REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) for the instance generator.
- Hand-written companions: `<package_name>_ext.go` (e.g. `data_types_quantity_ext.go`) hold method bodies, helper functions, and codec hooks.
- One Go package per BMM top-level package; sub-packages flattened into the same Go package via filename (Go does not require a one-to-one directory mapping with BMM packages).
- The generator emits `openehr/rm/typereg_gen.go` (and `openehr/aom/aom14/typereg_gen.go`) which register every concrete RM/AOM `_type` with its constructor (REQ-040). Per [`../docs/adr/0002-bmm-codegen-decisions.md`](../adr/0002-bmm-codegen-decisions.md) D3, the registration file lives in the target package — **not** under `openehr/rm/typereg/` — so the constructor closures can reference the target types directly without forming a `typereg → rm` import cycle.

## Documentation propagation

The generator carries the BMM `documentation` field on classes, properties, and functions into the Go doc comments of the generated types, fields and methods. The transformation is verbatim — no rewording, no abbreviation. Markdown-style cross-refs (`_units_`, `\`magnitude\``) are converted to Go-doc-friendly equivalents.

## v1 scope summary

**Generated in v1:**
- `openehr/rm/` from `openehr_rm_1.2.0` (excluding `ehr_extract`) + `openehr_base_1.3.0`.
- `openehr/rm/typereg/` from the same.
- `openehr/rm/rminfo/` from the same — the flattened attribute tables, the REQ-048 class graph and absence table, and the REQ-112 function table that `rminfo.IsNonStorableAttr` reads.
- `internal/rmaccess/access_gen.go` from the same — the attribute access of every registered class (REQ-171).
- `openehr/validation/inventory_gen.go` from the same and the hand-kept `invariants.json` — the invariant inventory (REQ-171).
- `openehr/aom/aom14/` from `openehr_am_1.4.0` + `openehr_base_1.3.0`.

**Deferred (BMM file kept, no v1 generation):**
- AOM 2 (`openehr_am_2.4.0`) — consumers requesting ADL 2 / AOM 2 are out of v1 scope.
- LANG (`openehr_lang_1.1.0`) — `openehr/bmm/` is hand-written for v1; the LANG BMM remains as a reference / cross-check.
- TERM (`openehr_term_3.1.0`) — terminology service interface not required by v1 consumers.
- RM `ehr_extract` package — EHR Extract is a separate exchange surface not exercised by v1 use cases.

Deferred items become future plan phases when consumer demand arises; the BMM-conformance contract above continues to apply.
