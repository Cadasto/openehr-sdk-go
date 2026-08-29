# Plan — AQL conformance corpus under CI: the containment relation's evidence base

**Date:** 2026-08-26
**Status:** Complete
**Owner:** SDK maintainers
**Covers:** no new REQ id — this puts the **evidence base** of landed [REQ-160](../../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation) under CI; REQ-160's prose gains one evidence sentence (implementation-aligned amendment) pointing at the new probe
**Probes:** [PROBE-100](../../specifications/conformance.md#probe-100--upstream-aql-admissibility-corpus-ratchet) — *Upstream AQL admissibility corpus ratchet*, In-repo, **Implemented (inline)**. PROBE-021's Cassette/Live ratification stays pending a reference deployment — unchanged by this plan, recorded here because the audit filed it beside this finding
**Implementation:** landed
**Depends on:** the sibling clone `/src/ehrbase/integration-tests` at the pinned commit `206ee8c` (the same clone and pin discipline [`testkit/cassettes/ROBOT_SOURCE.txt`](../../../testkit/cassettes/ROBOT_SOURCE.txt) already records); landed REQ-160/161/162 and the compatibility-guard tests in [`openehr/aql/contain/relation_test.go`](../../../openehr/aql/contain/relation_test.go)
**Defers:** replaying the suites' *result expectations* (expected-JSON comparison, row counts) — those are engine-behaviour tests against seeded data, out of an admissibility relation's scope; executing anything against a live engine (that is REQ-082 Phase 4 territory)

## Goal

Close audit finding **AQL-FIT-09** (AQL alignment audit, 2026-08-26 — maintainer's knowledge
base, fit-gap report Part 2): the EHRbase AQL conformance suites are cited as evidence in
REQ-160's derivation (the level-skipping rule points at the `FROM/CONTAINS_A_D` chaining suite),
but **nothing under `docs/` or `resources/` references the corpus and no test executes it** —
the relation's evidence is tested only against hand-written cases. A position derived from a
corpus, tested only against hand-written cases, drifts the day the corpus changes and nobody
notices. The SDK's own REQ-113 states the house standard: hold derived sets against the vendored
source *mechanically*, "rather than by an enumeration a maintainer must remember to extend."
This plan applies that standard to the containment relation.

## What the corpus actually is

The upstream suites are **data-driven**: the `.robot` files are harness, and the test *data* —
the part an admissibility oracle needs — lives in CSV files under
`tests/robot/_resources/test_data_sets/aql/fields_and_results/`, e.g. (from the chaining suite):

```
${from},${expected_file},${nr_of_results}
COMPOSITION CONTAINS OBSERVATION o,expected_…_compo_obs.json,5
SECTION CONTAINS OBSERVATION o,expected_…_section_obs.json,5
OBSERVATION o CONTAINS INTERVAL_EVENT,expected_…_obs_int_event.json,2
```

Every row is a FROM/CONTAINS shape the engine **accepted and answered** — an observed-behaviour
fact per row, in machine-readable form, with no Robot parsing required. The expected-JSON files
and row counts are execution facts this plan deliberately does not consume (§ Defers).

**Scope honesty (the audit's own caveat):** many of the 132 suites are engine-behaviour tests
(aggregates, ORDER BY collation, LIMIT paging), not RM-containment tests. The useful subset for
the relation is the **FROM family** — `CONTAINS_A_D`, `PREDICATE_A_D`, `USABLE_RM_TYPES_A_D`,
`EHR_STATUS`, `AND_OR` — and the excluded set is **generated, not hand-kept**: the ingest lists
what it skipped and why, so "covered" is never silently smaller than it reads.

## The ratchet property (PROBE-100)

For **every engine-accepted FROM/CONTAINS variant** in the vendored corpus:

1. it parses under the SDK grammar profile (`parse.ParseQuery` succeeds), and
2. the REQ-161 containment checks raise **no Error-severity code** on it — i.e. the REQ-160
   relation never verdicts **Never** on a pair a conformant engine demonstrably admits.

This generalises the landed `TestEHRbaseCompatibilityGuard` (hand-picked rows, same direction:
the SDK is deliberately the looser of the two, never the stricter) from a maintainer-kept
enumeration to the vendored corpus. Warnings are permitted — `aql_unknown_rm_class`,
`aql_containment_by_reference` and the portability advisories are not admissibility refusals.
The reverse direction (engine-rejected shapes) is **not** asserted: the SDK's documented
position is to stay looser, and the corpus encodes acceptance, not a rejection catalogue.

## Definition of Ready

- The upstream clone is present at the pin (`206ee8c`, the commit `ROBOT_SOURCE.txt` records) —
  the ingest reads the same tree the cassette ingest read.
- Phase 0's spec deltas are drafted: the PROBE-100 catalogue entry, the conformance.md
  § Vendored fixtures extension, and REQ-160's one evidence sentence.
- The corpus subset (the five FROM-family directories above) is confirmed against the upstream
  tree; any suite whose CSV cannot be located is recorded in the exclusion list up front.

## Definition of Done

- The vendored corpus lives under `testkit/cassettes/aql/conformance/` with its own provenance
  pin (`AQL_SOURCE.txt` in the `ROBOT_SOURCE.txt` format: repo, path, commit, dates, licence
  pointer — the licence is already covered by `THIRD_PARTY_LICENSES.md`, same upstream).
- PROBE-100 Implemented (inline), run by `make ci`; the exclusion list is emitted by the ingest
  and committed beside the corpus, so coverage claims are inspectable.
- The ratchet catches drift both ways: a relation change that newly refuses a corpus-accepted
  shape fails CI; a corpus refresh (re-run of the ingest at a newer pin) that adds
  newly-accepted shapes fails CI until the relation admits them or the row is explicitly
  excluded with a reason.
- Code and tests land with `// REQ-160` / `// PROBE-100` citations;
  [`traceability.yaml`](../../specifications/traceability.yaml) REQ-160 test list grows. It grew by
  more than the test list in the end: REQ-160's entry took the probe reader
  (`testkit/probes/aql/probe_100_conformance_corpus.go`) in `tests:`, PROBE-100 in `probes:` — legal
  only once Phase 2 flipped the catalogue entry off Draft — this plan in `plans:`, and a `notes:`
  sentence recording what the ratchet holds. The REQ-160 `- **Probes:**` trailer in
  clinical-modeling.md gained PROBE-100 beside PROBE-097; the `- **Plan:**` trailer did not, this
  plan allocating no requirement (the precedent the archived small-corrections plan set). No REQ.md
  registry change (no new id; the Impl. column was already `landed`).
- **The two indexes `make spec-check` cannot see:** a [roadmap.md](../../roadmap.md) note on the
  REQ-160 row (evidence base under CI); no numbering-band change (no new REQ).
- `make spec-check` and `make ci` pass. The probe needed no CI wiring: `make ci` reaches it through
  `make test`'s unrestricted `go test ./... -count=1`, which carries no path glob to miss
  `testkit/probes/aql/` — confirmed rather than assumed, and the reason Phase 3 landed no
  Makefile change.
- Plan archived under [`docs/plans/archive/`](./).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 0 — PROBE-100 entry + Vendored fixtures § + REQ-160 evidence sentence | ✅ landed 2026-08-29 |
| Phase 1 — ingest script + vendored corpus + provenance pin + exclusion list | ✅ landed 2026-08-29 — `scripts/ingest-robot-aql.sh`, 12 CSVs in five families, `AQL_SOURCE.txt`, `EXCLUDED.txt` |
| Phase 2 — CSV oracle + ratchet test | ✅ landed 2026-08-29 — 67 rows reconstructed and asserted; PROBE-100 Draft → Implemented (inline) |
| Phase 3 — CI wiring + close-out | ✅ landed 2026-08-29 — CI wiring **confirmed, not changed** (§ Phase 3); traceability, roadmap and both plan indexes updated |
| Index `spec-check` misses (roadmap note) | ✅ [roadmap.md](../../roadmap.md) REQ-160 row names the evidence base under CI; no numbering-band change (no new REQ) |
| `make spec-check` / `make ci` | ✅ both run in full on the host — `go` 1.26.6 and `golangci-lint` v2.13.0 are the versions the Makefile's own host-fast-path checks accept, so no pinned image was needed |

One [CHANGELOG.md](../../../CHANGELOG.md) bullet under `[Unreleased]`, per artefact class and
carried by this close-out commit — the way the REQ-163 and REQ-164 close-outs carried theirs. It
records the corpus ratchet and says plainly that no behaviour changed: the relation, the lint
codes and the public API are byte-unchanged, and what lands is a vendored corpus plus the probe
that holds them to it.

## Phases

### Phase 0 — Specify

**Tasks:** the PROBE-100 catalogue entry in [conformance.md](../../specifications/conformance.md)
(Title / Preconditions / Wire assertion — the ratchet property above / Modes: in-repo / Status:
Draft until Phase 2 / Satisfies: REQ-160); extend conformance.md § Vendored fixtures with the
AQL corpus provenance row; add one sentence to REQ-160 (§ Acceptance or the derivation notes)
naming PROBE-100 as the mechanical evidence gate, replacing the citation-only status the audit
flagged.

**Definition of done:** `make spec-check` passes; no code changes.

### Phase 1 — Ingest and vendor

**Tasks:** `scripts/ingest-robot-aql.sh` beside the existing
[`ingest-robot-cassettes.sh`](../../../scripts/ingest-robot-cassettes.sh) (separate script — the
cassette ingest is composition-fixture curation, this is corpus vendoring; same `ROBOT_ROOT`
convention): copy the FROM-family CSVs into
`testkit/cassettes/aql/conformance/<family>/…`, write `AQL_SOURCE.txt`, and emit
`EXCLUDED.txt` — every `aql/fields_and_results` file *not* taken, one per line with the family
and a reason tag (`execution-semantics`, `no-csv`, `non-from-family`) — generated by the script,
never edited by hand.

**Definition of done:** corpus + pin + exclusion list committed; re-running the script at the
same pin is byte-idempotent.

### Phase 2 — The oracle and the ratchet

**Tasks:** a small reader in the test tree (suggested home: `testkit/probes/aql/`, beside
PROBE-097's probe) that parses the vendored CSVs — header row names the template variables;
each data row's first field is the FROM/CONTAINS text — reconstructs the query
(`SELECT o FROM ` + variant, matching the suite's own template), and asserts the ratchet
property per row via `parse.ParseQuery` + `lint.LintString` (Error-severity containment codes
only; the code set taken from the REQ-161 catalogue, not hard-coded severities). Rows whose
variant is *not* pure FROM/CONTAINS (a stray WHERE in some CSVs) are normalised or excluded by
rule, recorded in the reader, not silently skipped. The probe result reports counts: rows
asserted, rows excluded, per-family.

**What shipped differed on one point:** `SELECT o FROM ` + variant is not one template but
twelve. A corpus row is not a query — upstream keeps the varying part in the CSV and the
invariant part in the consuming `.robot` suite — so the reader carries a per-suite reconstruction
table instead, each entry transcribed verbatim from the suite it names, and two of those
templates hold a run-time `${ehr_id}` that is substituted with a fixed literal. The catalogue
entry records that table as the other half of the probe's input.

**Definition of done:** PROBE-100 catalogue Status moves to Implemented (inline); the chaining
suite REQ-160 cites is among the asserted families; deliberately breaking one overlay edge in a
scratch build makes the ratchet fail (drift-detection rehearsed, not assumed).

### Phase 3 — CI wiring and close-out

**Tasks:** the probe runs under `go test ./testkit/...` (already in `make ci` — confirm, and
add the package if the testkit path glob misses it); `make probe-status` reflects PROBE-100;
traceability + roadmap note; archive the plan.

**What the confirmation found:** there is no testkit path glob to miss the package. `make ci`
runs `make test`, whose recipe is the unrestricted `go test ./... -count=1` (`Makefile`, `test:`
target), and `go list ./...` names `…/testkit/probes/aql` — the same route PROBE-097 and
PROBE-099 already take from the same package. GitHub Actions runs that same `make test` target.
So Phase 3 landed **no** Makefile or workflow change, and the wiring is recorded here rather than
added. `make probe-status` prints `PROBE-100 | Implemented (inline) |
testkit/probes/aql/probe_100_conformance_corpus.go`, the same three-column shape as its two
neighbours.

**Definition of done:** `make spec-check` and `make ci` pass with the ratchet active.

## Sequencing note

The audit's grouping called this slice "worth doing before the containment relation is extended
further, since it is the relation's evidence base." Concretely: land this **before or alongside**
the path-shape lint plan's Phase 4 (which adds a new relation query) and before any Phase-2
containment work revives PROBE-097's deferred rows — the ratchet is what makes such extensions
safe to review.

**What actually happened:** the path-shape lint plan landed first, so its `TypeRelation.Unavoidable`
widening went in ahead of the ratchet rather than behind it. The preference was soft and the
outcome is benign — the corpus holds the widened relation exactly as it holds the original, and
all 67 rows passed at first run — but the sequencing is recorded as missed rather than met, since
the next relation extension is the one this note is really written for.

## Mapping to specs

- [clinical-modeling.md § REQ-160](../../specifications/clinical-modeling.md#req-160--aql-containment-admissibility-relation) — the relation whose evidence this gates (+ one amended sentence)
- [clinical-modeling.md § REQ-161](../../specifications/clinical-modeling.md#req-161--aql-semantic-and-portability-lint) — the Error-code set the ratchet asserts through
- [conformance.md § Vendored fixtures](../../specifications/conformance.md) — provenance discipline extended to the AQL corpus
- [conformance.md § PROBE-021](../../specifications/conformance.md#probe-021--aql-parse-error-mapping) — the neighbouring pending item, recorded, unchanged
- [REQ.md](../../specifications/REQ.md) — no registry change (recorded as a decision)
