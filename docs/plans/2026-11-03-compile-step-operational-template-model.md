---
kind: plan
---

# Plan — The compile step reads the operational template model

**Date:** 2026-11-03
**Status:** In delivery (`/sdd-deliver`), one pull request
**Covers:** [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge), [REQ-100](../specifications/clinical-modeling.md#req-100--adl-14-operational-template-opt-parse-and-paths), [REQ-108](../specifications/clinical-modeling.md#req-108--untrusted-document-bounds), [REQ-103](../specifications/clinical-modeling.md#req-103--primitive-constraint-introspection), [REQ-176](../specifications/clinical-modeling.md#req-176--adl-14-operational-template-model), [REQ-013](../specifications/module-layout.md#req-013--building-block-independence), as [ADR 0035](../adr/0035-compile-step-reads-operational-template-model.md) decides
**Probes:** PROBE-022, PROBE-024, PROBE-025, PROBE-027, PROBE-074, PROBE-075, PROBE-076, PROBE-086 (restated pipeline)
**Depends on:** [the operational template model](2026-11-02-adl14-operational-template-model.md), merged in PR 369
**Defers:** removing `template.ParseOPT` and its tree (a later release); the model's content the compiled template still leaves out (`assumed_value`, `is_ordered`/`is_unique`, `component_ontologies`, `view`, revision history); a default size and depth bound for every canonical-XML decode

## Goal

`templatecompile.Compile` takes the generated `aom14.OperationalTemplate` instead of `template.ParseOPT`'s tree, so every consumer of the compiled template (validation, the example generator, the composition builder, FLAT and STRUCTURED, the Web Template builder, the AQL lint) reads what the parser leaves out, `C_CODE_REFERENCE` and the template-level `<constraints>` default values first. An untrusted template is bounded on the new path through `aom14.ReadOperationalTemplate`. Path lookup moves onto the compiled template, and the parser and its tree are deprecated.

## Decisions (maintainer, 2026-11-03)

- **One pull request:** the bounded reader, the switch with its corpus check, path lookup, the deprecation, the callers moved, and the two gains.
- **Tolerances:** the new path keeps what the corpus needs: `null_flavor` read as `null_flavour`, white space trimmed around identifiers and term text, the interval flag defaults (the model already applies them) and a byte-order mark. The compile step refuses what `ParseOPT` refused and the model's decode accepts: a missing `template_id` or `definition`, an inverted interval. The lenient-only readings (an unknown type read as a leaf, a missing `xsi:type` defaulted, a bad number skipped, a time bound that is not ISO 8601 read as unbounded, a missing `C_BOOLEAN` flag read as true) stay with the deprecated parser. All 74 corpus OPTs pass both the parser's strict mode and the model's.
- **Path lookup over the compiled template,** exposed by the public bridge; `template.ParsePath` stays a function over strings in `openehr/template`. ADR 0035's "over the model" is amended by a dated header line, as are its keep-list (`ArchetypeTerm`, `TermBinding`, `CodedTermRef` join `Cardinality`, `Multiplicity`, `ErrPathNotFound`) and its *Amends* list (REQ-103, REQ-107, REQ-116, REQ-176, wire.md and idiom.md rows as Task 1 finds them).
- **Bounds:** `aom14.ReadOperationalTemplate` applies the size limit and the depth limit, the depth enforced during decode by a new canonical-XML decode option with its own sentinel. A direct `canxml` call stays unbounded.

Defaults this plan takes, which the maintainer may overrule at review:

- The parity check is a golden of the compiled tree for every corpus OPT, written from the parser's path before the switch (Task 2) and held afterwards; it outlives the parser, so it is a REQ-111 test, not a new probe.
- An `ARCHETYPE_SLOT` with no `node_id` takes as its AQL predicate the archetype-id pattern of its first include, as its slot rules read it, in place of the include's raw inner XML; the five pinned paths change once, in Task 6.
- `templatecompile.Compile` keeps its name and takes `*aom14.OperationalTemplate`; the reader takes `canxml` decode options, so strict reading is `canxml.WithStrict`.

## Research

Delivery notes `feat--compile-step-operational-template-model.md` (git directory, `sdd/deliver/`) hold the research of 2026-11-03: the field map from the parser's tree to the model, the caller census (88 files, none in library code apart from the bridge), the tolerance table with corpus counts, the bounds measurements and the benchmarks. Briefs quote the rows they need.

## Tasks

Sequential on the branch in the main checkout unless a wave is named; one `sdd-implementer` per task. Workers load `go-coding:go-coding` and the focused skill for their diff. Every task: test first, red quoted, `go test` on the touched packages, `golangci-lint run` on the changed directories, `make spec-check`; commit by path with Conventional Commits and the `Assisted-by` trailer.

### Task 1: specification amendments

In `/sdd-specify`'s form, a citation rather than a restatement, no keyword added for internals:

- ADR 0035: a dated header line recording the four decisions above (header only; the body stays as written).
- clinical-modeling.md § REQ-108: a part for the model's reader, `aom14.ReadOperationalTemplate`: the 32 MiB size limit and the depth limit in XML elements equal to 128 constraint-object levels, both refused with the new `canxml` sentinel; the compile step's path walk keeps the 128-level bound. The heading "OPT parse and path walk" names the deprecated parser.
- § REQ-111: `Compile` takes the model; the compile step trims, renames `null_flavor`, refuses a missing `template_id` or `definition` and an inverted interval, builds the primitive constraints and slot rules from the model, and derives an unidentified slot's predicate from its first include's pattern; terms and bindings come from the model's archetype roots; path lookup on the compiled template with REQ-100's path grammar and resolution rules (first match, `WithStrictPaths`' ambiguity, `ErrPathNotFound`); `C_CODE_REFERENCE` and the `<constraints>` default values carried, their accessors named.
- § REQ-100: the parser, its tree, `NodeAt` and `ValidatePath` deprecated, binding until removal; `ParsePath` and the path grammar stay; *Strict parse mode* and the tolerances say they are the parser's.
- § REQ-103: the compile step builds the constraints from the model's primitive classes; a `C_CODE_REFERENCE` constraint type; the parser-only leniencies named as the parser's.
- § REQ-107 (:607 slot occurrences), § REQ-116 (node name), § REQ-176 (*Out of scope*'s "Until it lands" bullet, the tense at :129), rm-functions.md:50: links and tense.
- module-layout.md § REQ-013: the `openehr/template` row restated for what it keeps; the `templatecompile` row's closure gains `openehr/aom/aom14`; an `aom14` row if the reader needs one.
- wire.md: the reader-sentinel row and the interval row for the new path; :324 unchanged (no lenient mode).
- conformance.md: PROBE-022, -024, -025, -027, -074, -075, -076, -086 restate the pipeline as read, compile; PROBE-027's missing `template_id` refused by `Compile`.
- architecture.md, roadmap.md (the two rows: "Compile step on the operational template model" to in delivery; "`<OPERATIONAL_TEMPLATE>` OPT root" closed by the reader, which does not match the root by name).

Files: the docs above, `docs/specifications/traceability.yaml` (REQ-111 packages gain `openehr/aom/aom14`). Verify: `make spec-check`.

### Task 2: the compiled-tree golden, before the switch

A test in `internal/templatecompile` that writes, for every `.opt` under `testkit/corpus/` (74, walked as PROBE-111 walks them), a text dump of the compiled tree to `testdata/compiled/<name>.txt` with an `-update` flag: per node its AQL path, RM type, node id, archetype id, occurrences, primitive constraint (a stable rendering), slot rules, node name, required and not-in-RM marks; per attribute its name, cardinality, existence and child multiplicity; the terms and bindings. Written from today's path and committed; the test fails on any difference. Files: `internal/templatecompile/compiled_golden_test.go`, its testdata.

### Task 3: a depth bound for canonical-XML decode

`canxml.WithMaxDepth(n int) DecoderOption` refuses, during the decode, an element nested deeper than `n`, with a new sentinel `canxml.ErrLimitExceeded` wrapped in a short `DecodeError` (the depth and the path to the element, no value), for `Unmarshal` and `Decoder`. No default bound. Tests: at the bound passes, one past refuses, a can-fail control. Files: `openehr/serialize/canxml/` (decode, doc, tests), and the shared decoder under `openehr/internal/xmlunmatched/` if the walk lives there. Wave with Task 2 (disjoint files).

### Task 4: `aom14.ReadOperationalTemplate`

`ReadOperationalTemplate(r io.Reader, opts ...canxml.DecoderOption) (*OperationalTemplate, error)` in a hand-written file of `openehr/aom/aom14`: reads at most the size limit plus one byte and refuses a larger input with `canxml.ErrLimitExceeded`; decodes through the stream `Decoder` with `WithMaxDepth` for the depth limit and the caller's options. Tests: the corpus reads; the size and depth refusals with their can-fail controls; a byte-order mark. Files: `openehr/aom/aom14/read.go`, its tests. Depends on Task 3.

### Task 5: primitive constraints and slot rules from the model

In `internal/templatecompile`, functions that build `constraints.PrimitiveConstraint` values from the model's primitive classes (`CPrimitiveObject.Item`, `CCodePhrase`, `CDVQuantity`, `CDVOrdinal`, `ConstraintRef`), quantity property units included, and slot rules from `ArchetypeSlot.Includes`/`Excludes`, with the trimming the parser applies. A test holds each result equal to the parser's over every corpus node. `template.Multiplicity` gains the constructor the compile step needs if none is exported. Files: `internal/templatecompile/primitives_from_model.go`, `slots_from_model.go`, tests; `openehr/template` for a constructor only. Wave with Task 4 (disjoint files).

### Task 6: the compile step reads the model

`internal/templatecompile` walks `aom14.OperationalTemplate` (a pointer switch over the 13 `CObject` kinds with a default that refuses), reading node names, terms, bindings and intervals from the model with Task 5's builders, applying the tolerances and refusals of § REQ-111; `openehr/templatecompile.Compile` takes `*aom14.OperationalTemplate`. The golden of Task 2 shows only the slot-predicate change; `pathsnapshot` is updated for the same five paths. `BenchmarkCompile` gains the read step, and the before and after numbers go in the PR body. Files: `internal/templatecompile/`, `openehr/templatecompile/`, their tests and testdata. Callers outside these two packages are moved in Task 10; until then they read with `ReadOperationalTemplate` where a test needs to compile.

### Task 7: the gains

- `C_CODE_REFERENCE`: a `constraints` type carrying the code phrase constraint and its `reference_set_uri`, built in Task 5's builder; each consumer's switch over constraint types gains its case, read as a code phrase constraint where the consumer has no use for the reference set (validation, the example generator, the Web Template builder, FLAT).
- `<constraints>` default values: each `T_COMPLEX_OBJECT`'s `default_value` placed on the compiled node its `differential_path` names, through an accessor on `CompiledNode`; a path that names no node is refused by `Compile`.

The golden shows the two additions and nothing else. Files: `openehr/template/constraints/`, `internal/templatecompile/`, the consumers' switch files, tests.

### Task 8: path lookup on the compiled template

A lookup on `Compiled` taking a `template.Path`, with REQ-100's resolution rules and the strict-paths option, refusing with `template.ErrPathNotFound` and `template.ErrAmbiguousPath`, bounded at 128 levels; a `ValidatePath` beside it. PROBE-022 and PROBE-024 move onto it, and the AQL lint's walk (`openehr/aql/lint/resolve.go`) where it duplicates it. Files: `internal/templatecompile/`, `openehr/templatecompile/`, `testkit/probes/template/probe_022_*`, `probe_024_*`, `openehr/aql/lint/`.

### Task 9: deprecations

`// Deprecated:` on `ParseOPT`, `ParseOPTStrict`, `ParseFile`, `ParseFileStrict`, the tree types and their methods, `NodeAt`, `ValidatePath` and `WithStrictPaths` in `openehr/template`, each naming its replacement; the package doc says what the package keeps. Mechanical. Files: `openehr/template/`.

### Task 10: callers moved (wave of three, disjoint files)

Every caller of the parser outside `openehr/template` reads with `ReadOperationalTemplate` and compiles: (a) the unit tests under `openehr/` and `internal/` (54 files); (b) `testkit/fixtures/parse_opt.go` (its root rename goes), `testkit/conformance/`, `testkit/probe/`, `testkit/probes/` apart from PROBE-022 and PROBE-024; (c) `cmd/cdrcheck`. Tests that pin the parser's own behaviour stay in `openehr/template`. Mechanical once Task 6 lands.

### Task 11: examples and documents

`cmd/examples/` (opt-parse rebuilt around the reader and the compiled lookup, primitive-validate on the compiled lookup, the others' parse calls), with `docs/examples.md`, `pages/`, `README.md` and the package docs of `openehr/template`, `internal/templatecompile` and `openehr/templatecompile` in the same commit series, as [ai-workflow.md § Examples](../ai-workflow.md#examples) asks.

## Verification

`make ci` and `make spec-check` on the integrated branch; the golden of Task 2 as the parity check; PROBE-111 unchanged; the compile benchmarks before and after.
