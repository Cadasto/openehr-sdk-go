# Plan — Probe runnability: the sandbox transport and the three-mode runner

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-082](../specifications/conformance.md#req-082--runnability) (Runnability, **Impl. `partial`**); unblocks the deferred wire-level probes named under [REQ-080](../specifications/conformance.md#req-080--openehr-wire-conformance)
**Probes:** no new `PROBE-NNN` — this plan gives the existing catalog its missing execution modes. It promotes **PROBE-077**, **PROBE-078**, **PROBE-079** out of `Status: Deferred` (each one a landed, unit-covered requirement whose dedicated wire probe is unwritten) and unblocks **PROBE-065**, which is still `Status: Draft` — specified, never implemented. It is also the gate [STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups) item 1 names for four `testkit/probes/rest/*` probes.
**Implementation:** planned
**Depends on:** landed `transport/` (REQ-090–098), the probe catalog in [conformance.md](../specifications/conformance.md), and the REQ-082 normative prose landed with this plan
**Defers:** phases 3–4 (Cassette recording, Live mode) — both need access to a live openEHR deployment this project does not currently have; [STRAND-11](../specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) (the recording format) is deliberately unresolved until there is a capture to judge

## Goal

Make the 60-probe catalog runnable the way REQ-082 requires, starting with the two modes that need nothing external. Consumers are twofold: the SDK's own CI, which today reaches each probe only through a bespoke hand-written test, and SDK **integrators**, for whom `sandbox/` is the advertised way to test an application against a fake CDR — a package that currently contains no code.

## Why this is the next conformance step

Three probes carry `Status: Deferred` with near-identical wording — *"landed and unit-covered; the dedicated wire-level probe deferred to a follow-up cycle"* (PROBE-077 / 078 / 079) — PROBE-065 sits at `Status: Draft` for want of a mode to run in, and STRAND-09 defers four more explicitly *"until the sandbox/cassette probe runner (REQ-082) is the gate."* Eight probes and a strand are queued behind one unwritten package. REQ-082 is also one of the three `v1.0.0` gates in [releases.md](../releases.md#v100-gate).

The defect this closes is not a missing feature but a missing independent witness. Every probe test today stands up its own `net/http/httptest` server and hand-writes the response it expects, so it demonstrates that the SDK handles *what we believe a CDR sends*. The belief and the assertion have the same author. Sandbox mode does not fix that on its own — it makes the fake shared and reviewable rather than duplicated 30 times — but it is the precondition for the recording that does.

## Definition of Ready

Implementation may start when:

- **`Covers:`** names REQ-082 — done; the normative prose (mode selection, probe contract, per-mode rules, cross-mode agreement) lands with this plan.
- The recording-format fork is *recorded rather than settled* — STRAND-11, gated on a live capture. Phases 1–2 do not depend on it.
- Phases below name concrete tasks and their verification command.

## Definition of Done

Scoped to phases 1–2 (what is reachable now):

- One shared probe result type with a single canonical home; the 11 byte-identical per-package copies deleted.
- A runner that executes the catalog, a subset, or one probe, in Sandbox mode, and whose summary distinguishes pass from skip.
- `sandbox/` serves every catalog probe with no network listener and no credentials, and the hand-rolled `httptest` servers in `testkit/probes/**` are gone.
- **REQ-082's refusals are each pinned by a named test**, since a fail-closed rule nobody exercises is a fail-open rule with good intentions:
  - a run whose probes all skipped exits non-zero and does **not** report green;
  - selecting a mode the invocation cannot satisfy fails loudly rather than falling back to another mode;
  - a `skip` is never counted as a pass in the summary;
  - a probe with no **Effect** declaration is treated as `mutating` (so it cannot reach a live deployment by default).
- [`traceability.yaml`](../specifications/traceability.yaml) and the REQ.md **Impl.** column reflect what landed. REQ-082 stays **`partial`** until phases 3–4 — Sandbox alone does not satisfy a three-mode requirement, and marking it `landed` early is the drift this column exists to prevent.
- `make spec-check` and `make ci` pass.

## Implementation checklist

| Step | Status |
|---|---|
| REQ-082 normative prose + STRAND-11 (this PR) | done |
| Phase 1 — shared result + runner | |
| Phase 2 — `sandbox/` transport | |
| `traceability.yaml` / REQ.md row | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Specify (this PR)

**Tasks:** REQ-082 amended with mode selection, the probe result contract, per-mode rules, and cross-mode precedence; the recording-vs-fixture artefact split stated normatively; STRAND-11 opened for the format; the `v1.0.0` gate re-pointed at the per-REQ `Impl.` axis.

**Definition of done:** `make spec-check` passes; no code changes.

### Phase 1 — One result type, one runner

**Tasks:** hoist the `Result` struct — currently defined byte-identically in 11 `testkit/probes/*` packages — into a single home, adding the `Mode` field and the `skip` status REQ-082 now requires. Add the effect declaration (read-only / mutating) per probe. Build the runner: whole catalog, named subset, or single probe; a summary that reports skips separately from passes and fails a run whose probes all skipped.

**Definition of done:** every probe reports through the shared type; `make probe-status` reflects real per-mode state rather than file existence; each refusal above fails a named test when its guard is removed; `make ci` green.

### Phase 2 — The sandbox transport

**Tasks:** implement `sandbox/` as an in-memory openEHR backend — EHR, EHR_STATUS, composition, directory, contribution, query, definition — sufficient for every probe that today constructs a bespoke fake. Migrate the probe tests onto it and delete the ~30 hand-written `httptest` handlers. Keep it importable by consumers under REQ-013 (no `auth/`, no live transport).

**Definition of done:** no `httptest.NewServer` remains in `testkit/probes/**`; `sandbox/` is documented for consumers in [quick-start.md](../quick-start.md); `make ci` green.

### Phase 3 — Cassette recording and replay *(blocked: needs a live CDR)*

**Tasks:** resolve STRAND-11 with an ADR against a real capture; implement the recorder as a `transport` wrapper and the replayer as a transport; capture the corpus once; enforce capture-time redaction and provenance per REQ-082.

**Definition of done:** every probe that can run on replay does; an unmatched request fails closed.

### Phase 4 — Live mode *(blocked: needs a live CDR)*

**Tasks:** endpoint and credential configuration; the mutating-probe opt-in gate; per-run resource scoping; promote PROBE-077 / 078 / 079 out of `Status: Deferred`, implement PROBE-065, and write the four STRAND-09 REST probes.

**Definition of done:** REQ-082 reaches `Impl. landed`; the second and third `v1.0.0` gate conditions become measurable.

## Mapping to specs

- [conformance.md § REQ-082](../specifications/conformance.md#req-082--runnability) — normative contract
- [conformance.md § REQ-080](../specifications/conformance.md#req-080--openehr-wire-conformance) — the conformance suite this unblocks
- [research-strands.md § STRAND-11](../specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) — recording format, open
- [research-strands.md § STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups) — the REST probes gated on this plan
- [REQ.md](../specifications/REQ.md) — registry row
