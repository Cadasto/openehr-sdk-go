# Plan — Generic OPT-driven Composition builder

**Date:** 2026-05-21 (research-updated 2026-05-22)
**Status:** Landed (PRs #19 + #20)
**Owner:** SDK maintainers
**Covers:** REQ-013, REQ-024, REQ-030–033; **REQ-101** (generic composition builder)
**Probes:** PROBE-023 (Implemented Sandbox — build + canjson marshal-fragment parity v1; full unmarshal round-trip lands with [`2026-05-26-c-primitive-object-wire-parser.md`](2026-05-26-c-primitive-object-wire-parser.md) Phase 2)
**Implementation:** landed (Phases 0–2: spec + `NewSkeleton` + `Builder.Set/Build`; per-template typed builders deferred to OET-authoring plan)
**Depends on:** [`2026-05-21-template-parser.md`](archive/2026-05-21-template-parser.md) (REQ-100, landed); [`2026-05-22-template-req100-followups.md`](2026-05-22-template-req100-followups.md) Phases 4 + 4-bis + 5 (compiled template + RMInfoLookup + walker); [`2026-05-24-template-instance-example-generator.md`](2026-05-24-template-instance-example-generator.md) (proposed REQ-107 — skeleton engine); [`2026-05-15-canonical-json-serialization.md`](archive/2026-05-15-canonical-json-serialization.md); umbrella [`2026-05-21-phase-2-clinical-building-blocks.md`](2026-05-21-phase-2-clinical-building-blocks.md)
**Defers:** Per-template generated Go structs; FLAT/STRUCTURED ingest (REQ-053); automatic `EVENT` timing population beyond documented defaults; OET-driven authoring builder

## Goal

Provide **`openehr/composition`**: produce a `*rm.Composition` graph in memory **driven by a parsed OPT**. Two related entry points:

1. **Skeleton builder** — given an OPT alone, instantiate a default `*rm.Composition` with all required RM attributes filled. Output is a valid RM graph the consumer can serialise via `canjson` and `POST` to a CDR — no business data, but minimum-conformant.
2. **Path-assigning builder** — given an OPT, accept `Set(path, value)` calls and produce a `*rm.Composition` with the assigned values plus required-attribute defaults from the skeleton step.

The engine is generic — consumers do **not** import codegen'd vital-signs-specific structs. The OPT is the schema; the builder is the constructor.

Persistence remains **`openehr/client/ehr/composition/`** + `canjson` at the application boundary.

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| Compiled OPT | `openehr/template/` (`Compiled`) | Path → CompiledNode resolution; RM type per path; cardinality; default values |
| RMInfoLookup | `openehr/rm/rminfo/` | Implicit RM attribute injection; container-vs-single discrimination |
| Walker | `openehr/template/walk/` | Tree-walk primitive for skeleton synthesis and path assignment |
| RM types + typereg | `openehr/rm/` | Target Composition graph; constructor lookup by RM class name |
| Canonical JSON | `openehr/serialize/canjson/` | **Tests and apps only** — builder does not import `serialize/` |
| EHR client | `openehr/client/ehr/composition/` | `Create` / `Update` with `composition.WithTemplateID` |

## Design rationale (research baseline)

Reference Java implementations (notably [ehrbase openEHR_SDK](https://github.com/ehrbase/openEHR_SDK)'s `WebTemplateSkeletonBuilder` and the `ToCompositionWalker` hierarchy) converge on these design principles:

1. **Template-driven recursion, not data-driven.** The walker descends through the **template**, asking at each step "does the input have a value at this path?". Required RM attributes the template omits are filled in by an RMInfoLookup. Inverting this — descending input first — produces non-conformant graphs because the input doesn't know what the RM mandates.
2. **One walker, multiple modes.** Skeleton (defaults only) and assembly (defaults + user data) are the same walker with different "extract from input" callbacks. Validators reuse the same walker shape with a different accumulator (see REQ-102).
3. **Defaults from three sources, in priority order:**
   1. OPT-declared `assumed_value` / `default_value` on the constraint (template-explicit).
   2. RM-side defaults from RMInfoLookup (e.g. `DvInterval.lowerIncluded = true`, `event-time` initialised to `now()`).
   3. Hard-coded language/territory/composer from builder configuration (last resort).
4. **Implicit attribute injection happens at compile time** (Phase 4 of the follow-up plan), not at build time. By the time the builder walks the compiled tree, every required RM attribute is a `CompiledAttribute` even if the OPT didn't mention it. The builder doesn't need its own "what's required?" logic.

## Design principles (SDK-specific)

1. **Path-first API** — `Set(path, value)` / `SetQuantity(path, magnitude, unit)` style; generics where the RM target type is known from template metadata.
2. **Lazy graph materialisation** — create `SECTION` / `OBSERVATION` nodes on first `Set` along a path; archetype roots from OPT definition.
3. **Polymorphism via typereg** — concrete RM types per path hint; no `reflect` for clinical assignment (REQ-024).
4. **Template id propagation** — `Builder.TemplateID()` matches OPT; caller passes same id to REST `WithTemplateID` so the CDR validates against the same template.
5. **UIDs** — generate missing `uid` on nodes where openEHR expects them (document algorithm in REQ-101); do not require caller-supplied UUIDs for every node in v1.
6. **No silent type coercion** — `Set("/data/events/value", "42")` on a `DV_QUANTITY` path returns `ErrTypeMismatch`. Numeric strings must be passed via `SetQuantity` or a typed primitive helper.

## v1 scope (honest bounds)

| Feature | v1 | Later |
|---|---|---|
| Skeleton build (defaults only) | yes | |
| `Set(path, value)` for primitive paths (`DV_TEXT`, `DV_QUANTITY`, `DV_CODED_TEXT`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DATE_TIME`) | yes | |
| Single-root composition | yes | |
| Multi-archetype `/content` lists | yes | |
| `EVENT` defaults (POINT_EVENT with `time = now()`) | yes | INTERVAL_EVENT + custom event timing |
| `CLUSTER` / `ITEM_TREE` nested item assignment | yes | |
| Slot-filled archetypes | only if slot-fill is already in the OPT definition tree (pre-flattened) | runtime slot resolution via REQ-101.5 |
| `ACTION.ism_transition` defaults | configured default | OPT-driven state-machine respect |
| `DvInterval` lower/upper assignment | yes (with documented default `*_included = true`) | full interval-with-bound-flags ergonomics |
| Default values from `<default_value>` blocks | yes (via Phase 4 compile step) | |
| FLAT / STRUCTURED ingest | no | REQ-053 |
| Per-template generated typed builder | no | OET-driven authoring (separate plan) |

## Out of scope

- Validating against OPT constraints (→ `openehr/validation/`).
- Encoding to JSON/XML inside the package (apps import `canjson` separately).
- Multi-language composition merges / contributions (Contribution builder is a later extension).
- Implicit terminology resolution (composer.name from a directory, territory from app config) — caller's job.

## Phases

### Phase 0 — REQ-101, API contract, fixture composition

**Outcome:** Normative builder rules; failing tests describing desired behaviour.

**Tasks:**

1. **`docs/specifications/clinical-modeling.md` § REQ-101** — builder invariants:
   - Requires `*template.Compiled` at construction.
   - `Build() (*rm.Composition, error)` returns graph or aggregated path errors.
   - Path must exist on template; wrong RM type → typed error.
   - `language` / `territory` / `category` — document required defaults for v1 (e.g. `ISO_639-1::en`, `ISO_3166-1::NL`, openEHR event category `433`).
   - Required-attribute injection from RMInfoLookup is **non-optional** — a builder that omits `Composition.category` is non-conformant.
2. **REQ.md + traceability** — planned → partial as code lands.
3. **Golden target** — extend template fixture OPT; document expected paths + skeleton output JSON in `openehr/composition/testdata/README.md`.
4. **Test skeleton** — `builder_test.go` with `// REQ-101` cases (skip until Phase 1 if needed).

**Definition of done:** Spec + registry; tests compile; template Phase 1 fixture paths documented.

### Phase 1 — Skeleton builder

**Outcome:** Given an OPT alone, instantiate a structurally-conformant `*rm.Composition`.

**Tasks:**

1. **`NewSkeleton(c *template.Compiled, opts ...Option) (*rm.Composition, error)`** — walks the compiled tree via `template/walk`, instantiates an RM object at each `CompiledNode`:
   - Uses `typereg` to construct the RM type by name.
   - Sets `archetype_node_id` from the compiled node's `NodeID()`.
   - Sets `archetype_details.archetype_id` and `archetype_details.template_id` for archetype-root nodes.
   - Fills required attributes via RMInfoLookup; emits defaults from compiled-node `DefaultValue()` if present.
   - For multi-value attributes, initialises as empty slice (the path-assigning builder can then append).
2. **RM-specific defaults** — hard-coded set documented in REQ-101:
   - `Composition.category = openEHR::433|event|` unless overridden.
   - `Composition.language` from OPT's `Language()` or `Option.Language`.
   - `Composition.territory` from `Option.Territory` (required option — no global default).
   - `Composition.composer` from `Option.Composer` (required option).
   - `Composition.context.start_time = time.Now().UTC()` unless overridden.
   - `EVENT.time = time.Now().UTC()` for POINT_EVENT defaults.
   - `DvInterval.lower_included = true`, `upper_included = true`.
   - Object name: from the per-node terminology lookup (`Compiled.Term(nodeID, lang)`) or fall back to RM type name.
3. **Tests** — skeleton against `vital_signs.opt` produces a `*rm.Composition` with non-empty `category`, `language`, `territory`, `composer`, `context.start_time`, and `content` slice (possibly empty).
4. **`cmd/examples/composition-skeleton/main.go`** — load fixture OPT → build skeleton → emit canonical JSON via `canjson` (the example imports both packages; the `composition/` library does not import `serialize/`).

**Definition of done:**

- `go test ./openehr/composition/...` green.
- No `serialize/` import in `openehr/composition/*.go` (enforced by `TestNoSerializeImport` using `go list -deps`).
- `NewSkeleton(vital_signs.opt)` → marshal via `canjson` → posts cleanly to an httptest backend (smoke test in `composition_skeleton_test.go`).

### Phase 2 — Path-assigning builder

**Outcome:** `Builder.Set(path, value)` populates user data on top of the skeleton.

**Tasks:**

1. **`NewBuilder(c *template.Compiled, opts ...Option) *Builder`** — initialises with the skeleton from Phase 1 as the in-memory graph.
2. **`Set(path string, v any) error`** — dispatch on `template.Compiled.NodeAt(path).RMTypeName()`:
   - Primitive DV types (`DV_TEXT`, `DV_CODED_TEXT`, `DV_QUANTITY`, `DV_COUNT`, `DV_BOOLEAN`, `DV_DATE_TIME` per REQ-046 ISO 8601 strings).
   - Nested `CLUSTER` / `ITEM_TREE` via path prefixes — auto-create container nodes on first `Set` in a subtree.
   - Multi-value paths (`/content[archetype-id]`) — `Set` of a value at an indexed predicate (`[1]`, `[2]`) appends to the slice.
3. **Typed helpers** (`SetText`, `SetQuantity`, `SetCodedText`) — only where they remove non-trivial boilerplate. Do not duplicate every DV type with a setter.
4. **`Build() (*rm.Composition, error)`** — finalises the graph: trims empty multi-value attributes back to nil where allowed by cardinality, generates missing `uid` fields, returns aggregated path errors if any `Set` failed.
5. **PROBE-023** — build fixture composition → `canjson.Marshal` → unmarshal → key paths stable (sandbox; lives in `testkit/probes/composition/`).
6. **REQ-101** → `landed` in traceability when probe + assembly tests stable.

**Definition of done:**

- Benchmark seeder (STRAND-01) can adopt builder for at least one template (follow-up PR outside this plan).
- PROBE-023 implemented (Draft → Implemented in conformance table).
- Builder + skeleton + walker together fit under the v1 scope table above.

## Public API (target)

```go
package composition

// Builder configuration.
type Option interface { /* WithTerritory, WithComposer, WithLanguage, WithCategory, ... */ }

// Skeleton creates a structurally-conformant default Composition from an OPT.
// All required RM attributes are populated from RMInfoLookup or builder options.
// Body has no clinical data — feed it into a Builder for path-by-path assignment.
func NewSkeleton(c *template.Compiled, opts ...Option) (*rm.Composition, error)

// NewBuilder constructs a path-assigning builder over a skeleton.
func NewBuilder(c *template.Compiled, opts ...Option) *Builder

// Set assigns v at path. v must match the compiled-node RM type.
// Type-checked at call time; aggregated errors surfaced from Build().
func (b *Builder) Set(path string, v any) error

// Typed helpers for the most common targets.
func (b *Builder) SetText(path, value string) error
func (b *Builder) SetQuantity(path string, magnitude float64, units string) error
func (b *Builder) SetCodedText(path, terminology, code, display string) error

// Build returns the finalised Composition or an aggregated error.
func (b *Builder) Build() (*rm.Composition, error)

// TemplateID returns the OPT template id (for REST WithTemplateID).
func (b *Builder) TemplateID() string
```

## Implementation checklist

| Step | Status |
|---|---|
| Depends-on: REQ-100 follow-up plan Phases 4 + 4-bis + 5 | |
| Phase 0 REQ-101 spec + registry + tests skeleton | |
| Phase 1 Skeleton builder + RM-default catalogue | |
| Phase 2 Path-assigning Builder + Set + Build | |
| Typed helpers (SetText / SetQuantity / SetCodedText) | |
| PROBE-023 sandbox probe | |
| `composition-skeleton` example | |
| `TestNoSerializeImport` import-guard test | |
| `make ci` green | |

## Mapping to specs

- [`docs/specifications/module-layout.md`](../../docs/specifications/module-layout.md) — composition vs client split, no `serialize/` import
- [`docs/specifications/rm-modeling.md`](../../docs/specifications/rm-modeling.md) — concrete types, typereg
- [`docs/specifications/clinical-modeling.md`](../../docs/specifications/clinical-modeling.md) § REQ-100 — OPT parse + paths (landed)
- Proposed: `docs/specifications/clinical-modeling.md` § REQ-101 — composition builder
- [`docs/specifications/conformance.md`](../../docs/specifications/conformance.md) — PROBE-023 (proposed)

## References (research baseline, informational)

The composition-builder design draws design lessons from reference Java implementations. The SDK retains its own AOM 1.4 / ADL 1.4 typed surface; these references inform sequencing and corner-case handling.

- **ehrbase openEHR_SDK** — [`github.com/ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK). `web-template/.../WebTemplateSkeletonBuilder.java` is the closest analogue to Phase 1 (RM-type-by-RM-type defaults catalogue with explicit special-cases for `Entry.subject`, `Composition.category`, `DvInterval.*_included`, `Encoding.code_string = "UTF-8"`). The `ToCompositionWalker` hierarchy (`StdToCompositionWalker`, `DtoToCompositionWalker`) is the analogue to Phase 2 (template-driven recursion with per-call value extraction).
- **openEHR/archie** — [`github.com/openEHR/archie`](https://github.com/openEHR/archie). `tools/.../creation/RMObjectCreator.java` is the AOM-2-flavoured RM-instance constructor with per-RM defaulting rules.
- **openEHR Reference Model** — [`specifications.openehr.org/releases/RM/latest`](https://specifications.openehr.org/releases/RM/latest). Source of truth for required attributes per RM class and value-set defaults.
