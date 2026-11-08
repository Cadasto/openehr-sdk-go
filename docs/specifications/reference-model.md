---
kind: specification
---

# Reference Model in Go

**Status:** Draft

Rules for representing the openEHR Reference Model (RM) and the other BMM-derived models in Go: the shape of the generated types, the openEHR terminology vocabulary (REQ-034) and the type registry (REQ-040); the pinned BMM sources, the loader and the generator's mapping (REQ-041, REQ-045, REQ-048); and the RM's behavioural functions (REQ-120, REQ-121, REQ-123). The retired REQ-030 to REQ-033 are [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, summarised in [§ The shape of the RM in Go](#the-shape-of-the-rm-in-go); the retired REQ-042, The retired REQ-043, REQ-044, REQ-046 and REQ-047 are part of REQ-041, the retired REQ-049 is part of REQ-048, and the retired REQ-122 is part of REQ-120.

The RM types in `openehr/rm/` are **generated** from the SDK-authored RM root over the pinned BMM schemas in [`../resources/bmm/`](../../resources/bmm); the rules below state the *shape* the generator emits. Generator semantics (mapping rules per P_BMM construct, primitive types, file conventions) are [§ BMM sources and the generator](#bmm-sources-and-the-generator).

## The shape of the RM in Go

The RM has deep polymorphism: `LOCATABLE → ENTRY → COMPOSITION`; `DATA_VALUE → DV_QUANTITY → DV_DURATION`. Go's type system does not have inheritance, and emulating it leads to runtime-tag confusion, reflection-heavy decoders and runtime errors that belong at compile time. The generator's output rule is [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4; in brief:

- **Concrete structs.** Every concrete RM class is a concrete Go struct (`Composition`, `Observation`, `DVQuantity`, …). There is no runtime base struct that types inherit from, no single RM struct with a `Type` discriminator field, and no `interface{}`-typed payload with a `Kind` enum.
- **Flattened ancestors.** An abstract non-generic ancestor's properties are flattened into every concrete descendant: the generated `Composition` carries `Name`, `ArchetypeNodeID`, `UID` and the other `LOCATABLE` fields as its own. Only concrete ancestors and whitelisted abstract-generic ancestors (such as `EVENT`, [ADR 0003](../adr/0003-rm-event-polymorphism.md)) appear as embedded structs.
- **Marker interfaces for abstract categories.** An abstract RM category (`DATA_VALUE`, `ITEM_STRUCTURE`, `ENTRY`, …) is a Go interface with an unexported marker method (`isDataValue()`), which keeps a type outside the package from satisfying it by accident; a concrete type satisfies it through its method set, with no registration.
- **The type registry for `_type`.** The `_type` discriminator maps onto a concrete Go type only through the type registry ([§ Type registry (REQ-040)](#type-registry-req-040)): no tag-magic dispatch on a peeked `_type`, and no generic "RM node" that holds any kind of RM value indistinguishably.

## openEHR terminology vocabulary (REQ-034)

The openEHR Terminology — the `openehr` terminology's **groups** (*audit change type*, *version lifecycle state*, *setting*, *participation mode*, *composition category*, …) and **code sets** (*normal statuses*, *compression algorithms*, *integrity check algorithms*), and the **external code sets** the Foundation publishes beside them (*languages* ISO_639-1, *countries* ISO_3166-1, *character sets* IANA_character-sets, *media types* IANA_media-types), that the RM's own invariants reference (`EVENT_CONTEXT.Setting_valid`, `AUDIT_DETAILS.Change_type_valid`, `PARTICIPATION.Mode_valid`, `DV_ORDERED.Normal_status_validity`, `ENTRY.Language_valid`, …) — is part of the specifications the SDK targets, published as a computable artefact by the openEHR Foundation. It is small, closed and versioned with the specifications; it is **not** a runtime terminology-service lookup. The external code sets are the snapshots the Foundation publishes with the TERM release, not the live ISO and IANA registers. Beside them the release publishes the **property-unit table**, `PropertyUnitData.xml`: for each code of the *property* group, the units of measure that measure it, with their UCUM codes, which a `C_DV_QUANTITY` constraint's `property` names ([clinical-modeling.md § REQ-103](clinical-modeling.md#req-103--primitive-constraint-introspection)). The SDK carries the terminology the way it carries the BMM ([§ REQ-041](#req-041--pinned-bmm-sources)):

- **Pinned source.** The Foundation's `openehr_terminology.xml`, `openehr_external_terminologies.xml` and `PropertyUnitData.xml` for one named TERM release, all three taken from the same upstream commit, are vendored byte-identical under `resources/terminology/`, with the release, the upstream commit and each file's sha256 recorded beside them. A version bump is an explicit, reviewable commit that moves them together; the pin is never hand-edited, and the build gate fails when a vendored file no longer matches its recorded sha256.
- **Generated accessor.** A building-block package, `openehr/terminology`, **MUST** expose every group and every code set of the pin as typed, closed, source-ordered values — groups with code → rubric, rubric → code and membership lookups, code sets with membership lookups and the issuer and external identifier the pin gives each (`ISO_639-1`, `IANA_character-sets`, `openehr_normal_statuses`, …) — plus the pinned release version and the property-unit table: for each property code, the UCUM codes of the units the table lists for it, taken trimmed, once each and in source order, with the primary units marked, and a membership lookup. A unit the table gives no UCUM code contributes none. Its tables are generated from the pin, and the build gate fails when they drift from it; its import rule is in [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence).
- **Membership.** A code set's membership lookup **MUST** ignore letter case when the pin names an issuer other than openEHR (the ISO and IANA sets), because those issuers do: the IANA character-set registry makes no distinction between upper and lower case, RFC 6838 §4.2 makes media type names case-insensitive, and RFC 5646 §2.1.1 treats the ISO 639 language and ISO 3166-1 region subtags of a language tag as case-insensitive. A membership lookup **MUST** be exact for an openEHR-issued code set, as a group's lookups are, and for a UCUM code, which is case-sensitive. A lookup **MUST** accept only a code the pinned set lists, under the letter-case rule above: a code the live ISO or IANA register holds and the pinned snapshot leaves out, such as one the register added after the release, is not a member until a release bump brings it in, and nor is an IANA alias the set leaves out, such as `ISO-8859-1` beside the listed name `ISO_8859-1:1987`.
- **One home per code.** SDK code that mints, defaults, validates, or reconstructs an `openehr`-terminology coded value from a bare code or a bare rubric takes the code set, the rubric and the membership verdict from the accessor; a hand-typed openEHR code table or rubric outside it is a defect. A surface can keep named constants for the codes it promotes (`LifecycleStateComplete`, `ChangeTypeCreation`, …), but a surface that validates such a code **MUST** judge it by its group membership in the pin, and a `DV_CODED_TEXT` the SDK builds from a code **MUST** carry that code's pinned rubric as its `value`, never a rubric typed beside the code. A codec that instead *transports* a caller-supplied code-and-rubric pair whole — the FLAT `ctx/setting` `|code`+`|value`, say — transcribes it and does **not** re-validate its membership here; enforcing the RM instance invariants (`EVENT_CONTEXT.Setting_valid`, …) on decoded values is [REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)'s, not this requirement's.
- **Negative space.** A lookup of an unknown code or rubric **MUST** report absence (a false second return) rather than a fabricated value or a panic, on a nil group or property too.

*Out of scope:* external terminologies (SNOMED CT, LOINC, …) — [REQ-111](clinical-modeling.md#req-111--public-compiled-template-bridge) covers the bindings an OPT declares, and a terminology *service* stays a consumer concern. Evaluating the RM's coded invariants against the accessor on an RM instance is the template-less RM floor's ([REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor) § *Coded invariants*), not this requirement's.

## Type registry (REQ-040)

A central type registry in `openehr/rm/typereg` maps each `_type` string to the constructor of its concrete Go type, and polymorphic JSON decoding of RM fields goes through it.

```go
// package typereg (sketch)

type Registry struct { ... }

// Default is the package-level registry, populated by the rm package's init().
var Default *Registry

// Register associates a _type string with a constructor returning a zero value
// of the concrete Go type. Panics on duplicate registration.
func (r *Registry) Register(typeName string, constructor func() any)

// Decode reads {"_type":"DV_QUANTITY", ...} and returns a *DVQuantity (typed any).
// Returns an error if _type is missing, unknown, or the body fails to decode into
// the concrete type.
func (r *Registry) Decode(data []byte) (any, error)
```

Rules:

- The registry **MUST** decode the `_type` of every concrete RM type the SDK generates; the `rm` package's `init()` registers each one. A `_type` the registry does not know **MUST** fail with an error that carries `typereg.ErrUnknownType`. A missing `_type` **MUST** fail with an error that carries `typereg.ErrMissingType` where no declared type fixes the value, at the top level (`Registry.Decode`) and at an abstract slot; a narrow-interface slot whose declared parent is concrete reads it as that parent type ([wire.md § Polymorphic substitution](wire.md#polymorphic-substitution)). Neither case decodes as a guessed type.
- The registry is append-only at runtime. Calling `Register` twice for the same `_type` with different constructors **MUST** panic — a name collision is a programmer error, not a recoverable condition. Consumers do not register types unless they extend the RM, which the SDK does not actively support but does not prevent either.
- The constructor closure is the only mechanism that instantiates a type: the registry uses no reflection ([idiom.md § Generics and reflection](idiom.md#generics-and-reflection)).
- A decoded value of static type `any` is type-asserted at the call site. The SDK provides generic helpers (`typereg.DecodeAs[T any](data)`) where useful.

The registry is also **reversible** ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)):

- A generated `rm.RMTypeName(any) (string, bool)` **MUST** map every registered concrete Go type — including generic instantiations over the parameter bound's closed descendant set — back to its bare registration name (`DVInterval[DVQuantity]` → `"DV_INTERVAL"`), and report `("", false)` for a nil interface, a typed-nil pointer or a non-RM value.
- A generated `rm.IsTypedNil(any) bool` **MUST** report whether a value is an interface carrying a typed-nil pointer to a registered concrete type; it is the guard to use before calling `Locatable` accessors.

Both are generated with the forward registrations, so forward and reverse mappings cannot drift, and neither uses reflection.

### Generated LOCATABLE identity surface

Because ADR 0002 D4 flattens the `LOCATABLE` fields into every concrete descendant, the generator also emits a polymorphic identity surface over them ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)):

- Every LOCATABLE concrete type **MUST** satisfy the sealed `rm.Locatable` interface through four value-receiver read accessors — `GetArchetypeNodeID() string`, `GetName() DVTextLike`, `GetUID() UIDBasedID`, `GetArchetypeDetails() *Archetyped` — each returning the flattened field's declared type, so both `T` and `*T` satisfy it.
- For every LOCATABLE concrete type `T`, `*T` **MUST** satisfy the sealed `rm.MutableLocatable` interface, which collects the four matching pointer-receiver setters.
- Accessors return the field verbatim, so a getter called on a typed-nil `*T` panics; a caller guards with `rm.IsTypedNil` before asserting or calling.
- The surface is machine-owned: accessor emission resolves types through the same rules as struct-field rendering, and the generator's drift check covers it ([§ REQ-041](#req-041--pinned-bmm-sources)).

## What is NOT in scope here

- **Template-specific generated structs.** A typed Go struct for a specific OPT (e.g. a vital-signs template) is **not** part of `openehr/rm/` — that belongs in the consuming project (see [scope.md § Out of scope](scope.md#out-of-scope-v1)).
- **The full RM type list.** The dictionary of types lives in `openehr/rm/` once implemented; this spec defines the *rules*, not the inventory.

## Codec interaction

Codecs in `openehr/serialize/` are the consumers of the type registry on the read path, and the inverse on the write path:

- **Read:** the codec peeks `_type`, looks up the constructor in the registry, decodes into the concrete struct.
- **Write:** the codec encodes the concrete struct; `_type` is set by the struct's `MarshalJSONTo` (or a custom encoder), keyed off the struct's known `_type` name.

The underlying JSON library is the standard library's `encoding/json/v2` ([ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md)); the type-registry contract above holds regardless — codecs consult the registry, the registry does not depend on them. STRAND-04's remaining open questions are the full-RM inventory and codec performance, not the codec choice.

## BMM sources and the generator

The SDK's domain types (Reference Model, Archetype Object Model, base + language + terminology types) are **derived from openEHR BMM schemas** pinned in [`../resources/bmm/`](../../resources/bmm). This document is the normative contract between those BMM inputs and the Go code in `openehr/rm/`, `openehr/aom/aom14/`, and related packages.

### The BMM substrate

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

### REQ-041 — Pinned BMM sources

The BMM files in `resources/bmm/`, the pinned schemas and the SDK-authored roots over them, are the source of truth for the openEHR domain model. The generated RM and AOM 1.4 types, their primitive mappings and their type registry entries **MUST** correspond to the classes and properties of the pinned BMM files as the SDK-authored root schemas include and amend them ([§ SDK-authored root schemas](#sdk-authored-root-schemas)), mapped by [§ Mapping rules](#mapping-rules) and [§ Primitive type mapping](#primitive-type-mapping); hand-written Go code embeds no out-of-band information about openEHR types: what the pinned schemas lack or get wrong is declared in a root schema, never in a Go table.

How the SDK keeps that correspondence:

- **Generated, reproducibly.** An in-tree generator (`internal/bmmgen`, run by `cmd/bmmgen`) generates `openehr/rm/` (the type registrations included), `openehr/rm/rminfo/` and `openehr/aom/aom14/` from the **primary** BMM files in the table above. It is reproducible: two runs over the same inputs give byte-identical output.
- **Drift-checked.** The build gate re-runs the generator and fails when the working tree differs from its output, which is the mechanical guarantee against a hand edit silently diverging from the BMM.
- **Scope.** The generator skips the BMM package `org.openehr.rm.ehr_extract` and its sub-packages, and the deferred BMM files (`am 2.4.0`, `lang 1.1.0`, `term 3.1.0`) are not its inputs in v1.
- **Generated and hand-written files stay apart.** A generated file carries the standard generated-code header ([§ Generator output conventions](#generator-output-conventions)) and is never edited by hand. Hand-written code in a BMM-derived package lives in its own files, documented in a `doc.go` or named by the `<file>_ext.go` convention; it consumes the generated types, never redefines a generated type or method, and never weakens or overrides an invariant the BMM declares. A hand-written package can hold a generated `*_gen.go` file that the drift check covers like any other output, such as the generated attribute access in `internal/rmaccess` and the invariant inventory in `openehr/validation`, which a hand-kept `invariants.json` classifies.
- **Primitive types.** Each openEHR primitive type **MUST** map to the Go type the table in [§ Primitive type mapping](#primitive-type-mapping) gives it. The mapping is fixed; an alternative widening, such as `Real` to `float32`, is not permitted. `Character` is the one primitive whose canonical JSON spelling (a one-character string) disagrees with a numeric Go `rune`, so it maps to a named type carrying its own codec.
- **The BMM wins.** Where a BMM file declares a class, property or cardinality that contradicts a statement elsewhere in the openEHR specifications, the SDK follows the BMM. The BMM is the computable form the openEHR ecosystem uses for validation, AQL path typing and tooling, so diverging from it would make archetype validation disagree with templates compiled from it, and it is the form the openEHR Foundation actively maintains while spec prose lags. A suspected bug in a BMM file is raised upstream, in the openEHR specifications repository, and until the corrected schema is pinned the target's root schema carries the correction ([§ SDK-authored root schemas](#sdk-authored-root-schemas)); it is never worked around in Go. Where an ITS-XML schema and the BMM disagree on the wire form of an ADL 1.4 artefact, the schema is what the documents follow, and the root schema declares the schema's form. A deviation from the mapping rules takes an ADR and an update to [§ Mapping rules](#mapping-rules).

The openEHR Terminology vocabulary is pinned the same way under `resources/terminology/` ([§ REQ-034](#openehr-terminology-vocabulary-req-034)).

#### SDK-authored root schemas

<a id="sdk-authored-root-schemas"></a>

The schema a generation target loads is not a pinned file but a root schema the SDK authors, under `resources/bmm/sdk/`, which includes the pinned schema and declares only what the pinned ones lack or get wrong ([ADR 0034](../adr/0034-sdk-authored-root-schemas.md)). The loader's include merge ([§ REQ-045](#req-045--bmm-loader-is-a-building-block)) lets a root's class shadow an include's class of the same name ([ADR 0002 D2](../adr/0002-bmm-codegen-decisions.md#d2--descendant-shadows-ancestor-bmm-include-merge)), so a root amends a pinned class by declaring it again, whole. A pinned file is never edited.

- **What a root declares.** A class no pinned schema defines, or a pinned class in a corrected form. Each class and property a root declares carries, in its `documentation`, the reason and the upstream reference it rests on: the openEHR schema it follows, or the specification ticket or pull request that carries the same change upstream. What P_BMM has no construct for, a root says in SDK extension keys of its JSON, one key per kind, which the generator reads and the loader of § REQ-045 leaves aside: the classes a target leaves out, a type name a decoder reads as another class, an element read and dropped, the names given to a schema's anonymous types, and a schema form P_BMM cannot express, such as a choice between two element shapes. Each entry of an extension key carries its reason and upstream reference, as a declared class does. A root's id is its own: no pinned schema has it. A correction leaves the root in the commit that pins the upstream fix, so the root never outlives the gap it closes.
- **The AOM 1.4 root**, `openehr_operational_template_1.4.0`, includes `openehr_am_1.4.0` and the RM root and declares the ADL 1.4 operational template ([clinical-modeling.md § REQ-176](clinical-modeling.md#req-176--adl-14-operational-template-model)): `OPERATIONAL_TEMPLATE`, `C_ARCHETYPE_ROOT`, `FLAT_ARCHETYPE_ONTOLOGY`, `ANNOTATION`, `T_VIEW`, `T_CONSTRAINT`, `T_ATTRIBUTE`, `T_COMPLEX_OBJECT` and `C_CODE_REFERENCE`, from the AM Release-1.4 `Template.xsd`, the only formal definition openEHR has published for them; and each AOM 1.4 class whose AM Release-1.4 schema type differs from the pinned schema, in the schema type's form. `AUTHORED_RESOURCE` takes the form of the RM `common.resource` package, which the AOM 1.4 specification names as its dependency and the AM Release-1.4 schemas and every OPT carry, in place of the BASE form the pinned AOM 1.4 schema inherits: the merge's rule for an include diamond gives the root the RM form of each class BASE and RM both declare. Where the schema names a class otherwise than the BMM (`C_CODE_PHRASE`, `C_DV_ORDINAL` and `C_DV_QUANTITY` for `C_CODED_TEXT`, `C_ORDINAL` and `C_QUANTITY`), the root declares the schema's name and the target leaves the BMM's class out, so each generated type, its `_type` and its `xsi:type` carry one name.
- **The RM root**, under an id of its own, includes `openehr_rm_1.2.0`, keeps its `rm_release`, and corrects `TRANSLATION_DETAILS.accreditation`, the spelling the openEHR ITS-XML schemas and ITS-JSON use and the RM schema misspells, until the RM schema is re-pinned with the upstream fix.
- **Observable.** A generated type **MUST** carry the names its root declares, in Go, canonical JSON and canonical XML alike, `rm.TranslationDetails.Accreditation` and `aom14.CCodePhrase` among them. A document that writes a property in the pinned schema's spelling instead is unmatched content ([wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)); one that names a class by the pinned schema's name in `_type` or `xsi:type` names an unknown type and fails in every mode, as REQ-170 says.
- **Gate.** The build gate runs the root check, a Go test in `internal/bmmgen`, on every root with its includes: every type name resolves after the merge; every class, property and extension entry a root declares carries its reason and upstream reference; each shadow still differs from the class it shadows, so a re-pin that lands the upstream fix fails the check until the shadow goes; no root's id is a pinned schema's; and a root's top-level members are P_BMM schema members or the SDK extension keys, so a misspelt key cannot be passed over. The drift check holds the generated output to the roots as it holds it to a pinned file. A generated file's header names the root and the pinned schemas it includes.

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

`LoadAll` merges each include into the schema that includes it. A class the including schema declares shadows an included class of the same name ([ADR 0002 D2](../adr/0002-bmm-codegen-decisions.md#d2--descendant-shadows-ancestor-bmm-include-merge)). When one class reaches a schema through its includes in differing forms, the form whose schema descends, through its includes, from the schema of every other form wins, whatever the order of the includes; where several schemas bring one identical form, a differing form wins only when its schema descends from each of them: a root that includes AOM 1.4, which includes BASE, and RM, which includes BASE too, gets the RM form of each class RM and BASE both declare. Two differing forms neither of whose schemas descends from the other's give way to the form of a third schema that descends from both and declares the class again. When no form's schema descends from the schema of every other form, the load fails with `ErrSchemaConflict`.

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
- The set of `_type` values the SDK can actually decode is the question of the [REQ-040 type registry](#type-registry-req-040), which a caller asks instead. This surface answers the model's shape; the registry answers what is instantiable in Go.

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

### Mapping rules

**Implements:** REQ-041

How each P_BMM construct becomes Go code.

#### Schema → Go package set

| BMM schema | Go package | v1 |
|---|---|---|
| `openehr_base_1.3.0` | shared between `openehr/rm/` (primitives + foundational classes); no dedicated `openehr/base/` package — base types appear in their using packages. Where BASE and RM declare a class of one name (`AUTHORED_RESOURCE`, `RESOURCE_DESCRIPTION`, `RESOURCE_DESCRIPTION_ITEM`, `TRANSLATION_DETAILS`, `CODE_PHRASE`), `openehr/rm/` holds the RM form, which the root's precedence over its include gives it and which ADL 1.4 archetypes and OPTs carry; the BASE form, which AOM 2 uses, is emitted into AOM 2's package when that target is wired in | ✓ |
| `openehr_rm_1.2.0`, through the SDK-authored RM root ([§ SDK-authored root schemas](#sdk-authored-root-schemas)) | `openehr/rm/` — one struct file per BMM package, e.g. `openehr/rm/data_types_quantity_gen.go`, plus its JSON and XML codec files when the package has concrete classes. The `org.openehr.rm.ehr_extract` package is **skipped** for v1. | ✓ (less ehr_extract) |
| `openehr_am_1.4.0`, through the SDK-authored root `openehr_operational_template_1.4.0` ([§ SDK-authored root schemas](#sdk-authored-root-schemas)) | `openehr/aom/aom14/`, the AOM 1.4 classes and the ADL 1.4 operational template classes, one struct file per BMM package, e.g. `openehr/aom/aom14/archetype_gen.go`, plus its JSON and XML codec files when the package has concrete classes. The package is a sibling of `openehr/rm/`, **not** a sub-package of `openehr/template/`. Rationale: AOM is the in-memory model of an *archetype*; templates consume archetypes but AOM is the more fundamental sibling of RM. | ✓ |
| `openehr_am_2.4.0` | `openehr/aom/aom2/` (when wired in) | deferred |
| `openehr_lang_1.1.0` | would target `openehr/bmm/` (BMM meta-classes), but v1 hand-writes that loader against the P_BMM persistence shape — generation deferred | deferred |
| `openehr_term_3.1.0` | would target `openehr/rm/terminology/` (terminology service interface) | deferred |

**Why AOM is a sibling of RM, not a sub-package of template.** Archetypes and templates are distinct openEHR knowledge artefacts: archetypes define reusable clinical content (parsed from ADL into AOM); templates compose and constrain archetypes for a use case (parsed from OET into OPT). AOM is the in-memory form of an *archetype*; the compile step in `openehr/templatecompile` *consumes* AOM types but does not own them. Placing AOM at `openehr/aom/` keeps the parallel with `openehr/rm/`: both are top-level openEHR information models that the rest of the SDK depends on.

#### Class → Go type

| BMM class kind | Go output |
|---|---|
| Concrete class (`is_abstract` absent or `false`) | `type X struct { ... }` carrying its abstract ancestors' properties as its own, and embedding a concrete or whitelisted abstract-generic ancestor ([§ The shape of the RM in Go](#the-shape-of-the-rm-in-go)) |
| Abstract class (`is_abstract: true`) | `type X interface { isX() }` plus a marker method on every concrete descendant. Per [§ The shape of the RM in Go](#the-shape-of-the-rm-in-go). |
| `P_BMM_INTERFACE` | `type X interface { isX() }`, a marker only ([ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4). The generator emits no methods for its `functions`: today that leaves the four of `CODE_SET_ACCESS` and the six of `TERMINOLOGY_ACCESS` out, and no class in the pinned BMM descends from an interface ([STRAND-12](research-strands.md#strand-12--bmm-interface-classes-carry-no-is_abstract-flag)) |
| `P_BMM_ENUMERATION_STRING` | `type X string` + constants for each item |
| `P_BMM_ENUMERATION_INTEGER` | `type X int` + constants for each item |
| Generic class (`generic_parameter_defs` present) | Generic Go type `type X[T constraint] struct { ... }` where the constraint corresponds to the `conforms_to_type` upper bound |

Inheritance via `ancestors` follows [§ The shape of the RM in Go](#the-shape-of-the-rm-in-go): an abstract non-generic ancestor's properties are flattened into each concrete descendant, and only a concrete or whitelisted abstract-generic ancestor is embedded.

LOCATABLE concrete descendants additionally receive the generated identity surface and the reverse registry of [§ Type registry (REQ-040)](#type-registry-req-040).

#### Property → Go field

| P_BMM property kind | Go field type |
|---|---|
| `P_BMM_SINGLE_PROPERTY` + `is_mandatory: true` | `T` (the mapped type) |
| `P_BMM_SINGLE_PROPERTY` + `is_mandatory: false` or absent | `*T` (pointer to express optionality) |
| `P_BMM_SINGLE_PROPERTY_OPEN` | `T` where `T` is the open generic parameter |
| `P_BMM_CONTAINER_PROPERTY` | `[]T` for `List` / `Array`; `[]T` (with a documented uniqueness invariant) for `Set`; `map[string]V` for `Hash` — see § Container mapping |
| `P_BMM_GENERIC_PROPERTY` | The generic instantiation, e.g. `*DVInterval[DVQuantity]` for `DV_INTERVAL<DV_QUANTITY>` |

Three cases depart from the single-property rows. A non-mandatory property whose type the generator emits as a Go interface (an abstract class, or a concrete class with subtypes, emitted as a narrow `…Like` interface) stays `T`, because an interface field is already nilable. A mandatory property that closes a cycle of mandatory struct-typed properties, such as `ARCHETYPE.ontology` and `ARCHETYPE_ONTOLOGY.parent_archetype` in the pinned `openehr_am_1.4.0`, becomes `*T`, because Go cannot declare a struct that contains itself by value. A non-mandatory property whose BMM type is `Any` **MUST** be emitted as `*any`, because `Any` is a primitive and not a class the interface exception covers. The pinned `openehr_am_1.4.0` emits five, each an `assumed_value` it declares on `C_DEFINED_OBJECT` or `C_PRIMITIVE`; the AOM 1.4 root drops it from `C_COMPLEX_OBJECT` and `C_PRIMITIVE_OBJECT` and gives it, on the other classes that have one, the type the AM Release-1.4 schema gives it ([clinical-modeling.md § REQ-176](clinical-modeling.md#req-176--adl-14-operational-template-model)), so the generated types hold none. A mandatory `Any` property **MUST** be emitted as `any`.

JSON tags **MUST** preserve the original BMM property name (snake_case), with `omitzero` on a non-mandatory single or generic property held as a pointer (`*T`, `*DVInterval[DVQuantity]`, `*[]T`, `*map[string]V`), so that only a nil pointer is omitted, and `omitempty` on a non-mandatory one that is not a pointer (a Go interface, or a `P_BMM_SINGLE_PROPERTY_OPEN` typed by the open generic parameter). A mandatory property whose Go field can hold no value carries an omit option, so that an absent one is left out of canonical JSON ([REQ-052](wire.md#req-052) § An absent mandatory member): `omitzero` on a list, map or `any` field and on one held as a pointer, a pointer that breaks a cycle included, so that only a nil value is left out and a present empty list or map is still written; and `omitempty` on a Go interface or on a field typed by its class's own generic parameter (a `P_BMM_SINGLE_PROPERTY_OPEN`, or a `P_BMM_SINGLE_PROPERTY` naming that parameter), so that a typed-nil pointer, which encodes as `null`, is left out too. Any other mandatory property, a struct held by value or a primitive, carries no omit option. A `P_BMM_CONTAINER_PROPERTY` field (`[]T`) carries `omitzero` when its cardinality lower bound is 1 or more, and `omitempty` when it is 0 or absent, with one exception: the item-structure lists `ITEM_LIST.items`, `ITEM_TREE.items` and `ITEM_TABLE.rows` carry `omitzero`, so a present empty one is written as `[]` and only a nil one is left out. An archetype can make these lists mandatory by existence (an AOM 1.4 `C_MULTIPLE_ATTRIBUTE`'s `existence` says whether the list is there, its `cardinality` how many members it holds), a CDR refuses a structure that leaves its list out, and no RM invariant keeps them non-empty, so `[]` is RM-valid. Every other optional container keeps `omitempty`; one an invariant keeps non-empty, as `Mappings_valid` keeps `DV_TEXT.mappings`, could not take the exception, since `[]` would be RM-invalid. The generator reads the three from one list, `internal/rmlists`, which a test pins to the vendored BMM. A `P_BMM_GENERIC_PROPERTY` over `Hash` or `List` follows the single-property rule: a mandatory one is `map[string]V` or `[]T` with `omitzero`, and an optional one is `*map[string]V` or `*[]T` with `omitzero`. Why pointer and container fields differ under `encoding/json/v2` is recorded in [ADR 0002 § D8](../adr/0002-bmm-codegen-decisions.md#d8--the-generator-emits-no-bespoke-json-codec-methods) and [ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md). The example below is **conceptual** — it shows the BMM property → Go field mapping and the `DV_AMOUNT` ancestor relationship:

```go
// Conceptual mapping only — not the generated struct shape.
type DVQuantity struct {
    DVAmount                            // conceptual ancestor; flattened by generator (ADR 0002 D4)
    Magnitude            rm.Real        `json:"magnitude"`   // BMM Real → rm.Real per REQ-041 / ADR 0004
    Units                string         `json:"units"`
    Precision            *rm.Integer    `json:"precision,omitzero"`
    NormalRange          *DVInterval[DVQuantity] `json:"normal_range,omitzero"`
    OtherReferenceRanges []ReferenceRange[DVQuantity] `json:"other_reference_ranges,omitempty"`
    UnitsSystem          *string        `json:"units_system,omitzero"`
    UnitsDisplayName     *string        `json:"units_display_name,omitzero"`
}
```

**Generator output diverges from this example for abstract ancestors.** Per [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, the generator renders non-generic abstract classes as Go marker interfaces and **flattens their properties** into every concrete descendant struct. So the real `openehr/rm/data_types_quantity_gen.go` carries `DVQuantity` with `DVAmount`'s fields (`Accuracy`, `AccuracyIsPercent`, `MagnitudeStatus`, …) inlined and **no `DVAmount` embed**. Only concrete ancestors and whitelisted abstract-generic ancestors (e.g. `EVENT` per ADR 0003) appear as Go-level embeds.

#### Type → Go type expression

| P_BMM type kind | Go expression |
|---|---|
| `P_BMM_SIMPLE_TYPE` with `type: "X"` | `X` (or its Go alias from the primitive map; see § Primitive type mapping) |
| `P_BMM_GENERIC_TYPE` with `root_type: "Foo"`, `generic_parameters: ["Bar"]` | `Foo[Bar]` |
| `P_BMM_CONTAINER_TYPE` with `container_type: "List"`, nested type | `[]T` (see § Container mapping) |

#### Container mapping

| BMM container_type | Go shape | Notes |
|---|---|---|
| `List` | `[]T` | Order matters; duplicates allowed |
| `Array` | `[]T` | Same as List in Go; the distinction is historical |
| `Set` | `[]T` | With a generated `Set` invariant check in `Validate()` if validation hooks exist. Go has no native set; modelling as `map[T]struct{}` complicates JSON. |
| `Hash` | `map[string]V` | Keys are `String` |

The wire format JSON does not distinguish List/Set/Array — they all serialise as arrays. Modelling them all as `[]T` keeps round-tripping exact; the Set / Array semantics surface in validators and AQL path-resolution rather than in the type itself.

#### Cardinality

`P_BMM_CONTAINER_PROPERTY.cardinality` (with `lower`, `upper`, `upper_unbounded`) reaches the SDK through the generated attribute metadata in `openehr/rm/rminfo`: `AttrMeta.Container` marks a container property, and `AttrMeta.Required` is true when `cardinality.lower` is 1 or more, which the validation floor checks ([REQ-112](clinical-modeling.md#req-112--template-less-reference-model-validation-floor)). The upper bound is not kept: no container in the pinned RM and BASE BMMs has a finite one.

#### Functions

`P_BMM_*` function definitions on classes (e.g. `DV_QUANTITY.add`, `DV_QUANTITY.less_than`) carry pre/post-conditions and operator aliases. The SDK's posture:

- The generator emits **method stubs** for class functions, with the documented `pre_conditions` / `post_conditions` as `// Pre:` / `// Post:` comments. A `P_BMM_INTERFACE`'s functions are the exception: no stub is emitted for them (Class → Go type above, [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4).
- The generator does **not** emit the function body — implementations are hand-written in non-generated files ([REQ-041](#req-041--pinned-bmm-sources)) where they are needed.
- Functions that have not been hand-implemented **SHOULD** panic with a clear message (e.g. `panic("not implemented: DV_QUANTITY.add — implement in a non-generated file")`); this is one of the rare allowed panics in SDK code, justified by the fact that the function is documented in the BMM but not yet wired in.

For SDK v1, the priority is the **types**, not the **methods** — most consumers do not need RM-level arithmetic. The RM functions the SDK implements, and those it leaves out, are specified in [§ RM behavioural functions](#rm-behavioural-functions).

### Primitive type mapping

**Implements:** REQ-041

The 29 primitives in `openehr_base_1.3.0.bmm.json` map to Go types per the table below. Mappings are **fixed**; do not introduce alternative widenings.

| BMM primitive | Go type | Notes |
|---|---|---|
| `Boolean` | `bool` | |
| `Integer` | `rm.Integer` (underlying `int32`) | Pinned to 32-bit per the BMM spec. Emitted as the defined type `rm.Integer` so the codec can attach the wire-tolerance rule from [ADR 0004](../adr/0004-numeric-wire-tolerance.md). |
| `Integer64` | `rm.Integer64` (underlying `int64`) | Emitted as the defined type `rm.Integer64` so the codec can attach the same wire-tolerance rule as `Integer` ([ADR 0004](../adr/0004-numeric-wire-tolerance.md)). `DV_COUNT.magnitude` is the RM's one `Integer64` attribute. |
| `Real` | `rm.Real` (underlying `float64`) | The BMM `Real` is single-precision conceptually but openEHR JSON uses unbounded-precision numbers; `float64` is the safe choice. Emitted as the defined type `rm.Real` so the codec can attach the wire-tolerance rule from [ADR 0004](../adr/0004-numeric-wire-tolerance.md). |
| `Double` | `float64` | Same as `Real`, no codec tolerance attached. |
| `Character` | `rm.Character` | canonical JSON single-character **string**, not a number; hand-written named type, like `Integer`/`Real` |
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
| `Cardinality` | `rm.Cardinality` (struct) | Generated from the BASE schema — lower, upper, upper_unbounded |
| `Multiplicity_interval` | `rm.MultiplicityInterval` (struct) | Generated from base BMM |
| `Interval` | generic `Interval[T]` | Generated; ancestors set on dependent types |
| `List` | `[]T` | See § Container mapping |
| `Set` | `[]T` | See § Container mapping |
| `Array` | `[]T` | See § Container mapping |
| `Hash` | `map[string]V` | See § Container mapping |
| `Container` | abstract; not directly used | |
| `Terminology_code` | `rm.TerminologyCode` (struct) | Generated from base BMM |
| `Terminology_term` | `rm.TerminologyTerm` (struct) | Generated from base BMM |

#### ISO 8601 types

The four ISO 8601 BMM types map to Go `string` rather than `time.Time` because:

- The openEHR formats include partial dates (`yyyy`, `yyyy-mm`) and partial times (`HH`, `HH:MM`) that `time.Time` cannot round-trip without information loss.
- Wire-format byte-fidelity matters for canonical JSON conformance (REQ-052).

Helpers in `openehr/rm/` (hand-written, non-generated) **SHOULD** offer typed parsing into `time.Time` and back for consumers who need it, but the storage type is the wire-equivalent `string`.

### Generator output conventions

- File naming: `<package_name>_gen.go` for generated files. Example: `openehr/rm/data_types_quantity_gen.go` corresponds to BMM package `org.openehr.rm.data_types.quantity`.
- Header: every generated file begins with `// Code generated by bmmgen; DO NOT EDIT.` followed by a `// Source:` line naming the root schema and the pinned schemas it includes, for traceability.
- RM release: the public constant `rm.Release` **MUST** carry the primary RM schema's `rm_release`. What the SDK writes into `ARCHETYPED.rm_version` is [wire.md § REQ-053](wire.md#req-053) for FLAT and STRUCTURED decode and [clinical-modeling.md § REQ-107](clinical-modeling.md#req-107--template-driven-rm-instance-example-generator) for the instance generator.
- Hand-written companions: any file without the `_gen.go` suffix in the target package (such as the `*_funcs.go` files of `openehr/rm`) holds method bodies, helper functions, and codec hooks.
- One Go package per generation target (BASE and RM both emit into `openehr/rm`), one file per BMM package (Go does not require a one-to-one directory mapping with BMM packages).
- The generator emits a registrations file in `openehr/rm` (and in `openehr/aom/aom14`) which registers every concrete RM/AOM `_type` with its constructor (REQ-040). Per [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D3, the registration file lives in the target package — **not** under `openehr/rm/typereg/` — so the constructor closures can reference the target types directly without forming a `typereg → rm` import cycle.

### Documentation propagation

The generator carries the BMM `documentation` field on classes, properties, and functions into the Go doc comments of the generated types, fields and methods. The transformation is verbatim — no rewording, no abbreviation. Markdown-style cross-refs (`_units_`, `\`magnitude\``) are converted to Go-doc-friendly equivalents.

### v1 scope summary

**Generated in v1:**
- `openehr/rm/` from the SDK-authored RM root over `openehr_rm_1.2.0` (excluding `ehr_extract`) + `openehr_base_1.3.0`.
- The type registrations of `openehr/rm/typereg/`, which is itself hand-written, from the same.
- `openehr/rm/rminfo/` from the same — the flattened attribute tables, the REQ-048 class graph and absence table, and the REQ-112 function table that `rminfo.IsNonStorableAttr` reads.
- `internal/rmaccess` from the same — the attribute access of every registered class (REQ-171).
- The invariant inventory of `openehr/validation` from the same and the hand-kept invariant ledger — the invariant inventory (REQ-171).
- `openehr/aom/aom14/` from `openehr_operational_template_1.4.0`, the SDK-authored root over `openehr_am_1.4.0` + `openehr_base_1.3.0` and the RM root.

**Deferred (BMM file kept, no v1 generation):**
- AOM 2 (`openehr_am_2.4.0`) — consumers requesting ADL 2 / AOM 2 are out of v1 scope.
- LANG (`openehr_lang_1.1.0`) — `openehr/bmm/` is hand-written for v1; the LANG BMM remains as a reference / cross-check.
- TERM (`openehr_term_3.1.0`) — terminology service interface not required by v1 consumers.
- RM `ehr_extract` package — EHR Extract is a separate exchange surface not exercised by v1 use cases.

Deferred items become future plan phases when consumer demand arises; the BMM-conformance contract above continues to apply.

## RM behavioural functions

Normative contract for the **derived / behavioural functions** the openEHR Reference Model defines on its identifier, URI, `PATHABLE`/`LOCATABLE`, version-control, and temporal data-value classes — the operations that `bmmgen` emits as signatures from the pinned BMM but cannot implement from a schema (they carry algorithm, not just shape).

The structural rules above say how the types are generated and shaped; this part governs the **runtime behaviour** of the functions on those types. Identifier lexical forms are authoritative in the openEHR BASE *Identification* package and the RM *Common* package; agents look them up there ([base_types identification](https://specifications.openehr.org/releases/BASE/development/base_types.html#_identification_package), [RM common](https://specifications.openehr.org/releases/RM/development/common.html)) rather than guess.

Surface and fallibility decisions: [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md).

### Surface and fallibility (applies to all REQs here)

[ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) sets the surface. A concrete-typed derivation (identifier components, `is_branch`) is a method on the RM type, written by hand beside the generated code, at the generator's documented "implement in a non-generated file" extension point. A fallible parse (input may be malformed) also offers an error-returning entry point: a package-level `Parse…` function for the identifier forms, and the `ToTime` / `ToDuration` conversions for the temporal forms. The BMM-signature derivation methods are **best-effort**: they return a zero value on malformed input, and the package-level error-returning function is the validity check (there is no per-method `ok` boolean). Where a derivation method needs no validation beyond decomposition (e.g. `VersionTreeID.is_branch`), it stays purely lexical, and only the `Parse…` path speaks to well-formedness.

None of these functions panics on a malformed identifier string, on resolving an absent path, or on a typed-nil node in the object tree: the no-panic rule is [idiom.md § Errors (REQ-025)](idiom.md#errors-req-025)'s. Navigation uses no reflection ([idiom.md § Generics and reflection](idiom.md#generics-and-reflection)).

### REQ-120 — RM identifier parsing and derivation

The SDK **MUST** expose the derived components of the openEHR identifier types, and of `DV_URI`, from their normative lexical forms, on the corresponding `openehr/rm` types:

- `UID_BASED_ID` (lexical form `root '::' extension`): `root`, `extension`, and `has_extension`. `HIER_OBJECT_ID` and `OBJECT_VERSION_ID` inherit these; an `OBJECT_VERSION_ID`'s `object_id` is its `root`.
- `OBJECT_VERSION_ID` (lexical form `object_id '::' creating_system_id '::' version_tree_id`): `object_id`, `creating_system_id`, `version_tree_id`, and `is_branch`.
- `VERSION_TREE_ID` (lexical form `trunk_version [ '.' branch_number '.' branch_version ]`, 1 or 3 dot-separated parts): `trunk_version`, `branch_number`, `branch_version`, `is_branch` (true iff the 3-part branch form is present), and `is_first` (true iff the trunk version is `1`).
- `ARCHETYPE_ID` (lexical form `rm_originator '-' rm_name '-' rm_entity '.' concept { '-' specialisation }* '.v' version_id`): `rm_originator`, `rm_name`, `rm_entity`, `qualified_rm_entity`, `domain_concept`, `specialisation`, and `version_id`.
- `TERMINOLOGY_ID` (lexical form `name [ '(' version ')' ]`): `name` and `version_id`.
- `DV_URI` (an RFC 3986 URI reference, `[ scheme ':' ] hier-part [ '?' query ] [ '#' fragment ]`): `scheme`, `path`, `query` and `fragment_id`, which `DV_EHR_URI` inherits. Each is the RFC 3986 component as written, not percent-decoded, or empty when the reference has none. A relative reference has no scheme, and the path leaves out the authority (the `//system_id` of an `ehr://` URI). The RM admits plain text as a `DV_URI` value, so these functions split any text: the text before the first colon is the scheme only when RFC 3986 § 3.1 admits it as one (a letter, then letters, digits, `+`, `-` or `.`), and the rest splits as RFC 3986 appendix B reads a reference. A nil `DV_URI` or `DV_EHR_URI` answers empty for each, and the malformed-string rule below does not apply to them.

`OBJECT_REF` / `PARTY_REF` and the `PARTY_PROXY` family **SHOULD** expose convenience accessors for their reference and identity components, and `LOCATABLE_REF` **SHOULD** provide `as_uri()` — the `ehr:`-scheme URI built from `namespace` + `id.value` + `path` — because callers otherwise rebuild these from the fields by hand.

Each fallible identifier form has one error-returning parser in `openehr/rm` (`ParseObjectVersionID`, `ParseVersionTreeID`, `ParseArchetypeID`, `ParseTerminologyID`), and client-side helpers such as the version-uid splitter in `openehr/client/ehr` delegate to it rather than re-parse. `HIER_OBJECT_ID` / `UID_BASED_ID` decomposition is best-effort only, with no `Parse…` entry point.

`ParseObjectVersionID` and `ParseVersionTreeID` check each part of their form and **MUST** return an error when one fails:

- `ParseObjectVersionID`: `object_id` and `creating_system_id` are each a `UID`, that is a UUID, an ISO OID or an `INTERNET_ID`. An `INTERNET_ID` is read as dot-separated labels of ASCII letters, digits, `-` and `_`, none empty: wider than RFC 1034's label syntax, since creating systems send ids such as `EMR_APP`, yet refusing white space, control characters and other punctuation.
- `ParseVersionTreeID`: each part is a decimal integer of at least 1 that fits in a signed 64-bit integer on every platform, written without a sign or a leading zero, so that one version has one spelling where ids are compared as strings (`If-Match`, ETags, URL paths).

For a malformed identifier string, the `Parse…` function **MUST** return a non-nil error and the best-effort derivation methods **MUST** return a zero value; neither panics. `DV_URI`'s functions are not identifier derivations in this sense: they split any text, as its bullet above says.

**Acceptance:** for canonical and malformed sample strings of each identifier form, `Parse…`, where the form has one, round-trips the input and derived components equal the spec's lexical decomposition; malformed input yields an error from `Parse…` (no panic); best-effort methods never panic; the client version-uid helper produces identical results to the canonical parser. `ParseObjectVersionID` refuses an `object_id` or `creating_system_id` that is blank or holds white space, a control character or other punctuation, and accepts one with `_`; `ParseVersionTreeID` refuses a part with a sign, a leading zero or a value past the signed 64-bit range, and accepts `3000000000`. The `DV_URI` functions split sample references with and without each component, a `DV_EHR_URI` decoded from a `LINK` target answers them without a panic, and a nil `DV_URI` or `DV_EHR_URI` answers empty.

**Out of scope:** checking a `UID` part beyond the reading above, such as RFC 1034 label syntax or a UUID's version bits; generation of new identifiers (covered for instance synthesis by REQ-107). Of the recommended convenience accessors, only `LOCATABLE_REF.as_uri` exists; the `OBJECT_REF` / `PARTY_REF` / `PARTY_PROXY` ones are deferred, and the underlying fields are already directly accessible on the generated structs.

#### Version-control derived helpers

The SDK **MUST** implement the pure, derived version-control functions, computed from the version identifier:

- `VERSION.is_branch` **MUST** be derived from the version's `uid` (true iff its `version_tree_id` is a branch, as above).

Client-side version *container management* is **out of scope** and **MUST NOT** be implemented as in-memory RM behaviour: `VERSIONED_OBJECT` operations (`version_count`, `all_versions`, `all_version_ids`, `has_version_at_time`, `has_version_id`, `version_at_time`, `version_with_id`, `latest_version`, `latest_trunk_version`, `trunk_lifecycle_state`) and all `commit_*` mutators are server-mediated in this SDK — they are realised over REST in `openehr/client/ehr` and `openehr/client/demographic`, not against a materialised in-memory container. These methods **MUST** remain explicit, documented stubs that fail loudly rather than return a misleading value (e.g. a `version_count` of `0`).

**Acceptance:** `VERSION.is_branch` is true for a branch version uid and false for a trunk uid; the out-of-scope container operations are documented as server-side and do not silently return zero values.

### REQ-121 — Locatable path read access

The SDK **MUST** provide read navigation of an in-memory RM instance by an openEHR path, implementing the `PATHABLE` read operations over the actual object tree:

- `item_at_path(path)` **MUST** return the single item at a **unique** path, and **MUST** return an error when the path is absent or resolves to more than one item.
- `items_at_path(path)` **MUST** return all items matching a **non-unique** path (empty when none match).
- `path_exists(path)` **MUST** report whether the path resolves to at least one item; `path_unique(path)` **MUST** report whether it resolves to exactly one.

The accepted path grammar **MUST** follow the openEHR path syntax — `/`-separated RM attribute-name segments with optional `[archetype_node_id]` and/or `[name]` predicates — consistent with the archetype paths `template.ParsePath` reads ([clinical-modeling.md § REQ-111](clinical-modeling.md#path-lookup) *Path lookup*) and the AQL paths of [§ REQ-109](clinical-modeling.md#req-109--aql-static-lint). An `ISM_TRANSITION` is `PATHABLE` but not `LOCATABLE`, so it carries no `archetype_node_id`; a node-id predicate on `ism_transition` **MUST** therefore match the transition whose `careflow_step` is coded with that id in the `local` terminology. That is the ADL 1.4 convention by which an ACTION archetype's `ism_transition[atNNNN]` constrains `careflow_step` to `local::atNNNN`. A careflow step whose terminology is blank counts as `local`, the reading FLAT decode gives a careflow step that names none ([wire.md § REQ-053](wire.md#req-053)). A transition with no careflow step, another code, or the node id in another terminology **MUST NOT** match the predicate.

The read operations live in `openehr/rm/rmpath`, whose import closure holds neither `transport/` nor `auth/`. Because that package imports `openehr/rm`, the `LOCATABLE` path methods cannot delegate to it without an import cycle; the generated `LOCATABLE.{item_at_path,items_at_path,path_exists,path_unique}` stubs are therefore **suppressed** (not emitted) and the package functions are the canonical surface (see [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) decision refinement 2).

**Attribute coverage for Web Template leaves.** Path navigation is the resolution step the FLAT encoder ([wire.md § REQ-053](wire.md#req-053)) runs before leaf mapping, so an attribute this package cannot navigate is data the encoder cannot emit. Two guard tests beside the Web Template builder hold navigation to the leaves a Web Template carries. The first covers every RM attribute leaf the Web Template export synthesizes from the RM rather than from the template (the *in-context* leaves of [REQ-106](clinical-modeling.md#req-106--webtemplate-json-export)): each resolves through these read operations on a populated instance of its container RM type, or is a documented exemption that records why it stays unresolved and what that costs on encode. A newly synthesized in-context leaf with no navigation support fails that test rather than reach the encoder, and an exemption that no longer matches an emitted leaf is reported as stale. The second covers the nodes the Web Template builds from the template itself, over the vendored operational templates whose root is a `COMPOSITION` (the FLAT encoder takes compositions only): each RM attribute on the path of a node resolves on an instance populated along that path, or is a documented exemption with its reason. The exemptions are the composition metadata the `ctx/` short forms carry on encode (`/language`, `/territory`, `/context/start_time` and `/context/setting`), which resolving would spell under two keys, and attributes the RM derives rather than stores (`EVENT.offset`, a function).

The FLAT encoder **MUST NOT** silently drop a leaf that resolves and whose datatype it can serialise. Resolution failure **MAY** be treated as an absent optional (that is what an unpopulated optional attribute looks like); an absent leaf whose datatype the encoder cannot write **MAY** stay unemitted. A populated value at that leaf is covered by [wire.md § REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes). Neither licence extends to a resolvable, serialisable leaf — an omission there is a defect, not an absent optional.

The exemption list is kept by hand because `item_at_path` returns the same `ErrPathNotFound` whether the addressed attribute is *unknown to the navigator* or *known but unpopulated*. Whether to split that sentinel (e.g. `ErrUnknownAttribute`) and retire the exemption list is open as [STRAND-10](research-strands.md#strand-10--rmpath-one-sentinel-for-two-not-found-conditions), and code leaves it open until that strand resolves.

**Acceptance:** against a representative composition (the blood-pressure / vital-signs shape, with real archetype-node-id and name predicates), a known unique leaf path resolves via `item_at_path` to the expected value; a non-unique path returns the expected item count via `items_at_path`; an absent path reports `path_exists = false`; a multi-match path reports `path_unique = false` and `item_at_path` errors. A typed-nil node anywhere in the tree (including the root) yields no match rather than a panic. For the in-context coverage rule: every leaf the WebTemplate synthesizes resolves on a populated instance of its container RM type, or appears in the exemption list with its reason and encode consequence — and a stale exemption fails the guard.

**Out of scope:** `parent` and `path_of_item` (the inverse: object → its path). These require parent back-pointers the concrete RM structs do not carry and identity-based tree search; they remain documented, fail-loud panic stubs until a consumer need is established.

### REQ-123 — Temporal data-value helpers

The SDK **MUST** expose read, inspection, comparison, and conversion helpers for the ISO 8601-backed temporal data values `DV_DATE`, `DV_TIME`, `DV_DATE_TIME`, and `DV_DURATION`, parsing each type's `value` string per ISO 8601 (including openEHR's documented `DV_DURATION` deviations: a leading negative sign and mixing the `W` designator with others), in its extended and basic layouts, with a comma or a dot before fractional seconds and the zone forms `Z`, `±hh`, `±hhmm` and `±hh:mm`, refusing `24:00:00` as BASE states, and accepting two forms the BASE predicates do not list, as SDK deviations: a zone whose style differs from the layout of the time it follows (`10:30:00+0530`, `103000+05:30`), and a `DV_DATE_TIME` without its hour (`2024`, `2024-03`, `2024-03-15`), where BASE `Iso8601_date_time` needs one.

- **Component access** — each type **MUST** expose the components of its parsed form: `DV_DATE` → `year`/`month`/`day`; `DV_TIME` → `hour`/`minute`/`second`/`fractional_second`; `DV_DATE_TIME` → their union; all with `timezone` where present; `DV_DURATION` → `years`/`months`/`weeks`/`days`/`hours`/`minutes`/`seconds`/`fractional_seconds`.
- **Partial-form inspection** — `DV_DATE`/`DV_DATE_TIME`/`DV_TIME` **MUST** report partial forms (`is_partial`, and for dates `month_unknown`/`day_unknown`), since openEHR admits `"2024"` / `"2024-03"` approximate values that Go's `time.Time` cannot represent.
- **Magnitude & comparison** — each type **MUST** expose `magnitude()` (days for `DV_DATE`; seconds for `DV_TIME`/`DV_DATE_TIME`/`DV_DURATION`, a duration's being its length in seconds within the precision of the result, computed without an intermediate integer overflow; a `DV_TIME`'s counting the seconds from the start of its day as written, its time zone not applied; a `DV_DATE_TIME`'s counting the seconds from `0001-01-01T00:00:00Z`, as the RM defines it, a value with a time zone normalised to UTC and a value without one counting its clock time) and a total ordering consistent with the spec's `less_than` / `is_strictly_comparable_to`; the package-level `Compare` helper **MAY** implement the shared ordering logic for idiomatic Go callers.
- **Go bridge** — each type **SHOULD** offer idiomatic conversion (`DV_DATE`/`DV_TIME`/`DV_DATE_TIME` → `time.Time`; `DV_DURATION` → `time.Duration`), because a Go caller computes with those types; a value that cannot map cleanly gets an error, as below.

These helpers **MUST NOT** panic on any value or receiver, and **MUST** answer a value they cannot use as follows:

- the component accessors, the partial-form inspections and `magnitude()` answer zero, the empty string or false for a value they cannot read and for a nil receiver, components parsed before the error included, so a malformed value is never reported as partial;
- `ToTime` and `ToDuration` return a non-nil error for a malformed, partial or calendar-nominal (`Y`/`M` duration components) value, for a definite duration too long for `time.Duration` (never a value wrapped around by overflow), and for a nil receiver;
- for a leap second (second `60`) on a `DV_TIME` or `DV_DATE_TIME`, `ToTime`'s error wraps `rm.ErrTemporalConversion`: a Go `time.Time` cannot hold it, and normalising it into the next minute would change the instant the value names.

The leap second is still a valid ISO 8601 value: the accessors and `is_partial` read it as written. A duration component too large for a Go `int` is unparseable input.

**Acceptance:** for canonical, partial and basic-layout sample strings of each type, with comma fractions, each zone form and the two SDK deviations (`10:30:00+0530`, a `DV_DATE_TIME` of `2024`), the component accessors and `is_partial` match the ISO 8601 decomposition; `magnitude()` and comparison order a known sequence correctly, and the `DV_DATE_TIME` `2026-01-01T10:00:00+05:00`, which is `05:00Z`, compares below `2026-01-01T08:00:00Z`; the Go-bridge conversion succeeds for full values and errors for partial/nominal ones; malformed samples (an illegal day such as `2024-04-31`, a repeated or misordered duration designator such as `P1Y2Y`) return an error from `ToTime`/`ToDuration`; `23:59:60` and `2016-12-31T23:59:60Z` return an error wrapping `rm.ErrTemporalConversion` from `ToTime`; and malformed samples give zero components and no partial flag, and never panic. A nil receiver never panics and answers as an unparseable value does. `ToDuration` errors for `PT9999999999999H` and `P400000D`, and for a length whose intermediate sum or product would wrap back into range, such as `PT18446744074S`; `PT1.9999999999S` converts to 2 s, its fraction rounded to the nearest nanosecond. `magnitude()` of `PT9999999999999999H` is that duration's length in seconds within the precision of the result, and `PT9999999999999999999H`, whose hours do not fit an `int`, answers as an unparseable value does.

**Out of scope:** temporal **arithmetic** — `add`/`subtract`/`diff` against `DV_DURATION`, `DV_DURATION.multiply`/`negative`, and the calendar-aware `add_nominal`/`subtract_nominal` (leap-year / short-month semantics). Deferred to a follow-up REQ; the generated arithmetic methods remain documented stubs.
