---
kind: adr
id: ADR-0022
title: "Canonical JSON: `encoding/json/v2` through generated streaming marshalers; member order not promised; numbers strict on encode, tolerant on decode"
status: accepted
date: 2026-09-14
---

# ADR 0022 — Canonical JSON: `encoding/json/v2` through generated streaming marshalers; member order not promised; numbers strict on encode, tolerant on decode

- **Status:** Accepted, 2026-09-14.
- **Amended:** 2026-11-25: takes in the member-order and number-tolerance decisions and the design of the generated encoders, each class now writing its members itself.
- **Strand:** resolves the `encoding/json/v2` and default-codec sub-questions of [STRAND-04](../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance), and retires the byte-stability premise of its codec gate.
- **Amends:** [REQ-052](../specifications/wire.md#req-052) (member order is unspecified; a quoted number decodes).
- **Related:** [REQ-040](../specifications/reference-model.md#type-registry-req-040) (the registry the dispatch hooks read), [REQ-041](../specifications/reference-model.md#req-041--pinned-bmm-sources) (the primitive mapping the number types keep), [REQ-056](../specifications/wire.md#req-056) (canonical XML keeps its element order), [REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) (the floor a round trip is checked against); [ADR 0002](0002-bmm-codegen-decisions.md) (the generator that emits the codec).

## Context

**What `encoding/json` v1 cannot give.**
Canonical JSON needs `_type` on a value held in an interface or a map, dispatch on `_type` at a substitutable slot, and one place that classifies a shape failure.
v1 gives none of them without generated methods per class and a helper per interface slot.
`encoding/json/v2`, standard library in Go 1.27 and so within the module floor, gives the first two: it calls a marshal method whatever the value's addressability (`go doc encoding/json` § Migrating to v2), and a `json.UnmarshalFromFunc` registered through `json.WithUnmarshalers` dispatches at a slot.

**Member order.**
No openEHR specification gives JSON member order a meaning, RFC 8259 § 4 gives it none, and CDRs differ in the order they write.
A promised order constrains every encoder the SDK might adopt, and a probe that compares two encodes tests only that the encoder agrees with itself, which it does when it drops the same field twice.

**Numbers.**
Some producers write a `Real` or `Integer` quoted: `testkit/corpus/compositions/vital_signs.json` carries `"magnitude": "1256"`.
A strict-number decoder refuses that output; a consumer of the SDK's own output needs numbers.

**Encode cost.**
Marshalling a wrapper struct per value allocates for every value written, and json/v2 copies a value held by value in an interface slot before calling its method.

## Decision

**Canonical JSON is encoded and decoded by `encoding/json/v2` through a streaming marshaler pair the generator emits per concrete class. Member order is not promised. `Real`, `Integer` and `Integer64` are written as numbers and read from a number or a quoted decimal.**

### The generated pair

- The generator emits `MarshalJSONTo(*jsontext.Encoder) error` and `UnmarshalJSONFrom(*jsontext.Decoder) error` for each concrete RM and AOM 1.4 class, and no `MarshalJSON` / `UnmarshalJSON` pair.
- `MarshalJSONTo` has a value receiver, so it is in the method set of a value and of a pointer: a value held in an interface slot or a map writes its `_type`.
  The receiver matters under a v1 entry point, whose `CallMethodsWithLegacySemantics` skips a pointer-receiver method on an unaddressable value.
- `UnmarshalJSONFrom` refuses a nil receiver and calls one shared helper, `typereg.DecodeInto`.
  It decodes into a method-free alias of the class, or into a flat wire struct for a class that embeds a concrete ancestor with methods of its own, whose alias would promote them.
  It then checks `_type` and classifies a shape failure ([wire.md § Decode-side shape sentinel](../specifications/wire.md#decode-side-shape-sentinel)).
- A substitutable slot is served by one `json.UnmarshalFromFunc` per polymorphic interface, built from `typereg.Default` at init and supplied through `json.WithUnmarshalers`.
  Every hook passes `dec.Options()` to its nested decode; without it a deeper slot loses its hook.
- The generated decoders hold no logic per class beyond that delegation, except the interval classes.
  Encode leaves out an open side's bound ([wire.md § An open interval side carries no bound](../specifications/wire.md#an-open-interval-side-carries-no-bound)), and decode gives an absent boundary flag its default ([wire.md § REQ-169](../specifications/wire.md#req-169--interval-boundary-flag-defaults)); the delegation sees neither.
  The generator emits that logic for every class carrying the four flags, BASE `Interval` and the AOM 1.4 constraint intervals included, and refuses a flag-carrying class whose shape it does not handle.
- The decoders are specified for a zero-value receiver: reuse of a populated receiver is unspecified, and the alias and the flat struct differ on it ([wire.md § Decode state after a failure](../specifications/wire.md#decode-state-after-a-failure)).
- Determinism is an option, not an emergent property: `typereg.MarshalOptions` passes `json.Deterministic(true)` to every generated marshaler, so `Hash` keys are written sorted.
- `internal/bmmgen`, `openehr/rm`, `openehr/aom/aom14`, `openehr/rm/typereg` and `openehr/serialize/canjson` are on v2, and SDK code decodes no RM or AOM 1.4 value through v1 ([wire.md § Decode-side shape sentinel](../specifications/wire.md#decode-side-shape-sentinel)), so one decode-error taxonomy sits behind the sentinels.
  A package that decodes no RM value may stay on v1, which is itself implemented on v2.
- The generator emits one shape, with no flag to choose another; rollback is a revert and `make codegen`.

### Encode writes each member itself

The generated `MarshalJSONTo` writes `{`, `_type` and each member in struct declaration order as tokens on the encoder:

- a string, a boolean or a pointer to one, as a token;
- `rm.Real`, `rm.Integer` and `rm.Integer64`, through unexported writers in `openehr/rm` that write the bytes their `MarshalJSON` writes; the three types have no `MarshalJSONTo`, which a struct embedding one would promote and prefer to its own method;
- a generated class, by calling its `MarshalJSONTo`;
- a value in an interface slot, through the `json.MarshalerTo` it implements;
- a `Hash` map, an `Any` member, `[]byte`, `rm.Character`, an AOM 1.4 number and an integer enumeration, through `json.MarshalEncode`, so `json.Deterministic` still sorts `Hash` keys and json/v2 names the type in a failure.

It applies each member's omit option as json/v2 applies the tag, a typed-nil pointer in an interface slot included.
It marshals the wrapper instead, the alias or the flat struct with `_type` leading, through `json.MarshalEncode`, so the output and the error are json/v2's:

- when the encoder carries an option that changes how a member is written or a failure reported: a caller's marshalers, `OmitZeroStructFields`, `StringifyNumbers`, v1's legacy `omitempty` or v1's legacy error reporting (`typereg.DirectEncode`);
- when the value sits more than 900 levels deep, so a pointer cycle fails as json/v2's cycle;
- before writing anything, when a member that may be left out, typed by an interface or a type parameter, holds a value of a type the registry does not list.

The encoder's own options, indentation and escaping, apply to the tokens either way, and a failure names the class whose method failed, as json/v2 does at a method boundary.

### Omit options

The rules are [reference-model.md § Property → Go field](../specifications/reference-model.md#property--go-field)'s; they differ by field kind for these reasons:

- An optional pointer takes `omitzero`, so only a nil pointer is left out; under v2, `omitempty` would also drop a pointer to `""`, which encodes as an empty value.
- An optional list takes `omitempty`, so a nil list and a present empty one are both left out: `omitzero` would write the empty one as `[]`, which is RM-invalid for a list an invariant keeps non-empty, as `Mappings_valid` keeps `DV_TEXT.mappings`.
- The item-structure lists `ITEM_LIST.items`, `ITEM_TREE.items` and `ITEM_TABLE.rows` take `omitzero`, read from `internal/rmlists`: an archetype can make them mandatory, a CDR refuses a structure that leaves its list out, and no invariant keeps them non-empty.
  Canonical XML, and FLAT or STRUCTURED decoded without the template, cannot carry their `[]`, and every other list an archetype can require keeps `omitempty`.
- A mandatory list or map takes `omitzero`, so a present empty one is written.
  A mandatory interface or type-parameter field takes `omitempty`, so a typed-nil pointer, which encodes as `null`, is left out; `any` takes `omitzero`, since an empty string or object it holds is a value.

### Member order

Encoded member order is not part of the contract ([wire.md § Member order](../specifications/wire.md#member-order)).
`_type` first is recommended, so a consumer decoding a stream can select the type before the rest; beyond it the order is unspecified, and decode accepts any order.
Writing `Hash` keys sorted is a recommended property of the encoder, so one unchanged value has one encoding, not an order promise ([wire.md § Map-valued (`Hash`) members](../specifications/wire.md#map-valued-hash-members)).
A round trip is compared by value, never by bytes ([wire.md § Round-trip comparison](../specifications/wire.md#round-trip-comparison)).
The SDK ships no canonical form for hashing or signing; a consumer that needs a stable digest canonicalises the output itself, with RFC 8785 for instance.
Canonical XML keeps its element order ([REQ-056](../specifications/wire.md#req-056)), since element order is part of an XML document's identity in a way member order is not part of a JSON object's.

### Numbers

`rm.Real`, `rm.Integer` and `rm.Integer64` are defined types over `float64`, `int32` and `int64`: the fixed primitive mapping of [REQ-041](../specifications/reference-model.md#primitive-type-mapping) holds for the underlying type, and the codec has a type to attach the tolerance to.
They are encoded as JSON numbers only, and decoded from a number or from a quoted decimal string, which reads as the number it spells; the integer rules, the lossy-literal refusal and the error each failure takes are [wire.md § Floating-point precision](../specifications/wire.md#floating-point-precision)'s.
Each tolerance answers a failure observed in real input, never one foreseen: `Double` keeps a plain `float64` and plain decode, since no fixture writes it quoted.
The SDK has no strict-number mode: the tolerant default reads the real output of CDRs and loses nothing, so a second strictness knob would only make the contract conditional.
A caller that must refuse quoted numbers checks the input before `canjson.Unmarshal`.

## Consequences

- Every RM value is self-describing under any JSON entry point: a value held in an interface or a map writes its `_type`, so each entry of a `Hash<String, RM_TYPE>` such as `AUTHORED_RESOURCE.translations` carries one.
- `canjson` and a v1 caller are not byte-identical: v2 leaves `<`, `>` and `&` unescaped, and an optional polymorphic slot holding a typed-nil pointer is absent through `canjson` and `null` through v1 ([wire.md § Typed-nil in an optional polymorphic slot](../specifications/wire.md#typed-nil-in-an-optional-polymorphic-slot)).
- Decode refuses invalid UTF-8, a lone surrogate escape and a repeated member name, and matches member names case-sensitively ([wire.md § Decode-side shape sentinel](../specifications/wire.md#decode-side-shape-sentinel)).
  `canjson.Marshal` refuses a string holding invalid UTF-8, and `canjson.MarshalIndent` an indent or prefix that is not spaces and tabs, with `canjson.ErrInvalidValue` ([wire.md § Encode-side refusal sentinel](../specifications/wire.md#encode-side-refusal-sentinel)).
- The standard library's decode errors are `*jsontext.SyntacticError` and `*json.SemanticError`; the SDK's sentinels and `*transport.DecodeError` keep their identity.
  A `SemanticError` carries the value that failed, which [wire.md § Value-free decode errors](../specifications/wire.md#value-free-decode-errors) keeps off the boundary.
- `contribution.CommitVersion` embeds `json.MarshalerTo`, since a generated class implements no `json.Marshaler`.
- A quoted number comes back as a bare number, and `DVCount.Magnitude` is `rm.Integer64`, so a caller assigning a typed `int64` converts it.
- Writing members directly cut the Demonstration composition's encode from 147 to 58 µs and from 1,012 to 6 allocations (darwin/arm64); 177 of those allocations were the numbers' `MarshalJSON` calls.
  The cost is code: the generated JSON encoders grew from 4,883 to 19,572 lines, and a program that encodes canonical JSON from 8.44 to 9.63 MB.
  Tests encode every corpus document and every registered class both ways and compare the bytes, under each option the gate checks.
- The codec carries a performance budget: a benchmark regresses materially when its time/op is more than 20 percent worse than the retired generated marshalers' at p < 0.05, measured with `benchstat` ([PR 171](https://github.com/Cadasto/openehr-sdk-go/pull/171)).
- Withdrawing the order promise is one-way: restoring it after consumers have absorbed its removal would break them again.

## Alternatives considered

- **Keep v1 and its generated marshalers.** Rejected: 278 generated methods reproduce what v2 gives, in the part of the tree every BMM bump re-emits.
- **Move to v2 but keep the `MarshalJSON` / `UnmarshalJSON` pair.** Rejected: it keeps every method, books a second migration, and forgoes the allocation saving `go doc encoding/json/v2.MarshalerTo` names.
- **No methods, only options built from the registry.** Rejected: `json.Marshal(&rm.DVText{})` without the options would write no `_type`.
- **`sonic` or `easyjson`.** Rejected: a third-party dependency in the SDK's hottest correctness path, or a second code-generation surface.
- **Leave `openehr/aom/aom14` or the RM primitives on v1.** Rejected: [REQ-052](../specifications/wire.md#req-052) holds AOM 1.4 decode to the RM's failure classes, and two taxonomies would sit behind one sentinel set.
- **Keep the member-order profile as a requirement.** Rejected: no specification asks for it, and `go doc encoding/json/v2.Deterministic` holds determinism only across identical binaries, so the promise would be weaker than it reads.
- **Ship a canonicalisation helper for consumers who hash.** Rejected: the withdrawn promise under another name, for a format whose specification defines no canonical form.
- **A strict-number decode option.** Rejected, as *Numbers* says.

## Traceability

- Resolves: STRAND-04 (the `encoding/json/v2` and default-codec sub-questions)
- Amends: REQ-052
