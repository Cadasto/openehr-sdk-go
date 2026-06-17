# Datamap V2 (REQ-058)

Status: implementation in flight on `cadasto/datamap`. This document is the canonical normative source for the Datamap-V2 wire shape consumed and produced by `cadasto/datamap/{ToComposition,FromComposition,Empty,Validate}` and is referenced from [`REQ.md`](REQ.md).

## Purpose

Datamap is the Cadasto-specific **resource-free** payload format used to read and write clinical and demographic openEHR data without forcing callers to construct full RM instances. The SDK consumes V2; V1 is out of scope ([glossary.md § Datamap](glossary.md)).

Two boundaries to remember:

- **Datamap is not openEHR canonical JSON.** It uses `archetype-id|Label` keys, lifts at-codes to labelled keys (`at0005|Test result name`), and accepts "bare" payload shapes for primitive values that the encoder expands against the OPT.
- **The codec maps Datamap V2 ↔ canonical JSON.** Two profiles share the wire conventions:
  - **Composition profile** — `ToComposition` / `FromComposition` for `COMPOSITION` OPTs.
  - **Party profile (Option B)** — `ToParty` / `FromParty` for demographic PARTY OPTs (`PERSON`, `ORGANISATION`, `AGENT`, `GROUP`, `ROLE`, …). `Schema(opt)` and `Empty(opt)` auto-select via `IsPartyTemplate(opt)`.

## Party profile (Option B)

Demographic templates root on a PARTY subtype instead of `COMPOSITION`. The party datamap uses the same labelled-key and short/expanded value rules as compositions, but top-level structure follows the RM party attributes:

| Datamap key | RM attribute | Notes |
|-------------|--------------|-------|
| `identities` | `identities[]` | Keyed by `PARTY_IDENTITY` archetype id \| purpose label |
| `details` | `details` | Party-level ITEM_TREE |
| `contacts` | `contacts[]` | Array; each entry keyed by contact node \| label, holding address archetypes |
| `relationships` | `relationships[]` | `source` / `target` party refs + optional details tree |
| `roles` | `roles[]` | Party ref objects (`id`, `namespace`, `type`) |
| `languages` | `languages[]` | Plain strings |

Coded runtime names on ADDRESS and PARTY_IDENTITY use `_code` / `_name` (same as cluster names in compositions).

See [`../plans/2026-06-16-datamap-demographics.md`](../plans/2026-06-16-datamap-demographics.md) for the implementation plan and phase breakdown.

## Terminology binding: short form vs expanded form

Coded references inside Datamap V2 (cluster runtime names, value-side terminology mappings, etc.) MUST accept two interchangeable wire shapes. Both forms produce the same canonical-JSON output and the codec MUST treat them as equivalent on input.

| Form     | Example                                                                       |
|----------|-------------------------------------------------------------------------------|
| Short    | `"SNOMED-CT::386725007"`                                                       |
| Expanded | `{ "code": "386725007", "value": "Body temperature", "terminology": "SNOMED-CT" }` |

### Parsing rules (REQ-058)

The codec MUST parse the short form as follows:

1. If the value contains `::` → split into `terminology` and `code` (always external, even if `code` starts with `at`).
2. Otherwise, if the value starts with `at` → local at-code; `terminology` defaults to `local`.
3. Otherwise → local arbitrary code; `terminology` defaults to `local`.

The expanded form is unambiguous: `terminology` is read verbatim; when absent it defaults to `local`. `value` is the display text.

The `::` separator is reserved for the short-form discriminator. It MUST NOT appear inside a `code` field of the expanded form.

## Cluster runtime name (`_name`, `_code`)

In Datamap V2 the runtime `name` attribute of a `CLUSTER` (RM type `DV_TEXT`, subtypes `DV_CODED_TEXT`/`DV_PARAGRAPH`) is expressed via two reserved keys on the cluster payload:

- `_name` — display string (DV_TEXT case). MAY be omitted; encoder falls back to the archetype's term label.
- `_code` — coded reference (DV_CODED_TEXT case). Accepts either a short-form string or an expanded object per the table above. When present, the encoder emits the cluster name as `DV_CODED_TEXT` whose `value` is `_name` (falling back to the archetype label) and whose `defining_code` is built from the parsed `(terminology, code)`.

Examples:

```jsonc
// DV_TEXT (plain) — display falls back to archetype label "Result"
"at0096|Result": [{
  "at0078|Result value": { "rmType": "DV_QUANTITY", "magnitude": 78.7, "units": "umol/L" }
}]

// DV_CODED_TEXT (short form)
"at0096|Result": [{
  "_code": "SNOMED-CT::386725007",
  "_name": "Body temperature",
  "at0078|Result value": { "rmType": "DV_QUANTITY", "magnitude": 37.0, "units": "Cel" }
}]

// DV_CODED_TEXT (expanded form) — REQUIRED to be accepted; equivalent to the short form above
"at0096|Result": [{
  "_code": { "code": "386725007", "value": "Body temperature", "terminology": "SNOMED-CT" },
  "at0078|Result value": { "rmType": "DV_QUANTITY", "magnitude": 37.0, "units": "Cel" }
}]
```

## RM substitutability on decode

When the codec round-trips a Datamap-V2-derived composition through the typed `*rm.Composition` path (e.g. `care.SaveData` preflight, `canjson.Unmarshal`), the decoder MUST honour RM substitutability for the cluster `name` attribute:

> Since `DV_CODED_TEXT` is a subtype of `DV_TEXT`, it can be used in place of it. — `rm.DVCodedText` doc-comment, derived from openEHR RM §`data_types.text`

Concretely: a payload `{"_type": "DV_CODED_TEXT", …}` MUST be accepted in any slot typed as `DV_TEXT`, with the subtype concretely decoded (the `defining_code` MUST survive the round-trip). The Go surface for such slots is the `rm.DataValueText` marker interface, satisfied by `*rm.DVText`, `*rm.DVCodedText`, and `*rm.DVParagraph`.

## Conformance probes

Probes attached to REQ-058 in [traceability.yaml](traceability.yaml):

- `PROBE-058a` — short-form `_code` round-trips via Datamap → canonical-JSON → `*rm.Composition` → canonical-JSON and re-encodes identically.
- `PROBE-058b` — expanded-form `_code` round-trips to the same canonical-JSON as its short-form equivalent (interchange contract).
- `PROBE-058c` — a `DV_CODED_TEXT` payload in a cluster `name` slot decodes losslessly: the resulting `*rm.Composition`'s `Cluster.Name` is a `*DVCodedText` (not `*DVText`), and re-encoding preserves `defining_code`.
- `PROBE-058d` — encoder rejects a malformed expanded-form `_code` (missing `code`, both `::` in `code` field, unknown extra fields).
