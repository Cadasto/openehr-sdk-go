# Plan — AQL semantic layer (containment admissibility + semantic lint + builder verification)

> **For agentic workers:** execute task-by-task (superpowers:subagent-driven-development or
> superpowers:executing-plans). **Read "The big picture" and "Guard-rails" sections before ANY
> task** — they exist because implementers of a single task tend to tunnel-vision; this feature
> only makes sense against its context. Normative details (issue-code catalogue, verdict/field
> definitions, overlay tables) are settled in Phase 0's REQ prose — the spec is the source of
> truth; this plan gives direction and boundaries, not final wording.

**Date:** 2026-08-21
**Status:** Done
**Owner:** SDK maintainers
**Covers:** **REQ-160** (AQL containment admissibility relation), **REQ-161** (AQL semantic & portability lint), **REQ-162** (builder containment verification) — all landed; canonical homes at [clinical-modeling.md](../../specifications/clinical-modeling.md) beside REQ-109/113. Band **160–169 = AQL semantics** (allocation approved 2026-08-21; REQ.md numbering paragraph updated in Phase 0).
**Decision record:** **ADR-0017** — AQL semantic layer architecture (authored in Phase 0).
**Probes:** **PROBE-097** (semantic lint corpus) — Implemented (inline).
**Implementation:** landed
**Depends on:** landed [REQ-048](../../specifications/bmm-conformance.md#req-048--rm-meta-model-introspection-surface) (`openehr/rm/rminfo` class graph — built with "AQL class-expression expansion and CONTAINS conformance" as a named consumer), landed [REQ-109](../../specifications/clinical-modeling.md#req-109--aql-static-lint) (lint layers + Issue model), landed [REQ-113](../../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) (`openehr/aql/parse`), [REQ-117](../../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) containment algebra (`openehr/aql.Containment`).
**Defers:** see § Deferred follow-ups (typed result columns, template-derived FROM builder, engine-capability profiles, demographic overlay data, RM-path shape lint).

---



## The big picture — read this first

**The failure this addresses is the costliest one a clinical query stack has.** Almost any
`X CONTAINS Y` parses; only pairs that can nest in the openEHR Reference Model can ever
return rows. `OBSERVATION o CONTAINS COMPOSITION c` is upside-down;
`COMPOSITION c CONTAINS OBSERVATION o CONTAINS SECTION s` is structurally impossible.
Server-side validation of RM structural legality is not common practice today — EHRbase, for
example, checks containment at storage-root granularity, so such queries are accepted and
return zero rows (observed behaviour; kb `notes/ecosystem/ehrbase-aql.md` §4.1.2). The
consequence is engine-independent: an empty result set cannot be told apart from an impossible
query, and in a clinical system "no data found" reads as a clinical fact. Client-side static
analysis is therefore the natural place to catch this class of defect. The EHRbase Java SDK
keeps its AQL front-end deliberately syntactic (kb `notes/ecosystem/ehrbase-sdk.md`
§5a.1/§5a.3); this layer adds the semantic tier above the same kind of neutral AST.

**The raw material already exists in this repo.** `openehr/rm/rminfo` (REQ-048) ships the
BMM-derived class graph — attribute RM types, abstractness, `ConformsTo`,
`ConcreteDescendants` — and its doc comment already names AQL CONTAINS conformance as an
intended consumer. This plan wires that groundwork to the AQL surface. **Do not hand-write RM
rules the BMM can answer** — derive them.

**Three design facts an implementer must internalise** (getting any one wrong produces a
subtly broken layer):

1. **CONTAINS means "descendant at any depth", not "direct child".** Every shipping engine
  skips levels; `COMPOSITION c CONTAINS ELEMENT l` is legal. The admissibility question is
   therefore *reachability* in the RM composition graph (can a Y node ever appear anywhere in
   an X's subtree), not a parent/child table. A naive direct-child table false-flags valid
   queries — the one failure mode worse than missing a defect.
2. **Abstract classes stand for all their concrete kinds.** `CONTAINS ENTRY e` must pass if
  *any* concrete descendant fits. This is `ConformsTo` / `ConcreteDescendants` on **both**
   operands — not a fixed name list; REQ-160 § Reachability semantics is the contract.
   Commonly-seen abstract operands are examples only (`CONTENT_ITEM`, `ENTRY`, `CARE_ENTRY`,
   `EVENT`, `ITEM_STRUCTURE`, `ITEM`, and equally `LOCATABLE`, `PARTY`, `VERSIONED_OBJECT`) —
   do not hard-code any such set.
3. **The relation cannot be a closed table.** The RM links some things by *reference*, not
  containment — `EHR → COMPOSITION` (universal AQL practice), `VERSION` wrappers,
   `FOLDER → COMPOSITION` (engine-specific) — and dialects extend it: Cadasto's own PHP
   platform ships demographic containment (`FROM PERSON p CONTAINS … AND EHR e CONTAINS …`,
   kb `notes/aql-spec-change-proposals.md` AQL-C-010). The core must be BMM-derived and the
   exceptions must be **overlay data**, so a consumer can extend the relation without forking it.

**Why severities are what they are.** The openEHR QUERY spec is *silent* on whether an engine
must reject RM-impossible containment — that silence is a registered spec gap
(kb `notes/aql-spec-change-proposals.md` **AQL-C-009(d)**). So an Error here is the SDK taking
a documented position on an open working-group question, not enforcing spec text — the REQ
prose says so explicitly, and this implementation doubles as implementation experience for the
WG proposal. Three diagnostic categories, fixed in ADR-0017:


| Category                                                 | Example                                           | Severity                                         |
| -------------------------------------------------------- | ------------------------------------------------- | ------------------------------------------------ |
| RM-impossible (provable from the pinned BMM)             | `OBSERVATION CONTAINS COMPOSITION`                | **Error**                                        |
| Spec-gap portability hazard (cite the governance ticket) | `CONTAINS VERSION` with no predicate (SPECPR-481) | **Warning**                                      |
| Engine capability difference                             | `NOT CONTAINS` is valid AQL, not implemented on every engine | **out of scope here** — deferred engine profiles |


**What this layer must never do:** adjudicate **row semantics**. What a row *is* when sibling
containments multiply (the "cartesian product problem") has been unresolved for seventeen
years and engines still diverge (kb `notes/aql-containment-row-semantics.md`). The SDK may
*warn* that row grain is engine-defined (one advisory check below); it must never pick an
answer, dedupe, zip, or refuse. Structural admissibility — this plan — is decidable; row
semantics is not ours.

**Where this could head** (context only — all deferred and *not all committed*, see
§ Deferred follow-ups for the honest status of each): possible later consumers of the same
relation include typed result columns (unproven value — only if a concrete consumer asks),
an experimental template-derived FROM builder (approach with scepticism — AQL scopes by
archetype, not template; see the deferral entry), and the consuming CDR's plan/lower stage,
which already plans over `openehr/aql/parse`. Build the relation so such consumers *could*
exist; do not build any of them now.

## Goal

Ship a **building-block** containment-admissibility relation (`openehr/aql/contain`), wire it
into the existing AQL linter as new Layer-2 semantic checks plus spec-gap portability checks
(REQ-161), and expose an opt-in verification entry point on the builder (REQ-162) — while the
parser stays permissive and `Builder.Build` keeps its exact current contract. Consumers:
integrators linting hand-written or stored AQL, programs verifying built queries before
submission, CI pipelines, and (later) the consuming CDR's lowering stage.

## Architecture

```
                    openehr/rm/rminfo (REQ-048, landed)
                    attribute types · ConformsTo · ConcreteDescendants
                              │  (runtime derivation, memoized — no new codegen)
                              ▼
              openehr/aql/contain          ← NEW leaf package (REQ-160)
              reachability relation + overlay edges + verdicts
              imports rminfo + openehr/rm (ParseArchetypeID, REQ-120)
              ONLY — never aql, parse, or lint
               │                                    │
               ▼                                    ▼
openehr/aql/lint (REQ-161)             openehr/aql (REQ-162)
adjacent CONTAINS pairs from           Builder walks its own Containment
parse.Document → Issues                tree → opt-in verification findings
(+ VERSION/fan-out portability         (Build() itself UNCHANGED)
 checks, no contain needed)
```

Placement is forced by the existing import graph: `lint` imports `aql`, so the shared rule
engine must sit **below both** — a new sub-package whose direct imports are `rminfo` and
`openehr/rm` (REQ-120's canonical `ParseArchetypeID`; REQ-013 safe: no transport/auth
imports).

## Guard-rails (anti-hallucination — binding for every task)

- **Look it up, never guess** (AGENTS.md ground truth): RM structure comes from
`resources/bmm/` via `rminfo`; overlay/exception facts come from the kb citations given per
task and are fixed in REQ-160 prose during Phase 0. If a containment fact is not derivable
from `rminfo` and not in the REQ prose, it does not go in the code.
- **Engine neutrality — EHRbase is a first-class target.** This SDK must keep working against
EHRbase and any other conformant CDR. The default relation and default lint severities express
the openEHR RM and specifications, never one vendor's storage model; Cadasto-specific
semantics enter only through overlays/profiles a caller explicitly opts into. Acceptance
guard: an RM-valid query that runs on EHRbase must never draw an Error from the default
profile. Where engines differ, the code and tests *observe* the difference neutrally
(technical fact + citation) — no judgement of any implementation.
- **Conservative flagging policy** (same policy as `lint/resolve.go` pathDivergence; the
normative home is REQ-161 § Flagging policy): flag only what is *provably* wrong from the
pinned BMM; anything uncertain stays out of **Error**. This does **not** mean "return
`Admissible` when in doubt" — the relation's verdict for an unknown name is `UnknownClass`
(→ `aql_unknown_rm_class` Warning), and a known pair with no route is `Never`; per REQ-160
§ Verdicts. A false Error is worse than a missed defect.
- **Unknown ≠ wrong.** A class the pinned BMM (and overlays) does not know — future RM,
demographic profile, dialect — yields the `UnknownClass` verdict → `aql_unknown_rm_class`
Warning (REQ-160 § Verdicts). Never Error on unknown names.
- **Value-free diagnostics discipline** (REQ-109): follow the existing `lint.Issue` field
contracts exactly — `Code`/`Severity`/`Span` value-free, `Path`/`Detail` value-bearing and
documented as such. New findings carry positions via `Span`, never by embedding query text
in value-free fields.
- **Do not touch:** the ANTLR grammar / `parse` package acceptance (the parser must keep
reading foreign, broken queries — that is what makes the linter useful), `Builder.Build`'s
existing validation set and error texts, existing issue codes or their severities.
- **No row-semantics logic.** No deduping, no result shaping, no "expected row count"
reasoning anywhere in this plan's code.
- Repo mechanics: `make fmt` via hooks; `make ci` is the gate; commits follow Conventional
Commits; code/tests carry `// REQ-160` / `// REQ-161` / `// REQ-162` / `// PROBE-097`
citations; every phase re-runs `make spec-check`.



## Ground truth & background reading (per-task pointers repeat these)


| Fact                                                         | Source                                                                   |
| ------------------------------------------------------------ | ------------------------------------------------------------------------ |
| RM class graph, attribute types, conformance                 | `openehr/rm/rminfo` (generated from `resources/bmm/`)                    |
| Semantic containment tree + rules table                      | kb `/src/cadasto/openehr-kb/notes/aql-language-reference.md` §6.1a       |
| EHRbase admissibility matrix (test oracle + observed-difference list) | kb `notes/ecosystem/ehrbase-aql.md` §4.1.2                               |
| Spec gaps this layer takes positions on                      | kb `notes/aql-spec-change-proposals.md` AQL-C-009, AQL-C-010; SPECPR-481 |
| Row-semantics no-go zone                                     | kb `notes/aql-containment-row-semantics.md` (§2, §16.3)                  |
| VERSION portability guidance written for this SDK            | kb `notes/aql-versioning-patterns.md` §5                                 |
| Existing lint layers, severity model, false-positive policy  | `openehr/aql/lint/lint.go`, `lint/resolve.go`, REQ-109 §                 |


The kb tree is read-only background; **normative content is copied into REQ prose in Phase 0**
so the spec tree stays self-contained (implementation agents work from the spec, with kb as
context).

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed**:

- ADR-0017 **Accepted**; REQ-160/161/162 sections + REQ.md registry rows + band paragraph
exist; PROBE-097 catalogued in conformance.md; `traceability.yaml` carries planned entries.
- The REQ prose fixes what this plan deliberately leaves open: exact verdict names and
`Finding` fields (REQ-160/162), the issue-code catalogue with per-code severity and
value-free classification (REQ-161), and the overlay edge table with citations (REQ-160).
- **The negative space is normative too** — hold it while implementing: REQ-160's **Never** /
**UnknownClass** verdicts and the pair-totality short-circuit (an UnknownClass operand makes the
pair UnknownClass; a Never-containability operand makes the pair Never —
[§ REQ-160](../../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation));
REQ-161's suppression rules (one finding per defect, and never an Error built on an unknown name
— [§ REQ-161](../../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint));
REQ-162's `Build()` **MUST NOT** change, down to byte-identical emission, and verification never
runs implicitly
([§ REQ-162](../../specifications/clinical-modeling.md#req-162--builder-containment-verification)).
- `make spec-check` green on the spec-only commit.



## Definition of Done

- `openehr/aql/contain` landed with exhaustive table tests + EHRbase-matrix difference tests
  (incl. the EHRbase-compatibility guard).
- REQ-161 checks live in `openehr/aql/lint`; PROBE-097 passes; `cmd/examples/lint-aql` shows a
semantic finding.
- REQ-162 verification entry point on the builder with a worked example.
- `traceability.yaml` + REQ.md **Impl.** column flipped; CHANGELOG (short, artefact-class
entry); `docs/roadmap.md` updated; `make spec-check` + `make ci` green; plan archived.



## Implementation checklist


| Step                                                              | Status |
| ----------------------------------------------------------------- | ------ |
| Phase 0: ADR-0017 + REQ-160/161/162 + band + PROBE-097 registered | ✅ landed 2026-08-22 |
| Phase 1: `openehr/aql/contain` + tests                            | ✅ landed 2026-08-24 |
| Phase 2: lint checks + PROBE-097 + example                        | ✅ landed 2026-08-24 |
| Phase 3: builder verification + example                           | ✅ landed 2026-08-24 |
| Phase 4: traceability / CHANGELOG / roadmap / archive             | ✅ landed 2026-08-24 |
| `make spec-check` + `make ci` green at every phase boundary       | ✅ (constituent commands green; `make ci` itself blocked on this host — Docker unavailable) |


---



## Phases



### Phase 0 — Specify (sdd-specify; no production code)

> **Landed 2026-08-22.** The task descriptions below were the authoring brief; the canonical
> wording now lives in the REQ-160/161/162 sections and ADR-0017. Where this brief and the
> landed prose differ (they do — review rounds tightened the verdict model, the containable
> set, the overlay rules, and the import contract), **the spec governs.**

**Tasks:**

1. **ADR-0017 — AQL semantic layer architecture.** Records, with rationale and alternatives:
  (a) relation derived at runtime from `rminfo` (no new codegen; bmmgen table generation is
   the rejected alternative — revisit only if init cost measurably matters); (b) BMM core +
   overlay edges + extensibility for dialects (AQL-C-010 is the forcing example);
   (c) the three diagnostic categories and their severities, incl. the explicit statement that
   Error-on-impossible is an SDK position on open gap AQL-C-009(d);
   (d) row semantics declared out of scope (advisory warning only);
   (e) builder stays permissive — verification is opt-in, `Build()` unchanged.
2. **REQ-160 — containment admissibility relation** (clinical-modeling.md, band opener).
  Authors the verdict vocabulary, reachability semantics, the containable-operand rule, the
   overlay edge table, archetype/class conformance, and the extensibility contract. Canonical
   wording: [REQ-160](../../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation).
3. **REQ-161 — semantic & portability lint.** Authors the issue-code catalogue — eight codes:
  `aql_impossible_containment` · `aql_contains_not_containable` ·
   `aql_archetype_class_mismatch` · `aql_unknown_rm_class` · `aql_containment_by_reference` ·
   `aql_version_no_predicate` · `aql_versioned_object_unreferenced` · `aql_fanout_row_grain` —
   each with its severity, firing rule, and value-free field classification, in the existing
   Layer-2 slot. Canonical catalogue:
   [REQ-161](../../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint).
4. **REQ-162 — builder containment verification.** Opt-in entry point on the write side;
  same verdicts/finding vocabulary as REQ-160; `Build()` contract explicitly unchanged.
5. **Registry & plumbing:** REQ.md rows (Impl. `planned`), band paragraph rewrite ("160–169 =
  AQL semantics band, opened by REQ-160…" — replacing the "stays free" sentence),
   PROBE-097 row in conformance.md, planned entries in `traceability.yaml`.

**Verification:** `make spec-check` green; sdd-doc-reviewer pass on the three new sections.

**Definition of done:** DoR above satisfied; single spec-only commit.

### Phase 1 — `openehr/aql/contain` (REQ-160)

**Files:** create `openehr/aql/contain/{doc.go,relation.go,overlay.go,relation_test.go,overlay_test.go,imports_test.go}`.

**Interfaces (Produces — later phases use these exact names):**

```go
package contain
type Verdict int              // Admissible, Never, ByReference, UnknownClass (exact set per REQ-160)
type Edge struct { From, To string; ByReference bool }
type Relation struct { /* opaque; memoized reachability over rminfo */ }
func Default() *Relation                                  // RM profile: BMM core + REQ-160 overlay table
func (r *Relation) WithOverlay(edges ...Edge) *Relation   // returns extended COPY (immutability, like aql.Containment)
func (r *Relation) CanContain(ancestor, descendant string) Verdict
func (r *Relation) Containable(rmType string) Verdict     // legal CONTAINS operand at all
func (r *Relation) ArchetypeMatches(rmType, archetypeID string) Verdict
type Finding struct { Code string; Detail string; /* value-bearing; exact fields per REQ-160/162 */ }
```

**Tasks:**

1. Reachability core: derive "which classes can appear in X's subtree" from
  `rminfo.Default()` (`AttributeRMType` walk + `ConformsTo`/`ConcreteDescendants` expansion),
   transitive closure memoized on first use; case handling per the lexer's rules
   (`validateRMTypeToken` / `asciiKeyword` in `openehr/aql/identifier.go` show the precedent).
   TDD from the REQ-160 acceptance table.
2. Overlay mechanism + the RM-profile overlay data from REQ-160 (EHR root, VERSION wrappers,
  FOLDER→COMPOSITION by-reference).
3. `ArchetypeMatches`: decompose the HRID via `rm.ParseArchetypeID` (REQ-120's canonical
  parser — write no lexical logic), `ConformsTo` against the declared class; unparseable
   HRID or unknown class/segment → `UnknownClass` (REQ-160 § Archetype/class conformance).
4. **Oracle tests:** (a) exhaustive table tests straight from the REQ-160 acceptance table
  (the kb §6.1a rules: `EHR CONTAINS OBSERVATION` ✅, `OBSERVATION CONTAINS COMPOSITION` ❌,
   `COMPOSITION CONTAINS ELEMENT` ✅ depth-skip, entries never contain entries,
   `INSTRUCTION CONTAINS ACTIVITY` ✅, `CLUSTER CONTAINS CLUSTER` ✅, `SECTION CONTAINS  SECTION` ✅, `CONTAINS DV_TEXT` ❌-not-containable, abstract `ENTRY` expansion ✅, …);
   (b) a **documented-difference test** against the EHRbase admissibility matrix, in neutral
   observed-behaviour terms: pairs this relation marks Never that EHRbase admits
   (RM-impossible pairs sharing a storage root) and pairs the RM permits that EHRbase
   restricts for engine-specific reasons (`EHR CONTAINS ELEMENT` root disambiguation,
   standalone `FROM CLUSTER`) — each difference asserted and commented with its kb citation,
   so the boundary is executable documentation. This test doubles as the
   **EHRbase-compatibility guard**: an RM-valid pair that EHRbase admits must never verdict
   `Never`.
5. `imports_test.go`: assert the package's direct imports are only `rminfo` + `openehr/rm` +
  stdlib (pattern exists in `openehr/aql/lint/imports_test.go`).

**Verification:** `go test ./openehr/aql/contain/...` · `make ci`.

**Definition of done:** verdicts match the REQ-160 acceptance table; difference tests document
every deliberate difference from EHRbase's observed behaviour, and the EHRbase-compatibility
guard holds; no import leaks.

### Phase 2 — Lint integration (REQ-161)

**Files:** modify `openehr/aql/lint/lint.go` (wire new check group), create
`openehr/aql/lint/semantic.go` + `semantic_test.go`; extend `testkit/probes/aql` (PROBE-097
fixtures: impossible / unknown-class / portability corpus with expected issue-code multisets);
update `cmd/examples/lint-aql` output; update the lint package doc.

**Interfaces:** consumes Phase 1's `contain.Default()`, `CanContain`, `Containable`,
`ArchetypeMatches`. New `lint.Options` field for a caller-supplied `*contain.Relation`
(nil → `contain.Default()`), so dialect users (AQL-C-010) can lint without false Errors.

**Tasks:**

1. Containment-pair extraction from `*parse.Document`: **adjacent** CONTAINS pairs only
  (reachability composes — pairwise checks are exactly what the query asserts), junction
   operands checked against the junction's parent, `NOT CONTAINS` pairs checked identically
   (an impossible pair under NOT CONTAINS is a dead constraint — same code; REQ-161 wording
   decides the Detail text). Emit `aql_impossible_containment` / `aql_unknown_rm_class` /
   `aql_containment_by_reference` / `aql_contains_not_containable` with Spans on the class
   expression.
2. `aql_archetype_class_mismatch` on every literal archetype predicate (skip `$param`
  predicates — PROBE-021 territory, same skip reason as `resolve.go`).
3. VERSION portability checks (pure `parse.Document` walks, no `contain` needed):
  `aql_version_no_predicate`, `aql_versioned_object_unreferenced` (VERSIONED_OBJECT alias
   never referenced in SELECT/WHERE/ORDER BY — Frankel's redundancy rule, kb
   `aql-versioning-patterns.md` §3.3/§5).
4. `aql_fanout_row_grain` advisory — fire ONLY on the high-confidence shape REQ-161 defines
  (sibling junction class operands whose aliases are both projected as leaf paths); Detail
   states row grain is engine-defined. No other heuristics.
5. PROBE-097 fixtures + stable-multiset assertion (mirror PROBE-028's pattern); refresh
  `docs/specifications/conformance.md` probe row status.

**Verification:** `go test ./openehr/aql/lint/...` · probe run · `make ci` · `make spec-check`.

**Definition of done:** all REQ-161 codes fire on the probe corpus, and the existing corpus
obeys REQ-161 § Additivity — a query carrying no REQ-161 defect keeps its issue-code multiset,
while a corpus query that genuinely carries a newly-checked defect (e.g. an impossible
containment) is re-baselined deliberately and recorded, per PROBE-097's additivity guard. (Not
a blanket "zero new findings" rule — that would contradict the feature.)

### Phase 3 — Builder verification (REQ-162)

**Files:** modify `openehr/aql/builder.go` (or a new `verify.go` beside it) + tests; update
`openehr/aql/doc.go`; extend `cmd/examples/aql-build` with a verification snippet.

**Interfaces:** consumes `contain` exactly as lint does. Produces (name fixed here, details
per REQ-162):

```go
// package aql
func (b *Builder) VerifyContainment(r *contain.Relation) []contain.Finding // nil → contain.Default()
```

**Tasks:**

1. Walk the builder's own `Containment` tree + FROM root (write-side structures — no emit,
  no re-parse), emitting the same finding codes as REQ-161's containment checks. Junction
   and NOT-CONTAINS traversal mirrors the shapes `validateContainsChain`/`validateTree`
   already walk — reuse their traversal order, do not duplicate their validation.
2. `Build()` untouched — add a test asserting a query with an RM-impossible containment still
  builds and emits byte-identically to today (the permissiveness contract, pinned).
3. Worked example + doc.go paragraph (positioning: "Build validates grammar shape;
  VerifyContainment is the opt-in RM-semantics gate; LintString covers the read side").

**Verification:** `go test ./openehr/aql/...` · `make ci`.

**Definition of done:** builder findings agree with lint findings for equivalent queries
(shared-engine parity test: build → emit → LintString, compare code multisets).

### Phase 4 — Close-out

**Tasks:** flip `traceability.yaml` + REQ.md **Impl.** to `landed`; CHANGELOG Unreleased entry
(short, artefact-class style); `docs/roadmap.md`; `docs/examples.md` if example inventory is
listed there; run `/sdd-trace`; archive this plan via sdd-archive **in the implementing PR**.

**Verification:** `make spec-check` + `make ci` green; probe-status shows PROBE-097.

---



## Deferred follow-ups (named so nobody "helpfully" builds them early)


| Deferral | Direction and honest status |
| --- | --- |
| **Typed result columns** | *Optional — value unproven, no confirmed demand.* Resolved SELECT paths know their leaf DV type, which *could* type `aql.ResultCell` consumers (`openehr/client/query`). Pick up only if a concrete consumer asks for it, or if it falls out of other work nearly free. |
| **Template-derived FROM builder** | *Exploratory experiment — approach with scepticism.* AQL scopes by **archetype** predicates and is usually template-agnostic: the same archetype aggregates data persisted under several different templates, so a containment chain derived from one `templatecompile.Compiled` risks over-fitting the query to that template. Template identity belongs in a `WHERE …/archetype_details/template_id/value` condition, not in containment. If prototyped: emit an archetype-scoped chain (template-agnostic by default), add the template_id condition only as an explicit opt-in, and treat the prototype as a feasibility probe — curiosity-driven, not committed. |
| **Engine-capability profiles** | Per-engine lint category for valid AQL a given engine does not implement (e.g. EHRbase: `NOT CONTAINS`, `EXISTS`, standalone ambiguous roots) — neutral, observed-behaviour data from the kb admissibility matrices. Also the vehicle for "will this query run on engine X" portability reports. |
| **Demographic overlay data**                       | AQL-C-010 dialect edges (`PERSON`→`EHR` by-reference…) as a shipped overlay; mechanism lands now, data waits for the dialect spec.                           |
| **RM-path shape lint**                             | Template-free SELECT/WHERE path checking against `rminfo` attribute walks (`rmpath` precedent) — a Layer-2.5 between shape and template checks.              |
| **Full semantic resolver / CDR lower-stage reuse** | One resolution pass producing a typed query model consumed by lint, builder, executor, and the consuming CDR's plan/lower stage.                             |
| **Non-containable FROM root** | *Deferred — spec-sanctioned silence, not an oversight.* `aql_contains_not_containable` fires only on a `CONTAINS` operand; a non-containable FROM root (e.g. `FROM DV_TEXT t …`) raises no *containability* code today, in either spelling — other role-orthogonal codes (`aql_archetype_class_mismatch`, `aql_unknown_rm_class`) are unaffected and still fire on the root as usual. Widening the containability code to the anchor position needs its own REQ/spec sentence, not a silent code change. |




## Mapping to specs

- [clinical-modeling.md § REQ-160 / § REQ-161 / § REQ-162](../../specifications/clinical-modeling.md) — canonical normative contracts (authored Phase 0)
- [docs/adr/0017-aql-semantic-layer.md](../../adr/0017-aql-semantic-layer.md) — architecture decision (authored Phase 0)
- [REQ.md](../../specifications/REQ.md) — registry rows + 160–169 band paragraph
- [conformance.md § PROBE-097](../../specifications/conformance.md) — probe catalogue row
- [traceability.yaml](../../specifications/traceability.yaml) — REQ→code/test map
