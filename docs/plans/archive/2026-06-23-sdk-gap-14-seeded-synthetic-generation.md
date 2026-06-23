# Plan — SDK-GAP-14: seeded synthetic value generation for instance / NewSkeleton

**Date:** 2026-06-23
**Status:** Landed — value-fill + seed (PR #55, 2026-06-24; v0.11.0). `medium` detail_level deferred to a follow-up plan.
**Owner:** SDK maintainers
**Covers:** [REQ-103](../../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection), [REQ-107](../../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator), [REQ-101](../../specifications/clinical-modeling.md#req-101--generic-opt-driven-composition-builder)
**Implementation:** landed (value-fill + seed); `medium` detail_level deferred
**Relates:** SDK-GAP-12 (NewSkeleton real-world OPT coverage — landed; [2026-06-19-sdk-gap-12-newskeleton.md](2026-06-19-sdk-gap-12-newskeleton.md)); SDK-GAP-13 (polymorphic encode/decode round-trip — landed; [2026-06-23-sdk-gap-13-polymorphic-encode-decode.md](2026-06-23-sdk-gap-13-polymorphic-encode-decode.md); [STRAND-04](../../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance))
**Source (inbound):** a consuming CDR project — its template `/example` endpoint and a write benchmark need a *diverse* corpus of template-valid COMPOSITIONs (so persisted data is realistic enough to exercise AQL range / code-equality / aggregation retrieval), not byte-identical leaves.
**Severity:** medium — no production write-path impact. Today every generated COMPOSITION for a given OPT carries byte-identical clinical leaves (only consumer-stamped uid/time/composer vary), so a CDR cannot self-seed a realistic corpus and AQL value-predicate testing has no signal.

## Definition of Ready (analysis gate)

- [x] Maintainer sign-off (2026-06-23): orthogonal `ValueFill` axis (not a `Synthetic` policy); `ValueSource` seed seam; **generator-internal** sampler (no `PrimitiveConstraint` interface change); `medium` / `detail_level` **deferred**.
- [x] `Covers:` finalized — REQ-103/107 surface is sufficient for value-fill + seed; no new normative criteria promoted.

## Accepted approach (2026-06-23)

Lands on the shared branch `fix/sdk-gap-13-14`. Scope this pass: **value-fill + seed only**; the `medium` structural level is deferred (it is the bulk of the new structural work and independently useful — see [Deferred](#deferred-follow-up)).

- **Orthogonal `ValueFill` axis** on [`instance.Options`](../../openehr/instance/options.go): `ValueFill ∈ {ExampleFill (default — today's `ExampleValue()` behaviour), RandomFill}`, composable with any `Policy`. No `Synthetic` policy (would conflate structure and values).
- **`ValueSource rand.Source` seam**, mirroring the existing `UIDSource`. A fixed source ⇒ byte-reproducible output; nil ⇒ a fresh (time-seeded) source so successive calls differ. Threaded through the `generator` exactly like `UIDSource`.
- **Generator-internal sampler** (`openehr/instance/sample.go`): draws a value from within each primitive constraint by reading the fields it already carries (`CInteger.Range/List`, `CReal.Range/List`, `CodePhrase.CodeList`, `CDvOrdinal`, `DvQuantity`, `CString`, `CDate*`/temporal), self-verified against the existing `PrimitiveConstraint.Validate(value any)` so output is *valid by construction* (falls back to `ExampleValue()` for the unbounded case or any draw that fails `Validate`). The REQ-103 `PrimitiveConstraint` interface is **unchanged** — no `RandomValue` method is added.
- `applyPrimitiveExample` routes through the sampler when `ValueFill == RandomFill`, else keeps `ExampleValue()`.
- Surfaced through [`composition.NewSkeleton`](../../openehr/composition/skeleton.go) as `Option`s (`WithValueFill` / `WithValueSource`, names TBD in impl) so COMPOSITION roots get the mode without dropping to `instance.Generate`.

### Deferred (follow-up)

`detail_level` alignment (`required ≈ Minimal`, `complete ≈ Example`, new `medium` representative-optional-subset). `medium` needs a crisp, testable rule for which optional nodes/occurrences it includes; tracked for a later plan so the value-fill + seed ask (the corpus-diversity blocker) lands now.

## Goal

Give `openehr/instance` (and therefore `composition.NewSkeleton`) a generation mode that fills each primitive leaf with a value **drawn from within its constraint** (valid by construction) and that **can vary** between calls, with a **seedable** source for reproducibility — plus alignment with the ITS-REST `/example` `detail_level` structural levels.

## Background — what the contract does and doesn't mandate

The openEHR ITS-REST `GET /definition/template/adl1.4/{template_id}/example` endpoint (development-track; the SDK client landed in the conformance remediation) takes two query parameters — `type` (`input|output`, default `input`) and `detail_level` (`required|medium|complete`, default `required`) — see [resources/its-rest/definition-validation.openapi.yaml](../../resources/its-rest/definition-validation.openapi.yaml) (`example_type` / `example_detail_level`). Two load-bearing facts:

1. **`detail_level` is a *structural* axis, not a value-variation axis.** It controls *which* nodes/occurrences and how deep, not whether leaf *values* differ. `required` ≈ the SDK's current `Minimal` (mandatory nodes); `complete` ≈ the SDK's `Example` (all optional nodes); **`medium` (a representative optional subset) has no SDK equivalent today**.
2. **The spec disclaims value validity and variation.** It states the example's completeness "are not specified", output "should not be used in production", and "vendors may produce different results." Nothing mandates that a generated `DV_QUANTITY` sit in range, a `DV_CODED_TEXT` draw from its value set, or that values vary between calls. So **constraint-valid, varied value generation is a quality choice the SDK is free to own and document as policy** — it is not an ITS-REST mandate.

## Current state (verified on `main`, v0.10.0)

- `instance.Options` exposes `Policy` ∈ {`Minimal`, `Example`} and a `UIDSource func() *rm.HierObjectID` seam ([openehr/instance/options.go:10-68](../../openehr/instance/options.go)). `Minimal` materialises mandatory nodes; `Example` adds every optional leaf.
- **Both policies fill primitive leaves deterministically** from `constraints.PrimitiveConstraint.ExampleValue()` ([generate.go:119-132](../../openehr/instance/generate.go#L119), [generate.go:720-729](../../openehr/instance/generate.go#L720)) — a single representative value. Repeat generation for one OPT is byte-identical in its data leaves.
- **No `medium` structural level, no random/seeded value fill, no `Synthetic` policy** exist. (`UIDSource` already varies the uid; only that and consumer post-stamping break the byte-identity.)

**Feasibility — the constraint data needed for an in-constraint sampler is already on the compiled OPT.** The `constraints` types carry ranges / lists / value-sets and a self-check `Validate(value any)`: `CInteger{Range NumericRange; List []int64}`, `CodePhrase{CodeList []string}`, and the sibling `CReal` / `CString` / `CDvOrdinal` / `DvQuantity` / `CDate*` types ([openehr/template/constraints/](../../openehr/template/constraints/)). So a "random point in constraint" fill is reachable today without new OPT plumbing.

## The asks (from the inbound report)

1. **Value-fill mode** — every materialised leaf filled with a value valid against its `C_*` constraint and RM data-type invariants (in-range magnitudes, valid units, value-set-member codes, regex/enumeration-satisfying strings), which can vary between invocations.
2. **Seedable randomness** — a seedable source on `instance.Options` (mirroring `UIDSource`); fixed seed ⇒ byte-reproducible output, no seed ⇒ successive calls differ in leaf values.
3. **`detail_level` alignment (recommended)** — expose the three ITS-REST structural levels under the spec's exact tokens (`required` ≈ `Minimal`, `medium` = new representative-optional-subset, `complete` ≈ `Example`) so a CDR `/example` handler can map the query parameter directly. `medium` is the only genuinely new structural level.
4. **Surface through `composition.NewSkeleton`** as an option, so COMPOSITION roots get the mode without dropping to the lower-level `instance.Generate`.

## Candidate design (decision required before implementing)

- **Orthogonal `ValueFill` over a new policy (recommended).** Keep `Policy` (structural: which nodes) and a new **value-fill** axis (how leaves are valued) orthogonal: `ValueFill ∈ {ExampleValue (today's default), Random}` on `instance.Options`, composable with any policy. Cleaner than a `Synthetic` policy that would conflate structure and values.
- **Seed seam mirrors `UIDSource`.** Add `ValueSource rand.Source` (or `Seed int64`) to `instance.Options`; thread it like `UIDSource`. Deterministic golden tests pass a fixed source; a CDR `/example` passes a fresh one per request.
- **Per-constraint sampler.** Add a `RandomValue(rng)` alongside `ExampleValue()` on `PrimitiveConstraint` (each type already knows its `Range`/`List`/`CodeList`), or a generator-side sampler that reads those fields; self-verify each emitted value via the existing `Validate(value any)` so "valid by construction" is enforced, not assumed.
- **`detail_level` levels.** Alias the structural levels to the spec tokens and add `medium` (a representative optional subset — e.g. include optional nodes up to a bounded depth / first-occurrence). `medium` is the bulk of the new structural work.

## Open decisions (for the maintainer / brainstorm before any code)

1. Value-fill as an orthogonal `ValueFill` option vs a new `Synthetic` policy.
2. `Seed int64` vs `ValueSource rand.Source` (consistency with `UIDSource` argues for a source).
3. Whether to add `RandomValue` to the `PrimitiveConstraint` interface (touches REQ-103 surface) or keep sampling generator-internal.
4. Exact semantics of `medium` (which optional nodes/occurrences it includes) — the only genuinely new structural behaviour; needs a crisp, testable rule.
5. Whether this lands as one plan or splits "value-fill + seed" (REQ-107/103) from "`detail_level`/`medium`" (REQ-107) — they're independently useful.

## Acceptance (from the consuming CDR project)

For every corpus OPT, calling the synthetic generator N times yields N COMPOSITIONs that (a) each pass `validation.ValidateComposition`, and (b) **differ in their primitive leaf values** (not only uid/time/composer). With a fixed seed, output is byte-reproducible.

## Out of scope

- **Clinically-plausible value *distributions*** (coherent vitals curves, real problem lists — Synthea-class). This gap is constraint-conformance + variation only.
- SDK-GAP-13 (polymorphic encode/decode round-trip) — independent; a synthetic instance is still subject to it until that lands.
- ITS-REST `type=input|output` semantics and FLAT/STRUCTURED `Accept` web-template formats — serialization concerns, not generation.
