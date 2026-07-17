# Plan — FLAT author linter (pre-submit path validation)

**Date:** 2026-07-16
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **REQ-115** (FLAT author linter) — proposed; canonical home to be authored at [clinical-modeling.md § REQ-115](../specifications/clinical-modeling.md#req-115--flat-author-linter) in Phase 0, next to the REQ-109 (AQL static lint) / REQ-110 (template-driven validation) tooling. Numbered in the clinical-modeling headroom (110–119), not the wire band (050–059, exhausted).
**Verifies / builds on:** landed [REQ-053](../specifications/wire.md#req-053) (FLAT codec), [REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) (Web Template export), [REQ-111](../specifications/clinical-modeling.md#req-111--public-compiled-template-bridge) (compiled-template bridge)
**Probes:** **PROBE-083** (FLAT author linter corpus)
**Implementation:** planned
**Depends on:** landed `openehr/serialize/simplified/`, `openehr/template/webtemplate/`, `openehr/templatecompile/` (REQ-106/111)
**Defers:** Better-platform FLAT dialect rules (EHRbase-only v1); editor/language-server integration; auto-fix / quick-fix suggestions; STRUCTURED author linter (FLAT first — STRUCTURED can reuse the path model in a follow-up)

## Goal

Ship a **building-block** FLAT composition linter that validates a raw FLAT map (JSON object or `path|suffix: value` lines) against a Web Template **before** decode or CDR submission. Consumers: integrators hand-authoring FLAT, CI pipelines, format-data tooling, and the synthetic seeder when emitting FLAT directly.

It mirrors the lint/issue model already landed for AQL (REQ-109) and complements landed `UnmarshalFlat` + `ValidateComposition` (the post-RM path) without replacing them. Motivation: the FLAT-validator gap identified in the peer-SDK ecosystem fit-gap review.

## Architecture

```
FLAT map (untrusted) + WebTemplate (from OPT or JSON file)
        │
        ▼
openehr/serialize/simplified/lint   ← new subpackage (REQ-013 safe)
        │  enumerate valid paths from WT nodes (reuse webtemplate tree)
        │  check: unknown keys, missing required composition-level keys
        │         (the required-field set defined once in REQ-115),
        │         malformed suffixes, orphan indices
        ▼
lint.Result { Issues[] with path, code, severity, suggestion }
```

- **Input:** `map[string]any` or `[]byte` JSON object; Web Template from `*webtemplate.WebTemplate`, compiled OPT (`templatecompile.Compile`), or WT JSON file.
- **Output:** structured issues (not Go `error` alone) — mirror the `validation.Result` severity model (as REQ-109 does).
- **No transport/auth imports** (REQ-013).
- **Platform pin:** the EHRbase tree-id prefix dialect is a normative choice that must be stated in REQ-115 and aligned with `deviations.md` (not settled only in this plan); if which dialects v1 supports is genuinely open, raise a STRAND.

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed REQ-115**:

- `Covers:` names the REQ this plan implements (REQ-115) and the landed REQs it builds on (REQ-053/106/111).
- Canonical normative prose for REQ-115 exists — a `clinical-modeling.md § REQ-115` section + a `REQ.md` registry row — authored via `sdd-specify` (Phase 0 below). Until then this DoR item is **pending**, not satisfied.
- The required-field set and the issue-code catalogue are defined **once**, in REQ-115 (not duplicated in this plan).
- Each phase names its verification command (`make spec-check`, `make ci`, `go test …`).

## Definition of Done

- `openehr/serialize/simplified/lint` landed with `// REQ-115` citations.
- `cmd/examples/lint-flat/` worked example.
- PROBE-083 passes on vendored FLAT fixtures + negative cases.
- `traceability.yaml` + the REQ.md **Impl.** column (REQ-115 `planned → landed`), `roadmap.md`, `docs/examples.md` updated.
- `make spec-check` + `make ci` green; plan archived (or **Status:** complete).

## Implementation checklist

| Step | Status |
|---|---|
| REQ-115 § + registry row (`clinical-modeling.md`, `REQ.md`) | |
| PROBE-083 defined in `conformance.md` (Draft) | |
| Linter package + code | |
| Tests with `// REQ-115` / `// PROBE-083` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Spec & registry (the specify gate)

Author the canonical contract first, so Phases 1–3 cite an existing REQ rather than inventing one.

**Tasks:**

1. Author **REQ-115** in `docs/specifications/clinical-modeling.md` (via `sdd-specify`). Draft normative surface to land there (canonical home is the spec, not this plan):
   - the linter validates path keys against Web Template leaf + structural paths;
   - paths absent from the template are flagged (error);
   - the **required composition-level FLAT key set** — the single authoritative list, e.g. `category`, `ctx/time` (context `start_time`), `ctx/setting`, and the composer — is defined here and nowhere else;
   - info-level hints for empty optional branches;
   - no CDR or canonical composition required — Web Template + FLAT only;
   - the **stable issue-code catalogue** (e.g. `flat.unknown_path`, `flat.missing_required`, `flat.malformed_suffix`) and severity rules — durable identifiers, so they live in the spec.
2. Add the `REQ.md` registry row (**Impl.:** `planned`; the spec section's stability **Status:** `Draft`).
3. Define PROBE-083 in `conformance.md` (status Draft) + add the `traceability.yaml` row.

**Definition of done:** `make spec-check` passes with the new rows.

### Phase 1 — Core linter library

**Tasks:**

1. Create `openehr/serialize/simplified/lint/`:
   - `issue.go` — `Issue`, `Severity`, the stable `Code` constants defined by REQ-115.
   - `paths.go` — walk `*webtemplate.WebTemplate` to build the valid path set (reuse node `id`, `aqlPath`, input suffixes from the REQ-106 model).
   - `lint.go` — `LintFlat(map[string]any, *webtemplate.WebTemplate, ...Option) Result`.
   - `lint_json.go` — `LintFlatJSON([]byte, *webtemplate.WebTemplate)`.
2. Options: `WithCompositionPrefix(string)` for the EHRbase tree root; `WithStrictRequired(bool)`.
3. Unit tests in `lint_test.go`:
   - Valid FLAT from `testkit/cassettes/` (reuse the `flat-roundtrip` template).
   - Unknown path → error.
   - Missing a required composition-level key → error.
   - Typos close to a valid path → optional suggestion (Levenshtein on the final segment, cap 3 suggestions).

**Files:**

- Create: `openehr/serialize/simplified/lint/{doc.go,issue.go,paths.go,lint.go,lint_test.go}`

**Definition of done:** `go test ./openehr/serialize/simplified/lint/...` green; no new non-stdlib deps.

### Phase 2 — CLI example

**Tasks:**

1. Add `cmd/examples/lint-flat/main.go`:
   - Flags: `-opt`, `-webtemplate`, `-flat` (JSON file); compile OPT → WT when `-opt` is given.
   - Exit code 1 on errors, 2 on warnings when `-strict`.
   - JSON output mode `-format json` for CI.
2. Document in `docs/examples.md` + `cmd/examples/doc.go`.

**Definition of done:** `go run ./cmd/examples/lint-flat -opt … -flat …` runs on a corpus fixture; `make ci` green.

### Phase 3 — PROBE-083 & traceability

**Tasks:**

1. Add `testkit/probes/serialize/probe_083_flat_linter.go` (Sandbox — pure, no HTTP), the package where PROBE-076 already lives:
   - Positive: known-good FLAT from the EHRbase `Test_dv_*` corpus.
   - Negative: inject an unknown path, assert the issue code.
2. Confirm the probe is wired in the `conformance.md` catalog (defined in Phase 0) + `traceability.yaml`.
3. Update `roadmap.md` and flip the REQ.md **Impl.** column for REQ-115 to `landed`.
4. Set plan **Status:** complete → archive.

**Definition of done:** PROBE-083 in `make test`; REQ-115 **Impl.** = `landed`; `make spec-check` green.

## Mapping to specs

- [clinical-modeling.md § REQ-115](../specifications/clinical-modeling.md#req-115--flat-author-linter) — the requirement this plan implements (registry row: [REQ.md](../specifications/REQ.md))
- [clinical-modeling.md § REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) — the lint/issue-model precedent this mirrors
- [wire.md § REQ-053](../specifications/wire.md#req-053) — FLAT codec (decode path; the linter is pre-decode)
- [clinical-modeling.md § REQ-106](../specifications/clinical-modeling.md#req-106--webtemplate-json-export) — path-enumeration source
- [use-cases.md § Synthetic data seeder](../specifications/use-cases.md#synthetic-data-seeder) — FLAT-emit validation

## References

- A peer Python openEHR SDK's FLAT validator (path checker + required-field checks) — the pattern this adapts; see the peer-SDK ecosystem fit-gap review for the P-priority.
- Cadasto: `openehr/serialize/simplified/`, `cmd/examples/flat-roundtrip/`, REQ-109 AQL lint (issue model).
- EHRbase corpus: `testkit/cassettes/compositions/`, the PROBE-076 fixtures.
