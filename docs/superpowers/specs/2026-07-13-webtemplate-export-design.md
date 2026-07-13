# Design — WebTemplate JSON export

**Date:** 2026-07-13
**Status:** Approved (brainstorming) — feeds `sdd-specify` (REQ-106, ADR-0014) + `writing-plans`
**Feature:** project the landed compiled OPT into the EHRbase-flavoured **WebTemplate** JSON.
**Plan:** [`../../plans/2026-05-22-webtemplate-export.md`](../../plans/2026-05-22-webtemplate-export.md) (umbrella Phase 2; direct-slice decision recorded here)
**Umbrella:** [`../../plans/2026-06-23-simplified-formats.md`](../../plans/2026-06-23-simplified-formats.md)

> This is **input narrative**, not normative truth. On approval, the capability is captured as **REQ-106** and the behaviour is written into the canonical [`clinical-modeling.md`](../../specifications/clinical-modeling.md) spec via `sdd-specify`; the reference-implementation lock becomes **ADR-0014**. When those land, the spec + ADR win over this file.

## 1. Purpose & scope

Consumers (form renderers, data-entry UIs, the CDR) need the simplified, flattened **WebTemplate** projection of an operational template — a lossy, UI-oriented view of the OPT that lists each node's id, cardinality, AQL path, and the leaf **inputs** a form must render. The SDK already compiles an OPT into an introspectable tree (REQ-100/111, [`openehr/templatecompile`](../../../openehr/templatecompile/)); this feature adds the JSON **export** of that tree in the WebTemplate shape.

**In scope (this slice):**

- A new `openehr/template/webtemplate` package exporting `*templatecompile.Compiled` → WebTemplate JSON.
- The EHRbase openEHR_SDK **v2.3** WebTemplate shape (`version: "2.3"`, lower-snake `id`s).
- Core clinical datatype coverage for `inputs` (see §6).
- Structural parity with a vendored EHRbase reference fixture (PROBE-075) + round-trip goldens.

**Out of scope (deferred, documented):**

- The shared simplified-template model abstraction — extracted only when the FLAT/STRUCTURED codecs (REQ-053) land and a second consumer exists (YAGNI until then; umbrella plan tracks it).
- FLAT / STRUCTURED composition serialization (REQ-053).
- WebTemplate → OPT round-trip (import).
- Serving the export over a REST endpoint / content negotiation — the media type `application/openehr.wt+json` is **documented only**, not emitted by this package.
- Exotic datatype inputs (DV_MULTIMEDIA / DV_PARSABLE / DV_IDENTIFIER / DV_INTERVAL) — nodes emit **without** inputs and are recorded as documented gaps.
- The Better/Kotlin camelCase `id` variant — EHRbase lower-snake only.

## 2. Reference implementation lock (→ ADR-0014)

WebTemplate is a **de facto** format with no openEHR-normative schema; two reference implementations diverge (Better/Kotlin camelCase ids; EHRbase openEHR_SDK Java lower-snake ids, version `"2.3"`). We anchor to **EHRbase openEHR_SDK v2.3** so parity is verifiable against a concrete artefact. ADR-0014 records: chosen implementation + version, the `id`-generation algorithm, and that we target **structural** parity (id/rmType/aqlPath/cardinality/input-shape), **not** byte-parity — field ordering, absent optional fields, and localized-string packaging may differ, captured in a documented-deviations list.

Fixture provenance: EHRbase openEHR_SDK is Apache-2.0; the `corona_anamnese` OPT + reference WebTemplate JSON are vendored from a pinned commit with a provenance + license note. If the environment blocks the fetch, we fall back to **goldens-first** (our own generated goldens as the regression anchor) and flag PROBE-075 as deferred with the reason recorded.

## 3. Package layout

```
openehr/template/webtemplate/
  doc.go            // package doc — what WebTemplate is, the v2.3 lock, deviations
  webtemplate.go    // public WebTemplate / Node / Input structs (+ Marshal / Build)
  build.go          // *templatecompile.Compiled → *WebTemplate recursive walk
  id.go             // web-id generation + sibling disambiguation  ← consumer-critical
  inputs.go         // per-RM-datatype inputs[] mapping (core clinical subset)
  webtemplate_test.go / build_test.go / id_test.go / inputs_test.go
  testdata/webtemplate/*.json   // round-trip goldens per existing OPT fixture
```

**Dependencies (building-block independence, REQ-013):** `openehr/templatecompile` (public compiled bridge, the input type), `openehr/template` (OPT metadata: template id, languages), `rminfo` (RM datatype introspection), stdlib `encoding/json`. **No** `transport` / `auth` import.

**Placement rationale:** nested under `openehr/template/` (as directed) to group it with template machinery and to distinguish it by name from the other template representations (OET, operational/ADL, native `.t.json`). Package name `webtemplate`; functions `webtemplate.Build` / `webtemplate.Marshal`. It imports the sibling public bridge `openehr/templatecompile` (no import cycle — `template` does not import `template/webtemplate`).

## 4. Public API

```go
package webtemplate

// Build projects a compiled operational template into the typed WebTemplate tree.
func Build(c *templatecompile.Compiled, opts ...Option) (*WebTemplate, error)

// Marshal is Build followed by deterministic JSON encoding.
func Marshal(c *templatecompile.Compiled, opts ...Option) ([]byte, error)

type Option func(*config) // e.g. WithDefaultLanguage(code), WithLanguages(...), WithVersion(v)
```

`Build` returns the typed tree so callers can post-process before encoding; `Marshal` is the common path. Errors are returned (not panics) for a nil/empty compiled input or an unresolvable default language.

## 5. Structs (EHRbase v2.3 shape — camelCase JSON, `omitempty`)

```go
type WebTemplate struct {
    TemplateID      string   `json:"templateId"`
    Version         string   `json:"version"`          // "2.3"
    DefaultLanguage string   `json:"defaultLanguage"`
    Languages       []string `json:"languages"`
    Tree            *Node    `json:"tree"`
}

type Node struct {
    ID                   string            `json:"id"`
    Name                 string            `json:"name,omitempty"`
    LocalizedName        string            `json:"localizedName,omitempty"`
    RMType               string            `json:"rmType"`
    NodeID               string            `json:"nodeId,omitempty"`
    Min                  int               `json:"min"`
    Max                  int               `json:"max"`               // -1 = unbounded
    LocalizedNames       map[string]string `json:"localizedNames,omitempty"`
    LocalizedDescriptions map[string]string `json:"localizedDescriptions,omitempty"`
    AQLPath              string            `json:"aqlPath"`
    Inputs               []Input           `json:"inputs,omitempty"`
    Children             []*Node           `json:"children,omitempty"`
    // sourced-when-present: TermBindings, Annotations, Cardinalities, InContext
}

type Input struct {
    Suffix      string          `json:"suffix,omitempty"`
    Type        string          `json:"type"`               // TEXT|CODED_TEXT|DECIMAL|INTEGER|BOOLEAN|DATE|DATETIME|TIME|QUANTITY|COUNT|PROPORTION|DURATION
    List        []InputListItem `json:"list,omitempty"`
    ListOpen    bool            `json:"listOpen,omitempty"`
    Validation  *Validation     `json:"validation,omitempty"`
    Terminology string          `json:"terminology,omitempty"`
    DefaultValue any            `json:"defaultValue,omitempty"`
}
// InputListItem{Value,Label,Ordinal,LocalizedLabels,...}; Validation{Range,Precision,Pattern,...}
```

**Determinism:** fixed struct field order + `omitempty`; Go's `encoding/json` sorts map keys — so re-encoding a given OPT is byte-stable, which the round-trip goldens (§7) assert.

## 6. The transform (`build.go`)

Recursive walk of the compiled tree:

- Root `Node` from `c.Root()`; recurse through `CompiledNode.Attributes()` → child nodes.
- Field mapping via the confirmed accessors: `RMTypeName()`→`rmType`, `NodeID()`/`ArchetypeID()`→`nodeId`, `AQLPath()`→`aqlPath`, `Occurrences()`→`min`/`max` (unbounded → `-1`), `Term(code, lang)`→`name` + localized maps.
- `IsSlot()` nodes: emitted as tree nodes (an unfilled slot has no children); slot-fill expansion is the compiler's concern, not ours.
- The WebTemplate tree **keeps** structural RM nodes (HISTORY / EVENT / ITEM_TREE) — "level removal" is a FLAT-path concern (REQ-053), not the tree shape. This keeps the transform a straight structural mirror.
- `defaultLanguage` / `languages` from the OPT metadata (`openehr/template`), overridable via `Option`.

## 7. Web-id generation (`id.go`) — consumer-critical

The `id` is the FLAT-path segment a form binds to, so its stability and parity matter most. Algorithm (locked by ADR-0014, verified against the fixture):

- Base = node's default-language display name (`Term` text); lowercase; transliterate/strip diacritics; non-alphanumeric runs → single `_`; trim leading/trailing `_`; digit-leading guard.
- Fallback to a normalized RM-type token when a node has no display name.
- **Sibling disambiguation:** when two siblings normalize to the same id, apply the EHRbase suffix rule (empirically derived from the fixture, pinned in `id_test.go`).

This is the one piece we do **not** invent: the exact normalization + disambiguation is derived from the vendored `corona_anamnese` reference (name→id parity) and pinned by table-driven unit tests.

## 8. Inputs mapping (`inputs.go`) — core clinical subset

Each ELEMENT's value constraint (via `PrimitiveConstraint()` / the DV child C_COMPLEX) maps to `inputs[]`:

| RM datatype | inputs |
|---|---|
| DV_TEXT | one `{type: TEXT}` (no suffix) |
| DV_CODED_TEXT | `{suffix: "code", type: CODED_TEXT, list[], listOpen, terminology}` |
| DV_QUANTITY | `{suffix: "magnitude", type: DECIMAL, validation}` + `{suffix: "unit", type: CODED_TEXT, list[]}` |
| DV_COUNT | `{type: INTEGER/COUNT, validation}` |
| DV_ORDINAL | one CODED_TEXT with ordinal `list[]` (value/label/ordinal) |
| DV_DATE_TIME / DV_DATE / DV_TIME | `{type: DATETIME/DATE/TIME, validation.pattern}` |
| DV_BOOLEAN | `{type: BOOLEAN}` |
| DV_PROPORTION | `{suffix: "numerator"}` + `{suffix: "denominator"}` (+ `proportionTypes`) |

**Exotic types** (DV_MULTIMEDIA / DV_PARSABLE / DV_IDENTIFIER / DV_INTERVAL): node emitted **without** inputs and appended to a documented gaps list — never a silent error, never a panic.

## 9. Testing & conformance

- **Unit (TDD, red-first):** `id_test.go` (normalization, diacritics, sibling disambiguation), `inputs_test.go` (each datatype row above), `build_test.go` (tree shape, min/max, structural-node retention).
- **Round-trip goldens:** `testdata/webtemplate/<opt>.json` for a few existing OPT fixtures (e.g. `minimal_observation`, `body_weight`, `Demonstration.v1`); the test regenerates and asserts **byte-equality** (determinism guard).
- **PROBE-075 (structural parity):** vendor EHRbase's pinned `corona_anamnese.opt` + reference `corona_anamnese.json` under `testkit/cassettes/webtemplate/` (provenance + Apache-2.0 note); build ours from the OPT and compare **structurally** — id set, `rmType`, `aqlPath`, `min`/`max`, input `suffix`/`type` per node — against a **documented-deviations** list absorbing version/ordering/id edge diffs. Cataloged in [`conformance.md`](../../specifications/conformance.md). If the fixture fetch is blocked, PROBE-075 is deferred (goldens-first) with the reason recorded.

## 10. SDD artefacts (produced by `sdd-specify`, then `writing-plans`)

- **REQ-106** registered in [`REQ.md`](../../specifications/REQ.md) (clinical-modeling band): capability + observable acceptance (a spaced-id OPT → valid WebTemplate JSON whose ids/inputs match the reference for the covered subset) + explicit out-of-scope.
- **ADR-0014**: "WebTemplate reference implementation & id-generation lock" — EHRbase openEHR_SDK v2.3; id algorithm; structural-not-byte parity.
- Canonical prose in [`clinical-modeling.md`](../../specifications/clinical-modeling.md): what we mirror, what we emit, the stability guarantee, deviations, out-of-scope.
- [`traceability.yaml`](../../specifications/traceability.yaml): REQ-106 row (package `openehr/template/webtemplate`, PROBE-075, tests, this plan, ADR-0014).
- **Activate & refine** [`2026-05-22-webtemplate-export.md`](../../plans/2026-05-22-webtemplate-export.md) to the direct-slice decision (was a deferred placeholder).

## 11. Alternatives considered (rejected)

- **Shared-model-first** (build the umbrella simplified-template model, then project WebTemplate from it): rejected as premature abstraction — one consumer today; extract when REQ-053 gives a second (YAGNI).
- **Goldens-only** (no external reference): rejected as the primary anchor — weaker parity guarantee; retained only as the documented fallback if the fixture fetch is blocked.
