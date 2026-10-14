---
kind: plan
---

# Plan — Validation engine: one walk, admission first, generated access

**Date:** 2026-10-13
**Status:** In progress — Phases 0 to 2 merged with [PR 263](https://github.com/Cadasto/openehr-sdk-go/pull/263); Phase 3 in [PR 265](https://github.com/Cadasto/openehr-sdk-go/pull/265), closed out and awaiting merge; Phase 4 waits on it
**Covers:** REQ-171 ([clinical-modeling.md § REQ-171](../specifications/clinical-modeling.md#req-171--validation-engine), planned); amends REQ-102 ([§ REQ-102](../specifications/clinical-modeling.md#req-102--composition-validation)), REQ-110 ([§ REQ-110](../specifications/clinical-modeling.md#req-110--template-driven-validation-beyond-composition)), REQ-112 ([§ REQ-112](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor)), REQ-168 ([§ REQ-168](../specifications/clinical-modeling.md#req-168--value-free-validation-diagnostics)) and, in its last phase, REQ-170 ([wire.md § REQ-170](../specifications/wire.md#req-170--unmatched-content-on-decode))
**Probes:** PROBE-025, PROBE-026, PROBE-074 (re-baselined in Phase 4), PROBE-077 (buildable from Phase 3), PROBE-081 (re-homed in Phase 5)
**Depends on:** [PR 256](https://github.com/Cadasto/openehr-sdk-go/pull/256), merged, and [ADR 0025](../adr/0025-template-validation-includes-the-rm-floor.md); [ADR 0028](../adr/0028-generated-rm-attribute-access.md) accepted before Phase 3; [ADR 0027](../adr/0027-template-admission-before-rm-rules.md) accepted before Phase 4; [STRAND-15](../specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types) resolved before Phase 5
**Defers:** the external terminology seam (a no-op default until a service exists); rendering issue paths as Web Template ids or FLAT keys; the interval representation (a bound modelled as value plus included, a change to generated types for a later major); the OPT author validator ([its own plan](2026-07-16-opt-author-validator.md)); the policy options of [STRAND-16](../specifications/research-strands.md#strand-16--validator-policy-beyond-the-limits)

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

One validator value and one walk replace the template walk, the floor walk and the merge that joins them. At each node the walk asks the template whether the content is admitted, then applies the RM rules, then the template's value constraints, then descends. Every finding says which layer and which rule produced it, a result says what it did not check, and both layers read every RM class through access generated from the BMM. The decode gets a cheaper report of unmatched content and a way to tell an absent mandatory member from a zero one. The consumers are every caller of `openehr/validation`, and in particular a server that validates on commit and applies one policy to template findings and another to RM findings.

## What stays and what changes

Each decision PR 256 took was reassessed. These stay, with no work in this plan: template validation includes the RM rules with no switch to turn them off (ADR 0025); one path rule for both layers; type conformance by inheritance and binding reused archetypes by name; the three decode modes for unmatched content (ignore, report, refuse) and the rule that reference spellings the SDK cannot carry fail in every mode; the interval boundary flag defaults.

These change: two walks joined by code and path become one walk with the template's admission first (ADR 0027); `rm_invariant` as one label for several rules becomes one rule name per invariant, taken from the BMM, beside a layer on every finding; readers that cover 71 of the 110 registered classes become generated access that covers all of them (ADR 0028); the report of unmatched members, which costs a second and a third read of the bytes today, comes out of the decode itself (STRAND-15).

## Facts this plan stands on

Verified on `main` at `6ba94eaf` (v0.33.0).

| Fact | Where |
|---|---|
| 110 concrete RM classes are registered for decoding | `openehr/rm/typereg_gen.go` |
| The readers descend into 71 of them; every other class is a leaf | `openehr/validation/rmread/handles_test.go` (the `handledTypes` pin), [§ REQ-112 *Known gap — classes rmread does not model*](../specifications/clinical-modeling.md#req-112--template-less-reference-model-validation-floor) |
| The pinned RM 1.2.0 BMM names 155 invariants in 68 classes; BASE 1.3.0 names 22 in 10 | `resources/bmm/openehr_rm_1.2.0.bmm.json`, `resources/bmm/openehr_base_1.3.0.bmm.json`, the `invariants` maps |
| Hand-written walker code: `walk_composition.go` 1,085 lines, `rmfloor.go` 943, `rmread/read.go` 1,931 | `openehr/validation/` |
| The template walk checks only the existence lower bound; an attribute the OPT prohibits (upper bound 0) is not flagged when present | `openehr/validation/walk_composition.go`, `isRequired` |
| The floor validates a shared Go value once and reports the first path only | `openehr/validation/rmfloor.go`, the `walk` comment on `reached` |
| The two results are joined by dropping a floor issue whose code and path a template issue already carries | `openehr/validation/validate.go`, `withoutRepeats` |
| The report option for canonical JSON reads the bytes again after the decode, a type pass and a name pass, through `internal/rmjsonwalk` | `openehr/serialize/canjson/unmatched.go` |
| `encoding/json/v2` captures every unmatched member in one pass into an embedded fallback field (`embed` on a `jsontext.Value` or `map[string]T`) | Go 1.27 `encoding/json/v2` package documentation |
| Measured cost on the BMI corpus composition: decode 106 µs, template walk 16 µs, floor 66 µs | [ADR 0025 § Consequences](../adr/0025-template-validation-includes-the-rm-floor.md) |

## Design principles

Agreed with the maintainer on 2026-10-13. They are the test each phase is held to.

1. RM validation is always on. Template validation is the layer a caller configures, by passing no template or by applying a policy to template findings.
2. Any archetypable root, any node. Coverage of RM classes comes from generated code, never from a hand-maintained list.
3. Template admission comes before RM rules at every node. Content the template does not admit is reported once and not entered.
4. Collect-all: one run reports every problem.
5. Unknown content is ignored by default and reported on request, never refused by default. The report has to be cheap enough to leave on.
6. Terminology is openEHR's own, inside the RM rules. A seam for an external service exists in the design and gets no work until such a service exists.
7. One finding per defect, naming its layer, its rule and its path.
8. No I/O, no reflection, no globals. The validator is a value that is safe to share once built.

## Phases

Sizes: S under a day, M days, L a week or more. Phases 1 and 2 are independent of each other and of Phase 3; Phase 4 needs all three; Phase 5 needs its strand resolved and can run beside Phases 3 and 4; Phase 6 is a study.

### Phase 0 — Definition layer (this branch)

**Tasks:**
- REQ-171 drafted in `clinical-modeling.md` with its map row (`implementation: planned`); ADR 0027 and ADR 0028 written as proposed; STRAND-15 and STRAND-16 opened; this plan; a roadmap row.

**Definition of done:** `make spec-check` green; the maintainer has accepted or amended the two ADRs and answered, or deferred, the questions the two strands carry.

### Phase 1 — Finding identity and the prohibited attribute (S)

The two small items with no dependency, delivered first so that the single walk can later be compared with the old walks finding by finding.

**Tasks:**
- Add `Layer` and `Rule` to `Issue` as [§ REQ-171 *Finding identity*](../specifications/clinical-modeling.md#req-171--validation-engine) defines them: amend the `Issue` block of § REQ-102 *Contract* and the field-class row of § REQ-168 so both name the two fields; set them at every emit site in `walk_composition.go`, `rmfloor.go`, `rmfloor_coded.go`, `rmfloor_adapters.go` and `rmfloor_bytes.go`.
- Report an attribute the template prohibits (existence upper bound 0) when present, under the code `prohibited` with the sentinel `ErrProhibited`, and amend § REQ-102 *Validation dimensions*, *Issue codes* and *Sentinels* with the rows. No vendored OPT prohibits an attribute, so the test builds its own compiled template.
- Tests: a census over the vendored corpus that every issue carries a layer and a non-empty rule, one table test per rule name over the existing fixture matrix, and an extension of the godoc field-class test. Each guard has a can-fail control: removing the assignment at one emit site fails a named test.
- Map: REQ-171 `implementation: partial`.

**Definition of done:** `make ci` green; CHANGELOG `### Added` names the two fields and `### Changed` the new report; the PR body carries `Lane: full` with REQ-102, REQ-168 and REQ-171.

### Phase 2 — The validator value, limits as options, coverage (M)

**Tasks:**
- `validation.New(opts ...Option)`, `WithLimits`, `(*Validator).Validate(root, c)`, with a nil `c` meaning the RM rules alone; the package-level functions delegate to a validator built with no options and keep their nil guards. The hard-coded depth and visit limits become the defaults of `Limits`.
- `Coverage` on `Result`, with `Stopped` and `External` and the method `Complete`; `External` lists each `CONSTRAINT_REF` leaf the walk binds. `Unevaluated` arrives in Phase 3 with the generated inventory: a hand-kept list of the 215 invariants (155 RM, 22 BASE classes, 38 BASE primitive types) would be thrown away there, and `Complete` does not read it.
- `max_depth` reported once per walk, at the first path where the walk stopped, instead of once per node at the limit.
- Spec: amend § REQ-112 *Surface* to say the typed wrappers delegate to the validator, and the `Result` block of § REQ-102 *Contract* to carry `Coverage`.
- Tests: limits honoured and a zero or negative limit read as the default; each `Coverage` list filled and `Complete` false for its case; a corpus composition with `Complete` true; the floor's nil guards report an empty path.

**Definition of done:** `make ci` green; the benchmark `BenchmarkValidateComposition` shows no regression past the budget line of [STRAND-04](../specifications/research-strands.md#strand-04--rm-polymorphism-and-codec-performance) (more than 20 percent worse at p < 0.05).

### Phase 3 — Generated structural access and the invariant inventory (L)

**Gate:** ADR 0028 accepted.

**Tasks:**
- `internal/bmmgen` emits, into a module-internal package, per-class attribute access for every registered concrete class: name, declared RM type, container flag, presence, value, by plain field access. A test pins that the set of classes covered equals the set the registry decodes, so `make codegen-verify` fails on a BMM bump until the generator runs.
- Both walks read through the generated access; `openehr/validation/rmread` is removed, with its `Handles` pin and its reader tests. The REQ-013 import guard and the block list in `module-layout.md` stop naming it; the architecture narrative records the generator change, as ADR 0013 did.
- `internal/bmmgen` emits the invariant inventory from the `invariants` maps of the pinned RM and BASE BMMs (BASE's interval invariants sit under `primitive_types`), keyed by class and name, with a checked-in classification file (evaluated with its Go function, not mechanical with its reason, out of scope with its reason). `make codegen-verify` fails when an invariant has no classification. The floor dispatches its rules through the inventory, and `Coverage.Unevaluated` is added to `Result`, filled from it.
- Spec: replace the *Known gap — classes rmread does not model* paragraph of § REQ-112 with a sentence pointing at the generated coverage table, and fold the other known-gap paragraphs into inventory entries (`Limits_comparable`, a bound beside its own open flag, the change-control and resource-description coded invariants, `Other_reference_ranges_validity`, `Range_is_simple`, `Scheme_valid`, the DV_AMOUNT accuracy rules, `Magnitude_status_valid`, `Participations_validity`, `Other_participations_valid`). Amend § REQ-110 *Implementation* and § REQ-112 *Driver* where they describe the readers.
- PROBE-077, the RM-floor invariant matrix, becomes buildable from the inventory: implement it over the fixture matrix, or re-scope its catalogue entry to say what the inventory now proves.
- Expect new findings: the walk now enters classes and optional attributes it never read (INSTRUCTION_DETAILS, LINK, FEEDER_AUDIT, DV_STATE, DV_PARAGRAPH, the time-specification types; LOCATABLE `uid`, `links` and `feeder_audit`; ENTRY `workflow_id`; DV_TEXT `hyperlink`, `language`, `encoding`). Every new corpus-ratchet row is reviewed, and the generator (REQ-107) is fixed where it is the cause.

**Definition of done:** `make codegen-verify`, the corpus ratchet and `make ci` green; CHANGELOG `### Removed` names the reader package and `### Changed` the classes now checked; the architecture narrative updated.

### Phase 4 — The single walk (L)

**Gate:** ADR 0027 accepted; Phases 1, 2 and 3 landed.

**Tasks:**
- Build the engine on the generated access, with the union rule and the step order of § REQ-171 *One walk*: admission, presence, counts, then per admitted item identity and type, the RM rules of its class, the template's value constraint at a leaf, and descent. Cycle detection on the active ancestry alone; a shared value validated at each place.
- Keep the two-walk build running beside the engine until the comparison is done. A comparison harness runs both over the vendored corpus, the constraint-fixture matrix and every probe input, and diffs the results finding by finding on path, code, layer and rule. Each difference is one of the three kinds § REQ-171 *Acceptance* allows: a finding from inside refused content, a second code for one defect at one path, or a shared value's second path. Any other difference is a defect in one of the two.
- Where the template reaches, precompute at compile time what is resolved once per template rather than once per instance: the accessor for each declared attribute and the conformance answer for each alternative. Measure before and after.
- Remove the floor as a separate driver, `withoutRepeats`, the `reached` record and the pinned-name bookkeeping the merge needed; the path rule stays.
- Spec: rewrite § REQ-102 *RM floor* to cite § REQ-171 and state the admitted-node scope; amend § REQ-112 *Driver* (one walk, a shared value validated at each place); the ADR 0025 header already names ADR 0027 as its amendment.
- Re-baseline PROBE-025, PROBE-026 and PROBE-074 as a reviewed change, each changed multiset explained in the PR body by the kinds § REQ-171 *Acceptance* allows (refused content, a second code for one defect, a shared value's second path, a class or attribute the readers did not read).
- Benchmark acceptance: the engine over the largest corpus composition and over a synthetic one an order of magnitude larger, with allocation counts, costs no more than today's template walk plus the floor. The figures in ADR 0025 come from two small fixtures and say nothing about large documents or allocations, so the synthetic case is part of the acceptance, not an extra.

**Definition of done:** the comparison harness reports only allowed differences; `make ci` green; CHANGELOG `### Changed` marks the smaller result as a break; map: REQ-171 `implementation: landed`, REQ-112 updated.

### Phase 5 — Decode-time presence and unmatched capture (M)

**Gate:** STRAND-15 resolved by an ADR; the capture contract allocated its own requirement through `/sdd-specify`, which also decides REQ-170's amendment (keep or discard).

**Tasks:**
- The generator emits the embedded fallback field on each generated class, and an absence record for by-value mandatory attributes, set only by a decoder. Canonical JSON's report and strict options read the fallback instead of walking the bytes; the type pass and the name pass go, and with them the attribute tables linked into every importer of `canjson`. `internal/rmjsonwalk` stays if `testkit/expectedform` still needs it, or goes.
- The floor reads the absence record, so `ValidateRM` reports an omitted by-value mandatory member on a decoded value; `ValidateRMEHRStatusBytes` retires, § REQ-112 *Value-typed mandatory presence* is rewritten, and PROBE-081 is re-homed on the value entry.
- A write-path helper that decodes with the report and validates in one call and returns both, so a dropped FLAT key, which a typo makes indistinguishable from an extension, is seen beside the validation result on a commit path.
- Canonical XML and FLAT get the same absence record where their decoders can set it; where they cannot, the helper's report is the only signal, and the godoc says so.

**Definition of done:** `make ci` green; the `canjson` decode benchmarks within the STRAND-04 budget line with the fallback field present; CHANGELOG `### Removed` names the Bytes entry and `### Added` the helper.

### Phase 6 — Study: generated token-level JSON decoders (L, no code)

The SDK already owns the XML decode. Owning the JSON decode as well would validate while reading, with presence and unmatched members for free and no fights with the library's hooks. It is the largest step and is decided only after a study: an analysis note under this folder, like the [STRAND-11 evidence](strand-11-evidence/README.md), that sizes the generated code, measures a prototype on the `canjson` benchmarks against the Phase 5 result, and recommends or declines. No requirement and no code until the note exists.

## Consumer-visible changes

| Phase | Change | Kind |
|---|---|---|
| 1 | `Issue` gains `Layer` and `Rule`; a prohibited attribute present is reported | Added; tightened validation |
| 2 | `Validator`, `New`, `WithLimits`; `Result` gains `Coverage`; `max_depth` once per walk | Added; a smaller result at the depth limit |
| 3 | `openehr/validation/rmread` removed; classes and attributes never checked before are checked | Breaking; tightened validation |
| 4 | No findings from inside refused content; one finding where two codes named one defect; a shared value reported at each place | Breaking: a smaller result |
| 5 | `ValidateRMEHRStatusBytes` removed; omitted by-value mandatory members reported on any decoded root; the write-path helper | Breaking; Added |

Each lands as a `v0.x` minor named in the release notes, as ADR 0025's own tightening did.

## Guardrails

- Spec first in every phase: the amendments each phase names land in the same PR as the code, and `make spec-gen` writes the registry and the tests lists.
- No phase lands code before its gate: the ADR accepted, the strand resolved. An unmet gate stops the dispatch and is named in the PR.
- The instruments that control behavioural drift already exist and are used, not rebuilt: the conformance probes that pin issue-code multisets, the corpus ratchet (`TestREQ107_CorpusRatchet`), the FLAT corpus (PROBE-086), and the `canjson` and validation benchmarks.
- Workers do not run `go test -race` on a host without a C compiler; CI runs it on `main`.
- Godoc on exported identifiers carries no requirement, decision or plan identifiers.

## Backlog lines this plan absorbs

Each is a lead in [docs/backlog.md](../backlog.md) under *Validation engine Phase 4*, verified when Phase 4 rewrites its path and deleted when settled:

- `openehr/validation/rmfloor.go:359`, an empty mandatory container reported under two codes at one path; `rmfloor.go:250`, the cost of the visit-once record; `rmfloor.go:532`, a temporal value reported twice; `walk_composition.go:156`, an attribute's children copied on every visit.

The leads Phases 2 and 3 absorbed are settled. The backlog triage of 2026-10-14 dropped the rest: the ADR 0025 lines, settled or wording only, and the Phase 5 leads, the member-check route (settled or trivial) and the `rminfo` link-size follow-up, a cost the maintainer had accepted.

## Open questions

Recorded, not settled here: [STRAND-15](../specifications/research-strands.md#strand-15--decode-time-presence-and-unmatched-member-capture-on-the-generated-types) (captured members kept or discarded; the direction of the presence record) and [STRAND-16](../specifications/research-strands.md#strand-16--validator-policy-beyond-the-limits) (descent under refused content, a severity policy per rule, a partial-instance mode). Phase 5 waits on the first; nothing waits on the second.
