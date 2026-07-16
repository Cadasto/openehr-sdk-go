# Plan — Wire upstream web-template-tests conformance harness

**Date:** 2026-07-16
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-080](../specifications/conformance.md#conformance-scope) (openEHR wire conformance — the simplified-format slice this plan advances)
**Verifies:** [REQ-053](../specifications/wire.md#req-053) (FLAT/STRUCTURED), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (Web Template export) — exercised, not advanced
**Partially satisfies:** [REQ-082](../specifications/conformance.md#req-082--runnability) — PROBE-086 is **Sandbox-only** in v1; the Cassette/Live modes REQ-082 mandates are deferred (see Defers), so this is a documented partial, not full runnability.
**Probes:** **PROBE-086** (web-template-tests serialisation conformance adapter)
**Implementation:** planned
**Depends on:** landed FLAT/STRUCTURED codecs (REQ-053), Web Template export (REQ-106), PROBE-075 structural parity
**Defers:** Running the full Java test JAR verbatim in Go (adapter translates fixtures instead); Live/Cassette modes for PROBE-086 (Sandbox-only v1); Better-platform dialect tests

## Goal

Integrate the **[better-care/web-template-tests](https://github.com/better-care/web-template-tests)** conformance corpus (also consumed by the EHRbase `openEHR_SDK` `serialisation_conformance_test` module) into Cadasto CI as a **Go-native adapter**, catching FLAT/STRUCTURED/WebTemplate drift without requiring Java in the default `make test` path. Closes the P2 gap identified in the peer-SDK ecosystem fit-gap review; it fills the upstream-byte-conformance follow-up already named by PROBE-076 in `conformance.md`.

## Architecture

EHRbase runs the upstream JUnit suite via Maven `dependenciesToScan`. Cadasto **vendors fixture data** and reimplements assertions in Go:

```
resources/conformance/web-template-tests/   ← pinned git subtree or script-fetched JSON/YAML
        │
        ▼
testkit/conformance/webtemplate/            ← adapter: load case → run SDK codec path
        │
        ├─ WT structural checks (PROBE-075 overlap — dedupe)
        ├─ FLAT encode/decode round-trip per case
        └─ STRUCTURED interop where fixture provides both
        │
        ▼
PROBE-086 (go test ./testkit/conformance/webtemplate/...)
```

**Pin strategy:** Add `resources/conformance/web-template-tests/PIN` (commit SHA) + `scripts/sync-web-template-tests.sh` (or extend Makefile `make wt-tests-sync` / `make wt-tests-check`) — same pattern as `its-rest-sync`.

**Adapter-vs-JAR (scoping decision, not an irreversible fork):** v1 vendors fixtures and reimplements assertions in Go rather than running the upstream Java suite — a continuation of ADR 0014's vendored-fixture pattern (PROBE-075/076 already assert against vendored EHRbase reference bytes in Go). The JAR route stays open (Defers). No new ADR required; recorded here so the choice is explicit.

**Scope v1:** Subset of upstream tests that map cleanly to Go fixtures (prioritise cases already mirrored in the EHRbase `test-data/` FLAT corpus). Expand coverage incrementally; document skipped cases in `testkit/conformance/webtemplate/SKIPPED.md`.

## Definition of Ready

Implementation may start when:

- **`Covers:`** lists every REQ this plan advances (REQ-080) and separates the merely-exercised REQs (REQ-053/106) and the partial (REQ-082).
- Canonical prose exists for each: REQ-080/082 in `conformance.md`, REQ-053 in `wire.md`, REQ-106 in `clinical-modeling.md` (all landed/planned in the registry).
- The vendored-fixture-vs-JAR scoping is recorded (Architecture, above); no irreversible fork, so no ADR gate.
- **PROBE-086 is defined in `conformance.md` (status Draft) before any adapter code lands** (`conformance.md` "Adding probes" rule) — see Phase 0.
- Pin policy recorded in `resources/conformance/README.md`; vendored fixture layout chosen (prefer the EHRbase `test-data` trio structure: `operationaltemplate/`, `webtemplate/`, `composition/flat/`).

## Definition of Done

- `make wt-tests-check` fails on fixture drift.
- PROBE-086 runs under `make test` / `make ci` (Sandbox, no Docker Java).
- `traceability.yaml` maps PROBE-086 to REQ-080 (and records the REQ-053/106 coverage, REQ-082 Sandbox-partial).
- REQ.md **Impl.** column for REQ-080 reflects the advance; residual skips documented; plan archived (or **Status:** complete).
- `make spec-check` and `make ci` pass.

## Implementation checklist

| Step | Status |
|---|---|
| PROBE-086 defined in `conformance.md` (Draft) | |
| Fixtures vendored + PIN committed | |
| Adapter + runner code | |
| Tests with `// PROBE-086` comments | |
| `traceability.yaml` + REQ.md row | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Inventory, pin & probe definition

**Tasks:**

1. Audit the upstream EHRbase `openEHR_SDK` `serialisation_conformance_test/` + `test-data/` layout (source: `github.com/ehrbase/openEHR_SDK`).
2. Audit the upstream `com.github.better-care:web-template-tests` artifact version from EHRbase `bom/pom.xml` (`web-template-tests.version`).
3. Create `resources/conformance/README.md`: source repos, pin SHA, sync command, license note.
4. Add Makefile targets:
   - `wt-tests-sync` — fetch/copy fixtures into `resources/conformance/web-template-tests/`
   - `wt-tests-check` — verify tree matches PIN (checksum or git diff)
5. **Define PROBE-086 in `conformance.md`** (status Draft): preconditions (vendored fixtures at PIN), assertion (round-trip + structural parity per case), mode (Sandbox, default CI). This precedes all adapter code per the "Adding probes" rule.

**Definition of done:** fixtures vendored; PIN file committed; PROBE-086 catalogued (Draft). Verify: `make spec-check`.

### Phase 1 — Go adapter (core)

**Tasks:**

1. Create `testkit/conformance/webtemplate/`:
   - `case.go` — describe one conformance case (opt path, wt JSON, flat JSON, expected errors).
   - `runner.go` — for each case:
     1. `template.ParseFile` → `templatecompile.Compile` → `webtemplate.Build`
     2. Compare WT to golden (structural — delegate to existing PROBE-075 helpers where possible)
     3. `simplified.UnmarshalFlat` → `simplified.MarshalFlat` round-trip
   - `runner_test.go` — table-driven over vendored cases, each carrying a `// PROBE-086` comment.
2. Start with **10–20 cases** from the EHRbase `corona_anamnese` / `Test_dv_*` overlap; expand to 50+.

**Files:**

- Create: `testkit/conformance/webtemplate/*`
- Modify: `Makefile` (test target includes package)

**Definition of done:** `go test ./testkit/conformance/webtemplate/...` green on vendored subset.

### Phase 2 — PROBE-086 wiring & traceability

**Tasks:**

1. Add `testkit/probes/serialize/probe_086_web_template_tests.go` — thin wrapper calling the shared runner (explicit probe file for traceability), citing `// PROBE-086` (the `serialize` probe package is where PROBE-076 already lives).
2. Update `traceability.yaml`:
   ```yaml
   REQ-080:
     probes: [..., PROBE-086]
   ```
   (PROBE-086's catalog definition already landed in Phase 0.)

### Phase 3 — CI gate & documentation

**Tasks:**

1. Add `wt-tests-check` to `make ci` (after sync verify).
2. Document in `docs/ci.md` and the `AGENTS.md` tooling table.
3. Update `roadmap.md` conformance row: REQ-080 `planned → partial` (Sandbox slice landed).
4. Update the peer-SDK ecosystem notes with a cross-link to PROBE-086.
5. Archive plan.

## Skipped / deferred cases (initial)

Document in `SKIPPED.md` — expected for v1:

| Upstream test class | Reason |
|---|---|
| Java-specific builder tests | No generated entity layer in Go |
| Tests requiring Archie RM quirks | Go RM path differs; assert wire FLAT only |
| Exotic null-flavour combinations | Land when REQ-053 deviations close |

## Mapping to specs

- [conformance.md § REQ-080](../specifications/conformance.md#conformance-scope) — the requirement this plan advances (registry row: [REQ.md](../specifications/REQ.md))
- [conformance.md § REQ-082](../specifications/conformance.md#req-082--runnability) — runnability (Sandbox partial)
- [wire.md § REQ-053](../specifications/wire.md#req-053) — FLAT/STRUCTURED codec (exercised)
- [clinical-modeling.md § REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — Web Template export (exercised)
- [ADR 0014](../adr/0014-webtemplate-reference-implementation-lock.md) — vendored-fixture reference lock

## References

- Upstream corpus: `github.com/better-care/web-template-tests` (pin via `bom/pom.xml` `web-template-tests.version`).
- Upstream consumer: EHRbase `openEHR_SDK` `serialisation_conformance_test` module (`github.com/ehrbase/openEHR_SDK`).
- Cadasto: `openehr/serialize/simplified/roundtrip_test.go`, PROBE-075, PROBE-076.
- Motivation: peer-SDK ecosystem fit-gap review, § Wire web-template-tests.
