---
kind: plan
---

# Plan — Generator and builder: RM-valid output for every vendored template

**Date:** 2026-09-30
**Status:** Draft — from the audit of the PR 181–189 review leftovers
**Covers:** REQ-107 ([clinical-modeling.md § REQ-107](../specifications/clinical-modeling.md#req-107--template-driven-rm-instance-example-generator)), REQ-101 ([§ REQ-101](../specifications/clinical-modeling.md#req-101--generic-opt-driven-composition-builder))
**Probes:** PROBE-027 ([conformance.md](../specifications/conformance.md#probe-027--generated-instance-validates-clean)), widened
**Depends on:** nothing; the [validation plan](2026-09-30-validation-false-passes.md)'s ordinal-symbol phase needs this plan's Phase 2 first
**Defers:** unsatisfiable-constraint errors, honouring a C_BOOLEAN on `*_unbounded`, generic non-interval RM type names (all latent; recorded in the findings file)

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

`instance.Generate` and `composition.NewBuilder` return an RM value for every vendored template that compiles, and that value passes both validators. Today § REQ-107 calls the generator *sound*, but its output breaks RM invariants on every template with an ENTRY, and it refuses about a fifth of the vendored templates. Both defects have existed since v0.28.0, so they are not regressions, and the one probe that cross-checks the generator (PROBE-027) runs only two templates through `ValidateComposition`.

## Evidence (main `bfa10c10`; 62 vendored OPTs, 60 compile)

| Defect | Scope | Where |
|---|---|---|
| Refusal `makeChild STRING: rmwrite: unknown RM type` | 13 of 62 vendored OPTs, including `IDCR - Laboratory Test Report.v0` and `GECCO_Diagnose`: any `C_STRING` list or pattern on a text value | `IsAOMPrimitiveShortName` has no `STRING` (`internal/templatecompile/node.go:18-20`) |
| Refusal `rmwrite: unknown attribute on parent` | `conformance_ehrbase.de.v0` (ISM_TRANSITION.current_state), `TestPerson.v2` (PERSON.details), `Address.v2` (ADDRESS.name) | no `rmwrite` dispatch arm for those parents |
| `EVENT.time` and `HISTORY.origin` are the string `"example"`, not ISO 8601, even with `WithNow` | every OBSERVATION | `generate.go:285` and `:336` write `"example"` into every `String` attribute through `rmwrite.EnsureSingle`, a setter, overwriting the value `:387` set from `Options.Now` |
| ENTRY `language` and `encoding` are `local::example`, even with `WithLanguage("en")` | every ENTRY | the same pass; `Options.Language` is documented to drive them (`openehr/instance/options.go:73`) |
| DV_ORDINAL `symbol` is empty | every ordinal | no symbol is built from the template's (value, code) pair |
| RandomFill inverts temporal intervals and mixes units (`cm` against `[in_i]`) | random fill | bounds drawn independently per side |
| Empty CLUSTER `items`; ELEMENT without `name` or `archetype_node_id`; ACTION without `time` or `ism_transition` | `vital_signs`, `Demonstration.v1`, `minimal_action_2` | seen only through `ValidateRM`, which PROBE-027 does not run |

`ValidateComposition` reports none of this. The template-less floor's `required` findings on generated output rose from 32 to 88 after PRs 187 and 188, mostly the ordinal symbol.

## Phases

### Phase 0 — a corpus-wide harness, landed green with a named ratchet

**Tasks:**
- One test that runs `Generate` (both policies, both fills, fixed seeds) and `composition.NewBuilder` over every vendored OPT that compiles (`fixtures.ListAllOPTs`), then `ValidateComposition` and `ValidateRM` on the output.
- Known failures go in a named table with the reason, so the test lands green and each later phase deletes rows. An unlisted failure, or a listed template that now passes, fails the test. This is the ratchet PROBE-086 already uses.

**Definition of done:** the table lists today's failures exactly; deleting any row turns the test red.

### Phase 1 — no refusals

**Tasks:**
- Add `STRING` to `IsAOMPrimitiveShortName`. The validator shares the switch, so check what `C_STRING` checks now start firing on the vendored compositions, and record any new finding as consumer-visible.
- Add `rmwrite` arms for `ISM_TRANSITION.current_state`, `PERSON.details` and `ADDRESS.name`, each with a unit test.

**Definition of done:** every compiling vendored OPT generates and builds without error; the refusal rows are gone from the table.

### Phase 2 — no placeholder values

**Tasks:**
- Fill a `String` attribute only where nothing set it, and give typed defaults to the attributes that have them: DV_DATE_TIME from `Options.Now`, ENTRY `language` from `Options.Language`, ENTRY `encoding` from the openEHR default character set.
- Build the DV_ORDINAL `symbol` from the template's (value, symbol) pair.
- Mirror each fix in the builder path (`openehr/composition` calls the same engine; check `WithLanguage` and `WithNow` end to end).

**Definition of done:** no `"example"` string and no `local::example` code anywhere in generated output (a test walks the output and asserts it); the ordinal rows are gone from the table.

### Phase 3 — RandomFill stays ordered

**Tasks:**
- Order DV_DATE, DV_TIME, DV_DATE_TIME, DV_PROPORTION and DV_ORDINAL bounds as DV_COUNT and DV_QUANTITY already are, and draw both DV_QUANTITY sides in one unit.

**Definition of done:** 40 seeds over the vendored interval templates give no inverted or mixed-unit interval.

### Phase 4 — the rest of the floor findings, and PROBE-027 widened

**Tasks:**
- Fix what `ValidateRM` still reports on generated output: give CLUSTER `items` at least one member, and fill ELEMENT `name` / `archetype_node_id` and ACTION `time` / `ism_transition`.
- Widen PROBE-027 in `conformance.md`: the whole compiling corpus, with a `ValidateRM` leg beside `ValidateComposition`. This is a spec change, so the PR is full lane.
- § REQ-107 is edited anyway: drop its stale "lands with Phase 2" markers in the same PR.

**Definition of done:** the ratchet table is empty; PROBE-027 runs the corpus and fails when either validator reports a finding; `make ci` green.

## Consumer-visible changes

Generated and builder-built values change: real timestamps, languages and encodings instead of `"example"`, ordinal symbols, ordered random bounds, and templates that failed before now build. The builder's output was invalid before, so this is a fix. The release notes still need to name it, because callers may have snapshot tests.
