---
kind: specification
---

# Clinical modeling

**Status:** Draft

Normative contract for the SDK's clinical-modeling artefacts: operational templates (OPT), templated composition assembly, and validation against templates.
The AQL requirements, including the lint's checks against a compiled template, are in [aql.md](aql.md).
Covers REQ-101 onwards, except those AQL requirements.

The "clinical modeling" band sits above the openEHR Reference Model and below the REST clients: it consumes RM types from `openehr/rm/` and AOM 1.4 constraint types from `openehr/aom/aom14/`, and produces typed building blocks usable by `openehr/composition/`, `openehr/validation/`, and `openehr/aql/`.

---

## REQ-176 — ADL 1.4 operational template model

The SDK **MUST** provide, in its AOM 1.4 package, generated types for the ADL 1.4 operational template as the AM Release-1.4 `Template.xsd` defines it, `OPERATIONAL_TEMPLATE` at the root, with the canonical-XML codec of [wire.md § REQ-056](wire.md#req-056) and the canonical-JSON codec of [wire.md § REQ-052](wire.md#req-052), so a consumer reads an OPT into typed values that carry everything the document carries and writes it back.
The types come from the SDK-authored root schema [reference-model.md § SDK-authored root schemas](reference-model.md#sdk-authored-root-schemas) describes, which declares the operational template classes and includes the RM target's root, so that `AUTHORED_RESOURCE`, which `ARCHETYPE` inherits, takes its RM `common.resource` form by the include merge of [reference-model.md § REQ-045](reference-model.md#req-045--bmm-loader-is-a-building-block).
`OPERATIONAL_TEMPLATE` inherits from no class: it declares its own members, in the order and with the types `Template.xsd` gives them, not the `AUTHORED_RESOURCE` ones, so it has `language` and not `original_language`, and no `translations`.
The model is what the compile step of [§ REQ-111](#req-111--public-compiled-template-bridge) reads, so that the compiled template can carry what an OPT says ([ADR 0035](../adr/0035-compile-step-reads-operational-template-model.md)).
`aom14.ReadOperationalTemplate` reads an untrusted OPT into the model within the bounds of [§ REQ-108](#req-108--untrusted-document-bounds), with the tolerances of § *What the reader tolerates*.

### Round trip

An OPT **MUST** decode with no unmatched content, so [wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)'s report is empty and its strict option refuses nothing, where the OPT:

- validates against the pinned AM Release-1.4 schemas (`Template.xsd` and its closure, their `targetNamespace` read as the `http://schemas.openehr.org/v1` that OPTs and the SDK write, [wire.md § REQ-056](wire.md#req-056));
- has no `xsi:type` value that names a class under a namespace prefix, which [wire.md § REQ-056](wire.md#req-056) refuses;
- has no `EXPR_LEAF.item` element that holds element content, unless its `xsi:type` names a class;
- repeats no key in a `Hash` member, a repeat being unmatched content ([wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)).

Encoding the value such an OPT decodes to **MUST** give a document that validates against the same schemas and holds the same content: the same elements in the same order with the same attributes and text, compared after the normalisation below, with the XML declaration, white space between elements and the rewrites the next section names set aside.
The normalisation:

- the elements of one `Hash` member compare as a set of entries, since a `Hash` keeps no order;
- the text of a numeric element compares by its value, so `1.0` equals `1`, and the text of a boolean element by its value, as XML Schema reads a boolean, so `1` equals `true`;
- an interval element that leaves out a boundary flag compares as one that writes the flag's [wire.md § REQ-169](wire.md#req-169--interval-boundary-flag-defaults) default;
- an `xsi:type` compares as the qualified name it denotes;
- a `SCALE`'s `value` and `symbol`, which the schema admits in either order, compare in either order;
- an `xsi:type` the encoder does not write, on an element whose slot holds a single class, is set aside, as is the root element's.

The encoder names the root element after the schema's global element for the root's class, `template` for an `OPERATIONAL_TEMPLATE`, where [wire.md § REQ-056](wire.md#req-056) gives another root the lower-cased class name; decode does not match the root by name ([wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)).

The `Hash` members of the model, `ARCHETYPE_TERM.items`, `translations` and the maps of `RESOURCE_DESCRIPTION`, `RESOURCE_DESCRIPTION_ITEM` and `TRANSLATION_DETAILS`, round-trip in the shapes the AM Release-1.4 schemas give them:

- a `Hash<String,String>` as one `StringDictionaryItem` element per entry, the key in its `id` attribute and the value as its text;
- a `Hash` whose values are a class as one element per value, keyed by the value's `id` attribute where its schema type declares one.

The resource classes `RESOURCE_DESCRIPTION_ITEM` and `TRANSLATION_DETAILS` are keyed by their `language` code and carry no `id` attribute, as AM Release-1.4 writes them.
They are the `rm` ones, so their canonical XML takes that shape wherever they occur, in an RM document too, although BASE Release-1.2.0's `Resource.xsd` gives the two an `id` attribute.
The encoder writes a `Hash` member's entries in ascending order of their keys, so a value encodes to one document whatever the order of its map, and refuses with `canxml.ErrInvalidShape` an entry of a language-keyed member held under a key other than its value's language code, which would read back under that code.

### Shapes the pinned BMM lacks

The root restates each AOM 1.4 class whose AM Release-1.4 schema type differs from the pinned BMM in the schema type's form ([reference-model.md § SDK-authored root schemas](reference-model.md#sdk-authored-root-schemas)): `C_QUANTITY_ITEM` gains `precision`, and `assumed_value`, `Any` on `C_DEFINED_OBJECT` in the BMM, takes the schema's type on each class, `CODE_PHRASE` on `C_CODE_PHRASE` and `DV_QUANTITY` on `C_DV_QUANTITY` among them.
The shapes the schema types alone do not settle:

- The model's two integer enumerations, `OPERATOR_KIND` on `EXPR_OPERATOR.operator` and the type of `timezone_validity` on `C_DATE`, `C_TIME` and `C_DATE_TIME`, are written as their integer codes: as element text in canonical XML, such as `2007` or `1001`, and as a JSON number of the same value in canonical JSON.
  Decode reads the code or the item name the root declares for it, as element text or as a JSON string.
  `timezone_validity` takes either spelling the AM 1.4 schemas give it: a word of the BASE `VALIDITY_KIND` enumeration the current schema uses (`mandatory`, `optional`, `prohibited`, and `disallowed`, the BASE name AOM 1.4 uses for prohibited), or a code of the Release 1.0.2 schemas (`1001`, `1002`, `1003`).
  The reader and the compile step ([§ REQ-111](#req-111--public-compiled-template-bridge)) **MUST** read both: `mandatory` and `1001` as mandatory, `prohibited`, `disallowed` and `1003` as disallowed (`1003`), and `optional`, `1002`, an absent element or an empty one as optional, the zero value.
  A `canxml` or `canjson` decode refuses any other value, in every mode, with `canxml.ErrInvalidShape` or `canjson.ErrInvalidShape`.
  The model's reader reads a `timezone_validity` that names no `VALIDITY_KIND` as § *What the reader tolerates* says.
  The pinned `Archetype.xsd` admits both spellings; the encoder writes the code, since some CDRs refuse the words on template upload.
- `EXPR_LEAF.item`, `Any` in the BMM and `xs:anyType` in the schema, decodes as the type its `xsi:type` names, read as [wire.md § REQ-056](wire.md#req-056) reads a discriminator: an AOM 1.4 class such as `C_STRING`, or an XML Schema built-in under a prefix bound to the XML Schema namespace, `xs:string` and `xsd:string` alike, as a `String`.
  An item with no `xsi:type` reads its text as a `String`.
  Element content in an item read as a `String`, which `xs:anyType` allows, is unmatched content ([wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)).
  The encoder writes the `xsi:type` the item was read with, declaring the prefix it uses, and none where it had none.
  In canonical JSON an item read with an XML Schema built-in is an object whose `_type` is the BMM primitive the built-in reads as and whose `value` is its text as that primitive's JSON value, such as `{"_type":"String","value":"archetype_id/value"}`, which decodes with the `xsd` prefix and one built-in per primitive, `xsd:string` for a `String`.
- `match_negated`, which Archetype Designer writes as `false` on every `C_SINGLE_ATTRIBUTE` and `C_MULTIPLE_ATTRIBUTE`, is an element no AOM class declares, which the pinned `Archetype.xsd` accepts fixed to `false`, and it carries no meaning:
  - with the value `false`, written `false` or `0` as the schema's boolean allows, it is read and dropped in every mode, neither reported nor refused, and never written;
  - with any other text it is refused in every mode with `canxml.ErrInvalidShape`, since AOM 1.4 cannot express a negated match.
- `ORDINAL.symbol` and `SCALE.symbol` take one of the two shapes of the schema's `ORDINAL_SYMBOL`, the `DV_CODED_TEXT` shape or the `CODE_PHRASE` shape, which the element's first child chooses.
  The `DV_CODED_TEXT` shape makes `value` optional, since a constraint gives the text no meaning: Archetype Designer leaves it out and other tools write it empty.
  The encoder writes that `value` only when it is non-empty, so a symbol read with no `value` or an empty one is written with none.
  A `SCALE` is read with its `value` and `symbol` in either order, as `OpenehrProfile.xsd` admits them and Archetype Designer writes the symbol first, and is written with its `value` first, the order of `ORDINAL` and of the RM's `DV_SCALE`.
- `xsi:type="T_ARCHETYPE_ROOT"`, which some exporters write for the same shape where openEHR [`Template.xsd`](https://github.com/openEHR/specifications-ITS-XML/blob/master/components/AM/Release-1.4/Template.xsd) defines only `C_ARCHETYPE_ROOT`, reads as `C_ARCHETYPE_ROOT`, with its archetype id, term definitions and subtree, in every mode, so a strict decode does not refuse it; it is written as `C_ARCHETYPE_ROOT`.

### What the reader tolerates

In its default mode, `aom14.ReadOperationalTemplate` **MUST** read every OPT that *Round trip* names, and every OPT that differs from one only in the shapes the table below and [§ REQ-111](#req-111--public-compiled-template-bridge) *What the compile step tolerates* name, as they read each shape, except an OPT that:

- holds one of the four readings the reader refuses in every mode (below);
- is one the compile step refuses ([§ REQ-111](#req-111--public-compiled-template-bridge) *Reading the model*);
- lies past the bounds of [§ REQ-108](#req-108--untrusted-document-bounds).

| The OPT holds | The reader reads it | Given `canxml.WithStrict` |
|---|---|---|
| a UTF-8 byte-order mark at its start | as the document after it | the same |
| an element in the `http://schemas.openehr.org/v2` namespace, which the pinned AM Release-1.4 schemas declare | as the element of that name in the `http://schemas.openehr.org/v1` namespace canonical XML reads ([wire.md § REQ-056](wire.md#req-056)) | the same |
| a `C_BOOLEAN` whose `true_valid` or `false_valid` is absent or empty | with that flag true | the same |
| an `assumed_value` of a `C_BOOLEAN`, `C_INTEGER` or `C_REAL` that is not a value of its type, an assumed boolean written `t` or `f` among them, and one of a `C_DV_QUANTITY` that the model cannot hold | as absent, and an assumed boolean written in another case, such as `tRuE`, as its value | the same |
| a `C_DV_QUANTITY` magnitude or precision bound that is not a number | as unbounded on its side | the same |
| a `C_INTEGER` range bound or a precision bound written as a decimal number | as the integer bound that admits the same integers, a lower bound rounded up and an upper one down, and a lower bound of negative infinity, an upper one of positive infinity, or one that is not a number (`NaN`), which bounds no integer, as unbounded on its side | the same |
| a bound written on a side marked unbounded | without it | the same |
| an empty `timezone_validity` | as absent | the same |
| an `<attributes>`, `<children>` or `<item>` inside `<definition>` with no type in the XSI namespace | such an `<attributes>` as `C_MULTIPLE_ATTRIBUTE` with its cardinality where it carries a `<cardinality>` and as `C_SINGLE_ATTRIBUTE` otherwise, such a `<children>` as `C_ARCHETYPE_ROOT` where it carries an archetype id and as `C_COMPLEX_OBJECT` otherwise, and such an `<item>` as no constraint, losing any constraint its type would have carried | refused |
| a `<children>` or the `<definition>` whose `xsi:type` names no AOM 1.4 class, or a class it cannot hold such as `C_SINGLE_ATTRIBUTE`, other than the classes of a `<definition>` it refuses in every mode (below) | as a `C_COMPLEX_OBJECT` leaf with the `rm_type_name`, `node_id` and `occurrences` it carries and none of its attributes | refused |
| a `C_INTEGER` or `C_REAL` list entry or range bound that is not a number of its type, an empty one among them | the entry left out and the bound read as unbounded on its side | refused |
| a `timezone_validity` that names no `VALIDITY_KIND` | as absent | refused |
| an `xsi:type` inside `<definition>` with white space around it | as the name without it | refused |
| an `<item>` of a `C_PRIMITIVE_OBJECT` whose type names no `C_PRIMITIVE` class and none of `C_CODE_PHRASE`, `C_DV_QUANTITY`, `C_DV_ORDINAL`, `CONSTRAINT_REF` and `C_PRIMITIVE_OBJECT` | as no constraint | refused |
| an element in a namespace other than none, v1 and v2, the root and the XSI namespace among them | as the v1 one, a type attribute keeping its XSI namespace | refused |
| a `<children>` typed `C_COMPLEX_OBJECT` that carries an archetype id | as `C_ARCHETYPE_ROOT` | refused |
| a `<children>` typed with a `C_PRIMITIVE` class | as a `C_PRIMITIVE_OBJECT` leaf that carries it | refused |
| an `<item>` typed `C_CODE_PHRASE`, `C_DV_QUANTITY`, `C_DV_ORDINAL` or `CONSTRAINT_REF`, or typed `C_PRIMITIVE_OBJECT` | as the object that holds it, of the item's class, and one typed `C_PRIMITIVE_OBJECT` as the item it holds | refused |
| a `C_REAL` value, or a `C_DV_QUANTITY` magnitude bound, that Go's float parse reads and XML Schema's `double` does not, such as `Inf`, `0x1p-2` or more digits than a `double` holds | as the number Go reads | refused |

Given `canxml.WithStrict`, the reader **MUST** refuse each of these shapes:

- a `<children>` whose `xsi:type` names no AOM 1.4 class and that carries nested `<attributes>`, which the default mode reads as a leaf and so drops a subtree;
- an `<attributes>` or `<children>` inside `<definition>`, or an `<item>` of a `C_PRIMITIVE_OBJECT`, that carries no `type` attribute in the XSI namespace (`http://www.w3.org/2001/XMLSchema-instance`), whether it has no `type`, an empty one, or one whose prefix is bound to another namespace, such as a misspelt one;
- a `timezone_validity` that names no `VALIDITY_KIND`.

ITS-XML makes `C_ATTRIBUTE`, `C_OBJECT` and `C_PRIMITIVE`, the declared types of those elements, abstract ([`Archetype.xsd`](https://github.com/openEHR/specifications-ITS-XML/blob/master/components/AM/Release-1.4/Archetype.xsd)), so a valid document names a concrete type on each; `<definition>` itself is declared `C_ARCHETYPE_ROOT` by `Template.xsd` and needs none.
The reader's strict decode refuses as well what canonical XML's strict decode refuses, an unknown `xsi:type` and unmatched content among it ([wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)), and keeps the readings the table gives for every mode.

What a tolerance leaves out, such as the attributes of that leaf, is unmatched content, which the report option of [wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode) reports.
Well-formedness does not depend on the mode: in both, the reader holds what comes before, inside and after the root element to the rules of `canxml.Unmarshal` ([wire.md § REQ-056](wire.md#req-056)).
Content it refuses there, and an element that carries one attribute twice outside what a tolerance leaves out, are not well-formed XML: the error wraps `canxml.ErrNotWellFormed` as well as `canxml.ErrInvalidShape`.
Markup before the root that `encoding/xml` cannot read fails as its `*xml.SyntaxError` instead, unless content `canxml.Unmarshal` refuses comes before it: the reader then refuses that content in either mode.
The reader refuses, in every mode, four readings, the first three with an error wrapping `canxml.ErrInvalidShape`:

- an integer that the model's `Integer`, 32 bits as openEHR's is, cannot hold;
- a `match_negated` of true, which AOM 1.4 cannot express (§ *Shapes the pinned BMM lacks*);
- an integer bound of positive infinity on a lower side or negative infinity on an upper one, which admits no integer;
- a `<definition>` typed `ARCHETYPE_SLOT`, or with a class that carries a primitive constraint ([§ REQ-103](#req-103--primitive-constraint-introspection)), a `C_PRIMITIVE` class such as `C_STRING`, one of `C_CODE_PHRASE`, `C_DV_QUANTITY`, `C_DV_ORDINAL` and `CONSTRAINT_REF`, or a `C_PRIMITIVE_OBJECT` whose items carry such a constraint, which the model's definition, a `C_ARCHETYPE_ROOT`, cannot hold, with a `*canxml.DecodeError` of the definition that wraps `typereg.ErrTypeMismatch`.

A `canxml` decode called directly has none of these tolerances.

### Out of scope

- What the compile step makes of the model: [§ REQ-111](#req-111--public-compiled-template-bridge) *Reading the model* says it, and [§ REQ-108](#req-108--untrusted-document-bounds) bounds the reader ([ADR 0035](../adr/0035-compile-step-reads-operational-template-model.md)).
- OET authoring templates, ADL 2 operational templates and AOM 2.
- Validating an OPT against its archetypes, and validating a composition against the generated model: [§ REQ-102](#req-102--composition-validation) stays on the compiled template.

- **Probes:** PROBE-111 (corpus round trip)

---

## REQ-108 — Untrusted document bounds

Clinical-modeling and codec entry points **MUST** bound how much untrusted input they read and how deeply they recurse, so hostile OPT XML, BMM JSON, uploaded templates, or crafted canonical JSON cannot exhaust memory or CPU before the caller's own policy kicks in.
Two threats need no guard beyond these bounds.
Go's `encoding/xml` does not expand DTD entity declarations and refuses an undeclared entity, so the model's OPT reader is open to neither external-entity injection nor entity expansion.
`C_STRING` patterns compile with Go's `regexp` (RE2, linear time), so a crafted pattern cannot backtrack catastrophically.

The limits **MUST** be these:

| Limit | Value | Applies to |
|---|---|---|
| size limit | 32 MiB | an OPT, an uploaded template and a BMM document |
| depth limit | 128 constraint-object levels below an OPT's `definition` | the model the reader reads (counted in its XML elements, below), the compile step and the compiled template's path walk |
| decode depth limit | 512 levels below the document root | an RM value |

Each leaves wide room above real templates, schemas and records while still bounding memory and recursion.

### Operational template model reader (`openehr/aom/aom14/`)

- `aom14.ReadOperationalTemplate` **MUST** refuse an input larger than the size limit, and an element nested deeper than the element limit below, with an error wrapping `canxml.ErrLimitExceeded`.
  It reads at most one byte past the size limit, and it refuses a deep element during the decode, through `canxml.WithMaxDepth`.
  A `canxml.WithMaxDepth` among the caller's options applies as well; the smaller limit wins.
  An input with no root element is refused with an error wrapping `io.ErrUnexpectedEOF`, not a bare `io.EOF`.
- The element limit is the depth limit counted in the model's XML elements, with the root element at depth 1.
  It is 320 = 2 × 128 + 64: an `<attributes>` and a `<children>` for each of the 128 levels below the definition, and 64 for the root `<template>`, its `<definition>` and the deepest content one constraint object holds, such as an assumed value or an expression tree.
- A caller that decodes an OPT with `canxml.Unmarshal` or a `canxml.Decoder` itself gets neither limit unless it applies them.

### Canonical-XML decode depth (`openehr/serialize/canxml/`)

- With `canxml.WithMaxDepth(n)`, `canxml.Unmarshal` and a `canxml.Decoder` **MUST** refuse, during the decode, an element nested deeper than `n`, the root element at depth 1, with a `*canxml.DecodeError` that wraps `canxml.ErrLimitExceeded` and names the depth and the element's path but none of its content.
  Without the option a decode has no depth limit of its own, but `encoding/xml` decodes no element nested deeper than 10 000 elements, and a decode refuses the first such element it hands to `encoding/xml` the same way, naming 10 000 as the limit.
  Given more than one, the smallest positive `n` holds, in any order; an `n` of 0 or less sets no limit and does not lift one already given.
- Neither call sets a depth or size limit by default: a caller that decodes an untrusted document gives `canxml.WithMaxDepth` and bounds the bytes it reads, as `aom14.ReadOperationalTemplate` does.

### Compile step and path lookup (`openehr/templatecompile/`)

- `Compiled.Resolve` and `Compiled.ValidatePath` **MUST** refuse a walk deeper than the depth limit with an error wrapping `template.ErrPathNotFound`.
- `Compile` refuses a model whose constraint objects nest deeper than the depth limit, with an error wrapping `ErrInvalidInput`.
  The reader's element limit keeps a model it reads within that depth, so only a model built in code, or decoded with `canxml` directly, reaches it.

### BMM load (`openehr/bmm/`)

- `bmm.Load` **MUST** refuse an input larger than the size limit with `bmm.ErrInputTooLarge`.
  See also REQ-045.

### Definition template upload (`openehr/client/definition/`)

- `UploadTemplate` **MUST** refuse a body larger than the size limit, which the package exports as `definition.MaxUploadBytes`, before it forwards bytes to the CDR, with an error wrapping `definition.ErrUploadTooLarge` and `transport.ErrInvalidConfig`.

### Polymorphic JSON decode (`openehr/rm/typereg/`)

- An RM value that opens deeper than the decode depth limit below the document root **MUST** be refused with `typereg.ErrMaxDepthExceeded` on every canonical-JSON decode route, `Registry.Decode`, `canjson.Unmarshal`, `canjson.Decoder`, and a bare `encoding/json/v2` or v1 `encoding/json` caller of the generated methods; on the two routes that buffer a value, `Registry.Decode` for its document and a polymorphic slot for its value, so is any bracket of that value deeper than the limit.
  Nesting below which no RM value opens, inside a declared array, an `any` or map member, or an undeclared member, counts toward the limit only on those two routes; outside a buffered value, `jsontext`'s own limit of about 10 000 levels bounds it.
  See [ADR 0002](../adr/0002-bmm-codegen-decisions.md) and REQ-040.

---

## REQ-103 — Primitive constraint introspection

The SDK **MUST** expose every OPT primitive constraint as a typed value attached to its leaf node, so validators and composition-builder consumers can introspect ranges, allowed lists, patterns, units, and code lists without re-parsing the OPT XML.
The `constraints` package imports neither the OPT reader nor the compile step, so code generators and downstream validators can use the constraint types alone ([module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence)).

The compile step builds these values from the primitive classes of the operational template model ([§ REQ-176](#req-176--adl-14-operational-template-model)): the `item` of a `C_PRIMITIVE_OBJECT`, `C_CODE_PHRASE`, `C_CODE_REFERENCE`, `C_DV_QUANTITY`, `C_DV_ORDINAL` and `CONSTRAINT_REF` ([§ REQ-111](#req-111--public-compiled-template-bridge) *Reading the model*).
The rules below bind the compile step, which reads a value the OPT's schema does not allow as [§ REQ-111](#req-111--public-compiled-template-bridge) *What the compile step tolerates* says.

### Scope

The closed set of REQ-103 primitive constraints maps **one-to-one** to ADL 1.4 OPT XSD primitive `xsi:type` values:

| OPT `xsi:type` | Go type (`openehr/template/constraints/`) | Surface |
|---|---|---|
| `C_BOOLEAN` | `CBoolean` | `TrueValid`, `FalseValid`, optional `Default` |
| `C_INTEGER` | `CInteger` | `Range`, optional closed `List`, optional `Default` |
| `C_REAL` | `CReal` | `Range`, optional closed `List`, optional `Default` |
| `C_STRING` | `CString` | `Pattern` (regex), optional `List`, `ListOpen` (closed `List` when false, open when true), optional `Default` |
| `C_DATE` | `CDate` | `Pattern` (ADL 1.4 date pattern, raw), `Range` (`TemporalRange`), `TimezoneValidity` |
| `C_TIME` | `CTime` | `Pattern` (ADL 1.4 time pattern, raw), `Range` (`TemporalRange`), `TimezoneValidity` |
| `C_DATE_TIME` | `CDateTime` | `Pattern` (ADL 1.4 date/time pattern, raw), `Range` (`TemporalRange`), `TimezoneValidity` |
| `C_DURATION` | `CDuration` | `Pattern` (ADL 1.4 duration pattern, raw), `Range` (`TemporalRange`) |
| `C_CODE_PHRASE` | `CodePhrase` | `Terminology`, optional `CodeList`, `External()` predicate |
| `C_CODE_REFERENCE` | `CodeReference` | the code phrase constraint it refines, with the `CodePhrase` surface, and the `reference_set_uri` that names its value set |
| `C_DV_QUANTITY` | `DvQuantity` | enumerated `Units` (each with magnitude / precision `NumericRange`), optional `Property` (CodedTermRef), `PropertyUnits` (the UCUM units the property admits, primary units first) |
| `C_DV_ORDINAL` | `CDvOrdinal` | `Values` (closed list of `(int, CodedTermRef)` pairs) |
| `CONSTRAINT_REF` | `ConstraintRef` | `Reference` (the `ac` code that names the value set; the node's `RMTypeName()` is the RM type it constrains, typically `CODE_PHRASE`). The value set is external terminology, so `Validate` reports no violation for a value of that RM type ([§ REQ-102](#req-102--composition-validation) *Terminology binding value-set*) |

Each type implements the sealed interface `constraints.PrimitiveConstraint`:

```go
type PrimitiveConstraint interface {
    Validate(value any) []Violation
    ExampleValue() any         // § Example value emission
    isPrimitive()              // unexported — closes the interface
}
```

The set is closed by `isPrimitive()`; new primitive shapes appear in the `constraints` package only, each added to the table above.

### Accessor

- `templatecompile.CompiledNode.PrimitiveConstraint() constraints.PrimitiveConstraint` — returns the typed value the compile step builds from the node's primitive class in the model; returns nil for non-primitive nodes (composition root, archetype roots, slots, plain complex objects).

### Validate contract

`Validate(value any) []Violation` returns nil when the input satisfies every clause of the constraint, or one `Violation` per failing clause (range, list, pattern, …).
Validators are pure functions with no I/O, which the import rule of [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence) holds, and the reflection rule of [idiom.md § Generics and reflection](idiom.md#generics-and-reflection) applies to them unchanged.
Each accepts a small fixed set of Go input types.
Concretely:

- Integer / real validators accept any Go integer kind (`int`, `int8`..`int64`, `uint`, `uint8`..`uint64`).
  `uint` and `uint64` values exceeding `math.MaxInt64` return `CodeWrongType` rather than silently wrapping.
  `CReal.Validate` additionally accepts `float32` / `float64`.
- String, date, time, date-time, duration validators accept Go `string`.
- `CString.Validate` **MUST** match `Pattern` against the whole string, as if written `^(?:Pattern)$`, so a value that only contains a match is a `CodePatternMismatch`.
  The AOM leaves the match mode open, and the SDK reads the pattern as whole-string because the specification's own examples (`/.+/`, `/km\/h|mi\/h/`) read most naturally that way and Archie, a widely used AOM implementation, matches with Java `String.matches`, which is whole-string.
- `CString.ListOpen` carries the AOM 1.4 `C_STRING.list_open` attribute, "True if the list is being used to specify the constraint but is not considered exhaustive" (AOM 1.4, class `C_STRING`).
  When it is true, `CString.Validate` **MUST NOT** report a value outside `List`, since the list is not exhaustive; it still checks `Pattern` when one is set.
  When it is false, `List` is closed, and a value outside a non-empty `List` is a `CodeNotInList`.
  AOM 1.4 makes the attribute mandatory, but the ITS-XML AM 1.4 schema (`Archetype.xsd`, complex type `C_STRING`) makes the `list_open` element optional and gives it no default, so the compile step **MUST** read an absent or empty element as false: a list stays closed unless the OPT opens it.
  Text that is not a boolean fails the read, as it does for the `C_BOOLEAN` flags.
  With a `List`, open or closed, `ExampleValue` returns the first entry that `Pattern` matches, in the whole-string reading `Validate` applies, or the first entry when no `Pattern` is set.
  When `Pattern` matches no entry, a closed list admits no value at all, so it has no example `Validate` accepts (§ *Example value emission*); `ExampleValue` then still returns the first entry.
  An open list bounds nothing: with no `Pattern` set `Validate` accepts its example, as it accepts any string.
- `CDate`, `CTime` and `CDateTime` **MUST** report a value their `Pattern` does not admit as a `CodePatternMismatch`.
  They read the pattern as ADL 1.4 reads a date, time or date/time pattern (ADL 1.4, *Constraints on Dates, Times and Durations*, *Patterns*), its letters in either case, since OPTs write both (`yyyy-mm-ddTHH:MM:SS`).
  A field written in its letters, or as a literal number (`1995-??-XX`), is mandatory, and a literal requires that value.
  A field written `??` is optional, and one written `XX` is not allowed.
  A fraction is allowed wherever seconds are.
  A trailing timezone pattern makes a zone mandatory: `Z` admits `Z` only, and `±hh`, `±hh:mm` or `±hhmm`, signed `+`, `-` or `±`, admits any zone the RM parse accepts, `Z` included, and without one a zone is optional.
  ADL 1.4 reads `±hh` as an hours-only zone and `±hh:mm` as a full one; the SDK requires a zone but does not hold it to the shape the suffix writes, so `±hh:mm` admits `+01` and `±hh` admits `+05:30`.
  The ITS-XML pattern grammars of `C_TIME` and `C_DATE_TIME` (`Archetype.xsd`) carry no zone suffix, so only an OPT that breaks that schema, or a constraint built in code, reaches the suffix.
  An empty `Pattern` constrains nothing.
  A pattern outside that grammar, or one that breaks its validity rules for `??` and `XX`, is malformed (below).
- `CDate`, `CTime` and `CDateTime` carry as `TimezoneValidity` the `timezone_validity` that the ITS-XML `C_DATE`, `C_TIME` and `C_DATE_TIME` (`Archetype.xsd`) write beside `pattern`: mandatory, disallowed, or optional, the zero value, read from either of its spellings as [§ REQ-176](#req-176--adl-14-operational-template-model) *Shapes the pinned BMM lacks* says.
  `CTime.Validate` and `CDateTime.Validate` **MUST** report a value with no zone under mandatory, and a value with a zone under disallowed, as a `CodePatternMismatch`: the rule is the zone part of the pattern, which ADL 1.4 writes as a suffix.
  A date-time with no time has no zone.
  Where `Pattern` writes a zone suffix as well, both rules apply.
  A date has no zone, so on a `C_DATE` optional and disallowed admit every date and mandatory admits none.
  The SDK reads mandatory there as an OPT defect, since AOM 1.4 gives `C_DATE` no `timezone_validity` and only the ITS-XML schema does: the compile step reads it as [§ REQ-111](#req-111--public-compiled-template-bridge) *What the compile step tolerates* says, and on a `CDate` built in code it is malformed (below).
- `CDuration` **MUST** report a value that carries a designator its `Pattern` does not name as a `CodePatternMismatch`.
  It reads the pattern as ADL 1.4 reads a duration pattern (ADL 1.4, *Constraints on Dates, Times and Durations*, *Duration Constraints*): the letters before `T` name `Y`, `M`, `W` and `D`, those after it `H`, `M` and `S`, in either case, and the value may carry any one or more of them.
  A value of either sign is admitted, and a fraction wherever `S` is.
  A pattern outside the grammar of the ITS-XML `DurationConstraintPattern` (`Archetype.xsd`), read in either case, or one that names no designator, is malformed (below).
- `CDate`, `CTime`, `CDateTime` and `CDuration` carry the AOM 1.4 `range` as a `TemporalRange`: `Lower` and `Upper` as ISO 8601 text of the constrained type, with the included and unbounded flags of a `NumericRange` (§ *Numeric range*).
  Its zero value constrains nothing.
  `Validate` **MUST** report a value outside the range as a `CodeOutOfRange`, judging it only against a bound it is ordered against.
  A duration is ordered as the RM's `Compare` orders it, by BASE `Iso8601_duration.to_seconds`, which `rm.DVDuration.Magnitude` computes.
  A date, a time or a date-time is compared with a bound at the precision the two share: field by field from the largest down, over the fields both carry, the seconds with their fraction.
  The first field that differs decides, and two values equal on every shared field are equal.
  A bound so covers the whole period it names: `2021-05` lies within `1900..2021` and outside `1900..<2021`.
  A time or a date-time is ordered against a bound as [§ REQ-112](#req-112--template-less-reference-model-validation-floor) *DV_INTERVAL* orders two DV_TIME or two DV_DATE_TIME values:
  - at that shared precision when the two are written with one zone designator, or both with none;
  - as instants, each read in UTC by the `magnitude()` of [REQ-123](reference-model.md#req-123--temporal-data-value-helpers), in which a partial value stands for the start of the period it names, when both carry a zone and the designators differ;
  - not at all when one carries a zone and the other does not, so such a bound draws no `CodeOutOfRange`.

  So a `CTime` with the range `>09:30:00+02:00` admits `08:00:00Z`, which is 10:00 at `+02:00`, and `09:00:00`, which has no zone.
  ADL 1.4 asks a closed interval to give a zone on both bounds or on neither (*Intervals*); the SDK does not check that, and orders the value against each bound on its own.
  The compile step reads a bound that the ISO 8601 parse of the constrained type refuses as [§ REQ-111](#req-111--public-compiled-template-bridge) *What the compile step tolerates* says; on a `TemporalRange` built in code, such a bound on a side that is not unbounded is malformed (below).
- `CBoolean.Validate` accepts Go `bool`.
- `CodePhrase.Validate` accepts either a bare `string` (treated as the code under the constrained terminology) or a `constraints.CodedTermRef`.
- `ConstraintRef.Validate` accepts a bare `string` or a `constraints.CodedTermRef` and reports no violation for either, since the value set it names is external terminology; any other Go type is `CodeWrongType`.
- `CodeReference.Validate` accepts what `CodePhrase.Validate` accepts and judges a value by the code phrase constraint it carries.
  It does not look up the value set its `reference_set_uri` names, which is external terminology.
  `constraints.CodePhraseOf` returns the code phrase constraint a `CodePhrase` or a `CodeReference` carries, so a consumer with no use for the reference set reads the two alike: the validator reads a `C_CODE_REFERENCE` as its code phrase constraint.
  `CodeReference.ReferenceSetTerminology` reads the terminology id its `reference_set_uri` names, where that is a `terminology:` URI: `terminology:<name>[/<version>]`, with any query or fragment, names `<name>`, or `<name>(<version>)` where it gives a version, and a URI of another scheme, or one that names no terminology or does not take that form, names none.
  `constraints.CodePhraseWithTerminologyOf` reads the two alike too, but gives a `CodeReference` whose code phrase names no terminology the one its URI names; a terminology the code phrase names is kept.
  The example generator ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator)), the Web Template builder ([§ REQ-106](#req-106--webtemplate-json-export)) and the FLAT decode with the template ([wire.md § REQ-053](wire.md#req-053)) read a `C_CODE_REFERENCE` through `constraints.CodePhraseWithTerminologyOf`.
- `DvQuantity.Validate` accepts a `constraints.QuantityValue` `{Magnitude, Units, Precision}` triple.
  It **MUST** judge units by `Units` where that list is non-empty and by `PropertyUnits` only where it is empty, reporting a value whose units the list in force does not hold as a `CodeUnitUnknown`:
  - with entries listed, a value whose units no entry lists is refused and `PropertyUnits` is not consulted, so a listed unit the property does not admit is accepted;
  - with an empty `Units` list, a value whose units are not in a non-empty `PropertyUnits` is refused, as the specifications-CNF conformance schedule's case `CONT-DV_QUANTITY-validate_property` rejects `mg` under the property `openehr::122` (length);
  - with both empty, any units are accepted.

  The compile step **MUST** fill `PropertyUnits` when `Property` is an `openehr` code that the property-unit table of [reference-model.md § REQ-034](reference-model.md#openehr-terminology-vocabulary-req-034) (TERM `PropertyUnitData.xml`) lists, with that property's UCUM units, its primary units first and each part in source order, and **MUST** leave it empty for any other terminology or code.
  The table is closed: a UCUM code that measures the property but that the table does not list for it is a `CodeUnitUnknown`, such as `ml` under `openehr::129` (volume), where the table lists `mL`.
  With an empty `Units` list, `ExampleValue` returns magnitude 0 in the first entry of `PropertyUnits`, or `{0, "1"}` when that is empty too.
- `CDvOrdinal.Validate` accepts either an `int` (ordinal value) or a `constraints.OrdinalSymbol` `(value, symbol)` pair.
  An `OrdinalSymbol` **MUST** match a listed pair when the values and the code strings are equal and the terminologies are equal or either is empty, the rule `CodePhrase.Validate` applies to a terminology, so a listed symbol whose OPT names no terminology admits the one the instance carries.
  An empty `Values` list constrains the type only: `Validate` **MUST** accept any `int` or `OrdinalSymbol` against it, because an OPT that names `DV_ORDINAL` and lists no pairs leaves the value open, and it **MUST** refuse any other Go type with `CodeWrongType`.

A value whose Go type is not in the accepted set returns a single `CodeWrongType` violation; this is a contract failure on the caller side, not a constraint failure.

A constraint that carries a malformed value of its own **MUST** report a `CodeInvalidValue` on every value, as an unparseable `CString` pattern does.
Its malformed values are:

- a date, time, date-time or duration pattern the bullets above call malformed;
- on a constraint built in code, a `TimezoneValidity` outside the three values or mandatory on a `CDate`, and a `TemporalRange` bound the ISO 8601 parse of its type refuses on a side that is not unbounded, which the compile step never leaves on a constraint ([§ REQ-111](#req-111--public-compiled-template-bridge) *What the compile step tolerates*).

### Violation taxonomy

Every `Violation` carries a typed `ViolationCode`.
The closed set is:

| Code | Triggered by |
|---|---|
| `CodeOutOfRange` | numeric value outside a `NumericRange`, or temporal value outside a `TemporalRange` |
| `CodePatternMismatch` | string fails a regex / pattern (matched against the whole string) |
| `CodeNotInList` | value is not a member of a closed list (strings, codes, ordinals, etc.) |
| `CodeWrongType` | input Go type cannot be coerced to the constraint's expected type |
| `CodeUnitUnknown` | DV_QUANTITY units string is not in the enumerated allowed list, or, with no list, not among the units of the constraint's property |
| `CodeInvalidValue` | constraint or input is malformed (e.g. unparseable regex in the OPT, malformed date string) |

`Violation.Detail` carries a human-readable message; a consumer that builds structured diagnostics dispatches on `Code`, not on `Detail`.
The class of each `Violation` field, and where the value goes, are in [§ REQ-168](#req-168--value-free-validation-diagnostics) and not restated here.

### Numeric range

`NumericRange` is the inclusive / exclusive interval shape used by `CInteger`, `CReal`, and the `Magnitude` and `Precision` of each `DvQuantity` entry (`QuantityUnit`):

- `Lower` / `Upper` (float64; lossless for INTEGER up to 2^53)
- `LowerInclusive` / `UpperInclusive` (a flag the OPT omits is read by [wire.md § REQ-169](wire.md#req-169--interval-boundary-flag-defaults): true on a bounded side, false on an unbounded one; a caller building a range sets the flags itself, since the struct's zero value is *exclusive* on both sides)
- `LowerUnbounded` / `UpperUnbounded` (when true, the corresponding bound is ignored)

The zero-value `NumericRange{}` (no fields set) is treated as "any value accepted" by `Contains` and `IsBounded` — a no-op constraint.
AOM 1.4 also gives `C_DURATION` eight per-component allowed-flags (`years_allowed`, `months_allowed`, …, `fractional_seconds_allowed`).
The ITS-XML `C_DURATION` an OPT follows carries none of them: its `pattern` string encodes them, and a `CDuration` carries that pattern (§ *Validate contract*).

### Out of scope (this REQ)

- **External terminology lookup** — [§ REQ-111](#req-111--public-compiled-template-bridge) *Term definitions and bindings* surfaces the bindings an OPT carries; `Validate` calls no remote terminology service.
- **AOM 2 `tuple_constraint`** — not used by ADL 1.4.
- **UCUM dimension analysis** — the specifications-CNF conformance schedule's case `CONT-DV_QUANTITY-validate_property` notes that "all units for the `property` are allowed" and that the validation is to look "into UCUM for all the possible units of measure" of that physical property.
  That is the fuller reading: any UCUM code of the property's dimension.
  The SDK checks the closed TERM property-unit table instead, with exact matching (§ *Validate contract*).

### Example value emission (REQ-107 hook)

Every `PrimitiveConstraint` additionally exposes `ExampleValue() any` — a minimal-valid Go example value in the shape `Validate` accepts.
For bounded constraints (closed lists, bounded ranges, enumerated units, a property's units) that admit a value, `Validate(c.ExampleValue())` **MUST** return an empty `Violation` slice.
An unbounded primitive, and a temporal constraint for which no candidate is accepted, **SHOULD** give a fixed sentinel of its type (e.g. `"example"`, `int64(0)`; for the temporal types `"2020-01-01"`, `"12:00:00"`, `"2020-01-01T12:00:00Z"` and `"P0D"`), so the example is a well-formed value of its type that stays the same from call to call.
A constraint that admits no value has no example `Validate` accepts and is outside the first rule: a closed `CString` list whose `Pattern` matches no entry gives its first entry (§ *Validate contract*), and a temporal constraint that no value satisfies gives its sentinel.
A temporal constraint is bounded by its `Pattern`, its `Range` and, for a time or a date-time, its `TimezoneValidity`.
Its `ExampleValue` is the first value `Validate` accepts of these, in this order, as each method's documentation says, and else the sentinel:

- its sentinel, written to the pattern;
- each bound of the range, written to the pattern and moved inside the range;
- the value halfway between the bounds, with its fraction of a second, where the pattern allows seconds; for a time, a range open on one side reads as ending one second past its bound, within the day, so the range above `23:59:59` gives `"23:59:59.5"`;
- for a time or a date-time, each bound that carries a zone, written with its clock and its zone an hour across, so that it names the same instant and `Validate` compares it with the bound as an instant.

Two cases are exceptions to the rule for bounded constraints, where the example is the sentinel `Validate` refuses although a valid value exists:

- a pattern that writes a field after the first as a literal number, beside a range, which can move a bound written to it out of the range (`CDate{Pattern: "yyyy-06-dd"}` with the range `<=2004-05-20` gives `"2020-01-01"`, though `"2003-06-01"` is valid).
  The ITS-XML pattern grammars admit no literal number, so only an OPT that breaks that schema, or a constraint built in code, reaches it;
- a range that only a value its bounds do not order satisfies, a time with a zone where the bounds carry none, or a date-time with no zone where they carry one, since a candidate takes or drops a zone only where the pattern or the `TimezoneValidity` asks it to.
  `CTime` with the range `<00:00:00` gives `"12:00:00"`, though `"00:00:00Z"` is valid.
  An OPT reaches it where neither the pattern nor a `timezone_validity` constrains the zone, through an excluded bound that reaches the end of the day, such as `23:59`, or lies at its start, for a time, or through two bounds closer together than the last field the pattern writes.

The factory is the leaf primitive of the REQ-107 template-driven instance generator and stays on the sealed interface so the closed type-switch remains the only entry point for new primitive shapes, with no reflection ([idiom.md § Generics and reflection](idiom.md#generics-and-reflection)).
See § REQ-107 for the generator contract.

### Slot assertions

The SDK **MUST** parse the ADL 1.4 `ARCHETYPE_SLOT` include and exclude assertion subset that slot-fit checks in validators and in instance synthesis need: the `archetype_id matches {regex}` form, written either:

- as plain text in the OPT XML (`archetype_id matches {/openEHR-EHR-OBSERVATION\.body_weight\..*/}`);
- or as the OPT XML expression tree in which operator `2007` (`matches`) binds `archetype_id/value` to a `C_STRING` `<pattern>` (the Ocean Template Designer shape).

An assertion that does not parse is one the structured matcher ignores; when every include fails to parse, the slot widens to the RM-type-prefix rule below, which `SlotRules.IncludesDroppedUnparsed` reports.

- `constraints.SlotAssertion` carries a compiled Go `regexp` and exposes `MatchesArchetypeID(string) bool`.
- `constraints.SlotRules` holds a slot's includes and excludes.
  `AllowsArchetypeID` applies the excludes first, then requires a match against at least one include when includes are present; when no include was parsed it **MUST** fall back to the RM-type-prefix rule (`openEHR-EHR-<rmType>.`).
  A catch-all exclude (`.*`) is **ignored when includes are present**: template editors generate it as the complement of a closed includes list, so applying it literally would reject the slot's own includes.
- The compile step builds the same rules from the includes and excludes of the model's `ARCHETYPE_SLOT`, and `CompiledNode` exposes `AllowsArchetypeID` and `ExampleSlotFillArchetypeID` for the validators and the instance generator ([§ REQ-111](#req-111--public-compiled-template-bridge)).

- **Probes:** PROBE-024 (primitive constraint validation against fixture inputs)

---

## REQ-102 — Composition validation

The SDK **MUST** ship a `ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result` entry point that walks a parsed RM `Composition` against a compiled OPT and returns a `Result` aggregating every issue found in a single pass.

### Contract

- **Pure function.** No I/O and no goroutines, and the reflection rule of [idiom.md § Generics and reflection](idiom.md#generics-and-reflection) applies unchanged.
  Stateless — concurrent callers share `c` safely.
- **Collect-all, not fail-fast.** Validators emit one `Issue` per failing clause; the walk completes regardless of how many issues fire.
  UIs and CI consumers need the full list.
- **Result shape.** `Result` and `Issue` are declared in [§ REQ-171 *Finding identity*](#finding-identity) and [*Coverage*](#coverage); `Issue.Severity` is:
  ```go
  type Severity int
  const (
      Error   Severity = iota
      Warning              // advisory; does not flip Result.OK ([ValidateAQL] emits these)
  )
  ```

### Roots beyond COMPOSITION

The walker is value-source-generic: the compiled OPT drives the traversal, and the root is read attribute by attribute through the generated access of [§ REQ-171](#req-171--validation-engine) *Structural access*.
The SDK **MUST** expose it for every archetypeable RM root, not only `COMPOSITION`: the demographic PARTY hierarchy and the EHR-IM container roots, so a demographic or directory OPT validates through the same machinery as a clinical template.

```go
// Generic entry — root is any RM LOCATABLE concrete the walker recognises.
func Validate(root any, c *templatecompile.Compiled, opts ...ValidateOption) Result

// Typed convenience wrappers (delegate to Validate; they take no option):
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result
func ValidateDemographic(party rm.Party, c *templatecompile.Compiled) Result        // PERSON/ORGANISATION/GROUP/AGENT/ROLE
func ValidateFolder(folder *rm.Folder, c *templatecompile.Compiled) Result
func ValidateEHRStatus(status *rm.EHRStatus, c *templatecompile.Compiled) Result
```

Every entry point includes the RM rules (§ *RM floor*).
`ValidateComposition` keeps its `nil_composition` guard, then delegates.
A nil or typed-nil root yields `nil_root` (or the wrapper's `nil_party`, `nil_folder` or `nil_ehr_status`); a root whose concrete RM type does not match the OPT root draws `rm_type_mismatch` at `/`, never a silent pass.

The covered roots are the demographic PARTY hierarchy, `PERSON`, `ORGANISATION`, `GROUP`, `AGENT` and `ROLE`, with the archetypeable parts walked in place or as roots (`ADDRESS`, `CONTACT`, `PARTY_IDENTITY`, `PARTY_RELATIONSHIP`, `CAPABILITY`), and the EHR-IM roots `FOLDER` (directory trees, recursing `folders`) and `EHR_STATUS`.
Two limits remain: a round-tripped `DV_INTERVAL` with no bound present carries no `T` to check, so it matches any `DV_INTERVAL<T>` node (*Structural — RM type match* below); and reference-typed attributes (`PARTY.roles`, `FOLDER.items` → `OBJECT_REF`/`PARTY_REF`) are addressable for existence and cardinality, but their targets are not descended.

### Trust model

The validator treats the **compiled OPT as authoritative for structure** and the **composition as the instance under test**.
The walk that applies the template is [§ REQ-171 *One walk*](#one-walk).
Composition-supplied predicates never form lookup keys, so missing nodes are reported instead of silently bypassed.

### Validation dimensions

#### Structural — root archetype match

comp.ArchetypeNodeID matches the OPT root's archetype id.

#### Structural — required attributes (composition root + recursive)

- RM-mandatory attrs at the root (Category, Composer, Language, Territory);
- template-driven existence checks at every OPT node whose attribute existence lower bound is ≥ 1;
- a multi-valued attribute fails that check as [*Multi-valued attributes*](#multi-valued-attributes) below says.

#### Structural — prohibited attributes

Which OPT children are *allowed* and which attributes the OPT *prohibits* is defined in [§ REQ-111 *Template terms*](#template-terms).
A prohibited attribute that is present (a single-valued attribute that holds a value, a multi-valued one that holds at least one item) is reported as `prohibited` at the attribute's path, and the template walk does not walk into it.
Under a single-valued attribute, a child that is not allowed binds no value: the value binds among the other children by the alternatives rule below, or fails as `rm_type_mismatch` or `alternative_mismatch`.

#### Structural — child cardinality

For each C_MULTIPLE_ATTRIBUTE of an object the instance carries, the items bound to each counted child are counted against its `Occurrences()`, and the list's items against the parsed `CompiledAttribute.ChildMultiplicity()` interval, in the cases [*Multi-valued attributes*](#multi-valued-attributes) below gives.

#### Structural — alternatives (C_SINGLE_ATTRIBUTE)

The value is checked against the attribute's child constraints.
The children are alternatives, only one of which the value needs to match (ADL 1.4 *cADL*, *Single-valued Attributes*; AOM 1.4 `C_SINGLE_ATTRIBUTE`, whose children are alternative object constraints).
Binding prefers the most specific child, in three tiers:

1. a child whose `RMTypeName` equals the value's RM type (for a round-tripped bare `DV_INTERVAL`, the parameterised type its bounds give; one with no bound present has no parameter, so only a bare `DV_INTERVAL` child names its type, and a `DV_INTERVAL<T>` child admits it in the third tier, as § *Roots beyond COMPOSITION* says);
2. then a child naming the AOM 1.4 primitive short name whose canonical DV wrapper the value is;
3. then a child the type only conforms to by BMM descent, or an untyped child.

Within the first tier that admits the value, validators **MUST** bind it to a child under whose constraints it validates, where one does, and otherwise to the first child of that tier, reporting the findings it draws there.
A value validates under a child when walking it under that child reports no finding that makes `Result.OK` false and, at a slot child, the slot's assertions admit its archetype id; the walk tries the children in OPT order and binds to the first under which the value validates.
This trial is the validators' alone: the builder ([§ REQ-101](#req-101--generic-opt-driven-composition-builder) *Type enforcement*) and the generator ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator) *What a visit writes*) bind by the tiers alone, to the first child of the first tier that admits the type.
When no tier admits the value: exactly one child → `rm_type_mismatch`; two or more children → `alternative_mismatch`.

#### Structural — RM type match

The RM instance's concrete type conforms to the OPT child's `RMTypeName`: equal to it, or a descendant of it in the pinned RM BMM's class hierarchy, whether that ancestor is abstract or concrete (a `DV_CODED_TEXT` satisfies `DV_TEXT`, a `PARTY_REF` satisfies `OBJECT_REF`).
A `DV_INTERVAL<T>` child matches a typed instantiation `DV_INTERVAL<U>` whose parameter `U` is `T` or descends from it, so `DV_INTERVAL<DV_COUNT>` matches a `DV_INTERVAL<DV_QUANTIFIED>` child, and a round-tripped interval that decodes as the bare `DV_INTERVAL` by the runtime types of its bounds, each present bound having to be a `T` or of a class that descends from it.
An AOM 1.4 primitive short name admits its canonical DV wrapper, the type the [REQ-107](#req-107--template-driven-rm-instance-example-generator) generator builds for it (`DURATION` → `DV_DURATION`, `DATE` → `DV_DATE`, `TIME` → `DV_TIME`, `DATE_TIME` → `DV_DATE_TIME`, `BOOLEAN` → `DV_BOOLEAN`, `INTEGER` → `DV_COUNT`).
Single-child attributes surface failures as `rm_type_mismatch` at the attribute path.

#### Identity — archetype / node id pinning

LOCATABLE.archetype_node_id is checked against the matched OPT child's `ArchetypeID()` (for archetype roots) or `NodeID()` (for inner at-codes).

#### Structural — binding items (C_MULTIPLE_ATTRIBUTE)

A LOCATABLE item binds among the non-slot children whose `ArchetypeID()` (archetype root) or `NodeID()` equals its `archetype_node_id`, else to the first slot whose assertions admit that id.
Where two or more non-slot children carry the id, as when a template reuses one archetype under several template-level names ([§ REQ-111 § Node names and name predicates](#node-names-and-name-predicates)), the item's `name` value ranks them: first those whose pinned name equals it; failing any, those that pin no name; failing any, all of them.
The item binds among the children of the first rank that holds one.
Among the children of one rank, validators **MUST** bind the item to a child that has room and under whose constraints the item validates, as *Structural — alternatives* reads it, where one does; failing that, to the first of them that has room; failing that, to the first of them, whose occurrences or name constraint then reports the mismatch.
The walk tries them in OPT order and binds to the first that has room and under which the item validates.
A child has room while the items of the same list bound to it before, in item order, number fewer than its occurrences upper bound; one that states no upper bound always has room.
The item's `name` decides the binding only and never reaches a path or a message ([REQ-168](#req-168--value-free-validation-diagnostics)).
Items that are not a LOCATABLE bind by RM type (§ *Items that are not a LOCATABLE*).

#### Primitive constraints

REQ-103 `PrimitiveConstraint.Validate` runs at every primitive leaf the OPT declares; bound to the RM value found by the structural walk.
A part of a value that the RM rules report as absent or empty is theirs to report, not the constraint's, as [§ REQ-171 *One finding per defect*](#one-finding-per-defect) says.
The parts are:

- the `code_string` of a CODE_PHRASE, a DV_CODED_TEXT's defining code included, whose terminology the constraint still judges;
- the `units` of a DV_QUANTITY, which leaves the constraint nothing to judge, its ranges being per unit;
- the `value` of the DV_DATE, DV_TIME, DV_DATE_TIME or DV_DURATION an AOM 1.4 short name binds;
- and the code string of a DV_ORDINAL's `symbol`, when the validator passes the `value` alone for the constraint to judge.

The constraint **MUST** still judge a CODE_PHRASE's terminology; a DV_ORDINAL's `value`, which the validator passes alone when the RM rules report the symbol's code string; a part the RM rules let be blank, such as a DV_SCALE symbol's `code_string`; and a part the RM does not require, such as an optional String, which it judges as any value.
At a DV_ORDINAL bound to a `C_DV_ORDINAL` whose symbol's defining code carries a code string, a symbol that does not match its value **MUST** be reported: the validator passes the `value` and that code together as one `constraints.OrdinalSymbol`; a code that names no terminology matches a listed pair under § REQ-103's rule, and the RM rules report the missing terminology.
A `CONSTRAINT_REF` leaf constrains nothing the walker can check locally, so the walker still checks the RM-mandatory attributes of the value it binds there (a CODE_PHRASE's `terminology_id` and `code_string`), as at a node the OPT leaves unconstrained.

#### Slot fit — assertion grammar

§ REQ-103 *Slot assertions*: `CompiledNode.AllowsArchetypeID` (includes / excludes with RM-type-prefix fallback when no includes parsed).

#### Extra RM nodes not declared in OPT

Not flagged.

#### Terminology binding value-set

Not checked: it needs a live external terminology; [§ REQ-111](#req-111--public-compiled-template-bridge) *Term definitions and bindings* surfaces the bindings the OPT carries.

### Issue codes

`Issue.Code` is a stable programmatic identifier; the closed set is:

| Code | Triggered by |
|---|---|
| `required` | a required attribute (OPT-declared or RM-mandatory) is absent / zero-valued, or an item of a list the RM rules reach holds no RM value (REQ-112 *Driver*; a nil item the template refuses draws the template's finding instead, § *Items that are not a LOCATABLE*) |
| `cardinality` | a multi-valued attribute's child count violates the OPT-declared cardinality / occurrences interval |
| `prohibited` | an attribute the OPT prohibits (existence upper bound 0, or at least one OPT child and none allowed) is present (*Structural — prohibited attributes* above) |
| `alternative_mismatch` | no child of a C_SINGLE_ATTRIBUTE with **two or more** alternatives matches the RM value, or no child of a C_MULTIPLE_ATTRIBUTE with two or more children admits an item that is not a LOCATABLE |
| `rm_type_mismatch` | the RM instance's concrete type does not conform to the OPT child's declared `RMTypeName` (*RM type match* above), including single-child C_SINGLE_ATTRIBUTE type constraints and an item that is not a LOCATABLE that a single-child C_MULTIPLE_ATTRIBUTE does not admit |
| `archetype_id_mismatch` | LOCATABLE.archetype_node_id does not equal the OPT-pinned archetype id at the matched node |
| `node_id_mismatch` | LOCATABLE.archetype_node_id does not equal the OPT-pinned at-code at the matched node |
| `primitive_*` | a REQ-103 primitive `Violation.Code` (`out_of_range`, `pattern_mismatch`, `not_in_list`, `wrong_type`, `unit_unknown`, `invalid_value`) at a leaf |
| `slot_fill` | a LOCATABLE under a slot-constrained attribute whose `archetype_node_id` satisfies no OPT child or parsed slot assertion; slots fall back to the RM-type-prefix rule only when no include assertions were parsed. An item that is not a LOCATABLE is never reported this way, and a nil item only as § *Items that are not a LOCATABLE* says |
| `rm_invariant` | a REQ-112 RM-floor invariant that carries no rule-specific code (the default for the floor's per-RM-type checks) |
| `mappings_valid` | a present-but-empty `mappings` on DV_TEXT / DV_CODED_TEXT (REQ-112 floor) |
| `term_mapping_match` | a TERM_MAPPING `match` outside `{'>', '=', '<', '?'}` (REQ-112 floor) |
| `lower_included_valid` / `upper_included_valid` | an interval side marked both unbounded and included (REQ-112 floor, *Interval flags*) |
| `is_archetype_root` | an absent or `null` `archetype_details` on an archetype-root node (REQ-112 floor, *Archetype roots*) |
| `archetype_root_id` | an `archetype_details.archetype_id.value` that differs from `archetype_node_id` on an archetype-root node (REQ-112 floor, *Archetype roots*) |
| `rm_version_valid` | an empty `ARCHETYPED.rm_version`, and one the document leaves out or sends as `null` on a value validated with no report of missing members; with the report it is `required` instead ([§ *Value-typed mandatory presence*](#value-typed-mandatory-presence)) |
| `code_not_in_value_set` | a coded attribute outside the openEHR terminology group or code set its RM invariant names, reported at the CODE_PHRASE the invariant reads (REQ-112 floor, *Coded invariants*) |
| `max_depth` | the walk hit its recursion-depth limit ([§ REQ-171 *Cycles and limits*](#cycles-and-limits)) |
| `max_nodes` | the walk hit its per-call visit limit and stopped ([§ REQ-171 *Cycles and limits*](#cycles-and-limits)) |
| `cyclic_value` | a value that contains itself, reported where it recurs ([§ REQ-171 *Cycles and limits*](#cycles-and-limits)) |
| `invalid_shape` | input that does not decode into the root handed to `ValidateJSON`, `ValidateXML`, their `Validator` methods or the deprecated `ValidateRMEHRStatusBytes`: malformed canonical JSON or XML, or a JSON root that is not an object (REQ-112 floor, *Value-typed mandatory presence*) |
| `rm_type_unknown` | a Go value handed to `ValidateRM` whose type is no RM type the type registry knows (REQ-112 floor) |
| `nil_root` / `nil_folder` / `nil_ehr_status` / `nil_ehr_access` / `nil_party` | global guards: a nil argument to `ValidateRM` or a typed RM-floor wrapper (REQ-112 floor) |
| `nil_composition` / `nil_template` | global guards — caller supplied a nil argument |

### Multi-valued attributes

Existence and child-count cardinality are **independent constraints** on a list: existence says whether the container exists, and cardinality how many members it holds.
On an attribute the OPT declares multi-valued, of an object the instance carries, validators **MUST** report exactly the template findings this table gives for the list's existence, its own cardinality and the occurrences of its counted children:

| The list | Existence, rule `existence`, at the list's path | Its cardinality, code `cardinality`, rule `cardinality`, at the list's path | Each counted child, code `cardinality`, rule `occurrences` |
|---|---|---|---|
| is absent, and required by its existence | `required` | none | none |
| is absent, and not required by its existence | none | none | where it fails |
| is present and empty, and required by its existence | `required`, where the template asks for a member | where it fails | where it fails |
| is present and empty, and not required by its existence | none | none | where it fails |
| holds an item, on an attribute the template does not prohibit | none | where it fails | where it fails |
| holds an item, on an attribute the template prohibits | `prohibited`, and the list is not entered | none | none |

The table's terms:

- a list is absent or present as [§ REQ-171 *Structural access*](#structural-access) reads presence;
- a list is *required by its existence* where its existence lower bound is bounded and at least 1;
- where the template *asks for a member* is defined in *An existence lower bound alone asks for no member* below, and which children are *counted* in *A mandatory child under an optional list* below;
- a count *fails* where it lies outside its interval: the list's items outside its cardinality, or the items bound to a child outside the child's occurrences, so a list with no item fails each counted child whose occurrences lower bound is bounded and at least 1.

So a list that is present and empty, with existence and cardinality lower bounds of 1, draws both `required` and `cardinality` at its path, as collect-all makes natural; a consumer that de-duplicates for display can treat the pair as one failure at that path.
A list required by its existence that the instance does not carry draws `required` alone, whatever its counts ask: there is no container to count.
Which layer reports a list both layers find absent or empty is in [§ REQ-171 *One finding per defect*](#one-finding-per-defect).

#### An existence lower bound alone asks for no member

On a multi-valued attribute, existence says whether the container exists and cardinality how many members it holds (ADL 1.4, *Attribute constraints*, *Existence*).
So a container with no member meets an existence lower bound of 1 where the counts allow none ([§ REQ-111 *Template terms*](#template-terms)).
A list the instance does not carry is no container, so it fails that existence whatever its counts ask.
Canonical JSON can carry such a list with no member: it writes `ITEM_LIST.items`, `ITEM_TREE.items` and `ITEM_TABLE.rows` present and empty ([reference-model.md § Property → Go field](reference-model.md#property--go-field)).
Two readings cannot carry it: canonical XML writes no element for an empty list ([wire.md § REQ-056](wire.md#req-056)), and a FLAT or STRUCTURED decode without the template has no key for a structure with no member ([wire.md § REQ-053](wire.md#req-053)), so a document read from either with such a list holding no member draws `required` there.
A FLAT or STRUCTURED decode given the template rebuilds a required structure with no member with its list present and empty where the generator writes it so ([wire.md § REQ-053](wire.md#req-053)).
The template *asks for a member* where its cardinality lower bound, or the occurrences lower bound of a child it allows, is bounded and at least 1, or where the list is one the floor holds non-empty once present.
A child that states no occurrences, or occurrences with an unbounded lower bound, asks for none here, although the generator builds a member of it ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator) *What a visit writes*).
The lists the floor holds non-empty once present are those [§ REQ-112 *Non-empty lists*](#non-empty-lists) evaluates: `COMPOSITION.content`, `EVENT_CONTEXT.participations`, `INSTRUCTION.activities`, `SECTION.items` and `ATTESTATION.items`, and `HISTORY.events` while the HISTORY has no `summary`.
Those rules refuse such a list that is present and holds no item, and a HISTORY with neither an event nor a summary, so the container its existence asks for cannot exist without a member.
So such a list that its existence requires draws `required` both when it is absent and when it is present and empty.
The RM gives other lists the same rule, `ENTRY.other_participations`, `ACTOR.roles` and `PARTY.contacts` among them; the floor does not evaluate it there, so their existence asks for no member.
`LOCATABLE.links` is one of them that the floor does evaluate, and it reports a `links` that is present and holds no item ([§ REQ-112 *LOCATABLE links and EVENT_CONTEXT location*](#locatable-links-and-event_context-location)), but it is not among the lists the floor holds non-empty for the template's sake, so the existence of `links` asks for no member either.
So an `ITEM_LIST` or `ITEM_TREE` whose `items` carry existence `1..1`, cardinality `0..*` and no allowed child validates with its `items` present and empty, and draws `required` at them where they are left out.

#### A mandatory child under an optional list

The *counted* children of a list are the allowed children, not slots, of an attribute the OPT declares multi-valued, *allowed* as [§ REQ-111 *Template terms*](#template-terms) defines it, with two exceptions:

- a list whose items are primitive values, such as a DV_MULTIMEDIA's `data`, has items the walk counts but binds to no child, so its children are counted neither when it holds items nor when it holds none;
- a list the template prohibits (*Structural — prohibited attributes* above) leaves its children nothing to count: holding an item, it draws `prohibited` alone and is not entered, and holding none, absent or present and empty, it draws nothing from its children.

So an instance that leaves a prohibited list out validates, unless its existence lower bound is at least 1, which asks for the container (*An existence lower bound alone asks for no member*).
A counted child is counted whether the list holds items, is present and empty, or is absent, except on an absent list the template requires by its existence, which fails existence alone (the table's first row).
The children of a list the OPT writes as a single-valued attribute, as some OPTs write a `COMPOSITION`'s `content` under a `C_SINGLE_ATTRIBUTE`, are alternatives that attribute's existence decides ([§ REQ-111 *Template terms*](#template-terms)), so an absent such list draws nothing from them.
An absent list holds no item, so a counted child's occurrences finding there is written at the path § *Issue paths* writes for a node the OPT requires and the instance lacks: the child's own predicate below the list's path, or the list's path for a child that declares no id.
That a child whose occurrences lower bound is at least 1 asks for its container is the reading [§ REQ-111 *Template terms*](#template-terms) chooses, so that a list `Generate` leaves out under `Required` that the template leaves optional by its existence is one validators accept as absent.
On a list the template leaves optional by its existence, the list's own `cardinality` is not reported when the list is absent, since a cardinality lower bound does not ask for the container ([§ REQ-111 *Template terms*](#template-terms)).
Nor is it reported when the list is present and empty, since canonical XML writes no element for an empty list ([wire.md § REQ-056](wire.md#req-056)) and canonical JSON leaves out a present empty optional list other than those [reference-model.md § Property → Go field](reference-model.md#property--go-field) names ([wire.md § REQ-052](wire.md#req-052), *An absent mandatory member*).
So an absent and an empty optional list draw the same template findings, as they do on existence.

#### Open multi-valued attributes

When a C_MULTIPLE_ATTRIBUTE declares no `<children>` (the OPT pinned only existence / cardinality, not membership), validators **MUST** accept any RM item under that attribute without firing `slot_fill`.
The constraint surface is the attribute itself; the items inside are unconstrained.
This admits the OPT idiom "items here, any shape allowed" — e.g. a SECTION whose /items is open to any archetype-root content.

#### Items that are not a LOCATABLE

An item of a C_MULTIPLE_ATTRIBUTE that is not a LOCATABLE (a PARTICIPATION under `participations` or `other_participations`, a TERM_MAPPING, a REFERENCE_RANGE, a DV_IDENTIFIER, a PARTY_REF) carries no `archetype_node_id`, so it cannot be matched by id.
Validators **MUST**:

- bind such an item to a non-slot child whose `RMTypeName` admits the item's RM type, by the same conformance rule and the same three binding tiers a C_SINGLE_ATTRIBUTE uses, choosing among the children of the first tier that admits it as *Structural — binding items* chooses among those of one rank;
- never report it as `slot_fill`;
- and report one that no child admits at its own path, as `rm_type_mismatch` if the attribute has exactly one child and as `alternative_mismatch` if it has two or more.

A bound item counts toward its child's occurrences like any other item.
A nil item (a nil element, or a nil pointer) under an attribute that declares children has no RM value to bind, so validators **MUST NOT** bind it to any child, an untyped one included.
It is reported as `slot_fill`, like a LOCATABLE whose `archetype_node_id` is empty, when it is a nil pointer to a LOCATABLE class or a nil element of an attribute whose declared item type is a LOCATABLE, and as `rm_type_mismatch` or `alternative_mismatch` by the rule above otherwise.
An attribute that declares no children admits a nil item like any other item (*Open multi-valued attributes* above), so the template reports nothing for it and the RM rules report it, as [§ REQ-112 *Driver*](#driver) says; a nil item the template refuses is reported by the rule above alone ([§ REQ-171](#req-171--validation-engine) *One walk*).

### Issue paths

`Issue.Path` is an openEHR path from the validated root: RM attribute names joined by `/`, with a predicate on each item of a multi-valued attribute and none on a single-valued one.
The template-driven validators (this REQ) and the floor ([REQ-112](#req-112--template-less-reference-model-validation-floor)) **MUST** build it by one rule, so one node has one path whichever validator reports it.
An item's predicate is:

- `[atNNNN]`, its `archetype_node_id` (an archetype id on an archetype root), when no other item of the same attribute carries that id;
- `[atNNNN, 'name']` when another item shares the id and the OPT node the item binds to pins a name ([§ REQ-111 § Node names and name predicates](#node-names-and-name-predicates)) that tells it apart, the name single-quoted with `'` and `\` escaped by a backslash;
- `[n]`, the item's 1-based position in the attribute, in every other case: an item with no `archetype_node_id`, or one whose id is shared and whose OPT node pins no telling name.

An entry of a hash attribute ([§ REQ-171 *Structural access*](#structural-access)) **MUST** be written `[n]`, its 1-based position in ascending key order, never with its key: a key is instance data, and the quoted-key form ODIN shows in its examples (`["en"]`) would carry it into the path.

A node the OPT requires and the instance lacks is written with the OPT child's own predicate, since no item exists to number; a child that declares no id has none and is written at the attribute's path.
The floor binds no OPT node, so on its own it never writes a name predicate; inside a template-driven validation the RM rules run in the walk that binds the template ([§ REQ-171 *One walk*](#one-walk)), so a node they judge carries the path that walk wrote for it, its name predicate included.

A path carries no value from the instance, as [REQ-168](#req-168--value-free-validation-diagnostics) requires of every value-free field: a name predicate is always a name the OPT pins, never the instance's own `name`.
A path with no positional predicate is a valid AQL path; the AQL grammar has no positional predicate, so one with `[n]` is an openEHR path but not AQL.

### RM floor

Every entry point of this REQ, `ValidateComposition`, `Validate` and the typed wrappers of § *Roots beyond COMPOSITION*, **MUST** apply the [REQ-112](#req-112--template-less-reference-model-validation-floor) rules in the walk that applies the template and return their findings in the same `Result` ([ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md), [ADR 0027](../adr/0027-template-admission-before-rm-rules.md)): on every node the template admits, the nodes it does not constrain included, and on no content it refuses.
[§ REQ-171 *One walk*](#one-walk) orders the steps at each node and says which layer reports an attribute both layers find missing or empty, so that absence is one finding; an RM rule's own finding at the same path, such as a code outside its terminology group, is reported beside it.
No option leaves the RM rules out of these entry points; `ValidateRM` and its typed wrappers are the entries for the RM rules alone.

### Sentinels

The package **MUST** expose typed sentinels callers compare via `errors.Is` for programmatic dispatch.
Issues bridge to sentinels via `Issue.Err() error`:

| Sentinel | Triggered by |
|---|---|
| `ErrCardinality` | `cardinality` code |
| `ErrRequired` | `required` code |
| `ErrProhibited` | `prohibited` code |
| `ErrTypeMismatch` | `rm_type_mismatch`, `alternative_mismatch`, `archetype_id_mismatch`, `node_id_mismatch` |
| `ErrPrimitive` | any `primitive_*` code |
| `ErrSlotFill` | `slot_fill` code |
| `ErrAQLSyntax` | `aql_syntax`, `aql_empty` codes — the AQL lint surface (REQ-109) |

Caller pattern:

```go
for _, i := range r.Issues {
    if errors.Is(i.Err(), validation.ErrRequired) {
        // typed handling for missing required attributes
    }
}
```

Global guard codes (`nil_composition`, `nil_template`) return `nil` from `Issue.Err()` — they represent caller-side argument errors, not validation failures of a structurally-present composition.

### Out of scope (this REQ)

- **AQL lint** (`ValidateAQL`) — a separate entry point under REQ-109; it does not change the composition-validation surface.
- **Decoding wire bytes** — `ValidateComposition` takes a decoded value; the decode-aware entries `ValidateJSON` and `ValidateXML` are [§ REQ-112](#value-typed-mandatory-presence)'s.
- **External terminology lookup** — value-set membership against SNOMED CT / LOINC / external services.
  REQ-103 closed-code-list checking is the ceiling.
- **A coded text's `value` against its code's term text** — no RM invariant ties a DV_CODED_TEXT's `value` to the term text of its `defining_code`, so the template validator does not compare them; the generator writes the term text ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator)).
- **Cross-archetype slot-fill resolution** — no federated archetype repository; slot fit is local to the parsed slot assertions of § REQ-103, with RM-type-prefix fallback only when no include assertions were parsed.
- **Full ADL2 / AOM 2 validation semantics.**

- **Probes:** PROBE-025 (composition validation against fixture OPT + composition); PROBE-026 (missing required node, cardinality, alternative_mismatch, rm_type_mismatch, and primitive negative cases); PROBE-074 (template-driven validation of non-COMPOSITION roots: the issue-code multiset per (OPT, root) shape)

---

## REQ-107 — Template-driven RM instance example generator

The SDK **MUST** ship a template-authoritative RM instance synthesiser at `openehr/instance/`: given a compiled OPT, produce a conformant RM object graph whose structure and primitive leaves satisfy the same template-driven contract REQ-102 validates against.
The generator is the inverse of validation: the same compiled-OPT walk, in the opposite direction.

### Scope

The generator is the single skeleton-and-populate engine the composition builder (REQ-101), tests, examples, and data seeding all consume.
The root is whatever RM class the OPT's root `rm_type_name` declares: an archetype-root class (`COMPOSITION`, an ENTRY, a `PARTY`, …), a content or structure class (`SECTION`, `GENERIC_ENTRY`, `CLUSTER`, `ELEMENT`, an item structure, …), a `PARTY_RELATIONSHIP`, a data value or a `CODE_PHRASE`.
Output is **synthetic example data**: valid as § Contract below says, not clinically meaningful.

### Contract

Public entry point:

```go
package instance

type Policy int

const (
    Medium   Policy = iota // the default: required attributes and those with an allowed OPT child, a member of each optional archetype
    Required               // required attributes, a composition's context, a member of each entry the content names at its mandatory parts
    Large                  // every attribute the visit rule allows, a member of each optional archetype and of each use of one, two of each repeating node, a few optional RM attributes

    Minimal  = Required // alias
    Complete = Large    // alias
)

func ParsePolicy(name string) (Policy, error)

type Options struct {
    Policy      Policy
    Language    string                  // ISO 639-1; empty = Compiled.Language(), then "en"
    Territory   string                  // ISO 3166-1; required for a COMPOSITION root
    Composer    rm.PartyProxy           // required for a COMPOSITION root
    Now         time.Time               // clock for unconstrained date-times; zero = current time
    UIDSource   func() *rm.HierObjectID // uids and reference ids (nil = crypto/rand)
    ValueFill   ValueFill               // ExampleFill (default) or RandomFill
    ValueSource mrand.Source            // seeds RandomFill; nil or typed-nil = auto-seeded global
}

func UIDsFrom(src mrand.Source) func() *rm.HierObjectID // a UIDSource of version-4 UUIDs drawn from src; nil or typed-nil src = nil

func Generate(ctx context.Context, c *templatecompile.Compiled, opts Options) (any, error)
func AsComposition(v any) (*rm.Composition, error)
func AsObservation(v any) (*rm.Observation, error)
// … and AsEvaluation, AsInstruction, AsAction, AsAdminEntry, AsCluster, AsSection, AsGenericEntry, AsElement
```

#### Levels and names

`Medium` is the zero value, so `Options` with no `Policy` builds the medium level.
`Minimal` and `Complete` are aliases, the same values as `Required` and `Large`, so every rule here for `Required` and `Large` holds for them, and `Policy.String` **MUST** return `required` for `Required`, `medium` for `Medium` and `large` for `Large`, so an alias prints its target's name, and `unknown` for any other value.
`ParsePolicy` **MUST** map the names `required`, `minimal`, `medium`, `large` and `complete`, compared without regard to case, to those values, and return an error wrapping `ErrUnsupportedPolicy` for any other text, the empty string included.

#### Output

Under each policy above (`Large`, `Medium` and `Required`) and both `ValueFill` values, every root `Generate` returns, and so every skeleton `composition.NewSkeleton` returns and `composition.NewBuilder` seeds, **MUST**:

- satisfy the OPT's structural rules and REQ-103 primitive constraints, which the template validator (REQ-102) checks;
- pass the RM floor (`validation.ValidateRM`, [REQ-112](#req-112--template-less-reference-model-validation-floor));
- and satisfy every RM invariant the floor does not evaluate, such as ELEMENT `Inv_null_reason_valid` (a `null_reason` only on an ELEMENT with no `value`).

The cases § [Exceptions and known gaps](#exceptions-and-known-gaps) lists are the only ones outside this rule.
Two rows of the floor's catalogue need generator work: every ELEMENT carries exactly one of `value` and `null_flavour` (`Inv_null_flavour_indicated`), and no primitive leaf holds a placeholder its RM type rejects, so an ISO 8601 temporal `value` is always a valid date, time, date-time or duration (`Value_valid`).

#### Inputs

`Generate` **MUST** return an error wrapping `ErrUnsupportedPolicy`, and no root, for an `Options.Policy` that is none of `Large`, `Medium` and `Required`, before it builds anything, at every root type and either `ValueFill`, and, for a `COMPOSITION` root, `ErrComposerRequired` when `Options.Composer` is nil or holds a nil pointer, such as a nil `*rm.PartyIdentified`, and `ErrTerritoryRequired` when `Options.Territory` is empty.
For a `COMPOSITION` root it **MUST** write `Options.Composer` as the root's `composer`, as given, whatever the OPT constrains there.
An empty `Options.Language` **SHOULD** be read as `Compiled.Language()`, and as `en` when that is empty too, and a zero `Options.Now` as the current time, so that a caller who sets neither gets a root in the template's own language, stamped with the time it was built.

#### Visit rule

The words *allowed*, *prohibits*, *required* and a child that *asks for its container* are those of [§ REQ-111 *Template terms*](#template-terms).

##### Attributes no policy visits

Under every policy, `Generate` does not visit these attributes:

- an attribute that no serialization carries, a BMM function among them: the set `rminfo.IsNonStorableAttr` names and [§ REQ-112 *Driver*](#driver) lists;
- an attribute the compile marks as one the node's RM class does not have (`CompiledAttribute.NotInRM`, [§ REQ-111](#req-111--public-compiled-template-bridge) *Attributes the RM does not have*), such as `ISM_TRANSITION.name`, whatever existence the OPT gives it, since no instance can carry it and the template validator leaves it out and lists its path in its coverage ([§ REQ-171 *One walk*](#one-walk));
- a DV_MULTIMEDIA's `data`, whose octets bind to no OPT child ([§ REQ-102](#req-102--composition-validation) *A mandatory child under an optional list*) and which *RM defaults* below write instead;
- a locatable's `uid`, which *Identity* below governs;
- a `COMPOSITION` root's `composer`, which `Generate` writes from `Options.Composer` (*Inputs*).

Under every policy, `Generate` does not visit an attribute the generator has no writer for (§ [Exceptions and known gaps](#exceptions-and-known-gaps), *Known gap: attributes the generator cannot write*), unless both hold:

- it is required by more than its cardinality lower bound alone, which does not ask for the container;
- it is not a list that only its existence asks a member of (*What a visit writes*), which is left out (*Known gap: required lists canonical JSON cannot carry empty*).

Where the walk would visit one, `Generate` returns the error § [Errors](#errors) names for it, since a root without it would fail the template validator.
`Generate` does not visit an attribute the OPT prohibits unless an RM rule needs it (§ [Precedence](#precedence)), though it writes such a list present and empty where its existence asks for it (*What a visit writes*).

##### Required attributes

Wherever this requirement says the BMM marks an attribute mandatory, it reads the BMM on the class the walk builds for the attribute's node: a concrete descendant for a node that names an abstract class (*What a visit writes*), and the type *Generic and untyped types* gives an untyped one.
An attribute is *required* as [§ REQ-111 *Template terms*](#template-terms) defines it: a child that asks for its container makes the attribute that holds it required, the reading the validators share ([§ REQ-102](#req-102--composition-validation) *A mandatory child under an optional list*).

##### Attributes whose members would all be undescribed

Under `Large` and `Medium`, `Generate` does not visit an attribute whose members would all be undescribed or need undescribed content (§ [Slot fills](#slot-fills) *Undescribed content*), such as an `EVALUATION`'s `data` the OPT leaves without a child, where that attribute is not required or only its cardinality lower bound requires it.
A `COMPOSITION`'s `context` is the exception, which *RM defaults* writes under every policy.
A cardinality lower bound does not ask for the container, so this rule reads such a list as `Required` reads it, and validators accept it absent ([§ REQ-102](#req-102--composition-validation) *A mandatory child under an optional list*).
So each of these lists, at existence `0..1` and cardinality `1..*`, is left out under every policy:

- a `SECTION`'s `items` whose only child is an optional `ADMIN_ENTRY` the OPT names no archetype id for;
- a `COMPOSITION`'s `content` whose only child is an optional `SECTION` that requires such an entry;
- an `ITEM_TREE`'s `items` whose only children are slots, whose members would be slot fills, or that has no allowed child, whose members would be ELEMENTs built from its BMM type.

##### Item structures that would hold no item

Under every policy, `Generate` does not visit a single-valued attribute that is not required where the value a visit would write (*What a visit writes*) is an item structure that would hold no item: an `ITEM_TREE` or `ITEM_LIST` whose `items`, or an `ITEM_TABLE` whose `rows`, would get no member, because

- the structure's OPT node does not carry that attribute,
- the walk under the policy in force does not visit it, or
- the visit gives it no member (*What a visit writes*).

A list whose cardinality upper bound is less than the members its allowed children ask for is not one the visit gives no member: the walk visits it, and § [Errors](#errors) refuses the root.
The generator decides this by what the walk would write once the rules here and in *What a visit writes* have left children out, not by the OPT alone.
So an optional `protocol`, `state` or `other_context` whose `ITEM_TREE` holds no child, only slots, or only optional children that need undescribed content is left out, while one whose `ITEM_TREE` holds an optional `ELEMENT` is visited under `Large` and `Medium`, which build that `ELEMENT`.
An `ITEM_SINGLE` and a `CLUSTER` always hold an item, since the BMM marks their `item` and `items` mandatory.
That an optional structure with no item is left out is the reading this specification chooses, as such a structure carries no data the RM or the OPT asks for.
A structure the RM or the OPT requires, as the BMM requires `EVENT.data`, or as an OPT requires a `protocol` it gives an existence lower bound of 1, is written, with no item where the walk gives it none, and with its list present and empty where the OPT requires that list by its existence (*What a visit writes*).

##### What each policy visits

Of the attributes the rules above leave, `Generate` **MUST** visit those the table below gives for the policy in force and no other, with three exceptions:

- a `COMPOSITION`'s `context`, which it writes under every policy, built under the OPT's constraints as `Medium` does;
- the optional RM attributes `Large` writes where the OPT does not name them (*Optional RM attributes under `Large`*);
- the optional attributes *Leaf values*, *RM defaults* and § [Precedence](#precedence) 4 write under every policy: a `DV_IDENTIFIER`'s `issuer`, a `DV_PROPORTION`'s `precision` of 0 and a `DV_MULTIMEDIA`'s `uri`.

| Policy | `Generate` visits |
|---|---|
| `Large` | every attribute the compiled node carries |
| `Medium` | every attribute that is required or that has an allowed OPT child |
| `Required` | every attribute that is required, except one that only its cardinality lower bound requires, and none for its allowed OPT child alone unless that child leads to an entry (*Members of a list*) |

A cardinality lower bound counts the members of a container that exists and does not ask for the container, which its existence decides ([§ REQ-111 *Template terms*](#template-terms)).
So `Required` leaves out an optional list that only its cardinality asks a member of, such as an `ITEM_TREE`'s `items` at existence `0..1` and cardinality `1..*` whose children are all optional, and the bound counts the members of a list `Required` visits for another reason (*What a visit writes*).
The three follow the levels of the ITS-REST template example's `detail_level` ([`definition-validation.openapi.yaml`](../../resources/its-rest/definition-validation.openapi.yaml)):

- `Required` builds its `required` level, the data points the template makes mandatory, a composition's `context`, and a member of each entry the content names, at its mandatory parts (*Members of a list*);
- `Medium` its `medium` level, with a member of each optional archetype under an attribute, and of a further use of one only where that use holds another archetype (*Sibling identity*);
- `Large` the SDK's own `complete` level (§ [Primitive-leaf value fill](#primitive-leaf-value-fill), *The `Complete` level*):
  - a member of every optional archetype the template names that needs no undescribed content (§ [Slot fills](#slot-fills)), and of every use of one, named or not;
  - two members of each node that may occur more than once (*Members of a list*);
  - the optional RM attributes *Optional RM attributes under `Large`* lists.

An attribute the BMM marks mandatory that a compiled node does not carry, as under `templatecompile.WithoutImplicitAttributes`, is visited as if the node carried it with no OPT child.

#### What a visit writes

##### Single-valued attributes

A visited single-valued attribute takes the RM type its first allowed OPT child names, or, where that child names an abstract RM class or an AOM 1.4 primitive short name, the concrete type the generator builds for it:

- a concrete descendant of the abstract class, the *RM defaults* below where one applies;
- and for a short name its canonical DV wrapper (§ REQ-102 *RM type match*).

The value is built under the constraints of the child the binding tiers of [§ REQ-102](#req-102--composition-validation) *Structural — alternatives* bind a value of that type to: the first child, unless a later one names that type exactly or is the AOM 1.4 short name that type wraps.
That child is the first of its tier and the value built under it validates under it, so the validator, which tries the children of the tier in OPT order, binds what the generator built to the child whose constraints built it.
When the attribute has no allowed OPT child, it takes a value built from its BMM type.

##### Members of a list

A visited multi-valued attribute gets, in OPT order, these members of each allowed OPT child that is not a slot:

- under `Large`, `max(occurrences.lower, 2)` of a child whose occurrences lower bound is bounded and whose upper bound is above 1 or unbounded, and `max(occurrences.lower, 1)` of any other;
- under `Medium`, and in the skeleton the composition builder seeds a `Builder` with under `Large` ([§ REQ-101](#req-101--generic-opt-driven-composition-builder)), `max(occurrences.lower, 1)`;
- under `Large` and `Medium` alike, none of an optional child (occurrences lower bound 0) that needs undescribed content (§ [Slot fills](#slot-fills));
- under `Required`, `occurrences.lower`, so that an optional child gets none of its own there, except one that leads to an entry (below) and that the `Medium` rule below does not skip, which gets one;
- under every policy, one of a child the OPT gives no occurrences, or occurrences with an unbounded lower bound, which is not optional.

It gets no more members in all than its `cardinality.upper` when that is bounded.
Within that bound the room goes first to the members asked for by the children whose occurrences lower bound is bounded and at least 1, as many of each as that bound, which fit wherever § [Errors](#errors) does not refuse the list.
Any other member gets only the room they leave, whether it is of a child before or after them, of an optional child, or the second member `Large` gives a child whose lower bound is 1.
That room goes child by child in OPT order, so a child's second `Large` member takes it before a later child's first.
The members still stand in OPT order.
So `CLUSTER.items` at cardinality `0..1` over an optional `at0001` and a required `at0002` holds the `at0002` member alone, and under `Large`, at cardinality `0..2` over an optional `at0001` that may occur any number of times and a required `at0002`, one member of each.
Under `Medium`, an optional child (occurrences lower bound 0) gets no member of its own where both hold, though the top-up below can still build one:

- a sibling that gets a member of its own carries its identity (*Sibling identity*): a required sibling, before or after it, or an earlier optional sibling that gets one;
- every archetype it holds (*Sibling identity*) is one such a sibling holds.

An optional sibling that gets no member of its own, such as one that needs undescribed content, carries no identity and holds no archetype for this rule.
Under `Large`, an optional child whose identity a sibling carries gets its members as any other optional child does, so a `Large` root holds a member of every sibling of one identity that needs no undescribed content, whether the OPT pins a name on it or not, within the cardinality bound above.
The validator binds those members by room and by trial ([§ REQ-102](#req-102--composition-validation) *Structural — binding items*), each to the sibling whose constraints built it wherever that sibling comes first among those with room (§ *Exceptions and known gaps*).
An optional multi-valued attribute with no allowed OPT child gets no member, so nothing archetype-rooted appears without its `archetype_details`.

A child *leads to an entry* where it is not a slot, is not an optional child that needs undescribed content (§ [Slot fills](#slot-fills)), and is one of:

- an archetype root of an `ENTRY` class or of `GENERIC_ENTRY`;
- a `SECTION` whose `items` hold a child that leads to an entry.

Only a `COMPOSITION`'s `content` and a `SECTION`'s `items` hold such a child.
So under `Required` the content holds a member of each entry archetype `Medium` builds there, and of each `SECTION` on the way to one, and `Required`'s own rule builds only the mandatory parts inside each member.

##### Sibling identity

A child's identity is its archetype id when it is an archetype root and its `node_id` otherwise, and a child with neither has no identity and is never skipped.
A sibling other than a slot carries an identity when it is that sibling's archetype id or `node_id`, as [§ REQ-102](#req-102--composition-validation) *Structural — binding items* binds a LOCATABLE member.
A child holds the archetype it is a root of, and that of each root in its template subtree that `Large` and `Medium` build, which a search reaches:

- through the allowed children of each attribute but those no policy visits (*Attributes no policy visits*);
- through only the child a single-valued attribute binds its value to (*Single-valued attributes*);
- through no slot, and no optional child that needs undescribed content (§ [Slot fills](#slot-fills)).

So under `Medium` an optional child that is not an archetype root gets no member when an archetype root before it that gets one has its `node_id`, unless it holds an archetype that no sibling with a member of its own holds.
Optional archetype roots of different archetypes under one attribute each get their members under `Medium` and `Large` alike.
A further use of one archetype gets a member under `Medium` only where it holds another archetype, as four uses of one SECTION archetype that each hold other entries each do; only `Large` gives a member to every use (*Members of a list*).
A name the OPT pins on a child is not part of this identity, although the validator also tells siblings apart by it.

##### Top-up

When that leaves the attribute with fewer members than it needs (its cardinality lower bound, or one member when the BMM or its existence requires it), it is topped up to that count one member at a time, in OPT order, from a child that still has room.
A child has room while the members built of it, its own and the top-up's, number fewer than its occurrences upper bound, and always where it states no upper bound, the same room by which the validator binds an item ([§ REQ-102](#req-102--composition-validation) *Structural — binding items*).
A slot always has room, since the compiled template does not carry its occurrences (*Known gap: slot occurrences*).
Each member is one of the first of these sources the attribute has:

1. its first allowed child with room that needs no undescribed content (§ [Slot fills](#slot-fills)) and whose identity no earlier sibling with room carries, a child that gets no member of its own under `Large` or `Medium` included;
2. its first allowed child with room that is not a slot and that `Generate` does not refuse for what it holds itself (an object of an archetype-root class the OPT names no archetype id for, or a node with an attribute the generator has no writer for that the walk would visit, § [Errors](#errors));
3. a fill of its first slot;
4. its first allowed child with room that is not a slot, which `Generate` then refuses;
5. where it has no allowed child, a member built from its BMM type.

So `CLUSTER.items` at cardinality `3..*` over an `at0001` at occurrences `1..1` and an `at0002` at `0..*` holds one `at0001` member and two `at0002` members under every policy.
Only where no allowed child has room, as in an OPT whose cardinality lower bound exceeds what its children's occurrences allow (§ [Exceptions and known gaps](#exceptions-and-known-gaps)), is the member one of its first allowed child, past that child's occurrences.

##### Lists only their existence asks for

Only its existence asks for that member where:

- the BMM does not mark the attribute mandatory;
- neither its cardinality lower bound nor the occurrences lower bound of a child it allows is bounded and at least 1;
- and it is not a list that [§ REQ-102](#req-102--composition-validation) *An existence lower bound alone asks for no member* names as one the RM makes non-empty once present.

The top-up then takes only its first source (*Top-up*, item 1), and the attribute gets no member where it has no such child, since, as that section says, a present list with no member meets its existence: the list is still there ([§ REQ-111 *Template terms*](#template-terms)).
So a list is written present and empty where canonical JSON carries an empty list, as § Contract *Output* asks, and is left out otherwise (*Known gap: required lists canonical JSON cannot carry empty*), where:

- the walk gives it no member, this way or any other;
- its existence lower bound is bounded and at least 1;
- the walk visits it, or the OPT prohibits it by prohibiting every child of it.

Canonical JSON carries `ITEM_LIST.items`, `ITEM_TREE.items` and `ITEM_TABLE.rows` empty ([reference-model.md § Property → Go field](reference-model.md#property--go-field)).
A list the RM makes non-empty once present does not meet its existence with no member.
A HISTORY's `events`, while the HISTORY has no `summary`, are left to § [Precedence](#precedence) 2, which gives a HISTORY left with neither one of them.
Any other such list whose existence lower bound is bounded and at least 1 is topped up to one member from the sources above, as a list its cardinality asks a member of is, so that it holds a member or `Generate` returns the error § [Errors](#errors) names for the member it cannot build.
So, at that existence:

- an `ITEM_LIST` or `ITEM_TREE` whose `items` the OPT leaves without a child, or with slots only, holds no items: it is written with an empty `items` where the RM or the OPT requires the structure, as the BMM requires `EVENT.data`, and left out where the attribute that holds it is not required (*Visit rule*);
- a `COMPOSITION`'s `content` whose only child is a slot gets that slot's fill;
- a `SECTION`'s `items` the OPT leaves without a child is refused with `ErrArchetypeIDMissing`, since the member its BMM type gives is an archetype root.

##### Lists the OPT writes single-valued

An attribute the RM declares multi-valued that the OPT writes as a single-valued one, such as `COMPOSITION.content` or `ITEM_TREE.items` under a `C_SINGLE_ATTRIBUTE`, is written as the list the RM declares, as the template validator reads it ([§ REQ-171](#req-171--validation-engine) *Structural access*).
The value a visit builds from its allowed children, as for any single-valued attribute, is its one member, so such an `ITEM_TREE.items` over an `ELEMENT` holds that `ELEMENT`.
With no allowed child, where the BMM does not mark it mandatory and it is not a list the RM makes non-empty once present whose existence lower bound is at least 1, only its existence asks for a member.
So it gets none, written present and empty as above where its existence lower bound is at least 1, and `Generate` does not refuse it with `ErrArchetypeIDMissing`, which § [Errors](#errors) keeps for a member the walk has to build.
Where it is such a list, or the BMM marks it mandatory, as it does `PERSON.identities`, it is topped up as above, from its BMM type.

#### Leaf values

Under every policy, a primitive leaf the generator writes for an OPT primitive constraint is valued as the `ValueFill` in force says (§ [Primitive-leaf value fill](#primitive-leaf-value-fill)), except where:

- an RM rule narrows it or keeps it within a set (§ [Precedence](#precedence) 3 and 4);
- the constraint lists no code or ordinal to choose from, where *Placeholders* below decides;
- it is the text of a coded value, which takes the rubric or term text of its code where the OPT's constraint on the text admits that text (*RM defaults*, *The text of a coded value*), under either `ValueFill`;
- the template's `<constraints>` set a default value that validates on the data value that holds it and keeps the RM rules § *Trust model* names, which takes that default under either `ValueFill` (§ *Trust model*);
- the OPT leaves it open, where the list below decides.

Under either `ValueFill`, where the OPT leaves one of these values open, with no constraint of its own or a `C_STRING` that has neither a list nor a pattern, the generator writes a value that carries a meaning:

- the `value` of a plain `DV_TEXT` that is an ELEMENT's `value`: the name the generator writes for that ELEMENT, or `example` where that name is only the RM type of an ELEMENT the OPT gives no term;
- the `value` of any other plain `DV_TEXT`, such as a PARTICIPATION's `function`, an ISM_TRANSITION's `reason` or a REFERENCE_RANGE's `meaning`: `example`, except the `function` of a participation *Optional RM attributes under `Large`* writes; a `DV_CODED_TEXT` keeps the text the third bullet of the list above gives it;
- a `DV_PARSABLE`'s `value`: `<example/>` where its `formalism` is an XML media type (`text/xml`, `application/xml` or one with the `+xml` suffix), `{}` where it is a JSON one (`application/json`, `text/json` or one with the `+json` suffix), and `example` otherwise;
- a `DV_IDENTIFIER`'s `id`: `ID-0001`, a fixed identifier of a recognisable shape;
- a `DV_IDENTIFIER`'s `issuer`, wherever the OPT does not prohibit it: `Example issuer`, or, where the OPT's `C_STRING` on the issuer rejects that, the example that `C_STRING` gives.

A `C_STRING` with a list or a pattern gives its own example, a list member or a string the pattern builds, which `ExampleFill` writes and from which `RandomFill` draws.

#### Clock

Every `DV_DATE_TIME` the generator writes whose OPT node carries no primitive constraint and gets no OPT value, `HISTORY.origin`, `EVENT.time`, `ACTION.time` and `EVENT_CONTEXT.start_time` and `end_time` included, **MUST** take its value from `Options.Now`, written in UTC as RFC 3339.

#### Placeholders

Where neither the OPT nor an RM default below gives a value, the generator **SHOULD** write fixed values a reader recognises as placeholders, in the terminology the OPT asks for:

- the node id `at0000` on a locatable;
- the code `at0000` on a coded text, a code phrase, or an ordinal or scale symbol, that code in the terminology the OPT names for it where the OPT names one and lists no code (a `C_CODE_PHRASE` with an empty code list), under either `ValueFill`, and in `local` otherwise;
- and the URI `ehr://example` on a `DV_EHR_URI`.

Any other value the generator writes without a constraint, a placeholder, a default here or a value *Leaf values* gives is the implementation's choice, within § Contract *Output*.

#### RM defaults

When both of these hold, the generator **SHOULD** write the default given here, subject to § [Precedence](#precedence) 1, a coded default with the openEHR rubric of its code as its text, since each is a value the RM rules accept where the OPT leaves the choice open (§ Contract *Output*):

- the generator writes one of the attributes below, because the visit rule or § [Precedence](#precedence) 2 has it written;
- the OPT gives it no value, because it leaves the attribute silent or names it with no code (or only `at0000`) to choose.

A `COMPOSITION`'s `context` is written under every policy, `Required` included, as its item says:

- an abstract `PARTY_PROXY` other than a `COMPOSITION`'s `composer`, such as an ENTRY's `subject` or a `PARTICIPATION`'s `performer`: a `PARTY_SELF`, never an empty `PARTY_IDENTIFIED`, which would break `Basic_validity`;
- an abstract `OBJECT_ID` or `UID_BASED_ID`, such as the `id` of a `PARTY_REF`, an `OBJECT_REF` or a `LOCATABLE_REF`: a `HIER_OBJECT_ID` whose `value` *Identity* gives;
- an ENTRY's `language`: `Options.Language` in `ISO_639-1`; its `encoding`: `UTF-8` in `IANA_character-sets`; so that `Language_valid` and `Encoding_valid` hold (§ [REQ-112](#req-112--template-less-reference-model-validation-floor) *Coded invariants*);
- a `DV_TEXT`'s or `DV_CODED_TEXT`'s `language`, and a `DV_ENCAPSULATED`'s: `Options.Language` in `ISO_639-1`; a `DV_TEXT`'s or `DV_CODED_TEXT`'s `encoding`, and a `DV_ENCAPSULATED`'s `charset`: `UTF-8` in `IANA_character-sets`; so that their `Language_valid`, `Encoding_valid` and `Charset_valid` hold;
- a `COMPOSITION`'s `category`: `openehr` `433|event|`; its `language`: `Options.Language` in `ISO_639-1`; its `territory`: `Options.Territory` in `ISO_3166-1`; and, under every policy, its `context`: an `EVENT_CONTEXT`, whose `start_time` *Clock* gives;
- an `EVENT_CONTEXT`'s `setting`: `openehr` `238|other care|`, so that `Setting_valid` holds, which the generator also writes over a code outside the openEHR setting group where the OPT admits it;
- an `ISM_TRANSITION`, so that `Transition_valid` holds:
  - its `current_state`: `openehr` `524|initial|`;
  - its `transition`: the transition of the openEHR instruction state machine that leads into the `current_state` written, `535|initiate|` into `526|planned|`, `537|postpone|` into `527|postponed|`, `539|schedule|` into `529|scheduled|`, `540|start|` into `245|active|`, `544|suspend|` into `530|suspended|`, `548|finish|` into `532|completed|`, `547|abort|` into `531|aborted|`, `166|cancel|` into `528|cancelled|` and `549|time out|` into `533|expired|`, and `535|initiate|` beside `524|initial|`, which no transition leads into, or beside a `current_state` outside the instruction states group;
- an `ACTIVITY`'s `action_archetype_id`, so that `Action_archetype_id_valid` holds:
  - where the OPT leaves it silent, or constrains it with a `C_STRING` that lists no string and admits `/.*/`, as one with neither a pattern nor a list does: `/.*/`, the RM default (RM `ACTIVITY.action_archetype_id`: "Defaults to `/.*/`, meaning any archetype");
  - where the OPT constrains it with a `C_STRING` with a list, or one that rejects `/.*/`: a string that `C_STRING` admits (§ [Precedence](#precedence) 1);
- an `INTERVAL_EVENT`'s `math_function`: `openehr` `146|mean|`, so that `Math_function_validity` holds;
- a `PARTY_RELATED`'s `relationship`: `openehr` `253|unknown|` of the subject relationship group, so that `Relationship_valid` holds;
- a `PARTICIPATION`'s `function` the generator writes as a `DV_CODED_TEXT`: `openehr` `253|unknown|` of the participation function group, so that `Function_valid` holds; its `mode`: `openehr` `193|not specified|`, so that `Mode_valid` holds;
- a `TERM_MAPPING`'s `match`: `?`, so that `Match_valid` holds; its `purpose`: `openehr` `669|public health|`, so that `Purpose_valid` holds;
- a `DV_PERIODIC_TIME_SPECIFICATION`'s `value`: a `DV_PARSABLE` whose `formalism` is `HL7:PIVL`, so that `Value_valid` holds;
- an ELEMENT with no `value`: the `null_flavour` `openehr` `271|no information|`;
- a `DV_MULTIMEDIA`'s `media_type`: `text/plain` in `IANA_media-types`, so that `Media_type_valid` holds; and its `uri`, where § [Precedence](#precedence) 2 or the content rule below has it written and the OPT names no allowed child for it: `http://example.com`.

Every `DV_MULTIMEDIA` the generator writes **SHOULD** carry the content its settled `media_type` suits, so that it meets DV_MULTIMEDIA `Not_empty`:

- where the OPT prohibits `data`: a `uri`, which `Not_empty` then needs (§ [Precedence](#precedence) 2), with the fixed size below;
- else, inline `data`, a non-empty example payload that is the same under both `ValueFill` values, and a `size` that is the number of bytes of that `data`, which the RM defines as the original size of the unencoded data, for one:
  - of a text media type (`text/` and a subtype);
  - of another media type, where the OPT prohibits the `uri` or requires the `data`;
- else: a `uri` and no `data`, and the `size` 1024, a fixed positive number, since the generator cannot read the content at the `uri`.

The `uri` is built from its first allowed OPT child where the OPT names one, under every policy, and is the default `http://example.com` above otherwise.
Where the OPT's constraint on `size` rejects the number, the size the walk wrote stays (§ [Precedence](#precedence) 1).

##### Optional RM attributes under `Large`

Under `Large`, `Generate` **MUST** write the optional RM attributes below where the OPT node of the object that holds one does not name it, or no OPT node describes that object, as for the `context` *RM defaults* writes where the OPT names none:

- a `COMPOSITION`'s `feeder_audit`: a `FEEDER_AUDIT` whose `originating_system_audit` has a `system_id` and a `time` *Clock* gives;
- an `EVENT_CONTEXT`'s `end_time`, which *Clock* gives, its `health_care_facility`, a `PARTY_IDENTIFIED` whose `name` is `example`, and one member of its `participations`;
- an ENTRY's `provider`, a `PARTY_SELF`, and one member of its `other_participations`;
- an `ACTIVITY`'s `timing`: a `DV_PARSABLE` whose `formalism` is `ISO8601` and whose `value` is an ISO 8601 recurring interval, `R/`, the date-time *Clock* gives and `/P1D`, the form the RM prefers for it (RM `ACTIVITY.timing`: "the preferred syntax is ISO8601 'R' format").

Each participation has a `PARTY_SELF` performer, the `mode` `openehr` `193|not specified|` with its rubric, and a `function` that is a plain `DV_TEXT` whose `value` is `unknown`, the rubric of its RM default `253`, since FLAT's `|function` key carries only a plain `DV_TEXT` ([wire.md § REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes)) and `Function_valid` binds only a coded `function`.
An attribute the OPT node names is the visit's: `Large` builds it under the OPT's constraints where the OPT allows it, a `feeder_audit` excepted (*Known gap: attributes the generator cannot write*), and leaves it out where the OPT prohibits it.
Beyond those *Leaf values*, *RM defaults* and § [Precedence](#precedence) 4 write under every policy, `Large` writes no other optional RM attribute the OPT does not name, such as an ENTRY's `workflow_id` or `guideline_id`, a LOCATABLE's `links`, or a `HISTORY`'s `duration` or `period`.
The skeleton the composition builder seeds a `Builder` with under `Large` holds none of them ([§ REQ-101](#req-101--generic-opt-driven-composition-builder)).

##### The text of a coded value

Where the OPT itself pins an `openehr` code on a coded text, the generator:

- **SHOULD** write the code's rubric below as its text where the OPT's constraint on the text admits it, since a coded text's value is its code's term;
- and keeps the text the walk wrote where it does not (§ [Precedence](#precedence) 1) or where the code has no such rubric.

The rubric is:

- on an attribute the RM ties to a group of the openEHR terminology, each attribute the *Coded invariants* table of [§ REQ-112](#req-112--template-less-reference-model-validation-floor) pairs with a group, the code's rubric in that group;
- on any other attribute, such as a locatable's `name`, the rubric every group that holds the code gives it, where they all give the same one: `433` is `event`, while `532`, `complete` in one group and `completed` in another, has none.

Every `DV_CODED_TEXT` the generator writes whose `defining_code` is in a terminology other than `openehr`, and whose code the archetype it belongs to defines:

- **SHOULD** take that term's text as its `value` where the OPT's constraint on the text admits it, since a coded text's value is the rubric of its term (RM `DV_TEXT.value`: for a `DV_CODED_TEXT`, "the rubric of the complete term");
- and keeps the text the walk wrote where it does not (§ [Precedence](#precedence) 1), except a coded `name` below.

The archetype defines:

- a `local` code under its code string;
- and a code of another terminology:
  - under `<terminology id>::<code string>`, the key an OPT's term definitions give it (for example `SNOMED-CT::248152002` with the text `Female`);
  - or, where it gives no text under that key, under its code string alone, the key some OPTs give it (`code="248152002"`, `code="planned"`), unless that code string is an archetype term code, `at` and digits with any dot extensions, which names a term of the archetype's own (ADL 1.4 *Node Identification*: codes "defined within the same archetype are prefixed with `at`").

The archetype is the coded text's nearest enclosing archetype root, whose term definitions alone give a local code its meaning; the text is in `Options.Language`, with the language fallback of [§ REQ-111](#req-111--public-compiled-template-bridge) *Term definitions and bindings*.
A code that archetype does not define keeps the text the walk wrote, even where an enclosing archetype defines the same code.
A locatable's `name` coded so, whose `value` a `C_STRING` constrains and whose `defining_code` a `C_CODE_PHRASE` constrains to a list of codes:

- takes, where the code the walk wrote has a term whose text the `C_STRING` rejects, the first code of that list whose term text, read as above, the `C_STRING` admits, with that text as its `value`;
- where the `C_STRING` admits the term text of no listed code, it keeps the code and the text the walk wrote.

#### Identity

Under every policy, the generator **SHOULD** give a `uid`, from `Options.UIDSource` or from its random fallback when `UIDSource` is nil, to a locatable it writes only where the OPT requires its `uid` (an existence lower bound of at least 1), since the server that stores a top-level object gives it its version id and paths, not uids, identify the nodes inside it.
No other locatable gets one: not a `COMPOSITION`, an ENTRY, a `GENERIC_ENTRY`, a `PARTY` or a `PARTY_RELATIONSHIP`, nor a slot fill or a locatable built from a BMM type.
The RM recommends that a top-level object's `uid` copy its version id, so the `uid` "will usually be empty in most EHR data" (RM Common IM, *Unique Node Identification*).
That a `PARTY` and a `PARTY_RELATIONSHIP` are left without a `uid`, as a `COMPOSITION` and an ENTRY are, is the reading this specification chooses, so that the server that stores a generated object assigns its id; a `PARTY` then breaks `Uid_mandatory` until it does (§ [Exceptions and known gaps](#exceptions-and-known-gaps)).
When an `OBJECT_REF` the generator writes, a `PARTY_RELATIONSHIP`'s `source` and `target` included, carries no id value, the generator **SHOULD** give the id a value in its class's form, so that the reference's mandatory id holds a well-formed value:

- the value of a uid drawn the same way for a HIER_OBJECT_ID, a GENERIC_ID, a TEMPLATE_ID or a TERMINOLOGY_ID;
- that value followed by `::example::1`, a creating system `example` and the first trunk version, for an OBJECT_VERSION_ID;
- and `openEHR-EHR-CLUSTER.example.v1` for an ARCHETYPE_ID.

Every `ARCHETYPED` the generator writes **MUST** carry `rm.Release`, the RM release the SDK is generated from, as its `rm_version` ([reference-model.md § Generator output conventions](reference-model.md#generator-output-conventions)).
The template root's `ARCHETYPED`, when it has one, **MUST** carry the template id, and the generator **MUST NOT** write the template id in any other.

#### Reproducible uids

`UIDsFrom(src)` **MUST** return:

- a uid source that draws each uid from `src` as an RFC 9562 version-4 UUID, its version and variant bits set and its other 122 bits taken from `src` ([RFC 9562 §5.4](https://www.rfc-editor.org/rfc/rfc9562.html#section-5.4)), so two sources in the same state give the same uids in the same order;
- and a nil source for a nil or typed-nil `src`, so the random fallback applies.

Two `Generate` calls on one compiled OPT, neither with its context done, under any policy and either `ValueFill`, whose options are the same, a non-zero `Now` included, except that each call's `UIDSource`, from `UIDsFrom`, and, under `RandomFill`, its `ValueSource` are built on sources in the same state, **MUST** either:

- both return a root, the two equal value for value;
- or both return no root and an error wrapping the same one of the errors *Inputs* above and § [Errors](#errors) name.

The generator **MUST NOT** draw a uid from `ValueSource`: with `UIDSource` nil, every uid comes from the random fallback, whatever `ValueSource` holds.
A source shared between `ValueSource` and `UIDsFrom` stays reproducible, but each uid drawn from it shifts the leaf values drawn after it; a separate source keeps the leaf values those of a run with no `UIDSource`.
The source `UIDsFrom` returns is not safe for concurrent use, as a `Source` is not, so each concurrent `Generate` takes its own.
The composition builder surfaces it as `composition.WithUIDSource` ([§ REQ-101](#req-101--generic-opt-driven-composition-builder)).

#### Generic and untyped types

An OPT-declared `DV_INTERVAL` instantiation (e.g. `DV_INTERVAL<DV_QUANTITY>`) is built as that typed interval, and `DV_INTERVAL<DV_ORDERED>` as an interval whose bounds can be any ordered value.
One over another abstract `DV_ORDERED` class, such as `DV_INTERVAL<DV_QUANTIFIED>`, is built over the first of these types that descends from that parameter, that the generator builds an interval over, and that is, or descends from, the class each bound names:

- the concrete class the OPT names for its lower bound, then the one it names for its upper bound;
- the concrete type the generator builds for the class the OPT names for its lower bound, then for its upper bound (*What a visit writes*);
- and each concrete descendant of the parameter in name order.

Each bound is built as that type.
So `DV_INTERVAL<DV_QUANTIFIED>` with no bound is built as `DV_INTERVAL<DV_COUNT>`, which the template validator admits, and a `DV_INTERVAL<DV_TEMPORAL>` with a `DV_TEMPORAL` lower bound and a `DV_DATE_TIME` upper bound as `DV_INTERVAL<DV_DATE_TIME>`.
Where no such type exists, as for a `DV_DATE` lower bound and a `DV_DATE_TIME` upper bound of a `DV_INTERVAL<DV_TEMPORAL>`, or for a `DV_COUNT` bound of one, `Generate` returns an error wrapping `ErrConstraintUnsatisfiable` and no root; a `DV_INTERVAL<DV_ORDERED>` leaves two bounds of different classes as generated instead (*Intervals*).
An untyped OPT child, one with no `rm_type_name`, is built as the type the compile reads it as ([§ REQ-111](#req-111--public-compiled-template-bridge) *Reading the model*), with a formal parameter of the attribute that holds it taken as the actual type the walk builds for the holding node, as the `T` of a `DV_INTERVAL<DV_COUNT>`'s `lower` is a `DV_COUNT`, or as the concrete type the generator builds for that type, so an untyped child of an `ELEMENT`'s `value` is built as a `DV_TEXT`.
Every rule here reads it as that type, its attributes as the BMM declares them there, as for a child that names the type.
Another generic `rm_type_name` is a known gap (below).

#### Intervals

When both sides of an interval are bounded and its two bounds are two DV_QUANTITY, or two DV_PROPORTION, the generator, before it orders them, gives them one unit, or one type, that both sides admit, where there is one, so that the two are strictly comparable (RM `DV_INTERVAL.Limits_consistent`).
The same holds in a `DV_INTERVAL<DV_ORDERED>` whose two bounds are both of one of those types.
The generator tries the lower bound's units, then the upper bound's, then the units the lower side lists, and then those the upper side lists.
Of these, it takes the first that both sides admit and in which an ordered pair fits both sides' OPT constraints, as the next paragraph puts a pair in order, and, where no unit both sides admit has such a pair, the first that both sides admit.
So two bounds that share their units keep them, unless an ordered pair fits both sides only in another unit.
A side admits the units its OPT constraint accepts: its C_DV_QUANTITY ([§ REQ-103](#req-103--primitive-constraint-introspection)), or the C_STRING on the bound's `units` where the OPT builds the bound attribute by attribute; a side the OPT leaves without a constraint on units admits any.
A side lists the units of its C_DV_QUANTITY's entries; where its C_DV_QUANTITY constrains a `property` and lists no entry, the units § REQ-103 gives that property (`PropertyUnits`), which are the units such a side admits; and where the OPT builds the bound attribute by attribute, the list of the C_STRING on its `units`.
A side whose C_DV_QUANTITY lists units takes, in a new unit, the magnitude `ExampleValue` gives for the first entry in that unit, and any other side keeps its magnitude.
The type is chosen in the same way, among the lower bound's type, then the upper bound's, then every kind in code order, and a side admits a type where its bound can be settled as that kind inside its OPT constraints, as § [Precedence](#precedence) 4 settles a kind, and is then so settled.
A side whose bound carries a `normal_range` of its own admits only its bound's own units and type.
Where the two sides admit no one unit or type, the generator leaves the pair as generated; it leaves two bounds of different classes as generated too.

When both sides of an interval are bounded, the generator puts the two bounds in order (BASE `Interval.Limits_consistent`) where it can compare them and finds an ordered pair that fits both sides' OPT constraints among the pairs it tries, and leaves any other pair as generated (*Known gap: interval pairs the generator does not try*).
A bound that carries a `normal_range` and a `normal_status` of its own fits its side only with a value that keeps its own `Normal_range_and_status_consistency` (§ [Precedence](#precedence) 3).
The pairs it can compare are:

- two DV_COUNT;
- two DV_QUANTITY with equal `units`;
- two DV_PROPORTION of one `type`, by ratio;
- two DV_ORDINAL, by `value`;
- two DV_DATE, two DV_TIME or two DV_DATE_TIME values the REQ-123 parse accepts, in the order [§ REQ-112](#req-112--template-less-reference-model-validation-floor) *DV_INTERVAL* gives them: at the precision they share, as [§ REQ-103](#req-103--primitive-constraint-introspection) *Validate contract* defines it, and two times or date-times written with different zone designators as instants;
- and two DV_DURATION values the REQ-123 parse accepts, by the magnitude it gives, as [§ REQ-112](#req-112--template-less-reference-model-validation-floor) *DV_INTERVAL* orders them.

The same holds in a `DV_INTERVAL<DV_ORDERED>` whose two bounds are one of those types.
No other pair is one it can compare, for example, a zoned against a zoneless time or date-time, or a value the REQ-123 parse refuses.

#### Precedence

Where the rules above meet the OPT or the RM, the generator decides as follows.

1. The OPT wins over an RM default.
   Where the OPT's own constraint on an attribute rejects a default, as a `C_CODE_PHRASE` that names another terminology does (an OPT can name a `DV_MULTIMEDIA` media type in terminology `openEHR`), or a `C_STRING` on a coded text's `value` that rejects the rubric, the generator keeps the value the walk wrote, which that constraint admits, where item 4 gives it no other.
   Where the OPT prohibits an attribute that no RM rule needs, the generator writes nothing there, neither a default nor a `uid`.
2. An RM rule that needs an attribute present or absent wins over the OPT and over the visit rule.
   Present:
   - an attribute the BMM marks mandatory;
   - an ELEMENT's `value` when the OPT prohibits its `null_flavour` and the walk wrote no value, and its `null_flavour` when it has no value (`Inv_null_flavour_indicated`);
   - a `PARTY_IDENTIFIED`'s `name` when it has no `name`, `identifiers` or `external_ref` (`Basic_validity`), as the text `example` where the OPT gives it no allowed child;
   - a `DV_MULTIMEDIA`'s `uri` when it has neither `uri` nor `data` (`Not_empty`);
   - and a `HISTORY`'s `events`, or else its `summary`, when it has neither (`Events_valid`).

   The generator writes such an attribute as a visit writes it (§ Contract *What a visit writes*), whether the OPT prohibits it or the policy would not visit it: from its first allowed OPT child where it has one, and from its BMM type where it has none.
   Two take a value of their own instead, where the OPT names no allowed child for them: a `DV_MULTIMEDIA`'s `uri` the one *RM defaults* gives and a `PARTY_IDENTIFIED`'s `name` the text `example`.
   A `PARTY`'s `uid`, which `Uid_mandatory` needs, is not in this list: *Identity* above governs it (§ [Exceptions and known gaps](#exceptions-and-known-gaps)).
   A `HISTORY`'s two are tried in this order:
   - a member of `events` built from its first allowed child that needs no undescribed content (§ [Slot fills](#slot-fills); a slot always needs it);
   - a `summary` built from its first allowed child that needs no undescribed content;
   - a member of `events` built from its BMM type, where the OPT names no child under `events`;
   - a `summary` built from its BMM type, where the OPT names no child under `summary`;
   - a member of `events` built from its first allowed child that is not a slot;
   - a `summary` built from its first allowed child that is not a slot.

   So a child that needs undescribed content is passed over for a `summary` or a BMM type the OPT allows, and is built only where neither is allowed.
   An attribute the OPT prohibits, and an `events` whose cardinality upper bound is 0, allow none of them.
   The generator writes the first of the six the OPT allows, and neither where it allows none (§ [Exceptions and known gaps](#exceptions-and-known-gaps), *An OPT that contradicts itself*).
   Absent: an ELEMENT with a `value` carries no `null_flavour` and no `null_reason` (`Inv_null_flavour_indicated`, `Inv_null_reason_valid`); a bound of a REFERENCE_RANGE's `range` carries no `normal_range` and no `other_reference_ranges` (`Range_is_simple`); where the OPT allows both a `value` and a `null_flavour`, the `value` gives way when the OPT requires the `null_flavour` or the `null_reason` but not the `value`, and wins otherwise.
3. An RM rule that narrows a value the OPT leaves open wins over the leaf's `ExampleValue` and a `RandomFill` draw.
   These are the rules the generator narrows by, and the only ones (item 4 names those that keep a value within a set):
   - BASE `Type_validity` narrows a PARTY_REF `type` to a party class name and an ACCESS_GROUP_REF `type` to `ACCESS_GROUP`;
   - `Match_valid` narrows a TERM_MAPPING `match` to `>`, `=`, `<` or `?`;
   - `Magnitude_status_valid` narrows a `magnitude_status` to `=`, `<`, `>`, `<=`, `>=` or `~`;
   - `Accuracy_is_percent_validity` with `Accuracy_validity` narrows a percentage `accuracy` to more than 0 and at most 100, or, where the OPT admits no such accuracy, its `accuracy_is_percent` to false where the OPT admits that;
   - `Normal_range_and_status_consistency` narrows an ordered value whose `normal_status` is `N` into its `normal_range` and one with another `normal_status` out of it, where the value and the range's bounds are a pair § Contract *Intervals* says the generator can compare, other than two DV_DURATION;
   - the lower bound -1 of a `DV_QUANTITY`'s or `DV_PROPORTION`'s `precision` ([§ REQ-112](#req-112--template-less-reference-model-validation-floor)) narrows a precision range that reaches below it;
   - `Scheme_valid` narrows a `DV_EHR_URI`'s `value` to the `ehr` scheme.
     The value § Contract *Leaf values* gives stays where it has that scheme.
     Otherwise the generator writes the first value with that scheme the OPT lists, or, where the OPT lists none or leaves its list open, one its pattern admits, and else `ehr://example` where the OPT admits it.
     An empty value, which only a pattern that admits nothing else gives, is not narrowed: it gets the placeholder (§ [Exceptions and known gaps](#exceptions-and-known-gaps)).

   A `C_INTEGER`, a `C_DV_ORDINAL` value or a `precision` range that reaches past the 32-bit range of an RM `Integer` is refused when the template is read: its text does not decode as an `Integer` ([wire.md § REQ-056](wire.md#req-056)), whose range [wire.md § Floating-point precision](wire.md#floating-point-precision) gives.
   So the generator meets none.
4. An RM rule that keeps a value within a set the OPT's constraint can reach past wins over the leaf's `ExampleValue` and a `RandomFill` draw where that constraint admits a member of the set, and gives way to the OPT where it admits none.
   These rules are the code set or openEHR terminology group each row of the *Coded invariants* table of [§ REQ-112](#req-112--template-less-reference-model-validation-floor) ties a coded attribute to, DV_PERIODIC_TIME_SPECIFICATION `Value_valid` (`HL7:PIVL` or `HL7:EIVL`), and the rules of a DV_PROPORTION's kind (`Type_validity`, `Unitary_validity`, `Percent_validity`, `Fraction_validity`, `Precision_validity`, `Valid_denominator`).
   A value the walk wrote that the rule accepts stays.
   Otherwise the generator writes, without a draw, the RM default § Contract gives where the OPT admits it, a coded default with its rubric, and else the first value the OPT's constraint names that the rule accepts:
   - for a coded attribute the first one the constraint lists;
   - for a formalism the first of `HL7:PIVL` and `HL7:EIVL` the constraint admits;
   - and for a DV_PROPORTION the first kind, in code order, whose rules the OPT's constraints on its parts let it meet.

   Where there is none, the value the walk wrote stays and the root breaks the rule (§ [Exceptions and known gaps](#exceptions-and-known-gaps)), as it does where the OPT admits a coded value of the set that it does not list.
   A DV_PROPORTION whose OPT pins `is_integral` to true, with a `C_BOOLEAN` that admits `true` alone, takes a whole-number `numerator` and `denominator` whatever its kind, under either `ValueFill`.
   Such a proportion, a fraction and an integer fraction take the `precision` 0 wherever the OPT admits 0, whether or not the walk wrote a precision.
   So `Fraction_validity` holds under both readings of `is_integral`: whole-number operands, as the RM floor reads it, and a `precision` of 0, as the BMM defines it ("integers, i.e. if precision is 0").

#### Errors

An object the walk reaches, of a class the floor treats as an archetype root ([§ REQ-112](#req-112--template-less-reference-model-validation-floor) *Archetype roots*) for which the OPT names no archetype id, **MUST** make `Generate` return an error wrapping `instance.ErrArchetypeIDMissing` that names the RM type and the OPT path, at every policy, either `ValueFill`, and through the builder, whether it is:

- an OPT node without one, a slot excepted, which § Slot fills governs;
- the member of a required attribute with no allowed OPT child, where a list that only its existence requires asks for no member (§ Contract *What a visit writes*);
- the template root itself.

The generator invents no archetype id for it.
An OPT node of that kind, such as an `ADMIN_ENTRY` an OPT defines inline under a `SECTION`, is undescribed content, as a slot nothing can fill is (§ [Slot fills](#slot-fills)), so the walk reaches it only where the OPT or the RM requires it:

- as a child the OPT does not leave optional;
- or as the member a top-up of a required attribute takes (§ Contract *What a visit writes*).

Where the walk leaves such a node out, the defect is the OPT's, and the generator does not report it.

`Generate` **MUST** return no root and an error wrapping `instance.ErrConstraintUnsatisfiable` that names the RM type, the attribute and the OPT path in each of these cases, so it never writes a value the constraint rejects:

- no value can satisfy a primitive leaf's constraint (for example a STRING whose pattern and list admit no string the generator can derive), or none the generator can derive is left once an RM rule § [Precedence](#precedence) 3 names narrows it (for example a C_STRING on a TERM_MAPPING `match` that admits none of the four match codes);
- the walk visits an attribute the OPT declares multi-valued whose cardinality upper bound is bounded and less than the sum of the occurrences lower bounds, each bounded, of its allowed children that are not slots.
  So no instance holds every member those children ask for within that bound (ADL 1.4 *cADL*, validity rule VCOC: the interval from "the sum of all occurrences minimum values" to "the sum of all occurrences maximum values" lies "inside the interval of the cardinality"; AOM 2 validity rule VACMCO: "one instance of every mandatory child object" is "to be included within the cardinality range").
  A child that states no occurrences, or occurrences with an unbounded lower bound, adds nothing to that sum, as it asks a list for no member ([§ REQ-102](#req-102--composition-validation) *An existence lower bound alone asks for no member*);
- the walk would visit an attribute the generator has no writer for (§ Contract *Visit rule*): no value it can derive satisfies the OPT there.

A required slot whose includes cannot be satisfied makes `Generate` return `ErrSlotFillUnsupported`, as § [Slot fills](#slot-fills) says.

An error `Generate` returns **MUST NOT** wrap more than one of `ErrArchetypeIDMissing`, `ErrConstraintUnsatisfiable` and `ErrSlotFillUnsupported`, so a caller tells a missing archetype id, an unsatisfiable constraint and an unfillable slot apart with `errors.Is`: `ErrSlotFillUnsupported` is kept for slots, and `ErrArchetypeIDMissing` for archetype roots.

### Exceptions and known gaps

A root `Generate` returns can break § Contract's output rule in these cases, and in no other:

- **An interval pair left as generated** (§ Contract *Intervals*) can break `DV_INTERVAL.Limits_consistent`, which the floor reports for a pair it orders whose bounds are out of order and for a pair it reads as not strictly comparable (§ REQ-112 *DV_INTERVAL*): bounds of two classes, and two DV_QUANTITY or two DV_PROPORTION whose two sides admit no one unit or type.
  An ordered value that forms no pair § Contract *Intervals* lists with the bounds of its `normal_range`, and a DV_DURATION whatever its `normal_range`, is left as generated too (§ [Precedence](#precedence) 3), so it can break `Normal_range_and_status_consistency`.
- **An OPT constraint that admits no value an RM rule accepts**, where § [Precedence](#precedence) 3 does not name that rule, or that admits a coded value a rule of § [Precedence](#precedence) 4 accepts without listing it, such as an RM default that yields to the OPT (§ [Precedence](#precedence) 1), or a PARTICIPATION `mode` the OPT codes in `local`, breaks that RM rule.
- **An OPT that contradicts itself** in a way § [Errors](#errors) does not refuse, such as one that requires a single-valued attribute and prohibits every child of it, or requires a list and gives it a cardinality upper bound of 0, gets whichever side § Contract writes.
  One whose `HISTORY` allows none of the events and summaries § [Precedence](#precedence) 2 tries gets neither, and breaks `Events_valid`.
  One whose list's cardinality lower bound exceeds the members its allowed children's occurrences upper bounds admit, none of them a slot, gets the members its top-up adds past its first allowed child's occurrences (§ Contract *What a visit writes*).
  One whose children's occurrences lower bounds add up past the cardinality upper bound of their list gets no root where the walk visits that list (§ [Errors](#errors)).
  Where the walk does not, as under `Required` where the optional child that holds the list leads to no entry (§ Contract *Members of a list*), the root holds no instance of the list and the contradiction does not reach it.
- **A value the caller supplies** through `Options` (a language, a territory, a composer, a uid from `UIDSource`) is written as given, valid or not.
- **An RM invariant that relates the object to another**, such as PARTY `Relationships_validity` or PARTY_RELATIONSHIP `Source_valid` and `Target_valid`, which a single generated object cannot meet, is not kept.
- **A placeholder code on an attribute an RM rule ties to a code set or terminology group** for which § Contract gives no default, such as a `normal_status`, or a DV_MULTIMEDIA `compression_algorithm` or `integrity_check_algorithm`, can break that rule.
- **An attribute the OPT prohibits that an RM rule needs, a list the BMM requires a member in whose cardinality upper bound is 0, or an attribute the OPT requires that an RM rule needs absent** (§ [Precedence](#precedence) 2), such as `CLUSTER.items` or a PARTY's `identities` at cardinality `0..0`, or a `normal_range` on a bound of a REFERENCE_RANGE's `range`, breaks the OPT's prohibition, bound or requirement.
- **A `DV_EHR_URI`'s `value`, or the text and code of a `DV_ORDINAL`'s or `DV_SCALE`'s `symbol`, whose OPT pattern admits only the empty string**, such as `x{0}`, gets the placeholder *Placeholders* gives, since the RM needs it present (§ [Precedence](#precedence) 2), and breaks the OPT's pattern.
- **An earlier sibling with room that carries a later, required one's identity, where neither pins a name** (§ Contract *What a visit writes*): the validator binds an item to the first of them that has room and under whose constraints it validates ([§ REQ-102](#req-102--composition-validation) *Structural — binding items*).
  So the earlier sibling takes the member built for the later one wherever that member validates under it.
  Under `Medium` an optional earlier sibling gets no member of its own and takes the later one's.
  Under `Large` it gets members of its own, two where its occurrences upper bound is above 1, unless the list's cardinality upper bound leaves it no room once the later sibling has its members (§ Contract *What a visit writes*).
  It then gets none and takes the later one's, as under `Medium`, as an optional `at0001` before a required `at0001` in `CLUSTER.items` at cardinality `1..1` does.
  Its own members fill it where its occurrences upper bound is 1 or 2; where that occurrences bound leaves it room for another member, as a bound above 2, or none, does, it takes the later one's as well.
  A required earlier sibling with no occurrences upper bound always has room and takes the later one's member beside its own, under every policy.
  A root can then fail the template validator by the later sibling's occurrences.
- **A `PARTY` without a `uid`** (§ Contract *Identity*): the generator writes a `PARTY`'s `uid` only where the OPT requires it, so a `PARTY` it writes otherwise breaks `PARTY.Uid_mandatory`, which the floor does not evaluate ([§ REQ-112](#req-112--template-less-reference-model-validation-floor) *What the floor does not evaluate*).
  The object the server stores meets the rule once the server assigns its id.
- **A list the OPT requires by its existence that canonical JSON cannot carry empty**, which the walk gives no member, is left out, and the template validator reports it `required` (*Known gap: required lists canonical JSON cannot carry empty* below).
- **A template default that breaks an RM rule the default check does not read** (§ *Trust model*): an interval's flags or order (`Lower_included_valid`, `Limits_consistent`), a DV_MULTIMEDIA's `Not_empty`, a coded rule on a value below the default's own, or a rule § [Precedence](#precedence) 3 applies only to a value the walk draws, such as a `magnitude_status` or a `DV_EHR_URI`'s scheme, is written as the template sets it, and breaks that rule.
- **A slot whose occurrences the OPT constrains** (below) can be filled where the OPT prohibits it, or left empty where the OPT requires it.

The known gaps below describe the generator as it is; they are informative.

#### Known gap: attributes the generator cannot write

The generator has no writer for a LOCATABLE's `links`, for a `feeder_audit` the OPT names, for an EHR_ACCESS's `settings`, whose RM type ACCESS_CONTROL_SETTINGS has no concrete class, or for a list whose items the RM types as primitive values other than a DV_MULTIMEDIA's `data`, such as its `integrity_check`; the RM requires none of them.
The one `feeder_audit` it writes is the `COMPOSITION`'s that `Large` writes where the OPT does not name it (§ Contract *Optional RM attributes under `Large`*), which no OPT constraint reaches.
So `Generate` leaves such an attribute unwritten, whatever the OPT puts under it, as § Contract *Visit rule* says.
Where the visit rule does not, as for an optional `links` that holds a mandatory LINK, the node that holds the attribute needs undescribed content (§ [Slot fills](#slot-fills)), and `Generate` returns `ErrConstraintUnsatisfiable` and no root (§ [Errors](#errors)) only where the walk builds that node all the same:

- where the OPT or the RM requires it;
- or where it is the member a top-up takes.

A DV_MULTIMEDIA's `data` is an array of octets that no OPT child describes, so the walk does not visit it, and the inline `data` § Contract gives a DV_MULTIMEDIA of a text media type is written instead.

#### Known gap: required lists canonical JSON cannot carry empty

A list the OPT requires by its existence that the walk gives no member is left out where canonical JSON cannot carry it empty, which is any list but those [reference-model.md § Property → Go field](reference-model.md#property--go-field) names (§ Contract *What a visit writes*).
Among the lists that cannot is a list the RM keeps non-empty once present other than those [§ REQ-112 *Non-empty lists*](#non-empty-lists) evaluates, such as `PARTY.contacts`, `ACTOR.roles` or `ENTRY.other_participations`.
Validators report such a list `required` ([§ REQ-102](#req-102--composition-validation)), so a root that leaves one out is outside § Contract *Output*.

#### Known gap: other generic types

An OPT node whose `rm_type_name` is a generic type other than a `DV_INTERVAL` instantiation, such as `POINT_EVENT<ITEM_TREE>` or `REFERENCE_RANGE<DV_QUANTITY>`, cannot be built, and `Generate` returns an error.
Nor can a `DV_INTERVAL` over a type that is no `DV_ORDERED` class, such as `DV_INTERVAL<DV_TEXT>`, which the RM does not admit as its parameter; a `DV_INTERVAL` over an abstract `DV_ORDERED` class is built as *Generic and untyped types* above says.

#### Known gap: interval pairs the generator does not try

Where swapping two bounds does not fit, the generator tries each side's range ends, written to the side's pattern, and, for a DV_PROPORTION, a denominator that brings one side's ratio to the other's.
It does not try:

- a date, time or date-time with a fraction of a second, or in a zone other than its range end's;
- a DV_DURATION in more than one designator, other than a range end as written;
- a DV_PROPORTION pair whose order needs both denominators moved.

An interval whose only ordered pairs are of these kinds is left as generated, as § *Exceptions and known gaps* allows; no vendored OPT has one.

#### Known gap: slot occurrences

The compiled template does not carry an `ARCHETYPE_SLOT`'s occurrences, which the model of [§ REQ-176](#req-176--adl-14-operational-template-model) reads ([§ REQ-111](#req-111--public-compiled-template-bridge)), so the generator cannot tell a slot the OPT prohibits (an occurrences upper bound of 0) or requires (a lower bound of at least 1) from an optional one:

- a top-up can fill a prohibited one where its only allowed children are slots, or where every allowed child that is not a slot is full or one `Generate` refuses (§ Contract *What a visit writes*);
- and a required slot is not filled where it sits beside a non-slot child, or under a list that only its existence otherwise requires (§ Contract *What a visit writes*).

The template validator reads the same compiled tree, so it cannot count a slot's occurrences either and reports neither case.

### Primitive-leaf value fill

`Policy` selects *which* nodes are materialised; an orthogonal **`ValueFill`** selects *how* primitive leaves are valued.
The SDK **MUST** offer two fills, and read a `ValueFill` other than `RandomFill` as `ExampleFill`, with no error:

- `ExampleFill` (default) populates each leaf with its REQ-103 `PrimitiveConstraint.ExampleValue`, a single representative value, byte-identical across calls for one OPT.
- `RandomFill` draws each leaf from within its constraint (in-range magnitudes, value-set-member codes, enumeration entries), valid by construction and varying between calls.
  A `DV_DURATION`'s `value` under a `C_DURATION` is a duration that is not zero: a count of one designator the constraint accepts it in, drawn, such as `P3Y` or `PT7H`.
  Where the constraint has no range, the count is from 1 to 12; where it has one, it is drawn among the counts of that designator inside the range, between its bounds or twelve from its one bound, a designator a bound is written in tried first.
  Else the value is the count 1 of the first designator, in written order, the constraint accepts, and else its `ExampleValue`.
  A `DV_DURATION` whose `value` the OPT leaves unconstrained, naming none or a `C_PRIMITIVE_OBJECT` with no item, is drawn as one under an empty `C_DURATION`.
  It rounds what it draws where a precision applies, and keeps a draw unrounded where the rounded one would leave the constraint:
  - a `DV_QUANTITY`'s `magnitude`: to as many decimals as its precision allows where that is 0 or 1, so to none where the precision admits 0 alone, and to two otherwise.
    Its precision is the upper bound of the precision range of the `C_DV_QUANTITY` entry it is drawn in, or the `precision` the walk wrote where the OPT builds the quantity attribute by attribute.
    A `C_DV_QUANTITY` entry's `magnitude` is drawn inside the entry's range, away from 0 and from the range's bounds, or from 1 to 100 where the range is open;
  - a `DV_PROPORTION`'s `numerator` and `denominator`: to whole numbers where its kind or a `precision` of 0 asks for them (§ Contract *Precedence* 4), and otherwise as a magnitude is, to its `precision` up to two decimals.

`RandomFill` reproducibility is caller-controlled via **`Options.ValueSource`** (a `math/rand/v2.Source`): a fixed source makes leaf values byte-reproducible (§ Contract *Reproducible uids*); `nil`, or a typed-nil source such as a nil `*rand.PCG`, draws from the auto-seeded package global so successive calls differ — mirroring the `UIDSource` determinism seam.
A `Source` is not safe for concurrent use, so each concurrent `Generate` takes its own source, or leaves it `nil` for the concurrency-safe global.
The composition builder surfaces the seam as `composition.WithValueFill` / `composition.WithValueSource`.

#### The `Complete` level

`Complete` is `Large`, the SDK's own `complete` level.
It differs from the ITS-REST `detail_level=complete` level, every data point the template allows, which that level gives as reference guidance not expected to be committable.
Every root `Large` builds meets § Contract *Output*, so it can be committed, and it holds the optional RM attributes *Optional RM attributes under `Large`* lists, not every one the RM allows.

### Trust model

The compiled OPT is **authoritative for structure**.
The RM graph is assembled attribute-by-attribute from compiled metadata; the generator never guesses paths from an empty composition.
Primitive leaves are valued from their constraint as § [Primitive-leaf value fill](#primitive-leaf-value-fill) says; REQ-103 guarantees `Validate(ExampleValue()) == nil` for bounded constraints.
Under either `ValueFill`, a data value whose node carries a default value from the template's `<constraints>` (`CompiledNode.DefaultValue`, [§ REQ-111](#req-111--public-compiled-template-bridge)) takes that default in place of the value the `ValueFill` gives, where the default validates under the node's constraints and keeps the RM rules below.
It validates where it is of the RM type the walk builds for the node and meets these, which the walk holds without the template validator:

- the node's own primitive constraint, such as a `C_DV_QUANTITY`, a `C_DV_ORDINAL` or a `C_CODE_PHRASE`;
- the existence of each attribute the node names: one it requires is there, and one it prohibits is not;
- for each value an attribute holds, one of the attribute's allowed children: its primitive constraint, for an AOM 1.4 primitive such as `STRING`, and otherwise these same rules, on a value of its RM type.

It keeps the RM rules where:

- each attribute the BMM marks mandatory is present on it and on each value it holds, as the RM floor reads presence;
- the settles § [Precedence](#precedence) 3 and 4 run on the walk's own value (a percentage `accuracy`, a DV_PROPORTION's kind, a DV_PERIODIC_TIME_SPECIFICATION's formalism, a coded attribute's group or code set, and `Normal_range_and_status_consistency`) leave it as it is.

A default that does not validate, or breaks such a rule, leaves the value the `ValueFill` gives.
Compile captures an OPT `<assumed_value>` into the constraint's `Default` field, but the generator does not read it.

The generator is **sound** (every root it returns meets § Contract's output rule, apart from § [Exceptions and known gaps](#exceptions-and-known-gaps)), not **complete** (it does not enumerate every valid instance — different policies may produce different but equally valid trees).
Sound × validator-aligned ⇒ PROBE-027 cross-checks the contract.

### Slot fills

#### Undescribed content

A member is *undescribed* when no OPT node describes it: a slot fill, or a LOCATABLE built from the BMM type of an attribute with no allowed OPT child.
An OPT node, not a slot, of a class the floor treats as an archetype root ([§ REQ-112](#req-112--template-less-reference-model-validation-floor) *Archetype roots*) for which the OPT names no archetype id describes no member the generator can build either, since the generator invents no archetype id (§ [Errors](#errors)): this specification reads it as undescribed content, as a slot nothing can fill is.
A node *needs undescribed content* when:

- it is a slot or such an archetype-root node;
- an attribute on it that is required (§ Contract *Visit rule*) can be given no member but an undescribed one.
  That is so where the attribute has no allowed OPT child and its member would be a LOCATABLE, where every allowed OPT child of it is a slot or such a node, or where it is single-valued and the child that builds its member (§ Contract *What a visit writes*) is a slot or such a node;
- it has an attribute the generator has no writer for that the walk would visit (§ [Exceptions and known gaps](#exceptions-and-known-gaps), *Known gap: attributes the generator cannot write*), since nothing the walk can build satisfies it.

The rule reads the node's own attributes only: below a child the OPT or the RM requires, the walk builds what § Contract says, undescribed members included where the BMM, a child's occurrences, or the cardinality of a list that one of those or its existence requires asks for a member, so the described content around them is kept.
An attribute that only its existence requires, which a list the RM makes non-empty once present never is, gets no undescribed member, and is left with none where it has no other (*What a visit writes*).
Such an attribute still counts as required in the definition above, so a node whose attribute of this kind can be given no member but an undescribed one still needs undescribed content, and is still left out where it is optional.
So a slot the OPT leaves open, and an attribute it leaves without a child, get content only where the BMM, the OPT's counts, or the existence of a list the RM makes non-empty once present (*What a visit writes*) ask for a member of a node the RM or the OPT requires:

- never because the policy builds an optional node that holds nothing else;
- and never for any other existence or for a cardinality lower bound alone.

#### Required slots

A *required slot* is one the walk fills: one a top-up fills, or the first allowed child a visited single-valued attribute takes (§ Contract *What a visit writes*).
The walk stamps a required slot, at every policy and through the builder, with an archetype id drawn from the parsed include grammar of [§ REQ-103](#req-103--primitive-constraint-introspection) *Slot assertions* when a safe example can be synthesized; when the OPT carried no parseable includes, the id **SHOULD** be `openEHR-EHR-<RMType>.example.v1`, which the validator's RM-type-prefix rule admits.
A required slot whose includes cannot be satisfied **MUST** make `Generate` return `ErrSlotFillUnsupported`, and the generator invents no archetype id for it.

### Out of scope

- **Clinically realistic distributions** (plausible names, plausible vitals, FHIR Synthea-style synthetic patient data).
- **FLAT / STRUCTURED example strings** — REQ-053.
- **Encoding to wire bytes** — callers run `canjson` or `canxml` themselves.
- **Authoring-time templates (OET)**: the generator reads operational templates only ([§ REQ-176](#req-176--adl-14-operational-template-model)).
- **Generating every valid instance** — combinatorial coverage is out of scope.
- **Writing to a CDR** — caller's `openehr/client/ehr/` responsibility.
- **Validating during generation** — separate `validation.ValidateComposition` call (cross-checked by PROBE-027).
- **Runtime federated slot-fill repository** — same compromise as validation.
- **Multi-language term translation** — caller seeds `Options.Language`.

- **Probes:** PROBE-027 — `instance.Generate` checked with `validation.ValidateComposition` (COMPOSITION roots) or `validation.Validate`, and with `validation.ValidateRM`, across the vendored OPTs that compile.

---

## REQ-101 — Generic OPT-driven composition builder

The SDK **MUST** ship a composition-specific authoring layer at `openehr/composition/` that produces an in-memory `*rm.Composition` graph driven by a compiled OPT.
The builder adds the composition options and the path-first API to the generator of [§ REQ-107](#req-107--template-driven-rm-instance-example-generator), which builds every skeleton: every implicit RM attribute, primitive default and LOCATABLE identity comes from `instance.Generate`.

### Scope

Two entry points:

1. **`NewSkeleton(ctx, c, opts...) (*rm.Composition, error)`** — produces a structurally-conformant default composition whose leaves hold REQ-107's example values, and, where the template sets a default value that validates, that default ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator) *Trust model*).
   Delegates to `instance.Generate` with the policy `WithPolicy` selects, `Medium`, the instance package's default, when none is given, and unwraps the root via `instance.AsComposition`.
2. **`NewBuilder(ctx, c, opts...) (*Builder, error)`** — seeds a `Builder` from a `Large` skeleton short of its repeats and its optional RM attributes (*Skeleton policy* below), then accepts `Set(path, value)` calls.
   `Build()` returns the populated graph and aggregates per-path errors.

### Contract

- **Composition-specific options** — `WithLanguage(code)`, `WithTerritory(code)`, `WithComposer(p)`, `WithCategory(c)`, `WithNow(t)`.
  The first three translate to fields on `instance.Options` and pin `Composition.language` / `.territory` / `.composer`; `WithCategory` sets `Composition.category` on the generated skeleton, over any category the OPT declares.
  `WithNow` injects the clock used for `EVENT.time` and `EVENT_CONTEXT.start_time` defaults so tests stay deterministic.
- **Uid source** — `WithUIDSource(src)` **MUST** pass `src` to `instance.Options.UIDSource`, for `NewSkeleton` and `NewBuilder` alike, so every uid the skeleton carries comes from `src` ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator) *Identity*); a nil `src`, or no `WithUIDSource`, leaves the generator's random fallback in force.
  With `instance.UIDsFrom` on a fixed source, `WithNow` and, under `instance.RandomFill`, `WithValueSource`, a skeleton is reproducible as REQ-107 *Reproducible uids* says.
- **Skeleton policy** — `WithPolicy(p)` selects the [§ REQ-107](#req-107--template-driven-rm-instance-example-generator) `instance.Policy` the entry point passes to `instance.Generate`.
  With no `WithPolicy`, `NewSkeleton` **SHOULD** use `Medium`, the zero value, so a skeleton holds what `instance.Generate` builds from default `Options`, and `NewBuilder` `Large`, the one policy it accepts, for the reason below.
  `Set` routes a value into a node the skeleton already holds and does not build a missing one, so it needs a member of every optional archetype and of every named use of one, which only `Large` builds.
  `NewBuilder` **MUST** seed a `Builder` with what `Large` builds short of two parts ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator)):
  - the second member `Large` gives a node that may occur more than once, so each node gets `max(occurrences.lower, 1)` members (*Members of a list*);
  - the optional RM attributes `Large` writes where the OPT does not name them (*Optional RM attributes under `Large`*).

  No field of `instance.Options` and no composition option selects that skeleton.
  A later use of one identity that pins no name ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator) *What a visit writes*: its archetype id or `node_id`) has the compiled path of the first, so `Set` reaches the first use's member alone, and the member `Large` builds for the later use keeps the values the skeleton gave it.
  `NewSkeleton` and `NewBuilder` **MUST** accept and refuse the policy `WithPolicy` gives as this table says, a refusal returning no skeleton or `Builder`, before either builds a skeleton:

  | `WithPolicy` gives | `NewSkeleton` | `NewBuilder` |
  |---|---|---|
  | `Large` (and so `Complete`) | accepts | accepts |
  | `Medium` or `Required` (and so `Minimal`), an explicit `WithPolicy(instance.Medium)` included, though `Medium` is the zero value | accepts | refuses with an error that wraps `composition.ErrUnsupportedPolicy` and not `instance.ErrUnsupportedPolicy`, since REQ-107 defines both |
  | a `Policy` value that is none of the three | refuses with an error that wraps `composition.ErrUnsupportedPolicy` and `instance.ErrUnsupportedPolicy`, since REQ-107 defines no other | refuses the same way |

- **Path-first API** — `Set(path string, v any) error` looks up the compiled node via `Compiled.NodeAt(path)` and routes the assignment through the parent attribute.
  Typed helpers `SetText`, `SetQuantity`, `SetCodedText` wrap the most common DV shapes.
  A path is a canonical OPT path as the compile step produces it, predicate-bracketed segments included where the OPT pins archetype roots or at-codes.
- **Type enforcement** — `Set` **MUST** accept a value:
  - whose type the binding tiers of [§ REQ-102](#req-102--composition-validation) *Structural — alternatives* bind to a node the template allows at the path, one whose occurrences are not 0..0:
    - the node `Compiled.NodeAt(path)` returns;
    - or another alternative the OPT lists at the same path under the same attribute (the tiers bind by the type alone, to the first node of the first tier that admits it; the trial within a tier is the validator's);
  - and one the RM attribute the path writes can hold, its RM type conforming to that attribute's type in the pinned RM BMM, or, where that type is a generic parameter the template leaves open, to the parameter's constraint (`DV_ORDERED` for the bounds of a bare `DV_INTERVAL`).

  So a `*rm.DVCodedText` is accepted at a DV_TEXT path; a concrete data value at a node the OPT types as an abstract class or leaves untyped; a `*rm.DVDuration` at an ELEMENT value the OPT types with the AOM 1.4 short name `DURATION`; a `DV_INTERVAL<DV_COUNT>` at a `DV_INTERVAL<DV_ORDERED>` node; and a `*rm.DVQuantity` at an ELEMENT value the OPT allows as DV_CODED_TEXT or DV_QUANTITY.
  `Set` **MUST** return `ErrTypeMismatch` for any other value:
  - one that binds to no such node (e.g. a `*rm.DVText` passed at a DV_QUANTITY path);
  - one the attribute cannot hold (e.g. a `*rm.DVDuration` at the `DURATION` primitive under `DV_DURATION.value`, which holds a String);
  - one at a path below the root that writes no RM attribute (a template may constrain a BMM function, such as `DV_PROPORTION.is_integral`, as if it were one);
  - one that is not an RM value;
  - and a nil or typed-nil value.

  `Set` **MUST** return `ErrUnknownPath` for a path the template does not have.
  The root path writes no attribute and is not settable: `Build` **MUST** refuse a value set there with `ErrInvalidPath`.
  Each wraps its context with `fmt.Errorf("...: %w", err)`, so a caller compares it with `errors.Is`.
- **Aggregated errors** — `Set` records errors against the builder but does NOT short-circuit; subsequent assignments still attempt.
  `Build()` returns the populated `*rm.Composition` plus the aggregated error (joined via `errors.Join`) so callers can recover every faulty path in one round-trip rather than chasing one error at a time.
- **TemplateID propagation** — `Builder.TemplateID()` returns the OPT's `Compiled.TemplateID()`, suitable for the REST `composition.WithTemplateID` option so the CDR validates against the same template.

### Out of scope

- **Per-template generated Go structs.** The builder stays generic: consumers do not import generated per-template structs through this package.
- **FLAT / STRUCTURED ingest.** Caller decodes externally (REQ-053) and feeds the resulting `*rm.Composition` through validation.
- **Slot resolution against a federated archetype repository.** Same compromise as REQ-102 / REQ-107: pinned slot fills come from the OPT.
- **Encoding to wire bytes.** Callers run `canjson.Marshal` / `canxml.Marshal` themselves ([REQ-013](module-layout.md#req-013--building-block-independence)).
- **Validating during Build.** A `Build()` result is validated by a separate `validation.ValidateComposition` call; the builder is sound by construction but not a validator.

- **Probes:** PROBE-023 — `composition.NewBuilder` + `Set` → `Build` → `canjson.Marshal` → `canjson.Unmarshal` → re-marshal round-trip preserves values at key paths.

---

## REQ-111 — Public compiled-template bridge

The compiled-template form (`templatecompile.Compiled`) is the argument the template-driven entry points take, among them the composition builder (REQ-101, `NewBuilder` and `NewSkeleton`), the RM instance synthesiser (REQ-107, `Generate`), the validator (REQ-102, `Validate` and its typed wrappers), and the AQL static lint (REQ-109, `lint.Options.Compiled`).

The SDK **MUST** ship a public constructor that turns an OPT, read into the operational template model of [§ REQ-176](#req-176--adl-14-operational-template-model), into that compiled form without exposing any `internal/` package, so external consumers can drive the full read → compile → build → validate pipeline through public packages alone.
The reader's tolerances are [§ REQ-176 § What the reader tolerates](#what-the-reader-tolerates)'s, and its bounds [§ REQ-108](#req-108--untrusted-document-bounds)'s.

### Surface

```go
// Package github.com/cadasto/openehr-sdk-go/openehr/templatecompile

// Compiled is the public, externally-constructable compiled template.
// It is a type alias of the internal compiled form, so values returned
// by Compile are accepted as-is by composition, instance, validation
// and aql/lint, with no conversion.
type Compiled = <internal compiled form>

func Compile(opt *aom14.OperationalTemplate, opts ...Option) (*Compiled, error)

type Option func(*config)
func WithRMInfo(l rminfo.Lookup) Option       // custom RM-info source
func WithoutImplicitAttributes() Option        // OPT-declared attributes only
func WithStrict() Option                       // refuse the weaker readings (§ Reading the model)

var ErrInvalidInput error  // re-export; errors.Is works across the boundary
var ErrPathNotFound error

// Path lookup (§ Path lookup), methods of Compiled:
//   Resolve(p template.Path, opts ...ResolveOption) (*CompiledNode, error)
//   ValidatePath(p template.Path, opts ...ResolveOption) error
type ResolveOption = <internal resolve option>
func WithStrictPaths() ResolveOption           // a predicate-less segment with several children is ambiguous

// Introspection tree — also public, for form generation, path discovery,
// and custom mapping/validation. Aliases of the engine node types.
type CompiledNode = <internal compiled node>
type CompiledAttribute = <internal compiled attribute>
```

The committed public surface is `Compile`, `Compiled`, the introspection tree (`CompiledNode` / `CompiledAttribute`), the path lookup, the functional `Option`s, and the two sentinel errors.
All are aliases of the engine types, so a downstream package can navigate the compiled template (`Compiled.Root`, `NodeAt` by the exact AQL path the compile wrote, or `Resolve` by a path `template.ParsePath` reads → `CompiledNode.Attributes` → `CompiledAttribute.Children`) and hold the node types in its own signatures.
Pre-1.0 the one area expected to change is multi-language term resolution (`CompiledNode.Term`'s `lang` parameter, § *Term definitions and bindings* below); the surface is otherwise stable.
Everything reachable as a method on `Compiled` / `CompiledNode` / `CompiledAttribute` is committed (including the slot accessors `SlotIncludes` / `SlotExcludes` / `SlotRules` / `AllowsArchetypeID` / `ExampleSlotFillArchetypeID` and the default value `DefaultValue`); the compile engine and free helpers that are not methods on the exported types (e.g. `IsAOMPrimitiveShortName`) stay internal.

### Reading the model

In its default mode, `Compile` **MUST** compile every model `aom14.ReadOperationalTemplate` reads in its default mode from an OPT that [§ REQ-176 § What the reader tolerates](#what-the-reader-tolerates) says it reads, reading the values § *What the compile step tolerates* names as it says, except a model it refuses:

- two nodes under different attributes that share an AQL path (§ *Node names and name predicates*);
- a refusal in the `Compile` list below, a definition that names no RM type among them.

`Compile` **MUST**:

- read an `rm_attribute_name` of `null_flavor`, on a node whose `rm_type_name` is `ELEMENT`, as the attribute `null_flavour`, unless that node also declares an attribute named `null_flavour`.
  It is the US spelling some OPTs write, which no RM class declares; read so, its constraint applies to the RM attribute `ELEMENT.null_flavour`, where CDRs that accept such a template apply it.
  Where the node declares both, `null_flavor` is kept as written, so the node does not declare `null_flavour` twice: the `null_flavour` constraint applies, and the compile step marks `null_flavor` (§ *Attributes the RM does not have*).
  Every other attribute name is kept as written;
- read each identifier the OPT writes, each `rm_attribute_name`, `rm_type_name` and `node_id`, and the `<value>` of an `archetype_id` or a `template_id`, and each code, terminology id, list entry and term text, without its leading and trailing white space, as Unicode defines it (XML's space, tab, carriage return and line feed among it).
  A pretty-printed OPT can break the line inside such an element, writing `<rm_attribute_name>defining_code` with `</rm_attribute_name>` on the next line, and no identifier holds white space at either end, so one that holds only white space reads as an empty one.
  The model's decode reads an `xsi:type` as written, and the reader in its default mode without white space around it ([§ REQ-176 § What the reader tolerates](#what-the-reader-tolerates));
- close each `occurrences`, `existence` and `cardinality` interval into a `Multiplicity`, which counts whole items, closed on each bounded side: an excluded end becomes the closed end it denotes (`0..<3` is `0..2`, `>0..*` is `1..*`), and an unbounded side keeps no bound whatever its included flag says.
  A numeric range keeps its included flags as `NumericRange` does, and a temporal range as `TemporalRange` does ([§ REQ-103](#req-103--primitive-constraint-introspection));
- refuse, with an error wrapping `ErrInvalidInput`:
  - a model with no `template_id`, or with no `definition` or one that names no RM type;
  - and an interval that is inverted, that closing leaves empty, such as `>2..<3`, or whose excluded end denotes a closed end the model's 32-bit `Integer` cannot hold (`>` its largest value on a lower side, `<` its smallest on an upper one), on every platform.

An interval the model holds at its zero value, as it holds one the template leaves out, is read as absent.
The model's decode reads `T_ARCHETYPE_ROOT` ([§ REQ-176](#req-176--adl-14-operational-template-model) *Shapes the pinned BMM lacks*) and the interval flags an OPT leaves out ([wire.md § REQ-169](wire.md#req-169--interval-boundary-flag-defaults)).

#### A node that names no RM type

The compile step reads the class of a node with no `rm_type_name` as the type of the attribute that holds the node, or, where that attribute is typed by a generic parameter the holding node leaves open, as the type the BMM constrains that parameter to (`ITEM_STRUCTURE` for a `POINT_EVENT`'s `data`).
So it marks, types, flags as mandatory (`CompiledAttribute.Required`) and injects the attributes of such a node as it does those of a node that names that type, and the node's own RM type stays empty.
A definition that names no type is no definition, which the compile step refuses (above).

#### What the compile step tolerates

In its default mode `Compile` reads these values the model holds in a weaker form:

| The model holds | `Compile` reads it | Given `templatecompile.WithStrict` |
|---|---|---|
| a `CONSTRAINT_REF` with no `reference`, or a blank one | with an empty `Reference` | refused |
| a `timezone_validity` that names no `VALIDITY_KIND`, which only a model built in code holds, since the reader leaves one out ([§ REQ-176 § What the reader tolerates](#what-the-reader-tolerates)) | as optional ([§ REQ-103](#req-103--primitive-constraint-introspection) *Validate contract*) | refused |
| a mandatory `timezone_validity` on a `C_DATE` | as optional ([§ REQ-103](#req-103--primitive-constraint-introspection) *Validate contract*) | refused |
| a temporal range bound that is not ISO 8601 text of its type | with that side unbounded | refused |
| a `C_PRIMITIVE_OBJECT` with no `item` | as no constraint | refused |
| a default value that addresses no node, or that canonical JSON cannot hold, such as a `NaN` magnitude (below) | without it | refused |

Given `templatecompile.WithStrict`, `Compile` **MUST** refuse a `CONSTRAINT_REF` with no `reference` or a blank one, a `timezone_validity` that names no `VALIDITY_KIND` or is mandatory on a `C_DATE`, and a temporal range bound that is not ISO 8601 text of its type.
It refuses each of these, and the table's other rows, with an error wrapping `ErrInvalidInput`.
In every mode it refuses a model nested deeper than [§ REQ-108 § Compile step and path lookup](#compile-step-and-path-lookup-openehrtemplatecompile) allows.

The compile step builds each primitive constraint from the model's primitive classes, and each slot's rules from its includes and excludes ([§ REQ-103](#req-103--primitive-constraint-introspection)).
An `ARCHETYPE_SLOT` with no `node_id` takes as the predicate of its AQL path segment the archetype-id pattern of its first include, as its slot rules read it.
The term definitions and bindings come from the model's archetype roots (§ *Term definitions and bindings*).

The compiled template carries as well:

- a `C_CODE_REFERENCE`, as a `constraints.CodeReference` primitive constraint ([§ REQ-103](#req-103--primitive-constraint-introspection));
- each default value of the template's `<constraints>`, the `default_value` of a `T_COMPLEX_OBJECT`, on the compiled node that its `T_ATTRIBUTE`'s `differential_path` and `rm_attribute_name` address: the first child, under the attribute of that name on the node the path reaches, of the `T_COMPLEX_OBJECT`'s RM type, or of its default value's RM type where it names none, a child's generic parameters set aside.
  So `DV_INTERVAL` matches `DV_INTERVAL<DV_COUNT>`.
  The compile step walks the `differential_path` from the definition's root, past the definition's archetype id in brackets where the path opens with it, by the rules `Compiled.Resolve` applies (§ *Path lookup*).
  Unlike `Resolve`, it also reads a name in a segment's predicate, `[at0004, 'Systolic']` or `[at0004 and name/value='Systolic']`, which matches a node of that node id or archetype id whose template-level name (§ *Node names and name predicates*) is that name, each run of white space inside the predicate read as one space; `template.ParsePath` reads no name predicate.
  A default value whose path opens otherwise, or that reaches no node, no attribute of that name or no child of that RM type, addresses no node, and `Compile` skips it, as it skips one canonical JSON cannot hold (§ *What the compile step tolerates*).
  `CompiledNode.DefaultValue` returns a placed default value, a new value on each call; a `T_COMPLEX_OBJECT` with no `default_value` places nothing.

### Path lookup

`template.ParsePath` **MUST** accept the following subset of the openEHR path syntax and reject anything else with an error wrapping `template.ErrPathSyntax`:

- Absolute paths only, with a leading `/`.
- Segments are RM attribute names: `/content`, `/data/events/data/items`.
- Optional **archetype node predicate** on a segment: `/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]` or `/items[at0001]`.
- A segment carries at most one predicate (no `[at0001,name="Systolic"]`).
- A trailing slash is not permitted.
- AQL name predicates (`[name='...']`) are not part of this grammar: that surface belongs to `openehr/aql/`.

`Compiled.Resolve` **MUST** resolve a path that `template.ParsePath` reads by walking the compiled tree from its root, attribute by attribute:

- A segment matches the `CompiledAttribute` of its name, exactly and case-sensitively.
- A segment's predicate matches a child whose `NodeID` or `ArchetypeID` equals it.
- A segment without a predicate takes the attribute's first child, in document order.
  With `templatecompile.WithStrictPaths` it returns an error wrapping `template.ErrAmbiguousPath` where the attribute has more than one child; without the option it never returns `template.ErrAmbiguousPath`.
- A segment that matches nothing, an unknown attribute or an unmatched predicate, returns an error wrapping both `template.ErrPathNotFound` and `templatecompile.ErrPathNotFound`, as does the depth refusal of [§ REQ-108 § Compile step and path lookup](#compile-step-and-path-lookup-openehrtemplatecompile).

`Compiled.ValidatePath` is `Resolve` without the node, a shorthand for a code generator's precondition.
`Compiled.NodeAt` is the lookup of an AQL path by the exact text the compile step wrote.
`openehr/template` **MUST** export the three path sentinels, `ErrPathSyntax`, `ErrPathNotFound` and `ErrAmbiguousPath`, and each error that carries one wraps it, so a caller compares it with `errors.Is`.

The consuming packages name the public `*templatecompile.Compiled` in their **exported** signatures, so the rendered API docs link the public package; because `Compiled` is a type alias, the public and the engine name denote the identical type and no conversion is needed.

### Term definitions and bindings

The SDK **MUST** expose structured accessors for the archetype term definitions and the external terminology bindings an OPT carries, without resolving any of them against a live terminology.
The compile step reads them from the `term_definitions` and `term_bindings` of each archetype root in the model and holds them as `template.ArchetypeTerm` and `template.TermBinding` records.
`Compiled.TermLang(nodeID, lang)` resolves an at-code's term text scoped to the composition root archetype, and `CompiledNode.Term(code, lang)` walks up through the parent archetype roots for a context-sensitive lookup.
An ADL 1.4 OPT carries a single document language (`Compiled.Language()`): when `lang` is empty or equals it, the OPT's `Items` map (`text`, `description`, …) is returned, and when `lang` differs and the OPT holds no translation, the document-language term **MUST** be returned, with no error; a caller tells absence by the `ok` result alone.
`Compiled.TermBindingsForNode(nodeID)` filters the compile-time flattened binding list to the entries whose `NodeOrPath` equals the at-code, or whose AQL-like locator contains `[nodeID]` or ends with `/nodeID`.
Looking a code up in SNOMED CT, LOINC, ICD or another external terminology is out of scope: the SDK surfaces the bindings the OPT carries and no more.

### Attributes the RM does not have

The compile step **MUST** mark, through `CompiledAttribute.NotInRM()`, each attribute the OPT declares whose name is neither a property nor a function of its node's RM class, of an ancestor of that class, or of a descendant of it.
A node that names no RM type has the class § *Reading the model* reads it as.
The class is read without generic parameters; the properties are those the compile's RM lookup gives, the ancestors and descendants those its class hierarchy (`rminfo.Hierarchy`) gives, and the functions those `rminfo.IsNonStorableAttr` names ([§ REQ-112 *Driver*](#driver)).
`ISM_TRANSITION.name` is one.
`EVENT.offset`, a function, is not, and nor is `ITEM_STRUCTURE.items`, a property of the descendants `ITEM_TREE` and `ITEM_LIST`.
The compile **MUST NOT** mark an attribute of a class the lookup does not know, any attribute when the lookup gives no class hierarchy, or an attribute it injects.
No instance of the node's class can carry a marked attribute, so the model's reader refuses no OPT that declares one, in either mode, and the validators leave it out ([§ REQ-171 *One walk*](#one-walk)) and list it in `Coverage` ([§ REQ-171 *Coverage*](#coverage)).

### Template terms

The generator ([§ REQ-107](#req-107--template-driven-rm-instance-example-generator)) and the validators ([§ REQ-102](#req-102--composition-validation)) read an OPT's counts in these terms:

- An OPT child is *allowed* unless its occurrences upper bound is 0.
- The OPT *prohibits* an attribute whose existence upper bound is 0, or that has at least one OPT child and allows none of them.
- On a multi-valued attribute, existence says whether the container is there at all, and cardinality how many members it holds (AOM 1.4, *Attribute Node Types*: existence "says whether the container object will be there (at all)", cardinality "how many items" the container holds).
  So a cardinality lower bound counts the members of a container that exists and does not ask for the container: ADL 1.4 makes such a container "optional, but if present" bound by its cardinality (*Cardinality*).
- An allowed child, not a slot, of an attribute the OPT declares multi-valued *asks for its container* where its occurrences lower bound is bounded and at least 1, unless the RM types the list's items as primitive values, such as a DV_MULTIMEDIA's `data`, to which no item binds ([§ REQ-102](#req-102--composition-validation) *A mandatory child under an optional list*).
  ADL 1.4 calls such a child mandatory in its container (*cADL*, *Occurrences*: the `PERSON` whose occurrences are `{1}` "is mandatory in the `GROUP`"), as AOM 2 does (validity rule VACMCO, "every mandatory child object"), and leaves an optional container to its existence; that the child also asks for its container is the reading this specification chooses.
- The child of a single-valued attribute never asks for its container: ADL 1.4 says the attribute's existence already decides how often that child occurs (*Occurrences*).
  Nor does a child that states no occurrences, or occurrences with an unbounded lower bound, which the validators ask for no member ([§ REQ-102](#req-102--composition-validation) *An existence lower bound alone asks for no member*).
- An attribute is *required* when the BMM marks it mandatory, its existence or cardinality lower bound is bounded and at least 1, or a child of it asks for its container.

### Placement (ADR 0010)

The constructor lives in the sibling package `openehr/templatecompile`, not in `openehr/template`, since a `Compile` there would form an import cycle with the compile engine and bring `openehr/rm/rminfo` and `openehr/aom/aom14` into the closure [module-layout.md § REQ-013](module-layout.md#req-013--building-block-independence) keeps them out of; [ADR 0010](../adr/0010-public-compiled-template-bridge.md) records the placement.

### Node names and name predicates

An OPT node may pin its runtime `LOCATABLE.name` by constraining the `name` attribute to a fixed value — in ADL 1.4 XML, a `C_STRING` whose raw wire `list` holds **exactly one non-blank entry** (`<item xsi:type="C_STRING"><list>Husten</list></item>`), taken trimmed, and whose list is closed.
Fixedness is a property of what the OPT declared, so it is judged on the wire list *before* any blank-entry normalisation: a multi-entry list is a choice even when all but one entry is blank, and a lone blank entry pins nothing.
An open list fixes nothing either: a one-entry list whose `list_open` is true admits names outside it, so it pins no name.
`list_open` is read as [§ REQ-103](#req-103--primitive-constraint-introspection) reads it for `CString.ListOpen`, so an absent element leaves the list closed.
This **template-level node name** is distinct from the *archetype's* concept term: a template that fills a slot with the same archetype more than once gives each occurrence its own name, while the archetype concept is identical across all of them.
It is the only thing that distinguishes those siblings, and both the reference WebTemplate `id` ([§ REQ-106](#req-106--webtemplate-json-export)) and the AQL paths addressing them depend on it.

The compile step **MUST** carry the template-level node name through the compiled tree, where `CompiledNode.NodeName` returns it, and the empty string, never the archetype concept term, for a node that constrains no fixed name; the compiled tree uses it as the disambiguator described below.

#### Name predicates in AQL paths

An AQL node predicate can carry a name alongside the archetype node id — `[openEHR-EHR-SECTION.adhoc.v1,'Symptome']` — which is how openEHR addresses one of several siblings that share an archetype node id.

Where a node pins a template-level name, the compiled AQL path **MUST** carry the name predicate on that node's segment.
The trigger is the **pinned name alone**, not sibling collision: the reference emits the predicate on every named node — including a sole child, and siblings whose archetype node ids already differ — so a rule conditioned on siblings sharing a node id emits too few.
Where a node pins **no** name, the SDK **MUST NOT** invent a predicate; this includes siblings that share an archetype node id and pin no distinct names, which legitimately share a path and are distinguished positionally, and whose compiled form **MUST** retain each of them rather than discarding the collision.

Path shape is consumed by the instance generator ([REQ-107](#req-107--template-driven-rm-instance-example-generator)) and FLAT/STRUCTURED path resolution ([REQ-053](wire.md#req-053)).
Composition validation does not consume it: its `Issue.Path` follows [REQ-102](#req-102--composition-validation) § Issue paths, which writes a name predicate only where siblings share an archetype id.
PROBE-075 holds this rule against reference Web Templates ([§ REQ-106](#req-106--webtemplate-json-export)).

- **Probes:** PROBE-022 (path lookup against a fixture OPT); the builder round-trip is PROBE-023.

## REQ-106 — WebTemplate JSON export

**WebTemplate** is a consumer-facing, UI-oriented projection of an operational template: a lossy, flattened JSON view listing each node's stable `id`, cardinality, AQL path, and the leaf **inputs** a form renders.
It is a **vendor de-facto** serialisation (Better → EHRbase), **not** a normative openEHR artefact — only the downstream FLAT/STRUCTURED *serialization* it enables is standardised (ITS-REST Simplified Formats).
The SDK mirrors it from a single locked reference implementation ([ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)) and keeps it stable for consumers, as § *Output shape* says.

The SDK **MUST** provide a WebTemplate JSON export that projects a compiled operational template — the public `*templatecompile.Compiled` (REQ-111) — into the **EHRbase `openEHR_SDK` v2.3** WebTemplate shape.
The export reads only the compiled form, never `.oet` / `.opt` / `.t.json` authoring artefacts, and **MUST NOT** mutate it.

### Surface

```go
// Package github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate

// Build projects a compiled OPT into the typed WebTemplate tree.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error)

// Marshal is Build followed by deterministic JSON encoding.
func Marshal(c *templatecompile.Compiled, opts ...Option) ([]byte, error)

type Option func(*config) // reserved: takes no options yet (single-language compiled input; fixed schema version)
```

`Build` returns the typed tree for callers that post-process before encoding; `Marshal` is the common path.
A nil or empty compiled input, or an unresolvable default language, **MUST** return an error (never panic).

### Output shape

The exported root **MUST** carry `templateId`, `version` (the string `"2.3"`), `defaultLanguage`, `languages`, and a `tree` of nodes.
Each node **MUST** carry `id`, `rmType`, `min`, `max` (with `-1` denoting unbounded), and `aqlPath`; and where the compiled template supplies them, `name` / localized names, `nodeId`, `inputs`, and `children`.
A node with a `nodeId` takes its `name` and localized names from the template-level name its template pins on it, where it pins one, else from its archetype term ([§ Node names and name predicates](#node-names-and-name-predicates)).
JSON field names are camelCase.
Encoding **MUST** be deterministic: the same compiled template produces byte-identical output across runs and SDK patch releases (fixed field order; map keys sorted).

The exported tree is **not** a 1:1 mirror of the compiled OPT; it follows the reference implementation's node model.
The transform **MUST**:

- keep `COMPOSITION`, the ENTRY types (`OBSERVATION` / `EVALUATION` / `INSTRUCTION` / `ACTION` / `ADMIN_ENTRY`), `EVENT_CONTEXT`, and `CLUSTER` as nodes;
- keep the event types (`EVENT` / `POINT_EVENT` / `INTERVAL_EVENT`) as nodes **except** an abstract `EVENT` constrained to at most one occurrence, which is **lifted**.
  The reference emits no node for such a degenerate wrapper, while retaining its `events[…]` path segment on descendants and its synthesized `time` leaf (a repeating `EVENT`, and any concrete `POINT_EVENT` / `INTERVAL_EVENT` even at `max=1`, stays a node);
- give each `ELEMENT` the leaf or node its value alternatives call for, as the table below says;
- **drop** the pure structural wrappers (`HISTORY` and the `ITEM_STRUCTURE` family — `ITEM_TREE` / `ITEM_LIST` / `ITEM_SINGLE` / `ITEM_TABLE`) as nodes while **folding their node predicates into the `aqlPath`** of the descendants they enclose;
- emit RM-attribute values that carry data directly (e.g. `EVENT_CONTEXT.start_time` / `setting`, `EVENT.time`, `ACTION.time`, the ENTRY `language` / `encoding` / `subject`, `INTERVAL_EVENT.math_function` / `width`) as leaves whose `id` is the attribute name.

The `aqlPath` therefore carries every archetype-id and at-code node predicate along the retained path.

An `ELEMENT` gives, by the value alternatives its template constrains (the `C_OBJECT`s of its `value` attribute):

| The ELEMENT's `value` has | The export emits |
|---|---|
| one alternative | a single leaf that **collapses** the ELEMENT: the alternative's type as `rmType`, the ELEMENT's node id as `nodeId`, and the ELEMENT path extended by `/value` as `aqlPath` |
| two alternatives, one `DV_CODED_TEXT` and one `DV_TEXT`, in either order | that leaf for the `DV_CODED_TEXT`, its `code` input `listOpen`, followed by an `other` TEXT input |
| any other two or more alternatives | the ELEMENT as a node without `inputs`, holding one child per alternative in template order: its type as `rmType`, the ELEMENT's `nodeId` and names, the ELEMENT path extended by `/value` as `aqlPath`, the alternative's own occurrences as `min`/`max`, and its `inputs` (§ *`inputs`*) |
| none | the ELEMENT as a node holding one child per type the reference offers such an ELEMENT, in its order: `DV_TEXT`, `DV_CODED_TEXT`, `DV_MULTIMEDIA`, `DV_PARSABLE`, `DV_STATE`, `DV_BOOLEAN`, `DV_IDENTIFIER`, `DV_URI`, `DV_EHR_URI`, `DV_DURATION`, `DV_QUANTITY`, `DV_COUNT`, `DV_PROPORTION`, `DV_DATE_TIME`, `DV_TIME`, `DV_ORDINAL`, `DV_DATE`; each with no `nodeId`, `min` 0 and `max` 1, the ELEMENT's names, the ELEMENT path extended by `/value` as `aqlPath`, and the `inputs` of an unconstrained value of the type |

The children of one ELEMENT share one `aqlPath`, as the reference's do, so a consumer tells them apart by `id`.
An unfilled slot for an ELEMENT archetype is no ELEMENT: like any unfilled slot, it gives no node.

Every ACTION **MUST** carry the reference's in-context `ism_transition` node, with no `nodeId`, since the RM requires the transition and the format has no other place for it:

| The ACTION's template | The `ism_transition` node |
|---|---|
| models archetyped `ism_transition[atNNNN]` nodes, one per careflow step | merges them as the reference does: `min` and `max` from the first, and the leaves `careflow_step` (each step's careflow-step code, or its node id and name where it constrains none, with the `currentStates` that step admits), `current_state` (every step's states, in step order) and `transition` (the first step's own transition leaf where that step constrains one, else the open one) |
| models none, or only ones constraining no attribute of the transition | holds the transition's `current_state` and `transition` as leaves |

The archetyped `ism_transition[atNNNN]` nodes stay beside the merged node.

### `id` generation (ADR 0014)

The node `id` is the FLAT-path segment consumers bind to, so its stability and cross-implementation fidelity are the export's load-bearing property.
The `id` **MUST** mirror the locked EHRbase reference: a lower-snake sanitisation of the node's default-language display name.
Two spellings below depart from it, a duplicate sibling's suffix and an interval value alternative's `id` ([ADR 0036](../adr/0036-webtemplate-id-divergences.md)).

For a node that pins a **template-level name**, that name is the display name the `id` derives from — not the archetype's concept term, which is shared by every occurrence of a reused archetype and so cannot distinguish siblings; see [§ REQ-111 § Node names and name predicates](#node-names-and-name-predicates), which owns the naming rule.

A pinned name is the disambiguator wherever one exists, so no suffixing is needed for it.
Where siblings would **still** collide, with no pinned name and one shared display name, as two ELEMENTs that both sanitise to `dv_text` do, the export **MUST** make their `id`s unique with the numeric suffix of the *Simplified Formats* specification (§ Node ID Generation Rules, rule 7):

- the first claimant, in document order, keeps the bare `id`;
- each later claimant takes the next **free** `<id>_N`, N counting from 1 (`dv_text`, `dv_text_1`, `dv_text_2`);
- "free" is over the whole sibling list: a sibling whose own name sanitises to `<id>_N` ("Blood Pressure 1" is `blood_pressure_1`) keeps it, before or after the claimants, and the claimant skips to the next N, since renaming onto it would recreate the collision.

This spelling departs from the reference, which writes the second claimant `<id>2`; [ADR 0036](../adr/0036-webtemplate-id-divergences.md) gives the reasons.
How decode reads a body keyed in the reference's spelling is [§ REQ-053](wire.md#req-053)'s.
Emitting duplicate sibling `id`s is **never** permitted; an export that still cannot make siblings unique **MUST** fail with a typed error rather than emit an ambiguous tree.

The child an ELEMENT gives one of its value alternatives ([§ *Output shape*](#output-shape)) takes the reference's `id` for the alternative's type: the type without its `DV_` prefix, lower-cased, followed by `_value` (`quantity_value`, `coded_text_value`).
Two alternatives of one type take the numeric suffix (`text_value`, `text_value_1`).
An interval of a type **MUST** take `interval_of_<type>_value` (`interval_of_date_time_value`), the spelling of the reference's own IPS test Web Template.
It departs from the reference's code, which writes `interval<dv_date_time>_value` ([ADR 0036](../adr/0036-webtemplate-id-divergences.md)).

### `inputs` (core clinical subset)

Each `ELEMENT` value constraint (via REQ-103 primitive constraints) **MUST** map to `inputs[]` for the core clinical datatypes:

- `DV_TEXT` (one `TEXT` input):
  - when the value's `C_STRING` carries a `List`, the input carries it as `list`, each entry with the list string as both `value` and `label`, as the `Corona_Anamnese` golden spells it;
  - whenever that `C_STRING` sets `CString.ListOpen` ([§ REQ-103](#req-103--primitive-constraint-introspection)), with a `List` or without one, the input carries the flag as `listOpen`, as the reference reads it;
  - a false `listOpen` is left out, an absent optional field under § *Conformance and deviations*;
- `DV_CODED_TEXT` (a `code` CODED_TEXT input with `list` / `listOpen` / `terminology`);
- `DV_QUANTITY` (`magnitude` DECIMAL + `unit` CODED_TEXT with validation);
- `DV_COUNT` (INTEGER/COUNT);
- `DV_ORDINAL` (an ordinal `list`);
- `DV_DATE_TIME` / `DV_DATE` / `DV_TIME` (temporal with pattern validation);
- `DV_BOOLEAN` (BOOLEAN);
- `DV_DURATION` (pattern-ordered INTEGER component fields);
- and `DV_PROPORTION` (`numerator` + `denominator`, with the percent-kind–derived denominator bound).

Datatypes outside this subset (e.g. `DV_MULTIMEDIA`, `DV_PARSABLE`, `DV_IDENTIFIER`, `DV_INTERVAL`) **MUST** emit the node **without** `inputs`, and no error — the omission is a documented gap, recorded so consumers can distinguish "no inputs" from "unsupported".

### Conformance and deviations

Conformance against the reference is **structural, not byte-exact**: PROBE-075 compares the SDK output to the vendored EHRbase fixture on the surface its entry names, and on each input's coded and ordinal list values, a careflow step's `currentStates`, `listOpen`, `terminology`, temporal validation patterns and numeric validation ranges as well.
Any structural difference on that pinned surface is a failure; the following categories of divergence are **permitted deviations**, which PROBE-075 does not count as failures:

1. **Field-level presentation** — JSON field ordering, absent optional fields, and localized-string packaging (`localizedName` / localized maps emitted for the compiled template's single document language only).
2. **Reference-only node metadata** — `termBindings`, `annotations`, and the `inContext` flag on synthesized RM-attribute leaves.
3. **Input-content omissions** — `defaultValue`; DV_DURATION per-field ranges; DV_QUANTITY `precision`; per-unit and multi-unit magnitude validation; list `label` for external-terminology codes; `localizedLabels` and per-item `termBindings`.
4. **Local terminology** — `terminology` is emitted only for external bindings (e.g. `openehr`); the archetype-internal `local` value is omitted, mirroring the reference everywhere but on the merged ACTION `careflow_step` input, where the reference writes it.
5. **Fixture-outlier `min` on in-context leaves** — where a vendored golden marks synthesized RM-attribute leaves `min=1` and the OPT constrains no `existence`, the export **MAY** emit `min=0`.
6. **Reference-set terminology** — for a `C_CODE_REFERENCE`, `terminology` is the one `constraints.CodePhraseWithTerminologyOf` reads ([§ REQ-103](#req-103--primitive-constraint-introspection)), `LOINC(2.73)` for `terminology:LOINC/2.73`.
   The reference takes the URI with its scheme dropped, `LOINC/2.73`, whatever the code phrase names.
7. **Per-careflow-step ISM transition nodes** — the export keeps an ACTION's archetyped `ism_transition[atNNNN]` nodes beside the merged in-context `ism_transition` node; the reference drops them.
8. **ACTION instruction details** — the reference's `instruction_details/activity_id` and `instruction_details/instruction_id` leaves under an ACTION are not emitted.
9. **Coded-text and free-text pair in the `GECCO_Diagnose` golden** — that golden gives `name_des_problems_der_diagnose`, whose value admits `DV_TEXT` then `DV_CODED_TEXT`, as a `DV_TEXT` leaf.
   The export merges the pair into the `DV_CODED_TEXT` leaf, as the reference's code does; the reference's own test reports that node as differing from its golden.
10. **Duplicate sibling ids** — the export writes a duplicate sibling `<id>_1`, `<id>_2` where the reference writes `<id>2`, `<id>3` (§ *`id` generation*).

An informative per-field catalogue behind these categories ships with the package; this section is the contract.

The media type for the format is `application/openehr.wt+json` (documented for consumers).
Emitting the export over a REST endpoint / content negotiation is **out of scope** for this REQ — the package produces the bytes only.
Also out of scope: the WebTemplate → OPT round-trip (the format is lossy by design); the Better camelCase `id` variant; multi-version output; and the shared simplified-template model abstraction.

Templates that **reuse one archetype under a multi-valued slot** (name-distinguished instances) **are exported**.
The compiled template keeps shared-path subtrees, and node identity comes from the template-level node name, with `aqlPath` carrying the matching name predicate ([§ REQ-111 § Node names and name predicates](#node-names-and-name-predicates), [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md)), so each reused sibling is distinct by construction instead of colliding on the shared concept term.

- **Probes:** PROBE-075, structural parity against vendored EHRbase references ([conformance.md](conformance.md)); [PROBE-086](conformance.md#probe-086--upstream-flat-serialisation-parity), upstream FLAT serialisation parity.

## REQ-112 — Template-less Reference Model validation floor

The validators of REQ-102 are **template-driven**: every entry point takes a `*templatecompile.Compiled` as the authoritative driver.
A consumer that persists RM roots binding to no operational template (FOLDER, EHR_STATUS, EHR_ACCESS, and untemplated demographic PARTY on a write path) needs another way to assert RM conformance.
A strict `canjson` typed decode proves JSON↔type correctness but **not** RM invariants: mandatory-attribute omissions decode cleanly, as do `DV_INTERVAL` lower>upper, empty `CODE_PHRASE.code_string`, and `DV_QUANTITY.precision < -1`.

The SDK **MUST** expose a **template-less RM validation floor** beneath the OPT-driven path — an entry point that walks any RM root with the BMM as its sole driver and reports `Issue`s for (a) RM-mandatory attribute breaches and (b) per-RM-type invariants on every node it visits.
The OPT-driven path stays the authoritative template-conformance layer; this floor is what runs when no template applies.
The template-driven entry points include this floor ([§ REQ-102](#req-102--composition-validation) *RM floor*, [ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md)), so a template-valid result is also RM-valid; this floor's own entries run it alone, for a root no template applies to.

### Surface

```go
// Generic entry: any RM value the type registry knows.
func ValidateRM(root any, opts ...ValidateOption) Result

// Typed convenience wrappers (delegate to ValidateRM):
func ValidateRMFolder(folder *rm.Folder) Result
func ValidateRMEHRStatus(status *rm.EHRStatus) Result
func ValidateRMEHRAccess(access *rm.EHRAccess) Result
func ValidateRMDemographic(party rm.Party) Result   // PERSON / ORGANISATION / GROUP / AGENT / ROLE

// Decode-aware entries: decode a canonical document, read the members it
// leaves out (wire.md § REQ-175) and validate in one call; a nil c applies
// the RM rules alone. See § Value-typed mandatory presence.
func ValidateJSON(data []byte, root any, c *templatecompile.Compiled) Result
func ValidateXML(data []byte, root any, c *templatecompile.Compiled) Result

// A decode's own report of missing members, handed to the value-based entries.
type ValidateOption func(/* unexported */)
func WithMissing(pointers []string) ValidateOption

// The same report as either codec's pointer values, read without their text.
func WithMissingPointers[P interface {
	comparable
	Parent() (P, bool)
	Token() string
}](pointers []P) ValidateOption

// Deprecated: ValidateJSON(data, new(rm.EHRStatus), nil) reads presence at
// every level; this entry remains as that call.
func ValidateRMEHRStatusBytes(data []byte) Result
```

The typed wrappers carry the nil-guard contract of [§ REQ-102](#req-102--composition-validation) *Roots beyond COMPOSITION*: `nil_folder` / `nil_ehr_status` / `nil_ehr_access` / `nil_party` distinguish wrapper-side guards from the generic `nil_root` that `ValidateRM(nil)` emits.
A nil guard is a global issue, so its `Path` **MUST** be empty, as on the template-driven guards; `rm_type_unknown` stays at `/`, the root it could not descend.
`ValidateRM` and the typed wrappers behave as [§ REQ-171](#req-171--validation-engine) *Surface* says of the package-level entry points; a caller who needs other limits builds a `Validator`.
A Go value whose type is no RM type the type registry knows surfaces `rm_type_unknown` at `/`; the floor cannot descend further but does not panic.

### Driver

The floor is the walk of [§ REQ-171 *One walk*](#one-walk) with no template: every attribute is unconstrained, so the walk applies the RM rules alone.
It reads values through the generated attribute access of § REQ-171 *Structural access* and takes the structural knowledge from `openehr/rm/rminfo` for the pinned Reference Model.
The RM rules **MUST** read the pinned Reference Model's facts whatever `rminfo.Lookup` compiled a template: a lookup given to `templatecompile.WithRMInfo` changes which attributes the compiled template carries, not which attributes the RM makes mandatory.

- `RequiredAttributes(rmType)` drives the per-node required-set check.
  A single-valued attribute absent (or typed-nil) emits `required`; a multi-valued attribute absent emits `required`, present-but-empty emits `cardinality`.
  An item of a list that holds no RM value, a nil element or a nil pointer, **MUST** be:
  - reported as `required` at the item's own path, on the RM layer with the rule `mandatory`;
  - not walked;
  - and counted toward its list, so a list that holds only such items is present and not empty, and toward the visit limit as one visit, so an item past that limit is reported as `max_nodes` and ends the walk like any value ([§ REQ-171 *Cycles and limits*](#cycles-and-limits)).

  A JSON `null` item decodes to a nil element in a list whose items are polymorphic or held by pointer, such as CLUSTER `items`; in a list of items held by value, such as LOCATABLE `links`, it decodes to an empty item, which the walk enters like any other and reports by its missing attributes.
  The check leaves out an attribute that no serialization carries ([§ REQ-171](#req-171--validation-engine) *One walk*), the set `rminfo.IsNonStorableAttr` names: every BMM function, which the RM computes, and RESOURCE_DESCRIPTION `parent_resource`, the back-reference to the resource that holds the description, whose type AUTHORED_RESOURCE has no concrete class, so no value can be held there.
  A function of a class is one the pinned BMM declares on the class or on an ancestor that is a property neither of the class nor of any class descending from it, such as EVENT `offset`, DV_QUANTITY `is_integral` and DV_DURATION `magnitude`.
  DV_QUANTITY `magnitude`, which DV_QUANTITY declares as a property, is not, and nor is DV_AMOUNT `magnitude`, which its descendant DV_QUANTITY declares as one.
- `AttributeNames(rmType)` enumerates the descend candidates, the per-type attribute list `Lookup` gives.
- `AttributeRMType` / `IsContainer` carry the declared shape for each attribute.
  The walker recurses into every present attribute of every registered class, as § REQ-171 *Structural access* says, using the value's *runtime* RM type when known so the invariant evaluators dispatch correctly across Liskov substitution (`DV_TEXT` carrying `DV_CODED_TEXT`, a `CONTENT_ITEM` slot carrying an `OBSERVATION`, etc.).
  BASE `Interval` defines `lower_unbounded` / `upper_unbounded` as marking that boundary open (-infinity / +infinity), so an open side carries no bound value, and every BASE invariant that reads a bound (`Limits_consistent`, `Limits_comparable`) is guarded by the flags.
  The walk therefore reads a bound as absent, and skips it, only on an open side whose bound is empty, as [wire.md § REQ-052](wire.md#req-052) defines "empty" (`rm.IsEmptyIntervalBound`), the reading [wire.md § REQ-140](wire.md#req-140--underscore-prefixed-rm-attributes) gives a concrete-typed bound.
  A non-empty bound on an open side is walked like any bound.
- A cycle, a value shared between two attributes, and the walk's depth and visit limits are handled as [§ REQ-171 *Cycles and limits*](#cycles-and-limits) says.

### Per-RM-type invariant catalogue

The floor **MUST** report every invariant the inventory of [§ REQ-171 *Where the RM rules come from*](#where-the-rm-rules-come-from) classifies as evaluated, on every node of a class subject to it that the walk visits, reading it as the entries below say, with the code `rm_invariant` unless an entry names a code of its own.
Beside the structural required-set walk above, the entries cover the node-level and leaf invariants that the canjson lenient decode accepts but the RM forbids.

#### CODE_PHRASE

`code_string` non-empty (`Code_string_valid`).
The required-set walk decides this invariant: an empty `code_string` is an absent RM-mandatory attribute, reported as `required` at `…/code_string` with the rule `mandatory` alone, as [§ REQ-171 *One finding per defect*](#one-finding-per-defect) says.
A CODE_PHRASE at a mandatory attribute whose terminology is empty too is itself absent (§ REQ-171 *Structural access*), reported as `required` at its own path and not entered.
The one exception is a DV_SCALE's `symbol`: the RM lets a scale value that has no code carry a `symbol` with its `terminology_id` and a blank `code_string`, so the floor **MUST NOT** report a blank `code_string` on a DV_SCALE `symbol.defining_code`, in either form.
A DV_ORDINAL's `symbol` has no such allowance.

#### DV_QUANTITY

`precision`, when set, ≥ -1: per the RM, -1 means no limit (any number of decimal places), so only `precision < -1` is out of range.

#### DV_DATE_TIME / DV_DATE / DV_TIME / DV_DURATION

`Value_valid`: `value` is a valid ISO 8601 value of its type as the [REQ-123](reference-model.md#req-123--temporal-data-value-helpers) parse reads it, that is the type's BASE predicate (`valid_iso8601_date_time`, `valid_iso8601_date`, `valid_iso8601_time`, `valid_iso8601_duration`) with the deviations REQ-123 names.
The floor reports a non-empty `value` that parse refuses, a placeholder such as `example` included, as `rm_invariant` on the node.
An empty `value`, which is also what a JSON `null` decodes to, is an absent RM-mandatory attribute, reported as `required` at `…/value` alone, as [§ REQ-171 *One finding per defect*](#one-finding-per-defect) says.

#### ELEMENT

`Inv_null_flavour_indicated` (`is_null() xor null_flavour = Void`, where `is_null()` means there is no `value`): exactly one of `value` and `null_flavour` is present.
The floor reports an ELEMENT that carries both, and one that carries neither, as `rm_invariant` on the ELEMENT.

#### DV_PROPORTION

Two precision arms and the proportion-kind rules.
`precision`, when set, ≥ -1 under the same RM rule as DV_QUANTITY (-1 means no limit); and the RM invariant `Precision_validity` (`precision = 0 implies is_integral`), read through `Is_integral_validity` as whole-number operands, so the floor reports a `precision` of 0 over a non-integral `numerator` or `denominator`.
It also reports the proportion-kind rules:

- a `type` that is not a `PROPORTION_KIND` value, 0 to 4 (`Type_validity`);
- a `denominator` other than 1 on a unitary proportion (`Unitary_validity`) and other than 100 on a percent (`Percent_validity`);
- a `numerator` or `denominator` that is not a whole number on a fraction or an integer fraction (`Fraction_validity`, `is_integral` read as for `Precision_validity`);
- and a `denominator` of 0 (`Valid_denominator`).

The *DV_INTERVAL* entry below leaves a pair of bounds unordered when either has a zero denominator.

#### DV_INTERVAL

`Limits_consistent`: when neither side is unbounded, `lower.is_strictly_comparable_to(upper) and lower <= upper`.
The floor reads two bounds as strictly comparable as the RM defines `is_strictly_comparable_to` for their type: two DV_COUNT; two DV_QUANTITY with equal `units`; two DV_PROPORTION of one `type`; two DV_DATE, two DV_TIME, two DV_DATE_TIME or two DV_DURATION.
Each of these definitions compares a value with one of its own class, so the floor reads bounds of two classes as not strictly comparable, a DV_COUNT beside another DV_ORDERED included, although DV_COUNT's definition reads `Return True` for any.
When neither side is unbounded and each side holds a bound, the floor reports, each as `rm_invariant` at the interval with the rule `DV_INTERVAL.Limits_consistent`:

- a pair it reads as not strictly comparable (bounds of two classes, two DV_QUANTITY in different `units`, or two DV_PROPORTION of different `type`);
- and a strictly comparable pair whose `lower` is above its `upper` by the order below.

The RM leaves DV_ORDINAL's and DV_SCALE's undefined; the floor reads two of them as comparable when their symbols are coded in one named terminology, and **MUST NOT** report two whose symbols are coded in different terminologies or in none, which it does not order either.
The order is the RM's `less_than` for the type: `magnitude` for the quantified types, `value` for DV_ORDINAL and DV_SCALE, and for a DV_DURATION the magnitude the REQ-123 parse gives.
Two DV_DATE, two DV_TIME or two DV_DATE_TIME values written with one zone designator, or both with none, are ordered at the precision they share, as [§ REQ-103](#req-103--primitive-constraint-introspection) *Validate contract* defines it, which departs from reading `less_than` by `magnitude()` for a partial value, whose magnitude is the start of the period it names.
Two DV_TIME or two DV_DATE_TIME values whose zone designators differ are ordered as instants, each read in UTC, partial values and leap seconds included, and a zoned DV_TIME or DV_DATE_TIME is not ordered against one without a zone.
Equal `units` is only part of DV_QUANTITY's `is_strictly_comparable_to`, which also needs a matching `units_system` where one is set.
The floor does not compare `units_system`, so it orders a same-units pair whatever its `units_system`, although the RM makes two different systems not strictly comparable.
Of the pairs it reads as strictly comparable, it orders none that holds a DV_PROPORTION with a zero denominator or a temporal value the REQ-123 parse refuses, nor a zoned DV_TIME or DV_DATE_TIME against one without a zone, and reports none of them here (see *What the floor does not evaluate* below).

#### Interval flags

BASE `Lower_included_valid` and `Upper_included_valid` (`lower_unbounded implies not lower_included`, and the same for `upper`).
On every interval the walk reaches, a DV_INTERVAL value, a typed `normal_range` and a REFERENCE_RANGE `range` among them, the floor reports a side marked both unbounded and included as `lower_included_valid` or `upper_included_valid` at the interval's path.
Canonical JSON, canonical XML, FLAT and STRUCTURED decode such a side as written ([wire.md § REQ-169](wire.md#req-169--interval-boundary-flag-defaults)), and an RM value built in Go can carry it too, so this entry is where it is reported.

#### OBJECT_REF / PARTY_REF / ACCESS_GROUP_REF / LOCATABLE_REF

`id` present, and `type` and `namespace` non-empty, each reported as `rm_invariant` at its own path (`…/id`, `…/type`, `…/namespace`).
The floor walks into a reference like any node, so the OBJECT_ID a present `id` holds is checked in turn, but its required-set check leaves a reference's `id`, `namespace` and `type` to this evaluator, so a missing part is reported once.

#### PARTY_PROXY family (PARTY_SELF, PARTY_IDENTIFIED, PARTY_RELATED)

Wherever a party proxy sits (a COMPOSITION's `composer`, an EVENT_CONTEXT's `health_care_facility`, an ENTRY's `subject` and `provider`, a PARTICIPATION's `performer`, an EHR_STATUS's `subject`, or the root), the floor walks it, and a present `external_ref` whatever its parts hold, so the entry above checks that PARTY_REF at `…/external_ref/id`, `…/external_ref/type` and `…/external_ref/namespace`.
On a PARTY_IDENTIFIED, and on a PARTY_RELATED, which inherits them, the floor reports as `rm_invariant`: `Basic_validity` (none of `name`, `identifiers` and `external_ref` is present) at the node; `Name_valid` (a present `name` that is empty) at `…/name`; and `Identifiers_valid` (a present `identifiers` that is empty) at `…/identifiers`.
Presence follows the decoded value, as for DV_TEXT `mappings`: an absent key and JSON `null` both read as absent, so only a decoded `"name":""` or `"identifiers":[]` is present and empty.
PARTY_RELATED's RM-mandatory `relationship` falls under the required-set walk, and its `Relationship_valid` under the *Coded invariants* entry below.
PARTY_SELF has no invariant of its own; an EHR_STATUS's value-typed `subject` follows § *Value-typed mandatory presence*.

#### PARTICIPATION

Wherever a participation sits (an EVENT_CONTEXT's `participations`, an ENTRY's `other_participations`, or the root), its RM-mandatory `function` and `performer` fall under the required-set walk, which reports an absent one as `required` at `…/function` or `…/performer`, and a `function` whose `value` is empty counts as absent, as a LOCATABLE's `name` does ([§ REQ-171](#req-171--validation-engine) *Structural access*).
The `performer` proxy, `mode` and `time` are walked like any other present attribute.
`Function_valid` and `Mode_valid`, which tie a coded `function` and the `mode` to openEHR terminology groups, fall under the *Coded invariants* entry below.

#### DV_TEXT / DV_CODED_TEXT

The floor reports a present-but-empty `mappings` (`Mappings_valid`) as `mappings_valid`; DV_CODED_TEXT inherits `mappings` from DV_TEXT via embedding, so both dispatch through the same evaluator.
The floor cannot distinguish an absent `mappings` key from an explicit JSON `null` — canjson decodes both to a nil slice — so only a decoded literal `"mappings":[]` (a non-nil empty slice) is flagged; null and absent are indistinguishable and both valid.
`mappings` is a walked container: each element is a TERM_MAPPING node of its own, checked by the entry below wherever it sits.

#### TERM_MAPPING

The floor reports a `match` outside `{'>', '=', '<', '?'}` (`term_mapping_match`) as an invariant violation on the mapping node itself, so the check fires for a mapping under a DV_TEXT / DV_CODED_TEXT `mappings` container, for one nested under a mapping's own `purpose`, and for a TERM_MAPPING handed to `ValidateRM` as the root.
The RM-mandatory `match` and `target` (CODE_PHRASE) fall under the required-set walk, and `target`'s and `purpose`'s own invariants apply inside each mapping; an empty `match` therefore reports both `required` and `term_mapping_match` at the same path.

#### Non-empty lists

`COMPOSITION.Content_valid`, `EVENT_CONTEXT.Participations_validity`, `INSTRUCTION.Activities_valid`, `SECTION.Items_valid` and `ATTESTATION.Items_valid` each read `x /= Void implies not x.is_empty` for the list `x` they name: `content`, `participations`, `activities`, and `items` on the last two.
The floor reports such a list that is present and holds no item, presence as [§ REQ-171 *Structural access*](#structural-access) reads it, as `rm_invariant` at the list's own path (`…/context/participations`), with the invariant as its `Rule`, as it reports a present-but-empty `mappings`, and not an absent one.
`HISTORY.Events_valid` reads `(events /= Void and then not events.is_empty) or summary /= Void`.
The floor reports a HISTORY whose `events` is absent or holds no item and whose `summary` is absent as `rm_invariant` at the HISTORY's own path, as it reports the other rules of this catalogue over more than one attribute, `Basic_validity` and `Inv_null_flavour_indicated`, at their node, and not one that carries an event or a summary.
Under a template, an existence lower bound of 1 or more on one of these lists asks for a member ([§ REQ-102](#req-102--composition-validation) *An existence lower bound alone asks for no member*), and which layer reports such a list is in [§ REQ-171 *One finding per defect*](#one-finding-per-defect).

#### LOCATABLE links and EVENT_CONTEXT location

`LOCATABLE.Links_valid` and `EVENT_CONTEXT.location_valid` each read `x /= Void implies not x.is_empty` for the attribute `x` they name, `links` and `location`.
The floor reports a `links` that is present and holds no item, on every LOCATABLE it visits, and a `location` that is present and empty, as `rm_invariant` at the attribute's own path (`…/links`, `…/context/location`), with the invariant as its `Rule`, and not an absent one.
Presence is as [§ REQ-171 *Structural access*](#structural-access) reads it, so an absent key and a JSON `null` are both absent, and only a decoded `"links":[]` or `"location":""` is present and empty.
`links` is not one of the lists the *Non-empty lists* entry evaluates, so under a template an existence lower bound on it asks for no member ([§ REQ-102](#req-102--composition-validation) *An existence lower bound alone asks for no member*), and the finding is the RM rule's own, as it is with no template.
The diagnostic names the class and the attribute, never a value ([§ REQ-168](#req-168--value-free-validation-diagnostics)).

#### DV_EHR_URI

`Scheme_valid` (`scheme.is_equal (Ehr_scheme)`): the scheme of the `value` is `ehr`.
The floor reports a DV_EHR_URI whose `value` does not have the scheme `ehr`, wherever the walk visits one (the `target` of a LINK, an ELEMENT's `value`, an ATTESTATION's `items`), as `rm_invariant` at the DV_EHR_URI itself, as it reports the ISO 8601 `Value_valid` rule at its node.
The scheme is the text before the first `:` when RFC 3986 section 3.1 admits it as one, `ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )`, and the comparison ignores letter case, so `EHR://x` keeps the rule and `http://x` breaks it.
A `value` with no scheme, such as `example.org/x`, has none equal to `ehr` and breaks it too; an empty one is the required-set walk's (`DV_URI.Value_valid`).
The diagnostic names the class and the attribute, never a value, the scheme being part of it, and the issue's `Value` is empty ([§ REQ-168](#req-168--value-free-validation-diagnostics)).

#### Archetype roots (`Is_archetype_root`, `Archetyped_valid`)

`LOCATABLE.Archetyped_valid` (`is_archetype_root xor archetype_details = Void`) makes `archetype_details` mandatory on an archetype root, although LOCATABLE declares it 0..1.
The RM defines `is_archetype_root` in prose: `archetype_node_id` holds an at-code, except at an archetype root, where it is the stringified `archetype_id` of `archetype_details`.
So the floor decides it per node, on every LOCATABLE the walk visits.
A node is an archetype root when any of these holds:

- its class is always one: the class-level `Is_archetype_root` invariant fixes `is_archetype_root` true on the class that declares it and on its descendants;
- its `archetype_node_id` parses as an `ARCHETYPE_ID`, as [REQ-120](reference-model.md#req-120--rm-identifier-parsing-and-derivation)'s `ParseArchetypeID` reads it;
- it carries `archetype_details`.

The classes that are always roots are COMPOSITION, EHR_ACCESS, EHR_STATUS, PARTY (with its concrete descendants PERSON, ORGANISATION, GROUP, AGENT, ROLE) and ENTRY (with its concrete descendants ADMIN_ENTRY, OBSERVATION, EVALUATION, INSTRUCTION, ACTION).
This set is closed: the floor takes it from the `Is_archetype_root` entries of the inventory of [§ REQ-171 *Where the RM rules come from*](#where-the-rm-rules-come-from).
The [REQ-107](#req-107--template-driven-rm-instance-example-generator) generator takes it from a closed list held equal to it and to the vendored BMM's `Is_archetype_root` declarations, so a BMM bump that adds a root class fails the build ([ADR 0001](../adr/0001-bmm-version-bump-runbook.md)).

The floor **MUST** report an absent or `null` `archetype_details` on an archetype root once, as `is_archetype_root` at `…/archetype_details`, with the rule this table gives:

| The node | `Rule` |
|---|---|
| is of a class that is always a root | the `Is_archetype_root` the class declares or inherits (`ENTRY.Is_archetype_root` on an OBSERVATION) |
| is of any other class, and its `archetype_node_id` parses as an `ARCHETYPE_ID` | `LOCATABLE.Archetyped_valid` |

A node of any other class whose `archetype_node_id` is no `ARCHETYPE_ID`, an at-code such as `at0001` among them, may leave `archetype_details` out.

At an archetype root, the floor **MUST** report an `archetype_details.archetype_id.value` that differs from `archetype_node_id`, compared exactly, as `archetype_root_id` at `…/archetype_details/archetype_id/value`, rule `archetype_root_id`.
The RM states this rule in the definition of `LOCATABLE.archetype_node_id`, not as a named invariant.
A node whose `archetype_node_id` is an at-code and that carries `archetype_details` is a root, so it draws this finding too.
An empty id on either side draws only the `required` it already draws, so one defect draws one finding.
Each diagnostic names the class and the attributes, never a value ([§ REQ-168](#req-168--value-free-validation-diagnostics)).

This is a class invariant of the object, not a storage rule, so it holds for a detached object exactly as for a persisted one.
Rules that need stored history (`VERSIONED_COMPOSITION.Archetype_node_id_valid`) and requiring `ARCHETYPED.template_id` (0..1 in the RM) stay out of scope.

#### ARCHETYPED

Wherever an `archetype_details` node is present, on any LOCATABLE, its two RM-mandatory attributes `archetype_id` (`ARCHETYPE_ID`) and `rm_version` (`String`) are value-typed, so the value-based walk cannot tell absent from `null` from empty (they decode to the same zero value).
On a value validated with no report of missing members, the value-based entries (`ValidateRM` and the typed sugars) report an empty `archetype_id.value` as `required` at `…/archetype_details/archetype_id/value` (SDK policy: `OBJECT_ID.value` is RM-mandatory and has no non-empty invariant of its own) and an empty `rm_version` as `rm_version_valid` (`Rm_version_valid: not rm_version.is_empty`) at `…/archetype_details/rm_version`.
Absent and `null` decode to the same zero values and are reported the same way, and an ARCHETYPED handed to `ValidateRM` as the root is checked the same way, with the findings at `/archetype_id/value` and `/rm_version`.
With a decode's report of missing members, an absent or `null` member is read as [§ *Value-typed mandatory presence*](#value-typed-mandatory-presence) says.
The floor does not validate the `ARCHETYPE_ID` grammar of `archetype_id`; it reads that grammar only to tell an archetype root by its `archetype_node_id` (*Archetype roots*).

#### Coded invariants

The floor evaluates each RM invariant in the table below, which ties a coded attribute to a group or a code set of the openEHR terminology, on every node of the listed classes it visits, taking the group, the code set and the membership verdict from the [REQ-034](reference-model.md#openehr-terminology-vocabulary-req-034) accessor.
An optional attribute is checked only when present, ELEMENT's `null_flavour` only when the ELEMENT has no `value` (the RM states `Inv_null_flavour_valid` as `is_null implies …`), and PARTICIPATION's `function` only when it is a DV_CODED_TEXT (`Function_valid`'s own guard).

| Class | Invariant | Attribute | Group or code set |
|---|---|---|---|
| COMPOSITION | `Category_validity` | `category` | group *composition category* |
| COMPOSITION | `Language_valid` | `language` | code set *languages* |
| COMPOSITION | `Territory_valid` | `territory` | code set *countries* |
| EVENT_CONTEXT | `Setting_valid` | `setting` | group *setting* |
| ENTRY (every concrete) | `Language_valid` | `language` | code set *languages* |
| ENTRY (every concrete) | `Encoding_valid` | `encoding` | code set *character sets* |
| ELEMENT | `Inv_null_flavour_valid` | `null_flavour` | group *null flavours* |
| INTERVAL_EVENT | `Math_function_validity` | `math_function` | group *event math function* |
| ISM_TRANSITION | `Current_state_valid` | `current_state` | group *instruction states* |
| ISM_TRANSITION | `Transition_valid` | `transition` | group *instruction transitions* |
| PARTICIPATION | `Function_valid` | `function` | group *participation function* |
| PARTICIPATION | `Mode_valid` | `mode` | group *participation mode* |
| PARTY_RELATED | `Relationship_valid` | `relationship` | group *subject relationship* |
| TERM_MAPPING | `Purpose_valid` | `purpose` | group *term mapping purpose* |
| DV_TEXT, DV_CODED_TEXT | `Language_valid` | `language` | code set *languages* |
| DV_TEXT, DV_CODED_TEXT | `Encoding_valid` | `encoding` | code set *character sets* |
| DV_ORDERED (every concrete) | `Normal_status_validity` | `normal_status` | code set *normal statuses* |
| DV_MULTIMEDIA, DV_PARSABLE | DV_ENCAPSULATED `Language_valid` | `language` | code set *languages* |
| DV_MULTIMEDIA, DV_PARSABLE | DV_ENCAPSULATED `Charset_valid` | `charset` | code set *character sets* |
| DV_MULTIMEDIA | `Media_type_valid` | `media_type` | code set *media types* |
| DV_MULTIMEDIA | `Compression_algorithm_validity` | `compression_algorithm` | code set *compression algorithms* |
| DV_MULTIMEDIA | `Integrity_check_algorithm_validity` | `integrity_check_algorithm` | code set *integrity check algorithms* |

A group invariant asks the openEHR terminology itself (`terminology (Terminology_id_openehr).has_code_for_group_id (…)`), so it holds only when the CODE_PHRASE's `terminology_id` is `openehr` and its `code_string` is a member of the group: a setting coded in `local`, or in any terminology other than `openehr`, breaks `Setting_valid` whatever its code string.
A code-set invariant holds when the `code_string` is a member of the code set under the accessor's membership rule, which ignores letter case for the ISO and IANA sets; the `terminology_id` is not compared, because the RM's `CODE_SET_ACCESS.has_code` takes the bare code.
The CODE_PHRASE an invariant reads is the attribute itself for a CODE_PHRASE attribute, and the attribute's `defining_code` for a DV_CODED_TEXT one.
The floor reports each breach as `code_not_in_value_set` at that CODE_PHRASE's path (`…/language`, `…/context/setting/defining_code`).
An empty `code_string` is no member.
So on a value validated with no report of missing members, `code_not_in_value_set` is drawn by a mandatory value-typed attribute that decoded empty, such as a COMPOSITION `language` the source left out or a PARTY_RELATED `relationship`.
It is drawn beside whatever the walk already reports for it: `required` for an omitted COMPOSITION `language`, `required` at its `code_string` for one that carries a terminology id and a blank code.
With the report, an omitted `language` is `required` at `…/language` alone (§ *Value-typed mandatory presence*).
A PARTICIPATION and a PARTY_RELATED are nodes of the walk wherever they sit (the PARTICIPATION and PARTY_PROXY entries above), a participation's `performer` and a root included, so their rows apply there as on any other node: at `…/other_participations[i]/mode/defining_code`, say, or `…/performer/relationship/defining_code`.
The RM's other coded invariants are not in this entry (*What the floor does not evaluate* below).

#### Known gap — a bound beside its own open flag

A bound present on a side whose `*_unbounded` flag is set contradicts BASE `Interval`'s open-boundary meaning.
The floor walks it as a bound but does not report the contradiction.

#### What the floor does not evaluate

Every invariant of the pinned RM and BASE BMMs that the floor does not evaluate is an entry of the inventory of [§ REQ-171 *Where the RM rules come from*](#where-the-rm-rules-come-from), with its reason, among them:

- BASE `Interval.Limits_comparable`, which `DV_INTERVAL.Limits_consistent` restates and the floor evaluates on a DV_INTERVAL as the *DV_INTERVAL* entry says, but on no BASE `Proper_interval` or `Point_interval`, whose bounds the SDK leaves untyped;
- and the coded invariants of the change-control and resource-description classes, which the *Coded invariants* table leaves out.

A result lists those of the classes it visited in `Coverage.Unevaluated`.
The inventory classifies `DV_INTERVAL.Limits_consistent` as evaluated, but the floor leaves parts of it unchecked:

- the `units_system` of two DV_QUANTITY;
- the comparability of two DV_ORDINAL or DV_SCALE, which the RM leaves undefined;
- and the order of a pair it reads as strictly comparable but cannot order, one that holds a DV_PROPORTION with a zero denominator or a temporal value the REQ-123 parse refuses, or a zoned DV_TIME or DV_DATE_TIME against one without a zone.

### Value-typed mandatory presence

The required-set walk reads presence from the decoded Go value, as [§ REQ-171 *Structural access*](#structural-access) says: an attribute held by value is absent when it holds its type's zero value, so an omitted DV_ORDINAL or DV_SCALE `symbol`, or an omitted REFERENCE_RANGE `range`, is reported as `required`.
What the walk cannot detect is an omitted **value-typed** mandatory attribute whose zero value is itself a valid value:

- a number or a boolean, such as `DV_QUANTITY.magnitude`, `DV_COUNT.magnitude`, `DV_BOOLEAN.value` or `DV_PROPORTION.denominator`;
- and a value struct, such as `EHR_STATUS.subject` (typed `rm.PartySelf`, whose only field is optional, so an omitted subject and a valid bare `{"_type":"PARTY_SELF"}` decode to the identical zero value) and an ARCHETYPED's `archetype_id` and `rm_version`.

Interface-, pointer- and slice-typed mandatories are unaffected, nil being a reliable absence signal, as are the value-level invariants, which inspect fields rather than presence.

The floor closes this by reading what the decode found missing, the report of missing members [wire.md § REQ-175](wire.md#req-175--missing-members-on-decode) defines ([ADR 0033](../adr/0033-missing-members-reported-by-decoders.md)).

- Every value-based entry, `ValidateRM`, `Validate` and `Validator.Validate`, **MUST** accept such a report through `WithMissing`, a trailing option, so a caller that decodes with a codec's report option hands the pointers on and validates once; the typed sugars take none and are for a value with no report.
  `WithMissingPointers` **MUST** accept the report as either codec's pointer values and read them without writing their text, giving what `WithMissing` of their text gives, with a cost of reading the report into the walk that grows with the distinct members on the report's paths, not with each member's depth times their number.
  Each finding's own issue path is written as it is without a report.
  A nil option, an empty list and a repeated option, of either kind and in any mix, are read as the union of what was given.
- `ValidateJSON` and `ValidateXML`, and their `Validator` methods, **MUST** decode `data` into `root`, a non-nil pointer to one registered RM value, read the document's missing members, and validate `root` with that report in one call, returning what `Validator.Validate(root, c, WithMissing(…))` returns: with a nil `c` the RM rules alone, with a compiled template the template rules too ([§ REQ-102](#req-102--composition-validation)), under the entry's own limits.
  The decode is the codec's default decode: unmatched content is ignored ([wire.md § REQ-170](wire.md#req-170--unmatched-content-on-decode)), and `root` holds the decoded value afterwards, so a caller that wants the value and the result decodes once.
- A reported member **MUST** be read as absent at the node its pointer names, whatever the Go value holds there, inside the one walk ([§ REQ-171 *One walk*](#one-walk)): the walk reports it as it reports any absent attribute, the template's `required` where the template requires it and the RM's `required` with the rule `mandatory` otherwise ([§ REQ-171 *Finding identity*](#finding-identity)), does not enter it and runs no other rule at its path, so its path carries that one finding.
  An absent `rm_version` is therefore `required` at `…/archetype_details/rm_version`, not `rm_version_valid`, and an absent `archetype_id` is `required` at `…/archetype_details/archetype_id` alone, with nothing at `…/archetype_id/value`.
  Presence is read at every level, a nested ARCHETYPED and a data value deep in a COMPOSITION included; the root is no exception.
  Two readings of the Go value remain.
  A rule of the node that holds the member reads the node's value, where a decode left the member's zero value, and reports at the node's own path as it would without the report: a DV_PROPORTION with no `denominator` draws `required` at `…/denominator` and `Valid_denominator` at the DV_PROPORTION.
  And the three parts of an OBJECT_REF, which the floor's OBJECT_REF rule checks in place of the required-set check ([§ *Per-RM-type invariant catalogue*](#per-rm-type-invariant-catalogue)), are reported by that rule from the Go value, which a decode leaves empty for a part it reports.
- A pointer that names no attribute the walk visits draws no finding: a node the template refused, a node past a limit, which `Coverage.Stopped` lists, a node the walk does not enter.
  The walk reports what it visits.
  A pointer into a value held by a `Hash` is matched by the entry's key, and one into a list by the item's position.
- A value built in Go has no report, so a by-value mandatory member left unset there reads as present; only a decode can say what a document said.
  The typed wrappers and `ValidateRM` without the option keep that blind spot, which their godoc names.
- On `ValidateJSON` and `ValidateXML`, input that does not decode into `root`, malformed input, a canonical-JSON document whose root is not an object, a canonical-XML document with content before or after its root element or a root that fails its decode, **MUST** surface a single `invalid_shape` at `/` and a not-OK `Result`, with the decode error, whose text may quote the input, in the issue's `Value` and not in its `Detail` ([§ REQ-168](#req-168--value-free-validation-diagnostics)).
  JSON `null` raises no decode error, so its issue's `Value` is empty.
  A nil `root`, or one that is no registered RM value, draws the entry's `nil_root` or `rm_type_unknown` guard, as `ValidateRM` does.
- `ValidateRMEHRStatusBytes(data)` is kept as `ValidateJSON(data, new(rm.EHRStatus), nil)` and is deprecated: an omitted or `null` `subject` is `required` at `/subject`, a supplied subject, even the bare form, draws no `required`, and presence is read at every level.

### Out of scope

The floor does **not** evaluate:

- archetype-level or template-level constraints (that is REQ-102);
- terminology bindings an OPT declares ([§ REQ-111](#req-111--public-compiled-template-bridge) *Term definitions and bindings*), and codes from terminologies outside the pinned openEHR terminology (SNOMED CT, LOINC, …), which need a terminology service; the closed groups and code sets of the *Coded invariants* entry are the only codes it checks;
- semantic validity beyond the BMM and the explicit invariant catalogue.

- **Probes:** PROBE-077 (every invariant the inventory classifies as evaluated, on every registered class subject to it, with a breaking and a conforming value); PROBE-081 (the EHR_STATUS root: `subject` and the ARCHETYPED keys); PROBE-110 (missing members at every level, in both canonical formats, and the validators' one answer).

---

## REQ-168 — Value-free validation diagnostics

A diagnostic **MUST NOT** carry a submitted value (§ Submitted values and structure) outside a field its type names value-bearing (§ Field classes).
The rule binds [§ REQ-103](#req-103--primitive-constraint-introspection)'s `Violation`, the `Issue` that [§ REQ-102](#req-102--composition-validation) and [§ REQ-112](#req-112--template-less-reference-model-validation-floor) return, and the AQL diagnostics: the `lint.Issue` of [§ REQ-109](aql.md#req-109--aql-static-lint), which the `Issue` that `ValidateAQL` returns carries over, and the drop record of [§ REQ-113](aql.md#value-free-structured-drop-records).
Diagnostics travel to places a caller does not choose for clinical data: an HTTP error body, a log line, a monitoring event.
The values they describe are clinical data (a date of birth, a measured magnitude, a free-text name), and an AQL literal can be one, so a message that quotes one carries it to each of those places, in callers that never meant to send it.
A consumer whose refusal reasons may name structure but never a value would otherwise discard the diagnostics wholesale, and its clients would get a generic refusal where the SDK knew what was wrong and where.
[REQ-093](transport.md#req-093--openehr-error-envelope-mapping) holds the transport errors to the same line.
On a `Violation` and an instance validator's `Issue` the value stays reachable through `Value.Reveal()` ([§ The redacting carrier](#the-redacting-carrier)).

### Submitted values and structure

A **submitted value** is data taken from the input under validation:

- for `PrimitiveConstraint.Validate`, the argument and each of its fields: a `QuantityValue`'s magnitude, units and precision, a `CodedTermRef`'s terminology and code, an `OrdinalSymbol`'s value and symbol;
- for the instance validators, the content of a data value or a primitive attribute in the instance (a `value`, `magnitude`, `units`, `precision`, `numerator` or `denominator`, an interval bound, a `CODE_PHRASE`'s terminology and code, a `TERM_MAPPING`'s `match`), and the text of an error raised while decoding the instance;
- for the AQL linter and parser, any source text of the query, a path spelling included, since a path carries its own predicates ([§ REQ-109 § Value-free lint diagnostics](aql.md#value-free-lint-diagnostics)).

**Structure** is not a submitted value: RM type and attribute names, archetype and node ids, a path an instance validator writes, a child count, the Go type name of a wrong-typed argument, and the constraint itself (its allowed list, range, pattern, units and terminology), which comes from the template rather than the instance.
Naming the clause a value failed lets a reader infer something about the value: that it lies outside a range, which units entry it matched, which of the two booleans it was.
That inference is what a diagnostic is for, and it is not an echo.
Copying the value, or any part of it, is.

### Field classes

A field's class is fixed by its type and, for an `Issue`, by the entry point that returned it, which a consumer knows because it made the call.
It never depends on an issue's content.

#### The two field classes

Each field of a diagnostic **MUST** be in exactly one of two classes, and the godoc of the type a consumer holds names each field's class:

| Class | Rule |
|---|---|
| **Value-free** | **MUST NOT** carry a submitted value, in any circumstance: a field that cannot say a thing without the value says less (a zero span, an unclassified kind, a structural wording) and never falls back to quoting it |
| **Value-bearing** | **MAY** carry a submitted value or a query spelling; a disclosure boundary treats it accordingly |

What cannot be said without the value goes in a value-bearing field, or nowhere.
The classes of the validators' fields:

| Type and entry point | Value-free | Value-bearing |
|---|---|---|
| `constraints.Violation` | `Code`, `Detail` | `Value` |
| `validation.Issue` from the REQ-102 entry points and the REQ-112 floor entries | `Path`, `Code`, `Layer`, `Rule`, `Detail`, `Severity` | `Value` |
| `validation.Issue` from `ValidateAQL` and `ValidateAQLWithTypeRelation` | `Code`, `Layer`, `Rule`, `Severity`, which carry no source text ([§ REQ-109 § Value-free lint diagnostics](aql.md#value-free-lint-diagnostics)) | `Detail` and `Path`, as § REQ-109 classifies them |

The `lint.Issue` fields are classed in [§ REQ-109 § Value-free lint diagnostics](aql.md#value-free-lint-diagnostics), and the drop record's in [§ REQ-113 § Value-free structured drop records](aql.md#value-free-structured-drop-records).

### The redacting carrier

`Value` **MUST** be of the type `constraints.Redacted`, which holds one submitted value and never prints it:

- `constraints.Redact(v)` **MUST** return a `Redacted` holding `v`.
  The zero `Redacted` holds no value.
- `Reveal()` **MUST** return the held value with its type, or nil when none is held, and be the only method of `Redacted` that returns the value.
  A value that `==` can compare comes back equal under `==` to the one held, with two exceptions:
  - a floating-point zero may come back with either sign;
  - and a NaN, alone or inside an array or a struct, comes back as a NaN in the same place, though `==` never finds it equal, so a value that holds one never compares equal to the one given.

  Any other value comes back as the same value.
- Printing, encoding and logging a `Violation`, `Issue`, `Result` or `Redacted` **MUST** behave as follows, so that none of them writes the value:
  - every `fmt` verb other than `%T` and `%p`, `%v`, `%+v` and `%#v` included, prints `[redacted]` for a `Redacted` that holds a value and nothing for one that does not, and `String()` returns the same text, so printing a whole `Violation`, `Issue` or `Result` prints no submitted value;
  - under `%T` and `%p`, and through an unexported field of the caller's own struct, the output contains no value either;
  - `encoding/json`, v1 and v2 alike, leaves the `Value` field of `Violation` and of `Issue` out, so encoding a `Result` writes no `Value` member, and encodes a `Redacted` on its own as JSON `null`;
  - `encoding/gob` encodes a `Violation`, `Issue` or `Result` without an error, a `Redacted` as empty, which decodes as the zero `Redacted`;
  - the text and JSON handlers of `log/slog` write no value.
- `fmt` itself answers `%T` with the type name, and prints a `Redacted` without calling its methods under `%p` and through an unexported field of the caller's own struct.
  Those paths show memory addresses instead, and equal held values show the same address, so two such lines reveal that their values are equal.
- Tools other than `fmt` that read unexported fields by reflection, such as a diff reporter or a debugger, are outside these rules.
- `Violation` and `Issue` values **MUST** be comparable with `==` without a panic, whatever a `Redacted` holds, and `Redacted` **MUST** have an `Equal(Redacted) bool` method that agrees with `==`, so a comparison library that refuses unexported fields but honours such a method can still compare a `Violation`, `Issue` or `Result`.
  Two diagnostics that hold equal values **MUST** compare equal under `==`: the SDK puts only comparable values in a `Redacted`, and holds a value that `==` cannot compare, such as a slice, by reference, so only copies of one `Redacted` compare equal.

### What `Value` holds

On a `Violation`, `Value` **MUST** hold the part of the input that the failing clause tested, as this table says, and be empty on `CodeWrongType`, where no clause tested the input and `Detail` names its Go type, and on a violation the constraint causes itself, such as an unparseable `CString` pattern:

| Validator and clause | `Value` |
|---|---|
| `CBoolean`, `CInteger`, `CReal`, `CString`, `CDate`, `CTime`, `CDateTime`, `CDuration`, `CDvOrdinal` | the argument as passed |
| `CodePhrase`, terminology mismatch | the input's terminology id |
| `CodePhrase`, code not in the list | the input's code string |
| `DvQuantity`, magnitude range | the input's `Magnitude` |
| `DvQuantity`, precision range | the input's `Precision` |
| `DvQuantity`, units not enumerated | the input's `Units` |

On an `Issue` from the instance validators, `Value` **MUST** hold what this table names, and be empty on every other `Issue` (an absence, a count, a type or identity mismatch, a guard) and on every `Issue` from `ValidateAQL` or `ValidateAQLWithTypeRelation`:

| Issue | `Value` |
|---|---|
| `primitive_*` | the `Value` of the violation it reports |
| `rm_invariant` on a `DV_QUANTITY` or `DV_PROPORTION` `precision` below -1 | the precision |
| `rm_invariant` on a `DV_PROPORTION` with precision 0 and a fractional operand | the numerator and the denominator, as a `[2]float64` |
| `rm_invariant` on a `DV_PROPORTION` `type` that is not a proportion kind (`Type_validity`) | the type |
| `rm_invariant` on a `DV_PROPORTION` `denominator` its kind rejects, or of 0 (`Unitary_validity`, `Percent_validity`, `Valid_denominator`) | the denominator |
| `rm_invariant` on a fraction or integer-fraction `DV_PROPORTION` with a fractional operand (`Fraction_validity`) | the numerator and the denominator, as a `[2]float64` |
| `rm_invariant` on a `DV_INTERVAL` whose lower bound is above its upper bound | the two bounds' magnitudes, lower first, as a `[2]float64`, two DV_TIME written with different zone designators each read in UTC |
| `rm_invariant` on a temporal `value` (`Value_valid`) | the `value` string |
| `term_mapping_match` | the `match` |
| `invalid_shape` from a failed decode | the decode error, which carries the literal through unwrapping where one of its causes records it, as a `*json.SemanticError` or a `*strconv.NumError` does ([wire.md § REQ-052](wire.md#req-052)) |

### Compatibility

- This requirement says which fields may carry a submitted value and what `Value` holds; it defines no code, path, severity or finding.
  `Code`, and on an instance validator's `Issue` also `Path` and `Severity`, are value-free (§ Field classes).
  Their vocabularies, `Result.OK` and the set of findings are the ones [§ REQ-102](#req-102--composition-validation), [§ REQ-103](#req-103--primitive-constraint-introspection) and [§ REQ-112](#req-112--template-less-reference-model-validation-floor) define.
  A caller dispatches on `Code`, not on `Detail` text ([§ REQ-103 § Violation taxonomy](#violation-taxonomy), [§ REQ-102 § Issue codes](#issue-codes)).
- The issues of `ValidateAQL` follow [§ REQ-109 § Value-free lint diagnostics](aql.md#value-free-lint-diagnostics).

**Acceptance.** For each row of § What `Value` holds, a test plants a marker value that no constraint text contains and checks that `Value.Reveal()` returns what the row names and that no value-free field carries the marker.
A `Violation`, an `Issue` and a `Result` holding a marker are rendered with `%v`, `%+v`, `%#v`, `%s`, `%d` and `%p`, directly and through an unexported field, encoded with `encoding/json` v1 and v2 and `encoding/gob`, and logged through the text and JSON handlers of `log/slog`, and the marker appears in none of that output.
The empty-`Value` cases (`CodeWrongType`, an unparseable pattern, an absence, an issue from `ValidateAQL`) each have a named test, and `ValidateComposition` over a composition with a marker at a constrained primitive leaf reports the marker in that issue's `Value` and in no other field.

### Out of scope

- Errors the codecs return as `error` values; [wire.md](wire.md) governs their text.
- A redacting carrier on `lint.Issue` and the AQL drop record: the lint issue's query spellings stay in `Detail` and `Path`, and the drop record has no value-bearing field.
- Redaction policy for structure.
  Whether an archetype id or a path is sensitive in a deployment is the consumer's decision.

---

## REQ-171 — Validation engine

Template-driven validation ([§ REQ-102](#req-102--composition-validation)) and the Reference Model floor ([§ REQ-112](#req-112--template-less-reference-model-validation-floor)) run as one engine: one validator value, one walk, the template's admission decided before any RM rule at every node, one finding per defect that names its layer and its rule, and a result that says what it did not check ([ADR 0027](../adr/0027-template-admission-before-rm-rules.md), [ADR 0028](../adr/0028-generated-rm-attribute-access.md)).

### Surface

```go
// One validator value; immutable after New, safe to share between goroutines.
type Validator struct { /* unexported */ }
func New(opts ...Option) *Validator

// c == nil: the RM rules alone. opts carries a decode's report of missing
// members (§ REQ-112 Value-typed mandatory presence); the decode-aware
// entries ValidateJSON and ValidateXML, and their Validator methods, decode
// and validate in one call.
func (v *Validator) Validate(root any, c *templatecompile.Compiled, opts ...ValidateOption) Result
func (v *Validator) ValidateJSON(data []byte, root any, c *templatecompile.Compiled) Result
func (v *Validator) ValidateXML(data []byte, root any, c *templatecompile.Compiled) Result

type Limits struct {
    Depth int // nesting depth at which the walk stops descending; 256 by default
    Nodes int // values, and list items holding none, visited per call before the walk stops; 100,000 by default
}
func WithLimits(l Limits) Option
```

- `New` **MUST** take functional options ([idiom.md § Functional options](idiom.md#functional-options)), ignore a nil `Option`, and read a `Limits` field that is zero or negative as its default, so no option disables a limit; the limits are the first options, and a later setting is added as an option, never as a parameter.
- The zero `Validator` and a nil `*Validator` **MUST** behave as one built with no options.
- The value **MUST** hold no per-call state after `New`, so one instance serves every goroutine of a server ([idiom.md § Concurrency](idiom.md#concurrency-req-026)); the compiled template is built once per template and shared.
  What belongs to one call, a decode's report of missing members, travels as a trailing `ValidateOption` of the call, never as a setting of the value.
- `Validate` **MUST** accept any RM value the registry knows ([reference-model.md § REQ-040](reference-model.md#type-registry-req-040)) and, given a nil `c`, apply the RM rules alone, so a root that binds to no template is validated by the same walk.
- The package-level entry points of § REQ-102 and § REQ-112 **MUST** behave as a `Validator` built with no options, keeping their nil guards: `Validate(root, nil)` still reports `nil_template`, and `ValidateRM` is the package-level entry for the RM rules alone, returning for a root the issues `Validate(root, nil)` on a `Validator` returns.

### One walk

The walk visits the instance's nodes.
At each node it knows the RM class of the value and, where the template reaches, the compiled node the value binds to.
The attributes it visits at a node are the union of three sets: the attributes the bound template node declares, the attributes present on the value, and the attributes the RM marks mandatory for the class.
That union is what lets one walk find a required node the instance lacks, content outside the template, and an RM omission.
A declared attribute that no instance of the value's class can carry **MUST** be left out, with no issue reported for it, its existence included:

- an attribute no serialization carries, the set [§ REQ-112 *Driver*](#driver) leaves out;
- and an attribute the compile marks as absent from its node's RM class ([§ REQ-111](#req-111--public-compiled-template-bridge) *Attributes the RM does not have*), whose path *Coverage* lists.

At each attribute, in this order:

1. **Admission.** Where the bound template node prohibits the attribute (§ REQ-102 *Structural — prohibited attributes*) and the attribute is present, the engine reports one issue, code `prohibited`, at the attribute's path, and does not enter the attribute.
   Where the value of a single-valued attribute, or an item of a multi-valued one, binds to none of the children the template allows under the binding rules of § REQ-102, the engine reports it as that section says (`slot_fill`, `rm_type_mismatch` or `alternative_mismatch`) and does not enter it.
   Where an item binds to a child the template does not allow, one whose occurrences upper bound is 0 (§ REQ-102 *Structural — prohibited attributes*), the child's occurrences finding of step 3 is the report for it, and the engine does not enter the item.
   Where the template does not mention the attribute, it is unconstrained: the engine visits it with the RM rules alone.
2. **Presence.** An attribute the template requires or the RM mandates that the instance lacks is reported once, by the layer *One finding per defect* below names.
   § REQ-102's rule that existence and cardinality both fire on one present, empty list concerns two template clauses and stays.
3. **Counts.** The cardinality of the attribute and the occurrences of each child, as § REQ-102 defines them, on the template layer.
4. **Each admitted item.** Identity and type conformance against its bound child (§ REQ-102); the RM rules of its class (§ REQ-112's catalogue); the template's value constraint where the child is a leaf ([§ REQ-103](#req-103--primitive-constraint-introspection)); then its own attributes by the same steps.

#### Refused content

Content the admission step refuses **MUST** draw its one admission finding and no other: the engine evaluates no RM rule on it and reports no issue from inside it.
A rule of an admitted node is still evaluated on that node and reads the node's attributes as the instance holds them, a refused one included, and its finding at the node's own path **MUST** be reported; what it would report at or below the refused content is from inside it.

#### One finding per defect

A defect two checks find **MUST** be reported once, by the check named here:

- an attribute the instance lacks, by `required` on the template layer where the template requires it, and on the RM layer where only the RM mandates it, since a template requirement already implies the RM's;
- an RM-mandatory container that is present and empty, by the template's finding where the template reports that container at the same path, not by the RM layer's `cardinality`;
- a list that holds no item, by the template's finding where the template reports that list at the list's path or at a path below it (the list's path followed by an attribute or a predicate, as where the template reports the occurrences of a child the list allows, `/content[at0001]`), not by the rule of § REQ-112 *Non-empty lists* that reads it, at the list's path or, for a HISTORY's `events`, at the HISTORY's.
  For a HISTORY's `events`, likewise where the template reports that HISTORY's `summary`, at the summary's path or below it;
- a part of a value a primitive constraint binds that the RM rules report as absent or empty (§ REQ-102 *Primitive constraints*), by the RM rules, unless the walk stops at the depth limit or the visit limit before they report it, when the constraint judges the part as it judges any value;
- an empty CODE_PHRASE `code_string` and an empty temporal `value`, by the required-set walk's `required` (§ REQ-112 *Per-RM-type invariant catalogue*).

A member a decode reports missing (§ REQ-112 *Value-typed mandatory presence*) is read as absent at its path, so it is reported by the absent-attribute finding alone, the template's or the RM's, and nothing is reported below it.

An RM rule's own finding at the same path, such as a code outside its terminology group, is a second defect, reported beside the first.

#### Cycles and limits

A Go value reached from two places, a pointer shared between two attributes, **MUST** be validated at each place, against each place's template node, so a defect at the second place is reported at the second path.
A value the walk reaches again inside itself, on the path it is walking, is a cycle: each place it recurs is reported as `cyclic_value` at that path and not entered.
The engine detects a cycle on the active ancestry alone and keeps no record of values it has finished.
A decoded document holds neither a cycle nor a shared value; only a value built in Go can.
A value held by value whose list shares its own backing array is no cycle the walk can recognise, which the visit limit bounds.
At the depth limit the engine **MUST** stop descending and report `max_depth` once per walk, at the first path where it stopped, and at the visit limit stop and report `max_nodes` once, at the path where it stopped.
A decoded document can reach either limit: a large one the visit limit, a deeply nested one the depth limit.
Both limits are the `Limits` of *Surface*, with its defaults, and *Coverage* lists each path where one stopped the walk in `Coverage.Stopped`.

#### Trials

Binding a value among the children of one tier, or an item among those of one rank (§ REQ-102 *Structural — alternatives*, *Structural — binding items*), walks it under each child it tries, in OPT order, until one validates it.
That trial is part of the walk, and the order of the steps and the union rule do not change inside it.
A trial **MUST** obey three rules:

- its visits count toward the visit limit, and the depth limit applies inside it;
- a trial in which the depth limit stops the walk never counts as one under which the value validates, whether or not the walk reports that stop there;
- nothing a trial under a child the value does not bind to found reaches the result: no issue, `max_depth` included, no `Coverage` entry and no class for `Unevaluated`.

The second rule keeps the binding from turning on what came before it: the walk reports `max_depth` once, so a stop an earlier part of the walk reported leaves none to report inside the trial.
A value of a single-valued attribute under k children of one tier so costs at most k walks of its subtree, and an item of a list at most k + 1.
Where the visit limit ends the walk inside a trial, no binding is decided at that place: of what its trials found, the engine **MUST** report only the `max_depth` and `max_nodes` issues of the trial in which the walk ended, and, where that trial is of an item of a list, at the path of the list's attribute.
The item's own path waits on the binding the stop left undecided, so it would change with the limit.

### Finding identity

```go
type Issue struct {
    Path     string               // § REQ-102 Issue paths; empty for a global issue
    Code     string               // § REQ-102 Issue codes
    Layer    Layer                // LayerTemplate, LayerRM, or LayerNone
    Rule     string               // the clause this issue evaluates; empty with LayerNone
    Detail   string               // human-readable message
    Severity Severity             // § REQ-102 Contract: Error, or Warning for an advisory
    Value    constraints.Redacted // § REQ-168
}

type Layer uint8
const (
    LayerNone     Layer = iota // a guard, a walk condition, or an issue from ValidateAQL
    LayerTemplate              // a template clause
    LayerRM                    // an RM rule or an RM-mandatory presence
)
func (l Layer) String() string // "", "template", "rm"; any other value "layer(N)"
```

Every issue the instance validators report **MUST** name its `Layer` and carry a non-empty `Rule`, except the issues that judge the call or the walk rather than the instance: the guards (`nil_root`, `nil_template`, `nil_composition`, the nil wrappers' codes, `rm_type_unknown`, and `invalid_shape` on undecodable input) and the walk conditions (`max_depth`, `max_nodes`, `cyclic_value`) carry no layer and an empty `Rule`.
An issue a template clause produces is on the template layer; one an RM rule or an RM-mandatory presence produces is on the RM layer.
An attribute the template walk finds absent is on the template layer when the template requires it by its existence, a list included ([§ REQ-102](#req-102--composition-validation) *An existence lower bound alone asks for no member*), and on the RM layer when only the RM marks it mandatory.
An issue from `ValidateAQL` or `ValidateAQLWithTypeRelation` is not an instance finding and carries neither.

`Rule` names the clause the issue evaluates:

- on the RM layer, for a rule the pinned BMM names, `CLASS.Invariant`: the class that declares the invariant in the pinned RM or BASE BMM and the invariant's name as that BMM spells it (`DV_INTERVAL.Limits_consistent`, `ELEMENT.Inv_null_flavour_indicated`, `ENTRY.Language_valid` on any concrete entry, `Interval.Lower_included_valid` on any interval, `TERM_MAPPING.Match_valid`).
  That is the key the inventory of *Where the RM rules come from* uses;
- on the RM layer, for a rule the SDK applies that the BMM states in an attribute's definition rather than as a named invariant, a lower-case name with no class:
  - `mandatory` for an RM-mandatory attribute the instance lacks, a mandatory string left empty included, and for an item of a list that holds no RM value (§ REQ-112 *Driver*);
  - `cardinality` for an RM-mandatory container that is present and empty;
  - `precision_range` for a `precision` below -1;
  - `archetype_root_id` for an archetype root whose `archetype_details` names another archetype than its `archetype_node_id` (§ REQ-112 *Archetype roots*);
- on the template layer, one of:
  - `existence` (an attribute the template requires and the instance lacks, or one it prohibits and the instance carries);
  - `cardinality` (the attribute's item count);
  - `occurrences` (a child's count of bound items);
  - `slot` (an item that fits no child or slot);
  - `type` (type conformance and alternatives);
  - `identity` (archetype and node ids);
  - or, at a primitive leaf, the `Violation.Code` of § REQ-103.

`Code` keeps the vocabulary of § REQ-102 *Issue codes*, so a caller that dispatches on it keeps working.
An issue whose code is already specific to a rule, any code [§ REQ-102 *Issue codes*](#issue-codes) gives an RM-floor rule other than `rm_invariant`, **MUST** carry both that code and the rule's name.
`rm_invariant` stays the code of every other RM rule; `Rule` is what tells them apart, so a caller never needs `Detail` to learn which rule failed.
`Layer` and `Rule` carry structure, never a submitted value ([§ REQ-168 *Field classes*](#field-classes)).

### Where the RM rules come from

Every invariant of the pinned RM and BASE BMMs, BASE's primitive types included, is an entry of an inventory keyed by class and invariant name, classified as one of: *evaluated*; *not mechanical*, with the reason (the rule needs a terminology service, stored history, or a comparison the RM leaves undefined); or *out of scope*, with the reason.
A BMM bump that adds an invariant fails code generation until the invariant is classified ([ADR 0001](../adr/0001-bmm-version-bump-runbook.md)).
An invariant that another evaluated check already decides, the mandatory-presence rule among them, is *evaluated*; its findings carry the rule of the check that reports them (*Finding identity*).

The RM rules the validators apply are the *evaluated* entries together with the rules *Finding identity* takes from an attribute's definition (`mandatory`, `cardinality`, `precision_range`, `archetype_root_id`).
The validators interpret no BMM invariant expression at run time and load no rule from outside the binary.
The openEHR terminology checks of § REQ-112 *Coded invariants* are RM rules, classified in the inventory like any other.

### Structural access

The validators read a node's attributes through access the generator emits for every concrete RM class the registry knows: for each attribute its name, declared RM type, whether it is a container, whether it is present, and its value, by plain field access ([idiom.md § Generics and reflection](idiom.md#generics-and-reflection), [ADR 0028](../adr/0028-generated-rm-attribute-access.md)).
No registered class is a leaf for want of a reader: the walk **MUST** go below every present attribute of every registered class, the entries of a hash included, in ascending key order, and report an RM-mandatory attribute absent on any of them, unless no serialization carries it, the set [§ REQ-112 *Driver*](#driver) leaves out.

Whether an attribute is present **MUST** be decided from its declared type and the Go value alone, as follows.
A pointer is present when it is not nil; an interface when it holds a value that is not a nil pointer; a string when it is not empty; a number or a boolean always; a list or a hash when it is not nil, and it is empty when it is present and holds no item or entry.
A value held by value is present when it is not its type's zero value, an interface field holding a nil pointer counting as zero.
At an attribute the RM marks mandatory, three types are also empty when their content is missing: a DV_TEXT whose `value` is empty, whether held directly or through an interface; a DV_CODED_TEXT with an empty `value` and an empty defining code; a CODE_PHRASE with an empty `code_string` and an empty terminology.
At an optional attribute such a value is present, so the walk enters it and reports what it lacks; an object reference held by value is likewise present unless it is the zero value, so § REQ-112's reference entry reports each missing part at its own path.
Three readings are fixed by § REQ-112: an interval bound is absent as [*Driver*](#driver) says; EHR_STATUS `subject` is always present from the Go value; and ARCHETYPED `archetype_id` and `rm_version` are always present from the Go value, their emptiness being the *ARCHETYPED* entry's to report.
A decode's report of missing members overrides the Go value's reading at each member it names ([*Value-typed mandatory presence*](#value-typed-mandatory-presence)).

### Coverage

```go
type Result struct {
    OK       bool
    Issues   []Issue
    Coverage Coverage
}

type Coverage struct {
    Stopped     []string // paths where a limit stopped the walk
    External    []string // paths of leaves whose value set is outside the SDK (a CONSTRAINT_REF, or a C_CODE_REFERENCE that names a reference set and lists no code)
    NotInRM     []string // paths of declared attributes the RM class of their node does not have, which the walk left out
    Unevaluated []string // "CLASS.Invariant" entries, of classes the walk visited, the SDK does not evaluate
}
func (c Coverage) Complete() bool // Stopped and External are both empty
```

`OK` reports the absence of error findings and nothing more.
`Coverage` **MUST** say what the result does not prove, in four fields:

| Field | Lists |
|---|---|
| `Stopped` | the path of each `max_depth` and `max_nodes` issue, so at most one per limit |
| `External` | in walk order, the path of each leaf the template binds to a `CONSTRAINT_REF` ([§ REQ-103](#req-103--primitive-constraint-introspection)), or to a `C_CODE_REFERENCE` that names a reference set and lists no code, whose value set is bound outside the template ([§ REQ-111](#req-111--public-compiled-template-bridge) *Term definitions and bindings*) and so cannot be consulted |
| `NotInRM` | in walk order and once per node, the path of each attribute the bound template node declares that the compile marks as absent from the node's RM class ([§ REQ-111](#req-111--public-compiled-template-bridge) *Attributes the RM does not have*) and the walk left out (*One walk*) |
| `Unevaluated` | sorted and once each, the invariants of the classes the walk visited, those their ancestors declare included, that the inventory of *Where the RM rules come from* does not classify as evaluated |

A `C_CODE_REFERENCE` that `External` lists is still judged by the code phrase it carries.
`External` does not list a `C_CODE_PHRASE` that names an external terminology without listing codes, which is checked for that terminology, since whether a code exists in an external terminology is outside every check the SDK makes (§ REQ-112 *Out of scope*).
Nor does it list a `C_CODE_REFERENCE` that lists codes or names no reference set, which is checked as a `C_CODE_PHRASE` is.

`Complete` **MUST** report true exactly when `Stopped` and `External` are both empty: no limit stopped the walk, and every constraint it reached could be checked locally.
`Complete` says nothing of `NotInRM`: no instance can carry such an attribute, so its constraint is a defect of the template, not a part of the instance the result failed to check.
Nor does it say anything of `Unevaluated`, which lists RM rules the SDK does not evaluate on any instance, so it is reported beside `Complete` rather than folded into it.
The zero `Coverage`, which a guard and `ValidateAQL` return, is complete.
A coverage path is written by § REQ-102 *Issue paths* and is value-free.

### Out of scope

- Validation during the decode, and unmatched members captured on the value: [STRAND-15](research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types).
- Policy beyond the limits: descent under refused content, a severity policy per rule, a partial-instance mode: [STRAND-16](research-strands.md#strand-16--validator-policy-beyond-the-limits).
- An external terminology service: a seam for one, one question (is this code a member of this value set) with a no-op default, is reserved and not built.
- Rendering an issue path as a Web Template id or a FLAT key; the interval representation.

- **Probes:** PROBE-077 (the invariant matrix over the inventory).

