# Reference Model in Go

**Status:** Draft

Rules for representing the openEHR Reference Model (RM) in Go. Covers REQ-030 through REQ-040.

The RM types in `openehr/rm/` are **generated** from the pinned BMM schemas in [`../resources/bmm/`](../../resources/bmm); the rules below state the *shape* the generator emits. Generator semantics (mapping rules per P_BMM construct, primitive types, file conventions) are in [bmm-conformance.md](bmm-conformance.md).

The RM has deep polymorphism: `LOCATABLE → ENTRY → COMPOSITION`; `DATA_VALUE → DV_QUANTITY → DV_DURATION`. Go's type system does not have inheritance, and emulating inheritance leads to runtime-tag confusion and reflection-heavy decoders. This SDK takes a different approach: **concrete structs**, **embedded base structs**, **interfaces for abstract categories**, and **a central type registry for `_type`**.

## Concrete types (REQ-030)

Every concrete openEHR RM type **MUST** be expressed as a concrete Go struct. There is no `LocatableBase struct` that everything inherits from at runtime.

```go
// Concrete types
type Composition struct { ... }
type Observation struct { ... }
type DVQuantity   struct { ... }
type DVCodedText  struct { ... }
type DVText       struct { ... }
```

The SDK **MUST NOT** emulate inheritance via:

- A single `RM` struct with a `Type` discriminator field and a typed payload.
- A `Base` struct embedded into a `Composition` plus a runtime cast that pretends `*Base` "is" a `Composition`.
- An `interface{}`-typed `Data` field with a `Kind` enum.

These are common in dynamic-language ports. They produce reflection-heavy decoders, unergonomic call sites, and runtime errors that should have been compile-time errors.

## Embedded base structs (REQ-031)

Shared RM fields **MUST** be expressed as embedded structs in concrete types.

> **Generator output.** The example below teaches the conceptual layering REQ-031 requires. The actual `bmmgen` output diverges per [`../docs/adr/0002-bmm-codegen-decisions.md`](../adr/0002-bmm-codegen-decisions.md) D4: abstract non-generic ancestors are emitted as Go marker interfaces and their properties are **flattened** into every concrete descendant struct. The generated `Composition` therefore carries `Name`, `ArchetypeNodeID`, `UID`, etc. as own fields rather than via a `Locatable` embed. Only concrete ancestors and whitelisted abstract-generic ancestors (e.g. `EVENT` per ADR 0003) appear as Go-level embeds. The teaching example below remains the normative *model* of the layering REQ-031 expresses; ADR 0002 D4 is the canonical *output rule*.

```go
// Locatable carries the fields that every LOCATABLE descendant shares.
type Locatable struct {
    Name            DVText
    ArchetypeNodeID string
    UID             *UIDBasedID
}

// Pathable carries the parent-pointer that PATHABLE descendants share.
// (In the wire format this is implicit; in the in-memory model it's optional
// and set by the parser when reconstructing a tree.)
type Pathable struct {
    parent any // unexported; set by the parser
}

// Identified is a common base for entities with a system_id.
type Identified struct {
    SystemID HierObjectID
}

// Concrete COMPOSITION uses embeds:
type Composition struct {
    Locatable
    Identified
    // Composition-specific fields:
    Language        CodePhrase
    Territory       CodePhrase
    Category        DVCodedText
    Context         *EventContext
    Composer        PartyProxy
    Content         []ContentItem
}
```

Rules for embedded base structs:

- Each base struct **SHOULD** correspond to a real RM ancestor (`LOCATABLE`, `PATHABLE`, `ENTRY`, `EVENT`).
- Embedded fields **MUST NOT** be promoted into a "do-everything" base; if `EVENT` and `INSTRUCTION` share three fields by accident, define a third base struct rather than packing all six fields into `Entry`.
- Embeds **MUST NOT** be used as interface implementations by composition trickery (e.g. embed a base struct that "implements" `DataValue` to make every descendant satisfy it) — interfaces are method sets (see § Abstract categories below); embeds carry data, not behaviour.

### Generated LOCATABLE identity surface

Because ADR 0002 D4 flattens the `LOCATABLE` fields into every concrete descendant, the generator **MUST** also emit a polymorphic identity surface over them ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)):

- Every LOCATABLE concrete **MUST** carry four value-receiver read accessors — `GetArchetypeNodeID() string`, `GetName() DVTextLike`, `GetUID() UIDBasedID`, `GetArchetypeDetails() *Archetyped` — whose return types **MUST** equal the flattened field's declared type. The sealed `rm.Locatable` interface lists them; both `T` and `*T` satisfy it.
- Every LOCATABLE concrete **MUST** carry the four matching pointer-receiver setters, collected behind the sealed `rm.MutableLocatable` interface (satisfied by `*T` only).
- Accessors return the field verbatim: a getter invoked on a typed-nil `*T` panics, so callers **MUST** guard with `rm.IsTypedNil` (§ Type registry below) before asserting/calling.
- The surface is machine-owned: accessor emission resolves types through the same rules as struct-field rendering, and `make codegen-verify` gates drift (REQ-042).

## Abstract categories (REQ-032)

Abstract RM categories — types that no instance can be directly — **MUST** be expressed as Go interfaces:

```go
// DATA_VALUE is abstract in the RM; every DV_* type is concrete.
type DataValue interface {
    isDataValue() // unexported marker — concrete types implement it
}

// ITEM_STRUCTURE is abstract; ITEM_TREE, ITEM_LIST, ITEM_SINGLE, ITEM_TABLE are concrete.
type ItemStructure interface {
    isItemStructure()
}

// ENTRY is abstract; OBSERVATION, EVALUATION, INSTRUCTION, ACTION, ADMIN_ENTRY are concrete.
type Entry interface {
    isEntry()
    // Entry-level methods that all concrete entries must support:
    Language() CodePhrase
    Encoding() CodePhrase
}
```

Implementation rules:

- A concrete type satisfies the interface by virtue of its method set; **MUST NOT** require explicit registration.
- For abstract categories with no shared behaviour (pure type tags), use an unexported marker method (e.g. `isDataValue()`). This prevents external packages from declaring "imposter" types that satisfy the interface accidentally.
- For abstract categories with shared behaviour (e.g. `Entry.Language()`), declare those methods in the interface and implement them per concrete type — typically by promotion from an embedded base struct.

## No inheritance emulation (REQ-033)

The combination of REQ-030, REQ-031, REQ-032 is **the** way the SDK models openEHR polymorphism. The SDK **MUST NOT**:

- Use tag-magic alone to dispatch on `_type` (e.g. `json.RawMessage` + a switch on a peeked string field with no central registry — REQ-040 below).
- Provide a "generic RM node" type that holds any kind of RM value indistinguishably (`type RMNode struct { Type string; Data map[string]any }`).
- Generate Go types from the RM dictionary as a single mega-file — generation **MAY** be used per package, but the rules above still apply to the output.

## Type registry (REQ-040)

A central type registry in `openehr/rm/typereg` **MUST** map each `_type` string to the constructor of its concrete Go type. Polymorphic JSON decoding of RM fields **MUST** consult the registry.

```go
// openehr/rm/typereg/registry.go (sketch)

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

- The registry **MUST** be append-only at runtime. Calling `Register` twice for the same `_type` with different constructors **MUST** panic — a name collision is a programmer error, not a recoverable condition.
- The `rm` package's `init()` **MUST** register every concrete RM type. Consumers do not register types unless they extend the RM (which the SDK does not actively support, but must not actively prevent).
- The registry **MUST NOT** use reflection to instantiate types — the constructor closure is the only sanctioned mechanism.
- A decoded value of static type `any` **MUST** be type-asserted at the call site. The SDK provides generic helpers (`typereg.DecodeAs[T DataValue](data)`) where useful.

The registry is also **reversible** ([ADR 0013](../adr/0013-generated-locatable-identity-surface.md)):

- A generated `rm.RMTypeName(any) (string, bool)` **MUST** map every registered concrete Go type — including generic instantiations over the parameter bound's closed descendant set — back to its bare registration name (`DVInterval[DVQuantity]` → `"DV_INTERVAL"`). Nil interfaces, typed-nil pointers, and non-RM values report `("", false)`.
- A generated `rm.IsTypedNil(any) bool` **MUST** report whether a value is an interface carrying a typed-nil pointer to a registered concrete; it is the sanctioned guard before calling `Locatable` accessors.
- Both **MUST** be reflection-free and regenerated with the forward registrations, so forward and reverse mappings cannot drift.

## Generics for clients, validators, repositories (REQ-024)

Generics carry typed responses through clients and validators without forcing reflection or `any`-casts at every call site:

```go
// openehr/client/ehr/composition (sketch)
func Get[T rm.CompositionLike](ctx context.Context, c *Client, id ObjectVersionID) (T, error)
```

`rm.CompositionLike` is a constraint interface bounding the generic to types in the Composition family (the rare consumer-defined extension of `Composition`; most callers use `*rm.Composition` directly).

Constraints to follow:

- A constraint interface **SHOULD** be a marker (`isComposition()`) plus the necessary method set — same rule as abstract categories.
- Constraint interfaces **MUST NOT** appear in the *function* signature outside the type parameter list — `Get[T rm.CompositionLike](...) (T, error)` is correct; `func Get(...) (rm.CompositionLike, error)` is wrong because the runtime type is erased.

## What is NOT in scope here

- **Schema generation strategy.** The RM types in `openehr/rm/` are generated from the pinned BMM schemas; see [bmm-conformance.md](bmm-conformance.md) for the generator contract and [`../docs/plans/`](../plans) for the implementation plan. The rules in this document apply to the *output* of the generator.
- **Template-specific generated structs.** A typed Go struct for a specific OPT (e.g. a vital-signs template) is **not** part of `openehr/rm/` — that belongs in the consuming project (see [scope.md § Out of scope](scope.md#out-of-scope-v1)).
- **The full RM type list.** The dictionary of types lives in `openehr/rm/` once implemented; this spec defines the *rules*, not the inventory.

## Codec interaction

Codecs in `openehr/serialize/` are the consumers of the type registry on the read path, and the inverse on the write path:

- **Read:** the codec peeks `_type`, looks up the constructor in the registry, decodes into the concrete struct.
- **Write:** the codec encodes the concrete struct; `_type` is set by the struct's `MarshalJSON` (or a custom encoder), keyed off the struct's known `_type` constant (typically a package-level `const TypeDVQuantity = "DV_QUANTITY"`).

An open research question (STRAND-04) is the choice of underlying JSON library (`encoding/json` vs `sonic` vs `easyjson`). Whatever the choice, the type-registry contract above stays — codecs are pluggable; the registry is not.
