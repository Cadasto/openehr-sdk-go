---
kind: specification
---

# Reference Model in Go

**Status:** Draft

Rules for representing the openEHR Reference Model (RM) in Go. Covers REQ-034 and REQ-040; the retired REQ-030 to REQ-033 are [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4, summarised in [§ The shape of the RM in Go](#the-shape-of-the-rm-in-go).

The RM types in `openehr/rm/` are **generated** from the pinned BMM schemas in [`../resources/bmm/`](../../resources/bmm); the rules below state the *shape* the generator emits. Generator semantics (mapping rules per P_BMM construct, primitive types, file conventions) are in [bmm-conformance.md](bmm-conformance.md).

## The shape of the RM in Go

The RM has deep polymorphism: `LOCATABLE → ENTRY → COMPOSITION`; `DATA_VALUE → DV_QUANTITY → DV_DURATION`. Go's type system does not have inheritance, and emulating it leads to runtime-tag confusion, reflection-heavy decoders and runtime errors that belong at compile time. The generator's output rule is [ADR 0002](../adr/0002-bmm-codegen-decisions.md) D4; in brief:

- **Concrete structs.** Every concrete RM class is a concrete Go struct (`Composition`, `Observation`, `DVQuantity`, …). There is no runtime base struct that types inherit from, no single RM struct with a `Type` discriminator field, and no `interface{}`-typed payload with a `Kind` enum.
- **Flattened ancestors.** An abstract non-generic ancestor's properties are flattened into every concrete descendant: the generated `Composition` carries `Name`, `ArchetypeNodeID`, `UID` and the other `LOCATABLE` fields as its own. Only concrete ancestors and whitelisted abstract-generic ancestors (such as `EVENT`, [ADR 0003](../adr/0003-rm-event-polymorphism.md)) appear as embedded structs.
- **Marker interfaces for abstract categories.** An abstract RM category (`DATA_VALUE`, `ITEM_STRUCTURE`, `ENTRY`, …) is a Go interface with an unexported marker method (`isDataValue()`), which keeps a type outside the package from satisfying it by accident; a concrete type satisfies it through its method set, with no registration.
- **The type registry for `_type`.** The `_type` discriminator maps onto a concrete Go type only through the type registry ([§ Type registry (REQ-040)](#type-registry-req-040)): no tag-magic dispatch on a peeked `_type`, and no generic "RM node" that holds any kind of RM value indistinguishably.

## openEHR terminology vocabulary (REQ-034)

The openEHR Terminology — the `openehr` terminology's **groups** (*audit change type*, *version lifecycle state*, *setting*, *participation mode*, *composition category*, …) and **code sets** (*normal statuses*, *compression algorithms*, *integrity check algorithms*), and the **external code sets** the Foundation publishes beside them (*languages* ISO_639-1, *countries* ISO_3166-1, *character sets* IANA_character-sets, *media types* IANA_media-types), that the RM's own invariants reference (`EVENT_CONTEXT.Setting_valid`, `AUDIT_DETAILS.Change_type_valid`, `PARTICIPATION.Mode_valid`, `DV_ORDERED.Normal_status_validity`, `ENTRY.Language_valid`, …) — is part of the specifications the SDK targets, published as a computable artefact by the openEHR Foundation. It is small, closed and versioned with the specifications; it is **not** a runtime terminology-service lookup. The external code sets are the snapshots the Foundation publishes with the TERM release, not the live ISO and IANA registers. Beside them the release publishes the **property-unit table**, `PropertyUnitData.xml`: for each code of the *property* group, the units of measure that measure it, with their UCUM codes, which a `C_DV_QUANTITY` constraint's `property` names ([clinical-modeling.md § REQ-103](clinical-modeling.md#req-103--primitive-constraint-introspection)). The SDK carries the terminology the way it carries the BMM ([bmm-conformance.md § REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources)):

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
- A decoded value of static type `any` is type-asserted at the call site. The SDK provides generic helpers (`typereg.DecodeAs[T DataValue](data)`) where useful.

The registry is also **reversible** ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)):

- A generated `rm.RMTypeName(any) (string, bool)` **MUST** map every registered concrete Go type — including generic instantiations over the parameter bound's closed descendant set — back to its bare registration name (`DVInterval[DVQuantity]` → `"DV_INTERVAL"`), and report `("", false)` for a nil interface, a typed-nil pointer or a non-RM value.
- A generated `rm.IsTypedNil(any) bool` **MUST** report whether a value is an interface carrying a typed-nil pointer to a registered concrete type; it is the guard to use before calling `Locatable` accessors.

Both are generated with the forward registrations, so forward and reverse mappings cannot drift, and neither uses reflection.

### Generated LOCATABLE identity surface

Because ADR 0002 D4 flattens the `LOCATABLE` fields into every concrete descendant, the generator also emits a polymorphic identity surface over them ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)):

- Every LOCATABLE concrete type **MUST** satisfy the sealed `rm.Locatable` interface through four value-receiver read accessors — `GetArchetypeNodeID() string`, `GetName() DVTextLike`, `GetUID() UIDBasedID`, `GetArchetypeDetails() *Archetyped` — each returning the flattened field's declared type, so both `T` and `*T` satisfy it.
- For every LOCATABLE concrete type `T`, `*T` **MUST** satisfy the sealed `rm.MutableLocatable` interface, which collects the four matching pointer-receiver setters.
- Accessors return the field verbatim, so a getter called on a typed-nil `*T` panics; a caller guards with `rm.IsTypedNil` before asserting or calling.
- The surface is machine-owned: accessor emission resolves types through the same rules as struct-field rendering, and the generator's drift check covers it ([bmm-conformance.md § REQ-041](bmm-conformance.md#req-041--pinned-bmm-sources)).

## Generics for clients, validators, repositories

Client, validator and repository generics follow [idiom.md § Generics and reflection](idiom.md#generics-and-reflection).

```go
// openehr/client/ehr/composition (sketch)
func Get[T rm.CompositionLike](ctx context.Context, c *Client, id ObjectVersionID) (T, error)
```

`rm.CompositionLike` is a constraint interface bounding the generic to types in the Composition family (the rare consumer-defined extension of `Composition`; most callers use `*rm.Composition` directly).

## What is NOT in scope here

- **Schema generation strategy.** The RM types in `openehr/rm/` are generated from the pinned BMM schemas; see [bmm-conformance.md](bmm-conformance.md) for the generator contract. The rules in this document apply to the *output* of the generator.
- **Template-specific generated structs.** A typed Go struct for a specific OPT (e.g. a vital-signs template) is **not** part of `openehr/rm/` — that belongs in the consuming project (see [scope.md § Out of scope](scope.md#out-of-scope-v1)).
- **The full RM type list.** The dictionary of types lives in `openehr/rm/` once implemented; this spec defines the *rules*, not the inventory.

## Codec interaction

Codecs in `openehr/serialize/` are the consumers of the type registry on the read path, and the inverse on the write path:

- **Read:** the codec peeks `_type`, looks up the constructor in the registry, decodes into the concrete struct.
- **Write:** the codec encodes the concrete struct; `_type` is set by the struct's `MarshalJSONTo` (or a custom encoder), keyed off the struct's known `_type` constant (typically a package-level `const TypeDVQuantity = "DV_QUANTITY"`).

The underlying JSON library is the standard library's `encoding/json/v2` ([ADR 0022](../adr/0022-canonical-json-encoding-json-v2.md)); the type-registry contract above holds regardless — codecs consult the registry, the registry does not depend on them. STRAND-04's remaining open questions are the full-RM inventory and codec performance, not the codec choice.
