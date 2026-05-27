# Datamap Format Specification

**Version:** 0.1.0-draft
**Status:** Draft
**Format:** JSON
**Author:** Alessandro Torrisi — [Cadasto](https://cadasto.com)

---

## 1. Why Datamap?

openEHR is a powerful standard for clinical data, but its Reference Model is complex.
Building an openEHR-compliant application today requires deep knowledge of archetypes, RM types,
compositions, paths, terminology bindings, and serialization formats. That complexity is a barrier
for frontend developers, AI integrations, and teams that simply want to store and retrieve
clinical data correctly.

Datamap removes that barrier.

**Datamap** is a JSON format that lets you read and write openEHR data without
having to understand the full Reference Model. You load a template, you get a simple JSON structure
to fill in, and the SDK takes care of the rest — validation, defaults, and conversion to a fully
compliant openEHR Composition.

### What you get

1. **A datamap** — a simple, flat-ish JSON structure for reading and writing clinical data.
   No RM paths, no technical attributes, no XML. Just keys and values.

2. **A JSON Schema** — generated from the same template, describing exactly which fields exist,
   which are required, and what values are allowed. Use it to render a form, validate input
   client-side, or generate UI components — all without knowing anything about openEHR internals.

3. **Full openEHR compliance** — the SDK converts your simple datamap into a valid openEHR
   Composition (and back). You stay compliant without writing a single line of RM-aware code.

### Who is this for?

- **Frontend developers** who need to build clinical forms from templates.
- **Backend developers** who want to read/write openEHR data through a simple JSON API.
- **AI and low-code clients** that need a minimal, predictable data format.
- **Anyone** who wants to work with openEHR without the learning curve.

---

## 2. Overview

A Datamap payload is called a **datamap**. A datamap is always derived from a specific openEHR
template (OPT) and can be round-tripped to and from a full openEHR Composition.

### 2.1 Design Goals

- **Minimal payloads** — clients send only user-entered data; system defaults are inferred.
- **Short and expanded forms** — every data value can be sent as a primitive or as an object.
- **Deterministic round-trip** — datamap → Composition → datamap produces stable output.
- **Template-driven** — all keys, constraints, and allowed values come from the OPT.
- **Language-agnostic** — the format is independent of any SDK or programming language.

---

## 3. Datamap Structure

A datamap is a JSON object with the following top-level structure:

```json
{
  "template_id": "<string>",
  "uid":         "<string>",
  "vuid":        "<string>",
  "composer":    "<string>",
  "language":    "<string>",
  "territory":   "<string>",
  "context":     { ... },
  "content":     { ... }
}
```

### 3.1 Top-Level Fields

| Field         | Type   | Required | RM source | Description |
|---------------|--------|----------|-----------|-------------|
| `template_id` | string | no       | `ARCHETYPE_DETAILS.template_id` | The OPT template identifier. Injected by SDK if absent. |
| `uid`         | string | no       | `OBJECT_VERSION_ID` (first segment) | Short object UID. |
| `vuid`        | string | no       | `OBJECT_VERSION_ID` | Full versioned UID (`uid::system::version`). |
| `composer`    | string | no       | `COMPOSITION.composer` (1..1) | Name of the composing party. Injected by SDK if absent. |
| `language`    | string | no       | `COMPOSITION.language` (1..1) | ISO 639-1 language code (e.g. `"nl"`, `"en"`). Default: from OPT `original_language`. |
| `territory`   | string | no       | `COMPOSITION.territory` (1..1) | ISO 3166-1 territory code (e.g. `"NL"`, `"EN"`). Default: derived from `language`. |
| `context`     | object | no       | `COMPOSITION.context` (0..1) | Event context (see §3.2). Only present for event-type compositions; absent for persistent compositions. |
| `content`     | object | **yes*** | `COMPOSITION.content` (0..*) | Clinical content (see §3.3). |

\* `content` is optional in the openEHR RM (0..*) but required by the Datamap format —
a datamap without content has no purpose.

**Field order:** When serialized, fields SHOULD appear in the order listed above,
for readability and diff stability. SDKs MUST accept fields in any order.

#### uid and vuid — Versioning

The `uid` and `vuid` fields relate to composition versioning in an openEHR system:

- **`uid`** — the object identifier (first segment of the versioned UID). Unique per composition,
  stable across versions.
- **`vuid`** — the full versioned identifier (`uid::system_id::version_tree_id`). Changes with
  each new version.

| Operation | uid | vuid | Description |
|-----------|-----|------|-------------|
| **Create** | omit | omit | The CDR assigns both on first commit. |
| **Update** | one or both | one or both | Identifies which composition (version) to update. |
| **Read** | present | present | Returned by the CDR after commit or query. |

When creating a new composition, leave `uid` and `vuid` absent — the CDR will assign them.

When updating, provide **either or both**:
- If `vuid` is present, the SDK uses it (it already contains the uid, system, and version).
- If only `uid` is present, the SDK uses that.
- If both are present, `vuid` takes precedence.

> **Note on context:** The RM constraint `is_persistent implies context = Void` means that
> persistent compositions (category `431|persistent|`) do not have a context.
> For event compositions (category `433|event|`), context is optional but typically present.

### 3.2 Context

```json
"context": {
  "start_time": "2026-02-01T09:30:00Z",
  "end_time": "2026-02-01T10:00:00Z",
  "location": "Room 3",
  "setting": "238|other care|",
  "health_care_facility": {
    "name": "Hospital XYZ"
  },
  "participations": [
    {
      "function": "requester",
      "performer": { "name": "Dr. Jones" }
    }
  ],
  "other_context": { ... }
}
```

| Field                    | Type          | Required | RM source | Description |
|--------------------------|---------------|----------|-----------|-------------|
| `start_time`             | string        | no       | `EVENT_CONTEXT.start_time` (1..1) | ISO 8601 date-time. Defaults to event time if absent. |
| `end_time`               | string        | no       | `EVENT_CONTEXT.end_time` (0..1) | ISO 8601 date-time. End of the clinical session. |
| `location`               | string        | no       | `EVENT_CONTEXT.location` (0..1) | Free-text location description. |
| `setting`                | string/object | no       | `EVENT_CONTEXT.setting` (1..1) | Clinical setting (DV_CODED_TEXT, short or expanded). Default: from OPT. |
| `health_care_facility`   | object        | no       | `EVENT_CONTEXT.health_care_facility` (0..1) | Facility where the event took place (see §3.2.1). |
| `participations`         | array         | no       | `EVENT_CONTEXT.participations` (0..*) | Other parties involved (see §3.2.2). |
| `other_context`          | object        | no       | `EVENT_CONTEXT.other_context` (0..1) | Template-defined context fields (same key rules as content items). |

All context fields are optional. If omitted, the SDK applies defaults where applicable (see §8).

#### 3.2.1 health_care_facility

A `PARTY_IDENTIFIED` object identifying the facility:

```json
"health_care_facility": {
  "name": "Hospital XYZ",
  "identifiers": [
    { "id": "12345", "issuer": "NHR", "assigner": "NHR", "type": "nhrid" }
  ]
}
```

| Field         | Type   | Required | Description |
|---------------|--------|----------|-------------|
| `name`        | string | no       | Name of the facility. |
| `identifiers` | array  | no       | Array of `DV_IDENTIFIER` objects. |

#### 3.2.2 participations

An array of `PARTICIPATION` objects:

```json
"participations": [
  {
    "function": "requester",
    "performer": { "name": "Dr. Jones" },
    "mode": "face-to-face"
  }
]
```

| Field       | Type          | Required | Description |
|-------------|---------------|----------|-------------|
| `function`  | string/object | no       | Role of the participant (DV_TEXT, short or expanded). |
| `performer` | object        | no       | The participating party (`{ "name": "..." }`). |
| `mode`      | string/object | no       | Communication mode (DV_CODED_TEXT, short or expanded). |
| `time`      | string        | no       | ISO 8601 interval or date-time of participation. |

### 3.3 Content

`content` is an object where each key represents a top-level archetype or section from the template.

```json
"content": {
  "<archetype_id>|<label>": {
    "events": [ ... ]
  }
}
```

Content keys follow the **archetype key format** (see §4.1).

---

## 4. Key Format

Keys in a datamap consist of an **identifier** and an optional **label**, separated by a pipe (`|`):

```
<identifier>|<label>
```

The identifier (archetype ID or at-code) is the machine-readable part — this is what the SDK
uses to match fields to the template. The label is purely for **human readability**. It makes
a datamap self-documenting: a developer or clinician can read the JSON and understand what
each field means without looking up at-codes.

> **The label is always optional.** SDKs MUST ignore the label when processing keys and only
> use the identifier part for matching. A key with a label and a key without a label refer
> to the same field.

### 4.1 Archetype Keys

Top-level content entries and embedded archetype clusters use the format:

```
<archetype_id>|<label>
```

Example — these are equivalent:
```
"openEHR-EHR-OBSERVATION.vital_signs.v1|vital_signs": { ... }
"openEHR-EHR-OBSERVATION.vital_signs.v1": { ... }
```

- `archetype_id` is the full openEHR archetype identifier. **This is the identifying part.**
- `label` is an optional, normalized human-readable name derived from the template's `term_definitions`.
- Label normalization: lowercased, spaces replaced by underscores, special characters removed.

### 4.2 Node Keys

Data items within events and clusters use the format:

```
<at_code>|<label>
```

Example — these are equivalent:
```
"at0006|Systolic": 120
"at0006": 120
```

- `at_code` is the archetype node identifier (e.g. `at0004`). **This is the identifying part.**
- `label` is an optional, human-readable term from the template's `term_definitions`.

### 4.3 Key Parsing

The **identifier** of a key is defined as:

```
identifier = substring before the first '|'
```

If the key contains no `|`, the entire key is the identifier.

Examples:

| JSON key                  | Identifier | Label (ignored for matching) |
|---------------------------|------------|------------------------------|
| `at0006`                  | `at0006`   | *(none)*                     |
| `at0006\|Systolic`      | `at0006`   | `Systolic`                 |
| `at0006\|blood_pressure`  | `at0006`   | `blood_pressure`             |
| `at0006\|foo\|bar`        | `at0006`   | `foo\|bar`                   |

> Only the **first** `|` is significant. Any subsequent `|` characters are part of the label
> and are not semantically meaningful.

### 4.4 Matching Rules

SDKs MUST apply the following rules when resolving keys:

1. **Extract the identifier** — parse the key using the rule above (substring before first `|`).
2. **Accept with or without label** — `at0006|Systolic` and `at0006` both match the same field.
3. **Labels are informational** — SDKs MUST NOT reject a key because its label differs from the template.
   For example, `at0006|Blood_pressure` and `at0006|Systolic` both resolve to `at0006`.
4. **Output includes labels** — when generating a datamap (e.g. from a Composition), SDKs SHOULD
   include labels for readability.

---

## 5. Structural Nodes

### 5.1 Events

OBSERVATION archetypes contain an `events` array. Each event is an object.
openEHR defines two concrete event types: **POINT_EVENT** and **INTERVAL_EVENT**.

#### 5.1.1 POINT_EVENT

A point event represents a single moment in time:

```json
{
  "events": [
    {
      "time": "2026-02-01T09:30:00Z",
      "at0004|Blood pressure": {
        "at0006|Systolic": 120,
        "at0007|Diastolic": 80
      }
    }
  ]
}
```

#### 5.1.2 INTERVAL_EVENT

An interval event represents a period of time with an aggregated value (e.g. average, minimum, maximum).
In addition to `time`, an interval event contains:

| Field            | Type          | Required | Description                                         |
|------------------|---------------|----------|-----------------------------------------------------|
| `width`          | `string`      | yes      | ISO 8601 duration (e.g. `"PT1H"`, `"PT5M"`)        |
| `math_function`  | `string`/`object` | yes  | Coded text — short (`"146\|mean\|"`) or expanded    |
| `sample_count`   | `integer`     | no       | Number of original samples                          |

```json
{
  "events": [
    {
      "time": "2026-02-01T10:30:00Z",
      "width": "PT1H",
      "math_function": "146|mean|",
      "sample_count": 12,
      "at0004|Blood pressure": {
        "at0006|Systolic": 118,
        "at0007|Diastolic": 78
      }
    }
  ]
}
```

The `math_function` field follows the same short/expanded pattern as DV_CODED_TEXT (§6):

| Short                | Expanded                                                    |
|----------------------|-------------------------------------------------------------|
| `"146\|mean\|"`      | `{"code": "146", "value": "mean", "terminology": "openehr"}` |

> **Event type detection:** If `width` is present, the event is an `INTERVAL_EVENT`.
> Otherwise it is a `POINT_EVENT`. SDKs MUST use this rule when generating compositions.
>
> **OPT validation:** In addition to detecting the event type from the datamap, SDKs SHOULD
> validate that the archetype definition in the OPT actually supports the detected event type.
> If an `INTERVAL_EVENT` is submitted but the OPT archetype only constrains `POINT_EVENT`,
> the SDK MUST reject the data with a validation error.

#### 5.1.3 General Rules

- `events` is always an array, even for single events.
- `time` is a reserved key for the event timestamp (ISO 8601 date-time).
- `width`, `math_function`, and `sample_count` are reserved keys for interval events.
- Remaining keys are data items or clusters.

### 5.2 Clusters

Clusters are nested objects. Their key follows §3.1 (archetype key) or §3.2 (node key):

```json
"at0004|Blood pressure": {
  "at0006|Systolic": 120,
  "at0007|Diastolic": 80
}
```

### 5.3 Repeating Nodes

When a template allows multiple occurrences (`max > 1` or unbounded), the value MUST be an array:

```json
"at0004|Notes": [
  "First note",
  "Second note"
]
```

When `max = 1`, the value MUST be a scalar or object (not an array).

---

## 6. Data Values — Short and Expanded

Every openEHR data value has two valid representations: **short** and **expanded**.

**Mixed mode:** Short and expanded forms can be freely mixed within the same payload.
A client may send one field as a short value and another as an expanded object —
there is no requirement to be consistent. For example, this is perfectly valid:

```json
{
  "at0006|Systolic": 120,
  "at0007|Diastolic": { "magnitude": 90, "unit": "mm[Hg]" },
  "at0009|Position": "at0012"
}
```

SDKs MUST accept any combination of short and expanded forms in a single datamap.

### 6.1 DV_TEXT

| Form | Example |
|------|---------|
| Short | `"Patient developing normally"` |
| Expanded | `{ "value": "Patient developing normally", "formatting": "markdown", "hyperlink": "https://...", "language": "en" }` |

Short form maps to `value`. All other fields are optional.

### 6.2 DV_CODED_TEXT

| Form | Example |
|------|---------|
| Short | `"at0014"` |
| Expanded | `{ "code": "at0014", "value": "Normal development", "terminology": "local" }` |

Short form is always the at-code string. `value` and `terminology` can be resolved from the template.

#### External Terminologies

When a field is bound to an external terminology (e.g. SNOMED-CT, LOINC, ICD-10), the
short form uses the pattern `terminology::code`:

| Form | Example |
|------|---------|
| Short | `"SNOMED-CT::386725007"` |
| Expanded | `{ "code": "386725007", "value": "Body temperature", "terminology": "SNOMED-CT" }` |

The `::` separator distinguishes external codes from local at-codes. SDKs MUST parse the
short form as follows:
1. If the value contains `::` → split into `terminology` and `code` (always external, even if code starts with `at`).
2. Otherwise, if the value starts with `at` → local code, `terminology` defaults to `local`.

In the expanded form, `terminology` is required for external codes — it cannot be inferred.

> **Mixed bindings:** A single field may accept both local at-codes and external codes
> when the template defines multiple terminology bindings. The schema `enum` lists only
> the local at-codes; external codes are validated against the terminology binding
> constraints in the OPT.

### 6.3 DV_BOOLEAN

| Form | Example |
|------|---------|
| Short | `true` |
| Expanded | `{ "value": true }` |

### 6.4 DV_COUNT

| Form | Example |
|------|---------|
| Short | `3` |
| Expanded | `{ "magnitude": 3 }` |

### 6.5 DV_QUANTITY

| Form | Example |
|------|---------|
| Short | `12.5` |
| Expanded | `{ "magnitude": 12.5, "unit": "kg" }` |

Short form maps to `magnitude`. `unit` defaults from the template.

> **Short form and multiple units:** When a template constrains a DV_QUANTITY to a single unit
> (e.g. only `mm[Hg]`), the short form is unambiguous — the SDK injects the unit automatically.
> When a template allows **multiple units** (e.g. `cm` and `m`), the short form is ambiguous
> because the unit cannot be inferred. In this case, the expanded form with an explicit `unit`
> is **required**. SDKs MUST reject the short form with a validation error when multiple units
> are allowed, to prevent incorrect unit assumptions in clinical data.

### 6.6 DV_DATE_TIME

| Form | Example |
|------|---------|
| Short | `"2026-02-01T09:30:00Z"` |
| Expanded | `{ "value": "2026-02-01T09:30:00Z" }` |

### 6.7 DV_DATE

| Form | Example |
|------|---------|
| Short | `"2026-02-01"` |
| Expanded | `{ "value": "2026-02-01" }` |

### 6.8 DV_TIME

| Form | Example |
|------|---------|
| Short | `"14:00"` |
| Expanded | `{ "value": "14:00" }` |

### 6.9 DV_ORDINAL

| Form | Example |
|------|---------|
| Short | `"at0016"` |
| Expanded | `{ "code": "at0016", "ordinal": 1, "value": "Mild" }` |

Short form is the at-code. `ordinal` and `value` are resolved from the template.

### 6.10 DV_PROPORTION

| Form | Example |
|------|---------|
| Short | `0.75` |
| Expanded | `{ "type": "percent", "numerator": 75, "denominator": 100 }` |

### 6.11 DV_DURATION

| Form | Example |
|------|---------|
| Short | `"PT30M"` |
| Expanded | `{ "value": "PT30M" }` |

ISO 8601 duration format.

### 6.12 DV_URI / DV_EHR_URI

| Form | Example |
|------|---------|
| Short | `"https://openehr.org"` |
| Expanded | `{ "value": "https://openehr.org" }` |

### 6.13 DV_IDENTIFIER

| Form | Example |
|------|---------|
| Short | `"123456"` |
| Expanded | `{ "id": "123456", "issuer": "Hospital", "assigner": "EHR", "type": "MRN" }` |

Short form maps to `id` only — `issuer`, `assigner`, and `type` are lost.

> **Short form and context loss:** DV_IDENTIFIER carries four attributes (`id`, `issuer`,
> `assigner`, `type`) that together give meaning to the identifier. The short form SHOULD
> only be used when the identifier semantics are implicit from the template context
> (e.g. a field that always represents an MRN). When `issuer`, `assigner`, or `type`
> are needed, use the expanded form.

### 6.14 DV_MULTIMEDIA

| Form | Example |
|------|---------|
| Short | `"https://example.org/image.jpg"` |
| Expanded | `{ "uri": "https://example.org/image.jpg", "mediaType": "image/jpeg", "size": 345678 }` |

Short form must be a valid URI. Binary inline data is not allowed.

### 6.15 DV_PARSABLE

| Form | Example |
|------|---------|
| Short | `"a+b"` |
| Expanded | `{ "value": "a+b", "formalism": "math" }` |

### 6.16 DV_STATE

| Form | Example |
|------|---------|
| Short | `"active"` |
| Expanded | `{ "value": "active", "is_terminal": false }` |

### 6.17 DV_INTERVAL\<T\>

Always an object (no short form):

```json
{ "low": 10, "high": 20 }
{ "low": 10, "high": 20, "low_included": true, "high_included": false }
```

### 6.18 DV_PARAGRAPH

| Form | Example |
|------|---------|
| Short | `["First sentence.", "Second sentence."]` |
| Expanded | `{ "items": [{ "value": "First sentence." }, { "value": "Second sentence." }] }` |

### 6.19 Null Flavours (Missing Data)

In openEHR, a field may be present but explicitly empty, with a reason why the value is
missing. This is represented as a **null flavour** — a coded reason for absence. Null flavours
apply to **any data type** and take priority over the normal value.

A null flavour replaces the value with a `null_flavour` object:

```json
{
  "at0006|Systolic": {
    "null_flavour": "253"
  }
}
```

The `null_flavour` field follows the DV_CODED_TEXT short/expanded pattern (§6.2):

| Form | Example |
|------|---------|
| Short | `{ "null_flavour": "253" }` |
| Expanded | `{ "null_flavour": { "code": "253", "value": "unknown", "terminology": "openehr" } }` |

Standard openEHR null flavour codes (terminology `openehr`):

| Code | Meaning |
|------|---------|
| `253` | unknown |
| `271` | no information |
| `272` | masked |
| `273` | not applicable |

> **Validation:** When `null_flavour` is present, the normal value MUST be absent.
> A field with `null_flavour` satisfies required-field validation — it is not treated as
> a missing value. SDKs MUST map null flavours to the RM `null_flavour` attribute
> on the corresponding `ELEMENT`.

---

## 7. Schema Format

Each template produces a JSON Schema (draft 2020-12) describing the valid datamap structure.

### 7.1 Root Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "<template_id> Datamap",
  "type": "object",
  "additionalProperties": false,
  "required": ["content"],
  "properties": {
    "template_id": { "type": "string" },
    "uid":         { "type": "string" },
    "vuid":        { "type": "string" },
    "composer":    { "type": "string" },
    "context":     { ... },
    "content":     { ... }
  }
}
```

### 7.2 Schema Conventions

- **Self-contained** — no external `$ref`. Schemas MAY use internal `$defs` to define
  reusable data type definitions (e.g. DV_CODED_TEXT, DV_QUANTITY) referenced via `$ref:
  "#/$defs/..."`. This reduces schema size and improves maintainability without breaking
  the self-contained principle. External references remain prohibited.
- **`oneOf`** — data value fields use `oneOf` with the short form and expanded form as alternatives.
- **`alias`** — schema keys use bare `atNNNN`; the `alias` field holds the full `atNNNN|Label`.
  The alias is **informational only** — it provides the suggested label from the template's
  `term_definitions`. Datamap keys do not need to match the alias; per §4.3, only the identifier
  (the part before the first `|`) is used for matching. A datamap key of `at0006|blood_pressure`
  is equally valid as `at0006|Systolic` or just `at0006`.
- **`rmType`** — expanded objects include `rmType` as a `const` property for disambiguation.
- **`additionalProperties: false`** — enforced on all objects.

### 7.3 Occurrences

Template cardinality is reflected in the schema:

| Schema field  | Source | Description |
|---------------|--------|-------------|
| `minOccurs`   | OPT `occurrences.lower` | Minimum occurrences. |
| `maxOccurs`   | OPT `occurrences.upper` | Maximum occurrences (`null` = unbounded). |
| `minItems`    | same | On array types: minimum array length. |
| `maxItems`    | same | On array types: maximum array length. |

### 7.4 UI Metadata

The `ui` property in the schema is a **rendering hint for user interfaces**. It is part of
the schema only — it never appears in the datamap itself and plays no role in data transport
or validation.

Its purpose is to give frontend developers everything they need to render a form field
without consulting the openEHR template directly: what the field is called, what options
are available in a dropdown, and where it lives in the RM path hierarchy.

Every leaf field in the schema includes a `ui` object:

```json
"ui": {
  "path": "content[...]/data[...]/items[at0006]/value",
  "description": "Systolic blood pressure",
  "options": [
    { "code": "at0014", "text": "Normal" },
    { "code": "at0015", "text": "Abnormal" }
  ]
}
```

| Field         | Type   | Present when |
|---------------|--------|-------------|
| `path`        | string | Always. The resolved RM path in the template. |
| `description` | string | When the OPT provides a description for the node. |
| `options`     | array  | For DV_CODED_TEXT and DV_ORDINAL with a code list. Each entry has `code` and `text`. |

Field order within `ui` is fixed: `path`, `description`, `options`.

---

## 8. Implicit Defaults

When a field is absent from the datamap, the SDK applies defaults depending on the field category.

### 8.1 System Fields vs Clinical Fields

Not all missing values are equal. The specification distinguishes two categories:

**System fields** — infrastructure fields with deterministic defaults. Missing values trigger
default injection and are never validation errors:

| Field | Default |
|-------|---------|
| `template_id` | From loaded OPT. |
| `composer` | From `setComposer()` or empty. |
| `language` | From OPT `original_language`. |
| `territory` | Derived from language (e.g. `nl` → `NL`). |
| `encoding` | `UTF-8`. |
| `subject` | `PARTY_SELF` (the patient). |
| `category` | From OPT (default: `event` / `433`). |
| `context.start_time` | Inferred from first event time. |
| `context.setting` | From OPT or SDK default. |

**Clinical fields** — data entered by clinicians, tied to clinical observation. These fields
have no logical default. If a clinical field is required by the OPT (`occurrences.lower > 0`)
and missing from the datamap, the SDK MUST raise a validation error instead of silently
injecting a value.

Defaults for data type metadata within clinical fields are still allowed:

| Field | Default |
|-------|---------|
| DV_CODED_TEXT `value` | Resolved from template `term_definitions`. |
| DV_CODED_TEXT `terminology` | `local` (default). |
| DV_ORDINAL `ordinal`, `value` | Resolved from template. |
| DV_QUANTITY `unit` | From template constraints (single unit only — see §6.5). |

---

## 9. Validation Rules

A datamap can be validated against the schema before building a Composition.

### 9.1 Error Format

```json
{
  "path": "content.development.events[0].dv_coded_text",
  "code": "invalid_enum",
  "message": "Value must be one of: at0014, at0015"
}
```

### 9.2 Validation Checks

- **Required fields** — `content` is always required; other required fields come from OPT `occurrences.lower > 0`.
- **Enum values** — DV_CODED_TEXT and DV_ORDINAL short forms must match a code from the template.
- **Type checking** — values must match their short or expanded type.
- **Cardinality** — array lengths must respect `minItems` / `maxItems`.
- **No unknown keys** — keys not defined in the schema are rejected.

---

## 10. Transport Envelope (Optional)

Clients MAY wrap a datamap in a transport envelope:

```json
{
  "templateId": "vital_signs.v1",
  "language": "nl",
  "datamap": {
    "content": { ... }
  }
}
```

SDKs MUST accept both the raw datamap and the envelope form. When the envelope is used,
`templateId` is promoted to `template_id` inside the datamap.

---

## 11. Conformance

An implementation conforms to this specification if it:

1. Accepts both short and expanded forms for all data values.
2. Accepts both `atNNNN|Label` and bare `atNNNN` keys.
3. Produces datamaps with keys in template-defined order.
4. Generates schemas following §7.
5. Applies implicit defaults per §8.
6. Validates per §9.
7. Round-trips in both directions without data loss:
   - **datamap → Composition → datamap:** the resulting datamap MUST contain the same
     clinical values as the original (keys may be normalised to expanded form).
   - **Composition → datamap → Composition:** the resulting Composition MUST be
     semantically identical to the original — same structure, same values, same types.
     Insignificant differences (XML attribute order, whitespace) are acceptable.

---

## Appendix A — Complete Example: Vital Signs

This appendix shows the same clinical data in three forms: the openEHR Composition (XML),
the Datamap (JSON), and the Datamap Schema (JSON Schema). The template used is `vital_signs.v1`.

The XML composition is what openEHR systems store. The datamap is what clients work with.
The schema describes what the datamap looks like. Together they illustrate the full round-trip.

> For readability, the examples below are trimmed to the **Blood pressure** cluster.
> The full template contains 7 clusters. The full examples are available in the `examples/` directory.

### A.1 openEHR Composition (XML)

This is what an openEHR backend stores. Note the deeply nested structure, RM attributes
(`archetype_node_id`, `xsi:type`, `terminology_id`), and boilerplate that clients should
never have to deal with.

```xml
<?xml version="1.0" encoding="utf-8"?>
<COMPOSITION xmlns="http://schemas.openehr.org/v1"
             xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
             xsi:type="COMPOSITION"
             archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1">
  <name><value>Encounter</value></name>
  <uid xsi:type="OBJECT_VERSION_ID">
    <value>6931a4b8-a7c4-4c4c-b78b-a1f419f56628::5HB6J4AHXS00KOOW::1</value>
  </uid>
  <archetype_details>
    <archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id>
    <template_id><value>vital_signs.v1::d0ed9069-6b66-3c46-9516-15674e8d55cc</value></template_id>
    <rm_version>1.0.2</rm_version>
  </archetype_details>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>nl</code_string>
  </language>
  <territory>
    <terminology_id><value>ISO_3166-1</value></terminology_id>
    <code_string>NL</code_string>
  </territory>
  <category>
    <value>event</value>
    <defining_code>
      <terminology_id><value>openehr</value></terminology_id>
      <code_string>433</code_string>
    </defining_code>
  </category>
  <composer xsi:type="PARTY_IDENTIFIED">
    <name>Dr. Sarah Chen</name>
  </composer>
  <context>
    <start_time xsi:type="DV_DATE_TIME">
      <value>20260202T074336,000+0100</value>
    </start_time>
  </context>

  <content archetype_node_id="openEHR-EHR-OBSERVATION.vital_signs.v1"
           xsi:type="OBSERVATION">
    <name><value>vital_signs</value></name>
    <language>
      <terminology_id><value>ISO_639-1</value></terminology_id>
      <code_string>nl</code_string>
    </language>
    <encoding>
      <terminology_id><value>IANA_character-sets</value></terminology_id>
      <code_string>UTF-8</code_string>
    </encoding>
    <subject xsi:type="PARTY_SELF"/>
    <data archetype_node_id="at0001">
      <name><value>Event Series</value></name>
      <origin><value/></origin>
      <events archetype_node_id="at0002" xsi:type="POINT_EVENT">
        <name><value>Any event</value></name>
        <time><value/></time>
        <data xsi:type="ITEM_TREE" archetype_node_id="at0003">
          <name><value>Tree</value></name>

          <!-- Cluster: Blood pressure -->
          <items archetype_node_id="at0004" xsi:type="CLUSTER">
            <name><value>Blood pressure</value></name>
            <items archetype_node_id="at0005" xsi:type="ELEMENT">
              <name><value>Date</value></name>
              <value xsi:type="DV_DATE_TIME"><value/></value>
            </items>
            <items archetype_node_id="at0006" xsi:type="ELEMENT">
              <name><value>Systolic</value></name>
              <value xsi:type="DV_QUANTITY">
                <magnitude>120</magnitude>
                <units>mm[Hg]</units>
              </value>
            </items>
            <items archetype_node_id="at0007" xsi:type="ELEMENT">
              <name><value>Diastolic</value></name>
              <value xsi:type="DV_QUANTITY">
                <magnitude>90</magnitude>
                <units>mm[Hg]</units>
              </value>
            </items>
            <items archetype_node_id="at0008" xsi:type="ELEMENT">
              <name><value>Pulse rate</value></name>
              <value xsi:type="DV_QUANTITY">
                <magnitude>76</magnitude>
                <units>/min</units>
              </value>
            </items>
            <items archetype_node_id="at0009" xsi:type="ELEMENT">
              <name><value>Position</value></name>
              <value xsi:type="DV_CODED_TEXT">
                <value>standing</value>
                <defining_code>
                  <terminology_id><value>local</value></terminology_id>
                  <code_string>at0012</code_string>
                </defining_code>
              </value>
            </items>
          </items>

          <!-- ... 6 more clusters omitted for brevity ... -->
        </data>
      </events>
    </data>
  </content>
</COMPOSITION>
```

**~100 lines of XML** for a single blood pressure measurement with 5 fields.

### A.2 Datamap (short form)

The same data as a Datamap. This is what a frontend sends and receives:

```json
{
  "template_id": "vital_signs.v1",
  "uid": "6931a4b8-a7c4-4c4c-b78b-a1f419f56628",
  "vuid": "6931a4b8-a7c4-4c4c-b78b-a1f419f56628::5HB6J4AHXS00KOOW::1",
  "composer": "Dr. Sarah Chen",
  "context": {
    "start_time": "2026-02-02T07:43:36+01:00"
  },
  "content": {
    "openEHR-EHR-OBSERVATION.vital_signs.v1|vital_signs": {
      "events": [
        {
          "time": "2026-02-02T07:43:36+01:00",
          "at0004|Blood pressure": {
            "at0005|Date": "2026-02-02T07:43:36+01:00",
            "at0006|Systolic": 120,
            "at0007|Diastolic": 90,
            "at0008|Pulse rate": 76,
            "at0009|Position": "at0012"
          }
        }
      ]
    }
  }
}
```

**20 lines.** Same data, no RM noise. A developer can read, build, and submit this without
knowing what `ITEM_TREE`, `POINT_EVENT`, or `defining_code` means.

### A.3 Datamap (expanded form)

The same data in expanded form, showing the full object structure:

```json
{
  "template_id": "vital_signs.v1",
  "composer": "Dr. Sarah Chen",
  "content": {
    "openEHR-EHR-OBSERVATION.vital_signs.v1|vital_signs": {
      "events": [
        {
          "time": "2026-02-02T07:43:36+01:00",
          "at0004|Blood pressure": {
            "at0005|Date": {
              "value": "2026-02-02T07:43:36+01:00"
            },
            "at0006|Systolic": {
              "magnitude": 120,
              "unit": "mm[Hg]"
            },
            "at0007|Diastolic": {
              "magnitude": 90,
              "unit": "mm[Hg]"
            },
            "at0008|Pulse rate": {
              "magnitude": 76,
              "unit": "/min"
            },
            "at0009|Position": {
              "code": "at0012",
              "value": "standing",
              "terminology": "local"
            }
          }
        }
      ]
    }
  }
}
```

Both forms are valid. They can be mixed freely within the same payload.

### A.4 Datamap (empty)

An empty datamap as returned by `getDatamap(empty: true)`. This is the starting point for
building a form — all fields are present with `null` values:

```json
{
  "template_id": "vital_signs.v1",
  "content": {
    "openEHR-EHR-OBSERVATION.vital_signs.v1|vital_signs": {
      "events": [
        {
          "at0002|Any event": null,
          "at0004|Blood pressure": {
            "at0005|Date": null,
            "at0006|Systolic": null,
            "at0007|Diastolic": null,
            "at0008|Pulse rate": null,
            "at0009|Position": null
          },
          "at0013|Height weight": {
            "at0014|Date": null,
            "at0015|Height": null,
            "at0016|Weight": null,
            "at0017|BMI": null,
            "at0018|Waist": null
          },
          "at0019|Temperature": {
            "at0020|Date": null,
            "at0021|Temperature": null,
            "at0046|Measurement type": null
          },
          "at0022|Defecation": {
            "at0023|Date": null,
            "at0024|Stool chart": null
          },
          "at0033|Blood glucose": {
            "at0034|Date": null,
            "at0036|Value": null,
            "at0035|Comments": null
          },
          "at0037|Anticoagulation": {
            "at0038|Date": null,
            "at0039|INR": null,
            "at0041|Sintrom dosage": null,
            "at0040|Comments": null,
            "at0045|Next INR": null
          },
          "at0042|Oxygen saturation": {
            "at0043|Date": null,
            "at0044|Saturation": null
          }
        }
      ]
    }
  }
}
```

This empty datamap reveals the full template structure: 7 clusters with 24 fields total.
A UI framework can iterate this structure to generate a form automatically.

### A.5 Datamap Schema (JSON Schema, excerpt)

The schema for the Blood pressure cluster. Generated from the template, it describes the exact
structure, types, and constraints for each field.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "vital_signs.v1 Datamap",
  "type": "object",
  "additionalProperties": false,
  "required": ["content"],
  "properties": {
    "template_id": { "type": "string" },
    "uid": { "type": "string" },
    "vuid": { "type": "string" },
    "composer": { "type": "string" },
    "content": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "openEHR-EHR-OBSERVATION.vital_signs.v1|vital_signs": {
          "type": "object",
          "additionalProperties": false,
          "required": ["events"],
          "properties": {
            "events": {
              "type": "array",
              "items": {
                "type": "object",
                "additionalProperties": false,
                "required": ["time"],
                "properties": {
                  "time": {
                    "type": "string",
                    "format": "date-time",
                    "ui": {
                      "path": "content[openEHR-EHR-OBSERVATION.vital_signs.v1]/data[at0001]/events[at0002]/time"
                    }
                  },
                  "at0004": {
                    "alias": "at0004|Blood pressure",
                    "type": "object",
                    "additionalProperties": false,
                    "properties": {
                      "at0005": {
                        "alias": "at0005|Date",
                        "oneOf": [
                          { "type": "string", "format": "date-time" },
                          {
                            "type": "object",
                            "additionalProperties": false,
                            "required": ["value"],
                            "properties": {
                              "value": { "type": "string", "format": "date-time" },
                              "rmType": { "const": "DV_DATE_TIME" }
                            }
                          }
                        ],
                        "ui": {
                          "path": "content[...]/items[at0004]/items[at0005]/value"
                        },
                        "minOccurs": 0,
                        "maxOccurs": 1
                      },
                      "at0006": {
                        "alias": "at0006|Systolic",
                        "oneOf": [
                          { "type": "number" },
                          {
                            "type": "object",
                            "additionalProperties": false,
                            "required": ["magnitude"],
                            "properties": {
                              "magnitude": { "type": "number" },
                              "unit": { "type": "string" },
                              "rmType": { "const": "DV_QUANTITY" }
                            }
                          }
                        ],
                        "ui": {
                          "path": "content[...]/items[at0004]/items[at0006]/value"
                        },
                        "minOccurs": 0,
                        "maxOccurs": 1
                      },
                      "at0009": {
                        "alias": "at0009|Position",
                        "oneOf": [
                          { "type": "string", "enum": ["at0010", "at0011", "at0012"] },
                          {
                            "type": "object",
                            "additionalProperties": false,
                            "required": ["code"],
                            "properties": {
                              "code": { "type": "string", "enum": ["at0010", "at0011", "at0012"] },
                              "value": { "type": "string" },
                              "terminology": { "type": "string" },
                              "rmType": { "const": "DV_CODED_TEXT" }
                            }
                          }
                        ],
                        "ui": {
                          "path": "content[...]/items[at0004]/items[at0009]/value",
                          "options": [
                            { "code": "at0010", "text": "lying" },
                            { "code": "at0011", "text": "sitting" },
                            { "code": "at0012", "text": "standing" }
                          ]
                        },
                        "minOccurs": 0,
                        "maxOccurs": 1
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
```

Key observations:

- **`oneOf`** — every field accepts both its short form (e.g. `number`) and expanded form (e.g. `{ magnitude, unit }`).
- **`alias`** — the schema key is `at0006`, the alias is `at0006|Systolic`. The alias is informational — any label works (e.g. `at0006|blood_pressure`), only the identifier `at0006` matters (see §4.3).
- **`ui.options`** — for coded fields like Position, the schema includes the allowed values with human-readable labels.
- **`ui.path`** — the RM path for each field, useful for mapping back to the openEHR model.
- **`rmType`** — in expanded objects, identifies the openEHR data type.
- **`minOccurs` / `maxOccurs`** — template cardinality, directly from the OPT.

> The full schema for this template is ~1000 lines. See `examples/vital_signs_v1.dmv2.schema.json`.
