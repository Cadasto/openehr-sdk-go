# Plan: widen the composition corpus (the seven excluded cassettes)

**Date:** 2026-09-15
**Status:** in progress (2026-09-15). The one open question (the constraint-axis coupling) is ruled below: decoupled.
**Owner:** SDK maintainers
**Worktree:** `/src/cadasto/openehr-sdk-go/.claude/worktrees/jsonv2-fixtures`, branch `test/widen-composition-corpus`, from `7a11918e`. The main checkout is never touched.
**Covers:** [REQ-052](../../specifications/wire.md#req-052) (Canonical JSON, Impl. `landed`, no status change), exercised through [PROBE-030](../../specifications/conformance.md#probe-030--canonical-json-round-trip). No REQ id is allocated and no probe id is allocated.
**Probes:** PROBE-030 gains four `SkipFloor` hold-out entries. The PROBE-030 catalog entry already sanctions the floor-leg hold-out mechanism (`conformance.md:581` block), so no catalog wording changes and no guarded census or all-three-modes count moves.
**Implementation:** test-corpus widening. No production code path changes: only fixture discovery, one probe input list, test assertions, and prose.
**Depends on:** the landed json/v2 semantic round trip on this branch (PROBE-030 is now decode/encode/decode/encode/decode with `reflect.DeepEqual` across the second encode, `validation.ValidateRM`, and `testkit/wireequiv.Equivalent`), and ruling R33 of `docs/plans/archive/2026-09-14-json-v2-migration.md:698,708`, which withdrew the byte-stability rationale the exclusion cited.

## Goal

`testkit/fixtures/discover.go:23-32` holds seven templates with operational template and composition JSON on disk out of `ListCompositionJSON`, on a byte-stability rationale that the json/v2 migration withdrew (ruling R33). The maintainer decision for this follow-up (recorded in the Global constraints below) is that all seven join the corpus, and that an exclusion survives only for a genuine, named finding the plan cannot fix in scope, as a leg-specific hold-out carrying the finding (the PROBE-030 `SkipFloor` shape), never a blanket skip.

The evidence below shows all seven pass the fidelity legs of the round trip (typed deep comparison of the decoded values, wire-equivalence of the two SDK encodes, and the JSON to XML to JSON cross-format invariant). Three also pass the reference-model floor and join with no hold-out. Four carry a genuine finding in the vendored content that the floor reports before and after the round trip alike, so they join the input set and are held out of the `validation.ValidateRM` leg only, each named with its finding, exactly as `compositions/clinical_notes.v0.json` already is.

## How the evidence was produced

Read-only. The seven cassettes were driven through the real exported entry points (`serializeprobes.Probe030CanjsonRoundTrip`, `canjson.Unmarshal`/`Marshal`, `canxml.Marshal`/`Unmarshal`, `validation.ValidateRM`, `serializeprobes.Probe076SimplifiedRoundTrip`, `templatecompile.Compile` with `validation.ValidateComposition`) from a throwaway program outside the worktree, with a replace directive into the worktree, so no tracked file was edited and no scratch file was left inside the worktree. The current test suite for every affected package was confirmed green at `7a11918e` before any analysis (`go test ./testkit/probes/serialize/ ./openehr/serialize/canjson/ ./openehr/serialize/canxml/ ./testkit/fixtures/ ./openehr/validation/ ./testkit/probes/template/` all `ok`).

## Findings

### Who consumes the exclusion (item 1)

`ListCompositionJSON` (`discover.go:58`) is read by:

| Consumer | `file:line` | Effect when the seven join |
|---|---|---|
| PROBE-030 input build | `testkit/probes/serialize/probe_030_canjson_round_trip.go:242` (`loadCassetteInputs`) | Seven cassettes added to `Probe030Inputs`; four need `SkipFloor` (below) |
| `TestProbe030` | `testkit/probes/serialize/probes_test.go:40` | Runs every input; green once the four carry `SkipFloor` |
| `TestProbe030InputsCoverWholeCorpus` | `testkit/probes/serialize/probes_test.go:59` | `want` and `got` both rise by seven (all seven have a factory); stays balanced by design |
| `TestRoundTripCassettes` | `openehr/serialize/canjson/roundtrip_test.go:154` (list at `:19`) | Fidelity legs only; all seven pass |
| `TestCrossFormatRoundTripFromJSONCassettes` | `openehr/serialize/canxml/crossformat_test.go:46` (list at `:18`) | JSON to XML to JSON structural invariant; all seven pass |
| `findCassette` | `openehr/serialize/canjson/bare_v2_decode_test.go:58` (list at `:60`) | Looks up `BMI.json` only; unaffected |
| `TestListCompositionJSON_excludesRobotEHRStatusInvalid` | `testkit/fixtures/discover_test.go:9` | Only checks `rm/` stems; unaffected |

`compositionJSONExcluded` (`discover.go:23`) is read at three code sites, not one:

| Site | `file:line` | Meaning today | After |
|---|---|---|---|
| `ListCompositionJSON` | `discover.go:83` | keep the seven out of the round-trip corpus | removed; the seven join |
| `TemplateIDsWithCompositionXML` | `discover.go:143` | keep them out of the vendor-XML pairing | removed; only `Address.v2` newly qualifies (has XML, not in `compositionXMLExcluded`), and it decodes cleanly |
| `ConstraintTemplateIDs` | `constraint_templates.go:29` | keep them out of the constraint-cassette tests | see the open question |

`ConstraintTemplateIDs` (`constraint_templates.go:14`) is itself read by four tests: `testkit/probes/serialize/probes_test.go:170` (PROBE-076), `testkit/probes/template/constraint_cassettes_test.go:17`, `openehr/validation/constraint_cassettes_test.go:19`, and `testkit/fixtures/constraint_templates_test.go:11`. The last one asserts at `:20-22` that `Test_dv_interval_*` templates MUST NOT appear in `ConstraintTemplateIDs`. That is the coupling the open question is about.

`rmJSONExcluded` and `rmJSONExcludedPrefixes` (`discover.go:35-43`) are read only through `excludedRMJSONStem` in `ListCompositionJSON`. They are not round-trip hold-outs: they name deliberately invalid `ehr_status_invalid_*` API-validation payloads and the `ehr_status_valid_000_ehr_status_ecis` alternate-wire sample, pinned present-and-excluded by `discover_test.go:14-25`. They stay, with that stated reason (item 4).

The other repository references to the seven names (`openehr/validation/noncomposition_test.go:88,116,149`, `testkit/probes/validation/probe_074_test.go:75,82`, `openehr/composition/typecheck_test.go:20`, `openehr/template/slot_assertion_test.go:63`, `openehr/template/webtemplate/build_test.go:93`, `openehr/instance/polymorphic_roundtrip_test.go:36`, `testkit/probes/instance/probes_test.go:102`, `openehr/serialize/simplified/datatypes.go:312`) read the files directly by name through `fixtures.CompositionJSON` / `TemplateOpt`, never through `ListCompositionJSON` or `ConstraintTemplateIDs`, so the exclusion change does not reach them.

### Per cassette (item 2)

Legs: fidelity is decode, encode `b1`, decode `A`, encode `b2`, decode `B`, then `reflect.DeepEqual(A, B)` plus `wireequiv.Equivalent(b1, b2)`; cross-format is JSON to struct to XML to struct to JSON with structural equality; floor is `validation.ValidateRM` on the decoded value. All three fidelity outcomes below were confirmed for every cassette.

| Cassette (root) | Fidelity | Cross-format | RM floor | Cause of any floor failure | Verdict |
|---|---|---|---|---|---|
| `Address.v2` (ADDRESS) | pass | pass | OK | none | joins clean, no hold-out |
| `Test_dv_interval_dv_count_lower_upper_constraint.v0` (COMPOSITION) | pass | pass | OK | none | joins clean, no hold-out |
| `Test_dv_interval_dv_quantity_lower_upper_constraint.v0` (COMPOSITION) | pass | pass | OK | none | joins clean, no hold-out |
| `Demonstration.v1` (COMPOSITION) | pass | pass | fail (7 issues) | vendored content: seven inverted `DV_INTERVAL<DV_QUANTITY>` bounds, lower greater than upper (for example lower 30, upper 12.25 cm at `/content[0]/data/events[0]/data/items[1]/items[4]/value`), caught by the floor invariant at `openehr/validation/rmfloor.go:396` | joins, `SkipFloor` |
| `TestPerson.v2` (PERSON) | pass | pass | fail (2 issues) | vendored content: `DV_MULTIMEDIA.media_type` is a `CODE_PHRASE` with `code_string: null` at `/details/items[13]/items[5]/value/media_type`, so the required-attribute walk reports `code_string` absent and `checkCodePhrase` (`rmfloor.go:293`) reports the non-empty invariant | joins, `SkipFloor` |
| `Test_dv_interval_dv_count_open_constraint.v0` (COMPOSITION) | pass | pass | fail (1 issue) | vendored content: inverted `DV_INTERVAL<DV_COUNT>` bounds, lower 200, upper 100, at `/content[0]/data/events[0]/data/items[0]/value` (`rmfloor.go:396`) | joins, `SkipFloor` |
| `Test_dv_interval_dv_quantity_open_constraint.v0` (COMPOSITION) | pass | pass | fail (1 issue) | vendored content: inverted `DV_INTERVAL<DV_QUANTITY>` bounds, lower 200, upper 100 mm, at `/content[0]/data/events[0]/data/items[0]/value` (`rmfloor.go:396`) | joins, `SkipFloor` |

Every floor failure is a finding in the vendored upstream content, not an SDK defect and not a test assumption. It is present when the floor runs on the input decode and on the re-encoded value alike, so it is invariant to the round trip: the exact `SkipFloor` doctrine at `probe_030_canjson_round_trip.go:213-226`. None of the seven exposes a codec gap, a validation-reader defect, or a template-compile gap (item 3): the codec round-trips all seven with fidelity, and the floor is reading the vendored data correctly.

### Root causes worth fixing in scope (item 3)

None. Every failing leg is a genuine vendored-content finding, and vendored content is not edited. The floor invariants that report them (`rmfloor.go:396` for the ordered-interval bound, `rmfloor.go:293` and the required-attribute walk for the empty `CODE_PHRASE.code_string`) are behaving as specified for REQ-112. The correct handling is the named floor-leg hold-out, not a code change.

### Spec and docs (item 4)

- `make spec-check` guarded counts (probe census, all-three-modes tally, runnable examples per `scripts/spec-check.sh:307-373`) do not involve the composition corpus size, so none move.
- The PROBE-030 catalog entry (`conformance.md:581` block) already carries the floor-leg hold-out clause ("An input whose vendored content carries an RM-floor finding independent of the round trip MAY be held out of the ValidateRM leg only, named in the probe with the finding"), so its wording does not change.
- `docs/specifications/conformance.md:148` names `compositionJSONExcluded` as a mechanism to keep templates out of PROBE-030 discovery. That map is deleted by this plan, so the sentence goes stale and is edited to drop it (keeping `compositionXMLExcluded` and `rmJSONExcluded`). This is prose accuracy, not a spec-check gate.
- `traceability.yaml` REQ-052 (`:135`) lists tests and probes by file path (`:145-166`). The seven cassettes are data, not new test files, and no listed file is added or removed, so the REQ-052 row does not change. No other REQ row is touched.
- `docs/specifications/wire.md` REQ-052 has no corpus-size or excluded-cassette count to change (line 143 only names `compositions/BMI.json` as an example; unaffected).

### Benchmarks (item 5)

No benchmark iterates `ListCompositionJSON`, so no benchmark denominator changes. `BenchmarkDecodeCompositionCassette` reads `Demonstration.v1` by name via `fixtures.CompositionJSON(benchCassette)` (`bench_test.go:118,120`), independent of the corpus list. Its comment at `bench_test.go:127-128` claims the cassette "is held out of the round-trip probes (its DV_MULTIMEDIA content is not byte-stable through the profile)". After this plan the cassette is in the round-trip probes, on its fidelity legs, and is held out of the floor leg only, for its inverted `DV_INTERVAL` bounds, not for `DV_MULTIMEDIA`. The comment is reworded to match (Task 2).

## Open question, ruled

**Ruling F5 (controller, 2026-09-15):** decouple. The maintainer decision covers the round-trip corpus; the constraint-cassette axis keeps its own test-pinned exclusion of the four interval templates, and enrolling them there (with the two positive `primitive_out_of_range` assertions it needs) is a named follow-up in the Definition of done, not part of this plan. Cost if wrong: one small later plan.

`compositionJSONExcluded` does double duty: it gates the round-trip corpus (`ListCompositionJSON`) and, incidentally, the constraint-cassette axis (`ConstraintTemplateIDs`, `constraint_templates.go:29`). The maintainer decision is about the round-trip corpus. The constraint axis has its own pinned decision: `constraint_templates_test.go:20-22` asserts `Test_dv_interval_*` templates stay out of `ConstraintTemplateIDs`.

If the four interval templates were also enrolled into the constraint axis, `TestValidateComposition_ConstraintCassettes_NoPrimitiveViolations` (`openehr/validation/constraint_cassettes_test.go:18`) would go red: the two `lower_upper` instances genuinely violate their operational template range `[0..100]` (lower `-10` and upper `200`), confirmed as two `primitive_out_of_range` issues each. Handling that faithfully means flipping the `constraint_templates_test.go:20` assertion and adding two positive-violation assertions in `constraint_cassettes_test.go`, mirroring the three cassettes already handled there (`:30-44`). PROBE-076 passes on all four interval templates (43 to 49 FLAT keys round-tripped), so the FLAT axis is not the blocker.

**Question:** should this plan keep the constraint axis exactly as it is today (the recommended decoupling: the four interval templates stay out of `ConstraintTemplateIDs`, `constraint_templates_test.go` stays green unchanged), or also sweep the constraint axis (enroll the four, flip the pinned assertion, add two positive-violation assertions)?

**Recommendation:** decouple, and keep the constraint axis unchanged. The brief targets the round-trip corpus; the constraint-axis exclusion of interval templates is a separate, test-pinned decision; enrolling them adds scope not asked for. If the controller wants the axis swept, it is a cleanly scoped follow-up (new positive assertions for two genuine violations). The tasks below assume the recommendation. If the controller rules the other way, Task 2 keeps the corpus change and Task 3 (a new task) does the constraint-axis enrollment.

## Global constraints

Binding constraints: plain English, no em dashes, no second person, no bare `#N` ordinals; Conventional Commits with a scope and a `why` body; the trailer `Assisted-by: Claude Code (<model id>)` with the implementer's own model id; commit with explicit pathspecs, no `git add -A`, no `git stash`, no branch switching, no touching other worktrees; every task ends green on its named gate; a guard test names the mutation that turns it red. This plan changes no generator output, so `make codegen` is not involved.

## Tasks

### Task 1: correct the conformance prose that names the deleted map

- [ ] Edit `docs/specifications/conformance.md:148`: in the sentence "Templates with JSON or XML on disk but known codec gaps MAY be listed in `compositionJSONExcluded`, `compositionXMLExcluded`, or `rmJSONExcluded` in that package so probes stay green while the files remain available for template and validation work", drop `compositionJSONExcluded` (the round-trip corpus no longer excludes any template), leaving `compositionXMLExcluded` (canxml pairing) and `rmJSONExcluded` (deliberately invalid or alternate-wire rm samples). Keep the sentence otherwise intact and free of em dashes.
- [ ] Run `make spec-check`. Expected: `spec-check: OK` (the guarded counts do not involve this sentence; the run confirms no regression).
- [ ] Commit, explicit pathspec `docs/specifications/conformance.md`:

  ```
  docs(conformance): drop compositionJSONExcluded from the discovery note

  The seven-cassette round-trip exclusion is removed; only the XML pairing
  and the rm-sample exclusions remain, so the discovery note must not name a
  map that no longer gates PROBE-030 input discovery.

  Assisted-by: Claude Code (<model id>)
  ```

### Task 2: all seven join the corpus; four are held out of the floor leg only

This is the core change. The corpus widening and the four floor-leg hold-outs land together, because widening without the hold-outs leaves `TestProbe030` red for the four floor-failing cassettes, and every task must end green.

**Test first (the corpus-membership guard).**

- [ ] In `testkit/fixtures/discover_test.go`, add a test that fails while the seven are excluded and passes once they are listed. It is a can-fail control: deleting any one stem from the corpus turns it red.

  ```go
  // TestListCompositionJSON_includesFormerlyExcludedCompositions pins that the
  // seven cassettes once held out on the withdrawn byte-stability rationale
  // (ruling R33) are now in the corpus. Dropping any one from ListCompositionJSON
  // turns this red.
  func TestListCompositionJSON_includesFormerlyExcludedCompositions(t *testing.T) {
  	rels, err := fixtures.ListCompositionJSON()
  	if err != nil {
  		t.Fatal(err)
  	}
  	got := map[string]bool{}
  	for _, rel := range rels {
  		got[rel.Template] = true
  	}
  	for _, stem := range []string{
  		"Address.v2",
  		"Demonstration.v1",
  		"TestPerson.v2",
  		"Test_dv_interval_dv_count_lower_upper_constraint.v0",
  		"Test_dv_interval_dv_count_open_constraint.v0",
  		"Test_dv_interval_dv_quantity_lower_upper_constraint.v0",
  		"Test_dv_interval_dv_quantity_open_constraint.v0",
  	} {
  		if !got[stem] {
  			t.Errorf("composition %q missing from ListCompositionJSON; it must be in the corpus", stem)
  		}
  	}
  }
  ```

- [ ] Run `go test ./testkit/fixtures/ -run TestListCompositionJSON_includesFormerlyExcludedCompositions`. Expected red: the seven are still excluded, so every stem reports missing.

**Decouple discovery from the shared map.**

- [ ] In `testkit/fixtures/discover.go`, delete the `compositionJSONExcluded` map (`:21-32`) and both reads of it: the `kind == "compositions"` branch in `collectJSON` (`:83-85`) and the guard in `TemplateIDsWithCompositionXML` (`:143-145`). Leave `compositionXMLExcluded`, `rmJSONExcluded`, `rmJSONExcludedPrefixes`, and `excludedRMJSONStem` unchanged.
- [ ] In `testkit/fixtures/constraint_templates.go`, remove the `if compositionJSONExcluded[id] { continue }` guard (`:29-31`) and keep the interval templates out of the constraint axis with a dedicated, documented rule in `isConstraintTemplateID` (`:42-44`):

  ```go
  func isConstraintTemplateID(id string) bool {
  	// Test_dv_interval_* templates carry deliberately out-of-range and
  	// inverted-bound instances (REQ-052 round-trip inputs), not primitive-
  	// constraint conformance inputs, so they stay out of the constraint axis;
  	// constraint_templates_test.go pins that. Their JSON round trip is covered
  	// by PROBE-030 via ListCompositionJSON.
  	if strings.HasPrefix(id, "Test_dv_interval_") {
  		return false
  	}
  	return id == "clinical_content_validation" || strings.HasPrefix(id, "Test_dv_")
  }
  ```

  This keeps `ConstraintTemplateIDs` returning exactly the set it returns today, so `constraint_templates_test.go` (which asserts intervals absent and at least 20 `Test_dv_` ids present) stays green as the regression guard for the decoupling.

**Hold the four floor-failing cassettes out of the floor leg only.**

- [ ] In `testkit/probes/serialize/probe_030_canjson_round_trip.go`, extend `probe030SkipFloor` (`:224-226`) to name the four, each with its finding in the comment above, in the same style as the existing `clinical_notes.v0` entry:

  ```go
  var probe030SkipFloor = map[string]bool{
  	"compositions/clinical_notes.v0.json": true,
  	// Demonstration.v1: seven DV_INTERVAL<DV_QUANTITY> bounds are inverted in
  	// the vendored content (lower greater than upper, for example 30 over
  	// 12.25 cm), which the RM floor reports on the input decode and the
  	// re-encoded value alike (REQ-112). Fidelity legs still run.
  	"compositions/Demonstration.v1.json": true,
  	// TestPerson.v2: DV_MULTIMEDIA.media_type is a CODE_PHRASE with a null
  	// code_string in the vendored content, an RM-required non-empty attribute
  	// the floor reports independent of the round trip (REQ-112).
  	"compositions/TestPerson.v2.json": true,
  	// Test_dv_interval_dv_count_open_constraint.v0: a DV_INTERVAL<DV_COUNT>
  	// with inverted bounds (lower 200, upper 100) in the vendored content.
  	"compositions/Test_dv_interval_dv_count_open_constraint.v0.json": true,
  	// Test_dv_interval_dv_quantity_open_constraint.v0: a DV_INTERVAL<DV_QUANTITY>
  	// with inverted bounds (lower 200, upper 100 mm) in the vendored content.
  	"compositions/Test_dv_interval_dv_quantity_open_constraint.v0.json": true,
  }
  ```

**Test the hold-outs are load-bearing (the discriminating control).**

- [ ] In `testkit/probes/serialize/probes_test.go`, add a test that proves each held-out input genuinely fails the floor and passes the fidelity legs. It reads `SkipFloor` off `Probe030Inputs` (exported), so it needs no access to the unexported map. The mutation it catches: adding a `SkipFloor` entry for a cassette that actually passes the floor (a hold-out masking nothing, or hiding a real regression) turns the floor-on assertion red; a codec regression that breaks a held-out cassette's fidelity turns the second assertion red.

  ```go
  // TestProbe030HeldOutInputsFailFloorButPassFidelity pins that every SkipFloor
  // hold-out is load-bearing: with the floor leg on it fails on the RM floor
  // (REQ-112), and with the hold-out honored it passes on the fidelity legs. A
  // spurious hold-out (one whose cassette already passes the floor) turns the
  // first assertion red; a fidelity regression turns the second red.
  func TestProbe030HeldOutInputsFailFloorButPassFidelity(t *testing.T) {
  	var held int
  	for _, in := range serializeprobes.Probe030Inputs {
  		if !in.SkipFloor {
  			continue
  		}
  		held++
  		t.Run(in.Name, func(t *testing.T) {
  			floorOn, err := serializeprobes.Probe030CanjsonRoundTrip(in.Body, in.Factory)
  			if err != nil {
  				t.Fatalf("floor-on framework error: %v", err)
  			}
  			if floorOn.Status != "fail" {
  				t.Errorf("floor-on status = %q, want fail: a SkipFloor hold-out must genuinely fail the floor", floorOn.Status)
  			}
  			if !strings.Contains(floorOn.Detail, "RM floor") {
  				t.Errorf("floor-on detail = %q, want it to name the RM floor (REQ-112)", floorOn.Detail)
  			}
  			honored, err := serializeprobes.Probe030CanjsonRoundTripInput(in)
  			if err != nil {
  				t.Fatalf("hold-out framework error: %v", err)
  			}
  			if honored.Status != "pass" {
  				t.Errorf("hold-out status = %q (detail: %s), want pass on the fidelity legs", honored.Status, honored.Detail)
  			}
  		})
  	}
  	if held == 0 {
  		t.Fatal("no SkipFloor inputs found; the hold-out guard is asserting nothing")
  	}
  }
  ```

  This test imports `strings` (already imported in that file at `:6`).

**Reword the stale benchmark comment.**

- [ ] In `openehr/serialize/canjson/bench_test.go:127-128`, replace the parenthetical claim so it reads that the cassette now feeds PROBE-030's fidelity legs and is held out of the ValidateRM leg only, for its inverted DV_INTERVAL bounds. Keep the rest of the comment (the decode-and-encode-cleanly control) intact and free of em dashes. For example: "The cassette now feeds PROBE-030's fidelity legs and is held out of the ValidateRM leg only (its vendored content has inverted DV_INTERVAL bounds); it decodes and encodes cleanly, which is all this benchmark asks of it."

**Green.**

- [ ] Run, in order:
  - `go test ./testkit/fixtures/ -run TestListCompositionJSON_includesFormerlyExcludedCompositions` (now green).
  - `go test ./testkit/fixtures/` (the decoupling regression guard `TestConstraintTemplateIDs_includesTestDvAndClinical` still green).
  - `go test ./testkit/probes/serialize/` (`TestProbe030`, `TestProbe030InputsCoverWholeCorpus`, `TestProbe076`, and the new hold-out guard all green).
  - `go test ./openehr/serialize/canjson/ ./openehr/serialize/canxml/ ./openehr/validation/ ./testkit/probes/template/` (the corpus-wide round trip, cross-format, and constraint-cassette tests all green).
  - `go vet ./...` and `golangci-lint run` on the touched packages.
  Expected: all `ok`, no vet or lint finding.
- [ ] Commit, explicit pathspecs `testkit/fixtures/discover.go testkit/fixtures/discover_test.go testkit/fixtures/constraint_templates.go testkit/probes/serialize/probe_030_canjson_round_trip.go testkit/probes/serialize/probes_test.go openehr/serialize/canjson/bench_test.go`:

  ```
  test(fixtures): widen the composition corpus to the seven former hold-outs

  The byte-stability rationale for compositionJSONExcluded was withdrawn by
  the json/v2 migration (ruling R33): PROBE-030 is now a semantic round trip.
  All seven cassettes pass the fidelity legs, so they join ListCompositionJSON.
  Four carry a genuine finding in the vendored content that the RM floor reports
  before and after the round trip alike (inverted DV_INTERVAL bounds in
  Demonstration.v1 and the two open-constraint interval samples; a null
  CODE_PHRASE.code_string in TestPerson.v2 DV_MULTIMEDIA.media_type), so they
  are held out of the ValidateRM leg only via SkipFloor, each named.

  The map also gated the constraint-cassette axis, which deliberately excludes
  Test_dv_interval_* (constraint_templates_test.go). That exclusion is re-homed
  into isConstraintTemplateID so the constraint axis is unchanged while the
  round-trip corpus widens.

  A new guard pins that every SkipFloor hold-out genuinely fails the floor and
  passes the fidelity legs, and the stale Demonstration.v1 benchmark comment is
  corrected.

  Assisted-by: Claude Code (<model id>)
  ```

### Task 3: whole-tree gate before the plan is done

- [ ] Run `make ci` (fmt-check, mod-tidy-check, vet, test, lint, spec-check, and the rest). Expected: green. The race detector is main-only in CI and needs cgo locally; this change touches no shared state, so a local run without `-race` is sufficient and that fact is stated in the PR body.
- [ ] Confirm `git status` shows only the intended files across the two commits and nothing else.

## Definition of done

- All seven cassettes are in `ListCompositionJSON`; `Address.v2` and the two `lower_upper` interval cassettes carry no hold-out; `Demonstration.v1`, `TestPerson.v2`, and the two `open` interval cassettes carry a named `SkipFloor` entry.
- `TestProbe030`, `TestProbe030InputsCoverWholeCorpus`, `TestRoundTripCassettes`, `TestCrossFormatRoundTripFromJSONCassettes`, `TestProbe076`, and the two constraint-cassette tests are green, and the two new guards pass.
- `ConstraintTemplateIDs` returns the same set as before; `constraint_templates_test.go` is unchanged and green.
- `conformance.md:148` no longer names a deleted map; `make spec-check` is green; `traceability.yaml` is unchanged.
- `make ci` is green.
