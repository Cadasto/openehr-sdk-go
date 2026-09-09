# Plan — Probe runnability: the sandbox transport and the three-mode runner

**Date:** 2026-08-18
**Status:** Phases 0–2 landed (Phase 0 2026-08-18; Phases 1–2 2026-09-08); Phase 3 partial — the Cassette recorder, replayer, `cmd/probe-record` capture harness and the `ehr-create` / `ehr-lifecycle` recordings are in, the rest of the corpus is open; Phase 4 partial — the runner's Live path with `TestLiveCreateEHR`, the `TestLiveCoreSnapshot` core-EHR snapshot, the `TestLiveCompositionSnapshot` template/composition snapshot and the `TestLiveStoredQuerySnapshot` stored-AQL snapshot (PROBE-079 + PROBE-066) are in, catalog-wide Live runnability is the open work
**Owner:** SDK maintainers
**Covers:** [REQ-082](../specifications/conformance.md#req-082--runnability) (Runnability, **Impl. `partial`**); unblocks the deferred wire-level probes named under [REQ-080](../specifications/conformance.md#req-080--openehr-wire-conformance)
**Probes:** the runnability phases add no new `PROBE-NNN` — they give the existing catalog its missing execution modes. It has promoted **PROBE-078** (a Sandbox request-capture probe under `testkit/probes/query/`) and **PROBE-079** (witnessed on the Live snapshot) out of `Status: Deferred`, and **PROBE-065** out of `Status: Draft` to `Implemented (Sandbox)` under `testkit/probes/versioned/`; **PROBE-077** is still `Deferred` — a landed, unit-covered requirement whose dedicated wire probe is unwritten. Resolving [STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups) item 1 (the four `testkit/probes/rest/*` probes) did add three new catalog ids — **PROBE-102** (System `OPTIONS`), **PROBE-103** (Admin bulk-delete), **PROBE-104** (Definition `/example`) — for wire surfaces that had no probe, alongside **PROBE-062** (audit-details) and **PROBE-060** (EHR creation) promoted from `Draft`.
**Implementation:** partial
**Depends on:** landed `transport/` (REQ-090–098), the probe catalog in [conformance.md](../specifications/conformance.md), and the REQ-082 normative prose landed with this plan
**Defers:** Cassette corpus coverage beyond the two vendored recordings — `ehr-create` (`POST /ehr`) and `ehr-lifecycle` (`POST /ehr`, then `GET` and `HEAD` the created id) — which between them reach one catalog probe (Phase 3 remainder); and the full REQ-082 replay key, which today names method and resource path only. Encoding is settled ([ADR 0020](../adr/0020-cassette-recording-har.md), HAR 1.2). Live mode is no longer blocked: both CDRs are reachable locally (EHRbase `:8080`, FerroEHR `:8090`).

## Goal

Make the probe catalog (75 entries) runnable the way REQ-082 requires, starting with the two modes CI can run with nothing external at run time — Sandbox outright, Cassette by replaying a vendored recording (capturing one still needs a deployment). Consumers are twofold: the SDK's own CI, which today reaches each probe only through a bespoke hand-written test, and SDK **integrators**, for whom `sandbox/` is the advertised way to test an application against a fake CDR — a package whose in-memory backend (`backend.go`, `script.go`) now covers EHR create/get/head and scripted routes, with the remaining catalog surface still to migrate.

## Why this is the next conformance step

When this plan opened, three probes carried `Status: Deferred` with near-identical wording — *"landed and unit-covered; the dedicated wire-level probe deferred to a follow-up cycle"* (PROBE-077 / 078 / 079) — PROBE-065 sat at `Status: Draft` for want of a mode to run in, and STRAND-09 deferred four more explicitly *"until the sandbox/cassette probe runner (REQ-082) is the gate."* Eight probes and a strand were queued behind one unwritten package. As of 2026-09-09 PROBE-065 / 078 / 079 are Implemented, STRAND-09 item 1 is resolved (PROBE-062 / 102 / 103 / 104 under `testkit/probes/rest/`), and PROBE-077 alone remains Deferred. REQ-082 is also one of the three `v1.0.0` gates in [releases.md](../releases.md#v100-gate).

The defect this closes is not a missing feature but a missing independent witness. Every probe test today stands up its own `net/http/httptest` server and hand-writes the response it expects, so it demonstrates that the SDK handles *what we believe a CDR sends*. The belief and the assertion have the same author. Sandbox mode does not fix that on its own — it makes the fake shared and reviewable rather than duplicated 30 times — but it is the precondition for the recording that does.

## Definition of Ready

Implementation may start when:

- **`Covers:`** names REQ-082 — done; the normative prose (mode selection, probe contract, per-mode rules, cross-mode agreement) lands with this plan.
- The recording-format fork is *settled* — STRAND-11 Resolved by [ADR 0020](../adr/0020-cassette-recording-har.md) (HAR 1.2, judged against a live capture). Phases 1–2 never depended on it.
- REQ-082's fail-closed clauses are normative and name their refusal paths — an unsatisfiable mode fails loudly, an all-skipped run is not green, a `skip` is never a pass, an unclassified **Effect** is `mutating` — so phase 1 can pin each with a named test (the DoD lists them).
- Phases below name concrete tasks and their verification command.

## Definition of Done

Scoped to phases 1–2 (what is reachable now):

- One shared probe result type with a single canonical home; the 12 per-package `Result` types are now `type Result = probe.Result` aliases (same identity), not separate copies.
- A runner that executes the catalog, a subset, or one probe, in Sandbox mode, and whose summary distinguishes pass from skip.
- `sandbox/` serves every backend-facing catalog probe with no network listener and no credentials, and the hand-rolled `httptest` servers in `testkit/probes/**` are gone (in-repo probes reach no transport and are out of this bullet's scope).
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
| Phase 1 — shared result + runner | done — `testkit/probe`; the 12 per-package `Result` types are `type Result = probe.Result` aliases, not copies; refusals pinned by named tests; per-probe **Effect** metadata partial (5 of the 56 backend-facing entries carry it today; the 16 in-repo entries need none) |
| Phase 2 — `sandbox/` transport | partial — EHR + scripted routes; versioned / definition / demographic / transport probes off httptest. Auth/discovery httptest remain (OIDC/JWKS, not CDR) |
| Phase 4 — Live mode (local CDRs) | partial — runner Live path + `TestLiveCreateEHR` + `TestLiveCoreSnapshot` (core EHR create/get/exists/status green against **EHRbase 2.35.1**; the System capabilities probe *skips* — EHRbase does not implement the spec's `OPTIONS /` conformance operation, an accepted deployment deviation) + `TestLiveCompositionSnapshot` (per-run template upload, composition save and read-back — the SDK's canonical encoding accepted by a real CDR; passes when run, opt-in) + `TestLiveStoredQuerySnapshot` (per-run stored query: `PutStoredQuery` `{name, version}` recovered from `Location` — **PROBE-079** — then stored execution returning a typed `ResultSet` scoped to the run's own EHR — **PROBE-066**): `OPENEHR_LIVE_*` names the target, `OPENEHR_LIVE_ALLOW_MUTATING` is the separate write opt-in REQ-082 requires; both unset in CI, so the tests skip without dialing |
| Phase 3 — Cassette recording | partial — HAR 1.2 recorder/replayer + the `cmd/probe-record` capture harness (`make probe-record`) landed; `testkit/recordings/{ehr-create,ehr-lifecycle}.har` are the first corpus files; a capture is validated and replayed in memory before it is published, and unmatched replay fails closed. Remaining catalog probes still need recordings |
| `traceability.yaml` / REQ.md row | done (REQ-082 stays `partial`) |
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

### Phase 3 — Cassette recording and replay *(encoding settled: [ADR 0020](../adr/0020-cassette-recording-har.md))*

**Tasks:** resolve STRAND-11 with an ADR against a real capture (done: ADR 0020); implement the recorder as a `transport` wrapper and the replayer as a transport; capture the corpus once; enforce capture-time redaction and provenance per REQ-082.

**Landed:** `probe.Recorder` / `probe.Replayer`; the `cmd/probe-record` capture harness (`make probe-record`) drives a named scenario against a live CDR and re-validates the result before it is vendored; unmatched replay returns `ErrUnmatchedRecording` without dialling; capture-time redaction is attested and content-scanned; `testkit/recordings/ehr-create.har` replays `ehr.Create` and `ehr-lifecycle.har` the create-then-read path (`POST /ehr`, then `GET`/`HEAD` the created id).

**Definition of done:** every probe that can run on replay does; an unmatched request fails closed. The unmatched-request half is done; catalog-wide recordings are not.

### Phase 4 — Live mode *(wired, pre-release; not blocked)*

**Tasks:** endpoint and credential configuration; the mutating-probe opt-in gate; per-run resource scoping; promote PROBE-077 / 078 / 079 out of `Status: Deferred` (**PROBE-079** and **PROBE-066** witnessed on the Live snapshot; **PROBE-078** now implemented as a Sandbox request-capture probe under `testkit/probes/query/`), implement PROBE-065 (**done** — Sandbox composition-minimal round-trip under `testkit/probes/versioned/`), and write the four STRAND-09 REST probes (**done** — PROBE-062 / PROBE-102 / PROBE-103 / PROBE-104 under `testkit/probes/rest/`, plus PROBE-060 EHR-creation round-trip alongside them; [STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups) item 1 resolved).

**Definition of done:** REQ-082 reaches `Impl. landed`; the `v1.0.0` gates on the REQ-080 probe suite passing and on a reference deployment passing the live suite (REQ-082) become measurable.

## Mapping to specs

- [conformance.md § REQ-082](../specifications/conformance.md#req-082--runnability) — normative contract
- [conformance.md § REQ-080](../specifications/conformance.md#req-080--openehr-wire-conformance) — the conformance suite this unblocks
- [research-strands.md § STRAND-11](../specifications/research-strands.md#strand-11--probe-recording-format-har-or-a-purpose-built-yaml) — recording format, resolved ([ADR 0020](../adr/0020-cassette-recording-har.md))
- [research-strands.md § STRAND-09](../specifications/research-strands.md#strand-09--its-rest-conformance-follow-ups) — the REST probes gated on this plan
- [REQ.md](../specifications/REQ.md) — registry row
